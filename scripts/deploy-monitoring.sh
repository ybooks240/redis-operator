#!/bin/bash

# Redis Operator 监控、告警和日志采集系统部署脚本
# 该脚本用于一键部署 Prometheus、Grafana、AlertManager、Fluent Bit 和 Jaeger

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 日志函数
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

# 检查依赖
check_dependencies() {
    log_info "检查依赖..."
    
    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl 未安装或不在 PATH 中"
        exit 1
    fi
    
    if ! command -v helm &> /dev/null; then
        log_warning "helm 未安装，将跳过 Helm 相关部署"
        HELM_AVAILABLE=false
    else
        HELM_AVAILABLE=true
    fi
    
    log_success "依赖检查完成"
}

# 创建命名空间
create_namespaces() {
    log_info "创建命名空间..."
    
    kubectl create namespace redis-operator-system --dry-run=client -o yaml | kubectl apply -f -
    kubectl create namespace monitoring --dry-run=client -o yaml | kubectl apply -f -
    kubectl create namespace logging --dry-run=client -o yaml | kubectl apply -f -
    kubectl create namespace tracing --dry-run=client -o yaml | kubectl apply -f -
    
    log_success "命名空间创建完成"
}

# 部署 Prometheus Operator
deploy_prometheus_operator() {
    if [ "$HELM_AVAILABLE" = true ]; then
        log_info "使用 Helm 部署 Prometheus Operator..."
        
        helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
        helm repo update
        
        helm upgrade --install prometheus-operator prometheus-community/kube-prometheus-stack \
            --namespace monitoring \
            --set prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues=false \
            --set prometheus.prometheusSpec.ruleSelectorNilUsesHelmValues=false \
            --set grafana.adminPassword=admin123 \
            --wait
    else
        log_info "使用 kubectl 部署 Prometheus Operator..."
        
        kubectl apply --server-side -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/v0.68.0/bundle.yaml
        kubectl wait --for=condition=Ready pods -l app.kubernetes.io/name=prometheus-operator -n default --timeout=300s
    fi
    
    log_success "Prometheus Operator 部署完成"
}

# 部署监控配置
deploy_monitoring_config() {
    log_info "部署监控配置..."
    
    # 应用 Prometheus 规则
    kubectl apply -f config/monitoring/prometheus-rules.yaml
    
    # 应用 AlertManager 配置
    kubectl apply -f config/monitoring/alertmanager-config.yaml
    
    # 创建 Grafana Dashboard ConfigMap
    kubectl create configmap redis-operator-dashboard \
        --from-file=config/monitoring/grafana-dashboard.json \
        --namespace=monitoring \
        --dry-run=client -o yaml | kubectl apply -f -
    
    # 为 Dashboard 添加标签
    kubectl label configmap redis-operator-dashboard \
        grafana_dashboard=1 \
        --namespace=monitoring --overwrite
    
    log_success "监控配置部署完成"
}

# 部署日志采集
deploy_logging() {
    log_info "部署日志采集系统..."
    
    # 部署 Fluent Bit
    kubectl apply -f config/logging/fluent-bit-config.yaml
    
    # 等待 Fluent Bit 就绪
    kubectl wait --for=condition=Ready pods -l app=fluent-bit -n logging --timeout=300s
    
    if [ "$HELM_AVAILABLE" = true ]; then
        log_info "使用 Helm 部署 Elasticsearch 和 Kibana..."
        
        helm repo add elastic https://helm.elastic.co
        helm repo update
        
        # 部署 Elasticsearch
        helm upgrade --install elasticsearch elastic/elasticsearch \
            --namespace logging \
            --set replicas=1 \
            --set minimumMasterNodes=1 \
            --set resources.requests.memory=1Gi \
            --set resources.limits.memory=2Gi \
            --wait
        
        # 部署 Kibana
        helm upgrade --install kibana elastic/kibana \
            --namespace logging \
            --set service.type=ClusterIP \
            --wait
    else
        log_warning "跳过 Elasticsearch 和 Kibana 部署（需要 Helm）"
    fi
    
    log_success "日志采集系统部署完成"
}

# 部署链路追踪
deploy_tracing() {
    log_info "部署链路追踪系统..."
    
    # 部署 Jaeger
    kubectl apply -f config/tracing/jaeger-config.yaml
    
    # 等待 Jaeger 就绪
    kubectl wait --for=condition=Ready pods -l app=jaeger -n redis-operator-system --timeout=300s
    
    log_success "链路追踪系统部署完成"
}

# 验证部署
verify_deployment() {
    log_info "验证部署状态..."
    
    echo "\n=== Prometheus Operator ==="
    kubectl get pods -n monitoring -l app.kubernetes.io/name=prometheus-operator
    
    echo "\n=== Fluent Bit ==="
    kubectl get pods -n logging -l app=fluent-bit
    
    echo "\n=== Jaeger ==="
    kubectl get pods -n redis-operator-system -l app=jaeger
    
    echo "\n=== ServiceMonitors ==="
    kubectl get servicemonitors -n redis-operator-system
    
    echo "\n=== PrometheusRules ==="
    kubectl get prometheusrules -n redis-operator-system
    
    log_success "部署验证完成"
}

# 显示访问信息
show_access_info() {
    log_info "获取访问信息..."
    
    echo "\n=== 访问信息 ==="
    
    # Grafana 访问信息
    if kubectl get service prometheus-grafana -n monitoring &> /dev/null; then
        echo "Grafana:"
        echo "  端口转发: kubectl port-forward svc/prometheus-grafana 3000:80 -n monitoring"
        echo "  访问地址: http://localhost:3000"
        echo "  用户名: admin"
        echo "  密码: admin123"
        echo ""
    fi
    
    # Prometheus 访问信息
    if kubectl get service prometheus-kube-prometheus-prometheus -n monitoring &> /dev/null; then
        echo "Prometheus:"
        echo "  端口转发: kubectl port-forward svc/prometheus-kube-prometheus-prometheus 9090:9090 -n monitoring"
        echo "  访问地址: http://localhost:9090"
        echo ""
    fi
    
    # AlertManager 访问信息
    if kubectl get service alertmanager-kube-prometheus-alertmanager -n monitoring &> /dev/null; then
        echo "AlertManager:"
        echo "  端口转发: kubectl port-forward svc/alertmanager-kube-prometheus-alertmanager 9093:9093 -n monitoring"
        echo "  访问地址: http://localhost:9093"
        echo ""
    fi
    
    # Kibana 访问信息
    if kubectl get service kibana-kibana -n logging &> /dev/null; then
        echo "Kibana:"
        echo "  端口转发: kubectl port-forward svc/kibana-kibana 5601:5601 -n logging"
        echo "  访问地址: http://localhost:5601"
        echo ""
    fi
    
    # Jaeger 访问信息
    if kubectl get service jaeger-query -n redis-operator-system &> /dev/null; then
        echo "Jaeger:"
        echo "  端口转发: kubectl port-forward svc/jaeger-query 16686:16686 -n redis-operator-system"
        echo "  访问地址: http://localhost:16686"
        echo ""
    fi
    
    log_success "访问信息显示完成"
}

# 主函数
main() {
    log_info "开始部署 Redis Operator 监控、告警和日志采集系统..."
    
    check_dependencies
    create_namespaces
    deploy_prometheus_operator
    deploy_monitoring_config
    deploy_logging
    deploy_tracing
    verify_deployment
    show_access_info
    
    log_success "所有组件部署完成！"
    log_info "请使用上述访问信息来访问各个组件的 Web 界面"
}

# 脚本选项处理
case "${1:-}" in
    --monitoring-only)
        log_info "仅部署监控组件..."
        check_dependencies
        create_namespaces
        deploy_prometheus_operator
        deploy_monitoring_config
        verify_deployment
        show_access_info
        ;;
    --logging-only)
        log_info "仅部署日志采集组件..."
        check_dependencies
        create_namespaces
        deploy_logging
        verify_deployment
        ;;
    --tracing-only)
        log_info "仅部署链路追踪组件..."
        check_dependencies
        create_namespaces
        deploy_tracing
        verify_deployment
        show_access_info
        ;;
    --help|-h)
        echo "用法: $0 [选项]"
        echo "选项:"
        echo "  --monitoring-only  仅部署监控组件"
        echo "  --logging-only     仅部署日志采集组件"
        echo "  --tracing-only     仅部署链路追踪组件"
        echo "  --help, -h         显示此帮助信息"
        exit 0
        ;;
    "")
        main
        ;;
    *)
        log_error "未知选项: $1"
        echo "使用 --help 查看可用选项"
        exit 1
        ;;
esac