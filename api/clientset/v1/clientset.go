package v1

import (
	"github.com/notfromstatefarm/namespice/api/types/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

type V1Interface interface {
	NamespaceClasses() NamespaceClassInterface
}

type V1Client struct {
	restClient rest.Interface
}

func NewForConfig(c *rest.Config) (*V1Client, error) {
	config := *c
	config.ContentConfig.GroupVersion = &schema.GroupVersion{Group: v1.GroupName, Version: v1.GroupVersion}
	config.APIPath = "/apis"
	config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	config.UserAgent = rest.DefaultKubernetesUserAgent()

	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, err
	}

	return &V1Client{restClient: client}, nil
}

func (c *V1Client) NamespaceClasses() NamespaceClassInterface {
	return &ncClient{
		restClient: c.restClient,
	}
}
