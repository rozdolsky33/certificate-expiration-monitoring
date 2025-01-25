# Certificate Expiration Monitoring

This document provides instructions to set up and run the `Certificate Expiration Monitoring` tool, which checks the number of days remaining until a certificate expires and publishes this data to Oracle Cloud Infrastructure (OCI) Monitoring.

## Prerequisites

1. **OCI Tenancy**: Access to an Oracle Cloud Infrastructure tenancy with appropriate permissions.
2. **Dynamic Group Setup**:
    - Create a dynamic group (`Cert-MonitoringFunc-DG`) to include your function OCID:
      ```text
      ALL {resource.type = 'fnfunc', resource.compartment.id = '<ocid1.fnfunc.oc1>'}
      ```
      
3. **IAM Policies**:
    - Add the following policies to enable your dynamic group (`Cert-MonitoringFunc-DG`) to access the necessary resources:
      ```text
      Allow dynamic-group Cert-MonitoringFunc-DG to use metrics in compartment <compartment_name> where target.metrics.namespace=certificate_expiration_monitoring
      Allow dynamic-group Cert-MonitoringFunc-DG to read metrics in compartment <compartment_name>
      Allow dynamic-group Cert-MonitoringFunc-DG to manage alarms in compartment <compartment_name>
      Allow dynamic-group Cert-MonitoringFunc-DG to manage ons-topics in compartment <compartment_name>
      Allow dynamic-group Cert-MonitoringFunc-DG to use streams in compartment <compartment_name>
      ```
      or
      ```txt
       Allow dynamic-group <dynamic-group-name> to use metrics in compartment <compartment-name>
      ```

4. **Resource Principal Example**:
   Resource principals offer credentials tied to specific OCI resources. Code running in the context of those resources may be granted the rights to act "as the resource".

   The example code in this directory can be assembled into an OCI Functions container. If that function is given the permissions to read (or use) resources in a tenancy, it may do so. As with all rights grants, this involves two steps:

    - Construct a dynamic group whose membership includes the function, for example  `MonitoringFunc-DG`:
      ```text
      ALL {resource.type = 'fnfunc', resource.compartment.id = '<ocid1.compartment1.id>'}
      ```
    - Add the rights to that dynamic group with a suitable policy, such as:
      ```text
      Allow dynamic-group Cert-MonitoringFunc-DG to manage all-resources in <example-compartment>
      
      Allow dynamic-group <dynamic-group-name> to use metrics in compartment <compartment-name>
      ```

   Once the dynamic group and policies are set, the function may then be deployed and invoked as usual.

   **References**:
    - [General overview of OCI Functions](https://docs.oracle.com/en-us/iaas/Content/Functions/Concepts/functionsconcepts.htm)
    - [Using Resource Principals from within OCI Functions](https://docs.oracle.com/en-us/iaas/Content/Functions/Tasks/functionsaccessingotherresources.htm)
    - [Resource Principals OCI Functions Go Example](https://github.com/oracle/oci-go-sdk/tree/master/example/example_resource_principal_function)

5. **OCI SDK Configuration**:
    - Ensure your OCI CLI or SDK configuration file is properly set up. The configuration file typically resides at `~/.oci/config`.

6. **Environment Variables**:
    - Create a `.env` file in the project root directory and populate it with the following variables:
      ```env
      ENDPOINT=<your_endpoint> # e.g., oracle.com:443
      COMPARTMENT_ID=<your_compartment_id>
      NAMESPACE=certificate_expiration_monitoring
      METRIC_NAME=CertificateExpiryDays
      ```

## Recent Updates

The following features have been added:

- **Certificate Expiry Check**: The `GetDaysRemaining` function calculates the SSL certificate expiry days for a given endpoint in the format `<hostname>:<port>`. It directly connects to the endpoint, retrieves the certificate, and computes the remaining days.
- **OCI Monitoring Integration**: The `createMonitoringClient` function initializes a monitoring client using OCI's `ResourcePrincipalConfigurationProvider`.
- **Automatic Metric Publishing**: The `publishMetricData` function now publishes metrics directly to the OCI Monitoring service, with specific dimensions like `resourceId`.

## Setting Up the Application

### Local Setup

1. **Clone the Repository**:
   ```bash
   git clone <repository_url>
   cd <repository_directory>
   ```

2. **Set Up Go Environment**:
    - Ensure you have Go 1.23 or later installed.
    - Run the application locally:
      ```bash
      go run main.go
      ```

### Run with Docker

1. **Build the Docker Image**:
   ```bash
   docker build -t certificate-checker .
   ```

2. **Run the Docker Container**:
   ```bash
   docker run --rm -e OCI_CONFIG_FILE=/path/to/oci/config -v /your/oci/config:/home/appuser/.oci certificate-checker
   ```

### Environment Variables

- Ensure a `.env` file is included in the application directory. This file gets automatically loaded during runtime.
- Key variables required:
    - `ENDPOINT`: The endpoint to check, e.g., `hostname:443`.
    - `COMPARTMENT_ID`: OCI Compartment OCID where metrics will be published.
    - `NAMESPACE`: Target namespace for metrics.
    - `METRIC_NAME`: Custom name for the monitored metric, default is `CertificateExpiryDays`.

## Code Workflow

1. **Environment Variables Handling**:
    - The application initializes by loading environment variables from the `.env` file using the `godotenv` package. Missing or invalid variables result in a fatal error.

2. **Certificate Expiry Check**:
    - The `GetDaysRemaining` function connects to the specified `endpoint` to retrieve the SSL certificate. It calculates and returns the number of days remaining until the certificate expires.

3. **OCI Monitoring Client**:
    - The `createMonitoringClient` function prepares the client based on the default resource principal configuration for metric publishing.

4. **Publishing Metrics**:
    - The `publishMetricData` function takes the calculated expiry days and ensures they are published to the specified namespace in OCI Monitoring. If any posting errors occur, the function returns detailed error logs.

## Expected Metric Details in OCI

- **Metric Name**: `CertificateExpiryDays`
- **Namespace**: `certificate_expiration_monitoring`
- **Dimension**:
    - **Key**: `resourceId` (to identify the specific endpoint).

## Debugging

- Common issues might be related to:
    - Incorrect endpoint format (ensure `hostname:port` format)
    - Missing or incorrect `.env` variables
    - OCI policies not properly configured or propagated
    - Resource principal misconfiguration for hosted environments

- Check application logs to locate pinpointed errors. Enable verbose logging or debug mode if applicable.

## Example Execution

When executed successfully, the application will:
1. Retrieve the SSL certificate expiry days for a given endpoint.
2. Publish a metric (e.g., `CertificateExpiryDays`) to OCI Monitoring.

Example success message:
```text
Successfully published metric 'CertificateExpiryDays' with value: 50
```
This indicates that the tool has successfully determined that the monitored endpoint certificate expires in 50 days, and the data is now available on the OCI Monitoring dashboard.

## Additional Notes

- Be cautious while enabling `InsecureSkipVerify` in TLS configuration for development.
- Ensure your policies and OCI configurations are secure and valid before deploying the tool in any production-grade environment.
