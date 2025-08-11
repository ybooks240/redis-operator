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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RedisSpec defines the desired state of Redis (aggregated view)
type RedisSpec struct {
	// Type of Redis resource (cluster, instance, masterreplica, sentinel)
	Type string `json:"type"`

	// Name of the actual Redis resource
	ResourceName string `json:"resourceName"`

	// Namespace of the actual Redis resource
	// +optional
	ResourceNamespace string `json:"resourceNamespace,omitempty"`
}

// RedisStatus defines the observed state of Redis (aggregated view)
type RedisStatus struct {
	// Type of Redis resource
	Type string `json:"type,omitempty"`

	// Ready status
	Ready string `json:"ready,omitempty"`

	// Current status/phase
	Status string `json:"status,omitempty"`

	// Last condition message
	LastConditionMessage string `json:"lastConditionMessage,omitempty"`

	// Resource-specific status information
	ResourceStatus *RedisResourceStatus `json:"resourceStatus,omitempty"`

	// Conditions from the underlying resource
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// RedisResourceStatus contains type-specific status information
type RedisResourceStatus struct {
	// For RedisCluster
	Cluster *ClusterStatusInfo `json:"cluster,omitempty"`

	// For RedisInstance
	Instance *InstanceStatusInfo `json:"instance,omitempty"`

	// For RedisMasterReplica
	MasterReplica *MasterReplicaStatusInfo `json:"masterReplica,omitempty"`

	// For RedisSentinel
	Sentinel *SentinelStatusInfo `json:"sentinel,omitempty"`
}

// ClusterStatusInfo contains RedisCluster-specific status
type ClusterStatusInfo struct {
	Masters     int32  `json:"masters,omitempty"`
	Replicas    int32  `json:"replicas,omitempty"`
	NodesReady  int32  `json:"nodesReady,omitempty"`
	ServiceName string `json:"serviceName,omitempty"`
}

// InstanceStatusInfo contains RedisInstance-specific status
type InstanceStatusInfo struct {
	Replicas      int32 `json:"replicas,omitempty"`
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
}

// MasterReplicaStatusInfo contains RedisMasterReplica-specific status
type MasterReplicaStatusInfo struct {
	MasterReady  bool   `json:"masterReady,omitempty"`
	MasterPod    string `json:"masterPod,omitempty"`
	ReplicaCount int32  `json:"replicaCount,omitempty"`
	ReplicaReady int32  `json:"replicaReady,omitempty"`
}

// SentinelStatusInfo contains RedisSentinel-specific status
type SentinelStatusInfo struct {
	SentinelCount   int32  `json:"sentinelCount,omitempty"`
	SentinelReady   int32  `json:"sentinelReady,omitempty"`
	MonitoredMaster string `json:"monitoredMaster,omitempty"`
	ServiceName     string `json:"serviceName,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=redis
// +kubebuilder:printcolumn:name="TYPE",type=string,JSONPath=`.status.type`,description="Redis resource type"
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.ready`,description="Ready status"
// +kubebuilder:printcolumn:name="STATUS",type=string,JSONPath=`.status.status`,description="Current status"
// +kubebuilder:printcolumn:name="RESOURCE",type=string,JSONPath=`.spec.resourceName`,description="Resource name"
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`,description="Age"
// +kubebuilder:printcolumn:name="MESSAGE",type=string,JSONPath=`.status.lastConditionMessage`,description="Message",priority=1

// Redis is the Schema for the redis API (aggregated view)
type Redis struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of Redis
	// +required
	Spec RedisSpec `json:"spec"`

	// status defines the observed state of Redis
	// +optional
	Status RedisStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// RedisList contains a list of Redis
type RedisList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Redis `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Redis{}, &RedisList{})
}
