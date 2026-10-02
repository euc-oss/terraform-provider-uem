package platform

import (
	"strconv"
	"strings"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

// This file implements the read-modify-write (RMW) overlay used by Update.
//
// UEM's per-platform profile update endpoints are FULL-REPLACE: whatever
// entity is sent becomes the new profile, wholesale. Update therefore can't
// simply send the entity built fresh from the Terraform plan (the same
// Build*CreateEntity used by Create) — any payload section the provider
// doesn't model (and isn't present in the plan-built entity) would be wiped
// server-side.
//
// The fix is: Get the live entity, and overlay only the sections this
// provider actually models (taken from the plan-built entity) onto it.
// Everything else on the live entity passes through untouched. Within
// General, only the specific fields BuildGeneralV2Create /
// BuildGeneralV4Create populate are overlaid — the rest of General passes
// through from the live entity.
//
// KNOWN RESIDUAL (tracked as follow-up internal-task): this overlay operates at
// SECTION granularity, not field granularity, for every modeled section
// other than General, Android's AndroidForWorkCustomMessages (g1, see
// overlayAndroidForWorkCustomMessages), and macOS's CredentialsList entries
// (g2, see overlayAppleOsXCredentialsList). If a modeled section (e.g.
// Passcode, Restrictions, DiskEncryption) has sub-fields that this
// provider's Terraform schema does not expose, those sub-fields are NOT
// preserved from the live entity — the whole section is replaced by
// whatever Build*CreateEntity produced from the plan. The D1 builder
// coverage guard (builder_coverage_test.go) pins the exact set of such gaps
// that exist today, so any new one is caught instead of silently landing.
// A true field-level deep merge for every modeled section is out of scope
// for this pass; g1 and g2 are the two gaps promoted out of that residual
// (internal-task task D2) because they are the ones the D1 guard found to be
// live-data losses on every Update, not merely un-set-by-the-plan fields.
// lists elsewhere (NetworkList, CustomSettingsList, etc.) remain plan-owned
// as a whole, same as before.
//
// Each Overlay* function below must be kept in sync with the matching
// Build*CreateEntity: the modeled-section list here must exactly match the
// top-level fields that builder function populates.
//
// PERMANENT masked-secret guard (internal-task): on GET, UEM renders every
// ENCRYPTED setting (per-setting flag, not by field name) as the literal
// "*****" (see UEMMaskedSecret in masked.go for the canonical source). This
// overlay's live entity can therefore contain masked secrets in any
// unmodeled section/field, and since UEM's write path has no sentinel
// compare, echoing "*****" back stores it as the new secret — silently
// destroying the real value. The modeled-section/modeled-General-field sets
// declared next to each Overlay* function below are also the single source
// of truth for internal/profile/platform/masked.go's per-platform
// MaskedUnmodeled* detectors, which resource_update_osx.go and
// resource_platforms.go call, right before svc.Update, to refuse an update
// that would echo a masked secret from an unmodeled part of the live
// entity. This guard is NOT a temporary workaround: UEM has no way to tell
// the provider "this value is unchanged," so it is a durable requirement of
// operating against this API's full-replace Update endpoint.

// modeledGeneralV2Keys is the JSON-key source of truth for the General
// (GeneralPayloadV2Entity) fields overlayGeneralV2Fields overlays from the
// plan. Used by masked.go's MaskedUnmodeled* detectors to know which
// General sub-keys are plan-controlled (and therefore exempt from the
// masked-secret guard — a user may legitimately type "*****" into a modeled
// field) versus which General sub-keys pass through from live untouched
// (and so must never legitimately be "*****" from Update's perspective).
var modeledGeneralV2Keys = map[string]bool{
	"Name":                   true,
	"Description":            true,
	"AssignmentType":         true,
	"ProfileScope":           true,
	"AssignedSmartGroups":    true,
	"ExcludedSmartGroups":    true,
	"IsActive":               true,
	"ProfileContext":         true,
	"ManagedLocationGroupID": true,
}

// overlayGeneralV2Fields overlays onto live only the General fields that
// BuildGeneralV2Create populates: Name, Description, AssignmentType,
// ProfileScope, AssignedSmartGroups, ExcludedSmartGroups, IsActive,
// ProfileContext, ManagedLocationGroupID. All other General fields
// (Password, AllowRemoval, ExpirationDate, AssignedSchedule,
// OrganizationGroupID, Version, ProfileID, ProfileUUID, Platform, etc.)
// pass through unchanged from live. If live is nil, planned is used as-is
// (nothing to overlay onto). If planned is nil, live is returned unchanged.
//
// AssignmentType and IsActive here are whatever
// resource_crud.go's Create/Update already defaulted onto the plan model
// before Build*CreateEntity ran (see EnsureComputedDefaults and the
// assignment_type/is_active comments in resource_crud.go, citing canonical
// Q4) — this function does not apply its own defaulting, it only overlays
// the already-resolved value. ProfileContext is likewise whatever
// resource_crud.go resolved (see the B16 decision table row #39, left
// untouched pending an owner decision).
func overlayGeneralV2Fields(live, planned *sdk.GeneralPayloadV2Entity) *sdk.GeneralPayloadV2Entity {
	if live == nil {
		return planned
	}
	if planned == nil {
		return live
	}
	live.Name = planned.Name
	live.Description = planned.Description
	live.AssignmentType = planned.AssignmentType
	live.ProfileScope = planned.ProfileScope
	live.AssignedSmartGroups = planned.AssignedSmartGroups
	live.ExcludedSmartGroups = planned.ExcludedSmartGroups
	live.IsActive = planned.IsActive
	live.ProfileContext = planned.ProfileContext
	live.ManagedLocationGroupID = planned.ManagedLocationGroupID
	return live
}

// modeledGeneralV4Keys is the V4 (Linux) equivalent of modeledGeneralV2Keys:
// the JSON-key source of truth for the General (GeneralPayloadV4Entity)
// fields overlayGeneralV4Fields overlays from the plan.
var modeledGeneralV4Keys = map[string]bool{
	"Name":                   true,
	"Description":            true,
	"AssignmentType":         true,
	"ProfileScope":           true,
	"IsActive":               true,
	"ProfileContext":         true,
	"ManagedLocationGroupID": true,
}

// overlayGeneralV4Fields is the V4 (Linux) equivalent of
// overlayGeneralV2Fields, overlaying the same modeled field set as
// BuildGeneralV4Create: Name, Description, AssignmentType, ProfileScope,
// IsActive, ProfileContext, ManagedLocationGroupID. BuildGeneralV4Create
// does not populate AssignedSmartGroups/ExcludedSmartGroups, so those are
// not overlaid here and pass through from live.
func overlayGeneralV4Fields(live, planned *sdk.GeneralPayloadV4Entity) *sdk.GeneralPayloadV4Entity {
	if live == nil {
		return planned
	}
	if planned == nil {
		return live
	}
	live.Name = planned.Name
	live.Description = planned.Description
	live.AssignmentType = planned.AssignmentType
	live.ProfileScope = planned.ProfileScope
	live.IsActive = planned.IsActive
	live.ProfileContext = planned.ProfileContext
	live.ManagedLocationGroupID = planned.ManagedLocationGroupID
	return live
}

// OverlayAppleOsXUpdateEntity overlays the planned (plan-built) entity's
// modeled sections onto the live (fetched) entity for a macOS
// (AppleOsXDeviceProfileEntityV2) Update. Modeled sections (must match
// BuildAppleOsXCreateEntity): General (field-level overlay only, see
// overlayGeneralV2Fields), Passcode, CustomSettingsList, NetworkList,
// CredentialsList, DiskEncryption, GateKeeper, Restrictions,
// SystemExtensions, PrivacyPreferences, ScepList, WebClipsList, VpnList,
// EasMicrosoftOutlook.
//
// Every other top-level section on live — AssociatedDomains,
// CustomAttributes, EasNativeMailclient, EmailList, KernelExtension,
// SkipSetupAssistant, SmartCard, SsoExtensionList, and so on — is left as fetched. Each modeled
// section is assigned unconditionally, including nil/empty, so that a
// section removed from HCL clears it on the server, preserving today's
// semantics.

// appleOsXModeledSections is the JSON-key source of truth for
// OverlayAppleOsXUpdateEntity's modeled top-level sections (using the
// AppleOsXDeviceProfileEntityV2 struct's JSON tags, which match the Go
// field names). Used by masked.go's MaskedUnmodeledAppleOsX.
var appleOsXModeledSections = map[string]bool{
	"General":             true,
	"Passcode":            true,
	"CustomSettingsList":  true,
	"NetworkList":         true,
	"CredentialsList":     true,
	"DiskEncryption":      true,
	"GateKeeper":          true,
	"Restrictions":        true,
	"SystemExtensions":    true,
	"PrivacyPreferences":  true,
	"ScepList":            true,
	"WebClipsList":        true,
	"VpnList":             true,
	"EasMicrosoftOutlook": true,
	"CustomAttributes":    true,
	"KernelExtension":     true,
}

func OverlayAppleOsXUpdateEntity(live, planned *sdk.AppleOsXDeviceProfileEntityV2) *sdk.AppleOsXDeviceProfileEntityV2 {
	if live == nil {
		return planned
	}
	if planned == nil {
		return live
	}
	// Capture live's CredentialsList before it's overwritten below — the
	// per-entry merge needs to look it up by natural key.
	liveCredentials := live.CredentialsList
	live.General = overlayGeneralV2Fields(live.General, planned.General)
	live.Passcode = planned.Passcode
	live.CustomSettingsList = planned.CustomSettingsList
	live.NetworkList = planned.NetworkList
	live.CredentialsList = overlayAppleOsXCredentialsList(liveCredentials, planned.CredentialsList)
	live.DiskEncryption = planned.DiskEncryption
	live.GateKeeper = planned.GateKeeper
	live.ScepList = planned.ScepList
	live.WebClipsList = planned.WebClipsList
	live.VpnList = planned.VpnList
	live.EasMicrosoftOutlook = planned.EasMicrosoftOutlook
	live.CustomAttributes = planned.CustomAttributes
	live.KernelExtension = planned.KernelExtension
	live.Restrictions = planned.Restrictions
	live.SystemExtensions = planned.SystemExtensions
	live.PrivacyPreferences = planned.PrivacyPreferences
	return live
}

// canonicalDefinedCACredentialSource is the canonical CredentialSource
// string for a Certificate-Authority-backed credential.
//
// UEM source: Database/AirWatchDB/AirWatchDB/deviceProfile/Seeds/deviceProfile.DevicePlatformSettingOption.seed.sql:1133-1142,
// AirWatch API/AirWatch.Api.Entity/PickListItem.cs:47 (canonical Q7):
// "DefinedCA" is the only value UEM's API/picklist Key ever actually
// returns or expects for this credential source — CORRECTED from a prior
// (backwards) belief that "DefinedCertificateAuthority" was the canonical
// wire spelling. "DefinedCertificateAuthority" is a display/localization
// label, never a distinct stored/API value; it is kept here only as a
// legacy spelling this provider's own docs used to recommend (see
// internal/profile/resource.go's credentialSourceCanonicalizeModifier,
// which canonicalizes it at plan time so an existing config or state using
// either spelling does not perpetually diff against UEM's own "DefinedCA"
// readback).
const canonicalDefinedCACredentialSource = "DefinedCA"

// definedCACredentialSourceLegacySpelling is the older, non-canonical
// spelling normalizeCredentialSource still collapses onto
// canonicalDefinedCACredentialSource — see that constant's comment.
const definedCACredentialSourceLegacySpelling = "DefinedCertificateAuthority"

// normalizeCredentialSource normalizes a CredentialSource value for
// cross-entry comparison between a planned entry and a live entry,
// collapsing the two spellings UEM treats as equivalent for a CA-backed
// credential ("DefinedCA" and the legacy "DefinedCertificateAuthority")
// onto the canonical canonicalDefinedCACredentialSource ("DefinedCA").
// Every other value (including "Upload" and "") is returned trimmed and
// otherwise unchanged.
func normalizeCredentialSource(source string) string {
	trimmed := strings.TrimSpace(source)
	if trimmed == canonicalDefinedCACredentialSource || trimmed == definedCACredentialSourceLegacySpelling {
		return canonicalDefinedCACredentialSource
	}
	return trimmed
}

// appleOsXCredentialTierKeyFunc computes one tier's natural-key component
// for a single CredentialsList entry (planned or live). It returns
// ok == false when the entry doesn't carry the data this tier needs, in
// which case overlayAppleOsXCredentialsList moves on to try the next
// (weaker) tier for that entry.
type appleOsXCredentialTierKeyFunc func(sdk.AppleOsXCredentialPayloadEntityV2) (string, bool)

// appleOsXCredentialIDTierKey is Tier 1: CertificateID, when non-nil and
// non-zero. This is the strongest key because it identifies one specific
// uploaded/issued certificate object in UEM, independent of any
// user-supplied or UEM-generated display name.
func appleOsXCredentialIDTierKey(c sdk.AppleOsXCredentialPayloadEntityV2) (string, bool) {
	if c.CertificateID == nil || *c.CertificateID == 0 {
		return "", false
	}
	return strconv.Itoa(*c.CertificateID), true
}

// appleOsXCredentialCATemplateTierKey is Tier 2, for
// DefinedCA entries only: the pair
// (CertificateAuthority, CertificateTemplate), when both are non-nil and
// non-zero. A DefinedCA credential has no CertificateID of its own (the ID
// belongs to whatever certificate the CA currently issues), so
// CA+Template is the strongest available key for that credential type.
func appleOsXCredentialCATemplateTierKey(c sdk.AppleOsXCredentialPayloadEntityV2) (string, bool) {
	if normalizeCredentialSource(c.CredentialSource) != canonicalDefinedCACredentialSource {
		return "", false
	}
	if c.CertificateAuthority == nil || *c.CertificateAuthority == 0 {
		return "", false
	}
	if c.CertificateTemplate == nil || *c.CertificateTemplate == 0 {
		return "", false
	}
	return strconv.Itoa(*c.CertificateAuthority) + "/" + strconv.Itoa(*c.CertificateTemplate), true
}

// appleOsXCredentialNameTierKey is Tier 3, the last resort: trimmed
// CredentialName if non-empty, else trimmed Name (UEM's auto-generated
// display name, e.g. "Certificate #1" — see
// internal/profile/state/read_mappers.go's mapAppleOsXCredentialsList
// comment, ~L309-320).
func appleOsXCredentialNameTierKey(c sdk.AppleOsXCredentialPayloadEntityV2) (string, bool) {
	name := strings.TrimSpace(c.CredentialName)
	if name == "" {
		name = strings.TrimSpace(c.Name)
	}
	if name == "" {
		return "", false
	}
	return name, true
}

// appleOsXCredentialTiers lists the g2 natural-key tiers in
// strongest-to-weakest order. See overlayAppleOsXCredentialsList for how
// they're tried.
var appleOsXCredentialTiers = []appleOsXCredentialTierKeyFunc{
	appleOsXCredentialIDTierKey,
	appleOsXCredentialCATemplateTierKey,
	appleOsXCredentialNameTierKey,
}

// appleOsXCredentialTierIndex builds a {normalized-source}|{tier key} ->
// entry-indexes index for one tier over a CredentialsList, so
// overlayAppleOsXCredentialsList can look up how many entries (live or
// planned) share a given tier key AND source. Folding the normalized source
// into the index key is what implements the "every tier ALSO requires the
// same credential source type" rule: two entries with different normalized
// sources simply land under different index keys and can never collide.
func appleOsXCredentialTierIndex(list []sdk.AppleOsXCredentialPayloadEntityV2, keyFn appleOsXCredentialTierKeyFunc) map[string][]int {
	idx := make(map[string][]int)
	for i, c := range list {
		raw, ok := keyFn(c)
		if !ok {
			continue
		}
		full := normalizeCredentialSource(c.CredentialSource) + "|" + raw
		idx[full] = append(idx[full], i)
	}
	return idx
}

// overlayAppleOsXCredentialsList keeps CredentialsList plan-owned as a
// whole list: the returned list's entries and order always come from
// plannedList (a modeled-null plannedList still clears the section, same as
// every other modeled list on this entity). Per 01o task D2 gap g2, for
// each planned entry, the builder-unmodeled sub-objects
// (CertificatePreference/IdentityPreference/CertificateMetadata) are
// carried over from a matched live entry, found via a TIERED natural key
// (appleOsXCredentialTiers, strongest first: CertificateID, then
// CertificateAuthority+CertificateTemplate for DefinedCA entries, then
// name).
//
// Plain name-only matching (the previous implementation) fails on REAL UEM
// data, which is why this is tiered rather than name-only:
//
//   - UEM returns DefinedCA credentials with
//     CredentialName == "" and Name == "Certificate #1" (see
//     internal/profile/state/read_mappers.go's mapAppleOsXCredentialsList,
//     ~L309-320) — the live entry never carries the user's label at all.
//   - UEM renames Upload credentials to "Certificate #N" on the live entity
//     (see internal/profile/state/state_helpers.go's
//     MergeCredentialsListWithPriorState comment, ~L355-358) — so even an
//     Upload credential's live name diverges from the planned label the
//     instant it's applied once.
//
// The planned entry, by contrast, always carries the user's own label in
// CredentialName (it's built fresh from Terraform config/plan). So on real
// data the planned and live names essentially never coincide, and a
// name-only matcher silently drops CertificatePreference/IdentityPreference
// on almost every Update.
//
// For a given planned entry, tiers are tried strongest-to-weakest:
//
//   - If a tier's key is absent for the planned entry (e.g. no
//     CertificateID), move on to the next (weaker) tier.
//   - If the tier's key IS present, it decides the outcome for this entry
//     — there's no fallthrough to a weaker tier after this point, whether
//     the tier resolves a match or not. A tier resolves cleanly only when
//     exactly one live entry AND exactly one planned entry share that
//     (source, key) pair; anything else — zero live matches, more than one
//     live match, or a duplicate key among the planned entries themselves —
//     is AMBIGUOUS (or absent) for this tier and carries nothing.
//
// There is deliberately NO positional/index fallback (matching by list
// position when no key matches): a wrong IdentityPreference/
// CertificatePreference/CertificateMetadata landing on the wrong
// certificate is worse than a missing one — the missing one is merely
// inconvenient (re-pick it in the UI), the wrong one is a silent trust
// misconfiguration. Safety over convenience.
//
// Each live entry can be claimed by at most one planned entry. Planned
// entries are processed in plan order; the first planned entry whose
// decided tier resolves to a given live entry claims it. If a LATER
// planned entry's decided tier also resolves to that same (already
// claimed) live entry, nothing is carried for that later entry — it does
// NOT search a weaker tier instead, since the tier that decided its
// outcome already ran and resolved cleanly (to a now-unusable live entry).
// Without this, a single live entry could otherwise be carried into two
// different planned entries via two different tiers (e.g. one planned
// entry matching it on CertificateID, another matching it on name).
//
// Carried over from the matched live entry:
//
//   - CertificatePreference and IdentityPreference: copied unconditionally
//     on any tier match.
//   - CertificateMetadata: copied ONLY when the matched live entry's
//     CertificateID is non-nil, non-zero, and equal to the planned entry's
//     CertificateID (also non-nil/non-zero) — metadata describes one
//     specific uploaded certificate, so after a new upload (a different
//     CertificateID) it would be stale and must not be carried over, even
//     if the entries matched via a weaker tier (e.g. CA+Template or name).
func overlayAppleOsXCredentialsList(liveList, plannedList []sdk.AppleOsXCredentialPayloadEntityV2) []sdk.AppleOsXCredentialPayloadEntityV2 {
	if plannedList == nil {
		return nil
	}

	liveIndex := make([]map[string][]int, len(appleOsXCredentialTiers))
	plannedIndex := make([]map[string][]int, len(appleOsXCredentialTiers))
	for ti, keyFn := range appleOsXCredentialTiers {
		liveIndex[ti] = appleOsXCredentialTierIndex(liveList, keyFn)
		plannedIndex[ti] = appleOsXCredentialTierIndex(plannedList, keyFn)
	}

	out := make([]sdk.AppleOsXCredentialPayloadEntityV2, len(plannedList))
	claimedLive := make(map[int]bool)
	for i, p := range plannedList {
		out[i] = p
		normSrc := normalizeCredentialSource(p.CredentialSource)

		for ti, keyFn := range appleOsXCredentialTiers {
			raw, ok := keyFn(p)
			if !ok {
				continue // tier's key absent on the planned entry: try the next tier.
			}
			full := normSrc + "|" + raw
			liveIdxs := liveIndex[ti][full]
			plannedIdxs := plannedIndex[ti][full]
			if len(liveIdxs) != 1 || len(plannedIdxs) != 1 {
				break // absent/ambiguous at a present tier: carry nothing, no fallthrough.
			}
			liveIdx := liveIdxs[0]
			if claimedLive[liveIdx] {
				break // live entry already claimed by an earlier planned entry: carry nothing, no fallthrough.
			}
			matched := liveList[liveIdx]
			claimedLive[liveIdx] = true

			out[i].CertificatePreference = matched.CertificatePreference
			out[i].IdentityPreference = matched.IdentityPreference

			if matched.CertificateID != nil && *matched.CertificateID != 0 &&
				p.CertificateID != nil && *p.CertificateID != 0 &&
				*matched.CertificateID == *p.CertificateID {
				out[i].CertificateMetadata = matched.CertificateMetadata
			}
			break // this tier decided the outcome for this entry.
		}
	}
	return out
}

// appleOsXKeptLiveSubtrees is the production source of truth for the g2
// sub-objects overlayAppleOsXCredentialsList can carry over from a live
// CredentialsList entry (see the D1-pinned gap in builder_coverage_test.go:
// TestBuilderCoverageGuard_AppleOsX's "CredentialsList" case) — masked.go's
// MaskedUnmodeledAppleOsX walks these paths against the RAW live entity
// (before the overlay/match runs) to catch a masked "*****" that could be
// carried over — conservatively, over every live entry's sub-objects, even
// ones that won't end up matching any planned entry, because at guard time
// we don't yet know which entries will match (see masked.go for detail).
var appleOsXKeptLiveSubtrees = []string{
	"CredentialsList[].CertificateMetadata",
	"CredentialsList[].CertificatePreference",
	"CredentialsList[].IdentityPreference",
}

// OverlayAndroidUpdateEntity overlays the planned entity's modeled sections
// onto the live entity for an Android (AndroidDeviceProfileV2Entity)
// Update. Modeled sections (must match BuildAndroidCreateEntity): General
// (field-level overlay only), AndroidForWorkCustomMessages,
// CustomSettingsList. All other top-level sections on live (the many
// AndroidContainer*/AndroidForWork* payloads this provider does not model)
// are left as fetched.

// androidModeledSections is the JSON-key source of truth for
// OverlayAndroidUpdateEntity's modeled top-level sections. Used by
// masked.go's MaskedUnmodeledAndroid.
var androidModeledSections = map[string]bool{
	"General":                      true,
	"AndroidForWorkCustomMessages": true,
	"CustomSettingsList":           true,
}

func OverlayAndroidUpdateEntity(live, planned *sdk.AndroidDeviceProfileV2Entity) *sdk.AndroidDeviceProfileV2Entity {
	if live == nil {
		return planned
	}
	if planned == nil {
		return live
	}
	live.General = overlayGeneralV2Fields(live.General, planned.General)
	live.AndroidForWorkCustomMessages = overlayAndroidForWorkCustomMessages(live.AndroidForWorkCustomMessages, planned.AndroidForWorkCustomMessages)
	live.CustomSettingsList = planned.CustomSettingsList
	return live
}

// overlayAndroidForWorkCustomMessages merges planned's
// AndroidForWorkCustomMessages onto live at FIELD granularity (01o task D2,
// gap g1), unlike every other modeled section on this entity, which is
// replaced wholesale. AndroidForWorkCustomMessages is not itself a
// user-facing Terraform block — it's a UEM implementation grouping around
// the single lock_screen_message attribute — so "a modeled-null value
// clears the field" is applied per modeled FIELD (LockScreenMessage) here,
// rather than to the section as a whole:
//
//   - LockScreenMessage always comes from planned (nil/empty when the plan's
//     lock_screen_message is null or planned itself is nil — clearing it).
//   - LongSupportMessage and ShortSupportMessage are never modeled by this
//     provider (see BuildAndroidCreateEntity / the D1 pinned gap in
//     builder_coverage_test.go) and are always kept from live.
//
// If the resulting section has no non-empty field at all, the section
// itself is set to nil, preserving today's "omitted when nothing is set"
// wire behavior (see TestProfileResourceUpdate_PreservesExistingPayloads).
func overlayAndroidForWorkCustomMessages(live, planned *sdk.AndroidForWorkCustomMessagesPayloadV2Entity) *sdk.AndroidForWorkCustomMessagesPayloadV2Entity {
	result := &sdk.AndroidForWorkCustomMessagesPayloadV2Entity{}
	if live != nil {
		result.LongSupportMessage = live.LongSupportMessage
		result.ShortSupportMessage = live.ShortSupportMessage
	}
	if planned != nil {
		result.LockScreenMessage = planned.LockScreenMessage
	}
	if result.LockScreenMessage == "" && result.LongSupportMessage == "" && result.ShortSupportMessage == "" {
		return nil
	}
	return result
}

// androidKeptLiveLeaves is the production source of truth for the g1 leaves
// overlayAndroidForWorkCustomMessages carries over from a live
// AndroidForWorkCustomMessages (see the D1-pinned gap in
// builder_coverage_test.go: TestBuilderCoverageGuard_Android's
// "AndroidForWorkCustomMessages" case) — masked.go's MaskedUnmodeledAndroid
// walks these paths against the RAW live entity to catch a masked "*****"
// that would otherwise be echoed back as the new value.
var androidKeptLiveLeaves = []string{
	"AndroidForWorkCustomMessages.LongSupportMessage",
	"AndroidForWorkCustomMessages.ShortSupportMessage",
}

// OverlayAppleiOSUpdateEntity overlays the planned entity's modeled
// sections onto the live entity for an Apple iOS (AppleDeviceProfileV2Entity)
// Update. Modeled sections (must match BuildAppleiOSCreateEntity): General
// (field-level overlay only), Passcode, CustomSettingsList.
//
// Every other top-level section on live — AWMailCredentialList,
// CredentialsList, Domains, EASNativeMailClientList, EasAwMailClient,
// EmailList, GoogleAccount, HomeScreen, Notifications, Restrictions,
// ScepList, SharedDevice, and so on — is left as fetched.

// appleiOSModeledSections is the JSON-key source of truth for
// OverlayAppleiOSUpdateEntity's modeled top-level sections. Used by
// masked.go's MaskedUnmodeledAppleiOS.
var appleiOSModeledSections = map[string]bool{
	"General":            true,
	"Passcode":           true,
	"CustomSettingsList": true,
}

func OverlayAppleiOSUpdateEntity(live, planned *sdk.AppleDeviceProfileV2Entity) *sdk.AppleDeviceProfileV2Entity {
	if live == nil {
		return planned
	}
	if planned == nil {
		return live
	}
	live.General = overlayGeneralV2Fields(live.General, planned.General)
	live.Passcode = planned.Passcode
	live.CustomSettingsList = planned.CustomSettingsList
	return live
}

// OverlayWindows10UpdateEntity overlays the planned entity's modeled
// sections onto the live entity for a Windows 10 (WinRTDeviceProfileV2Entity)
// Update. Modeled sections (must match BuildWindows10CreateEntity):
// General only (field-level overlay only).
//
// Every other top-level section on live — AntiVirus, AppControl, Bios,
// Credentials, CustomSettings, Customization, DefenderExploitGuard, Dem,
// Edp, Encryption, ExchangeActiveSync, ExchangeWebServices, Firewall,
// FirewallV2, and so on — is left as fetched.

// windows10ModeledSections is the JSON-key source of truth for
// OverlayWindows10UpdateEntity's modeled top-level sections. Used by
// masked.go's MaskedUnmodeledWindows10.
var windows10ModeledSections = map[string]bool{
	"General": true,
}

func OverlayWindows10UpdateEntity(live, planned *sdk.WinRTDeviceProfileV2Entity) *sdk.WinRTDeviceProfileV2Entity {
	if live == nil {
		return planned
	}
	if planned == nil {
		return live
	}
	live.General = overlayGeneralV2Fields(live.General, planned.General)
	return live
}

// OverlayWindowsRuggedUpdateEntity overlays the planned entity's modeled
// sections onto the live entity for a Windows Rugged (QnxDeviceProfileEntityV2)
// Update. Modeled sections (must match BuildWindowsRuggedCreateEntity):
// General only (field-level overlay only). CustomAttributePayload is left
// as fetched.

// windowsRuggedModeledSections is the JSON-key source of truth for
// OverlayWindowsRuggedUpdateEntity's modeled top-level sections. Used by
// masked.go's MaskedUnmodeledWindowsRugged.
var windowsRuggedModeledSections = map[string]bool{
	"General": true,
}

func OverlayWindowsRuggedUpdateEntity(live, planned *sdk.QnxDeviceProfileEntityV2) *sdk.QnxDeviceProfileEntityV2 {
	if live == nil {
		return planned
	}
	if planned == nil {
		return live
	}
	live.General = overlayGeneralV2Fields(live.General, planned.General)
	return live
}

// OverlayLinuxUpdateEntity overlays the planned entity's modeled sections
// onto the live entity for a Linux (LinuxDeviceProfileEntity1V4) Update.
// Modeled sections (must match BuildLinuxCreateEntity): General only
// (V4 field-level overlay only, see overlayGeneralV4Fields). Credentials,
// CustomConfigurations, and Wifis are left as fetched.

// linuxModeledSections is the JSON-key source of truth for
// OverlayLinuxUpdateEntity's modeled top-level sections. Used by masked.go's
// MaskedUnmodeledLinux. Note the lowercase "general": unlike every other
// platform's GeneralPayloadV2Entity ("General"), LinuxDeviceProfileEntity1V4
// tags its General field `json:"general,omitempty"`.
var linuxModeledSections = map[string]bool{
	"general": true,
}

func OverlayLinuxUpdateEntity(live, planned *sdk.LinuxDeviceProfileEntity1V4) *sdk.LinuxDeviceProfileEntity1V4 {
	if live == nil {
		return planned
	}
	if planned == nil {
		return live
	}
	live.General = overlayGeneralV4Fields(live.General, planned.General)
	return live
}
