# Tekton Pruner Metrics Documentation

This document describes the comprehensive observability metrics exposed by the Tekton Pruner controller using OpenTelemetry. These metrics are exposed on port 9090 alongside Knative's built-in system metrics.

## Metrics Overview

The Tekton Pruner exposes metrics in the following categories:
- **Resource Processing**: Track resource processing and deletion operations
- **Performance Timing**: Monitor reconciliation and processing durations
- **State Tracking**: Current state of active and pending resources
- **Error Monitoring**: Detailed error tracking with classification
- **Resource Age Analysis**: Understanding resource lifecycle patterns

## Metrics Reference

### Resource Processing Metrics

| Metric Name | Type | Description | Labels |
|-------------|------|-------------|--------|
| `tekton_pruner_controller_resources_processed` | Counter | Total number of Tekton resources processed by the pruner | `namespace`, `resource_type`, `status` |
| `tekton_pruner_controller_resources_deleted` | Counter | Total number of Tekton resources deleted by the pruner | `namespace`, `resource_type`, `operation` |
| `tekton_pruner_controller_resources_errors` | Counter | Total number of errors encountered while processing Tekton resources | `namespace`, `resource_type`, `error_type`, `reason` |

### Performance Timing Metrics

| Metric Name | Type | Description | Labels | Buckets |
|-------------|------|-------------|--------|---------|
| `tekton_pruner_controller_reconciliation_duration` | Histogram | Time spent in reconciliation loops | `namespace`, `resource_type` | 0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0 |
| `tekton_pruner_controller_ttl_processing_duration` | Histogram | Time spent processing TTL-based pruning | `namespace`, `resource_type`, `operation` | 0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0 |
| `tekton_pruner_controller_history_processing_duration` | Histogram | Time spent processing history-based pruning | `namespace`, `resource_type`, `operation` | 0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0 |

### State Tracking Metrics

| Metric Name | Type | Description | Labels |
|-------------|------|-------------|--------|
| `tekton_pruner_controller_active_resources` | UpDownCounter | Current number of active Tekton resources being tracked | `namespace`, `resource_type` |
| `tekton_pruner_controller_pending_deletions` | UpDownCounter | Current number of resources pending deletion | `namespace`, `resource_type` |

### Resource Age Analysis Metrics

| Metric Name | Type | Description | Labels | Buckets |
|-------------|------|-------------|--------|---------|
| `tekton_pruner_controller_resource_age_at_deletion` | Histogram | Age of resources when they are deleted | `namespace`, `resource_type`, `operation` | 60, 300, 600, 1800, 3600, 7200, 14400, 28800, 86400, 172800, 345600, 604800 |

## Label Values

### Resource Types
- `pipelinerun`: Tekton PipelineRun resources
- `taskrun`: Tekton TaskRun resources

### Operations
- `ttl`: TTL-based pruning operations
- `history`: History-based pruning operations

### Status Values
- `success`: Successful operation
- `failed`: Failed operation
- `error`: Error occurred during operation

### Error Types
- `api_error`: Kubernetes API errors
- `timeout`: Timeout-related errors
- `validation`: Validation errors
- `internal`: Internal processing errors
- `not_found`: Resource not found errors
- `permission`: Permission/authorization errors

## Useful PromQL Queries

### Resource Processing Analysis

#### Overall Resource Processing Rate
```promql
# Total resources processed per second across all namespaces
rate(tekton_pruner_controller_resources_processed[5m])

# Resources processed by type
sum(rate(tekton_pruner_controller_resources_processed[5m])) by (resource_type)

# Resources processed by namespace
sum(rate(tekton_pruner_controller_resources_processed[5m])) by (namespace)
```

#### Resource Deletion Rates
```promql
# Total deletion rate across all operations
rate(tekton_pruner_controller_resources_deleted[5m])

# Deletion rate by operation type (TTL vs History)
sum(rate(tekton_pruner_controller_resources_deleted[5m])) by (operation)

# Deletion rate by resource type
sum(rate(tekton_pruner_controller_resources_deleted[5m])) by (resource_type)

# Namespace-specific deletion patterns
sum(rate(tekton_pruner_controller_resources_deleted[5m])) by (namespace, resource_type)
```

### Performance Monitoring

#### Reconciliation Performance
```promql
# Average reconciliation duration by resource type
histogram_quantile(0.95, rate(tekton_pruner_controller_reconciliation_duration_bucket[5m])) by (resource_type)

# Slow reconciliation loops (> 1 second)
histogram_quantile(0.99, rate(tekton_pruner_controller_reconciliation_duration_bucket[5m])) > 1

# Reconciliation duration trends
increase(tekton_pruner_controller_reconciliation_duration_sum[5m]) / increase(tekton_pruner_controller_reconciliation_duration_count[5m])
```

#### Processing Duration Analysis
```promql
# TTL processing performance
histogram_quantile(0.95, rate(tekton_pruner_controller_ttl_processing_duration_bucket[5m])) by (namespace)

# History processing performance
histogram_quantile(0.95, rate(tekton_pruner_controller_history_processing_duration_bucket[5m])) by (namespace)

# Compare TTL vs History processing efficiency
histogram_quantile(0.95, rate(tekton_pruner_controller_ttl_processing_duration_bucket[5m])) by (operation)
```

### Error Monitoring

#### Error Rate Analysis
```promql
# Overall error rate
rate(tekton_pruner_controller_resources_errors[5m])

# Error rate by type
sum(rate(tekton_pruner_controller_resources_errors[5m])) by (error_type)

# Critical errors requiring attention
rate(tekton_pruner_controller_resources_errors{error_type=~"api_error|permission"}[5m])

# Error ratio compared to successful operations
rate(tekton_pruner_controller_resources_errors[5m]) / rate(tekton_pruner_controller_resources_processed[5m])
```

#### Error Troubleshooting
```promql
# Errors by namespace (identify problematic namespaces)
sum(rate(tekton_pruner_controller_resources_errors[5m])) by (namespace)

# Errors by reason (identify common failure patterns)
sum(rate(tekton_pruner_controller_resources_errors[5m])) by (reason)

# Recent error spikes
increase(tekton_pruner_controller_resources_errors[1h]) > 10
```

### Resource Age and Lifecycle Analysis

#### Resource Age Patterns
```promql
# Average age of deleted resources
histogram_quantile(0.50, rate(tekton_pruner_controller_resource_age_at_deletion_bucket[5m]))

# Resources deleted within 1 hour vs older resources
sum(rate(tekton_pruner_controller_resource_age_at_deletion_bucket{le="3600"}[5m])) / sum(rate(tekton_pruner_controller_resource_age_at_deletion_count[5m]))

# Very old resources being cleaned up (> 1 week)
histogram_quantile(0.95, rate(tekton_pruner_controller_resource_age_at_deletion_bucket[5m])) > 604800
```

#### TTL vs History Cleanup Patterns
```promql
# Compare resource age at deletion between TTL and history operations
histogram_quantile(0.95, rate(tekton_pruner_controller_resource_age_at_deletion_bucket[5m])) by (operation)

# TTL effectiveness (resources deleted close to their TTL expiry)
histogram_quantile(0.50, rate(tekton_pruner_controller_resource_age_at_deletion_bucket{operation="ttl"}[5m]))
```

### State and Capacity Monitoring

#### Current Resource State
```promql
# Current active resources by type
tekton_pruner_controller_active_resources by (resource_type)

# Resources pending deletion
tekton_pruner_controller_pending_deletions

# Resource accumulation rate (if positive, resources are accumulating faster than being cleaned)
rate(tekton_pruner_controller_active_resources[5m])
```

#### Capacity Planning
```promql
# Growth trend of active resources
deriv(tekton_pruner_controller_active_resources[1h])

# Backlog of resources pending deletion
sum(tekton_pruner_controller_pending_deletions) by (namespace)

# Processing vs accumulation rate
rate(tekton_pruner_controller_resources_processed[5m]) - rate(tekton_pruner_controller_active_resources[5m])
```

## Alerting Rules Examples

### Critical Alerts
```yaml
groups:
- name: tekton-pruner-critical
  rules:
  - alert: TektonPrunerHighErrorRate
    expr: rate(tekton_pruner_controller_resources_errors[5m]) / rate(tekton_pruner_controller_resources_processed[5m]) > 0.1
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "High error rate in Tekton Pruner"
      description: "Error rate is {{ $value | humanizePercentage }} for namespace {{ $labels.namespace }}"

  - alert: TektonPrunerProcessingStalled
    expr: rate(tekton_pruner_controller_resources_processed[10m]) == 0 and tekton_pruner_controller_active_resources > 0
    for: 10m
    labels:
      severity: critical
    annotations:
      summary: "Tekton Pruner processing has stalled"
      description: "No resources processed in 10 minutes despite active resources present"
```

### Warning Alerts
```yaml
  - alert: TektonPrunerSlowReconciliation
    expr: histogram_quantile(0.95, rate(tekton_pruner_controller_reconciliation_duration_bucket[5m])) > 5
    for: 10m
    labels:
      severity: warning
    annotations:
      summary: "Slow Tekton Pruner reconciliation"
      description: "95th percentile reconciliation duration is {{ $value }}s"

  - alert: TektonPrunerResourceAccumulation
    expr: deriv(tekton_pruner_controller_active_resources[1h]) > 100
    for: 30m
    labels:
      severity: warning
    annotations:
      summary: "Resources accumulating faster than being pruned"
      description: "Resource count growing at {{ $value }} per hour in namespace {{ $labels.namespace }}"
```

## Dashboard Recommendations

### Key Panels for Monitoring Dashboard

1. **Resource Processing Overview**
   - Total resources processed (counter)
   - Resources deleted by operation type (stacked area chart)
   - Processing rate trends (line graph)

2. **Performance Metrics**
   - Reconciliation duration percentiles (heatmap)
   - TTL vs History processing duration comparison (histogram)
   - Processing efficiency trends (gauge)

3. **Error Analysis**
   - Error rate by type (pie chart)
   - Error trends over time (line graph)
   - Error distribution by namespace (bar chart)

4. **Resource Lifecycle**
   - Resource age at deletion distribution (histogram)
   - Active vs pending resources (gauge)
   - Resource accumulation trends (area chart)

5. **Operational Health**
   - Current error rate (single stat)
   - Processing rate (single stat)
   - Pending deletions count (single stat)

This metrics implementation provides comprehensive observability into the Tekton Pruner's operations, enabling effective monitoring, troubleshooting, and capacity planning.
