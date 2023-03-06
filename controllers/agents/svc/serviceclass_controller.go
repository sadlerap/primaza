/*
Copyright 2023 The Primaza Authors.

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

package svc

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/jsonpath"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/primaza/primaza/api/v1alpha1"
	"github.com/primaza/primaza/pkg/primaza/workercluster"
)

// ServiceClassReconciler reconciles a ServiceClass object
type ServiceClassReconciler struct {
	client.Client
	dynamic.Interface
	RemoteScheme *runtime.Scheme
	Mapper       meta.RESTMapper
}

//+kubebuilder:rbac:groups=primaza.io,resources=serviceclasses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=primaza.io,resources=serviceclasses/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=primaza.io,resources=serviceclasses/finalizers,verbs=update

func NewServiceClassReconciler(mgr ctrl.Manager, scheme *runtime.Scheme) *ServiceClassReconciler {
	return &ServiceClassReconciler{
		Client:       mgr.GetClient(),
		Interface:    dynamic.NewForConfigOrDie(mgr.GetConfig()),
		RemoteScheme: scheme,
		Mapper:       mgr.GetRESTMapper(),
	}
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ServiceClass object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *ServiceClassReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reconcileLog := log.FromContext(ctx)
	reconcileLog.Info("Reconciling service class", "namespace", req.Namespace, "name", req.Name)

	serviceClass := v1alpha1.ServiceClass{}
	err := r.Get(ctx, req.NamespacedName, &serviceClass)
	if err != nil {
		reconcileLog.Error(err, "Failed to retrieve ServiceClass", "namespace", req.Namespace, "name", req.Name)
		return ctrl.Result{}, nil
	}

	typemeta := metav1.TypeMeta{
		Kind:       serviceClass.Spec.Resource.Kind,
		APIVersion: serviceClass.Spec.Resource.APIVersion,
	}
	gvk := typemeta.GroupVersionKind()
	mapping, err := r.Mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		reconcileLog.Error(err, "Failed to retrieve resource type", "gvk", gvk)
		return ctrl.Result{}, nil
	}

	services, err := r.Interface.Resource(mapping.Resource).
		Namespace(serviceClass.Namespace).
		List(ctx, metav1.ListOptions{})

	if err != nil || services == nil {
		reconcileLog.Error(err, "Failed to retrieve resources", "gvr", mapping.Resource)
		return ctrl.Result{}, nil
	}

	requeue := false
	err = r.CreateRegisteredServices(ctx, &serviceClass, *services)
	if err != nil {
		reconcileLog.Error(err, "Failed to write registered services", "namespace", req.Namespace, "name", req.Name)
		requeue = true
	}
	err = r.Client.Status().Update(ctx, &serviceClass)
	if err != nil {
		requeue = true
	}
	return ctrl.Result{Requeue: requeue}, nil
}

func (r *ServiceClassReconciler) CreateRegisteredServices(ctx context.Context, serviceClass *v1alpha1.ServiceClass, services unstructured.UnstructuredList) error {
	l := log.FromContext(ctx)
	mappings := map[string]*jsonpath.JSONPath{}
	for _, mapping := range serviceClass.Spec.Resource.ServiceEndpointDefinitionMapping {
		path := jsonpath.New("")
		err := path.Parse(fmt.Sprintf("{%s}", mapping.JsonPath))
		if err != nil {
			return err
		}
		mappings[mapping.Name] = path
	}

	config, remote_namespace, err := r.getPrimazaKubeconfig(ctx, serviceClass.Namespace)
	if err != nil {
		return err
	}
	l.Info("remote cluster", "address", config.Host)

	status := workercluster.TestConnection(ctx, config)
	serviceClass.Status.Conditions = append(serviceClass.Status.Conditions, metav1.Condition{
		Type:    "Connection",
		Message: status.Message,
		Reason:  string(status.Reason),
		Status:  metav1.ConditionStatus(status.State),
	})
	if status.State == v1alpha1.ClusterEnvironmentStateOffline {
		return fmt.Errorf("Failed to connect to cluster")
	}

	remote_client, err := client.New(config, client.Options{
		Scheme: r.RemoteScheme,
		Mapper: r.Mapper,
	})
	if err != nil {
		return err
	}

	for _, data := range services.Items {
		sedMappings, err := LookupServiceEndpointDescriptor(mappings, data)
		if err != nil {
			l.Error(err, "Failed to lookup service endpoint descriptor values",
				"name", data.GetName(),
				"namespace", data.GetNamespace(),
				"gvk", data.GroupVersionKind())
		}

		rs := v1alpha1.RegisteredService{
			ObjectMeta: metav1.ObjectMeta{
				// FIXME(sadlerap): this could cause naming conflicts; we need
				// to take into account the type of resource somehow.
				Name:      data.GetName(),
				Namespace: remote_namespace,
			},
			Spec: v1alpha1.RegisteredServiceSpec{
				ServiceEndpointDefinition: sedMappings,
				ServiceClassIdentity:      serviceClass.Spec.ServiceClassIdentity,
				HealthCheck:               serviceClass.Spec.HealthCheck,
			},
		}

		if serviceClass.Spec.Constraints != nil {
			rs.Spec.Constraints = &v1alpha1.RegisteredServiceConstraints{
				Environments: serviceClass.Spec.Constraints.Environments,
			}
		}

		if err := remote_client.Create(ctx, &rs); err != nil {
			l.Error(err, "Failed to create registered service",
				"service", data.GetName(),
				"namespace", remote_namespace)
			return err
		}
	}

	return nil
}

func LookupServiceEndpointDescriptor(mappings map[string]*jsonpath.JSONPath, service unstructured.Unstructured) ([]v1alpha1.ServiceEndpointDefinitionItem, error) {
	var sedMappings []v1alpha1.ServiceEndpointDefinitionItem
	for key, jsonPath := range mappings {
		results, err := jsonPath.FindResults(service.Object)
		if err != nil {
			return nil, err
		}
		if len(results) == 1 && len(results[0]) == 1 {
			value := fmt.Sprintf("%v", results[0][0])
			sedMappings = append(sedMappings, v1alpha1.ServiceEndpointDefinitionItem{
				Name:  key,
				Value: value,
			})
		} else {
			return nil, fmt.Errorf("jsonPath lookup into resource returned multiple results: %v", results)
		}
	}

	return sedMappings, nil
}

const PRIMAZA_CONTROLLER_REFERENCE string = "primaza-kubeconfig"

func (r *ServiceClassReconciler) getPrimazaKubeconfig(ctx context.Context, namespace string) (*rest.Config, string, error) {
	s := v1.Secret{}
	k := client.ObjectKey{Namespace: namespace, Name: PRIMAZA_CONTROLLER_REFERENCE}
	if err := r.Get(ctx, k, &s); err != nil {
		return nil, "", err
	}
	if _, found := s.Data["kubeconfig"]; !found {
		return nil, "", fmt.Errorf("Field \"kubeconfig\" field in secret %s:%s does not exist", s.Name, s.Namespace)
	}

	if _, found := s.Data["namespace"]; !found {
		return nil, "", fmt.Errorf("Field \"namespace\" field in secret %s:%s does not exist", s.Name, s.Namespace)
	}

	restConfig, err := clientcmd.RESTConfigFromKubeConfig(s.Data["kubeconfig"])
	if err != nil {
		return nil, "", err
	}
	return restConfig, string(s.Data["namespace"]), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServiceClassReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ServiceClass{}).
		Complete(r)
}
