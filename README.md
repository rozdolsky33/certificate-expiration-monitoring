# Certificate Expiration Monitoring

This document provides instructions to set up and run the `Certificate Expiration Monitoring` tool, which checks the number of days remaining until a certificate expires and publishes this data to Oracle Cloud Infrastructure (OCI) Monitoring.

## Prerequisites

1. **OCI Tenancy**: Access to an Oracle Cloud Infrastructure tenancy with appropriate permissions.
2. **Dynamic Group Setup**:
    - Create a dynamic group (`CertMonitoringFunc-DG`) to include your function OCID:
      ```text
      ALL {resource.id = '<ocid1.fnfunc.oc1>'}
      ```
      
3. **IAM Policies**:
    - Add the following policies to enable your dynamic group (`CertMonitoringFunc-DG`) to access the necessary resources:
      ```text
      Allow dynamic-group CertMonitoringFunc-DG to manage metrics in compartment <compartment_name> 
      
      Allow dynamic-group CertMonitoringFunc-DG to read metrics in compartment <compartment_name>
      Allow dynamic-group CertMonitoringFunc-DG to manage alarms in compartment <compartment_name>
      Allow dynamic-group CertMonitoringFunc-DG to manage ons-topics in compartment <compartment_name>
      Allow dynamic-group CertMonitoringFunc-DG to use streams in compartment <compartment_name>
      
      Allow dynamic-group CertMonitoringFunc-DG to inspect tenancies in compartment <compartment_name>
      ``` 

4. **Resource Principal Example** (Redundant):
   
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

6. **Configure Environment Variables**:
    - Under Function Resorce configure environments variables for your function endpoint and monitoring setup e.g metric name and namespace 
      ```env
      ENDPOINT=<your_endpoint> # e.g., oracle.com:443
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

1. **Build the Docker Image**: (GENERIC_X86)
   ```bash
    docker build --platform linux/amd64 -t region_code.ocir.io/namespace/certificate-exparation-monitoring:v0.0.0
   e.g (docker build -t iad.ocir.io/idjgqqtt6zep/certificate-checker:v0.2.2 .)
   ```

2. **Trigger function locally**: 
   ```bash
   oci fn function invoke --function-id <ocid1.fnfunc.oc1.> --file "-" --body ""\n
   ```

## Code Workflow

1. **Certificate Expiry Check**:
    - The `GetDaysRemaining` function connects to the specified `endpoint` to retrieve the SSL certificate. It calculates and returns the number of days remaining until the certificate expires.

2. **OCI Monitoring Client**:
    - The `createMonitoringClient` function prepares the client based on the default resource principal configuration for metric publishing.

3. **Publishing Metrics**:
    - The `publishMetricData` function takes the calculated expiry days and ensures they are published to the specified namespace in OCI Monitoring. If any posting errors occur, the function returns detailed error logs.

## Expected Metric Details in OCI

- **Metric Name**: `CertificateExpiryDays`
- **Namespace**: `certificate_expiration_monitoring`
- **Dimension**:
    - **Key**: `resourceId` (to identify the specific endpoint).

## Debugging

- Common issues might be related to:
    - Incorrect endpoint format (ensure `hostname:port` format)
    - Missing or incorrect function variables configured 
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
