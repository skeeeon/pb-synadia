// Package synadia is a thin adapter over the Synadia Cloud Go SDK.
//
// All SDK calls are funneled through this package so a future SDK bump or
// API rename is a one-file change. The exported Client type is what hooks
// and Reconcile depend on; nothing else in pb-synadia imports the SDK
// directly.
package synadia

import (
	"context"
	"net/http"

	"github.com/synadia-io/control-plane-sdk-go/syncp"
)

// Client wraps the Synadia syncp client with our auth context applied.
type Client struct {
	api      *syncp.APIClient
	ctx      context.Context
	systemID string
}

// NewClient builds a Client. baseURL defaults to https://cloud.synadia.com
// when empty.
func NewClient(systemID, apiToken, baseURL string) *Client {
	cfg := syncp.NewConfiguration()
	api := syncp.NewAPIClient(cfg)

	ctx := context.Background()
	if baseURL != "" {
		ctx = context.WithValue(ctx, syncp.ContextServerVariables, map[string]string{
			"baseUrl": baseURL,
		})
	}
	ctx = context.WithValue(ctx, syncp.ContextAccessToken, apiToken)

	return &Client{
		api:      api,
		ctx:      ctx,
		systemID: systemID,
	}
}

// SystemID returns the configured Synadia system id.
func (c *Client) SystemID() string { return c.systemID }

// IsNotFound reports whether err is a 404 from the Synadia API. Used by
// delete hooks to treat "already gone" as success.
func IsNotFound(resp *http.Response, err error) bool {
	if err == nil {
		return false
	}
	if resp != nil && resp.StatusCode == http.StatusNotFound {
		return true
	}
	return false
}
