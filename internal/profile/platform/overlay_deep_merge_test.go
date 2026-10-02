package platform

// internal-task, task D2: direct Overlay-level tests for the two promoted
// field/entry-level merges (g1: Android AndroidForWorkCustomMessages, g2:
// macOS CredentialsList per-entry sub-objects), plus a drift-guard test
// tying the production "kept live leaves" lists (androidKeptLiveLeaves /
// appleOsXKeptLiveSubtrees in overlay.go) back to the D1-pinned unmodeled
// leaves in builder_coverage_test.go, so the two lists can't silently drift
// apart (g3 is excluded here: it's a ruled "skip", not a kept leaf).

import (
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

// --- g1: Android AndroidForWorkCustomMessages ---

func TestOverlayAndroidUpdateEntity_G1_LiveLongShortSurviveLockScreenChange(t *testing.T) {
	live := &sdk.AndroidDeviceProfileV2Entity{
		General: &sdk.GeneralPayloadV2Entity{Name: "live"},
		AndroidForWorkCustomMessages: &sdk.AndroidForWorkCustomMessagesPayloadV2Entity{
			LockScreenMessage:   "old lock message",
			LongSupportMessage:  "live long message",
			ShortSupportMessage: "live short message",
		},
	}
	planned := &sdk.AndroidDeviceProfileV2Entity{
		General: &sdk.GeneralPayloadV2Entity{Name: "planned"},
		AndroidForWorkCustomMessages: &sdk.AndroidForWorkCustomMessagesPayloadV2Entity{
			LockScreenMessage: "new lock message",
		},
	}

	out := OverlayAndroidUpdateEntity(live, planned)

	if out.AndroidForWorkCustomMessages == nil {
		t.Fatal("expected AndroidForWorkCustomMessages to survive, got nil")
	}
	if got := out.AndroidForWorkCustomMessages.LockScreenMessage; got != "new lock message" {
		t.Errorf("LockScreenMessage = %q, want %q (from plan)", got, "new lock message")
	}
	if got := out.AndroidForWorkCustomMessages.LongSupportMessage; got != "live long message" {
		t.Errorf("LongSupportMessage = %q, want %q (kept from live)", got, "live long message")
	}
	if got := out.AndroidForWorkCustomMessages.ShortSupportMessage; got != "live short message" {
		t.Errorf("ShortSupportMessage = %q, want %q (kept from live)", got, "live short message")
	}
}

func TestOverlayAndroidUpdateEntity_G1_PlanNullClearsLockScreenButKeepsLiveLongShort(t *testing.T) {
	live := &sdk.AndroidDeviceProfileV2Entity{
		AndroidForWorkCustomMessages: &sdk.AndroidForWorkCustomMessagesPayloadV2Entity{
			LockScreenMessage:   "old lock message",
			LongSupportMessage:  "live long message",
			ShortSupportMessage: "live short message",
		},
	}
	planned := &sdk.AndroidDeviceProfileV2Entity{
		AndroidForWorkCustomMessages: nil, // lock_screen_message removed from HCL.
	}

	out := OverlayAndroidUpdateEntity(live, planned)

	if out.AndroidForWorkCustomMessages == nil {
		t.Fatal("expected AndroidForWorkCustomMessages to survive with kept Long/Short, got nil")
	}
	if got := out.AndroidForWorkCustomMessages.LockScreenMessage; got != "" {
		t.Errorf("LockScreenMessage = %q, want \"\" (cleared by null plan)", got)
	}
	if got := out.AndroidForWorkCustomMessages.LongSupportMessage; got != "live long message" {
		t.Errorf("LongSupportMessage = %q, want %q (kept from live)", got, "live long message")
	}
	if got := out.AndroidForWorkCustomMessages.ShortSupportMessage; got != "live short message" {
		t.Errorf("ShortSupportMessage = %q, want %q (kept from live)", got, "live short message")
	}
}

func TestOverlayAndroidUpdateEntity_G1_NothingLiveNothingPlanned_SectionOmitted(t *testing.T) {
	live := &sdk.AndroidDeviceProfileV2Entity{}
	planned := &sdk.AndroidDeviceProfileV2Entity{}

	out := OverlayAndroidUpdateEntity(live, planned)

	if out.AndroidForWorkCustomMessages != nil {
		t.Errorf("expected AndroidForWorkCustomMessages to be nil (omitted), got %+v", out.AndroidForWorkCustomMessages)
	}
}

// --- g2: macOS CredentialsList ---
//
// These fixtures model REAL UEM live shapes, not the "cred-a on both sides"
// shape a name-only matcher would need: a live Upload credential's
// CredentialName is UEM-rewritten to "Certificate #N" (state_helpers.go
// ~L355-358) and a live DefinedCA credential's CredentialName comes back ""
// with Name "Certificate #N" (read_mappers.go ~L309-320) — the planned
// entry, by contrast, always carries the user's own label.

func intPtrLocal(n int) *int { return &n }

// macOSUploadCredEntry builds a live-shaped Upload CredentialsList entry:
// UEM rewrites both CredentialName and Name to the same "Certificate #N".
func macOSUploadCredEntry(displayName string, certID *int, certPref, identPref []string, meta *sdk.CertificateMetadataModel1V2) sdk.AppleOsXCredentialPayloadEntityV2 {
	c := sdk.AppleOsXCredentialPayloadEntityV2{
		CredentialSource: "Upload",
		CredentialName:   displayName,
		Name:             displayName,
		CertificateID:    certID,
	}
	if certPref != nil {
		c.CertificatePreference = &sdk.MacOsCredentialCertificatePreferencePayloadV2Model{Names: certPref}
	}
	if identPref != nil {
		c.IdentityPreference = &sdk.MacOsCredentialIdentityPreferencePayloadV2Model{Names: identPref}
	}
	c.CertificateMetadata = meta
	return c
}

// macOSDefinedCACredEntry builds a live-shaped DefinedCA CredentialsList
// entry: CredentialName is empty, Name is UEM's auto-generated display name.
func macOSDefinedCACredEntry(displayName string, ca, template *int, certPref, identPref []string, meta *sdk.CertificateMetadataModel1V2) sdk.AppleOsXCredentialPayloadEntityV2 {
	c := sdk.AppleOsXCredentialPayloadEntityV2{
		CredentialSource:     canonicalDefinedCACredentialSource,
		Name:                 displayName,
		CertificateAuthority: ca,
		CertificateTemplate:  template,
	}
	if certPref != nil {
		c.CertificatePreference = &sdk.MacOsCredentialCertificatePreferencePayloadV2Model{Names: certPref}
	}
	if identPref != nil {
		c.IdentityPreference = &sdk.MacOsCredentialIdentityPreferencePayloadV2Model{Names: identPref}
	}
	c.CertificateMetadata = meta
	return c
}

// macOSPlannedCredEntry builds a plan-shaped CredentialsList entry: the
// user's own label always lands in CredentialName, Name is never set (the
// provider never builds Name — it's UEM's own auto-generated field).
func macOSPlannedCredEntry(source, credentialName string, certID, ca, template *int) sdk.AppleOsXCredentialPayloadEntityV2 {
	return sdk.AppleOsXCredentialPayloadEntityV2{
		CredentialSource:     source,
		CredentialName:       credentialName,
		CertificateID:        certID,
		CertificateAuthority: ca,
		CertificateTemplate:  template,
	}
}

// (a) Tier 1: Upload credential matched by CertificateID — all three carried.
func TestOverlayAppleOsXUpdateEntity_G2_UploadSameCertID_AllThreeCarried(t *testing.T) {
	liveMeta := &sdk.CertificateMetadataModel1V2{SubjectName: "live-subject"}
	live := &sdk.AppleOsXDeviceProfileEntityV2{
		CredentialsList: []sdk.AppleOsXCredentialPayloadEntityV2{
			macOSUploadCredEntry("Certificate #2", intPtrLocal(501), []string{"live-cert-name"}, []string{"live-ident-name"}, liveMeta),
		},
	}
	planned := &sdk.AppleOsXDeviceProfileEntityV2{
		CredentialsList: []sdk.AppleOsXCredentialPayloadEntityV2{
			macOSPlannedCredEntry("Upload", "my-wifi-cert", intPtrLocal(501), nil, nil),
		},
	}

	out := OverlayAppleOsXUpdateEntity(live, planned)

	if len(out.CredentialsList) != 1 {
		t.Fatalf("expected 1 credential, got %d", len(out.CredentialsList))
	}
	got := out.CredentialsList[0]
	if got.CertificatePreference == nil || len(got.CertificatePreference.Names) != 1 || got.CertificatePreference.Names[0] != "live-cert-name" {
		t.Errorf("expected CertificatePreference carried from live, got %+v", got.CertificatePreference)
	}
	if got.IdentityPreference == nil || len(got.IdentityPreference.Names) != 1 || got.IdentityPreference.Names[0] != "live-ident-name" {
		t.Errorf("expected IdentityPreference carried from live, got %+v", got.IdentityPreference)
	}
	if got.CertificateMetadata == nil || got.CertificateMetadata.SubjectName != "live-subject" {
		t.Errorf("expected CertificateMetadata carried from live (same CertificateID), got %+v", got.CertificateMetadata)
	}
	if got.CredentialName != "my-wifi-cert" {
		t.Errorf("expected planned CredentialName (user label) to survive, got %q", got.CredentialName)
	}
}

// (b) Upload with a brand-new certificate: different CertificateID, no
// other tier applies (no CA/template, and the planned/live display names
// differ) — nothing carried.
func TestOverlayAppleOsXUpdateEntity_G2_UploadNewCertificate_NoneCarried(t *testing.T) {
	live := &sdk.AppleOsXDeviceProfileEntityV2{
		CredentialsList: []sdk.AppleOsXCredentialPayloadEntityV2{
			macOSUploadCredEntry("Certificate #2", intPtrLocal(501), []string{"live-cert-name"}, []string{"live-ident-name"}, &sdk.CertificateMetadataModel1V2{SubjectName: "s"}),
		},
	}
	planned := &sdk.AppleOsXDeviceProfileEntityV2{
		CredentialsList: []sdk.AppleOsXCredentialPayloadEntityV2{
			macOSPlannedCredEntry("Upload", "my-wifi-cert", intPtrLocal(777), nil, nil),
		},
	}

	out := OverlayAppleOsXUpdateEntity(live, planned)

	got := out.CredentialsList[0]
	if got.CertificatePreference != nil || got.IdentityPreference != nil || got.CertificateMetadata != nil {
		t.Errorf("expected nothing carried for a re-uploaded certificate with no other matching tier, got %+v", got)
	}
}

// (c) Tier 2: DefinedCA credential matched by CertificateAuthority+
// CertificateTemplate (planned has no CertificateID at all, but the live
// entry DOES have one) — prefs carried, metadata NOT: the metadata
// condition requires CertificateID to be non-nil/non-zero AND EQUAL on
// BOTH sides, and the planned side has no CertificateID at all here, so a
// live-side-only CertificateID must not be enough to carry metadata. Run
// for both the canonical "DefinedCertificateAuthority" spelling and the
// "DefinedCA" spelling UEM also accepts.
func TestOverlayAppleOsXUpdateEntity_G2_DefinedCA_CATemplateMatch_PrefsCarriedMetadataNot(t *testing.T) {
	for _, plannedSource := range []string{"DefinedCertificateAuthority", "DefinedCA"} {
		t.Run(plannedSource, func(t *testing.T) {
			liveMeta := &sdk.CertificateMetadataModel1V2{SubjectName: "live-subject"}
			liveEntry := macOSDefinedCACredEntry("Certificate #1", intPtrLocal(12), intPtrLocal(34), []string{"live-cert-name"}, []string{"live-ident-name"}, liveMeta)
			liveEntry.CertificateID = intPtrLocal(900)
			live := &sdk.AppleOsXDeviceProfileEntityV2{
				CredentialsList: []sdk.AppleOsXCredentialPayloadEntityV2{liveEntry},
			}
			planned := &sdk.AppleOsXDeviceProfileEntityV2{
				CredentialsList: []sdk.AppleOsXCredentialPayloadEntityV2{
					macOSPlannedCredEntry(plannedSource, "corp-ca", nil, intPtrLocal(12), intPtrLocal(34)),
				},
			}

			out := OverlayAppleOsXUpdateEntity(live, planned)

			got := out.CredentialsList[0]
			if got.CertificatePreference == nil || got.CertificatePreference.Names[0] != "live-cert-name" {
				t.Errorf("expected CertificatePreference carried via CA+Template match, got %+v", got.CertificatePreference)
			}
			if got.IdentityPreference == nil || got.IdentityPreference.Names[0] != "live-ident-name" {
				t.Errorf("expected IdentityPreference carried via CA+Template match, got %+v", got.IdentityPreference)
			}
			if got.CertificateMetadata != nil {
				t.Errorf("expected CertificateMetadata NOT carried (planned CertificateID is nil, even though live has one), got %+v", got.CertificateMetadata)
			}
			if got.CredentialName != "corp-ca" {
				t.Errorf("expected planned CredentialName (user label) to survive, got %q", got.CredentialName)
			}
		})
	}
}

// (d) Two live DefinedCA entries with the same CA/Template: ambiguous at
// Tier 2, and Tier 3 (name) also can't help since the planned entry has no
// name in common with either live entry's auto-generated Name — nothing
// carried.
func TestOverlayAppleOsXUpdateEntity_G2_DefinedCA_AmbiguousCATemplate_NoneCarried(t *testing.T) {
	live := &sdk.AppleOsXDeviceProfileEntityV2{
		CredentialsList: []sdk.AppleOsXCredentialPayloadEntityV2{
			macOSDefinedCACredEntry("Certificate #1", intPtrLocal(12), intPtrLocal(34), []string{"live-1"}, nil, nil),
			macOSDefinedCACredEntry("Certificate #2", intPtrLocal(12), intPtrLocal(34), []string{"live-2"}, nil, nil),
		},
	}
	planned := &sdk.AppleOsXDeviceProfileEntityV2{
		CredentialsList: []sdk.AppleOsXCredentialPayloadEntityV2{
			macOSPlannedCredEntry(canonicalDefinedCACredentialSource, "corp-ca", nil, intPtrLocal(12), intPtrLocal(34)),
		},
	}

	out := OverlayAppleOsXUpdateEntity(live, planned)

	got := out.CredentialsList[0]
	if got.CertificatePreference != nil || got.IdentityPreference != nil || got.CertificateMetadata != nil {
		t.Errorf("expected nothing carried when live has an ambiguous (duplicate CA+Template) match, got %+v", got)
	}
}

// (e) Source mismatch: the same CertificateID on both sides, but one side
// Upload and the other DefinedCA — Tier 1's key is present for the planned
// entry, so there's no fallthrough to a weaker tier, and no live entry
// shares the (source, CertificateID) pair — nothing carried.
func TestOverlayAppleOsXUpdateEntity_G2_SourceMismatchSameCertID_NoneCarried(t *testing.T) {
	live := &sdk.AppleOsXDeviceProfileEntityV2{
		CredentialsList: []sdk.AppleOsXCredentialPayloadEntityV2{
			macOSUploadCredEntry("Certificate #2", intPtrLocal(501), []string{"live-cert-name"}, []string{"live-ident-name"}, &sdk.CertificateMetadataModel1V2{SubjectName: "s"}),
		},
	}
	planned := &sdk.AppleOsXDeviceProfileEntityV2{
		CredentialsList: []sdk.AppleOsXCredentialPayloadEntityV2{
			macOSPlannedCredEntry(canonicalDefinedCACredentialSource, "corp-ca", intPtrLocal(501), nil, nil),
		},
	}

	out := OverlayAppleOsXUpdateEntity(live, planned)

	got := out.CredentialsList[0]
	if got.CertificatePreference != nil || got.IdentityPreference != nil || got.CertificateMetadata != nil {
		t.Errorf("expected nothing carried across a credential-source mismatch, got %+v", got)
	}
}

// (f) Tier 3: name-only match — planned name "Certificate #1" (the user
// literally chose that label) matching a live DefinedCA entry's
// auto-generated Name "Certificate #1", with no CertificateID/CA/Template
// on either side to use a stronger tier — prefs carried.
func TestOverlayAppleOsXUpdateEntity_G2_NameOnlyMatch_PrefsCarried(t *testing.T) {
	live := &sdk.AppleOsXDeviceProfileEntityV2{
		CredentialsList: []sdk.AppleOsXCredentialPayloadEntityV2{
			macOSDefinedCACredEntry("Certificate #1", nil, nil, []string{"live-cert-name"}, []string{"live-ident-name"}, nil),
		},
	}
	planned := &sdk.AppleOsXDeviceProfileEntityV2{
		CredentialsList: []sdk.AppleOsXCredentialPayloadEntityV2{
			macOSPlannedCredEntry(canonicalDefinedCACredentialSource, "Certificate #1", nil, nil, nil),
		},
	}

	out := OverlayAppleOsXUpdateEntity(live, planned)

	got := out.CredentialsList[0]
	if got.CertificatePreference == nil || got.CertificatePreference.Names[0] != "live-cert-name" {
		t.Errorf("expected CertificatePreference carried via Tier 3 name match, got %+v", got.CertificatePreference)
	}
	if got.IdentityPreference == nil || got.IdentityPreference.Names[0] != "live-ident-name" {
		t.Errorf("expected IdentityPreference carried via Tier 3 name match, got %+v", got.IdentityPreference)
	}
}

// (g) A duplicate key among the PLANNED entries: two planned entries share
// the same Tier 3 name — ambiguous on the planned side, so nothing is
// carried for either of them.
func TestOverlayAppleOsXUpdateEntity_G2_DuplicatePlannedNameKey_NoneCarried(t *testing.T) {
	live := &sdk.AppleOsXDeviceProfileEntityV2{
		CredentialsList: []sdk.AppleOsXCredentialPayloadEntityV2{
			macOSDefinedCACredEntry("dup-name", nil, nil, []string{"live-1"}, nil, nil),
		},
	}
	planned := &sdk.AppleOsXDeviceProfileEntityV2{
		CredentialsList: []sdk.AppleOsXCredentialPayloadEntityV2{
			macOSPlannedCredEntry(canonicalDefinedCACredentialSource, "dup-name", nil, nil, nil),
			macOSPlannedCredEntry(canonicalDefinedCACredentialSource, "dup-name", nil, nil, nil),
		},
	}

	out := OverlayAppleOsXUpdateEntity(live, planned)

	for i, got := range out.CredentialsList {
		if got.CertificatePreference != nil || got.IdentityPreference != nil || got.CertificateMetadata != nil {
			t.Errorf("entry %d: expected nothing carried when planned entries share a duplicate key, got %+v", i, got)
		}
	}
}

// (h) Mixed list order: plan and live list the same two credentials in
// opposite order — correct pairing must come from the tiered keys, not
// position, proving there's no positional/index fallback in the matcher.
func TestOverlayAppleOsXUpdateEntity_G2_MixedOrder_PairsByKeyNotPosition(t *testing.T) {
	live := &sdk.AppleOsXDeviceProfileEntityV2{
		CredentialsList: []sdk.AppleOsXCredentialPayloadEntityV2{
			macOSUploadCredEntry("Certificate #2", intPtrLocal(501), []string{"live-upload"}, nil, nil),
			macOSDefinedCACredEntry("Certificate #1", intPtrLocal(12), intPtrLocal(34), []string{"live-ca"}, nil, nil),
		},
	}
	planned := &sdk.AppleOsXDeviceProfileEntityV2{
		CredentialsList: []sdk.AppleOsXCredentialPayloadEntityV2{
			// Opposite order vs. live: CA entry first, Upload entry second.
			macOSPlannedCredEntry(canonicalDefinedCACredentialSource, "corp-ca", nil, intPtrLocal(12), intPtrLocal(34)),
			macOSPlannedCredEntry("Upload", "my-wifi-cert", intPtrLocal(501), nil, nil),
		},
	}

	out := OverlayAppleOsXUpdateEntity(live, planned)

	if len(out.CredentialsList) != 2 {
		t.Fatalf("expected 2 credentials, got %d", len(out.CredentialsList))
	}
	ca := out.CredentialsList[0]
	if ca.CertificatePreference == nil || ca.CertificatePreference.Names[0] != "live-ca" {
		t.Errorf("expected the CA entry to pair with the live CA entry by key, got %+v", ca.CertificatePreference)
	}
	upload := out.CredentialsList[1]
	if upload.CertificatePreference == nil || upload.CertificatePreference.Names[0] != "live-upload" {
		t.Errorf("expected the Upload entry to pair with the live Upload entry by key, got %+v", upload.CertificatePreference)
	}
}

// (i) Present-tier no-match must NOT fall through to a weaker tier: the
// planned entry's Tier 1 key (CertificateID) IS present (this is a
// re-uploaded certificate, so the ID changed), and it fails to resolve
// (the live entry has a different CertificateID) — even though the
// planned and live entries' Tier 3 names DO coincide ("Certificate #2"),
// that weaker tier must never be consulted once a stronger, present tier
// has already decided the outcome.
func TestOverlayAppleOsXUpdateEntity_G2_PresentTierNoMatch_NoFallthroughToWeakerTier(t *testing.T) {
	liveMeta := &sdk.CertificateMetadataModel1V2{SubjectName: "live-subject"}
	live := &sdk.AppleOsXDeviceProfileEntityV2{
		CredentialsList: []sdk.AppleOsXCredentialPayloadEntityV2{
			macOSUploadCredEntry("Certificate #2", intPtrLocal(501), []string{"live-cert-name"}, []string{"live-ident-name"}, liveMeta),
		},
	}
	planned := &sdk.AppleOsXDeviceProfileEntityV2{
		CredentialsList: []sdk.AppleOsXCredentialPayloadEntityV2{
			// Re-uploaded cert: imported CredentialName kept, but a new
			// CertificateID from the fresh upload.
			macOSPlannedCredEntry("Upload", "Certificate #2", intPtrLocal(777), nil, nil),
		},
	}

	out := OverlayAppleOsXUpdateEntity(live, planned)

	got := out.CredentialsList[0]
	if got.CertificatePreference != nil || got.IdentityPreference != nil || got.CertificateMetadata != nil {
		t.Errorf("expected nothing carried: Tier 1's key is present but doesn't match, so Tier 3's name match must not be used, got %+v", got)
	}
}

// (j) A single live entry can be claimed by at most one planned entry.
// Here the first planned entry matches the shared live entry via Tier 1
// (CertificateID) and the second planned entry would ALSO match it via
// Tier 3 (name) — but since the live entry is already claimed by the
// first planned entry, the second must carry nothing rather than pairing
// with an already-claimed live entry.
func TestOverlayAppleOsXUpdateEntity_G2_LiveEntryClaimedOnce_SecondPlannedGetsNothing(t *testing.T) {
	live := &sdk.AppleOsXDeviceProfileEntityV2{
		CredentialsList: []sdk.AppleOsXCredentialPayloadEntityV2{
			macOSUploadCredEntry("shared-name", intPtrLocal(501), []string{"live-cert-name"}, []string{"live-ident-name"}, nil),
		},
	}
	planned := &sdk.AppleOsXDeviceProfileEntityV2{
		CredentialsList: []sdk.AppleOsXCredentialPayloadEntityV2{
			macOSPlannedCredEntry("Upload", "cert-a-label", intPtrLocal(501), nil, nil), // Tier 1 match on the shared live entry
			macOSPlannedCredEntry("Upload", "shared-name", nil, nil, nil),               // Tier 3 match on the SAME live entry
		},
	}

	out := OverlayAppleOsXUpdateEntity(live, planned)

	first := out.CredentialsList[0]
	if first.CertificatePreference == nil || first.CertificatePreference.Names[0] != "live-cert-name" {
		t.Errorf("expected the first planned entry (Tier 1 claimant) to carry prefs, got %+v", first.CertificatePreference)
	}
	second := out.CredentialsList[1]
	if second.CertificatePreference != nil || second.IdentityPreference != nil || second.CertificateMetadata != nil {
		t.Errorf("expected the second planned entry to carry nothing (live entry already claimed), got %+v", second)
	}
}

func TestOverlayAppleOsXUpdateEntity_G2_PlanNullClearsList(t *testing.T) {
	live := &sdk.AppleOsXDeviceProfileEntityV2{
		CredentialsList: []sdk.AppleOsXCredentialPayloadEntityV2{
			macOSUploadCredEntry("Certificate #2", intPtrLocal(501), []string{"live-1"}, nil, nil),
		},
	}
	planned := &sdk.AppleOsXDeviceProfileEntityV2{
		CredentialsList: nil,
	}

	out := OverlayAppleOsXUpdateEntity(live, planned)

	if out.CredentialsList != nil {
		t.Errorf("expected CredentialsList cleared by a modeled-null plan, got %+v", out.CredentialsList)
	}
}

// --- modeled sub-fields still reflect the plan ---

func TestOverlayAppleOsXUpdateEntity_ModeledCredentialFieldsReflectPlan(t *testing.T) {
	live := &sdk.AppleOsXDeviceProfileEntityV2{
		CredentialsList: []sdk.AppleOsXCredentialPayloadEntityV2{
			macOSUploadCredEntry("cred-a", intPtrLocal(555), []string{"live-1"}, nil, nil),
		},
	}
	planned := &sdk.AppleOsXDeviceProfileEntityV2{
		CredentialsList: []sdk.AppleOsXCredentialPayloadEntityV2{
			{
				CredentialName:       "cred-a",
				CredentialSource:     "DefinedCertificateAuthority",
				CertificateID:        intPtrLocal(555),
				CertificateAuthority: intPtrLocal(7),
			},
		},
	}

	out := OverlayAppleOsXUpdateEntity(live, planned)
	got := out.CredentialsList[0]
	if got.CredentialSource != "DefinedCertificateAuthority" {
		t.Errorf("CredentialSource = %q, want plan value", got.CredentialSource)
	}
	if got.CertificateAuthority == nil || *got.CertificateAuthority != 7 {
		t.Errorf("CertificateAuthority = %v, want 7 (from plan)", got.CertificateAuthority)
	}
}

// --- drift guard: kept-leaf production lists match D1's pinned gaps ---

// d1PinnedLeafPaths extracts just the leaf-path strings out of
// d1PinnedUnmodeled[platform][section] (builder_coverage_test.go), the
// SHARED source of truth D1 itself asserts against — so this test and D1
// can't silently drift apart the way two independently hard-coded literal
// lists could (M3).
func d1PinnedLeafPaths(platform, section string) []string {
	entries := d1PinnedUnmodeled[platform][section]
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.path
	}
	return out
}

// TestKeptLiveLeaves_MatchD1PinnedGaps asserts androidKeptLiveLeaves and
// appleOsXKeptLiveSubtrees (the masked.go D4 source of truth, declared in
// overlay.go) cover EXACTLY the g1/g2 leaves d1PinnedUnmodeled
// (builder_coverage_test.go, D1's own pinned expectations — not a second,
// independently hard-coded copy of them) pins as unmodeled — so the "what
// the overlay now keeps from live" list and "what masked.go now scans" list
// can't silently drift apart from what D1 itself asserts. G3 (General
// smart-group Name) is deliberately excluded: 01o ruled it a no-op skip,
// not a kept leaf, so it is neither overlaid field-by-field nor scanned by
// masked.go, and it isn't a key in d1PinnedUnmodeled at all (General is
// asserted separately from every other modeled section — see
// generalV2SmartGroupUnmodeled/expectedSmartGroupNameUnmodeled in
// builder_coverage_test.go).
func TestKeptLiveLeaves_MatchD1PinnedGaps(t *testing.T) {
	wantAndroid := setFromSlice(d1PinnedLeafPaths("android", "AndroidForWorkCustomMessages")...)
	gotAndroid := setFromSlice(androidKeptLiveLeaves...)
	assertSetEqual(t, "androidKeptLiveLeaves vs D1 g1 pinned gaps", gotAndroid, wantAndroid)

	// appleOsXKeptLiveSubtrees pins SUBTREE roots (e.g.
	// "CredentialsList[].CertificateMetadata"), while D1's pinned gaps are
	// individual LEAVES under those roots (e.g.
	// "CredentialsList[].CertificateMetadata.CertificateUuid"). Assert every
	// D1-pinned g2 leaf falls under at least one of the declared subtree
	// roots (the loop below stops at the FIRST matching root it finds,
	// it doesn't assert the match is unique), and that every declared root
	// has at least one D1-pinned leaf under it (so a stale/unused root
	// would also be caught).
	wantG2Leaves := d1PinnedLeafPaths("appleOsX", "CredentialsList")
	rootHit := make(map[string]bool, len(appleOsXKeptLiveSubtrees))
	for _, leaf := range wantG2Leaves {
		matched := ""
		for _, root := range appleOsXKeptLiveSubtrees {
			if len(leaf) > len(root) && leaf[:len(root)] == root && leaf[len(root)] == '.' {
				matched = root
				break
			}
		}
		if matched == "" {
			t.Errorf("D1-pinned g2 leaf %q is not covered by any appleOsXKeptLiveSubtrees root %v", leaf, appleOsXKeptLiveSubtrees)
			continue
		}
		rootHit[matched] = true
	}
	for _, root := range appleOsXKeptLiveSubtrees {
		if !rootHit[root] {
			t.Errorf("appleOsXKeptLiveSubtrees root %q has no corresponding D1-pinned g2 leaf — stale entry?", root)
		}
	}
}
