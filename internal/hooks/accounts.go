package hooks

import (
	"fmt"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/skeeeon/pb-synadia/internal/synadia"
	pbtypes "github.com/skeeeon/pb-synadia/internal/types"
)

// SetupAccountHooks registers create/update/delete hooks for the accounts
// collection. Create + update fire BEFORE the PB save so we can mutate the
// record (synadia_account_id, sync_state, etc.) and have it persist in a
// single transaction — no recursion. Delete is pre-delete so a Synadia
// failure aborts the PB delete and prevents orphaned Synadia resources.
func SetupAccountHooks(app *pocketbase.PocketBase, deps *Deps) {
	app.OnRecordCreate(deps.Options.AccountCollectionName).BindFunc(func(e *core.RecordEvent) error {
		if !deps.shouldHandle(deps.Options.AccountCollectionName, pbtypes.EventTypeAccountCreate) {
			return e.Next()
		}
		if err := pushAccountCreate(deps, e.Record); err != nil {
			deps.Logger.Warning("account create on Synadia failed for %s: %v", e.Record.GetString("name"), err)
			// Fall through — record persists in pending_create state for Reconcile to retry.
		}
		return e.Next()
	})

	app.OnRecordUpdate(deps.Options.AccountCollectionName).BindFunc(func(e *core.RecordEvent) error {
		if !deps.shouldHandle(deps.Options.AccountCollectionName, pbtypes.EventTypeAccountUpdate) {
			return e.Next()
		}
		if e.Record.GetString("synadia_account_id") == "" {
			// Never made it to Synadia. Try create on next save.
			if err := pushAccountCreate(deps, e.Record); err != nil {
				deps.Logger.Warning("account retry-create on Synadia failed for %s: %v", e.Record.Id, err)
			}
			return e.Next()
		}
		if !accountSynadiaFieldsChanged(e.Record) {
			return e.Next()
		}
		if err := pushAccountUpdate(deps, e.Record); err != nil {
			deps.Logger.Warning("account update on Synadia failed for %s: %v", e.Record.Id, err)
		}
		return e.Next()
	})

	app.OnRecordDelete(deps.Options.AccountCollectionName).BindFunc(func(e *core.RecordEvent) error {
		if !deps.shouldHandle(deps.Options.AccountCollectionName, pbtypes.EventTypeAccountDelete) {
			return e.Next()
		}
		synadiaID := e.Record.GetString("synadia_account_id")
		if synadiaID == "" {
			return e.Next()
		}
		resp, err := deps.Client.DeleteAccount(synadiaID)
		if err != nil && !synadia.IsNotFound(resp, err) {
			return fmt.Errorf("Synadia delete failed, aborting PB delete: %w", err)
		}
		deps.Logger.Delete("account %s removed from Synadia", e.Record.GetString("name"))
		return e.Next()
	})
}

func pushAccountCreate(deps *Deps, rec *core.Record) error {
	acc := recordToAccount(rec)
	result, err := deps.Client.CreateAccount(&acc)
	if err != nil {
		markPending(rec, pbtypes.SyncStatePendingCreate, err.Error())
		return err
	}
	rec.Set("synadia_account_id", result.ID)
	rec.Set("public_key", result.PublicKey)

	// Resolve the unscoped sk group used for users. Failure here doesn't
	// undo the account create — we leave the account synced and let the
	// next user write or Reconcile resolve the group.
	if skGroupID, gerr := deps.Client.EnsureDefaultSkGroup(result.ID); gerr == nil {
		rec.Set("synadia_sk_group_id", skGroupID)
	} else {
		deps.Logger.Warning("default sk group resolution failed for account %s: %v", result.ID, gerr)
	}

	markSynced(rec)
	deps.Logger.Success("account %s created on Synadia (id=%s)", rec.GetString("name"), result.ID)
	return nil
}

func pushAccountUpdate(deps *Deps, rec *core.Record) error {
	acc := recordToAccount(rec)
	if _, err := deps.Client.UpdateAccount(&acc); err != nil {
		markPending(rec, pbtypes.SyncStatePendingUpdate, err.Error())
		return err
	}
	markSynced(rec)
	return nil
}

// accountSynadiaFieldsChanged reports whether any field that pb-synadia
// pushes to Synadia (name, description, limits) differs from the record's
// original (loaded) state. Used by the update hook to skip no-op pushes.
func accountSynadiaFieldsChanged(rec *core.Record) bool {
	orig := rec.Original()
	if orig.Id == "" {
		// New record being created — treat as changed.
		return true
	}
	if orig.GetString("name") != rec.GetString("name") ||
		orig.GetString("description") != rec.GetString("description") {
		return true
	}
	for _, f := range []string{
		"max_connections", "max_subscriptions", "max_data", "max_payload",
		"max_jetstream_disk_storage", "max_jetstream_memory_storage",
	} {
		if orig.GetInt(f) != rec.GetInt(f) {
			return true
		}
	}
	return false
}

func recordToAccount(rec *core.Record) pbtypes.AccountRecord {
	return pbtypes.AccountRecord{
		ID:                        rec.Id,
		Name:                      rec.GetString("name"),
		Description:               rec.GetString("description"),
		SynadiaAccountID:          rec.GetString("synadia_account_id"),
		SynadiaSkGroupID:          rec.GetString("synadia_sk_group_id"),
		PublicKey:                 rec.GetString("public_key"),
		Active:                    rec.GetBool("active"),
		MaxConnections:            int64FromRec(rec, "max_connections"),
		MaxSubscriptions:          int64FromRec(rec, "max_subscriptions"),
		MaxData:                   int64FromRec(rec, "max_data"),
		MaxPayload:                int64FromRec(rec, "max_payload"),
		MaxJetStreamDiskStorage:   int64FromRec(rec, "max_jetstream_disk_storage"),
		MaxJetStreamMemoryStorage: int64FromRec(rec, "max_jetstream_memory_storage"),
	}
}

func int64FromRec(rec *core.Record, field string) int64 {
	return int64(rec.GetInt(field))
}
