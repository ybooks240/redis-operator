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

// RedisInstanceSpec defines the desired state of RedisInstance
type RedisInstanceSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// foo is an example field of RedisInstance. Edit redisinstance_types.go to remove/update
	Image string `json:"image"`

	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// Resources defines the resource requirements for Redis
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// +optional
	Storage StorageSpec `json:"storage,omitempty"`

	Config map[string]string `json:"config,omitempty"`
}

type StorageSpec struct {
	// +kubebuilder:validation:Required
	Size string `json:"size,omitempty"`

	// +kubebuilder:validation:Required,default="standard"
	StorageClassName string `json:"storageClassName,omitempty"`
}

// RedisInstanceStatus defines the observed state of RedisInstance.

type RedisInstanceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// +optional
	// ObservedGeneration 字段已移除，使用 Conditions 中的 ObservedGeneration 代替
	Conditions           []metav1.Condition `json:"conditions,omitempty"`
	Ready                string             `json:"ready,omitempty"`
	Status               string             `json:"status,omitempty"`
	LastConditionMessage string             `json:"lastConditionMessage,omitempty"`
}

type RedisPhase string

const (
	RedisPhaseUnknown    RedisPhase = "Unknown"
	RedisPhaseCreating   RedisPhase = "Creating"
	RedisPhasePending    RedisPhase = "Pending"
	RedisPhaseRunning    RedisPhase = "Running"
	RedisPhaseFailed     RedisPhase = "Failed"
	RedisPhaseTerminated RedisPhase = "Terminated"
	RedisPhaseUpdating   RedisPhase = "Updating"
)

const (
	RedisInstanceFinalizer = "redis.github.com/finalizer"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.ready`,description="Ready status"
// +kubebuilder:printcolumn:name="STATUS",type=string,JSONPath=`.status.status`,description="Status of the resource"
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`,description="Age of the resource"
// +kubebuilder:printcolumn:name="MESSAGE",type=string,JSONPath=`.status.lastConditionMessage`,description="Message of the resource"
// +kubebuilder:printcolumn:name="CREATE-DATE",type=string,JSONPath=`.metadata.creationTimestamp`,description="Creation time of the resource",priority=1

// RedisInstance is the Schema for the redisinstances API
type RedisInstance struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of RedisInstance
	// +required
	Spec RedisInstanceSpec `json:"spec"`

	// status defines the observed state of RedisInstance
	// +optional
	Status RedisInstanceStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true
// RedisInstanceList contains a list of RedisInstance
type RedisInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RedisInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RedisInstance{}, &RedisInstanceList{})
}
