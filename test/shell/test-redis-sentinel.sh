#!/bin/bash

# Redis Sentinel 自动化测试脚本
# 测试 RedisSentinel 资源的创建、配置和故障转移功能

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

# 配置变量
NAMESPACE="redis-sentinel-test"
SENTINEL_NAME="test-sentinel"
MASTER_REPLICA_NAME="test-master-replica"
TEST_TIMEOUT=300
CHECK_INTERVAL=5

# 清理函数
cleanup() {
    log_info "清理测试资源..."
    kubectl delete namespace $NAMESPACE --ignore-not-found=true
    log_success "清理完成"
}

# 错误处理
trap cleanup EXIT

# 检查 kubectl 是否可用
check_kubectl() {
    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl 命令未找到，请确保 Kubernetes CLI 已安装"
        exit 1
    fi
    
    if ! kubectl cluster-info &> /dev/null; then
        log_error "无法连接到 Kubernetes 集群"
        exit 1
    fi
    
    log_success "Kubernetes 集群连接正常"
}

# 创建测试命名空间
create_namespace() {
    log_info "创建测试命名空间: $NAMESPACE"
    kubectl create namespace $NAMESPACE || true
    kubectl config set-context --current --namespace=$NAMESPACE
    log_success "命名空间创建完成"
}

# 等待资源就绪
wait_for_resource() {
    local resource_type=$1
    local resource_name=$2
    local condition=$3
    local timeout=$4
    
    log_info "等待 $resource_type/$resource_name 达到条件: $condition"
    
    if kubectl wait --for=condition=$condition $resource_type/$resource_name --timeout=${timeout}s; then
        log_success "$resource_type/$resource_name 已就绪"
        return 0
    else
        log_error "$resource_type/$resource_name 未能在 ${timeout}s 内就绪"
        return 1
    fi
}

# 等待 Pod 运行
wait_for_pods() {
    local label_selector=$1
    local expected_count=$2
    local timeout=$3
    
    log_info "等待 Pod 运行 (标签: $label_selector, 期望数量: $expected_count)"
    
    local start_time=$(date +%s)
    while true; do
        local current_time=$(date +%s)
        local elapsed=$((current_time - start_time))
        
        if [ $elapsed -gt $timeout ]; then
            log_error "等待 Pod 运行超时 (${timeout}s)"
            return 1
        fi
        
        local running_pods=$(kubectl get pods -l "$label_selector" --field-selector=status.phase=Running --no-headers | wc -l)
        
        if [ "$running_pods" -eq "$expected_count" ]; then
            log_success "所有 Pod 已运行 ($running_pods/$expected_count)"
            return 0
        fi
        
        log_info "当前运行的 Pod: $running_pods/$expected_count，等待中..."
        sleep $CHECK_INTERVAL
    done
}

# 测试内嵌 Redis 配置的 RedisSentinel
test_embedded_redis_sentinel() {
    log_info "=== 测试内嵌 Redis 配置的 RedisSentinel ==="
    
    # 创建 RedisSentinel 资源
    cat <<EOF | kubectl apply -f -
apiVersion: redis.github.com/v1
kind: RedisSentinel
metadata:
  name: $SENTINEL_NAME-embedded
  namespace: $NAMESPACE
spec:
  image: redis:7.0
  replicas: 3
  resources:
    requests:
      cpu: 50m
      memory: 128Mi
  storage:
    size: 500Mi
    storageClassName: standard
  config:
    quorum: 2
    downAfterMilliseconds: 30000
    failoverTimeout: 180000
    parallelSyncs: 1
  redis:
    master:
      resources:
        requests:
          cpu: 100m
          memory: 128Mi
      storage:
        size: 1Gi
        storageClassName: standard
    replica:
      replicas: 2
      resources:
        requests:
          cpu: 100m
          memory: 128Mi
      storage:
        size: 1Gi
        storageClassName: standard
    masterName: mymaster
  # masterReplicaRef 为空，使用内嵌配置
  masterReplicaRef:
    name: ""
EOF
    
    log_success "RedisSentinel 资源已创建"
    
    # 等待 RedisSentinel 就绪
    if ! wait_for_resource "redissentinel" "$SENTINEL_NAME-embedded" "Ready" $TEST_TIMEOUT; then
        log_error "RedisSentinel 未能就绪"
        kubectl describe redissentinel $SENTINEL_NAME-embedded
        return 1
    fi
    
    # 检查 Pod 状态
    log_info "检查 Redis Master Pod"
    if ! wait_for_pods "app=redis-master,redis-sentinel=$SENTINEL_NAME-embedded" 1 $TEST_TIMEOUT; then
        return 1
    fi
    
    log_info "检查 Redis Replica Pod"
    if ! wait_for_pods "app=redis-replica,redis-sentinel=$SENTINEL_NAME-embedded" 2 $TEST_TIMEOUT; then
        return 1
    fi
    
    log_info "检查 Sentinel Pod"
    if ! wait_for_pods "app=redis-sentinel,redis-sentinel=$SENTINEL_NAME-embedded" 3 $TEST_TIMEOUT; then
        return 1
    fi
    
    # 检查服务
    log_info "检查服务创建"
    kubectl get svc -l "redis-sentinel=$SENTINEL_NAME-embedded"
    
    # 检查 ConfigMap
    log_info "检查 ConfigMap 创建"
    kubectl get configmap -l "redis-sentinel=$SENTINEL_NAME-embedded"
    
    log_success "内嵌 Redis 配置的 RedisSentinel 测试通过"
}

# 测试外部引用的 RedisSentinel
test_external_reference_sentinel() {
    log_info "=== 测试外部引用的 RedisSentinel ==="
    
    # 首先创建 RedisMasterReplica 资源
    cat <<EOF | kubectl apply -f -
apiVersion: redis.github.com/v1
kind: RedisMasterReplica
metadata:
  name: $MASTER_REPLICA_NAME
  namespace: $NAMESPACE
spec:
  image: redis:7.0
  master:
    resources:
      requests:
        cpu: 100m
        memory: 128Mi
    storage:
      size: 1Gi
      storageClassName: standard
  replica:
    replicas: 2
    resources:
      requests:
        cpu: 100m
        memory: 128Mi
    storage:
      size: 1Gi
      storageClassName: standard
EOF
    
    log_success "RedisMasterReplica 资源已创建"
    
    # 等待 RedisMasterReplica 就绪
    if ! wait_for_resource "redismasterreplica" "$MASTER_REPLICA_NAME" "Ready" $TEST_TIMEOUT; then
        log_error "RedisMasterReplica 未能就绪"
        kubectl describe redismasterreplica $MASTER_REPLICA_NAME
        return 1
    fi
    
    # 创建引用外部 RedisMasterReplica 的 RedisSentinel
    cat <<EOF | kubectl apply -f -
apiVersion: redis.github.com/v1
kind: RedisSentinel
metadata:
  name: $SENTINEL_NAME-external
  namespace: $NAMESPACE
spec:
  image: redis:7.0
  replicas: 3
  resources:
    requests:
      cpu: 50m
      memory: 128Mi
  storage:
    size: 500Mi
    storageClassName: standard
  config:
    quorum: 2
    downAfterMilliseconds: 30000
    failoverTimeout: 180000
    parallelSyncs: 1
  masterReplicaRef:
    name: $MASTER_REPLICA_NAME
    namespace: $NAMESPACE
    masterName: mymaster
EOF
    
    log_success "外部引用的 RedisSentinel 资源已创建"
    
    # 等待 RedisSentinel 就绪
    if ! wait_for_resource "redissentinel" "$SENTINEL_NAME-external" "Ready" $TEST_TIMEOUT; then
        log_error "外部引用的 RedisSentinel 未能就绪"
        kubectl describe redissentinel $SENTINEL_NAME-external
        return 1
    fi
    
    # 检查只有 Sentinel Pod 被创建（不应该创建额外的 Redis Pod）
    log_info "检查 Sentinel Pod"
    if ! wait_for_pods "app=redis-sentinel,redis-sentinel=$SENTINEL_NAME-external" 3 $TEST_TIMEOUT; then
        return 1
    fi
    
    # 确认没有创建额外的 Redis Master/Replica Pod
    log_info "确认没有创建额外的 Redis Pod"
    local extra_redis_pods=$(kubectl get pods -l "redis-sentinel=$SENTINEL_NAME-external" --field-selector=status.phase=Running --no-headers | grep -v sentinel | wc -l)
    if [ "$extra_redis_pods" -ne "0" ]; then
        log_error "发现了不应该存在的 Redis Pod: $extra_redis_pods"
        return 1
    fi
    
    log_success "外部引用的 RedisSentinel 测试通过"
}

# 测试 Sentinel 配置
test_sentinel_configuration() {
    log_info "=== 测试 Sentinel 配置 ==="
    
    # 获取 Sentinel ConfigMap
    local configmap_name="$SENTINEL_NAME-embedded-sentinel-config"
    
    if ! kubectl get configmap $configmap_name &> /dev/null; then
        log_error "Sentinel ConfigMap 不存在: $configmap_name"
        return 1
    fi
    
    # 检查配置内容
    log_info "检查 Sentinel 配置内容"
    local config_content=$(kubectl get configmap $configmap_name -o jsonpath='{.data.sentinel\.conf}')
    
    # 检查关键配置项
    if echo "$config_content" | grep -q "sentinel monitor mymaster"; then
        log_success "找到 master 监控配置"
    else
        log_error "未找到 master 监控配置"
        return 1
    fi
    
    if echo "$config_content" | grep -q "sentinel down-after-milliseconds mymaster 30000"; then
        log_success "找到 down-after-milliseconds 配置"
    else
        log_error "未找到 down-after-milliseconds 配置"
        return 1
    fi
    
    if echo "$config_content" | grep -q "sentinel failover-timeout mymaster 180000"; then
        log_success "找到 failover-timeout 配置"
    else
        log_error "未找到 failover-timeout 配置"
        return 1
    fi
    
    log_success "Sentinel 配置测试通过"
}

# 测试 Sentinel 连接性
test_sentinel_connectivity() {
    log_info "=== 测试 Sentinel 连接性 ==="
    
    # 获取 Sentinel Service
    local service_name="$SENTINEL_NAME-embedded-sentinel-service"
    
    if ! kubectl get service $service_name &> /dev/null; then
        log_error "Sentinel Service 不存在: $service_name"
        return 1
    fi
    
    # 获取 Service 端口
    local service_port=$(kubectl get service $service_name -o jsonpath='{.spec.ports[0].port}')
    log_info "Sentinel Service 端口: $service_port"
    
    # 创建测试 Pod 来检查连接性
    kubectl run redis-test --image=redis:7.0 --rm -i --restart=Never -- redis-cli -h $service_name -p $service_port ping
    
    if [ $? -eq 0 ]; then
        log_success "Sentinel 连接性测试通过"
    else
        log_error "Sentinel 连接性测试失败"
        return 1
    fi
}

# 测试状态更新
test_status_updates() {
    log_info "=== 测试状态更新 ==="
    
    # 检查 RedisSentinel 状态
    local status=$(kubectl get redissentinel $SENTINEL_NAME-embedded -o jsonpath='{.status.status}')
    local ready=$(kubectl get redissentinel $SENTINEL_NAME-embedded -o jsonpath='{.status.ready}')
    
    if [ "$status" = "Running" ] && [ "$ready" = "true" ]; then
        log_success "RedisSentinel 状态正确: status=$status, ready=$ready"
    else
        log_error "RedisSentinel 状态异常: status=$status, ready=$ready"
        return 1
    fi
    
    # 检查副本数量
    local replicas=$(kubectl get redissentinel $SENTINEL_NAME-embedded -o jsonpath='{.status.replicas}')
    local ready_replicas=$(kubectl get redissentinel $SENTINEL_NAME-embedded -o jsonpath='{.status.readyReplicas}')
    
    if [ "$replicas" = "3" ] && [ "$ready_replicas" = "3" ]; then
        log_success "副本数量正确: replicas=$replicas, readyReplicas=$ready_replicas"
    else
        log_error "副本数量异常: replicas=$replicas, readyReplicas=$ready_replicas"
        return 1
    fi
    
    log_success "状态更新测试通过"
}

# 主测试函数
main() {
    log_info "开始 Redis Sentinel 自动化测试"
    
    # 检查前置条件
    check_kubectl
    
    # 创建测试环境
    create_namespace
    
    # 运行测试
    test_embedded_redis_sentinel
    test_external_reference_sentinel
    test_sentinel_configuration
    test_sentinel_connectivity
    test_status_updates
    
    log_success "所有测试通过！"
}

# 运行主函数
main "$@"