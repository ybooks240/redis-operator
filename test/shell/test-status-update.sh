#!/bin/bash

# 测试 RedisInstance 状态更新功能
# 验证配置变化时状态是否正确设置为 RedisPhaseUpdating

set -e

echo "=== RedisInstance 状态更新功能测试 ==="

# 清理函数
cleanup() {
    echo "\n=== 清理测试资源 ==="
    kubectl delete redisinstance test-status-update --ignore-not-found=true
    kubectl delete configmap test-status-update --ignore-not-found=true
    kubectl delete service test-status-update --ignore-not-found=true
    kubectl delete statefulset test-status-update --ignore-not-found=true
    echo "清理完成"
}

# 设置清理陷阱
trap cleanup EXIT

# 检查状态的辅助函数
check_status() {
    local expected_status="$1"
    local description="$2"
    local max_wait="${3:-30}"
    
    echo "\n--- 检查状态: $description ---"
    
    local count=0
    while [ $count -lt $max_wait ]; do
        local current_status=$(kubectl get redisinstance test-status-update -o jsonpath='{.status.status}' 2>/dev/null || echo "")
        local ready_status=$(kubectl get redisinstance test-status-update -o jsonpath='{.status.ready}' 2>/dev/null || echo "")
        local message=$(kubectl get redisinstance test-status-update -o jsonpath='{.status.lastConditionMessage}' 2>/dev/null || echo "")
        
        echo "当前状态: $current_status, Ready: $ready_status"
        echo "消息: $message"
        
        if [ "$current_status" = "$expected_status" ]; then
            echo "✅ 状态检查通过: $expected_status"
            if [ "$expected_status" = "Updating" ]; then
                if [ "$ready_status" = "False" ]; then
                    echo "✅ Ready状态正确: False"
                else
                    echo "❌ Ready状态错误，期望: False, 实际: $ready_status"
                    return 1
                fi
            fi
            return 0
        fi
        
        sleep 2
        count=$((count + 1))
    done
    
    echo "❌ 状态检查失败，期望: $expected_status, 实际: $current_status"
    return 1
}

# 等待StatefulSet就绪的辅助函数
wait_for_statefulset_ready() {
    local max_wait="${1:-60}"
    echo "\n--- 等待 StatefulSet 就绪 ---"
    
    local count=0
    while [ $count -lt $max_wait ]; do
        local ready_replicas=$(kubectl get statefulset test-status-update -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
        local replicas=$(kubectl get statefulset test-status-update -o jsonpath='{.status.replicas}' 2>/dev/null || echo "0")
        
        echo "StatefulSet 状态: $ready_replicas/$replicas ready"
        
        if [ "$ready_replicas" = "$replicas" ] && [ "$replicas" != "0" ]; then
            echo "✅ StatefulSet 已就绪"
            return 0
        fi
        
        sleep 3
        count=$((count + 1))
    done
    
    echo "❌ StatefulSet 未能在预期时间内就绪"
    return 1
}

echo "\n=== 步骤 1: 创建初始 RedisInstance ==="
cat <<EOF | kubectl apply -f -
apiVersion: redis.github.com/v1
kind: RedisInstance
metadata:
  name: test-status-update
spec:
  image: redis:7.0
  replicas: 1
  storage:
    size: 1Gi
    storageClassName: standard
  resources:
    requests:
      memory: "128Mi"
      cpu: "100m"
    limits:
      memory: "256Mi"
      cpu: "200m"
  config:
    maxmemory: "100mb"
    maxmemory-policy: "allkeys-lru"
EOF

echo "等待初始部署完成..."
sleep 5

# 等待StatefulSet创建并就绪
wait_for_statefulset_ready 60

# 检查初始状态应该是Running
check_status "Running" "初始部署完成后" 30

echo "\n=== 步骤 2: 测试副本数变化 ==="
echo "修改副本数从 1 到 2"
kubectl patch redisinstance test-status-update --type='merge' -p='{"spec":{"replicas":2}}'

# 检查状态是否变为Updating
check_status "Updating" "副本数变化后" 10

# 等待更新完成
echo "等待副本数更新完成..."
wait_for_statefulset_ready 60

# 检查状态是否恢复为Running
check_status "Running" "副本数更新完成后" 30

# 验证副本数确实变为2
actual_replicas=$(kubectl get statefulset test-status-update -o jsonpath='{.spec.replicas}')
if [ "$actual_replicas" = "2" ]; then
    echo "✅ 副本数更新成功: $actual_replicas"
else
    echo "❌ 副本数更新失败，期望: 2, 实际: $actual_replicas"
    exit 1
fi

echo "\n=== 步骤 3: 测试镜像版本变化 ==="
echo "修改镜像版本从 redis:7.0 到 redis:7.2"
kubectl patch redisinstance test-status-update --type='merge' -p='{"spec":{"image":"redis:7.2"}}'

# 检查状态是否变为Updating
check_status "Updating" "镜像版本变化后" 10

# 等待更新完成
echo "等待镜像更新完成..."
wait_for_statefulset_ready 90

# 检查状态是否恢复为Running
check_status "Running" "镜像更新完成后" 30

# 验证镜像确实变为redis:7.2
actual_image=$(kubectl get statefulset test-status-update -o jsonpath='{.spec.template.spec.containers[0].image}')
if [ "$actual_image" = "redis:7.2" ]; then
    echo "✅ 镜像更新成功: $actual_image"
else
    echo "❌ 镜像更新失败，期望: redis:7.2, 实际: $actual_image"
    exit 1
fi

echo "\n=== 步骤 4: 测试资源配置变化 ==="
echo "修改内存限制从 256Mi 到 512Mi"
kubectl patch redisinstance test-status-update --type='merge' -p='{"spec":{"resources":{"limits":{"memory":"512Mi"}}}}'

# 检查状态是否变为Updating
check_status "Updating" "资源配置变化后" 10

# 等待更新完成
echo "等待资源配置更新完成..."
wait_for_statefulset_ready 90

# 检查状态是否恢复为Running
check_status "Running" "资源配置更新完成后" 30

# 验证内存限制确实变为512Mi
actual_memory=$(kubectl get statefulset test-status-update -o jsonpath='{.spec.template.spec.containers[0].resources.limits.memory}')
if [ "$actual_memory" = "512Mi" ]; then
    echo "✅ 资源配置更新成功: $actual_memory"
else
    echo "❌ 资源配置更新失败，期望: 512Mi, 实际: $actual_memory"
    exit 1
fi

echo "\n=== 步骤 5: 测试配置文件变化（需要重建）==="
echo "修改 Redis 配置，添加新的超时设置"
kubectl patch redisinstance test-status-update --type='merge' -p='{"spec":{"config":{"timeout":"300"}}}'

# 检查状态是否变为Updating
check_status "Updating" "配置文件变化后" 10

# 等待重建完成
echo "等待配置文件更新（重建）完成..."
wait_for_statefulset_ready 120

# 检查状态是否恢复为Running
check_status "Running" "配置文件更新完成后" 30

# 验证配置确实包含timeout设置
echo "验证配置文件是否包含新的timeout设置..."
sleep 5
config_content=$(kubectl get configmap test-status-update -o jsonpath='{.data.redis\.conf}' 2>/dev/null || echo "")
if echo "$config_content" | grep -q "timeout 300"; then
    echo "✅ 配置文件更新成功，包含 timeout 300"
else
    echo "❌ 配置文件更新失败，未找到 timeout 300"
    echo "当前配置内容:"
    echo "$config_content"
    exit 1
fi

echo "\n=== 步骤 6: 检查状态历史记录 ==="
echo "查看 RedisInstance 的状态条件历史:"
kubectl get redisinstance test-status-update -o jsonpath='{.status.conditions}' | jq '.' 2>/dev/null || echo "无法解析状态条件"

echo "\n=== 测试总结 ==="
echo "✅ 所有状态更新测试通过！"
echo "✅ 副本数变化时状态正确设置为 Updating"
echo "✅ 镜像版本变化时状态正确设置为 Updating"
echo "✅ 资源配置变化时状态正确设置为 Updating"
echo "✅ 配置文件变化时状态正确设置为 Updating"
echo "✅ 更新完成后状态正确恢复为 Running"
echo "✅ Ready 状态在更新期间正确设置为 False"

echo "\n=== 功能验证完成 ==="