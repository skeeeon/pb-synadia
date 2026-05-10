package hooks

import (
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	pbtypes "github.com/skeeeon/pb-synadia/internal/types"
)

// SetupRoleHooks registers minimal role hooks for v0.1.
//
// v0.1 deliberately does NOT cascade role updates to all users in the role.
// That cascade is part of the v0.2 milestone (along with Reconcile and the
// pending_update retry loop). For now, role permission changes only take
// effect on subsequent user writes that touch the role's users.
//
// Users who want immediate cascade in v0.1 can manually update each user
// record to trigger a Synadia push.
func SetupRoleHooks(app *pocketbase.PocketBase, deps *Deps) {
	app.OnRecordUpdate(deps.Options.RoleCollectionName).BindFunc(func(e *core.RecordEvent) error {
		if !deps.shouldHandle(deps.Options.RoleCollectionName, pbtypes.EventTypeRoleUpdate) {
			return e.Next()
		}
		// Soft warning only — no Synadia push happens here in v0.1.
		deps.Logger.Info("role %s updated; users with this role keep their existing Synadia permissions until next user write (v0.2 will cascade)",
			e.Record.GetString("name"))
		return e.Next()
	})
}
