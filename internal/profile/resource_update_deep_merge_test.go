package profile

// internal-task, task D2: full read-modify-write (Update) coverage for the two
// promoted field/entry-level merges — g1 (Android
// AndroidForWorkCustomMessages.LongSupportMessage/ShortSupportMessage) and
// g2 (macOS CredentialsList[].CertificatePreference/IdentityPreference/
// CertificateMetadata) — plus the D4 masked-secret refusal extension over
// those same kept leaves. Uses the fakeProfileService/runRMWUpdate/
// live*Entity helpers from resource_update_rmw_test.go, so these drive the
// real ProfileResource.Update method end to end (Get -> masked guard ->
// Build*CreateEntity -> Overlay*UpdateEntity -> Update), same as that
// file's tests. See internal/profile/platform/overlay_deep_merge_test.go
// and masked_test.go for the lower-level direct-function equivalents of
// this same matrix.

import (
	"strings"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// --- g1: Android AndroidForWorkCustomMessages, full RMW ---

func liveAndroidEntityWithCustomMessages() *sdk.AndroidDeviceProfileV2Entity {
	e := liveAndroidEntity()
	e.AndroidForWorkCustomMessages = &sdk.AndroidForWorkCustomMessagesPayloadV2Entity{
		LockScreenMessage:   "old lock message",
		LongSupportMessage:  "live long message",
		ShortSupportMessage: "live short message",
	}
	return e
}

func TestProfileResourceUpdate_Android_G1_LockScreenChange_KeepsLiveLongShort(t *testing.T) {
	t.Parallel()
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{Android: liveAndroidEntityWithCustomMessages()}}
	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAndroid), map[string]tftypes.Value{
		"id":                  stringVal("999"),
		"lock_screen_message": stringVal("new lock message"),
	})

	resp := runRMWUpdate(t, fake, planValues)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics.Errors())
	}
	sent, ok := fake.updateEntity.(*sdk.AndroidDeviceProfileV2Entity)
	if !ok {
		t.Fatalf("expected *sdk.AndroidDeviceProfileV2Entity, got %T", fake.updateEntity)
	}
	msgs := sent.AndroidForWorkCustomMessages
	if msgs == nil {
		t.Fatal("expected AndroidForWorkCustomMessages to survive, got nil")
	}
	if msgs.LockScreenMessage != "new lock message" {
		t.Errorf("LockScreenMessage = %q, want the plan's new value", msgs.LockScreenMessage)
	}
	if msgs.LongSupportMessage != "live long message" {
		t.Errorf("LongSupportMessage = %q, want kept from live", msgs.LongSupportMessage)
	}
	if msgs.ShortSupportMessage != "live short message" {
		t.Errorf("ShortSupportMessage = %q, want kept from live", msgs.ShortSupportMessage)
	}
}

func TestProfileResourceUpdate_Android_G1_LockScreenNull_SectionKeptWithLiveLongShortOnly(t *testing.T) {
	t.Parallel()
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{Android: liveAndroidEntityWithCustomMessages()}}
	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAndroid), map[string]tftypes.Value{
		"id":                  stringVal("999"),
		"lock_screen_message": nullString(),
	})

	resp := runRMWUpdate(t, fake, planValues)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics.Errors())
	}
	sent, ok := fake.updateEntity.(*sdk.AndroidDeviceProfileV2Entity)
	if !ok {
		t.Fatalf("expected *sdk.AndroidDeviceProfileV2Entity, got %T", fake.updateEntity)
	}
	msgs := sent.AndroidForWorkCustomMessages
	if msgs == nil {
		t.Fatal("expected AndroidForWorkCustomMessages to survive with kept Long/Short, got nil")
	}
	if msgs.LockScreenMessage != "" {
		t.Errorf("LockScreenMessage = %q, want \"\" (cleared by null plan)", msgs.LockScreenMessage)
	}
	if msgs.LongSupportMessage != "live long message" {
		t.Errorf("LongSupportMessage = %q, want kept from live", msgs.LongSupportMessage)
	}
	if msgs.ShortSupportMessage != "live short message" {
		t.Errorf("ShortSupportMessage = %q, want kept from live", msgs.ShortSupportMessage)
	}
}

func TestProfileResourceUpdate_Android_G1_NothingLiveNothingPlanned_SectionOmitted(t *testing.T) {
	t.Parallel()
	// liveAndroidEntity has no AndroidForWorkCustomMessages at all, and the
	// base plan values leave lock_screen_message null — matches (and
	// supersedes, for this specific field-level assertion)
	// TestProfileResourceUpdate_PreservesExistingPayloads in resource_test.go.
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{Android: liveAndroidEntity()}}
	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAndroid), map[string]tftypes.Value{
		"id": stringVal("999"),
	})

	resp := runRMWUpdate(t, fake, planValues)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics.Errors())
	}
	sent, ok := fake.updateEntity.(*sdk.AndroidDeviceProfileV2Entity)
	if !ok {
		t.Fatalf("expected *sdk.AndroidDeviceProfileV2Entity, got %T", fake.updateEntity)
	}
	if sent.AndroidForWorkCustomMessages != nil {
		t.Errorf("expected AndroidForWorkCustomMessages omitted, got %+v", sent.AndroidForWorkCustomMessages)
	}
}

// --- D4: Android masked kept-leaf refusal, full RMW ---

func TestProfileResourceUpdate_Android_D4_MaskedLongSupportMessage_Refused(t *testing.T) {
	t.Parallel()
	live := liveAndroidEntity()
	live.AndroidForWorkCustomMessages = &sdk.AndroidForWorkCustomMessagesPayloadV2Entity{
		LongSupportMessage: "*****",
	}
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{Android: live}}
	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAndroid), map[string]tftypes.Value{"id": stringVal("999")})

	resp := runRMWUpdate(t, fake, planValues)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected refusal diagnostic for a masked kept leaf")
	}
	if fake.updateCalls != 0 {
		t.Fatalf("expected Update not to be called, got %d calls", fake.updateCalls)
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Detail(), "AndroidForWorkCustomMessages.LongSupportMessage") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected diagnostic to name the masked path, got: %v", resp.Diagnostics.Errors())
	}
}

// --- g2: macOS CredentialsList, full RMW ---
//
// Live fixtures below use REAL UEM shapes: an Upload credential's
// CredentialName/Name are UEM-rewritten to "Certificate #N"
// (internal/profile/state/state_helpers.go ~L355-358), and a DefinedCA
// credential's CredentialName comes back "" with Name "Certificate #N"
// (internal/profile/state/read_mappers.go ~L309-320). The planned side, by
// contrast, always carries the user's own label — so pairing must go
// through internal/profile/platform's tiered CertificateID/CA+Template/name
// key, not a name-only match.

// networkReferencingCredential returns a network_list plan value with one
// entry whose identity_certificate references credentialName, so
// PrepareCredentialsForPayload's AnyNetworkReferencesCredential check
// doesn't drop the whole credentials_list before the overlay ever runs.
// Note: credentialName must be the PLANNED (user-label) CredentialName, not
// any live-side name.
func networkReferencingCredential(credentialName string) tftypes.Value {
	return networkListVal([]map[string]tftypes.Value{
		{
			"network_interface":      stringVal("BuiltInWireless"),
			"identity_certificate":   stringVal(credentialName),
			"service_set_identifier": stringVal("corp-ssid"),
		},
	})
}

// liveAppleOsXEntityWithCredential returns a live entity with one
// live-shaped Upload credential (CertificateID 501, CredentialName/Name
// both UEM-rewritten to "Certificate #2") carrying the given
// CertificatePreference/IdentityPreference names plus CertificateMetadata.
func liveAppleOsXEntityWithCredential(certPrefName, identPrefName string) *sdk.AppleOsXDeviceProfileEntityV2 {
	e := liveAppleOsXEntity()
	e.CredentialsList = []sdk.AppleOsXCredentialPayloadEntityV2{
		{
			CredentialSource:      "Upload",
			CredentialName:        "Certificate #2",
			Name:                  "Certificate #2",
			CertificateID:         intPtr(501),
			CertificatePreference: &sdk.MacOsCredentialCertificatePreferencePayloadV2Model{Names: []string{certPrefName}},
			IdentityPreference:    &sdk.MacOsCredentialIdentityPreferencePayloadV2Model{Names: []string{identPrefName}},
			CertificateMetadata:   &sdk.CertificateMetadataModel1V2{SubjectName: "live-subject"},
		},
	}
	return e
}

// (a) Tier 1: Upload credential matched by CertificateID — all three carried.
func TestProfileResourceUpdate_AppleOsX_G2_UploadSameCertID_AllCarried(t *testing.T) {
	t.Parallel()
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{AppleOsX: liveAppleOsXEntityWithCredential("live-cert-pref", "live-ident-pref")}}
	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{
		"id":           stringVal("999"),
		"network_list": networkReferencingCredential("my-wifi-cert"),
		"credentials_list": credentialsListVal([]map[string]tftypes.Value{
			{
				"credential_source": stringVal("Upload"),
				"credential_name":   stringVal("my-wifi-cert"),
				"certificate_id":    int64Val(501),
			},
		}),
	})

	resp := runRMWUpdate(t, fake, planValues)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics.Errors())
	}
	sent, ok := fake.updateEntity.(*sdk.AppleOsXDeviceProfileEntityV2)
	if !ok {
		t.Fatalf("expected *sdk.AppleOsXDeviceProfileEntityV2, got %T", fake.updateEntity)
	}
	if len(sent.CredentialsList) != 1 {
		t.Fatalf("expected 1 credential, got %d", len(sent.CredentialsList))
	}
	got := sent.CredentialsList[0]
	if got.CredentialName != "my-wifi-cert" {
		t.Errorf("expected modeled CredentialName (user label) to reflect plan, got %q", got.CredentialName)
	}
	if got.CertificatePreference == nil || got.CertificatePreference.Names[0] != "live-cert-pref" {
		t.Errorf("expected CertificatePreference carried from live, got %+v", got.CertificatePreference)
	}
	if got.IdentityPreference == nil || got.IdentityPreference.Names[0] != "live-ident-pref" {
		t.Errorf("expected IdentityPreference carried from live, got %+v", got.IdentityPreference)
	}
	if got.CertificateMetadata == nil || got.CertificateMetadata.SubjectName != "live-subject" {
		t.Errorf("expected CertificateMetadata carried from live (same CertificateID), got %+v", got.CertificateMetadata)
	}
}

// (b) Upload with a brand-new certificate: different CertificateID and no
// other tier applies — nothing carried (Tier 1's key is present, so there's
// no fallthrough to weaker tiers even though nothing else changed).
func TestProfileResourceUpdate_AppleOsX_G2_UploadNewCertificate_NoneCarried(t *testing.T) {
	t.Parallel()
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{AppleOsX: liveAppleOsXEntityWithCredential("live-cert-pref", "live-ident-pref")}}
	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{
		"id":           stringVal("999"),
		"network_list": networkReferencingCredential("my-wifi-cert"),
		"credentials_list": credentialsListVal([]map[string]tftypes.Value{
			{
				"credential_source": stringVal("Upload"),
				"credential_name":   stringVal("my-wifi-cert"),
				"certificate_id":    int64Val(777), // re-uploaded -> different ID
			},
		}),
	})

	resp := runRMWUpdate(t, fake, planValues)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics.Errors())
	}
	sent, ok := fake.updateEntity.(*sdk.AppleOsXDeviceProfileEntityV2)
	if !ok {
		t.Fatalf("expected *sdk.AppleOsXDeviceProfileEntityV2, got %T", fake.updateEntity)
	}
	got := sent.CredentialsList[0]
	if got.CertificatePreference != nil || got.IdentityPreference != nil || got.CertificateMetadata != nil {
		t.Errorf("expected nothing carried for a re-uploaded certificate, got %+v", got)
	}
}

// (c) Tier 2: DefinedCA credential matched by CertificateAuthority+
// CertificateTemplate (no CertificateID on the planned side at all) —
// prefs carried, metadata NOT (no CertificateID to compare).
func TestProfileResourceUpdate_AppleOsX_G2_DefinedCA_CATemplateMatch_PrefsCarriedMetadataNot(t *testing.T) {
	t.Parallel()
	live := liveAppleOsXEntity()
	live.CredentialsList = []sdk.AppleOsXCredentialPayloadEntityV2{
		{
			CredentialSource:      "DefinedCertificateAuthority",
			Name:                  "Certificate #1",
			CertificateAuthority:  intPtr(12),
			CertificateTemplate:   intPtr(34),
			CertificatePreference: &sdk.MacOsCredentialCertificatePreferencePayloadV2Model{Names: []string{"live-cert-pref"}},
			IdentityPreference:    &sdk.MacOsCredentialIdentityPreferencePayloadV2Model{Names: []string{"live-ident-pref"}},
			CertificateMetadata:   &sdk.CertificateMetadataModel1V2{SubjectName: "live-subject"},
		},
	}
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{AppleOsX: live}}
	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{
		"id":           stringVal("999"),
		"network_list": networkReferencingCredential("corp-ca"),
		"credentials_list": credentialsListVal([]map[string]tftypes.Value{
			{
				"credential_source":     stringVal("DefinedCertificateAuthority"),
				"credential_name":       stringVal("corp-ca"),
				"certificate_authority": int64Val(12),
				"certificate_template":  int64Val(34),
			},
		}),
	})

	resp := runRMWUpdate(t, fake, planValues)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics.Errors())
	}
	sent, ok := fake.updateEntity.(*sdk.AppleOsXDeviceProfileEntityV2)
	if !ok {
		t.Fatalf("expected *sdk.AppleOsXDeviceProfileEntityV2, got %T", fake.updateEntity)
	}
	got := sent.CredentialsList[0]
	if got.CertificatePreference == nil || got.CertificatePreference.Names[0] != "live-cert-pref" {
		t.Errorf("expected CertificatePreference carried via CA+Template match, got %+v", got.CertificatePreference)
	}
	if got.IdentityPreference == nil || got.IdentityPreference.Names[0] != "live-ident-pref" {
		t.Errorf("expected IdentityPreference carried via CA+Template match, got %+v", got.IdentityPreference)
	}
	if got.CertificateMetadata != nil {
		t.Errorf("expected CertificateMetadata NOT carried (planned CertificateID is nil), got %+v", got.CertificateMetadata)
	}
}

// A brand-new planned entry (no CertificateID/CA-Template/name in common
// with anything live) — nothing carried.
func TestProfileResourceUpdate_AppleOsX_G2_NewEntry_NoneCarried(t *testing.T) {
	t.Parallel()
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{AppleOsX: liveAppleOsXEntityWithCredential("live-cert-pref", "live-ident-pref")}}
	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{
		"id":           stringVal("999"),
		"network_list": networkReferencingCredential("cred-b"),
		"credentials_list": credentialsListVal([]map[string]tftypes.Value{
			{
				"credential_source":     stringVal("DefinedCertificateAuthority"),
				"credential_name":       stringVal("cred-b"),
				"certificate_authority": int64Val(999),
				"certificate_template":  int64Val(888),
			},
		}),
	})

	resp := runRMWUpdate(t, fake, planValues)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics.Errors())
	}
	sent, ok := fake.updateEntity.(*sdk.AppleOsXDeviceProfileEntityV2)
	if !ok {
		t.Fatalf("expected *sdk.AppleOsXDeviceProfileEntityV2, got %T", fake.updateEntity)
	}
	got := sent.CredentialsList[0]
	if got.CertificatePreference != nil || got.IdentityPreference != nil || got.CertificateMetadata != nil {
		t.Errorf("expected nothing carried for a brand-new planned entry, got %+v", got)
	}
}

// (d) Two live DefinedCA entries with the same CA/Template: ambiguous at
// Tier 2 — nothing carried, no fallthrough to a weaker tier.
func TestProfileResourceUpdate_AppleOsX_G2_DefinedCA_AmbiguousCATemplate_NoneCarried(t *testing.T) {
	t.Parallel()
	live := liveAppleOsXEntity()
	live.CredentialsList = []sdk.AppleOsXCredentialPayloadEntityV2{
		{CredentialSource: "DefinedCertificateAuthority", Name: "Certificate #1", CertificateAuthority: intPtr(12), CertificateTemplate: intPtr(34), CertificatePreference: &sdk.MacOsCredentialCertificatePreferencePayloadV2Model{Names: []string{"one"}}},
		{CredentialSource: "DefinedCertificateAuthority", Name: "Certificate #2", CertificateAuthority: intPtr(12), CertificateTemplate: intPtr(34), CertificatePreference: &sdk.MacOsCredentialCertificatePreferencePayloadV2Model{Names: []string{"two"}}},
	}
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{AppleOsX: live}}
	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{
		"id":           stringVal("999"),
		"network_list": networkReferencingCredential("corp-ca"),
		"credentials_list": credentialsListVal([]map[string]tftypes.Value{
			{
				"credential_source":     stringVal("DefinedCertificateAuthority"),
				"credential_name":       stringVal("corp-ca"),
				"certificate_authority": int64Val(12),
				"certificate_template":  int64Val(34),
			},
		}),
	})

	resp := runRMWUpdate(t, fake, planValues)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics.Errors())
	}
	sent, ok := fake.updateEntity.(*sdk.AppleOsXDeviceProfileEntityV2)
	if !ok {
		t.Fatalf("expected *sdk.AppleOsXDeviceProfileEntityV2, got %T", fake.updateEntity)
	}
	got := sent.CredentialsList[0]
	if got.CertificatePreference != nil {
		t.Errorf("expected nothing carried for an ambiguous (duplicate CA+Template) live match, got %+v", got.CertificatePreference)
	}
}

// (e) Source mismatch: the same CertificateID on both sides, but one side
// Upload and the other DefinedCA — nothing carried.
func TestProfileResourceUpdate_AppleOsX_G2_SourceMismatchSameCertID_NoneCarried(t *testing.T) {
	t.Parallel()
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{AppleOsX: liveAppleOsXEntityWithCredential("live-cert-pref", "live-ident-pref")}}
	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{
		"id":           stringVal("999"),
		"network_list": networkReferencingCredential("corp-ca"),
		"credentials_list": credentialsListVal([]map[string]tftypes.Value{
			{
				"credential_source": stringVal("DefinedCertificateAuthority"),
				"credential_name":   stringVal("corp-ca"),
				"certificate_id":    int64Val(501), // same numeric ID as live's Upload credential
			},
		}),
	})

	resp := runRMWUpdate(t, fake, planValues)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics.Errors())
	}
	sent, ok := fake.updateEntity.(*sdk.AppleOsXDeviceProfileEntityV2)
	if !ok {
		t.Fatalf("expected *sdk.AppleOsXDeviceProfileEntityV2, got %T", fake.updateEntity)
	}
	got := sent.CredentialsList[0]
	if got.CertificatePreference != nil || got.IdentityPreference != nil || got.CertificateMetadata != nil {
		t.Errorf("expected nothing carried across a credential-source mismatch, got %+v", got)
	}
}

func TestProfileResourceUpdate_AppleOsX_G2_PlanNullClearsList(t *testing.T) {
	t.Parallel()
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{AppleOsX: liveAppleOsXEntityWithCredential("live-cert-pref", "live-ident-pref")}}
	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{
		"id": stringVal("999"),
		// credentials_list stays null (rmwBasePlanValues default).
	})

	resp := runRMWUpdate(t, fake, planValues)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics.Errors())
	}
	sent, ok := fake.updateEntity.(*sdk.AppleOsXDeviceProfileEntityV2)
	if !ok {
		t.Fatalf("expected *sdk.AppleOsXDeviceProfileEntityV2, got %T", fake.updateEntity)
	}
	if sent.CredentialsList != nil {
		t.Errorf("expected CredentialsList cleared by a modeled-null plan, got %+v", sent.CredentialsList)
	}
}

// --- D4: macOS masked kept-subtree refusal, full RMW ---

func TestProfileResourceUpdate_AppleOsX_D4_MaskedCertificatePreference_Refused(t *testing.T) {
	t.Parallel()
	live := liveAppleOsXEntity()
	live.CredentialsList = []sdk.AppleOsXCredentialPayloadEntityV2{
		{
			CredentialName:        "cred-a",
			CertificatePreference: &sdk.MacOsCredentialCertificatePreferencePayloadV2Model{Names: []string{"*****"}},
		},
	}
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{AppleOsX: live}}
	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{"id": stringVal("999")})

	resp := runRMWUpdate(t, fake, planValues)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected refusal diagnostic for a masked kept leaf")
	}
	if fake.updateCalls != 0 {
		t.Fatalf("expected Update not to be called, got %d calls", fake.updateCalls)
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Detail(), "CredentialsList[0].CertificatePreference.Names[0]") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected diagnostic to name the masked path, got: %v", resp.Diagnostics.Errors())
	}
}

func TestProfileResourceUpdate_AppleOsX_D4_NonMaskedKeptValue_Proceeds(t *testing.T) {
	t.Parallel()
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{AppleOsX: liveAppleOsXEntityWithCredential("perfectly-normal", "also-normal")}}
	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{
		"id":           stringVal("999"),
		"network_list": networkReferencingCredential("cred-a"),
		"credentials_list": credentialsListVal([]map[string]tftypes.Value{
			{
				"credential_source": stringVal("DefinedCertificateAuthority"),
				"credential_name":   stringVal("cred-a"),
				"certificate_id":    int64Val(555),
			},
		}),
	})

	resp := runRMWUpdate(t, fake, planValues)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics.Errors())
	}
	if fake.updateCalls != 1 {
		t.Fatalf("expected exactly 1 Update call, got %d", fake.updateCalls)
	}
}
