# Conduit Telemetry

The goal is to gain information about how our users use Conduit in the wild. This currently does not include things like logging (logrus) or metrics (Prometheus and Datadog for internal performance tests).

## Implementation

Telemetry related utilities are in this directory.

### Configurations
The `conduit.yml` config file includes a telemetry boolean, and optional fields to enter the OpenSearch URI, credentials (username/password), and Index name. When true, `telemetry.Config` is initialized with the optional fields. The GUID is generated when the pipeline is started, and saved to the pipeline's `metadata.json`. If the GUID already exists, it will be read from the file. 

### Client
The configuration and client is represented by the `telemetry.Client` interface and will use the official [Go opensearch client](https://opensearch.org/docs/latest/clients/go/) by default. Users can also define their own clients to send telemetry events. This will also have client functions to write to OpenSearch using the official Go client. Currently, the default telemetry client marshalls the marshalls the `telemetry.Event` struct into JSON before sending the event. 
