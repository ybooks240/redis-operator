# Redis 聚合视图使用指南

## 概述

Redis Operator 提供了一个统一的聚合视图功能，允许用户通过单一的 `kubectl get redis` 命令查看所有类型的 Redis 资源，包括：

- **RedisCluster** - Redis 集群模式
- **RedisInstance** - 单实例 Redis
- **RedisMasterReplica** - 主从复制模式
- **RedisSentinel** - 哨兵模式

## 功能特性

### 1. 统一查询接口

```bash
# 查看所有 Redis 资源
kubectl get redis

# 查看详细信息
kubectl get redis -o wide

# 查看所有命名空间的 Redis 资源
kubectl get redis --all-namespaces
```

### 2. 聚合状态信息

每个 Redis 聚合资源会显示以下信息：
- **TYPE**: Redis 资源类型（cluster/instance/masterreplica/sentinel）
- **READY**: 就绪状态
- **STATUS**: 当前状态
- **RESOURCE**: 引用的实际资源名称
- **AGE**: 创建时间
- **MESSAGE**: 详细状态信息

### 3. 跨命名空间支持

支持引用不同命名空间的 Redis 资源，实现跨命名空间的统一管理。

## 使用方法

### 步骤 1: 创建 Redis 聚合资源

为每个需要聚合管理的 Redis 资源创建对应的 `Redis` 聚合资源：

```yaml
apiVersion: redis.github.com/v1
kind: Redis
metadata:
  name: <聚合资源名称>
  namespace: <聚合资源命名空间>
spec:
  type: <Redis类型>  # cluster/instance/masterreplica/sentinel
  resourceName: <实际Redis资源名称>
  resourceNamespace: <实际Redis资源命名空间>  # 可选，默认为聚合资源的命名空间
```

### 步骤 2: 应用配置

```bash
kubectl apply -f redis-aggregated-view.yaml
```

### 步骤 3: 查看聚合结果

```bash
kubectl get redis
```

## 示例配置

### RedisSentinel 聚合视图

```yaml
apiVersion: redis.github.com/v1
kind: Redis
metadata:
  name: sentinel-view
  namespace: default
spec:
  type: sentinel
  resourceName: redissentinel-sample
  resourceNamespace: default
```

### RedisInstance 聚合视图

```yaml
apiVersion: redis.github.com/v1
kind: Redis
metadata:
  name: instance-view
  namespace: default
spec:
  type: instance
  resourceName: redisinstance-sample
  resourceNamespace: default
```

### RedisCluster 聚合视图

```yaml
apiVersion: redis.github.com/v1
kind: Redis
metadata:
  name: cluster-view
  namespace: default
spec:
  type: cluster
  resourceName: rediscluster-sample
  resourceNamespace: default
```

### RedisMasterReplica 聚合视图

```yaml
apiVersion: redis.github.com/v1
kind: Redis
metadata:
  name: masterreplica-view
  namespace: default
spec:
  type: masterreplica
  resourceName: redismasterreplica-sample
  resourceNamespace: default
```

## 实际使用示例

### 查看当前所有 Redis 资源

```bash
$ kubectl get redis
NAME                        TYPE       READY   STATUS    RESOURCE               AGE
redisinstance-sample-view   instance   True    Running   redisinstance-sample   5m
redissentinel-sample-view   sentinel   true    Running   redissentinel-sample   10m
```

### 查看详细信息

```bash
$ kubectl get redis -o wide
NAME                        TYPE       READY   STATUS    RESOURCE               AGE   MESSAGE
redisinstance-sample-view   instance   True    Running   redisinstance-sample   5m    RedisInstance is running. StatefulSet: 1, Replicas: 1
redissentinel-sample-view   sentinel   true    Running   redissentinel-sample   10m   All 3 sentinels are ready
```

### 查看特定资源的详细状态

```bash
kubectl describe redis redissentinel-sample-view
```

## 状态同步机制

Redis 聚合控制器会：

1. **监听变化**: 自动监听所有类型的 Redis 资源变化
2. **状态同步**: 实时同步底层资源的状态到聚合视图
3. **错误处理**: 当底层资源不存在时，显示 "NotFound" 状态
4. **自动更新**: 每30秒自动刷新状态信息

## 高级功能

### 跨命名空间聚合

```yaml
apiVersion: redis.github.com/v1
kind: Redis
metadata:
  name: cross-namespace-view
  namespace: monitoring
spec:
  type: sentinel
  resourceName: redis-monitoring
  resourceNamespace: redis-system
```

### 批量管理

可以在一个 YAML 文件中定义多个聚合资源：

```yaml
apiVersion: redis.github.com/v1
kind: Redis
metadata:
  name: prod-sentinel
  namespace: production
spec:
  type: sentinel
  resourceName: redis-sentinel-prod
---
apiVersion: redis.github.com/v1
kind: Redis
metadata:
  name: prod-cluster
  namespace: production
spec:
  type: cluster
  resourceName: redis-cluster-prod
```

## 故障排除

### 常见问题

1. **状态未更新**
   - 检查 Redis 控制器是否正常运行
   - 确认底层资源存在且状态正常

2. **资源未找到**
   - 验证 `resourceName` 和 `resourceNamespace` 配置正确
   - 检查底层资源是否存在

3. **权限问题**
   - 确认 Redis 控制器有足够的 RBAC 权限访问所有类型的 Redis 资源

### 调试命令

```bash
# 检查控制器日志
kubectl logs -l app=redis-operator -n redis-operator-system

# 检查底层资源状态
kubectl get redissentinel,rediscluster,redisinstance,redismasterreplica --all-namespaces

# 检查聚合资源详细信息
kubectl describe redis <resource-name>
```

## 最佳实践

1. **命名规范**: 使用描述性的聚合资源名称，如 `<type>-<env>-<purpose>`
2. **命名空间组织**: 将相关的聚合资源放在同一命名空间中
3. **监控集成**: 结合监控系统使用聚合视图进行统一监控
4. **自动化**: 在 CI/CD 流程中自动创建聚合资源

## 总结

Redis 聚合视图功能提供了一个统一的接口来管理和查看所有类型的 Redis 资源，简化了运维工作，提高了管理效率。通过 `kubectl get redis` 命令，用户可以快速了解整个集群中所有 Redis 资源的状态和健康情况。