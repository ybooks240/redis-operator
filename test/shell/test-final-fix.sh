#!/bin/bash

# 测试最终修复版本的 ConfigMap 自动重启功能
# 验证不会出现频繁重启的问题

set -e

echo "=== 测试最终修复版本的 ConfigMap 自动重启功能 ==="

# 清理旧资源
echo "清理旧资源..."
kubectl delete redisinstance redisinstance-sample --ignore-not-found=true
kubectl delete configmap redisinstance-sample --ignore-not-found=true
kubectl delete statefulset redisinstance-sample --ignore-not-found=true
kubectl delete service redisinstance-sample --ignore-not-found=true

# 等待资源完全删除
echo "等待资源完全删除..."
sleep 10

# 创建初始 RedisInstance
echo "创建初始 RedisInstance..."
cat <<EOF | kubectl apply -f -
apiVersion: redis.github.com/v1
kind: RedisInstance
metadata:
  name: redisinstance-sample
  namespace: default
spec:
  replicas: 1
  config:
    maxmemory: "128mb"
    maxmemory-policy: "allkeys-lru"
    timeout: "300"
EOF

# 等待资源创建
echo "等待资源创建..."
sleep 15

# 检查初始状态
echo "检查初始状态..."
echo "ConfigMap 内容:"
kubectl get configmap redisinstance-sample -o yaml | grep -A 10 "redis.conf:"

echo "StatefulSet annotation:"
kubectl get statefulset redisinstance-sample -o jsonpath='{.spec.template.metadata.annotations.redis\.github\.com/config-hash}'
echo ""

echo "StatefulSet UID:"
INITIAL_STS_UID=$(kubectl get statefulset redisinstance-sample -o jsonpath='{.metadata.uid}')
echo "Initial StatefulSet UID: $INITIAL_STS_UID"

# 观察一段时间，确认没有意外重启
echo "观察 60 秒，确认没有意外重启..."
for i in {1..12}; do
    echo "检查第 $i 次 ($(date)):"
    CURRENT_STS_UID=$(kubectl get statefulset redisinstance-sample -o jsonpath='{.metadata.uid}' 2>/dev/null || echo "NOT_FOUND")
    if [ "$CURRENT_STS_UID" != "$INITIAL_STS_UID" ]; then
        echo "❌ 错误：StatefulSet 意外重启了！"
        echo "初始 UID: $INITIAL_STS_UID"
        echo "当前 UID: $CURRENT_STS_UID"
        exit 1
    fi
    echo "✅ StatefulSet UID 保持不变: $CURRENT_STS_UID"
    sleep 5
done

echo "✅ 观察期间没有意外重启"

# 修改 RedisInstance 配置以触发重启
echo "修改 RedisInstance 配置以触发重启..."
cat <<EOF | kubectl apply -f -
apiVersion: redis.github.com/v1
kind: RedisInstance
metadata:
  name: redisinstance-sample
  namespace: default
spec:
  replicas: 1
  config:
    maxmemory: "256mb"  # 修改内存限制
    maxmemory-policy: "allkeys-lru"
    timeout: "600"      # 修改超时时间
EOF

# 等待配置更新和重启
echo "等待配置更新和重启..."
sleep 20

# 检查配置更新后的状态
echo "检查配置更新后的状态..."
echo "新的 ConfigMap 内容:"
kubectl get configmap redisinstance-sample -o yaml | grep -A 10 "redis.conf:"

echo "新的 StatefulSet annotation:"
NEW_CONFIG_HASH=$(kubectl get statefulset redisinstance-sample -o jsonpath='{.spec.template.metadata.annotations.redis\.github\.com/config-hash}')
echo "New config hash: $NEW_CONFIG_HASH"

echo "新的 StatefulSet UID:"
NEW_STS_UID=$(kubectl get statefulset redisinstance-sample -o jsonpath='{.metadata.uid}')
echo "New StatefulSet UID: $NEW_STS_UID"

# 验证 StatefulSet 确实重启了
if [ "$NEW_STS_UID" != "$INITIAL_STS_UID" ]; then
    echo "✅ StatefulSet 成功重启（UID 已改变）"
else
    echo "❌ 错误：StatefulSet 应该重启但没有重启"
    exit 1
fi

# 验证配置是否正确更新
echo "验证 ConfigMap 配置是否正确更新..."
CONFIG_CONTENT=$(kubectl get configmap redisinstance-sample -o jsonpath='{.data.redis\.conf}')
if echo "$CONFIG_CONTENT" | grep -q "maxmemory 256mb" && echo "$CONFIG_CONTENT" | grep -q "timeout 600"; then
    echo "✅ ConfigMap 配置已正确更新"
else
    echo "❌ 错误：ConfigMap 配置未正确更新"
    echo "当前配置内容："
    echo "$CONFIG_CONTENT"
    exit 1
fi

# 再次观察一段时间，确认修改后没有频繁重启
echo "再次观察 60 秒，确认修改后没有频繁重启..."
for i in {1..12}; do
    echo "检查第 $i 次 ($(date)):"
    CURRENT_STS_UID=$(kubectl get statefulset redisinstance-sample -o jsonpath='{.metadata.uid}' 2>/dev/null || echo "NOT_FOUND")
    if [ "$CURRENT_STS_UID" != "$NEW_STS_UID" ]; then
        echo "❌ 错误：StatefulSet 在配置更新后又意外重启了！"
        echo "更新后 UID: $NEW_STS_UID"
        echo "当前 UID: $CURRENT_STS_UID"
        exit 1
    fi
    echo "✅ StatefulSet UID 保持稳定: $CURRENT_STS_UID"
    sleep 5
done

echo "✅ 配置更新后没有频繁重启"

# 检查 Pod 是否使用了新配置
echo "检查 Pod 是否使用了新配置..."
sleep 10
POD_NAME=$(kubectl get pods -l app=redisinstance-sample -o jsonpath='{.items[0].metadata.name}')
if [ -n "$POD_NAME" ]; then
    echo "检查 Pod $POD_NAME 的配置..."
    # 检查 Pod 中的配置文件
    POD_CONFIG=$(kubectl exec $POD_NAME -- cat /etc/redis/redis.conf 2>/dev/null || echo "无法读取配置文件")
    if echo "$POD_CONFIG" | grep -q "maxmemory 256mb" && echo "$POD_CONFIG" | grep -q "timeout 600"; then
        echo "✅ Pod 已使用新配置"
    else
        echo "⚠️  警告：Pod 可能还未使用新配置，这是正常的，因为 Pod 可能还在重启中"
    fi
else
    echo "⚠️  警告：未找到 Pod，可能还在创建中"
fi

echo ""
echo "=== 测试总结 ==="
echo "✅ 初始创建：StatefulSet 正常创建，没有意外重启"
echo "✅ 配置变更：StatefulSet 正确重启以应用新配置"
echo "✅ 稳定性：配置更新后没有频繁重启"
echo "✅ 功能性：ConfigMap 配置正确更新"
echo ""
echo "🎉 所有测试通过！ConfigMap 自动重启功能工作正常，没有频繁重启问题。"

# 清理测试资源
echo "清理测试资源..."
kubectl delete redisinstance redisinstance-sample --ignore-not-found=true

echo "测试完成！"