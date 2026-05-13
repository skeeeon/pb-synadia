package hooks

import (
	"fmt"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/skeeeon/pb-synadia/internal/synadia"
	pbtypes "github.com/skeeeon/pb-synadia/internal/types"
)

// SetupSubjectExportHooks registers create/update/delete hooks for the
// subject-exports collection. The hook flow mirrors the accounts/users
// pattern: pre-save call to Synadia → mutate record with returned id and
// sync state → e.Next() persists in one save. Delete is pre-delete and
// aborts on Synadia failure (other than 404).
//
// A subject export depends on its owning account having a
// synadia_account_id. If the parent account is still pending_*, the
// export is marked pending_create — Reconcile picks it up after the
// account succeeds.
func SetupSubjectExportHooks(app *pocketbase.PocketBase, deps *Deps) {
	app.OnRecordCreate(deps.Options.ExportCollectionName).BindFunc(func(e *core.RecordEvent) error {
		if !deps.shouldHandle(deps.Options.ExportCollectionName, pbtypes.EventTypeSubjectExportCreate) {
			return e.Next()
		}
		if err := pushSubjectExportCreate(app, deps, e.Record); err != nil {
			deps.Logger.Warning("subject export create on Synadia failed for %s: %v",
				e.Record.GetString("name"), err)
		}
		return e.Next()
	})

	app.OnRecordUpdate(deps.Options.ExportCollectionName).BindFunc(func(e *core.RecordEvent) error {
		if !deps.shouldHandle(deps.Options.ExportCollectionName, pbtypes.EventTypeSubjectExportUpdate) {
			return e.Next()
		}
		if e.Record.GetString("synadia_export_id") == "" {
			// Never made it to Synadia. Retry as create.
			if err := pushSubjectExportCreate(app, deps, e.Record); err != nil {
				deps.Logger.Warning("subject export retry-create on Synadia failed for %s: %v",
					e.Record.Id, err)
			}
			return e.Next()
		}
		if !subjectExportSynadiaFieldsChanged(e.Record) {
			return e.Next()
		}
		if err := pushSubjectExportUpdate(deps, e.Record); err != nil {
			deps.Logger.Warning("subject export update on Synadia failed for %s: %v",
				e.Record.Id, err)
		}
		return e.Next()
	})

	app.OnRecordDelete(deps.Options.ExportCollectionName).BindFunc(func(e *core.RecordEvent) error {
		if !deps.shouldHandle(deps.Options.ExportCollectionName, pbtypes.EventTypeSubjectExportDelete) {
			return e.Next()
		}
		synadiaID := e.Record.GetString("synadia_export_id")
		if synadiaID == "" {
			return e.Next()
		}
		resp, err := deps.Client.DeleteSubjectExport(synadiaID)
		if err != nil && !synadia.IsNotFound(resp, err) {
			return fmt.Errorf("Synadia delete failed, aborting PB delete: %w", err)
		}
		deps.Logger.Delete("subject export %s removed from Synadia", e.Record.GetString("name"))
		return e.Next()
	})
}

func pushSubjectExportCreate(app *pocketbase.PocketBase, deps *Deps, rec *core.Record) error {
	synadiaAccountID, err := resolveSynadiaAccountID(app, deps, rec)
	if err != nil {
		markPending(rec, pbtypes.SyncStatePendingCreate, err.Error())
		return err
	}
	result, err := deps.Client.CreateSubjectExport(synadiaAccountID, recordToSubjectExportInput(rec))
	if err != nil {
		markPending(rec, pbtypes.SyncStatePendingCreate, err.Error())
		return err
	}
	rec.Set("synadia_export_id", result.ID)
	markSynced(rec)
	deps.Logger.Success("subject export %s created on Synadia (id=%s)", rec.GetString("name"), result.ID)
	return nil
}

func pushSubjectExportUpdate(deps *Deps, rec *core.Record) error {
	_, err := deps.Client.UpdateSubjectExport(rec.GetString("synadia_export_id"), recordToSubjectExportInput(rec))
	if err != nil {
		markPending(rec, pbtypes.SyncStatePendingUpdate, err.Error())
		return err
	}
	markSynced(rec)
	return nil
}

// resolveSynadiaAccountID loads the parent account record and returns its
// synadia_account_id, or an error suitable for stamping into last_sync_error.
// Used by both the export and import hooks.
func resolveSynadiaAccountID(app *pocketbase.PocketBase, deps *Deps, rec *core.Record) (string, error) {
	accountID := rec.GetString("account_id")
	if accountID == "" {
		return "", fmt.Errorf("missing account_id on record %q", rec.Id)
	}
	accRec, err := app.FindRecordById(deps.Options.AccountCollectionName, accountID)
	if err != nil {
		return "", fmt.Errorf("find account %q: %w", accountID, err)
	}
	synadiaAccountID := accRec.GetString("synadia_account_id")
	if synadiaAccountID == "" {
		return "", fmt.Errorf("account %q has no synadia_account_id yet", accRec.GetString("name"))
	}
	return synadiaAccountID, nil
}

func recordToSubjectExportInput(rec *core.Record) synadia.SubjectExportInput {
	return synadia.SubjectExportInput{
		Name:                 rec.GetString("name"),
		Subject:              rec.GetString("subject"),
		Type:                 rec.GetString("type"),
		TokenReq:             rec.GetBool("token_req"),
		ResponseType:         rec.GetString("response_type"),
		ResponseThreshold:    int64FromRec(rec, "response_threshold"),
		AccountTokenPosition: int64FromRec(rec, "account_token_position"),
		Advertise:            rec.GetBool("advertise"),
		Description:          rec.GetString("description"),
	}
}

// subjectExportSynadiaFieldsChanged short-circuits no-op updates by comparing
// the user-facing Synadia-relevant fields against the record's original
// loaded state. account_id is excluded — Synadia exports are bound to the
// account at create time and cannot be reparented.
func subjectExportSynadiaFieldsChanged(rec *core.Record) bool {
	orig := rec.Original()
	if orig.Id == "" {
		return true
	}
	for _, f := range []string{"name", "subject", "type", "response_type", "description"} {
		if orig.GetString(f) != rec.GetString(f) {
			return true
		}
	}
	if orig.GetBool("token_req") != rec.GetBool("token_req") ||
		orig.GetBool("advertise") != rec.GetBool("advertise") {
		return true
	}
	for _, f := range []string{"response_threshold", "account_token_position"} {
		if orig.GetInt(f) != rec.GetInt(f) {
			return true
		}
	}
	return false
}
