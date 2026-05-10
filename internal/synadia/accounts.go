package synadia

import (
	"fmt"
	"net/http"

	"github.com/synadia-io/control-plane-sdk-go/syncp"
	pbtypes "github.com/skeeeon/pb-synadia/internal/types"
)

// AccountResult holds the fields we cache back onto the PB record after a
// successful create/update.
type AccountResult struct {
	ID        string
	PublicKey string
	Name      string
}

// CreateAccount creates an account in the configured Synadia system.
//
// Limits semantics: pb-nats uses -1=unlimited, 0=disabled, positive=limit.
// We forward non-zero values to Synadia and treat zero as "unset". The
// "disabled (zero blocks)" pb-nats convention does not apply here — Synadia
// has no equivalent. Document this in the README.
func (c *Client) CreateAccount(acc *pbtypes.AccountRecord) (AccountResult, error) {
	req := syncp.AccountCreateRequest{
		Name:        acc.NormalizeName(),
		JwtSettings: buildAccountJWTSettings(acc),
	}
	resp, _, err := c.api.SystemAPI.CreateAccount(c.ctx, c.systemID).
		AccountCreateRequest(req).
		Execute()
	if err != nil {
		return AccountResult{}, fmt.Errorf("create account %q: %w", acc.Name, err)
	}
	return AccountResult{
		ID:        resp.Id,
		PublicKey: resp.AccountPublicKey,
		Name:      resp.Name,
	}, nil
}

// UpdateAccount updates an existing Synadia account's JWT settings.
func (c *Client) UpdateAccount(acc *pbtypes.AccountRecord) (AccountResult, error) {
	req := syncp.AccountUpdateRequest{
		JwtSettings: buildAccountJWTSettingsPatch(acc),
	}
	resp, _, err := c.api.AccountAPI.UpdateAccount(c.ctx, acc.SynadiaAccountID).
		AccountUpdateRequest(req).
		Execute()
	if err != nil {
		return AccountResult{}, fmt.Errorf("update account %q: %w", acc.SynadiaAccountID, err)
	}
	return AccountResult{
		ID:        resp.Id,
		PublicKey: resp.AccountPublicKey,
		Name:      resp.Name,
	}, nil
}

// DeleteAccount removes the Synadia account. Caller should use IsNotFound on
// the returned response+error to treat already-gone as success.
func (c *Client) DeleteAccount(synadiaAccountID string) (*http.Response, error) {
	resp, err := c.api.AccountAPI.DeleteAccount(c.ctx, synadiaAccountID).Execute()
	if err != nil {
		return resp, fmt.Errorf("delete account %q: %w", synadiaAccountID, err)
	}
	return resp, nil
}

// ListAccounts returns all accounts in the configured system. Used by Reconcile.
func (c *Client) ListAccounts() ([]syncp.AccountViewResponse, error) {
	resp, _, err := c.api.SystemAPI.ListAccounts(c.ctx, c.systemID).Execute()
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	return resp.Items, nil
}

func buildAccountJWTSettings(acc *pbtypes.AccountRecord) *syncp.AccountJWTSettings {
	limits := buildOperatorLimits(acc)
	if limits == nil && acc.Description == "" {
		return nil
	}
	settings := &syncp.AccountJWTSettings{}
	if acc.Description != "" {
		settings.Info = syncp.Info{Description: syncp.Ptr(acc.Description)}
	}
	if limits != nil {
		settings.Limits = limits
	}
	return settings
}

func buildAccountJWTSettingsPatch(acc *pbtypes.AccountRecord) *syncp.AccountJWTSettingsPatch {
	patch := &syncp.AccountJWTSettingsPatch{}
	if acc.Description != "" {
		nb := syncp.NewNullable(acc.Description)
		patch.Description = &nb
	}
	if limits := buildOperatorLimitsPatch(acc); limits != nil {
		nb := syncp.NewNullable(*limits)
		patch.Limits = &nb
	}
	return patch
}

func buildOperatorLimits(acc *pbtypes.AccountRecord) *syncp.OperatorLimits {
	if !hasAnyLimit(acc) {
		return nil
	}
	lim := &syncp.OperatorLimits{}
	if acc.MaxSubscriptions != 0 {
		lim.NatsLimits.Subs = syncp.Ptr(acc.MaxSubscriptions)
	}
	if acc.MaxData != 0 {
		lim.NatsLimits.Data = syncp.Ptr(acc.MaxData)
	}
	if acc.MaxPayload != 0 {
		lim.NatsLimits.Payload = syncp.Ptr(acc.MaxPayload)
	}
	if acc.MaxConnections != 0 {
		lim.AccountLimits.Conn = syncp.Ptr(acc.MaxConnections)
	}
	if acc.MaxJetStreamDiskStorage != 0 {
		lim.JetStreamLimits.DiskStorage = syncp.Ptr(acc.MaxJetStreamDiskStorage)
	}
	if acc.MaxJetStreamMemoryStorage != 0 {
		lim.JetStreamLimits.MemStorage = syncp.Ptr(acc.MaxJetStreamMemoryStorage)
	}
	return lim
}

func buildOperatorLimitsPatch(acc *pbtypes.AccountRecord) *syncp.OperatorLimitsPatch {
	if !hasAnyLimit(acc) {
		return nil
	}
	lim := &syncp.OperatorLimitsPatch{}
	if acc.MaxSubscriptions != 0 {
		lim.Subs = syncp.Ptr(acc.MaxSubscriptions)
	}
	if acc.MaxData != 0 {
		lim.Data = syncp.Ptr(acc.MaxData)
	}
	if acc.MaxPayload != 0 {
		lim.Payload = syncp.Ptr(acc.MaxPayload)
	}
	if acc.MaxConnections != 0 {
		lim.Conn = syncp.Ptr(acc.MaxConnections)
	}
	if acc.MaxJetStreamDiskStorage != 0 {
		lim.DiskStorage = syncp.Ptr(acc.MaxJetStreamDiskStorage)
	}
	if acc.MaxJetStreamMemoryStorage != 0 {
		lim.MemStorage = syncp.Ptr(acc.MaxJetStreamMemoryStorage)
	}
	return lim
}

func hasAnyLimit(acc *pbtypes.AccountRecord) bool {
	return acc.MaxConnections != 0 || acc.MaxSubscriptions != 0 || acc.MaxData != 0 ||
		acc.MaxPayload != 0 || acc.MaxJetStreamDiskStorage != 0 ||
		acc.MaxJetStreamMemoryStorage != 0
}
