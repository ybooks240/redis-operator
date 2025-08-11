#!/bin/bash

# Redis Operator 监控、告警和日志采集系统清理脚本
# 该脚本用于清理所有已部署的监控组件

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

# 确认清理操作
confirm_cleanup() {
    echo -e "${YELLOW}警告: 此操作将删除所有监控、告警和日志采集组件！${NC}"
    echo "这包括:"
    echo "  - Prometheus Operator 和相关组件"
    echo "  - Grafana 仪表板和配置"
    echo "  - AlertManager 配置"
    echo "  - Fluent Bit 日志采集"
    echo "  - Elasticsearch 和 Kibana (如果通过 Helm 安装)"
    echo "  - Jaeger 链路追踪"
    echo "  - 相关的命名空间和数据"
    echo ""
    read -p "确定要继续吗？(y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        log_info "操作已取消"
        exit 0
    fi
}

# 检查 Helm 可用性
check_helm() {
    if command -v helm &> /dev/null; then
        HELM_AVAILABLE=true
        log_info "检测到 Helm，将清理 Helm 部署的组件"
    else
        HELM_AVAILABLE=false
        log_warning "未检测到 Helm，跳过 Helm 组件清理"
    fi
}

# 清理 Helm 部署的组件
cleanup_helm_components() {
    if [ "$HELM_AVAILABLE" = true ]; then
        log_info "清理 Helm 部署的组件..."
        
        # 清理 Prometheus Operator
        if helm list -n monitoring | grep -q prometheus-operator; then
            log_info "卸载 Prometheus Operator..."
            helm uninstall prometheus-operator -n monitoring || true
        fi
        
        # 清理 Elasticsearch
        if helm list -n logging | grep -q elasticsearch; then
            log_info "卸载 Elasticsearch..."
            helm uninstall elasticsearch -n logging || true
        fi
        
        # 清理 Kibana
        if helm list -n logging | grep -q kibana; then
            log_info "卸载 Kibana..."
            helm uninstall kibana -n logging || true
        fi
        
        log_success "Helm 组件清理完成"
    fi
}

# 清理 Kubernetes 资源
cleanup_k8s_resources() {
    log_info "清理 Kubernetes 资源..."
    
    # 清理监控配置
    log_info "清理监控配置..."
    kubectl delete -f config/monitoring/ --ignore-not-found=true || true
    kubectl delete configmap redis-operator-dashboard -n monitoring --ignore-not-found=true || true
    
    # 清理日志采集配置
    log_info "清理日志采集配置..."
    kubectl delete -f config/logging/ --ignore-not-found=true || true
    
    # 清理链路追踪配置
    log_info "清理链路追踪配置..."
    kubectl delete -f config/tracing/ --ignore-not-found=true || true
    
    # 清理 Prometheus Operator CRDs (如果不是通过 Helm 安装)
    if [ "$HELM_AVAILABLE" = false ]; then
        log_info "清理 Prometheus Operator CRDs..."
        kubectl delete --ignore-not-found=true -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/v0.68.0/bundle.yaml || true
    fi
    
    log_success "Kubernetes 资源清理完成"
}

# 清理 PVC 和持久化数据
cleanup_persistent_data() {
    log_info "清理持久化数据..."
    
    # 清理 Prometheus 数据
    kubectl delete pvc -l app.kubernetes.io/name=prometheus -n monitoring --ignore-not-found=true || true
    
    # 清理 Grafana 数据
    kubectl delete pvc -l app.kubernetes.io/name=grafana -n monitoring --ignore-not-found=true || true
    
    # 清理 AlertManager 数据
    kubectl delete pvc -l app.kubernetes.io/name=alertmanager -n monitoring --ignore-not-found=true || true
    
    # 清理 Elasticsearch 数据
    kubectl delete pvc -l app=elasticsearch-master -n logging --ignore-not-found=true || true
    
    log_success "持久化数据清理完成"
}

# 清理命名空间
cleanup_namespaces() {
    log_info "清理命名空间..."
    
    # 等待资源清理完成
    sleep 10
    
    # 删除命名空间
    kubectl delete namespace monitoring --ignore-not-found=true --timeout=60s || true
    kubectl delete namespace logging --ignore-not-found=true --timeout=60s || true
    kubectl delete namespace tracing --ignore-not-found=true --timeout=60s || true
    
    # 清理 redis-operator-system 命名空间中的监控相关资源
    kubectl delete servicemonitor,prometheusrule,configmap -l app.kubernetes.io/component=monitoring -n redis-operator-system --ignore-not-found=true || true
    kubectl delete servicemonitor,prometheusrule,configmap -l app.kubernetes.io/component=tracing -n redis-operator-system --ignore-not-found=true || true
    
    log_success "命名空间清理完成"
}

# 验证清理结果
verify_cleanup() {
    log_info "验证清理结果..."
    
    echo "\n=== 剩余的监控相关资源 ==="
    
    # 检查命名空间
    echo "命名空间:"
    kubectl get namespaces | grep -E "(monitoring|logging|tracing)" || echo "  无相关命名空间"
    
    # 检查 CRDs
    echo "\nPrometheus CRDs:"
    kubectl get crd | grep -E "(prometheus|alertmanager|servicemonitor)" || echo "  无相关 CRDs"
    
    # 检查 Helm releases
    if [ "$HELM_AVAILABLE" = true ]; then
        echo "\nHelm Releases:"
        helm list -A | grep -E "(prometheus|grafana|elasticsearch|kibana)" || echo "  无相关 Helm releases"
    fi
    
    # 检查 redis-operator-system 命名空间中的监控资源
    echo "\nredis-operator-system 命名空间中的监控资源:"
    kubectl get servicemonitor,prometheusrule,configmap -l app.kubernetes.io/component=monitoring -n redis-operator-system 2>/dev/null || echo "  无监控相关资源"
    kubectl get servicemonitor,prometheusrule,configmap -l app.kubernetes.io/component=tracing -n redis-operator-system 2>/dev/null || echo "  无追踪相关资源"
    
    log_success "清理验证完成"
}

# 显示清理后的建议
show_post_cleanup_info() {
    log_info "清理完成后的建议:"
    
    echo "\n=== 清理完成 ==="
    echo "所有监控、告警和日志采集组件已被清理。"
    echo ""
    echo "如果需要重新部署，请运行:"
    echo "  ./scripts/deploy-monitoring.sh"
    echo ""
    echo "注意事项:"
    echo "  - 所有监控数据和日志已被删除"
    echo "  - Grafana 仪表板配置已被清理"
    echo "  - AlertManager 告警规则已被移除"
    echo "  - 如果有自定义配置，请重新应用"
    echo ""
}

# 主函数
main() {
    log_info "开始清理 Redis Operator 监控、告警和日志采集系统..."
    
    confirm_cleanup
    check_helm
    cleanup_helm_components
    cleanup_k8s_resources
    cleanup_persistent_data
    cleanup_namespaces
    verify_cleanup
    show_post_cleanup_info
    
    log_success "清理操作完成！"
}

# 脚本选项处理
case "${1:-}" in
    --force)
        log_warning "强制清理模式，跳过确认..."
        check_helm
        cleanup_helm_components
        cleanup_k8s_resources
        cleanup_persistent_data
        cleanup_namespaces
        verify_cleanup
        show_post_cleanup_info
        ;;
    --monitoring-only)
        log_info "仅清理监控组件..."
        confirm_cleanup
        check_helm
        if [ "$HELM_AVAILABLE" = true ]; then
            helm uninstall prometheus-operator -n monitoring || true
        fi
        kubectl delete -f config/monitoring/ --ignore-not-found=true || true
        kubectl delete configmap redis-operator-dashboard -n monitoring --ignore-not-found=true || true
        kubectl delete pvc -l app.kubernetes.io/name=prometheus -n monitoring --ignore-not-found=true || true
        kubectl delete pvc -l app.kubernetes.io/name=grafana -n monitoring --ignore-not-found=true || true
        kubectl delete pvc -l app.kubernetes.io/name=alertmanager -n monitoring --ignore-not-found=true || true
        ;;
    --logging-only)
        log_info "仅清理日志采集组件..."
        confirm_cleanup
        check_helm
        if [ "$HELM_AVAILABLE" = true ]; then
            helm uninstall elasticsearch -n logging || true
            helm uninstall kibana -n logging || true
        fi
        kubectl delete -f config/logging/ --ignore-not-found=true || true
        kubectl delete pvc -l app=elasticsearch-master -n logging --ignore-not-found=true || true
        ;;
    --tracing-only)
        log_info "仅清理链路追踪组件..."
        confirm_cleanup
        kubectl delete -f config/tracing/ --ignore-not-found=true || true
        ;;
    --help|-h)
        echo "用法: $0 [选项]"
        echo "选项:"
        echo "  --force            强制清理，跳过确认"
        echo "  --monitoring-only  仅清理监控组件"
        echo "  --logging-only     仅清理日志采集组件"
        echo "  --tracing-only     仅清理链路追踪组件"
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