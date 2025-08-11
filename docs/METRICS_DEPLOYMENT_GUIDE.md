# Redis Operator 指标部署指南

本指南说明如何在 Kubernetes 集群中正确部署 Redis Operator 以启用指标收集功能，并确保 Grafana 能够显示自定义指标。

## 问题诊断

如果您在集群内部署 Redis Operator 后仍然无法在 Grafana 中看到自定义指标，可能的原因包括：

1. **指标收集功能未启用**：部署时没有启用 `--enable-metrics-collection` 参数
2. **ServiceMonitor 配置问题**：Prometheus 无法发现或抓取指标端点
3. **网络策略限制**：指标端点被网络策略阻止访问
4. **Prometheus 配置问题**：Prometheus 没有正确配置抓取 Redis Operator 指标

## 解决方案

### 1. 确保指标收集功能已启用

Redis Operator 的部署配置已经更新，在 `config/manager/manager.yaml` 中包含了 `--enable-metrics-collection=true` 参数：

```yaml
args:
  - --leader-elect
  - --health-probe-bind-address=:8081
  - --enable-metrics-collection=true
```

### 2. 部署 Redis Operator

使用以下命令部署 Redis Operator：

```bash
# 构建并推送镜像（如果需要）
make docker-build docker-push IMG=<your-registry>/redis-operator:latest

# 部署到集群
make deploy IMG=<your-registry>/redis-operator:latest
```

### 3. 部署监控组件

Redis Operator 包含完整的监控配置，包括：

- **ServiceMonitor**：`config/prometheus/monitor.yaml`
- **Metrics Service**：`config/default/metrics_service.yaml`
- **Grafana Dashboard**：`config/monitoring/grafana-dashboard.json`
- **Prometheus Rules**：`config/monitoring/prometheus-rules.yaml`

部署监控组件：

```bash
# 部署监控配置
make deploy-monitoring

# 或者只部署监控组件（不包括日志和追踪）
make deploy-monitoring-only
```

### 4. 验证部署

#### 检查 Redis Operator Pod

```bash
kubectl get pods -n redis-operator-system
kubectl logs -n redis-operator-system deployment/redis-operator-controller-manager
```

确保日志中显示：
- `Metrics collection enabled for production deployment`
- 没有连接错误或指标收集失败的消息

#### 检查 Metrics Service

```bash
kubectl get svc -n redis-operator-system redis-operator-controller-manager-metrics-service
```

#### 检查 ServiceMonitor

```bash
kubectl get servicemonitor -n redis-operator-system redis-operator-controller-manager-metrics-monitor
```

#### 测试指标端点

```bash
# 端口转发到指标服务
kubectl port-forward -n redis-operator-system svc/redis-operator-controller-manager-metrics-service 8443:8443

# 在另一个终端中测试（需要跳过 TLS 验证）
curl -k https://localhost:8443/metrics
```

### 5. Prometheus 配置

确保您的 Prometheus 实例配置了正确的服务发现：

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: prometheus-config
data:
  prometheus.yml: |
    global:
      scrape_interval: 15s
    scrape_configs:
    - job_name: 'kubernetes-service-endpoints'
      kubernetes_sd_configs:
      - role: endpoints
      relabel_configs:
      - source_labels: [__meta_kubernetes_service_annotation_prometheus_io_scrape]
        action: keep
        regex: true
      - source_labels: [__meta_kubernetes_service_annotation_prometheus_io_path]
        action: replace
        target_label: __metrics_path__
        regex: (.+)
```

或者如果使用 Prometheus Operator，确保 ServiceMonitor 的标签选择器匹配：

```yaml
apiVersion: monitoring.coreos.com/v1
kind: Prometheus
metadata:
  name: prometheus
spec:
  serviceMonitorSelector:
    matchLabels:
      app.kubernetes.io/name: redis-operator
```

### 6. Grafana 配置

#### 导入仪表板

1. 在 Grafana 中导入 `config/monitoring/grafana-dashboard.json`
2. 或者使用 ConfigMap 自动加载：

```bash
kubectl create configmap redis-operator-dashboard \
  --from-file=config/monitoring/grafana-dashboard.json \
  -n monitoring

kubectl label configmap redis-operator-dashboard \
  grafana_dashboard=1 -n monitoring
```

#### 验证数据源

确保 Grafana 中的 Prometheus 数据源配置正确，URL 指向您的 Prometheus 实例。

### 7. 可用的指标

Redis Operator 提供以下自定义指标：

- `redis_instance_status`：Redis 实例状态（0=Down, 1=Up）
- `redis_instance_memory_usage_bytes`：Redis 内存使用量
- `redis_instance_connected_clients`：连接的客户端数量
- `redis_instance_commands_processed_total`：处理的命令总数
- `redis_sentinel_masters`：Sentinel 监控的主节点数量
- `redis_cluster_nodes`：集群节点数量
- `redis_operator:reconcile_rate`：控制器协调速率
- `redis_operator:reconcile_success_rate`：协调成功率

### 8. 故障排除

#### 指标端点无法访问

```bash
# 检查网络策略
kubectl get networkpolicy -n redis-operator-system

# 检查服务端点
kubectl get endpoints -n redis-operator-system
```

#### Prometheus 无法抓取指标

```bash
# 检查 Prometheus targets
# 在 Prometheus UI 中访问 Status > Targets
# 查找 redis-operator 相关的 targets
```

#### Grafana 无数据

1. 检查 Prometheus 数据源连接
2. 在 Grafana 中执行查询测试：`redis_instance_status`
3. 检查时间范围设置

## 本地开发

对于本地开发，使用以下命令禁用指标收集以避免连接错误：

```bash
make run  # 默认禁用指标收集

# 或者显式启用（需要集群内 Redis 实例）
go run ./cmd/main.go --enable-metrics-collection=true
```

## 总结

通过以上步骤，您应该能够：

1. 在集群中正确部署启用指标收集的 Redis Operator
2. 配置 Prometheus 抓取 Redis Operator 指标
3. 在 Grafana 中查看 Redis 相关的监控仪表板
4. 监控 Redis 实例的状态、性能和运行指标

如果仍然遇到问题，请检查所有组件的日志，并确保网络连接和权限配置正确。