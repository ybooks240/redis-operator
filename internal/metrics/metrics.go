package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// 控制器级别指标
	ReconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "redis_operator_reconcile_total",
			Help: "Total number of reconcile operations",
		},
		[]string{"controller", "namespace", "name", "result"},
	)

	ReconcileDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "redis_operator_reconcile_duration_seconds",
			Help:    "Duration of reconcile operations in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"controller", "namespace", "name"},
	)

	ReconcileErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "redis_operator_reconcile_errors_total",
			Help: "Total number of reconcile errors",
		},
		[]string{"controller", "namespace", "name", "error_type"},
	)

	ResourceStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "redis_operator_resource_status",
			Help: "Status of Redis resources (0=down, 1=up, 2=pending)",
		},
		[]string{"controller", "namespace", "name", "status"},
	)

	// Redis 实例级别指标
	RedisInstanceStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "redis_instance_status",
			Help: "Redis instance status (0=down, 1=up)",
		},
		[]string{"namespace", "name", "role"},
	)

	RedisInstanceMemoryUsage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "redis_instance_memory_usage_bytes",
			Help: "Redis instance memory usage in bytes",
		},
		[]string{"namespace", "name", "role"},
	)

	RedisInstanceConnectedClients = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "redis_instance_connected_clients",
			Help: "Number of connected clients",
		},
		[]string{"namespace", "name", "role"},
	)

	RedisInstanceCommandsProcessed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "redis_instance_commands_processed_total",
			Help: "Total number of commands processed",
		},
		[]string{"namespace", "name", "role"},
	)

	RedisInstanceKeyspaceHits = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "redis_instance_keyspace_hits_total",
			Help: "Total number of keyspace hits",
		},
		[]string{"namespace", "name", "role"},
	)

	RedisInstanceKeyspaceMisses = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "redis_instance_keyspace_misses_total",
			Help: "Total number of keyspace misses",
		},
		[]string{"namespace", "name", "role"},
	)

	// Redis Sentinel 级别指标
	RedisSentinelMasters = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "redis_sentinel_masters_total",
			Help: "Number of masters monitored by sentinel",
		},
		[]string{"namespace", "name"},
	)

	RedisSentinelSentinels = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "redis_sentinel_sentinels_total",
			Help: "Number of sentinel instances",
		},
		[]string{"namespace", "name", "master_name"},
	)

	RedisSentinelFailovers = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "redis_sentinel_failover_total",
			Help: "Total number of failovers",
		},
		[]string{"namespace", "name", "master_name"},
	)

	RedisSentinelMasterStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "redis_sentinel_master_status",
			Help: "Status of monitored master (0=down, 1=up)",
		},
		[]string{"namespace", "name", "master_name"},
	)

	// Redis Cluster 级别指标
	RedisClusterNodes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "redis_cluster_nodes_total",
			Help: "Total number of cluster nodes",
		},
		[]string{"namespace", "name"},
	)

	RedisClusterSlotsAssigned = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "redis_cluster_slots_assigned",
			Help: "Number of assigned slots",
		},
		[]string{"namespace", "name"},
	)

	RedisClusterState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "redis_cluster_state",
			Help: "Cluster state (0=fail, 1=ok)",
		},
		[]string{"namespace", "name"},
	)

	// 资源使用指标
	RedisResourceCPUUsage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "redis_resource_cpu_usage_cores",
			Help: "CPU usage in cores",
		},
		[]string{"namespace", "name", "pod", "container"},
	)

	RedisResourceMemoryUsage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "redis_resource_memory_usage_bytes",
			Help: "Memory usage in bytes",
		},
		[]string{"namespace", "name", "pod", "container"},
	)

	RedisResourceNetworkIO = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "redis_resource_network_io_bytes_total",
			Help: "Network I/O in bytes",
		},
		[]string{"namespace", "name", "pod", "direction"},
	)

	RedisPersistenceOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "redis_persistence_operations_total",
			Help: "Total number of persistence operations",
		},
		[]string{"namespace", "name", "operation", "result"},
	)
)

// init 函数注册所有指标
func init() {
	// 注册控制器级别指标
	metrics.Registry.MustRegister(
		ReconcileTotal,
		ReconcileDuration,
		ReconcileErrors,
		ResourceStatus,
	)

	// 注册 Redis 实例级别指标
	metrics.Registry.MustRegister(
		RedisInstanceStatus,
		RedisInstanceMemoryUsage,
		RedisInstanceConnectedClients,
		RedisInstanceCommandsProcessed,
		RedisInstanceKeyspaceHits,
		RedisInstanceKeyspaceMisses,
	)

	// 注册 Redis Sentinel 级别指标
	metrics.Registry.MustRegister(
		RedisSentinelMasters,
		RedisSentinelSentinels,
		RedisSentinelFailovers,
		RedisSentinelMasterStatus,
	)

	// 注册 Redis Cluster 级别指标
	metrics.Registry.MustRegister(
		RedisClusterNodes,
		RedisClusterSlotsAssigned,
		RedisClusterState,
	)

	// 注册资源使用指标
	metrics.Registry.MustRegister(
		RedisResourceCPUUsage,
		RedisResourceMemoryUsage,
		RedisResourceNetworkIO,
		RedisPersistenceOperations,
	)
}

// RecordReconcile 记录协调操作指标
func RecordReconcile(controller, namespace, name, result string, duration float64) {
	ReconcileTotal.WithLabelValues(controller, namespace, name, result).Inc()
	ReconcileDuration.WithLabelValues(controller, namespace, name).Observe(duration)
}

// RecordReconcileError 记录协调错误指标
func RecordReconcileError(controller, namespace, name, errorType string) {
	ReconcileErrors.WithLabelValues(controller, namespace, name, errorType).Inc()
}

// SetResourceStatus 设置资源状态指标
func SetResourceStatus(controller, namespace, name, status string, value float64) {
	ResourceStatus.WithLabelValues(controller, namespace, name, status).Set(value)
}

// SetRedisInstanceStatus 设置 Redis 实例状态指标
func SetRedisInstanceStatus(namespace, name, role string, status float64) {
	RedisInstanceStatus.WithLabelValues(namespace, name, role).Set(status)
}

// SetRedisInstanceMemoryUsage 设置 Redis 实例内存使用指标
func SetRedisInstanceMemoryUsage(namespace, name, role string, usage float64) {
	RedisInstanceMemoryUsage.WithLabelValues(namespace, name, role).Set(usage)
}

// SetRedisInstanceConnectedClients 设置 Redis 实例连接客户端数指标
func SetRedisInstanceConnectedClients(namespace, name, role string, clients float64) {
	RedisInstanceConnectedClients.WithLabelValues(namespace, name, role).Set(clients)
}

// IncRedisInstanceCommandsProcessed 增加 Redis 实例处理命令数指标
func IncRedisInstanceCommandsProcessed(namespace, name, role string) {
	RedisInstanceCommandsProcessed.WithLabelValues(namespace, name, role).Inc()
}

// IncRedisInstanceKeyspaceHits 增加 Redis 实例键空间命中数指标
func IncRedisInstanceKeyspaceHits(namespace, name, role string) {
	RedisInstanceKeyspaceHits.WithLabelValues(namespace, name, role).Inc()
}

// IncRedisInstanceKeyspaceMisses 增加 Redis 实例键空间未命中数指标
func IncRedisInstanceKeyspaceMisses(namespace, name, role string) {
	RedisInstanceKeyspaceMisses.WithLabelValues(namespace, name, role).Inc()
}

// SetRedisSentinelMasters 设置 Sentinel 监控的主节点数指标
func SetRedisSentinelMasters(namespace, name string, masters float64) {
	RedisSentinelMasters.WithLabelValues(namespace, name).Set(masters)
}

// SetRedisSentinelSentinels 设置 Sentinel 实例数指标
func SetRedisSentinelSentinels(namespace, name, masterName string, sentinels float64) {
	RedisSentinelSentinels.WithLabelValues(namespace, name, masterName).Set(sentinels)
}

// IncRedisSentinelFailovers 增加 Sentinel 故障转移次数指标
func IncRedisSentinelFailovers(namespace, name, masterName string) {
	RedisSentinelFailovers.WithLabelValues(namespace, name, masterName).Inc()
}

// SetRedisSentinelMasterStatus 设置 Sentinel 监控的主节点状态指标
func SetRedisSentinelMasterStatus(namespace, name, masterName string, status float64) {
	RedisSentinelMasterStatus.WithLabelValues(namespace, name, masterName).Set(status)
}

// SetRedisClusterNodes 设置集群节点数指标
func SetRedisClusterNodes(namespace, name string, nodes float64) {
	RedisClusterNodes.WithLabelValues(namespace, name).Set(nodes)
}

// SetRedisClusterSlotsAssigned 设置集群已分配槽位数指标
func SetRedisClusterSlotsAssigned(namespace, name string, slots float64) {
	RedisClusterSlotsAssigned.WithLabelValues(namespace, name).Set(slots)
}

// SetRedisClusterState 设置集群状态指标
func SetRedisClusterState(namespace, name string, state float64) {
	RedisClusterState.WithLabelValues(namespace, name).Set(state)
}

// SetRedisResourceCPUUsage 设置资源 CPU 使用指标
func SetRedisResourceCPUUsage(namespace, name, pod, container string, usage float64) {
	RedisResourceCPUUsage.WithLabelValues(namespace, name, pod, container).Set(usage)
}

// SetRedisResourceMemoryUsage 设置资源内存使用指标
func SetRedisResourceMemoryUsage(namespace, name, pod, container string, usage float64) {
	RedisResourceMemoryUsage.WithLabelValues(namespace, name, pod, container).Set(usage)
}

// IncRedisResourceNetworkIO 增加资源网络 I/O 指标
func IncRedisResourceNetworkIO(namespace, name, pod, direction string, bytes float64) {
	RedisResourceNetworkIO.WithLabelValues(namespace, name, pod, direction).Add(bytes)
}

// IncRedisPersistenceOperations 增加持久化操作指标
func IncRedisPersistenceOperations(namespace, name, operation, result string) {
	RedisPersistenceOperations.WithLabelValues(namespace, name, operation, result).Inc()
}
