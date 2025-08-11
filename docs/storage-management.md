# 通用存储管理工具

本文档介绍了 Redis Operator 中新增的通用存储管理工具，该工具优化了存储变更处理逻辑，提供了更好的错误信息和可复用性。

## 概述

通用存储管理工具 (`internal/utils/storage.go`) 提供了统一的存储变更分析和 PVC 扩容功能，可以在多个控制器中复用。

## 主要功能

### 1. 存储变更分析

`StorageManager.AnalyzeStorageChange()` 方法可以分析存储大小变更，并返回详细的结果：

```go
storageResult := storageManager.AnalyzeStorageChange(
    currentSize,  // 当前存储大小，如 "1Gi"
    desiredSize,  // 期望存储大小，如 "2Gi"
    componentName, // 组件名称，如 "Redis"
)
```

### 2. 存储变更类型

- `StorageExpansion`: 存储扩容（允许）
- `StorageShrinkage`: 存储缩容（拒绝）
- `StorageNoChange`: 存储无变更

### 3. PVC 动态扩容

`StorageManager.ExpandStatefulSetPVCs()` 方法可以自动扩展 StatefulSet 相关的 PVC：

```go
err := storageManager.ExpandStatefulSetPVCs(
    ctx,
    statefulSet,
    newStorageSize, // 新的存储大小
    componentName,  // 组件名称
)
```

## 优化内容

### 1. 错误信息优化

**之前的错误信息：**
```
storage shrinkage from 1Gi to 500Mi is not supported for safety reasons
```

**优化后的错误信息：**
```
[Redis] Storage shrinkage from 1Gi to 500Mi is not supported for data safety reasons. 
Please consider: 1) Backup data first, 2) Create new instance with smaller storage, 3) Migrate data manually.
```

### 2. 日志信息优化

- 添加了组件名称标识 `[Redis]`
- 提供了具体的解决方案建议
- 增加了详细的操作指导

### 3. 代码复用性

- 将存储管理逻辑抽取为通用工具
- 支持在多个控制器中复用
- 统一的错误处理和日志格式

## 使用示例

### 在控制器中使用

```go
type MyController struct {
    client.Client
    storageManager *utils.StorageManager
}

func (r *MyController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 初始化存储管理器
    if r.storageManager == nil {
        r.storageManager = utils.NewStorageManager(r.Client, logs)
    }
    
    // 分析存储变更
    storageResult := r.storageManager.AnalyzeStorageChange(
        currentSize, desiredSize, "MyComponent",
    )
    
    // 处理结果
    if storageResult.ErrorMessage != "" {
        return ctrl.Result{}, fmt.Errorf("%s", storageResult.ErrorMessage)
    }
    
    if storageResult.ChangeType == utils.StorageExpansion {
        // 执行 PVC 扩容
        err := r.storageManager.ExpandStatefulSetPVCs(
            ctx, statefulSet, desiredSize, "MyComponent",
        )
        if err != nil {
            return ctrl.Result{}, err
        }
    }
    
    return ctrl.Result{}, nil
}
```

### 错误处理示例

```go
func handleStorageErrors(err error, componentName string, logger logr.Logger) {
    if err == nil {
        return
    }
    
    errorMsg := err.Error()
    
    switch {
    case strings.Contains(errorMsg, "shrinkage"):
        logger.Error(err, "Storage shrinkage blocked for safety",
            "component", componentName,
            "action", "Consider creating new instance with smaller storage")
            
    case strings.Contains(errorMsg, "invalid storage size"):
        logger.Error(err, "Invalid storage size specified",
            "component", componentName,
            "action", "Check storage size format (e.g., 1Gi, 500Mi)")
            
    case strings.Contains(errorMsg, "failed to expand PVC"):
        logger.Error(err, "PVC expansion failed",
            "component", componentName,
            "action", "Check StorageClass supports volume expansion")
            
    default:
        logger.Error(err, "Storage operation failed",
            "component", componentName)
    }
}
```

## 测试验证

### 1. 存储扩容测试

```bash
# 将存储从 1Gi 扩容到 2Gi
kubectl patch redissentinel redissentinel-sample --type='merge' \
  -p='{"spec":{"redis":{"master":{"storage":{"size":"2Gi"}}}}}'
```

**预期结果：**
- 日志显示：`Storage expansion detected`
- PVC 成功扩容到 2Gi
- 服务无中断

### 2. 存储缩容测试

```bash
# 尝试将存储从 2Gi 缩容到 500Mi
kubectl patch redissentinel redissentinel-sample --type='merge' \
  -p='{"spec":{"redis":{"master":{"storage":{"size":"500Mi"}}}}}'
```

**预期结果：**
- 操作被拒绝
- 显示优化的错误信息
- 提供解决方案建议

## 配置要求

### StorageClass 要求

确保 StorageClass 支持动态扩容：

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: expandable-storage
provisioner: kubernetes.io/host-path
allowVolumeExpansion: true  # 必须设置为 true
volumeBindingMode: Immediate
```

### 权限要求

控制器需要以下 RBAC 权限：

```yaml
- apiGroups: [""]
  resources: ["persistentvolumeclaims"]
  verbs: ["get", "list", "watch", "update", "patch"]
- apiGroups: ["apps"]
  resources: ["statefulsets"]
  verbs: ["get", "list", "watch"]
```

## 最佳实践

1. **存储扩容**：
   - 使用支持 `allowVolumeExpansion: true` 的 StorageClass
   - 逐步扩容，避免一次性大幅增加
   - 监控扩容过程和应用性能

2. **存储缩容**：
   - 不支持直接缩容，需要手动迁移数据
   - 建议先备份数据
   - 创建新实例并迁移数据

3. **错误处理**：
   - 监控控制器日志
   - 根据错误信息采取相应措施
   - 定期检查 PVC 状态

## 故障排除

### 常见问题

1. **PVC 扩容失败**
   - 检查 StorageClass 是否支持扩容
   - 确认底层存储驱动支持动态扩容
   - 检查节点存储空间是否充足

2. **存储大小格式错误**
   - 使用正确的 Kubernetes 资源格式（如 1Gi, 500Mi）
   - 避免使用小数点或特殊字符

3. **权限不足**
   - 检查控制器的 RBAC 权限
   - 确认 ServiceAccount 配置正确

### 调试命令

```bash
# 检查 PVC 状态
kubectl get pvc -l app=redis

# 查看 PVC 详细信息
kubectl describe pvc <pvc-name>

# 检查 StorageClass
kubectl get storageclass

# 查看控制器日志
kubectl logs -f deployment/redis-operator-controller-manager
```

## 总结

通用存储管理工具提供了以下改进：

1. **更好的用户体验**：优化的错误信息和解决方案建议
2. **代码复用性**：可在多个控制器中使用
3. **统一的处理逻辑**：标准化的存储变更分析和处理
4. **增强的可观测性**：详细的日志和状态信息
5. **安全性保障**：防止意外的存储缩容操作

这些改进使得存储管理更加安全、可靠和用户友好。