# Redis Operator 监控、告警和日志采集系统集成方案

## 概述

本文档描述了如何为 Redis Operator 集成完整的监控、告警和日志采集系统，包括：
- Prometheus 监控指标收集
- Grafana 可视化仪表板
- AlertManager 告警管理
- ELK/EFK 日志采集和分析
- Jaeger 分布式链路追踪

## 1. 监控系统 (Prometheus + Grafana)

### 1.1 Prometheus 监控指标

#### 控制器级别指标
- `redis_operator_reconcile_total` - 协调操作总数
- `redis_operator_reconcile_duration_seconds` - 协调操作耗时
- `redis_operator_reconcile_errors_total` - 协调操作错误数
- `redis_operator_resource_status_total` - 资源状态统计

#### Redis 实例级别指标
- `redis_instance_status` - Redis 实例状态 (0=down, 1=up)
- `redis_instance_memory_usage_bytes` - 内存使用量
- `redis_instance_connected_clients` - 连接客户端数
- `redis_instance_commands_processed_total` - 处理命令总数
- `redis_instance_keyspace_hits_total` - 键空间命中数
- `redis_instance_keyspace_misses_total` - 键空间未命中数

#### Redis Sentinel 级别指标
- `redis_sentinel_masters_total` - 监控的主节点数
- `redis_sentinel_sentinels_total` - Sentinel 节点数
- `redis_sentinel_failover_total` - 故障转移次数
- `redis_sentinel_master_status` - 主节点状态

#### Redis Cluster 级别指标
- `redis_cluster_nodes_total` - 集群节点总数
- `redis_cluster_slots_assigned` - 已分配槽位数
- `redis_cluster_state` - 集群状态 (0=fail, 1=ok)

### 1.2 ServiceMonitor 配置

当前已有基础的 ServiceMonitor 配置，需要扩展以支持更多指标：

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: redis-operator-metrics
  namespace: redis-operator-system
spec:
  endpoints:
  - path: /metrics
    port: https
    scheme: https
    interval: 30s
    scrapeTimeout: 10s
  - path: /redis-metrics
    port: redis-metrics
    scheme: http
    interval: 15s
  selector:
    matchLabels:
      app.kubernetes.io/name: redis-operator
```

### 1.3 Grafana 仪表板

需要创建以下仪表板：
1. **Redis Operator 概览** - 整体运行状态
2. **Redis 实例监控** - 单个实例详细指标
3. **Redis Sentinel 监控** - 高可用监控
4. **Redis Cluster 监控** - 集群状态监控
5. **性能分析** - 性能指标和趋势

## 2. 告警系统 (AlertManager)

### 2.1 告警规则

#### 控制器告警
```yaml
groups:
- name: redis-operator.rules
  rules:
  - alert: RedisOperatorDown
    expr: up{job="redis-operator-metrics"} == 0
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "Redis Operator is down"
      description: "Redis Operator has been down for more than 5 minutes."

  - alert: RedisOperatorHighReconcileErrors
    expr: rate(redis_operator_reconcile_errors_total[5m]) > 0.1
    for: 2m
    labels:
      severity: warning
    annotations:
      summary: "High reconcile error rate"
      description: "Redis Operator reconcile error rate is {{ $value }} errors/sec."
```

#### Redis 实例告警
```yaml
  - alert: RedisInstanceDown
    expr: redis_instance_status == 0
    for: 1m
    labels:
      severity: critical
    annotations:
      summary: "Redis instance is down"
      description: "Redis instance {{ $labels.instance }} is down."

  - alert: RedisHighMemoryUsage
    expr: redis_instance_memory_usage_bytes / redis_instance_memory_limit_bytes > 0.9
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "Redis high memory usage"
      description: "Redis instance {{ $labels.instance }} memory usage is {{ $value | humanizePercentage }}."

  - alert: RedisHighConnections
    expr: redis_instance_connected_clients > 1000
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "Redis high connection count"
      description: "Redis instance {{ $labels.instance }} has {{ $value }} connected clients."
```

#### Redis Sentinel 告警
```yaml
  - alert: RedisSentinelQuorumLost
    expr: redis_sentinel_sentinels_total < redis_sentinel_quorum_required
    for: 1m
    labels:
      severity: critical
    annotations:
      summary: "Redis Sentinel quorum lost"
      description: "Redis Sentinel quorum lost for master {{ $labels.master_name }}."

  - alert: RedisMasterDown
    expr: redis_sentinel_master_status == 0
    for: 1m
    labels:
      severity: critical
    annotations:
      summary: "Redis master is down"
      description: "Redis master {{ $labels.master_name }} is down."
```

### 2.2 告警通知配置

支持多种通知方式：
- Slack
- 钉钉
- 企业微信
- 邮件
- PagerDuty

```yaml
route:
  group_by: ['alertname', 'cluster', 'service']
  group_wait: 10s
  group_interval: 10s
  repeat_interval: 1h
  receiver: 'default'
  routes:
  - match:
      severity: critical
    receiver: 'critical-alerts'
  - match:
      severity: warning
    receiver: 'warning-alerts'

receivers:
- name: 'default'
  slack_configs:
  - api_url: 'YOUR_SLACK_WEBHOOK_URL'
    channel: '#redis-alerts'
    title: 'Redis Operator Alert'
    text: '{{ range .Alerts }}{{ .Annotations.description }}{{ end }}'

- name: 'critical-alerts'
  slack_configs:
  - api_url: 'YOUR_SLACK_WEBHOOK_URL'
    channel: '#redis-critical'
    title: '🚨 Critical Redis Alert'
    text: '{{ range .Alerts }}{{ .Annotations.description }}{{ end }}'
```

## 3. 日志采集系统 (ELK/EFK)

### 3.1 日志结构化

#### 控制器日志格式
```json
{
  "timestamp": "2025-01-11T10:30:00Z",
  "level": "info",
  "logger": "redis-operator",
  "msg": "Reconciling Redis instance",
  "redis_instance": "my-redis",
  "namespace": "default",
  "operation": "create",
  "duration_ms": 150,
  "trace_id": "abc123",
  "span_id": "def456"
}
```

#### Redis 实例日志格式
```json
{
  "timestamp": "2025-01-11T10:30:00Z",
  "level": "info",
  "source": "redis-server",
  "instance": "my-redis-0",
  "namespace": "default",
  "role": "master",
  "msg": "Client connected",
  "client_addr": "10.244.0.5:45678",
  "connected_clients": 15
}
```

### 3.2 Fluent Bit 配置

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: fluent-bit-config
  namespace: logging
data:
  fluent-bit.conf: |
    [SERVICE]
        Flush         1
        Log_Level     info
        Daemon        off
        Parsers_File  parsers.conf

    [INPUT]
        Name              tail
        Path              /var/log/containers/*redis-operator*.log
        Parser            docker
        Tag               redis.operator
        Refresh_Interval  5

    [INPUT]
        Name              tail
        Path              /var/log/containers/*redis-instance*.log
        Parser            docker
        Tag               redis.instance
        Refresh_Interval  5

    [FILTER]
        Name                kubernetes
        Match               redis.*
        Kube_URL            https://kubernetes.default.svc:443
        Kube_CA_File        /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
        Kube_Token_File     /var/run/secrets/kubernetes.io/serviceaccount/token
        Merge_Log           On
        K8S-Logging.Parser  On
        K8S-Logging.Exclude Off

    [OUTPUT]
        Name  es
        Match redis.*
        Host  elasticsearch.logging.svc.cluster.local
        Port  9200
        Index redis-logs
        Type  _doc
```

### 3.3 Elasticsearch 索引模板

```json
{
  "index_patterns": ["redis-logs-*"],
  "template": {
    "settings": {
      "number_of_shards": 1,
      "number_of_replicas": 1,
      "index.lifecycle.name": "redis-logs-policy",
      "index.lifecycle.rollover_alias": "redis-logs"
    },
    "mappings": {
      "properties": {
        "timestamp": {
          "type": "date"
        },
        "level": {
          "type": "keyword"
        },
        "logger": {
          "type": "keyword"
        },
        "redis_instance": {
          "type": "keyword"
        },
        "namespace": {
          "type": "keyword"
        },
        "operation": {
          "type": "keyword"
        },
        "duration_ms": {
          "type": "long"
        },
        "trace_id": {
          "type": "keyword"
        }
      }
    }
  }
}
```

### 3.4 Kibana 仪表板

创建以下 Kibana 仪表板：
1. **Redis Operator 日志概览**
2. **错误日志分析**
3. **性能日志分析**
4. **操作审计日志**

## 4. 分布式链路追踪 (Jaeger)

### 4.1 OpenTelemetry 集成

在控制器中集成 OpenTelemetry：

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/trace"
    "go.opentelemetry.io/otel/exporters/jaeger"
)

func (r *RedisInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    tracer := otel.Tracer("redis-operator")
    ctx, span := tracer.Start(ctx, "reconcile-redis-instance")
    defer span.End()
    
    span.SetAttributes(
        attribute.String("redis.instance", req.Name),
        attribute.String("redis.namespace", req.Namespace),
    )
    
    // 协调逻辑...
    
    return ctrl.Result{}, nil
}
```

### 4.2 Jaeger 配置

```yaml
apiVersion: jaegertracing.io/v1
kind: Jaeger
metadata:
  name: redis-operator-jaeger
  namespace: observability
spec:
  strategy: production
  storage:
    type: elasticsearch
    elasticsearch:
      nodeCount: 3
      storage:
        size: 50Gi
      redundancyPolicy: SingleRedundancy
  collector:
    maxReplicas: 5
    resources:
      limits:
        memory: 128Mi
  query:
    replicas: 2
```

## 5. 部署和配置

### 5.1 Kustomize 配置

创建 `config/observability/kustomization.yaml`：

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
- prometheus/
- grafana/
- alertmanager/
- fluent-bit/
- jaeger/

namespace: redis-operator-system

commonLabels:
  app.kubernetes.io/part-of: redis-operator
  app.kubernetes.io/component: observability
```

### 5.2 Helm Chart

创建 Helm Chart 用于一键部署监控栈：

```yaml
# charts/redis-operator-monitoring/Chart.yaml
apiVersion: v2
name: redis-operator-monitoring
description: Complete monitoring stack for Redis Operator
type: application
version: 0.1.0
appVersion: "1.0.0"

dependencies:
- name: prometheus
  version: "15.x.x"
  repository: https://prometheus-community.github.io/helm-charts
- name: grafana
  version: "6.x.x"
  repository: https://grafana.github.io/helm-charts
- name: elasticsearch
  version: "7.x.x"
  repository: https://helm.elastic.co
```

### 5.3 部署脚本

```bash
#!/bin/bash
# deploy-monitoring.sh

set -e

echo "部署 Redis Operator 监控栈..."

# 创建命名空间
kubectl create namespace redis-operator-system --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace observability --dry-run=client -o yaml | kubectl apply -f -

# 部署 Prometheus Operator
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
helm upgrade --install prometheus-operator prometheus-community/kube-prometheus-stack \
  --namespace observability \
  --set prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues=false

# 部署 ELK 栈
helm repo add elastic https://helm.elastic.co
helm upgrade --install elasticsearch elastic/elasticsearch --namespace observability
helm upgrade --install kibana elastic/kibana --namespace observability

# 部署 Jaeger
kubectl apply -f https://github.com/jaegertracing/jaeger-operator/releases/download/v1.42.0/jaeger-operator.yaml -n observability

# 应用自定义配置
kubectl apply -k config/observability/

echo "监控栈部署完成！"
echo "访问地址："
echo "- Prometheus: http://prometheus.local"
echo "- Grafana: http://grafana.local (admin/admin)"
echo "- Kibana: http://kibana.local"
echo "- Jaeger: http://jaeger.local"
```

## 6. 运维和维护

### 6.1 监控数据保留策略

- Prometheus: 30天
- Elasticsearch: 90天
- Jaeger: 7天

### 6.2 备份策略

- 每日备份 Grafana 仪表板配置
- 每周备份 Prometheus 配置和规则
- 每日备份 Elasticsearch 索引

### 6.3 性能优化

- 合理设置采集间隔
- 使用标签过滤减少数据量
- 定期清理过期数据
- 监控系统资源使用情况

## 7. 故障排查

### 7.1 常见问题

1. **指标缺失**
   - 检查 ServiceMonitor 配置
   - 验证网络连接
   - 查看 Prometheus targets 状态

2. **告警不触发**
   - 检查告警规则语法
   - 验证 AlertManager 配置
   - 查看通知渠道设置

3. **日志丢失**
   - 检查 Fluent Bit 状态
   - 验证 Elasticsearch 连接
   - 查看索引模板配置

### 7.2 调试命令

```bash
# 检查 Prometheus targets
kubectl port-forward svc/prometheus-operated 9090:9090 -n observability

# 检查 Grafana 数据源
kubectl port-forward svc/grafana 3000:80 -n observability

# 检查 Elasticsearch 状态
kubectl exec -it elasticsearch-0 -n observability -- curl localhost:9200/_cluster/health

# 查看 Fluent Bit 日志
kubectl logs -f daemonset/fluent-bit -n logging
```

## 8. 总结

通过集成完整的监控、告警和日志采集系统，Redis Operator 将具备：

1. **全面的可观测性** - 指标、日志、链路追踪
2. **主动的故障发现** - 实时告警和通知
3. **深入的问题分析** - 结构化日志和链路追踪
4. **直观的数据展示** - Grafana 仪表板和 Kibana 分析
5. **高效的运维管理** - 自动化部署和维护

这套方案将大大提升 Redis Operator 的运维效率和系统可靠性。