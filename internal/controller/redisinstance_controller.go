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
	"crypto/sha256"
	"fmt"
	"reflect"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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

// RedisInstanceReconciler reconciles a RedisInstance object
type RedisInstanceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=redis.github.com,resources=redisinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=redis.github.com,resources=redisinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=redis.github.com,resources=redisinstances/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the RedisInstance object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *RedisInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logs := logf.FromContext(ctx)

	// TODO(user): your logic here
	// logs.Info("Reconcile RedisInstance", "req", req)

	// Fetch the RedisInstance instance
	redisInstance := &redisv1.RedisInstance{}
	configMap := &corev1.ConfigMap{}
	statefulSet := &appsv1.StatefulSet{}
	service := &corev1.Service{}

	// DELETED
	err := r.Get(ctx, req.NamespacedName, redisInstance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			logs.Info("已经删除 Reconcile RedisInstance not found", "redisInstance", redisInstance)
			if err = r.cleanupResources(ctx, req, redisInstance, logs); err != nil {
				logs.Error(err, "Failed to cleanup resources")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, err
	}
	// DELETE
	// 更新redisinstance为terminated，等待手动移除finalizers
	// CREATE
	if redisInstance.Status.Conditions == nil {
		logs.Info("创建 Reconcile RedisInstance will be create", "redisInstance", redisInstance)
		// 设置 finalizer 但不设置到子资源上
		redisInstance.ObjectMeta.Finalizers = []string{string(redisv1.RedisInstanceFinalizer)}
		err = r.Update(ctx, redisInstance)
		if err != nil {
			logs.Error(err, "Failed to update RedisInstance finalizer")
			return ctrl.Result{}, err
		}
	}

	// 检查并创建或更新所有资源
	err = r.ensureResources(ctx, req, redisInstance, statefulSet, configMap, service, logs)
	if err != nil {
		logs.Error(err, "Failed to ensure resources")
		return ctrl.Result{}, err
	}

	// 无论是否更新，都检查并更新状态
	logs.Info("Reconcile updateRedisInstanceStatus")
	err = r.updateRedisInstanceStatus(ctx, redisInstance)
	if err != nil {
		logs.Error(err, "Failed to update RedisInstance status")
		return ctrl.Result{}, err
	}

	// 返回结果，并设置重新队列的时间，以便定期检查状态
	return ctrl.Result{RequeueAfter: time.Second * 30}, nil
}

func (r *RedisInstanceReconciler) cleanupResources(ctx context.Context, req ctrl.Request, redisInstance *redisv1.RedisInstance, logs logr.Logger) error {
	// TODO: implement cleanupResources
	// delete service
	logs.Info("cleanupResources delete service configMap statefulSet")
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redisInstance.Name,
			Namespace: redisInstance.Namespace,
		},
	}
	if err := r.Get(ctx, req.NamespacedName, service); err == nil {
		controllerutil.RemoveFinalizer(service, redisv1.RedisInstanceFinalizer)
		if err = r.Update(ctx, service); err != nil {
			logs.Error(err, "Failed to update RedisInstance service finalizer")
			return err
		}
		if err = r.Delete(ctx, service); err != nil {
			logs.Error(err, "Failed to delete Service")
			return err
		}
	}

	// delete statefulSet
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redisInstance.Name,
			Namespace: redisInstance.Namespace,
		},
	}
	if err := r.Get(ctx, req.NamespacedName, statefulSet); err == nil {
		controllerutil.RemoveFinalizer(statefulSet, redisv1.RedisInstanceFinalizer)
		if err = r.Update(ctx, statefulSet); err != nil {
			logs.Error(err, "Failed to update RedisInstance statefulSet finalizer")
			return err
		}
		if err = r.Delete(ctx, statefulSet); err != nil {
			logs.Error(err, "Failed to delete StatefulSet")
			return err
		}
	}
	// delete configMap
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redisInstance.Name,
			Namespace: redisInstance.Namespace,
		},
	}
	if err := r.Get(ctx, req.NamespacedName, configMap); err == nil {
		controllerutil.RemoveFinalizer(configMap, redisv1.RedisInstanceFinalizer)
		if err = r.Update(ctx, configMap); err != nil {
			logs.Error(err, "Failed to update RedisInstance configMap finalizer")
			return err
		}
		if err = r.Delete(ctx, configMap); err != nil {
			logs.Error(err, "Failed to delete ConfigMap")
			return err
		}
	}
	return nil
}

// func (r *RedisInstanceReconciler) CreateOrUpdate(ctx context.Context, req ctrl.Request, redisInstance *redisv1.RedisInstance, statefulSet *appsv1.StatefulSet, configMap *corev1.ConfigMap, service *corev1.Service, logs logr.Logger) error {
// 	// configMap
// 	err := r.Get(ctx, req.NamespacedName, configMap)
// 	if errors.IsNotFound(err) {
// 		// Request object not found, could have been deleted after reconcile request.
// 		// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
// 		// Return and don't requeue
// 		logs.Info("Reconcile RedisInstance.configMap not found", "configMap", configMap)
// 		configMap, err = r.configMapForRedisInstance(redisInstance, logs)
// 		if err != nil {
// 			return err
// 		}
// 		// err = r.Create(ctx, configMap)
// 		err = r.Create(ctx, configMap)
// 		if err != nil {
// 			return err
// 		}
// 	} else {
// 		return err
// 	}

// 	// StatefulSet
// 	err = r.Get(ctx, req.NamespacedName, statefulSet)
// 	if errors.IsNotFound(err) {
// 		// Request object not found, could have been deleted after reconcile request.
// 		// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
// 		// Return and don't requeue
// 		logs.Info("Reconcile RedisInstance.statefulSet not found", "statefulSet", statefulSet)
// 		statefulSet, err = r.statefulSetForRedisInstance(redisInstance, logs)
// 		if err != nil {
// 			return err
// 		}
// 		err = r.Create(ctx, statefulSet)
// 		if err != nil {
// 			return err
// 		}
// 	} else {
// 		return err
// 	}
// 	// SERVICE
// 	err = r.Get(ctx, req.NamespacedName, service)
// 	if errors.IsNotFound(err) {
// 		// Request object not found, could have been deleted after reconcile request.
// 		// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
// 		// Return and don't requeue
// 		service, err = r.serviceForRedisInstance(redisInstance, logs)
// 		if err != nil {
// 			return err
// 		}
// 		logs.Info("Reconcile RedisInstance.service not found", "service", service)
// 		err = r.Create(ctx, service)
// 		if err != nil {
// 			return err
// 		}
// 	} else {
// 		return err
// 	}
// 	return nil
// }

func (r *RedisInstanceReconciler) statefulSetForRedisInstance(redisInstance *redisv1.RedisInstance, logs logr.Logger) (*appsv1.StatefulSet, error) {
	label := utils.LabelsForRedis(redisInstance.Name)
	owner := []metav1.OwnerReference{
		{
			APIVersion: "redis.github.com/v1",
			Kind:       "RedisInstance",
			Name:       redisInstance.Name,
			UID:        redisInstance.UID,
		},
	}
	redisInstanceFinalizer := []string{
		redisv1.RedisInstanceFinalizer,
	}
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            redisInstance.Name,
			Namespace:       redisInstance.Namespace,
			Labels:          label,
			OwnerReferences: owner,
			Finalizers:      redisInstanceFinalizer,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &redisInstance.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: label,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: label,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "redis",
							Image: redisInstance.Spec.Image,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 6379,
									Name:          "redis",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "redis-data",
									MountPath: "/data",
								},
								{
									Name:      "redis-config",
									MountPath: "/usr/local/etc/redis",
								},
							},
							Resources: redisInstance.Spec.Resources,
							Command: []string{
								"redis-server",
								"/usr/local/etc/redis/redis.conf",
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "redis-data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "redis-data",
								},
							},
						},
						{
							Name: "redis-config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: redisInstance.Name,
									},
								},
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
						StorageClassName: &redisInstance.Spec.Storage.StorageClassName,
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								"storage": resource.MustParse(redisInstance.Spec.Storage.Size),
							},
						},
					},
				},
			},
			ServiceName: redisInstance.Name,
		},
	}

	if err := ctrl.SetControllerReference(redisInstance, sts, r.Scheme); err != nil {
		logs.Error(err, "statefulSetForRedisInstance SetControllerReference Error")
		return nil, err
	}
	return sts, nil
}

func (r *RedisInstanceReconciler) configMapForRedisInstance(redisInstance *redisv1.RedisInstance, logs logr.Logger) (*corev1.ConfigMap, error) {
	label := utils.LabelsForRedis(redisInstance.Name)
	config := utils.GenerateRedisConfig(redisInstance.Spec.Config)
	owner := []metav1.OwnerReference{
		{
			APIVersion: "redis.github.com/v1",
			Kind:       "RedisInstance",
			Name:       redisInstance.Name,
			UID:        redisInstance.UID,
		},
	}
	redisInstanceFinalizer := []string{
		redisv1.RedisInstanceFinalizer,
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            redisInstance.Name,
			Namespace:       redisInstance.Namespace,
			Labels:          label,
			OwnerReferences: owner,
			Finalizers:      redisInstanceFinalizer,
		},
		Data: map[string]string{
			"redis.conf": config,
		},
	}
	if err := ctrl.SetControllerReference(redisInstance, cm, r.Scheme); err != nil {
		logs.Error(err, "configMapForRedisInstance SetControllerReference Error")
		return nil, err
	}
	return cm, nil
}

func (r *RedisInstanceReconciler) serviceForRedisInstance(redisInstance *redisv1.RedisInstance, logs logr.Logger) (*corev1.Service, error) {
	label := utils.LabelsForRedis(redisInstance.Name)
	owner := []metav1.OwnerReference{
		{
			APIVersion: "redis.github.com/v1",
			Kind:       "RedisInstance",
			Name:       redisInstance.Name,
			UID:        redisInstance.UID,
		},
	}
	// 移除 finalizers 设置
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            redisInstance.Name,
			Namespace:       redisInstance.Namespace,
			Labels:          label,
			OwnerReferences: owner,
			// 不设置 Finalizers
		},
		Spec: corev1.ServiceSpec{
			Selector: label,
			Ports: []corev1.ServicePort{
				{
					Port: 6379,
					Name: "redis",
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}
	if err := ctrl.SetControllerReference(redisInstance, svc, r.Scheme); err != nil {
		logs.Error(err, "serviceForRedisInstance SetControllerReference Error")
		return nil, err
	}
	return svc, nil
}

// calculateConfigHash 计算配置的哈希值
func (r *RedisInstanceReconciler) calculateConfigHash(config string) string {
	h := sha256.Sum256([]byte(config))
	return fmt.Sprintf("%x", h)
}

// needsStatefulSetRestart 检查是否需要重启 StatefulSet
// needsStatefulSetRestart 检查是否需要重建StatefulSet
// 只有配置文件变化和存储变化需要重建，其他变化可以通过滚动更新处理
func (r *RedisInstanceReconciler) needsStatefulSetRestart(ctx context.Context, redisInstance *redisv1.RedisInstance, statefulSet *appsv1.StatefulSet, logs logr.Logger) (bool, error) {
	needsRestart := false
	reasonMsg := ""

	// 1. 检查配置文件变化 - 需要重建
	expectedConfig := utils.GenerateRedisConfig(redisInstance.Spec.Config)
	expectedHash := r.calculateConfigHash(expectedConfig)

	var stsConfigHash string
	if statefulSet.Spec.Template.Annotations != nil {
		stsConfigHash = statefulSet.Spec.Template.Annotations["redis.github.com/config-hash"]
	}

	if stsConfigHash != expectedHash {
		logs.Info("Config change detected, StatefulSet restart required", "expected", expectedHash, "current", stsConfigHash)
		needsRestart = true
		reasonMsg = "Redis configuration has changed, StatefulSet will be recreated"
	}

	// 2. 检查存储配置变化 - 需要重建（PVC不能直接修改）
	if len(statefulSet.Spec.VolumeClaimTemplates) > 0 {
		currentStorageSize := statefulSet.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests["storage"]
		expectedStorageSize := resource.MustParse(redisInstance.Spec.Storage.Size)
		if !currentStorageSize.Equal(expectedStorageSize) {
			logs.Info("Storage size change detected, StatefulSet restart required",
				"expected", expectedStorageSize.String(), "current", currentStorageSize.String())
			needsRestart = true
			reasonMsg = "Storage size has changed, StatefulSet will be recreated"
		}

		currentStorageClass := ""
		if statefulSet.Spec.VolumeClaimTemplates[0].Spec.StorageClassName != nil {
			currentStorageClass = *statefulSet.Spec.VolumeClaimTemplates[0].Spec.StorageClassName
		}
		if currentStorageClass != redisInstance.Spec.Storage.StorageClassName {
			logs.Info("Storage class change detected, StatefulSet restart required",
				"expected", redisInstance.Spec.Storage.StorageClassName, "current", currentStorageClass)
			needsRestart = true
			reasonMsg = "Storage class has changed, StatefulSet will be recreated"
		}
	}

	// 如果需要重建，设置RedisInstance状态为Updating
	if needsRestart {
		r.setUpdatingStatus(ctx, redisInstance, "StatefulSetRestart", reasonMsg)
	} else {
		logs.Info("No restart-requiring changes detected")
	}

	return needsRestart, nil
}

// needsStatefulSetUpdate 检查是否需要更新StatefulSet（不重建）
// 副本数、镜像、资源配置等可以通过滚动更新处理
func (r *RedisInstanceReconciler) needsStatefulSetUpdate(ctx context.Context, redisInstance *redisv1.RedisInstance, statefulSet *appsv1.StatefulSet, logs logr.Logger) bool {
	updated := false

	// 1. 检查副本数变化 - 可以直接更新
	if statefulSet.Spec.Replicas == nil || *statefulSet.Spec.Replicas != redisInstance.Spec.Replicas {
		currentReplicas := int32(0)
		if statefulSet.Spec.Replicas != nil {
			currentReplicas = *statefulSet.Spec.Replicas
		}
		logs.Info("Replicas change detected, will update", "expected", redisInstance.Spec.Replicas, "current", currentReplicas)
		statefulSet.Spec.Replicas = &redisInstance.Spec.Replicas
		updated = true
	}

	// 2. 检查容器配置变化
	if len(statefulSet.Spec.Template.Spec.Containers) > 0 {
		container := &statefulSet.Spec.Template.Spec.Containers[0]

		// 检查镜像变化 - 可以滚动更新
		if container.Image != redisInstance.Spec.Image {
			logs.Info("Image change detected, will update", "expected", redisInstance.Spec.Image, "current", container.Image)
			container.Image = redisInstance.Spec.Image
			updated = true
		}

		// 检查资源配置变化 - 可以滚动更新
		if !reflect.DeepEqual(container.Resources, redisInstance.Spec.Resources) {
			logs.Info("Resources change detected, will update")
			container.Resources = redisInstance.Spec.Resources
			updated = true
		}
	}

	// 如果有更新，设置RedisInstance状态为Updating
	if updated {
		r.setUpdatingStatus(ctx, redisInstance, "StatefulSetUpdate", "StatefulSet is being updated with new configuration")
	}

	return updated
}

func (r *RedisInstanceReconciler) ensureResources(ctx context.Context, req ctrl.Request, redisInstance *redisv1.RedisInstance, statefulSet *appsv1.StatefulSet, configMap *corev1.ConfigMap, service *corev1.Service, logs logr.Logger) error {
	// 检查 ConfigMap 是否存在
	configMapErr := r.Get(ctx, types.NamespacedName{Name: redisInstance.Name, Namespace: redisInstance.Namespace}, configMap)
	configMapRecreated := false

	if errors.IsNotFound(configMapErr) {
		logs.Info("ConfigMap not found, creating new one", "name", redisInstance.Name)
		newConfigMap, err := r.configMapForRedisInstance(redisInstance, logs)
		if err != nil {
			return err
		}
		// 移除 finalizer
		newConfigMap.ObjectMeta.Finalizers = []string{}
		if err := r.Create(ctx, newConfigMap); err != nil {
			logs.Error(err, "Failed to create ConfigMap")
			return err
		}
		// 标记 ConfigMap 被重新创建
		configMapRecreated = true
	} else if configMapErr != nil {
		return configMapErr
	} else {
		// ConfigMap 存在，检查配置是否需要更新
		expectedConfig := utils.GenerateRedisConfig(redisInstance.Spec.Config)
		currentConfig := configMap.Data["redis.conf"]

		if expectedConfig != currentConfig {
			logs.Info("ConfigMap configuration changed, updating", "name", redisInstance.Name)
			configMap.Data["redis.conf"] = expectedConfig
			if err := r.Update(ctx, configMap); err != nil {
				logs.Error(err, "Failed to update ConfigMap")
				return err
			}
		}

		// 检查是否需要移除 finalizer
		if controllerutil.ContainsFinalizer(configMap, redisv1.RedisInstanceFinalizer) {
			controllerutil.RemoveFinalizer(configMap, redisv1.RedisInstanceFinalizer)
			if err := r.Update(ctx, configMap); err != nil {
				logs.Error(err, "Failed to remove ConfigMap finalizer")
				return err
			}
		}
	}

	// 检查 StatefulSet 是否存在
	statefulSetErr := r.Get(ctx, types.NamespacedName{Name: redisInstance.Name, Namespace: redisInstance.Namespace}, statefulSet)
	needsRestart := false

	// 如果 StatefulSet 存在，检查是否需要重启
	if statefulSetErr == nil {
		restart, err := r.needsStatefulSetRestart(ctx, redisInstance, statefulSet, logs)
		if err != nil {
			logs.Error(err, "Failed to check if StatefulSet needs restart")
			return err
		}
		needsRestart = restart
	}

	// 如果 StatefulSet 不存在或 ConfigMap 被重新创建，则创建 StatefulSet
	if errors.IsNotFound(statefulSetErr) || configMapRecreated {
		logs.Info("StatefulSet not found or ConfigMap recreated, creating new StatefulSet", "name", redisInstance.Name)
		newStatefulSet, err := r.statefulSetForRedisInstance(redisInstance, logs)
		if err != nil {
			return err
		}

		// 添加配置哈希值到 StatefulSet 的 annotation 中
		expectedConfig := utils.GenerateRedisConfig(redisInstance.Spec.Config)
		configHash := r.calculateConfigHash(expectedConfig)
		if newStatefulSet.Spec.Template.Annotations == nil {
			newStatefulSet.Spec.Template.Annotations = make(map[string]string)
		}
		newStatefulSet.Spec.Template.Annotations["redis.github.com/config-hash"] = configHash

		// 移除 finalizer
		newStatefulSet.ObjectMeta.Finalizers = []string{}
		if err := r.Create(ctx, newStatefulSet); err != nil {
			logs.Error(err, "Failed to create StatefulSet")
			return err
		}
	} else if statefulSetErr != nil {
		return statefulSetErr
	} else {
		// StatefulSet 存在，检查是否需要重启
		if needsRestart {
			logs.Info("StatefulSet needs restart due to config change, deleting existing StatefulSet", "name", redisInstance.Name)
			// 移除 finalizer
			if controllerutil.ContainsFinalizer(statefulSet, redisv1.RedisInstanceFinalizer) {
				controllerutil.RemoveFinalizer(statefulSet, redisv1.RedisInstanceFinalizer)
				if err := r.Update(ctx, statefulSet); err != nil {
					logs.Error(err, "Failed to remove StatefulSet finalizer")
					return err
				}
			}
			if err := r.Delete(ctx, statefulSet); err != nil {
				logs.Error(err, "Failed to delete StatefulSet for recreation")
				return err
			}
			// 等待 StatefulSet 被删除
			time.Sleep(5 * time.Second)

			// 创建新的 StatefulSet
			newStatefulSet, err := r.statefulSetForRedisInstance(redisInstance, logs)
			if err != nil {
				return err
			}

			// 添加配置哈希值到 StatefulSet 的 annotation 中
			expectedConfig := utils.GenerateRedisConfig(redisInstance.Spec.Config)
			configHash := r.calculateConfigHash(expectedConfig)
			if newStatefulSet.Spec.Template.Annotations == nil {
				newStatefulSet.Spec.Template.Annotations = make(map[string]string)
			}
			newStatefulSet.Spec.Template.Annotations["redis.github.com/config-hash"] = configHash

			// 移除 finalizer
			newStatefulSet.ObjectMeta.Finalizers = []string{}
			if err := r.Create(ctx, newStatefulSet); err != nil {
				logs.Error(err, "Failed to create StatefulSet")
				return err
			}
		} else {
			// StatefulSet 存在且不需要重启，检查是否需要更新规格或移除 finalizer
			updated := false

			// 检查是否需要移除 finalizer
			if controllerutil.ContainsFinalizer(statefulSet, redisv1.RedisInstanceFinalizer) {
				controllerutil.RemoveFinalizer(statefulSet, redisv1.RedisInstanceFinalizer)
				updated = true
			}

			// 检查是否需要更新 StatefulSet 规格（副本数、镜像、资源等）
			specUpdated := r.needsStatefulSetUpdate(ctx, redisInstance, statefulSet, logs)
			if specUpdated {
				logs.Info("StatefulSet spec needs update, performing rolling update")
				updated = true
			}

			// 只有在需要移除 finalizer 或更新规格时才更新 StatefulSet
			if updated {
				if err := r.Update(ctx, statefulSet); err != nil {
					logs.Error(err, "Failed to update StatefulSet")
					return err
				}
			}
		}
	}

	// 检查 Service 是否存在
	serviceErr := r.Get(ctx, types.NamespacedName{Name: redisInstance.Name, Namespace: redisInstance.Namespace}, service)
	if errors.IsNotFound(serviceErr) {
		logs.Info("Service not found, creating new one", "name", redisInstance.Name)
		newService, err := r.serviceForRedisInstance(redisInstance, logs)
		if err != nil {
			return err
		}
		// 移除 finalizer
		newService.ObjectMeta.Finalizers = []string{}
		if err := r.Create(ctx, newService); err != nil {
			logs.Error(err, "Failed to create Service")
			return err
		}
	} else if serviceErr != nil {
		return serviceErr
	} else {
		// Service 存在，检查是否需要移除 finalizer
		if controllerutil.ContainsFinalizer(service, redisv1.RedisInstanceFinalizer) {
			controllerutil.RemoveFinalizer(service, redisv1.RedisInstanceFinalizer)
			if err := r.Update(ctx, service); err != nil {
				logs.Error(err, "Failed to remove Service finalizer")
				return err
			}
		}
	}

	return nil
}

// setUpdatingStatus 设置RedisInstance状态为Updating
func (r *RedisInstanceReconciler) setUpdatingStatus(ctx context.Context, redisInstance *redisv1.RedisInstance, reason, message string) {
	// 设置状态为Updating
	meta.SetStatusCondition(&redisInstance.Status.Conditions, metav1.Condition{
		Type:               string(redisv1.RedisPhaseUpdating),
		Status:             metav1.ConditionFalse, // Ready为false
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
		ObservedGeneration: redisInstance.Generation,
	})

	// 更新状态字段
	redisInstance.Status.Status = string(redisv1.RedisPhaseUpdating)
	redisInstance.Status.Ready = string(metav1.ConditionFalse)
	redisInstance.Status.LastConditionMessage = message

	// 异步更新状态，避免阻塞主流程
	go func() {
		if err := r.Status().Update(ctx, redisInstance); err != nil {
			// 记录错误但不影响主流程
			ctrl.Log.Error(err, "Failed to update RedisInstance status to Updating")
		}
	}()
}

func (r *RedisInstanceReconciler) updateRedisInstanceStatus(ctx context.Context, redisInstance *redisv1.RedisInstance) error {
	// 获取 ConfigMap 状态
	configMap := &corev1.ConfigMap{}
	configMapErr := r.Get(ctx, types.NamespacedName{Name: redisInstance.Name, Namespace: redisInstance.Namespace}, configMap)

	// 获取 Service 状态
	service := &corev1.Service{}
	serviceErr := r.Get(ctx, types.NamespacedName{Name: redisInstance.Name, Namespace: redisInstance.Namespace}, service)

	// 获取 StatefulSet 状态
	sts := &appsv1.StatefulSet{}
	statefulSetErr := r.Get(ctx, types.NamespacedName{Name: redisInstance.Name, Namespace: redisInstance.Namespace}, sts)

	// 设置状态条件
	var conditionStatus metav1.ConditionStatus
	var conditionType string
	var reason string
	var message string

	// 检查是否正在更新中
	isUpdating := false
	if statefulSetErr == nil {
		// 检查StatefulSet是否正在进行滚动更新
		// 只有当UpdatedReplicas < Replicas时才认为正在更新
		// 如果CurrentRevision != UpdateRevision但UpdatedReplicas == Replicas，说明更新已完成
		if sts.Status.UpdatedReplicas < sts.Status.Replicas {
			isUpdating = true
		}
		// 额外检查：如果有Pod还没有就绪，也可能是在更新过程中
		if sts.Status.ReadyReplicas < sts.Status.UpdatedReplicas {
			isUpdating = true
		}
	}

	// 更新 RedisInstance 状态
	if !redisInstance.DeletionTimestamp.IsZero() {
		// 如果正在删除，设置为 Terminated 状态
		conditionStatus = metav1.ConditionUnknown
		conditionType = string(redisv1.RedisPhaseTerminated)
		reason = "Deleteing"
		message = fmt.Sprintf("If you want to delete RedisInstance %s,you need remove finalizer.", redisInstance.Name)
	} else if configMapErr != nil || serviceErr != nil || statefulSetErr != nil {
		// 如果任何资源不存在，设置为 Failed 状态
		conditionStatus = metav1.ConditionFalse
		conditionType = string(redisv1.RedisPhaseFailed)
		reason = "ResourceMissing"
		message = fmt.Sprintf("One or more required resources are missing. ConfigMap: %v, Service: %v, StatefulSet: %v",
			configMapErr, serviceErr, statefulSetErr)
	} else if isUpdating {
		// 如果StatefulSet正在更新，保持Updating状态
		conditionStatus = metav1.ConditionFalse // Ready为false
		conditionType = string(redisv1.RedisPhaseUpdating)
		reason = "StatefulSetUpdating"
		message = fmt.Sprintf("StatefulSet is being updated. Updated: %d/%d, Ready: %d/%d",
			sts.Status.UpdatedReplicas, sts.Status.Replicas, sts.Status.ReadyReplicas, sts.Status.Replicas)
	} else if sts.Status.ReadyReplicas == 0 {
		// 如果 StatefulSet 没有就绪的副本，设置为 Pending 状态
		conditionStatus = metav1.ConditionFalse
		conditionType = string(redisv1.RedisPhasePending)
		reason = "RedisInstancePending"
		message = fmt.Sprintf("statefulSet for RedisInstance %s is pending.", redisInstance.Name)
	} else if sts.Status.ReadyReplicas == sts.Status.Replicas {
		// 如果所有副本都就绪，设置为 Running 状态
		conditionStatus = metav1.ConditionTrue
		conditionType = string(redisv1.RedisPhaseRunning)
		reason = "RedisInstanceReady"
		message = fmt.Sprintf("RedisInstance is running. StatefulSet: %v, Replicas: %v",
			sts.Status.ReadyReplicas, sts.Status.Replicas)
	} else {
		// 如果部分副本就绪，设置为 Running 状态但条件为 False
		conditionStatus = metav1.ConditionFalse
		conditionType = string(redisv1.RedisPhaseRunning)
		reason = "RedisInstanceRunning"
		message = fmt.Sprintf("RedisInstance is running with %d/%d ready replicas.",
			sts.Status.ReadyReplicas, sts.Status.Replicas)
	}

	// 限制 Conditions 数量最多为 10 个
	if len(redisInstance.Status.Conditions) >= 10 {
		// 按时间排序，保留最新的 9 个
		sort.Slice(redisInstance.Status.Conditions, func(i, j int) bool {
			return redisInstance.Status.Conditions[i].LastTransitionTime.Before(&redisInstance.Status.Conditions[j].LastTransitionTime)
		})
		// 删除最旧的条件，直到只剩下 9 个
		redisInstance.Status.Conditions = redisInstance.Status.Conditions[len(redisInstance.Status.Conditions)-9:]
	}

	// 设置状态条件，并添加 ObservedGeneration 到 Condition 中
	meta.SetStatusCondition(&redisInstance.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             conditionStatus,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
		ObservedGeneration: redisInstance.Generation,
	})

	// 直接使用当前计算出的状态，而不是从conditions数组中获取
	redisInstance.Status.LastConditionMessage = message
	redisInstance.Status.Status = conditionType
	redisInstance.Status.Ready = string(conditionStatus)

	return r.Status().Update(ctx, redisInstance)
}

// SetupWithManager sets up the controller with the Manager.
func (r *RedisInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&redisv1.RedisInstance{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Owns(&appsv1.StatefulSet{}).
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				cm := obj.(*corev1.ConfigMap)
				for _, ownerRef := range cm.OwnerReferences {
					if ownerRef.Kind == "RedisInstance" {
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
			&corev1.Service{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				svc := obj.(*corev1.Service)
				for _, ownerRef := range svc.OwnerReferences {
					if ownerRef.Kind == "RedisInstance" {
						return []reconcile.Request{{
							NamespacedName: types.NamespacedName{
								Name:      ownerRef.Name,
								Namespace: svc.Namespace,
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
					if ownerRef.Kind == "RedisInstance" {
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
