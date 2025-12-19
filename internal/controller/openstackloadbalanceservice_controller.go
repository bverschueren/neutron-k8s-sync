/*
Copyright 2025.

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

package controller

import (
	"context"
	"fmt"
	"sync"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openstackv1alpha1 "github.com/bverschueren/neutron-k8s-sync/api/v1alpha1"
	"github.com/bverschueren/neutron-k8s-sync/internal/helpers"
)

var (
	l2GVR = schema.GroupVersionResource{
		Group:    "metallb.io",
		Version:  "v1beta1",
		Resource: "l2advertisements",
	}

	bgpGVR = schema.GroupVersionResource{
		Group:    "metallb.io",
		Version:  "v1beta1",
		Resource: "bgpadvertisements",
	}
	ipPoolGVR = schema.GroupVersionResource{
		Group:    "metallb.io",
		Version:  "v1beta1",
		Resource: "ipaddresspools",
	}
)

type OpenStackLoadBalanceServiceReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Config    *rest.Config
	dynClient dynamic.Interface

	informerCancel context.CancelFunc
	informerLock   sync.Mutex

	poolCache map[string][]string
	poolLock  sync.RWMutex
}

func (r *OpenStackLoadBalanceServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Config = mgr.GetConfig()

	return ctrl.NewControllerManagedBy(mgr).
		For(&openstackv1alpha1.OpenStackLoadBalanceService{}).
		Complete(r)
}

func (r *OpenStackLoadBalanceServiceReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	log := ctrl.Log.WithName("osplbreconciler")

	var list openstackv1alpha1.OpenStackLoadBalanceServiceList
	if err := r.List(ctx, &list); err != nil {
		return ctrl.Result{}, err
	}

	if len(list.Items) == 0 {
		log.Info("No OpenStackLoadBalanceService present, stopping MetalLB informers")
		r.stopInformers()
		return ctrl.Result{}, nil
	}

	log.Info("OpenStackLoadBalanceService present, ensuring MetalLB informers running")
	r.startInformers(ctx)
	return ctrl.Result{}, nil
}

func (r *OpenStackLoadBalanceServiceReconciler) startInformers(ctx context.Context) {
	r.informerLock.Lock()
	defer r.informerLock.Unlock()

	if r.informerCancel != nil {
		return
	}

	log := ctrl.Log.WithName("metallb-informers")

	dynClient, err := dynamic.NewForConfig(r.Config)
	if err != nil {
		log.Error(err, "failed to create dynamic client")
		return
	}

	r.dynClient = dynClient
	r.poolCache = make(map[string][]string)

	ctx, cancel := context.WithCancel(ctx)
	r.informerCancel = cancel

	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(
		dynClient,
		0,
		metav1.NamespaceAll,
		nil,
	)

	l2Informer := factory.ForResource(l2GVR).Informer()
	bgpInformer := factory.ForResource(bgpGVR).Informer()

	l2Informer.AddEventHandler(r.advertisementHandler())
	bgpInformer.AddEventHandler(r.advertisementHandler())

	poolInformer := factory.ForResource(ipPoolGVR).Informer()
	poolInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    r.handlePoolChange,
		UpdateFunc: func(_, newObj interface{}) { r.handlePoolChange(newObj) },
		DeleteFunc: r.handlePoolChange,
	})

	log.Info("Starting MetalLB informers")
	go factory.Start(ctx.Done())
}

func (r *OpenStackLoadBalanceServiceReconciler) stopInformers() {
	r.informerLock.Lock()
	defer r.informerLock.Unlock()

	if r.informerCancel != nil {
		r.informerCancel()
		r.informerCancel = nil
	}
}

func (r *OpenStackLoadBalanceServiceReconciler) advertisementHandler() cache.ResourceEventHandlerFuncs {
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			r.applyAdvertisement(context.Background(), obj, nil)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			r.applyAdvertisement(context.Background(), newObj, oldObj)
		},
		DeleteFunc: func(obj interface{}) {
			r.applyAdvertisement(context.Background(), nil, obj)
		},
	}
}

func (r *OpenStackLoadBalanceServiceReconciler) applyAdvertisement(
	ctx context.Context,
	newObj interface{},
	oldObj interface{},
) {
	log := ctrl.Log.WithName("advertisement")

	var newSel map[string]string
	var newAddresses, oldAddresses []string
	var poolNames []string

	if newObj != nil {
		u, err := unwrapUnstructured(newObj)
		if err != nil {
			return
		}

		newSel, poolNames, err = extractAdvertisement(u)
		if err != nil {
			return
		}

		// Resolve addresses from cached IPAddressPools
		for _, pool := range poolNames {
			r.poolLock.RLock()
			addresses := r.poolCache[pool]
			r.poolLock.RUnlock()
			newAddresses = append(newAddresses, addresses...)
		}
	}

	if oldObj != nil {
		u, err := unwrapUnstructured(oldObj)
		if err != nil {
			return
		}

		_, poolNames, err := extractAdvertisement(u)
		if err != nil {
			return
		}
		for _, pool := range poolNames {
			r.poolLock.RLock()
			addresses := r.poolCache[pool]
			r.poolLock.RUnlock()
			oldAddresses = append(oldAddresses, addresses...)
		}
	}

	addIPs, delIPs := diffStringSlice(newAddresses, oldAddresses)
	log.V(1).Info("adding allowed pool", "address", addIPs)
	log.V(1).Info("removing allowed pool", "address", delIPs)

	nodes := &corev1.NodeList{}
	if err := r.List(ctx, nodes, client.MatchingLabels(newSel)); err != nil {
		log.Error(err, "list nodes")
		return
	}

	for _, n := range nodes.Items {
		if err := helpers.UpdateAllowedAddressPairs(ctx, n, addIPs, delIPs); err != nil {
			log.Error(err, "update allowed address pairs", "node", n.Name)
		}
	}
}

func (r *OpenStackLoadBalanceServiceReconciler) handlePoolChange(obj interface{}) {
	u, err := unwrapUnstructured(obj)
	if err != nil {
		return
	}

	name := u.GetName()
	addresses, _, _ := unstructured.NestedStringSlice(u.Object, "spec", "addresses")

	r.poolLock.Lock()
	r.poolCache[name] = addresses
	r.poolLock.Unlock()

	// trigger reconciliation for all advertisements referencing this pool
	r.requeueAdvertisementsReferencingPool(name)
}

func (r *OpenStackLoadBalanceServiceReconciler) requeueAdvertisementsReferencingPool(poolName string) {

	adsGVRs := []schema.GroupVersionResource{l2GVR, bgpGVR}

	for _, gvr := range adsGVRs {
		list, err := r.dynClient.Resource(gvr).Namespace(metav1.NamespaceAll).List(context.Background(), metav1.ListOptions{})
		if err != nil {
			continue
		}

		for _, item := range list.Items {
			// check if this ad references pool
			pools, _, _ := unstructured.NestedStringSlice(item.Object, "spec", "ipAddressPools")
			for _, p := range pools {
				if p == poolName {
					r.applyAdvertisement(context.Background(), &item, nil)
					break
				}
			}
		}
	}
}

func diffStringSlice(new, old []string) (add, del []string) {
	m := map[string]int{}
	for _, s := range new {
		m[s]++
	}
	for _, s := range old {
		m[s]--
	}
	for k, v := range m {
		if v > 0 {
			add = append(add, k)
		}
		if v < 0 {
			del = append(del, k)
		}
	}
	return
}

func unwrapUnstructured(obj interface{}) (*unstructured.Unstructured, error) {
	switch o := obj.(type) {

	case *unstructured.Unstructured:
		return o, nil

	case cache.DeletedFinalStateUnknown:
		if o.Obj == nil {
			return nil, fmt.Errorf("tombstone contained nil object")
		}
		u, ok := o.Obj.(*unstructured.Unstructured)
		if !ok {
			return nil, fmt.Errorf("tombstone object was %T, not Unstructured", o.Obj)
		}
		return u, nil

	default:
		return nil, fmt.Errorf("unexpected object type: %T", obj)
	}
}

func extractAdvertisement(
	u *unstructured.Unstructured,
) (map[string]string, []string, error) {

	switch u.GetKind() {

	case "L2Advertisement", "BGPAdvertisement":

		pools, _, err := unstructured.NestedStringSlice(
			u.Object, "spec", "ipAddressPools",
		)
		if err != nil {
			return nil, nil, err
		}

		selectors, _, err := unstructured.NestedStringMap(
			u.Object, "spec", "nodeSelectors", "0", "matchLabels",
		)
		if err != nil {
			return nil, nil, err
		}

		return selectors, pools, nil
	}

	return nil, nil, fmt.Errorf("unsupported advertisement kind: %q", u.GetKind())
}
