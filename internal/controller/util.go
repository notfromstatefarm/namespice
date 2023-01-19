package controller

import (
	spicev1 "github.com/notfromstatefarm/namespice/api/types/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/strings/slices"
	"strings"
)

func mapToUnstructured(ns *v1.Namespace, classes []*spicev1.NamespaceClass) []unstructured.Unstructured {
	objs := make([]unstructured.Unstructured, 0)
	for _, class := range classes {
		classCopy := spicev1.NamespaceClass{}
		class.DeepCopyInto(&classCopy)
		for _, resource := range classCopy.Resources {
			obj := unstructured.Unstructured{Object: resource}
			obj.SetNamespace(ns.Name)
			l := obj.GetLabels()
			if l == nil {
				l = make(map[string]string)
			}
			l[ObjectLabel] = class.Name
			obj.SetLabels(l)
			objs = append(objs, *obj.DeepCopy())
		}
	}
	return objs
}

func areObjectReferencesEqual(obj1 unstructured.Unstructured, obj2 unstructured.Unstructured) bool {
	obj1Kind := obj1.GetObjectKind().GroupVersionKind().String()
	obj2Kind := obj2.GetObjectKind().GroupVersionKind().String()
	return obj1Kind == obj2Kind && obj1.GetName() == obj2.GetName() && obj1.GetNamespace() == obj2.GetNamespace()
}

type ObjectDelta struct {
	CreateOrUpdate []unstructured.Unstructured
	Delete         []unstructured.Unstructured
}

func calculateObjectDelta(exists []unstructured.Unstructured, shouldExist []unstructured.Unstructured) ObjectDelta {
	toCreateOrUpdate := make([]unstructured.Unstructured, 0)
	toDelete := make([]unstructured.Unstructured, 0)

	// find what's missing
	for _, obj := range shouldExist {
		found := false
		for _, existingObj := range exists {
			if areObjectReferencesEqual(obj, existingObj) {
				found = true
				break
			}
		}
		if !found {
			toCreateOrUpdate = append(toCreateOrUpdate, obj)
		}
	}

	// find what's extraneous or needs to be updated
	for _, existingObj := range exists {
		found := false
		for _, obj := range shouldExist {
			if areObjectReferencesEqual(obj, existingObj) {
				found = true
				// TODO: figure out a way to determine if an update is required or not
				// for now we just won't support updates
			}
		}
		if !found {
			toDelete = append(toDelete, existingObj)
		}
	}

	return ObjectDelta{
		CreateOrUpdate: toCreateOrUpdate,
		Delete:         toDelete,
	}
}

func namespaceHasClass(ns *v1.Namespace, class string) bool {
	if val, ok := ns.Annotations[NamespaceAnnotation]; ok {
		classes := strings.Split(val, ",")
		return slices.Contains(classes, class)
	}
	return false
}
