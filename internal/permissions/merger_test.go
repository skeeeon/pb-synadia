package permissions

import (
	"encoding/json"
	"reflect"
	"testing"

	pbtypes "github.com/skeeeon/pb-synadia/internal/types"
)

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestMerge_UnionAndDenyPrecedence(t *testing.T) {
	role := &pbtypes.RoleRecord{
		PublishPermissions:       mustJSON(t, []string{"sensors.>"}),
		SubscribePermissions:     mustJSON(t, []string{"sensors.>", "alerts.>"}),
		PublishDenyPermissions:   mustJSON(t, []string{"sensors.internal.>"}),
		SubscribeDenyPermissions: mustJSON(t, []string{}),
	}
	user := &pbtypes.NatsUserRecord{
		PublishPermissions:       mustJSON(t, []string{"admin.reports.>"}),
		SubscribePermissions:     mustJSON(t, []string{"admin.reports.>"}),
		PublishDenyPermissions:   mustJSON(t, []string{}),
		SubscribeDenyPermissions: mustJSON(t, []string{"alerts.admin.>"}),
	}
	got, err := Merge(role, user, pbtypes.Options{})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	wantPub := []string{"sensors.>", "admin.reports.>"}
	wantSub := []string{"sensors.>", "alerts.>", "admin.reports.>"}
	wantPubDeny := []string{"sensors.internal.>"}
	wantSubDeny := []string{"alerts.admin.>"}
	if !reflect.DeepEqual(got.Pub, wantPub) {
		t.Errorf("Pub = %v, want %v", got.Pub, wantPub)
	}
	if !reflect.DeepEqual(got.Sub, wantSub) {
		t.Errorf("Sub = %v, want %v", got.Sub, wantSub)
	}
	if !reflect.DeepEqual(got.PubDeny, wantPubDeny) {
		t.Errorf("PubDeny = %v, want %v", got.PubDeny, wantPubDeny)
	}
	if !reflect.DeepEqual(got.SubDeny, wantSubDeny) {
		t.Errorf("SubDeny = %v, want %v", got.SubDeny, wantSubDeny)
	}
}

func TestMerge_DefaultsWhenBothEmpty(t *testing.T) {
	role := &pbtypes.RoleRecord{}
	user := &pbtypes.NatsUserRecord{}
	defaults := pbtypes.Options{
		DefaultPublishPermissions:   []string{">"},
		DefaultSubscribePermissions: []string{">", "_INBOX.>"},
	}
	got, err := Merge(role, user, defaults)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if !reflect.DeepEqual(got.Pub, []string{">"}) {
		t.Errorf("Pub = %v, want [>]", got.Pub)
	}
	if !reflect.DeepEqual(got.Sub, []string{">", "_INBOX.>"}) {
		t.Errorf("Sub = %v, want [> _INBOX.>]", got.Sub)
	}
}

func TestMerge_UserOnlyOverrideSkipsDefaults(t *testing.T) {
	role := &pbtypes.RoleRecord{}
	user := &pbtypes.NatsUserRecord{
		PublishPermissions: mustJSON(t, []string{"app.>"}),
	}
	defaults := pbtypes.Options{
		DefaultPublishPermissions:   []string{">"},
		DefaultSubscribePermissions: []string{">"},
	}
	got, err := Merge(role, user, defaults)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if !reflect.DeepEqual(got.Pub, []string{"app.>"}) {
		t.Errorf("Pub = %v, want [app.>] (defaults should not apply when user provides one)", got.Pub)
	}
	if !reflect.DeepEqual(got.Sub, []string{">"}) {
		t.Errorf("Sub = %v, want [>] (defaults should still apply for empty sub)", got.Sub)
	}
}

func TestMerge_DeduplicatesUnion(t *testing.T) {
	role := &pbtypes.RoleRecord{
		PublishPermissions: mustJSON(t, []string{"a", "b"}),
	}
	user := &pbtypes.NatsUserRecord{
		PublishPermissions: mustJSON(t, []string{"b", "c"}),
	}
	got, err := Merge(role, user, pbtypes.Options{})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got.Pub, want) {
		t.Errorf("Pub = %v, want %v", got.Pub, want)
	}
}

func TestMerge_LimitsAndResponseFromRole(t *testing.T) {
	role := &pbtypes.RoleRecord{
		AllowResponse:    true,
		AllowResponseMax: 3,
		AllowResponseTTL: 30,
		MaxSubscriptions: 100,
		MaxData:          -1,
		MaxPayload:       1024,
	}
	user := &pbtypes.NatsUserRecord{}
	got, err := Merge(role, user, pbtypes.Options{})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if !got.AllowResponse || got.AllowResponseMax != 3 || got.AllowResponseTTL != 30 {
		t.Errorf("response perms not propagated: %+v", got)
	}
	if got.MaxSubscriptions != 100 || got.MaxData != -1 || got.MaxPayload != 1024 {
		t.Errorf("limits not propagated: %+v", got)
	}
}
