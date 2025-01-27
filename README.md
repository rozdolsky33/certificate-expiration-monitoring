# Certificate Expiration Monitoring Tool

Monitor SSL certificate expiration and publish data to Oracle Cloud Infrastructure (OCI) Monitoring.

## Overview

This tool calculates the number of days remaining until an SSL certificate expires for a specified endpoint and publishes the metric to OCI Monitoring.

## Prerequisites

To use this tool, ensure the following:

1. **OCI Tenancy**: Access to an OCI tenancy with required permissions.
2. **Dynamic Group Setup**:
   - Create a dynamic group (`CertMonitoringFunc-DG`) including your function's OCID:
     ```text
     ALL {resource.id = '<ocid1.fnfunc.oc1>'}
     ```
3. **IAM Policies**:
   - Add policies to enable the dynamic group to manage monitoring-related resources:
     ```text
     Allow dynamic-group CertMonitoringFunc-DG to manage metrics in compartment <compartment_name> 
     Allow dynamic-group CertMonitoringFunc-DG to read functions-family in compartment <compartment_name>
     ```
4. **Environment Variables**:
   - Configure the following variables for the function:
     ```bash
     ENDPOINT=<your_endpoint> # e.g., example.com:443
     NAMESPACE=<namespace>   # e.g., certificate_expiration_monitoring
     METRIC_NAME=<metric_name> # e.g., CertificateExpiryDays
     ```

## Features

- **SSL Certificate Monitoring**: Calculates the number of days until a certificate expires using the `GetDaysRemaining` function.
- **OCI Monitoring Integration**: Publishes certificate expiration metrics using OCI Monitoring.
- **Resource Principals**: Authenticates using OCI's Resource Principal for secure access to resources.

## Usage Instructions

### Local Deployment

1. Clone the repository:
   ```bash
   git clone <repository_url>
   cd <repository_directory>
   ```

2. Ensure **Go 1.23+** is installed.

### Docker Deployment

1. Build the Docker image:
   ```bash
   docker build --platform=linux/amd64 -t <region_code>.ocir.io/<namespace>/certificate-check:v0.2.2 .
   ```

2. Test the function locally:
   ```bash
   oci fn function invoke --function-id <function_ocid> --body ""
   ```

Deployment to OCI can proceed after ensuring functionality.

## Workflow

1. **Certificate Expiry Check**: The endpoint's certificate is retrieved, and the days remaining until expiration are calculated.
2. **Metrics Client Initialization**: A monitoring client is prepared using `ResourcePrincipalConfigurationProvider`.
3. **Metric Publishing**: Data is published to the specified namespace (`NAMESPACE`) in OCI Monitoring with the metric name (`METRIC_NAME`) and associated dimensions.

## Metrics in OCI

- **Metric Name**: `CertificateExpiryDays`
- **Namespace**: As specified by the `NAMESPACE` environment variable.
- **Dimension**: Includes `resourceId`, identifying the monitored endpoint.

## Debugging and Best Practices

- **Issues**:
   - Verify correct format (e.g., `hostname:port`) for `ENDPOINT`.
   - Ensure OCI policies are properly configured and propagated.
   - Check logs for any environment or permission-related errors.

- **Security**:
   - Avoid using `InsecureSkipVerify` for production. Update TLS settings accordingly.
   - Review IAM policies and ensure proper access control.