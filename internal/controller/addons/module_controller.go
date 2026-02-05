/*
Copyright 2026 The OtterScale Authors.

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

package addons

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	sigsyaml "sigs.k8s.io/yaml"

	addonsv1alpha1 "github.com/otterscale/otterscale-operator/api/addons/v1alpha1"
)

const (
	defaultTemplatesConfigMapName      = "otterscale-modules"
	defaultTemplatesConfigMapNamespace = "otterscale-operator-system"
	templatesConfigMapDataKey          = "modules.yaml"
	defaultFluxNamespace               = "flux-system"

	envPodNamespace                = "POD_NAMESPACE"
	envTemplatesConfigMapName      = "MODULE_TEMPLATES_CONFIGMAP_NAME"
	envTemplatesConfigMapNamespace = "MODULE_TEMPLATES_CONFIGMAP_NAMESPACE"
	envDefaultFluxNamespace        = "MODULE_DEFAULT_FLUX_NAMESPACE"
)

type moduleTemplateEntry struct {
	ID       string `json:"id"`
	Manifest string `json:"manifest"`
}

// ModuleReconciler reconciles a Module object
type ModuleReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Version is used for standard labels on created resources.
	Version string

	TemplatesConfigMapName      string
	TemplatesConfigMapNamespace string
	DefaultFluxNamespace        string
}

// +kubebuilder:rbac:groups=addons.otterscale.io,resources=modules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=addons.otterscale.io,resources=modules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=addons.otterscale.io,resources=modules/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=kustomize.toolkit.fluxcd.io,resources=kustomizations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=helm.toolkit.fluxcd.io,resources=helmreleases,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ModuleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName(req.Name)
	ctx = log.IntoContext(ctx, logger)

	var m addonsv1alpha1.Module
	if err := r.Get(ctx, req.NamespacedName, &m); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	originalStatus := *m.Status.DeepCopy()

	r.applyDefaults()

	if !m.Spec.Enabled {
		res, err := r.reconcileDisabled(ctx, &m)
		if err != nil {
			return res, err
		}
		return res, r.updateStatusIfChanged(ctx, &m, originalStatus, nil)
	}

	cm, templates, err := r.loadTemplates(ctx)
	if err != nil {
		r.setCondition(&m, metav1.Condition{
			Type:    "TemplateResolved",
			Status:  metav1.ConditionFalse,
			Reason:  "TemplatesUnavailable",
			Message: err.Error(),
		})
		r.setCondition(&m, metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionFalse,
			Reason:  "TemplatesUnavailable",
			Message: err.Error(),
		})
		return ctrl.Result{RequeueAfter: time.Minute}, r.updateStatusIfChanged(ctx, &m, originalStatus, cm)
	}

	templateID := m.Name
	if m.Spec.TemplateRef != nil {
		templateID = *m.Spec.TemplateRef
	}
	entry, ok := templates[templateID]
	if !ok {
		r.setCondition(&m, metav1.Condition{
			Type:    "TemplateResolved",
			Status:  metav1.ConditionFalse,
			Reason:  "TemplateNotFound",
			Message: fmt.Sprintf("template id %q not found in ConfigMap %s/%s key %q", templateID, cm.Namespace, cm.Name, templatesConfigMapDataKey),
		})
		r.setCondition(&m, metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionFalse,
			Reason:  "TemplateNotFound",
			Message: "template not found",
		})
		return ctrl.Result{RequeueAfter: time.Minute}, r.updateStatusIfChanged(ctx, &m, originalStatus, cm)
	}

	desired, err := r.parseTemplateManifest(entry.Manifest, m.Name)
	if err != nil {
		r.setCondition(&m, metav1.Condition{
			Type:    "TemplateResolved",
			Status:  metav1.ConditionFalse,
			Reason:  "TemplateInvalid",
			Message: err.Error(),
		})
		r.setCondition(&m, metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionFalse,
			Reason:  "TemplateInvalid",
			Message: err.Error(),
		})
		return ctrl.Result{RequeueAfter: time.Minute}, r.updateStatusIfChanged(ctx, &m, originalStatus, cm)
	}

	ref, readyCond, err := r.applyFluxObject(ctx, &m, desired)
	if err != nil {
		r.setCondition(&m, metav1.Condition{
			Type:    "Applied",
			Status:  metav1.ConditionFalse,
			Reason:  "ApplyFailed",
			Message: err.Error(),
		})
		r.setCondition(&m, metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionFalse,
			Reason:  "ApplyFailed",
			Message: err.Error(),
		})
		return ctrl.Result{RequeueAfter: time.Minute}, r.updateStatusIfChanged(ctx, &m, originalStatus, cm)
	}

	m.Status.AppliedResources = []corev1.ObjectReference{ref}
	r.setCondition(&m, metav1.Condition{
		Type:    "TemplateResolved",
		Status:  metav1.ConditionTrue,
		Reason:  "Resolved",
		Message: "template resolved successfully",
	})
	r.setCondition(&m, metav1.Condition{
		Type:    "Applied",
		Status:  metav1.ConditionTrue,
		Reason:  "Reconciled",
		Message: "flux resource applied successfully",
	})
	r.setCondition(&m, metav1.Condition{
		Type:    "Disabled",
		Status:  metav1.ConditionFalse,
		Reason:  "Enabled",
		Message: "module is enabled",
	})
	if readyCond != nil {
		r.setCondition(&m, *readyCond)
	} else {
		r.setCondition(&m, metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionUnknown,
			Reason:  "Unknown",
			Message: "flux resource does not expose status.conditions",
		})
	}

	if err := r.updateStatusIfChanged(ctx, &m, originalStatus, cm); err != nil {
		return ctrl.Result{}, err
	}

	if readyCond == nil || readyCond.Status != metav1.ConditionTrue {
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ModuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.applyDefaults()

	cmPred := predicate.NewPredicateFuncs(func(obj client.Object) bool {
		return obj.GetName() == r.TemplatesConfigMapName && obj.GetNamespace() == r.TemplatesConfigMapNamespace
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&addonsv1alpha1.Module{}).
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				var list addonsv1alpha1.ModuleList
				if err := r.List(ctx, &list); err != nil {
					log.FromContext(ctx).Error(err, "failed to list modules for templates change")
					return nil
				}
				reqs := make([]reconcile.Request, 0, len(list.Items))
				for i := range list.Items {
					reqs = append(reqs, reconcile.Request{
						NamespacedName: types.NamespacedName{Name: list.Items[i].Name},
					})
				}
				return reqs
			}),
			builder.WithPredicates(cmPred),
		).
		Named("addons-module").
		Complete(r)
}

func (r *ModuleReconciler) applyDefaults() {
	if r.TemplatesConfigMapName == "" {
		if v := os.Getenv(envTemplatesConfigMapName); v != "" {
			r.TemplatesConfigMapName = v
		} else {
			r.TemplatesConfigMapName = defaultTemplatesConfigMapName
		}
	}
	if r.TemplatesConfigMapNamespace == "" {
		if v := os.Getenv(envTemplatesConfigMapNamespace); v != "" {
			r.TemplatesConfigMapNamespace = v
		} else if v := os.Getenv(envPodNamespace); v != "" {
			r.TemplatesConfigMapNamespace = v
		} else {
			r.TemplatesConfigMapNamespace = defaultTemplatesConfigMapNamespace
		}
	}
	if r.DefaultFluxNamespace == "" {
		if v := os.Getenv(envDefaultFluxNamespace); v != "" {
			r.DefaultFluxNamespace = v
		} else {
			r.DefaultFluxNamespace = defaultFluxNamespace
		}
	}
}

func (r *ModuleReconciler) loadTemplates(ctx context.Context) (*corev1.ConfigMap, map[string]moduleTemplateEntry, error) {
	cm := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Name: r.TemplatesConfigMapName, Namespace: r.TemplatesConfigMapNamespace}, cm); err != nil {
		if errors.IsNotFound(err) {
			return nil, nil, fmt.Errorf("templates ConfigMap %s/%s not found", r.TemplatesConfigMapNamespace, r.TemplatesConfigMapName)
		}
		return nil, nil, err
	}
	raw, ok := cm.Data[templatesConfigMapDataKey]
	if !ok {
		return cm, nil, fmt.Errorf("templates ConfigMap %s/%s missing key %q", cm.Namespace, cm.Name, templatesConfigMapDataKey)
	}

	var list []moduleTemplateEntry
	if err := sigsyaml.Unmarshal([]byte(raw), &list); err != nil {
		return cm, nil, fmt.Errorf("failed to parse templates key %q: %w", templatesConfigMapDataKey, err)
	}
	templates := make(map[string]moduleTemplateEntry, len(list))
	seen := make(map[string]struct{}, len(list))
	var dups []string
	for _, e := range list {
		if e.ID == "" {
			return cm, nil, fmt.Errorf("template entry has empty id")
		}
		if _, exists := seen[e.ID]; exists {
			dups = append(dups, e.ID)
			continue
		}
		seen[e.ID] = struct{}{}
		templates[e.ID] = e
	}
	if len(dups) > 0 {
		return cm, templates, fmt.Errorf("duplicate template ids in %s/%s: %v", cm.Namespace, cm.Name, dups)
	}
	return cm, templates, nil
}

func (r *ModuleReconciler) parseTemplateManifest(manifest, defaultName string) (*unstructured.Unstructured, error) {
	decoder := yamlutil.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	var obj map[string]any
	if err := decoder.Decode(&obj); err != nil {
		return nil, fmt.Errorf("failed to decode manifest: %w", err)
	}
	u := &unstructured.Unstructured{Object: obj}
	if u.GetAPIVersion() == "" || u.GetKind() == "" {
		return nil, fmt.Errorf("manifest must include apiVersion and kind")
	}
	if u.GetName() == "" {
		u.SetName(defaultName)
	}
	if u.GetNamespace() == "" {
		u.SetNamespace(r.DefaultFluxNamespace)
	}
	if u.GetNamespace() == "" {
		return nil, fmt.Errorf("manifest must be namespaced (metadata.namespace)")
	}
	if !isSupportedFluxKind(u.GetKind()) {
		return nil, fmt.Errorf("unsupported manifest kind %q (expected Kustomization or HelmRelease)", u.GetKind())
	}
	u.SetGroupVersionKind(schema.FromAPIVersionAndKind(u.GetAPIVersion(), u.GetKind()))
	return u, nil
}

func isSupportedFluxKind(kind string) bool {
	return kind == "Kustomization" || kind == "HelmRelease"
}

func (r *ModuleReconciler) applyFluxObject(ctx context.Context, m *addonsv1alpha1.Module, desired *unstructured.Unstructured) (corev1.ObjectReference, *metav1.Condition, error) {
	unstructured.RemoveNestedField(desired.Object, "status")

	labels := desired.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	for k, v := range labelsForModule(m.Name, r.Version) {
		labels[k] = v
	}
	desired.SetLabels(labels)

	current := &unstructured.Unstructured{}
	current.SetGroupVersionKind(desired.GroupVersionKind())
	current.SetName(desired.GetName())
	current.SetNamespace(desired.GetNamespace())

	if err := r.Get(ctx, client.ObjectKeyFromObject(current), current); err != nil {
		if !errors.IsNotFound(err) {
			return corev1.ObjectReference{}, nil, err
		}
		if err := ctrlutil.SetControllerReference(m, desired, r.Scheme); err != nil {
			return corev1.ObjectReference{}, nil, err
		}
		if err := r.Create(ctx, desired); err != nil {
			return corev1.ObjectReference{}, nil, err
		}
		current = desired
	} else {
		if !isOwnedBy(current.GetOwnerReferences(), m.UID) {
			return corev1.ObjectReference{}, nil, fmt.Errorf("%s %s/%s exists but is not owned by Module %s", desired.GetKind(), desired.GetNamespace(), desired.GetName(), m.Name)
		}
		rv := current.GetResourceVersion()
		current.Object = desired.Object
		current.SetGroupVersionKind(desired.GroupVersionKind())
		current.SetResourceVersion(rv)
		if err := ctrlutil.SetControllerReference(m, current, r.Scheme); err != nil {
			return corev1.ObjectReference{}, nil, err
		}
		if err := r.Update(ctx, current); err != nil {
			return corev1.ObjectReference{}, nil, err
		}
	}

	ref := corev1.ObjectReference{
		APIVersion: current.GetAPIVersion(),
		Kind:       current.GetKind(),
		Name:       current.GetName(),
		Namespace:  current.GetNamespace(),
	}
	return ref, mapReadyConditionFromFlux(current), nil
}

func mapReadyConditionFromFlux(u *unstructured.Unstructured) *metav1.Condition {
	conds, found, err := unstructured.NestedSlice(u.Object, "status", "conditions")
	if err != nil || !found {
		return nil
	}
	for _, c := range conds {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		t, _, _ := unstructured.NestedString(m, "type")
		if t != "Ready" {
			continue
		}
		statusStr, _, _ := unstructured.NestedString(m, "status")
		reason, _, _ := unstructured.NestedString(m, "reason")
		message, _, _ := unstructured.NestedString(m, "message")

		var st metav1.ConditionStatus
		switch statusStr {
		case "True":
			st = metav1.ConditionTrue
		case "False":
			st = metav1.ConditionFalse
		default:
			st = metav1.ConditionUnknown
		}
		return &metav1.Condition{
			Type:    "Ready",
			Status:  st,
			Reason:  reason,
			Message: message,
		}
	}
	return nil
}

func (r *ModuleReconciler) reconcileDisabled(ctx context.Context, m *addonsv1alpha1.Module) (ctrl.Result, error) {
	for _, ref := range m.Status.AppliedResources {
		if ref.APIVersion == "" || ref.Kind == "" || ref.Name == "" {
			continue
		}
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(schema.FromAPIVersionAndKind(ref.APIVersion, ref.Kind))
		u.SetName(ref.Name)
		u.SetNamespace(ref.Namespace)
		if err := r.Delete(ctx, u); err != nil && !errors.IsNotFound(err) && !meta.IsNoMatchError(err) {
			r.setCondition(m, metav1.Condition{
				Type:    "Applied",
				Status:  metav1.ConditionFalse,
				Reason:  "DeleteFailed",
				Message: err.Error(),
			})
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
	}

	m.Status.AppliedResources = nil
	r.setCondition(m, metav1.Condition{
		Type:    "Disabled",
		Status:  metav1.ConditionTrue,
		Reason:  "Disabled",
		Message: "module is disabled; flux resources deleted",
	})
	r.setCondition(m, metav1.Condition{
		Type:    "Applied",
		Status:  metav1.ConditionFalse,
		Reason:  "Disabled",
		Message: "module is disabled",
	})
	r.setCondition(m, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  "Disabled",
		Message: "module is disabled",
	})
	return ctrl.Result{}, nil
}

func (r *ModuleReconciler) updateStatusIfChanged(ctx context.Context, m *addonsv1alpha1.Module, original addonsv1alpha1.ModuleStatus, templatesCM *corev1.ConfigMap) error {
	m.Status.ObservedGeneration = m.Generation
	if templatesCM != nil {
		rv := templatesCM.ResourceVersion
		m.Status.TemplateResourceVersion = &rv
	}
	if equality.Semantic.DeepEqual(original, m.Status) {
		return nil
	}
	return r.Status().Update(ctx, m)
}

func (r *ModuleReconciler) setCondition(m *addonsv1alpha1.Module, c metav1.Condition) {
	if c.ObservedGeneration == 0 {
		c.ObservedGeneration = m.Generation
	}
	meta.SetStatusCondition(&m.Status.Conditions, c)
}

func labelsForModule(module, version string) map[string]string {
	labels := map[string]string{
		"app.kubernetes.io/name":       "module",
		"app.kubernetes.io/instance":   module,
		"app.kubernetes.io/component":  "module",
		"app.kubernetes.io/part-of":    "otterscale",
		"app.kubernetes.io/managed-by": "otterscale-operator",
	}
	if version != "" {
		labels["app.kubernetes.io/version"] = version
	}
	return labels
}

func isOwnedBy(refs []metav1.OwnerReference, uid types.UID) bool {
	for _, ref := range refs {
		if ref.UID == uid {
			return true
		}
	}
	return false
}
