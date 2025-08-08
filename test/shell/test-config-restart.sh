#!/bin/bash

# 配置修改自动重启功能测试脚本

set -e

echo "=== Redis Operator 配置修改自动重启功能测试 ==="

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 检查 kubectl 是否可用
if ! command -v kubectl &> /dev/null; then
    echo -e "${RED}错误: kubectl 未安装或不在 PATH 中${NC}"
    exit 1
fi

# 检查集群连接
if ! kubectl cluster-info &> /dev/null; then
    echo -e "${RED}错误: 无法连接到 Kubernetes 集群${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Kubernetes 集群连接正常${NC}"

# 清理函数
cleanup() {
    echo -e "${YELLOW}清理测试资源...${NC}"
    kubectl delete redisinstance redis-config-test --ignore-not-found=true
    kubectl delete configmap redis-config-test --ignore-not-found=true
    kubectl delete statefulset redis-config-test --ignore-not-found=true
    kubectl delete service redis-config-test --ignore-not-found=true
    echo -e "${GREEN}✓ 清理完成${NC}"
}

# 捕获退出信号进行清理
trap cleanup EXIT

# 步骤 1: 创建初始 RedisInstance
echo -e "${BLUE}步骤 1: 创建初始 RedisInstance${NC}"
cat <<EOF | kubectl apply -f -
apiVersion: redis.github.com/v1
kind: RedisInstance
metadata:
  name: redis-config-test
spec:
  image: redis:7.0
  replicas: 1
  storage:
    size: 1Gi
    storageClassName: standard
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 200m
      memory: 256Mi
  config:
    maxmemory: 100mb
    maxmemory-policy: allkeys-lru
    appendonly: "yes"
EOF

echo -e "${GREEN}✓ RedisInstance 已创建${NC}"

# 等待资源创建
echo -e "${YELLOW}等待 StatefulSet 就绪...${NC}"
kubectl wait --for=condition=ready pod -l app.kubernetes.io/instance=redis-config-test --timeout=120s

# 获取初始配置哈希值
echo -e "${BLUE}步骤 2: 检查初始配置${NC}"
INITIAL_HASH=$(kubectl get statefulset redis-config-test -o jsonpath='{.spec.template.metadata.annotations.redis\.github\.com/config-hash}')
echo -e "初始配置哈希值: ${INITIAL_HASH}"

# 检查 ConfigMap 内容
echo -e "${BLUE}初始 ConfigMap 配置:${NC}"
kubectl get configmap redis-config-test -o jsonpath='{.data.redis\.conf}' | head -5
echo ""

# 步骤 3: 修改配置
echo -e "${BLUE}步骤 3: 修改 RedisInstance 配置${NC}"
cat <<EOF | kubectl apply -f -
apiVersion: redis.github.com/v1
kind: RedisInstance
metadata:
  name: redis-config-test
spec:
  image: redis:7.0
  replicas: 1
  storage:
    size: 1Gi
    storageClassName: standard
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 200m
      memory: 256Mi
  config:
    maxmemory: 200mb
    maxmemory-policy: allkeys-lfu
    appendonly: "yes"
    tcp-keepalive: "300"
EOF

echo -e "${GREEN}✓ 配置已修改${NC}"

# 等待 StatefulSet 重启
echo -e "${YELLOW}等待 StatefulSet 重启...${NC}"
sleep 10

# 等待新的 Pod 就绪
kubectl wait --for=condition=ready pod -l app.kubernetes.io/instance=redis-config-test --timeout=120s

# 获取新的配置哈希值
echo -e "${BLUE}步骤 4: 验证配置更新${NC}"
NEW_HASH=$(kubectl get statefulset redis-config-test -o jsonpath='{.spec.template.metadata.annotations.redis\.github\.com/config-hash}')
echo -e "新配置哈希值: ${NEW_HASH}"

# 检查哈希值是否发生变化
if [ "$INITIAL_HASH" != "$NEW_HASH" ]; then
    echo -e "${GREEN}✓ 配置哈希值已更新，StatefulSet 已重启${NC}"
else
    echo -e "${RED}✗ 配置哈希值未变化，可能存在问题${NC}"
    exit 1
fi

# 检查新的 ConfigMap 内容
echo -e "${BLUE}更新后的 ConfigMap 配置:${NC}"
kubectl get configmap redis-config-test -o jsonpath='{.data.redis\.conf}' | head -10
echo ""

# 验证配置是否包含新的设置
if kubectl get configmap redis-config-test -o jsonpath='{.data.redis\.conf}' | grep -q "maxmemory 200mb"; then
    echo -e "${GREEN}✓ 新配置 'maxmemory 200mb' 已应用${NC}"
else
    echo -e "${RED}✗ 新配置未正确应用${NC}"
    exit 1
fi

if kubectl get configmap redis-config-test -o jsonpath='{.data.redis\.conf}' | grep -q "maxmemory-policy allkeys-lfu"; then
    echo -e "${GREEN}✓ 新配置 'maxmemory-policy allkeys-lfu' 已应用${NC}"
else
    echo -e "${RED}✗ 新配置未正确应用${NC}"
    exit 1
fi

if kubectl get configmap redis-config-test -o jsonpath='{.data.redis\.conf}' | grep -q "tcp-keepalive 300"; then
    echo -e "${GREEN}✓ 新配置 'tcp-keepalive 300' 已应用${NC}"
else
    echo -e "${RED}✗ 新配置未正确应用${NC}"
    exit 1
fi

# 检查 Pod 是否使用了新配置
echo -e "${BLUE}步骤 5: 验证 Pod 配置挂载${NC}"
POD_NAME=$(kubectl get pods -l app.kubernetes.io/instance=redis-config-test -o jsonpath='{.items[0].metadata.name}')
echo -e "检查 Pod: ${POD_NAME}"

# 检查配置文件是否正确挂载
if kubectl exec $POD_NAME -- test -f /usr/local/etc/redis/redis.conf; then
    echo -e "${GREEN}✓ 配置文件已正确挂载到 /usr/local/etc/redis/redis.conf${NC}"
else
    echo -e "${RED}✗ 配置文件挂载失败${NC}"
    exit 1
fi

# 检查 Redis 进程是否使用了配置文件
if kubectl exec $POD_NAME -- ps aux | grep -q "/usr/local/etc/redis/redis.conf"; then
    echo -e "${GREEN}✓ Redis 进程正在使用配置文件${NC}"
else
    echo -e "${YELLOW}⚠ 无法确认 Redis 进程是否使用配置文件${NC}"
fi

echo -e "${GREEN}=== 测试完成！配置修改自动重启功能正常工作 ===${NC}"
echo -e "${BLUE}测试总结:${NC}"
echo -e "1. ✓ RedisInstance 创建成功"
echo -e "2. ✓ 配置修改被检测到"
echo -e "3. ✓ StatefulSet 自动重启"
echo -e "4. ✓ 新配置正确应用"
echo -e "5. ✓ 配置文件正确挂载"
