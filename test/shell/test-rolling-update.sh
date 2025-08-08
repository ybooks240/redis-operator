#!/bin/bash

# 测试 StatefulSet 滚动更新功能
# 验证副本数、镜像、资源配置变化时使用滚动更新而不是重建

set -e

echo "=== 测试 StatefulSet 滚动更新功能 ==="

# 清理旧资源
echo "清理旧资源..."
kubectl delete redisinstance test-rolling-update --ignore-not-found=true
kubectl delete statefulset test-rolling-update --ignore-not-found=true
kubectl delete configmap test-rolling-update --ignore-not-found=true
kubectl delete service test-rolling-update --ignore-not-found=true
kubectl delete pvc redis-data-test-rolling-update-0 --ignore-not-found=true

# 等待资源清理完成
echo "等待资源清理完成..."
sleep 10

# 创建初始 RedisInstance
echo "\n=== 创建初始 RedisInstance ==="
cat <<EOF | kubectl apply -f -
apiVersion: redis.github.com/v1
kind: RedisInstance
metadata:
  name: test-rolling-update
spec:
  image: redis:6.2
  replicas: 1
  resources:
    requests:
      memory: "128Mi"
      cpu: "100m"
    limits:
      memory: "256Mi"
      cpu: "200m"
  storage:
    size: 1Gi
    storageClassName: standard
  config:
    maxmemory: "100mb"
    timeout: 300
EOF

# 等待初始部署完成
echo "等待初始部署完成..."
sleep 15

# 检查初始状态
echo "\n=== 检查初始状态 ==="
echo "初始副本数:"
kubectl get statefulset test-rolling-update -o jsonpath='{.spec.replicas}'
echo "\n初始镜像:"
kubectl get statefulset test-rolling-update -o jsonpath='{.spec.template.spec.containers[0].image}'
echo "\n初始内存限制:"
kubectl get statefulset test-rolling-update -o jsonpath='{.spec.template.spec.containers[0].resources.limits.memory}'
echo "\n初始 StatefulSet UID:"
INITIAL_STS_UID=$(kubectl get statefulset test-rolling-update -o jsonpath='{.metadata.uid}')
echo "Initial StatefulSet UID: $INITIAL_STS_UID"

# 测试1: 修改副本数（应该使用滚动更新）
echo "\n=== 测试1: 修改副本数（滚动更新）==="
echo "将副本数从 1 改为 3..."
kubectl patch redisinstance test-rolling-update --type='merge' -p='{"spec":{"replicas":3}}'

# 等待更新
sleep 10

# 检查副本数是否更新
NEW_REPLICAS=$(kubectl get statefulset test-rolling-update -o jsonpath='{.spec.replicas}')
echo "新副本数: $NEW_REPLICAS"

# 检查 StatefulSet UID 是否保持不变（滚动更新）
NEW_STS_UID=$(kubectl get statefulset test-rolling-update -o jsonpath='{.metadata.uid}')
echo "New StatefulSet UID: $NEW_STS_UID"

if [ "$NEW_REPLICAS" = "3" ] && [ "$INITIAL_STS_UID" = "$NEW_STS_UID" ]; then
    echo "✅ 副本数滚动更新成功（StatefulSet 未重建）"
else
    echo "❌ 副本数更新失败或 StatefulSet 被重建"
    exit 1
fi

# 测试2: 修改镜像版本（应该使用滚动更新）
echo "\n=== 测试2: 修改镜像版本（滚动更新）==="
echo "将镜像从 redis:6.2 改为 redis:7.0..."
kubectl patch redisinstance test-rolling-update --type='merge' -p='{"spec":{"image":"redis:7.0"}}'

# 等待更新
sleep 10

# 检查镜像是否更新
NEW_IMAGE=$(kubectl get statefulset test-rolling-update -o jsonpath='{.spec.template.spec.containers[0].image}')
echo "新镜像: $NEW_IMAGE"

# 检查 StatefulSet UID 是否保持不变（滚动更新）
NEW_STS_UID2=$(kubectl get statefulset test-rolling-update -o jsonpath='{.metadata.uid}')
echo "New StatefulSet UID: $NEW_STS_UID2"

if [ "$NEW_IMAGE" = "redis:7.0" ] && [ "$INITIAL_STS_UID" = "$NEW_STS_UID2" ]; then
    echo "✅ 镜像滚动更新成功（StatefulSet 未重建）"
else
    echo "❌ 镜像更新失败或 StatefulSet 被重建"
    exit 1
fi

# 测试3: 修改资源配置（应该使用滚动更新）
echo "\n=== 测试3: 修改资源配置（滚动更新）==="
echo "修改内存限制从 256Mi 到 512Mi..."
kubectl patch redisinstance test-rolling-update --type='merge' -p='{"spec":{"resources":{"limits":{"memory":"512Mi","cpu":"200m"},"requests":{"memory":"128Mi","cpu":"100m"}}}}'

# 等待更新
sleep 10

# 检查资源配置是否更新
NEW_MEMORY_LIMIT=$(kubectl get statefulset test-rolling-update -o jsonpath='{.spec.template.spec.containers[0].resources.limits.memory}')
echo "新内存限制: $NEW_MEMORY_LIMIT"

# 检查 StatefulSet UID 是否保持不变（滚动更新）
NEW_STS_UID3=$(kubectl get statefulset test-rolling-update -o jsonpath='{.metadata.uid}')
echo "New StatefulSet UID: $NEW_STS_UID3"

if [ "$NEW_MEMORY_LIMIT" = "512Mi" ] && [ "$INITIAL_STS_UID" = "$NEW_STS_UID3" ]; then
    echo "✅ 资源配置滚动更新成功（StatefulSet 未重建）"
else
    echo "❌ 资源配置更新失败或 StatefulSet 被重建"
    exit 1
fi

# 测试4: 修改配置文件（应该重建 StatefulSet）
echo "\n=== 测试4: 修改配置文件（重建）==="
echo "修改配置文件，添加新的配置项..."
kubectl patch redisinstance test-rolling-update --type='merge' -p='{"spec":{"config":{"maxmemory":"200mb","timeout":600,"save":"900 1"}}}'

# 等待更新
sleep 15

# 检查配置是否更新
NEW_CONFIG=$(kubectl get configmap test-rolling-update -o jsonpath='{.data.redis\.conf}')
echo "新配置内容:"
echo "$NEW_CONFIG" | grep -E "(maxmemory|timeout|save)" || echo "配置检查失败"

# 检查 StatefulSet UID 是否改变（重建）
NEW_STS_UID4=$(kubectl get statefulset test-rolling-update -o jsonpath='{.metadata.uid}')
echo "New StatefulSet UID: $NEW_STS_UID4"

if [ "$INITIAL_STS_UID" != "$NEW_STS_UID4" ]; then
    echo "✅ 配置文件变化触发 StatefulSet 重建成功"
else
    echo "❌ 配置文件变化未触发 StatefulSet 重建"
    exit 1
fi

# 检查最终状态
echo "\n=== 检查最终状态 ==="
FINAL_REPLICAS=$(kubectl get statefulset test-rolling-update -o jsonpath='{.spec.replicas}')
FINAL_IMAGE=$(kubectl get statefulset test-rolling-update -o jsonpath='{.spec.template.spec.containers[0].image}')
FINAL_MEMORY=$(kubectl get statefulset test-rolling-update -o jsonpath='{.spec.template.spec.containers[0].resources.limits.memory}')

echo "最终副本数: $FINAL_REPLICAS"
echo "最终镜像: $FINAL_IMAGE"
echo "最终内存限制: $FINAL_MEMORY"

if [ "$FINAL_REPLICAS" = "3" ] && [ "$FINAL_IMAGE" = "redis:7.0" ] && [ "$FINAL_MEMORY" = "512Mi" ]; then
    echo "✅ 所有更新都成功应用"
else
    echo "❌ 最终状态检查失败"
    exit 1
fi

# 清理测试资源
echo "\n=== 清理测试资源 ==="
kubectl delete redisinstance test-rolling-update
echo "等待资源清理..."
sleep 10

echo "\n🎉 滚动更新测试全部通过！"
echo "✅ 副本数、镜像、资源配置变化使用滚动更新（StatefulSet 未重建）"
echo "✅ 配置文件变化正确触发 StatefulSet 重建"
echo "✅ 修复了 StatefulSet 更新问题，实现了精细化的更新策略"