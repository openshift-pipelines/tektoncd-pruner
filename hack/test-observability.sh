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

# 2. Check metrics
echo "📊 Checking metrics endpoint..."
kubectl port-forward -n tekton-pipelines svc/tekton-pruner-controller 9090:9090 &
PF_PID=$!
sleep 5

echo "📈 Available metrics:"
curl -s http://localhost:9090/metrics | grep "tektoncd_pruner" | head -10

echo ""
echo "🔍 Specific metric values:"
echo "Resources processed: $(curl -s http://localhost:9090/metrics | grep "tektoncd_pruner_resources_processed_total" | tail -1)"
echo "Queue depth: $(curl -s http://localhost:9090/metrics | grep "tektoncd_pruner_current_resources_queued" | tail -1)"

# 3. Test error scenarios
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
echo "Error metrics: $(curl -s http://localhost:9090/metrics | grep "tektoncd_pruner_resources_errors_total" | tail -1)"

# Cleanup
kill $PF_PID 2>/dev/null || true

echo "✅ Testing complete!"
echo "📊 View full metrics: kubectl port-forward -n tekton-pipelines svc/tekton-pruner-controller 9090:9090" 