# ConfigMap 频繁重启问题修复说明

## 问题描述

在之前的实现中，即使没有修改 ConfigMap 配置，StatefulSet 也会频繁重启，出现以下日志：

```
ConfigMap was updated or StatefulSet needs restart, deleting existing StatefulSet
```

## 问题根本原因

### 1. 逻辑错误在 `needsStatefulSetRestart` 方法

**原始错误逻辑：**
```go
// 错误的判断逻辑
return expectedHash != currentHash || stsConfigHash != expectedHash, nil
```

**问题分析：**
- 当 ConfigMap 被更新后，`currentHash` 会立即变成新的哈希值
- 但 StatefulSet 的 annotation 中的 `stsConfigHash` 还是旧的哈希值
- 这导致 `expectedHash != currentHash` 为 `false`，但 `stsConfigHash != expectedHash` 为 `true`
- 结果：即使配置没有真正变化，也会触发重启

### 2. 重启触发条件过于宽泛

**原始错误逻辑：**
```go
// 过于宽泛的重启条件
if errors.IsNotFound(statefulSetErr) || configMapRecreated || configMapUpdated || needsRestart {
    // 删除并重建 StatefulSet
}
```

**问题分析：**
- `configMapUpdated` 标志会在每次 ConfigMap 更新时触发 StatefulSet 重建
- 但实际上，ConfigMap 更新后应该先更新 StatefulSet 的 annotation，而不是立即重建

## 修复方案

### 1. 修复配置变更检测逻辑

**修复后的 `needsStatefulSetRestart` 方法：**
```go
func (r *RedisInstanceReconciler) needsStatefulSetRestart(ctx context.Context, redisInstance *redisv1.RedisInstance, statefulSet *appsv1.StatefulSet, logs logr.Logger) (bool, error) {
    // 生成期望的配置
    expectedConfig := utils.GenerateRedisConfig(redisInstance.Spec.Config)
    expectedHash := r.calculateConfigHash(expectedConfig)

    // 检查 StatefulSet 的 annotation 中是否有配置哈希值
    if statefulSet.Spec.Template.Annotations == nil {
        statefulSet.Spec.Template.Annotations = make(map[string]string)
    }

    stsConfigHash := statefulSet.Spec.Template.Annotations["redis.github.com/config-hash"]

    logs.Info("Config hash comparison", "expected", expectedHash, "sts", stsConfigHash)

    // 只有当 StatefulSet 中的配置哈希值与期望的不同时，才需要重启
    // 这避免了因为 ConfigMap 已经更新但 StatefulSet annotation 还未更新导致的误判
    return stsConfigHash != expectedHash, nil
}
```

**关键改进：**
- 移除了对 `currentHash` 的比较
- 只比较 StatefulSet annotation 中的哈希值与期望哈希值
- 避免了 ConfigMap 更新后的误判

### 2. 优化资源管理逻辑

**修复后的 `ensureResources` 方法逻辑：**

1. **分离创建和重启逻辑：**
   ```go
   // 只在 StatefulSet 不存在或 ConfigMap 被重新创建时创建 StatefulSet
   if errors.IsNotFound(statefulSetErr) || configMapRecreated {
       // 创建新的 StatefulSet
   }
   ```

2. **智能处理配置更新：**
   ```go
   if needsRestart {
       // 真正需要重启时才删除重建
   } else {
       // 只更新 annotation，不重建
       if configMapUpdated {
           // 更新 StatefulSet 的配置哈希值 annotation
       }
   }
   ```

## 修复效果

### 1. 避免频繁重启
- ConfigMap 更新后不会立即触发 StatefulSet 重建
- 只有在配置真正发生变化时才会重启

### 2. 提高性能
- 减少不必要的 StatefulSet 删除和重建操作
- 降低对 Kubernetes API 的压力

### 3. 更精确的配置管理
- 通过哈希值精确跟踪配置变化
- 避免误判导致的服务中断

## 测试验证

使用 `test-fix-verification.sh` 脚本可以验证修复效果：

1. **稳定性测试：** 创建 RedisInstance 后观察 30 秒，确保没有意外重启
2. **功能测试：** 修改配置后确保 StatefulSet 正确重建
3. **哈希值验证：** 确保配置哈希值正确更新

## 关键改进点总结

1. **精确的配置变更检测：** 只基于 StatefulSet annotation 中的哈希值判断是否需要重启
2. **分离的资源管理逻辑：** 区分创建、更新和重启场景
3. **智能的 annotation 更新：** ConfigMap 更新后先更新 annotation，避免不必要的重启
4. **更好的日志记录：** 提供清晰的操作日志，便于问题排查

这个修复确保了 ConfigMap 修改自动重启功能的正确性和稳定性，避免了频繁重启问题。