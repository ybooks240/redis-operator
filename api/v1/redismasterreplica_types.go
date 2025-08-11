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

// RedisMasterReplicaSpec defines the desired state of RedisMasterReplica
type RedisMasterReplicaSpec struct {
	// Redis image to use
	Image string `json:"image"`

	// Master configuration
	Master MasterSpec `json:"master"`

	// Replica configuration
	Replica ReplicaSpec `json:"replica"`

	// Resources defines the resource requirements for Redis
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Storage configuration
	// +optional
	Storage StorageSpec `json:"storage,omitempty"`

	// Redis configuration
	Config map[string]string `json:"config,omitempty"`

	// Security configuration
	// +optional
	Security SecuritySpec `json:"security,omitempty"`
}

// MasterSpec defines the master node configuration
type MasterSpec struct {
	// Resources for master node
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Storage for master node
	// +optional
	Storage StorageSpec `json:"storage,omitempty"`

	// Master-specific configuration
	// +optional
	Config map[string]string `json:"config,omitempty"`
}

// ReplicaSpec defines the replica nodes configuration
type ReplicaSpec struct {
	// Number of replica nodes
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=2
	Replicas int32 `json:"replicas,omitempty"`

	// Resources for replica nodes
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Storage for replica nodes
	// +optional
	Storage StorageSpec `json:"storage,omitempty"`

	// Replica-specific configuration
	// +optional
	Config map[string]string `json:"config,omitempty"`
}

// SecuritySpec defines security configuration
type SecuritySpec struct {
	// Enable authentication
	// +optional
	AuthEnabled bool `json:"authEnabled,omitempty"`

	// Password secret reference
	// +optional
	PasswordSecret *corev1.SecretKeySelector `json:"passwordSecret,omitempty"`

	// TLS configuration
	// +optional
	TLS *TLSSpec `json:"tls,omitempty"`
}

// TLSSpec defines TLS configuration
type TLSSpec struct {
	// Enable TLS
	Enabled bool `json:"enabled"`

	// Secret containing TLS certificates
	// +optional
	SecretName string `json:"secretName,omitempty"`
}

// RedisMasterReplicaStatus defines the observed state of RedisMasterReplica.
type RedisMasterReplicaStatus struct {
	// Conditions represent the latest available observations of the resource's current state
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Ready indicates whether the master-replica setup is ready
	Ready string `json:"ready,omitempty"`

	// Status represents the current phase of the master-replica setup
	Status string `json:"status,omitempty"`

	// LastConditionMessage contains the message from the last condition
	LastConditionMessage string `json:"lastConditionMessage,omitempty"`

	// Master status information
	Master MasterStatus `json:"master,omitempty"`

	// Replica status information
	Replica ReplicaStatus `json:"replica,omitempty"`
}

// MasterStatus defines the status of the master node
type MasterStatus struct {
	// Ready indicates if the master is ready
	Ready bool `json:"ready,omitempty"`

	// Pod name of the master
	PodName string `json:"podName,omitempty"`

	// Service name for the master
	ServiceName string `json:"serviceName,omitempty"`

	// Role of the node (should be "master")
	Role string `json:"role,omitempty"`
}

// ReplicaStatus defines the status of replica nodes
type ReplicaStatus struct {
	// Ready replicas count
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// Total replicas count
	Replicas int32 `json:"replicas,omitempty"`

	// List of replica pod names
	PodNames []string `json:"podNames,omitempty"`

	// Service name for replicas
	ServiceName string `json:"serviceName,omitempty"`
}

// RedisMasterReplicaPhase represents the phase of RedisMasterReplica
type RedisMasterReplicaPhase string

const (
	RedisMasterReplicaPhaseUnknown    RedisMasterReplicaPhase = "Unknown"
	RedisMasterReplicaPhaseCreating   RedisMasterReplicaPhase = "Creating"
	RedisMasterReplicaPhasePending    RedisMasterReplicaPhase = "Pending"
	RedisMasterReplicaPhaseRunning    RedisMasterReplicaPhase = "Running"
	RedisMasterReplicaPhaseFailed     RedisMasterReplicaPhase = "Failed"
	RedisMasterReplicaPhaseTerminated RedisMasterReplicaPhase = "Terminated"
	RedisMasterReplicaPhaseUpdating   RedisMasterReplicaPhase = "Updating"
)

const (
	RedisMasterReplicaFinalizer = "redis.github.com/masterreplica-finalizer"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=redisr
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.ready`,description="Ready status"
// +kubebuilder:printcolumn:name="STATUS",type=string,JSONPath=`.status.status`,description="Status of the resource"
// +kubebuilder:printcolumn:name="MASTER",type=string,JSONPath=`.status.master.podName`,description="Master pod name"
// +kubebuilder:printcolumn:name="REPLICAS",type=string,JSONPath=`.status.replica.readyReplicas`,description="Ready replicas"
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`,description="Age of the resource"
// +kubebuilder:printcolumn:name="MESSAGE",type=string,JSONPath=`.status.lastConditionMessage`,description="Message of the resource"

// RedisMasterReplica is the Schema for the redismasterreplicas API
type RedisMasterReplica struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of RedisMasterReplica
	// +required
	Spec RedisMasterReplicaSpec `json:"spec"`

	// status defines the observed state of RedisMasterReplica
	// +optional
	Status RedisMasterReplicaStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// RedisMasterReplicaList contains a list of RedisMasterReplica
type RedisMasterReplicaList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RedisMasterReplica `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RedisMasterReplica{}, &RedisMasterReplicaList{})
}
