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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RedisClusterSpec defines the desired state of RedisCluster
type RedisClusterSpec struct {
	// Redis image to use
	Image string `json:"image"`

	// Number of master nodes in the cluster
	// +kubebuilder:validation:Minimum=3
	// +kubebuilder:validation:Maximum=1000
	// +kubebuilder:default=3
	Masters int32 `json:"masters,omitempty"`

	// Number of replica nodes per master
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=5
	// +kubebuilder:default=1
	ReplicasPerMaster int32 `json:"replicasPerMaster,omitempty"`

	// Resources defines the resource requirements for Redis cluster nodes
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Storage configuration for cluster nodes
	// +optional
	Storage StorageSpec `json:"storage,omitempty"`

	// Redis cluster configuration
	Config ClusterConfig `json:"config,omitempty"`

	// Security configuration
	// +optional
	Security SecuritySpec `json:"security,omitempty"`

	// Node selector for pod assignment
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations for pod assignment
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Affinity for pod assignment
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`
}

// ClusterConfig defines Redis cluster-specific configuration
type ClusterConfig struct {
	// Cluster configuration timeout
	// +kubebuilder:default=15000
	ClusterConfigEpoch int32 `json:"clusterConfigEpoch,omitempty"`

	// Cluster node timeout
	// +kubebuilder:default=15000
	ClusterNodeTimeout int32 `json:"clusterNodeTimeout,omitempty"`

	// Cluster require full coverage
	// +kubebuilder:default="yes"
	ClusterRequireFullCoverage string `json:"clusterRequireFullCoverage,omitempty"`

	// Cluster migration barrier
	// +kubebuilder:default=1
	ClusterMigrationBarrier int32 `json:"clusterMigrationBarrier,omitempty"`

	// Additional Redis configuration
	// +optional
	AdditionalConfig map[string]string `json:"additionalConfig,omitempty"`
}

// RedisClusterStatus defines the observed state of RedisCluster.
type RedisClusterStatus struct {
	// Conditions represent the latest available observations of the resource's current state
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Ready indicates whether the cluster is ready
	Ready string `json:"ready,omitempty"`

	// Status represents the current phase of the cluster
	Status string `json:"status,omitempty"`

	// LastConditionMessage contains the message from the last condition
	LastConditionMessage string `json:"lastConditionMessage,omitempty"`

	// Cluster state information
	Cluster ClusterStatus `json:"cluster,omitempty"`

	// Nodes information
	Nodes []NodeStatus `json:"nodes,omitempty"`

	// Service name for the cluster
	ServiceName string `json:"serviceName,omitempty"`
}

// ClusterStatus defines the status of the Redis cluster
type ClusterStatus struct {
	// Cluster state (ok, fail, etc.)
	State string `json:"state,omitempty"`

	// Number of slots assigned
	SlotsAssigned int32 `json:"slotsAssigned,omitempty"`

	// Number of slots ok
	SlotsOk int32 `json:"slotsOk,omitempty"`

	// Number of slots pfail
	SlotsPfail int32 `json:"slotsPfail,omitempty"`

	// Number of slots fail
	SlotsFail int32 `json:"slotsFail,omitempty"`

	// Number of known nodes
	KnownNodes int32 `json:"knownNodes,omitempty"`

	// Cluster size
	Size int32 `json:"size,omitempty"`

	// Current epoch
	CurrentEpoch int32 `json:"currentEpoch,omitempty"`

	// My epoch
	MyEpoch int32 `json:"myEpoch,omitempty"`
}

// NodeStatus defines the status of a Redis cluster node
type NodeStatus struct {
	// Node ID
	ID string `json:"id,omitempty"`

	// Pod name
	PodName string `json:"podName,omitempty"`

	// IP address
	IP string `json:"ip,omitempty"`

	// Port
	Port int32 `json:"port,omitempty"`

	// Role (master/slave)
	Role string `json:"role,omitempty"`

	// Master ID (for slaves)
	MasterID string `json:"masterID,omitempty"`

	// Ping sent timestamp
	PingSent int64 `json:"pingSent,omitempty"`

	// Pong received timestamp
	PongRecv int64 `json:"pongRecv,omitempty"`

	// Config epoch
	ConfigEpoch int32 `json:"configEpoch,omitempty"`

	// Link state
	LinkState string `json:"linkState,omitempty"`

	// Slots served by this node
	Slots []string `json:"slots,omitempty"`
}

// RedisClusterPhase represents the phase of RedisCluster
type RedisClusterPhase string

const (
	RedisClusterPhaseUnknown    RedisClusterPhase = "Unknown"
	RedisClusterPhaseCreating   RedisClusterPhase = "Creating"
	RedisClusterPhasePending    RedisClusterPhase = "Pending"
	RedisClusterPhaseRunning    RedisClusterPhase = "Running"
	RedisClusterPhaseFailed     RedisClusterPhase = "Failed"
	RedisClusterPhaseTerminated RedisClusterPhase = "Terminated"
	RedisClusterPhaseUpdating   RedisClusterPhase = "Updating"
	RedisClusterPhaseScaling    RedisClusterPhase = "Scaling"
)

const (
	RedisClusterFinalizer = "redis.github.com/cluster-finalizer"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=redisc
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.ready`,description="Ready status"
// +kubebuilder:printcolumn:name="STATUS",type=string,JSONPath=`.status.status`,description="Status of the resource"
// +kubebuilder:printcolumn:name="MASTERS",type=integer,JSONPath=`.spec.masters`,description="Number of masters"
// +kubebuilder:printcolumn:name="NODES",type=integer,JSONPath=`.status.cluster.knownNodes`,description="Total nodes"
// +kubebuilder:printcolumn:name="SLOTS",type=string,JSONPath=`.status.cluster.slotsOk`,description="Slots OK"
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`,description="Age of the resource"
// +kubebuilder:printcolumn:name="MESSAGE",type=string,JSONPath=`.status.lastConditionMessage`,description="Message of the resource"

// RedisCluster is the Schema for the redisclusters API
type RedisCluster struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of RedisCluster
	// +required
	Spec RedisClusterSpec `json:"spec"`

	// status defines the observed state of RedisCluster
	// +optional
	Status RedisClusterStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// RedisClusterList contains a list of RedisCluster
type RedisClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RedisCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RedisCluster{}, &RedisClusterList{})
}
