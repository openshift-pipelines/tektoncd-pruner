#!/bin/bash
# test-observability.sh

set -euo pipefail

echo "🧪 Testing Tekton Pruner Observability..."

# 1. Create test resources
echo "📝 Creating test TaskRuns..."
for i in {1..5}; do
kubectl apply -f - << EOF
apiVersion: tekton.dev/v1
kind: TaskRun
metadata:
  name: observability-test-$i
  namespace: default
  annotations:
    tekton.dev/ttlSecondsAfterFinished: "300"
spec:
  taskSpec:
    steps:
    - name: test
      image: ubuntu
      command: ["echo"]
      args: ["Testing observability $i"]
EOF
done

echo "⏳ Waiting for TaskRuns to complete..."
sleep 30

# 2. Check metrics endpoint availability
echo "📊 Checking metrics endpoint..."
kubectl port-forward -n tekton-pipelines svc/tekton-pruner-controller 9090:9090 &
PF_PID=$!
sleep 5

echo "🔍 Testing metrics endpoint accessibility..."
if curl -s http://localhost:9090/metrics > /dev/null; then
    echo "✅ Metrics endpoint is accessible"
else
    echo "❌ Metrics endpoint is not accessible"
    kill $PF_PID 2>/dev/null || true
    exit 1
fi

echo ""
echo "📈 KNATIVE CONTROLLER METRICS:"
echo "================================"
echo "🔍 Searching for Knative controller metrics..."

# Check for Knative controller metrics
KNATIVE_METRICS=$(curl -s http://localhost:9090/metrics | grep -E "(reconcile_count|reconcile_latency|work_queue_depth|workqueue_|client_)" | head -10)
if [ -n "$KNATIVE_METRICS" ]; then
    echo "✅ Found Knative controller metrics:"
    echo "$KNATIVE_METRICS"
    echo ""
    echo "📊 Reconcile metrics:"
    curl -s http://localhost:9090/metrics | grep "reconcile_count" | head -5
    echo ""
    echo "⏱️ Latency metrics:"
    curl -s http://localhost:9090/metrics | grep "reconcile_latency" | head -3
    echo ""
    echo "📋 Queue metrics:"
    curl -s http://localhost:9090/metrics | grep "work_queue_depth" | head -3
else
    echo "❌ No Knative controller metrics found!"
fi

echo ""
echo "🤖 TEKTONCD PRUNER OPENTELEMETRY METRICS:"
echo "=========================================="
echo "🔍 Searching for OpenTelemetry pruner metrics..."

# Check for OpenTelemetry pruner metrics
PRUNER_METRICS=$(curl -s http://localhost:9090/metrics | grep "tektoncd_pruner_" | head -10)
if [ -n "$PRUNER_METRICS" ]; then
    echo "✅ Found OpenTelemetry pruner metrics:"
    echo "$PRUNER_METRICS"
    echo ""
    echo "📊 Resource processing metrics:"
    curl -s http://localhost:9090/metrics | grep "tektoncd_pruner_resources_processed_total" | tail -1
    echo ""
    echo "🗑️ Resource deletion metrics:"
    curl -s http://localhost:9090/metrics | grep "tektoncd_pruner_resources_deleted_total" | tail -1
    echo ""
    echo "⚠️ Error metrics:"
    curl -s http://localhost:9090/metrics | grep "tektoncd_pruner_resources_errors_total" | tail -1
    echo ""
    echo "⏱️ Performance metrics:"
    curl -s http://localhost:9090/metrics | grep "tektoncd_pruner_reconciliation_duration_seconds" | head -3
else
    echo "❌ No OpenTelemetry pruner metrics found!"
fi

echo ""
echo "📊 METRICS SUMMARY:"
echo "==================="

# Count metrics
KNATIVE_COUNT=$(curl -s http://localhost:9090/metrics | grep -cE "(reconcile_count|reconcile_latency|work_queue_depth|workqueue_|client_)" || echo "0")
PRUNER_COUNT=$(curl -s http://localhost:9090/metrics | grep -c "tektoncd_pruner_" || echo "0")
TOTAL_METRICS=$(curl -s http://localhost:9090/metrics | grep -c "^# HELP" || echo "0")

echo "🎯 Knative controller metrics: $KNATIVE_COUNT"
echo "🤖 OpenTelemetry pruner metrics: $PRUNER_COUNT" 
echo "📊 Total metrics available: $TOTAL_METRICS"

if [ "$KNATIVE_COUNT" -gt 0 ] && [ "$PRUNER_COUNT" -gt 0 ]; then
    echo "✅ SUCCESS: Both metric systems are working!"
else
    echo "⚠️  WARNING: One or both metric systems may not be working properly"
    if [ "$KNATIVE_COUNT" -eq 0 ]; then
        echo "   - Knative controller metrics missing"
    fi
    if [ "$PRUNER_COUNT" -eq 0 ]; then
        echo "   - OpenTelemetry pruner metrics missing"
    fi
fi

# 3. Test error scenarios
echo ""
echo "❌ Testing error scenarios..."
kubectl apply -f - << EOF
apiVersion: tekton.dev/v1
kind: TaskRun
metadata:
  name: error-test
  namespace: default
spec:
  taskSpec:
    steps:
    - name: error
      image: ubuntu
      command: ["false"]  # This will fail
EOF

sleep 10
echo "⚠️ Error metrics after failed TaskRun:"
curl -s http://localhost:9090/metrics | grep "tektoncd_pruner_resources_errors_total" | tail -1

echo ""
echo "🔍 DEBUGGING INFORMATION:"
echo "========================="
echo "📋 Controller pod logs (last 20 lines):"
kubectl logs -n tekton-pipelines -l app=tekton-pruner-controller --tail=20 | head -20

echo ""
echo "🔧 TROUBLESHOOTING TIPS:"
echo "========================"
if [ "$KNATIVE_COUNT" -eq 0 ]; then
    echo "❌ Knative metrics missing - possible causes:"
    echo "   1. Controller not fully started"
    echo "   2. No reconciliation activity yet"
    echo "   3. Metrics initialization order issue"
    echo "   4. Check controller logs for errors"
fi

if [ "$PRUNER_COUNT" -eq 0 ]; then
    echo "❌ OpenTelemetry metrics missing - possible causes:"
    echo "   1. OpenTelemetry setup failed"
    echo "   2. Prometheus exporter not properly configured"
    echo "   3. Check for 'Failed to setup observability' in logs"
fi

echo ""
echo "🛠️ Manual verification commands:"
echo "kubectl port-forward -n tekton-pipelines svc/tekton-pruner-controller 9090:9090"
echo "curl http://localhost:9090/metrics | grep -E '(reconcile_count|tektoncd_pruner_)'"

# Cleanup
kill $PF_PID 2>/dev/null || true

echo ""
echo "✅ Testing complete!"
echo "📊 View full metrics: kubectl port-forward -n tekton-pipelines svc/tekton-pruner-controller 9090:9090" 