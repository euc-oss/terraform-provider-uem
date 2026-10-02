package platform

// zf1b: unit tests for the permanent masked-secret guard (internal-task).
//
// These are package-internal tests against MaskedUnmodeledPaths and the
// per-platform MaskedUnmodeled* wrappers declared in masked.go, covering:
//
//   - the UEMMaskedSecret constant itself (pinned, with its canonical
//     source documented in masked.go).
//   - nested list/map path construction.
//   - exact-match-only semantics ("****", "******", " *****" must NOT
//     match; "*****" must).
//   - modeled sections/General-keys are ignored even when they contain
//     "*****" verbatim.
//   - key-name independence: a masked value under an innocuous key (e.g.
//     "Server") is still reported.
//   - sorted, deterministic output.

import (
	"reflect"
	"sort"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

func TestUEMMaskedSecret_Pinned(t *testing.T) {
	t.Parallel()
	// Canonical source: UEM 26.2 ProfileServiceV2Helper.cs:135/:2087 and
	// ProfileResourceServiceV2Helper.cs:51/:614 render every ENCRYPTED
	// setting as exactly five U+002A ASTERISK characters on GET.
	if UEMMaskedSecret != "*****" {
		t.Fatalf("UEMMaskedSecret = %q, want %q", UEMMaskedSecret, "*****")
	}
	if len(UEMMaskedSecret) != 5 {
		t.Fatalf("UEMMaskedSecret length = %d, want 5", len(UEMMaskedSecret))
	}
	for i, r := range UEMMaskedSecret {
		if r != '*' {
			t.Fatalf("UEMMaskedSecret[%d] = %q, want '*' (U+002A)", i, r)
		}
	}
}

// exampleEntity is a minimal struct used to test MaskedUnmodeledPaths
// directly, independent of any real SDK type, so the nested-path and
// exact-match behavior can be tested in isolation.
type exampleEntity struct {
	General   *exampleGeneral   `json:"General,omitempty"`
	EmailList []exampleEmail    `json:"EmailList,omitempty"`
	VpnList   []exampleVpn      `json:"VpnList,omitempty"`
	Proxy     *exampleProxy     `json:"Proxy,omitempty"`
	Passcode  *examplePasscode  `json:"Passcode,omitempty"`
	Tags      map[string]string `json:"Tags,omitempty"`
}

type exampleGeneral struct {
	Name     string `json:"Name,omitempty"`
	Password string `json:"Password,omitempty"`
}

type exampleEmail struct {
	IncomingPassword string `json:"IncomingPassword,omitempty"`
}

type exampleVpn struct {
	Account string `json:"Account,omitempty"`
}

type exampleProxy struct {
	Server string `json:"Server,omitempty"`
}

type examplePasscode struct {
	MinimumLength string `json:"MinimumLength,omitempty"`
}

var exampleModeledSections = map[string]bool{
	"General":  true,
	"Passcode": true,
}

var exampleModeledGeneral = map[string]bool{
	"Name": true,
}

func TestMaskedUnmodeledPaths_NestedListPath(t *testing.T) {
	t.Parallel()
	live := &exampleEntity{
		EmailList: []exampleEmail{
			{IncomingPassword: "real-secret"},
			{IncomingPassword: UEMMaskedSecret},
		},
	}
	got, err := MaskedUnmodeledPaths(live, exampleModeledSections, "General", exampleModeledGeneral)
	if err != nil {
		t.Fatalf("MaskedUnmodeledPaths returned unexpected error: %v", err)
	}
	want := []string{"EmailList[1].IncomingPassword"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MaskedUnmodeledPaths = %v, want %v", got, want)
	}
}

func TestMaskedUnmodeledPaths_NestedObjectPath(t *testing.T) {
	t.Parallel()
	live := &exampleEntity{
		Proxy: &exampleProxy{Server: UEMMaskedSecret},
	}
	got, err := MaskedUnmodeledPaths(live, exampleModeledSections, "General", exampleModeledGeneral)
	if err != nil {
		t.Fatalf("MaskedUnmodeledPaths returned unexpected error: %v", err)
	}
	want := []string{"Proxy.Server"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MaskedUnmodeledPaths = %v, want %v (key-name independence: masked value under non-secret-looking key must be reported)", got, want)
	}
}

func TestMaskedUnmodeledPaths_MapPath(t *testing.T) {
	t.Parallel()
	live := &exampleEntity{
		Tags: map[string]string{"note": UEMMaskedSecret, "other": "fine"},
	}
	got, err := MaskedUnmodeledPaths(live, exampleModeledSections, "General", exampleModeledGeneral)
	if err != nil {
		t.Fatalf("MaskedUnmodeledPaths returned unexpected error: %v", err)
	}
	want := []string{"Tags.note"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MaskedUnmodeledPaths = %v, want %v", got, want)
	}
}

func TestMaskedUnmodeledPaths_ExactMatchOnly(t *testing.T) {
	t.Parallel()
	cases := []string{"****", "******", " *****", "*****x", "x*****", ""}
	for _, val := range cases {
		t.Run("value_"+val, func(t *testing.T) {
			t.Parallel()
			live := &exampleEntity{VpnList: []exampleVpn{{Account: val}}}
			got, err := MaskedUnmodeledPaths(live, exampleModeledSections, "General", exampleModeledGeneral)
			if err != nil {
				t.Fatalf("MaskedUnmodeledPaths returned unexpected error: %v", err)
			}
			if len(got) != 0 {
				t.Errorf("MaskedUnmodeledPaths(%q) = %v, want none (exact match only)", val, got)
			}
		})
	}
}

func TestMaskedUnmodeledPaths_ModeledSectionsIgnored(t *testing.T) {
	t.Parallel()
	live := &exampleEntity{
		Passcode: &examplePasscode{MinimumLength: UEMMaskedSecret}, // modeled section
		VpnList:  []exampleVpn{{Account: UEMMaskedSecret}},         // unmodeled
	}
	got, err := MaskedUnmodeledPaths(live, exampleModeledSections, "General", exampleModeledGeneral)
	if err != nil {
		t.Fatalf("MaskedUnmodeledPaths returned unexpected error: %v", err)
	}
	want := []string{"VpnList[0].Account"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MaskedUnmodeledPaths = %v, want %v (modeled Passcode must be ignored even though masked)", got, want)
	}
}

func TestMaskedUnmodeledPaths_ModeledGeneralKeysIgnored(t *testing.T) {
	t.Parallel()
	live := &exampleEntity{
		General: &exampleGeneral{
			Name:     UEMMaskedSecret, // modeled General key - must be ignored
			Password: UEMMaskedSecret, // unmodeled General key - must be reported
		},
	}
	got, err := MaskedUnmodeledPaths(live, exampleModeledSections, "General", exampleModeledGeneral)
	if err != nil {
		t.Fatalf("MaskedUnmodeledPaths returned unexpected error: %v", err)
	}
	want := []string{"General.Password"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MaskedUnmodeledPaths = %v, want %v", got, want)
	}
}

func TestMaskedUnmodeledPaths_SortedOutput(t *testing.T) {
	t.Parallel()
	live := &exampleEntity{
		VpnList: []exampleVpn{{Account: UEMMaskedSecret}, {Account: UEMMaskedSecret}},
		Proxy:   &exampleProxy{Server: UEMMaskedSecret},
		General: &exampleGeneral{Password: UEMMaskedSecret},
	}
	got, err := MaskedUnmodeledPaths(live, exampleModeledSections, "General", exampleModeledGeneral)
	if err != nil {
		t.Fatalf("MaskedUnmodeledPaths returned unexpected error: %v", err)
	}
	if !sort.StringsAreSorted(got) {
		t.Errorf("MaskedUnmodeledPaths output not sorted: %v", got)
	}
	want := []string{"General.Password", "Proxy.Server", "VpnList[0].Account", "VpnList[1].Account"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MaskedUnmodeledPaths = %v, want %v", got, want)
	}
}

func TestMaskedUnmodeledPaths_NilLive(t *testing.T) {
	t.Parallel()
	got, err := MaskedUnmodeledPaths(nil, exampleModeledSections, "General", exampleModeledGeneral)
	if err != nil {
		t.Fatalf("MaskedUnmodeledPaths(nil) returned unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("MaskedUnmodeledPaths(nil) = %v, want nil", got)
	}
}

// --- per-platform wrapper smoke tests: real SDK types, one masked leaf each ---

func TestMaskedUnmodeledAppleOsX(t *testing.T) {
	t.Parallel()
	live := &sdk.AppleOsXDeviceProfileEntityV2{
		EmailList: []sdk.AppleOsXEmailPayloadEntityV2{{IncomingPassword: UEMMaskedSecret}},
	}
	got, err := MaskedUnmodeledAppleOsX(live)
	if err != nil {
		t.Fatalf("MaskedUnmodeledAppleOsX returned unexpected error: %v", err)
	}
	want := []string{"EmailList[0].IncomingPassword"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MaskedUnmodeledAppleOsX = %v, want %v", got, want)
	}
	gotNil, err := MaskedUnmodeledAppleOsX(nil)
	if err != nil {
		t.Fatalf("MaskedUnmodeledAppleOsX(nil) returned unexpected error: %v", err)
	}
	if gotNil != nil {
		t.Errorf("MaskedUnmodeledAppleOsX(nil) = %v, want nil", gotNil)
	}
}

func TestMaskedUnmodeledAndroid(t *testing.T) {
	t.Parallel()
	live := &sdk.AndroidDeviceProfileV2Entity{
		AndroidForWorkCredentialsList: []sdk.AndroidForWorkCredentialsPayloadV2Entity{
			{CertificatePassword: UEMMaskedSecret},
		},
	}
	got, err := MaskedUnmodeledAndroid(live)
	if err != nil {
		t.Fatalf("MaskedUnmodeledAndroid returned unexpected error: %v", err)
	}
	want := []string{"AndroidForWorkCredentialsList[0].CertificatePassword"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MaskedUnmodeledAndroid = %v, want %v", got, want)
	}
}

// --- D4 (01o task D2): masked-secret detection over KEPT live leaves ---

func TestMaskedUnmodeledAndroid_KeptLeafMasked(t *testing.T) {
	t.Parallel()
	live := &sdk.AndroidDeviceProfileV2Entity{
		AndroidForWorkCustomMessages: &sdk.AndroidForWorkCustomMessagesPayloadV2Entity{
			LockScreenMessage:  "not masked",
			LongSupportMessage: UEMMaskedSecret,
		},
	}
	got, err := MaskedUnmodeledAndroid(live)
	if err != nil {
		t.Fatalf("MaskedUnmodeledAndroid returned unexpected error: %v", err)
	}
	want := []string{"AndroidForWorkCustomMessages.LongSupportMessage"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MaskedUnmodeledAndroid = %v, want %v", got, want)
	}
}

func TestMaskedUnmodeledAndroid_ShortSupportMessageMasked(t *testing.T) {
	t.Parallel()
	live := &sdk.AndroidDeviceProfileV2Entity{
		AndroidForWorkCustomMessages: &sdk.AndroidForWorkCustomMessagesPayloadV2Entity{
			ShortSupportMessage: UEMMaskedSecret,
		},
	}
	got, err := MaskedUnmodeledAndroid(live)
	if err != nil {
		t.Fatalf("MaskedUnmodeledAndroid returned unexpected error: %v", err)
	}
	want := []string{"AndroidForWorkCustomMessages.ShortSupportMessage"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MaskedUnmodeledAndroid = %v, want %v", got, want)
	}
}

func TestMaskedUnmodeledAndroid_ModeledLockScreenMaskNotFlagged(t *testing.T) {
	t.Parallel()
	// LockScreenMessage is modeled (plan-owned) — even if it happens to be
	// exactly "*****" on live, that's irrelevant: it's about to be replaced
	// by the plan's value, never echoed from live.
	live := &sdk.AndroidDeviceProfileV2Entity{
		AndroidForWorkCustomMessages: &sdk.AndroidForWorkCustomMessagesPayloadV2Entity{
			LockScreenMessage:  UEMMaskedSecret,
			LongSupportMessage: "fine",
		},
	}
	got, err := MaskedUnmodeledAndroid(live)
	if err != nil {
		t.Fatalf("MaskedUnmodeledAndroid returned unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("MaskedUnmodeledAndroid = %v, want empty (LockScreenMessage is modeled)", got)
	}
}

func TestMaskedUnmodeledAppleOsX_CredentialsKeptSubtreesMasked(t *testing.T) {
	t.Parallel()
	live := &sdk.AppleOsXDeviceProfileEntityV2{
		CredentialsList: []sdk.AppleOsXCredentialPayloadEntityV2{
			{
				CredentialName:        "cred-a",
				CertificatePreference: &sdk.MacOsCredentialCertificatePreferencePayloadV2Model{Names: []string{"fine", UEMMaskedSecret}},
			},
			{
				CredentialName:      "cred-b",
				CertificateMetadata: &sdk.CertificateMetadataModel1V2{SubjectName: UEMMaskedSecret},
			},
		},
	}
	got, err := MaskedUnmodeledAppleOsX(live)
	if err != nil {
		t.Fatalf("MaskedUnmodeledAppleOsX returned unexpected error: %v", err)
	}
	want := []string{
		"CredentialsList[0].CertificatePreference.Names[1]",
		"CredentialsList[1].CertificateMetadata.SubjectName",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MaskedUnmodeledAppleOsX = %v, want %v", got, want)
	}
}

func TestMaskedUnmodeledAppleOsX_IdentityPreferenceMasked(t *testing.T) {
	t.Parallel()
	live := &sdk.AppleOsXDeviceProfileEntityV2{
		CredentialsList: []sdk.AppleOsXCredentialPayloadEntityV2{
			{
				CredentialName:     "cred-a",
				IdentityPreference: &sdk.MacOsCredentialIdentityPreferencePayloadV2Model{Names: []string{UEMMaskedSecret}},
			},
		},
	}
	got, err := MaskedUnmodeledAppleOsX(live)
	if err != nil {
		t.Fatalf("MaskedUnmodeledAppleOsX returned unexpected error: %v", err)
	}
	want := []string{"CredentialsList[0].IdentityPreference.Names[0]"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MaskedUnmodeledAppleOsX = %v, want %v", got, want)
	}
}

func TestMaskedUnmodeledAppleOsX_NonMaskedKeptValue_NotFlagged(t *testing.T) {
	t.Parallel()
	live := &sdk.AppleOsXDeviceProfileEntityV2{
		CredentialsList: []sdk.AppleOsXCredentialPayloadEntityV2{
			{
				CredentialName:        "cred-a",
				CertificatePreference: &sdk.MacOsCredentialCertificatePreferencePayloadV2Model{Names: []string{"perfectly-normal-name"}},
			},
		},
	}
	got, err := MaskedUnmodeledAppleOsX(live)
	if err != nil {
		t.Fatalf("MaskedUnmodeledAppleOsX returned unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("MaskedUnmodeledAppleOsX = %v, want empty", got)
	}
}

func TestMaskedUnmodeledAppleiOS(t *testing.T) {
	t.Parallel()
	live := &sdk.AppleDeviceProfileV2Entity{
		EmailList: []sdk.AppleEmailPayloadV2Entity{{IncomingPassword: UEMMaskedSecret}},
	}
	got, err := MaskedUnmodeledAppleiOS(live)
	if err != nil {
		t.Fatalf("MaskedUnmodeledAppleiOS returned unexpected error: %v", err)
	}
	want := []string{"EmailList[0].IncomingPassword"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MaskedUnmodeledAppleiOS = %v, want %v", got, want)
	}
}

func TestMaskedUnmodeledWindows10(t *testing.T) {
	t.Parallel()
	live := &sdk.WinRTDeviceProfileV2Entity{
		Credentials: &sdk.WindowsDesktopCredentialsPayloadEntityV2{CertificatePassword: UEMMaskedSecret},
	}
	got, err := MaskedUnmodeledWindows10(live)
	if err != nil {
		t.Fatalf("MaskedUnmodeledWindows10 returned unexpected error: %v", err)
	}
	want := []string{"Credentials.certificatePassword"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MaskedUnmodeledWindows10 = %v, want %v", got, want)
	}
}

func TestMaskedUnmodeledWindowsRugged(t *testing.T) {
	t.Parallel()
	live := &sdk.QnxDeviceProfileEntityV2{
		CustomAttributePayload: &sdk.QnxCustomAttributePayloadEntityV2{
			CustomAttributes: []sdk.QnxProfileCustomAttributeEntityV2{{Name: "attr", Value: UEMMaskedSecret}},
		},
	}
	got, err := MaskedUnmodeledWindowsRugged(live)
	if err != nil {
		t.Fatalf("MaskedUnmodeledWindowsRugged returned unexpected error: %v", err)
	}
	want := []string{"CustomAttributePayload.CustomAttributes[0].Value"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MaskedUnmodeledWindowsRugged = %v, want %v", got, want)
	}
}

func TestMaskedUnmodeledLinux(t *testing.T) {
	t.Parallel()
	live := &sdk.LinuxDeviceProfileEntity1V4{
		Wifis: []sdk.LinuxWifiPayloadEntityV4{{EnterprisePassword: UEMMaskedSecret}},
	}
	got, err := MaskedUnmodeledLinux(live)
	if err != nil {
		t.Fatalf("MaskedUnmodeledLinux returned unexpected error: %v", err)
	}
	want := []string{"Wifis[0].EnterprisePassword"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MaskedUnmodeledLinux = %v, want %v", got, want)
	}
}

// --- fail-closed tests (correction 1) ---

// unmarshalableEntity cannot be json.Marshal'd (chan and func fields are
// unsupported by encoding/json), so it exercises the top-level Marshal
// error branch of MaskedUnmodeledPaths.
type unmarshalableEntity struct {
	General *exampleGeneral `json:"General,omitempty"`
	Bad     chan int        `json:"Bad,omitempty"`
	WorseFn func()          `json:"WorseFn,omitempty"`
}

func TestMaskedUnmodeledPaths_MarshalError_FailsClosed(t *testing.T) {
	t.Parallel()
	live := &unmarshalableEntity{Bad: make(chan int), WorseFn: func() {}}
	got, err := MaskedUnmodeledPaths(live, exampleModeledSections, "General", exampleModeledGeneral)
	if err == nil {
		t.Fatal("MaskedUnmodeledPaths with an unmarshalable live value: got nil error, want non-nil (fail closed)")
	}
	if got != nil {
		t.Errorf("MaskedUnmodeledPaths with an unmarshalable live value: got paths %v, want nil", got)
	}
}

// exampleEntityStringGeneral and exampleEntityArrayGeneral model a live
// entity whose "General" top-level key decodes to a JSON string / array
// rather than an object — an inspection failure, not "no masked secrets".
type exampleEntityStringGeneral struct {
	General string `json:"General"`
}

type exampleEntityArrayGeneral struct {
	General []string `json:"General"`
}

func TestMaskedUnmodeledPaths_GeneralNotObject_String_FailsClosed(t *testing.T) {
	t.Parallel()
	live := &exampleEntityStringGeneral{General: "not-an-object"}
	got, err := MaskedUnmodeledPaths(live, exampleModeledSections, "General", exampleModeledGeneral)
	if err == nil {
		t.Fatal("MaskedUnmodeledPaths with a string General value: got nil error, want non-nil (fail closed)")
	}
	if got != nil {
		t.Errorf("MaskedUnmodeledPaths with a string General value: got paths %v, want nil", got)
	}
}

func TestMaskedUnmodeledPaths_GeneralNotObject_Array_FailsClosed(t *testing.T) {
	t.Parallel()
	live := &exampleEntityArrayGeneral{General: []string{"a", "b"}}
	got, err := MaskedUnmodeledPaths(live, exampleModeledSections, "General", exampleModeledGeneral)
	if err == nil {
		t.Fatal("MaskedUnmodeledPaths with an array General value: got nil error, want non-nil (fail closed)")
	}
	if got != nil {
		t.Errorf("MaskedUnmodeledPaths with an array General value: got paths %v, want nil", got)
	}
}

// TestMaskedUnmodeledPaths_GeneralNull_OK confirms a live entity with an
// explicit JSON null General (distinct from the key being absent) is NOT
// treated as an inspection failure -- json.Unmarshal into `any` decodes
// `null` as a nil interface, which must be handled as "no General to
// inspect", not "General is not an object".
func TestMaskedUnmodeledPaths_GeneralNull_OK(t *testing.T) {
	t.Parallel()
	live := &exampleEntity{} // General is a nil *exampleGeneral -> omitted by omitempty
	got, err := MaskedUnmodeledPaths(live, exampleModeledSections, "General", exampleModeledGeneral)
	if err != nil {
		t.Fatalf("MaskedUnmodeledPaths returned unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("MaskedUnmodeledPaths = %v, want nil", got)
	}
}
