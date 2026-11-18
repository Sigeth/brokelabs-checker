package checks

import (
	"context"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/Sigeth/brokelabs-core-go/api/v1alpha1"
	"github.com/Sigeth/brokelabs-core-go/pkg/checkerio"
)

func kubePod(ns, name string, labels map[string]any, automount bool) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]any{"name": name, "namespace": ns, "labels": labels},
		"spec":     map[string]any{"automountServiceAccountToken": automount},
	}}
}

func fakeKube(objs ...runtime.Object) KubeClients {
	return func(string) (dynamic.Interface, meta.RESTMapper, error) {
		mapper := meta.NewDefaultRESTMapper(nil)
		mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "Pod"}, meta.RESTScopeNamespace)
		gvr := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
		dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
			map[schema.GroupVersionResource]string{gvr: "PodList"}, objs...)
		return dyn, mapper, nil
	}
}

func TestKubeCEL(t *testing.T) {
	ch := &Checker{Kube: fakeKube(
		kubePod("payments", "api", map[string]any{"app": "api"}, false),
		kubePod("payments", "worker", map[string]any{"app": "worker"}, true),
		kubePod("other", "x", map[string]any{"app": "api"}, true),
	)}
	target := checkerio.Target{Runtime: v1alpha1.RuntimeVCluster, KubeconfigFile: "/kube/cluster"}
	expr := "object.spec.automountServiceAccountToken == false"
	tests := []struct {
		name    string
		sel     v1alpha1.ResourceSelector
		pass    bool
		message string
	}{
		{"namespace", v1alpha1.ResourceSelector{APIVersion: "v1", Kind: "Pod", Namespace: "payments"}, false, "payments/worker"},
		{"labels", v1alpha1.ResourceSelector{APIVersion: "v1", Kind: "Pod", Namespace: "payments", LabelSelector: "app=api"}, true, "all 1 Pod"},
		{"all namespaces", v1alpha1.ResourceSelector{APIVersion: "v1", Kind: "Pod", LabelSelector: "app=api"}, false, "other/x"},
		{"name", v1alpha1.ResourceSelector{APIVersion: "v1", Kind: "Pod", Namespace: "payments", Name: "api"}, true, ""},
		{"missing name", v1alpha1.ResourceSelector{APIVersion: "v1", Kind: "Pod", Namespace: "payments", Name: "gone"}, false, "no Pod found"},
		// A kind the cluster does not know (a CRD the student removed) selects nothing.
		{"unknown kind", v1alpha1.ResourceSelector{APIVersion: "example.com/v1", Kind: "Widget"}, false, "no Widget found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := ch.kubeCEL(context.Background(), target, "cluster", &v1alpha1.KubeCELCheck{Resource: tt.sel, All: expr})
			if err != nil {
				t.Fatal(err)
			}
			if res.Passed != tt.pass || !strings.Contains(res.Message, tt.message) {
				t.Fatalf("result = %+v, want %v containing %q", res, tt.pass, tt.message)
			}
		})
	}
	if _, err := ch.kubeCEL(context.Background(), checkerio.Target{}, "cluster", &v1alpha1.KubeCELCheck{All: "true"}); err == nil {
		t.Error("a target without kubeconfig was checked")
	}
}
