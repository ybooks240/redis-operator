#!/bin/bash

# Redis 聚合视图自动创建脚本
# 该脚本会扫描集群中的所有 Redis 资源，并为它们创建对应的聚合视图

set -e

echo "=== Redis 聚合视图自动创建脚本 ==="
echo

# 函数：创建聚合视图
create_aggregated_view() {
    local resource_type=$1
    local resource_name=$2
    local resource_namespace=$3
    local view_name="${resource_name}-view"
    
    echo "创建 ${resource_type} 聚合视图: ${view_name}"
    
    cat <<EOF | kubectl apply -f -
apiVersion: redis.github.com/v1
kind: Redis
metadata:
  name: ${view_name}
  namespace: ${resource_namespace}
spec:
  type: ${resource_type}
  resourceName: ${resource_name}
  resourceNamespace: ${resource_namespace}
EOF
}

# 扫描 RedisCluster 资源
echo "扫描 RedisCluster 资源..."
kubectl get rediscluster --all-namespaces --no-headers 2>/dev/null | while read namespace name rest; do
    if [ ! -z "$name" ]; then
        create_aggregated_view "cluster" "$name" "$namespace"
    fi
done

# 扫描 RedisInstance 资源
echo "扫描 RedisInstance 资源..."
kubectl get redisinstance --all-namespaces --no-headers 2>/dev/null | while read namespace name rest; do
    if [ ! -z "$name" ]; then
        create_aggregated_view "instance" "$name" "$namespace"
    fi
done

# 扫描 RedisMasterReplica 资源
echo "扫描 RedisMasterReplica 资源..."
kubectl get redismasterreplica --all-namespaces --no-headers 2>/dev/null | while read namespace name rest; do
    if [ ! -z "$name" ]; then
        create_aggregated_view "masterreplica" "$name" "$namespace"
    fi
done

# 扫描 RedisSentinel 资源
echo "扫描 RedisSentinel 资源..."
kubectl get redissentinel --all-namespaces --no-headers 2>/dev/null | while read namespace name rest; do
    if [ ! -z "$name" ]; then
        create_aggregated_view "sentinel" "$name" "$namespace"
    fi
done

echo
echo "=== 聚合视图创建完成 ==="
echo
echo "查看所有 Redis 聚合资源:"
kubectl get redis --all-namespaces

echo
echo "查看详细信息:"
kubectl get redis --all-namespaces -o wide