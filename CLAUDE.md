# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A Go library (`github.com/skeeeon/pb-synadia`, v0.1.0) that extends [PocketBase](https://pocketbase.io/) to mirror NATS auth — accounts, users, roles — into [Synadia Cloud](https://cloud.synadia.com/). PocketBase CRUD on three collections triggers synchronous write-through hooks that call the Synadia REST API. Sister library to [pb-nats](https://github.com/skeeeon/pb-nats) (same conceptual surface, self-hosted backend).

The full design rationale — including why this library *doesn't* use Synadia signing key groups for roles, and the sync-state retry model — lives in [README.md](README.md). Read it once for context; the rest of this file covers what affects editing the code.

## Common commands

No Makefile. Standard Go tooling:

- `go build ./...` — compile everything (catches breakage in all packages)
- `go test ./...` — run all tests (currently only `internal/permissions/merger_test.go` exists)
- `go test ./internal/permissions/ -run TestMerge_UnionAndDenyPrecedence -v` — pattern for running a single test
- `go vet ./...`
- `go mod tidy`

Run the example PocketBase server (requires real Synadia credentials):

```powershell
$env:SYNADIA_SYSTEM_ID="..."; $env:SYNADIA_API_TOKEN="..."; go run ./examples/basic serve
```

Then `http://localhost:8090/_/` for the admin UI. Synadia's free Personal tier is enough for trial.

## Architecture

```
Public API (root .go files)
    └── re-exports internal/types so consumers don't touch internal/
internal/hooks/        PocketBase OnRecordCreate/Update/Delete bindings
    ├── internal/permissions/  role ∪ user-override merge
    └── internal/synadia/      single chokepoint over the syncp SDK
internal/collections/  one-shot schema bootstrap on PB BootstrapEvent
internal/types/        shared structs + sync_state / event-type constants
internal/utils/        logger
```

**Public surface lives only in root files** — [pb-synadia.go](pb-synadia.go), [options.go](options.go), [errors.go](errors.go). Everything else is `internal/` and not importable by consumers. The root files use Go type aliases (`type Options = pbtypes.Options`) so consumers never see the internal package — preserve that pattern when adding exports.

**`internal/synadia/` is a deliberate chokepoint.** Nothing else imports `github.com/synadia-io/control-plane-sdk-go/syncp`. The Synadia Control Plane API is at `/core/beta/...` and is subject to change — funnelling SDK calls through this one package keeps surface changes to a single-file edit. Preserve this.

## Hook contract (the non-obvious part)

PocketBase v0.38's `OnRecordCreate` / `OnRecordUpdate` fire **pre-save**; `OnRecordDelete` fires **pre-delete**. The library leans on this hard:

- **Create / Update**: hook calls Synadia, mutates the record (`synadia_account_id`, `creds_file`, etc.), then `e.Next()` persists everything in one PB save. **No recursion** — do not call `app.Save` inside these hooks for the record being saved.
- **Delete**: Synadia is called first. On failure the hook returns the error, aborting the PB delete to prevent orphaned Synadia resources. A 404 from Synadia (`synadia.IsNotFound`) is treated as success.
- **Failure of a create/update is non-fatal to the PB save.** The hook calls `markPending(rec, state, errMsg)` to set `sync_state` and `last_sync_error`, logs a warning, and still calls `e.Next()`. The record lands in `pending_create` / `pending_update`. Today, re-saving the record retries; in v0.2 `Reconcile` will retry without a manual save.
- **Update hooks short-circuit no-op pushes** via `*SynadiaFieldsChanged` helpers (`accountSynadiaFieldsChanged` in [internal/hooks/accounts.go](internal/hooks/accounts.go), `userSynadiaFieldsChanged` in [internal/hooks/users.go](internal/hooks/users.go)). When you add a new field that gets pushed to Synadia, extend these.
- **`Options.EventFilter`** lets consumers opt out per `(collectionName, eventType)`. Every hook starts with `if !deps.shouldHandle(...) { return e.Next() }`. Event-type constants are in [internal/types/types.go](internal/types/types.go).
- **Reconcile saves records outside the hook lifecycle.** [internal/hooks/reconcile.go](internal/hooks/reconcile.go) calls the same `pushAccountCreate` / `pushUserCreate` / `pushUserUpdate` helpers and then `app.Save(rec)` itself. That `Save` re-fires `OnRecordUpdate`. The `*SynadiaFieldsChanged` short-circuits are what prevent a redundant Synadia call — if you add a new Synadia-pushed field and forget to extend those helpers, Reconcile will start double-pushing.

## Sync-state contract

Every `nats_accounts` and `nats_users` record carries `sync_state` ∈ {`synced`, `creating`, `pending_create`, `pending_update`}. Constants live in [internal/types/types.go](internal/types/types.go) and are re-exported as `SyncStateSynced` etc. These are load-bearing — v0.2's `Reconcile` will scan `pending_*` to retry. Don't repurpose them or add ad-hoc states.

## Permissions merge

[internal/permissions/merger.go](internal/permissions/merger.go) produces the inline permissions Synadia sees: union of role + user-override allow-lists, union of deny-lists, defaults (`Options.DefaultPublishPermissions` / `DefaultSubscribePermissions`) applied per-direction *only* when both role and user lists are empty. Synadia never sees the role concept — only the merged result per user. The contract is exercised by [merger_test.go](internal/permissions/merger_test.go).

## Constraints and gotchas

These only become obvious after reading several files together — observe them:

- **`nats_username` is the natural correlation key** with Synadia: immutable and unique per account. Don't add code paths that mutate or rename it post-create.
- **Synadia signing key groups are NOT used as the role mapping.** v0.1 auto-creates one default sk group per account (`pb-synadia-default`, marked `Programmatic: true`) just to satisfy Synadia's "every user must belong to a group" rule. `EnsureDefaultSkGroup` resolves it lazily. The README section "Why inline merge, not Synadia signing key groups" explains why scoped groups don't fit a pb-nats-style mutable-role model.
- **Limit semantics differ from pb-nats.** pb-nats: `0 = disabled (block)`, `-1 = unlimited`. Synadia: `0` or negative = unset. pb-synadia forwards positives and treats `0` as "leave unset." Don't add code that tries to translate `0` to a Synadia "block" — that mode doesn't exist.
- **Role updates do NOT cascade in v0.1.** [internal/hooks/roles.go](internal/hooks/roles.go) only logs a warning. Cascade is a deliberate v0.2 milestone alongside `Reconcile` — don't add a partial implementation; the milestone boundary matters.
- **Collections are created with `nil` API rules** by default. The consuming app must grant access explicitly. Don't change the defaults in [internal/collections/manager.go](internal/collections/manager.go).
- **`regenerate: true` on a user is destructive and one-shot.** The update hook calls `NatsUserAPI.RotateNatsUser` (via `rotateUserKeys` in [internal/hooks/users.go](internal/hooks/users.go)) to rotate the user's nkey + seed, then downloads fresh creds. Any previously distributed `.creds` file stops working immediately. The flag auto-resets to `false`. Do not change this back to a plain re-download — the field name promises rotation.

## Roadmap pointers

If asked to implement work that touches:
- **`pending_*` retry** → shipped in v0.2.1 via `Reconcile`; extending or rewriting it lands in [internal/hooks/reconcile.go](internal/hooks/reconcile.go)
- **drift detection** (PB-known `synadia_*_id` no longer on Synadia) or **orphan adoption** (Synadia resource unknown to PB, stitched by `name` / `nats_username`) → v0.2 milestone, not yet started; see README "Roadmap"
- **role-update cascade to all users in a role** → v0.2 milestone; the easy path once retry exists is to mark affected users as `pending_update` in the role hook and let the next Reconcile pass push them
- **cross-account exports/imports** → v0.3 (collections deferred until SDK surface is verified)
- **scoped-group role strategy** → v0.4 speculative; ships only after group-update cascade behavior is verified by integration test

## Critical files for any non-trivial change

- [pb-synadia.go](pb-synadia.go) — `Setup`, type re-exports
- [options.go](options.go) — `Options` + `DefaultOptions` + `applyDefaultOptions`
- [errors.go](errors.go) — typed errors, `IsTemporaryError` / `IsPermanentError` / `IsConfigurationError`
- [internal/types/types.go](internal/types/types.go) — record structs, sync-state and event-type constants
- [internal/hooks/common.go](internal/hooks/common.go) — `Deps`, `markPending`, `markSynced`, `shouldHandle`
- [internal/hooks/accounts.go](internal/hooks/accounts.go), [internal/hooks/users.go](internal/hooks/users.go), [internal/hooks/roles.go](internal/hooks/roles.go) — the hook bindings themselves
- [internal/hooks/reconcile.go](internal/hooks/reconcile.go), [reconcile.go](reconcile.go) — periodic retry loop for `pending_*` records
- [internal/synadia/client.go](internal/synadia/client.go) — SDK chokepoint
- [internal/permissions/merger.go](internal/permissions/merger.go) — merge algorithm
- [internal/collections/manager.go](internal/collections/manager.go) — collection schema bootstrap
