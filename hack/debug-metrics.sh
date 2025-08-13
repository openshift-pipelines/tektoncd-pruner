#!/bin/bash
# debug-metrics.sh - Debug Knative and OpenTelemetry metrics integration

set -euo pipefail

echo "🔍 Debugging Tekton Pruner Metrics Integration"
echo "=============================================="

# Check if controller is running
echo "1. 📋 Checking controller status..."
CONTROLLER_PODS=$(  kubectl get pods -n tekton-pipelines -l app=controller | wc -l)
if [ "$CONTROLLER_PODS" -eq 0 ]; then
    echo "❌ No controller pods found!"
    echo "   Deploy with: ko apply -f config/"
    exit 1
else
    echo "✅ Found $CONTROLLER_PODS controller pod(s)"
    kubectl get pods -n tekton-pipelines -l app=controller
fi

echo ""
echo "2. 📊 Testing metrics endpoint..."
kubectl port-forward -n tekton-pipelines svc/tekton-pruner-controller 9090:9090 &
PF_PID=$!
sleep 5

# Test endpoint accessibility
if ! curl -s http://localhost:9090/metrics > /dev/null; then
    echo "❌ Metrics endpoint not accessible"
    kill $PF_PID 2>/dev/null || true
    exit 1
fi

echo "✅ Metrics endpoint is accessible"

echo ""
echo "3. 🔍 Analyzing metrics content..."

# Save all metrics to a file for analysis
curl -s http://localhost:9090/metrics > /tmp/all_metrics.txt
TOTAL_METRICS=$(grep -c "^# HELP" /tmp/all_metrics.txt || echo "0")
echo "📊 Total metrics available: $TOTAL_METRICS"

echo ""
echo "4. 🎯 Knative Controller Metrics Analysis:"
echo "   ========================================="

# Check for Knative metrics
RECONCILE_COUNT=$(grep -c "reconcile_count" /tmp/all_metrics.txt || echo "0")
RECONCILE_LATENCY=$(grep -c "reconcile_latency" /tmp/all_metrics.txt || echo "0")
WORKQUEUE_COUNT=$(grep -c "workqueue_" /tmp/all_metrics.txt || echo "0")
CLIENT_COUNT=$(grep -c "client_" /tmp/all_metrics.txt || echo "0")

echo "   📊 reconcile_count metrics: $RECONCILE_COUNT"
echo "   ⏱️ reconcile_latency metrics: $RECONCILE_LATENCY"
echo "   📋 workqueue metrics: $WORKQUEUE_COUNT"
echo "   🔌 client metrics: $CLIENT_COUNT"

if [ "$RECONCILE_COUNT" -gt 0 ]; then
    echo "   ✅ Knative controller metrics are present"
    echo "   📋 Sample reconcile metrics:"
    grep "reconcile_count" /tmp/all_metrics.txt | head -3
else
    echo "   ❌ Knative controller metrics are MISSING"
fi

echo ""
echo "5. 🤖 OpenTelemetry Pruner Metrics Analysis:"
echo "   =========================================="

PRUNER_TOTAL=$(grep -c "tektoncd_pruner_" /tmp/all_metrics.txt || echo "0")
PROCESSED_COUNT=$(grep -c "tektoncd_pruner_resources_processed_total" /tmp/all_metrics.txt || echo "0")
DELETED_COUNT=$(grep -c "tektoncd_pruner_resources_deleted_total" /tmp/all_metrics.txt || echo "0")
ERROR_COUNT=$(grep -c "tektoncd_pruner_resources_errors_total" /tmp/all_metrics.txt || echo "0")

echo "   📊 Total pruner metrics: $PRUNER_TOTAL"
echo "   ⚙️ Processing metrics: $PROCESSED_COUNT"
echo "   🗑️ Deletion metrics: $DELETED_COUNT"
echo "   ⚠️ Error metrics: $ERROR_COUNT"

if [ "$PRUNER_TOTAL" -gt 0 ]; then
    echo "   ✅ OpenTelemetry pruner metrics are present"
    echo "   📋 Sample pruner metrics:"
    grep "tektoncd_pruner_" /tmp/all_metrics.txt | head -3
else
    echo "   ❌ OpenTelemetry pruner metrics are MISSING"
fi

echo ""
echo "6. 📋 Controller Logs Analysis:"
echo "   ============================"

echo "   🔍 Looking for observability initialization..."
if kubectl logs -n tekton-pipelines -l app=controller --tail=100 | grep -q "observability"; then
    echo "   ✅ Found observability logs:"
    kubectl logs -n tekton-pipelines -l app=controller --tail=100 | grep "observability" | tail -3
else
    echo "   ⚠️ No observability logs found"
fi

echo ""
echo "   🔍 Looking for errors..."
if kubectl logs -n tekton-pipelines -l app=controller --tail=100 | grep -i "error\|fail\|fatal"; then
    echo "   ❌ Found errors in logs"
else
    echo "   ✅ No errors found in recent logs"
fi

echo ""
echo "7. 🛠️ Recommendations:"
echo "   ==================="

# Provide specific recommendations based on findings
if [ "$RECONCILE_COUNT" -eq 0 ] && [ "$PRUNER_TOTAL" -eq 0 ]; then
    echo "   ❌ Both metric systems are missing - possible causes:"
    echo "      1. Controller not fully initialized"
    echo "      2. Metrics initialization failed"
    echo "      3. No reconciliation activity yet"
    echo ""
         echo "   🔧 Try these steps:"
     echo "      1. Check controller logs: kubectl logs -n tekton-pipelines -l app=controller"
    echo "      2. Create test resources: kubectl apply -f config/samples/"
    echo "      3. Wait 30 seconds and re-run this script"
    
elif [ "$RECONCILE_COUNT" -eq 0 ]; then
    echo "   ⚠️ Only OpenTelemetry metrics present - Knative metrics missing"
    echo "      This suggests the controller is not reconciling resources yet"
    echo ""
    echo "   🔧 Try creating some TaskRuns or PipelineRuns to trigger reconciliation"
    
elif [ "$PRUNER_TOTAL" -eq 0 ]; then
    echo "   ⚠️ Only Knative metrics present - OpenTelemetry metrics missing"
    echo "      This suggests OpenTelemetry setup failed"
    echo ""
    echo "   🔧 Check for 'Failed to setup observability' in controller logs"
    
else
    echo "   ✅ Both metric systems are working correctly!"
    echo "   📊 Knative metrics: $RECONCILE_COUNT reconcile + $WORKQUEUE_COUNT workqueue"
    echo "   🤖 OpenTelemetry metrics: $PRUNER_TOTAL pruner-specific"
fi

echo ""
echo "8. 📁 Debug Files Created:"
echo "   ======================="
echo "   📄 /tmp/all_metrics.txt - Complete metrics dump"

echo ""
echo "9. 🔗 Useful Commands:"
echo "   =================="
echo "   📊 View all metrics: curl http://localhost:9090/metrics"
echo "   🎯 Knative only: curl http://localhost:9090/metrics | grep -E '(reconcile_|workqueue_|client_)'"
echo "   🤖 OpenTelemetry only: curl http://localhost:9090/metrics | grep tektoncd_pruner_"
echo "   📋 Controller logs: kubectl logs -n tekton-pipelines -l app=controller"

# Cleanup
kill $PF_PID 2>/dev/null || true

echo ""
echo "🏁 Debug analysis complete!" 