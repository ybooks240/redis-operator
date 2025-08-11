package e2e

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ybooks240/redis-operator/test/utils"
)

var _ = Describe("RedisSentinel E2E Tests", func() {
	var namespace string

	BeforeEach(func() {
		namespace = fmt.Sprintf("redis-sentinel-e2e-%d", time.Now().Unix())

		// 创建测试命名空间
		By("creating test namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create test namespace")
	})

	AfterEach(func() {
		// 清理测试命名空间
		By("cleaning up test namespace")
		cmd := exec.Command("kubectl", "delete", "ns", namespace, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	})

	Context("When deploying RedisSentinel with embedded Redis", func() {
		var sentinelName string

		BeforeEach(func() {
			sentinelName = "test-sentinel-embedded"

			// 创建 RedisSentinel 资源
			By("creating RedisSentinel with embedded Redis")
			manifest := fmt.Sprintf(`
apiVersion: redis.io/v1
kind: RedisSentinel
metadata:
  name: %s
  namespace: %s
spec:
  image: redis:7.0
  replicas: 3
  resources:
    requests:
      cpu: 50m
      memory: 128Mi
  storage:
    size: 500Mi
    storageClassName: standard
  config:
    quorum: 2
    downAfterMilliseconds: 30000
    failoverTimeout: 180000
    parallelSyncs: 1
  redis:
    master:
      resources:
        requests:
          cpu: 100m
          memory: 128Mi
      storage:
        size: 1Gi
        storageClassName: standard
    replica:
      replicas: 2
      resources:
        requests:
          cpu: 100m
          memory: 128Mi
      storage:
        size: 1Gi
        storageClassName: standard
`, sentinelName, namespace)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(manifest)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create RedisSentinel")
		})

		AfterEach(func() {
			// 清理 RedisSentinel 资源
			By("cleaning up RedisSentinel")
			cmd := exec.Command("kubectl", "delete", "redissentinel", sentinelName, "-n", namespace, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)
		})

		It("should create all required resources", func() {
			By("Waiting for RedisSentinel to be created")
			Eventually(func() bool {
				cmd := exec.Command("kubectl", "get", "redissentinel", sentinelName, "-n", namespace)
				_, err := utils.Run(cmd)
				return err == nil
			}, time.Minute*2, time.Second*5).Should(BeTrue())

			By("Checking that StatefulSets are created")
			Eventually(func() bool {
				cmd := exec.Command("kubectl", "get", "statefulset", "-n", namespace, "-l", fmt.Sprintf("redis-sentinel=%s", sentinelName))
				output, err := utils.Run(cmd)
				if err != nil {
					return false
				}
				// 检查是否有 StatefulSet 创建
				return strings.Contains(string(output), sentinelName)
			}, time.Minute*3, time.Second*10).Should(BeTrue())

			By("Checking that Services are created")
			Eventually(func() bool {
				cmd := exec.Command("kubectl", "get", "service", "-n", namespace, "-l", fmt.Sprintf("redis-sentinel=%s", sentinelName))
				output, err := utils.Run(cmd)
				if err != nil {
					return false
				}
				return strings.Contains(string(output), sentinelName)
			}, time.Minute*2, time.Second*5).Should(BeTrue())

			By("Checking that ConfigMaps are created")
			Eventually(func() bool {
				cmd := exec.Command("kubectl", "get", "configmap", "-n", namespace, "-l", fmt.Sprintf("redis-sentinel=%s", sentinelName))
				output, err := utils.Run(cmd)
				if err != nil {
					return false
				}
				return strings.Contains(string(output), sentinelName)
			}, time.Minute*2, time.Second*5).Should(BeTrue())
		})

		It("should have correct Sentinel configuration", func() {
			By("Waiting for Sentinel ConfigMap to be created")
			var configMapName string
			Eventually(func() bool {
				cmd := exec.Command("kubectl", "get", "configmap", "-n", namespace, "-o", "name", "-l", fmt.Sprintf("redis-sentinel=%s", sentinelName))
				output, err := utils.Run(cmd)
				if err != nil {
					return false
				}
				lines := strings.Split(strings.TrimSpace(string(output)), "\n")
				for _, line := range lines {
					if strings.Contains(line, "sentinel-config") {
						configMapName = strings.TrimPrefix(line, "configmap/")
						return true
					}
				}
				return false
			}, time.Minute*2, time.Second*5).Should(BeTrue())

			By("Checking Sentinel configuration content")
			cmd := exec.Command("kubectl", "get", "configmap", configMapName, "-n", namespace, "-o", "jsonpath={.data.sentinel\\.conf}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			config := string(output)
			Expect(config).To(ContainSubstring("sentinel monitor"))
			Expect(config).To(ContainSubstring("sentinel down-after-milliseconds"))
			Expect(config).To(ContainSubstring("sentinel failover-timeout"))
			Expect(config).To(ContainSubstring("sentinel parallel-syncs"))
		})

		It("should have pods running", func() {
			By("Waiting for Sentinel pods to be running")
			Eventually(func() bool {
				cmd := exec.Command("kubectl", "get", "pods", "-n", namespace, "-l", fmt.Sprintf("app=redis-sentinel,redis-sentinel=%s", sentinelName), "--field-selector=status.phase=Running")
				output, err := utils.Run(cmd)
				if err != nil {
					return false
				}
				// 检查是否有运行中的 pod
				lines := strings.Split(strings.TrimSpace(string(output)), "\n")
				return len(lines) > 1 // 第一行是 header
			}, time.Minute*5, time.Second*10).Should(BeTrue())
		})
	})

	Context("When deploying RedisSentinel with external MasterReplica reference", func() {
		var sentinelName, masterReplicaName string

		BeforeEach(func() {
			sentinelName = "test-sentinel-external"
			masterReplicaName = "test-master-replica"

			// 首先创建 RedisMasterReplica 资源
			By("creating RedisMasterReplica")
			masterReplicaManifest := fmt.Sprintf(`
apiVersion: redis.io/v1
kind: RedisMasterReplica
metadata:
  name: %s
  namespace: %s
spec:
  image: redis:7.0
  master:
    resources:
      requests:
        cpu: 100m
        memory: 128Mi
    storage:
      size: 1Gi
      storageClassName: standard
  replica:
    replicas: 2
    resources:
      requests:
        cpu: 100m
        memory: 128Mi
    storage:
      size: 1Gi
      storageClassName: standard
`, masterReplicaName, namespace)

			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(masterReplicaManifest)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create RedisMasterReplica")

			// 然后创建引用外部 MasterReplica 的 RedisSentinel
			By("creating RedisSentinel with external MasterReplica reference")
			sentinelManifest := fmt.Sprintf(`
apiVersion: redis.io/v1
kind: RedisSentinel
metadata:
  name: %s
  namespace: %s
spec:
  image: redis:7.0
  replicas: 3
  resources:
    requests:
      cpu: 50m
      memory: 128Mi
  storage:
    size: 500Mi
    storageClassName: standard
  config:
    quorum: 2
    downAfterMilliseconds: 30000
    failoverTimeout: 180000
    parallelSyncs: 1
  masterReplicaRef:
    name: %s
`, sentinelName, namespace, masterReplicaName)

			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(sentinelManifest)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create RedisSentinel")
		})

		AfterEach(func() {
			// 清理资源
			By("cleaning up RedisSentinel and RedisMasterReplica")
			cmd := exec.Command("kubectl", "delete", "redissentinel", sentinelName, "-n", namespace, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "redismasterreplica", masterReplicaName, "-n", namespace, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)
		})

		It("should reference external MasterReplica correctly", func() {
			By("Waiting for RedisMasterReplica to be created")
			Eventually(func() bool {
				cmd := exec.Command("kubectl", "get", "redismasterreplica", masterReplicaName, "-n", namespace)
				_, err := utils.Run(cmd)
				return err == nil
			}, time.Minute*2, time.Second*5).Should(BeTrue())

			By("Waiting for RedisSentinel to be created")
			Eventually(func() bool {
				cmd := exec.Command("kubectl", "get", "redissentinel", sentinelName, "-n", namespace)
				_, err := utils.Run(cmd)
				return err == nil
			}, time.Minute*2, time.Second*5).Should(BeTrue())

			By("Checking that Sentinel references the correct MasterReplica")
			cmd := exec.Command("kubectl", "get", "redissentinel", sentinelName, "-n", namespace, "-o", "jsonpath={.spec.masterReplicaRef.name}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.TrimSpace(string(output))).To(Equal(masterReplicaName))
		})
	})
})
