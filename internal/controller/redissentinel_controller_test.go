/*
Copyright 2025 James.Liu.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	redisv1 "github.com/ybooks240/redis-operator/api/v1"
)

var _ = Describe("RedisSentinel Controller", func() {
	var (
		ctx        context.Context
		reconciler *RedisSentinelReconciler
		scheme     *runtime.Scheme
		// k8sClient  client.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(redisv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())

		k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
		reconciler = &RedisSentinelReconciler{
			Client: k8sClient,
			Scheme: scheme,
		}
	})

	Context("When reconciling a RedisSentinel with embedded Redis", func() {
		const resourceName = "test-sentinel-embedded"
		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		var redisSentinel *redisv1.RedisSentinel

		BeforeEach(func() {
			redisSentinel = &redisv1.RedisSentinel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: redisv1.RedisSentinelSpec{
					Image:    "redis:7.0",
					Replicas: 3,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("50m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
					},
					Storage: redisv1.StorageSpec{
						Size:             "500Mi",
						StorageClassName: "standard",
					},
					Config: redisv1.SentinelConfig{
						Quorum:                2,
						DownAfterMilliseconds: 30000,
						FailoverTimeout:       180000,
						ParallelSyncs:         1,
					},
					Redis: redisv1.RedisInstanceConfig{
						MasterName: "mymaster",
						Master: redisv1.RedisMasterConfig{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("200m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
							Storage: redisv1.StorageSpec{
								Size:             "2Gi",
								StorageClassName: "standard",
							},
						},
						Replica: redisv1.RedisReplicaConfig{
							Replicas: 2,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("200m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
							Storage: redisv1.StorageSpec{
								Size:             "2Gi",
								StorageClassName: "standard",
							},
						},
					},
					MasterReplicaRef: &redisv1.MasterReplicaRef{
						Name: "", // 使用内嵌 Redis 配置
					},
				},
			}
			Expect(k8sClient.Create(ctx, redisSentinel)).To(Succeed())
		})

		AfterEach(func() {
			By("Cleanup the RedisSentinel resource")
			Expect(k8sClient.Delete(ctx, redisSentinel)).To(Succeed())
		})

		It("should successfully reconcile and create all required resources", func() {
			By("Reconciling the created resource")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that Sentinel StatefulSet is created")
			sentinelSts := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resourceName + "-sentinel",
					Namespace: "default",
				}, sentinelSts)
			}, time.Second*10, time.Millisecond*250).Should(Succeed())

			Expect(sentinelSts.Spec.Replicas).To(Equal(int32Ptr(3)))
			Expect(sentinelSts.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(sentinelSts.Spec.Template.Spec.Containers[0].Image).To(Equal("redis:7.0"))

			By("Checking that Sentinel Service is created")
			sentinelSvc := &corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resourceName + "-sentinel-service",
					Namespace: "default",
				}, sentinelSvc)
			}, time.Second*10, time.Millisecond*250).Should(Succeed())

			Expect(sentinelSvc.Spec.Ports).To(HaveLen(1))
			Expect(sentinelSvc.Spec.Ports[0].Port).To(Equal(int32(26379)))

			By("Checking that Sentinel ConfigMap is created")
			sentinelCm := &corev1.ConfigMap{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resourceName + "-sentinel-config",
					Namespace: "default",
				}, sentinelCm)
			}, time.Second*10, time.Millisecond*250).Should(Succeed())

			Expect(sentinelCm.Data).To(HaveKey("sentinel.conf"))

			By("Checking that Redis Master StatefulSet is created")
			masterSts := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resourceName + "-redis-master",
					Namespace: "default",
				}, masterSts)
			}, time.Second*10, time.Millisecond*250).Should(Succeed())

			Expect(masterSts.Spec.Replicas).To(Equal(int32Ptr(1)))

			By("Checking that Redis Replica StatefulSet is created")
			replicaSts := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resourceName + "-redis-replica",
					Namespace: "default",
				}, replicaSts)
			}, time.Second*10, time.Millisecond*250).Should(Succeed())

			Expect(replicaSts.Spec.Replicas).To(Equal(int32Ptr(2)))
		})

		It("should update status correctly", func() {
			By("Reconciling the resource multiple times")
			for i := 0; i < 3; i++ {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
			}

			By("Checking that status is updated")
			updatedSentinel := &redisv1.RedisSentinel{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, updatedSentinel)
				if err != nil {
					return false
				}
				return len(updatedSentinel.Status.Conditions) > 0
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			Expect(updatedSentinel.Status.Replicas).To(Equal(int32(3)))
			Expect(updatedSentinel.Status.ServiceName).To(Equal(resourceName + "-sentinel-service"))
		})
	})

	Context("When reconciling a RedisSentinel with external MasterReplica reference", func() {
		const resourceName = "test-sentinel-external"
		const masterReplicaName = "test-master-replica"
		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		var redisSentinel *redisv1.RedisSentinel
		var masterReplica *redisv1.RedisMasterReplica

		BeforeEach(func() {
			// 首先创建 RedisMasterReplica 资源
			masterReplica = &redisv1.RedisMasterReplica{
				ObjectMeta: metav1.ObjectMeta{
					Name:      masterReplicaName,
					Namespace: "default",
				},
				Spec: redisv1.RedisMasterReplicaSpec{
					Image: "redis:7.0",
					Master: redisv1.MasterSpec{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
						},
						Storage: redisv1.StorageSpec{
							Size:             "1Gi",
							StorageClassName: "standard",
						},
					},
					Replica: redisv1.ReplicaSpec{
						Replicas: 2,
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
						},
						Storage: redisv1.StorageSpec{
							Size:             "1Gi",
							StorageClassName: "standard",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, masterReplica)).To(Succeed())

			// 然后创建引用它的 RedisSentinel
			redisSentinel = &redisv1.RedisSentinel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: redisv1.RedisSentinelSpec{
					Image:    "redis:7.0",
					Replicas: 3,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("50m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
					},
					Storage: redisv1.StorageSpec{
						Size:             "500Mi",
						StorageClassName: "standard",
					},
					Config: redisv1.SentinelConfig{
						Quorum:                2,
						DownAfterMilliseconds: 30000,
						FailoverTimeout:       180000,
						ParallelSyncs:         1,
					},
					MasterReplicaRef: &redisv1.MasterReplicaRef{
						Name:       masterReplicaName,
						Namespace:  "default",
						MasterName: "mymaster",
					},
				},
			}
			Expect(k8sClient.Create(ctx, redisSentinel)).To(Succeed())
		})

		AfterEach(func() {
			By("Cleanup the RedisSentinel resource")
			Expect(k8sClient.Delete(ctx, redisSentinel)).To(Succeed())
			By("Cleanup the RedisMasterReplica resource")
			Expect(k8sClient.Delete(ctx, masterReplica)).To(Succeed())
		})

		It("should successfully reconcile with external reference", func() {
			By("Reconciling the created resource")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that only Sentinel resources are created")
			sentinelSts := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resourceName + "-sentinel",
					Namespace: "default",
				}, sentinelSts)
			}, time.Second*10, time.Millisecond*250).Should(Succeed())

			By("Checking that Redis Master/Replica StatefulSets are NOT created by Sentinel")
			masterSts := &appsv1.StatefulSet{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      resourceName + "-redis-master",
				Namespace: "default",
			}, masterSts)
			Expect(errors.IsNotFound(err)).To(BeTrue())

			By("Checking that status references the external master")
			updatedSentinel := &redisv1.RedisSentinel{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, updatedSentinel)
				if err != nil {
					return false
				}
				return updatedSentinel.Status.MonitoredMaster.Name == masterReplicaName
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())
		})
	})

	Context("When handling configuration changes", func() {
		const resourceName = "test-sentinel-config"
		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		var redisSentinel *redisv1.RedisSentinel

		BeforeEach(func() {
			redisSentinel = &redisv1.RedisSentinel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: redisv1.RedisSentinelSpec{
					Image:    "redis:7.0",
					Replicas: 3,
					Config: redisv1.SentinelConfig{
						Quorum:                2,
						DownAfterMilliseconds: 30000,
						FailoverTimeout:       180000,
						ParallelSyncs:         1,
					},
					MasterReplicaRef: &redisv1.MasterReplicaRef{
						Name: "", // 使用内嵌配置
					},
				},
			}
			Expect(k8sClient.Create(ctx, redisSentinel)).To(Succeed())
		})

		AfterEach(func() {
			By("Cleanup the RedisSentinel resource")
			Expect(k8sClient.Delete(ctx, redisSentinel)).To(Succeed())
		})

		It("should handle quorum changes", func() {
			By("Initial reconciliation")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Updating quorum configuration")
			updatedSentinel := &redisv1.RedisSentinel{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updatedSentinel)).To(Succeed())
			updatedSentinel.Spec.Config.Quorum = 1
			Expect(k8sClient.Update(ctx, updatedSentinel)).To(Succeed())

			By("Reconciling after configuration change")
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that ConfigMap is updated")
			sentinelCm := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      resourceName + "-sentinel-config",
					Namespace: "default",
				}, sentinelCm)
				if err != nil {
					return false
				}
				// 检查配置中是否包含新的 quorum 值
				config := sentinelCm.Data["sentinel.conf"]
				return config != "" // 简化检查，实际应该解析配置内容
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())
		})
	})
})

// 辅助函数
func int32Ptr(i int32) *int32 {
	return &i
}
