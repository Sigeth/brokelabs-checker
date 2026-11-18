package checks

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/Sigeth/brokelabs-core-go/api/v1alpha1"
	"github.com/Sigeth/brokelabs-core-go/pkg/checkerio"
	"github.com/Sigeth/brokelabs-core-go/pkg/kubecel"

	"github.com/Sigeth/brokelabs-checker/pkg/sdk"
)

// KubeClients connects to a cluster from a kubeconfig file.
type KubeClients func(kubeconfigFile string) (dynamic.Interface, meta.RESTMapper, error)

// Kubeconfig connects with the kubeconfig the operator mounts for a vcluster
// machine (D48).
func Kubeconfig(file string) (dynamic.Interface, meta.RESTMapper, error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", file)
	if err != nil {
		return nil, nil, err
	}
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, nil, err
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, nil, err
	}
	return dyn, restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc)), nil
}

// kubeCEL evaluates a CEL expression over the selected objects of a virtual
// cluster. A kind the cluster does not know selects no object.
func (ch *Checker) kubeCEL(ctx context.Context, target checkerio.Target, machine string, c *v1alpha1.KubeCELCheck) (sdk.Result, error) {
	if target.KubeconfigFile == "" {
		return sdk.Result{}, fmt.Errorf("kubeCEL: machine %s has no kubeconfig", machine)
	}
	if ch.Kube == nil {
		return sdk.Result{}, fmt.Errorf("kubeCEL: no Kubernetes client")
	}
	dyn, mapper, err := ch.Kube(target.KubeconfigFile)
	if err != nil {
		return sdk.Result{}, fmt.Errorf("kubeCEL: %w", err)
	}
	objects, err := selectObjects(ctx, dyn, mapper, c.Resource)
	if err != nil {
		return sdk.Result{}, fmt.Errorf("kubeCEL: %w", err)
	}
	pass, message, err := kubecel.Evaluate(c, objects)
	if err != nil {
		return sdk.Result{}, fmt.Errorf("kubeCEL: %w", err)
	}
	return sdk.Result{Passed: pass, Message: message}, nil
}

// selectObjects reads the objects a resource selector names.
func selectObjects(ctx context.Context, dyn dynamic.Interface, mapper meta.RESTMapper, sel v1alpha1.ResourceSelector) ([]kubecel.Object, error) {
	gv, err := schema.ParseGroupVersion(sel.APIVersion)
	if err != nil {
		return nil, err
	}
	mapping, err := mapper.RESTMapping(schema.GroupKind{Group: gv.Group, Kind: sel.Kind}, gv.Version)
	if meta.IsNoMatchError(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var res dynamic.ResourceInterface = dyn.Resource(mapping.Resource)
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace && sel.Namespace != "" {
		res = dyn.Resource(mapping.Resource).Namespace(sel.Namespace)
	}
	var items []unstructured.Unstructured
	if sel.Name != "" {
		obj, err := res.Get(ctx, sel.Name, metav1.GetOptions{})
		switch {
		case apierrors.IsNotFound(err):
		case err != nil:
			return nil, err
		default:
			items = append(items, *obj)
		}
	} else {
		list, err := res.List(ctx, metav1.ListOptions{LabelSelector: sel.LabelSelector})
		if err != nil {
			return nil, err
		}
		items = list.Items
	}
	objects := make([]kubecel.Object, 0, len(items))
	for _, item := range items {
		name := item.GetName()
		if ns := item.GetNamespace(); ns != "" {
			name = ns + "/" + name
		}
		objects = append(objects, kubecel.Object{Name: name, Value: item.Object})
	}
	return objects, nil
}
