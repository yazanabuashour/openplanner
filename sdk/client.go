package sdk

import (
	"errors"
	"net/http"

	"github.com/yazanabuashour/openplanner/internal/api"
	"github.com/yazanabuashour/openplanner/internal/service"
	"github.com/yazanabuashour/openplanner/internal/store"
	"github.com/yazanabuashour/openplanner/sdk/generated"
)

type Options struct {
	DatabasePath string
}

type Client struct {
	*generated.APIClient
	closeFn func() error
}

func OpenLocal(options Options) (*Client, error) {
	if options.DatabasePath == "" {
		return nil, errors.New("database path is required")
	}

	repository, err := store.Open(options.DatabasePath)
	if err != nil {
		return nil, err
	}

	handler := api.NewHandler(service.New(repository))
	configuration := generated.NewConfiguration()
	configuration.HTTPClient = &http.Client{
		Transport: &localRoundTripper{handler: handler},
	}
	configuration.Servers = generated.ServerConfigurations{
		{URL: "http://openplanner.local"},
	}

	return &Client{
		APIClient: generated.NewAPIClient(configuration),
		closeFn:   repository.Close,
	}, nil
}

func (client *Client) Close() error {
	if client == nil || client.closeFn == nil {
		return nil
	}

	return client.closeFn()
}
