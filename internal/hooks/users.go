package hooks

import (
	"encoding/json"
	"fmt"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/skeeeon/pb-synadia/internal/permissions"
	"github.com/skeeeon/pb-synadia/internal/synadia"
	pbtypes "github.com/skeeeon/pb-synadia/internal/types"
)

// SetupUserHooks registers create/update/delete hooks for the users
// collection. Pre-save hooks resolve the user's account + role, build merged
// permissions, and call Synadia. The .creds file is downloaded immediately
// and cached for self-service download via the user record.
func SetupUserHooks(app *pocketbase.PocketBase, deps *Deps) {
	app.OnRecordCreate(deps.Options.UserCollectionName).BindFunc(func(e *core.RecordEvent) error {
		if !deps.shouldHandle(deps.Options.UserCollectionName, pbtypes.EventTypeUserCreate) {
			return e.Next()
		}
		if err := pushUserCreate(app, deps, e.Record); err != nil {
			deps.Logger.Warning("user create on Synadia failed for %s: %v", e.Record.GetString("nats_username"), err)
		}
		return e.Next()
	})

	app.OnRecordUpdate(deps.Options.UserCollectionName).BindFunc(func(e *core.RecordEvent) error {
		if !deps.shouldHandle(deps.Options.UserCollectionName, pbtypes.EventTypeUserUpdate) {
			return e.Next()
		}
		if e.Record.GetString("synadia_user_id") == "" {
			// Never made it to Synadia. Retry as create.
			if err := pushUserCreate(app, deps, e.Record); err != nil {
				deps.Logger.Warning("user retry-create on Synadia failed for %s: %v", e.Record.Id, err)
			}
			return e.Next()
		}
		if e.Record.GetBool("regenerate") {
			e.Record.Set("regenerate", false)
			if err := rotateUserKeys(deps, e.Record); err != nil {
				deps.Logger.Warning("user key rotation failed for %s: %v", e.Record.Id, err)
			}
		}
		if !userSynadiaFieldsChanged(e.Record) {
			return e.Next()
		}
		if err := pushUserUpdate(app, deps, e.Record); err != nil {
			deps.Logger.Warning("user update on Synadia failed for %s: %v", e.Record.Id, err)
		}
		return e.Next()
	})

	app.OnRecordDelete(deps.Options.UserCollectionName).BindFunc(func(e *core.RecordEvent) error {
		if !deps.shouldHandle(deps.Options.UserCollectionName, pbtypes.EventTypeUserDelete) {
			return e.Next()
		}
		synadiaID := e.Record.GetString("synadia_user_id")
		if synadiaID == "" {
			return e.Next()
		}
		resp, err := deps.Client.DeleteUser(synadiaID)
		if err != nil && !synadia.IsNotFound(resp, err) {
			return fmt.Errorf("Synadia delete failed, aborting PB delete: %w", err)
		}
		deps.Logger.Delete("user %s removed from Synadia", e.Record.GetString("nats_username"))
		return e.Next()
	})
}

func pushUserCreate(app *pocketbase.PocketBase, deps *Deps, rec *core.Record) error {
	user, account, role, err := resolveUserContext(app, deps, rec)
	if err != nil {
		markPending(rec, pbtypes.SyncStatePendingCreate, err.Error())
		return err
	}
	if account.SynadiaAccountID == "" {
		err := fmt.Errorf("account %q has no synadia_account_id yet", account.Name)
		markPending(rec, pbtypes.SyncStatePendingCreate, err.Error())
		return err
	}

	skGroupID := account.SynadiaSkGroupID
	if skGroupID == "" {
		// Account record never resolved its default group — try now.
		gid, gerr := deps.Client.EnsureDefaultSkGroup(account.SynadiaAccountID)
		if gerr != nil {
			markPending(rec, pbtypes.SyncStatePendingCreate, gerr.Error())
			return gerr
		}
		skGroupID = gid
		// Persist on the account record for next time.
		if accRec, ferr := app.FindRecordById(deps.Options.AccountCollectionName, account.ID); ferr == nil {
			accRec.Set("synadia_sk_group_id", skGroupID)
			_ = app.Save(accRec)
		}
	}

	merged, err := permissions.Merge(role, user, deps.Options)
	if err != nil {
		markPending(rec, pbtypes.SyncStatePendingCreate, err.Error())
		return err
	}

	result, err := deps.Client.CreateUser(synadia.UserCreateInput{
		SynadiaAccountID: account.SynadiaAccountID,
		SkGroupID:        skGroupID,
		NatsUsername:     user.NatsUsername,
		BearerToken:      user.BearerToken,
		Permissions:      merged,
	})
	if err != nil {
		markPending(rec, pbtypes.SyncStatePendingCreate, err.Error())
		return err
	}

	rec.Set("synadia_user_id", result.ID)
	rec.Set("public_key", result.PublicKey)
	rec.Set("jwt", result.JWT)

	creds, err := deps.Client.DownloadCreds(result.ID)
	if err != nil {
		// Synadia user exists, but creds download failed. Mark pending so
		// Reconcile retries the download — id and JWT are already saved.
		markPending(rec, pbtypes.SyncStatePendingUpdate, err.Error())
		return err
	}
	rec.Set("creds_file", creds)
	markSynced(rec)
	deps.Logger.Success("user %s created on Synadia (id=%s)", user.NatsUsername, result.ID)
	return nil
}

func pushUserUpdate(app *pocketbase.PocketBase, deps *Deps, rec *core.Record) error {
	user, _, role, err := resolveUserContext(app, deps, rec)
	if err != nil {
		markPending(rec, pbtypes.SyncStatePendingUpdate, err.Error())
		return err
	}
	merged, err := permissions.Merge(role, user, deps.Options)
	if err != nil {
		markPending(rec, pbtypes.SyncStatePendingUpdate, err.Error())
		return err
	}
	result, err := deps.Client.UpdateUser(rec.GetString("synadia_user_id"), merged, user.BearerToken)
	if err != nil {
		markPending(rec, pbtypes.SyncStatePendingUpdate, err.Error())
		return err
	}
	rec.Set("jwt", result.JWT)
	markSynced(rec)
	return nil
}

// rotateUserKeys rotates the user's nkey/seed on Synadia and downloads the
// resulting creds. The old creds are invalid the moment rotation succeeds —
// we clear the cached creds_file before the download so a download failure
// leaves the record honestly empty rather than holding a now-dead creds blob.
func rotateUserKeys(deps *Deps, rec *core.Record) error {
	result, err := deps.Client.RotateUser(rec.GetString("synadia_user_id"))
	if err != nil {
		markPending(rec, pbtypes.SyncStatePendingUpdate, err.Error())
		return err
	}
	rec.Set("public_key", result.PublicKey)
	rec.Set("jwt", result.JWT)
	rec.Set("creds_file", "")

	creds, err := deps.Client.DownloadCreds(result.ID)
	if err != nil {
		markPending(rec, pbtypes.SyncStatePendingUpdate, err.Error())
		return err
	}
	rec.Set("creds_file", creds)
	markSynced(rec)
	deps.Logger.Success("user %s keys rotated on Synadia", rec.GetString("nats_username"))
	return nil
}

// resolveUserContext loads the user, account, and role records for a user record.
func resolveUserContext(app *pocketbase.PocketBase, deps *Deps, rec *core.Record) (*pbtypes.NatsUserRecord, *pbtypes.AccountRecord, *pbtypes.RoleRecord, error) {
	user := recordToUser(rec)

	if user.AccountID == "" {
		return nil, nil, nil, fmt.Errorf("user %q missing account_id", user.NatsUsername)
	}
	if user.RoleID == "" {
		return nil, nil, nil, fmt.Errorf("user %q missing role_id", user.NatsUsername)
	}

	accRec, err := app.FindRecordById(deps.Options.AccountCollectionName, user.AccountID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("find account %q: %w", user.AccountID, err)
	}
	roleRec, err := app.FindRecordById(deps.Options.RoleCollectionName, user.RoleID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("find role %q: %w", user.RoleID, err)
	}

	account := recordToAccount(accRec)
	role := recordToRole(roleRec)
	return &user, &account, &role, nil
}

func recordToUser(rec *core.Record) pbtypes.NatsUserRecord {
	return pbtypes.NatsUserRecord{
		ID:                       rec.Id,
		NatsUsername:             rec.GetString("nats_username"),
		Description:              rec.GetString("description"),
		AccountID:                rec.GetString("account_id"),
		RoleID:                   rec.GetString("role_id"),
		SynadiaUserID:            rec.GetString("synadia_user_id"),
		PublicKey:                rec.GetString("public_key"),
		JWT:                      rec.GetString("jwt"),
		CredsFile:                rec.GetString("creds_file"),
		BearerToken:              rec.GetBool("bearer_token"),
		Active:                   rec.GetBool("active"),
		Regenerate:               rec.GetBool("regenerate"),
		PublishPermissions:       jsonRawFromRec(rec, "publish_permissions"),
		SubscribePermissions:     jsonRawFromRec(rec, "subscribe_permissions"),
		PublishDenyPermissions:   jsonRawFromRec(rec, "publish_deny_permissions"),
		SubscribeDenyPermissions: jsonRawFromRec(rec, "subscribe_deny_permissions"),
	}
}

func recordToRole(rec *core.Record) pbtypes.RoleRecord {
	return pbtypes.RoleRecord{
		ID:                       rec.Id,
		Name:                     rec.GetString("name"),
		Description:              rec.GetString("description"),
		IsDefault:                rec.GetBool("is_default"),
		PublishPermissions:       jsonRawFromRec(rec, "publish_permissions"),
		SubscribePermissions:     jsonRawFromRec(rec, "subscribe_permissions"),
		PublishDenyPermissions:   jsonRawFromRec(rec, "publish_deny_permissions"),
		SubscribeDenyPermissions: jsonRawFromRec(rec, "subscribe_deny_permissions"),
		AllowResponse:            rec.GetBool("allow_response"),
		AllowResponseMax:         rec.GetInt("allow_response_max"),
		AllowResponseTTL:         rec.GetInt("allow_response_ttl"),
		MaxSubscriptions:         int64FromRec(rec, "max_subscriptions"),
		MaxData:                  int64FromRec(rec, "max_data"),
		MaxPayload:               int64FromRec(rec, "max_payload"),
	}
}

func jsonRawFromRec(rec *core.Record, field string) json.RawMessage {
	raw := rec.Get(field)
	if raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case json.RawMessage:
		return v
	case []byte:
		return json.RawMessage(v)
	case string:
		if v == "" {
			return nil
		}
		return json.RawMessage(v)
	}
	// Fall back to remarshaling whatever PocketBase handed back.
	if b, err := json.Marshal(raw); err == nil {
		return b
	}
	return nil
}

// userSynadiaFieldsChanged reports whether the user's permissions, role,
// or bearer-token flag differ from the record's original loaded state.
// Account changes are not considered — Synadia doesn't allow moving a user
// between accounts (or sk groups) anyway.
func userSynadiaFieldsChanged(rec *core.Record) bool {
	orig := rec.Original()
	if orig.Id == "" {
		return true
	}
	if orig.GetString("role_id") != rec.GetString("role_id") {
		return true
	}
	if orig.GetBool("bearer_token") != rec.GetBool("bearer_token") {
		return true
	}
	for _, f := range []string{
		"publish_permissions", "subscribe_permissions",
		"publish_deny_permissions", "subscribe_deny_permissions",
	} {
		if !rawJSONEqual(jsonRawFromRec(orig, f), jsonRawFromRec(rec, f)) {
			return true
		}
	}
	return false
}

func rawJSONEqual(a, b json.RawMessage) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return string(a) == string(b)
}
