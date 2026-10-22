package platform

// internal-task, task D1: a PERMANENT builder-coverage guard, per platform, per
// modeled section (see overlay.go's appleOsXModeledSections /
// androidModeledSections / appleiOSModeledSections / windows10ModeledSections
// / windowsRuggedModeledSections / linuxModeledSections /
// modeledGeneralV2Keys / modeledGeneralV4Keys — the single source of truth
// for what this provider claims to model).
//
// For each modeled section, this file:
//
//  1. Builds a FULLY-POPULATED profilemodels.ProfileResourceModel (every
//     schema attribute relevant to every modeled section set to a non-null,
//     non-empty value — see fullyPopulatedProfileModel/fullRestrictionsModel
//     below).
//  2. Runs the platform's Build*CreateEntity on it and computes, via
//     json.Marshal, the set of non-null LEAF paths the builder actually
//     produced within the section ("built").
//  3. Computes, via reflection over the SDK entity TYPE (not a value), every
//     leaf path the wire schema DECLARES within the section ("declared").
//  4. Asserts declared-minus-built ("unmodeled") equals a pinned expected
//     set, each entry carrying a one-line reason. Any unmodeled leaf NOT in
//     the pinned set fails loudly and actionably, so a new SDK field that
//     silently isn't covered — or a builder regression that stops setting a
//     field — is caught immediately instead of being masked-secret bait
//     (see masked.go) or a silent RMW data-loss risk (see overlay.go).
//
// General is handled specially (see generalV2TopLevelKeys/
// generalV4TopLevelKeys): the overlay for General is deliberately
// FIELD-level, not section-level (only modeledGeneralV2Keys /
// modeledGeneralV4Keys are overlaid from the plan; every other General field
// intentionally passes through from the live entity on Update). Running the
// generic declared-vs-built diff against General would just re-list that
// whole intentional design as "unmodeled", so General coverage is instead
// asserted as built-keys-equal-modeled-keys, plus the AssignedSmartGroups[]/
// ExcludedSmartGroups[].Name sub-leaf case (01o ruling g3).

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// --- generic leaf-path machinery ---

var jsonMarshalerType = reflect.TypeOf((*json.Marshaler)(nil)).Elem()

// declaredLeaves walks t (a reflect.Type, NOT a value) and records every
// possible JSON leaf path under prefix into out. A "leaf" is a JSON scalar
// (string/bool/number), a map, an interface, or any type implementing
// json.Marshaler (e.g. client.UEMTime, which marshals to a JSON string and
// whose own fields are all unexported — without this check it would
// silently vanish from the declared set instead of appearing as a leaf).
// Slices/arrays normalize to "[]". Struct fields use their JSON tag name,
// falling back to the Go field name; unexported, non-embedded fields and
// fields tagged json:"-" are skipped.
//
// EMBEDDED (Anonymous) fields are special-cased to mirror encoding/json's
// own field-promotion algorithm (encoding/json's typeFields), because json
// promotes the EXPORTED fields of an embedded struct even when the embedded
// field itself is "unexported" in the ordinary sense — i.e. f.PkgPath is
// set purely because the field's Go name (which, for an anonymous field, is
// the embedded TYPE's name) starts lowercase, NOT because the field is
// inaccessible to encoding/json. Treating that PkgPath-set embedded field
// like any other unexported field would silently drop its promoted leaves
// from the "declared" set, producing false coverage (a builder that never
// sets one of those leaves would wrongly look "fully covered" instead of
// surfacing as an unmodeled gap). So, following json's own rules:
//
//   - An unexported embedded field is only skipped entirely when its
//     (dereferenced) type is NOT a struct — an embedded unexported scalar
//     type has nothing to promote, so json ignores it, same as an ordinary
//     unexported field.
//   - An embedded struct (or pointer to struct) field with NO json tag (or
//     a tag with an empty name, e.g. just `json:",omitempty"`) is promoted:
//     its own fields are recursed into at the PARENT's prefix, with no name
//     segment of its own, regardless of whether the embedded type is
//     exported.
//   - An embedded struct field WITH a non-empty tag name is, per json's own
//     rules, NOT promoted — it becomes an ordinary named field under that
//     tag name, same as any other field.
func declaredLeaves(t reflect.Type, prefix string, out map[string]bool) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Implements(jsonMarshalerType) || reflect.PointerTo(t).Implements(jsonMarshalerType) {
		out[prefix] = true
		return
	}
	switch t.Kind() {
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)

			ft := f.Type
			for ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}

			if f.Anonymous {
				if f.PkgPath != "" && ft.Kind() != reflect.Struct {
					continue // unexported embedded non-struct: nothing to promote.
				}
				// Unexported embedded STRUCT fields are NOT skipped — json
				// still promotes their exported fields.
			} else if f.PkgPath != "" {
				continue // unexported, non-embedded field.
			}

			tag := f.Tag.Get("json")
			if tag == "-" {
				continue
			}
			name := strings.Split(tag, ",")[0]

			if name == "" && f.Anonymous && ft.Kind() == reflect.Struct {
				// Promoted: recurse into the embedded struct at the same
				// prefix, no name segment of its own.
				declaredLeaves(ft, prefix, out)
				continue
			}

			if name == "" {
				name = f.Name
			}
			child := name
			if prefix != "" {
				child = prefix + "." + name
			}
			declaredLeaves(f.Type, child, out)
		}
	case reflect.Slice, reflect.Array:
		declaredLeaves(t.Elem(), prefix+"[]", out)
	default:
		// string, bool, ints, floats, map, interface, chan, func, etc.
		out[prefix] = true
	}
}

// --- M2 probe: declaredLeaves must promote embedded-struct fields ---
//
// probeInner/probeOuter/probeTaggedInner/probeOuterTagged are made-up types
// (not real SDK types) exercising encoding/json's embedded-field promotion
// rules directly: probeInner is UNEXPORTED (lowercase type name) but its
// field A is exported, so encoding/json still promotes A onto probeOuter's
// top level. In contrast, probeOuterTagged embeds probeTaggedInner WITH a
// json tag, which per json's own rules means the embedded struct is NOT
// promoted — it becomes an ordinary named field.
type probeInner struct {
	A string `json:"A"`
}

type probeOuter struct {
	probeInner
	B string `json:"B"`
}

type probeTaggedInner struct {
	C string `json:"C"`
}

type probeOuterTagged struct {
	probeTaggedInner `json:"tagged"`
	D                string `json:"D"`
}

func TestDeclaredLeaves_PromotesEmbeddedStructFields(t *testing.T) {
	got := make(map[string]bool)
	declaredLeaves(reflect.TypeOf(probeOuter{}), "", got)
	want := map[string]bool{"A": true, "B": true}
	assertSetEqual(t, "declaredLeaves(probeOuter)", got, want)
}

func TestDeclaredLeaves_TaggedEmbeddedFieldIsNotPromoted(t *testing.T) {
	got := make(map[string]bool)
	declaredLeaves(reflect.TypeOf(probeOuterTagged{}), "", got)
	want := map[string]bool{"tagged.C": true, "D": true}
	assertSetEqual(t, "declaredLeaves(probeOuterTagged)", got, want)
}

// builtLeaves walks a decoded JSON value (from json.Unmarshal into
// interface{}) and records every NON-NULL leaf path under prefix into out.
func builtLeaves(v interface{}, prefix string, out map[string]bool) {
	switch val := v.(type) {
	case map[string]interface{}:
		for k, child := range val {
			p := k
			if prefix != "" {
				p = prefix + "." + k
			}
			builtLeaves(child, p, out)
		}
	case []interface{}:
		for _, item := range val {
			builtLeaves(item, prefix+"[]", out)
		}
	case nil:
		// absent/null: not a built leaf.
	default:
		out[prefix] = true
	}
}

// jsonRoundTrip marshals v and decodes it back into a generic tree of
// maps/slices/scalars, for use with builtLeaves.
func jsonRoundTrip(t *testing.T, v interface{}) interface{} {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal(%T): %v", v, err)
	}
	var out interface{}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	return out
}

// sectionUnmodeled computes (declared leaf paths under fieldName) minus
// (leaf paths actually built), where "declared" reflects over the STATIC
// TYPE of entityValue's field named fieldName, and "built" comes from
// json.Marshal-ing the ACTUAL value of that field.
func sectionUnmodeled(t *testing.T, entityValue interface{}, fieldName string) map[string]bool {
	t.Helper()
	rv := reflect.ValueOf(entityValue)
	for rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	fv := rv.FieldByName(fieldName)
	if !fv.IsValid() {
		t.Fatalf("field %q not found on %T", fieldName, entityValue)
	}

	declared := make(map[string]bool)
	declaredLeaves(fv.Type(), fieldName, declared)

	built := make(map[string]bool)
	builtLeaves(jsonRoundTrip(t, fv.Interface()), fieldName, built)

	out := make(map[string]bool)
	for k := range declared {
		if !built[k] {
			out[k] = true
		}
	}
	return out
}

// generalTopLevelKeys returns the set of top-level JSON keys present
// (non-null) on the built General entity. This is intentionally NOT a
// recursive leaf walk: General's overlay is field-level by design (see
// overlay.go), so most General fields are meant to stay unmodeled forever
// (they pass through from the live entity on Update) — a full declared-vs-
// built diff on General would just re-list that entire deliberate design as
// spurious "gaps".
func generalTopLevelKeys(t *testing.T, general interface{}) map[string]bool {
	t.Helper()
	raw := jsonRoundTrip(t, general)
	m, ok := raw.(map[string]interface{})
	if !ok {
		t.Fatalf("expected General to marshal to a JSON object, got %T (%v)", raw, raw)
	}
	out := make(map[string]bool, len(m))
	for k := range m {
		out[k] = true
	}
	return out
}

// --- pinned-expectation plumbing ---

// unmodeledEntry pins one expected-unmodeled leaf path with a one-line
// reason, per the D1 task's requirement that every pinned entry carry its
// justification inline.
type unmodeledEntry struct {
	path   string
	reason string
}

func unmodeledSet(entries ...unmodeledEntry) map[string]string {
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		out[e.path] = e.reason
	}
	return out
}

func setFromSlice(items ...string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, it := range items {
		out[it] = true
	}
	return out
}

func mergeBoolSets(sets ...map[string]bool) map[string]bool {
	out := make(map[string]bool)
	for _, s := range sets {
		for k := range s {
			out[k] = true
		}
	}
	return out
}

// assertUnmodeled fails the test with an actionable message if the computed
// unmodeled-leaf set (got) doesn't exactly match the pinned expected set.
func assertUnmodeled(t *testing.T, label string, got map[string]bool, expected map[string]string) {
	t.Helper()
	for path, reason := range expected {
		if !got[path] {
			t.Errorf("%s: pinned-unmodeled leaf %q (reason: %s) is now BUILT — the builder now sets this field (or it was removed from the SDK type); remove it from the pinned expected list", label, path, reason)
		}
	}
	for path := range got {
		if _, ok := expected[path]; !ok {
			t.Errorf("%s: leaf %q is unmodeled but NOT in the pinned expected list — either the builder no longer sets %q (regression: check builders.go), or this is a newly-declared SDK field the builder never covered (gap: pin it here with reason \"UNREVIEWED: found by D1\" and flag it for review)", label, path, path)
		}
	}
}

// assertSetEqual fails with an actionable message naming exactly which keys
// are missing/extra, sorted for stable diffs.
func assertSetEqual(t *testing.T, label string, got, want map[string]bool) {
	t.Helper()
	var missing, extra []string
	for k := range want {
		if !got[k] {
			missing = append(missing, k)
		}
	}
	for k := range got {
		if !want[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 {
		t.Errorf("%s: builder no longer sets %v (expected these General keys to be built)", label, missing)
	}
	if len(extra) > 0 {
		t.Errorf("%s: builder now sets %v, which is not in modeledGeneralV2Keys/modeledGeneralV4Keys — update overlay.go's modeled-keys map to match, or this is an unreviewed General field leak", label, extra)
	}
}

// checkModeledSectionSet is the "meta" guard: it fails if the D1 coverage
// tests below (via the `tested` set) drift from what overlay.go actually
// declares modeled (`modeled`), in either direction — so this permanent
// guard can't silently go stale if overlay.go's modeled-section set changes.
func checkModeledSectionSet(t *testing.T, label string, modeled, tested map[string]bool) {
	t.Helper()
	for k := range modeled {
		if !tested[k] {
			t.Errorf("%s: overlay.go declares modeled section %q but the D1 builder-coverage guard does not test it — add coverage", label, k)
		}
	}
	for k := range tested {
		if !modeled[k] {
			t.Errorf("%s: D1 guard tests section %q, but overlay.go no longer lists it as modeled — remove or reconcile", label, k)
		}
	}
}

// --- fully-populated model fixture ---

func strList(items ...string) types.List {
	elems := make([]attr.Value, len(items))
	for i, it := range items {
		elems[i] = types.StringValue(it)
	}
	return types.ListValueMust(types.StringType, elems)
}

func fullMediaAccessModel() *profilemodels.RestrictionsMediaAccessModel {
	return &profilemodels.RestrictionsMediaAccessModel{
		Allow:        types.BoolValue(true),
		Authenticate: types.BoolValue(true),
		ReadOnly:     types.BoolValue(true),
	}
}

func fullPreferencesModel() *profilemodels.RestrictionsPreferencesModel {
	return &profilemodels.RestrictionsPreferencesModel{
		Accessibility:          types.BoolValue(true),
		AppStore:               types.BoolValue(true),
		Bluetooth:              types.BoolValue(true),
		CDsAndDVDs:             types.BoolValue(true),
		DateAndTime:            types.BoolValue(true),
		DesktopAndScreenSaver:  types.BoolValue(true),
		DictationAndSpeech:     types.BoolValue(true),
		Displays:               types.BoolValue(true),
		Dock:                   types.BoolValue(true),
		EnabledPreferencePanes: types.BoolValue(true),
		EnergySaver:            types.BoolValue(true),
		Extensions:             types.BoolValue(true),
		FibreChannel:           types.BoolValue(true),
		FlashPlayer:            types.BoolValue(true),
		General:                types.BoolValue(true),
		Ink:                    types.BoolValue(true),
		InternetAccounts:       types.BoolValue(true),
		Keyboard:               types.BoolValue(true),
		LanguageAndText:        types.BoolValue(true),
		MissionControl:         types.BoolValue(true),
		MobileMe:               types.BoolValue(true),
		Mouse:                  types.BoolValue(true),
		Network:                types.BoolValue(true),
		Notifications:          types.BoolValue(true),
		ParentalControls:       types.BoolValue(true),
		PreferenceBehavior:     types.StringValue("Blacklist"),
		PrintAndScan:           types.BoolValue(true),
		Profiles:               types.BoolValue(true),
		SecurityAndPrivacy:     types.BoolValue(true),
		Sharing:                types.BoolValue(true),
		SoftwareUpdate:         types.BoolValue(true),
		Sound:                  types.BoolValue(true),
		Spotlight:              types.BoolValue(true),
		StartupDisk:            types.BoolValue(true),
		TimeMachine:            types.BoolValue(true),
		Trackpad:               types.BoolValue(true),
		UsersAndGroups:         types.BoolValue(true),
		Xsan:                   types.BoolValue(true),
		ICloud:                 types.BoolValue(true),
	}
}

func fullRestrictionsModel() *profilemodels.RestrictionsModel {
	return &profilemodels.RestrictionsModel{
		Applications: &profilemodels.RestrictionsApplicationsModel{
			AllowApplication: strList("com.example.allowed"),
			AllowFolders:     strList("/Applications/Allowed"),
			DisallowFolders:  strList("/Applications/Disallowed"),
			RestrictWhichApplicationsAreAllowedToLaunch: types.BoolValue(true),
			AppStore: &profilemodels.RestrictionsAppStoreModel{
				AllowAppStoreAppAdoption:                 types.BoolValue(true),
				RequireAdminPasswordToInstallOrUpdateApp: types.BoolValue(true),
				RestrictAppStoreToSoftwareUpdatesOnly:    types.BoolValue(true),
			},
			AppleMusic: &profilemodels.RestrictionsAppleMusicModel{AllowMusicService: types.BoolValue(true)},
			Camera:     &profilemodels.RestrictionsCameraModel{AllowUseOfBuiltInCamera: types.BoolValue(true)},
			GameCentre: &profilemodels.RestrictionsGameCentreModel{
				AllowAddingGameCenterFriends: types.BoolValue(true),
				AllowGameCenterModification:  types.BoolValue(true),
				AllowMultiplayerGaming:       types.BoolValue(true),
				AllowUseOfGameCenter:         types.BoolValue(true),
			},
			Safari: &profilemodels.RestrictionsSafariModel{
				AllowDeprecatedWebKitTls: types.BoolValue(true),
				AllowSafariAutoFill:      types.BoolValue(true),
			},
		},
		Desktop: &profilemodels.RestrictionsDesktopModel{
			DesktopPicturePath: types.StringValue("/Library/Desktop Pictures/corp.png"),
			LockDesktopPicture: types.BoolValue(true),
		},
		Functionality: &profilemodels.RestrictionsFunctionalityModel{
			AirPrint: &profilemodels.RestrictionsAirPrintModel{
				AllowAirPrint:                      types.BoolValue(true),
				AllowAirPrintiBeaconDiscovery:      types.BoolValue(true),
				ForceAirPrintTrustedTLSRequirement: types.BoolValue(true),
			},
			ContentCaching: &profilemodels.RestrictionsContentCachingModel{AllowContentCaching: types.BoolValue(true)},
			ICloud: &profilemodels.RestrictionsICloudModel{
				AllowAirPrint:                          types.BoolValue(true),
				AllowAirPrintiBeaconDiscovery:          types.BoolValue(true),
				AllowCloudDesktopAndDocuments:          types.BoolValue(true),
				AllowDeprecatedWebKitTls:               types.BoolValue(true),
				AllowICloudFMM:                         types.BoolValue(true),
				AllowIcloudAddressBook:                 types.BoolValue(true),
				AllowIcloudBTMM:                        types.BoolValue(true),
				AllowIcloudBookmarks:                   types.BoolValue(true),
				AllowIcloudCalendar:                    types.BoolValue(true),
				AllowIcloudDocumentsAndData:            types.BoolValue(true),
				AllowIcloudKeychainSync:                types.BoolValue(true),
				AllowIcloudMail:                        types.BoolValue(true),
				AllowIcloudNotes:                       types.BoolValue(true),
				AllowIcloudReminders:                   types.BoolValue(true),
				AllowPasswordAutoFill:                  types.BoolValue(true),
				AllowPasswordProximityRequests:         types.BoolValue(true),
				AllowPasswordSharing:                   types.BoolValue(true),
				AllowUseIcloudPasswordForLocalAccounts: types.BoolValue(true),
				ForceAirPrintTrustedTLSRequirement:     types.BoolValue(true),
			},
			Passwords: &profilemodels.RestrictionsPasswordsModel{
				AllowPasswordAutoFill:          types.BoolValue(true),
				AllowPasswordProximityRequests: types.BoolValue(true),
				AllowPasswordSharing:           types.BoolValue(true),
			},
			Spotlight: &profilemodels.RestrictionsSpotlightModel{AllowSpotlightSuggestions: types.BoolValue(true)},
		},
		Media: &profilemodels.RestrictionsMediaModel{
			AutoEjectMedia:              types.BoolValue(true),
			DiskMediaCDs:                fullMediaAccessModel(),
			DiskMediaDVDs:               fullMediaAccessModel(),
			ExternalHardDiskMediaAccess: fullMediaAccessModel(),
			HardDiskDvdRam:              fullMediaAccessModel(),
			HardDiskImages:              fullMediaAccessModel(),
			InternalHardDiskMediaAccess: fullMediaAccessModel(),
			NetworkAccess:               &profilemodels.RestrictionsNetworkAccessModel{AirDrop: types.BoolValue(true)},
			RecordableDisc:              &profilemodels.RestrictionsBurnSupportModel{BurnSupport: fullMediaAccessModel()},
		},
		Preferences: fullPreferencesModel(),
		Sharing: &profilemodels.RestrictionsSharingModel{
			AddtoAperture:                          types.BoolValue(true),
			AddtoReadingList:                       types.BoolValue(true),
			AddtoiPhoto:                            types.BoolValue(true),
			AirDrop:                                types.BoolValue(true),
			AutomaticallyEnableNewSharingServices:  types.BoolValue(true),
			Facebook:                               types.BoolValue(true),
			Mail:                                   types.BoolValue(true),
			Messages:                               types.BoolValue(true),
			RestrictWhichSharingServicesAreEnabled: types.BoolValue(true),
			SinaWeibo:                              types.BoolValue(true),
			Twitter:                                types.BoolValue(true),
			VideoServices:                          types.BoolValue(true),
		},
		Widgets: &profilemodels.RestrictionsWidgetsModel{
			AllowOnlyConfiguredWidgets: types.BoolValue(true),
			AllowedWidgets:             strList("com.apple.widget.weather"),
		},
	}
}

// fullyPopulatedProfileModel returns a ProfileResourceModel with EVERY
// schema attribute relevant to EVERY modeled section (across all 6
// platforms) set to a non-null, non-empty, valid value. There are no
// ConflictsWith/oneOf validators on this schema, so every block can coexist
// on one model — the same fixture is reused for every platform's
// Build*CreateEntity, since each builder only reads the fields it cares
// about and ignores the rest.
func fullyPopulatedProfileModel() *profilemodels.ProfileResourceModel {
	return &profilemodels.ProfileResourceModel{
		ID:                  types.StringValue("1"),
		Name:                types.StringValue("D1 Coverage Guard Profile"),
		Description:         types.StringValue("D1 builder coverage guard fixture"),
		Platform:            types.StringValue("dummy"),
		OrgGroupID:          types.StringValue("14165"),
		AssignmentType:      types.StringValue("Auto"),
		ProfileScope:        types.StringValue("Production"),
		IsActive:            types.BoolValue(true),
		LockScreenMessage:   types.StringValue("Lock screen message"),
		UUID:                types.StringValue("uuid-1"),
		ProfileContext:      types.StringValue("Device"),
		AssignedSmartGroups: []types.String{types.StringValue("101"), types.StringValue("102")},
		ExcludedSmartGroups: []types.String{types.StringValue("201")},
		Passcode: &profilemodels.PasscodeModel{
			RequirePasscodeOnDevice:          types.BoolValue(true),
			AllowSimpleValue:                 types.BoolValue(false),
			RequireAlphanumericValue:         types.BoolValue(true),
			MinimumPasscodeLength:            types.Int64Value(6),
			MinimumNumberOfComplexCharacters: types.StringValue("2"),
			MaximumPasscodeAge:               types.StringValue("90"),
			AutoLock:                         types.StringValue("5"),
			GracePeriod:                      types.Int64Value(15),
			MaxFailedAttempts:                types.StringValue("10"),
			PinHistory:                       types.StringValue("3"),
			MinutesUntilFailedLoginReset:     types.Int64Value(15),
		},
		CustomSettingsList: []profilemodels.CustomSettingsItemModel{
			{CustomSettings: types.StringValue("<plist>settings</plist>")},
		},
		NetworkList: []profilemodels.NetworkItemModel{
			{
				NetworkInterface:                   types.StringValue("BuiltInWireless"),
				ServiceSetIdentifier:               types.StringValue("corp-ssid"),
				HiddenNetwork:                      types.BoolValue(true),
				AutoJoin:                           types.BoolValue(true),
				SecurityType:                       types.StringValue("WPA2"),
				Password:                           types.StringValue("wifi-pass"),
				UseAsLoginWindowConfiguration:      types.BoolValue(true),
				UseDirectoryAuthentication:         types.BoolValue(true),
				TLS:                                types.BoolValue(true),
				TTLS:                               types.BoolValue(true),
				LEAP:                               types.BoolValue(true),
				PEAP:                               types.BoolValue(true),
				EAPFAST:                            types.BoolValue(true),
				EAPSIM:                             types.BoolValue(true),
				EAPAKA:                             types.BoolValue(true),
				TLSMinimumVersion:                  types.StringValue("1.0"),
				TLSMaximumVersion:                  types.StringValue("1.2"),
				DisableAssociationMACRandomization: types.BoolValue(true),
				UserName:                           types.StringValue("network-user"),
				UserPassword:                       types.StringValue("network-pass"),
				IdentityCertificate:                types.StringValue("cert-1"),
				InnerIdentity:                      types.StringValue("inner-id"),
				OuterIdentity:                      types.StringValue("outer-id"),
				UsePAC:                             types.BoolValue(true),
				AllowTwoRANDs:                      types.BoolValue(true),
				TrustedCertificates:                strList("cert-a", "cert-b"),
				AllowTrustExceptions:               types.BoolValue(true),
				ProxyType:                          types.StringValue("Manual"),
				ProxyServer:                        types.StringValue("proxy.example.com"),
				ProxyServerPort:                    types.Int64Value(8080),
				ProxyUsername:                      types.StringValue("proxy-user"),
				ProxyPassword:                      types.StringValue("proxy-pass"),
				ProxyUrl:                           types.StringValue("http://proxy.example.com/pac"),
				PacFallback:                        types.BoolValue(true),
			},
		},
		CredentialsList: []profilemodels.CredentialItemModel{
			{
				CredentialSource:             types.StringValue("Upload"),
				CredentialName:               types.StringValue("corp-cert"),
				CertificatePayload:           types.StringValue("base64-cert-data"),
				CertificatePassword:          types.StringValue("cert-pass"),
				CertificateID:                types.Int64Value(555),
				CertificateAuthority:         types.Int64Value(7),
				CertificateTemplate:          types.Int64Value(3),
				AllowAccessToAllApplications: types.BoolValue(true),
				KeyIsExtractable:             types.BoolValue(true),
			},
		},
		DiskEncryption: &profilemodels.DiskEncryptionModel{
			AirWatch: &profilemodels.DiskEncryptionAirWatchModel{
				StoreKey:                                    types.BoolValue(true),
				RotateKeyAfter:                              types.Int64Value(30),
				UseIntelligentHub:                           types.BoolValue(true),
				NotifyUserForEncryption:                     types.BoolValue(true),
				EncryptionNotificationTitle:                 types.StringValue("Encrypt now"),
				EncryptionNotificationMessage:               types.StringValue("Please encrypt"),
				EncryptionMaxNotifyAttempts:                 types.Int64Value(3),
				EncryptionNotificationRetryIntervalInHours:  types.Int64Value(4),
				EncryptionActionAfterLastNotification:       types.Int64Value(1),
				EnableRecoveryKey:                           types.BoolValue(true),
				RecoveryKeyNotificationTitle:                types.StringValue("Recovery key"),
				RecoveryKeyNotificationMessage:              types.StringValue("Rotate key"),
				RecoveryKeyNotificationRetryIntervalInHours: types.Int64Value(6),
				RecoveryKeyPromptTitle:                      types.StringValue("Prompt title"),
				RecoveryKeyPromptMessage:                    types.StringValue("Prompt message"),
				RecoveryKeySuccessTitle:                     types.StringValue("Success title"),
				RecoveryKeySuccessMessage:                   types.StringValue("Success message"),
				RecoveryKeyErrorTitle:                       types.StringValue("Error title"),
				RecoveryKeyErrorMessage:                     types.StringValue("Error message"),
				RecoveryKeyMaxFailureCount:                  types.Int64Value(5),
			},
			FileVault: &profilemodels.DiskEncryptionFileVaultModel{
				Enable:                         types.BoolValue(true),
				ShowRecoveryKey:                types.BoolValue(true),
				RecoveryType:                   types.Int64Value(1),
				FileVaultEnterpriseCertificate: types.StringValue("Certificate #1"),
				FileVaultUser:                  types.Int64Value(1),
				Username:                       types.StringValue("filevault-user"),
				PromptToEnableFileVaultAt:      types.Int64Value(1),
				NumberOfTimesUserCanBypass:     types.Int64Value(3),
			},
			MCX: types.ObjectValueMust(profilemodels.DiskEncryptionMCXAttrTypes, map[string]attr.Value{
				"destroy_fv_key_on_standby": types.BoolValue(true),
			}),
		},
		Gatekeeper: &profilemodels.GatekeeperModel{
			AllowAutoUnlock:              types.BoolValue(true),
			AllowFingerprintForUnlock:    types.BoolValue(true),
			AllowHandoff:                 types.BoolValue(true),
			AllowScreenCapture:           types.BoolValue(true),
			EnableAppSoftwareUpdateDelay: types.BoolValue(true),
			EnableSoftwareUpdateDelay:    types.BoolValue(true),
			EnforcedSoftwareUpdateDelay:  types.Int64Value(7),
		},
		Restrictions: fullRestrictionsModel(),
		SystemExtensions: &profilemodels.SystemExtensionsModel{
			AllowUserOverrides: types.BoolValue(true),
			AllowedSystemExtensionTypes: []profilemodels.AllowedSystemExtensionTypeModel{
				{
					TeamIdentifier:                     types.StringValue("ABCDE12345"),
					AllowDriverExtensionType:           types.BoolValue(true),
					AllowEndpointSecurityExtensionType: types.BoolValue(true),
					AllowNetworkExtensionType:          types.BoolValue(false),
				},
			},
			AllowedSystemExtensions: []profilemodels.AllowedSystemExtensionModel{
				{
					BundleIdentifier: types.StringValue("com.example.extension"),
					TeamIdentifier:   types.StringValue("ABCDE12345"),
				},
			},
		},
		ScepList: []profilemodels.ScepItemModel{
			{
				Name:                    types.StringValue("SCEP #1"),
				CredentialSource:        types.StringValue("DefinedCA"),
				CertificateAuthorityID:  types.Int64Value(1001),
				CertificateTemplateID:   types.Int64Value(2002),
				AllowExportFromKeyChain: types.BoolValue(true),
				IdentityPreference: &profilemodels.ScepIdentityPreferenceModel{
					Names: types.ListValueMust(types.StringType, []attr.Value{types.StringValue("id.example")}),
				},
			},
		},
		VpnList: []profilemodels.VPNItemModel{
			{
				Account:              types.StringValue("account-value"),
				AppMapping:           types.BoolValue(true),
				ApplicationBundleID:  types.ListValueMust(types.StringType, []attr.Value{types.StringValue("applicationbundleid")}),
				AssociatedDomains:    types.ListValueMust(types.StringType, []attr.Value{types.StringValue("associateddomains")}),
				CalendarDomains:      types.ListValueMust(types.StringType, []attr.Value{types.StringValue("calendardomains")}),
				ConnectAutomatically: types.BoolValue(true),
				ConnectionName:       types.StringValue("connectionname-value"),
				ConnectionType:       types.StringValue("connectiontype-value"),
				ContactsDomains:      types.ListValueMust(types.StringType, []attr.Value{types.StringValue("contactsdomains")}),
				CustomDatas: []profilemodels.VPNItemCustomDatasModel{
					{
						Key:   types.StringValue("key-value"),
						Value: types.StringValue("value-value"),
					},
				},
				EnableSafariDomains:           types.BoolValue(true),
				EnableVPNOnDemand:             types.BoolValue(true),
				EncryptionLevel:               types.Int64Value(1),
				ExcludeLocalNetworks:          types.BoolValue(true),
				ExcludedDomains:               types.ListValueMust(types.StringType, []attr.Value{types.StringValue("excludeddomains")}),
				GroupName:                     types.StringValue("groupname-value"),
				IdentityCertificate:           types.StringValue("identitycertificate-value"),
				IncludeAllNetworks:            types.BoolValue(true),
				IncludeUserPIN:                types.BoolValue(true),
				MachineAuthentication:         types.Int64Value(1),
				MailDomains:                   types.ListValueMust(types.StringType, []attr.Value{types.StringValue("maildomains")}),
				MdmAssignedID:                 types.StringValue("mdmassignedid-value"),
				MdmDeviceSerialNumber:         types.StringValue("mdmdeviceserialnumber-value"),
				MdmDeviceUniqueID:             types.StringValue("mdmdeviceuniqueid-value"),
				MdmDeviceWifiMACAddress:       types.StringValue("mdmdevicewifimacaddress-value"),
				Password:                      types.StringValue("password-value"),
				PerAppVPN:                     types.BoolValue(true),
				Port:                          types.Int64Value(1),
				PromptForPassword:             types.BoolValue(true),
				ProviderDesignatedRequirement: types.StringValue("providerdesignatedrequirement-value"),
				ProviderType:                  types.StringValue("providertype-value"),
				Proxy:                         types.StringValue("proxy-value"),
				ProxyServer:                   types.StringValue("proxyserver-value"),
				ProxyServerAutoConfigURL:      types.StringValue("proxyserverautoconfigurl-value"),
				SafariDomains:                 types.ListValueMust(types.StringType, []attr.Value{types.StringValue("safaridomains")}),
				SendAllTraffic:                types.BoolValue(true),
				Server:                        types.StringValue("server-value"),
				SharedSecret:                  types.StringValue("sharedsecret-value"),
				UseHybridAuthentication:       types.BoolValue(true),
				UserAuthentication:            types.StringValue("userauthentication-value"),
				UserName:                      types.StringValue("username-value"),
				VPNOnDemand: []profilemodels.VPNItemVPNOnDemandModel{
					{
						Domain:         types.StringValue("domain-value"),
						OnDemandAction: types.StringValue("ondemandaction-value"),
					},
				},
				VPNPassword: types.StringValue("vpnpassword-value"),
				WebLogon:    types.BoolValue(true),
			},
		},
		EasMicrosoftOutlook: &profilemodels.EasMicrosoftOutlookModel{
			AccountName:                types.StringValue("accountname-value"),
			DirectoryServer:            types.StringValue("directoryserver-value"),
			DirectoryServerPort:        types.StringValue("directoryserverport-value"),
			DirectoryServerRequiresSSL: types.BoolValue(true),
			Domain:                     types.StringValue("domain-value"),
			EmailAddress:               types.StringValue("emailaddress-value"),
			ExchangeHost:               types.StringValue("exchangehost-value"),
			ExchangePort:               types.StringValue("exchangeport-value"),
			Password:                   types.StringValue("password-value"),
			SearchBase:                 types.StringValue("searchbase-value"),
			UseSSL:                     types.BoolValue(true),
			UserName:                   types.StringValue("username-value"),
		},
		KernelExtension: &profilemodels.KernelExtensionModel{
			AllowUserOverrides: types.BoolValue(true),
			AllowedKernelExtensions: []profilemodels.KernelExtensionAllowedKernelExtensionsModel{
				{
					BundleIdentifier: types.StringValue("bundleidentifier-value"),
					TeamIdentifier:   types.StringValue("teamidentifier-value"),
				},
			},
			AllowedTeamIdentifiers: types.ListValueMust(types.StringType, []attr.Value{types.StringValue("allowedteamidentifiers")}),
		},
		CustomAttributes: []profilemodels.CustomAttributeModel{
			{
				AttributeName:   types.StringValue("attributename-value"),
				AttributeScript: types.StringValue("attributescript-value"),
				Events:          types.ListValueMust(types.StringType, []attr.Value{types.StringValue("events")}),
				Schedule:        types.Int64Value(1),
			},
		}, WebClipsList: []profilemodels.WebClipItemModel{
			{
				Label:            types.StringValue("Portal"),
				URL:              types.StringValue("https://example.invalid"),
				ShowInAppCatalog: types.BoolValue(true),
				Icon:             types.Int64Value(3001),
			},
		},
		PrivacyPreferences: []profilemodels.PrivacyPreferenceModel{
			{
				Identifier:      types.StringValue("com.example.app"),
				IdentifierType:  types.StringValue("bundleID"),
				CodeRequirement: types.StringValue("identifier \"com.example.app\""),
				Comment:         types.StringValue("PPPC rule comment"),
				AppleEventsList: []profilemodels.AppleEventModel{
					{
						CodeRequirement: types.StringValue("identifier \"com.example.receiver\""),
						Identifier:      types.StringValue("com.example.receiver"),
						IdentifierType:  types.StringValue("bundleID"),
						Permission:      types.StringValue("Allow"),
					},
				},
				StaticCode:                   types.BoolValue(true),
				Accessibility:                types.StringValue("Allow"),
				AddressBook:                  types.StringValue("Allow"),
				Calendar:                     types.StringValue("Allow"),
				Camera:                       types.StringValue("Disallow"),
				FileProviderPresence:         types.StringValue("Allow"),
				ListenEvent:                  types.StringValue("Disallow"),
				MediaLibrary:                 types.StringValue("Allow"),
				Microphone:                   types.StringValue("Disallow"),
				Photos:                       types.StringValue("Allow"),
				PostEvent:                    types.StringValue("Allow"),
				Reminders:                    types.StringValue("Allow"),
				ScreenCapture:                types.StringValue("Disallow"),
				SpeechRecognition:            types.StringValue("Allow"),
				SystemPolicyAllFiles:         types.StringValue("Allow"),
				SystemPolicyDesktopFolder:    types.StringValue("Allow"),
				SystemPolicyDocumentsFolder:  types.StringValue("Allow"),
				SystemPolicyDownloadsFolder:  types.StringValue("Allow"),
				SystemPolicyNetworkVolumes:   types.StringValue("Allow"),
				SystemPolicyRemovableVolumes: types.StringValue("Allow"),
				SystemPolicySysAdminFiles:    types.StringValue("Allow"),
			},
		},
	}
}

// --- General coverage (shared across all V2 platforms, plus V4/Linux) ---

// generalV2SmartGroupUnmodeled returns the unmodeled sub-leaves under
// General.AssignedSmartGroups[]/ExcludedSmartGroups[] for any V2 platform's
// built General entity.
func generalV2SmartGroupUnmodeled(t *testing.T, general interface{}) map[string]bool {
	t.Helper()
	return mergeBoolSets(
		sectionUnmodeled(t, general, "AssignedSmartGroups"),
		sectionUnmodeled(t, general, "ExcludedSmartGroups"),
	)
}

var expectedSmartGroupNameUnmodeled = unmodeledSet(
	unmodeledEntry{"AssignedSmartGroups[].Name", "UEM derives Name from SmartGroupID; intentionally not carried (01o ruling g3)"},
	unmodeledEntry{"ExcludedSmartGroups[].Name", "UEM derives Name from SmartGroupID; intentionally not carried (01o ruling g3)"},
)

// d1PinnedUnmodeled is the single package-level source of truth for D1's
// pinned expected-unmodeled leaf sets, keyed by platform then modeled
// section (mirroring the shape of overlay.go's *ModeledSections maps). Both
// TestBuilderCoverageGuard_* (D1, below) and
// TestKeptLiveLeaves_MatchD1PinnedGaps (M3 drift guard,
// overlay_deep_merge_test.go) read from this SAME map — before this
// refactor, the drift guard instead compared production's kept-leaf lists
// against a second, independently hard-coded copy of just the g1/g2
// entries, which could silently drift out of sync with D1's real pinned
// set. Now there is exactly one place that says what D1 pins as unmodeled;
// the drift guard derives its expectation from it instead of restating it.
//
// General is intentionally NOT a key here: its coverage is asserted
// differently (built-keys-equal-modeled-keys, see
// generalV2SmartGroupUnmodeled/expectedSmartGroupNameUnmodeled above) since
// the overlay for General is deliberately field-level, not section-level.
var d1PinnedUnmodeled = map[string]map[string][]unmodeledEntry{
	"appleOsX": {
		"Passcode":           nil,
		"CustomSettingsList": nil,
		"NetworkList":        nil,
		"CredentialsList": {
			{"CredentialsList[].CertificateMetadata.CertificateUuid", "gap g2: buildAppleOsXCredentialsListEntity never sets CertificateMetadata"},
			{"CredentialsList[].CertificateMetadata.IssueSerialNumber", "gap g2: buildAppleOsXCredentialsListEntity never sets CertificateMetadata"},
			{"CredentialsList[].CertificateMetadata.IssuerName", "gap g2: buildAppleOsXCredentialsListEntity never sets CertificateMetadata"},
			{"CredentialsList[].CertificateMetadata.SubjectName", "gap g2: buildAppleOsXCredentialsListEntity never sets CertificateMetadata"},
			{"CredentialsList[].CertificateMetadata.Thumbprint", "gap g2: buildAppleOsXCredentialsListEntity never sets CertificateMetadata"},
			{"CredentialsList[].CertificateMetadata.ValidFrom", "gap g2: buildAppleOsXCredentialsListEntity never sets CertificateMetadata"},
			{"CredentialsList[].CertificateMetadata.ValidTo", "gap g2: buildAppleOsXCredentialsListEntity never sets CertificateMetadata"},
			{"CredentialsList[].CertificatePreference.Names[]", "gap g2: buildAppleOsXCredentialsListEntity never sets CertificatePreference"},
			{"CredentialsList[].IdentityPreference.Names[]", "gap g2: buildAppleOsXCredentialsListEntity never sets IdentityPreference"},
		},
		"DiskEncryption":      nil,
		"GateKeeper":          nil,
		"Restrictions":        nil,
		"SystemExtensions":    nil,
		"ScepList":            nil,
		"WebClipsList":        nil,
		"VpnList":             nil,
		"EasMicrosoftOutlook": nil,
		"CustomAttributes":    nil,
		"KernelExtension":     nil,
		"PrivacyPreferences": {
			{"PrivacyPreferences.Identities[].AEReceiverIdentifier", "canonical (SDK team, MacOsPrivacyPreferencesV2Model.cs:274-283): UEM folds an identity-level Apple Events receiver into AppleEventsList on write (storing 2 identical entries when both forms are sent) and never repopulates this field on read -- apple_events_list is the only supported way to configure Apple Events"},
			{"PrivacyPreferences.Identities[].AEReceiverIdentifierType", "canonical (SDK team, MacOsPrivacyPreferencesV2Model.cs:274-283): same fold-into-AppleEventsList behavior as AEReceiverIdentifier"},
			{"PrivacyPreferences.Identities[].AEReceiverCodeRequirement", "canonical (SDK team, MacOsPrivacyPreferencesV2Model.cs:274-283): same fold-into-AppleEventsList behavior as AEReceiverIdentifier"},
			{"PrivacyPreferences.Identities[].AppleEvents", "canonical (SDK team, MacOsPrivacyPreferencesV2Model.cs:274-283): the coarse-grained identity-level field is superseded by apple_events_list; not modeled"},
		},
	},
	"android": {
		"AndroidForWorkCustomMessages": {
			{"AndroidForWorkCustomMessages.LongSupportMessage", "gap g1: BuildAndroidCreateEntity only sets LockScreenMessage"},
			{"AndroidForWorkCustomMessages.ShortSupportMessage", "gap g1: BuildAndroidCreateEntity only sets LockScreenMessage"},
		},
		"CustomSettingsList": nil,
	},
	"appleiOS": {
		"Passcode":           nil,
		"CustomSettingsList": nil,
	},
}

// d1PinnedUnmodeledSet converts d1PinnedUnmodeled[platform][section] into
// the map[string]string shape assertUnmodeled expects.
func d1PinnedUnmodeledSet(platform, section string) map[string]string {
	return unmodeledSet(d1PinnedUnmodeled[platform][section]...)
}

func expectedGeneralV2Keys() map[string]bool {
	out := make(map[string]bool, len(modeledGeneralV2Keys))
	for k, v := range modeledGeneralV2Keys {
		out[k] = v
	}
	return out
}

func expectedGeneralV4Keys() map[string]bool {
	out := make(map[string]bool, len(modeledGeneralV4Keys))
	for k, v := range modeledGeneralV4Keys {
		out[k] = v
	}
	return out
}

// --- macOS (AppleOsX) ---

func TestBuilderCoverageGuard_AppleOsX(t *testing.T) {
	model := fullyPopulatedProfileModel()
	entity, err := BuildAppleOsXCreateEntity(model)
	if err != nil {
		t.Fatalf("BuildAppleOsXCreateEntity: %v", err)
	}

	t.Run("General", func(t *testing.T) {
		assertSetEqual(t, "AppleOsX General", generalTopLevelKeys(t, entity.General), expectedGeneralV2Keys())
		assertUnmodeled(t, "AppleOsX General smart groups", generalV2SmartGroupUnmodeled(t, entity.General), expectedSmartGroupNameUnmodeled)
	})

	t.Run("Passcode", func(t *testing.T) {
		assertUnmodeled(t, "AppleOsX Passcode", sectionUnmodeled(t, entity, "Passcode"), d1PinnedUnmodeledSet("appleOsX", "Passcode"))
	})

	t.Run("CustomSettingsList", func(t *testing.T) {
		assertUnmodeled(t, "AppleOsX CustomSettingsList", sectionUnmodeled(t, entity, "CustomSettingsList"), d1PinnedUnmodeledSet("appleOsX", "CustomSettingsList"))
	})

	t.Run("NetworkList", func(t *testing.T) {
		assertUnmodeled(t, "AppleOsX NetworkList", sectionUnmodeled(t, entity, "NetworkList"), d1PinnedUnmodeledSet("appleOsX", "NetworkList"))
	})

	t.Run("CredentialsList", func(t *testing.T) {
		assertUnmodeled(t, "AppleOsX CredentialsList", sectionUnmodeled(t, entity, "CredentialsList"), d1PinnedUnmodeledSet("appleOsX", "CredentialsList"))
	})

	t.Run("DiskEncryption", func(t *testing.T) {
		assertUnmodeled(t, "AppleOsX DiskEncryption", sectionUnmodeled(t, entity, "DiskEncryption"), d1PinnedUnmodeledSet("appleOsX", "DiskEncryption"))
	})

	t.Run("GateKeeper", func(t *testing.T) {
		assertUnmodeled(t, "AppleOsX GateKeeper", sectionUnmodeled(t, entity, "GateKeeper"), d1PinnedUnmodeledSet("appleOsX", "GateKeeper"))
	})

	t.Run("Restrictions", func(t *testing.T) {
		assertUnmodeled(t, "AppleOsX Restrictions", sectionUnmodeled(t, entity, "Restrictions"), d1PinnedUnmodeledSet("appleOsX", "Restrictions"))
	})

	t.Run("SystemExtensions", func(t *testing.T) {
		assertUnmodeled(t, "AppleOsX SystemExtensions", sectionUnmodeled(t, entity, "SystemExtensions"), d1PinnedUnmodeledSet("appleOsX", "SystemExtensions"))
	})

	t.Run("ScepList", func(t *testing.T) {
		assertUnmodeled(t, "AppleOsX ScepList", sectionUnmodeled(t, entity, "ScepList"), d1PinnedUnmodeledSet("appleOsX", "ScepList"))
	})

	t.Run("WebClipsList", func(t *testing.T) {
		assertUnmodeled(t, "AppleOsX WebClipsList", sectionUnmodeled(t, entity, "WebClipsList"), d1PinnedUnmodeledSet("appleOsX", "WebClipsList"))
	})

	t.Run("VpnList", func(t *testing.T) {
		assertUnmodeled(t, "AppleOsX VpnList", sectionUnmodeled(t, entity, "VpnList"), d1PinnedUnmodeledSet("appleOsX", "VpnList"))
	})

	t.Run("EasMicrosoftOutlook", func(t *testing.T) {
		assertUnmodeled(t, "AppleOsX EasMicrosoftOutlook", sectionUnmodeled(t, entity, "EasMicrosoftOutlook"), d1PinnedUnmodeledSet("appleOsX", "EasMicrosoftOutlook"))
	})

	t.Run("KernelExtension", func(t *testing.T) {
		assertUnmodeled(t, "AppleOsX KernelExtension", sectionUnmodeled(t, entity, "KernelExtension"), d1PinnedUnmodeledSet("appleOsX", "KernelExtension"))
	})

	t.Run("CustomAttributes", func(t *testing.T) {
		assertUnmodeled(t, "AppleOsX CustomAttributes", sectionUnmodeled(t, entity, "CustomAttributes"), d1PinnedUnmodeledSet("appleOsX", "CustomAttributes"))
	})

	t.Run("PrivacyPreferences", func(t *testing.T) {
		assertUnmodeled(t, "AppleOsX PrivacyPreferences", sectionUnmodeled(t, entity, "PrivacyPreferences"), d1PinnedUnmodeledSet("appleOsX", "PrivacyPreferences"))
	})

	checkModeledSectionSet(t, "AppleOsX", appleOsXModeledSections, setFromSlice(
		"General", "Passcode", "CustomSettingsList", "NetworkList", "CredentialsList", "DiskEncryption", "GateKeeper", "Restrictions", "SystemExtensions", "PrivacyPreferences", "ScepList", "WebClipsList", "VpnList", "EasMicrosoftOutlook", "KernelExtension", "CustomAttributes",
	))
}

// --- Android ---

func TestBuilderCoverageGuard_Android(t *testing.T) {
	model := fullyPopulatedProfileModel()
	entity := BuildAndroidCreateEntity(model)

	t.Run("General", func(t *testing.T) {
		assertSetEqual(t, "Android General", generalTopLevelKeys(t, entity.General), expectedGeneralV2Keys())
		assertUnmodeled(t, "Android General smart groups", generalV2SmartGroupUnmodeled(t, entity.General), expectedSmartGroupNameUnmodeled)
	})

	t.Run("AndroidForWorkCustomMessages", func(t *testing.T) {
		assertUnmodeled(t, "Android AndroidForWorkCustomMessages", sectionUnmodeled(t, entity, "AndroidForWorkCustomMessages"), d1PinnedUnmodeledSet("android", "AndroidForWorkCustomMessages"))
	})

	t.Run("CustomSettingsList", func(t *testing.T) {
		assertUnmodeled(t, "Android CustomSettingsList", sectionUnmodeled(t, entity, "CustomSettingsList"), d1PinnedUnmodeledSet("android", "CustomSettingsList"))
	})

	checkModeledSectionSet(t, "Android", androidModeledSections, setFromSlice(
		"General", "AndroidForWorkCustomMessages", "CustomSettingsList",
	))
}

// --- Apple iOS ---

func TestBuilderCoverageGuard_AppleiOS(t *testing.T) {
	model := fullyPopulatedProfileModel()
	entity := BuildAppleiOSCreateEntity(model)

	t.Run("General", func(t *testing.T) {
		assertSetEqual(t, "AppleiOS General", generalTopLevelKeys(t, entity.General), expectedGeneralV2Keys())
		assertUnmodeled(t, "AppleiOS General smart groups", generalV2SmartGroupUnmodeled(t, entity.General), expectedSmartGroupNameUnmodeled)
	})

	t.Run("Passcode", func(t *testing.T) {
		assertUnmodeled(t, "AppleiOS Passcode", sectionUnmodeled(t, entity, "Passcode"), d1PinnedUnmodeledSet("appleiOS", "Passcode"))
	})

	t.Run("CustomSettingsList", func(t *testing.T) {
		assertUnmodeled(t, "AppleiOS CustomSettingsList", sectionUnmodeled(t, entity, "CustomSettingsList"), d1PinnedUnmodeledSet("appleiOS", "CustomSettingsList"))
	})

	checkModeledSectionSet(t, "AppleiOS", appleiOSModeledSections, setFromSlice(
		"General", "Passcode", "CustomSettingsList",
	))
}

// --- Windows 10 ---

func TestBuilderCoverageGuard_Windows10(t *testing.T) {
	model := fullyPopulatedProfileModel()
	entity := BuildWindows10CreateEntity(model)

	t.Run("General", func(t *testing.T) {
		assertSetEqual(t, "Windows10 General", generalTopLevelKeys(t, entity.General), expectedGeneralV2Keys())
		assertUnmodeled(t, "Windows10 General smart groups", generalV2SmartGroupUnmodeled(t, entity.General), expectedSmartGroupNameUnmodeled)
	})

	checkModeledSectionSet(t, "Windows10", windows10ModeledSections, setFromSlice("General"))
}

// --- Windows Rugged (QNX) ---

func TestBuilderCoverageGuard_WindowsRugged(t *testing.T) {
	model := fullyPopulatedProfileModel()
	entity := BuildWindowsRuggedCreateEntity(model)

	t.Run("General", func(t *testing.T) {
		assertSetEqual(t, "WindowsRugged General", generalTopLevelKeys(t, entity.General), expectedGeneralV2Keys())
		assertUnmodeled(t, "WindowsRugged General smart groups", generalV2SmartGroupUnmodeled(t, entity.General), expectedSmartGroupNameUnmodeled)
	})

	checkModeledSectionSet(t, "WindowsRugged", windowsRuggedModeledSections, setFromSlice("General"))
}

// --- Linux (V4) ---

func TestBuilderCoverageGuard_Linux(t *testing.T) {
	model := fullyPopulatedProfileModel()
	entity, err := BuildLinuxCreateEntity(model)
	if err != nil {
		t.Fatalf("BuildLinuxCreateEntity: %v", err)
	}

	t.Run("general", func(t *testing.T) {
		// V4's General has no smart groups modeled at all (not in
		// modeledGeneralV4Keys, and BuildGeneralV4Create never sets them),
		// so there's no sub-leaf case here — the top-level key-set check
		// alone fully covers V4 General's modeled surface.
		assertSetEqual(t, "Linux general", generalTopLevelKeys(t, entity.General), expectedGeneralV4Keys())
	})

	checkModeledSectionSet(t, "Linux", linuxModeledSections, setFromSlice("general"))
}
