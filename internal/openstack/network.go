package osclients

import (
	"context"
	"fmt"

	gophercloud "github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"

	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/ports"
)

type NetworkClient interface {
	ListPorts(ctx context.Context, opts ports.ListOptsBuilder) ([]ports.Port, error)
	UpdatePort(ctx context.Context, id string, opts ports.UpdateOptsBuilder) (*ports.Port, error)
}

type networkClient struct {
	serviceClient *gophercloud.ServiceClient
}

func NewNetworkClient(providerClient *gophercloud.ProviderClient, endpointOpts *gophercloud.EndpointOpts) (NetworkClient, error) {
	serviceClient, err := openstack.NewNetworkV2(providerClient, *endpointOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create service client: %v", err)
	}

	return networkClient{serviceClient: serviceClient}, nil
}

func (c networkClient) ListPorts(ctx context.Context, opts ports.ListOptsBuilder) ([]ports.Port, error) {
	allPages, err := ports.List(c.serviceClient, opts).AllPages(ctx)
	if err != nil {
		return nil, err
	}

	prts, err := ports.ExtractPorts(allPages)
	if err != nil {
		return nil, err
	}
	return prts, nil
}

func (c networkClient) UpdatePort(ctx context.Context, portId string, opts ports.UpdateOptsBuilder) (*ports.Port, error) {
	prts, err := ports.Update(ctx, c.serviceClient, portId, opts).Extract()
	if err != nil {
		return nil, err
	}
	return prts, nil
}
