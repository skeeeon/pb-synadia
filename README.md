# PocketBase + Synadia Cloud

A Go library for extending [PocketBase](https://pocketbase.io/) to manage NATS authentication on [Synadia Cloud](https://cloud.synadia.com/). Mirror PocketBase records — accounts, users, roles — to Synadia's managed NATS service. Hooks call the Synadia REST API on every write; the user record's `creds_file` field is populated automatically for self-service download.

Sister library to [pb-nats](https://github.com/skeeeon/pb-nats), which manages a self-hosted NATS server instead. Same conceptual surface (accounts/users/roles/creds_file), different backend.

> **Disclaimer:** This is an independent, community-maintained project. It is not affiliated with, endorsed by, or sponsored by Synadia Communications, Inc. "Synadia" and "Synadia Cloud" are trademarks of their respective owners and are used here only to describe interoperability.

## Status

**v0.3** — accounts, users, roles, and cross-account subject exports/imports work end-to-end against Synadia Cloud. `Reconcile(app, opts, ctx)` retries `pending_*` records on a cron.

Still to come:
- Drift detection (PB-known `synadia_*_id` no longer on Synadia) and orphan adoption (Synadia resource unknown to PB, stitched by `name` / `nats_username`) — still v0.2 milestone.
- Role-update cascade — changing a role's permissions does not push to Synadia for users in that role until those user records are individually written (v0.2).
- JetStream stream sharing via Synadia's separate `StreamExportAPI` / `StreamImportAPI` (v0.4+).
- No CLI commands.

See [Roadmap](#roadmap).

## Key Features

- **Synadia Cloud as source of truth.** PocketBase mirrors. No operator-key custody, no bootstrap dance, no `$SYS.REQ.CLAIMS.UPDATE` plumbing.
- **Synchronous write-through.** Every PB create/update/delete is one HTTP call to Synadia inside the request lifecycle. Synadia failures on delete block the PB delete to prevent orphaned resources.
- **Roles stay PB-side.** `nats_roles` is a permission template applied via union merge with per-user overrides; pushed inline to Synadia as unscoped user permissions. Synadia never sees the role concept.
- **Sync state on every record.** `sync_state` (`synced` / `creating` / `pending_create` / `pending_update`) plus `last_sync_error` make the mirror self-describing — the field doubles as a retry queue when v0.2 ships `Reconcile()`.
- **Locked-down defaults.** All collections have `nil` API rules — the consuming app explicitly grants access.

## Installation

```bash
go get github.com/skeeeon/pb-synadia
```

Requires Go 1.25+ and PocketBase v0.38+.

## Quick Start

```go
package main

import (
    "log"
    "os"

    "github.com/pocketbase/pocketbase"
    pbsynadia "github.com/skeeeon/pb-synadia"
)

func main() {
    app := pocketbase.New()

    opts := pbsynadia.DefaultOptions()
    opts.SystemID = os.Getenv("SYNADIA_SYSTEM_ID")
    opts.APIToken = os.Getenv("SYNADIA_API_TOKEN")

    if err := pbsynadia.Setup(app, opts); err != nil {
        log.Fatalf("pb-synadia setup failed: %v", err)
    }

    if err := app.Start(); err != nil {
        log.Fatal(err)
    }
}
```

Get a Personal Access Token at [cloud.synadia.com/profile/personal-access-tokens](https://cloud.synadia.com/profile/personal-access-tokens) and find your System ID in the Synadia console. The free Personal tier (2 accounts, 10 connections, no card required) is enough to try the library.

## Architecture

```
PocketBase CRUD on nats_accounts/nats_users/nats_roles
    |
    v
Pre-save hooks (internal/hooks/) — write-through, synchronous
    |
    v
Synadia client adapter (internal/synadia/) — thin wrapper over syncp SDK
    |
    v
Synadia Cloud REST API (cloud.synadia.com/core/beta/...)
```

No queue, no debouncer, no operator keys, no JWT generation, no NATS connection. Every PB CRUD = one HTTP call to Synadia.

### Hook timing

- **Create / Update**: `OnRecordCreate` / `OnRecordUpdate` (pre-save). pb-synadia calls Synadia, mutates the record with returned ids and the `creds_file`, then `e.Next()` persists everything in one save. No recursion.
- **Delete**: `OnRecordDelete` (pre-delete). Synadia is called first; on failure the PB delete is aborted. A 404 from Synadia is treated as success so already-gone resources clean up naturally.

## Collections

Five collections (accounts, roles, users, subject exports, subject imports). All have `nil` API rules — set them in your app:

```go
accounts, _ := app.FindCollectionByNameOrId("nats_accounts")
accounts.ListRule = types.Pointer("@request.auth.id != ''")
accounts.ViewRule = types.Pointer("@request.auth.id != '' && active = true")
app.Save(accounts)
```

### `nats_accounts`

| Field | Type | Notes |
|---|---|---|
| `name` | Text | display name |
| `description` | Text | |
| `synadia_account_id` | Text | populated after create |
| `synadia_sk_group_id` | Text | cached unscoped signing key group id (see [Why a default sk group?](#why-a-default-signing-key-group)) |
| `public_key` | Text | cached from Synadia |
| `active` | Bool | |
| `max_connections`, `max_subscriptions`, `max_data`, `max_payload` | Number | account-level limits |
| `max_jetstream_disk_storage`, `max_jetstream_memory_storage` | Number | JetStream limits |
| `sync_state` | Select | `synced` / `creating` / `pending_create` / `pending_update` |
| `last_sync_error` | Text | populated on Synadia failure |

### `nats_roles`

PB-side permission template. Synadia never sees it; pb-synadia merges role + user overrides on every user write.

| Field | Type | Notes |
|---|---|---|
| `name`, `description` | Text | |
| `is_default` | Bool | |
| `publish_permissions`, `subscribe_permissions` | JSON | allow lists |
| `publish_deny_permissions`, `subscribe_deny_permissions` | JSON | deny lists (deny wins) |
| `allow_response`, `allow_response_max`, `allow_response_ttl` | Bool / Number | request-reply response perms |
| `max_subscriptions`, `max_data`, `max_payload` | Number | per-user limits |

### `nats_users` (auth collection)

| Field | Type | Notes |
|---|---|---|
| `nats_username` | Text | natural correlation key with Synadia; **must be unique per account and immutable** |
| `account_id`, `role_id` | Relation | required |
| `synadia_user_id` | Text | populated after create |
| `public_key`, `jwt`, `creds_file` | Text | cached from Synadia; `creds_file` is a complete `.creds` for client connection |
| `bearer_token` | Bool | |
| `active` | Bool | |
| `regenerate` | Bool | trigger: rotate the user's nkey + seed on Synadia and download new creds on next save. **Destructive — invalidates any previously distributed `.creds` file.** Auto-reset to false. |
| `publish_permissions` / `subscribe_permissions` / deny variants | JSON | per-user overrides, merged with role |
| `sync_state`, `last_sync_error` | Select / Text | |

### `nats_account_exports`

Declares a subject the owning account exposes to other accounts. Each row maps to one Synadia subject export (top-level resource on Synadia, *not* embedded in the account JWT — that's the v0.1 pb-nats model). Type values match pb-nats: `stream` for one-way data flow, `service` for request-reply.

| Field | Type | Notes |
|---|---|---|
| `account_id` | Relation | owning account; cascade delete |
| `name` | Text | export name |
| `subject` | Text | NATS subject pattern (wildcards OK) |
| `type` | Select | `stream` or `service` |
| `token_req` | Bool | require activation token for import |
| `response_type` | Select | `Singleton` / `Stream` / `Chunked` — service only |
| `response_threshold` | Number | response timeout, ms — service only |
| `account_token_position` | Number | position of account token in wildcard subject |
| `advertise` | Bool | advertise this export |
| `allow_trace` | Bool | **PB-side only** — `syncp.Export` has no AllowTrace field; kept for pb-nats parity |
| `description` | Text | |
| `synadia_export_id` | Text | populated after create |
| `sync_state`, `last_sync_error` | Select / Text | |

### `nats_account_imports`

Consumes a subject another account exports. The exporting account is referenced by its **public NKey** in the `account` field, not by relation — the exporter may live in a different deployment.

| Field | Type | Notes |
|---|---|---|
| `account_id` | Relation | importing account; cascade delete |
| `name` | Text | import name |
| `subject` | Text | subject being imported (publisher-perspective) |
| `account` | Text | exporting account's public NKey |
| `token` | Text | activation JWT — required when the export has `token_req` |
| `local_subject` | Text | local subject remapping (supports `$1`, `$2` wildcards) |
| `type` | Select | `stream` or `service` |
| `share` | Bool | enable latency tracking — service only |
| `allow_trace` | Bool | **PB-side only** — `syncp.Import` has no AllowTrace field |
| `description` | Text | **PB-side only** — `syncp.Import` has no Description field |
| `synadia_import_id` | Text | populated after create |
| `sync_state`, `last_sync_error` | Select / Text | |

## Permissions

Permissions are pushed to Synadia as **inline (unscoped) user permissions**, computed on every user write as the union of role + per-user override:

```
allow_pub  = role.publish_permissions ∪ user.publish_permissions
allow_sub  = role.subscribe_permissions ∪ user.subscribe_permissions
deny_pub   = role.publish_deny_permissions ∪ user.publish_deny_permissions
deny_sub   = role.subscribe_deny_permissions ∪ user.subscribe_deny_permissions
```

Defaults (`Options.DefaultPublishPermissions` / `DefaultSubscribePermissions`) apply only when both role and user lists are empty for that direction. Deny precedence is enforced server-side by NATS.

### Why inline merge, not Synadia signing key groups

Synadia exposes [Signing Key Groups](https://github.com/synadia-io/control-plane-sdk-go/blob/main/syncp/docs/SigKeyGroupAPI.md) which structurally mirror pb-synadia roles — same allow/deny + limits shape. We don't use them as the role mapping in v0.1 because Synadia's group lifecycle is incompatible with pb-nats-style mutable roles:

1. **No user reassignment.** `NatsUserUpdateRequest` has no `SkGroupId` field. Changing a user's `role_id` via groups would force copy + delete on Synadia, destroying creds and breaking live connections.
2. **No hybrid scoped+inline.** A Synadia user is fully scoped or fully unscoped — adding an override forces a destructive migration.
3. **Cascade on group update is undocumented.** Whether `UpdateAccountSkGroup` re-issues users' JWTs server-side is unspecified.

Inline merge gives one mode, no cliff edges, and the same per-user override pattern pb-nats users already understand. The cost is N API calls when a role updates many users — handled in v0.2 with `Reconcile` and `Options.SynadiaCallDelay`. v0.4+ may add an opt-in `RoleStrategy: StrategyScopedGroups` for very large deployments.

### Why a default signing key group

Synadia requires every NATS user to be assigned to a signing key group at create time. For inline-permission users we still need *some* group. pb-synadia auto-creates one named `pb-synadia-default` per account on first account create (with `Programmatic: true` so reconcile can distinguish library-managed groups), and caches its id on the account record.

## Cross-Account Exports and Imports

NATS accounts are isolated by default. Exports declare a subject one account makes available; imports consume a subject another account has exported. The user-facing surface matches pb-nats (`type: "stream" | "service"`, same field set on each row) so apps targeting both backends can share their consuming code.

**Stream export → import** (one-way data):

```http
POST /api/collections/nats_account_exports/records
{ "account_id": "ACCT_A_ID", "name": "sensors", "subject": "sensors.>", "type": "stream" }

POST /api/collections/nats_account_imports/records
{ "account_id": "ACCT_B_ID", "name": "sensors", "subject": "sensors.>",
  "account": "ABCD...ACCT_A_PUBLIC_KEY", "type": "stream" }
```

**Service export → import** (request-reply):

```http
POST /api/collections/nats_account_exports/records
{ "account_id": "ACCT_A_ID", "name": "echo", "subject": "echo.req",
  "type": "service", "response_type": "Singleton" }

POST /api/collections/nats_account_imports/records
{ "account_id": "ACCT_B_ID", "name": "echo", "subject": "echo.req",
  "account": "ABCD...ACCT_A_PUBLIC_KEY", "type": "service" }
```

**Token-required exports.** Set `token_req: true` on the export. Importing accounts must obtain an activation JWT (from the exporting side) and put it on the import's `token` field. pb-synadia does not generate activation tokens — that's the responsibility of the exporting account's operator (today, via the Synadia console).

**Architectural note vs pb-nats.** pb-nats embeds exports and imports inside the account JWT and republishes the parent account whenever an export or import changes. Synadia treats them as **top-level resources** with their own ids — pb-synadia's hooks therefore call `CreateSubjectExport(synadia_account_id, ...)` / `CreateSubjectImport(...)` and cache an `synadia_export_id` / `synadia_import_id` on the PB record. The account record is never mutated by an export/import write.

**Pending-account cascade.** An export or import whose owning account is still in `pending_create` cannot succeed yet — its hook marks it `pending_create` with a descriptive `last_sync_error`. The next `Reconcile` pass picks them up in order (accounts → users → exports → imports) so a single pass can heal a chain.

## Sync States and Retry

Every account and user record carries a `sync_state`:

| State | Meaning |
|---|---|
| `synced` | PB and Synadia agree. |
| `creating` | Reserved (set briefly during multi-step operations). |
| `pending_create` | Synadia create failed; record exists in PB only. |
| `pending_update` | Synadia update or creds-download failed; record stale. |

On any failure the hook sets `last_sync_error` and returns nil to the caller — the PB save still succeeds and the record is left in `pending_*` state. Re-saving the record retries the Synadia call.

### Periodic reconciliation

`Reconcile(app, opts, ctx)` scans accounts and users in `pending_*` states and re-runs the same push that the hook would. Records that succeed transition to `synced`; records that still fail are left in `pending_*` for the next pass. Accounts are processed before users so a user whose account just got its `synadia_account_id` resolved on this pass can succeed on the same pass.

Wire it to `app.Cron()` at whatever cadence suits the deployment:

```go
app.Cron().MustAdd("pb-synadia-reconcile", "*/5 * * * *", func() {
    if err := pbsynadia.Reconcile(app, opts, context.Background()); err != nil {
        log.Printf("reconcile: %v", err)
    }
})
```

Reconcile is safe to call concurrently — overlapping invocations skip with a log line rather than running in parallel. Per-record push failures are swallowed and logged; only setup-level failures are returned. Set `Options.SynadiaCallDelay` to throttle between records if you have many pending rows.

What Reconcile in v0.2.1 does **not** yet do: drift detection (does the PB-known `synadia_*_id` still exist on Synadia?), orphan adoption (a Synadia resource that PB doesn't know about — stitching by `name` / `nats_username`), or role-update cascade. Those are subsequent slices.

## Limit Semantics — Heads Up

pb-nats uses `-1 = unlimited`, `0 = disabled (blocks access)`, `positive = limit`. **Synadia does not have a "disabled" semantic** — it interprets 0 / negative as "no limit configured" (field omitted from the JWT).

pb-synadia v0.1 treats any non-zero limit as a value to forward, and a zero as "leave unset on Synadia". If you need to fully block a resource, use Synadia's account-level controls in their console rather than relying on pb-nats's `0` convention.

## Configuration

```go
opts := pbsynadia.DefaultOptions()

// Required
opts.SystemID = "..."     // Synadia system UUID
opts.APIToken = "..."     // Personal Access Token

// Optional
opts.SynadiaBaseURL = ""                            // default: https://cloud.synadia.com
opts.AccountCollectionName = "nats_accounts"        // override default names
opts.UserCollectionName    = "nats_users"
opts.RoleCollectionName    = "nats_roles"
opts.DefaultPublishPermissions   = []string{">"}
opts.DefaultSubscribePermissions = []string{">", "_INBOX.>"}
opts.SynadiaCallDelay = 0                           // throttle for cascaded calls (v0.2)
opts.LogToConsole = true
opts.EventFilter = func(collection, event string) bool { return true }
```

A single `pb-synadia` instance manages one Synadia system. To span multiple systems, run multiple PocketBase instances (or wait for a future revision).

## Client Connection

After a user record is saved, its `creds_file` field contains a full `.creds` for connecting to Synadia's NATS:

```javascript
const pb = new PocketBase('http://localhost:8090');
await pb.collection('nats_users').authWithPassword('user@example.com', 'password');
const user = await pb.collection('nats_users').getOne(pb.authStore.record.id);

import { connect, credsAuthenticator } from 'nats';
const nc = await connect({
    servers: ["tls://connect.ngs.global:4222"],
    authenticator: credsAuthenticator(new TextEncoder().encode(user.creds_file))
});
```

Set `bearer_token: true` on the user record before save if you need bearer-token auth.

## Error Handling

```go
if pbsynadia.IsTemporaryError(err) {
    // Network or 5xx — record left in pending_* state, retry on next save (or via Reconcile in v0.2)
}
if pbsynadia.IsPermanentError(err) {
    // 4xx — fix the data; retry won't help
}
if pbsynadia.IsConfigurationError(err) {
    // Missing SystemID, missing APIToken, etc. — fail fast at startup
}
```

## When to use pb-synadia vs. pb-nats

- **pb-synadia** if you want managed NATS, no operator-key custody, no NATS bootstrap dance, JetStream/KV/Object Store included, and you're OK with Synadia's pricing model and vendor lock-in.
- **pb-nats** if you need to run NATS yourself (air-gapped, on-prem, custom topology), want full control over operator/account signing-key rotation, or need cross-deployment imports/exports keyed by public key.

The conceptual surface (accounts/users/roles/creds_file) is the same — the consuming app's integration code is similar between the two libraries.

## Roadmap

- **v0.2** — `Reconcile(app, opts, ctx)` shipped (wire to `app.Cron()`); pending-state retries and `Options.SynadiaCallDelay` honored. Still to come: drift detection (PB-known ids that no longer exist on Synadia), orphan adoption (Synadia resources unknown to PB, stitched by `name` / `nats_username`), and role-update cascade.
- **v0.3** — `nats_account_exports` / `nats_account_imports` shipped (subject-based, matching pb-nats's `type: "stream" | "service"` model). Still to come: JetStream stream sharing via Synadia's separate `StreamExportAPI` / `StreamImportAPI`.
- **v0.4 (speculative)** — Opt-in `Options.RoleStrategy = StrategyScopedGroups` mode that maps roles to Synadia signing key groups. Documented constraints (no per-user overrides on scoped users; role changes destroy creds). Only ships once group-update cascade behavior is verified by integration test.

## Troubleshooting

**`pending_create` rows piling up.** Synadia is unreachable or the API token is invalid. Check `last_sync_error` on the records. Once Synadia is reachable, the next `Reconcile` pass picks them up automatically — or re-save individual records to retry inline.

**User created in PB but no creds_file.** Synadia user creation succeeded but `DownloadNatsUserCreds` failed. The record is in `pending_update`. The next `Reconcile` pass will retry the push (which downloads creds again on success). If you can't wait, set `regenerate: true` to rotate the keys and fetch fresh creds — safe here since no creds were ever in circulation.

**Account exists in Synadia but PB doesn't know its id.** Either created outside pb-synadia, or a `pending_create` retry succeeded but the second PB save failed. Orphan stitching by name (`nats_username` for users, `name` for accounts) is on the v0.2 roadmap but not yet implemented — for now, copy the `synadia_account_id` onto the PB record manually.

**Permission changes on a role aren't reflected.** Expected in v0.1 — role cascade is v0.2. Re-save each affected user record to push their merged permissions.

**Synadia API beta status.** The Synadia Control Plane API is at `/core/beta/...`. Field names may shift; pb-synadia funnels every SDK call through `internal/synadia/` so an SDK bump is one-file work.

## License

MIT — see [LICENSE](LICENSE).
