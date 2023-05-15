# Conduit Telemetry

The goal is to gain information about how our users use Conduit in the wild. This currently does not include things like logging (logrus) or metrics (Prometheus and Datadog for internal performance tests).

We want to closely follow how algod reports telemetry. However, there are some divergence in terms of client software, e.g. currently using ElasticSearch, but in process of migrating to OpenSearch. We currently use the official [Go opensearch client](https://opensearch.org/docs/latest/clients/go/).

## Implementation

Telemetry related functionality will be put into the `conduit/telemetry` directory.

### Configurations

Opting into telemetry should be optional and must not contain any personally identifiable information. This can be done by adding a boolean to the `conduit.yml` file. If true, `TelemetryConfig` (which hold the OpenSearch URI, hardcoded credentials, GUID, Index name) will be initialized. The GUID is generated when the pipeline is started, and saved to the pipeline's `metadata.json`. If this field already exists, it will be read from the file.

### Client

The configuration and client is held in a struct called `TelemetryState`. This will also have client functions to write to OpenSearch using the official Go client. There is no plan to use a particular logging library as of now, and just marshalls the `TelemetryEvent` struct into JSON before sending the event.
