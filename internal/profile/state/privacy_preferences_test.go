package state

import (
	"context"
	"reflect"

	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestMapAppleOsXPrivacyPreferences_NilInNilOut(t *testing.T) {
	t.Parallel()

	if got := mapAppleOsXPrivacyPreferences(nil); got != nil {
		t.Errorf("mapAppleOsXPrivacyPreferences(nil) = %+v, want nil", got)
	}
}

func TestMapAppleOsXPrivacyPreferences_EmptyIdentitiesIsNil(t *testing.T) {
	t.Parallel()

	if got := mapAppleOsXPrivacyPreferences(&sdk.MacOsPrivacyPreferencesPayloadV2Model{}); got != nil {
		t.Errorf("mapAppleOsXPrivacyPreferences(empty) = %+v, want nil", got)
	}
}

func TestMapAppleOsXPrivacyPreferences_FullyPopulated(t *testing.T) {
	t.Parallel()

	staticCode := true
	p := &sdk.MacOsPrivacyPreferencesPayloadV2Model{
		Identities: []sdk.MacOsPrivacyPreferencesV2Model{
			{
				Identifier:      "com.example.app",
				IdentifierType:  "bundleID",
				CodeRequirement: "identifier \"com.example.app\"",
				Comment:         "a rule",
				AppleEventsList: []sdk.AppleEventV2{
					{
						CodeRequirement: "cr",
						Identifier:      "com.example.receiver",
						IdentifierType:  "bundleID",
						Permission:      "Allow",
					},
				},
				StaticCode:           &staticCode,
				Accessibility:        "Allow",
				Camera:               "Disallow",
				SystemPolicyAllFiles: "Allow",
			},
		},
	}

	got := mapAppleOsXPrivacyPreferences(p)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	id := got[0]
	if id.Identifier.ValueString() != "com.example.app" {
		t.Errorf("Identifier = %q, want %q", id.Identifier.ValueString(), "com.example.app")
	}
	if id.IdentifierType.ValueString() != "bundleID" {
		t.Errorf("IdentifierType = %q, want %q", id.IdentifierType.ValueString(), "bundleID")
	}
	if !id.StaticCode.ValueBool() {
		t.Errorf("StaticCode = %v, want true", id.StaticCode)
	}
	if id.Camera.ValueString() != "Disallow" {
		t.Errorf("Camera = %q, want %q", id.Camera.ValueString(), "Disallow")
	}
	if len(id.AppleEventsList) != 1 {
		t.Fatalf("AppleEventsList len = %d, want 1", len(id.AppleEventsList))
	}
	if id.AppleEventsList[0].Identifier.ValueString() != "com.example.receiver" {
		t.Errorf("AppleEventsList[0].Identifier = %q, want %q", id.AppleEventsList[0].Identifier.ValueString(), "com.example.receiver")
	}
	if id.AppleEventsList[0].Permission.ValueString() != "Allow" {
		t.Errorf("AppleEventsList[0].Permission = %q, want %q", id.AppleEventsList[0].Permission.ValueString(), "Allow")
	}
}

// internal-ticket removed MergePrivacyPreferencesWithPriorState (row #83 of the
// B16 audit): the merge's keyed reorder, and its PreserveNull suppression of
// fields the user left unset, are both gone. readAppleOsXIntoState now
// stores mapAppleOsXPrivacyPreferences's result directly. The tests that
// used to assert the removed merge behavior (import surfacing, suppressing
// unset fields, keyed-not-positional pairing, unmatched-entry handling) are
// removed along with it; TestReadAppleOsXIntoState_PrivacyPreferencesPassThrough
// below asserts the new pass-through contract instead.

func TestReadAppleOsXIntoState_PrivacyPreferencesPassThrough(t *testing.T) {
	t.Parallel()

	prior := []PrivacyPreferenceModel{
		{Identifier: types.StringValue("com.example.a"), IdentifierType: types.StringValue("bundleID"), Comment: types.StringNull()},
	}
	ent := &sdk.AppleOsXDeviceProfileEntityV2{
		General: &sdk.GeneralPayloadV2Entity{Name: "P"},
		PrivacyPreferences: &sdk.MacOsPrivacyPreferencesPayloadV2Model{
			Identities: []sdk.MacOsPrivacyPreferencesV2Model{
				{Identifier: "com.example.a", IdentifierType: "bundleID", Comment: "server-set"},
			},
		},
	}
	data := &ProfileResourceModel{PrivacyPreferences: prior}
	readAppleOsXIntoState(context.Background(), data, ent)

	if len(data.PrivacyPreferences) != 1 {
		t.Fatalf("len = %d, want 1", len(data.PrivacyPreferences))
	}
	// The server value now wins outright -- no PreserveNull suppression back
	// to the prior null, even though the user never configured this field.
	if data.PrivacyPreferences[0].Comment.ValueString() != "server-set" {
		t.Errorf("Comment = %v, want %q (server value stored as-is, no prior-state suppression)", data.PrivacyPreferences[0].Comment, "server-set")
	}
}

// TestMapPrivacyPreferenceIdentity_EveryFieldRoundTrips reflects over the
// SDK identity struct: every string field gets a distinct value, and every
// same-named model field must carry it back. The only SDK fields allowed to
// have no model counterpart are the identity-level Apple Events shortcut
// fields UEM never returns on read (see the builder's pinned-unmodeled set).
// Dropping any field from mapPrivacyPreferenceIdentity/mapAppleEventsList
// fails this test.
func TestMapPrivacyPreferenceIdentity_EveryFieldRoundTrips(t *testing.T) {
	t.Parallel()

	unmodeled := map[string]bool{
		"AEReceiverCodeRequirement": true,
		"AEReceiverIdentifier":      true,
		"AEReceiverIdentifierType":  true,
		"AppleEvents":               true,
	}
	staticCode := true
	var in sdk.MacOsPrivacyPreferencesV2Model
	iv := reflect.ValueOf(&in).Elem()
	for i := 0; i < iv.NumField(); i++ {
		if iv.Field(i).Kind() == reflect.String {
			iv.Field(i).SetString("v-" + iv.Type().Field(i).Name)
		}
	}
	in.StaticCode = &staticCode
	var ae sdk.AppleEventV2
	av := reflect.ValueOf(&ae).Elem()
	for i := 0; i < av.NumField(); i++ {
		av.Field(i).SetString("ae-" + av.Type().Field(i).Name)
	}
	in.AppleEventsList = []sdk.AppleEventV2{ae}

	got := mapPrivacyPreferenceIdentity(&in)
	gv := reflect.ValueOf(got)
	for i := 0; i < iv.NumField(); i++ {
		name := iv.Type().Field(i).Name
		if iv.Field(i).Kind() != reflect.String {
			continue
		}
		mf := gv.FieldByName(name)
		if !mf.IsValid() {
			if !unmodeled[name] {
				t.Errorf("SDK field %s has no model field and is not pinned unmodeled", name)
			}
			continue
		}
		if s, ok := mf.Interface().(types.String); !ok || s.ValueString() != "v-"+name {
			t.Errorf("%s = %v, want %q", name, s, "v-"+name)
		}
	}
	if !got.StaticCode.ValueBool() {
		t.Errorf("StaticCode = %v, want true", got.StaticCode)
	}
	if len(got.AppleEventsList) != 1 {
		t.Fatalf("AppleEventsList len = %d, want 1", len(got.AppleEventsList))
	}
	gav := reflect.ValueOf(got.AppleEventsList[0])
	for i := 0; i < av.NumField(); i++ {
		name := av.Type().Field(i).Name
		mf := gav.FieldByName(name)
		if !mf.IsValid() {
			t.Errorf("AppleEventV2.%s has no model field", name)
			continue
		}
		if s, ok := mf.Interface().(types.String); !ok || s.ValueString() != "ae-"+name {
			t.Errorf("AppleEventsList[0].%s = %v, want %q", name, s, "ae-"+name)
		}
	}
}
