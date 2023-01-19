package main

import (
	"context"
	"errors"
	spiceclient "github.com/notfromstatefarm/namespice/api/clientset/v1"
	typev1 "github.com/notfromstatefarm/namespice/api/types/v1"
	"github.com/notfromstatefarm/namespice/internal/controller"
	"k8s.io/client-go/dynamic"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"os"
	"path/filepath"
)

func main() {
	kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")

	var config *rest.Config
	var err error

	if _, err := os.Stat(kubeconfig); errors.Is(err, os.ErrNotExist) {
		// no kube config present, probably in cluster
		config, err = rest.InClusterConfig()
		if err != nil {
			panic(err.Error())
		}
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			panic(err.Error())
		}
	}

	// Add our types to the scheme so it can be decoded
	err = typev1.AddToScheme(scheme.Scheme)
	if err != nil {
		panic(err.Error())
	}

	kubeclientset, err := kubeclient.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	spiceclientset, err := spiceclient.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	dynamicclientset, err := dynamic.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	c := controller.Create(context.Background(), kubeclientset, spiceclientset, dynamicclientset)
	c.Run()
	select {}
}
