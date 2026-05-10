package synadia

import (
	"fmt"

	"github.com/synadia-io/control-plane-sdk-go/syncp"
)

// DefaultSkGroupName is the name we use for the unscoped signing-key group
// pb-synadia provisions per account when one isn't already present.
const DefaultSkGroupName = "pb-synadia-default"

// EnsureDefaultSkGroup returns the id of an unscoped signing-key group on the
// given Synadia account, creating one if needed.
//
// pb-synadia uses inline (unscoped) user permissions, but every Synadia user
// must still be assigned to *some* sk group at create time. We pick the first
// existing unscoped, non-disabled group (preferring one we created — name +
// programmatic flag) and fall back to creating a new one.
func (c *Client) EnsureDefaultSkGroup(synadiaAccountID string) (string, error) {
	resp, _, err := c.api.AccountAPI.ListAccountSkGroup(c.ctx, synadiaAccountID).Execute()
	if err != nil {
		return "", fmt.Errorf("list sk groups for account %q: %w", synadiaAccountID, err)
	}

	var fallback string
	for _, g := range resp.Items {
		if g.Disabled {
			continue
		}
		if g.IsScoped {
			continue
		}
		if g.Name == DefaultSkGroupName && g.Programmatic {
			return g.Id, nil
		}
		if fallback == "" {
			fallback = g.Id
		}
	}
	if fallback != "" {
		return fallback, nil
	}

	created, _, err := c.api.AccountAPI.CreateAccountSkGroup(c.ctx, synadiaAccountID).
		SigningKeyGroupCreateRequest(syncp.SigningKeyGroupCreateRequest{
			Name:         DefaultSkGroupName,
			Programmatic: true,
			// Scope nil => unscoped: users get inline JwtSettings permissions
		}).
		Execute()
	if err != nil {
		return "", fmt.Errorf("create default sk group on account %q: %w", synadiaAccountID, err)
	}
	return created.Id, nil
}
