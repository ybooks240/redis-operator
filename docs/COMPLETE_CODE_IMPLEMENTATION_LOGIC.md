# Redis Operator 完整代码实现逻辑文档

## 项目概述

Redis Operator 是一个基于 Kubernetes Operator 模式的 Redis 实例管理系统，提供声明式的 Redis 集群部署、配置管理和状态监控功能。

## 项目结构

```
redis-operator/
├── api/v1/                          # API 定义
│   ├── redisinstance_types.go       # RedisInstance CRD 定义
│   └── groupversion_info.go         # API 版本信息
├── internal/controller/             # 控制器实现
│   └── redisinstance_controller.go  # 主控制器逻辑
├── internal/utils/                  # 工具函数
│   └── redis_config.go             # Redis 配置生成
├── config/                          # 部署配置
│   ├── crd/bases/                   # CRD 定义文件
│   ├── manager/                     # Manager 配置
│   └── rbac/                        # RBAC 权限配置
├── test-*.sh                        # 测试脚本
└── *.md                            # 文档文件
```

## 核心组件详解

### 1. API 定义层 (`api/v1/`)

#### 1.1 RedisInstance CRD 定义

**文件**: `api/v1/redisinstance_types.go`

##### RedisInstanceSpec 结构
```go
type RedisInstanceSpec struct {
    // Redis 镜像版本
    Image string `json:"image"`
    
    // 副本数量
    Replicas int32 `json:"replicas,omitempty"`
    
    // 资源配置
    Resources corev1.ResourceRequirements `json:"resources,omitempty"`
    
    // 存储配置
    Storage StorageSpec `json:"storage,omitempty"`
    
    // Redis 配置参数
    Config map[string]string `json:"config,omitempty"`
}
```

##### StorageSpec 结构
```go
type StorageSpec struct {
    // 存储大小
    Size string `json:"size,omitempty"`
    
    // 存储类名称
    StorageClassName string `json:"storageClassName,omitempty"`
}
```

##### RedisInstanceStatus 结构
```go
type RedisInstanceStatus struct {
    // 当前状态
    Status string `json:"status,omitempty"`
    
    // 就绪状态
    Ready string `json:"ready,omitempty"`
    
    // 最后状态消息
    LastConditionMessage string `json:"lastConditionMessage,omitempty"`
    
    // 状态条件历史
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}
```

##### RedisPhase 状态枚举
```go
type RedisPhase string

const (
    RedisPhaseUnknown    RedisPhase = "Unknown"     // 未知状态
    RedisPhaseCreating   RedisPhase = "Creating"    // 创建中
    RedisPhasePending    RedisPhase = "Pending"     // 等待中
    RedisPhaseRunning    RedisPhase = "Running"     // 运行中
    RedisPhaseFailed     RedisPhase = "Failed"      // 失败
    RedisPhaseTerminated RedisPhase = "Terminated"  // 已终止
    RedisPhaseUpdating   RedisPhase = "Updating"    // 更新中
)
```

### 2. 控制器实现层 (`internal/controller/`)

#### 2.1 RedisInstanceReconciler 主控制器

**文件**: `internal/controller/redisinstance_controller.go`

##### 控制器结构
```go
type RedisInstanceReconciler struct {
    client.Client
    Scheme *runtime.Scheme
}
```

##### 主要方法概览

1. **Reconcile**: 主调和循环
2. **ensureResources**: 确保所有子资源存在
3. **needsStatefulSetRestart**: 检查是否需要重建 StatefulSet
4. **needsStatefulSetUpdate**: 检查是否需要滚动更新 StatefulSet
5. **setUpdatingStatus**: 设置更新状态
6. **updateRedisInstanceStatus**: 更新 RedisInstance 状态
7. **statefulSetForRedisInstance**: 创建 StatefulSet 规格
8. **configMapForRedisInstance**: 创建 ConfigMap 规格
9. **serviceForRedisInstance**: 创建 Service 规格
10. **calculateConfigHash**: 计算配置哈希值

#### 2.2 Reconcile 主流程

```go
func (r *RedisInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. 获取 RedisInstance 资源
    redisInstance := &redisv1.RedisInstance{}
    err := r.Get(ctx, req.NamespacedName, redisInstance)
    
    // 2. 处理资源不存在的情况
    if errors.IsNotFound(err) {
        return ctrl.Result{}, nil
    }
    
    // 3. 处理删除逻辑
    if !redisInstance.DeletionTimestamp.IsZero() {
        return r.handleDeletion(ctx, redisInstance)
    }
    
    // 4. 添加 Finalizer
    if !controllerutil.ContainsFinalizer(redisInstance, redisv1.RedisInstanceFinalizer) {
        controllerutil.AddFinalizer(redisInstance, redisv1.RedisInstanceFinalizer)
        return ctrl.Result{}, r.Update(ctx, redisInstance)
    }
    
    // 5. 确保所有子资源存在和正确配置
    if err := r.ensureResources(ctx, req, redisInstance, ...); err != nil {
        return ctrl.Result{}, err
    }
    
    // 6. 更新 RedisInstance 状态
    if err := r.updateRedisInstanceStatus(ctx, redisInstance); err != nil {
        return ctrl.Result{}, err
    }
    
    return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
}
```

#### 2.3 资源确保逻辑 (ensureResources)

##### 流程概述
1. **ConfigMap 管理**
   - 检查 ConfigMap 是否存在
   - 更新 Redis 配置
   - 清理 Finalizer

2. **StatefulSet 管理**
   - 检查是否需要重建（破坏性更新）
   - 检查是否需要滚动更新（非破坏性更新）
   - 执行相应的更新策略

3. **Service 管理**
   - 确保 Service 存在
   - 清理 Finalizer

##### ConfigMap 处理逻辑
```go
// 检查 ConfigMap 是否存在
configMapErr := r.Get(ctx, types.NamespacedName{Name: redisInstance.Name, Namespace: redisInstance.Namespace}, configMap)

if errors.IsNotFound(configMapErr) {
    // 创建新的 ConfigMap
    newConfigMap, err := r.configMapForRedisInstance(redisInstance, logs)
    if err := r.Create(ctx, newConfigMap); err != nil {
        return err
    }
} else {
    // 更新现有 ConfigMap
    expectedConfig := utils.GenerateRedisConfig(redisInstance.Spec.Config)
    if expectedConfig != configMap.Data["redis.conf"] {
        configMap.Data["redis.conf"] = expectedConfig
        if err := r.Update(ctx, configMap); err != nil {
            return err
        }
    }
}
```

##### StatefulSet 更新策略

**重建策略** (needsStatefulSetRestart)
- **触发条件**:
  - Redis 配置文件变化
  - 存储大小变化
  - 存储类变化
- **执行流程**:
  1. 删除现有 StatefulSet
  2. 等待删除完成
  3. 创建新的 StatefulSet
  4. 设置 Updating 状态

**滚动更新策略** (needsStatefulSetUpdate)
- **触发条件**:
  - 副本数变化
  - 镜像版本变化
  - 资源配置变化
- **执行流程**:
  1. 直接修改 StatefulSet 规格
  2. Kubernetes 自动执行滚动更新
  3. 设置 Updating 状态

#### 2.4 状态管理系统

##### setUpdatingStatus 方法
```go
func (r *RedisInstanceReconciler) setUpdatingStatus(ctx context.Context, redisInstance *redisv1.RedisInstance, reason, message string) {
    // 设置状态条件
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

##### updateRedisInstanceStatus 方法

**状态检查优先级**:
1. **Terminated**: 正在删除
2. **Failed**: 资源缺失
3. **Updating**: StatefulSet 正在更新
4. **Pending**: 没有就绪副本
5. **Running**: 正常运行

**更新检测逻辑**:
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
```

#### 2.5 资源模板生成

##### StatefulSet 模板
```go
func (r *RedisInstanceReconciler) statefulSetForRedisInstance(redisInstance *redisv1.RedisInstance, logs logr.Logger) (*appsv1.StatefulSet, error) {
    labels := map[string]string{
        "app":                          "redis",
        "redis.github.com/instance":   redisInstance.Name,
    }
    
    sts := &appsv1.StatefulSet{
        ObjectMeta: metav1.ObjectMeta{
            Name:      redisInstance.Name,
            Namespace: redisInstance.Namespace,
            Labels:    labels,
        },
        Spec: appsv1.StatefulSetSpec{
            Replicas: &redisInstance.Spec.Replicas,
            Selector: &metav1.LabelSelector{
                MatchLabels: labels,
            },
            Template: corev1.PodTemplateSpec{
                ObjectMeta: metav1.ObjectMeta{
                    Labels: labels,
                },
                Spec: corev1.PodSpec{
                    Containers: []corev1.Container{
                        {
                            Name:  "redis",
                            Image: redisInstance.Spec.Image,
                            Ports: []corev1.ContainerPort{
                                {
                                    ContainerPort: 6379,
                                    Name:          "redis",
                                },
                            },
                            Resources: redisInstance.Spec.Resources,
                            VolumeMounts: []corev1.VolumeMount{
                                {
                                    Name:      "redis-data",
                                    MountPath: "/data",
                                },
                                {
                                    Name:      "redis-config",
                                    MountPath: "/usr/local/etc/redis",
                                },
                            },
                            Command: []string{"redis-server", "/usr/local/etc/redis/redis.conf"},
                        },
                    },
                    Volumes: []corev1.Volume{
                        {
                            Name: "redis-config",
                            VolumeSource: corev1.VolumeSource{
                                ConfigMap: &corev1.ConfigMapVolumeSource{
                                    LocalObjectReference: corev1.LocalObjectReference{
                                        Name: redisInstance.Name,
                                    },
                                },
                            },
                        },
                    },
                },
            },
            VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
                {
                    ObjectMeta: metav1.ObjectMeta{
                        Name: "redis-data",
                    },
                    Spec: corev1.PersistentVolumeClaimSpec{
                        AccessModes: []corev1.PersistentVolumeAccessMode{
                            corev1.ReadWriteOnce,
                        },
                        StorageClassName: &redisInstance.Spec.Storage.StorageClassName,
                        Resources: corev1.ResourceRequirements{
                            Requests: corev1.ResourceList{
                                corev1.ResourceStorage: resource.MustParse(redisInstance.Spec.Storage.Size),
                            },
                        },
                    },
                },
            },
        },
    }
    
    // 设置 OwnerReference
    if err := ctrl.SetControllerReference(redisInstance, sts, r.Scheme); err != nil {
        return nil, err
    }
    
    return sts, nil
}
```

### 3. 工具函数层 (`internal/utils/`)

#### 3.1 Redis 配置生成

**文件**: `internal/utils/redis_config.go`

```go
func GenerateRedisConfig(config map[string]string) string {
    var configLines []string
    
    // 默认配置
    configLines = append(configLines, "# Redis configuration generated by operator")
    configLines = append(configLines, "bind 0.0.0.0")
    configLines = append(configLines, "port 6379")
    configLines = append(configLines, "dir /data")
    configLines = append(configLines, "appendonly yes")
    configLines = append(configLines, "appendfsync everysec")
    
    // 用户自定义配置
    for key, value := range config {
        configLines = append(configLines, fmt.Sprintf("%s %s", key, value))
    }
    
    return strings.Join(configLines, "\n")
}
```

### 4. 配置哈希计算

```go
func (r *RedisInstanceReconciler) calculateConfigHash(config string) string {
    hash := sha256.Sum256([]byte(config))
    return fmt.Sprintf("%x", hash)
}
```

## 状态转换流程

### 4.1 完整状态转换图

```
[创建] → Creating → Pending → Running
                              ↓
[配置变更] → Updating ← → Running
                              ↓
[删除] → Terminated
                              ↓
[错误] → Failed
```

### 4.2 状态转换触发条件

| 源状态 | 目标状态 | 触发条件 |
|--------|----------|----------|
| * | Creating | RedisInstance 首次创建 |
| Creating | Pending | 资源创建完成，等待 Pod 启动 |
| Pending | Running | 所有 Pod 就绪 |
| Running | Updating | 配置变更检测到 |
| Updating | Running | 更新完成，所有 Pod 就绪 |
| * | Failed | 资源创建/更新失败 |
| * | Terminated | 删除操作触发 |

### 4.3 配置变更处理流程

```
用户修改 RedisInstance
         ↓
    Controller 检测变更
         ↓
    判断变更类型
    ↙          ↘
重建变更      滚动更新变更
(配置/存储)    (副本/镜像/资源)
    ↓              ↓
设置 Updating   设置 Updating
状态            状态
    ↓              ↓
删除并重建      直接更新
StatefulSet    StatefulSet
    ↓              ↓
监控重建进度    监控更新进度
    ↓              ↓
    更新完成
         ↓
    设置 Running 状态
```

## 错误处理和恢复机制

### 5.1 错误分类

1. **资源创建错误**
   - ConfigMap 创建失败
   - StatefulSet 创建失败
   - Service 创建失败

2. **资源更新错误**
   - StatefulSet 更新失败
   - 配置更新失败

3. **状态更新错误**
   - 状态写入失败
   - 条件更新失败

### 5.2 恢复策略

1. **重试机制**
   - 使用 Controller Runtime 的自动重试
   - 指数退避策略

2. **状态一致性**
   - 异步状态更新避免阻塞主流程
   - 错误记录但不影响资源操作

3. **资源清理**
   - Finalizer 确保正确清理
   - OwnerReference 自动级联删除

## 测试和验证

### 6.1 测试脚本

1. **test-rolling-update.sh**
   - 验证滚动更新功能
   - 测试副本数、镜像、资源配置变更

2. **test-statefulset-update.sh**
   - 验证 StatefulSet 更新功能
   - 测试配置文件变更触发重建

3. **test-status-update.sh**
   - 验证状态更新功能
   - 测试 Updating 状态设置和恢复

### 6.2 验证场景

1. **功能测试**
   - 基本 CRUD 操作
   - 配置变更处理
   - 状态转换验证

2. **性能测试**
   - 大规模部署
   - 并发操作
   - 资源消耗监控

3. **故障测试**
   - 网络分区
   - 节点故障
   - 资源不足

## 监控和可观测性

### 7.1 日志记录

- 使用结构化日志
- 记录关键操作和状态变化
- 错误详细信息记录

### 7.2 指标监控

- RedisInstance 数量和状态分布
- 操作延迟和成功率
- 资源使用情况

### 7.3 事件记录

- Kubernetes 事件记录
- 状态变化事件
- 错误和警告事件

## 最佳实践和建议

### 8.1 部署建议

1. **资源规划**
   - 合理设置资源请求和限制
   - 考虑存储性能要求

2. **配置管理**
   - 使用版本控制管理配置
   - 在低峰期进行配置变更

3. **监控告警**
   - 设置关键指标告警
   - 监控状态异常

### 8.2 故障排查

1. **状态检查**
   ```bash
   kubectl get redisinstance
   kubectl describe redisinstance <name>
   ```

2. **资源检查**
   ```bash
   kubectl get statefulset,configmap,service
   kubectl describe statefulset <name>
   ```

3. **日志查看**
   ```bash
   kubectl logs -l app=redis
   kubectl logs deployment/redis-operator-controller-manager
   ```

### 8.3 性能优化

1. **控制器优化**
   - 合理设置调和间隔
   - 优化资源监听范围

2. **资源优化**
   - 使用合适的存储类
   - 优化容器资源配置

## 总结

Redis Operator 提供了完整的 Redis 实例生命周期管理功能，通过声明式 API 简化了 Redis 的部署和运维。核心特性包括：

1. **智能更新策略**: 区分破坏性和非破坏性变更，选择最优更新方式
2. **精确状态管理**: 实时反映 Redis 实例状态，提供详细的状态历史
3. **健壮错误处理**: 完善的错误恢复机制，确保系统稳定性
4. **灵活配置管理**: 支持动态配置更新，满足不同业务需求
5. **完整可观测性**: 提供详细的日志、指标和事件记录

通过本文档的详细说明，开发者和运维人员可以深入理解 Redis Operator 的实现原理，更好地使用和维护系统。