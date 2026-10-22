package state

import (
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestMapAppleOsXSystemExtensions_NilInNilOut(t *testing.T) {
	t.Parallel()

	if got := mapAppleOsXSystemExtensions(nil, nil); got != nil {
		t.Errorf("mapAppleOsXSystemExtensions(nil) = %+v, want nil", got)
	}
}

func TestMapAppleOsXSystemExtensions_EmptyEntityIsNil(t *testing.T) {
	t.Parallel()

	// An SDK entity with every field at its zero value (no content at all)
	// must map to nil, exactly like appleOsXGatekeeperHasContent's contract —
	// otherwise every macOS profile read would spuriously populate an empty
	// system_extensions block.
	if got := mapAppleOsXSystemExtensions(&sdk.MacOsSystemExtensionsPayloadV2Model{}, nil); got != nil {
		t.Errorf("mapAppleOsXSystemExtensions(empty) = %+v, want nil", got)
	}
}

func TestMapAppleOsXSystemExtensions_FullyPopulated(t *testing.T) {
	t.Parallel()

	allowDriver := true
	allowEndpoint := false
	allowNetwork := true
	allowOverrides := false
	ent := &sdk.MacOsSystemExtensionsPayloadV2Model{
		AllowUserOverrides: &allowOverrides,
		AllowedSystemExtensionTypes: []sdk.MacOsAllowedSystemExtensionTypesV2Model{
			{
				TeamIdentifier:                     "ABCDE12345",
				AllowDriverExtensionType:           &allowDriver,
				AllowEndpointSecurityExtensionType: &allowEndpoint,
				AllowNetworkExtensionType:          &allowNetwork,
			},
		},
		AllowedSystemExtensions: []sdk.MacOsAllowedSystemExtensionV2Model{
			{BundleIdentifier: "com.example.extension", TeamIdentifier: "ABCDE12345"},
		},
	}

	got := mapAppleOsXSystemExtensions(ent, nil)
	if got == nil {
		t.Fatal("expected non-nil model")
	}
	if got.AllowUserOverrides.IsNull() || got.AllowUserOverrides.ValueBool() != false {
		t.Errorf("AllowUserOverrides = %v, want false", got.AllowUserOverrides)
	}
	if len(got.AllowedSystemExtensionTypes) != 1 {
		t.Fatalf("AllowedSystemExtensionTypes len = %d, want 1", len(got.AllowedSystemExtensionTypes))
	}
	tp := got.AllowedSystemExtensionTypes[0]
	if tp.TeamIdentifier.ValueString() != "ABCDE12345" {
		t.Errorf("TeamIdentifier = %q, want %q", tp.TeamIdentifier.ValueString(), "ABCDE12345")
	}
	if !tp.AllowDriverExtensionType.ValueBool() {
		t.Errorf("AllowDriverExtensionType = %v, want true", tp.AllowDriverExtensionType)
	}
	if tp.AllowEndpointSecurityExtensionType.IsNull() || tp.AllowEndpointSecurityExtensionType.ValueBool() {
		t.Errorf("AllowEndpointSecurityExtensionType = %v, want false (non-null)", tp.AllowEndpointSecurityExtensionType)
	}
	if !tp.AllowNetworkExtensionType.ValueBool() {
		t.Errorf("AllowNetworkExtensionType = %v, want true", tp.AllowNetworkExtensionType)
	}
	if got.AllowedSystemExtensions[0].TeamIdentifier.ValueString() != "ABCDE12345" {
		t.Errorf("AllowedSystemExtensions[0].TeamIdentifier = %v, want ABCDE12345", got.AllowedSystemExtensions[0].TeamIdentifier)
	}
	if len(got.AllowedSystemExtensions) != 1 || got.AllowedSystemExtensions[0].BundleIdentifier.ValueString() != "com.example.extension" {
		t.Errorf("AllowedSystemExtensions = %+v, want 1 entry with bundle com.example.extension", got.AllowedSystemExtensions)
	}
}

// TestMapAppleOsXSystemExtensions_ServerWildcardDroppedWhenNotConfigured is the
// fail-on-revert core of the "*"-entry fix: confirmed live 2026-09-25 (as<internal-env>),
// UEM injects a "*" (global team identifier, all-deny) entry into
// AllowedSystemExtensionTypes alongside any explicit team entry, even though
// the user never configured one. Without the drop, this API-only entry would
// appear as permanent, un-editable drift on every plan.
func TestMapAppleOsXSystemExtensions_ServerWildcardDroppedWhenNotConfigured(t *testing.T) {
	t.Parallel()

	allowDriver, allowEndpoint, allowNetwork := true, true, false
	serverDefault := false
	ent := &sdk.MacOsSystemExtensionsPayloadV2Model{
		AllowedSystemExtensionTypes: []sdk.MacOsAllowedSystemExtensionTypesV2Model{
			{
				TeamIdentifier:                     "*",
				AllowDriverExtensionType:           &serverDefault,
				AllowEndpointSecurityExtensionType: &serverDefault,
				AllowNetworkExtensionType:          &serverDefault,
			},
			{
				TeamIdentifier:                     "ABCDE12345",
				AllowDriverExtensionType:           &allowDriver,
				AllowEndpointSecurityExtensionType: &allowEndpoint,
				AllowNetworkExtensionType:          &allowNetwork,
			},
		},
	}

	// No prior state/config at all (e.g. first read after create) -- no "*" was configured.
	got := mapAppleOsXSystemExtensions(ent, nil)
	if got == nil {
		t.Fatal("expected non-nil model")
	}
	if len(got.AllowedSystemExtensionTypes) != 1 {
		t.Fatalf("AllowedSystemExtensionTypes len = %d, want 1 (server \"*\" default must be dropped)", len(got.AllowedSystemExtensionTypes))
	}
	if got.AllowedSystemExtensionTypes[0].TeamIdentifier.ValueString() != "ABCDE12345" {
		t.Errorf("TeamIdentifier = %q, want %q", got.AllowedSystemExtensionTypes[0].TeamIdentifier.ValueString(), "ABCDE12345")
	}
}

// TestMapAppleOsXSystemExtensions_ExplicitWildcardKept proves a user who
// genuinely configures team_identifier = "*" themselves keeps that entry --
// the drop in mapAllowedSystemExtensionTypes must be conditioned on prior
// state/config, not an unconditional filter.
func TestMapAppleOsXSystemExtensions_ExplicitWildcardKept(t *testing.T) {
	t.Parallel()

	deny := false
	ent := &sdk.MacOsSystemExtensionsPayloadV2Model{
		AllowedSystemExtensionTypes: []sdk.MacOsAllowedSystemExtensionTypesV2Model{
			{
				TeamIdentifier:                     "*",
				AllowDriverExtensionType:           &deny,
				AllowEndpointSecurityExtensionType: &deny,
				AllowNetworkExtensionType:          &deny,
			},
		},
	}
	prior := &SystemExtensionsModel{
		AllowedSystemExtensionTypes: []AllowedSystemExtensionTypeModel{
			{TeamIdentifier: types.StringValue("*")},
		},
	}

	got := mapAppleOsXSystemExtensions(ent, prior)
	if got == nil {
		t.Fatal("expected non-nil model")
	}
	if len(got.AllowedSystemExtensionTypes) != 1 {
		t.Fatalf("AllowedSystemExtensionTypes len = %d, want 1 (user-configured \"*\" must be kept)", len(got.AllowedSystemExtensionTypes))
	}
	if got.AllowedSystemExtensionTypes[0].TeamIdentifier.ValueString() != "*" {
		t.Errorf("TeamIdentifier = %q, want %q", got.AllowedSystemExtensionTypes[0].TeamIdentifier.ValueString(), "*")
	}
}

// TestMergeAllowedSystemExtensionTypesWithPriorState_ServerReordersNoDiff is
// the fail-on-revert core of the keyed-merge fix: confirmed live 2026-09-25
// (as<internal-env>) that UEM does not preserve request order in its response. A
// positional merge would report every field of every reordered entry as
// "changed" even though nothing did, tripping Terraform's post-apply
// consistency check.
func TestMergeAllowedSystemExtensionTypesWithPriorState_ServerReordersNoDiff(t *testing.T) {
	t.Parallel()

	state := []AllowedSystemExtensionTypeModel{
		{TeamIdentifier: types.StringValue("AAAAA11111"), AllowDriverExtensionType: types.BoolValue(true)},
		{TeamIdentifier: types.StringValue("BBBBB22222"), AllowNetworkExtensionType: types.BoolValue(true)},
	}
	// api comes back in the OPPOSITE order from what was configured.
	api := []AllowedSystemExtensionTypeModel{
		{TeamIdentifier: types.StringValue("BBBBB22222"), AllowNetworkExtensionType: types.BoolValue(true)},
		{TeamIdentifier: types.StringValue("AAAAA11111"), AllowDriverExtensionType: types.BoolValue(true)},
	}

	got := mergeAllowedSystemExtensionTypesWithPriorState(api, state)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].TeamIdentifier.ValueString() != "AAAAA11111" {
		t.Errorf("got[0].TeamIdentifier = %q, want %q (must match state's order)", got[0].TeamIdentifier.ValueString(), "AAAAA11111")
	}
	if got[1].TeamIdentifier.ValueString() != "BBBBB22222" {
		t.Errorf("got[1].TeamIdentifier = %q, want %q (must match state's order)", got[1].TeamIdentifier.ValueString(), "BBBBB22222")
	}
}

// TestMergeAllowedSystemExtensionTypesWithPriorState_WildcardFirstVsLast
// pins the exact live-observed shape: the "*" entry came back FIRST from
// UEM even though it was configured LAST.
func TestMergeAllowedSystemExtensionTypesWithPriorState_WildcardFirstVsLast(t *testing.T) {
	t.Parallel()

	state := []AllowedSystemExtensionTypeModel{
		{TeamIdentifier: types.StringValue("ABCDE12345"), AllowDriverExtensionType: types.BoolValue(true)},
		{TeamIdentifier: types.StringValue("*"), AllowEndpointSecurityExtensionType: types.BoolValue(true)},
	}
	api := []AllowedSystemExtensionTypeModel{
		{TeamIdentifier: types.StringValue("*"), AllowEndpointSecurityExtensionType: types.BoolValue(true)},
		{TeamIdentifier: types.StringValue("ABCDE12345"), AllowDriverExtensionType: types.BoolValue(true)},
	}

	got := mergeAllowedSystemExtensionTypesWithPriorState(api, state)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].TeamIdentifier.ValueString() != "ABCDE12345" {
		t.Errorf("got[0].TeamIdentifier = %q, want %q", got[0].TeamIdentifier.ValueString(), "ABCDE12345")
	}
	if got[1].TeamIdentifier.ValueString() != "*" {
		t.Errorf("got[1].TeamIdentifier = %q, want %q", got[1].TeamIdentifier.ValueString(), "*")
	}
}

// TestMergeAllowedSystemExtensionTypesWithPriorState_DuplicateKeysDeterministic
// pins that duplicate team_identifier keys pair up by arrival order on each
// side, not ambiguously or by first-match-wins-forever.
func TestMergeAllowedSystemExtensionTypesWithPriorState_DuplicateKeysDeterministic(t *testing.T) {
	t.Parallel()

	state := []AllowedSystemExtensionTypeModel{
		{TeamIdentifier: types.StringValue("ABCDE12345"), AllowDriverExtensionType: types.BoolValue(true)},
		{TeamIdentifier: types.StringValue("ABCDE12345"), AllowNetworkExtensionType: types.BoolValue(true)},
	}
	// UEM returns every boolean populated, so each api entry carries
	// explicit false values; a reversed pairing would surface them.
	api := []AllowedSystemExtensionTypeModel{
		{TeamIdentifier: types.StringValue("ABCDE12345"), AllowDriverExtensionType: types.BoolValue(true), AllowNetworkExtensionType: types.BoolValue(false)},
		{TeamIdentifier: types.StringValue("ABCDE12345"), AllowDriverExtensionType: types.BoolValue(false), AllowNetworkExtensionType: types.BoolValue(true)},
	}

	got := mergeAllowedSystemExtensionTypesWithPriorState(api, state)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if !got[0].AllowDriverExtensionType.ValueBool() {
		t.Errorf("got[0].AllowDriverExtensionType = %v, want true (state's 1st duplicate pairs with api's 1st)", got[0].AllowDriverExtensionType)
	}
	if !got[1].AllowNetworkExtensionType.ValueBool() {
		t.Errorf("got[1].AllowNetworkExtensionType = %v, want true (state's 2nd duplicate pairs with api's 2nd)", got[1].AllowNetworkExtensionType)
	}
}

// TestMergeAllowedSystemExtensionTypesWithPriorState_ApiOnlyEntryAppended
// proves an api entry with no matching prior key (e.g. a kept non-default
// "*" UEM injected with no prior config at all -- the fresh-import case) is
// appended rather than dropped or mismatched onto an unrelated prior entry.
func TestMergeAllowedSystemExtensionTypesWithPriorState_ApiOnlyEntryAppended(t *testing.T) {
	t.Parallel()

	api := []AllowedSystemExtensionTypeModel{
		{TeamIdentifier: types.StringValue("*"), AllowEndpointSecurityExtensionType: types.BoolValue(true)},
	}

	got := mergeAllowedSystemExtensionTypesWithPriorState(api, nil)
	if len(got) != 1 || got[0].TeamIdentifier.ValueString() != "*" {
		t.Errorf("got = %+v, want the api-only \"*\" entry appended unchanged", got)
	}
}

// TestMergeAllowedSystemExtensionsWithPriorState_ServerReordersNoDiff is the
// same keyed-merge fix for the sibling allowed_system_extensions list.
func TestMergeAllowedSystemExtensionsWithPriorState_ServerReordersNoDiff(t *testing.T) {
	t.Parallel()

	state := []AllowedSystemExtensionModel{
		{BundleIdentifier: types.StringValue("com.example.a"), TeamIdentifier: types.StringValue("ABCDE12345")},
		{BundleIdentifier: types.StringValue("com.example.b"), TeamIdentifier: types.StringValue("FGHIJ67890")},
	}
	api := []AllowedSystemExtensionModel{
		{BundleIdentifier: types.StringValue("com.example.b"), TeamIdentifier: types.StringValue("FGHIJ67890")},
		{BundleIdentifier: types.StringValue("com.example.a"), TeamIdentifier: types.StringValue("ABCDE12345")},
	}

	got := mergeAllowedSystemExtensionsWithPriorState(api, state)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].BundleIdentifier.ValueString() != "com.example.a" {
		t.Errorf("got[0].BundleIdentifier = %q, want %q (must match state's order)", got[0].BundleIdentifier.ValueString(), "com.example.a")
	}
	if got[1].BundleIdentifier.ValueString() != "com.example.b" {
		t.Errorf("got[1].BundleIdentifier = %q, want %q (must match state's order)", got[1].BundleIdentifier.ValueString(), "com.example.b")
	}
}

// internal-ticket removed the single-field (bundle-only/team-only) fallback
// matching pass and the PreserveNull* calls in
// mergeAllowedSystemExtensionsWithPriorState (row #82 of the B16 audit):
// only a prior entry that set BOTH bundle_identifier and team_identifier is
// now reordered to match state, and the matched item is the api value
// unmodified. TestMergeAllowedSystemExtensionsWithPriorState_DuplicateKeysDeterministic
// mirrors the types-list duplicate-key test for the extensions list, updated
// for that contract: state[0]'s team is null (single-field), so it is no
// longer matched at all; only state[1] (bundle+team both set) is reordered.
func TestMergeAllowedSystemExtensionsWithPriorState_DuplicateKeysDeterministic(t *testing.T) {
	t.Parallel()

	state := []AllowedSystemExtensionModel{
		{BundleIdentifier: types.StringValue("com.example.dup"), TeamIdentifier: types.StringNull()},
		{BundleIdentifier: types.StringValue("com.example.dup"), TeamIdentifier: types.StringValue("ABCDE12345")},
	}
	api := []AllowedSystemExtensionModel{
		{BundleIdentifier: types.StringValue("com.example.dup"), TeamIdentifier: types.StringValue("ZZZZZ99999")},
		{BundleIdentifier: types.StringValue("com.example.dup"), TeamIdentifier: types.StringValue("ABCDE12345")},
	}

	got := mergeAllowedSystemExtensionsWithPriorState(api, state)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	// state[1]'s exact bundle+team match (api[1]) is reordered first, stored
	// unmodified (no PreserveNull).
	if got[0].TeamIdentifier.ValueString() != "ABCDE12345" {
		t.Errorf("got[0].TeamIdentifier = %v, want %q (exact-match reorder, api value unmodified)", got[0].TeamIdentifier, "ABCDE12345")
	}
	// api[0] was never claimed (state[0]'s team is null, no fallback anymore)
	// so it is appended in its original position, unmodified.
	if got[1].TeamIdentifier.ValueString() != "ZZZZZ99999" {
		t.Errorf("got[1].TeamIdentifier = %v, want %q (unclaimed api entry appended as-is)", got[1].TeamIdentifier, "ZZZZZ99999")
	}
}

func TestMergeSystemExtensionsWithPriorState_APINilReturnsNil(t *testing.T) {
	t.Parallel()

	if got := MergeSystemExtensionsWithPriorState(nil, &SystemExtensionsModel{}); got != nil {
		t.Errorf("got %+v, want nil", got)
	}
}

func TestMergeSystemExtensionsWithPriorState_StateNilSurfacesEverything(t *testing.T) {
	t.Parallel()

	api := &SystemExtensionsModel{AllowUserOverrides: types.BoolValue(true)}
	got := MergeSystemExtensionsWithPriorState(api, nil)
	if got != api {
		t.Errorf("expected the api value returned unchanged on import (state nil), got %+v", got)
	}
}

// internal-ticket removed MergeSystemExtensionsWithPriorState's PreserveNull on
// AllowUserOverrides (row #78 of the B16 audit) and
// mergeAllowedSystemExtensionTypesWithPriorState's PreserveNull* calls on
// each matched item (row #80): a field the API echoed back now survives
// even when the user's prior config left it null. Renamed from
// TestMergeSystemExtensionsWithPriorState_SuppressesUserUnsetFields, which
// asserted the opposite (removed) behavior.
func TestMergeSystemExtensionsWithPriorState_APIValueWinsEvenWhenUserLeftItUnset(t *testing.T) {
	t.Parallel()

	api := &SystemExtensionsModel{
		AllowUserOverrides: types.BoolValue(true), // API echoed a value the user never set.
		AllowedSystemExtensionTypes: []AllowedSystemExtensionTypeModel{
			{
				TeamIdentifier:                     types.StringValue("ABCDE12345"),
				AllowDriverExtensionType:           types.BoolValue(true), // user never set this either
				AllowEndpointSecurityExtensionType: types.BoolNull(),
				AllowNetworkExtensionType:          types.BoolNull(),
			},
		},
	}
	prior := &SystemExtensionsModel{
		AllowUserOverrides: types.BoolNull(), // user left this null in their own config
		AllowedSystemExtensionTypes: []AllowedSystemExtensionTypeModel{
			{
				TeamIdentifier:           types.StringValue("ABCDE12345"),
				AllowDriverExtensionType: types.BoolNull(), // user left this null too
			},
		},
	}

	got := MergeSystemExtensionsWithPriorState(api, prior)
	if got.AllowUserOverrides.IsNull() || !got.AllowUserOverrides.ValueBool() {
		t.Errorf("AllowUserOverrides = %v, want true (API value, no suppression)", got.AllowUserOverrides)
	}
	if got.AllowedSystemExtensionTypes[0].AllowDriverExtensionType.IsNull() || !got.AllowedSystemExtensionTypes[0].AllowDriverExtensionType.ValueBool() {
		t.Errorf("AllowedSystemExtensionTypes[0].AllowDriverExtensionType = %v, want true (API value, no suppression)", got.AllowedSystemExtensionTypes[0].AllowDriverExtensionType)
	}
	// TeamIdentifier WAS set by the user in prior state, and the API agrees — must survive.
	if got.AllowedSystemExtensionTypes[0].TeamIdentifier.ValueString() != "ABCDE12345" {
		t.Errorf("TeamIdentifier = %q, want %q (user-set field must survive)", got.AllowedSystemExtensionTypes[0].TeamIdentifier.ValueString(), "ABCDE12345")
	}
}

// internal-ticket removed the extensions-list's single-field fallback matching
// and PreserveNull (row #82): a state entry that only set BundleIdentifier
// (TeamIdentifier null) no longer matches any api entry at all, so the api
// entry passes through in its original position and value, unmodified.
func TestMergeSystemExtensionsWithPriorState_AllowedSystemExtensionsPassesThroughUnmatchedEntry(t *testing.T) {
	t.Parallel()

	api := &SystemExtensionsModel{
		AllowedSystemExtensions: []AllowedSystemExtensionModel{
			{BundleIdentifier: types.StringValue("com.example.extension"), TeamIdentifier: types.StringValue("ABCDE12345")},
		},
	}
	prior := &SystemExtensionsModel{
		AllowedSystemExtensions: []AllowedSystemExtensionModel{
			{BundleIdentifier: types.StringValue("com.example.extension"), TeamIdentifier: types.StringNull()},
		},
	}

	got := MergeSystemExtensionsWithPriorState(api, prior)
	if got.AllowedSystemExtensions[0].BundleIdentifier.ValueString() != "com.example.extension" {
		t.Errorf("BundleIdentifier = %q, want %q", got.AllowedSystemExtensions[0].BundleIdentifier.ValueString(), "com.example.extension")
	}
	if got.AllowedSystemExtensions[0].TeamIdentifier.ValueString() != "ABCDE12345" {
		t.Errorf("TeamIdentifier = %v, want %q (API value, no single-field match, no suppression)", got.AllowedSystemExtensions[0].TeamIdentifier, "ABCDE12345")
	}
}

// TestMapAppleOsXSystemExtensions_ImportKeepsNonDefaultWildcard: on import
// (no prior) a "*" entry that allows any type is not UEM's synthetic default
// and must be kept. Dropping it would leave it out of the onboarded HCL, and
// the next update would silently delete it from the profile.
func TestMapAppleOsXSystemExtensions_ImportKeepsNonDefaultWildcard(t *testing.T) {
	t.Parallel()

	allow, deny := true, false
	for _, tc := range []struct {
		name                      string
		driver, endpoint, network *bool
	}{
		{"driver", &allow, &deny, &deny},
		{"endpoint", &deny, &allow, &deny},
		{"network", nil, nil, &allow},
	} {
		ent := &sdk.MacOsSystemExtensionsPayloadV2Model{
			AllowedSystemExtensionTypes: []sdk.MacOsAllowedSystemExtensionTypesV2Model{{
				TeamIdentifier:                     "*",
				AllowDriverExtensionType:           tc.driver,
				AllowEndpointSecurityExtensionType: tc.endpoint,
				AllowNetworkExtensionType:          tc.network,
			}},
		}
		got := mapAppleOsXSystemExtensions(ent, nil)
		if got == nil || len(got.AllowedSystemExtensionTypes) != 1 || got.AllowedSystemExtensionTypes[0].TeamIdentifier.ValueString() != "*" {
			t.Errorf("%s: non-default \"*\" entry dropped on import: %+v", tc.name, got)
		}
	}
}

// TestReadProfileIntoState_AppleOsXSystemExtensions pins the Read wiring:
// ReadProfileIntoState must hydrate system_extensions from the entity and
// drop the server's synthetic "*" entry. internal-ticket removed the
// PreserveNull suppression (rows #78/#80): fields the API returned now
// survive even when the prior left them null.
func TestReadProfileIntoState_AppleOsXSystemExtensions(t *testing.T) {
	t.Parallel()

	allow, deny := true, false
	ent := &sdk.AppleOsXDeviceProfileEntityV2{
		SystemExtensions: &sdk.MacOsSystemExtensionsPayloadV2Model{
			AllowUserOverrides: &deny,
			AllowedSystemExtensionTypes: []sdk.MacOsAllowedSystemExtensionTypesV2Model{
				{TeamIdentifier: "*", AllowDriverExtensionType: &deny, AllowEndpointSecurityExtensionType: &deny, AllowNetworkExtensionType: &deny},
				{TeamIdentifier: "ABCDE12345", AllowDriverExtensionType: &deny, AllowEndpointSecurityExtensionType: &deny, AllowNetworkExtensionType: &allow},
			},
		},
	}
	data := &ProfileResourceModel{
		SystemExtensions: &SystemExtensionsModel{
			AllowedSystemExtensionTypes: []AllowedSystemExtensionTypeModel{{
				TeamIdentifier:            types.StringValue("ABCDE12345"),
				AllowNetworkExtensionType: types.BoolValue(true),
			}},
		},
	}
	ReadProfileIntoState(t.Context(), data, &sdk.ProfileResult{AppleOsX: ent})

	se := data.SystemExtensions
	if se == nil {
		t.Fatal("system_extensions not hydrated by ReadProfileIntoState")
	}
	if se.AllowUserOverrides.IsNull() || se.AllowUserOverrides.ValueBool() != false {
		t.Errorf("AllowUserOverrides = %v, want false (API value, no suppression)", se.AllowUserOverrides)
	}
	if len(se.AllowedSystemExtensionTypes) != 1 {
		t.Fatalf("AllowedSystemExtensionTypes len = %d, want 1", len(se.AllowedSystemExtensionTypes))
	}
	tp := se.AllowedSystemExtensionTypes[0]
	if tp.TeamIdentifier.ValueString() != "ABCDE12345" || !tp.AllowNetworkExtensionType.ValueBool() ||
		tp.AllowDriverExtensionType.IsNull() || tp.AllowDriverExtensionType.ValueBool() {
		t.Errorf("entry = %+v, want ABCDE12345 network=true driver=false (API value, no suppression)", tp)
	}

	// Removal: an entity without the payload must clear the block from state.
	ReadProfileIntoState(t.Context(), data, &sdk.ProfileResult{AppleOsX: &sdk.AppleOsXDeviceProfileEntityV2{}})
	if data.SystemExtensions != nil {
		t.Errorf("system_extensions = %+v, want nil after the payload is gone", data.SystemExtensions)
	}
}

// internal-ticket removed the single-field (bundle-only/team-only) fallback
// matching pass in mergeAllowedSystemExtensionsWithPriorState (row #82 of
// the B16 audit): a state entry that sets only one of bundle/team no longer
// matches anything, so it can no longer "steal" or "prefer" an api entry --
// it simply never claims one, and every api entry is appended in its
// original order and value. TestMergeAllowedSystemExtensionsWithPriorState_FallbackCannotStealExactMatch
// now asserts that the bundle-only state entry (state[0]) still cannot
// interfere with the exact bundle+team match (state[1]), which is the part
// of the contract that survives.
func TestMergeAllowedSystemExtensionsWithPriorState_FallbackCannotStealExactMatch(t *testing.T) {
	t.Parallel()

	state := []AllowedSystemExtensionModel{
		{BundleIdentifier: types.StringValue("com.example.x"), TeamIdentifier: types.StringNull()},
		{BundleIdentifier: types.StringValue("com.example.x"), TeamIdentifier: types.StringValue("TEAMTWO222")},
	}
	api := []AllowedSystemExtensionModel{
		{BundleIdentifier: types.StringValue("com.example.x"), TeamIdentifier: types.StringValue("TEAMTWO222")},
		{BundleIdentifier: types.StringValue("com.example.x"), TeamIdentifier: types.StringNull()},
	}

	got := mergeAllowedSystemExtensionsWithPriorState(api, state)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	// state[1]'s exact match (api[0]) is reordered first, unmodified.
	if got[0].TeamIdentifier.ValueString() != "TEAMTWO222" {
		t.Errorf("got[0].TeamIdentifier = %v, want TEAMTWO222 (exact match, unmodified)", got[0].TeamIdentifier)
	}
	// api[1] was never claimed (state[0]'s team is null, no fallback), so it
	// is appended in its original position.
	if !got[1].TeamIdentifier.IsNull() {
		t.Errorf("got[1].TeamIdentifier = %v, want null (unclaimed api entry appended as-is)", got[1].TeamIdentifier)
	}
}

// TestMergeAllowedSystemExtensionsWithPriorState_UnmatchedSingleFieldEntriesPassThrough
// replaces TestMergeAllowedSystemExtensionsWithPriorState_SingleFieldPrefersLiteralEcho:
// a bundle-only prior entry no longer matches anything, so both api entries
// simply pass through in their original order and value.
func TestMergeAllowedSystemExtensionsWithPriorState_UnmatchedSingleFieldEntriesPassThrough(t *testing.T) {
	t.Parallel()

	state := []AllowedSystemExtensionModel{
		{BundleIdentifier: types.StringValue("com.example.x"), TeamIdentifier: types.StringNull()},
	}
	api := []AllowedSystemExtensionModel{
		{BundleIdentifier: types.StringValue("com.example.x"), TeamIdentifier: types.StringValue("TEAMNINE99")},
		{BundleIdentifier: types.StringValue("com.example.x"), TeamIdentifier: types.StringNull()},
	}

	got := mergeAllowedSystemExtensionsWithPriorState(api, state)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].TeamIdentifier.ValueString() != "TEAMNINE99" {
		t.Errorf("got[0].TeamIdentifier = %v, want TEAMNINE99 (unmodified, original order)", got[0].TeamIdentifier)
	}
	if !got[1].TeamIdentifier.IsNull() {
		t.Errorf("got[1].TeamIdentifier = %v, want null (unmodified, original order)", got[1].TeamIdentifier)
	}
}

// TestMergeAllowedSystemExtensionsWithPriorState_TeamOnlyFallback replaced:
// a team-only prior entry (bundle null) no longer matches anything, so both
// api entries pass through unmodified in their original order.
func TestMergeAllowedSystemExtensionsWithPriorState_TeamOnlyNoLongerMatched(t *testing.T) {
	t.Parallel()

	state := []AllowedSystemExtensionModel{
		{BundleIdentifier: types.StringNull(), TeamIdentifier: types.StringValue("TEAMONE111")},
		{BundleIdentifier: types.StringValue("com.example.b"), TeamIdentifier: types.StringValue("TEAMTWO222")},
	}
	api := []AllowedSystemExtensionModel{
		{BundleIdentifier: types.StringValue("com.example.b"), TeamIdentifier: types.StringValue("TEAMTWO222")},
		{BundleIdentifier: types.StringValue("com.example.a"), TeamIdentifier: types.StringValue("TEAMONE111")},
	}

	got := mergeAllowedSystemExtensionsWithPriorState(api, state)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	// state[1] (bundle+team both set) reorders its exact match first.
	if got[0].BundleIdentifier.ValueString() != "com.example.b" || got[0].TeamIdentifier.ValueString() != "TEAMTWO222" {
		t.Errorf("got[0] = %+v, want com.example.b/TEAMTWO222 (exact match)", got[0])
	}
	// api[1] was never claimed (state[0]'s bundle is null, no team-only
	// fallback anymore), so it's appended in its original position/value.
	if got[1].BundleIdentifier.ValueString() != "com.example.a" || got[1].TeamIdentifier.ValueString() != "TEAMONE111" {
		t.Errorf("got[1] = %+v, want com.example.a/TEAMONE111 (unclaimed api entry appended as-is)", got[1])
	}
}
