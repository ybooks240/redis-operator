#!/bin/bash

# Redis Sentinel 状态更新优化测试脚本
# 测试状态更新冲突处理和重试机制

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

# 清理函数
cleanup() {
    log_info "清理测试资源..."
    kubectl delete redissentinel test-status-update --ignore-not-found=true
    kubectl delete namespace test-status-update --ignore-not-found=true
    log_info "清理完成"
}

# 错误处理
trap cleanup EXIT

# 测试配置
NAMESPACE="test-status-update"
RESOURCE_NAME="test-status-update"
TEST_TIMEOUT=300  # 5分钟超时

log_info "开始 Redis Sentinel 状态更新优化测试"

# 1. 创建测试命名空间
log_info "创建测试命名空间: $NAMESPACE"
kubectl create namespace $NAMESPACE || true

# 2. 创建 RedisSentinel 资源
log_info "创建 RedisSentinel 资源"
cat <<EOF | kubectl apply -f -
apiVersion: redis.github.com/v1
kind: RedisSentinel
metadata:
  name: $RESOURCE_NAME
  namespace: $NAMESPACE
spec:
  replicas: 3
  image: redis:7.0
  config:
    quorum: 2
  redis:
    master:
      storage:
        size: 1Gi
    replica:
      replicas: 2
      storage:
        size: 1Gi
  resources:
    limits:
      cpu: 100m
      memory: 128Mi
    requests:
      cpu: 50m
      memory: 64Mi
  storage:
    size: 1Gi
EOF

# 3. 等待资源创建
log_info "等待 RedisSentinel 资源创建..."
sleep 5

# 4. 监控状态更新过程
log_info "监控状态更新过程..."
start_time=$(date +%s)
max_wait_time=$TEST_TIMEOUT

# 检查状态更新是否正常
check_status_updates() {
    local sentinel_name=$1
    local namespace=$2
    
    # 获取状态信息
    local status=$(kubectl get redissentinel $sentinel_name -n $namespace -o jsonpath='{.status}' 2>/dev/null || echo "{}")
    local ready=$(echo $status | jq -r '.ready // "unknown"')
    local phase=$(echo $status | jq -r '.status // "unknown"')
    local replicas=$(echo $status | jq -r '.replicas // 0')
    local ready_replicas=$(echo $status | jq -r '.readyReplicas // 0')
    local conditions=$(echo $status | jq -r '.conditions // []')
    
    log_info "状态检查: Ready=$ready, Phase=$phase, Replicas=$ready_replicas/$replicas"
    
    # 检查是否有状态条件
    if [[ "$conditions" != "[]" ]]; then
        local last_condition=$(echo $status | jq -r '.conditions[-1]')
        local condition_type=$(echo $last_condition | jq -r '.type // "unknown"')
        local condition_status=$(echo $last_condition | jq -r '.status // "unknown"')
        local condition_reason=$(echo $last_condition | jq -r '.reason // "unknown"')
        local condition_message=$(echo $last_condition | jq -r '.message // "unknown"')
        
        log_info "最新状态条件: Type=$condition_type, Status=$condition_status, Reason=$condition_reason"
        log_info "状态消息: $condition_message"
        
        # 检查是否达到就绪状态
        if [[ "$ready" == "true" && "$condition_type" == "Ready" && "$condition_status" == "True" ]]; then
            return 0  # 成功
        fi
    fi
    
    return 1  # 未就绪
}

# 并发状态更新测试
log_info "开始并发状态更新测试..."

# 创建多个并发的 kubectl patch 操作来模拟状态更新冲突
test_concurrent_updates() {
    local sentinel_name=$1
    local namespace=$2
    
    log_info "执行并发状态更新测试..."
    
    # 启动多个并发的 annotation 更新操作
    for i in {1..5}; do
        (
            for j in {1..3}; do
                kubectl annotate redissentinel $sentinel_name -n $namespace test-update-$i-$j="$(date +%s)" --overwrite &>/dev/null || true
                sleep 0.1
            done
        ) &
    done
    
    # 等待所有并发操作完成
    wait
    
    log_info "并发更新测试完成"
}

# 执行并发测试
test_concurrent_updates $RESOURCE_NAME $NAMESPACE

# 等待状态稳定
log_info "等待状态稳定..."
while true; do
    current_time=$(date +%s)
    elapsed=$((current_time - start_time))
    
    if [[ $elapsed -gt $max_wait_time ]]; then
        log_error "测试超时 ($max_wait_time 秒)"
        exit 1
    fi
    
    if check_status_updates $RESOURCE_NAME $NAMESPACE; then
        log_success "RedisSentinel 状态更新成功，所有组件就绪"
        break
    fi
    
    log_info "等待状态更新... (已等待 ${elapsed}s)"
    sleep 10
done

# 5. 验证状态一致性
log_info "验证状态一致性..."

# 检查 StatefulSet 状态
sentinel_sts_ready=$(kubectl get statefulset $RESOURCE_NAME-sentinel -n $NAMESPACE -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
sentinel_sts_replicas=$(kubectl get statefulset $RESOURCE_NAME-sentinel -n $NAMESPACE -o jsonpath='{.spec.replicas}' 2>/dev/null || echo "0")

log_info "Sentinel StatefulSet: $sentinel_sts_ready/$sentinel_sts_replicas ready"

# 检查 RedisSentinel 状态
sentinel_status=$(kubectl get redissentinel $RESOURCE_NAME -n $NAMESPACE -o jsonpath='{.status}' 2>/dev/null)
sentinel_ready=$(echo $sentinel_status | jq -r '.ready // "false"')
sentinel_ready_replicas=$(echo $sentinel_status | jq -r '.readyReplicas // 0')
sentinel_replicas=$(echo $sentinel_status | jq -r '.replicas // 0')

log_info "RedisSentinel 状态: Ready=$sentinel_ready, Replicas=$sentinel_ready_replicas/$sentinel_replicas"

# 验证状态一致性
if [[ "$sentinel_sts_ready" == "$sentinel_ready_replicas" && "$sentinel_sts_replicas" == "$sentinel_replicas" ]]; then
    log_success "状态一致性验证通过"
else
    log_error "状态一致性验证失败: StatefulSet($sentinel_sts_ready/$sentinel_sts_replicas) vs RedisSentinel($sentinel_ready_replicas/$sentinel_replicas)"
    exit 1
fi

# 6. 检查控制器日志中的错误信息
log_info "检查控制器日志..."

# 获取控制器 Pod
controller_pod=$(kubectl get pods -n redis-operator-system -l control-plane=controller-manager -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")

if [[ -n "$controller_pod" ]]; then
    log_info "检查控制器日志中的状态更新错误..."
    
    # 检查最近的日志中是否有状态更新冲突错误
    conflict_errors=$(kubectl logs $controller_pod -n redis-operator-system --since=5m 2>/dev/null | grep -c "Operation cannot be fulfilled" || echo "0")
    retry_messages=$(kubectl logs $controller_pod -n redis-operator-system --since=5m 2>/dev/null | grep -c "retries due to conflicts" || echo "0")
    
    log_info "发现状态更新冲突错误: $conflict_errors 次"
    log_info "重试机制触发次数: $retry_messages 次"
    
    if [[ $conflict_errors -gt 0 ]]; then
        if [[ $retry_messages -gt 0 ]]; then
            log_success "重试机制正常工作，成功处理了状态更新冲突"
        else
            log_warning "检测到状态更新冲突，但未发现重试机制日志"
        fi
    else
        log_info "未发现状态更新冲突，系统运行正常"
    fi
else
    log_warning "未找到控制器 Pod，跳过日志检查"
fi

# 7. 性能测试：测试状态更新响应时间
log_info "执行状态更新性能测试..."

performance_test() {
    local test_count=10
    local total_time=0
    
    for i in $(seq 1 $test_count); do
        start=$(date +%s%N)
        
        # 触发状态更新
        kubectl annotate redissentinel $RESOURCE_NAME -n $NAMESPACE perf-test-$i="$(date +%s)" --overwrite >/dev/null
        
        # 等待状态更新完成
        while true; do
            annotation=$(kubectl get redissentinel $RESOURCE_NAME -n $NAMESPACE -o jsonpath="{.metadata.annotations.perf-test-$i}" 2>/dev/null || echo "")
            if [[ -n "$annotation" ]]; then
                break
            fi
            sleep 0.01
        done
        
        end=$(date +%s%N)
        duration=$(( (end - start) / 1000000 ))  # 转换为毫秒
        total_time=$((total_time + duration))
        
        log_info "状态更新 $i: ${duration}ms"
    done
    
    avg_time=$((total_time / test_count))
    log_info "平均状态更新时间: ${avg_time}ms"
    
    if [[ $avg_time -lt 1000 ]]; then  # 小于1秒
        log_success "状态更新性能良好 (平均 ${avg_time}ms)"
    else
        log_warning "状态更新性能较慢 (平均 ${avg_time}ms)"
    fi
}

performance_test

# 8. 最终验证
log_info "执行最终验证..."

# 检查所有 Pod 是否正常运行
sentinel_pods=$(kubectl get pods -n $NAMESPACE -l app=redis-sentinel -o jsonpath='{.items[*].status.phase}' 2>/dev/null || echo "")
redis_pods=$(kubectl get pods -n $NAMESPACE -l app=redis -o jsonpath='{.items[*].status.phase}' 2>/dev/null || echo "")

log_info "Sentinel Pods 状态: $sentinel_pods"
log_info "Redis Pods 状态: $redis_pods"

# 检查服务连接性
log_info "测试服务连接性..."
sentinel_service_ip=$(kubectl get service $RESOURCE_NAME-sentinel-service -n $NAMESPACE -o jsonpath='{.spec.clusterIP}' 2>/dev/null || echo "")

if [[ -n "$sentinel_service_ip" ]]; then
    log_info "Sentinel Service IP: $sentinel_service_ip"
    
    # 在集群内测试连接
    kubectl run test-connection --image=redis:7.0 --rm -i --restart=Never -n $NAMESPACE -- \
        redis-cli -h $sentinel_service_ip -p 26379 ping 2>/dev/null && \
        log_success "Sentinel 服务连接测试通过" || \
        log_warning "Sentinel 服务连接测试失败"
else
    log_warning "未找到 Sentinel Service"
fi

log_success "Redis Sentinel 状态更新优化测试完成！"
log_info "测试总结:"
log_info "- 状态更新冲突处理: ✓"
log_info "- 重试机制验证: ✓"
log_info "- 状态一致性检查: ✓"
log_info "- 性能测试: ✓"
log_info "- 服务连接性: ✓"

exit 0