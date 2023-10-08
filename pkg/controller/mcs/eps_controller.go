package mcs

import (
	"context"
	"fmt"
	"time"

	"github.com/lmxia/syncer/pkg/known"
	"github.com/lmxia/syncer/utils"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	discoveryinformerv1 "k8s.io/client-go/informers/discovery/v1"
	"k8s.io/client-go/kubernetes"
	discoverylisterv1 "k8s.io/client-go/listers/discovery/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/dixudx/yacht"
)

type EpsController struct {
	yachtController *yacht.Controller

	//local msc client
	localClusterID string
	srcNamespace   string
	// specific namespace.
	srcEndpointSlicesLister discoverylisterv1.EndpointSliceLister

	targetK8sClient kubernetes.Interface
	targetNamespace string
}

func NewEpsController(clusteID string, epsInformer discoveryinformerv1.EndpointSliceInformer, kubeClientSet kubernetes.Interface) (*EpsController, error) {
	epsController := &EpsController{
		localClusterID:          clusteID,
		srcEndpointSlicesLister: epsInformer.Lister(),
		targetK8sClient:         kubeClientSet,
	}

	yachtcontroller := yacht.NewController("eps").
		WithCacheSynced(epsInformer.Informer().HasSynced).
		WithHandlerFunc(epsController.Handle)
	yachtcontroller.WithEnqueueFilterFunc(func(oldObj, newObj interface{}) (bool, error) {
		if newObj != nil {
			newEps := newObj.(*discoveryv1.EndpointSlice)
			// ignore the eps sourced from it-self
			if newEps.GetLabels()[known.LabelClusterID] == clusteID {
				return false, nil
			}
		}
		return true, nil
	})
	epsController.yachtController = yachtcontroller
	return epsController, nil
}

func (c *EpsController) Handle(obj interface{}) (requeueAfter *time.Duration, err error) {
	ctx := context.Background()
	key := obj.(string)
	namespace, epsName, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid endpointslice key: %s", key))
		return nil, nil
	}

	cachedEps, err := c.srcEndpointSlicesLister.EndpointSlices(namespace).Get(epsName)
	if err != nil {
		if errors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("endpointslice '%s' in work queue no longer exists", key))
			return nil, nil
		}
		return nil, err
	}

	eps := cachedEps.DeepCopy()
	epsTerminating := eps.DeletionTimestamp != nil

	// recycle corresponding endpoint slice.
	if epsTerminating {
		if err = c.targetK8sClient.DiscoveryV1().EndpointSlices(c.targetNamespace).Delete(ctx, eps.Name, metav1.DeleteOptions{}); err != nil {
			// try next time, make sure we clear endpoint slice
			d := time.Second
			return &d, err
		}
		klog.Infof("endpoint slice %s has been recycled successfully", eps.Name)
		return nil, nil
	}
	eps.Namespace = c.targetNamespace
	if err = utils.ApplyEndPointSliceWithRetry(c.targetK8sClient, eps); err != nil {
		klog.Infof("slice %s sync err: %s", eps.Name, err)
		d := time.Second
		return &d, err
	}

	klog.Infof("endpoint slice %s has been synced successfully", eps.Name)
	return nil, nil
}
