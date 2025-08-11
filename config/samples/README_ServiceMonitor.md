# Redis Operator ServiceMonitor 示例

本目录包含了为 Redis Operator 管理的各种 Redis 自定义资源（CR）配置 Prometheus 监控的 ServiceMonitor 示例文件。

## 📁 文件列表

| 文件名 | 描述 | 适用场景 |
|--------|------|----------|
| `servicemonitor_redisinstance.yaml` | RedisInstance 监控配置 | 单实例 Redis 监控 |
| `servicemonitor_redismasterreplica.yaml` | RedisMasterReplica 监控配置 | 主从架构 Redis 监控 |
| `servicemonitor_redissentinel.yaml` | RedisSentinel 监控配置 | 高可用 Redis 监控 |
| `servicemonitor_rediscluster.yaml` | RedisCluster 监控配置 | 集群架构 Redis 监控 |
| `servicemonitor_comprehensive.yaml` | 综合监控配置 | 统一监控所有类型 |

## 🚀 快速开始

### 前提条件

1. **Prometheus Operator 已安装**
   ```bash
   # 检查 Prometheus Operator
   kubectl get crd servicemonitors.monitoring.coreos.com
   ```

2. **Redis Operator 已部署**
   ```bash
   # 检查 Redis Operator
   kubectl get deployment redis-operator-controller-manager -n redis-operator-system
   ```

3. **Prometheus 配置了 ServiceMonitor 选择器**
   ```yaml
   # Prometheus 配置示例
   spec:
     serviceMonitorSelector:
       matchLabels:
         app.kubernetes.io/name: redis-operator
   ```

### 基础使用

#### 1. 为特定类型的 Redis CR 配置监控

```bash
# 为 RedisInstance 配置监控
kubectl apply -f servicemonitor_redisinstance.yaml

# 为 RedisMasterReplica 配置监控
kubectl apply -f servicemonitor_redismasterreplica.yaml

# 为 RedisSentinel 配置监控
kubectl apply -f servicemonitor_redissentinel.yaml

# 为 RedisCluster 配置监控
kubectl apply -f servicemonitor_rediscluster.yaml
```

#### 2. 使用综合监控配置

```bash
# 一次性配置所有类型的监控
kubectl apply -f servicemonitor_comprehensive.yaml
```

## 📊 监控配置详解

### RedisInstance 监控

**监控目标**: 单实例 Redis 服务

**关键指标**:
- `redis_instance_status`: 实例状态
- `redis_instance_memory_usage_bytes`: 内存使用量
- `redis_instance_connected_clients`: 连接客户端数
- `redis_instance_commands_processed_total`: 处理命令总数

**服务发现**:
```yaml
selector:
  matchLabels:
    app: redis
    instance: redisinstance-sample
```

### RedisMasterReplica 监控

**监控目标**: 主从架构 Redis 服务

**监控策略**:
- 通用监控: 监控所有节点
- Master 专用监控: 15s 间隔
- Replica 专用监控: 30s 间隔

**关键标签**:
- `redis_role`: master/replica
- `redis_instance`: 实例名称

### RedisSentinel 监控

**监控目标**: 高可用 Redis 架构

**监控组件**:
- Sentinel 节点 (端口 26379)
- Redis Master 节点 (端口 6379)
- Redis Replica 节点 (端口 6379)

**关键指标**:
- `sentinel_masters`: 监控的主节点数
- `sentinel_sentinels`: Sentinel 节点数
- `sentinel_master_status`: 主节点状态
- `sentinel_failovers_total`: 故障转移次数

### RedisCluster 监控

**监控目标**: 集群架构 Redis 服务

**监控策略**:
- Master 节点: 15s 间隔
- Replica 节点: 30s 间隔
- 集群整体: 60s 间隔
- 节点发现: 通过 Headless Service

**关键指标**:
- `cluster_nodes`: 集群节点总数
- `cluster_slots_assigned`: 已分配槽位数
- `cluster_size`: 集群大小
- `cluster_known_nodes`: 已知节点数

## 🔧 自定义配置

### 修改监控间隔

```yaml
endpoints:
- port: redis
  interval: 15s  # 修改为所需间隔
  scrapeTimeout: 10s
```

### 添加自定义标签

```yaml
relabelings:
- replacement: 'production'
  targetLabel: environment
- sourceLabels: [__meta_kubernetes_service_label_tier]
  targetLabel: service_tier
```

### 配置多命名空间监控

```yaml
namespaceSelector:
  matchNames:
  - default
  - redis-system
  - production
```

### 标记关键服务

```bash
# 为关键服务添加标签
kubectl label service my-redis-service monitoring.redis.io/critical=true
```

## 📈 Grafana 集成

### 导入仪表板

1. 使用预配置的仪表板:
   ```bash
   kubectl apply -f ../monitoring/grafana-dashboard.json
   ```

2. 或者在 Grafana 中手动导入仪表板

### 常用查询示例

```promql
# Redis 实例状态
redis_instance_status

# Redis 内存使用率
redis_instance_memory_usage_bytes / redis_instance_memory_limit_bytes * 100

# Redis 连接数趋势
rate(redis_instance_connected_clients[5m])

# Sentinel 故障转移次数
increase(sentinel_failovers_total[1h])

# 集群节点状态
cluster_nodes{redis_role="master"}
```

## 🚨 告警配置

### 基础告警规则

```yaml
groups:
- name: redis.rules
  rules:
  - alert: RedisInstanceDown
    expr: redis_instance_status == 0
    for: 1m
    labels:
      severity: critical
    annotations:
      summary: "Redis instance is down"
  
  - alert: RedisHighMemoryUsage
    expr: redis_instance_memory_usage_bytes / redis_instance_memory_limit_bytes > 0.9
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "Redis high memory usage"
```

## 🔍 故障排除

### 检查 ServiceMonitor 状态

```bash
# 查看 ServiceMonitor
kubectl get servicemonitor -n default

# 查看详细信息
kubectl describe servicemonitor redis-instance-monitor
```

### 验证服务发现

```bash
# 检查服务标签
kubectl get svc -l app=redis --show-labels

# 检查端点
kubectl get endpoints
```

### Prometheus 目标检查

1. 访问 Prometheus Web UI
2. 进入 Status > Targets
3. 查找 Redis 相关的目标
4. 检查状态和错误信息

### 常见问题

| 问题 | 原因 | 解决方案 |
|------|------|----------|
| 目标未发现 | 标签选择器不匹配 | 检查服务标签和选择器 |
| 抓取失败 | 端口名称错误 | 验证服务端口定义 |
| 无指标数据 | 指标端点不存在 | 确认 Redis Exporter 配置 |
| 权限错误 | RBAC 配置问题 | 检查 Prometheus 权限 |

## 📚 参考资料

- [Prometheus Operator 文档](https://prometheus-operator.dev/)
- [ServiceMonitor 规范](https://prometheus-operator.dev/docs/operator/api/#servicemonitor)
- [Redis Exporter](https://github.com/oliver006/redis_exporter)
- [Grafana Redis 仪表板](https://grafana.com/grafana/dashboards/763)

## 🤝 贡献

如果您有改进建议或发现问题，请：

1. 提交 Issue 描述问题
2. 提供 Pull Request 改进配置
3. 分享您的监控最佳实践

## 📄 许可证

本项目采用 Apache 2.0 许可证。详见 [LICENSE](../../LICENSE) 文件。