package v1

import (
	"context"
	v1 "github.com/notfromstatefarm/namespice/api/types/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

type NamespaceClassInterface interface {
	List(ctx context.Context, opts metav1.ListOptions) (*v1.NamespaceClassList, error)
	Get(ctx context.Context, name string, options metav1.GetOptions) (*v1.NamespaceClass, error)
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
}

type ncClient struct {
	restClient rest.Interface
}

func (c *ncClient) List(ctx context.Context, opts metav1.ListOptions) (*v1.NamespaceClassList, error) {
	result := v1.NamespaceClassList{}
	err := c.restClient.
		Get().
		Resource("namespaceclasses").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do(ctx).
		Into(&result)

	return &result, err
}

func (c *ncClient) Get(ctx context.Context, name string, opts metav1.GetOptions) (*v1.NamespaceClass, error) {
	result := v1.NamespaceClass{}
	err := c.restClient.
		Get().
		Resource("namespaceclasses").
		Name(name).
		VersionedParams(&opts, scheme.ParameterCodec).
		Do(ctx).
		Into(&result)

	return &result, err
}

func (c *ncClient) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	opts.Watch = true
	return c.restClient.
		Get().
		Resource("namespaceclasses").
		VersionedParams(&opts, scheme.ParameterCodec).
		Watch(ctx)
}
