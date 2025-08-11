# Redis Sentinel 改进方案

## 问题描述

1. **DNS 解析问题**：Redis Sentinel 无法解析主机名，导致 Pod 持续处于 `CrashLoopBackOff` 状态
2. **主从节点架构问题**：用户询问 `redismasterreplica-sample-master-0` 和 `redismasterreplica-sample-replica-0` 是否可以使用同一个 StatefulSet

## 解决方案

### 1. 自动获取 Service IP 地址

**修改内容**：
- 在 `RedisSentinelReconciler` 中添加 `getMasterServiceIP` 方法
- 修改 `ensureSentinelConfigMap` 方法，动态获取主节点 Service 的 ClusterIP
- 更新 `configMapForSentinel` 方法，使用 IP 地址而非主机名

**实现逻辑**：
```go
// 获取主节点Service的ClusterIP
func (r *RedisSentinelReconciler) getMasterServiceIP(ctx context.Context, redisSentinel *redisv1.RedisSentinel) (string, error) {
    if redisSentinel.Spec.MasterReplicaRef.Name == "" {
        return "redis-master", nil // 默认主机名
    }

    // 获取主节点Service
    masterServiceName := redisSentinel.Spec.MasterReplicaRef.Name + "-master-service"
    masterService := &corev1.Service{}
    err := r.Get(ctx, types.NamespacedName{Name: masterServiceName, Namespace: redisSentinel.Namespace}, masterService)
    if err != nil {
        if errors.IsNotFound(err) {
            // Service不存在，使用FQDN作为fallback
            return fmt.Sprintf("%s.%s.svc.cluster.local", masterServiceName, redisSentinel.Namespace), nil
        }
        return "", err
    }

    // 返回ClusterIP
    if masterService.Spec.ClusterIP != "" && masterService.Spec.ClusterIP != "None" {
        return masterService.Spec.ClusterIP, nil
    }

    // 如果没有ClusterIP，使用FQDN
    return fmt.Sprintf("%s.%s.svc.cluster.local", masterServiceName, redisSentinel.Namespace), nil
}
```

**优势**：
- 自动检测和使用 Service 的 ClusterIP
- 提供 FQDN 作为 fallback 机制
- 支持 ConfigMap 的动态更新

### 2. 主从节点架构分析

**当前架构**：
- **主节点**：使用独立的 StatefulSet (`redismasterreplica-sample-master`)
- **从节点**：使用独立的 StatefulSet (`redismasterreplica-sample-replica`)

**为什么不能合并为一个 StatefulSet**：

#### 技术原因

1. **配置差异**：
   - 主节点：不需要 `replicaof` 配置
   - 从节点：需要 `replicaof <master-ip> <master-port>` 配置

2. **角色固定性**：
   - StatefulSet 中的 Pod 具有固定的序号和身份
   - 主节点通常是 `master-0`，从节点是 `replica-0`, `replica-1` 等
   - 合并后无法区分哪个 Pod 应该是主节点

3. **故障转移复杂性**：
   - Redis Sentinel 进行故障转移时，需要将某个从节点提升为主节点
   - 如果使用同一个 StatefulSet，Pod 的角色变更会变得复杂
   - 需要动态修改配置文件，增加了实现复杂度

#### 哨兵模式的特殊考虑

**用户提到的问题**："哨兵模式多次变动无法识别那个为当前master节点"

**解决方案**：

1. **保持当前架构**：
   - 主节点和从节点使用独立的 StatefulSet
   - 通过 Service 提供稳定的网络标识
   - Sentinel 通过 Service IP 监控主节点

2. **Sentinel 自动发现**：
   - Sentinel 会自动发现主从关系
   - 当主节点故障时，Sentinel 会选举新的主节点
   - 新的主节点信息会自动更新到 Sentinel 配置中

3. **Service 抽象**：
   - `master-service` 始终指向当前的主节点
   - 即使发生故障转移，Service 的 ClusterIP 保持不变
   - Sentinel 配置使用 Service IP，确保连接稳定性

### 3. 推荐的最佳实践

#### 架构设计

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Sentinel-0    │    │   Sentinel-1    │    │   Sentinel-2    │
│                 │    │                 │    │                 │
│ Monitor Master  │    │ Monitor Master  │    │ Monitor Master  │
│ via Service IP  │    │ via Service IP  │    │ via Service IP  │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         └───────────────────────┼───────────────────────┘
                                 │
                                 ▼
                    ┌─────────────────────┐
                    │   Master Service    │
                    │   (ClusterIP)       │
                    └─────────────────────┘
                                 │
                                 ▼
┌─────────────────┐    ┌─────────────────┐
│   Master-0      │    │   Replica-0     │
│ (StatefulSet)   │    │ (StatefulSet)   │
│                 │◄───┤                 │
│ Primary Redis   │    │ Replica Redis   │
└─────────────────┘    └─────────────────┘
```

#### 配置管理

1. **动态 IP 获取**：控制器自动获取 Service ClusterIP
2. **配置更新**：当 Service IP 变化时，自动更新 Sentinel ConfigMap
3. **故障恢复**：Sentinel 自动处理主从切换，无需手动干预

## 总结

1. **不建议合并主从 StatefulSet**：
   - 配置复杂性增加
   - 故障转移逻辑复杂
   - 违反单一职责原则

2. **推荐当前架构**：
   - 主从分离，职责清晰
   - 通过 Service 提供稳定的网络抽象
   - Sentinel 使用 Service IP 监控，避免 DNS 解析问题

3. **改进措施**：
   - 自动获取和更新 Service IP
   - 提供 FQDN fallback 机制
   - 支持配置的动态更新

这种设计既保持了架构的简洁性，又解决了 DNS 解析和主节点识别的问题。