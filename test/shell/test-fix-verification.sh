#!/bin/bash

# 测试修复后的 ConfigMap 自动重启功能
# 验证不会出现频繁重启的问题

set -e

echo "=== 测试 ConfigMap 修改自动重启功能修复 ==="

# 清理之前的测试资源
echo "清理之前的测试资源..."
kubectl delete redisinstance test-redis-fix --ignore-not-found=true
kubectl delete configmap test-redis-fix --ignore-not-found=true
kubectl delete statefulset test-redis-fix --ignore-not-found=true
kubectl delete service test-redis-fix --ignore-not-found=true

# 等待资源清理完成
echo "等待资源清理完成..."
sleep 10

# 创建初始 RedisInstance
echo "创建初始 RedisInstance..."
cat <<EOF | kubectl apply -f -
apiVersion: redis.github.com/v1
kind: RedisInstance
metadata:
  name: test-redis-fix
  namespace: default
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
    appendfsync: "everysec"
EOF

# 等待 RedisInstance 创建完成
echo "等待 RedisInstance 创建完成..."
sleep 15

# 检查初始状态
echo "检查初始状态..."
kubectl get redisinstance test-redis-fix -o yaml
echo "\n=== ConfigMap 内容 ==="
kubectl get configmap test-redis-fix -o yaml
echo "\n=== StatefulSet Annotations ==="
kubectl get statefulset test-redis-fix -o jsonpath='{.spec.template.metadata.annotations}' | jq .

# 获取初始配置哈希值
INITIAL_HASH=$(kubectl get statefulset test-redis-fix -o jsonpath='{.spec.template.metadata.annotations.redis\.github\.com/config-hash}')
echo "初始配置哈希值: $INITIAL_HASH"

# 获取初始 StatefulSet UID
INITIAL_STS_UID=$(kubectl get statefulset test-redis-fix -o jsonpath='{.metadata.uid}')
echo "初始 StatefulSet UID: $INITIAL_STS_UID"

# 等待一段时间，观察是否有频繁重启
echo "\n=== 观察 30 秒，检查是否有频繁重启 ==="
for i in {1..6}; do
    echo "检查第 $i 次 ($(date))..."
    CURRENT_STS_UID=$(kubectl get statefulset test-redis-fix -o jsonpath='{.metadata.uid}' 2>/dev/null || echo "NOT_FOUND")
    CURRENT_HASH=$(kubectl get statefulset test-redis-fix -o jsonpath='{.spec.template.metadata.annotations.redis\.github\.com/config-hash}' 2>/dev/null || echo "NOT_FOUND")
    
    echo "  当前 StatefulSet UID: $CURRENT_STS_UID"
    echo "  当前配置哈希值: $CURRENT_HASH"
    
    if [ "$CURRENT_STS_UID" != "$INITIAL_STS_UID" ]; then
        echo "  ❌ 错误: StatefulSet 被意外重建！"
        echo "  初始 UID: $INITIAL_STS_UID"
        echo "  当前 UID: $CURRENT_STS_UID"
        exit 1
    fi
    
    if [ "$CURRENT_HASH" != "$INITIAL_HASH" ]; then
        echo "  ❌ 错误: 配置哈希值意外变化！"
        echo "  初始哈希: $INITIAL_HASH"
        echo "  当前哈希: $CURRENT_HASH"
        exit 1
    fi
    
    echo "  ✅ StatefulSet 稳定，没有意外重启"
    sleep 5
done

echo "\n=== 现在测试真正的配置修改 ==="

# 修改配置
echo "修改 RedisInstance 配置..."
cat <<EOF | kubectl apply -f -
apiVersion: redis.github.com/v1
kind: RedisInstance
metadata:
  name: test-redis-fix
  namespace: default
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
    maxmemory: "200mb"  # 修改这个值
    maxmemory-policy: "allkeys-lru"
    appendfsync: "everysec"
    tcp-keepalive: "60"  # 添加新配置
EOF

# 等待配置更新
echo "等待配置更新..."
sleep 10

# 检查配置是否正确更新
echo "\n=== 检查配置更新后的状态 ==="
NEW_HASH=$(kubectl get statefulset test-redis-fix -o jsonpath='{.spec.template.metadata.annotations.redis\.github\.com/config-hash}' 2>/dev/null || echo "NOT_FOUND")
NEW_STS_UID=$(kubectl get statefulset test-redis-fix -o jsonpath='{.metadata.uid}' 2>/dev/null || echo "NOT_FOUND")

echo "新的配置哈希值: $NEW_HASH"
echo "新的 StatefulSet UID: $NEW_STS_UID"

if [ "$NEW_HASH" = "$INITIAL_HASH" ]; then
    echo "❌ 错误: 配置哈希值没有更新！"
    exit 1
fi

if [ "$NEW_STS_UID" = "$INITIAL_STS_UID" ]; then
    echo "❌ 错误: StatefulSet 没有重建！"
    exit 1
fi

echo "✅ 配置修改后 StatefulSet 正确重建"

# 检查新的 ConfigMap 内容
echo "\n=== 检查新的 ConfigMap 内容 ==="
kubectl get configmap test-redis-fix -o jsonpath='{.data.redis\.conf}' | grep -E "maxmemory|tcp-keepalive"

echo "\n=== 测试完成 ==="
echo "✅ 所有测试通过！修复成功，不再出现频繁重启问题。"

# 清理测试资源
echo "\n清理测试资源..."
kubectl delete redisinstance test-redis-fix --ignore-not-found=true

echo "测试完成！"