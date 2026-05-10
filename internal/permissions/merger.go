// Package permissions merges role permissions with per-user overrides.
//
// pb-synadia keeps roles as a PB-side template that Synadia never sees.
// On every user write, this merger combines role + user permissions into a
// single MergedPermissions struct that is pushed to Synadia as inline
// (unscoped) user permissions.
//
// Merge rules (matching pb-nats):
//   - Allow lists: role ∪ user_override
//   - Deny lists:  role_deny ∪ user_deny_override
//   - Defaults apply only when both role and user lists are empty
//   - Deny precedence is enforced server-side by Synadia; we just push both
package permissions

import (
	pbtypes "github.com/skeeeon/pb-synadia/internal/types"
)

// Merge produces the inline permissions to send to Synadia for a user.
//
// defaults are applied for pub or sub independently when the corresponding
// allow list (role + user) is empty.
func Merge(role *pbtypes.RoleRecord, user *pbtypes.NatsUserRecord, defaults pbtypes.Options) (pbtypes.MergedPermissions, error) {
	rolePub, err := role.GetPublishPermissions()
	if err != nil {
		return pbtypes.MergedPermissions{}, err
	}
	roleSub, err := role.GetSubscribePermissions()
	if err != nil {
		return pbtypes.MergedPermissions{}, err
	}
	rolePubDeny, err := role.GetPublishDenyPermissions()
	if err != nil {
		return pbtypes.MergedPermissions{}, err
	}
	roleSubDeny, err := role.GetSubscribeDenyPermissions()
	if err != nil {
		return pbtypes.MergedPermissions{}, err
	}

	userPub, err := user.GetPublishPermissions()
	if err != nil {
		return pbtypes.MergedPermissions{}, err
	}
	userSub, err := user.GetSubscribePermissions()
	if err != nil {
		return pbtypes.MergedPermissions{}, err
	}
	userPubDeny, err := user.GetPublishDenyPermissions()
	if err != nil {
		return pbtypes.MergedPermissions{}, err
	}
	userSubDeny, err := user.GetSubscribeDenyPermissions()
	if err != nil {
		return pbtypes.MergedPermissions{}, err
	}

	pub := union(rolePub, userPub)
	sub := union(roleSub, userSub)
	if len(pub) == 0 {
		pub = append(pub, defaults.DefaultPublishPermissions...)
	}
	if len(sub) == 0 {
		sub = append(sub, defaults.DefaultSubscribePermissions...)
	}

	return pbtypes.MergedPermissions{
		Pub:              pub,
		Sub:              sub,
		PubDeny:          union(rolePubDeny, userPubDeny),
		SubDeny:          union(roleSubDeny, userSubDeny),
		AllowResponse:    role.AllowResponse,
		AllowResponseMax: role.AllowResponseMax,
		AllowResponseTTL: role.AllowResponseTTL,
		MaxSubscriptions: role.MaxSubscriptions,
		MaxData:          role.MaxData,
		MaxPayload:       role.MaxPayload,
	}, nil
}

// union returns the order-preserving union of two string slices.
func union(a, b []string) []string {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, s := range a {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	for _, s := range b {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
