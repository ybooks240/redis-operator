# ConfigMap 修改自动重启 StatefulSet 功能

## 功能概述

本功能实现了当 RedisInstance 的配置发生变化时，自动重启对应的 StatefulSet，确保 Redis 实例使用最新的配置。

## 实现原理

1. **配置哈希值跟踪**: 使用 SHA256 算法计算配置内容的哈希值
2. **配置变更检测**: 比较期望配置与当前配置的哈希值
3. **自动重启机制**: 当检测到配置变更时，删除并重新创建 StatefulSet
4. **配置挂载**: StatefulSet 通过 ConfigMap 卷挂载配置文件到 `/usr/local/etc/redis/redis.conf`

## 关键特性

### 1. 配置文件挂载
- ConfigMap 中的 `redis.conf` 被挂载到容器的 `/usr/local/etc/redis/` 目录
- Redis 服务器启动时使用该配置文件: `redis-server /usr/local/etc/redis/redis.conf`

### 2. 配置变更检测
- 每次 Reconcile 时检查 RedisInstance.Spec.Config 是否发生变化
- 使用 SHA256 哈希值进行精确比较
- 在 StatefulSet 的 Pod Template Annotations 中存储配置哈希值

### 3. 自动重启逻辑
当满足以下任一条件时，StatefulSet 会被自动重启：
- ConfigMap 不存在（首次创建）
- 期望配置与当前 ConfigMap 中的配置不同
- StatefulSet 的 annotation 中的配置哈希值与期望配置哈希值不同

## 使用示例

### 1. 创建 RedisInstance

```yaml
apiVersion: redis.github.com/v1
kind: RedisInstance
metadata:
  name: redis-example
spec:
  image: redis:7.0
  replicas: 1
  storage:
    size: 1Gi
    storageClassName: standard
  config:
    maxmemory: 100mb
    maxmemory-policy: allkeys-lru
    appendonly: "yes"
```

### 2. 修改配置触发重启

```yaml
apiVersion: redis.github.com/v1
kind: RedisInstance
metadata:
  name: redis-example
spec:
  image: redis:7.0
  replicas: 1
  storage:
    size: 1Gi
    storageClassName: standard
  config:
    maxmemory: 200mb  # 修改内存限制
    maxmemory-policy: allkeys-lfu  # 修改淘汰策略
    appendonly: "yes"
    save: "900 1 300 10 60 10000"  # 添加新配置
```

## 实现细节

### 核心方法

1. **calculateConfigHash()**: 计算配置的 SHA256 哈希值
2. **needsStatefulSetRestart()**: 检查是否需要重启 StatefulSet
3. **ensureResources()**: 确保所有资源状态正确，包括配置变更检测

### 配置哈希值存储

配置哈希值存储在 StatefulSet 的 Pod Template Annotations 中：
```yaml
spec:
  template:
    metadata:
      annotations:
        redis.github.com/config-hash: "abc123..."
```

### 重启流程

1. 检测到配置变更
2. 记录日志: "ConfigMap was updated or StatefulSet needs restart"
3. 移除 StatefulSet 的 finalizer
4. 删除现有 StatefulSet
5. 等待 5 秒确保删除完成
6. 创建新的 StatefulSet，包含最新的配置哈希值

## 日志监控

可以通过以下日志消息监控配置变更和重启过程：

- `ConfigMap configuration changed, updating`: ConfigMap 配置已更新
- `Config hash comparison`: 配置哈希值比较结果
- `ConfigMap was updated or StatefulSet needs restart`: StatefulSet 需要重启
- `StatefulSet not found or needs recreation, creating new one`: 正在重新创建 StatefulSet

## 注意事项

1. **重启过程**: StatefulSet 重启会导致 Redis 服务短暂不可用
2. **数据持久化**: 使用 PVC 确保数据在重启过程中不丢失
3. **配置验证**: 确保 Redis 配置语法正确，避免启动失败
4. **资源清理**: 旧的 StatefulSet 会被自动清理，不会留下孤儿资源

## 测试验证

1. 部署 RedisInstance
2. 检查 ConfigMap 和 StatefulSet 是否正确创建
3. 修改 RedisInstance 的 config 字段
4. 观察 StatefulSet 是否自动重启
5. 验证新配置是否生效