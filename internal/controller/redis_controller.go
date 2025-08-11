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

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	redisv1 "github.com/ybooks240/redis-operator/api/v1"
)

// RedisReconciler reconciles a Redis object
type RedisReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=redis.github.com,resources=redis,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=redis.github.com,resources=redis/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=redis.github.com,resources=redis/finalizers,verbs=update
// +kubebuilder:rbac:groups=redis.github.com,resources=redisclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=redis.github.com,resources=redisinstances,verbs=get;list;watch
// +kubebuilder:rbac:groups=redis.github.com,resources=redismasterreplicas,verbs=get;list;watch
// +kubebuilder:rbac:groups=redis.github.com,resources=redissentinels,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *RedisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logs := logf.FromContext(ctx)

	// 获取 Redis 聚合资源
	redis := &redisv1.Redis{}
	err := r.Get(ctx, req.NamespacedName, redis)
	if err != nil {
		if errors.IsNotFound(err) {
			// 资源已被删除，无需处理
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// 根据 spec 中的类型和资源名称，获取对应的实际资源状态
	err = r.updateRedisStatus(ctx, redis)
	if err != nil {
		logs.Error(err, "Failed to update Redis status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: time.Second * 30}, nil
}

// updateRedisStatus 更新 Redis 聚合资源的状态
func (r *RedisReconciler) updateRedisStatus(ctx context.Context, redis *redisv1.Redis) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return r.doUpdateRedisStatus(ctx, redis)
	})
}

// doUpdateRedisStatus 执行实际的状态更新逻辑
func (r *RedisReconciler) doUpdateRedisStatus(ctx context.Context, redis *redisv1.Redis) error {
	// 重新获取最新的资源版本
	latestRedis := &redisv1.Redis{}
	if err := r.Get(ctx, types.NamespacedName{Name: redis.Name, Namespace: redis.Namespace}, latestRedis); err != nil {
		return fmt.Errorf("failed to get latest Redis: %w", err)
	}

	// 根据类型获取对应资源的状态
	resourceNamespace := latestRedis.Spec.ResourceNamespace
	if resourceNamespace == "" {
		resourceNamespace = latestRedis.Namespace
	}

	switch latestRedis.Spec.Type {
	case "cluster":
		return r.updateFromRedisCluster(ctx, latestRedis, resourceNamespace)
	case "instance":
		return r.updateFromRedisInstance(ctx, latestRedis, resourceNamespace)
	case "masterreplica":
		return r.updateFromRedisMasterReplica(ctx, latestRedis, resourceNamespace)
	case "sentinel":
		return r.updateFromRedisSentinel(ctx, latestRedis, resourceNamespace)
	default:
		return fmt.Errorf("unknown Redis type: %s", latestRedis.Spec.Type)
	}
}

// updateFromRedisCluster 从 RedisCluster 更新状态
func (r *RedisReconciler) updateFromRedisCluster(ctx context.Context, redis *redisv1.Redis, namespace string) error {
	cluster := &redisv1.RedisCluster{}
	err := r.Get(ctx, types.NamespacedName{Name: redis.Spec.ResourceName, Namespace: namespace}, cluster)
	if err != nil {
		if errors.IsNotFound(err) {
			redis.Status.Status = "NotFound"
			redis.Status.Ready = "False"
			redis.Status.LastConditionMessage = fmt.Sprintf("RedisCluster %s not found", redis.Spec.ResourceName)
		} else {
			return err
		}
	} else {
		redis.Status.Type = "cluster"
		redis.Status.Ready = cluster.Status.Ready
		redis.Status.Status = cluster.Status.Status
		redis.Status.LastConditionMessage = cluster.Status.LastConditionMessage
		redis.Status.Conditions = cluster.Status.Conditions
		redis.Status.ResourceStatus = &redisv1.RedisResourceStatus{
			Cluster: &redisv1.ClusterStatusInfo{
				Masters:     cluster.Spec.Masters,
				Replicas:    cluster.Spec.ReplicasPerMaster,
				NodesReady:  int32(len(cluster.Status.Nodes)),
				ServiceName: cluster.Status.ServiceName,
			},
		}
	}
	return r.Status().Update(ctx, redis)
}

// updateFromRedisInstance 从 RedisInstance 更新状态
func (r *RedisReconciler) updateFromRedisInstance(ctx context.Context, redis *redisv1.Redis, namespace string) error {
	instance := &redisv1.RedisInstance{}
	err := r.Get(ctx, types.NamespacedName{Name: redis.Spec.ResourceName, Namespace: namespace}, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			redis.Status.Status = "NotFound"
			redis.Status.Ready = "False"
			redis.Status.LastConditionMessage = fmt.Sprintf("RedisInstance %s not found", redis.Spec.ResourceName)
		} else {
			return err
		}
	} else {
		redis.Status.Type = "instance"
		redis.Status.Ready = instance.Status.Ready
		redis.Status.Status = instance.Status.Status
		redis.Status.LastConditionMessage = instance.Status.LastConditionMessage
		redis.Status.Conditions = instance.Status.Conditions
		redis.Status.ResourceStatus = &redisv1.RedisResourceStatus{
			Instance: &redisv1.InstanceStatusInfo{
				Replicas:      instance.Spec.Replicas,
				ReadyReplicas: 0, // RedisInstance 没有 ReadyReplicas 字段，需要从 StatefulSet 获取
			},
		}
	}
	return r.Status().Update(ctx, redis)
}

// updateFromRedisMasterReplica 从 RedisMasterReplica 更新状态
func (r *RedisReconciler) updateFromRedisMasterReplica(ctx context.Context, redis *redisv1.Redis, namespace string) error {
	masterReplica := &redisv1.RedisMasterReplica{}
	err := r.Get(ctx, types.NamespacedName{Name: redis.Spec.ResourceName, Namespace: namespace}, masterReplica)
	if err != nil {
		if errors.IsNotFound(err) {
			redis.Status.Status = "NotFound"
			redis.Status.Ready = "False"
			redis.Status.LastConditionMessage = fmt.Sprintf("RedisMasterReplica %s not found", redis.Spec.ResourceName)
		} else {
			return err
		}
	} else {
		redis.Status.Type = "masterreplica"
		redis.Status.Ready = masterReplica.Status.Ready
		redis.Status.Status = masterReplica.Status.Status
		redis.Status.LastConditionMessage = masterReplica.Status.LastConditionMessage
		redis.Status.Conditions = masterReplica.Status.Conditions
		redis.Status.ResourceStatus = &redisv1.RedisResourceStatus{
			MasterReplica: &redisv1.MasterReplicaStatusInfo{
				MasterReady:  masterReplica.Status.Master.Ready,
				MasterPod:    masterReplica.Status.Master.PodName,
				ReplicaCount: masterReplica.Status.Replica.Replicas,
				ReplicaReady: masterReplica.Status.Replica.ReadyReplicas,
			},
		}
	}
	return r.Status().Update(ctx, redis)
}

// updateFromRedisSentinel 从 RedisSentinel 更新状态
func (r *RedisReconciler) updateFromRedisSentinel(ctx context.Context, redis *redisv1.Redis, namespace string) error {
	sentinel := &redisv1.RedisSentinel{}
	err := r.Get(ctx, types.NamespacedName{Name: redis.Spec.ResourceName, Namespace: namespace}, sentinel)
	if err != nil {
		if errors.IsNotFound(err) {
			redis.Status.Status = "NotFound"
			redis.Status.Ready = "False"
			redis.Status.LastConditionMessage = fmt.Sprintf("RedisSentinel %s not found", redis.Spec.ResourceName)
		} else {
			return err
		}
	} else {
		redis.Status.Type = "sentinel"
		redis.Status.Ready = sentinel.Status.Ready
		redis.Status.Status = sentinel.Status.Status
		redis.Status.LastConditionMessage = sentinel.Status.LastConditionMessage
		redis.Status.Conditions = sentinel.Status.Conditions
		redis.Status.ResourceStatus = &redisv1.RedisResourceStatus{
			Sentinel: &redisv1.SentinelStatusInfo{
				SentinelCount:   sentinel.Status.Replicas,
				SentinelReady:   sentinel.Status.ReadyReplicas,
				MonitoredMaster: sentinel.Status.MonitoredMaster.Name,
				ServiceName:     sentinel.Status.ServiceName,
			},
		}
	}
	return r.Status().Update(ctx, redis)
}

// SetupWithManager sets up the controller with the Manager.
func (r *RedisReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&redisv1.Redis{}).
		Watches(&redisv1.RedisCluster{}, handler.EnqueueRequestsFromMapFunc(r.mapToRedisRequests)).
		Watches(&redisv1.RedisInstance{}, handler.EnqueueRequestsFromMapFunc(r.mapToRedisRequests)).
		Watches(&redisv1.RedisMasterReplica{}, handler.EnqueueRequestsFromMapFunc(r.mapToRedisRequests)).
		Watches(&redisv1.RedisSentinel{}, handler.EnqueueRequestsFromMapFunc(r.mapToRedisRequests)).
		Complete(r)
}

// mapToRedisRequests 将其他 Redis 资源的变化映射到对应的 Redis 聚合资源
func (r *RedisReconciler) mapToRedisRequests(ctx context.Context, obj client.Object) []reconcile.Request {
	var requests []reconcile.Request
	logs := logf.FromContext(ctx)

	// 确定资源类型
	var resourceType string
	switch obj.(type) {
	case *redisv1.RedisCluster:
		resourceType = "cluster"
	case *redisv1.RedisInstance:
		resourceType = "instance"
	case *redisv1.RedisMasterReplica:
		resourceType = "masterreplica"
	case *redisv1.RedisSentinel:
		resourceType = "sentinel"
	default:
		return requests
	}

	// 自动创建聚合视图（如果不存在）
	aggregatedViewName := fmt.Sprintf("%s-view", obj.GetName())
	redisView := &redisv1.Redis{}
	err := r.Get(ctx, types.NamespacedName{Name: aggregatedViewName, Namespace: obj.GetNamespace()}, redisView)
	if errors.IsNotFound(err) {
		// 创建新的聚合视图
		newRedisView := &redisv1.Redis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      aggregatedViewName,
				Namespace: obj.GetNamespace(),
				Labels: map[string]string{
					"redis.github.com/auto-created":  "true",
					"redis.github.com/resource-type": resourceType,
					"redis.github.com/resource-name": obj.GetName(),
				},
			},
			Spec: redisv1.RedisSpec{
				Type:              resourceType,
				ResourceName:      obj.GetName(),
				ResourceNamespace: obj.GetNamespace(),
			},
		}

		if err = r.Create(ctx, newRedisView); err != nil {
			logs.Error(err, "Failed to create auto Redis aggregated view", "name", aggregatedViewName, "namespace", obj.GetNamespace())
		} else {
			logs.Info("Auto-created Redis aggregated view", "name", aggregatedViewName, "namespace", obj.GetNamespace(), "type", resourceType)
		}

		// 添加到请求列表
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      aggregatedViewName,
				Namespace: obj.GetNamespace(),
			},
		})
	} else if err == nil {
		// 聚合视图已存在，添加到请求列表
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      redisView.Name,
				Namespace: redisView.Namespace,
			},
		})
	}

	// 获取所有手动创建的 Redis 聚合资源
	redisList := &redisv1.RedisList{}
	if err := r.List(ctx, redisList); err != nil {
		return requests
	}

	// 查找引用了当前资源的手动创建的 Redis 聚合资源
	for _, redis := range redisList.Items {
		if redis.Spec.ResourceName == obj.GetName() && redis.Spec.Type == resourceType {
			// 检查命名空间是否匹配
			resourceNamespace := redis.Spec.ResourceNamespace
			if resourceNamespace == "" {
				resourceNamespace = redis.Namespace
			}
			if resourceNamespace == obj.GetNamespace() {
				// 避免重复添加自动创建的视图
				if redis.Name != aggregatedViewName {
					requests = append(requests, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      redis.Name,
							Namespace: redis.Namespace,
						},
					})
				}
			}
		}
	}

	return requests
}
