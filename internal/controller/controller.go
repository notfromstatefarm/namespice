package controller

import (
	"context"
	spiceclient "github.com/notfromstatefarm/namespice/api/clientset/v1"
	spicev1 "github.com/notfromstatefarm/namespice/api/types/v1"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/cache"
	"strings"
	"time"
)

const NamespaceAnnotation = "namespice.io/class"
const ObjectLabel = "namespice.io/class"

type Controller struct {
	kubeclient             *kubeclient.Clientset
	spiceclient            *spiceclient.V1Client
	dynamicclient          *dynamic.DynamicClient
	namespaceStore         cache.Store
	namespaceController    cache.Controller
	classStore             cache.Store
	classController        cache.Controller
	ctx                    context.Context
	hasInitiallyReconciled bool
}

func (c *Controller) mapToUnstructured(ns *v1.Namespace, classes []*spicev1.NamespaceClass) []unstructured.Unstructured {
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

func (c *Controller) executeDelta(delta ObjectDelta) {

	groupResources, err := restmapper.GetAPIGroupResources(c.kubeclient)
	if err != nil {
		log.WithError(err).Errorln("failed to get api group resources")
		return
	}
	mapper := restmapper.NewDiscoveryRESTMapper(groupResources)

	for _, obj := range delta.CreateOrUpdate {
		logger := log.WithFields(log.Fields{
			"namespace": obj.GetNamespace(),
			"name":      obj.GetName(),
			"gvk":       obj.GetObjectKind().GroupVersionKind().String(),
			"class":     obj.GetLabels()[ObjectLabel],
		})

		mapping, err := mapper.RESTMapping(obj.GroupVersionKind().GroupKind())
		if err != nil {
			logger.WithError(err).Errorln("failed to get resource mapping")
			continue
		}
		_, err = c.dynamicclient.Resource(mapping.Resource).Namespace(obj.GetNamespace()).Create(c.ctx, &obj, metav1.CreateOptions{})
		if err != nil {
			if statusError, isStatus := err.(*errors.StatusError); isStatus && statusError.ErrStatus.Reason == metav1.StatusReasonAlreadyExists {
				_, err = c.dynamicclient.Resource(mapping.Resource).Namespace(obj.GetNamespace()).Update(c.ctx, &obj, metav1.UpdateOptions{})
				if err != nil {
					logger.WithError(err).Errorln("failed to update resource")
				} else {
					logger.Infoln("updated resource")
				}
			} else {
				logger.WithError(err).Errorln("failed to create resource")
			}
		} else {
			logger.Infoln("created resource")
		}
	}

	for _, obj := range delta.Delete {
		logger := log.WithFields(log.Fields{
			"namespace": obj.GetNamespace(),
			"name":      obj.GetName(),
			"gvk":       obj.GetObjectKind().GroupVersionKind().String(),
			"class":     obj.GetLabels()[ObjectLabel],
		})

		mapping, err := mapper.RESTMapping(obj.GroupVersionKind().GroupKind())
		if err != nil {
			logger.WithError(err).Errorln("failed to get resource mapping")
			continue
		}
		err = c.dynamicclient.Resource(mapping.Resource).Namespace(obj.GetNamespace()).Delete(c.ctx, obj.GetName(), metav1.DeleteOptions{})
		if err != nil {
			logger.WithError(err).Errorln("failed to delete resource")
		} else {
			logger.Infoln("deleted resource")
		}
	}

}

func (c *Controller) cleanup() {
	// in case something was changed or deleted while the controller wasn't running, this will fully reconcile everything
	// and clean up any orphans by querying all available Kubernetes API groups. Obviously this is expensive, so it's only
	// done on initial start, after both caches have synced

	log.Infoln("starting cleanup")
	exists := c.getExistingObjects()
	log.Infof("found %d managed resources", len(exists))

	shouldExist := make([]unstructured.Unstructured, 0)

	for _, nsObj := range c.namespaceStore.List() {
		ns := nsObj.(*v1.Namespace)
		classes := c.getNamespaceClasses(ns, "")
		objs := c.mapToUnstructured(ns, classes)
		shouldExist = append(shouldExist, objs...)
	}

	delta := calculateObjectDelta(exists, shouldExist)
	c.executeDelta(delta)

	c.hasInitiallyReconciled = true

	log.Infoln("cleanup complete")
}

func (c *Controller) reconcile(ns *v1.Namespace, oldClasses []*spicev1.NamespaceClass, newClasses []*spicev1.NamespaceClass) {
	oldUnstructured := c.mapToUnstructured(ns, oldClasses)
	newUnstructured := c.mapToUnstructured(ns, newClasses)
	delta := calculateObjectDelta(oldUnstructured, newUnstructured)
	c.executeDelta(delta)
}

func (c *Controller) getExistingObjects() []unstructured.Unstructured {
	// this is very API heavy and should only be used on an initial cleanup/reconcile after starting up
	objs := make([]unstructured.Unstructured, 0)

	_, resources, err := c.kubeclient.ServerGroupsAndResources()
	if err != nil {
		log.WithError(err).Errorln("failed to get server groups and resources")
	}

	gvrs, err := discovery.GroupVersionResources(resources)
	if err != nil {
		log.WithError(err).Errorln("failed to get group version resources")
	}

	// build a label selector that finds any objects belonging to us
	req, _ := labels.NewRequirement(ObjectLabel, selection.Exists, nil)
	selector := labels.NewSelector().Add(*req)

	for gvr, _ := range gvrs {
		logger := log.WithFields(log.Fields{
			"group":    gvr.Group,
			"version":  gvr.Version,
			"resource": gvr.Resource,
		})
		logger.Debugf("requesting list")
		list, err := c.dynamicclient.Resource(gvr).List(c.ctx, metav1.ListOptions{LabelSelector: selector.String()})
		if err != nil {
			if statusError, isStatus := err.(*errors.StatusError); isStatus {
				if statusError.ErrStatus.Reason == metav1.StatusReasonNotFound || statusError.ErrStatus.Reason == metav1.StatusReasonMethodNotAllowed {
					continue
				}
			}
			logger.WithError(err).Errorln("failed to list resources")
			continue
		}
		logger.Debugf("retrieved %d resources", len(list.Items))
		for _, obj := range list.Items {
			objs = append(objs, obj)
		}
	}

	return objs
}

func (c *Controller) getNamespacesWithClass(class string) []*v1.Namespace {
	namespaces := make([]*v1.Namespace, 0)
	for _, nsObj := range c.namespaceStore.List() {
		ns := nsObj.(*v1.Namespace)
		if namespaceHasClass(ns, class) {
			namespaces = append(namespaces, ns)
		}
	}
	return namespaces
}

func (c *Controller) getNamespaceClasses(ns *v1.Namespace, exclude string) []*spicev1.NamespaceClass {
	classes := make([]*spicev1.NamespaceClass, 0)
	if val, ok := ns.Annotations[NamespaceAnnotation]; ok {
		classNames := strings.Split(val, ",")
		for _, className := range classNames {
			if className != exclude {
				classObj, exists, err := c.classStore.GetByKey(className)
				if err != nil {
					log.WithError(err).Errorln("classStore.GetByKey error")
				}
				if exists {
					classes = append(classes, classObj.(*spicev1.NamespaceClass))
				}
			}
		}
	}
	return classes
}

func (c *Controller) handleNamespaceAdd(obj interface{}) {
	if c.hasInitiallyReconciled {
		ns := obj.(*v1.Namespace)
		c.reconcile(ns, nil, c.getNamespaceClasses(ns, ""))
	}
}

func (c *Controller) handleNamespaceUpdate(oldObj interface{}, newObj interface{}) {
	if c.hasInitiallyReconciled {
		oldNs := oldObj.(*v1.Namespace)
		newNs := newObj.(*v1.Namespace)

		if oldNs.ResourceVersion == newNs.ResourceVersion {
			// this is a relist/reconnect/etc, not an actual update
			return
		}

		oldClasses := c.getNamespaceClasses(oldNs, "")
		newClasses := c.getNamespaceClasses(newNs, "")
		c.reconcile(newNs, oldClasses, newClasses)
	}
}

func (c *Controller) handleClassAdd(obj interface{}) {
	if c.hasInitiallyReconciled {
		class := obj.(*spicev1.NamespaceClass)
		namespaces := c.getNamespacesWithClass(class.Name)
		for _, ns := range namespaces {
			c.reconcile(ns, c.getNamespaceClasses(ns, class.Name), c.getNamespaceClasses(ns, ""))
		}
	}
}

func (c *Controller) handleClassUpdate(oldObj interface{}, newObj interface{}) {
	if c.hasInitiallyReconciled {
		oldClass := oldObj.(*spicev1.NamespaceClass)
		newClass := newObj.(*spicev1.NamespaceClass)

		if oldClass.ResourceVersion == newClass.ResourceVersion {
			// this is a relist/reconnect/etc, not an actual update
			return
		}

		namespaces := c.getNamespacesWithClass(newClass.Name)
		for _, ns := range namespaces {
			oldClasses := c.getNamespaceClasses(ns, "")
			// replace the updated class with the old class in this array so the delta is generated properly
			for k, class := range oldClasses {
				if class.Name == oldClass.Name {
					oldClasses[k] = oldClass
				}
			}
			newClasses := c.getNamespaceClasses(ns, "")
			c.reconcile(ns, oldClasses, newClasses)
		}
	}
}

func (c *Controller) handleClassDelete(obj interface{}) {
	if c.hasInitiallyReconciled {
		class := obj.(*spicev1.NamespaceClass)
		namespaces := c.getNamespacesWithClass(class.Name)
		for _, ns := range namespaces {
			oldClasses := c.getNamespaceClasses(ns, "")
			oldClasses = append(oldClasses, class)
			newClasses := c.getNamespaceClasses(ns, "")
			c.reconcile(ns, oldClasses, newClasses)
		}
	}
}

func (c *Controller) Run() {
	c.namespaceStore, c.namespaceController = cache.NewInformer(
		&cache.ListWatch{
			ListFunc: func(lo metav1.ListOptions) (result runtime.Object, err error) {
				return c.kubeclient.CoreV1().Namespaces().List(c.ctx, lo)
			},
			WatchFunc: func(lo metav1.ListOptions) (watch.Interface, error) {
				return c.kubeclient.CoreV1().Namespaces().Watch(c.ctx, lo)
			},
		},
		&v1.Namespace{},
		1*time.Minute,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.handleNamespaceAdd,
			UpdateFunc: c.handleNamespaceUpdate,
		},
	)

	c.classStore, c.classController = cache.NewInformer(
		&cache.ListWatch{
			ListFunc: func(lo metav1.ListOptions) (result runtime.Object, err error) {
				return c.spiceclient.NamespaceClasses().List(c.ctx, lo)
			},
			WatchFunc: func(lo metav1.ListOptions) (watch.Interface, error) {
				return c.spiceclient.NamespaceClasses().Watch(c.ctx, lo)
			},
		},
		&spicev1.NamespaceClass{},
		1*time.Minute,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.handleClassAdd,
			UpdateFunc: c.handleClassUpdate,
			DeleteFunc: c.handleClassDelete,
		},
	)

	go c.classController.Run(c.ctx.Done())
	go c.namespaceController.Run(c.ctx.Done())
	cache.WaitForCacheSync(c.ctx.Done(), c.namespaceController.HasSynced, c.classController.HasSynced)
	c.cleanup()

	ticker := time.NewTicker(10 * time.Minute)
	go func() {
		for {
			select {
			case <-c.ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				c.cleanup()
			}
		}
	}()
}

func Create(ctx context.Context, kubeclient *kubeclient.Clientset, spiceclient *spiceclient.V1Client, dynamicclient *dynamic.DynamicClient) Controller {
	return Controller{
		kubeclient:    kubeclient,
		spiceclient:   spiceclient,
		dynamicclient: dynamicclient,
		ctx:           ctx,
	}
}
