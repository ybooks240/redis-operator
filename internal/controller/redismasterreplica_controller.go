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
)

// RedisMasterReplicaReconciler reconciles a RedisMasterReplica object
type RedisMasterReplicaReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=redis.github.com,resources=redismasterreplicas,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=redis.github.com,resources=redismasterreplicas/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=redis.github.com,resources=redismasterreplicas/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *RedisMasterReplicaReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logs := logf.FromContext(ctx)

	// 获取 RedisMasterReplica 实例
	redisMasterReplica := &redisv1.RedisMasterReplica{}
	err := r.Get(ctx, req.NamespacedName, redisMasterReplica)
	if err != nil {
		if errors.IsNotFound(err) {
			// 资源已被删除，无需处理
			logs.Info("RedisMasterReplica not found, skipping reconciliation", "name", req.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// 检查是否正在删除
	if redisMasterReplica.DeletionTimestamp != nil {
		logs.Info("RedisMasterReplica is being deleted, cleaning up resources", "name", redisMasterReplica.Name)
		if err = r.cleanupResources(ctx, req, redisMasterReplica, logs); err != nil {
			logs.Error(err, "Failed to cleanup resources")
			return ctrl.Result{}, err
		}
		// 移除 finalizer，使用重试机制避免资源版本冲突
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			// 重新获取最新的资源版本
			latestRedisMasterReplica := &redisv1.RedisMasterReplica{}
			if err := r.Get(ctx, types.NamespacedName{Name: redisMasterReplica.Name, Namespace: redisMasterReplica.Namespace}, latestRedisMasterReplica); err != nil {
				return err
			}
			controllerutil.RemoveFinalizer(latestRedisMasterReplica, redisv1.RedisMasterReplicaFinalizer)
			return r.Update(ctx, latestRedisMasterReplica)
		})
		if err != nil {
			logs.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// 初始化状态和 finalizer
	if redisMasterReplica.Status.Conditions == nil {
		logs.Info("Initializing RedisMasterReplica", "name", redisMasterReplica.Name)
		controllerutil.AddFinalizer(redisMasterReplica, redisv1.RedisMasterReplicaFinalizer)
		err = r.Update(ctx, redisMasterReplica)
		if err != nil {
			logs.Error(err, "Failed to update RedisMasterReplica finalizer")
			return ctrl.Result{}, err
		}
	}

	// 确保所有资源存在并正确配置
	err = r.ensureResources(ctx, req, redisMasterReplica, logs)
	if err != nil {
		logs.Error(err, "Failed to ensure resources")
		return ctrl.Result{}, err
	}

	// 更新状态
	err = r.updateRedisMasterReplicaStatus(ctx, redisMasterReplica)
	if err != nil {
		logs.Error(err, "Failed to update RedisMasterReplica status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: time.Second * 30}, nil
}

// cleanupResources 清理相关资源
func (r *RedisMasterReplicaReconciler) cleanupResources(ctx context.Context, req ctrl.Request, redisMasterReplica *redisv1.RedisMasterReplica, logs logr.Logger) error {
	// 删除主节点 StatefulSet
	masterSts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redisMasterReplica.Name + "-master",
			Namespace: redisMasterReplica.Namespace,
		},
	}
	if err := r.Get(ctx, types.NamespacedName{Name: masterSts.Name, Namespace: masterSts.Namespace}, masterSts); err == nil {
		controllerutil.RemoveFinalizer(masterSts, redisv1.RedisMasterReplicaFinalizer)
		if err = r.Update(ctx, masterSts); err != nil {
			logs.Error(err, "Failed to remove master StatefulSet finalizer")
			return err
		}
		if err = r.Delete(ctx, masterSts); err != nil {
			logs.Error(err, "Failed to delete master StatefulSet")
			return err
		}
	}

	// 删除从节点 StatefulSet
	replicaSts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redisMasterReplica.Name + "-replica",
			Namespace: redisMasterReplica.Namespace,
		},
	}
	if err := r.Get(ctx, types.NamespacedName{Name: replicaSts.Name, Namespace: replicaSts.Namespace}, replicaSts); err == nil {
		controllerutil.RemoveFinalizer(replicaSts, redisv1.RedisMasterReplicaFinalizer)
		if err = r.Update(ctx, replicaSts); err != nil {
			logs.Error(err, "Failed to remove replica StatefulSet finalizer")
			return err
		}
		if err = r.Delete(ctx, replicaSts); err != nil {
			logs.Error(err, "Failed to delete replica StatefulSet")
			return err
		}
	}

	// 删除 Services 和 ConfigMaps
	resourcesToDelete := []client.Object{
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: redisMasterReplica.Name + "-master-service", Namespace: redisMasterReplica.Namespace}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: redisMasterReplica.Name + "-replica-service", Namespace: redisMasterReplica.Namespace}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: redisMasterReplica.Name + "-master-config", Namespace: redisMasterReplica.Namespace}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: redisMasterReplica.Name + "-replica-config", Namespace: redisMasterReplica.Namespace}},
	}

	for _, resource := range resourcesToDelete {
		if err := r.Get(ctx, types.NamespacedName{Name: resource.GetName(), Namespace: resource.GetNamespace()}, resource); err == nil {
			controllerutil.RemoveFinalizer(resource, redisv1.RedisMasterReplicaFinalizer)
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
func (r *RedisMasterReplicaReconciler) ensureResources(ctx context.Context, req ctrl.Request, redisMasterReplica *redisv1.RedisMasterReplica, logs logr.Logger) error {
	// 确保主节点 ConfigMap
	if err := r.ensureMasterConfigMap(ctx, redisMasterReplica, logs); err != nil {
		return err
	}

	// 确保从节点 ConfigMap
	if err := r.ensureReplicaConfigMap(ctx, redisMasterReplica, logs); err != nil {
		return err
	}

	// 确保主节点 StatefulSet
	if err := r.ensureMasterStatefulSet(ctx, redisMasterReplica, logs); err != nil {
		return err
	}

	// 确保从节点 StatefulSet
	if err := r.ensureReplicaStatefulSet(ctx, redisMasterReplica, logs); err != nil {
		return err
	}

	// 确保主节点 Service
	if err := r.ensureMasterService(ctx, redisMasterReplica, logs); err != nil {
		return err
	}

	// 确保从节点 Service
	if err := r.ensureReplicaService(ctx, redisMasterReplica, logs); err != nil {
		return err
	}

	return nil
}

// ensureMasterConfigMap 确保主节点 ConfigMap 存在
func (r *RedisMasterReplicaReconciler) ensureMasterConfigMap(ctx context.Context, redisMasterReplica *redisv1.RedisMasterReplica, logs logr.Logger) error {
	configMap := &corev1.ConfigMap{}
	configMapName := redisMasterReplica.Name + "-master-config"
	err := r.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: redisMasterReplica.Namespace}, configMap)

	if errors.IsNotFound(err) {
		// 创建新的 ConfigMap
		configMap = r.configMapForMaster(redisMasterReplica)
		if err := controllerutil.SetControllerReference(redisMasterReplica, configMap, r.Scheme); err != nil {
			return err
		}
		controllerutil.AddFinalizer(configMap, redisv1.RedisMasterReplicaFinalizer)
		logs.Info("Creating master ConfigMap", "name", configMap.Name)
		return r.Create(ctx, configMap)
	} else if err != nil {
		return err
	}

	// 检查 ConfigMap 是否需要更新
	desiredConfigMap := r.configMapForMaster(redisMasterReplica)
	if !reflect.DeepEqual(configMap.Data, desiredConfigMap.Data) {
		// 设置状态为 Updating
		if err := r.setUpdatingStatus(ctx, redisMasterReplica, "Updating master ConfigMap"); err != nil {
			logs.Error(err, "Failed to set updating status")
		}

		configMap.Data = desiredConfigMap.Data
		logs.Info("Updating master ConfigMap", "name", configMap.Name)
		return r.Update(ctx, configMap)
	}

	return nil
}

// ensureReplicaConfigMap 确保从节点 ConfigMap 存在
func (r *RedisMasterReplicaReconciler) ensureReplicaConfigMap(ctx context.Context, redisMasterReplica *redisv1.RedisMasterReplica, logs logr.Logger) error {
	configMap := &corev1.ConfigMap{}
	configMapName := redisMasterReplica.Name + "-replica-config"
	err := r.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: redisMasterReplica.Namespace}, configMap)

	if errors.IsNotFound(err) {
		// 创建新的 ConfigMap
		configMap = r.configMapForReplica(redisMasterReplica)
		if err := controllerutil.SetControllerReference(redisMasterReplica, configMap, r.Scheme); err != nil {
			return err
		}
		controllerutil.AddFinalizer(configMap, redisv1.RedisMasterReplicaFinalizer)
		logs.Info("Creating replica ConfigMap", "name", configMap.Name)
		return r.Create(ctx, configMap)
	} else if err != nil {
		return err
	}

	// 检查 ConfigMap 是否需要更新
	desiredConfigMap := r.configMapForReplica(redisMasterReplica)
	if !reflect.DeepEqual(configMap.Data, desiredConfigMap.Data) {
		// 设置状态为 Updating
		if err := r.setUpdatingStatus(ctx, redisMasterReplica, "Updating replica ConfigMap"); err != nil {
			logs.Error(err, "Failed to set updating status")
		}

		configMap.Data = desiredConfigMap.Data
		logs.Info("Updating replica ConfigMap", "name", configMap.Name)
		return r.Update(ctx, configMap)
	}

	return nil
}

// ensureMasterStatefulSet 确保主节点 StatefulSet 存在
func (r *RedisMasterReplicaReconciler) ensureMasterStatefulSet(ctx context.Context, redisMasterReplica *redisv1.RedisMasterReplica, logs logr.Logger) error {
	statefulSet := &appsv1.StatefulSet{}
	statefulSetName := redisMasterReplica.Name + "-master"
	err := r.Get(ctx, types.NamespacedName{Name: statefulSetName, Namespace: redisMasterReplica.Namespace}, statefulSet)

	if errors.IsNotFound(err) {
		// 创建新的 StatefulSet
		statefulSet = r.statefulSetForMaster(redisMasterReplica)
		if err := controllerutil.SetControllerReference(redisMasterReplica, statefulSet, r.Scheme); err != nil {
			return err
		}
		controllerutil.AddFinalizer(statefulSet, redisv1.RedisMasterReplicaFinalizer)
		logs.Info("Creating master StatefulSet", "name", statefulSet.Name)
		return r.Create(ctx, statefulSet)
	} else if err != nil {
		return err
	}

	// 检查 StatefulSet 是否需要更新
	desiredStatefulSet := r.statefulSetForMaster(redisMasterReplica)
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
		if err := r.setUpdatingStatus(ctx, redisMasterReplica, "Updating master StatefulSet"); err != nil {
			logs.Error(err, "Failed to set updating status")
		}

		statefulSet.Spec.Replicas = desiredStatefulSet.Spec.Replicas
		statefulSet.Spec.Template = desiredStatefulSet.Spec.Template
		logs.Info("Updating master StatefulSet", "name", statefulSet.Name)
		return r.Update(ctx, statefulSet)
	}

	return nil
}

// ensureReplicaStatefulSet 确保从节点 StatefulSet 存在
func (r *RedisMasterReplicaReconciler) ensureReplicaStatefulSet(ctx context.Context, redisMasterReplica *redisv1.RedisMasterReplica, logs logr.Logger) error {
	statefulSet := &appsv1.StatefulSet{}
	statefulSetName := redisMasterReplica.Name + "-replica"
	err := r.Get(ctx, types.NamespacedName{Name: statefulSetName, Namespace: redisMasterReplica.Namespace}, statefulSet)

	if errors.IsNotFound(err) {
		// 创建新的 StatefulSet
		statefulSet = r.statefulSetForReplica(redisMasterReplica)
		if err := controllerutil.SetControllerReference(redisMasterReplica, statefulSet, r.Scheme); err != nil {
			return err
		}
		controllerutil.AddFinalizer(statefulSet, redisv1.RedisMasterReplicaFinalizer)
		logs.Info("Creating replica StatefulSet", "name", statefulSet.Name)
		return r.Create(ctx, statefulSet)
	} else if err != nil {
		return err
	}

	// 检查 StatefulSet 是否需要更新
	desiredStatefulSet := r.statefulSetForReplica(redisMasterReplica)
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
		if err := r.setUpdatingStatus(ctx, redisMasterReplica, "Updating replica StatefulSet"); err != nil {
			logs.Error(err, "Failed to set updating status")
		}

		statefulSet.Spec.Replicas = desiredStatefulSet.Spec.Replicas
		statefulSet.Spec.Template = desiredStatefulSet.Spec.Template
		logs.Info("Updating replica StatefulSet", "name", statefulSet.Name)
		return r.Update(ctx, statefulSet)
	}

	return nil
}

// ensureMasterService 确保主节点 Service 存在
func (r *RedisMasterReplicaReconciler) ensureMasterService(ctx context.Context, redisMasterReplica *redisv1.RedisMasterReplica, logs logr.Logger) error {
	service := &corev1.Service{}
	serviceName := redisMasterReplica.Name + "-master-service"
	err := r.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: redisMasterReplica.Namespace}, service)

	if errors.IsNotFound(err) {
		// 创建新的 Service
		service = r.serviceForMaster(redisMasterReplica)
		if err := controllerutil.SetControllerReference(redisMasterReplica, service, r.Scheme); err != nil {
			return err
		}
		controllerutil.AddFinalizer(service, redisv1.RedisMasterReplicaFinalizer)
		logs.Info("Creating master Service", "name", service.Name)
		return r.Create(ctx, service)
	} else if err != nil {
		return err
	}

	return nil
}

// ensureReplicaService 确保从节点 Service 存在
func (r *RedisMasterReplicaReconciler) ensureReplicaService(ctx context.Context, redisMasterReplica *redisv1.RedisMasterReplica, logs logr.Logger) error {
	service := &corev1.Service{}
	serviceName := redisMasterReplica.Name + "-replica-service"
	err := r.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: redisMasterReplica.Namespace}, service)

	if errors.IsNotFound(err) {
		// 创建新的 Service
		service = r.serviceForReplica(redisMasterReplica)
		if err := controllerutil.SetControllerReference(redisMasterReplica, service, r.Scheme); err != nil {
			return err
		}
		controllerutil.AddFinalizer(service, redisv1.RedisMasterReplicaFinalizer)
		logs.Info("Creating replica Service", "name", service.Name)
		return r.Create(ctx, service)
	} else if err != nil {
		return err
	}

	return nil
}

// updateRedisMasterReplicaStatus 更新 RedisMasterReplica 状态
func (r *RedisMasterReplicaReconciler) updateRedisMasterReplicaStatus(ctx context.Context, redisMasterReplica *redisv1.RedisMasterReplica) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return r.doUpdateRedisMasterReplicaStatus(ctx, redisMasterReplica)
	})
}

func (r *RedisMasterReplicaReconciler) doUpdateRedisMasterReplicaStatus(ctx context.Context, redisMasterReplica *redisv1.RedisMasterReplica) error {
	// 重新获取最新的资源版本以避免冲突
	latestMasterReplica := &redisv1.RedisMasterReplica{}
	if err := r.Get(ctx, types.NamespacedName{Name: redisMasterReplica.Name, Namespace: redisMasterReplica.Namespace}, latestMasterReplica); err != nil {
		return err
	}

	// 获取主节点 StatefulSet 状态
	masterSts := &appsv1.StatefulSet{}
	masterStsName := latestMasterReplica.Name + "-master"
	masterErr := r.Get(ctx, types.NamespacedName{Name: masterStsName, Namespace: latestMasterReplica.Namespace}, masterSts)

	// 获取从节点 StatefulSet 状态
	replicaSts := &appsv1.StatefulSet{}
	replicaStsName := latestMasterReplica.Name + "-replica"
	replicaErr := r.Get(ctx, types.NamespacedName{Name: replicaStsName, Namespace: latestMasterReplica.Namespace}, replicaSts)

	// 更新状态
	if masterErr != nil || replicaErr != nil {
		latestMasterReplica.Status.Status = string(redisv1.RedisMasterReplicaPhaseFailed)
		latestMasterReplica.Status.Ready = "False"
		latestMasterReplica.Status.LastConditionMessage = "Failed to get StatefulSets"
	} else {
		masterReady := masterSts.Status.ReadyReplicas == *masterSts.Spec.Replicas
		replicaReady := replicaSts.Status.ReadyReplicas == *replicaSts.Spec.Replicas

		if masterReady && replicaReady {
			latestMasterReplica.Status.Status = string(redisv1.RedisMasterReplicaPhaseRunning)
			latestMasterReplica.Status.Ready = "True"
			latestMasterReplica.Status.LastConditionMessage = "All master and replica nodes are ready"
		} else {
			latestMasterReplica.Status.Status = string(redisv1.RedisMasterReplicaPhasePending)
			latestMasterReplica.Status.Ready = "False"
			latestMasterReplica.Status.LastConditionMessage = "Waiting for replicas to be ready"
		}

		// 更新主节点状态
		latestMasterReplica.Status.Master.Ready = masterReady
		latestMasterReplica.Status.Master.PodName = masterStsName + "-0"
		latestMasterReplica.Status.Master.ServiceName = latestMasterReplica.Name + "-master-service"
		latestMasterReplica.Status.Master.Role = "master"

		// 更新从节点状态
		latestMasterReplica.Status.Replica.ReadyReplicas = replicaSts.Status.ReadyReplicas
		latestMasterReplica.Status.Replica.Replicas = *replicaSts.Spec.Replicas
		latestMasterReplica.Status.Replica.ServiceName = latestMasterReplica.Name + "-replica-service"
	}

	// 更新 Conditions
	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "NotReady",
		Message:            latestMasterReplica.Status.LastConditionMessage,
		LastTransitionTime: metav1.Now(),
	}

	if latestMasterReplica.Status.Ready == "true" {
		condition.Status = metav1.ConditionTrue
		condition.Reason = "Ready"
	}

	// 添加或更新条件
	if latestMasterReplica.Status.Conditions == nil {
		latestMasterReplica.Status.Conditions = []metav1.Condition{}
	}

	// 查找现有条件并更新，或添加新条件
	found := false
	for i, existingCondition := range latestMasterReplica.Status.Conditions {
		if existingCondition.Type == condition.Type {
			latestMasterReplica.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		latestMasterReplica.Status.Conditions = append(latestMasterReplica.Status.Conditions, condition)
	}

	// 限制条件数量
	if len(latestMasterReplica.Status.Conditions) > 10 {
		latestMasterReplica.Status.Conditions = latestMasterReplica.Status.Conditions[len(latestMasterReplica.Status.Conditions)-10:]
	}

	return r.Status().Update(ctx, latestMasterReplica)
}

// setUpdatingStatus 设置更新状态
func (r *RedisMasterReplicaReconciler) setUpdatingStatus(ctx context.Context, redisMasterReplica *redisv1.RedisMasterReplica, message string) error {
	redisMasterReplica.Status.Status = string(redisv1.RedisMasterReplicaPhaseUpdating)
	redisMasterReplica.Status.LastConditionMessage = message
	return r.Status().Update(ctx, redisMasterReplica)
}

// configMapForMaster 创建主节点 ConfigMap
func (r *RedisMasterReplicaReconciler) configMapForMaster(redisMasterReplica *redisv1.RedisMasterReplica) *corev1.ConfigMap {
	redisConfig := map[string]string{
		"redis.conf": `# Redis Master Configuration
port 6379
bind 0.0.0.0
save 900 1
save 300 10
save 60 10000
rdbcompression yes
rdbchecksum yes
dbfilename dump.rdb
dir /data
maxmemory-policy allkeys-lru
`,
	}

	// 合并用户自定义配置
	for key, value := range redisMasterReplica.Spec.Config {
		redisConfig[key] = value
	}
	for key, value := range redisMasterReplica.Spec.Master.Config {
		redisConfig[key] = value
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redisMasterReplica.Name + "-master-config",
			Namespace: redisMasterReplica.Namespace,
			Labels: map[string]string{
				"app":       "redis",
				"component": "master",
				"instance":  redisMasterReplica.Name,
			},
		},
		Data: redisConfig,
	}
}

// configMapForReplica 创建从节点 ConfigMap
func (r *RedisMasterReplicaReconciler) configMapForReplica(redisMasterReplica *redisv1.RedisMasterReplica) *corev1.ConfigMap {
	masterServiceName := redisMasterReplica.Name + "-master-service"
	redisConfig := map[string]string{
		"redis.conf": fmt.Sprintf(`# Redis Replica Configuration
port 6379
bind 0.0.0.0
replicaof %s 6379
replica-read-only yes
save ""
dir /data
maxmemory-policy allkeys-lru
`, masterServiceName),
	}

	// 合并用户自定义配置
	for key, value := range redisMasterReplica.Spec.Config {
		redisConfig[key] = value
	}
	for key, value := range redisMasterReplica.Spec.Replica.Config {
		redisConfig[key] = value
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redisMasterReplica.Name + "-replica-config",
			Namespace: redisMasterReplica.Namespace,
			Labels: map[string]string{
				"app":       "redis",
				"component": "replica",
				"instance":  redisMasterReplica.Name,
			},
		},
		Data: redisConfig,
	}
}

// statefulSetForMaster 创建主节点 StatefulSet
func (r *RedisMasterReplicaReconciler) statefulSetForMaster(redisMasterReplica *redisv1.RedisMasterReplica) *appsv1.StatefulSet {
	replicas := int32(1)
	resources := redisMasterReplica.Spec.Resources
	if len(redisMasterReplica.Spec.Master.Resources.Limits) > 0 || len(redisMasterReplica.Spec.Master.Resources.Requests) > 0 {
		resources = redisMasterReplica.Spec.Master.Resources
	}

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
						Name: redisMasterReplica.Name + "-master-config",
					},
				},
			},
		},
	}

	var volumeClaimTemplates []corev1.PersistentVolumeClaim
	if redisMasterReplica.Spec.Storage.Size != "" {
		volumeClaimTemplates = []corev1.PersistentVolumeClaim{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "data",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse(redisMasterReplica.Spec.Storage.Size),
						},
					},
					StorageClassName: &redisMasterReplica.Spec.Storage.StorageClassName,
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
			Name:      redisMasterReplica.Name + "-master",
			Namespace: redisMasterReplica.Namespace,
			Labels: map[string]string{
				"app":       "redis",
				"component": "master",
				"instance":  redisMasterReplica.Name,
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":       "redis",
					"component": "master",
					"instance":  redisMasterReplica.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":       "redis",
						"component": "master",
						"instance":  redisMasterReplica.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "redis",
							Image: redisMasterReplica.Spec.Image,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 6379,
									Name:          "redis",
								},
							},
							Command:      []string{"redis-server", "/usr/local/etc/redis/redis.conf"},
							VolumeMounts: volumeMounts,
							Resources:    resources,
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
					Volumes: volumes,
				},
			},
			VolumeClaimTemplates: volumeClaimTemplates,
		},
	}
}

// statefulSetForReplica 创建从节点 StatefulSet
func (r *RedisMasterReplicaReconciler) statefulSetForReplica(redisMasterReplica *redisv1.RedisMasterReplica) *appsv1.StatefulSet {
	replicas := redisMasterReplica.Spec.Replica.Replicas
	if replicas == 0 {
		replicas = 2 // 默认值
	}

	resources := redisMasterReplica.Spec.Resources
	if len(redisMasterReplica.Spec.Replica.Resources.Limits) > 0 || len(redisMasterReplica.Spec.Replica.Resources.Requests) > 0 {
		resources = redisMasterReplica.Spec.Replica.Resources
	}

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
						Name: redisMasterReplica.Name + "-replica-config",
					},
				},
			},
		},
	}

	var volumeClaimTemplates []corev1.PersistentVolumeClaim
	if redisMasterReplica.Spec.Storage.Size != "" {
		volumeClaimTemplates = []corev1.PersistentVolumeClaim{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "data",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse(redisMasterReplica.Spec.Storage.Size),
						},
					},
					StorageClassName: &redisMasterReplica.Spec.Storage.StorageClassName,
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
			Name:      redisMasterReplica.Name + "-replica",
			Namespace: redisMasterReplica.Namespace,
			Labels: map[string]string{
				"app":       "redis",
				"component": "replica",
				"instance":  redisMasterReplica.Name,
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":       "redis",
					"component": "replica",
					"instance":  redisMasterReplica.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":       "redis",
						"component": "replica",
						"instance":  redisMasterReplica.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "redis",
							Image: redisMasterReplica.Spec.Image,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 6379,
									Name:          "redis",
								},
							},
							Command:      []string{"redis-server", "/usr/local/etc/redis/redis.conf"},
							VolumeMounts: volumeMounts,
							Resources:    resources,
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
					Volumes: volumes,
				},
			},
			VolumeClaimTemplates: volumeClaimTemplates,
		},
	}
}

// serviceForMaster 创建主节点 Service
func (r *RedisMasterReplicaReconciler) serviceForMaster(redisMasterReplica *redisv1.RedisMasterReplica) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redisMasterReplica.Name + "-master-service",
			Namespace: redisMasterReplica.Namespace,
			Labels: map[string]string{
				"app":       "redis",
				"component": "master",
				"instance":  redisMasterReplica.Name,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app":       "redis",
				"component": "master",
				"instance":  redisMasterReplica.Name,
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

// serviceForReplica 创建从节点 Service
func (r *RedisMasterReplicaReconciler) serviceForReplica(redisMasterReplica *redisv1.RedisMasterReplica) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redisMasterReplica.Name + "-replica-service",
			Namespace: redisMasterReplica.Namespace,
			Labels: map[string]string{
				"app":       "redis",
				"component": "replica",
				"instance":  redisMasterReplica.Name,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app":       "redis",
				"component": "replica",
				"instance":  redisMasterReplica.Name,
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

// SetupWithManager sets up the controller with the Manager.
func (r *RedisMasterReplicaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&redisv1.RedisMasterReplica{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Owns(&appsv1.StatefulSet{}).
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				cm := obj.(*corev1.ConfigMap)
				for _, ownerRef := range cm.OwnerReferences {
					if ownerRef.Kind == "RedisMasterReplica" {
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
					if ownerRef.Kind == "RedisMasterReplica" {
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
