package metrics

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// RedisCollector 用于收集 Redis 实例的指标
type RedisCollector struct {
	client    *redis.Client
	namespace string
	name      string
	role      string
}

// NewRedisCollector 创建新的 Redis 指标收集器
func NewRedisCollector(addr, password, namespace, name, role string) *RedisCollector {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       0,
	})

	return &RedisCollector{
		client:    client,
		namespace: namespace,
		name:      name,
		role:      role,
	}
}

// CollectMetrics 收集 Redis 指标
func (rc *RedisCollector) CollectMetrics(ctx context.Context) error {
	logger := log.FromContext(ctx)

	// 检查连接状态
	pong, err := rc.client.Ping(ctx).Result()
	if err != nil {
		logger.Error(err, "Failed to ping Redis instance")
		SetRedisInstanceStatus(rc.namespace, rc.name, rc.role, 0) // down
		return err
	}

	if pong == "PONG" {
		SetRedisInstanceStatus(rc.namespace, rc.name, rc.role, 1) // up
	}

	// 获取 Redis INFO 信息
	info, err := rc.client.Info(ctx).Result()
	if err != nil {
		logger.Error(err, "Failed to get Redis info")
		return err
	}

	// 解析 INFO 信息并更新指标
	rc.parseAndUpdateMetrics(info)

	return nil
}

// parseAndUpdateMetrics 解析 Redis INFO 信息并更新指标
func (rc *RedisCollector) parseAndUpdateMetrics(info string) {
	lines := strings.Split(info, "\r\n")
	for _, line := range lines {
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				rc.updateMetricByKey(key, value)
			}
		}
	}
}

// updateMetricByKey 根据键值更新对应的指标
func (rc *RedisCollector) updateMetricByKey(key, value string) {
	switch key {
	case "used_memory":
		if val, err := strconv.ParseFloat(value, 64); err == nil {
			SetRedisInstanceMemoryUsage(rc.namespace, rc.name, rc.role, val)
		}
	case "connected_clients":
		if val, err := strconv.ParseFloat(value, 64); err == nil {
			SetRedisInstanceConnectedClients(rc.namespace, rc.name, rc.role, val)
		}
	case "total_commands_processed":
		if val, err := strconv.ParseFloat(value, 64); err == nil {
			// 这是累计值，需要计算增量
			// 这里简化处理，直接设置为当前值
			RedisInstanceCommandsProcessed.WithLabelValues(rc.namespace, rc.name, rc.role).Add(val)
		}
	case "keyspace_hits":
		if val, err := strconv.ParseFloat(value, 64); err == nil {
			RedisInstanceKeyspaceHits.WithLabelValues(rc.namespace, rc.name, rc.role).Add(val)
		}
	case "keyspace_misses":
		if val, err := strconv.ParseFloat(value, 64); err == nil {
			RedisInstanceKeyspaceMisses.WithLabelValues(rc.namespace, rc.name, rc.role).Add(val)
		}
	}
}

// Close 关闭 Redis 连接
func (rc *RedisCollector) Close() error {
	return rc.client.Close()
}

// SentinelCollector 用于收集 Redis Sentinel 的指标
type SentinelCollector struct {
	client    *redis.SentinelClient
	namespace string
	name      string
}

// NewSentinelCollector 创建新的 Sentinel 指标收集器
func NewSentinelCollector(addrs []string, namespace, name string) *SentinelCollector {
	client := redis.NewSentinelClient(&redis.Options{
		Addr: addrs[0], // 使用第一个地址
	})

	return &SentinelCollector{
		client:    client,
		namespace: namespace,
		name:      name,
	}
}

// CollectMetrics 收集 Sentinel 指标
func (sc *SentinelCollector) CollectMetrics(ctx context.Context) error {
	logger := log.FromContext(ctx)

	// 获取监控的主节点列表
	masters, err := sc.client.Masters(ctx).Result()
	if err != nil {
		logger.Error(err, "Failed to get sentinel masters")
		return err
	}

	// 设置主节点数量
	SetRedisSentinelMasters(sc.namespace, sc.name, float64(len(masters)))

	// 遍历每个主节点，收集详细信息
	for _, master := range masters {
		// 将 master 转换为 []interface{} 然后解析为 map[string]string
		masterSlice, ok := master.([]interface{})
		if !ok {
			continue
		}

		masterMap := make(map[string]string)
		for i := 0; i < len(masterSlice); i += 2 {
			if i+1 < len(masterSlice) {
				key, ok1 := masterSlice[i].(string)
				value, ok2 := masterSlice[i+1].(string)
				if ok1 && ok2 {
					masterMap[key] = value
				}
			}
		}

		masterName, exists := masterMap["name"]
		if !exists {
			continue
		}

		// 获取该主节点的 Sentinel 数量
		sentinels, err := sc.client.Sentinels(ctx, masterName).Result()
		if err != nil {
			logger.Error(err, "Failed to get sentinels for master", "master", masterName)
			continue
		}
		SetRedisSentinelSentinels(sc.namespace, sc.name, masterName, float64(len(sentinels)))

		// 检查主节点状态
		if flags, exists := masterMap["flags"]; exists {
			if strings.Contains(flags, "master") && !strings.Contains(flags, "down") {
				SetRedisSentinelMasterStatus(sc.namespace, sc.name, masterName, 1) // up
			} else {
				SetRedisSentinelMasterStatus(sc.namespace, sc.name, masterName, 0) // down
			}
		}

		// 获取故障转移次数（从 num-other-sentinels 字段推断）
		if numFailovers, exists := masterMap["num-other-sentinels"]; exists {
			if val, err := strconv.ParseFloat(numFailovers, 64); err == nil {
				// 这里简化处理，实际应该跟踪增量
				RedisSentinelFailovers.WithLabelValues(sc.namespace, sc.name, masterName).Add(val)
			}
		}
	}

	return nil
}

// Close 关闭 Sentinel 连接
func (sc *SentinelCollector) Close() error {
	return sc.client.Close()
}

// ClusterCollector 用于收集 Redis Cluster 的指标
type ClusterCollector struct {
	client    *redis.ClusterClient
	namespace string
	name      string
}

// NewClusterCollector 创建新的 Cluster 指标收集器
func NewClusterCollector(addrs []string, password, namespace, name string) *ClusterCollector {
	client := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:    addrs,
		Password: password,
	})

	return &ClusterCollector{
		client:    client,
		namespace: namespace,
		name:      name,
	}
}

// CollectMetrics 收集 Cluster 指标
func (cc *ClusterCollector) CollectMetrics(ctx context.Context) error {
	logger := log.FromContext(ctx)

	// 获取集群节点信息
	nodes, err := cc.client.ClusterNodes(ctx).Result()
	if err != nil {
		logger.Error(err, "Failed to get cluster nodes")
		return err
	}

	// 解析节点信息
	nodeLines := strings.Split(strings.TrimSpace(nodes), "\n")
	nodeCount := len(nodeLines)
	SetRedisClusterNodes(cc.namespace, cc.name, float64(nodeCount))

	// 获取集群槽位信息
	slots, err := cc.client.ClusterSlots(ctx).Result()
	if err != nil {
		logger.Error(err, "Failed to get cluster slots")
		return err
	}

	// 计算已分配的槽位数
	assignedSlots := 0
	for _, slot := range slots {
		assignedSlots += int(slot.End - slot.Start + 1)
	}
	SetRedisClusterSlotsAssigned(cc.namespace, cc.name, float64(assignedSlots))

	// 获取集群状态
	info, err := cc.client.ClusterInfo(ctx).Result()
	if err != nil {
		logger.Error(err, "Failed to get cluster info")
		return err
	}

	// 解析集群状态
	if strings.Contains(info, "cluster_state:ok") {
		SetRedisClusterState(cc.namespace, cc.name, 1) // ok
	} else {
		SetRedisClusterState(cc.namespace, cc.name, 0) // fail
	}

	return nil
}

// Close 关闭 Cluster 连接
func (cc *ClusterCollector) Close() error {
	return cc.client.Close()
}

// MetricsCollectionManager 管理所有指标收集器
type MetricsCollectionManager struct {
	redisCollectors    []*RedisCollector
	sentinelCollectors []*SentinelCollector
	clusterCollectors  []*ClusterCollector
	collectionInterval time.Duration
	stopCh             chan struct{}
}

// NewMetricsCollectionManager 创建新的指标收集管理器
func NewMetricsCollectionManager(interval time.Duration) *MetricsCollectionManager {
	return &MetricsCollectionManager{
		redisCollectors:    make([]*RedisCollector, 0),
		sentinelCollectors: make([]*SentinelCollector, 0),
		clusterCollectors:  make([]*ClusterCollector, 0),
		collectionInterval: interval,
		stopCh:             make(chan struct{}),
	}
}

// AddRedisCollector 添加 Redis 收集器
func (mcm *MetricsCollectionManager) AddRedisCollector(collector *RedisCollector) {
	mcm.redisCollectors = append(mcm.redisCollectors, collector)
}

// AddSentinelCollector 添加 Sentinel 收集器
func (mcm *MetricsCollectionManager) AddSentinelCollector(collector *SentinelCollector) {
	mcm.sentinelCollectors = append(mcm.sentinelCollectors, collector)
}

// AddClusterCollector 添加 Cluster 收集器
func (mcm *MetricsCollectionManager) AddClusterCollector(collector *ClusterCollector) {
	mcm.clusterCollectors = append(mcm.clusterCollectors, collector)
}

// Start 启动指标收集
func (mcm *MetricsCollectionManager) Start(ctx context.Context) {
	ticker := time.NewTicker(mcm.collectionInterval)
	defer ticker.Stop()

	logger := log.FromContext(ctx)
	logger.Info("Starting metrics collection", "interval", mcm.collectionInterval)

	for {
		select {
		case <-ticker.C:
			mcm.collectAllMetrics(ctx)
		case <-mcm.stopCh:
			logger.Info("Stopping metrics collection")
			return
		case <-ctx.Done():
			logger.Info("Context cancelled, stopping metrics collection")
			return
		}
	}
}

// collectAllMetrics 收集所有指标
func (mcm *MetricsCollectionManager) collectAllMetrics(ctx context.Context) {
	logger := log.FromContext(ctx)

	// 收集 Redis 实例指标
	for _, collector := range mcm.redisCollectors {
		if err := collector.CollectMetrics(ctx); err != nil {
			logger.Error(err, "Failed to collect Redis metrics",
				"namespace", collector.namespace,
				"name", collector.name,
				"role", collector.role)
		}
	}

	// 收集 Sentinel 指标
	for _, collector := range mcm.sentinelCollectors {
		if err := collector.CollectMetrics(ctx); err != nil {
			logger.Error(err, "Failed to collect Sentinel metrics",
				"namespace", collector.namespace,
				"name", collector.name)
		}
	}

	// 收集 Cluster 指标
	for _, collector := range mcm.clusterCollectors {
		if err := collector.CollectMetrics(ctx); err != nil {
			logger.Error(err, "Failed to collect Cluster metrics",
				"namespace", collector.namespace,
				"name", collector.name)
		}
	}
}

// Stop 停止指标收集
func (mcm *MetricsCollectionManager) Stop() {
	close(mcm.stopCh)

	// 关闭所有收集器
	for _, collector := range mcm.redisCollectors {
		collector.Close()
	}
	for _, collector := range mcm.sentinelCollectors {
		collector.Close()
	}
	for _, collector := range mcm.clusterCollectors {
		collector.Close()
	}
}
