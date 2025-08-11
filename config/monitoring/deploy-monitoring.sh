#!/bin/bash

# Redis Operator Grafana Dashboard 部署脚本
# 用于快速部署 Redis Operator 监控组件

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 配置变量
NAMESPACE="monitoring"
GRAFANA_NAMESPACE="monitoring"
PROMETHEUS_NAMESPACE="monitoring"
REDIS_OPERATOR_NAMESPACE="redis-operator-system"

# 函数定义
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

check_prerequisites() {
    log_info "检查前置条件..."
    
    # 检查 kubectl
    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl 未安装或不在 PATH 中"
        exit 1
    fi
    
    # 检查集群连接
    if ! kubectl cluster-info &> /dev/null; then
        log_error "无法连接到 Kubernetes 集群"
        exit 1
    fi
    
    # 检查 Prometheus Operator
    if ! kubectl get crd prometheuses.monitoring.coreos.com &> /dev/null; then
        log_warning "Prometheus Operator 未安装，某些功能可能不可用"
    fi
    
    log_success "前置条件检查完成"
}

create_namespace() {
    log_info "创建命名空间 $NAMESPACE..."
    
    kubectl create namespace $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -
    
    log_success "命名空间 $NAMESPACE 已创建或已存在"
}

deploy_redis_exporter() {
    log_info "部署 Redis Exporter..."
    
    cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis-exporter
  namespace: $NAMESPACE
  labels:
    app: redis-exporter
spec:
  replicas: 1
  selector:
    matchLabels:
      app: redis-exporter
  template:
    metadata:
      labels:
        app: redis-exporter
    spec:
      containers:
      - name: redis-exporter
        image: oliver006/redis_exporter:v1.45.0
        ports:
        - containerPort: 9121
          name: metrics
        env:
        - name: REDIS_ADDR
          value: "redis://redis-service:6379"
        - name: REDIS_EXPORTER_LOG_FORMAT
          value: "txt"
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 200m
            memory: 256Mi
        livenessProbe:
          httpGet:
            path: /metrics
            port: 9121
          initialDelaySeconds: 30
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /metrics
            port: 9121
          initialDelaySeconds: 5
          periodSeconds: 10
---
apiVersion: v1
kind: Service
metadata:
  name: redis-exporter
  namespace: $NAMESPACE
  labels:
    app: redis-exporter
spec:
  ports:
  - port: 9121
    targetPort: 9121
    name: metrics
  selector:
    app: redis-exporter
EOF

    log_success "Redis Exporter 部署完成"
}

deploy_service_monitor() {
    log_info "部署 ServiceMonitor..."
    
    cat <<EOF | kubectl apply -f -
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: redis-exporter
  namespace: $NAMESPACE
  labels:
    app: redis-exporter
spec:
  selector:
    matchLabels:
      app: redis-exporter
  endpoints:
  - port: metrics
    interval: 30s
    path: /metrics
    honorLabels: true
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: redis-operator-controller-manager-metrics-monitor
  namespace: $NAMESPACE
  labels:
    control-plane: controller-manager
spec:
  endpoints:
  - bearerTokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token
    path: /metrics
    port: https
    scheme: https
    tlsConfig:
      insecureSkipVerify: true
  selector:
    matchLabels:
      control-plane: controller-manager
EOF

    log_success "ServiceMonitor 部署完成"
}

deploy_grafana_dashboard() {
    local dashboard_type=$1
    local dashboard_file="redis-operator-${dashboard_type}-dashboard.json"
    
    log_info "部署 Grafana Dashboard: $dashboard_type"
    
    if [ ! -f "$dashboard_file" ]; then
        log_error "Dashboard 文件 $dashboard_file 不存在"
        return 1
    fi
    
    # 创建 ConfigMap
    kubectl create configmap "redis-operator-${dashboard_type}-dashboard" \
        --from-file="$dashboard_file" \
        --namespace="$GRAFANA_NAMESPACE" \
        --dry-run=client -o yaml | \
    kubectl label --local -f - grafana_dashboard="1" -o yaml | \
    kubectl apply -f -
    
    log_success "Grafana Dashboard ($dashboard_type) 部署完成"
}

setup_rbac() {
    log_info "设置 RBAC 权限..."
    
    cat <<EOF | kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: redis-monitoring
rules:
- apiGroups: [""]
  resources: ["pods", "services", "endpoints"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["redis.redis.opstreelabs.in"]
  resources: ["*"]
  verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: redis-monitoring
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: redis-monitoring
subjects:
- kind: ServiceAccount
  name: default
  namespace: $NAMESPACE
EOF

    log_success "RBAC 权限设置完成"
}

verify_deployment() {
    log_info "验证部署状态..."
    
    # 检查 Redis Exporter
    if kubectl get deployment redis-exporter -n $NAMESPACE &> /dev/null; then
        local ready_replicas=$(kubectl get deployment redis-exporter -n $NAMESPACE -o jsonpath='{.status.readyReplicas}')
        if [ "$ready_replicas" = "1" ]; then
            log_success "Redis Exporter 运行正常"
        else
            log_warning "Redis Exporter 可能未就绪"
        fi
    else
        log_warning "Redis Exporter 未找到"
    fi
    
    # 检查 ServiceMonitor
    if kubectl get servicemonitor redis-exporter -n $NAMESPACE &> /dev/null; then
        log_success "Redis Exporter ServiceMonitor 已创建"
    else
        log_warning "Redis Exporter ServiceMonitor 未找到"
    fi
    
    if kubectl get servicemonitor redis-operator-controller-manager-metrics-monitor -n $NAMESPACE &> /dev/null; then
        log_success "Redis Operator ServiceMonitor 已创建"
    else
        log_warning "Redis Operator ServiceMonitor 未找到"
    fi
    
    # 检查 Dashboard ConfigMaps
    local dashboards=("comprehensive" "simple")
    for dashboard in "${dashboards[@]}"; do
        if kubectl get configmap "redis-operator-${dashboard}-dashboard" -n $GRAFANA_NAMESPACE &> /dev/null; then
            log_success "Dashboard ConfigMap (${dashboard}) 已创建"
        else
            log_warning "Dashboard ConfigMap (${dashboard}) 未找到"
        fi
    done
}

show_access_info() {
    log_info "访问信息:"
    
    echo ""
    echo "📊 Grafana Dashboard:"
    echo "   - 综合版: redis-operator-comprehensive-dashboard"
    echo "   - 简化版: redis-operator-simple-dashboard"
    echo ""
    echo "🔍 Prometheus 目标:"
    echo "   - Redis Exporter: redis-exporter:9121"
    echo "   - Redis Operator: redis-operator-controller-manager-metrics-service:8443"
    echo ""
    echo "📋 验证命令:"
    echo "   kubectl get pods -n $NAMESPACE"
    echo "   kubectl get servicemonitor -n $NAMESPACE"
    echo "   kubectl get configmap -l grafana_dashboard=1 -n $GRAFANA_NAMESPACE"
    echo ""
    echo "🚀 端口转发 (用于测试):"
    echo "   kubectl port-forward svc/redis-exporter 9121:9121 -n $NAMESPACE"
    echo "   curl http://localhost:9121/metrics"
    echo ""
}

cleanup() {
    log_info "清理资源..."
    
    read -p "确定要删除所有监控组件吗? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        kubectl delete deployment redis-exporter -n $NAMESPACE --ignore-not-found
        kubectl delete service redis-exporter -n $NAMESPACE --ignore-not-found
        kubectl delete servicemonitor redis-exporter -n $NAMESPACE --ignore-not-found
        kubectl delete servicemonitor redis-operator-controller-manager-metrics-monitor -n $NAMESPACE --ignore-not-found
        kubectl delete configmap redis-operator-comprehensive-dashboard -n $GRAFANA_NAMESPACE --ignore-not-found
        kubectl delete configmap redis-operator-simple-dashboard -n $GRAFANA_NAMESPACE --ignore-not-found
        kubectl delete clusterrole redis-monitoring --ignore-not-found
        kubectl delete clusterrolebinding redis-monitoring --ignore-not-found
        
        log_success "清理完成"
    else
        log_info "取消清理操作"
    fi
}

show_help() {
    echo "Redis Operator 监控部署脚本"
    echo ""
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  install     安装所有监控组件"
    echo "  dashboard   仅部署 Grafana Dashboard"
    echo "  exporter    仅部署 Redis Exporter"
    echo "  verify      验证部署状态"
    echo "  cleanup     清理所有组件"
    echo "  help        显示此帮助信息"
    echo ""
    echo "环境变量:"
    echo "  NAMESPACE              监控组件命名空间 (默认: monitoring)"
    echo "  GRAFANA_NAMESPACE      Grafana 命名空间 (默认: monitoring)"
    echo "  PROMETHEUS_NAMESPACE   Prometheus 命名空间 (默认: monitoring)"
    echo ""
    echo "示例:"
    echo "  $0 install              # 完整安装"
    echo "  $0 dashboard            # 仅安装 Dashboard"
    echo "  NAMESPACE=kube-system $0 install  # 使用自定义命名空间"
    echo ""
}

# 主函数
main() {
    local action=${1:-help}
    
    case $action in
        "install")
            log_info "开始安装 Redis Operator 监控组件..."
            check_prerequisites
            create_namespace
            setup_rbac
            deploy_redis_exporter
            deploy_service_monitor
            deploy_grafana_dashboard "comprehensive"
            deploy_grafana_dashboard "simple"
            verify_deployment
            show_access_info
            log_success "安装完成!"
            ;;
        "dashboard")
            log_info "部署 Grafana Dashboard..."
            deploy_grafana_dashboard "comprehensive"
            deploy_grafana_dashboard "simple"
            log_success "Dashboard 部署完成!"
            ;;
        "exporter")
            log_info "部署 Redis Exporter..."
            check_prerequisites
            create_namespace
            setup_rbac
            deploy_redis_exporter
            deploy_service_monitor
            verify_deployment
            log_success "Redis Exporter 部署完成!"
            ;;
        "verify")
            verify_deployment
            ;;
        "cleanup")
            cleanup
            ;;
        "help")
            show_help
            ;;
        *)
            log_error "未知操作: $action"
            show_help
            exit 1
            ;;
    esac
}

# 脚本入口
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi