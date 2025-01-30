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
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

// Result represents the outcome of a TLS certificate analysis for a specific endpoint.
// It includes the endpoint's address, the number of days remaining until the certificate expires, and any error encountered.
type Result struct {
	Endpoint      string
	DaysRemaining int
	Err           error
}

// GetDaysRemaining retrieves the number of days remaining until the expiration of the TLS certificate of a given endpoint.
// It performs a TLS handshake with the endpoint and calculates the difference between the current time and the certificate's expiry date.
// Returns a Result containing the endpoint, days remaining until certificate expiration, or an error if the operation fails.
func GetDaysRemaining(ctx context.Context, endpoint string) Result {
	resultChan := make(chan Result, 1)

	// Perform TLS operations in a Goroutine
	go func() {
		parts := strings.Split(endpoint, ":")
		if len(parts) != 2 {
			resultChan <- Result{Endpoint: endpoint, Err: fmt.Errorf("invalid endpoint format, expected hostname:port")}
			return
		}

		conn, err := tls.DialWithDialer(&net.Dialer{
			Timeout: 10 * time.Second, // add a TLS dial timeout
		}, "tcp", endpoint, &tls.Config{
			InsecureSkipVerify: true,
		})
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

// createMonitoringClient initializes and returns an OCI MonitoringClient using a Resource Principal configuration provider.
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

// publishMetricData sends metric data to the OCI Monitoring service using the provided MonitoringClient instance.
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

// getCompartmentID retrieves the compartment ID of the current function using OCI Resource Principal credentials.
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

	request := functions.GetFunctionRequest{
		FunctionId: &functionOCID,
	}
	response, err := functionsClient.GetFunction(ctx, request)
	if err != nil {
		return "", fmt.Errorf("failed to get function details: %v", err)
	}

	return *response.CompartmentId, nil
}

func main() {
	fdk.Handle(fdk.HandlerFunc(func(ctx context.Context, in io.Reader, out io.Writer) {
		// Read environment variables
		endpoints := os.Getenv("ENDPOINTS")
		namespace := os.Getenv("NAMESPACE")
		metricName := os.Getenv("METRIC_NAME")

		if endpoints == "" || namespace == "" || metricName == "" {
			log.Fatalf("One or more required environment variables are missing (ENDPOINT, NAMESPACE, METRIC_NAME)")
		}

		// Initialize OCI monitoring client
		client, err := createMonitoringClient()
		if err != nil {
			log.Printf("Failed to create monitoring client: %v", err)
			return
		}

		// Retrieve compartment ID (OCI context dependency)
		compartmentID, err := getCompartmentID(ctx)
		if err != nil {
			log.Printf("Failed to retrieve compartment ID: %v", err)
			return
		}

		// Split endpoints into a slice
		endpointList := strings.Split(endpoints, ",")
		results := make(chan Result, len(endpointList)) // Channel to collect results
		var wg sync.WaitGroup

		// Process each endpoint concurrently
		for _, endpoint := range endpointList {
			if !strings.Contains(endpoint, ":") {
				endpoint = endpoint + ":443" // Ensure default port 443
			}

			wg.Add(1)
			go func(endpoint string) {
				defer wg.Done()

				// Set up timeout context per endpoint
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				// Get days remaining and send the Result to the channel
				result := GetDaysRemaining(ctx, endpoint)
				results <- result
			}(endpoint)
		}

		// Close results channel after all workers finish
		go func() {
			wg.Wait()
			close(results)
		}()

		// Collect and log results
		for result := range results {
			if result.Err != nil {
				log.Printf("Failed to process endpoint: %s, Error: %v", result.Endpoint, result.Err)
				_, _ = fmt.Fprintf(out, "Failed to process endpoint: %s, Error: %v\n", result.Endpoint, result.Err)
			} else {
				log.Printf("Days remaining for %s: %d days", result.Endpoint, result.DaysRemaining)
				_, _ = fmt.Fprintf(out, "Successfully processed endpoint: %s, Days Remaining: %d\n", result.Endpoint, result.DaysRemaining)
				// Optionally publish the metric
				err = publishMetricData(client, namespace, compartmentID, metricName, result.Endpoint, float64(result.DaysRemaining))
				if err != nil {
					log.Printf("Failed to publish metric for %s: %v", result.Endpoint, err)
				}
			}
		}
	}))
}
