# Redis Operator Grafana Dashboard 快速开始指南

## 🎯 概述

本指南提供了一套完整的 Redis Operator Grafana Dashboard 解决方案，包含：

- **综合监控面板**: 全功能 Dashboard，支持所有 Redis 部署模式
- **简化监控面板**: 轻量级 Dashboard，专注核心指标
- **自动化部署脚本**: 一键部署所有监控组件
- **详细文档**: 完整的配置和故障排除指南

## 📁 文件结构

```
config/monitoring/
├── redis-operator-comprehensive-dashboard.json  # 综合监控面板
├── redis-operator-simple-dashboard.json         # 简化监控面板
├── deploy-monitoring.sh                         # 部署脚本
├── GRAFANA_DASHBOARD_README.md                  # 详细使用指南
└── QUICK_START.md                               # 本文件
```

## 🚀 快速部署

### 方法一：一键自动部署（推荐）

```bash
# 进入监控配置目录
cd config/monitoring

# 完整安装（包含 Redis Exporter + Dashboard）
./deploy-monitoring.sh install

# 仅安装 Dashboard
./deploy-monitoring.sh dashboard

# 验证部署状态
./deploy-monitoring.sh verify
```

### 方法二：手动部署

#### 1. 部署 Redis Exporter

```bash
# 创建命名空间
kubectl create namespace monitoring

# 部署 Redis Exporter
kubectl apply -f - <<EOF
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
        image: oliver006/redis_exporter:v1.45.0
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
spec:
  ports:
  - port: 9121
    targetPort: 9121
    name: metrics
  selector:
    app: redis-exporter
EOF
```

#### 2. 配置 ServiceMonitor

```bash
kubectl apply -f - <<EOF
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
EOF
```

#### 3. 导入 Grafana Dashboard

**通过 ConfigMap（推荐）:**
```bash
# 综合版 Dashboard
kubectl create configmap redis-operator-comprehensive-dashboard \
  --from-file=redis-operator-comprehensive-dashboard.json \
  --namespace=monitoring

kubectl label configmap redis-operator-comprehensive-dashboard \
  grafana_dashboard="1" -n monitoring

# 简化版 Dashboard
kubectl create configmap redis-operator-simple-dashboard \
  --from-file=redis-operator-simple-dashboard.json \
  --namespace=monitoring

kubectl label configmap redis-operator-simple-dashboard \
  grafana_dashboard="1" -n monitoring
```

**通过 Grafana UI:**
1. 登录 Grafana
2. 导航到 Dashboards → Import
3. 上传 JSON 文件或复制内容
4. 选择 Prometheus 数据源
5. 点击 Import

## 📊 Dashboard 功能特性

### 综合监控面板 (Comprehensive)

**监控范围:**
- ✅ Redis Operator 调谐统计
- ✅ 单节点 Redis 监控
- ✅ Redis 集群状态
- ✅ Redis 哨兵监控
- ✅ 主从复制状态
- ✅ 系统资源监控

**关键指标:**
- Controller 调谐次数和速率
- Redis 连接数、内存使用、命令执行速率
- 集群节点数、槽位状态
- 哨兵监控的主节点数
- 主从复制偏移量和延迟
- CPU、内存、网络使用率

### 简化监控面板 (Simple)

**核心功能:**
- 🎯 Redis 实例状态概览
- 📈 关键性能指标
- 📋 实例列表和状态表格
- ⚡ 快速故障识别

**适用场景:**
- 快速部署和测试
- 资源受限环境
- 基础监控需求

## 🔧 配置要求

### 前置条件

1. **Kubernetes 集群** (v1.19+)
2. **Prometheus** 已部署并运行
3. **Grafana** 已部署并配置 Prometheus 数据源
4. **Prometheus Operator** (可选，用于 ServiceMonitor)
5. **Redis Operator** 已部署

### 必需的指标

确保以下指标可用：

```bash
# Redis Operator 指标
controller_runtime_reconcile_total
controller_runtime_active_workers

# Redis 实例指标
redis_connected_clients
redis_memory_used_bytes
redis_commands_processed_total
redis_up

# 集群指标
redis_cluster_nodes
redis_cluster_slots_assigned

# 哨兵指标
redis_sentinel_masters
redis_sentinel_known_sentinels

# 主从复制指标
redis_connected_slaves
redis_master_repl_offset
redis_slave_lag_in_seconds
```

## 🔍 验证部署

### 1. 检查组件状态

```bash
# 检查 Redis Exporter
kubectl get pods -l app=redis-exporter -n monitoring

# 检查 ServiceMonitor
kubectl get servicemonitor -n monitoring

# 检查 Dashboard ConfigMaps
kubectl get configmap -l grafana_dashboard=1 -n monitoring
```

### 2. 验证指标收集

```bash
# 端口转发到 Redis Exporter
kubectl port-forward svc/redis-exporter 9121:9121 -n monitoring

# 检查指标
curl http://localhost:9121/metrics | grep redis_
```

### 3. 验证 Prometheus 目标

```bash
# 端口转发到 Prometheus
kubectl port-forward svc/prometheus-server 9090:80 -n monitoring

# 访问 http://localhost:9090/targets
# 确认 redis-exporter 目标状态为 UP
```

## 📈 使用指南

### Dashboard 访问

1. **登录 Grafana**
2. **导航到 Dashboards**
3. **选择 Redis Operator 面板:**
   - `Redis Operator 综合监控面板` - 完整功能
   - `Redis Operator 简化监控面板` - 基础监控

### 变量配置

- **数据源**: 选择 Prometheus 实例
- **实例过滤**: 选择特定 Redis 实例
- **命名空间过滤**: 选择特定 Kubernetes 命名空间

### 告警配置

基于以下指标配置告警：

```yaml
# Redis 实例下线
- alert: RedisInstanceDown
  expr: redis_up == 0
  for: 1m

# 内存使用过高
- alert: RedisHighMemoryUsage
  expr: redis_memory_used_bytes / redis_memory_max_bytes > 0.9
  for: 5m

# 集群槽位异常
- alert: RedisClusterSlotsNotOK
  expr: redis_cluster_slots_ok < redis_cluster_slots_assigned
  for: 2m
```

## 🛠️ 故障排除

### 常见问题

#### 1. Dashboard 显示 "No data"

**检查步骤:**
```bash
# 1. 验证 Prometheus 数据源
kubectl get svc prometheus-server -n monitoring

# 2. 检查 Redis Exporter 状态
kubectl logs -l app=redis-exporter -n monitoring

# 3. 验证指标可用性
curl http://prometheus-server/api/v1/query?query=redis_up
```

#### 2. ServiceMonitor 不工作

**解决方案:**
```bash
# 检查 Prometheus Operator
kubectl get crd prometheuses.monitoring.coreos.com

# 检查标签选择器
kubectl describe servicemonitor redis-exporter -n monitoring
```

#### 3. 权限问题

**修复 RBAC:**
```bash
# 应用 RBAC 配置
./deploy-monitoring.sh install

# 或手动创建
kubectl apply -f rbac.yaml
```

### 日志检查

```bash
# Redis Exporter 日志
kubectl logs -l app=redis-exporter -n monitoring

# Prometheus 日志
kubectl logs -l app=prometheus -n monitoring

# Grafana 日志
kubectl logs -l app=grafana -n monitoring
```

## 🔄 维护和更新

### 定期检查

```bash
# 运行验证脚本
./deploy-monitoring.sh verify

# 检查资源使用
kubectl top pods -n monitoring

# 更新 Dashboard
./deploy-monitoring.sh dashboard
```

### 清理资源

```bash
# 完全清理
./deploy-monitoring.sh cleanup

# 手动清理
kubectl delete namespace monitoring
```

## 📚 扩展阅读

- [详细使用指南](GRAFANA_DASHBOARD_README.md)
- [Redis Operator 文档](../../README.md)
- [Prometheus 配置指南](https://prometheus.io/docs/)
- [Grafana Dashboard 开发](https://grafana.com/docs/grafana/latest/dashboards/)

## 🤝 支持和贡献

### 获取帮助

1. 查看 [故障排除指南](GRAFANA_DASHBOARD_README.md#故障排除)
2. 检查 GitHub Issues
3. 参与社区讨论

### 贡献改进

1. Fork 项目
2. 创建功能分支
3. 提交 Pull Request
4. 参与代码审查

---

**🎉 恭喜！** 您现在拥有了一套完整的 Redis Operator 监控解决方案。

通过这些 Dashboard，您可以：
- 📊 实时监控 Redis Operator 和所有 Redis 实例
- 🚨 及时发现和解决问题
- 📈 分析性能趋势和容量规划
- 🔧 优化 Redis 配置和资源分配

开始使用：`./deploy-monitoring.sh install` 🚀