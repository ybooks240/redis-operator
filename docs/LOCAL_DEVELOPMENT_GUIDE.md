# 本地开发指南

## 问题描述

在本地开发环境中运行 Redis Operator 时，会出现以下错误：

```
Failed to collect Redis metrics
Failed to get sentinel masters
Failed to collect Cluster metrics
dial tcp: lookup redissentinel-sample-sentinel-service.default.svc.cluster.local: i/o timeout
```

这些错误是因为 Operator 尝试连接 Kubernetes 集群内的 Redis 服务，但在本地开发环境中这些服务并不存在。

## 解决方案

我们添加了一个新的命令行标志 `--enable-metrics-collection` 来控制是否启用 Redis 指标收集功能。

### 本地开发模式（默认）

```bash
# 默认情况下，指标收集是禁用的，适合本地开发
make run
```

或者显式禁用：

```bash
go run ./cmd/main.go --enable-metrics-collection=false
```

### 生产环境模式

在生产环境中，当 Redis 服务实际存在时，可以启用指标收集：

```bash
go run ./cmd/main.go --enable-metrics-collection=true
```

## 指标收集功能

当启用指标收集时，Operator 会：

1. 收集 Redis 实例的性能指标
2. 收集 Redis Sentinel 的状态指标
3. 收集 Redis Cluster 的集群指标
4. 将指标暴露给 Prometheus 进行监控

当禁用指标收集时，Operator 仍然可以正常管理 Redis 资源，只是不会尝试连接 Redis 服务收集指标。

## Grafana 监控

- **本地开发**：由于指标收集被禁用，Grafana 中不会显示 Redis 相关指标
- **生产环境**：启用指标收集后，可以在 Grafana 中查看完整的 Redis 监控仪表板

## 注意事项

1. 这个标志主要用于解决本地开发环境的连接问题
2. 在生产环境中建议启用指标收集以获得完整的监控能力
3. 即使禁用指标收集，Operator 的核心功能（创建、更新、删除 Redis 资源）仍然正常工作