// Package pbsynadia integrates PocketBase with Synadia Cloud for managed
// NATS authentication.
//
// Synadia Cloud is the source of truth. PocketBase mirrors accounts, users,
// and roles via collections. Hooks call the Synadia REST API on every PB
// write (create/update/delete). Roles live entirely in PocketBase; user
// permissions are merged (role ∪ user override, deny precedence) and pushed
// inline to Synadia.
//
// See package pb-nats for a sister library that manages a self-hosted NATS
// server instead.
package pbsynadia

import (
	"fmt"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/skeeeon/pb-synadia/internal/collections"
	"github.com/skeeeon/pb-synadia/internal/hooks"
	"github.com/skeeeon/pb-synadia/internal/synadia"
	pbtypes "github.com/skeeeon/pb-synadia/internal/types"
	"github.com/skeeeon/pb-synadia/internal/utils"
)

// Re-exports for external use so consumers don't import internal/types.
type (
	Options             = pbtypes.Options
	AccountRecord       = pbtypes.AccountRecord
	NatsUserRecord      = pbtypes.NatsUserRecord
	RoleRecord          = pbtypes.RoleRecord
	MergedPermissions   = pbtypes.MergedPermissions
)

const (
	DefaultAccountCollectionName = pbtypes.DefaultAccountCollectionName
	DefaultUserCollectionName    = pbtypes.DefaultUserCollectionName
	DefaultRoleCollectionName    = pbtypes.DefaultRoleCollectionName
	DefaultExportCollectionName  = pbtypes.DefaultExportCollectionName
	DefaultImportCollectionName  = pbtypes.DefaultImportCollectionName
	DefaultSynadiaBaseURL        = pbtypes.DefaultSynadiaBaseURL

	SyncStateSynced        = pbtypes.SyncStateSynced
	SyncStateCreating      = pbtypes.SyncStateCreating
	SyncStatePendingCreate = pbtypes.SyncStatePendingCreate
	SyncStatePendingUpdate = pbtypes.SyncStatePendingUpdate
)

var (
	DefaultPublishPermissions   = pbtypes.DefaultPublishPermissions
	DefaultSubscribePermissions = pbtypes.DefaultSubscribePermissions
)

// Version is the library version.
const Version = "0.1.0"

// Setup initializes Synadia Cloud sync for a PocketBase instance.
//
// On bootstrap, the 5 collections are created if missing (accounts, roles,
// users; imports/exports deferred to v0.3) and PocketBase hooks are wired
// to push changes to Synadia synchronously.
func Setup(app *pocketbase.PocketBase, options Options) error {
	options = applyDefaultOptions(options)
	logger := utils.NewLogger(options.LogToConsole)

	if err := validateOptions(options); err != nil {
		return fmt.Errorf("invalid options: %w", err)
	}

	app.OnBootstrap().BindFunc(func(e *core.BootstrapEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		if err := initializeComponents(app, options, logger); err != nil {
			logger.Error("pb-synadia init failed: %v", err)
			return err
		}
		logger.Success("pb-synadia ready")
		return nil
	})

	logger.Start("pb-synadia scheduled for initialization")
	logger.Info("   System ID: %s", options.SystemID)
	logger.Info("   Synadia URL: %s", options.SynadiaBaseURL)
	logger.Info("   Account collection: %s", options.AccountCollectionName)
	logger.Info("   User collection: %s", options.UserCollectionName)
	logger.Info("   Role collection: %s", options.RoleCollectionName)

	return nil
}

func initializeComponents(app *pocketbase.PocketBase, options Options, logger *utils.Logger) error {
	logger.Process("Initializing pb-synadia components...")

	logger.Info("   Creating collections...")
	cm := collections.NewManager(app, options)
	if err := cm.InitializeCollections(); err != nil {
		return fmt.Errorf("init collections: %w", err)
	}
	logger.Success("   Collections ready")

	client := synadia.NewClient(options.SystemID, options.APIToken, options.SynadiaBaseURL)
	deps := &hooks.Deps{
		Client:  client,
		Options: options,
		Logger:  logger,
	}

	hooks.SetupAccountHooks(app, deps)
	hooks.SetupUserHooks(app, deps)
	hooks.SetupRoleHooks(app, deps)
	logger.Success("   Hooks registered")

	return nil
}

func validateOptions(options Options) error {
	if options.SystemID == "" {
		return ErrMissingSystem
	}
	if options.APIToken == "" {
		return ErrMissingToken
	}
	if options.AccountCollectionName == "" {
		return fmt.Errorf("%w: account collection name", ErrInvalidOptions)
	}
	if options.UserCollectionName == "" {
		return fmt.Errorf("%w: user collection name", ErrInvalidOptions)
	}
	if options.RoleCollectionName == "" {
		return fmt.Errorf("%w: role collection name", ErrInvalidOptions)
	}
	return nil
}
