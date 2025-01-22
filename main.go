package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/monitoring"
)

func init() {
	// Load the environment variables from .env file
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}
}

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

func createMonitoringClient() (monitoring.MonitoringClient, error) {
	provider := common.DefaultConfigProvider()
	client, err := monitoring.NewMonitoringClientWithConfigurationProvider(provider)
	if err != nil {
		return monitoring.MonitoringClient{}, fmt.Errorf("failed to create monitoring client: %v", err)
	}

	// Set the correct telemetry endpoint for your region
	client.Host = "https://telemetry-ingestion.us-ashburn-1.oraclecloud.com"
	return client, nil
}

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
	// Load values from environment variables
	endpoint := os.Getenv("ENDPOINT")
	compartmentID := os.Getenv("COMPARTMENT_ID")
	namespace := os.Getenv("NAMESPACE")
	metricName := os.Getenv("METRIC_NAME")
	resourceID := endpoint // Can also be overridden from env if required

	if endpoint == "" || compartmentID == "" || namespace == "" || metricName == "" {
		log.Fatalf("One or more required environment variables are missing")
	}

	daysRemaining, err := GetDaysRemaining(endpoint)
	if err != nil {
		fmt.Printf("Error calculating days remaining: %v\n", err)
		return
	}

	client, err := createMonitoringClient()
	if err != nil {
		fmt.Printf("Error creating monitoring client: %v\n", err)
		return
	}

	err = publishMetricData(client, namespace, compartmentID, metricName, resourceID, float64(daysRemaining))
	if err != nil {
		fmt.Printf("Error publishing metric data: %v\n", err)
		return
	}

	fmt.Printf("Successfully published metric '%s' with value: %d\n", metricName, daysRemaining)
}
