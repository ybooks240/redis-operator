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
	"fmt"
	"reflect"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	redisv1 "github.com/ybooks240/redis-operator/api/v1"
	"github.com/ybooks240/redis-operator/internal/utils"
)

// RedisSentinelReconciler reconciles a RedisSentinel object
type RedisSentinelReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	storageManager *utils.StorageManager
}

// +kubebuilder:rbac:groups=redis.github.com,resources=redissentinels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=redis.github.com,resources=redissentinels/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=redis.github.com,resources=redissentinels/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *RedisSentinelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logs := logf.FromContext(ctx)

	// 初始化存储管理器（如果尚未初始化）
	if r.storageManager == nil {
		r.storageManager = utils.NewStorageManager(r.Client, logs)
	}

	// 获取 RedisSentinel 实例
	redisSentinel := &redisv1.RedisSentinel{}
	err := r.Get(ctx, req.NamespacedName, redisSentinel)
	if err != nil {
		if errors.IsNotFound(err) {
			// 资源已被删除，无需处理
			logs.Info("RedisSentinel not found, skipping reconciliation", "name", req.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// 检查是否正在删除
	if redisSentinel.DeletionTimestamp != nil {
		logs.Info("RedisSentinel is being deleted, cleaning up resources", "name", redisSentinel.Name)
		if err = r.cleanupResources(ctx, req, redisSentinel, logs); err != nil {
			logs.Error(err, "Failed to cleanup resources")
			return ctrl.Result{}, err
		}
		// 移除 finalizer，使用重试机制避免资源版本冲突
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			// 重新获取最新的资源版本
			latestRedisSentinel := &redisv1.RedisSentinel{}
			if err = r.Get(ctx, types.NamespacedName{Name: redisSentinel.Name, Namespace: redisSentinel.Namespace}, latestRedisSentinel); err != nil {
				return err
			}
			controllerutil.RemoveFinalizer(latestRedisSentinel, redisv1.RedisSentinelFinalizer)
			return r.Update(ctx, latestRedisSentinel)
		})
		if err != nil {
			logs.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// 初始化状态和 finalizer
	if redisSentinel.Status.Conditions == nil {
		logs.Info("Initializing RedisSentinel", "name", redisSentinel.Name)
		controllerutil.AddFinalizer(redisSentinel, redisv1.RedisSentinelFinalizer)
		err = r.Update(ctx, redisSentinel)
		if err != nil {
			logs.Error(err, "Failed to update RedisSentinel finalizer")
			return ctrl.Result{}, err
		}
	}

	// 确保所有资源存在并正确配置
	err = r.ensureResources(ctx, req, redisSentinel, logs)
	if err != nil {
		logs.Error(err, "Failed to ensure resources")
		return ctrl.Result{}, err
	}

	// 更新状态
	err = r.updateRedisSentinelStatus(ctx, redisSentinel)
	if err != nil {
		logs.Error(err, "Failed to update RedisSentinel status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: time.Second * 30}, nil
}

// cleanupResources 清理相关资源
func (r *RedisSentinelReconciler) cleanupResources(ctx context.Context, req ctrl.Request, redisSentinel *redisv1.RedisSentinel, logs logr.Logger) error {
	// 如果配置了嵌入式 Redis，先删除 Redis 相关资源
	if r.hasEmbeddedRedis(redisSentinel) {
		// 删除 Redis Master StatefulSet
		redisMasterStatefulSet := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      redisSentinel.Name + "-redis-master",
				Namespace: redisSentinel.Namespace,
			},
		}
		if err := r.Get(ctx, types.NamespacedName{Name: redisMasterStatefulSet.Name, Namespace: redisMasterStatefulSet.Namespace}, redisMasterStatefulSet); err == nil {
			controllerutil.RemoveFinalizer(redisMasterStatefulSet, redisv1.RedisSentinelFinalizer)
			if err = r.Update(ctx, redisMasterStatefulSet); err != nil {
				logs.Error(err, "Failed to remove Redis Master StatefulSet finalizer")
				return err
			}
			if err = r.Delete(ctx, redisMasterStatefulSet); err != nil {
				logs.Error(err, "Failed to delete Redis Master StatefulSet")
				return err
			}
			logs.Info("Deleted Redis Master StatefulSet", "name", redisMasterStatefulSet.Name)
		}

		// 删除 Redis Replica StatefulSet
		redisReplicaStatefulSet := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      redisSentinel.Name + "-redis-replica",
				Namespace: redisSentinel.Namespace,
			},
		}
		if err := r.Get(ctx, types.NamespacedName{Name: redisReplicaStatefulSet.Name, Namespace: redisReplicaStatefulSet.Namespace}, redisReplicaStatefulSet); err == nil {
			controllerutil.RemoveFinalizer(redisReplicaStatefulSet, redisv1.RedisSentinelFinalizer)
			if err = r.Update(ctx, redisReplicaStatefulSet); err != nil {
				logs.Error(err, "Failed to remove Redis Replica StatefulSet finalizer")
				return err
			}
			if err = r.Delete(ctx, redisReplicaStatefulSet); err != nil {
				logs.Error(err, "Failed to delete Redis Replica StatefulSet")
				return err
			}
			logs.Info("Deleted Redis Replica StatefulSet", "name", redisReplicaStatefulSet.Name)
		}

		// 删除 Redis 统一 StatefulSet（如果存在）
		redisStatefulSet := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      redisSentinel.Name + "-redis",
				Namespace: redisSentinel.Namespace,
			},
		}
		if err := r.Get(ctx, types.NamespacedName{Name: redisStatefulSet.Name, Namespace: redisStatefulSet.Namespace}, redisStatefulSet); err == nil {
			controllerutil.RemoveFinalizer(redisStatefulSet, redisv1.RedisSentinelFinalizer)
			if err = r.Update(ctx, redisStatefulSet); err != nil {
				logs.Error(err, "Failed to remove Redis StatefulSet finalizer")
				return err
			}
			if err = r.Delete(ctx, redisStatefulSet); err != nil {
				logs.Error(err, "Failed to delete Redis StatefulSet")
				return err
			}
			logs.Info("Deleted Redis StatefulSet", "name", redisStatefulSet.Name)
		}

		// 删除 Redis Master Service
		redisMasterService := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      redisSentinel.Name + "-redis-master-service",
				Namespace: redisSentinel.Namespace,
			},
		}
		if err := r.Get(ctx, types.NamespacedName{Name: redisMasterService.Name, Namespace: redisMasterService.Namespace}, redisMasterService); err == nil {
			controllerutil.RemoveFinalizer(redisMasterService, redisv1.RedisSentinelFinalizer)
			if err = r.Update(ctx, redisMasterService); err != nil {
				logs.Error(err, "Failed to remove Redis Master Service finalizer")
				return err
			}
			if err = r.Delete(ctx, redisMasterService); err != nil {
				logs.Error(err, "Failed to delete Redis Master Service")
				return err
			}
			logs.Info("Deleted Redis Master Service", "name", redisMasterService.Name)
		}

		// 删除 Redis Replica Service
		redisReplicaService := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      redisSentinel.Name + "-redis-replica-service",
				Namespace: redisSentinel.Namespace,
			},
		}
		if err := r.Get(ctx, types.NamespacedName{Name: redisReplicaService.Name, Namespace: redisReplicaService.Namespace}, redisReplicaService); err == nil {
			controllerutil.RemoveFinalizer(redisReplicaService, redisv1.RedisSentinelFinalizer)
			if err = r.Update(ctx, redisReplicaService); err != nil {
				logs.Error(err, "Failed to remove Redis Replica Service finalizer")
				return err
			}
			if err = r.Delete(ctx, redisReplicaService); err != nil {
				logs.Error(err, "Failed to delete Redis Replica Service")
				return err
			}
			logs.Info("Deleted Redis Replica Service", "name", redisReplicaService.Name)
		}

		// 删除 Redis Headless Service
		redisHeadlessService := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      redisSentinel.Name + "-redis-headless",
				Namespace: redisSentinel.Namespace,
			},
		}
		if err := r.Get(ctx, types.NamespacedName{Name: redisHeadlessService.Name, Namespace: redisHeadlessService.Namespace}, redisHeadlessService); err == nil {
			controllerutil.RemoveFinalizer(redisHeadlessService, redisv1.RedisSentinelFinalizer)
			if err = r.Update(ctx, redisHeadlessService); err != nil {
				logs.Error(err, "Failed to remove Redis Headless Service finalizer")
				return err
			}
			if err = r.Delete(ctx, redisHeadlessService); err != nil {
				logs.Error(err, "Failed to delete Redis Headless Service")
				return err
			}
			logs.Info("Deleted Redis Headless Service", "name", redisHeadlessService.Name)
		}
	}

	// 删除 Sentinel StatefulSet
	sentinelSts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redisSentinel.Name + "-sentinel",
			Namespace: redisSentinel.Namespace,
		},
	}
	if err := r.Get(ctx, types.NamespacedName{Name: sentinelSts.Name, Namespace: sentinelSts.Namespace}, sentinelSts); err == nil {
		controllerutil.RemoveFinalizer(sentinelSts, redisv1.RedisSentinelFinalizer)
		if err = r.Update(ctx, sentinelSts); err != nil {
			logs.Error(err, "Failed to remove sentinel StatefulSet finalizer")
			return err
		}
		if err = r.Delete(ctx, sentinelSts); err != nil {
			logs.Error(err, "Failed to delete sentinel StatefulSet")
			return err
		}
		logs.Info("Deleted Sentinel StatefulSet", "name", sentinelSts.Name)
	}

	// 删除 Services 和 ConfigMaps
	resourcesToDelete := []client.Object{
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: redisSentinel.Name + "-sentinel-service", Namespace: redisSentinel.Namespace}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: redisSentinel.Name + "-sentinel-config", Namespace: redisSentinel.Namespace}},
	}

	for _, resource := range resourcesToDelete {
		if err := r.Get(ctx, types.NamespacedName{Name: resource.GetName(), Namespace: resource.GetNamespace()}, resource); err == nil {
			controllerutil.RemoveFinalizer(resource, redisv1.RedisSentinelFinalizer)
			if err = r.Update(ctx, resource); err != nil {
				logs.Error(err, "Failed to remove finalizer", "resource", resource.GetName())
				return err
			}
			if err = r.Delete(ctx, resource); err != nil {
				logs.Error(err, "Failed to delete resource", "resource", resource.GetName())
				return err
			}
			logs.Info("Deleted resource", "name", resource.GetName(), "type", fmt.Sprintf("%T", resource))
		}
	}

	return nil
}

// ensureResources 确保所有必要的资源存在
func (r *RedisSentinelReconciler) ensureResources(ctx context.Context, req ctrl.Request, redisSentinel *redisv1.RedisSentinel, logs logr.Logger) error {
	// 如果配置了嵌入式 Redis，则创建 Redis StatefulSet 和 Headless Service
	if r.hasEmbeddedRedis(redisSentinel) {
		// 确保 Redis Headless Service 存在
		if err := r.ensureRedisHeadlessService(ctx, redisSentinel, logs); err != nil {
			return err
		}

		// 确保 Redis StatefulSet 存在
		if err := r.ensureRedisStatefulSet(ctx, redisSentinel, logs); err != nil {
			return err
		}
	}

	// 确保 Sentinel ConfigMap
	if err := r.ensureSentinelConfigMap(ctx, redisSentinel, logs); err != nil {
		return err
	}

	// 确保 Sentinel StatefulSet
	if err := r.ensureSentinelStatefulSet(ctx, redisSentinel, logs); err != nil {
		return err
	}

	// 确保 Sentinel Service
	if err := r.ensureSentinelService(ctx, redisSentinel, logs); err != nil {
		return err
	}

	return nil
}

// ensureSentinelConfigMap 确保 Sentinel ConfigMap 存在
func (r *RedisSentinelReconciler) ensureSentinelConfigMap(ctx context.Context, redisSentinel *redisv1.RedisSentinel, logs logr.Logger) error {
	configMap := &corev1.ConfigMap{}
	configMapName := redisSentinel.Name + "-sentinel-config"
	err := r.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: redisSentinel.Namespace}, configMap)

	// 使用 DNS 名称直接指向 Redis master Pod
	masterDNS := fmt.Sprintf("%s-redis-0.%s-redis-headless.%s.svc.cluster.local",
		redisSentinel.Name, redisSentinel.Name, redisSentinel.Namespace)

	if errors.IsNotFound(err) {
		// 创建新的 ConfigMap
		configMap = r.configMapForSentinel(redisSentinel, masterDNS)
		if err := controllerutil.SetControllerReference(redisSentinel, configMap, r.Scheme); err != nil {
			return err
		}
		controllerutil.AddFinalizer(configMap, redisv1.RedisSentinelFinalizer)
		logs.Info("Creating sentinel ConfigMap", "name", configMap.Name)
		return r.Create(ctx, configMap)
	} else if err != nil {
		return err
	}

	// 检查是否需要更新ConfigMap
	newConfigMap := r.configMapForSentinel(redisSentinel, masterDNS)
	if configMap.Data["sentinel.conf"] != newConfigMap.Data["sentinel.conf"] {
		// 设置状态为 Updating
		if err := r.setUpdatingStatus(ctx, redisSentinel, "Updating sentinel ConfigMap"); err != nil {
			logs.Error(err, "Failed to set updating status")
		}

		configMap.Data = newConfigMap.Data
		logs.Info("Updating sentinel ConfigMap with new master DNS", "name", configMap.Name, "masterDNS", masterDNS)
		return r.Update(ctx, configMap)
	}

	return nil
}

// ensureSentinelStatefulSet 确保 Sentinel StatefulSet 存在
func (r *RedisSentinelReconciler) ensureSentinelStatefulSet(ctx context.Context, redisSentinel *redisv1.RedisSentinel, logs logr.Logger) error {
	statefulSet := &appsv1.StatefulSet{}
	statefulSetName := redisSentinel.Name + "-sentinel"
	err := r.Get(ctx, types.NamespacedName{Name: statefulSetName, Namespace: redisSentinel.Namespace}, statefulSet)

	if errors.IsNotFound(err) {
		// 创建新的 StatefulSet
		statefulSet = r.statefulSetForSentinelWithDynamicConfig(redisSentinel)
		if err := controllerutil.SetControllerReference(redisSentinel, statefulSet, r.Scheme); err != nil {
			return err
		}
		controllerutil.AddFinalizer(statefulSet, redisv1.RedisSentinelFinalizer)
		logs.Info("Creating sentinel StatefulSet with dynamic config", "name", statefulSet.Name)
		return r.Create(ctx, statefulSet)
	} else if err != nil {
		return err
	}

	// 检查 StatefulSet 是否需要更新
	desiredStatefulSet := r.statefulSetForSentinelWithDynamicConfig(redisSentinel)
	needsUpdate := false

	// 检查副本数
	if *statefulSet.Spec.Replicas != *desiredStatefulSet.Spec.Replicas {
		needsUpdate = true
	}

	// 检查镜像
	if len(statefulSet.Spec.Template.Spec.Containers) > 0 && len(desiredStatefulSet.Spec.Template.Spec.Containers) > 0 {
		if statefulSet.Spec.Template.Spec.Containers[0].Image != desiredStatefulSet.Spec.Template.Spec.Containers[0].Image {
			needsUpdate = true
		}
	}

	// 检查资源配置
	if len(statefulSet.Spec.Template.Spec.Containers) > 0 && len(desiredStatefulSet.Spec.Template.Spec.Containers) > 0 {
		if !reflect.DeepEqual(statefulSet.Spec.Template.Spec.Containers[0].Resources, desiredStatefulSet.Spec.Template.Spec.Containers[0].Resources) {
			needsUpdate = true
		}
	}

	if needsUpdate {
		// 设置状态为 Updating
		if err := r.setUpdatingStatus(ctx, redisSentinel, "Updating sentinel StatefulSet"); err != nil {
			logs.Error(err, "Failed to set updating status")
		}

		statefulSet.Spec.Replicas = desiredStatefulSet.Spec.Replicas
		statefulSet.Spec.Template = desiredStatefulSet.Spec.Template
		logs.Info("Updating sentinel StatefulSet", "name", statefulSet.Name)
		return r.Update(ctx, statefulSet)
	}

	return nil
}

// ensureRedisHeadlessService 确保 Redis Headless Service 存在
func (r *RedisSentinelReconciler) ensureRedisHeadlessService(ctx context.Context, redisSentinel *redisv1.RedisSentinel, logs logr.Logger) error {
	service := &corev1.Service{}
	serviceName := redisSentinel.Name + "-redis-headless"
	err := r.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: redisSentinel.Namespace}, service)

	if errors.IsNotFound(err) {
		// 创建新的 Headless Service
		service = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: redisSentinel.Namespace,
				Labels: map[string]string{
					"app":      "redis",
					"instance": redisSentinel.Name,
				},
			},
			Spec: corev1.ServiceSpec{
				ClusterIP: "None", // Headless Service
				Selector: map[string]string{
					"app":      "redis",
					"instance": redisSentinel.Name,
				},
				Ports: []corev1.ServicePort{
					{
						Name:       "redis",
						Port:       6379,
						TargetPort: intstr.FromInt(6379),
						Protocol:   corev1.ProtocolTCP,
					},
				},
			},
		}
		if err := controllerutil.SetControllerReference(redisSentinel, service, r.Scheme); err != nil {
			return err
		}
		controllerutil.AddFinalizer(service, redisv1.RedisSentinelFinalizer)
		logs.Info("Creating Redis Headless Service", "name", service.Name)
		return r.Create(ctx, service)
	} else if err != nil {
		return err
	}

	return nil
}

// ensureRedisStatefulSet 确保 Redis StatefulSet 存在
func (r *RedisSentinelReconciler) ensureRedisStatefulSet(ctx context.Context, redisSentinel *redisv1.RedisSentinel, logs logr.Logger) error {
	statefulSet := &appsv1.StatefulSet{}
	statefulSetName := redisSentinel.Name + "-redis"
	err := r.Get(ctx, types.NamespacedName{Name: statefulSetName, Namespace: redisSentinel.Namespace}, statefulSet)

	// 获取 Redis 配置
	redisConfig := redisSentinel.Spec.Redis
	// 计算总副本数：1个master + N个replica
	desiredReplicas := int32(1 + redisConfig.Replica.Replicas) // 1个master + replica数量

	if errors.IsNotFound(err) {
		// 创建新的 StatefulSet
		statefulSet = &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      statefulSetName,
				Namespace: redisSentinel.Namespace,
				Labels: map[string]string{
					"app":      "redis",
					"instance": redisSentinel.Name,
				},
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas:    &desiredReplicas,
				ServiceName: redisSentinel.Name + "-redis-headless",
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app":      "redis",
						"instance": redisSentinel.Name,
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app":      "redis",
							"instance": redisSentinel.Name,
						},
					},
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{
							{
								Name:  "redis-config",
								Image: "busybox:1.35",
								Command: []string{
									"/bin/sh",
									"-c",
									`
										# 获取当前 Pod 的序号
										ORDINAL=${HOSTNAME##*-}
										echo "Pod ordinal: $ORDINAL"
										
										# 创建 Redis 配置文件
										if [ "$ORDINAL" = "0" ]; then
											# 第一个 Pod 作为 Master
											echo "port 6379" > /etc/redis/redis.conf
											echo "bind 0.0.0.0" >> /etc/redis/redis.conf
											echo "protected-mode no" >> /etc/redis/redis.conf
											echo "save 900 1" >> /etc/redis/redis.conf
											echo "save 300 10" >> /etc/redis/redis.conf
											echo "save 60 10000" >> /etc/redis/redis.conf
										else
											# 其他 Pod 作为 Replica
											echo "port 6379" > /etc/redis/redis.conf
											echo "bind 0.0.0.0" >> /etc/redis/redis.conf
											echo "protected-mode no" >> /etc/redis/redis.conf
											echo "replicaof ` + redisSentinel.Name + `-redis-0.` + redisSentinel.Name + `-redis-headless.` + redisSentinel.Namespace + `.svc.cluster.local 6379" >> /etc/redis/redis.conf
										fi
										`,
								},
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "redis-config",
										MountPath: "/etc/redis",
									},
								},
							},
						},
						Containers: []corev1.Container{
							{
								Name:  "redis",
								Image: "redis:7.0",
								Command: []string{
									"redis-server",
									"/etc/redis/redis.conf",
								},
								Ports: []corev1.ContainerPort{
									{
										ContainerPort: 6379,
										Name:          "redis",
									},
								},
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "redis-config",
										MountPath: "/etc/redis",
									},
									{
										Name:      "redis-data",
										MountPath: "/data",
									},
								},
								ReadinessProbe: &corev1.Probe{
									ProbeHandler: corev1.ProbeHandler{
										Exec: &corev1.ExecAction{
											Command: []string{"redis-cli", "ping"},
										},
									},
									InitialDelaySeconds: 5,
									PeriodSeconds:       3,
								},
								LivenessProbe: &corev1.Probe{
									ProbeHandler: corev1.ProbeHandler{
										Exec: &corev1.ExecAction{
											Command: []string{"redis-cli", "ping"},
										},
									},
									InitialDelaySeconds: 30,
									PeriodSeconds:       3,
								},
							},
						},
						Volumes: []corev1.Volume{
							{
								Name: "redis-config",
								VolumeSource: corev1.VolumeSource{
									EmptyDir: &corev1.EmptyDirVolumeSource{},
								},
							},
						},
					},
				},
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "redis-data",
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{
								corev1.ReadWriteOnce,
							},
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse(func() string {
										if redisConfig.Master.Storage.Size != "" {
											return redisConfig.Master.Storage.Size
										}
										return "1Gi" // 默认存储大小
									}()),
								},
							},
							StorageClassName: func() *string {
								if redisConfig.Master.Storage.StorageClassName != "" {
									return &redisConfig.Master.Storage.StorageClassName
								}
								return nil
							}(),
						},
					},
				},
			},
		}

		if err := controllerutil.SetControllerReference(redisSentinel, statefulSet, r.Scheme); err != nil {
			return err
		}
		controllerutil.AddFinalizer(statefulSet, redisv1.RedisSentinelFinalizer)
		logs.Info("Creating Redis StatefulSet", "name", statefulSet.Name)
		return r.Create(ctx, statefulSet)
	} else if err != nil {
		return err
	}

	// StatefulSet 存在，检查是否需要更新
	currentReplicas := *statefulSet.Spec.Replicas
	needsUpdate := false
	updateType := ""

	// 检查副本数变更
	if currentReplicas != desiredReplicas {
		needsUpdate = true
		updateType = "replica scaling"
		logs.Info("Redis replica count change detected", "current", currentReplicas, "desired", desiredReplicas)
	}

	// 检查镜像变更
	currentImage := statefulSet.Spec.Template.Spec.Containers[0].Image
	desiredImage := redisSentinel.Spec.Image
	if desiredImage == "" {
		desiredImage = "redis:7.0" // 默认镜像
	}
	if currentImage != desiredImage {
		needsUpdate = true
		if updateType == "" {
			updateType = "rolling update"
		} else {
			updateType = "replica scaling and rolling update"
		}
		logs.Info("Redis image change detected", "current", currentImage, "desired", desiredImage)
	}

	// 检查资源配置变更
	if len(statefulSet.Spec.Template.Spec.Containers) > 0 {
		currentContainer := &statefulSet.Spec.Template.Spec.Containers[0]

		// 检查资源限制变更
		desiredResources := redisConfig.Master.Resources
		// 只有在明确指定了资源配置且与当前配置不同时才触发更新
		if !isEmptyResourceRequirements(desiredResources) && !reflect.DeepEqual(currentContainer.Resources, desiredResources) {
			needsUpdate = true
			if updateType == "" {
				updateType = "rolling update"
			} else if updateType == "replica scaling" {
				updateType = "replica scaling and rolling update"
			}
			logs.Info("Redis resource configuration change detected")
		}
	}

	// 检查存储配置变更
	storageNeedsExpansion := false
	if len(statefulSet.Spec.VolumeClaimTemplates) > 0 {
		currentStorage := statefulSet.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage]
		desiredStorageSize := "1Gi" // 默认存储大小
		if redisConfig.Master.Storage.Size != "" {
			desiredStorageSize = redisConfig.Master.Storage.Size
		}
		desiredStorage := resource.MustParse(desiredStorageSize)

		// 只有在存储大小真正不同时才触发更新
		if !currentStorage.Equal(desiredStorage) {
			// 使用通用存储管理器分析存储变更
			storageResult := r.storageManager.AnalyzeStorageChange(
				currentStorage.String(),
				desiredStorage.String(),
				"Redis",
			)

			// 处理存储变更结果
			if storageResult.ErrorMessage != "" {
				return fmt.Errorf("%s", storageResult.ErrorMessage)
			}

			if storageResult.ChangeType == utils.StorageExpansion {
				storageNeedsExpansion = true
			}
		}
	}

	// 注意：当前 Redis StatefulSet 使用 InitContainer 生成配置文件，不使用环境变量
	// 如果将来需要支持环境变量配置，可以在这里添加环境变量比较逻辑

	// 如果存储需要扩容，通过 PVC 动态扩展实现
	if storageNeedsExpansion {
		logs.Info("Expanding Redis storage via PVC expansion", "name", statefulSet.Name)

		// 设置状态为 Updating
		if err := r.setUpdatingStatus(ctx, redisSentinel, "Redis storage expansion detected"); err != nil {
			logs.Error(err, "Failed to set updating status")
		}

		// 扩展 PVCs
		desiredStorageSize := redisConfig.Master.Storage.Size
		if desiredStorageSize == "" {
			desiredStorageSize = "1Gi"
		}
		if err := r.storageManager.ExpandStatefulSetPVCs(ctx, statefulSet, desiredStorageSize, "Redis"); err != nil {
			return fmt.Errorf("failed to expand PVCs: %w", err)
		}

		logs.Info("PVC expansion completed", "name", statefulSet.Name)
		return nil
	}

	if needsUpdate {
		// 设置状态为 Updating
		if err := r.setUpdatingStatus(ctx, redisSentinel, fmt.Sprintf("Redis StatefulSet %s detected", updateType)); err != nil {
			logs.Error(err, "Failed to set updating status")
		}

		// 更新 StatefulSet
		statefulSet.Spec.Replicas = &desiredReplicas
		statefulSet.Spec.Template.Spec.Containers[0].Image = desiredImage

		// 更新资源配置（如果指定了的话）
		if !isEmptyResourceRequirements(redisConfig.Master.Resources) {
			statefulSet.Spec.Template.Spec.Containers[0].Resources = redisConfig.Master.Resources
		}

		logs.Info("Updating Redis StatefulSet", "name", statefulSet.Name, "type", updateType)
		return r.Update(ctx, statefulSet)
	}

	return nil
}

// ensureSentinelService 确保 Sentinel Service 存在
func (r *RedisSentinelReconciler) ensureSentinelService(ctx context.Context, redisSentinel *redisv1.RedisSentinel, logs logr.Logger) error {
	service := &corev1.Service{}
	serviceName := redisSentinel.Name + "-sentinel-service"
	err := r.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: redisSentinel.Namespace}, service)

	if errors.IsNotFound(err) {
		// 创建新的 Service
		service = r.serviceForSentinel(redisSentinel)
		if err := controllerutil.SetControllerReference(redisSentinel, service, r.Scheme); err != nil {
			return err
		}
		controllerutil.AddFinalizer(service, redisv1.RedisSentinelFinalizer)
		logs.Info("Creating sentinel Service", "name", service.Name)
		return r.Create(ctx, service)
	} else if err != nil {
		return err
	}

	return nil
}

// updateRedisSentinelStatus 更新 RedisSentinel 状态，带有重试机制避免冲突
func (r *RedisSentinelReconciler) updateRedisSentinelStatus(ctx context.Context, redisSentinel *redisv1.RedisSentinel) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return r.doUpdateRedisSentinelStatus(ctx, redisSentinel)
	})
}

// doUpdateRedisSentinelStatus 执行实际的状态更新逻辑
func (r *RedisSentinelReconciler) doUpdateRedisSentinelStatus(ctx context.Context, redisSentinel *redisv1.RedisSentinel) error {
	// 重新获取最新的资源版本以避免冲突
	latestSentinel := &redisv1.RedisSentinel{}
	if err := r.Get(ctx, types.NamespacedName{Name: redisSentinel.Name, Namespace: redisSentinel.Namespace}, latestSentinel); err != nil {
		return fmt.Errorf("failed to get latest RedisSentinel: %w", err)
	}

	// 获取 Sentinel StatefulSet 状态
	sentinelSts := &appsv1.StatefulSet{}
	sentinelStsName := redisSentinel.Name + "-sentinel"
	sentinelErr := r.Get(ctx, types.NamespacedName{Name: sentinelStsName, Namespace: redisSentinel.Namespace}, sentinelSts)

	// 更新状态
	if sentinelErr != nil {
		latestSentinel.Status.Status = string(redisv1.RedisSentinelPhaseFailed)
		latestSentinel.Status.Ready = "False"
		latestSentinel.Status.LastConditionMessage = fmt.Sprintf("Failed to get Sentinel StatefulSet: %v", sentinelErr)
	} else {
		sentinelReady := sentinelSts.Status.ReadyReplicas == *sentinelSts.Spec.Replicas

		if sentinelReady {
			latestSentinel.Status.Status = string(redisv1.RedisSentinelPhaseRunning)
			latestSentinel.Status.Ready = "True"
			latestSentinel.Status.LastConditionMessage = fmt.Sprintf("All %d sentinels are ready", sentinelSts.Status.ReadyReplicas)
		} else {
			latestSentinel.Status.Status = string(redisv1.RedisSentinelPhasePending)
			latestSentinel.Status.Ready = "False"
			latestSentinel.Status.LastConditionMessage = fmt.Sprintf("Waiting for sentinels to be ready: %d/%d", sentinelSts.Status.ReadyReplicas, *sentinelSts.Spec.Replicas)
		}

		// 更新 Sentinel 状态
		latestSentinel.Status.ReadyReplicas = sentinelSts.Status.ReadyReplicas
		latestSentinel.Status.Replicas = *sentinelSts.Spec.Replicas
		latestSentinel.Status.ServiceName = latestSentinel.Name + "-sentinel-service"
		// 初始化 MonitoredMaster 状态
		monitoredMaster := latestSentinel.Status.MonitoredMaster
		// 只有在 MasterReplicaRef 不为 nil 时才访问其字段
		if latestSentinel.Spec.MasterReplicaRef != nil && latestSentinel.Spec.MasterReplicaRef.Name != "" {
			monitoredMaster.Name = latestSentinel.Spec.MasterReplicaRef.Name
		} else {
			// 使用嵌入式 Redis 时的默认 master 名称
			monitoredMaster.Name = "mymaster"
		}
		latestSentinel.Status.MonitoredMaster = monitoredMaster
	}

	// 更新 Conditions
	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "NotReady",
		Message:            latestSentinel.Status.LastConditionMessage,
		LastTransitionTime: metav1.Now(),
	}

	if latestSentinel.Status.Ready == "true" {
		condition.Status = metav1.ConditionTrue
		condition.Reason = "Ready"
	}

	// 添加或更新条件
	if latestSentinel.Status.Conditions == nil {
		latestSentinel.Status.Conditions = []metav1.Condition{}
	}

	// 查找现有条件并更新，或添加新条件
	found := false
	for j, existingCondition := range latestSentinel.Status.Conditions {
		if existingCondition.Type == condition.Type {
			// 只有在状态真正改变时才更新
			if existingCondition.Status != condition.Status || existingCondition.Reason != condition.Reason {
				latestSentinel.Status.Conditions[j] = condition
			}
			found = true
			break
		}
	}
	if !found {
		latestSentinel.Status.Conditions = append(latestSentinel.Status.Conditions, condition)
	}

	// 限制条件数量
	if len(latestSentinel.Status.Conditions) > 10 {
		latestSentinel.Status.Conditions = latestSentinel.Status.Conditions[len(latestSentinel.Status.Conditions)-10:]
	}

	// 更新状态
	return r.Status().Update(ctx, latestSentinel)
}

// hasEmbeddedRedis 检查是否配置了嵌入式 Redis
func (r *RedisSentinelReconciler) hasEmbeddedRedis(redisSentinel *redisv1.RedisSentinel) bool {
	// 如果没有配置外部 MasterReplicaRef，则使用嵌入式 Redis
	return redisSentinel.Spec.MasterReplicaRef == nil || redisSentinel.Spec.MasterReplicaRef.Name == ""
}

// getMasterServiceIP 获取主节点Service的ClusterIP
func (r *RedisSentinelReconciler) getMasterServiceIP(ctx context.Context, redisSentinel *redisv1.RedisSentinel) (string, error) {
	var masterServiceName string

	if r.hasEmbeddedRedis(redisSentinel) {
		// 使用嵌入式 Redis 的 Service 名称
		masterServiceName = redisSentinel.Name + "-redis-master-service"
	} else {
		if redisSentinel.Spec.MasterReplicaRef.Name == "" {
			return "redis-master", nil // 默认主机名
		}
		// 使用外部 RedisMasterReplica 的 Service 名称
		masterServiceName = redisSentinel.Spec.MasterReplicaRef.Name + "-master-service"
	}

	// 获取主节点Service
	masterService := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: masterServiceName, Namespace: redisSentinel.Namespace}, masterService)
	if err != nil {
		if errors.IsNotFound(err) {
			// Service不存在，使用FQDN作为fallback
			return fmt.Sprintf("%s.%s.svc.cluster.local", masterServiceName, redisSentinel.Namespace), nil
		}
		return "", err
	}

	// 返回ClusterIP
	if masterService.Spec.ClusterIP != "" && masterService.Spec.ClusterIP != "None" {
		return masterService.Spec.ClusterIP, nil
	}

	// 如果没有ClusterIP，使用FQDN
	return fmt.Sprintf("%s.%s.svc.cluster.local", masterServiceName, redisSentinel.Namespace), nil
}

// configMapForSentinel 创建 Sentinel ConfigMap
func (r *RedisSentinelReconciler) configMapForSentinel(redisSentinel *redisv1.RedisSentinel, masterHost string) *corev1.ConfigMap {
	masterName := "mymaster"
	masterPort := "6379"
	quorum := "2"

	// 只有在 MasterReplicaRef 不为 nil 时才访问其字段
	if redisSentinel.Spec.MasterReplicaRef != nil && redisSentinel.Spec.MasterReplicaRef.Name != "" {
		if redisSentinel.Spec.MasterReplicaRef.MasterName != "" {
			masterName = redisSentinel.Spec.MasterReplicaRef.MasterName
		}
	}

	if redisSentinel.Spec.Config.Quorum > 0 {
		quorum = fmt.Sprintf("%d", redisSentinel.Spec.Config.Quorum)
	}

	sentinelConfig := map[string]string{
		"sentinel.conf": fmt.Sprintf(`# Redis Sentinel Configuration
port 26379
bind 0.0.0.0
sentinel resolve-hostnames yes
sentinel monitor %s %s %s %s
sentinel down-after-milliseconds %s 30000
sentinel parallel-syncs %s 1
sentinel failover-timeout %s 180000
sentinel deny-scripts-reconfig yes
`, masterName, masterHost, masterPort, quorum, masterName, masterName, masterName),
	}

	// 合并用户自定义配置
	if redisSentinel.Spec.Config.AdditionalConfig != nil {
		for key, value := range redisSentinel.Spec.Config.AdditionalConfig {
			sentinelConfig[key] = value
		}
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redisSentinel.Name + "-sentinel-config",
			Namespace: redisSentinel.Namespace,
			Labels: map[string]string{
				"app":       "redis-sentinel",
				"component": "sentinel",
				"instance":  redisSentinel.Name,
			},
		},
		Data: sentinelConfig,
	}
}

// statefulSetForSentinelWithDynamicConfig 创建带有动态配置的 Sentinel StatefulSet
func (r *RedisSentinelReconciler) statefulSetForSentinelWithDynamicConfig(redisSentinel *redisv1.RedisSentinel) *appsv1.StatefulSet {
	replicas := redisSentinel.Spec.Replicas
	if replicas == 0 {
		replicas = 3 // 默认值
	}

	resources := redisSentinel.Spec.Resources

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "config-writable",
			MountPath: "/usr/local/etc/redis",
		},
		{
			Name:      "data",
			MountPath: "/data",
		},
	}

	initContainerVolumeMounts := []corev1.VolumeMount{
		{
			Name:      "config",
			MountPath: "/config-source",
			ReadOnly:  true,
		},
		{
			Name:      "config-writable",
			MountPath: "/config-dest",
		},
	}

	volumes := []corev1.Volume{
		{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: redisSentinel.Name + "-sentinel-config",
					},
				},
			},
		},
		{
			Name: "config-writable",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	var volumeClaimTemplates []corev1.PersistentVolumeClaim
	if redisSentinel.Spec.Storage.Size != "" {
		volumeClaimTemplates = []corev1.PersistentVolumeClaim{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "data",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse(redisSentinel.Spec.Storage.Size),
						},
					},
					StorageClassName: &redisSentinel.Spec.Storage.StorageClassName,
				},
			},
		}
	} else {
		volumes = append(volumes, corev1.Volume{
			Name: "data",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redisSentinel.Name + "-sentinel",
			Namespace: redisSentinel.Namespace,
			Labels: map[string]string{
				"app":       "redis-sentinel",
				"component": "sentinel",
				"instance":  redisSentinel.Name,
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":       "redis-sentinel",
					"component": "sentinel",
					"instance":  redisSentinel.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":       "redis-sentinel",
						"component": "sentinel",
						"instance":  redisSentinel.Name,
					},
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name:         "config-init",
							Image:        "busybox:1.35",
							Command:      []string{"sh", "-c", "cp /config-source/* /config-dest/"},
							VolumeMounts: initContainerVolumeMounts,
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "sentinel",
							Image: redisSentinel.Spec.Image,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 26379,
									Name:          "sentinel",
								},
							},
							Command:      []string{"redis-sentinel", "/usr/local/etc/redis/sentinel.conf"},
							VolumeMounts: volumeMounts,
							Resources:    resources,
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.FromInt(26379),
									},
								},
								InitialDelaySeconds: 15,
								PeriodSeconds:       5,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.FromInt(26379),
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
							},
						},
					},
					Volumes: volumes,
				},
			},
			VolumeClaimTemplates: volumeClaimTemplates,
		},
	}
}

// serviceForSentinel 创建 Sentinel Service
func (r *RedisSentinelReconciler) serviceForSentinel(redisSentinel *redisv1.RedisSentinel) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redisSentinel.Name + "-sentinel-service",
			Namespace: redisSentinel.Namespace,
			Labels: map[string]string{
				"app":       "redis-sentinel",
				"component": "sentinel",
				"instance":  redisSentinel.Name,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app":       "redis-sentinel",
				"component": "sentinel",
				"instance":  redisSentinel.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "sentinel",
					Port:       26379,
					TargetPort: intstr.FromInt(26379),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}
}

// ensureRedisMasterStatefulSet 确保 Redis Master StatefulSet 存在
func (r *RedisSentinelReconciler) ensureRedisMasterStatefulSet(ctx context.Context, redisSentinel *redisv1.RedisSentinel, logs logr.Logger) error {
	statefulSet := &appsv1.StatefulSet{}
	statefulSetName := redisSentinel.Name + "-redis-master"
	err := r.Get(ctx, types.NamespacedName{Name: statefulSetName, Namespace: redisSentinel.Namespace}, statefulSet)

	if errors.IsNotFound(err) {
		// 创建新的 StatefulSet
		statefulSet = r.statefulSetForRedisMaster(redisSentinel)
		if err := controllerutil.SetControllerReference(redisSentinel, statefulSet, r.Scheme); err != nil {
			return err
		}
		controllerutil.AddFinalizer(statefulSet, redisv1.RedisSentinelFinalizer)
		logs.Info("Creating Redis master StatefulSet", "name", statefulSet.Name)
		return r.Create(ctx, statefulSet)
	} else if err != nil {
		return err
	}

	return nil
}

// ensureRedisMasterService 确保 Redis Master Service 存在
func (r *RedisSentinelReconciler) ensureRedisMasterService(ctx context.Context, redisSentinel *redisv1.RedisSentinel, logs logr.Logger) error {
	service := &corev1.Service{}
	serviceName := redisSentinel.Name + "-redis-master-service"
	err := r.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: redisSentinel.Namespace}, service)

	if errors.IsNotFound(err) {
		// 创建新的 Service
		service = r.serviceForRedisMaster(redisSentinel)
		if err := controllerutil.SetControllerReference(redisSentinel, service, r.Scheme); err != nil {
			return err
		}
		controllerutil.AddFinalizer(service, redisv1.RedisSentinelFinalizer)
		logs.Info("Creating Redis master Service", "name", service.Name)
		return r.Create(ctx, service)
	} else if err != nil {
		return err
	}

	return nil
}

// ensureRedisReplicaStatefulSet 确保 Redis Replica StatefulSet 存在
func (r *RedisSentinelReconciler) ensureRedisReplicaStatefulSet(ctx context.Context, redisSentinel *redisv1.RedisSentinel, logs logr.Logger) error {
	statefulSet := &appsv1.StatefulSet{}
	statefulSetName := redisSentinel.Name + "-redis-replica"
	err := r.Get(ctx, types.NamespacedName{Name: statefulSetName, Namespace: redisSentinel.Namespace}, statefulSet)

	if errors.IsNotFound(err) {
		// 创建新的 StatefulSet
		statefulSet = r.statefulSetForRedisReplica(redisSentinel)
		if err := controllerutil.SetControllerReference(redisSentinel, statefulSet, r.Scheme); err != nil {
			return err
		}
		controllerutil.AddFinalizer(statefulSet, redisv1.RedisSentinelFinalizer)
		logs.Info("Creating Redis replica StatefulSet", "name", statefulSet.Name)
		return r.Create(ctx, statefulSet)
	} else if err != nil {
		return err
	}

	return nil
}

// ensureRedisReplicaService 确保 Redis Replica Service 存在
func (r *RedisSentinelReconciler) ensureRedisReplicaService(ctx context.Context, redisSentinel *redisv1.RedisSentinel, logs logr.Logger) error {
	service := &corev1.Service{}
	serviceName := redisSentinel.Name + "-redis-replica-service"
	err := r.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: redisSentinel.Namespace}, service)

	if errors.IsNotFound(err) {
		// 创建新的 Service
		service = r.serviceForRedisReplica(redisSentinel)
		if err := controllerutil.SetControllerReference(redisSentinel, service, r.Scheme); err != nil {
			return err
		}
		controllerutil.AddFinalizer(service, redisv1.RedisSentinelFinalizer)
		logs.Info("Creating Redis replica Service", "name", service.Name)
		return r.Create(ctx, service)
	} else if err != nil {
		return err
	}

	return nil
}

// statefulSetForRedisMaster 创建 Redis Master StatefulSet
func (r *RedisSentinelReconciler) statefulSetForRedisMaster(redisSentinel *redisv1.RedisSentinel) *appsv1.StatefulSet {
	replicas := int32(1)
	resources := redisSentinel.Spec.Resources
	if len(redisSentinel.Spec.Redis.Master.Resources.Limits) > 0 || len(redisSentinel.Spec.Redis.Master.Resources.Requests) > 0 {
		resources = redisSentinel.Spec.Redis.Master.Resources
	}

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "data",
			MountPath: "/data",
		},
	}

	volumes := []corev1.Volume{}
	var volumeClaimTemplates []corev1.PersistentVolumeClaim

	storageSize := redisSentinel.Spec.Storage.Size
	storageClassName := redisSentinel.Spec.Storage.StorageClassName
	if redisSentinel.Spec.Redis.Master.Storage.Size != "" {
		storageSize = redisSentinel.Spec.Redis.Master.Storage.Size
		storageClassName = redisSentinel.Spec.Redis.Master.Storage.StorageClassName
	}

	if storageSize != "" {
		volumeClaimTemplates = []corev1.PersistentVolumeClaim{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "data",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse(storageSize),
						},
					},
					StorageClassName: &storageClassName,
				},
			},
		}
	} else {
		volumes = append(volumes, corev1.Volume{
			Name: "data",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redisSentinel.Name + "-redis-master",
			Namespace: redisSentinel.Namespace,
			Labels: map[string]string{
				"app":       "redis",
				"component": "master",
				"instance":  redisSentinel.Name,
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":       "redis",
					"component": "master",
					"instance":  redisSentinel.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":       "redis",
						"component": "master",
						"instance":  redisSentinel.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "redis",
							Image: redisSentinel.Spec.Image,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 6379,
									Name:          "redis",
								},
							},
							Command:      []string{"redis-server", "--port", "6379"},
							VolumeMounts: volumeMounts,
							Resources:    resources,
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.FromInt(6379),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       3,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.FromInt(6379),
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       3,
							},
						},
					},
					Volumes: volumes,
				},
			},
			VolumeClaimTemplates: volumeClaimTemplates,
		},
	}
}

// serviceForRedisMaster 创建 Redis Master Service
func (r *RedisSentinelReconciler) serviceForRedisMaster(redisSentinel *redisv1.RedisSentinel) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redisSentinel.Name + "-redis-master-service",
			Namespace: redisSentinel.Namespace,
			Labels: map[string]string{
				"app":       "redis",
				"component": "master",
				"instance":  redisSentinel.Name,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app":       "redis",
				"component": "master",
				"instance":  redisSentinel.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "redis",
					Port:       6379,
					TargetPort: intstr.FromInt(6379),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}
}

// statefulSetForRedisReplica 创建 Redis Replica StatefulSet
func (r *RedisSentinelReconciler) statefulSetForRedisReplica(redisSentinel *redisv1.RedisSentinel) *appsv1.StatefulSet {
	replicas := int32(2)
	if redisSentinel.Spec.Redis.Replica.Replicas > 0 {
		replicas = redisSentinel.Spec.Redis.Replica.Replicas
	}

	resources := redisSentinel.Spec.Resources
	if len(redisSentinel.Spec.Redis.Replica.Resources.Limits) > 0 || len(redisSentinel.Spec.Redis.Replica.Resources.Requests) > 0 {
		resources = redisSentinel.Spec.Redis.Replica.Resources
	}

	masterServiceName := redisSentinel.Name + "-redis-master-service"

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "data",
			MountPath: "/data",
		},
	}

	volumes := []corev1.Volume{}
	var volumeClaimTemplates []corev1.PersistentVolumeClaim

	storageSize := redisSentinel.Spec.Storage.Size
	storageClassName := redisSentinel.Spec.Storage.StorageClassName
	if redisSentinel.Spec.Redis.Replica.Storage.Size != "" {
		storageSize = redisSentinel.Spec.Redis.Replica.Storage.Size
		storageClassName = redisSentinel.Spec.Redis.Replica.Storage.StorageClassName
	}

	if storageSize != "" {
		volumeClaimTemplates = []corev1.PersistentVolumeClaim{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "data",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse(storageSize),
						},
					},
					StorageClassName: &storageClassName,
				},
			},
		}
	} else {
		volumes = append(volumes, corev1.Volume{
			Name: "data",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redisSentinel.Name + "-redis-replica",
			Namespace: redisSentinel.Namespace,
			Labels: map[string]string{
				"app":       "redis",
				"component": "replica",
				"instance":  redisSentinel.Name,
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":       "redis",
					"component": "replica",
					"instance":  redisSentinel.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":       "redis",
						"component": "replica",
						"instance":  redisSentinel.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "redis",
							Image: redisSentinel.Spec.Image,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 6379,
									Name:          "redis",
								},
							},
							Command:      []string{"redis-server", "--port", "6379", "--replicaof", masterServiceName, "6379"},
							VolumeMounts: volumeMounts,
							Resources:    resources,
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.FromInt(6379),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       3,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.FromInt(6379),
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       3,
							},
						},
					},
					Volumes: volumes,
				},
			},
			VolumeClaimTemplates: volumeClaimTemplates,
		},
	}
}

// serviceForRedisReplica 创建 Redis Replica Service
func (r *RedisSentinelReconciler) serviceForRedisReplica(redisSentinel *redisv1.RedisSentinel) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redisSentinel.Name + "-redis-replica-service",
			Namespace: redisSentinel.Namespace,
			Labels: map[string]string{
				"app":       "redis",
				"component": "replica",
				"instance":  redisSentinel.Name,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app":       "redis",
				"component": "replica",
				"instance":  redisSentinel.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "redis",
					Port:       6379,
					TargetPort: intstr.FromInt(6379),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}
}

// setUpdatingStatus 设置更新状态
func (r *RedisSentinelReconciler) setUpdatingStatus(ctx context.Context, redisSentinel *redisv1.RedisSentinel, message string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// 重新获取最新的资源版本以避免冲突
		latestSentinel := &redisv1.RedisSentinel{}
		if err := r.Get(ctx, types.NamespacedName{Name: redisSentinel.Name, Namespace: redisSentinel.Namespace}, latestSentinel); err != nil {
			return err
		}

		latestSentinel.Status.Status = string(redisv1.RedisSentinelPhaseUpdating)
		latestSentinel.Status.Ready = "False"
		latestSentinel.Status.LastConditionMessage = message

		// 更新 Conditions
		condition := metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "Updating",
			Message:            message,
			LastTransitionTime: metav1.Now(),
		}

		if latestSentinel.Status.Conditions == nil {
			latestSentinel.Status.Conditions = []metav1.Condition{}
		}

		// 更新或添加条件
		found := false
		for i, existingCondition := range latestSentinel.Status.Conditions {
			if existingCondition.Type == condition.Type {
				latestSentinel.Status.Conditions[i] = condition
				found = true
				break
			}
		}
		if !found {
			latestSentinel.Status.Conditions = append(latestSentinel.Status.Conditions, condition)
		}

		return r.Status().Update(ctx, latestSentinel)
	})
}

// buildRedisEnvVars 构建 Redis 容器的环境变量
func (r *RedisSentinelReconciler) buildRedisEnvVars(redisConfig redisv1.RedisInstanceConfig) []corev1.EnvVar {
	envVars := []corev1.EnvVar{}

	// 添加基础配置
	if redisConfig.MasterName != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "REDIS_MASTER_NAME",
			Value: redisConfig.MasterName,
		})
	}

	// 添加全局配置
	for key, value := range redisConfig.Config {
		envVars = append(envVars, corev1.EnvVar{
			Name:  fmt.Sprintf("REDIS_CONFIG_%s", key),
			Value: value,
		})
	}

	// 添加主节点配置
	for key, value := range redisConfig.Master.Config {
		envVars = append(envVars, corev1.EnvVar{
			Name:  fmt.Sprintf("REDIS_MASTER_CONFIG_%s", key),
			Value: value,
		})
	}

	// 添加从节点配置
	for key, value := range redisConfig.Replica.Config {
		envVars = append(envVars, corev1.EnvVar{
			Name:  fmt.Sprintf("REDIS_REPLICA_CONFIG_%s", key),
			Value: value,
		})
	}

	return envVars
}

// compareEnvVars 比较两个环境变量列表是否相等
func (r *RedisSentinelReconciler) compareEnvVars(current, desired []corev1.EnvVar) bool {
	if len(current) != len(desired) {
		return false
	}

	// 创建映射以便比较
	currentMap := make(map[string]string)
	for _, env := range current {
		currentMap[env.Name] = env.Value
	}

	desiredMap := make(map[string]string)
	for _, env := range desired {
		desiredMap[env.Name] = env.Value
	}

	return reflect.DeepEqual(currentMap, desiredMap)
}

// isEmptyResourceRequirements 检查资源配置是否为空
func isEmptyResourceRequirements(resources corev1.ResourceRequirements) bool {
	return len(resources.Limits) == 0 && len(resources.Requests) == 0
}

// SetupWithManager sets up the controller with the Manager.
func (r *RedisSentinelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&redisv1.RedisSentinel{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Owns(&appsv1.StatefulSet{}).
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				cm := obj.(*corev1.ConfigMap)
				for _, ownerRef := range cm.OwnerReferences {
					if ownerRef.Kind == "RedisSentinel" {
						return []reconcile.Request{{
							NamespacedName: types.NamespacedName{
								Name:      ownerRef.Name,
								Namespace: cm.Namespace,
							},
						}}
					}
				}
				return nil
			}),
		).
		Watches(
			&appsv1.StatefulSet{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				sts := obj.(*appsv1.StatefulSet)
				for _, ownerRef := range sts.OwnerReferences {
					if ownerRef.Kind == "RedisSentinel" {
						return []reconcile.Request{{
							NamespacedName: types.NamespacedName{
								Name:      ownerRef.Name,
								Namespace: sts.Namespace,
							},
						}}
					}
				}
				return nil
			}),
		).
		Complete(r)
}
