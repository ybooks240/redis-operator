# RedisInstance 状态更新实现逻辑文档

## 概述

本文档详细描述了 RedisInstance Operator 的完整实现逻辑，特别是新增的状态更新功能。当 RedisInstance 配置发生变化时，系统会自动将状态设置为 `RedisPhaseUpdating`，`Ready` 设置为 `false`，并相应更新 `message` 和 `reason`。

## 核心组件架构

### 1. API 定义 (`api/v1/redisinstance_types.go`)

#### RedisPhase 状态枚举
```go
type RedisPhase string

const (
    RedisPhaseUnknown    RedisPhase = "Unknown"
    RedisPhaseCreating   RedisPhase = "Creating"
    RedisPhasePending    RedisPhase = "Pending"
    RedisPhaseRunning    RedisPhase = "Running"
    RedisPhaseFailed     RedisPhase = "Failed"
    RedisPhaseTerminated RedisPhase = "Terminated"
    RedisPhaseUpdating   RedisPhase = "Updating"  // 新增状态
)
```

#### RedisInstance 规格定义
- **Image**: Redis 镜像版本
- **Replicas**: 副本数量
- **Resources**: CPU/内存资源配置
- **Storage**: 存储配置（大小和存储类）
- **Config**: Redis 配置参数

#### RedisInstance 状态定义
- **Status**: 当前状态字符串
- **Ready**: 就绪状态字符串
- **LastConditionMessage**: 最后状态消息
- **Conditions**: 状态条件历史记录

### 2. 控制器实现 (`internal/controller/redisinstance_controller.go`)

## 主要方法详解

### 2.1 Reconcile 主流程

```go
func (r *RedisInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error)
```

**执行流程：**
1. 获取 RedisInstance 资源
2. 处理删除逻辑（如果有删除时间戳）
3. 确保所有子资源存在和正确配置
4. 更新 RedisInstance 状态
5. 返回调和结果

### 2.2 资源确保逻辑

#### ensureResources 方法
负责确保所有必需的 Kubernetes 资源（ConfigMap、StatefulSet、Service）存在且配置正确。

**关键逻辑：**
- ConfigMap 配置更新检测
- StatefulSet 重建 vs 滚动更新决策
- Service 配置管理
- Finalizer 清理

### 2.3 StatefulSet 更新策略

#### needsStatefulSetRestart 方法
检查是否需要重建 StatefulSet（破坏性更新）。

**触发重建的变化：**
1. **配置文件变化**：Redis 配置参数修改
2. **存储大小变化**：PVC 大小修改（不可原地更新）
3. **存储类变化**：StorageClass 修改

**实现逻辑：**
```go
func (r *RedisInstanceReconciler) needsStatefulSetRestart(ctx context.Context, redisInstance *redisv1.RedisInstance, statefulSet *appsv1.StatefulSet, logs logr.Logger) (bool, error) {
    needsRestart := false
    reasonMsg := ""
    
    // 1. 检查配置文件变化
    expectedConfig := utils.GenerateRedisConfig(redisInstance.Spec.Config)
    expectedHash := r.calculateConfigHash(expectedConfig)
    
    var stsConfigHash string
    if statefulSet.Spec.Template.Annotations != nil {
        stsConfigHash = statefulSet.Spec.Template.Annotations["redis.github.com/config-hash"]
    }
    
    if stsConfigHash != expectedHash {
        needsRestart = true
        reasonMsg = "Redis configuration has changed, StatefulSet will be recreated"
    }
    
    // 2. 检查存储配置变化
    // ... 存储大小和存储类检查逻辑
    
    // 如果需要重建，设置状态为Updating
    if needsRestart {
        r.setUpdatingStatus(ctx, redisInstance, "StatefulSetRestart", reasonMsg)
    }
    
    return needsRestart, nil
}
```

#### needsStatefulSetUpdate 方法
检查是否需要滚动更新 StatefulSet（非破坏性更新）。

**触发滚动更新的变化：**
1. **副本数变化**：可以直接修改 StatefulSet.Spec.Replicas
2. **镜像版本变化**：触发 Pod 滚动更新
3. **资源配置变化**：CPU/内存限制修改

**实现逻辑：**
```go
func (r *RedisInstanceReconciler) needsStatefulSetUpdate(ctx context.Context, redisInstance *redisv1.RedisInstance, statefulSet *appsv1.StatefulSet, logs logr.Logger) bool {
    updated := false
    
    // 1. 检查副本数变化
    if statefulSet.Spec.Replicas == nil || *statefulSet.Spec.Replicas != redisInstance.Spec.Replicas {
        statefulSet.Spec.Replicas = &redisInstance.Spec.Replicas
        updated = true
    }
    
    // 2. 检查容器配置变化
    if len(statefulSet.Spec.Template.Spec.Containers) > 0 {
        container := &statefulSet.Spec.Template.Spec.Containers[0]
        
        // 检查镜像变化
        if container.Image != redisInstance.Spec.Image {
            container.Image = redisInstance.Spec.Image
            updated = true
        }
        
        // 检查资源配置变化
        if !reflect.DeepEqual(container.Resources, redisInstance.Spec.Resources) {
            container.Resources = redisInstance.Spec.Resources
            updated = true
        }
    }
    
    // 如果有更新，设置状态为Updating
    if updated {
        r.setUpdatingStatus(ctx, redisInstance, "StatefulSetUpdate", "StatefulSet is being updated with new configuration")
    }
    
    return updated
}
```

### 2.4 状态更新机制

#### setUpdatingStatus 方法
新增方法，用于设置 RedisInstance 状态为 Updating。

**功能特点：**
- 设置状态为 `RedisPhaseUpdating`
- 设置 `Ready` 为 `false`
- 更新 `message` 和 `reason`
- 异步更新状态，避免阻塞主流程

**实现逻辑：**
```go
func (r *RedisInstanceReconciler) setUpdatingStatus(ctx context.Context, redisInstance *redisv1.RedisInstance, reason, message string) {
    // 设置状态为Updating
    meta.SetStatusCondition(&redisInstance.Status.Conditions, metav1.Condition{
        Type:               string(redisv1.RedisPhaseUpdating),
        Status:             metav1.ConditionFalse, // Ready为false
        LastTransitionTime: metav1.Now(),
        Reason:             reason,
        Message:            message,
        ObservedGeneration: redisInstance.Generation,
    })
    
    // 更新状态字段
    redisInstance.Status.Status = string(redisv1.RedisPhaseUpdating)
    redisInstance.Status.Ready = string(metav1.ConditionFalse)
    redisInstance.Status.LastConditionMessage = message
    
    // 异步更新状态
    go func() {
        if err := r.Status().Update(ctx, redisInstance); err != nil {
            ctrl.Log.Error(err, "Failed to update RedisInstance status to Updating")
        }
    }()
}
```

#### updateRedisInstanceStatus 方法
更新后的状态检查逻辑，增加了对 StatefulSet 更新状态的检测。

**状态优先级：**
1. **Terminated**: 正在删除
2. **Failed**: 资源缺失
3. **Updating**: StatefulSet 正在更新
4. **Pending**: 没有就绪副本
5. **Running**: 正常运行

**更新检测逻辑：**
```go
// 检查是否正在更新中
isUpdating := false
if statefulSetErr == nil {
    // 检查StatefulSet是否正在进行滚动更新
    if sts.Status.UpdatedReplicas < sts.Status.Replicas {
        isUpdating = true
    }
    // 检查是否有正在更新的Pod
    if sts.Status.CurrentRevision != sts.Status.UpdateRevision {
        isUpdating = true
    }
}

if isUpdating {
    // 如果StatefulSet正在更新，保持Updating状态
    conditionStatus = metav1.ConditionFalse // Ready为false
    conditionType = string(redisv1.RedisPhaseUpdating)
    reason = "StatefulSetUpdating"
    message = fmt.Sprintf("StatefulSet is being updated. Updated: %d/%d, Ready: %d/%d",
        sts.Status.UpdatedReplicas, sts.Status.Replicas, sts.Status.ReadyReplicas, sts.Status.Replicas)
}
```

## 状态转换流程

### 配置变化时的状态转换

1. **用户修改 RedisInstance 配置**
   - 副本数、镜像、资源配置等

2. **Controller 检测变化**
   - `needsStatefulSetRestart` 或 `needsStatefulSetUpdate` 检测到变化

3. **设置 Updating 状态**
   - 调用 `setUpdatingStatus` 方法
   - 状态：`RedisPhaseUpdating`
   - Ready：`false`
   - 原因和消息：描述具体变化

4. **执行更新操作**
   - 重建 StatefulSet（破坏性变化）
   - 或滚动更新 StatefulSet（非破坏性变化）

5. **监控更新进度**
   - `updateRedisInstanceStatus` 持续检查 StatefulSet 状态
   - 检测 `UpdatedReplicas` 和 `CurrentRevision` vs `UpdateRevision`

6. **更新完成**
   - 所有副本更新完成且就绪
   - 状态转换为 `RedisPhaseRunning`
   - Ready：`true`

## 错误处理和恢复

### 异步状态更新
- 使用 goroutine 异步更新状态，避免阻塞主调和流程
- 错误记录但不影响资源更新操作

### 状态条件历史
- 限制 Conditions 数量最多 10 个
- 按时间排序，保留最新的 9 个条件
- 提供完整的状态变化历史

### Finalizer 管理
- 确保资源删除时正确清理
- 避免资源泄漏

## 测试和验证

### 测试脚本
- `test-rolling-update.sh`: 验证滚动更新功能
- `test-statefulset-update.sh`: 验证 StatefulSet 更新功能

### 验证场景
1. **副本数变化**: 验证滚动扩缩容
2. **镜像更新**: 验证滚动升级
3. **资源配置变化**: 验证资源限制更新
4. **配置文件变化**: 验证 StatefulSet 重建
5. **存储配置变化**: 验证 StatefulSet 重建

## 最佳实践

### 监控建议
1. 监控 RedisInstance 状态变化
2. 关注 StatefulSet 更新进度
3. 监控 Pod 就绪状态

### 运维建议
1. 在业务低峰期进行配置变更
2. 逐步更新，避免大批量变更
3. 备份重要数据后再进行存储配置变更

### 故障排查
1. 检查 RedisInstance 状态和条件
2. 查看 StatefulSet 事件和状态
3. 检查 Pod 日志和事件
4. 验证资源配置和限制

## 总结

本实现提供了完整的 RedisInstance 状态管理机制，特别是在配置变化时的状态更新功能。通过区分破坏性和非破坏性变化，系统能够选择最适合的更新策略，同时提供准确的状态反馈，提升了运维体验和系统可靠性。

### 关键改进
1. **精确状态反馈**: 配置变化时立即设置 Updating 状态
2. **智能更新策略**: 区分重建和滚动更新
3. **非阻塞设计**: 异步状态更新不影响主流程
4. **完整状态历史**: 提供详细的状态变化记录
5. **错误恢复**: 健壮的错误处理和恢复机制