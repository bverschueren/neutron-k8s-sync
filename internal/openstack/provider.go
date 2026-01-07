package osclients

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/config"
	"github.com/gophercloud/gophercloud/v2/openstack/config/clouds"
)

func NewProviderClient(ctx context.Context) (*gophercloud.ProviderClient, *gophercloud.EndpointOpts, error) {

	authOpts, endpointOpts, tlsConfig, err := clouds.Parse(
		clouds.WithCloudName("openstack"),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("parse clouds.yaml: %w", err)
	}

	provider, err := config.NewProviderClient(
		ctx,
		authOpts,
		config.WithTLSConfig(tlsConfig),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("authenticate OpenStack: %w", err)
	}

	return provider, &endpointOpts, nil
}
