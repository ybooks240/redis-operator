# Redis 部署模式设计文档

## 概述

本文档详细描述了 Redis Operator 中三种主要部署模式的设计理念、架构特点和实现方案：

1. **RedisMasterReplica** - 一主多从模式
2. **RedisSentinel** - 哨兵模式
3. **RedisCluster** - 集群模式

## 1. RedisMasterReplica（一主多从模式）

### 1.1 设计缘由

一主多从模式是 Redis 最基础的高可用部署方式，适用于以下场景：

- **读写分离需求**：主节点处理写操作，从节点处理读操作，提高系统吞吐量
- **数据备份**：从节点作为主节点的实时备份，防止数据丢失
- **简单高可用**：在主节点故障时，可以手动或通过外部工具切换到从节点
- **开发测试环境**：提供简单的 Redis 集群环境用于开发和测试

### 1.2 架构设计

```
┌─────────────────┐    ┌─────────────────┐
│   Master Node   │───▶│  Replica Node 1 │
│   (Read/Write)  │    │   (Read Only)   │
└─────────────────┘    └─────────────────┘
         │              
         ▼              
┌─────────────────┐    
│  Replica Node 2 │    
│   (Read Only)   │    
└─────────────────┘    
```

### 1.3 核心特性

- **主从复制**：主节点的所有写操作自动同步到从节点
- **读写分离**：主节点处理写操作，从节点处理读操作
- **独立配置**：主节点和从节点可以有不同的资源配置和存储配置
- **安全认证**：支持密码认证和 TLS 加密
- **资源隔离**：每个节点可以配置独立的资源限制

### 1.4 关键配置

```yaml
apiVersion: redis.github.com/v1
kind: RedisMasterReplica
metadata:
  name: redis-master-replica
spec:
  image: redis:7.0
  master:
    resources:
      requests:
        memory: "1Gi"
        cpu: "500m"
    storage:
      size: "10Gi"
      storageClassName: "fast-ssd"
  replica:
    replicas: 2
    resources:
      requests:
        memory: "512Mi"
        cpu: "250m"
    storage:
      size: "10Gi"
      storageClassName: "standard"
  security:
    authEnabled: true
    passwordSecret:
      name: redis-auth
      key: password
```

## 2. RedisSentinel（哨兵模式）

### 2.1 设计缘由

哨兵模式在一主多从的基础上增加了自动故障转移能力，适用于以下场景：

- **自动故障转移**：当主节点故障时，哨兵自动选举新的主节点
- **服务发现**：客户端通过哨兵获取当前主节点信息
- **监控告警**：哨兵持续监控 Redis 节点状态，提供故障告警
- **生产环境高可用**：提供企业级的 Redis 高可用解决方案

### 2.2 架构设计

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Sentinel 1    │    │   Sentinel 2    │    │   Sentinel 3    │
│   (Monitor)     │    │   (Monitor)     │    │   (Monitor)     │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         ▼                       ▼                       ▼
┌─────────────────────────────────────────────────────────────────┐
│                    RedisMasterReplica                           │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐          │
│  │   Master    │───▶│  Replica 1  │    │  Replica 2  │          │
│  │ (Read/Write)│    │ (Read Only) │    │ (Read Only) │          │
│  └─────────────┘    └─────────────┘    └─────────────┘          │
└─────────────────────────────────────────────────────────────────┘
```

### 2.3 核心特性

- **自动故障检测**：哨兵持续监控主从节点的健康状态
- **自动故障转移**：主节点故障时自动选举新主节点
- **配置管理**：动态更新 Redis 配置，无需重启
- **客户端通知**：通知客户端主节点变更信息
- **多哨兵协调**：多个哨兵节点协同工作，避免脑裂

### 2.4 关键配置

```yaml
apiVersion: redis.github.com/v1
kind: RedisSentinel
metadata:
  name: redis-sentinel
spec:
  image: redis:7.0
  replicas: 3
  config:
    quorum: 2
    downAfterMilliseconds: 30000
    failoverTimeout: 180000
    parallelSyncs: 1
  masterReplicaRef:
    name: redis-master-replica
    masterName: mymaster
  resources:
    requests:
      memory: "256Mi"
      cpu: "100m"
```

## 3. RedisCluster（集群模式）

### 3.1 设计缘由

集群模式是 Redis 的分布式解决方案，适用于以下场景：

- **水平扩展**：通过增加节点来扩展存储容量和处理能力
- **数据分片**：自动将数据分布到多个节点，突破单机内存限制
- **高可用性**：内置故障转移机制，无需外部组件
- **大规模应用**：支持 TB 级数据存储和高并发访问

### 3.2 架构设计

```
┌─────────────────────────────────────────────────────────────────┐
│                        Redis Cluster                           │
│                                                                 │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐        │
│  │  Master 1   │    │  Master 2   │    │  Master 3   │        │
│  │ Slots:0-5460│    │Slots:5461-  │    │Slots:10923- │        │
│  │             │    │    10922    │    │    16383    │        │
│  └─────────────┘    └─────────────┘    └─────────────┘        │
│         │                   │                   │              │
│         ▼                   ▼                   ▼              │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐        │
│  │  Replica 1  │    │  Replica 2  │    │  Replica 3  │        │
│  │   (Slave)   │    │   (Slave)   │    │   (Slave)   │        │
│  └─────────────┘    └─────────────┘    └─────────────┘        │
└─────────────────────────────────────────────────────────────────┘
```

### 3.3 核心特性

- **数据分片**：使用一致性哈希将数据分布到 16384 个槽位
- **自动故障转移**：主节点故障时从节点自动提升为主节点
- **动态扩缩容**：支持在线添加或移除节点
- **客户端路由**：客户端直接连接到数据所在的节点
- **无中心架构**：所有节点地位平等，无单点故障

### 3.4 关键配置

```yaml
apiVersion: redis.github.com/v1
kind: RedisCluster
metadata:
  name: redis-cluster
spec:
  image: redis:7.0
  masters: 3
  replicasPerMaster: 1
  config:
    clusterNodeTimeout: 15000
    clusterRequireFullCoverage: yes
    clusterMigrationBarrier: 1
  resources:
    requests:
      memory: "2Gi"
      cpu: "1000m"
  storage:
    size: "20Gi"
    storageClassName: "fast-ssd"
```

## 4. 部署模式对比

| 特性 | MasterReplica | Sentinel | Cluster |
|------|---------------|----------|----------|
| **复杂度** | 低 | 中 | 高 |
| **故障转移** | 手动 | 自动 | 自动 |
| **数据分片** | 否 | 否 | 是 |
| **扩展性** | 垂直扩展 | 垂直扩展 | 水平扩展 |
| **最小节点数** | 2 | 5 (3哨兵+2Redis) | 6 (3主+3从) |
| **适用场景** | 开发测试 | 生产环境 | 大规模生产 |
| **数据容量** | 单机限制 | 单机限制 | 无限制 |
| **客户端复杂度** | 低 | 中 | 高 |

## 5. 技术实现要点

### 5.1 共同设计原则

1. **声明式 API**：所有资源都遵循 Kubernetes 声明式 API 设计
2. **状态管理**：使用 Conditions 模式管理资源状态
3. **事件驱动**：基于 Controller 模式实现事件驱动的状态同步
4. **资源隔离**：支持独立的资源配置和存储配置
5. **安全性**：内置认证、授权和加密支持

### 5.2 状态管理

每种部署模式都包含以下状态字段：

- **Conditions**：详细的状态条件信息
- **Ready**：资源是否就绪
- **Status**：当前状态（Creating、Running、Failed 等）
- **LastConditionMessage**：最后状态变更消息

### 5.3 监控和可观测性

- **kubectl 输出**：自定义列显示关键状态信息
- **事件记录**：记录重要的状态变更事件
- **指标暴露**：暴露 Prometheus 指标用于监控
- **日志记录**：结构化日志记录便于问题排查

## 6. 最佳实践建议

### 6.1 选择合适的部署模式

- **开发测试环境**：使用 RedisMasterReplica
- **生产环境（中小规模）**：使用 RedisSentinel
- **生产环境（大规模）**：使用 RedisCluster

### 6.2 资源配置建议

- **CPU**：Redis 是单线程应用，单核性能比多核数量更重要
- **内存**：预留足够的内存用于数据存储和操作系统缓存
- **存储**：使用 SSD 存储提高 I/O 性能
- **网络**：确保节点间网络延迟低于 1ms

### 6.3 安全配置建议

- **启用认证**：生产环境必须启用密码认证
- **网络隔离**：使用 NetworkPolicy 限制网络访问
- **TLS 加密**：敏感数据传输使用 TLS 加密
- **定期备份**：配置定期数据备份策略

## 7. 总结

本设计文档详细阐述了三种 Redis 部署模式的设计理念和实现方案。每种模式都有其特定的适用场景和技术特点：

- **RedisMasterReplica** 提供了简单可靠的主从复制方案
- **RedisSentinel** 在主从基础上增加了自动故障转移能力
- **RedisCluster** 提供了完整的分布式 Redis 解决方案

通过合理选择和配置这些部署模式，可以满足从开发测试到大规模生产环境的各种 Redis 使用需求。