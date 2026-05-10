package pbsynadia

import (
	"context"
	"fmt"

	"github.com/pocketbase/pocketbase"
	"github.com/skeeeon/pb-synadia/internal/hooks"
	"github.com/skeeeon/pb-synadia/internal/synadia"
	"github.com/skeeeon/pb-synadia/internal/utils"
)

// Reconcile scans accounts and users in pending_* sync states and retries
// their Synadia push. Records that succeed are marked synced; records that
// still fail are left in pending_* for the next run.
//
// Accounts are processed before users so a user whose account just got its
// synadia_account_id resolved on this pass can succeed on the same pass.
//
// Wire to app.Cron() at whatever cadence suits the deployment:
//
//	app.Cron().MustAdd("pb-synadia-reconcile", "*/5 * * * *", func() {
//	    _ = pbsynadia.Reconcile(app, opts, context.Background())
//	})
//
// Reconcile is safe to call concurrently — overlapping invocations skip
// with a log line rather than running in parallel. Per-record push
// failures are swallowed (logged via the configured Logger) so the caller
// doesn't need to handle them; the records stay in pending_* and the next
// pass retries.
//
// Only setup-level failures (invalid options, the filter query itself
// failing, ctx cancellation mid-pass) are returned.
func Reconcile(app *pocketbase.PocketBase, options Options, ctx context.Context) error {
	options = applyDefaultOptions(options)
	if err := validateOptions(options); err != nil {
		return fmt.Errorf("invalid options: %w", err)
	}
	logger := utils.NewLogger(options.LogToConsole)
	client := synadia.NewClient(options.SystemID, options.APIToken, options.SynadiaBaseURL)
	deps := &hooks.Deps{
		Client:  client,
		Options: options,
		Logger:  logger,
	}
	return hooks.RunReconcile(app, deps, ctx)
}
