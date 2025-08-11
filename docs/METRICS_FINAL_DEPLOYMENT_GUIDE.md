# Redis Operator Metrics 最终部署指南

## 概述

本指南记录了 Redis Operator metrics 端点的完整配置过程，实现了通过 `make deploy` 命令的持久化部署。

## 问题背景

原始配置存在以下问题：
1. Metrics 端点默认启用 HTTPS 和认证，导致访问困难
2. ServiceMonitor 配置复杂，需要处理 TLS 证书
3. 临时修改无法通过 `make deploy` 持久化

## 解决方案

### 1. 修改 Manager 配置

在 `config/manager/manager.yaml` 中添加 `--metrics-secure=false` 参数：

```yaml
args:
  - --leader-elect
  - --health-probe-bind-address=:8081
  - --metrics-bind-address=:8443
  - --metrics-secure=false  # 禁用 metrics 认证
  - --enable-metrics-collection=true
```

### 2. 更新 ServiceMonitor 配置

在 `config/prometheus/monitor.yaml` 中简化配置：

```yaml
spec:
  endpoints:
    - path: /metrics
      port: https  # 端口名称保持不变
      scheme: http # 协议改为 HTTP
  selector:
    matchLabels:
      control-plane: controller-manager
      app.kubernetes.io/name: redis-operator
```

### 3. RBAC 配置

确保以下 RBAC 资源正确配置：
- `metrics_reader_role.yaml` - 定义 metrics 读取权限
- `metrics_reader_role_binding.yaml` - 绑定权限到 ServiceAccount

## 部署步骤

1. **构建镜像**：
   ```bash
   make docker-build IMG=controller:latest
   ```

2. **部署到集群**：
   ```bash
   make deploy IMG=controller:latest
   ```

3. **验证部署**：
   ```bash
   kubectl get pods -n redis-operator-system
   kubectl logs <pod-name> -n redis-operator-system | grep metrics
   ```

## 验证 Metrics 端点

### 方法 1：通过 Port Forward

```bash
# 端口转发
kubectl port-forward -n redis-operator-system svc/redis-operator-controller-manager-metrics-service 8443:8443

# 访问 metrics
curl http://localhost:8443/metrics
```

### 方法 2：通过 kubectl proxy

```bash
# 启动代理
kubectl proxy --port=8001

# 访问 metrics
curl 'http://localhost:8001/api/v1/namespaces/redis-operator-system/services/http:redis-operator-controller-manager-metrics-service:8443/proxy/metrics'
```

## 可用的 Metrics 指标

### Controller Runtime 指标
- `controller_runtime_active_workers` - 活跃的 worker 数量
- `controller_runtime_max_concurrent_reconciles` - 最大并发调和数
- `controller_runtime_reconcile_total` - 调和总次数
- `controller_runtime_reconcile_time_seconds` - 调和耗时

### Redis 相关指标
- `controller_runtime_active_workers{controller="redis"}` - Redis 控制器活跃 worker
- `controller_runtime_active_workers{controller="rediscluster"}` - RedisCluster 控制器活跃 worker
- `controller_runtime_active_workers{controller="redisinstance"}` - RedisInstance 控制器活跃 worker
- `controller_runtime_active_workers{controller="redismasterreplica"}` - RedisMasterReplica 控制器活跃 worker
- `controller_runtime_active_workers{controller="redissentinel"}` - RedisSentinel 控制器活跃 worker

## ServiceMonitor 配置

Prometheus 会自动发现并抓取 metrics，配置如下：

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: redis-operator-controller-manager-metrics-monitor
  namespace: redis-operator-system
spec:
  endpoints:
  - path: /metrics
    port: https
    scheme: http
  selector:
    matchLabels:
      app.kubernetes.io/name: redis-operator
      control-plane: controller-manager
```

## 故障排除

### 1. Metrics 端点无法访问

检查 Pod 日志：
```bash
kubectl logs <pod-name> -n redis-operator-system | grep metrics
```

应该看到：
```
INFO controller-runtime.metrics Serving metrics server {"bindAddress": ":8443", "secure": false}
```

### 2. ServiceMonitor 无法发现

检查标签匹配：
```bash
kubectl get svc -n redis-operator-system --show-labels
kubectl get servicemonitor -n redis-operator-system -o yaml
```

### 3. RBAC 权限问题

检查 ClusterRoleBinding：
```bash
kubectl get clusterrolebinding | grep redis-operator-metrics
```

## 最佳实践

1. **安全性**：在生产环境中，建议启用 TLS 和认证
2. **监控**：配置 Grafana 仪表板监控 Redis Operator 指标
3. **告警**：基于关键指标设置 Prometheus 告警规则
4. **日志**：结合日志监控，全面了解 Operator 运行状态

## 相关文件

- `config/manager/manager.yaml` - Manager 部署配置
- `config/prometheus/monitor.yaml` - ServiceMonitor 配置
- `config/rbac/metrics_reader_role.yaml` - Metrics 读取权限
- `config/rbac/metrics_reader_role_binding.yaml` - 权限绑定
- `config/default/kustomization.yaml` - Kustomize 配置

## 总结

通过以上配置，Redis Operator 的 metrics 端点现在可以：
1. 通过 `make deploy` 持久化部署
2. 无需复杂的认证配置
3. 被 Prometheus 自动发现和抓取
4. 提供丰富的 Redis 相关监控指标

这为 Redis Operator 的生产环境监控奠定了坚实的基础。