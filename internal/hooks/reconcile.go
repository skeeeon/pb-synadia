package hooks

import (
	"context"
	"sync"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	pbtypes "github.com/skeeeon/pb-synadia/internal/types"
)

// reconcileMu serializes Reconcile passes. Overlapping invocations skip
// rather than running in parallel — see RunReconcile.
var reconcileMu sync.Mutex

// RunReconcile is the entry point called from the package-root Reconcile.
//
// It scans accounts then users in pending_* sync states and retries the
// existing push helpers (pushAccountCreate / pushAccountUpdate /
// pushUserCreate / pushUserUpdate). Records that succeed are marked synced;
// records that still fail are left in pending_* for the next pass.
//
// Per-record failures are swallowed and logged — the whole point of the
// loop is that pending_* records get another shot. Only setup-level
// errors (the filter query failing, context cancellation) propagate.
//
// Reconcile saves records via app.Save, which re-fires OnRecordUpdate.
// The existing *SynadiaFieldsChanged short-circuits in accounts.go and
// users.go prevent that re-fire from making redundant Synadia calls.
func RunReconcile(app *pocketbase.PocketBase, deps *Deps, ctx context.Context) error {
	if !reconcileMu.TryLock() {
		deps.Logger.Info("reconcile already in progress, skipping")
		return nil
	}
	defer reconcileMu.Unlock()

	deps.Logger.Start("reconcile pass started")
	if err := reconcileAccounts(app, deps, ctx); err != nil {
		return err
	}
	if err := reconcileUsers(app, deps, ctx); err != nil {
		return err
	}
	if err := reconcileSubjectExports(app, deps, ctx); err != nil {
		return err
	}
	if err := reconcileSubjectImports(app, deps, ctx); err != nil {
		return err
	}
	deps.Logger.Success("reconcile pass complete")
	return nil
}

func reconcileAccounts(app *pocketbase.PocketBase, deps *Deps, ctx context.Context) error {
	recs, err := app.FindRecordsByFilter(
		deps.Options.AccountCollectionName,
		"sync_state = {:c} || sync_state = {:u}",
		"", 0, 0,
		dbx.Params{
			"c": pbtypes.SyncStatePendingCreate,
			"u": pbtypes.SyncStatePendingUpdate,
		},
	)
	if err != nil {
		return err
	}
	for _, rec := range recs {
		if err := ctx.Err(); err != nil {
			return err
		}
		switch rec.GetString("sync_state") {
		case pbtypes.SyncStatePendingCreate:
			_ = pushAccountCreate(deps, rec)
		case pbtypes.SyncStatePendingUpdate:
			_ = pushAccountUpdate(deps, rec)
		}
		if saveErr := app.Save(rec); saveErr != nil {
			deps.Logger.Warning("reconcile: save account %s failed: %v", rec.Id, saveErr)
		}
		sleepCallDelay(deps)
	}
	return nil
}

func reconcileUsers(app *pocketbase.PocketBase, deps *Deps, ctx context.Context) error {
	recs, err := app.FindRecordsByFilter(
		deps.Options.UserCollectionName,
		"sync_state = {:c} || sync_state = {:u}",
		"", 0, 0,
		dbx.Params{
			"c": pbtypes.SyncStatePendingCreate,
			"u": pbtypes.SyncStatePendingUpdate,
		},
	)
	if err != nil {
		return err
	}
	for _, rec := range recs {
		if err := ctx.Err(); err != nil {
			return err
		}
		switch rec.GetString("sync_state") {
		case pbtypes.SyncStatePendingCreate:
			_ = pushUserCreate(app, deps, rec)
		case pbtypes.SyncStatePendingUpdate:
			_ = pushUserUpdate(app, deps, rec)
		}
		if saveErr := app.Save(rec); saveErr != nil {
			deps.Logger.Warning("reconcile: save user %s failed: %v", rec.Id, saveErr)
		}
		sleepCallDelay(deps)
	}
	return nil
}

func reconcileSubjectExports(app *pocketbase.PocketBase, deps *Deps, ctx context.Context) error {
	recs, err := app.FindRecordsByFilter(
		deps.Options.ExportCollectionName,
		"sync_state = {:c} || sync_state = {:u}",
		"", 0, 0,
		dbx.Params{
			"c": pbtypes.SyncStatePendingCreate,
			"u": pbtypes.SyncStatePendingUpdate,
		},
	)
	if err != nil {
		return err
	}
	for _, rec := range recs {
		if err := ctx.Err(); err != nil {
			return err
		}
		switch rec.GetString("sync_state") {
		case pbtypes.SyncStatePendingCreate:
			_ = pushSubjectExportCreate(app, deps, rec)
		case pbtypes.SyncStatePendingUpdate:
			_ = pushSubjectExportUpdate(deps, rec)
		}
		if saveErr := app.Save(rec); saveErr != nil {
			deps.Logger.Warning("reconcile: save subject export %s failed: %v", rec.Id, saveErr)
		}
		sleepCallDelay(deps)
	}
	return nil
}

func reconcileSubjectImports(app *pocketbase.PocketBase, deps *Deps, ctx context.Context) error {
	recs, err := app.FindRecordsByFilter(
		deps.Options.ImportCollectionName,
		"sync_state = {:c} || sync_state = {:u}",
		"", 0, 0,
		dbx.Params{
			"c": pbtypes.SyncStatePendingCreate,
			"u": pbtypes.SyncStatePendingUpdate,
		},
	)
	if err != nil {
		return err
	}
	for _, rec := range recs {
		if err := ctx.Err(); err != nil {
			return err
		}
		switch rec.GetString("sync_state") {
		case pbtypes.SyncStatePendingCreate:
			_ = pushSubjectImportCreate(app, deps, rec)
		case pbtypes.SyncStatePendingUpdate:
			_ = pushSubjectImportUpdate(deps, rec)
		}
		if saveErr := app.Save(rec); saveErr != nil {
			deps.Logger.Warning("reconcile: save subject import %s failed: %v", rec.Id, saveErr)
		}
		sleepCallDelay(deps)
	}
	return nil
}

func sleepCallDelay(deps *Deps) {
	if deps.Options.SynadiaCallDelay > 0 {
		time.Sleep(deps.Options.SynadiaCallDelay)
	}
}
