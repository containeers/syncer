/*
Copyright (c) 2025 Containeers.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	syncerv1alpha1 "github.com/containeers/syncer/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

// ConfigMapSyncReconciler reconciles a ConfigMapSync object
type ConfigMapSyncReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=syncer.containeers.com,resources=configmapsyncs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=syncer.containeers.com,resources=configmapsyncs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=syncer.containeers.com,resources=configmapsyncs/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ConfigMapSync object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *ConfigMapSyncReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Get the ConfigMapSync resource
	var configMapSync syncerv1alpha1.ConfigMapSync
	if err := r.Get(ctx, req.NamespacedName, &configMapSync); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !configMapSync.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, &configMapSync)
	}

	// Add finalizer if it doesn't exist
	if !controllerutil.ContainsFinalizer(&configMapSync, FinalizerName) {
		controllerutil.AddFinalizer(&configMapSync, FinalizerName)
		if err := r.Update(ctx, &configMapSync); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Get target namespaces
	targetNamespaces, err := r.getTargetNamespaces(ctx, &configMapSync)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Update status with selected namespaces
	configMapSync.Status.LabelSelectedNamespaces = targetNamespaces

	// Sync each ConfigMap to target namespaces
	for _, cmName := range configMapSync.Spec.ConfigMaps {
		// Get source ConfigMap
		sourceConfigMap := &corev1.ConfigMap{}
		if err := r.Get(ctx, client.ObjectKey{
			Namespace: configMapSync.Spec.SourceNamespace,
			Name:      cmName,
		}, sourceConfigMap); err != nil {
			log.Error(err, "Failed to get source ConfigMap", "name", cmName)
			r.updateStatus(&configMapSync, err)
			return ctrl.Result{RequeueAfter: time.Minute}, err
		}

		// Sync to each target namespace
		for _, targetNS := range targetNamespaces {
			if err := r.syncConfigMap(ctx, sourceConfigMap, targetNS); err != nil {
				log.Error(err, "Failed to sync ConfigMap", "name", cmName, "targetNamespace", targetNS)
				r.updateStatus(&configMapSync, err)
				return ctrl.Result{RequeueAfter: time.Minute}, err
			}
		}
	}

	// Update status on successful sync
	r.updateStatus(&configMapSync, nil)
	return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
}

func (r *ConfigMapSyncReconciler) handleDeletion(ctx context.Context, cs *syncerv1alpha1.ConfigMapSync) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(cs, FinalizerName) {
		return ctrl.Result{}, nil
	}

	// Get target namespaces
	targetNamespaces, err := r.getTargetNamespaces(ctx, cs)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Delete synced ConfigMaps from target namespaces
	for _, configMapName := range cs.Spec.ConfigMaps {
		for _, namespace := range targetNamespaces {
			if err := r.Delete(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: namespace,
				},
			}); err != nil && !apierrors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(cs, FinalizerName)
	if err := r.Update(ctx, cs); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *ConfigMapSyncReconciler) getTargetNamespaces(ctx context.Context, cs *syncerv1alpha1.ConfigMapSync) ([]string, error) {
	var namespaces []string

	// Add explicitly specified namespaces
	namespaces = append(namespaces, cs.Spec.TargetNamespaces...)

	// If label selector is specified, add matching namespaces
	if cs.Spec.TargetNamespaceSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(cs.Spec.TargetNamespaceSelector)
		if err != nil {
			return nil, err
		}

		var nsList corev1.NamespaceList
		if err := r.List(ctx, &nsList, &client.ListOptions{
			LabelSelector: selector,
		}); err != nil {
			return nil, err
		}

		for _, ns := range nsList.Items {
			namespaces = append(namespaces, ns.Name)
		}
	}

	return namespaces, nil
}

func (r *ConfigMapSyncReconciler) syncConfigMap(ctx context.Context, source *corev1.ConfigMap, targetNamespace string) error {
	target := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      source.Name,
			Namespace: targetNamespace,
		},
	}

	_, err := ctrl.CreateOrUpdate(ctx, r.Client, target, func() error {
		target.Data = source.Data
		target.BinaryData = source.BinaryData
		return nil
	})

	return err
}

func (r *ConfigMapSyncReconciler) updateStatus(cs *syncerv1alpha1.ConfigMapSync, syncErr error) {
	status := metav1.ConditionTrue
	reason := "SyncSuccessful"
	message := "Successfully synced ConfigMaps to target namespaces"

	if syncErr != nil {
		status = metav1.ConditionFalse
		reason = "SyncFailed"
		message = fmt.Sprintf("Failed to sync ConfigMaps: %v", syncErr)
	}

	// Update the Ready condition
	meta.SetStatusCondition(&cs.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})

	// Update last sync time
	cs.Status.LastSyncTime = &metav1.Time{Time: time.Now()}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ConfigMapSyncReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&syncerv1alpha1.ConfigMapSync{}).
		// Watch for changes in source ConfigMaps
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.findConfigMapSyncs),
		).
		Complete(r)
}

// findConfigMapSyncs returns the ConfigMapSync objects that reference the given ConfigMap
func (r *ConfigMapSyncReconciler) findConfigMapSyncs(ctx context.Context, obj client.Object) []reconcile.Request {
	configMap := obj.(*corev1.ConfigMap)
	var configMapSyncList syncerv1alpha1.ConfigMapSyncList
	var requests []reconcile.Request

	if err := r.List(ctx, &configMapSyncList); err != nil {
		return nil
	}

	for _, cms := range configMapSyncList.Items {
		// Check if this ConfigMap is in the source namespace
		if cms.Spec.SourceNamespace != configMap.Namespace {
			continue
		}
		// Check if this ConfigMap is being synced
		for _, name := range cms.Spec.ConfigMaps {
			if name == configMap.Name {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      cms.Name,
						Namespace: cms.Namespace,
					},
				})
				break
			}
		}
	}
	return requests
}
