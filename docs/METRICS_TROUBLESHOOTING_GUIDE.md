# Redis Operator Metrics 故障排除指南

本文档记录了 Redis Operator metrics 端点配置和故障排除的完整过程。

## 问题描述

Redis Operator 部署后，Prometheus 无法正常采集 controller-manager 的 metrics 数据，主要问题包括：

1. Metrics 服务默认配置为 HTTPS (8443 端口)
2. 需要认证和授权才能访问 metrics 端点
3. ServiceMonitor 配置不匹配

## 解决方案

### 1. 修改 Metrics 配置为 HTTP 模式

#### 1.1 修改 Deployment 参数

```bash
# 将 metrics-bind-address 从 :8443 改为 :8080
kubectl patch deployment redis-operator-controller-manager -n redis-operator-system -p '{
  "spec": {
    "template": {
      "spec": {
        "containers": [{
          "name": "manager",
          "args": [
            "--health-probe-bind-address=:8081",
            "--metrics-bind-address=:8080",
            "--leader-elect",
            "--enable-metrics-collection=true",
            "--metrics-secure=false"
          ]
        }]
      }
    }
  }
}'
```

#### 1.2 更新 Service 端口配置

```bash
# 将服务端口从 8443 改为 8080
kubectl patch svc redis-operator-controller-manager-metrics-service -n redis-operator-system -p '{
  "spec": {
    "ports": [{
      "name": "http",
      "port": 8080,
      "protocol": "TCP",
      "targetPort": 8080
    }]
  }
}'
```

### 2. 创建 ServiceMonitor

创建 `config/samples/servicemonitor_operator.yaml` 文件：

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: redis-operator-controller-manager-monitor
  namespace: redis-operator-system
  labels:
    app.kubernetes.io/name: redis-operator
    app.kubernetes.io/component: monitoring
    control-plane: controller-manager
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: redis-operator
      control-plane: controller-manager
  
  endpoints:
  - port: http  # 8080 端口
    path: /metrics
    interval: 30s
    scrapeTimeout: 10s
    scheme: http
    
    relabelings:
    - sourceLabels: [__meta_kubernetes_service_name]
      targetLabel: service_name
    - sourceLabels: [__meta_kubernetes_namespace]
      targetLabel: namespace
    - sourceLabels: [__meta_kubernetes_pod_name]
      targetLabel: pod
    - replacement: 'redis-operator-controller'
      targetLabel: component
    - replacement: 'controller-manager'
      targetLabel: role
    
    metricRelabelings:
    - sourceLabels: [__name__]
      regex: 'controller_runtime_(.*)'
      targetLabel: __name__
      replacement: 'redis_operator_controller_${1}'
    - sourceLabels: [__name__]
      regex: 'workqueue_(.*)'
      targetLabel: __name__
      replacement: 'redis_operator_workqueue_${1}'
```

### 3. 应用配置

```bash
# 应用 ServiceMonitor
kubectl apply -f config/samples/servicemonitor_operator.yaml

# 验证配置
kubectl get servicemonitor redis-operator-controller-manager-monitor -n redis-operator-system
```

## 验证步骤

### 1. 检查 Pod 状态和日志

```bash
# 检查 Pod 状态
kubectl get pods -n redis-operator-system

# 检查 metrics 相关日志
kubectl logs <pod-name> -n redis-operator-system | grep -i metrics
```

预期输出：
```
INFO    controller-runtime.metrics  Serving metrics server   {"bindAddress": ":8080", "secure": false}
```

### 2. 测试 Metrics 端点

```bash
# 端口转发
kubectl port-forward -n redis-operator-system svc/redis-operator-controller-manager-metrics-service 8080:8080

# 测试访问
curl http://localhost:8080/metrics | head -20
```

### 3. 通过 kubectl proxy 访问

```bash
# 启动 proxy
kubectl proxy --port=8001 &

# 访问 metrics
curl http://localhost:8001/api/v1/namespaces/redis-operator-system/services/redis-operator-controller-manager-metrics-service:8080/proxy/metrics
```

## 可用的 Metrics 指标

Redis Operator 提供以下类型的指标：

### 1. Controller Runtime 指标
- `controller_runtime_active_workers`: 当前活跃的 worker 数量
- `controller_runtime_max_concurrent_reconciles`: 最大并发调和数量
- `controller_runtime_reconcile_time_seconds`: 调和时间直方图
- `controller_runtime_reconcile_total`: 调和总次数
- `controller_runtime_reconcile_errors_total`: 调和错误总次数

### 2. 工作队列指标
- `workqueue_adds_total`: 队列添加总次数
- `workqueue_depth`: 队列深度
- `workqueue_queue_duration_seconds`: 队列等待时间
- `workqueue_work_duration_seconds`: 工作处理时间

### 3. 进程指标
- `process_cpu_seconds_total`: CPU 使用时间
- `process_resident_memory_bytes`: 常驻内存
- `process_virtual_memory_bytes`: 虚拟内存

### 4. Go Runtime 指标
- `go_goroutines`: Goroutine 数量
- `go_memstats_*`: 内存统计
- `go_gc_*`: 垃圾回收统计

## 故障排除

### 问题 1: 访问被拒绝 (Unauthorized)

**原因**: Metrics 端点配置为 HTTPS 且需要认证

**解决方案**: 按照上述步骤修改为 HTTP 模式

### 问题 2: 端口不匹配

**原因**: Service 端口配置与 Pod 实际监听端口不匹配

**解决方案**: 确保 Service 和 Deployment 的端口配置一致

### 问题 3: ServiceMonitor 不生效

**原因**: 
- Prometheus Operator 未安装
- 标签选择器不匹配
- 命名空间不正确

**解决方案**: 
- 检查 Prometheus Operator 状态
- 验证标签匹配
- 确认命名空间配置

## 最佳实践

1. **安全考虑**: 生产环境建议使用 HTTPS 并配置适当的认证
2. **监控频率**: 根据实际需求调整 scrape interval
3. **标签管理**: 使用一致的标签策略便于查询和告警
4. **指标过滤**: 根据需要配置 metricRelabelings 过滤不必要的指标

## 相关文件

- `config/samples/servicemonitor_operator.yaml`: Operator ServiceMonitor 配置
- `config/default/metrics_service.yaml`: Metrics Service 配置
- `config/manager/manager.yaml`: Controller Manager 配置
- `cmd/main.go`: Metrics 相关代码

## 参考资源

- [Prometheus Operator Documentation](https://prometheus-operator.dev/)
- [Controller Runtime Metrics](https://book.kubebuilder.io/reference/metrics.html)
- [Kubernetes ServiceMonitor](https://github.com/prometheus-operator/prometheus-operator/blob/main/Documentation/user-guides/getting-started.md)