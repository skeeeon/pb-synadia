// Package collections handles PocketBase collection initialization for pb-synadia.
//
// All collections default to nil API rules — the consuming app must explicitly
// set rules appropriate to its deployment.
package collections

import (
	"fmt"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	pbcore "github.com/pocketbase/pocketbase/tools/types"
	pbtypes "github.com/skeeeon/pb-synadia/internal/types"
)

// Manager creates the 5 PB collections pb-synadia needs.
type Manager struct {
	app     *pocketbase.PocketBase
	options pbtypes.Options
}

// NewManager wires up a manager. Call InitializeCollections from a bootstrap hook.
func NewManager(app *pocketbase.PocketBase, options pbtypes.Options) *Manager {
	return &Manager{app: app, options: options}
}

// InitializeCollections creates collections in dependency order. Idempotent.
func (cm *Manager) InitializeCollections() error {
	if err := cm.createAccountsCollection(); err != nil {
		return fmt.Errorf("accounts: %w", err)
	}
	if err := cm.createRolesCollection(); err != nil {
		return fmt.Errorf("roles: %w", err)
	}
	if err := cm.createUsersCollection(); err != nil {
		return fmt.Errorf("users: %w", err)
	}
	return nil
}

func (cm *Manager) createAccountsCollection() error {
	if _, err := cm.app.FindCollectionByNameOrId(cm.options.AccountCollectionName); err == nil {
		return nil
	}

	c := core.NewBaseCollection(cm.options.AccountCollectionName)
	c.ListRule = nil
	c.ViewRule = nil
	c.CreateRule = nil
	c.UpdateRule = nil
	c.DeleteRule = nil

	c.Fields.Add(&core.TextField{Name: "name", Required: true, Max: 100})
	c.Fields.Add(&core.TextField{Name: "description", Max: 500})

	c.Fields.Add(&core.TextField{Name: "synadia_account_id", Max: 100})
	c.Fields.Add(&core.TextField{Name: "synadia_sk_group_id", Max: 100})
	c.Fields.Add(&core.TextField{Name: "public_key", Max: 200})

	c.Fields.Add(&core.BoolField{Name: "active"})

	// Limits — pass-through to Synadia. Min -1 to allow the pb-nats unlimited
	// convention even though Synadia treats 0/negative as unset.
	c.Fields.Add(&core.NumberField{Name: "max_connections", OnlyInt: true, Min: pbcore.Pointer(-1.0)})
	c.Fields.Add(&core.NumberField{Name: "max_subscriptions", OnlyInt: true, Min: pbcore.Pointer(-1.0)})
	c.Fields.Add(&core.NumberField{Name: "max_data", OnlyInt: true, Min: pbcore.Pointer(-1.0)})
	c.Fields.Add(&core.NumberField{Name: "max_payload", OnlyInt: true, Min: pbcore.Pointer(-1.0)})
	c.Fields.Add(&core.NumberField{Name: "max_jetstream_disk_storage", OnlyInt: true, Min: pbcore.Pointer(-1.0)})
	c.Fields.Add(&core.NumberField{Name: "max_jetstream_memory_storage", OnlyInt: true, Min: pbcore.Pointer(-1.0)})

	c.Fields.Add(&core.SelectField{
		Name:      "sync_state",
		MaxSelect: 1,
		Values: []string{
			pbtypes.SyncStateSynced,
			pbtypes.SyncStateCreating,
			pbtypes.SyncStatePendingCreate,
			pbtypes.SyncStatePendingUpdate,
		},
	})
	c.Fields.Add(&core.TextField{Name: "last_sync_error", Max: 1000})

	c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	c.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})

	return cm.app.Save(c)
}

func (cm *Manager) createRolesCollection() error {
	if _, err := cm.app.FindCollectionByNameOrId(cm.options.RoleCollectionName); err == nil {
		return nil
	}

	c := core.NewBaseCollection(cm.options.RoleCollectionName)
	c.ListRule = nil
	c.ViewRule = nil
	c.CreateRule = nil
	c.UpdateRule = nil
	c.DeleteRule = nil

	c.Fields.Add(&core.TextField{Name: "name", Required: true, Max: 100})
	c.Fields.Add(&core.TextField{Name: "description", Max: 500})
	c.Fields.Add(&core.BoolField{Name: "is_default"})

	c.Fields.Add(&core.JSONField{Name: "publish_permissions", MaxSize: 5000})
	c.Fields.Add(&core.JSONField{Name: "subscribe_permissions", MaxSize: 5000})
	c.Fields.Add(&core.JSONField{Name: "publish_deny_permissions", MaxSize: 5000})
	c.Fields.Add(&core.JSONField{Name: "subscribe_deny_permissions", MaxSize: 5000})

	c.Fields.Add(&core.BoolField{Name: "allow_response"})
	c.Fields.Add(&core.NumberField{Name: "allow_response_max", OnlyInt: true, Min: pbcore.Pointer(-1.0)})
	c.Fields.Add(&core.NumberField{Name: "allow_response_ttl", OnlyInt: true, Min: pbcore.Pointer(0.0)})

	c.Fields.Add(&core.NumberField{Name: "max_subscriptions", OnlyInt: true, Min: pbcore.Pointer(-1.0)})
	c.Fields.Add(&core.NumberField{Name: "max_data", OnlyInt: true, Min: pbcore.Pointer(-1.0)})
	c.Fields.Add(&core.NumberField{Name: "max_payload", OnlyInt: true, Min: pbcore.Pointer(-1.0)})

	c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	c.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})

	return cm.app.Save(c)
}

func (cm *Manager) createUsersCollection() error {
	if _, err := cm.app.FindCollectionByNameOrId(cm.options.UserCollectionName); err == nil {
		return nil
	}

	c := core.NewAuthCollection(cm.options.UserCollectionName)
	c.ListRule = nil
	c.ViewRule = nil
	c.CreateRule = nil
	c.UpdateRule = nil
	c.DeleteRule = nil

	c.Fields.Add(&core.TextField{Name: "nats_username", Required: true, Max: 100})
	c.Fields.Add(&core.TextField{Name: "description", Max: 500})

	if err := cm.app.Save(c); err != nil {
		return fmt.Errorf("save users (initial): %w", err)
	}

	accounts, err := cm.app.FindCollectionByNameOrId(cm.options.AccountCollectionName)
	if err != nil {
		return fmt.Errorf("accounts collection lookup: %w", err)
	}
	roles, err := cm.app.FindCollectionByNameOrId(cm.options.RoleCollectionName)
	if err != nil {
		return fmt.Errorf("roles collection lookup: %w", err)
	}

	c.Fields.Add(&core.RelationField{
		Name: "account_id", Required: true, MaxSelect: 1,
		CollectionId: accounts.Id, CascadeDelete: false,
	})
	c.Fields.Add(&core.RelationField{
		Name: "role_id", Required: true, MaxSelect: 1,
		CollectionId: roles.Id, CascadeDelete: false,
	})

	c.Fields.Add(&core.TextField{Name: "synadia_user_id", Max: 100})
	c.Fields.Add(&core.TextField{Name: "public_key", Max: 200})
	c.Fields.Add(&core.TextField{Name: "jwt", Max: 5000})
	c.Fields.Add(&core.TextField{Name: "creds_file", Max: 10000})

	c.Fields.Add(&core.BoolField{Name: "bearer_token"})
	c.Fields.Add(&core.BoolField{Name: "active"})
	c.Fields.Add(&core.BoolField{Name: "regenerate"})

	c.Fields.Add(&core.JSONField{Name: "publish_permissions", MaxSize: 5000})
	c.Fields.Add(&core.JSONField{Name: "subscribe_permissions", MaxSize: 5000})
	c.Fields.Add(&core.JSONField{Name: "publish_deny_permissions", MaxSize: 5000})
	c.Fields.Add(&core.JSONField{Name: "subscribe_deny_permissions", MaxSize: 5000})

	c.Fields.Add(&core.SelectField{
		Name:      "sync_state",
		MaxSelect: 1,
		Values: []string{
			pbtypes.SyncStateSynced,
			pbtypes.SyncStateCreating,
			pbtypes.SyncStatePendingCreate,
			pbtypes.SyncStatePendingUpdate,
		},
	})
	c.Fields.Add(&core.TextField{Name: "last_sync_error", Max: 1000})

	return cm.app.Save(c)
}
