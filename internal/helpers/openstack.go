package helpers

import (
	"context"
	"fmt"
	"strings"

	osclients "github.com/bverschueren/neutron-k8s-sync/internal/openstack"
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/config"
	"github.com/gophercloud/gophercloud/v2/openstack/config/clouds"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/ports"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

func GetOpenStackProvider(ctx context.Context) (*gophercloud.ProviderClient, error) {
	log := ctrl.Log.WithName("openstack-auth")

	authOpts, endpointOpts, tlsConfig, err := clouds.Parse(
		clouds.WithCloudName("openstack"),
	)
	if err != nil {
		return nil, fmt.Errorf("parse clouds.yaml: %w", err)
	}

	provider, err := config.NewProviderClient(
		ctx,
		authOpts,
		config.WithTLSConfig(tlsConfig),
	)
	if err != nil {
		return nil, fmt.Errorf("authenticate OpenStack: %w", err)
	}

	log.V(1).Info("Authenticated to OpenStack", "region", endpointOpts.Region)
	return provider, nil
}

func NewNetworkClient(ctx context.Context) (*gophercloud.ServiceClient, error) {
	provider, err := GetOpenStackProvider(ctx)
	if err != nil {
		return nil, err
	}

	return openstack.NewNetworkV2(provider, gophercloud.EndpointOpts{})
}

func NodeProviderID(node corev1.Node) (string, error) {
	// providerID: openstack:///UUID
	parts := strings.Split(node.Spec.ProviderID, "/")
	if len(parts) <= 1 {
		return "", fmt.Errorf("invalid providerID")
	}
	return parts[len(parts)-1], nil
}

func UpdateAllowedAddressPairs(
	ctx context.Context,
	netClient osclients.NetworkClient,
	node corev1.Node,
	addIPs []string,
	delIPs []string,
) error {
	log := ctrl.Log.WithName("openstack-neutron")

	serverID, err := NodeProviderID(node)
	if err != nil {
		return err
	}

	ps, err := netClient.ListPorts(ctx, ports.ListOpts{
		DeviceID: serverID,
	})
	if err != nil {
		return err
	}

	for _, p := range ps {
		current := map[string]bool{}
		for _, a := range p.AllowedAddressPairs {
			current[a.IPAddress] = true
		}

		for _, ip := range addIPs {
			current[ip] = true
		}
		for _, ip := range delIPs {
			delete(current, ip)
		}

		var updated []ports.AddressPair
		for ip := range current {
			updated = append(updated, ports.AddressPair{IPAddress: ip})
		}

		log.V(1).Info("updating to", "AllowedAddressPairs", &updated, "portID", p.ID)

		_, err := netClient.UpdatePort(ctx, p.ID, ports.UpdateOpts{
			AllowedAddressPairs: &updated,
		})
		if err != nil {
			return err
		}
	}

	return nil
}
