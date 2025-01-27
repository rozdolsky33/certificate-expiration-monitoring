package main

import (
	"context"
	"crypto/tls"
	"fmt"
	fdk "github.com/fnproject/fdk-go"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
	"github.com/oracle/oci-go-sdk/v65/functions"
	"github.com/oracle/oci-go-sdk/v65/monitoring"
	"io"
	"log"
	"os"
	"strings"
	"time"
)

// getCompartmentID retrieves the OCI Compartment ID associated with the current Function context.
// It uses the Resource Principal provider and the function OCID from the environment variable FN_FN_ID.
// Returns the Compartment ID as a string or an error if the retrieval fails.
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

// GetDaysRemaining calculates the number of days remaining until the TLS certificate for the given endpoint expires.
// endpoint specifies the target in the format "hostname:port".
// Returns the number of days remaining and an error if the operation fails.
func GetDaysRemaining(endpoint string) (int, error) {
	parts := strings.Split(endpoint, ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid endpoint format, expected hostname:port")
	}
	conn, err := tls.Dial("tcp", endpoint, &tls.Config{
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
	daysRemaining := int(time.Until(cert.NotAfter).Hours() / 24)
	return daysRemaining, nil
}

// createMonitoringClient initializes and returns an OCI MonitoringClient using a Resource Principal configuration provider.
func createMonitoringClient() (monitoring.MonitoringClient, error) {
	provider, err := auth.ResourcePrincipalConfigurationProvider()
	if err != nil {
		log.Printf("Resource Principal provider error: %v", err)
		return monitoring.MonitoringClient{}, fmt.Errorf("failed to create Resource Principal provider: %v", err)
	}
	region, _ := provider.Region()
	log.Printf("Using Resource Principal provider: %s", provider)
	client, err := monitoring.NewMonitoringClientWithConfigurationProvider(provider)
	if err != nil {
		return monitoring.MonitoringClient{}, fmt.Errorf("failed to create monitoring client: %v", err)
	}
	// Set the correct Monitoring endpoint for your region
	client.Host = fmt.Sprintf("https://telemetry-ingestion.%s.oraclecloud.com", region)
	log.Printf("Monitoring Client Host: %s", client.Host)
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

func main() {

	// Get configured environment variables
	endpoint := os.Getenv("ENDPOINT")
	namespace := os.Getenv("NAMESPACE")
	metricName := os.Getenv("METRIC_NAME")
	resourceID := endpoint // Can also be overridden if required

	log.Printf("Using configuration - ENDPOINT: %s, NAMESPACE: %s, METRIC_NAME: %s", endpoint, namespace, metricName)
	if endpoint == "" || namespace == "" || metricName == "" {
		log.Fatalf("One or more required environment variables are missing")
	}

	daysRemaining, err := GetDaysRemaining(endpoint)
	fmt.Printf("daysRemaining is %d\n", daysRemaining)
	if err != nil {
		fmt.Printf("Error calculating days remaining: %v\n", err)
		return
	}

	fdk.Handle(fdk.HandlerFunc(func(ctx context.Context, in io.Reader, out io.Writer) {
		compartmentID, err := getCompartmentID(ctx)
		if err != nil {
			log.Printf("Error retrieving compartment ID: %v", err)
			return
		}
		client, err := createMonitoringClient()
		if err != nil {
			log.Printf("Error creating monitoring client: %v", err)
			return
		}
		log.Printf("Monitoring client created successfully: %v", client)
		err = publishMetricData(client, namespace, compartmentID, metricName, resourceID, float64(daysRemaining))
		if err != nil {
			fmt.Printf("Error publishing metric data: %v\n", err)
			return
		}
		fmt.Printf("Successfully published metric '%s' with value: %d\n", metricName, daysRemaining)
	}))
}
