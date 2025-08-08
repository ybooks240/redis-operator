#!/bin/bash

# 测试 StatefulSet 副本数和资源配置更新功能

set -e

echo "=== 测试 StatefulSet 副本数和资源配置更新功能 ==="

# 清理旧资源
echo "清理旧资源..."
kubectl delete redisinstance test-sts-update --ignore-not-found=true
kubectl delete configmap test-sts-update --ignore-not-found=true
kubectl delete service test-sts-update --ignore-not-found=true
kubectl delete statefulset test-sts-update --ignore-not-found=true
kubectl delete pvc redis-data-test-sts-update-0 --ignore-not-found=true

# 等待资源清理完成
echo "等待资源清理完成..."
sleep 10

# 创建初始 RedisInstance
echo "创建初始 RedisInstance..."
cat <<EOF | kubectl apply -f -
apiVersion: redis.github.com/v1
kind: RedisInstance
metadata:
  name: test-sts-update
spec:
  image: redis:7.0
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
    maxmemory: 100mb
    maxmemory-policy: allkeys-lru
EOF

# 等待初始资源创建
echo "等待初始资源创建..."
sleep 15

# 检查初始状态
echo "\n=== 检查初始状态 ==="
echo "初始副本数:"
kubectl get statefulset test-sts-update -o jsonpath='{.spec.replicas}'
echo

echo "初始镜像:"
kubectl get statefulset test-sts-update -o jsonpath='{.spec.template.spec.containers[0].image}'
echo

echo "初始资源配置:"
kubectl get statefulset test-sts-update -o jsonpath='{.spec.template.spec.containers[0].resources}' | jq .

echo "初始 StatefulSet UID:"
INITIAL_STS_UID=$(kubectl get statefulset test-sts-update -o jsonpath='{.metadata.uid}')
echo "Initial StatefulSet UID: $INITIAL_STS_UID"

# 测试1: 修改副本数
echo "\n=== 测试1: 修改副本数 ==="
echo "将副本数从 1 改为 3..."
kubectl patch redisinstance test-sts-update --type='merge' -p='{"spec":{"replicas":3}}'

# 等待更新
echo "等待副本数更新..."
sleep 10

# 检查副本数是否更新
echo "检查副本数更新结果:"
NEW_REPLICAS=$(kubectl get statefulset test-sts-update -o jsonpath='{.spec.replicas}')
echo "New replicas: $NEW_REPLICAS"

NEW_STS_UID_1=$(kubectl get statefulset test-sts-update -o jsonpath='{.metadata.uid}')
echo "New StatefulSet UID: $NEW_STS_UID_1"

if [ "$NEW_REPLICAS" = "3" ]; then
    echo "✅ 副本数更新成功"
else
    echo "❌ 副本数更新失败，期望: 3，实际: $NEW_REPLICAS"
    exit 1
fi

if [ "$NEW_STS_UID_1" != "$INITIAL_STS_UID" ]; then
    echo "✅ StatefulSet 已重新创建（UID 已改变）"
else
    echo "❌ StatefulSet 未重新创建（UID 未改变）"
    exit 1
fi

# 测试2: 修改镜像版本
echo "\n=== 测试2: 修改镜像版本 ==="
echo "将镜像从 redis:7.0 改为 redis:7.2..."
kubectl patch redisinstance test-sts-update --type='merge' -p='{"spec":{"image":"redis:7.2"}}'

# 等待更新
echo "等待镜像更新..."
sleep 10

# 检查镜像是否更新
echo "检查镜像更新结果:"
NEW_IMAGE=$(kubectl get statefulset test-sts-update -o jsonpath='{.spec.template.spec.containers[0].image}')
echo "New image: $NEW_IMAGE"

NEW_STS_UID_2=$(kubectl get statefulset test-sts-update -o jsonpath='{.metadata.uid}')
echo "New StatefulSet UID: $NEW_STS_UID_2"

if [ "$NEW_IMAGE" = "redis:7.2" ]; then
    echo "✅ 镜像更新成功"
else
    echo "❌ 镜像更新失败，期望: redis:7.2，实际: $NEW_IMAGE"
    exit 1
fi

if [ "$NEW_STS_UID_2" != "$NEW_STS_UID_1" ]; then
    echo "✅ StatefulSet 已重新创建（UID 已改变）"
else
    echo "❌ StatefulSet 未重新创建（UID 未改变）"
    exit 1
fi

# 测试3: 修改资源配置
echo "\n=== 测试3: 修改资源配置 ==="
echo "修改资源配置..."
kubectl patch redisinstance test-sts-update --type='merge' -p='{
  "spec": {
    "resources": {
      "requests": {
        "memory": "256Mi",
        "cpu": "200m"
      },
      "limits": {
        "memory": "512Mi",
        "cpu": "400m"
      }
    }
  }
}'

# 等待更新
echo "等待资源配置更新..."
sleep 10

# 检查资源配置是否更新
echo "检查资源配置更新结果:"
NEW_RESOURCES=$(kubectl get statefulset test-sts-update -o jsonpath='{.spec.template.spec.containers[0].resources}')
echo "New resources: $NEW_RESOURCES" | jq .

NEW_STS_UID_3=$(kubectl get statefulset test-sts-update -o jsonpath='{.metadata.uid}')
echo "New StatefulSet UID: $NEW_STS_UID_3"

# 检查内存限制是否正确
NEW_MEMORY_LIMIT=$(kubectl get statefulset test-sts-update -o jsonpath='{.spec.template.spec.containers[0].resources.limits.memory}')
if [ "$NEW_MEMORY_LIMIT" = "512Mi" ]; then
    echo "✅ 资源配置更新成功"
else
    echo "❌ 资源配置更新失败，期望内存限制: 512Mi，实际: $NEW_MEMORY_LIMIT"
    exit 1
fi

if [ "$NEW_STS_UID_3" != "$NEW_STS_UID_2" ]; then
    echo "✅ StatefulSet 已重新创建（UID 已改变）"
else
    echo "❌ StatefulSet 未重新创建（UID 未改变）"
    exit 1
fi

# 测试4: 混合修改（同时修改多个参数）
echo "\n=== 测试4: 混合修改 ==="
echo "同时修改副本数、镜像和配置..."
kubectl patch redisinstance test-sts-update --type='merge' -p='{
  "spec": {
    "replicas": 2,
    "image": "redis:7.0",
    "config": {
      "maxmemory": "200mb",
      "maxmemory-policy": "allkeys-lru",
      "timeout": "300"
    }
  }
}'

# 等待更新
echo "等待混合更新..."
sleep 15

# 检查所有更新是否生效
echo "检查混合更新结果:"
FINAL_REPLICAS=$(kubectl get statefulset test-sts-update -o jsonpath='{.spec.replicas}')
FINAL_IMAGE=$(kubectl get statefulset test-sts-update -o jsonpath='{.spec.template.spec.containers[0].image}')
FINAL_CONFIG=$(kubectl get configmap test-sts-update -o jsonpath='{.data.redis\.conf}')

echo "Final replicas: $FINAL_REPLICAS"
echo "Final image: $FINAL_IMAGE"
echo "Final config contains timeout:"
echo "$FINAL_CONFIG" | grep -q "timeout 300" && echo "✅ 配置包含 timeout 300" || echo "❌ 配置不包含 timeout 300"

FINAL_STS_UID=$(kubectl get statefulset test-sts-update -o jsonpath='{.metadata.uid}')
echo "Final StatefulSet UID: $FINAL_STS_UID"

if [ "$FINAL_REPLICAS" = "2" ] && [ "$FINAL_IMAGE" = "redis:7.0" ]; then
    echo "✅ 混合更新成功"
else
    echo "❌ 混合更新失败"
    exit 1
fi

if [ "$FINAL_STS_UID" != "$NEW_STS_UID_3" ]; then
    echo "✅ StatefulSet 已重新创建（UID 已改变）"
else
    echo "❌ StatefulSet 未重新创建（UID 未改变）"
    exit 1
fi

# 检查最终状态
echo "\n=== 检查最终状态 ==="
echo "StatefulSet 状态:"
kubectl get statefulset test-sts-update

echo "\nPods 状态:"
kubectl get pods -l app=test-sts-update

echo "\nRedisInstance 状态:"
kubectl get redisinstance test-sts-update -o yaml | grep -A 10 "status:"

# 清理测试资源
echo "\n=== 清理测试资源 ==="
kubectl delete redisinstance test-sts-update
kubectl delete pvc redis-data-test-sts-update-0 redis-data-test-sts-update-1 --ignore-not-found=true

echo "\n🎉 StatefulSet 更新功能测试完成！"
echo "✅ 副本数变化检测和更新正常"
echo "✅ 镜像版本变化检测和更新正常"
echo "✅ 资源配置变化检测和更新正常"
echo "✅ 混合变化检测和更新正常"
echo "✅ 所有变化都触发了 StatefulSet 重新创建"
echo "✅ 修复了 StatefulSet 更新问题"