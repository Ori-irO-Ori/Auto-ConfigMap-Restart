/*
Copyright 2026.

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

package controller

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/Ori-irO-Ori/Auto-ConfigMap-Restart/api/v1alpha1"
)

// ConfigWatcherReconciler reconciles a ConfigWatcher object
type ConfigWatcherReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=apps.example.com,resources=configwatchers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.example.com,resources=configwatchers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.example.com,resources=configwatchers/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch

func (r *ConfigWatcherReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cw appsv1alpha1.ConfigWatcher
	if err := r.Get(ctx, req.NamespacedName, &cw); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	var cm corev1.ConfigMap
	cmKey := types.NamespacedName{Name: cw.Spec.ConfigMapName, Namespace: cw.Namespace}
	if err := r.Get(ctx, cmKey, &cm); err != nil {
		if errors.IsNotFound(err) {
			return r.setStatus(ctx, &cw, "", fmt.Sprintf("ConfigMap %q not found", cw.Spec.ConfigMapName))
		}
		return ctrl.Result{}, err
	}

	currentRV := cm.ResourceVersion
	if currentRV == cw.Status.LastSyncedResourceVersion {
		return ctrl.Result{}, nil
	}

	logger.Info("ConfigMap changed, restarting deployments",
		"configmap", cw.Spec.ConfigMapName,
		"oldRV", cw.Status.LastSyncedResourceVersion,
		"newRV", currentRV,
	)

	// Append ConfigMap ResourceVersion to the restart marker so each change produces a unique value.
	restartMarker := fmt.Sprintf("%s-rv%s", time.Now().UTC().Format(time.RFC3339), currentRV)
	var failedDeploys []string

	for _, deployName := range cw.Spec.Deployments {
		var deploy appsv1.Deployment
		deployKey := types.NamespacedName{Name: deployName, Namespace: cw.Namespace}
		if err := r.Get(ctx, deployKey, &deploy); err != nil {
			logger.Error(err, "Failed to get Deployment", "deployment", deployName)
			failedDeploys = append(failedDeploys, deployName)
			continue
		}

		patch := client.MergeFrom(deploy.DeepCopy())
		if deploy.Spec.Template.Annotations == nil {
			deploy.Spec.Template.Annotations = map[string]string{}
		}
		deploy.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] = restartMarker

		if err := r.Patch(ctx, &deploy, patch); err != nil {
			logger.Error(err, "Failed to patch Deployment", "deployment", deployName)
			failedDeploys = append(failedDeploys, deployName)
		}
	}

	msg := fmt.Sprintf("Restarted %d deployment(s)", len(cw.Spec.Deployments)-len(failedDeploys))
	if len(failedDeploys) > 0 {
		msg += fmt.Sprintf(", failed: %v", failedDeploys)
	}
	return r.setStatus(ctx, &cw, currentRV, msg)
}

func (r *ConfigWatcherReconciler) setStatus(ctx context.Context, cw *appsv1alpha1.ConfigWatcher, rv, msg string) (ctrl.Result, error) {
	cw.Status.LastSyncedResourceVersion = rv
	cw.Status.Message = msg
	if rv != "" {
		cw.Status.LastRestartedAt = metav1.Now()
	}
	if err := r.Status().Update(ctx, cw); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ConfigWatcherReconciler) SetupWithManager(mgr ctrl.Manager) error {
	mapToConfigWatcher := func(ctx context.Context, obj client.Object) []reconcile.Request {
		cm := obj.(*corev1.ConfigMap)
		var cwList appsv1alpha1.ConfigWatcherList
		if err := mgr.GetClient().List(ctx, &cwList, client.InNamespace(cm.Namespace)); err != nil {
			return nil
		}
		var requests []reconcile.Request
		for _, cw := range cwList.Items {
			if cw.Spec.ConfigMapName == cm.Name {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      cw.Name,
						Namespace: cw.Namespace,
					},
				})
			}
		}
		return requests
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1alpha1.ConfigWatcher{}).
		Watches(&corev1.ConfigMap{}, handler.EnqueueRequestsFromMapFunc(mapToConfigWatcher)).
		Named("configwatcher").
		Complete(r)
}
