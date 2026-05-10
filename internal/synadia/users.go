package synadia

import (
	"fmt"
	"net/http"

	"github.com/synadia-io/control-plane-sdk-go/syncp"
	pbtypes "github.com/skeeeon/pb-synadia/internal/types"
)

// UserCreateInput bundles everything needed to create a Synadia NATS user.
type UserCreateInput struct {
	SynadiaAccountID string
	SkGroupID        string
	NatsUsername     string
	BearerToken      bool
	Permissions      pbtypes.MergedPermissions
}

// UserResult holds the Synadia ids cached back onto the PB record.
type UserResult struct {
	ID        string
	PublicKey string
	JWT       string
}

// CreateUser provisions a new NATS user under the given account + sk group.
func (c *Client) CreateUser(in UserCreateInput) (UserResult, error) {
	req := syncp.NatsUserCreateRequest{
		Name:        in.NatsUsername,
		SkGroupId:   in.SkGroupID,
		JwtSettings: buildCreateUserJwtSettings(in.BearerToken, in.Permissions),
	}
	resp, _, err := c.api.AccountAPI.CreateUser(c.ctx, in.SynadiaAccountID).
		NatsUserCreateRequest(req).
		Execute()
	if err != nil {
		return UserResult{}, fmt.Errorf("create user %q in account %q: %w",
			in.NatsUsername, in.SynadiaAccountID, err)
	}
	return UserResult{
		ID:        resp.Id,
		PublicKey: resp.UserPublicKey,
		JWT:       resp.Jwt,
	}, nil
}

// UpdateUser pushes an updated permission/limit set for an existing user.
//
// Synadia's NatsUserUpdateRequest does not include SkGroupId — group
// assignment is fixed at create time. Permission changes flow via the
// JwtSettings patch.
func (c *Client) UpdateUser(synadiaUserID string, perms pbtypes.MergedPermissions, bearerToken bool) (UserResult, error) {
	req := syncp.NatsUserUpdateRequest{
		JwtSettings: buildUpdateUserJwtSettings(bearerToken, perms),
	}
	resp, _, err := c.api.NatsUserAPI.UpdateNatsUser(c.ctx, synadiaUserID).
		NatsUserUpdateRequest(req).
		Execute()
	if err != nil {
		return UserResult{}, fmt.Errorf("update user %q: %w", synadiaUserID, err)
	}
	return UserResult{
		ID:        resp.Id,
		PublicKey: resp.UserPublicKey,
		JWT:       resp.Jwt,
	}, nil
}

// DeleteUser removes a Synadia user. Caller should use IsNotFound on the
// returned response+error to treat already-gone as success.
func (c *Client) DeleteUser(synadiaUserID string) (*http.Response, error) {
	resp, err := c.api.NatsUserAPI.DeleteNatsUser(c.ctx, synadiaUserID).Execute()
	if err != nil {
		return resp, fmt.Errorf("delete user %q: %w", synadiaUserID, err)
	}
	return resp, nil
}

// DownloadCreds returns the .creds file content for a Synadia user.
func (c *Client) DownloadCreds(synadiaUserID string) (string, error) {
	creds, _, err := c.api.NatsUserAPI.DownloadNatsUserCreds(c.ctx, synadiaUserID).Execute()
	if err != nil {
		return "", fmt.Errorf("download creds for user %q: %w", synadiaUserID, err)
	}
	return creds, nil
}

// ListUsers returns all NATS users for an account. Used by Reconcile.
func (c *Client) ListUsers(synadiaAccountID string) ([]syncp.NatsUserViewResponse, error) {
	resp, _, err := c.api.AccountAPI.ListUsers(c.ctx, synadiaAccountID).Execute()
	if err != nil {
		return nil, fmt.Errorf("list users for account %q: %w", synadiaAccountID, err)
	}
	return resp.Items, nil
}

func buildCreateUserJwtSettings(bearer bool, p pbtypes.MergedPermissions) *syncp.NatsCreateUserJwtSettings {
	s := &syncp.NatsCreateUserJwtSettings{}
	s.Permissions = syncp.Permissions{
		Pub:  buildPermission(p.Pub, p.PubDeny),
		Sub:  buildPermission(p.Sub, p.SubDeny),
		Resp: buildResponsePermission(p.AllowResponse, p.AllowResponseMax, p.AllowResponseTTL),
	}
	if bearer {
		s.BearerToken = syncp.Ptr(true)
	}
	if p.MaxSubscriptions != 0 {
		s.Subs = syncp.Ptr(p.MaxSubscriptions)
	}
	if p.MaxData != 0 {
		s.Data = syncp.Ptr(p.MaxData)
	}
	if p.MaxPayload != 0 {
		s.Payload = syncp.Ptr(p.MaxPayload)
	}
	return s
}

func buildUpdateUserJwtSettings(bearer bool, p pbtypes.MergedPermissions) *syncp.NatsUserJwtSettingsPatch {
	patch := &syncp.NatsUserJwtSettingsPatch{
		BearerToken: syncp.Ptr(bearer),
	}
	if pp := buildPermissionPatch(p.Pub, p.PubDeny); pp != nil {
		nb := syncp.NewNullable(*pp)
		patch.Pub = &nb
	}
	if pp := buildPermissionPatch(p.Sub, p.SubDeny); pp != nil {
		nb := syncp.NewNullable(*pp)
		patch.Sub = &nb
	}
	if rp := buildResponsePermissionPatch(p.AllowResponse, p.AllowResponseMax, p.AllowResponseTTL); rp != nil {
		nb := syncp.NewNullable(*rp)
		patch.Resp = &nb
	}
	if p.MaxSubscriptions != 0 {
		patch.Subs = syncp.Ptr(p.MaxSubscriptions)
	}
	if p.MaxData != 0 {
		patch.Data = syncp.Ptr(p.MaxData)
	}
	if p.MaxPayload != 0 {
		patch.Payload = syncp.Ptr(p.MaxPayload)
	}
	return patch
}

func buildPermission(allow, deny []string) *syncp.Permission {
	if len(allow) == 0 && len(deny) == 0 {
		return nil
	}
	return &syncp.Permission{Allow: allow, Deny: deny}
}

func buildPermissionPatch(allow, deny []string) *syncp.PermissionPatch {
	if len(allow) == 0 && len(deny) == 0 {
		return nil
	}
	return &syncp.PermissionPatch{Allow: allow, Deny: deny}
}

func buildResponsePermission(allow bool, max, ttl int) *syncp.ResponsePermission {
	if !allow {
		return nil
	}
	return &syncp.ResponsePermission{
		Max: int64(max),
		Ttl: int64(ttl),
	}
}

func buildResponsePermissionPatch(allow bool, max, ttl int) *syncp.ResponsePermissionPatch {
	if !allow {
		return nil
	}
	return &syncp.ResponsePermissionPatch{
		Max: syncp.Ptr(int64(max)),
		Ttl: syncp.Ptr(int64(ttl)),
	}
}
