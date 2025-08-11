# StatefulSet 滚动更新功能实现

## 问题背景

在之前的实现中，修改 RedisInstance 的任何参数（副本数、镜像、资源配置等）都会触发 StatefulSet 的重建，这会导致：

1. **服务中断**：重建过程中 Redis 服务完全不可用
2. **数据风险**：重建可能导致数据丢失
3. **资源浪费**：不必要的重建消耗额外的计算资源
4. **违背最佳实践**：Kubernetes StatefulSet 支持滚动更新，应该充分利用

## 解决方案

### 核心思路

将 StatefulSet 的变化分为两类：

1. **需要重建的变化**：
   - 配置文件变化（需要重新挂载 ConfigMap）
   - 存储配置变化（PVC 不能直接修改）

2. **可以滚动更新的变化**：
   - 副本数变化
   - 镜像版本变化
   - 资源配置变化（CPU、内存限制等）

### 实现细节

#### 1. 重构检查逻辑

**原始方法**：`needsStatefulSetRestart`
- 检查所有变化
- 任何变化都返回 `true`，触发重建

**新的方法结构**：

```go
// 只检查需要重建的变化
func needsStatefulSetRestart() bool {
    // 1. 配置文件变化
    // 2. 存储配置变化
}

// 检查可以滚动更新的变化
func needsStatefulSetUpdate() bool {
    // 1. 副本数变化
    // 2. 镜像变化  
    // 3. 资源配置变化
}
```

#### 2. 修改 ensureResources 逻辑

```go
if needsRestart {
    // 删除并重建 StatefulSet
} else {
    // 检查是否需要滚动更新
    if needsStatefulSetUpdate() {
        // 直接更新 StatefulSet 规格
        r.Update(ctx, statefulSet)
    }
}
```

## 代码实现

### 1. needsStatefulSetRestart 方法

```go
func (r *RedisInstanceReconciler) needsStatefulSetRestart(ctx context.Context, redisInstance *redisv1.RedisInstance, statefulSet *appsv1.StatefulSet, logs logr.Logger) (bool, error) {
    // 1. 检查配置文件变化 - 需要重建
    expectedConfig := utils.GenerateRedisConfig(redisInstance.Spec.Config)
    expectedHash := r.calculateConfigHash(expectedConfig)
    
    var stsConfigHash string
    if statefulSet.Spec.Template.Annotations != nil {
        stsConfigHash = statefulSet.Spec.Template.Annotations["redis.github.com/config-hash"]
    }
    
    if stsConfigHash != expectedHash {
        logs.Info("Config change detected, StatefulSet restart required")
        return true, nil
    }
    
    // 2. 检查存储配置变化 - 需要重建（PVC不能直接修改）
    if len(statefulSet.Spec.VolumeClaimTemplates) > 0 {
        currentStorageSize := statefulSet.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests["storage"]
        expectedStorageSize := resource.MustParse(redisInstance.Spec.Storage.Size)
        if !currentStorageSize.Equal(expectedStorageSize) {
            logs.Info("Storage size change detected, StatefulSet restart required")
            return true, nil
        }
        
        currentStorageClass := ""
        if statefulSet.Spec.VolumeClaimTemplates[0].Spec.StorageClassName != nil {
            currentStorageClass = *statefulSet.Spec.VolumeClaimTemplates[0].Spec.StorageClassName
        }
        if currentStorageClass != redisInstance.Spec.Storage.StorageClassName {
            logs.Info("Storage class change detected, StatefulSet restart required")
            return true, nil
        }
    }
    
    return false, nil
}
```

### 2. needsStatefulSetUpdate 方法

```go
func (r *RedisInstanceReconciler) needsStatefulSetUpdate(ctx context.Context, redisInstance *redisv1.RedisInstance, statefulSet *appsv1.StatefulSet, logs logr.Logger) bool {
    updated := false
    
    // 1. 检查副本数变化 - 可以直接更新
    if statefulSet.Spec.Replicas == nil || *statefulSet.Spec.Replicas != redisInstance.Spec.Replicas {
        logs.Info("Replicas change detected, will update")
        statefulSet.Spec.Replicas = &redisInstance.Spec.Replicas
        updated = true
    }
    
    // 2. 检查容器配置变化
    if len(statefulSet.Spec.Template.Spec.Containers) > 0 {
        container := &statefulSet.Spec.Template.Spec.Containers[0]
        
        // 检查镜像变化 - 可以滚动更新
        if container.Image != redisInstance.Spec.Image {
            logs.Info("Image change detected, will update")
            container.Image = redisInstance.Spec.Image
            updated = true
        }
        
        // 检查资源配置变化 - 可以滚动更新
        if !reflect.DeepEqual(container.Resources, redisInstance.Spec.Resources) {
            logs.Info("Resources change detected, will update")
            container.Resources = redisInstance.Spec.Resources
            updated = true
        }
    }
    
    return updated
}
```

### 3. 更新 ensureResources 逻辑

```go
} else {
    // StatefulSet 存在且不需要重启，检查是否需要更新规格或移除 finalizer
    updated := false
    
    // 检查是否需要移除 finalizer
    if controllerutil.ContainsFinalizer(statefulSet, redisv1.RedisInstanceFinalizer) {
        controllerutil.RemoveFinalizer(statefulSet, redisv1.RedisInstanceFinalizer)
        updated = true
    }
    
    // 检查是否需要更新 StatefulSet 规格（副本数、镜像、资源等）
    specUpdated := r.needsStatefulSetUpdate(ctx, redisInstance, statefulSet, logs)
    if specUpdated {
        logs.Info("StatefulSet spec needs update, performing rolling update")
        updated = true
    }
    
    // 只有在需要移除 finalizer 或更新规格时才更新 StatefulSet
    if updated {
        if err := r.Update(ctx, statefulSet); err != nil {
            logs.Error(err, "Failed to update StatefulSet")
            return err
        }
    }
}
```

## 测试验证

创建了 `test-rolling-update.sh` 测试脚本，验证以下场景：

### 测试场景

1. **副本数变化**：
   - 修改 `spec.replicas` 从 1 到 3
   - 验证：StatefulSet UID 不变（未重建），副本数正确更新

2. **镜像版本变化**：
   - 修改 `spec.image` 从 `redis:6.2` 到 `redis:7.0`
   - 验证：StatefulSet UID 不变（未重建），镜像正确更新

3. **资源配置变化**：
   - 修改内存限制从 256Mi 到 512Mi
   - 验证：StatefulSet UID 不变（未重建），资源配置正确更新

4. **配置文件变化**：
   - 修改 Redis 配置参数
   - 验证：StatefulSet UID 改变（正确重建）

### 测试结果

```bash
./test-rolling-update.sh
```

预期输出：
```
✅ 副本数滚动更新成功（StatefulSet 未重建）
✅ 镜像滚动更新成功（StatefulSet 未重建）
✅ 资源配置滚动更新成功（StatefulSet 未重建）
✅ 配置文件变化触发 StatefulSet 重建成功
🎉 滚动更新测试全部通过！
```

## 优势和效果

### 1. 服务可用性提升
- **副本数扩缩容**：无服务中断，逐步增减副本
- **镜像升级**：滚动更新，始终保持部分副本可用
- **资源调整**：平滑更新，不影响现有连接

### 2. 数据安全性
- 避免不必要的重建，降低数据丢失风险
- 保持 PVC 连续性，数据持久化更可靠

### 3. 资源效率
- 减少不必要的 Pod 重建
- 降低网络和存储 I/O
- 提高集群资源利用率

### 4. 运维友好
- 符合 Kubernetes 最佳实践
- 更精细的更新控制
- 更好的可观测性（通过日志区分更新类型）

## 最佳实践建议

### 1. 监控和观察
- 通过日志监控更新类型：
  - `"StatefulSet spec needs update, performing rolling update"`
  - `"StatefulSet restart required"`

### 2. 更新策略
- **日常运维**：优先使用滚动更新（副本数、资源调整）
- **重大变更**：谨慎处理需要重建的变更（配置、存储）

### 3. 测试验证
- 在生产环境应用前，使用测试脚本验证功能
- 监控更新过程中的服务可用性

## 总结

这次实现完全解决了 StatefulSet 更新策略的问题：

1. **精细化更新策略**：区分重建和滚动更新场景
2. **提升服务可用性**：减少不必要的服务中断
3. **符合最佳实践**：充分利用 Kubernetes 滚动更新能力
4. **向后兼容**：保持原有功能的同时增强更新策略

修复后的 Operator 现在能够智能地选择最合适的更新策略，为用户提供更好的使用体验。