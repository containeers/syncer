/*
Copyright (c) 2025 Containeers.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ConfigMapSyncSpec defines the desired state of ConfigMapSync
type ConfigMapSyncSpec struct {
	// SourceNamespace is the namespace from which to sync ConfigMaps
	// +kubebuilder:validation:Required
	SourceNamespace string `json:"sourceNamespace"`

	// TargetNamespaces is an optional list of namespaces to sync ConfigMaps to
	// +optional
	TargetNamespaces []string `json:"targetNamespaces,omitempty"`

	// TargetNamespaceSelector allows selecting namespaces by their labels
	// +optional
	TargetNamespaceSelector *metav1.LabelSelector `json:"targetNamespaceSelector,omitempty"`

	// ConfigMaps is a list of ConfigMap names to sync
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	ConfigMaps []string `json:"configMaps"`
}

// ConfigMapSyncStatus defines the observed state of ConfigMapSync
type ConfigMapSyncStatus struct {
	// Conditions represent the latest available observations of the sync state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// TargetNamespaces contains the sync status for each target namespace
	// +optional
	TargetNamespaces []NamespaceSyncStatus `json:"targetNamespaces,omitempty"`

	// LabelSelectedNamespaces lists all namespaces selected by the label selector
	// +optional
	LabelSelectedNamespaces []string `json:"labelSelectedNamespaces,omitempty"`

	// LastSyncTime is the last time the resources were synced
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`
}

// NamespaceSyncStatus contains sync status for a specific namespace
type NamespaceSyncStatus struct {
	// Name of the target namespace
	Name string `json:"name"`

	// SyncStatus indicates the current sync status for this namespace
	SyncStatus string `json:"syncStatus"`

	// LastSyncTime is the last time resources were synced to this namespace
	LastSyncTime metav1.Time `json:"lastSyncTime"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Source",type="string",JSONPath=".spec.sourceNamespace"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ConfigMapSync is the Schema for the configmapsyncs API
type ConfigMapSync struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConfigMapSyncSpec   `json:"spec,omitempty"`
	Status ConfigMapSyncStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ConfigMapSyncList contains a list of ConfigMapSync
type ConfigMapSyncList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ConfigMapSync `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ConfigMapSync{}, &ConfigMapSyncList{})
}
