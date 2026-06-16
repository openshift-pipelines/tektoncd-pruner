# Tekton Pruner Metrics

This document describes the metrics exposed by the Tekton Pruner controller using OpenTelemetry. Metrics are available on port 9090 at `/metrics`.

The names below are the **Prometheus metric names** as they appear in
the `/metrics` endpoint. The OTel SDK automatically appends `_total`
to counters and `_seconds` to histograms with unit `s`.

## Available Metrics

### Counters

| Metric | Description | Labels |
|--------|-------------|--------|
| `tekton_pruner_controller_resources_processed_total` | Total unique resources processed | `namespace`, `resource_type`, `status` |
| `tekton_pruner_controller_reconciliation_events_total` | Total reconciliation events | `namespace`, `resource_type`, `status` |
| `tekton_pruner_controller_resources_deleted_total` | Total resources deleted | `namespace`, `resource_type`, `operation` |
| `tekton_pruner_controller_resources_errors_total` | Total processing errors | `namespace`, `resource_type`, `error_type`, `reason` |

### Histograms

| Metric | Description | Labels |
|--------|-------------|--------|
| `tekton_pruner_controller_reconciliation_duration_seconds` | Reconciliation time | `namespace`, `resource_type` |
| `tekton_pruner_controller_ttl_processing_duration_seconds` | TTL processing time | `namespace`, `resource_type`, `operation` |
| `tekton_pruner_controller_history_processing_duration_seconds` | History processing time | `namespace`, `resource_type`, `operation` |
| `tekton_pruner_controller_resource_age_at_deletion_seconds` | Resource age when deleted | `namespace`, `resource_type`, `operation` |

> **Note:** All metrics carry an `otel_scope_name` label
> (`tekton_pruner_controller`). This is informational and transparent
> to most PromQL queries.

## Label Values

- **resource_type**: `pipelinerun`, `taskrun`
- **operation**: `ttl`, `history`
- **status**: `success`, `failed`, `error`
- **error_type**: `api_error`, `timeout`, `validation`, `internal`, `not_found`, `permission`

## Useful Queries

### Processing Rate
```promql
# Resources processed per second
rate(tekton_pruner_controller_resources_processed_total[5m])

# Deletion rate by operation
sum(rate(tekton_pruner_controller_resources_deleted_total[5m])) by (operation)
```

### Performance
```promql
# 95th percentile reconciliation time
histogram_quantile(0.95, rate(tekton_pruner_controller_reconciliation_duration_seconds_bucket[5m]))

# Slow reconciliations (>5s)
histogram_quantile(0.95, rate(tekton_pruner_controller_reconciliation_duration_seconds_bucket[5m])) > 5
```

### Errors
```promql
# Error rate
rate(tekton_pruner_controller_resources_errors_total[5m])

# Error ratio
rate(tekton_pruner_controller_resources_errors_total[5m]) / rate(tekton_pruner_controller_resources_processed_total[5m])
```

### Resource State
```promql
# Current active resources
tekton_pruner_controller_active_resources

# Resources pending deletion  
tekton_pruner_controller_pending_deletions
```

## Basic Alerts

```yaml
- alert: TektonPrunerHighErrorRate
  expr: rate(tekton_pruner_controller_resources_errors_total[5m]) / rate(tekton_pruner_controller_resources_processed_total[5m]) > 0.1
  for: 5m

- alert: TektonPrunerStalled
  expr: rate(tekton_pruner_controller_resources_processed_total[10m]) == 0 and tekton_pruner_controller_active_resources > 0
  for: 10m
```

## Configuration

Metrics are configured via the `config-observability-tekton-pruner`
ConfigMap in the `tekton-pipelines` namespace. By default, Prometheus
export is enabled on port 9090.

See [config-observability.yaml](../config/config-observability.yaml)
for the full list of configuration options.

| Key | Values | Default |
|-----|--------|---------|
| `metrics-protocol` | `prometheus`, `grpc`, `http/protobuf`, `none` | `prometheus` |
| `metrics-endpoint` | OTLP endpoint (for grpc/http) | empty |
| `tracing-protocol` | `none`, `grpc`, `http/protobuf`, `stdout` | `none` |
| `tracing-endpoint` | OTLP tracing endpoint | empty |
