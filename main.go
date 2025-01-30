package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/fnproject/fdk-go"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
	"github.com/oracle/oci-go-sdk/v65/functions"
	"github.com/oracle/oci-go-sdk/v65/monitoring"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

// Max retry attempts for failed TLS operations
const maxRetries = 3

// Result represents the outcome of a TLS certificate analysis for a specific endpoint.
type Result struct {
	Endpoint      string
	DaysRemaining int
	Err           error
}

// LogError logs detailed error messages
func LogError(message string, err error) {
	log.Printf("[ERROR] %s: %v", message, err)
}

// LogInfo logs informational messages
func LogInfo(message string) {
	log.Printf("[INFO] %s", message)
}

// ExponentialBackoff provides retry logic with jitter
func ExponentialBackoff(attempt int) time.Duration {
	base := 2 << attempt     // Exponential growth
	jitter := rand.Intn(100) // Random jitter to avoid synchronization issues
	return time.Duration(base*100+jitter) * time.Millisecond
}

// GetDaysRemaining retrieves the number of days remaining before the TLS certificate expires.
func GetDaysRemaining(ctx context.Context, endpoint string) Result {
	resultChan := make(chan Result, 1)

	go func() {
		var conn *tls.Conn
		var err error

		for attempt := 0; attempt < maxRetries; attempt++ {
			conn, err = tls.DialWithDialer(&net.Dialer{
				Timeout: 10 * time.Second, // Timeout for TLS connection
			}, "tcp", endpoint, &tls.Config{
				InsecureSkipVerify: true,
			})

			if err == nil {
				break
			}

			LogError(fmt.Sprintf("Retrying connection to '%s' (attempt %d/%d)", endpoint, attempt+1, maxRetries), err)
			time.Sleep(ExponentialBackoff(attempt)) // Apply exponential backoff
		}

		if err != nil {
			resultChan <- Result{Endpoint: endpoint, Err: fmt.Errorf("failed to connect to '%s': %v", endpoint, err)}
			return
		}
		defer conn.Close()

		certs := conn.ConnectionState().PeerCertificates
		if len(certs) == 0 {
			resultChan <- Result{Endpoint: endpoint, Err: fmt.Errorf("no certificate found for endpoint '%s'", endpoint)}
			return
		}

		cert := certs[0]
		daysRemaining := int(time.Until(cert.NotAfter).Hours() / 24)
		resultChan <- Result{Endpoint: endpoint, DaysRemaining: daysRemaining}
	}()

	select {
	case <-ctx.Done():
		return Result{
			Endpoint: endpoint,
			Err:      fmt.Errorf("timeout while processing endpoint '%s'", endpoint),
		}
	case result := <-resultChan:
		return result
	}
}

// createMonitoringClient initializes and returns an OCI MonitoringClient.
func createMonitoringClient() (monitoring.MonitoringClient, error) {
	provider, err := auth.ResourcePrincipalConfigurationProvider()
	if err != nil {
		return monitoring.MonitoringClient{}, fmt.Errorf("failed to create Resource Principal provider: %v", err)
	}
	region, _ := provider.Region()
	client, err := monitoring.NewMonitoringClientWithConfigurationProvider(provider)
	if err != nil {
		return monitoring.MonitoringClient{}, fmt.Errorf("failed to create monitoring client: %v", err)
	}

	client.Host = fmt.Sprintf("https://telemetry-ingestion.%s.oraclecloud.com", region)
	return client, nil
}

// publishMetricData sends metric data to OCI Monitoring.
func publishMetricData(client monitoring.MonitoringClient, namespace, compartmentID, metricName, resourceID string, value float64) error {
	timestamp := common.SDKTime{Time: time.Now().UTC()}
	metricData := monitoring.MetricDataDetails{
		Namespace:     common.String(namespace),
		CompartmentId: common.String(compartmentID),
		Name:          common.String(metricName),
		Dimensions:    map[string]string{"resourceId": resourceID},
		Datapoints: []monitoring.Datapoint{
			{
				Timestamp: &timestamp,
				Value:     common.Float64(value),
			},
		},
	}

	request := monitoring.PostMetricDataRequest{
		PostMetricDataDetails: monitoring.PostMetricDataDetails{
			MetricData:     []monitoring.MetricDataDetails{metricData},
			BatchAtomicity: monitoring.PostMetricDataDetailsBatchAtomicityNonAtomic,
		},
	}

	response, err := client.PostMetricData(context.Background(), request)
	if err != nil {
		return fmt.Errorf("failed to post metric data: %v", err)
	}

	if response.FailedMetricsCount != nil && *response.FailedMetricsCount > 0 {
		return fmt.Errorf("encountered %d errors while posting metric data", *response.FailedMetricsCount)
	}

	return nil
}

// getCompartmentID retrieves the OCI Compartment ID.
func getCompartmentID(ctx context.Context) (string, error) {
	provider, err := auth.ResourcePrincipalConfigurationProvider()
	if err != nil {
		return "", fmt.Errorf("failed to create Resource Principal provider: %v", err)
	}
	functionsClient, err := functions.NewFunctionsManagementClientWithConfigurationProvider(provider)
	if err != nil {
		return "", fmt.Errorf("failed to create Functions Management client: %v", err)
	}

	functionOCID := os.Getenv("FN_FN_ID")
	if functionOCID == "" {
		return "", fmt.Errorf("FN_FN_ID is not set in the environment")
	}

	request := functions.GetFunctionRequest{FunctionId: &functionOCID}
	response, err := functionsClient.GetFunction(ctx, request)
	if err != nil {
		return "", fmt.Errorf("failed to get function details: %v", err)
	}

	return *response.CompartmentId, nil
}

// main function registered with FnProject's FDK framework.
func main() {
	fdk.Handle(fdk.HandlerFunc(func(ctx context.Context, in io.Reader, out io.Writer) {
		endpoints := os.Getenv("ENDPOINTS")
		namespace := os.Getenv("NAMESPACE")
		metricName := os.Getenv("METRIC_NAME")

		if endpoints == "" || namespace == "" || metricName == "" {
			log.Fatal("[ERROR] Missing required environment variables: ENDPOINTS, NAMESPACE, or METRIC_NAME")
		}

		client, err := createMonitoringClient()
		if err != nil {
			LogError("Failed to create monitoring client", err)
			return
		}

		compartmentID, err := getCompartmentID(ctx)
		if err != nil {
			LogError("Failed to retrieve compartment ID", err)
			return
		}

		endpointList := strings.Split(endpoints, ",")
		results := make(chan Result, len(endpointList))
		var wg sync.WaitGroup

		for _, endpoint := range endpointList {
			if !strings.Contains(endpoint, ":") {
				endpoint = endpoint + ":443"
			}

			wg.Add(1)
			go func(endpoint string) {
				defer wg.Done()
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				results <- GetDaysRemaining(ctx, endpoint)
			}(endpoint)
		}

		go func() {
			wg.Wait()
			close(results)
		}()

		for result := range results {
			if result.Err != nil {
				LogError(fmt.Sprintf("Failed to process endpoint: %s", result.Endpoint), result.Err)
			} else {
				LogInfo(fmt.Sprintf("Days remaining for %s: %d", result.Endpoint, result.DaysRemaining))
				if err := publishMetricData(client, namespace, compartmentID, metricName, result.Endpoint, float64(result.DaysRemaining)); err != nil {
					LogError(fmt.Sprintf("Failed to publish metric for %s", result.Endpoint), err)
				}
			}
		}
	}))
}
