#!/bin/bash
# setup-observability-simple.sh - Alternative setup without Prometheus Operator

set -euo pipefail

# ==============================
# Configurable variables
# ==============================
: "${KO_DOCKER_REPO:=quay.io/rh-ee-anataraj}"        # default to ko.local for kind auto-loader
: "${KIND_CLUSTER_NAME:=tekton-obs}"  # default cluster name

wait_for_deploy() {
  local ns="$1"
  local name="$2"
  echo "⏳ Waiting for deployment $name in namespace $ns to appear..."
  for i in {1..120}; do
    if kubectl -n "$ns" get deploy "$name" >/dev/null 2>&1; then
      break
    fi
    sleep 2
  done
  echo "⏳ Waiting for deployment $name rollout..."
  kubectl -n "$ns" rollout status deploy/"$name" --timeout=600s
}

echo "🚀 Setting up Tekton Pruner with simple observability stack..."

# 1. Create Kind cluster with port mappings (Kubernetes v1.32+)
cat > kind-config.yaml << EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  image: kindest/node:v1.32.0
  extraPortMappings:
  - containerPort: 30090  # Prometheus NodePort
    hostPort: 9091
    protocol: TCP
  - containerPort: 30900  # Tekton Pruner metrics NodePort  
    hostPort: 9090
    protocol: TCP
  - containerPort: 30686  # Jaeger NodePort
    hostPort: 16686
    protocol: TCP
EOF

# Create Kind cluster
echo "📦 Creating Kind cluster '${KIND_CLUSTER_NAME}'..."
kind create cluster --config kind-config.yaml --name "${KIND_CLUSTER_NAME}"

# 2. Install Tekton Pipeline
echo "📦 Installing Tekton Pipeline..."
kubectl apply -f https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml
wait_for_deploy tekton-pipelines tekton-pipelines-controller

# 3. Install basic Prometheus (without operator)
echo "📦 Installing basic Prometheus..."
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
    scrape_configs:
    - job_name: 'tekton-pruner'
      metrics_path: '/metrics'
      static_configs:
      - targets:
        - tekton-pruner-metrics.tekton-pipelines.svc.cluster.local:9090
      scrape_interval: 30s
    - job_name: 'kubernetes-pods'
      kubernetes_sd_configs:
      - role: pod
        namespaces:
          names:
          - tekton-pipelines
      relabel_configs:
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
        action: keep
        regex: true
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_path]
        action: replace
        target_label: __metrics_path__
        regex: (.+)
      - source_labels: [__address__, __meta_kubernetes_pod_annotation_prometheus_io_port]
        action: replace
        regex: ([^:]+)(?::\d+)?;(\d+)
        replacement: \$1:\$2
        target_label: __address__
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
    nodePort: 30090
  type: NodePort
EOF

wait_for_deploy monitoring prometheus

# 4. Install Jaeger (simple deployment)
echo "📦 Installing Jaeger..."
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
    nodePort: 30686
  - name: collector
    port: 14268
    targetPort: 14268
  type: NodePort
EOF

wait_for_deploy observability-system jaeger

# 5. Deploy observability configuration
echo "⚙️ Deploying observability configuration..."
kubectl apply -f - << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: config-observability-tekton-pruner
  namespace: tekton-pipelines
data:
  metrics.enabled: "true"
  metrics.backend-destination: "prometheus"
  metrics.port: "9090"
  tracing.enabled: "true"
  tracing.backend: "jaeger"
  tracing.endpoint: "http://jaeger.observability-system.svc.cluster.local:14268/api/traces"
  tracing.sample-rate: "0.1"
  metrics.debug: "true"
EOF

# 6. Create service for Tekton Pruner metrics
echo "📊 Creating metrics service..."
kubectl apply -f - << EOF
apiVersion: v1
kind: Service
metadata:
  name: tekton-pruner-metrics
  namespace: tekton-pipelines
  labels:
    app: controller
    service: metrics
spec:
  selector:
    app.kubernetes.io/name: tekton-pruner-controller
  ports:
  - name: metrics
    port: 9090
    targetPort: 9090
    nodePort: 30900
  type: NodePort
EOF

# 7. Deploy Tekton Pruner
echo "🚀 Building and deploying Tekton Pruner..."
export KO_DOCKER_REPO
ko apply -f config/

# Wait for Tekton Pruner deployment
wait_for_deploy tekton-pipelines tekton-pruner-controller

# Wait for other deployments to be ready
echo "⏳ Waiting for all services to be ready..."
wait_for_deploy monitoring prometheus
wait_for_deploy observability-system jaeger

echo "✅ Simple observability setup complete!"
echo ""
echo "🔗 Access the observability stack:"
echo "   Prometheus: http://localhost:9091 (or kubectl port-forward -n monitoring svc/prometheus 9091:9090)"
echo "   Jaeger: http://localhost:16686 (or kubectl port-forward -n observability-system svc/jaeger 16686:16686)"
echo "   Pruner Metrics: http://localhost:9090/metrics (or kubectl port-forward -n tekton-pipelines svc/tekton-pruner-metrics 9090:9090)"
echo ""
echo "📊 Test metrics with:"
echo "   curl http://localhost:9090/metrics | grep -E 'tektoncd_pruner_'"
echo "   curl http://localhost:9090/metrics | grep -E 'reconcile_count|workqueue_|client_'"
echo ""
echo "🎯 Check Prometheus targets:"
echo "   Open http://localhost:9091/targets to see if Tekton Pruner is being scraped"
echo ""
echo "🆘 If metrics are still not working:"
echo "   1. Check pod logs: kubectl logs -n tekton-pipelines -l app=controller"
echo "   2. Check service endpoints: kubectl get endpoints -n tekton-pipelines tekton-pruner-metrics"
echo "   3. Test direct connection: kubectl port-forward -n tekton-pipelines deployment/tekton-pruner-controller 9090:9090"