# Grafana Redis 组件指标查看指南

本指南详细说明如何在 Grafana 中查看和监控 Redis Operator 管理的各种 Redis 组件指标。

## 1. 访问 Grafana

### 1.1 启动 Grafana 服务

首先确保监控系统已部署：

```bash
# 部署完整监控系统
./scripts/deploy-monitoring.sh

# 或仅部署监控组件
./scripts/deploy-monitoring.sh --monitoring-only
```

### 1.2 访问 Grafana Web 界面

```bash
# 端口转发到本地
kubectl port-forward svc/prometheus-grafana 3000:80 -n monitoring
```

然后在浏览器中访问：
- **URL**: http://localhost:3000
- **用户名**: admin
- **密码**: admin123

## 2. Redis Operator 仪表板

### 2.1 导入仪表板

Redis Operator 仪表板会在部署时自动导入。如果需要手动导入：

1. 在 Grafana 中点击 "+" → "Import"
2. 上传 `config/monitoring/grafana-dashboard.json` 文件
3. 或者直接复制 JSON 内容粘贴

### 2.2 仪表板概览

仪表板包含以下主要面板：

#### 控制器级别指标
- **Reconcile Rate**: 控制器协调操作频率
- **Reconcile Success Rate**: 协调操作成功率
- **Reconcile Duration**: 协调操作耗时分布

#### Redis 实例指标
- **Redis Instance Status**: 实例状态（Up/Down）
- **Redis Memory Usage**: 内存使用量
- **Connected Clients**: 连接客户端数量
- **Cache Hit Rate**: 缓存命中率
- **Commands Per Second**: 每秒处理命令数

#### 集群和哨兵指标
- **Redis Cluster State**: 集群状态
- **Sentinel Master Status**: 哨兵监控的主节点状态
- **Cluster Nodes**: 集群节点数量

## 3. 关键指标详解

### 3.1 控制器健康指标

#### Reconcile Rate (协调频率)
```promql
redis_operator:reconcile_rate
```
- **含义**: 每秒协调操作次数
- **正常值**: 通常在 0.1-2.0 之间
- **异常**: 突然增高可能表示资源状态不稳定

#### Reconcile Success Rate (成功率)
```promql
redis_operator:reconcile_success_rate * 100
```
- **含义**: 协调操作成功百分比
- **正常值**: 应该接近 100%
- **异常**: 低于 95% 需要关注

### 3.2 Redis 实例指标

#### Instance Status (实例状态)
```promql
redis_instance_status
```
- **值含义**: 0=Down, 1=Up
- **标签**: namespace, name, role
- **用途**: 快速识别故障实例

#### Memory Usage (内存使用)
```promql
redis_instance_memory_usage_bytes
```
- **单位**: 字节
- **监控**: 内存使用趋势和峰值
- **告警**: 超过 1GB 时触发警告

#### Connected Clients (连接数)
```promql
redis_instance_connected_clients
```
- **监控**: 客户端连接数变化
- **告警**: 超过 1000 时触发警告

#### Cache Hit Rate (缓存命中率)
```promql
redis_instance:cache_hit_rate * 100
```
- **计算**: hits / (hits + misses)
- **正常值**: 通常应该 > 80%
- **优化**: 低命中率可能需要调整缓存策略

### 3.3 集群指标

#### Cluster State (集群状态)
```promql
redis_cluster_state
```
- **值含义**: 0=Fail, 1=OK
- **监控**: 集群整体健康状态

#### Cluster Nodes (集群节点)
```promql
redis_cluster_nodes_total
```
- **监控**: 集群节点数量变化
- **最小值**: 建议至少 6 个节点（3主3从）

#### Slots Assignment (槽位分配)
```promql
redis_cluster_slots_assigned
```
- **总槽位**: 16384
- **监控**: 确保所有槽位都已分配

### 3.4 哨兵指标

#### Sentinel Master Status (主节点状态)
```promql
redis_sentinel_master_status
```
- **值含义**: 0=Down, 1=Up
- **监控**: 主节点可用性

#### Sentinel Count (哨兵数量)
```promql
redis_sentinel_sentinels_total
```
- **建议值**: 至少 3 个哨兵
- **监控**: 哨兵集群完整性

## 4. 自定义查询和面板

### 4.1 创建自定义面板

1. 在仪表板中点击 "Add panel"
2. 选择可视化类型（Time series, Stat, Gauge 等）
3. 在 Query 中输入 PromQL 查询
4. 配置图例、单位、阈值等

### 4.2 常用 PromQL 查询示例

#### 错误率查询
```promql
# 控制器错误率
rate(redis_operator_reconcile_errors_total[5m])

# 按控制器类型分组
sum(rate(redis_operator_reconcile_errors_total[5m])) by (controller)
```

#### 资源使用查询
```promql
# CPU 使用率
redis_resource_cpu_usage_cores

# 内存使用率（按命名空间）
sum(redis_resource_memory_usage_bytes) by (namespace)

# 网络 IO
rate(redis_resource_network_io_bytes_total[5m])
```

#### 性能指标查询
```promql
# 命令处理速率
rate(redis_instance_commands_processed_total[5m])

# 键空间操作
rate(redis_instance_keyspace_hits_total[5m])
rate(redis_instance_keyspace_misses_total[5m])
```

### 4.3 设置告警

1. 在面板编辑模式下，切换到 "Alert" 标签
2. 点击 "Create Alert"
3. 设置告警条件：
   ```
   WHEN last() OF query(A, 5m, now) IS BELOW 0.8
   ```
4. 配置通知渠道

## 5. 故障排查

### 5.1 常见问题

#### 指标数据缺失
```bash
# 检查 ServiceMonitor
kubectl get servicemonitor -n redis-operator-system

# 检查 Prometheus 目标
kubectl port-forward svc/prometheus-kube-prometheus-prometheus 9090:9090 -n monitoring
# 访问 http://localhost:9090/targets
```

#### 仪表板显示异常
```bash
# 检查 Grafana 数据源配置
# 确保 Prometheus 数据源 URL 正确
# 默认: http://prometheus-kube-prometheus-prometheus.monitoring.svc.cluster.local:9090
```

### 5.2 调试查询

在 Grafana 的 Explore 页面中测试 PromQL 查询：

1. 点击左侧菜单的 "Explore"
2. 选择 Prometheus 数据源
3. 输入查询语句进行测试
4. 查看返回的时间序列数据

### 5.3 性能优化

#### 查询优化
- 使用适当的时间范围
- 避免过于复杂的聚合查询
- 使用记录规则预计算复杂指标

#### 仪表板优化
- 设置合理的刷新间隔（建议 30s-1m）
- 限制面板数量（建议 < 20 个）
- 使用变量过滤减少数据量

## 6. 高级功能

### 6.1 使用变量

在仪表板设置中添加变量：

```promql
# 命名空间变量
label_values(redis_instance_status, namespace)

# 实例名称变量
label_values(redis_instance_status{namespace="$namespace"}, name)
```

### 6.2 注解和事件

配置注解显示重要事件：

```promql
# 故障转移事件
increase(redis_sentinel_failover_total[1m]) > 0

# 实例重启事件
changes(redis_instance_status[5m]) > 0
```

### 6.3 导出和分享

- **导出仪表板**: Settings → JSON Model
- **分享链接**: Share → Link
- **快照**: Share → Snapshot

## 7. 最佳实践

### 7.1 监控策略

1. **分层监控**: 控制器 → 资源 → 实例 → 业务
2. **关键指标**: 专注于影响业务的核心指标
3. **趋势分析**: 关注指标变化趋势而非绝对值
4. **容量规划**: 基于历史数据预测资源需求

### 7.2 告警配置

1. **分级告警**: Critical/Warning/Info
2. **避免噪音**: 设置合理的阈值和持续时间
3. **上下文信息**: 告警消息包含足够的调试信息
4. **自动恢复**: 配置告警自动解除条件

### 7.3 团队协作

1. **权限管理**: 为不同角色设置适当权限
2. **仪表板组织**: 按团队或服务分组
3. **文档维护**: 保持监控文档更新
4. **知识分享**: 定期分享监控最佳实践

通过以上指南，您可以充分利用 Grafana 来监控和管理 Redis Operator 部署的各种 Redis 组件，确保系统的稳定性和性能。