# Certificate Expiration Monitoring

This document provides instructions to set up and run the `Certificate Expiration Monitoring` tool, which checks the number of days remaining until a certificate expires and publishes this data to Oracle Cloud Infrastructure (OCI) Monitoring.

## Prerequisites

1. **OCI Tenancy**: Access to an Oracle Cloud Infrastructure tenancy with appropriate permissions.
2. **Dynamic Group Setup**:
   - Create a dynamic group to include your instance or application:
     ```text
     ANY {instance.compartment.id = '<ocid1.tenancy.oc1..>'}
     ```

3. **IAM Policies**:
   - Add the following policies to enable your dynamic group (`cert_monitor_group`) to access the necessary resources:
     ```text
     Allow dynamic-group cert_monitor_group to use metrics in tenancy where target.metrics.namespace=certificate_expiration_monitoring
     Allow dynamic-group cert_monitor_group to read metrics in tenancy
     Allow dynamic-group cert_monitor_group to manage alarms in tenancy
     Allow dynamic-group cert_monitor_group to manage ons-topics in tenancy
     Allow dynamic-group cert_monitor_group to use streams in tenancy
     ```

4. **OCI SDK Configuration**:
   - Ensure your OCI CLI or SDK configuration file is properly set up. The configuration file typically resides at `~/.oci/config`.

5. **Environment Variables**:
   - Create a `.env` file in the project root directory and populate it with the following variables:
     ```env
     ENDPOINT=<your_endpoint> # e.g., oracle.com:443
     COMPARTMENT_ID=<your_compartment_id>
     NAMESPACE=certificate_expiration_monitoring
     METRIC_NAME=CertificateExpiryDays
     ```

## Building and Running the Application

### Local Setup

1. **Clone the Repository** (if applicable):
   ```bash
   git clone <repository_url>
   cd <repository_directory>
   ```

2. **Set Up Go Environment**:
   - Ensure you have Go installed and set up on your system.
   - Run the application locally:
     ```bash
     go run main.go
     ```

### Build Docker Image

1. **Build the Docker Image**:
   ```bash
   docker build -t certificate-checker .
   ```

2. **Run the Docker Container**:
   ```bash
   docker run --rm -e OCI_CONFIG_FILE=/path/to/oci/config -v /your/oci/config:/home/appuser/.oci certificate-checker
   ```

### Environment Variables

- Ensure the `.env` file is included in the same directory as the application, as it is loaded automatically at runtime.
- Key variables include:
   - `ENDPOINT`: The endpoint to check.
   - `COMPARTMENT_ID`: OCI Compartment OCID.
   - `NAMESPACE`: Monitoring namespace.
   - `METRIC_NAME`: Name of the metric to publish.

## Key Components of the Code

1. **Environment Variable Loading**:
   - The `init` function loads environment variables from a `.env` file using the `godotenv` package.

2. **Certificate Expiry Check**:
   - The `GetDaysRemaining` function connects to the given endpoint and retrieves the SSL certificate, calculating the number of days remaining until expiration.

3. **OCI Monitoring Client**:
   - The `createMonitoringClient` function initializes the OCI Monitoring client using the default configuration provider.

4. **Publish Metric Data**:
   - The `publishMetricData` function sends the calculated certificate expiry days to OCI Monitoring with a specific namespace and metric name.

## OCI Resources

1. **Monitoring Namespace**: `certificate_expiration_monitoring`
2. **Metric Name**: `CertificateExpiryDays`
3. **Dimensions**: Includes `resourceId`, which identifies the monitored resource (e.g., endpoint).

## Debugging and Logging

- Check logs from the application for detailed error messages if any part of the setup or execution fails.
- Ensure that your OCI policies and dynamic group configurations are correctly applied and propagated.

## Example

- For an endpoint `oracle.com:443`, the tool retrieves the certificate expiry days, publishes the metric, and displays the following message upon success:
  ```text
  Successfully published metric 'CertificateExpiryDays' with value: 50
  ```

