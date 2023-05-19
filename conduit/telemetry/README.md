# Conduit Telemetry

The goal is to gain information about how our users use Conduit in the wild. This currently does not include things like logging (logrus) or metrics (Prometheus and Datadog for internal performance tests).

We want to closely follow how algod reports telemetry. However, there is some divergence in the client, e.g. currently using ElasticSearch, but in process of migrating to OpenSearch. We currently use the official [Go opensearch client](https://opensearch.org/docs/latest/clients/go/).

## Implementation

Telemetry related functionality will be put into the `conduit/telemetry` directory.

### Configurations
The conduit.yml will have a new telemetry boolean. If true, `telemetry.Config` (which hold the OpenSearch URI, hardcoded credentials, GUID, Index name) will be initialized. The GUID is generated when the pipeline is started, and saved to the pipeline's `metadata.json`. If this field already exists, it will be read from the file. 
The index name in algod is set to the Chain ID (e.g. mainnet and testnet). In conduit, we consolidate to a common index name for now.

### Client
The configuration and client is represented by the `telemetry.Client` interface and will use the `OpenSearchClient` by default. Users should be able to define their own clients to send telemetry events. This will also have client functions to write to OpenSearch using the official Go client. There is no plan to use a particular logging library as of now, and just marshalls the `telemetry.Event` struct into JSON before sending the event. 
