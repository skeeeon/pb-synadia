// Package hooks registers PocketBase event hooks that mirror PB record
// changes into Synadia Cloud via the synadia client adapter.
package hooks

import (
	"github.com/pocketbase/pocketbase/core"
	"github.com/skeeeon/pb-synadia/internal/synadia"
	pbtypes "github.com/skeeeon/pb-synadia/internal/types"
	"github.com/skeeeon/pb-synadia/internal/utils"
)

// Deps bundles everything hooks need to do their job.
type Deps struct {
	Client  *synadia.Client
	Options pbtypes.Options
	Logger  *utils.Logger
}

// shouldHandle applies the optional EventFilter from Options.
func (d *Deps) shouldHandle(collectionName, eventType string) bool {
	if d.Options.EventFilter == nil {
		return true
	}
	return d.Options.EventFilter(collectionName, eventType)
}

// markPending marks a record as awaiting Synadia retry. Caller is responsible
// for calling app.Save afterward.
func markPending(rec *core.Record, state, errMsg string) {
	rec.Set("sync_state", state)
	rec.Set("last_sync_error", errMsg)
}

// markSynced marks a record as fully synchronized.
func markSynced(rec *core.Record) {
	rec.Set("sync_state", pbtypes.SyncStateSynced)
	rec.Set("last_sync_error", "")
}
