# Redis 部署模式示例和使用指南

## 概述

本文档提供了三种 Redis 部署模式的完整示例配置和使用指南，帮助用户快速上手和部署 Redis 集群。

## 1. RedisMasterReplica 示例

### 1.1 基础配置示例

```yaml
apiVersion: redis.github.com/v1
kind: RedisMasterReplica
metadata:
  name: redis-master-replica-basic
  namespace: default
spec:
  image: redis:7.0
  master:
    resources:
      requests:
        memory: "1Gi"
        cpu: "500m"
      limits:
        memory: "2Gi"
        cpu: "1000m"
    storage:
      size: "10Gi"
      storageClassName: "fast-ssd"
    config:
      maxmemory: "1gb"
      maxmemory-policy: "allkeys-lru"
  replica:
    replicas: 2
    resources:
      requests:
        memory: "512Mi"
        cpu: "250m"
      limits:
        memory: "1Gi"
        cpu: "500m"
    storage:
      size: "10Gi"
      storageClassName: "standard"
    config:
      maxmemory: "512mb"
      maxmemory-policy: "allkeys-lru"
  config:
    save: "900 1 300 10 60 10000"
    rdbcompression: "yes"
    rdbchecksum: "yes"
```

### 1.2 高安全性配置示例

```yaml
apiVersion: redis.github.com/v1
kind: RedisMasterReplica
metadata:
  name: redis-master-replica-secure
  namespace: production
spec:
  image: redis:7.0
  master:
    resources:
      requests:
        memory: "2Gi"
        cpu: "1000m"
      limits:
        memory: "4Gi"
        cpu: "2000m"
    storage:
      size: "50Gi"
      storageClassName: "fast-ssd"
  replica:
    replicas: 3
    resources:
      requests:
        memory: "1Gi"
        cpu: "500m"
      limits:
        memory: "2Gi"
        cpu: "1000m"
    storage:
      size: "50Gi"
      storageClassName: "fast-ssd"
  security:
    authEnabled: true
    passwordSecret:
      name: redis-auth-secret
      key: password
    tls:
      enabled: true
      secretName: redis-tls-secret
  config:
    maxmemory: "3gb"
    maxmemory-policy: "volatile-lru"
    timeout: "300"
    tcp-keepalive: "60"
---
apiVersion: v1
kind: Secret
metadata:
  name: redis-auth-secret
  namespace: production
type: Opaque
data:
  password: UmVkaXNQYXNzd29yZDEyMyE=  # RedisPassword123!
```

### 1.3 部署和验证

```bash
# 1. 部署 RedisMasterReplica
kubectl apply -f redis-master-replica.yaml

# 2. 查看状态
kubectl get redismasterreplica
kubectl describe redismasterreplica redis-master-replica-basic

# 3. 查看 Pod 状态
kubectl get pods -l app=redis-master-replica-basic

# 4. 连接到主节点测试
kubectl exec -it redis-master-replica-basic-master-0 -- redis-cli
127.0.0.1:6379> set test-key "hello world"
OK
127.0.0.1:6379> get test-key
"hello world"

# 5. 连接到从节点验证复制
kubectl exec -it redis-master-replica-basic-replica-0 -- redis-cli
127.0.0.1:6379> get test-key
"hello world"
127.0.0.1:6379> info replication
# Replication
role:slave
master_host:redis-master-replica-basic-master-service
master_port:6379
master_link_status:up
```

## 2. RedisSentinel 示例

### 2.1 基础配置示例

```yaml
apiVersion: redis.github.com/v1
kind: RedisSentinel
metadata:
  name: redis-sentinel-basic
  namespace: default
spec:
  image: redis:7.0
  replicas: 3
  config:
    quorum: 2
    downAfterMilliseconds: 30000
    failoverTimeout: 180000
    parallelSyncs: 1
    additionalConfig:
      sentinel-deny-scripts-reconfig: "yes"
      sentinel-resolve-hostnames: "yes"
  masterReplicaRef:
    name: redis-master-replica-basic
    masterName: mymaster
  resources:
    requests:
      memory: "256Mi"
      cpu: "100m"
    limits:
      memory: "512Mi"
      cpu: "200m"
  storage:
    size: "1Gi"
    storageClassName: "standard"
```

### 2.2 生产环境配置示例

```yaml
apiVersion: redis.github.com/v1
kind: RedisSentinel
metadata:
  name: redis-sentinel-production
  namespace: production
spec:
  image: redis:7.0
  replicas: 5  # 奇数个哨兵，推荐 3 或 5
  config:
    quorum: 3  # 大于一半的哨兵数量
    downAfterMilliseconds: 10000  # 10秒检测主节点下线
    failoverTimeout: 60000        # 1分钟故障转移超时
    parallelSyncs: 2              # 并行同步数量
    additionalConfig:
      sentinel-deny-scripts-reconfig: "yes"
      sentinel-resolve-hostnames: "yes"
      sentinel-announce-hostnames: "yes"
  masterReplicaRef:
    name: redis-master-replica-secure
    namespace: production
    masterName: production-master
  resources:
    requests:
      memory: "512Mi"
      cpu: "200m"
    limits:
      memory: "1Gi"
      cpu: "500m"
  storage:
    size: "5Gi"
    storageClassName: "fast-ssd"
  security:
    authEnabled: true
    passwordSecret:
      name: redis-auth-secret
      key: password
```

### 2.3 部署和验证

```bash
# 1. 确保 RedisMasterReplica 已部署并运行
kubectl get redismasterreplica redis-master-replica-basic

# 2. 部署 RedisSentinel
kubectl apply -f redis-sentinel.yaml

# 3. 查看 Sentinel 状态
kubectl get redissentinel
kubectl describe redissentinel redis-sentinel-basic

# 4. 查看 Sentinel Pod
kubectl get pods -l app=redis-sentinel-basic

# 5. 连接到 Sentinel 查看监控状态
kubectl exec -it redis-sentinel-basic-0 -- redis-cli -p 26379
127.0.0.1:26379> sentinel masters
1)  1) "name"
    2) "mymaster"
    3) "ip"
    4) "10.244.0.10"
    5) "port"
    6) "6379"
    7) "runid"
    8) "abc123..."
    9) "flags"
   10) "master"

# 6. 查看从节点信息
127.0.0.1:26379> sentinel replicas mymaster

# 7. 查看其他哨兵
127.0.0.1:26379> sentinel sentinels mymaster

# 8. 测试故障转移（模拟主节点故障）
kubectl delete pod redis-master-replica-basic-master-0

# 9. 观察故障转移过程
kubectl logs -f redis-sentinel-basic-0
```

## 3. RedisCluster 示例

### 3.1 基础配置示例

```yaml
apiVersion: redis.github.com/v1
kind: RedisCluster
metadata:
  name: redis-cluster-basic
  namespace: default
spec:
  image: redis:7.0
  masters: 3
  replicasPerMaster: 1
  config:
    clusterNodeTimeout: 15000
    clusterRequireFullCoverage: yes
    clusterMigrationBarrier: 1
    additionalConfig:
      maxmemory: "1gb"
      maxmemory-policy: "allkeys-lru"
      save: "900 1 300 10 60 10000"
  resources:
    requests:
      memory: "1Gi"
      cpu: "500m"
    limits:
      memory: "2Gi"
      cpu: "1000m"
  storage:
    size: "20Gi"
    storageClassName: "fast-ssd"
```

### 3.2 大规模生产配置示例

```yaml
apiVersion: redis.github.com/v1
kind: RedisCluster
metadata:
  name: redis-cluster-production
  namespace: production
spec:
  image: redis:7.0
  masters: 6  # 6个主节点
  replicasPerMaster: 2  # 每个主节点2个从节点
  config:
    clusterNodeTimeout: 5000
    clusterRequireFullCoverage: yes  # 允许部分槽位不可用时继续服务
    clusterMigrationBarrier: 2
    additionalConfig:
      maxmemory: "4gb"
      maxmemory-policy: "volatile-lru"
      timeout: "300"
      tcp-keepalive: "60"
      save: "900 1 300 10 60 10000"
      rdbcompression: "yes"
      rdbchecksum: "yes"
  resources:
    requests:
      memory: "4Gi"
      cpu: "2000m"
    limits:
      memory: "8Gi"
      cpu: "4000m"
  storage:
    size: "100Gi"
    storageClassName: "fast-ssd"
  security:
    authEnabled: true
    passwordSecret:
      name: redis-cluster-auth
      key: password
  nodeSelector:
    node-type: "redis"
  tolerations:
    - key: "redis"
      operator: "Equal"
      value: "true"
      effect: "NoSchedule"
  affinity:
    podAntiAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
        - weight: 100
          podAffinityTerm:
            labelSelector:
              matchExpressions:
                - key: "app"
                  operator: In
                  values: ["redis-cluster-production"]
            topologyKey: "kubernetes.io/hostname"
---
apiVersion: v1
kind: Secret
metadata:
  name: redis-cluster-auth
  namespace: production
type: Opaque
data:
  password: Q2x1c3RlclBhc3N3b3JkMTIzIQ==  # ClusterPassword123!
```

### 3.3 部署和验证

```bash
# 1. 部署 RedisCluster
kubectl apply -f redis-cluster.yaml

# 2. 查看集群状态
kubectl get rediscluster
kubectl describe rediscluster redis-cluster-basic

# 3. 查看所有节点 Pod
kubectl get pods -l app=redis-cluster-basic

# 4. 等待集群初始化完成
kubectl logs -f redis-cluster-basic-0

# 5. 连接到集群节点查看状态
kubectl exec -it redis-cluster-basic-0 -- redis-cli
127.0.0.1:6379> cluster info
cluster_state:ok
cluster_slots_assigned:16384
cluster_slots_ok:16384
cluster_slots_pfail:0
cluster_slots_fail:0
cluster_known_nodes:6
cluster_size:3

# 6. 查看集群节点信息
127.0.0.1:6379> cluster nodes
abc123... 10.244.0.10:6379@16379 master - 0 1640000000000 1 connected 0-5460
def456... 10.244.0.11:6379@16379 master - 0 1640000000000 2 connected 5461-10922
ghi789... 10.244.0.12:6379@16379 master - 0 1640000000000 3 connected 10923-16383

# 7. 测试数据分片
127.0.0.1:6379> set key1 "value1"
-> Redirected to slot [9189] located at 10.244.0.11:6379
OK
10.244.0.11:6379> set key2 "value2"
-> Redirected to slot [4998] located at 10.244.0.10:6379
OK

# 8. 使用集群模式客户端
kubectl exec -it redis-cluster-basic-0 -- redis-cli -c
127.0.0.1:6379> set key3 "value3"
-> Redirected to slot [1581] located at 10.244.0.10:6379
OK
10.244.0.10:6379> get key1
-> Redirected to slot [9189] located at 10.244.0.11:6379
"value1"
```

## 4. 客户端连接示例

### 4.1 Python 客户端连接

#### 4.1.1 连接 MasterReplica

```python
import redis
from redis.sentinel import Sentinel

# 直接连接主节点（用于写操作）
master_client = redis.Redis(
    host='redis-master-replica-basic-master-service',
    port=6379,
    password='your-password',  # 如果启用了认证
    decode_responses=True
)

# 连接从节点（用于读操作）
replica_client = redis.Redis(
    host='redis-master-replica-basic-replica-service',
    port=6379,
    password='your-password',
    decode_responses=True
)

# 写操作
master_client.set('key', 'value')

# 读操作
value = replica_client.get('key')
print(f"Value: {value}")
```

#### 4.1.2 连接 Sentinel

```python
import redis
from redis.sentinel import Sentinel

# 配置 Sentinel 连接
sentinel = Sentinel([
    ('redis-sentinel-basic-0.redis-sentinel-basic-service', 26379),
    ('redis-sentinel-basic-1.redis-sentinel-basic-service', 26379),
    ('redis-sentinel-basic-2.redis-sentinel-basic-service', 26379),
], password='your-password')

# 获取主节点连接
master = sentinel.master_for('mymaster', socket_timeout=0.1, password='your-password')

# 获取从节点连接
replica = sentinel.slave_for('mymaster', socket_timeout=0.1, password='your-password')

# 写操作（自动连接到主节点）
master.set('key', 'value')

# 读操作（自动连接到从节点）
value = replica.get('key')
print(f"Value: {value}")
```

#### 4.1.3 连接 Cluster

```python
import redis
from rediscluster import RedisCluster

# 配置集群节点
startup_nodes = [
    {"host": "redis-cluster-basic-0.redis-cluster-basic-service", "port": "6379"},
    {"host": "redis-cluster-basic-1.redis-cluster-basic-service", "port": "6379"},
    {"host": "redis-cluster-basic-2.redis-cluster-basic-service", "port": "6379"},
]

# 创建集群客户端
cluster_client = RedisCluster(
    startup_nodes=startup_nodes,
    password='your-password',
    decode_responses=True,
    skip_full_coverage_check=True
)

# 操作数据（自动路由到正确的节点）
cluster_client.set('key1', 'value1')
cluster_client.set('key2', 'value2')
cluster_client.set('key3', 'value3')

# 读取数据
value1 = cluster_client.get('key1')
value2 = cluster_client.get('key2')
value3 = cluster_client.get('key3')

print(f"Values: {value1}, {value2}, {value3}")
```

### 4.2 Java 客户端连接

#### 4.2.1 连接 MasterReplica (Jedis)

```java
import redis.clients.jedis.Jedis;
import redis.clients.jedis.JedisPool;
import redis.clients.jedis.JedisPoolConfig;

public class RedisMasterReplicaClient {
    private JedisPool masterPool;
    private JedisPool replicaPool;
    
    public RedisMasterReplicaClient() {
        JedisPoolConfig config = new JedisPoolConfig();
        config.setMaxTotal(20);
        config.setMaxIdle(10);
        
        // 主节点连接池
        masterPool = new JedisPool(config, 
            "redis-master-replica-basic-master-service", 6379, 2000, "your-password");
        
        // 从节点连接池
        replicaPool = new JedisPool(config, 
            "redis-master-replica-basic-replica-service", 6379, 2000, "your-password");
    }
    
    public void set(String key, String value) {
        try (Jedis jedis = masterPool.getResource()) {
            jedis.set(key, value);
        }
    }
    
    public String get(String key) {
        try (Jedis jedis = replicaPool.getResource()) {
            return jedis.get(key);
        }
    }
}
```

#### 4.2.2 连接 Sentinel (Jedis)

```java
import redis.clients.jedis.Jedis;
import redis.clients.jedis.JedisSentinelPool;
import java.util.HashSet;
import java.util.Set;

public class RedisSentinelClient {
    private JedisSentinelPool sentinelPool;
    
    public RedisSentinelClient() {
        Set<String> sentinels = new HashSet<>();
        sentinels.add("redis-sentinel-basic-0.redis-sentinel-basic-service:26379");
        sentinels.add("redis-sentinel-basic-1.redis-sentinel-basic-service:26379");
        sentinels.add("redis-sentinel-basic-2.redis-sentinel-basic-service:26379");
        
        sentinelPool = new JedisSentinelPool("mymaster", sentinels, "your-password");
    }
    
    public void set(String key, String value) {
        try (Jedis jedis = sentinelPool.getResource()) {
            jedis.set(key, value);
        }
    }
    
    public String get(String key) {
        try (Jedis jedis = sentinelPool.getResource()) {
            return jedis.get(key);
        }
    }
}
```

#### 4.2.3 连接 Cluster (Jedis)

```java
import redis.clients.jedis.JedisCluster;
import redis.clients.jedis.HostAndPort;
import java.util.HashSet;
import java.util.Set;

public class RedisClusterClient {
    private JedisCluster jedisCluster;
    
    public RedisClusterClient() {
        Set<HostAndPort> nodes = new HashSet<>();
        nodes.add(new HostAndPort("redis-cluster-basic-0.redis-cluster-basic-service", 6379));
        nodes.add(new HostAndPort("redis-cluster-basic-1.redis-cluster-basic-service", 6379));
        nodes.add(new HostAndPort("redis-cluster-basic-2.redis-cluster-basic-service", 6379));
        
        jedisCluster = new JedisCluster(nodes, 2000, 2000, 5, "your-password", new GenericObjectPoolConfig());
    }
    
    public void set(String key, String value) {
        jedisCluster.set(key, value);
    }
    
    public String get(String key) {
        return jedisCluster.get(key);
    }
}
```

## 5. 运维操作指南

### 5.1 扩容操作

#### 5.1.1 MasterReplica 扩容

```bash
# 增加从节点数量
kubectl patch redismasterreplica redis-master-replica-basic --type='merge' -p='{
  "spec": {
    "replica": {
      "replicas": 3
    }
  }
}'

# 查看扩容状态
kubectl get redismasterreplica redis-master-replica-basic -w
```

#### 5.1.2 Cluster 扩容

```bash
# 增加主节点数量（需要重新分配槽位）
kubectl patch rediscluster redis-cluster-basic --type='merge' -p='{
  "spec": {
    "masters": 4
  }
}'

# 查看扩容状态
kubectl get rediscluster redis-cluster-basic -w

# 手动重新分配槽位（如果需要）
kubectl exec -it redis-cluster-basic-0 -- redis-cli --cluster rebalance redis-cluster-basic-0:6379
```

### 5.2 备份和恢复

#### 5.2.1 创建备份

```bash
# 创建 RDB 备份
kubectl exec redis-master-replica-basic-master-0 -- redis-cli BGSAVE

# 复制备份文件
kubectl cp redis-master-replica-basic-master-0:/data/dump.rdb ./backup-$(date +%Y%m%d-%H%M%S).rdb
```

#### 5.2.2 恢复备份

```bash
# 停止 Redis 实例
kubectl scale redismasterreplica redis-master-replica-basic --replicas=0

# 复制备份文件到 Pod
kubectl cp ./backup-20231201-120000.rdb redis-master-replica-basic-master-0:/data/dump.rdb

# 重启 Redis 实例
kubectl scale redismasterreplica redis-master-replica-basic --replicas=1
```

### 5.3 监控和告警

#### 5.3.1 查看资源状态

```bash
# 查看所有 Redis 资源
kubectl get redismasterreplica,redissentinel,rediscluster

# 查看详细状态
kubectl describe redismasterreplica redis-master-replica-basic

# 查看事件
kubectl get events --field-selector involvedObject.name=redis-master-replica-basic
```

#### 5.3.2 性能监控

```bash
# 查看 Redis 信息
kubectl exec redis-master-replica-basic-master-0 -- redis-cli info

# 查看内存使用
kubectl exec redis-master-replica-basic-master-0 -- redis-cli info memory

# 查看复制状态
kubectl exec redis-master-replica-basic-master-0 -- redis-cli info replication

# 实时监控命令
kubectl exec redis-master-replica-basic-master-0 -- redis-cli monitor
```

## 6. 故障排查

### 6.1 常见问题

#### 6.1.1 Pod 无法启动

```bash
# 查看 Pod 状态
kubectl get pods -l app=redis-master-replica-basic

# 查看 Pod 日志
kubectl logs redis-master-replica-basic-master-0

# 查看 Pod 事件
kubectl describe pod redis-master-replica-basic-master-0

# 检查存储
kubectl get pvc -l app=redis-master-replica-basic
```

#### 6.1.2 主从复制失败

```bash
# 检查主节点状态
kubectl exec redis-master-replica-basic-master-0 -- redis-cli info replication

# 检查从节点状态
kubectl exec redis-master-replica-basic-replica-0 -- redis-cli info replication

# 检查网络连接
kubectl exec redis-master-replica-basic-replica-0 -- redis-cli ping

# 检查配置
kubectl exec redis-master-replica-basic-replica-0 -- redis-cli config get replicaof
```

#### 6.1.3 集群节点无法加入

```bash
# 查看集群状态
kubectl exec redis-cluster-basic-0 -- redis-cli cluster info

# 查看集群节点
kubectl exec redis-cluster-basic-0 -- redis-cli cluster nodes

# 检查网络连接
kubectl exec redis-cluster-basic-0 -- redis-cli cluster meet redis-cluster-basic-1 6379

# 重置集群节点
kubectl exec redis-cluster-basic-1 -- redis-cli cluster reset
```

### 6.2 性能调优

#### 6.2.1 内存优化

```yaml
spec:
  config:
    # 内存策略
    maxmemory-policy: "allkeys-lru"
    # 内存采样
    maxmemory-samples: "10"
    # 哈希表优化
    hash-max-ziplist-entries: "512"
    hash-max-ziplist-value: "64"
    # 列表优化
    list-max-ziplist-size: "-2"
    list-compress-depth: "0"
```

#### 6.2.2 持久化优化

```yaml
spec:
  config:
    # RDB 优化
    save: "900 1 300 10 60 10000"
    rdbcompression: "yes"
    rdbchecksum: "yes"
    # AOF 优化
    appendonly: "yes"
    appendfsync: "everysec"
    no-appendfsync-on-rewrite: "no"
    auto-aof-rewrite-percentage: "100"
    auto-aof-rewrite-min-size: "64mb"
```

## 7. 总结

本文档提供了三种 Redis 部署模式的完整示例和使用指南：

1. **RedisMasterReplica**：适用于简单的读写分离场景
2. **RedisSentinel**：提供自动故障转移的高可用方案
3. **RedisCluster**：支持水平扩展的分布式解决方案

通过这些示例和指南，用户可以根据自己的需求选择合适的部署模式，并快速部署和管理 Redis 集群。