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

// RedisSentinelSpec defines the desired state of RedisSentinel
type RedisSentinelSpec struct {
	// Redis image to use
	Image string `json:"image"`

	// Number of sentinel instances
	// +kubebuilder:validation:Minimum=3
	// +kubebuilder:validation:Maximum=7
	// +kubebuilder:default=3
	Replicas int32 `json:"replicas,omitempty"`

	// Resources defines the resource requirements for Sentinel
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Storage configuration for Sentinel
	// +optional
	Storage StorageSpec `json:"storage,omitempty"`

	// Sentinel configuration
	Config SentinelConfig `json:"config,omitempty"`

	// Security configuration
	// +optional
	Security SecuritySpec `json:"security,omitempty"`

	// Redis configuration for the managed Redis instances
	// +optional
	Redis RedisInstanceConfig `json:"redis,omitempty"`

	// Redis Master-Replica reference that this sentinel will monitor (optional, if not using embedded Redis)
	// +optional
	MasterReplicaRef *MasterReplicaRef `json:"masterReplicaRef,omitempty"`
}

// SentinelConfig defines sentinel-specific configuration
type SentinelConfig struct {
	// Quorum for sentinel decisions
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=2
	Quorum int32 `json:"quorum,omitempty"`

	// Down after milliseconds
	// +kubebuilder:default=30000
	DownAfterMilliseconds int32 `json:"downAfterMilliseconds,omitempty"`

	// Failover timeout
	// +kubebuilder:default=180000
	FailoverTimeout int32 `json:"failoverTimeout,omitempty"`

	// Parallel syncs
	// +kubebuilder:default=1
	ParallelSyncs int32 `json:"parallelSyncs,omitempty"`

	// Additional sentinel configuration
	// +optional
	AdditionalConfig map[string]string `json:"additionalConfig,omitempty"`
}

// RedisInstanceConfig defines configuration for embedded Redis instances
type RedisInstanceConfig struct {
	// Master node configuration
	Master RedisMasterConfig `json:"master,omitempty"`

	// Replica nodes configuration
	Replica RedisReplicaConfig `json:"replica,omitempty"`

	// Global Redis configuration
	// +optional
	Config map[string]string `json:"config,omitempty"`

	// Master name for sentinel configuration
	// +kubebuilder:default="mymaster"
	MasterName string `json:"masterName,omitempty"`
}

// RedisMasterConfig defines master node configuration
type RedisMasterConfig struct {
	// Resources for master node
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Storage configuration for master
	// +optional
	Storage StorageSpec `json:"storage,omitempty"`

	// Master-specific configuration
	// +optional
	Config map[string]string `json:"config,omitempty"`
}

// RedisReplicaConfig defines replica nodes configuration
type RedisReplicaConfig struct {
	// Number of replica instances
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=2
	Replicas int32 `json:"replicas,omitempty"`

	// Resources for replica nodes
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Storage configuration for replicas
	// +optional
	Storage StorageSpec `json:"storage,omitempty"`

	// Replica-specific configuration
	// +optional
	Config map[string]string `json:"config,omitempty"`
}

// MasterReplicaRef defines reference to a RedisMasterReplica resource
type MasterReplicaRef struct {
	// Name of the RedisMasterReplica resource
	Name string `json:"name"`

	// Namespace of the RedisMasterReplica resource (optional, defaults to same namespace)
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Master name for sentinel configuration
	// +kubebuilder:default="mymaster"
	MasterName string `json:"masterName,omitempty"`
}

// RedisSentinelStatus defines the observed state of RedisSentinel.
type RedisSentinelStatus struct {
	// Conditions represent the latest available observations of the resource's current state
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Ready indicates whether the sentinel cluster is ready
	Ready string `json:"ready,omitempty"`

	// Status represents the current phase of the sentinel cluster
	Status string `json:"status,omitempty"`

	// LastConditionMessage contains the message from the last condition
	LastConditionMessage string `json:"lastConditionMessage,omitempty"`

	// Ready sentinels count
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// Total sentinels count
	Replicas int32 `json:"replicas,omitempty"`

	// List of sentinel pod names
	SentinelPods []string `json:"sentinelPods,omitempty"`

	// Service name for sentinels
	ServiceName string `json:"serviceName,omitempty"`

	// Monitored master information
	MonitoredMaster MonitoredMasterStatus `json:"monitoredMaster,omitempty"`
}

// MonitoredMasterStatus defines the status of the monitored master
type MonitoredMasterStatus struct {
	// Name of the monitored master
	Name string `json:"name,omitempty"`

	// IP address of the master
	IP string `json:"ip,omitempty"`

	// Port of the master
	Port int32 `json:"port,omitempty"`

	// Number of replicas known to sentinels
	KnownReplicas int32 `json:"knownReplicas,omitempty"`

	// Number of sentinels monitoring this master
	KnownSentinels int32 `json:"knownSentinels,omitempty"`

	// Master status (up/down)
	Status string `json:"status,omitempty"`
}

// RedisSentinelPhase represents the phase of RedisSentinel
type RedisSentinelPhase string

const (
	RedisSentinelPhaseUnknown    RedisSentinelPhase = "Unknown"
	RedisSentinelPhaseCreating   RedisSentinelPhase = "Creating"
	RedisSentinelPhasePending    RedisSentinelPhase = "Pending"
	RedisSentinelPhaseRunning    RedisSentinelPhase = "Running"
	RedisSentinelPhaseFailed     RedisSentinelPhase = "Failed"
	RedisSentinelPhaseTerminated RedisSentinelPhase = "Terminated"
	RedisSentinelPhaseUpdating   RedisSentinelPhase = "Updating"
)

const (
	RedisSentinelFinalizer = "redis.github.com/sentinel-finalizer"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=rediss
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.ready`,description="Ready status"
// +kubebuilder:printcolumn:name="STATUS",type=string,JSONPath=`.status.status`,description="Status of the resource"
// +kubebuilder:printcolumn:name="SENTINELS",type=string,JSONPath=`.status.readyReplicas`,description="Ready sentinels"
// +kubebuilder:printcolumn:name="MASTER",type=string,JSONPath=`.status.monitoredMaster.name`,description="Monitored master"
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`,description="Age of the resource"
// +kubebuilder:printcolumn:name="MESSAGE",type=string,JSONPath=`.status.lastConditionMessage`,description="Message of the resource"

// RedisSentinel is the Schema for the redissentinels API
type RedisSentinel struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of RedisSentinel
	// +required
	Spec RedisSentinelSpec `json:"spec"`

	// status defines the observed state of RedisSentinel
	// +optional
	Status RedisSentinelStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// RedisSentinelList contains a list of RedisSentinel
type RedisSentinelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RedisSentinel `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RedisSentinel{}, &RedisSentinelList{})
}
