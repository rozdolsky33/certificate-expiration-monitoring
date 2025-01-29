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

// getCompartmentID retrieves the OCI Compartment ID associated with the current Function context.
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

// GetDaysRemaining calculates the days remaining for an endpoint until its TLS certificate expires.
func GetDaysRemaining(ctx context.Context, endpoint string) (int, error) {
	parts := strings.Split(endpoint, ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid endpoint format, expected hostname:port")
	}

	// TLS Dial with timeout
	conn, err := tls.DialWithDialer(&net.Dialer{
		Timeout: 5 * time.Second, // Timeout for connection
	}, "tcp", endpoint, &tls.Config{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to connect to '%s': %v", endpoint, err)
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return 0, fmt.Errorf("no certificate found for endpoint '%s'", endpoint)
	}
	cert := certs[0]

	// Calculate days remaining until certificate expiration
	daysRemaining := int(time.Until(cert.NotAfter).Hours() / 24)
	return daysRemaining, nil
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

// publishMetricData sends metric data to the OCI Monitoring service for the given resource.
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

func main() {
	fdk.Handle(fdk.HandlerFunc(func(ctx context.Context, in io.Reader, out io.Writer) {
		endpoints := os.Getenv("ENDPOINT")
		namespace := os.Getenv("NAMESPACE")
		metricName := os.Getenv("METRIC_NAME")

		if endpoints == "" || namespace == "" || metricName == "" {
			log.Fatalf("One or more required environment variables are missing (ENDPOINT, NAMESPACE, METRIC_NAME)")
			return
		}

		client, err := createMonitoringClient()
		if err != nil {
			log.Printf("Failed to create monitoring client: %v", err)
			return
		}

		compartmentID, err := getCompartmentID(ctx)
		if err != nil {
			log.Printf("Failed to retrieve compartment ID: %v", err)
			return
		}

		endpointList := strings.Split(endpoints, ",")
		for i, endpoint := range endpointList {
			if !strings.Contains(endpoint, ":") {
				endpointList[i] = endpoint + ":443" // Default to port 443
			}
		}

		var wg sync.WaitGroup
		results := make(chan string, len(endpointList)) // Channel to collect results

		// Process each endpoint concurrently with timeout
		for _, endpoint := range endpointList {
			wg.Add(1)
			go func(endpoint string) {
				defer wg.Done()

				// Set up timeout context for each endpoint
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				daysRemaining, err := GetDaysRemaining(ctx, endpoint)
				if err != nil {
					results <- fmt.Sprintf("Error for %s: %v", endpoint, err)
					return
				}

				// Publish the metric to Monitoring
				err = publishMetricData(client, namespace, compartmentID, metricName, endpoint, float64(daysRemaining))
				if err != nil {
					results <- fmt.Sprintf("Error publishing metric for %s: %v", endpoint, err)
					return
				}

				results <- fmt.Sprintf("Successfully processed '%s' with %d days remaining", endpoint, daysRemaining)
			}(endpoint)
		}

		// Wait for all goroutines to finish
		go func() {
			wg.Wait()
			close(results)
		}()

		// Write results to the FDK output
		for result := range results {
			log.Println(result)
			_, _ = fmt.Fprintln(out, result)
		}
	}))
}
