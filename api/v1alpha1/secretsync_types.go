/*
Copyright (c) 2025 Containeers.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SecretSyncSpec defines the desired state of SecretSync
type SecretSyncSpec struct {
	// SourceNamespace is the namespace from which to sync Secrets
	// +kubebuilder:validation:Required
	SourceNamespace string `json:"sourceNamespace"`

	// TargetNamespaces is an optional list of namespaces to sync Secrets to
	// +optional
	TargetNamespaces []string `json:"targetNamespaces,omitempty"`

	// TargetNamespaceSelector allows selecting namespaces by their labels
	// +optional
	TargetNamespaceSelector *metav1.LabelSelector `json:"targetNamespaceSelector,omitempty"`

	// Secrets is a list of Secret names to sync
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Secrets []string `json:"secrets"`
}

// SecretSyncStatus defines the observed state of SecretSync
type SecretSyncStatus struct {
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

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Source",type="string",JSONPath=".spec.sourceNamespace"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// SecretSync is the Schema for the secretsyncs API
type SecretSync struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SecretSyncSpec   `json:"spec,omitempty"`
	Status SecretSyncStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SecretSyncList contains a list of SecretSync
type SecretSyncList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SecretSync `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SecretSync{}, &SecretSyncList{})
}
