package hooks

import (
	"fmt"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/skeeeon/pb-synadia/internal/synadia"
	pbtypes "github.com/skeeeon/pb-synadia/internal/types"
)

// SetupSubjectImportHooks registers create/update/delete hooks for the
// subject-imports collection. Mirrors the export hook pattern. See
// SetupSubjectExportHooks for the hook lifecycle contract; the only
// differences here are the collection name, the Synadia client methods,
// and the field set.
//
// An import depends on its owning (importing) account having a
// synadia_account_id. The `account` text field on the record names the
// EXPORTING account by public NKey — pb-synadia does not validate that the
// exporting account exists in PB, since it may live in a different
// deployment entirely.
func SetupSubjectImportHooks(app *pocketbase.PocketBase, deps *Deps) {
	app.OnRecordCreate(deps.Options.ImportCollectionName).BindFunc(func(e *core.RecordEvent) error {
		if !deps.shouldHandle(deps.Options.ImportCollectionName, pbtypes.EventTypeSubjectImportCreate) {
			return e.Next()
		}
		if err := pushSubjectImportCreate(app, deps, e.Record); err != nil {
			deps.Logger.Warning("subject import create on Synadia failed for %s: %v",
				e.Record.GetString("name"), err)
		}
		return e.Next()
	})

	app.OnRecordUpdate(deps.Options.ImportCollectionName).BindFunc(func(e *core.RecordEvent) error {
		if !deps.shouldHandle(deps.Options.ImportCollectionName, pbtypes.EventTypeSubjectImportUpdate) {
			return e.Next()
		}
		if e.Record.GetString("synadia_import_id") == "" {
			if err := pushSubjectImportCreate(app, deps, e.Record); err != nil {
				deps.Logger.Warning("subject import retry-create on Synadia failed for %s: %v",
					e.Record.Id, err)
			}
			return e.Next()
		}
		if !subjectImportSynadiaFieldsChanged(e.Record) {
			return e.Next()
		}
		if err := pushSubjectImportUpdate(deps, e.Record); err != nil {
			deps.Logger.Warning("subject import update on Synadia failed for %s: %v",
				e.Record.Id, err)
		}
		return e.Next()
	})

	app.OnRecordDelete(deps.Options.ImportCollectionName).BindFunc(func(e *core.RecordEvent) error {
		if !deps.shouldHandle(deps.Options.ImportCollectionName, pbtypes.EventTypeSubjectImportDelete) {
			return e.Next()
		}
		synadiaID := e.Record.GetString("synadia_import_id")
		if synadiaID == "" {
			return e.Next()
		}
		resp, err := deps.Client.DeleteSubjectImport(synadiaID)
		if err != nil && !synadia.IsNotFound(resp, err) {
			return fmt.Errorf("Synadia delete failed, aborting PB delete: %w", err)
		}
		deps.Logger.Delete("subject import %s removed from Synadia", e.Record.GetString("name"))
		return e.Next()
	})
}

func pushSubjectImportCreate(app *pocketbase.PocketBase, deps *Deps, rec *core.Record) error {
	synadiaAccountID, err := resolveSynadiaAccountID(app, deps, rec)
	if err != nil {
		markPending(rec, pbtypes.SyncStatePendingCreate, err.Error())
		return err
	}
	result, err := deps.Client.CreateSubjectImport(synadiaAccountID, recordToSubjectImportInput(rec))
	if err != nil {
		markPending(rec, pbtypes.SyncStatePendingCreate, err.Error())
		return err
	}
	rec.Set("synadia_import_id", result.ID)
	markSynced(rec)
	deps.Logger.Success("subject import %s created on Synadia (id=%s)", rec.GetString("name"), result.ID)
	return nil
}

func pushSubjectImportUpdate(deps *Deps, rec *core.Record) error {
	_, err := deps.Client.UpdateSubjectImport(rec.GetString("synadia_import_id"), recordToSubjectImportInput(rec))
	if err != nil {
		markPending(rec, pbtypes.SyncStatePendingUpdate, err.Error())
		return err
	}
	markSynced(rec)
	return nil
}

func recordToSubjectImportInput(rec *core.Record) synadia.SubjectImportInput {
	return synadia.SubjectImportInput{
		Name:         rec.GetString("name"),
		Subject:      rec.GetString("subject"),
		Type:         rec.GetString("type"),
		Account:      rec.GetString("account"),
		Token:        rec.GetString("token"),
		LocalSubject: rec.GetString("local_subject"),
		Share:        rec.GetBool("share"),
	}
}

func subjectImportSynadiaFieldsChanged(rec *core.Record) bool {
	orig := rec.Original()
	if orig.Id == "" {
		return true
	}
	for _, f := range []string{"name", "subject", "type", "account", "token", "local_subject"} {
		if orig.GetString(f) != rec.GetString(f) {
			return true
		}
	}
	if orig.GetBool("share") != rec.GetBool("share") {
		return true
	}
	return false
}
