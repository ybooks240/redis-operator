# Redis Operator Grafana Dashboard 使用指南

## 概述

这是一个专为 Redis Operator 设计的综合性 Grafana Dashboard，提供了对 Redis Operator 及其管理的各种 Redis 部署模式的全面监控。

## 功能特性

### 🎯 监控范围
- **Redis Operator 监控**: Controller 调谐统计、实例总数
- **单节点 Redis**: 连接数、内存使用、命令执行速率
- **Redis 集群**: 节点数、槽位状态、已知节点
- **Redis 哨兵**: 监控主节点数、运行脚本、已知节点
- **主从复制**: 从节点数、复制偏移量、复制延迟
- **系统资源**: CPU、内存、网络流量

### 📊 面板组织
1. **Redis Operator 概览** - Operator 整体状态
2. **Redis 单节点监控** - 单实例性能指标
3. **Redis 集群监控** - 集群健康状态
4. **Redis 哨兵监控** - 哨兵服务状态
5. **Redis 主从复制监控** - 复制关系监控
6. **系统资源监控** - 底层资源使用

## 前置条件

### 1. Prometheus 数据源
确保已配置 Prometheus 数据源，并且能够收集以下指标：

#### Redis Operator 指标
```
controller_runtime_reconcile_total
controller_runtime_active_workers
controller_runtime_max_concurrent_reconciles
```

#### Redis 实例指标
```
redis_connected_clients
redis_memory_used_bytes
redis_commands_processed_total
redis_up
```

#### Redis 集群指标
```
redis_cluster_nodes
redis_cluster_slots_assigned
redis_cluster_slots_ok
redis_cluster_known_nodes
```

#### Redis 哨兵指标
```
redis_sentinel_masters
redis_sentinel_running_scripts
redis_sentinel_known_sentinels
```

#### 主从复制指标
```
redis_connected_slaves
redis_master_repl_offset
redis_slave_repl_offset
redis_slave_lag_in_seconds
```

#### 系统指标（Node Exporter）
```
node_cpu_seconds_total
node_memory_MemAvailable_bytes
node_memory_MemTotal_bytes
node_network_receive_bytes_total
node_network_transmit_bytes_total
```

### 2. Redis Exporter 配置

为了收集 Redis 指标，需要部署 Redis Exporter：

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis-exporter
  namespace: monitoring
spec:
  replicas: 1
  selector:
    matchLabels:
      app: redis-exporter
  template:
    metadata:
      labels:
        app: redis-exporter
    spec:
      containers:
      - name: redis-exporter
        image: oliver006/redis_exporter:latest
        ports:
        - containerPort: 9121
        env:
        - name: REDIS_ADDR
          value: "redis://redis-service:6379"
---
apiVersion: v1
kind: Service
metadata:
  name: redis-exporter
  namespace: monitoring
  labels:
    app: redis-exporter
spec:
  ports:
  - port: 9121
    targetPort: 9121
    name: metrics
  selector:
    app: redis-exporter
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: redis-exporter
  namespace: monitoring
spec:
  selector:
    matchLabels:
      app: redis-exporter
  endpoints:
  - port: metrics
    interval: 30s
    path: /metrics
```

## 安装步骤

### 1. 导入 Dashboard

#### 方法一：通过 Grafana UI
1. 登录 Grafana
2. 点击左侧菜单 "Dashboards" → "Import"
3. 点击 "Upload JSON file" 或复制 JSON 内容
4. 选择 `redis-operator-comprehensive-dashboard.json` 文件
5. 配置数据源为你的 Prometheus 实例
6. 点击 "Import"

#### 方法二：通过 API
```bash
curl -X POST \
  http://admin:admin@localhost:3000/api/dashboards/db \
  -H 'Content-Type: application/json' \
  -d @redis-operator-comprehensive-dashboard.json
```

#### 方法三：通过 ConfigMap（Kubernetes）
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: redis-operator-dashboard
  namespace: monitoring
  labels:
    grafana_dashboard: "1"
data:
  redis-operator-dashboard.json: |
    # 将 JSON 内容粘贴到这里
```

### 2. 配置数据源

确保 Prometheus 数据源配置正确：

```yaml
apiVersion: 1
datasources:
- name: prometheus
  type: prometheus
  access: proxy
  url: http://prometheus-server:80
  isDefault: true
```

### 3. 验证安装

1. 打开 Dashboard
2. 检查所有面板是否正常显示数据
3. 验证变量（数据源、实例、命名空间）是否正常工作
4. 确认时间范围选择器功能正常

## 使用说明

### 变量配置

Dashboard 包含以下可配置变量：

- **数据源**: 选择 Prometheus 数据源
- **Redis 实例**: 过滤特定的 Redis 实例
- **命名空间**: 过滤特定的 Kubernetes 命名空间

### 面板说明

#### Operator 概览
- **调谐统计**: 显示各类型 Redis 资源的调谐次数
- **实例总数**: 当前运行的 Redis Pod 总数

#### 单节点监控
- **连接数**: 当前客户端连接数
- **内存使用**: Redis 实例内存消耗
- **命令执行速率**: 每秒处理的命令数

#### 集群监控
- **节点数**: 集群中的节点总数
- **槽位状态**: 已分配和正常的槽位数
- **已知节点**: 集群中已知的节点数

#### 哨兵监控
- **监控主节点数**: 哨兵监控的主节点数量
- **运行脚本**: 当前运行的脚本数
- **已知节点**: 哨兵已知的节点数

#### 主从复制监控
- **从节点数**: 连接到主节点的从节点数
- **复制偏移量**: 主从节点的复制偏移量对比
- **复制延迟**: 从节点的复制延迟时间

#### 系统资源监控
- **CPU 使用率**: 节点 CPU 使用百分比
- **内存使用率**: 节点内存使用百分比
- **网络流量**: 网络接收和发送速率

### 告警配置

可以基于以下指标配置告警：

```yaml
# Redis 实例下线
- alert: RedisInstanceDown
  expr: redis_up == 0
  for: 1m
  labels:
    severity: critical
  annotations:
    summary: "Redis instance is down"
    description: "Redis instance {{ $labels.instance }} has been down for more than 1 minute."

# Redis 内存使用过高
- alert: RedisHighMemoryUsage
  expr: redis_memory_used_bytes / redis_memory_max_bytes > 0.9
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "Redis high memory usage"
    description: "Redis instance {{ $labels.instance }} memory usage is above 90%."

# 集群槽位异常
- alert: RedisClusterSlotsNotOK
  expr: redis_cluster_slots_ok < redis_cluster_slots_assigned
  for: 2m
  labels:
    severity: critical
  annotations:
    summary: "Redis cluster slots not OK"
    description: "Redis cluster has slots that are not OK."

# 主从复制延迟过高
- alert: RedisSlaveHighLag
  expr: redis_slave_lag_in_seconds > 10
  for: 3m
  labels:
    severity: warning
  annotations:
    summary: "Redis slave high lag"
    description: "Redis slave {{ $labels.instance }} lag is {{ $value }} seconds."
```

## 故障排除

### 常见问题

#### 1. 面板显示 "No data"
**原因**: 
- Prometheus 数据源配置错误
- Redis Exporter 未正确部署
- 指标标签不匹配

**解决方案**:
```bash
# 检查 Prometheus 目标状态
kubectl port-forward svc/prometheus-server 9090:80 -n monitoring
# 访问 http://localhost:9090/targets

# 检查 Redis Exporter 状态
kubectl get pods -l app=redis-exporter -n monitoring
kubectl logs -l app=redis-exporter -n monitoring

# 验证指标可用性
curl http://localhost:9090/api/v1/query?query=redis_up
```

#### 2. 变量下拉列表为空
**原因**: 
- Prometheus 查询语法错误
- 标签值不存在

**解决方案**:
```bash
# 检查可用标签
curl 'http://localhost:9090/api/v1/label/instance/values'
curl 'http://localhost:9090/api/v1/label/namespace/values'
```

#### 3. 集群指标缺失
**原因**: 
- Redis 集群未启用指标导出
- ServiceMonitor 配置错误

**解决方案**:
```yaml
# 确保 Redis 集群配置包含指标端口
apiVersion: redis.redis.opstreelabs.in/v1beta1
kind: RedisCluster
metadata:
  name: redis-cluster
spec:
  clusterSize: 3
  redisExporter:
    enabled: true
    image: oliver006/redis_exporter:latest
```

#### 4. 哨兵指标不准确
**原因**: 
- 哨兵配置不正确
- 指标收集间隔过长

**解决方案**:
```bash
# 检查哨兵状态
redis-cli -h sentinel-host -p 26379 SENTINEL masters
redis-cli -h sentinel-host -p 26379 SENTINEL sentinels mymaster
```

### 性能优化

#### 1. 减少查询负载
- 调整刷新间隔（默认 30 秒）
- 使用更短的时间范围
- 优化 Prometheus 查询

#### 2. 提高响应速度
- 增加 Prometheus 内存配置
- 使用 SSD 存储
- 配置适当的保留策略

```yaml
# Prometheus 配置优化
global:
  scrape_interval: 15s
  evaluation_interval: 15s

rule_files:
  - "redis_rules.yml"

scrape_configs:
  - job_name: 'redis-exporter'
    static_configs:
      - targets: ['redis-exporter:9121']
    scrape_interval: 30s
    metrics_path: /metrics
```

## 扩展功能

### 1. 自定义面板

可以根据需要添加更多面板：

```json
{
  "title": "自定义 Redis 指标",
  "type": "timeseries",
  "targets": [
    {
      "expr": "your_custom_redis_metric",
      "legendFormat": "自定义指标"
    }
  ]
}
```

### 2. 集成其他数据源

可以集成 Loki 日志或 Jaeger 链路追踪：

```json
{
  "title": "Redis 日志",
  "type": "logs",
  "datasource": {
    "type": "loki",
    "uid": "loki"
  },
  "targets": [
    {
      "expr": "{app=\"redis\"}"
    }
  ]
}
```

### 3. 导出和分享

```bash
# 导出 Dashboard JSON
curl -H "Authorization: Bearer $API_KEY" \
     "http://localhost:3000/api/dashboards/uid/redis-operator-dashboard" \
     | jq '.dashboard' > exported-dashboard.json

# 创建快照
curl -X POST \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"dashboard":{"uid":"redis-operator-dashboard"},"expires":3600}' \
  "http://localhost:3000/api/snapshots"
```

## 维护和更新

### 定期检查
- 验证所有指标正常收集
- 检查告警规则有效性
- 更新 Dashboard 版本
- 清理过期数据

### 版本升级
1. 备份当前 Dashboard 配置
2. 测试新版本兼容性
3. 逐步部署更新
4. 验证功能正常

## 支持和反馈

如果遇到问题或有改进建议，请：
1. 检查本文档的故障排除部分
2. 查看 Grafana 和 Prometheus 日志
3. 提交 Issue 或 Pull Request
4. 参与社区讨论

---

**注意**: 此 Dashboard 需要 Grafana 7.0+ 版本，建议使用 Grafana 9.0+ 以获得最佳体验。