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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	syncerv1alpha1 "github.com/containeers/syncer/api/v1alpha1"
)

// SecretSyncReconciler reconciles a SecretSync object
type SecretSyncReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=syncer.containeers.com,resources=secretsyncs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=syncer.containeers.com,resources=secretsyncs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=syncer.containeers.com,resources=secretsyncs/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the SecretSync object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *SecretSyncReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Get the SecretSync resource
	var secretSync syncerv1alpha1.SecretSync
	if err := r.Get(ctx, req.NamespacedName, &secretSync); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !secretSync.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, &secretSync)
	}

	// Add finalizer if it doesn't exist
	if !controllerutil.ContainsFinalizer(&secretSync, FinalizerName) {
		controllerutil.AddFinalizer(&secretSync, FinalizerName)
		if err := r.Update(ctx, &secretSync); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Get target namespaces
	targetNamespaces, err := r.getTargetNamespaces(ctx, &secretSync)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Update status with selected namespaces
	secretSync.Status.LabelSelectedNamespaces = targetNamespaces

	// Sync each Secret to target namespaces
	for _, secretName := range secretSync.Spec.Secrets {
		// Get source Secret
		sourceSecret := &corev1.Secret{}
		if err := r.Get(ctx, client.ObjectKey{
			Namespace: secretSync.Spec.SourceNamespace,
			Name:      secretName,
		}, sourceSecret); err != nil {
			log.Error(err, "Failed to get source Secret", "name", secretName)
			r.updateStatus(&secretSync, err)
			return ctrl.Result{RequeueAfter: time.Minute}, err
		}

		// Sync to each target namespace
		for _, targetNS := range targetNamespaces {
			if err := r.syncSecret(ctx, sourceSecret, targetNS); err != nil {
				log.Error(err, "Failed to sync Secret", "name", secretName, "targetNamespace", targetNS)
				r.updateStatus(&secretSync, err)
				return ctrl.Result{RequeueAfter: time.Minute}, err
			}
		}
	}

	// Update status on successful sync
	r.updateStatus(&secretSync, nil)
	return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
}

func (r *SecretSyncReconciler) getTargetNamespaces(ctx context.Context, ss *syncerv1alpha1.SecretSync) ([]string, error) {
	var namespaces []string

	// Add explicitly specified namespaces
	namespaces = append(namespaces, ss.Spec.TargetNamespaces...)

	// If label selector is specified, add matching namespaces
	if ss.Spec.TargetNamespaceSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(ss.Spec.TargetNamespaceSelector)
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

func (r *SecretSyncReconciler) syncSecret(ctx context.Context, source *corev1.Secret, targetNamespace string) error {
	target := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      source.Name,
			Namespace: targetNamespace,
		},
	}

	_, err := ctrl.CreateOrUpdate(ctx, r.Client, target, func() error {
		// Copy secret type
		target.Type = source.Type
		// Copy secret data
		target.Data = make(map[string][]byte)
		for k, v := range source.Data {
			target.Data[k] = v
		}
		// Copy string data if present
		if source.StringData != nil {
			target.StringData = make(map[string]string)
			for k, v := range source.StringData {
				target.StringData[k] = v
			}
		}
		return nil
	})

	return err
}

func (r *SecretSyncReconciler) updateStatus(ss *syncerv1alpha1.SecretSync, syncErr error) {
	status := metav1.ConditionTrue
	reason := "SyncSuccessful"
	message := "Successfully synced Secrets to target namespaces"

	if syncErr != nil {
		status = metav1.ConditionFalse
		reason = "SyncFailed"
		message = fmt.Sprintf("Failed to sync Secrets: %v", syncErr)
	}

	// Update the Ready condition
	meta.SetStatusCondition(&ss.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})

	// Update last sync time
	ss.Status.LastSyncTime = &metav1.Time{Time: time.Now()}
}

// SetupWithManager sets up the controller with the Manager.
func (r *SecretSyncReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&syncerv1alpha1.SecretSync{}).
		// Watch for changes in source Secrets
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findSecretSyncs),
		).
		Complete(r)
}

// findSecretSyncs returns the SecretSync objects that reference the given Secret
func (r *SecretSyncReconciler) findSecretSyncs(ctx context.Context, obj client.Object) []reconcile.Request {
	secret := obj.(*corev1.Secret)
	var secretSyncList syncerv1alpha1.SecretSyncList
	var requests []reconcile.Request

	if err := r.List(ctx, &secretSyncList); err != nil {
		return nil
	}

	for _, ss := range secretSyncList.Items {
		// Check if this Secret is in the source namespace
		if ss.Spec.SourceNamespace != secret.Namespace {
			continue
		}
		// Check if this Secret is being synced
		for _, name := range ss.Spec.Secrets {
			if name == secret.Name {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      ss.Name,
						Namespace: ss.Namespace,
					},
				})
				break
			}
		}
	}
	return requests
}

// Add deletion handler
func (r *SecretSyncReconciler) handleDeletion(ctx context.Context, ss *syncerv1alpha1.SecretSync) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(ss, FinalizerName) {
		return ctrl.Result{}, nil
	}

	// Get target namespaces
	targetNamespaces, err := r.getTargetNamespaces(ctx, ss)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Delete synced Secrets from target namespaces
	for _, secretName := range ss.Spec.Secrets {
		for _, namespace := range targetNamespaces {
			if err := r.Delete(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
			}); err != nil && !apierrors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(ss, FinalizerName)
	if err := r.Update(ctx, ss); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
