# Tekton Pruner Metrics

This document describes the metrics exposed by the Tekton Pruner controller using OpenTelemetry. Metrics are available on port 9090 at `/metrics`.

## Available Metrics

### Counters

| Metric | Description | Labels |
|--------|-------------|--------|
| `tekton_pruner_controller_resources_processed` | Total unique resources processed | `namespace`, `resource_type`, `status` |
| `tekton_pruner_controller_reconciliation_events` | Total reconciliation events | `namespace`, `resource_type`, `status` |
| `tekton_pruner_controller_resources_deleted` | Total resources deleted | `namespace`, `resource_type`, `operation` |
| `tekton_pruner_controller_resources_errors` | Total processing errors | `namespace`, `resource_type`, `error_type`, `reason` |

### Histograms

| Metric | Description | Labels |
|--------|-------------|--------|
| `tekton_pruner_controller_reconciliation_duration` | Reconciliation time (seconds) | `namespace`, `resource_type` |
| `tekton_pruner_controller_ttl_processing_duration` | TTL processing time (seconds) | `namespace`, `resource_type`, `operation` |
| `tekton_pruner_controller_history_processing_duration` | History processing time (seconds) | `namespace`, `resource_type`, `operation` |
| `tekton_pruner_controller_resource_age_at_deletion` | Resource age when deleted (seconds) | `namespace`, `resource_type`, `operation` |

### Gauges

| Metric | Description | Labels |
|--------|-------------|--------|
| `tekton_pruner_controller_active_resources` | Current active resources | `namespace`, `resource_type` |
| `tekton_pruner_controller_pending_deletions` | Resources pending deletion | `namespace`, `resource_type` |

## Label Values

- **resource_type**: `pipelinerun`, `taskrun`
- **operation**: `ttl`, `history`
- **status**: `success`, `failed`, `error`
- **error_type**: `api_error`, `timeout`, `validation`, `internal`, `not_found`, `permission`

## Useful Queries

### Processing Rate
```promql
# Resources processed per second
rate(tekton_pruner_controller_resources_processed[5m])

# Deletion rate by operation
sum(rate(tekton_pruner_controller_resources_deleted[5m])) by (operation)
```

### Performance
```promql
# 95th percentile reconciliation time
histogram_quantile(0.95, rate(tekton_pruner_controller_reconciliation_duration_bucket[5m]))

# Slow reconciliations (>5s)
histogram_quantile(0.95, rate(tekton_pruner_controller_reconciliation_duration_bucket[5m])) > 5
```

### Errors
```promql
# Error rate
rate(tekton_pruner_controller_resources_errors[5m])

# Error ratio
rate(tekton_pruner_controller_resources_errors[5m]) / rate(tekton_pruner_controller_resources_processed[5m])
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
  expr: rate(tekton_pruner_controller_resources_errors[5m]) / rate(tekton_pruner_controller_resources_processed[5m]) > 0.1
  for: 5m

- alert: TektonPrunerStalled
  expr: rate(tekton_pruner_controller_resources_processed[10m]) == 0 and tekton_pruner_controller_active_resources > 0
  for: 10m
```
