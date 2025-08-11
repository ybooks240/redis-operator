# Redis Controller 实现文档

## 概述

本文档详细描述了 Redis Operator 中三种部署模式控制器的实现架构、核心逻辑和技术细节。

## 1. 控制器架构设计

### 1.1 整体架构

```
┌─────────────────────────────────────────────────────────────────┐
│                        Redis Operator                           │
│                                                                 │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │
│  │ MasterReplica   │  │   Sentinel      │  │    Cluster      │  │
│  │   Controller    │  │   Controller    │  │   Controller    │  │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘  │
│           │                     │                     │         │
│           ▼                     ▼                     ▼         │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │
│  │   StatefulSet   │  │   StatefulSet   │  │   StatefulSet   │  │
│  │    Service      │  │    Service      │  │    Service      │  │
│  │   ConfigMap     │  │   ConfigMap     │  │   ConfigMap     │  │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

### 1.2 控制器模式

所有控制器都遵循标准的 Kubernetes Controller 模式：

1. **Watch**：监听资源变更事件
2. **Reconcile**：协调期望状态与实际状态
3. **Update**：更新资源状态
4. **Retry**：失败时重试机制

## 2. RedisMasterReplica Controller

### 2.1 核心职责

- 管理 Redis 主从架构的生命周期
- 确保主节点和从节点的正确配置
- 监控主从复制状态
- 处理节点故障和恢复

### 2.2 Reconcile 逻辑

```go
func (r *RedisMasterReplicaReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. 获取 RedisMasterReplica 资源
    masterReplica := &redisv1.RedisMasterReplica{}
    if err := r.Get(ctx, req.NamespacedName, masterReplica); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 2. 处理删除逻辑
    if masterReplica.DeletionTimestamp != nil {
        return r.handleDeletion(ctx, masterReplica)
    }

    // 3. 添加 Finalizer
    if !controllerutil.ContainsFinalizer(masterReplica, redisv1.RedisMasterReplicaFinalizer) {
        controllerutil.AddFinalizer(masterReplica, redisv1.RedisMasterReplicaFinalizer)
        return ctrl.Result{}, r.Update(ctx, masterReplica)
    }

    // 4. 协调子资源
    if err := r.reconcileConfigMap(ctx, masterReplica); err != nil {
        return ctrl.Result{}, err
    }

    if err := r.reconcileMasterStatefulSet(ctx, masterReplica); err != nil {
        return ctrl.Result{}, err
    }

    if err := r.reconcileReplicaStatefulSet(ctx, masterReplica); err != nil {
        return ctrl.Result{}, err
    }

    if err := r.reconcileServices(ctx, masterReplica); err != nil {
        return ctrl.Result{}, err
    }

    // 5. 更新状态
    return ctrl.Result{}, r.updateStatus(ctx, masterReplica)
}
```

### 2.3 关键实现细节

#### 2.3.1 主节点配置

```yaml
# Master ConfigMap
apiVersion: v1
kind: ConfigMap
metadata:
  name: redis-master-config
data:
  redis.conf: |
    # Master specific configuration
    save 900 1
    save 300 10
    save 60 10000
    rdbcompression yes
    rdbchecksum yes
    # User defined config
    maxmemory 1gb
    maxmemory-policy allkeys-lru
```

#### 2.3.2 从节点配置

```yaml
# Replica ConfigMap
apiVersion: v1
kind: ConfigMap
metadata:
  name: redis-replica-config
data:
  redis.conf: |
    # Replica specific configuration
    replicaof redis-master-service 6379
    replica-read-only yes
    replica-serve-stale-data yes
    # User defined config
    maxmemory 512mb
    maxmemory-policy allkeys-lru
```

#### 2.3.3 状态更新逻辑

```go
func (r *RedisMasterReplicaReconciler) updateStatus(ctx context.Context, mr *redisv1.RedisMasterReplica) error {
    // 获取主节点状态
    masterStatus, err := r.getMasterStatus(ctx, mr)
    if err != nil {
        return err
    }

    // 获取从节点状态
    replicaStatus, err := r.getReplicaStatus(ctx, mr)
    if err != nil {
        return err
    }

    // 更新整体状态
    mr.Status.Master = masterStatus
    mr.Status.Replica = replicaStatus
    
    // 判断整体就绪状态
    if masterStatus.Ready && replicaStatus.ReadyReplicas == replicaStatus.Replicas {
        mr.Status.Ready = "True"
        mr.Status.Status = string(redisv1.RedisMasterReplicaPhaseRunning)
        mr.Status.LastConditionMessage = "Master-Replica setup is running"
    } else {
        mr.Status.Ready = "False"
        mr.Status.Status = string(redisv1.RedisMasterReplicaPhasePending)
        mr.Status.LastConditionMessage = "Master-Replica setup is not ready"
    }

    return r.Status().Update(ctx, mr)
}
```

## 3. RedisSentinel Controller

### 3.1 核心职责

- 管理 Sentinel 集群的生命周期
- 配置 Sentinel 监控目标
- 处理故障转移事件
- 维护 Sentinel 配置一致性

### 3.2 Sentinel 配置生成

```go
func (r *RedisSentinelReconciler) generateSentinelConfig(sentinel *redisv1.RedisSentinel) string {
    config := fmt.Sprintf(`
# Sentinel configuration
port 26379
sentinel announce-ip $(POD_IP)
sentinel announce-port 26379

# Monitor master
sentinel monitor %s %s 6379 %d
sentinel down-after-milliseconds %s %d
sentinel failover-timeout %s %d
sentinel parallel-syncs %s %d

# Additional configuration
`,
        sentinel.Spec.MasterReplicaRef.MasterName,
        r.getMasterServiceName(sentinel),
        sentinel.Spec.Config.Quorum,
        sentinel.Spec.MasterReplicaRef.MasterName,
        sentinel.Spec.Config.DownAfterMilliseconds,
        sentinel.Spec.MasterReplicaRef.MasterName,
        sentinel.Spec.Config.FailoverTimeout,
        sentinel.Spec.MasterReplicaRef.MasterName,
        sentinel.Spec.Config.ParallelSyncs,
    )

    // 添加用户自定义配置
    for key, value := range sentinel.Spec.Config.AdditionalConfig {
        config += fmt.Sprintf("%s %s\n", key, value)
    }

    return config
}
```

### 3.3 监控目标发现

```go
func (r *RedisSentinelReconciler) discoverMasterReplica(ctx context.Context, sentinel *redisv1.RedisSentinel) (*redisv1.RedisMasterReplica, error) {
    masterReplica := &redisv1.RedisMasterReplica{}
    namespacedName := types.NamespacedName{
        Name:      sentinel.Spec.MasterReplicaRef.Name,
        Namespace: sentinel.Namespace,
    }
    
    if sentinel.Spec.MasterReplicaRef.Namespace != "" {
        namespacedName.Namespace = sentinel.Spec.MasterReplicaRef.Namespace
    }

    if err := r.Get(ctx, namespacedName, masterReplica); err != nil {
        return nil, fmt.Errorf("failed to get MasterReplica %s: %w", namespacedName, err)
    }

    return masterReplica, nil
}
```

## 4. RedisCluster Controller

### 4.1 核心职责

- 管理 Redis 集群的生命周期
- 初始化集群拓扑
- 处理节点加入和离开
- 管理槽位分配和迁移
- 监控集群健康状态

### 4.2 集群初始化逻辑

```go
func (r *RedisClusterReconciler) initializeCluster(ctx context.Context, cluster *redisv1.RedisCluster) error {
    // 1. 等待所有节点就绪
    if !r.allNodesReady(ctx, cluster) {
        return fmt.Errorf("not all nodes are ready")
    }

    // 2. 获取所有节点 IP
    nodeIPs, err := r.getNodeIPs(ctx, cluster)
    if err != nil {
        return err
    }

    // 3. 创建集群
    masterIPs := nodeIPs[:cluster.Spec.Masters]
    cmd := fmt.Sprintf("redis-cli --cluster create %s --cluster-replicas %d --cluster-yes",
        strings.Join(masterIPs, ":6379 ")+":6379",
        cluster.Spec.ReplicasPerMaster,
    )

    // 4. 在第一个节点上执行集群创建命令
    return r.executeCommand(ctx, cluster, nodeIPs[0], cmd)
}
```

### 4.3 集群状态监控

```go
func (r *RedisClusterReconciler) updateClusterStatus(ctx context.Context, cluster *redisv1.RedisCluster) error {
    // 1. 获取集群信息
    clusterInfo, err := r.getClusterInfo(ctx, cluster)
    if err != nil {
        return err
    }

    // 2. 获取节点信息
    nodes, err := r.getClusterNodes(ctx, cluster)
    if err != nil {
        return err
    }

    // 3. 更新状态
    cluster.Status.Cluster = redisv1.ClusterStatus{
        State:         clusterInfo.State,
        SlotsAssigned: clusterInfo.SlotsAssigned,
        SlotsOk:       clusterInfo.SlotsOk,
        SlotsPfail:    clusterInfo.SlotsPfail,
        SlotsFail:     clusterInfo.SlotsFail,
        KnownNodes:    clusterInfo.KnownNodes,
        Size:          clusterInfo.Size,
        CurrentEpoch:  clusterInfo.CurrentEpoch,
        MyEpoch:       clusterInfo.MyEpoch,
    }

    cluster.Status.Nodes = nodes

    // 4. 判断集群是否就绪
    if clusterInfo.State == "ok" && clusterInfo.SlotsAssigned == 16384 {
        cluster.Status.Ready = "True"
        cluster.Status.Status = string(redisv1.RedisClusterPhaseRunning)
        cluster.Status.LastConditionMessage = "Cluster is running"
    } else {
        cluster.Status.Ready = "False"
        cluster.Status.Status = string(redisv1.RedisClusterPhasePending)
        cluster.Status.LastConditionMessage = "Cluster is not ready"
    }

    return r.Status().Update(ctx, cluster)
}
```

## 5. 通用组件设计

### 5.1 ConfigMap 管理

```go
type ConfigMapManager struct {
    client.Client
    Scheme *runtime.Scheme
}

func (cm *ConfigMapManager) ReconcileConfigMap(ctx context.Context, owner metav1.Object, configMap *corev1.ConfigMap) error {
    // 1. 设置 OwnerReference
    if err := controllerutil.SetControllerReference(owner, configMap, cm.Scheme); err != nil {
        return err
    }

    // 2. 创建或更新 ConfigMap
    existing := &corev1.ConfigMap{}
    err := cm.Get(ctx, client.ObjectKeyFromObject(configMap), existing)
    if err != nil {
        if errors.IsNotFound(err) {
            return cm.Create(ctx, configMap)
        }
        return err
    }

    // 3. 更新现有 ConfigMap
    existing.Data = configMap.Data
    return cm.Update(ctx, existing)
}
```

### 5.2 StatefulSet 管理

```go
type StatefulSetManager struct {
    client.Client
    Scheme *runtime.Scheme
}

func (sm *StatefulSetManager) ReconcileStatefulSet(ctx context.Context, owner metav1.Object, sts *appsv1.StatefulSet) error {
    // 1. 设置 OwnerReference
    if err := controllerutil.SetControllerReference(owner, sts, sm.Scheme); err != nil {
        return err
    }

    // 2. 创建或更新 StatefulSet
    existing := &appsv1.StatefulSet{}
    err := sm.Get(ctx, client.ObjectKeyFromObject(sts), existing)
    if err != nil {
        if errors.IsNotFound(err) {
            return sm.Create(ctx, sts)
        }
        return err
    }

    // 3. 更新 StatefulSet（只更新允许的字段）
    existing.Spec.Replicas = sts.Spec.Replicas
    existing.Spec.Template = sts.Spec.Template
    existing.Spec.UpdateStrategy = sts.Spec.UpdateStrategy
    
    return sm.Update(ctx, existing)
}
```

### 5.3 Service 管理

```go
type ServiceManager struct {
    client.Client
    Scheme *runtime.Scheme
}

func (sm *ServiceManager) ReconcileService(ctx context.Context, owner metav1.Object, service *corev1.Service) error {
    // 1. 设置 OwnerReference
    if err := controllerutil.SetControllerReference(owner, service, sm.Scheme); err != nil {
        return err
    }

    // 2. 创建或更新 Service
    existing := &corev1.Service{}
    err := sm.Get(ctx, client.ObjectKeyFromObject(service), existing)
    if err != nil {
        if errors.IsNotFound(err) {
            return sm.Create(ctx, service)
        }
        return err
    }

    // 3. 更新 Service（保留 ClusterIP）
    existing.Spec.Ports = service.Spec.Ports
    existing.Spec.Selector = service.Spec.Selector
    existing.Spec.Type = service.Spec.Type
    
    return sm.Update(ctx, existing)
}
```

## 6. 错误处理和重试机制

### 6.1 错误分类

```go
type ReconcileError struct {
    Type    ErrorType
    Message string
    Cause   error
}

type ErrorType string

const (
    ErrorTypeTransient ErrorType = "Transient"  // 临时错误，可重试
    ErrorTypePermanent ErrorType = "Permanent"  // 永久错误，需人工干预
    ErrorTypeConfig    ErrorType = "Config"     // 配置错误
    ErrorTypeResource  ErrorType = "Resource"   // 资源错误
)

func (r *ReconcileError) IsRetryable() bool {
    return r.Type == ErrorTypeTransient || r.Type == ErrorTypeResource
}
```

### 6.2 重试策略

```go
func (r *BaseReconciler) handleError(err error) (ctrl.Result, error) {
    if err == nil {
        return ctrl.Result{}, nil
    }

    reconcileErr, ok := err.(*ReconcileError)
    if !ok {
        // 未知错误，使用默认重试
        return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
    }

    switch reconcileErr.Type {
    case ErrorTypeTransient:
        // 临时错误，短时间重试
        return ctrl.Result{RequeueAfter: time.Second * 30}, nil
    case ErrorTypeResource:
        // 资源错误，中等时间重试
        return ctrl.Result{RequeueAfter: time.Minute * 2}, nil
    case ErrorTypePermanent, ErrorTypeConfig:
        // 永久错误或配置错误，长时间重试
        return ctrl.Result{RequeueAfter: time.Minute * 10}, reconcileErr
    default:
        return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
    }
}
```

## 7. 监控和可观测性

### 7.1 指标定义

```go
var (
    reconcileTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "redis_operator_reconcile_total",
            Help: "Total number of reconcile operations",
        },
        []string{"controller", "result"},
    )

    reconcileDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "redis_operator_reconcile_duration_seconds",
            Help: "Duration of reconcile operations",
        },
        []string{"controller"},
    )

    redisInstancesTotal = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "redis_operator_instances_total",
            Help: "Total number of Redis instances",
        },
        []string{"type", "status"},
    )
)
```

### 7.2 事件记录

```go
func (r *BaseReconciler) recordEvent(object runtime.Object, eventType, reason, message string) {
    r.Recorder.Event(object, eventType, reason, message)
}

// 使用示例
r.recordEvent(masterReplica, corev1.EventTypeNormal, "Created", "Master-Replica setup created successfully")
r.recordEvent(masterReplica, corev1.EventTypeWarning, "Failed", "Failed to create master node")
```

## 8. 测试策略

### 8.1 单元测试

```go
func TestRedisMasterReplicaController_Reconcile(t *testing.T) {
    // 1. 设置测试环境
    scheme := runtime.NewScheme()
    _ = redisv1.AddToScheme(scheme)
    _ = corev1.AddToScheme(scheme)
    _ = appsv1.AddToScheme(scheme)

    // 2. 创建测试对象
    masterReplica := &redisv1.RedisMasterReplica{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "test-mr",
            Namespace: "default",
        },
        Spec: redisv1.RedisMasterReplicaSpec{
            Image: "redis:7.0",
            Master: redisv1.MasterSpec{},
            Replica: redisv1.ReplicaSpec{
                Replicas: 2,
            },
        },
    }

    // 3. 创建 fake client
    client := fake.NewClientBuilder().
        WithScheme(scheme).
        WithObjects(masterReplica).
        Build()

    // 4. 创建 reconciler
    reconciler := &RedisMasterReplicaReconciler{
        Client: client,
        Scheme: scheme,
    }

    // 5. 执行测试
    req := ctrl.Request{
        NamespacedName: types.NamespacedName{
            Name:      "test-mr",
            Namespace: "default",
        },
    }

    result, err := reconciler.Reconcile(context.TODO(), req)
    assert.NoError(t, err)
    assert.Equal(t, ctrl.Result{}, result)
}
```

### 8.2 集成测试

```go
func TestRedisMasterReplicaIntegration(t *testing.T) {
    // 使用 envtest 进行集成测试
    testEnv := &envtest.Environment{
        CRDDirectoryPaths: []string{filepath.Join("..", "config", "crd", "bases")},
    }

    cfg, err := testEnv.Start()
    require.NoError(t, err)
    defer testEnv.Stop()

    // 创建 client 和 reconciler
    k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
    require.NoError(t, err)

    reconciler := &RedisMasterReplicaReconciler{
        Client: k8sClient,
        Scheme: scheme,
    }

    // 执行集成测试逻辑
    // ...
}
```

## 9. 总结

本文档详细描述了 Redis Operator 中三种控制器的实现架构和核心逻辑：

1. **统一的控制器模式**：所有控制器都遵循标准的 Kubernetes Controller 模式
2. **模块化设计**：通用组件可以在不同控制器间复用
3. **完善的错误处理**：分类处理不同类型的错误，实现智能重试
4. **全面的监控**：提供指标、事件和日志等多维度的可观测性
5. **充分的测试**：单元测试和集成测试确保代码质量

这种设计确保了 Redis Operator 的可靠性、可维护性和可扩展性。