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
	"github.com/ybooks240/redis-operator/internal/metrics"
)

// RedisClusterReconciler reconciles a RedisCluster object
type RedisClusterReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	MetricsManager *metrics.MetricsCollectionManager
}

// +kubebuilder:rbac:groups=redis.github.com,resources=redisclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=redis.github.com,resources=redisclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=redis.github.com,resources=redisclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *RedisClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logs := logf.FromContext(ctx)

	// 获取 RedisCluster 实例
	redisCluster := &redisv1.RedisCluster{}
	err := r.Get(ctx, req.NamespacedName, redisCluster)
	if err != nil {
		if errors.IsNotFound(err) {
			// 资源已被删除，无需处理
			logs.Info("RedisCluster not found, skipping reconciliation", "name", req.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// 检查是否正在删除
	if redisCluster.DeletionTimestamp != nil {
		logs.Info("RedisCluster is being deleted, cleaning up resources", "name", redisCluster.Name)
		if err = r.cleanupResources(ctx, req, redisCluster, logs); err != nil {
			logs.Error(err, "Failed to cleanup resources")
			return ctrl.Result{}, err
		}
		// 移除 finalizer，使用重试机制避免资源版本冲突
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			// 重新获取最新的资源版本
			latestRedisCluster := &redisv1.RedisCluster{}
			if err = r.Get(ctx, types.NamespacedName{Name: redisCluster.Name, Namespace: redisCluster.Namespace}, latestRedisCluster); err != nil {
				return err
			}
			controllerutil.RemoveFinalizer(latestRedisCluster, redisv1.RedisClusterFinalizer)
			return r.Update(ctx, latestRedisCluster)
		})
		if err != nil {
			logs.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// 初始化状态和 finalizer
	if redisCluster.Status.Conditions == nil {
		logs.Info("Initializing RedisCluster", "name", redisCluster.Name)
		controllerutil.AddFinalizer(redisCluster, redisv1.RedisClusterFinalizer)
		err = r.Update(ctx, redisCluster)
		if err != nil {
			logs.Error(err, "Failed to update RedisCluster finalizer")
			return ctrl.Result{}, err
		}
	}

	// 确保所有资源存在并正确配置
	err = r.ensureResources(ctx, req, redisCluster, logs)
	if err != nil {
		logs.Error(err, "Failed to ensure resources")
		return ctrl.Result{}, err
	}

	// 更新状态
	err = r.updateRedisClusterStatus(ctx, redisCluster)
	if err != nil {
		logs.Error(err, "Failed to update RedisCluster status")
		return ctrl.Result{}, err
	}

	// 注册指标收集器
	if r.MetricsManager != nil {
		// 为 Redis Cluster 添加指标收集器
		clusterAddrs := []string{fmt.Sprintf("%s-service.%s.svc.cluster.local:6379", redisCluster.Name, redisCluster.Namespace)}
		clusterCollector := metrics.NewClusterCollector(
			clusterAddrs,
			"", // 无密码
			redisCluster.Namespace,
			redisCluster.Name,
		)
		r.MetricsManager.AddClusterCollector(clusterCollector)

		// 记录协调操作指标
		metrics.RecordReconcile("RedisCluster", redisCluster.Namespace, redisCluster.Name, "success", 0.0)

		// 更新集群状态指标
		if redisCluster.Status.Status != "" {
			var statusValue float64 = 0
			if redisCluster.Status.Status == string(redisv1.RedisPhaseRunning) {
				statusValue = 1
			}
			metrics.SetRedisClusterState(redisCluster.Namespace, redisCluster.Name, statusValue)
		}
	}

	return ctrl.Result{RequeueAfter: time.Second * 30}, nil
}

// cleanupResources 清理相关资源
func (r *RedisClusterReconciler) cleanupResources(ctx context.Context, req ctrl.Request, redisCluster *redisv1.RedisCluster, logs logr.Logger) error {
	// 删除 StatefulSet
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redisCluster.Name,
			Namespace: redisCluster.Namespace,
		},
	}
	if err := r.Get(ctx, types.NamespacedName{Name: statefulSet.Name, Namespace: statefulSet.Namespace}, statefulSet); err == nil {
		controllerutil.RemoveFinalizer(statefulSet, redisv1.RedisClusterFinalizer)
		if err = r.Update(ctx, statefulSet); err != nil {
			logs.Error(err, "Failed to remove StatefulSet finalizer")
			return err
		}
		if err = r.Delete(ctx, statefulSet); err != nil {
			logs.Error(err, "Failed to delete StatefulSet")
			return err
		}
	}

	// 删除 Services 和 ConfigMaps
	resourcesToDelete := []client.Object{
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: redisCluster.Name + "-service", Namespace: redisCluster.Namespace}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: redisCluster.Name + "-config", Namespace: redisCluster.Namespace}},
	}

	for _, resource := range resourcesToDelete {
		if err := r.Get(ctx, types.NamespacedName{Name: resource.GetName(), Namespace: resource.GetNamespace()}, resource); err == nil {
			controllerutil.RemoveFinalizer(resource, redisv1.RedisClusterFinalizer)
			if err = r.Update(ctx, resource); err != nil {
				logs.Error(err, "Failed to remove finalizer", "resource", resource.GetName())
				return err
			}
			if err = r.Delete(ctx, resource); err != nil {
				logs.Error(err, "Failed to delete resource", "resource", resource.GetName())
				return err
			}
		}
	}

	return nil
}

// ensureResources 确保所有必要的资源存在
func (r *RedisClusterReconciler) ensureResources(ctx context.Context, req ctrl.Request, redisCluster *redisv1.RedisCluster, logs logr.Logger) error {
	// 确保 ConfigMap
	if err := r.ensureConfigMap(ctx, redisCluster, logs); err != nil {
		return err
	}

	// 确保 StatefulSet
	if err := r.ensureStatefulSet(ctx, redisCluster, logs); err != nil {
		return err
	}

	// 确保 Service
	if err := r.ensureService(ctx, redisCluster, logs); err != nil {
		return err
	}

	return nil
}

// ensureConfigMap 确保 ConfigMap 存在
func (r *RedisClusterReconciler) ensureConfigMap(ctx context.Context, redisCluster *redisv1.RedisCluster, logs logr.Logger) error {
	configMap := &corev1.ConfigMap{}
	configMapName := redisCluster.Name + "-config"
	err := r.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: redisCluster.Namespace}, configMap)

	if errors.IsNotFound(err) {
		// 创建新的 ConfigMap
		configMap = r.configMapForCluster(redisCluster)
		if err = controllerutil.SetControllerReference(redisCluster, configMap, r.Scheme); err != nil {
			return err
		}
		controllerutil.AddFinalizer(configMap, redisv1.RedisClusterFinalizer)
		logs.Info("Creating cluster ConfigMap", "name", configMap.Name)
		return r.Create(ctx, configMap)
	} else if err != nil {
		return err
	} else {
		// 检查 ConfigMap 是否需要更新
		desiredConfigMap := r.configMapForCluster(redisCluster)
		needsUpdate := false

		// 比较配置数据
		for key, desiredValue := range desiredConfigMap.Data {
			if existingValue, exists := configMap.Data[key]; !exists || existingValue != desiredValue {
				needsUpdate = true
				break
			}
		}

		if needsUpdate {
			// 设置 Updating 状态
			if err := r.setUpdatingStatus(ctx, redisCluster, "ConfigMap update detected"); err != nil {
				logs.Error(err, "Failed to set updating status")
			}

			// 更新 ConfigMap
			configMap.Data = desiredConfigMap.Data
			logs.Info("Updating cluster ConfigMap", "name", configMap.Name)
			return r.Update(ctx, configMap)
		}
	}

	return nil
}

// setUpdatingStatus 设置 RedisCluster 为 Updating 状态
func (r *RedisClusterReconciler) setUpdatingStatus(ctx context.Context, redisCluster *redisv1.RedisCluster, message string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// 重新获取最新的资源版本
		latestCluster := &redisv1.RedisCluster{}
		if err := r.Get(ctx, types.NamespacedName{Name: redisCluster.Name, Namespace: redisCluster.Namespace}, latestCluster); err != nil {
			return err
		}

		// 设置 Updating 状态
		latestCluster.Status.Status = string(redisv1.RedisClusterPhaseUpdating)
		latestCluster.Status.Ready = "False"
		latestCluster.Status.LastConditionMessage = message

		// 更新 Conditions
		condition := metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "Updating",
			Message:            message,
			LastTransitionTime: metav1.Now(),
		}

		if latestCluster.Status.Conditions == nil {
			latestCluster.Status.Conditions = []metav1.Condition{}
		}

		// 查找现有条件并更新，或添加新条件
		found := false
		for i, existingCondition := range latestCluster.Status.Conditions {
			if existingCondition.Type == condition.Type {
				latestCluster.Status.Conditions[i] = condition
				found = true
				break
			}
		}
		if !found {
			latestCluster.Status.Conditions = append(latestCluster.Status.Conditions, condition)
		}

		return r.Status().Update(ctx, latestCluster)
	})
}

// ensureStatefulSet 确保 StatefulSet 存在
func (r *RedisClusterReconciler) ensureStatefulSet(ctx context.Context, redisCluster *redisv1.RedisCluster, logs logr.Logger) error {
	statefulSet := &appsv1.StatefulSet{}
	statefulSetName := redisCluster.Name
	err := r.Get(ctx, types.NamespacedName{Name: statefulSetName, Namespace: redisCluster.Namespace}, statefulSet)

	if errors.IsNotFound(err) {
		// 创建新的 StatefulSet
		statefulSet = r.statefulSetForCluster(redisCluster)
		if err = controllerutil.SetControllerReference(redisCluster, statefulSet, r.Scheme); err != nil {
			return err
		}
		controllerutil.AddFinalizer(statefulSet, redisv1.RedisClusterFinalizer)
		logs.Info("Creating cluster StatefulSet", "name", statefulSet.Name)
		return r.Create(ctx, statefulSet)
	} else if err != nil {
		return err
	} else {
		// 检查 StatefulSet 是否需要更新
		desiredStatefulSet := r.statefulSetForCluster(redisCluster)
		needsUpdate := false
		updateReason := ""

		// 检查副本数是否变化
		desiredReplicas := redisCluster.Spec.Masters * (1 + redisCluster.Spec.ReplicasPerMaster)
		if desiredReplicas == 0 {
			desiredReplicas = 6 // 默认值
		}
		if *statefulSet.Spec.Replicas != desiredReplicas {
			needsUpdate = true
			updateReason = fmt.Sprintf("Replica count change: %d -> %d", *statefulSet.Spec.Replicas, desiredReplicas)
		}

		// 检查镜像是否变化
		if !needsUpdate && len(statefulSet.Spec.Template.Spec.Containers) > 0 && len(desiredStatefulSet.Spec.Template.Spec.Containers) > 0 {
			if statefulSet.Spec.Template.Spec.Containers[0].Image != desiredStatefulSet.Spec.Template.Spec.Containers[0].Image {
				needsUpdate = true
				updateReason = fmt.Sprintf("Image change: %s -> %s", statefulSet.Spec.Template.Spec.Containers[0].Image, desiredStatefulSet.Spec.Template.Spec.Containers[0].Image)
			}
		}

		// 检查资源配置是否变化
		if !needsUpdate && len(statefulSet.Spec.Template.Spec.Containers) > 0 && len(desiredStatefulSet.Spec.Template.Spec.Containers) > 0 {
			existingResources := statefulSet.Spec.Template.Spec.Containers[0].Resources
			desiredResources := desiredStatefulSet.Spec.Template.Spec.Containers[0].Resources
			if !existingResources.Requests.Cpu().Equal(*desiredResources.Requests.Cpu()) ||
				!existingResources.Requests.Memory().Equal(*desiredResources.Requests.Memory()) ||
				!existingResources.Limits.Cpu().Equal(*desiredResources.Limits.Cpu()) ||
				!existingResources.Limits.Memory().Equal(*desiredResources.Limits.Memory()) {
				needsUpdate = true
				updateReason = "Resource configuration change detected"
			}
		}

		if needsUpdate {
			// 设置 Updating 状态
			if err := r.setUpdatingStatus(ctx, redisCluster, updateReason); err != nil {
				logs.Error(err, "Failed to set updating status")
			}

			// 更新 StatefulSet
			statefulSet.Spec = desiredStatefulSet.Spec
			logs.Info("Updating cluster StatefulSet", "name", statefulSet.Name, "reason", updateReason)
			return r.Update(ctx, statefulSet)
		}
	}

	return nil
}

// ensureService 确保 Service 存在
func (r *RedisClusterReconciler) ensureService(ctx context.Context, redisCluster *redisv1.RedisCluster, logs logr.Logger) error {
	service := &corev1.Service{}
	serviceName := redisCluster.Name + "-service"
	err := r.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: redisCluster.Namespace}, service)

	if errors.IsNotFound(err) {
		// 创建新的 Service
		service = r.serviceForCluster(redisCluster)
		if err = controllerutil.SetControllerReference(redisCluster, service, r.Scheme); err != nil {
			return err
		}
		controllerutil.AddFinalizer(service, redisv1.RedisClusterFinalizer)
		logs.Info("Creating cluster Service", "name", service.Name)
		return r.Create(ctx, service)
	} else if err != nil {
		return err
	}

	return nil
}

// updateRedisClusterStatus 更新 RedisCluster 状态
func (r *RedisClusterReconciler) updateRedisClusterStatus(ctx context.Context, redisCluster *redisv1.RedisCluster) error {
	// 使用重试机制来处理并发更新冲突
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// 重新获取最新的资源版本以避免冲突
		latestCluster := &redisv1.RedisCluster{}
		if err := r.Get(ctx, types.NamespacedName{Name: redisCluster.Name, Namespace: redisCluster.Namespace}, latestCluster); err != nil {
			return err
		}

		return r.doUpdateRedisClusterStatus(ctx, latestCluster)
	})

	return retryErr
}

// doUpdateRedisClusterStatus 执行实际的状态更新
func (r *RedisClusterReconciler) doUpdateRedisClusterStatus(ctx context.Context, latestCluster *redisv1.RedisCluster) error {

	// 获取 StatefulSet 状态
	statefulSet := &appsv1.StatefulSet{}
	statefulSetName := latestCluster.Name
	statefulSetErr := r.Get(ctx, types.NamespacedName{Name: statefulSetName, Namespace: latestCluster.Namespace}, statefulSet)

	// 更新状态
	if statefulSetErr != nil {
		latestCluster.Status.Status = string(redisv1.RedisClusterPhaseFailed)
		latestCluster.Status.Ready = "False"
		latestCluster.Status.LastConditionMessage = "Failed to get StatefulSet"
	} else {
		totalNodes := latestCluster.Spec.Masters * (1 + latestCluster.Spec.ReplicasPerMaster)
		clusterReady := statefulSet.Status.ReadyReplicas == totalNodes

		if clusterReady {
			latestCluster.Status.Status = string(redisv1.RedisClusterPhaseRunning)
			latestCluster.Status.Ready = "True"
			latestCluster.Status.LastConditionMessage = "All cluster nodes are ready"
		} else {
			latestCluster.Status.Status = string(redisv1.RedisClusterPhasePending)
			latestCluster.Status.Ready = "False"
			latestCluster.Status.LastConditionMessage = "Waiting for cluster nodes to be ready"
		}

		// 更新集群状态
		latestCluster.Status.ServiceName = latestCluster.Name + "-service"
		latestCluster.Status.Cluster.Size = latestCluster.Spec.Masters
		latestCluster.Status.Cluster.KnownNodes = statefulSet.Status.ReadyReplicas
		if clusterReady {
			latestCluster.Status.Cluster.State = "ok"
			latestCluster.Status.Cluster.SlotsAssigned = 16384
			latestCluster.Status.Cluster.SlotsOk = 16384
		} else {
			latestCluster.Status.Cluster.State = "fail"
		}
	}

	// 更新 Conditions
	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "NotReady",
		Message:            latestCluster.Status.LastConditionMessage,
		LastTransitionTime: metav1.Now(),
	}

	if latestCluster.Status.Ready == "true" {
		condition.Status = metav1.ConditionTrue
		condition.Reason = "Ready"
	}

	// 添加或更新条件
	if latestCluster.Status.Conditions == nil {
		latestCluster.Status.Conditions = []metav1.Condition{}
	}

	// 查找现有条件并更新，或添加新条件
	found := false
	for i, existingCondition := range latestCluster.Status.Conditions {
		if existingCondition.Type == condition.Type {
			latestCluster.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		latestCluster.Status.Conditions = append(latestCluster.Status.Conditions, condition)
	}

	// 限制条件数量
	if len(latestCluster.Status.Conditions) > 10 {
		latestCluster.Status.Conditions = latestCluster.Status.Conditions[len(latestCluster.Status.Conditions)-10:]
	}

	return r.Status().Update(ctx, latestCluster)
}

// configMapForCluster 创建 Cluster ConfigMap
func (r *RedisClusterReconciler) configMapForCluster(redisCluster *redisv1.RedisCluster) *corev1.ConfigMap {
	clusterConfig := map[string]string{
		"redis.conf": fmt.Sprintf(`# Redis Cluster Configuration
port 6379
bind 0.0.0.0
cluster-enabled yes
cluster-config-file nodes.conf
cluster-node-timeout %d
cluster-require-full-coverage %s
cluster-migration-barrier %d
appendonly yes
appendfsync everysec
save 900 1
save 300 10
save 60 10000
`,
			redisCluster.Spec.Config.ClusterNodeTimeout,
			redisCluster.Spec.Config.ClusterRequireFullCoverage,
			redisCluster.Spec.Config.ClusterMigrationBarrier),
	}

	// 合并用户自定义配置
	if redisCluster.Spec.Config.AdditionalConfig != nil {
		for key, value := range redisCluster.Spec.Config.AdditionalConfig {
			clusterConfig[key] = value
		}
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redisCluster.Name + "-config",
			Namespace: redisCluster.Namespace,
			Labels: map[string]string{
				"app":       "redis-cluster",
				"component": "cluster",
				"instance":  redisCluster.Name,
			},
		},
		Data: clusterConfig,
	}
}

// statefulSetForCluster 创建 Cluster StatefulSet
func (r *RedisClusterReconciler) statefulSetForCluster(redisCluster *redisv1.RedisCluster) *appsv1.StatefulSet {
	replicas := redisCluster.Spec.Masters * (1 + redisCluster.Spec.ReplicasPerMaster)
	if replicas == 0 {
		replicas = 6 // 默认值：3 masters + 3 replicas
	}

	resources := redisCluster.Spec.Resources

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "config",
			MountPath: "/usr/local/etc/redis",
		},
		{
			Name:      "data",
			MountPath: "/data",
		},
	}

	volumes := []corev1.Volume{
		{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: redisCluster.Name + "-config",
					},
				},
			},
		},
	}

	var volumeClaimTemplates []corev1.PersistentVolumeClaim
	if redisCluster.Spec.Storage.Size != "" {
		volumeClaimTemplates = []corev1.PersistentVolumeClaim{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "data",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse(redisCluster.Spec.Storage.Size),
						},
					},
					StorageClassName: &redisCluster.Spec.Storage.StorageClassName,
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
			Name:      redisCluster.Name,
			Namespace: redisCluster.Namespace,
			Labels: map[string]string{
				"app":       "redis-cluster",
				"component": "cluster",
				"instance":  redisCluster.Name,
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":       "redis-cluster",
					"component": "cluster",
					"instance":  redisCluster.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":       "redis-cluster",
						"component": "cluster",
						"instance":  redisCluster.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "redis",
							Image: redisCluster.Spec.Image,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 6379,
									Name:          "redis",
								},
								{
									ContainerPort: 16379,
									Name:          "cluster-bus",
								},
							},
							Command:      []string{"redis-server", "/usr/local/etc/redis/redis.conf"},
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
					Volumes:      volumes,
					NodeSelector: redisCluster.Spec.NodeSelector,
					Tolerations:  redisCluster.Spec.Tolerations,
					Affinity:     redisCluster.Spec.Affinity,
				},
			},
			VolumeClaimTemplates: volumeClaimTemplates,
		},
	}
}

// serviceForCluster 创建 Cluster Service
func (r *RedisClusterReconciler) serviceForCluster(redisCluster *redisv1.RedisCluster) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redisCluster.Name + "-service",
			Namespace: redisCluster.Namespace,
			Labels: map[string]string{
				"app":       "redis-cluster",
				"component": "cluster",
				"instance":  redisCluster.Name,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app":       "redis-cluster",
				"component": "cluster",
				"instance":  redisCluster.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "redis",
					Port:       6379,
					TargetPort: intstr.FromInt(6379),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "cluster-bus",
					Port:       16379,
					TargetPort: intstr.FromInt(16379),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: "None", // Headless service for StatefulSet
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *RedisClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&redisv1.RedisCluster{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Owns(&appsv1.StatefulSet{}).
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				cm := obj.(*corev1.ConfigMap)
				for _, ownerRef := range cm.OwnerReferences {
					if ownerRef.Kind == "RedisCluster" {
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
					if ownerRef.Kind == "RedisCluster" {
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
