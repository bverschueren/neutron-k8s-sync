package osclients

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/config"
	"github.com/gophercloud/gophercloud/v2/openstack/config/clouds"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	log "sigs.k8s.io/controller-runtime/pkg/log"
)

const CloudsYamlKeyName = "clouds.yaml"

type providerClientFactory struct {
	cloudName       string
	client          client.Client
	secretNamespace string
	secretName      string
}

func NewProviderClientFactory(client client.Client, cloudName, secretNamespace, secretName string) *providerClientFactory {
	return &providerClientFactory{
		client:          client,
		cloudName:       cloudName,
		secretNamespace: secretNamespace,
		secretName:      secretName,
	}
}

func (p *providerClientFactory) NewProviderClient(ctx context.Context) (*gophercloud.ProviderClient, *gophercloud.EndpointOpts, error) {
	log := log.FromContext(ctx)

	opts := []clouds.ParseOption{
		clouds.WithCloudName(p.cloudName),
	}

	// try to get the clouds.yaml from a k8s secret first, if not present fall-back to clouds.Parse() from file/env
	cloudsYaml, err := getCloudsYamlFromObject(ctx, p.client, p.secretNamespace, p.secretName)
	if err != nil {
		return nil, nil, err
	}
	if cloudsYaml != nil {
		opts = append(opts, clouds.WithCloudsYAML(cloudsYaml))
		log.V(1).Info("Reading clouds.yaml from object")
	} else {
		log.V(1).Info("Reading clouds.yaml from file")
	}

	authOpts, endpointOpts, tlsConfig, err := clouds.Parse(
		opts...,
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

func getCloudsYamlFromObject(ctx context.Context, c client.Client, secretNamespace, secretName string) (io.Reader, error) {

	secret := &corev1.Secret{}
	err := c.Get(ctx, types.NamespacedName{
		Namespace: secretNamespace,
		Name:      secretName,
	}, secret)
	// try reading from object and if not found, return (nil, nil) and continue reading elsewhere
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return nil, err
		}
		return nil, nil
	}

	content, ok := secret.Data[CloudsYamlKeyName]
	if !ok {
		return nil, fmt.Errorf("secret %s did not include expected key %s",
			secretName, CloudsYamlKeyName)
	}

	return bytes.NewReader(content), nil
}

func NewProviderClient(ctx context.Context) (*gophercloud.ProviderClient, *gophercloud.EndpointOpts, error) {

	cloudName := "openstack"

	authOpts, endpointOpts, tlsConfig, err := clouds.Parse(
		clouds.WithCloudName(cloudName),
		//		clouds.WithCloudsYAML(),
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
