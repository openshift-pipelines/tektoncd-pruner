#!/bin/bash
# setup-observability-simple.sh - Alternative setup without Prometheus Operator

set -euo pipefail

echo "🚀 Setting up Tekton Pruner with simple observability stack..."

# 1. Create Kind cluster with port mappings
cat > kind-config.yaml << EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: 9090
    hostPort: 9090
    protocol: TCP
  - containerPort: 16686  
    hostPort: 16686
    protocol: TCP
EOF

echo "📦 Creating Kind cluster..."
kind create cluster --config kind-config.yaml --name tekton-obs

# 2. Install Tekton Pipeline
echo "📦 Installing Tekton Pipeline..."
kubectl apply -f https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml
kubectl wait --for=condition=ready pod -l app=tekton-pipelines-controller -n tekton-pipelines --timeout=300s

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
      static_configs:
      - targets: ['tekton-pruner-controller.tekton-pipelines.svc.cluster.local:9090']
      metrics_path: '/metrics'
      scrape_interval: 30s
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
  name: tekton-pruner-controller
  namespace: tekton-pipelines
  labels:
    app: tekton-pruner-controller
spec:
  ports:
  - name: metrics
    port: 9090
    targetPort: 9090
  selector:
    app: tekton-pruner-controller
EOF

# 7. Deploy Tekton Pruner
echo "🚀 Building and deploying Tekton Pruner..."
export KO_DOCKER_REPO=quay.io/rh-ee-anataraj
ko apply -f config/

# Wait for deployments to be ready
echo "⏳ Waiting for services to be ready..."
kubectl wait --for=condition=ready pod -l app=prometheus -n monitoring --timeout=300s
kubectl wait --for=condition=ready pod -l app=jaeger -n observability-system --timeout=300s

echo "✅ Simple observability setup complete!"
echo ""
echo "🔗 Access the observability stack:"
echo "   Prometheus: kubectl port-forward -n monitoring svc/prometheus 9091:9090"
echo "   Jaeger: kubectl port-forward -n observability-system svc/jaeger 16686:16686"
echo "   Metrics: kubectl port-forward -n tekton-pipelines svc/tekton-pruner-controller 9090:9090"
echo ""
echo "📊 Test with: curl http://localhost:9090/metrics | grep tektoncd_pruner"
echo ""
echo "🆘 If you still have issues, use this simpler setup instead of the operator-based one." 