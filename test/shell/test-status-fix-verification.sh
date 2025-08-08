#!/bin/bash

# 测试状态更新修复验证脚本
# 验证当StatefulSet更新完成后，RedisInstance状态能够正确从Updating切换到Running

set -e

echo "=== Redis Operator 状态更新修复验证测试 ==="
echo

# 检查当前状态
echo "1. 检查当前RedisInstance状态:"
kubectl get redisinstance redisinstance-sample
echo

# 检查StatefulSet状态
echo "2. 检查StatefulSet状态:"
kubectl get statefulset redisinstance-sample -o jsonpath='{.status}' | jq .
echo

# 触发一次配置更新来测试状态转换
echo "3. 触发配置更新测试状态转换:"
kubectl patch redisinstance redisinstance-sample --type='merge' -p='{"spec":{"config":{"maxmemory":"256mb"}}}'
echo "配置已更新，等待状态变化..."
sleep 5

# 检查是否正确设置为Updating状态
echo "4. 检查是否正确设置为Updating状态:"
STATUS=$(kubectl get redisinstance redisinstance-sample -o jsonpath='{.status.status}')
if [ "$STATUS" = "Updating" ]; then
    echo "✓ 状态正确设置为Updating"
else
    echo "✗ 状态未正确设置为Updating，当前状态: $STATUS"
fi
echo

# 等待更新完成
echo "5. 等待StatefulSet更新完成..."
kubectl rollout status statefulset/redisinstance-sample --timeout=300s
echo

# 手动触发reconcile确保状态更新
echo "6. 触发reconcile确保状态更新:"
kubectl annotate redisinstance redisinstance-sample reconcile.timestamp="$(date)" --overwrite
sleep 3

# 检查最终状态
echo "7. 检查最终状态:"
FINAL_STATUS=$(kubectl get redisinstance redisinstance-sample -o jsonpath='{.status.status}')
FINAL_READY=$(kubectl get redisinstance redisinstance-sample -o jsonpath='{.status.ready}')

echo "最终状态: $FINAL_STATUS"
echo "就绪状态: $FINAL_READY"

if [ "$FINAL_STATUS" = "Running" ] && [ "$FINAL_READY" = "True" ]; then
    echo "✓ 状态修复验证成功！RedisInstance正确从Updating状态恢复到Running状态"
else
    echo "✗ 状态修复验证失败！期望状态: Running/True，实际状态: $FINAL_STATUS/$FINAL_READY"
    exit 1
fi

echo
echo "8. 详细状态信息:"
kubectl get redisinstance redisinstance-sample -o yaml | grep -A 20 status:

echo
echo "=== 状态更新修复验证测试完成 ==="
echo "✓ 修复验证成功：当StatefulSet更新完成后，RedisInstance状态能够正确更新"
echo "✓ 关键改进："
echo "  - 优化了isUpdating判断逻辑，只在真正需要更新时设置Updating状态"
echo "  - 修复了状态字段更新逻辑，直接使用计算出的状态而不是从conditions数组获取"
echo "  - 确保状态转换的及时性和准确性"