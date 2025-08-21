#!/bin/bash
# setup-observability-simple.sh - Simple observability setup for Tekton Pruner
# This script sets up a Kind cluster with Tekton Pruner, Prometheus, and Jaeger for observability.
# Services are accessed via port-forward for simplicity.
# It assumes you have `kind`, `kubectl`, and `ko` installed and configured.

set -euo pipefail

# Configuration
: "${KO_DOCKER_REPO:=<quay.io/some-repo>}" # Replace with your own repositorys
: "${KIND_CLUSTER_NAME:=tekton-obs}"

wait_for_deploy() {
  local ns="$1"
  local name="$2"
  echo "Waiting for deployment $name in namespace $ns..."
  for i in {1..60}; do
    if kubectl -n "$ns" get deploy "$name" >/dev/null 2>&1; then
      break
    fi
    sleep 2
  done
  kubectl -n "$ns" rollout status deploy/"$name" --timeout=300s
}

setup_port_forwards() {
  echo "Setting up port forwards..."
  
  # Kill any existing port-forwards
  pkill -f "kubectl.*port-forward" || true
  sleep 2
  
  # Setup port forwards in background
  kubectl port-forward -n monitoring svc/prometheus 9091:9090 > /dev/null 2>&1 &
  kubectl port-forward -n observability-system svc/jaeger 16686:16686 > /dev/null 2>&1 &
  kubectl port-forward -n tekton-pipelines svc/tekton-pruner-controller 9090:9090 > /dev/null 2>&1 &
  
  echo "Port forwards started in background"
}

echo "Setting up observability stack..."

# Create Kind cluster configuration
cat > kind-config.yaml << EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  image: kindest/node:v1.32.0
EOF

# Create Kind cluster
echo "Creating Kind cluster..."
kind create cluster --config kind-config.yaml --name "${KIND_CLUSTER_NAME}"

# Install Tekton Pipeline
echo "Installing Tekton Pipeline..."
kubectl apply -f https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml
wait_for_deploy tekton-pipelines tekton-pipelines-controller

# Install Prometheus
echo "Installing Prometheus..."
kubectl apply -f - << EOF
apiVersion: v1
kind: Namespace
metadata:
  name: monitoring
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: prometheus-config
  namespace: monitoring
data:
  prometheus.yml: |
    global:
      scrape_interval: 15s
      metric_name_validation_scheme: legacy
    scrape_configs:
    - job_name: 'tekton-pruner'
      metric_name_escaping_scheme: underscores
      static_configs:
      - targets: ['tekton-pruner-controller.tekton-pipelines.svc.cluster.local:9090']
    - job_name: 'kubernetes-pods'
      metric_name_escaping_scheme: underscores
      kubernetes_sd_configs:
      - role: pod
        namespaces:
          names: [tekton-pipelines]
      relabel_configs:
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
        action: keep
        regex: true
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: prometheus
  namespace: monitoring
spec:
  replicas: 1
  selector:
    matchLabels:
      app: prometheus
  template:
    metadata:
      labels:
        app: prometheus
    spec:
      containers:
      - name: prometheus
        image: prom/prometheus:latest
        ports:
        - containerPort: 9090
        volumeMounts:
        - name: config
          mountPath: /etc/prometheus
        args:
        - '--config.file=/etc/prometheus/prometheus.yml'
        - '--storage.tsdb.path=/prometheus'
        - '--web.console.libraries=/etc/prometheus/console_libraries'
        - '--web.console.templates=/etc/prometheus/consoles'
      volumes:
      - name: config
        configMap:
          name: prometheus-config
---
apiVersion: v1
kind: Service
metadata:
  name: prometheus
  namespace: monitoring
spec:
  selector:
    app: prometheus
  ports:
  - port: 9090
    targetPort: 9090
  type: ClusterIP
EOF

wait_for_deploy monitoring prometheus

# Install Jaeger
echo "Installing Jaeger..."
kubectl apply -f - << EOF
apiVersion: v1
kind: Namespace
metadata:
  name: observability-system
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: jaeger
  namespace: observability-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: jaeger
  template:
    metadata:
      labels:
        app: jaeger
    spec:
      containers:
      - name: jaeger
        image: jaegertracing/all-in-one:latest
        ports:
        - containerPort: 16686
        - containerPort: 14268
        env:
        - name: COLLECTOR_OTLP_ENABLED
          value: "true"
---
apiVersion: v1
kind: Service
metadata:
  name: jaeger
  namespace: observability-system
spec:
  selector:
    app: jaeger
  ports:
  - name: ui
    port: 16686
    targetPort: 16686
  - name: collector
    port: 14268
    targetPort: 14268
  type: ClusterIP
EOF

wait_for_deploy observability-system jaeger

# Deploy observability configuration
echo "Deploying observability configuration..."
kubectl apply -f - << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: config-observability-tekton-pruner
  namespace: tekton-pipelines
data:
  metrics.backend-destination: "prometheus"
  metrics.request-metrics-backend-destination: "prometheus"
  metrics.allow-stackdriver-custom-metrics: "false"
  tracing.backend: "jaeger"
  tracing.endpoint: "http://jaeger.observability-system.svc.cluster.local:14268/api/traces"
  tracing.zipkin-endpoint: "http://jaeger.observability-system.svc.cluster.local:9411/api/v2/spans"
EOF

# Deploy Tekton Pruner (includes observability service)
echo "Building and deploying Tekton Pruner..."
export KO_DOCKER_REPO
ko apply -f config/

# Wait for deployments
echo "Waiting for all services to be ready..."
wait_for_deploy tekton-pipelines tekton-pruner-controller
wait_for_deploy monitoring prometheus
wait_for_deploy observability-system jaeger

# Setup port forwards
setup_port_forwards

echo "Setup complete!"
echo ""
echo "Access URLs (via port-forward):"
echo "  Prometheus: http://localhost:9091"
echo "  Jaeger: http://localhost:16686"
echo "  Pruner Metrics: http://localhost:9090/metrics"
echo ""
echo "Test metrics:"
echo "  curl http://localhost:9090/metrics | grep -E 'tekton_pruner_controller_'"
echo ""
echo "Troubleshooting:"
echo "  kubectl logs -n tekton-pipelines -l app.kubernetes.io/name=controller"
echo "  kubectl get endpoints -n tekton-pipelines tekton-pruner-controller"
echo "  kubectl get configmap -n tekton-pipelines config-observability-tekton-pruner -o yaml"
echo "  kubectl exec -n tekton-pipelines deployment/tekton-pruner-controller -- printenv | grep -E '(METRICS|OBSERV)'"
echo ""
echo "Note: Port forwards are running in background. To stop them:"
echo "  pkill -f 'kubectl.*port-forward'"