// Package types defines shared types used throughout the pb-synadia library.
package types

import (
	"encoding/json"
	"strings"
	"time"
)

// AccountRecord represents a PocketBase mirror of a Synadia Cloud account.
//
// Synadia owns the canonical account state. PocketBase caches identity, limits,
// and the resolved synadia_account_id. Hooks call Synadia on writes and update
// these fields with returned values.
//
// Limit values follow NATS semantics:
//   - -1 = unlimited
//   - 0  = disabled (blocks access entirely)
//   - positive = specific limit
type AccountRecord struct {
	ID                        string `json:"id"`
	Name                      string `json:"name"`
	Description               string `json:"description"`
	SynadiaAccountID          string `json:"synadia_account_id"`
	// SynadiaSkGroupID is the cached id of the unscoped signing key group used
	// for users in this account. Resolved lazily on first user write or via
	// Reconcile, since Synadia requires every user to be assigned to a group
	// even when permissions are inline.
	SynadiaSkGroupID string `json:"synadia_sk_group_id"`
	PublicKey                 string `json:"public_key"`
	Active                    bool   `json:"active"`
	MaxConnections            int64  `json:"max_connections"`
	MaxSubscriptions          int64  `json:"max_subscriptions"`
	MaxData                   int64  `json:"max_data"`
	MaxPayload                int64  `json:"max_payload"`
	MaxJetStreamDiskStorage   int64  `json:"max_jetstream_disk_storage"`
	MaxJetStreamMemoryStorage int64  `json:"max_jetstream_memory_storage"`
	SyncState                 string `json:"sync_state"`
	LastSyncError             string `json:"last_sync_error"`

	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
}

// NatsUserRecord represents a PocketBase mirror of a Synadia Cloud NATS user.
//
// nats_username is the natural correlation key with Synadia and must be unique
// within an account and immutable. The .creds file is cached from Synadia for
// self-service download.
type NatsUserRecord struct {
	// Standard PocketBase auth fields
	ID       string `json:"id"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Verified bool   `json:"verified"`

	// Synadia mirror fields
	NatsUsername    string `json:"nats_username"`
	Description     string `json:"description"`
	AccountID       string `json:"account_id"`
	RoleID          string `json:"role_id"`
	SynadiaUserID   string `json:"synadia_user_id"`
	PublicKey       string `json:"public_key"`
	JWT             string `json:"jwt"`
	CredsFile       string `json:"creds_file"`
	BearerToken     bool   `json:"bearer_token"`
	Active          bool   `json:"active"`
	Regenerate      bool   `json:"regenerate"`
	SyncState       string `json:"sync_state"`
	LastSyncError   string `json:"last_sync_error"`

	// Per-user permission overrides (merged with role permissions via union)
	PublishPermissions       json.RawMessage `json:"publish_permissions"`
	SubscribePermissions     json.RawMessage `json:"subscribe_permissions"`
	PublishDenyPermissions   json.RawMessage `json:"publish_deny_permissions"`
	SubscribeDenyPermissions json.RawMessage `json:"subscribe_deny_permissions"`
}

// RoleRecord represents a permission template applied to users.
//
// Roles live entirely in PocketBase — Synadia never sees the role concept.
// The permission merger combines role permissions with user-level overrides
// and pushes the resulting inline permissions to Synadia per user.
type RoleRecord struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsDefault   bool   `json:"is_default"`

	PublishPermissions       json.RawMessage `json:"publish_permissions"`
	SubscribePermissions     json.RawMessage `json:"subscribe_permissions"`
	PublishDenyPermissions   json.RawMessage `json:"publish_deny_permissions"`
	SubscribeDenyPermissions json.RawMessage `json:"subscribe_deny_permissions"`

	AllowResponse    bool `json:"allow_response"`
	AllowResponseMax int  `json:"allow_response_max"`
	AllowResponseTTL int  `json:"allow_response_ttl"`

	MaxSubscriptions int64 `json:"max_subscriptions"`
	MaxData          int64 `json:"max_data"`
	MaxPayload       int64 `json:"max_payload"`

	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
}

// SubjectExportRecord mirrors a row in nats_account_exports. The user-facing
// fields match pb-nats's schema 1:1 for cross-library parity; the trailing
// trio (SynadiaExportID, SyncState, LastSyncError) is the pb-synadia mirror
// convention.
//
// AllowTrace is PB-side-only — syncp.Export has no AllowTrace field, so the
// value is accepted on the record but not transmitted to Synadia. Kept for
// pb-nats parity; revisit if Synadia adds support.
type SubjectExportRecord struct {
	ID                   string `json:"id"`
	AccountID            string `json:"account_id"`
	Name                 string `json:"name"`
	Subject              string `json:"subject"`
	Type                 string `json:"type"` // "stream" or "service"
	TokenReq             bool   `json:"token_req"`
	ResponseType         string `json:"response_type"`         // service only: Singleton | Stream | Chunked
	ResponseThreshold    int64  `json:"response_threshold"`    // ms, service only
	AccountTokenPosition int64  `json:"account_token_position"`
	Advertise            bool   `json:"advertise"`
	AllowTrace           bool   `json:"allow_trace"` // PB-side-only
	Description          string `json:"description"`
	SynadiaExportID      string `json:"synadia_export_id"`
	SyncState            string `json:"sync_state"`
	LastSyncError        string `json:"last_sync_error"`
}

// SubjectImportRecord mirrors a row in nats_account_imports. Fields match
// pb-nats's schema 1:1; the trailing trio is pb-synadia's mirror convention.
//
// AllowTrace and Description are PB-side-only — syncp.Import exposes neither.
// Kept for pb-nats parity; revisit if Synadia adds support.
type SubjectImportRecord struct {
	ID              string `json:"id"`
	AccountID       string `json:"account_id"`
	Name            string `json:"name"`
	Subject         string `json:"subject"`
	Type            string `json:"type"` // "stream" or "service"
	Account         string `json:"account"` // exporting account's public NKey
	Token           string `json:"token"`
	LocalSubject    string `json:"local_subject"`
	Share           bool   `json:"share"`
	AllowTrace      bool   `json:"allow_trace"` // PB-side-only
	Description     string `json:"description"` // PB-side-only
	SynadiaImportID string `json:"synadia_import_id"`
	SyncState       string `json:"sync_state"`
	LastSyncError   string `json:"last_sync_error"`
}

// MergedPermissions is the result of merging a role with a user's overrides.
// It is the structure pushed to Synadia as inline (unscoped) user permissions.
type MergedPermissions struct {
	Pub              []string
	Sub              []string
	PubDeny          []string
	SubDeny          []string
	AllowResponse    bool
	AllowResponseMax int
	AllowResponseTTL int
	MaxSubscriptions int64
	MaxData          int64
	MaxPayload       int64
}

// Sync states tracked on account and user records.
//
// The sync_state field doubles as a retry queue: rows in pending_* states are
// picked up by Reconcile() on its next run. No separate queue collection.
const (
	SyncStateSynced         = "synced"
	SyncStateCreating       = "creating"
	SyncStatePendingCreate  = "pending_create"
	SyncStatePendingUpdate  = "pending_update"
)

// Default collection names with nats_ prefix matching pb-nats convention.
const (
	DefaultAccountCollectionName = "nats_accounts"
	DefaultUserCollectionName    = "nats_users"
	DefaultRoleCollectionName    = "nats_roles"
	DefaultExportCollectionName  = "nats_account_exports"
	DefaultImportCollectionName  = "nats_account_imports"
)

// Default Synadia Cloud API base URL.
const DefaultSynadiaBaseURL = "https://cloud.synadia.com"

// Event types for logging and EventFilter.
const (
	EventTypeAccountCreate = "account_create"
	EventTypeAccountUpdate = "account_update"
	EventTypeAccountDelete = "account_delete"
	EventTypeUserCreate    = "user_create"
	EventTypeUserUpdate    = "user_update"
	EventTypeUserDelete    = "user_delete"
	EventTypeRoleCreate    = "role_create"
	EventTypeRoleUpdate    = "role_update"
	EventTypeRoleDelete    = "role_delete"

	EventTypeSubjectExportCreate = "subject_export_create"
	EventTypeSubjectExportUpdate = "subject_export_update"
	EventTypeSubjectExportDelete = "subject_export_delete"
	EventTypeSubjectImportCreate = "subject_import_create"
	EventTypeSubjectImportUpdate = "subject_import_update"
	EventTypeSubjectImportDelete = "subject_import_delete"
)

// Default permission arrays applied when both role and user permission lists are empty.
var DefaultPublishPermissions = []string{">"}
var DefaultSubscribePermissions = []string{">", "_INBOX.>"}

// Options configures pb-synadia behavior.
type Options struct {
	// Synadia Cloud connection.
	SystemID       string
	APIToken       string
	SynadiaBaseURL string

	// Collection names (customizable).
	UserCollectionName    string
	RoleCollectionName    string
	AccountCollectionName string
	ExportCollectionName  string
	ImportCollectionName  string

	// Default permissions applied when both role and user lists are empty.
	DefaultPublishPermissions   []string
	DefaultSubscribePermissions []string

	// Optional throttle between cascaded Synadia calls (e.g., role updates
	// touching N users). Default: 0 (no delay).
	SynadiaCallDelay time.Duration

	// Console logging toggle.
	LogToConsole bool

	// Optional event filter — return false to skip processing for a given event.
	EventFilter func(collectionName, eventType string) bool
}

// GetPublishPermissions extracts publish allow permissions from role's JSON field.
func (r *RoleRecord) GetPublishPermissions() ([]string, error) {
	return parseJSONPermissions(r.PublishPermissions)
}

// GetSubscribePermissions extracts subscribe allow permissions from role's JSON field.
func (r *RoleRecord) GetSubscribePermissions() ([]string, error) {
	return parseJSONPermissions(r.SubscribePermissions)
}

// GetPublishDenyPermissions extracts publish deny permissions from role's JSON field.
func (r *RoleRecord) GetPublishDenyPermissions() ([]string, error) {
	return parseJSONPermissions(r.PublishDenyPermissions)
}

// GetSubscribeDenyPermissions extracts subscribe deny permissions from role's JSON field.
func (r *RoleRecord) GetSubscribeDenyPermissions() ([]string, error) {
	return parseJSONPermissions(r.SubscribeDenyPermissions)
}

// GetPublishPermissions extracts publish allow permissions from user's JSON field.
func (u *NatsUserRecord) GetPublishPermissions() ([]string, error) {
	return parseJSONPermissions(u.PublishPermissions)
}

// GetSubscribePermissions extracts subscribe allow permissions from user's JSON field.
func (u *NatsUserRecord) GetSubscribePermissions() ([]string, error) {
	return parseJSONPermissions(u.SubscribePermissions)
}

// GetPublishDenyPermissions extracts publish deny permissions from user's JSON field.
func (u *NatsUserRecord) GetPublishDenyPermissions() ([]string, error) {
	return parseJSONPermissions(u.PublishDenyPermissions)
}

// GetSubscribeDenyPermissions extracts subscribe deny permissions from user's JSON field.
func (u *NatsUserRecord) GetSubscribeDenyPermissions() ([]string, error) {
	return parseJSONPermissions(u.SubscribeDenyPermissions)
}

func parseJSONPermissions(data json.RawMessage) ([]string, error) {
	if len(data) == 0 {
		return []string{}, nil
	}
	var permissions []string
	if err := json.Unmarshal(data, &permissions); err != nil {
		return nil, err
	}
	return permissions, nil
}

// NormalizeName creates a Synadia-friendly account name from the display name.
func (a *AccountRecord) NormalizeName() string {
	name := strings.ToLower(a.Name)
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "-", "_")

	var result strings.Builder
	for _, char := range name {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '_' {
			result.WriteRune(char)
		}
	}

	normalized := result.String()
	if normalized == "" {
		return "unnamed_account"
	}
	return normalized
}
