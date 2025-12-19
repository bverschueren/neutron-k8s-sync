package main

import (
	"fmt"
	"reflect"

	metallbv1beta1 "go.universe.tf/metallb/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

func handleAdvertisement(obj interface{}) error {
	fmt.Printf("obj.type: %s\n", reflect.TypeOf(obj))
	switch adv := obj.(type) {

	case *metallbv1beta1.L2Advertisement:
		fmt.Printf("ad.type: %s\n", reflect.TypeOf(adv))
		fmt.Printf("ADD L2Advertisement: %s/%s\n",

			adv.Namespace, adv.Name)

	case *metallbv1beta1.BGPAdvertisement:
		//		fmt.Printf("ADD BGPAdvertisement: %s/%s\n",
		//			adv.Namespace, adv.Name)
		fmt.Print(adv)

	default:
		fmt.Printf("ad.type: %s\n", reflect.TypeOf(adv))
		return fmt.Errorf("unsupported advertisement type: %T", obj)
	}

	return nil
}

func handleL2Advertisement(l2 *metallbv1beta1.L2Advertisement) {
	// L2-specific handling
}

func handleBGPAdvertisement(bgp *metallbv1beta1.BGPAdvertisement) {
	// BGP-specific handling
}

func main() {
	// Informer event handler
	eventHandler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if err := handleAdvertisement(obj); err != nil {
				fmt.Printf("error handling add event: %v\n", err)
			}
		},
	}

	// ---- Example objects (what the informer would pass in) ----

	l2Adv := &metallbv1beta1.L2Advertisement{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fake-l2-advertisement",
			Namespace: "metallb-system",
		},
		Spec: metallbv1beta1.L2AdvertisementSpec{
			IPAddressPools: []string{"pool-a", "pool-b"},
		},
	}
	l2Adv = &metallbv1beta1.L2Advertisement{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fake-l2-advertisement",
			Namespace: "metallb-system",
		},
	}

	// Simulate informer Add events
	eventHandler.AddFunc(l2Adv)
	eventHandler.AddFunc(l2Adv)
}
