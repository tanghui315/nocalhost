package watcher

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"nocalhost/pkg/nhctl/clientgoutils"
	"sync"
	"time"
)

type SimpleWatcher struct {
	indexer  cache.Indexer
	queue    workqueue.RateLimitingInterface
	informer cache.Controller
	watcher  Watcher
}

// NewSimpleWatcher returns the chan control the informer's live cycle
// you can stop the informer by calling
// quitChan <- whatever
// or block the thread by this quitChan
//
// the quitChan in createOrUpdateFunc, deleteFunc and returns is the same
func NewSimpleWatcher(
	cgu *clientgoutils.ClientGoUtils, resource, labelSelector string,
	createOrUpdateFun func(key string, object interface{}, quitChan chan struct{}),
	deleteFun func(key string, quitChan chan struct{}),
) chan struct{} {
	gvr := cgu.ResourceFor(resource, false)
	var lofun dynamicinformer.TweakListOptionsFunc = nil
	if labelSelector != "" {
		lofun = func(options *metav1.ListOptions) {
			options.LabelSelector = labelSelector
		}
	}

	dynamicInformerFactory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(
		cgu.GetDynamicClient(),
		time.Second*2,
		cgu.GetNameSpace(),
		lofun,
	)

	stopCh := make(chan struct{})

	informer := dynamicInformerFactory.ForResource(gvr)
	lock := sync.Mutex{}
	fun := func(key string) {
		lock.Lock()
		defer lock.Unlock()

		if obj, exists, err := informer.Informer().GetIndexer().GetByKey(key); err == nil {
			if exists {
				if createOrUpdateFun != nil {
					createOrUpdateFun(key, obj, stopCh)
				}
			} else {
				if deleteFun != nil {
					deleteFun(key, stopCh)
				}
			}
		}
	}

	informer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				if key, err := cache.MetaNamespaceKeyFunc(obj); err == nil {
					fun(key)
				}
			},
			UpdateFunc: func(old, new interface{}) {
				if key, err := cache.MetaNamespaceKeyFunc(new); err == nil {
					fun(key)
				}
			},
			DeleteFunc: func(obj interface{}) {
				if key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj); err == nil {
					fun(key)
				}
			},
		},
	)

	go informer.Informer().Run(stopCh)
	return stopCh
}
