package platform

import (
	"encoding/json"
	"strings"
	"testing"

	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// This file is the PIN-BUMP CHECK from
// internal-design-doc: after bumping to the SDK
// pin that fixes the parent-key marshal order for macOS Restrictions
// Preferences/Sharing, run the real Create and Update builders (including
// the overlay RMW) and assert the marshaled JSON puts each parent key ahead
// of every field it gates. The SDK's own send path (client.go) calls
// json.Marshal(body) directly on the typed struct -- never through
// map[string]any -- so marshaling the built *sdk.AppleOsXRestrictionsPayloadEntityV2
// here reproduces exactly what goes over the wire.
//
// UEM's deserializer applies JSON keys in request order and ignores a gated
// child that arrives before its parent. Before the SDK pin that added
// parent-first marshaling (every later pin, including 164e936a61c5, carries it),
// EnabledPreferencePanes and RestrictWhichSharingServicesAreEnabled sorted
// alphabetically among their own children (not first), so every child that
// sorts before its parent -- the 9 preference panes Accessibility..Dock and
// 7 sharing services AddtoAperture/AddtoReadingList/AddtoiPhoto/AirDrop/
// Facebook/Mail/Messages -- round-tripped as silently-reset false. This test
// fails against that shape by construction: it asserts a strict "parent
// index < every listed child index" ordering, which the old field order
// does not satisfy.

// restrictionsFieldOrderProblems mirrors keyIndexProblems from the SDK's own
// tests/macos_restrictions_field_order_test.go: it reports every child key
// that appears before, or is entirely missing from, the marshaled parent key.
func restrictionsFieldOrderProblems(body []byte, parent string, children []string) []string {
	s := string(body)
	quoted := func(k string) string { return `"` + k + `":` }
	p := strings.Index(s, quoted(parent))
	if p < 0 {
		return []string{"parent " + parent + " missing from marshaled JSON"}
	}
	var bad []string
	for _, c := range children {
		i := strings.Index(s, quoted(c))
		switch {
		case i < 0:
			bad = append(bad, "child "+c+" missing")
		case i < p:
			bad = append(bad, "child "+c+" precedes parent "+parent)
		}
	}
	return bad
}

// restrictionsPinBumpPreferenceChildren are the 9 preference panes the SDK's
// own CHANGELOG names as previously affected (alphabetically before
// "EnabledPreferencePanes").
var restrictionsPinBumpPreferenceChildren = []string{
	"Accessibility", "AppStore", "Bluetooth", "CDsAndDVDs", "DateAndTime",
	"DesktopAndScreenSaver", "DictationAndSpeech", "Displays", "Dock",
}

// restrictionsPinBumpSharingChildren are the 7 sharing services the SDK's own
// CHANGELOG names as previously affected (alphabetically before
// "RestrictWhichSharingServicesAreEnabled").
var restrictionsPinBumpSharingChildren = []string{
	"AddtoAperture", "AddtoReadingList", "AddtoiPhoto", "AirDrop", "Facebook", "Mail", "Messages",
}

// restrictionsPinBumpModel sets EnabledPreferencePanes plus all 9 affected
// panes, and RestrictWhichSharingServicesAreEnabled plus all 7 affected
// sharing services, matching the maintainer's live-proof scope exactly.
func restrictionsPinBumpModel() *profilemodels.RestrictionsModel {
	return &profilemodels.RestrictionsModel{
		Preferences: &profilemodels.RestrictionsPreferencesModel{
			EnabledPreferencePanes: types.BoolValue(true),
			Accessibility:          types.BoolValue(true),
			AppStore:               types.BoolValue(true),
			Bluetooth:              types.BoolValue(true),
			CDsAndDVDs:             types.BoolValue(true),
			DateAndTime:            types.BoolValue(true),
			DesktopAndScreenSaver:  types.BoolValue(true),
			DictationAndSpeech:     types.BoolValue(true),
			Displays:               types.BoolValue(true),
			Dock:                   types.BoolValue(true),
		},
		Sharing: &profilemodels.RestrictionsSharingModel{
			RestrictWhichSharingServicesAreEnabled: types.BoolValue(true),
			AddtoAperture:                          types.BoolValue(true),
			AddtoReadingList:                       types.BoolValue(true),
			AddtoiPhoto:                            types.BoolValue(true),
			AirDrop:                                types.BoolValue(true),
			Facebook:                               types.BoolValue(true),
			Mail:                                   types.BoolValue(true),
			Messages:                               types.BoolValue(true),
		},
	}
}

// assertRestrictionsFieldOrder marshals restrictions.Preferences and
// restrictions.Sharing independently (matching how the SDK marshals the
// whole entity in one pass, field by field) and asserts each parent key
// precedes every one of its gated children in the resulting JSON.
func assertRestrictionsFieldOrder(t *testing.T, restrictions *sdk.AppleOsXRestrictionsPayloadEntityV2, label string) {
	t.Helper()
	if restrictions == nil {
		t.Fatalf("%s: restrictions is nil", label)
	}

	prefBody, err := json.Marshal(restrictions.Preferences)
	if err != nil {
		t.Fatalf("%s: marshal Preferences: %v", label, err)
	}
	if problems := restrictionsFieldOrderProblems(prefBody, "EnabledPreferencePanes", restrictionsPinBumpPreferenceChildren); len(problems) > 0 {
		t.Errorf("%s: Preferences field order: %v\nbody: %s", label, problems, prefBody)
	}

	sharingBody, err := json.Marshal(restrictions.Sharing)
	if err != nil {
		t.Fatalf("%s: marshal Sharing: %v", label, err)
	}
	if problems := restrictionsFieldOrderProblems(sharingBody, "RestrictWhichSharingServicesAreEnabled", restrictionsPinBumpSharingChildren); len(problems) > 0 {
		t.Errorf("%s: Sharing field order: %v\nbody: %s", label, problems, sharingBody)
	}
}

// TestRestrictionsFieldOrder_PinBumpCheck_Create covers the Create path:
// BuildAppleOsXCreateEntity is the only builder Create calls
// (resource_platforms.go), and its output is marshaled and sent as-is.
func TestRestrictionsFieldOrder_PinBumpCheck_Create(t *testing.T) {
	t.Parallel()

	data := &profilemodels.ProfileResourceModel{
		Name:         types.StringValue("PinBumpCheck"),
		Restrictions: restrictionsPinBumpModel(),
	}
	entity, err := BuildAppleOsXCreateEntity(data)
	if err != nil {
		t.Fatalf("BuildAppleOsXCreateEntity: %v", err)
	}
	assertRestrictionsFieldOrder(t, entity.Restrictions, "Create")
}

// TestRestrictionsFieldOrder_PinBumpCheck_UpdateOverlayRMW covers the Update
// (read-modify-write) path: resource_update_osx.go builds a fresh entity
// from the plan via BuildAppleOsXCreateEntity, then overlays it onto the
// live entity via OverlayAppleOsXUpdateEntity, which replaces
// live.Restrictions with planned.Restrictions wholesale (overlay.go). This
// proves the RMW path sends the identical, correctly-ordered Restrictions
// payload Create does, not a re-derived or re-ordered one.
func TestRestrictionsFieldOrder_PinBumpCheck_UpdateOverlayRMW(t *testing.T) {
	t.Parallel()

	data := &profilemodels.ProfileResourceModel{
		Name:         types.StringValue("PinBumpCheck"),
		Restrictions: restrictionsPinBumpModel(),
	}
	planned, err := BuildAppleOsXCreateEntity(data)
	if err != nil {
		t.Fatalf("BuildAppleOsXCreateEntity: %v", err)
	}

	live := liveAppleOsXFull()
	out := OverlayAppleOsXUpdateEntity(live, planned)
	assertRestrictionsFieldOrder(t, out.Restrictions, "Update (overlay RMW)")
}
