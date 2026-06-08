// Package controller reconciles WordPressSite resources into the concrete
// Kubernetes objects (Secret, Deployment, Service, Ingress) plus a per-site
// MySQL database and user.
package controller

import (
	"context"
	"fmt"
	"time"

	wpv1 "github.com/benji/wordpress-manager-operator/api/v1alpha1"
	"github.com/benji/wordpress-manager-operator/internal/mysql"
	"github.com/benji/wordpress-manager-operator/internal/resources"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// WordPressSiteReconciler reconciles a WordPressSite object.
type WordPressSiteReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	MySQL  *mysql.Provisioner

	// DBHost / DBPort are injected into every site so WordPress can reach the
	// shared MySQL server.
	DBHost string
	DBPort string

	// DropDataOnDelete, when true, drops the per-site database when the site is
	// deleted. Defaults to false to avoid accidental data loss.
	DropDataOnDelete bool
}

// +kubebuilder:rbac:groups=wp.benji.dev,resources=wordpresssites,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=wp.benji.dev,resources=wordpresssites/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=wp.benji.dev,resources=wordpresssites/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete

func (r *WordPressSiteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	site := &wpv1.WordPressSite{}
	if err := r.Get(ctx, req.NamespacedName, site); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion.
	if !site.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, site)
	}

	// Ensure finalizer is present so we can clean up MySQL on delete.
	if controllerutil.AddFinalizer(site, wpv1.Finalizer) {
		if err := r.Update(ctx, site); err != nil {
			return ctrl.Result{}, err
		}
	}

	if site.Spec.Suspend {
		// Still reconcile the deployment (scaled to zero) but mark suspended.
		if err := r.reconcileWorkload(ctx, site); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, r.setPhase(ctx, site, wpv1.PhaseSuspended, "Site suspended")
	}

	// 1. Per-site Secret (DB password + WP salts). Idempotent / preserves values.
	secret, err := r.ensureSecret(ctx, site)
	if err != nil {
		return r.fail(ctx, site, "secret", err)
	}

	// 2. Provision MySQL database + dedicated user using the secret's password.
	password := string(secret.Data[resources.SecretKeyDBPassword])
	if err := r.MySQL.EnsureDatabase(ctx,
		resources.DatabaseName(site),
		resources.DatabaseUser(site),
		resources.DatabaseHost(site),
		password,
	); err != nil {
		l.Error(err, "database provisioning failed; will retry")
		_ = r.setPhase(ctx, site, wpv1.PhaseProvisioning, "Waiting for MySQL: "+err.Error())
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	// 3. Deployment + Service + Ingress.
	if err := r.reconcileWorkload(ctx, site); err != nil {
		return r.fail(ctx, site, "workload", err)
	}

	// 4. Status.
	site.Status.DatabaseName = resources.DatabaseName(site)
	site.Status.DatabaseUser = resources.DatabaseUser(site)
	site.Status.SecretName = resources.SecretName(site)
	site.Status.URL = resources.URL(site)
	site.Status.ObservedGeneration = site.Generation
	if err := r.setPhase(ctx, site, wpv1.PhaseReady, "Site reconciled"); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *WordPressSiteReconciler) reconcileWorkload(ctx context.Context, site *wpv1.WordPressSite) error {
	dep := resources.DesiredDeployment(site, r.DBHost, r.DBPort)
	if err := r.apply(ctx, site, dep, &appsv1.Deployment{}); err != nil {
		return err
	}
	svc := resources.DesiredService(site)
	if err := r.apply(ctx, site, svc, &corev1.Service{}); err != nil {
		return err
	}
	ing := resources.DesiredIngress(site)
	if err := r.apply(ctx, site, ing, &netv1.Ingress{}); err != nil {
		return err
	}
	return nil
}

func (r *WordPressSiteReconciler) ensureSecret(ctx context.Context, site *wpv1.WordPressSite) (*corev1.Secret, error) {
	existing := &corev1.Secret{}
	key := client.ObjectKey{Namespace: site.Namespace, Name: resources.SecretName(site)}
	err := r.Get(ctx, key, existing)
	if apierrors.IsNotFound(err) {
		existing = nil
	} else if err != nil {
		return nil, err
	}

	desired := resources.DesiredSecret(site, existing)
	if err := controllerutil.SetControllerReference(site, desired, r.Scheme); err != nil {
		return nil, err
	}
	if existing == nil {
		if err := r.Create(ctx, desired); err != nil {
			return nil, err
		}
		return desired, nil
	}
	desired.ResourceVersion = existing.ResourceVersion
	if err := r.Update(ctx, desired); err != nil {
		return nil, err
	}
	return desired, nil
}

// apply creates or updates an owned object, setting the controller reference so
// garbage collection removes it when the site is deleted.
func (r *WordPressSiteReconciler) apply(ctx context.Context, site *wpv1.WordPressSite, desired, found client.Object) error {
	if err := controllerutil.SetControllerReference(site, desired, r.Scheme); err != nil {
		return err
	}
	key := client.ObjectKeyFromObject(desired)
	err := r.Get(ctx, key, found)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	} else if err != nil {
		return err
	}
	desired.SetResourceVersion(found.GetResourceVersion())
	return r.Update(ctx, desired)
}

func (r *WordPressSiteReconciler) reconcileDelete(ctx context.Context, site *wpv1.WordPressSite) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	if controllerutil.ContainsFinalizer(site, wpv1.Finalizer) {
		if r.DropDataOnDelete {
			if err := r.MySQL.DropDatabase(ctx,
				resources.DatabaseName(site),
				resources.DatabaseUser(site),
				resources.DatabaseHost(site),
			); err != nil {
				l.Error(err, "failed to drop database; will retry")
				return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
			}
		}
		// Owned objects (Deployment/Service/Ingress/Secret) are garbage-collected
		// via owner references.
		controllerutil.RemoveFinalizer(site, wpv1.Finalizer)
		if err := r.Update(ctx, site); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *WordPressSiteReconciler) setPhase(ctx context.Context, site *wpv1.WordPressSite, phase, msg string) error {
	site.Status.Phase = phase
	meta := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             phase,
		Message:            msg,
		ObservedGeneration: site.Generation,
		LastTransitionTime: metav1.Now(),
	}
	if phase != wpv1.PhaseReady {
		meta.Status = metav1.ConditionFalse
	}
	setCondition(&site.Status.Conditions, meta)
	return r.Status().Update(ctx, site)
}

func (r *WordPressSiteReconciler) fail(ctx context.Context, site *wpv1.WordPressSite, stage string, err error) (ctrl.Result, error) {
	_ = r.setPhase(ctx, site, wpv1.PhaseError, fmt.Sprintf("%s: %v", stage, err))
	return ctrl.Result{}, err
}

func setCondition(conds *[]metav1.Condition, c metav1.Condition) {
	for i := range *conds {
		if (*conds)[i].Type == c.Type {
			if (*conds)[i].Status == c.Status {
				c.LastTransitionTime = (*conds)[i].LastTransitionTime
			}
			(*conds)[i] = c
			return
		}
	}
	*conds = append(*conds, c)
}

// SetupWithManager wires the reconciler and the objects it owns.
func (r *WordPressSiteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&wpv1.WordPressSite{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Owns(&netv1.Ingress{}).
		Complete(r)
}
