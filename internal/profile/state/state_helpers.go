package state

import (
	"context"
	"fmt"
	"strings"

	commonerrors "github.com/euc-oss/terraform-provider-uem/internal/common/errors"
	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func NormalizePasscodeUnknownsToNull(p *profilemodels.PasscodeModel) {
	if p == nil {
		return
	}
	if p.RequirePasscodeOnDevice.IsUnknown() {
		p.RequirePasscodeOnDevice = types.BoolNull()
	}
	if p.AllowSimpleValue.IsUnknown() {
		p.AllowSimpleValue = types.BoolNull()
	}
	if p.RequireAlphanumericValue.IsUnknown() {
		p.RequireAlphanumericValue = types.BoolNull()
	}
	if p.MinimumPasscodeLength.IsUnknown() {
		p.MinimumPasscodeLength = types.Int64Null()
	}
	if p.MinimumNumberOfComplexCharacters.IsUnknown() {
		p.MinimumNumberOfComplexCharacters = types.StringNull()
	}
	if p.MaximumPasscodeAge.IsUnknown() {
		p.MaximumPasscodeAge = types.StringNull()
	}
	if p.AutoLock.IsUnknown() {
		p.AutoLock = types.StringNull()
	}
	if p.GracePeriod.IsUnknown() {
		p.GracePeriod = types.Int64Null()
	}
	if p.MaxFailedAttempts.IsUnknown() {
		p.MaxFailedAttempts = types.StringNull()
	}
	if p.PinHistory.IsUnknown() {
		p.PinHistory = types.StringNull()
	}
	if p.MinutesUntilFailedLoginReset.IsUnknown() {
		p.MinutesUntilFailedLoginReset = types.Int64Null()
	}
}

// NormalizeNetworkUnknownsToNull replaces any Unknown attribute on each
// NetworkItemModel with its typed null. Required after Create because the
// resource schema marks every network attribute Optional+Computed; UEM only
// returns the attributes the caller actually configured, so the rest stay
// Unknown after the post-create readback. Terraform's post-apply consistency
// check then rejects the response. Mirrors NormalizePasscodeUnknownsToNull.
func NormalizeNetworkUnknownsToNull(items []profilemodels.NetworkItemModel) {
	for i := range items {
		n := &items[i]
		if n.NetworkInterface.IsUnknown() {
			n.NetworkInterface = types.StringNull()
		}
		if n.ServiceSetIdentifier.IsUnknown() {
			n.ServiceSetIdentifier = types.StringNull()
		}
		if n.HiddenNetwork.IsUnknown() {
			n.HiddenNetwork = types.BoolNull()
		}
		if n.AutoJoin.IsUnknown() {
			n.AutoJoin = types.BoolNull()
		}
		if n.SecurityType.IsUnknown() {
			n.SecurityType = types.StringNull()
		}
		if n.Password.IsUnknown() {
			n.Password = types.StringNull()
		}
		if n.UseAsLoginWindowConfiguration.IsUnknown() {
			n.UseAsLoginWindowConfiguration = types.BoolNull()
		}
		if n.UseDirectoryAuthentication.IsUnknown() {
			n.UseDirectoryAuthentication = types.BoolNull()
		}
		if n.TLS.IsUnknown() {
			n.TLS = types.BoolNull()
		}
		if n.TTLS.IsUnknown() {
			n.TTLS = types.BoolNull()
		}
		if n.LEAP.IsUnknown() {
			n.LEAP = types.BoolNull()
		}
		if n.PEAP.IsUnknown() {
			n.PEAP = types.BoolNull()
		}
		if n.EAPFAST.IsUnknown() {
			n.EAPFAST = types.BoolNull()
		}
		if n.EAPSIM.IsUnknown() {
			n.EAPSIM = types.BoolNull()
		}
		if n.EAPAKA.IsUnknown() {
			n.EAPAKA = types.BoolNull()
		}
		if n.TLSMinimumVersion.IsUnknown() {
			n.TLSMinimumVersion = types.StringNull()
		}
		if n.TLSMaximumVersion.IsUnknown() {
			n.TLSMaximumVersion = types.StringNull()
		}
		if n.DisableAssociationMACRandomization.IsUnknown() {
			n.DisableAssociationMACRandomization = types.BoolNull()
		}
		if n.UserName.IsUnknown() {
			n.UserName = types.StringNull()
		}
		if n.UserPassword.IsUnknown() {
			n.UserPassword = types.StringNull()
		}
		if n.IdentityCertificate.IsUnknown() {
			n.IdentityCertificate = types.StringNull()
		}
		if n.InnerIdentity.IsUnknown() {
			n.InnerIdentity = types.StringNull()
		}
		if n.OuterIdentity.IsUnknown() {
			n.OuterIdentity = types.StringNull()
		}
		if n.UsePAC.IsUnknown() {
			n.UsePAC = types.BoolNull()
		}
		if n.AllowTwoRANDs.IsUnknown() {
			n.AllowTwoRANDs = types.BoolNull()
		}
		if n.TrustedCertificates.IsUnknown() {
			n.TrustedCertificates = types.ListNull(types.StringType)
		}
		if n.AllowTrustExceptions.IsUnknown() {
			n.AllowTrustExceptions = types.BoolNull()
		}
		if n.ProxyType.IsUnknown() {
			n.ProxyType = types.StringNull()
		}
		if n.ProxyServer.IsUnknown() {
			n.ProxyServer = types.StringNull()
		}
		if n.ProxyServerPort.IsUnknown() {
			n.ProxyServerPort = types.Int64Null()
		}
		if n.ProxyUsername.IsUnknown() {
			n.ProxyUsername = types.StringNull()
		}
		if n.ProxyPassword.IsUnknown() {
			n.ProxyPassword = types.StringNull()
		}
		if n.ProxyUrl.IsUnknown() {
			n.ProxyUrl = types.StringNull()
		}
		if n.PacFallback.IsUnknown() {
			n.PacFallback = types.BoolNull()
		}
	}
}

// NormalizeCredentialsUnknownsToNull mirrors NormalizeNetworkUnknownsToNull
// for credentials_list entries.
func NormalizeCredentialsUnknownsToNull(items []profilemodels.CredentialItemModel) {
	for i := range items {
		c := &items[i]
		if c.CredentialSource.IsUnknown() {
			c.CredentialSource = types.StringNull()
		}
		if c.CredentialName.IsUnknown() {
			c.CredentialName = types.StringNull()
		}
		if c.CertificatePayload.IsUnknown() {
			c.CertificatePayload = types.StringNull()
		}
		if c.CertificatePassword.IsUnknown() {
			c.CertificatePassword = types.StringNull()
		}
		if c.CertificateID.IsUnknown() {
			c.CertificateID = types.Int64Null()
		}
		if c.CertificateAuthority.IsUnknown() {
			c.CertificateAuthority = types.Int64Null()
		}
		if c.CertificateTemplate.IsUnknown() {
			c.CertificateTemplate = types.Int64Null()
		}
		if c.AllowAccessToAllApplications.IsUnknown() {
			c.AllowAccessToAllApplications = types.BoolNull()
		}
		if c.KeyIsExtractable.IsUnknown() {
			c.KeyIsExtractable = types.BoolNull()
		}
	}
}

func IsApplePlatform(platform string) bool {
	return platform == sdk.PlatformAppleOsX || platform == sdk.PlatformAppleiOS
}

func IsValidProfileContext(v string) bool {
	return v == "User" || v == "Device"
}

// internal-ticket removed EnsureComputedDefaults, which used to force a null
// ProfileScope/AssignmentType/IsActive/UUID to "Production"/"Auto"/true/""
// after every Read (row #96 of the B16 audit: never observed live, no
// source). Read now stores exactly what UEM returned for those fields, with
// no post-hoc defaulting.

func UploadCertificatesTyped(ctx context.Context, client *sdk.Client, items, priorCreds []profilemodels.CredentialItemModel) ([]profilemodels.CredentialItemModel, error) {
	priorByName := make(map[string]profilemodels.CredentialItemModel, len(priorCreds))
	for _, p := range priorCreds {
		if key, ok := CredentialKey(p); ok {
			priorByName[key] = p
		}
	}

	out := make([]profilemodels.CredentialItemModel, len(items))
	copy(out, items)
	var uploadSvc *sdk.ProfilesV1Service
	for i, c := range out {
		if c.CredentialSource.ValueString() != "Upload" {
			continue
		}
		if c.CertificatePayload.IsNull() || c.CertificatePayload.IsUnknown() || c.CertificatePayload.ValueString() == "" {
			continue
		}

		if key, ok := CredentialKey(c); ok {
			if prior, found := priorByName[key]; found {
				samePayload := !prior.CertificatePayload.IsNull() &&
					prior.CertificatePayload.ValueString() == c.CertificatePayload.ValueString()
				hasID := !prior.CertificateID.IsNull() && prior.CertificateID.ValueInt64() != 0
				if samePayload && hasID {
					out[i].CertificateID = prior.CertificateID
					continue
				}
			}
		}

		if !c.CertificateID.IsNull() && !c.CertificateID.IsUnknown() && c.CertificateID.ValueInt64() != 0 {
			continue
		}

		name := credentialDisplayName(c, i)

		if client == nil {
			return nil, fmt.Errorf("credential %q requires certificate upload but no SDK client was provided: %w", name, commonerrors.ErrCertificateUploadUnsupported)
		}

		if uploadSvc == nil {
			uploadSvc = sdk.NewProfilesV1Service(client)
		}

		_, resp, err := uploadSvc.UploadCertificate(ctx, &sdk.CertificateV1{
			CertificatePayload: c.CertificatePayload.ValueString(),
			Password:           c.CertificatePassword.ValueString(),
		})
		if err != nil {
			return nil, fmt.Errorf("upload certificate %q: %w", name, err)
		}
		if resp == nil || resp.Value == nil {
			return nil, fmt.Errorf("upload certificate %q: empty response (no certificate ID)", name)
		}
		out[i].CertificateID = types.Int64Value(*resp.Value)
	}
	return out, nil
}

// internal-ticket: no longer drops the whole credentials_list when no
// network_list entry references it (row #97 of the B16 audit: never
// observed live, no source -- this silently sent an empty list to UEM even
// though the user configured credentials). Every configured credential is
// now sent, referenced or not.
func PrepareCredentialsForPayload(ctx context.Context, client *sdk.Client, data *profilemodels.ProfileResourceModel, priorCreds []profilemodels.CredentialItemModel) error {
	if len(data.CredentialsList) == 0 {
		return nil
	}
	updated, err := UploadCertificatesTyped(ctx, client, data.CredentialsList, priorCreds)
	if err != nil {
		return err
	}
	data.CredentialsList = updated
	return nil
}

func CredentialKey(item profilemodels.CredentialItemModel) (string, bool) {
	if item.CredentialName.IsNull() || item.CredentialName.IsUnknown() {
		return "", false
	}
	name := strings.TrimSpace(item.CredentialName.ValueString())
	if name == "" {
		return "", false
	}
	return name, true
}

// internal-ticket removed MergeCredentialsListWithPriorState and
// MergeNetworkListWithPriorState (rows #87/#88 of the B16 audit: never
// observed live, no source). credentials_list and network_list are now
// stored exactly as mapAppleOsXCredentialsList/mapAppleOsXNetworkList
// returned them, with no name/index-keyed pinning of CredentialSource,
// CredentialName, IdentityCertificate, or any other field back to the prior
// state, and no KeepState/PreserveNull fallback for fields UEM's response
// left null.
//
// EXPECTED DIFF RISK: if UEM rewrites CredentialName (e.g. "DefinedCA" ->
// "DefinedCertificateAuthority", or an Upload credential's name to
// "Certificate #N") or the network's IdentityCertificate join key, plans
// will now show that rename as drift on every apply. This is real
// server-side drift the removed code was masking, not a bug -- see the
// CHANGELOG entry.

// internal-ticket removed KeepStateString/KeepStateBool/KeepStateInt64 along
// with their last callers (mergeNetworkItem, mergeAirWatch, mergeFileVault,
// MergeGatekeeperWithPriorState's MCX field); nothing in this package falls
// back to the prior state for a field UEM's response left null anymore.

// internal-ticket removed MergeGatekeeperWithPriorState (row #74 of the B16
// audit: never observed live, no source for masking fields the user left
// null). mapAppleOsXGatekeeper's result is now stored directly, with no
// merge against the prior state.

// MergeSystemExtensionsWithPriorState reconciles UEM's response with the
// user's prior config. The two nested lists get index/key-aligned reorder
// treatment (see mergeAllowedSystemExtensionTypesWithPriorState /
// mergeAllowedSystemExtensionsWithPriorState) confirmed live 2026-09-25;
// internal-ticket removed this function's own PreserveNull on AllowUserOverrides
// (row #78 of the B16 audit: never observed live, no source) -- that field
// is now taken from the API response unmodified.
func MergeSystemExtensionsWithPriorState(api, state *profilemodels.SystemExtensionsModel) *profilemodels.SystemExtensionsModel {
	if api == nil {
		return nil
	}
	if state == nil {
		return api
	}
	api.AllowedSystemExtensionTypes = mergeAllowedSystemExtensionTypesWithPriorState(api.AllowedSystemExtensionTypes, state.AllowedSystemExtensionTypes)
	api.AllowedSystemExtensions = mergeAllowedSystemExtensionsWithPriorState(api.AllowedSystemExtensions, state.AllowedSystemExtensions)
	return api
}

// mergeAllowedSystemExtensionTypesWithPriorState is keyed by team_identifier
// (the defining key of each entry -- see the design doc) rather than by
// position. Confirmed live 2026-09-25 (as<internal-env>): UEM does not preserve
// request order in its response (the "*" entry can come back first or last
// independent of where it was sent), so a positional merge falsely reports
// "changed" on every field of every reordered entry, tripping Terraform's
// post-apply consistency check even though nothing actually changed. The
// merged output is reordered to match stateList's own order (the user's
// config on Create, or the previously-stable state on Update/Read), and any
// api-only entry (no matching key in stateList -- e.g. a kept non-default
// "*" default UEM injects with no prior config at all, see
// isServerDefaultWildcardType) is appended, unmerged, in its original API
// order. Duplicate keys on either side are paired deterministically by
// arrival order (state's Nth occurrence of a key pairs with api's Nth
// not-yet-consumed occurrence of that same key).
//
// internal-ticket removed the PreserveNull* calls this used to apply to each
// matched item's fields (row #80 of the B16 audit: never confirmed live,
// unlike the reorder itself). A matched item is now the API's value
// unmodified; only its position in the result is reordered to match
// stateList.
func mergeAllowedSystemExtensionTypesWithPriorState(apiList, stateList []profilemodels.AllowedSystemExtensionTypeModel) []profilemodels.AllowedSystemExtensionTypeModel {
	if apiList == nil {
		return nil
	}
	apiByKey := make(map[string][]int, len(apiList))
	for i := range apiList {
		k := apiList[i].TeamIdentifier.ValueString()
		apiByKey[k] = append(apiByKey[k], i)
	}
	consumed := make([]bool, len(apiList))

	result := make([]profilemodels.AllowedSystemExtensionTypeModel, 0, len(apiList))
	for _, s := range stateList {
		idx, ok := nextUnconsumed(apiByKey[s.TeamIdentifier.ValueString()], consumed)
		if !ok {
			continue
		}
		consumed[idx] = true
		result = append(result, apiList[idx])
	}
	for i := range apiList {
		if !consumed[i] {
			result = append(result, apiList[i])
		}
	}
	return result
}

// mergeAllowedSystemExtensionsWithPriorState is keyed by
// bundle_identifier+team_identifier, same reordering shape as
// mergeAllowedSystemExtensionTypesWithPriorState above and for the same
// live-confirmed reason: UEM does not preserve request order.
//
// internal-ticket removed the second pass that used to match a prior entry
// setting only one of bundle/team against an api entry sharing just that
// one field, and the PreserveNull* calls applied to matched items (both row
// #82 of the B16 audit: never confirmed live, unlike the exact-match reorder
// itself). Only a prior entry that set BOTH fields is now reordered to match
// stateList's position; every other api entry (including ones a
// single-field prior entry would previously have claimed) is appended,
// unmodified, in its original API order.
func mergeAllowedSystemExtensionsWithPriorState(apiList, stateList []profilemodels.AllowedSystemExtensionModel) []profilemodels.AllowedSystemExtensionModel {
	if apiList == nil {
		return nil
	}
	byBoth := make(map[string][]int, len(apiList))
	for i := range apiList {
		b, t := apiList[i].BundleIdentifier.ValueString(), apiList[i].TeamIdentifier.ValueString()
		byBoth[allowedSystemExtensionKey(b, t)] = append(byBoth[allowedSystemExtensionKey(b, t)], i)
	}
	consumed := make([]bool, len(apiList))

	match := make([]int, len(stateList))
	for i := range match {
		match[i] = -1
	}
	for i, s := range stateList {
		if s.BundleIdentifier.IsNull() || s.TeamIdentifier.IsNull() {
			continue
		}
		if idx, ok := nextUnconsumed(byBoth[allowedSystemExtensionKey(s.BundleIdentifier.ValueString(), s.TeamIdentifier.ValueString())], consumed); ok {
			consumed[idx] = true
			match[i] = idx
		}
	}

	result := make([]profilemodels.AllowedSystemExtensionModel, 0, len(apiList))
	for _, idx := range match {
		if idx < 0 {
			continue
		}
		result = append(result, apiList[idx])
	}
	for i := range apiList {
		if !consumed[i] {
			result = append(result, apiList[i])
		}
	}
	return result
}

func allowedSystemExtensionKey(bundleIdentifier, teamIdentifier string) string {
	return bundleIdentifier + "\x00" + teamIdentifier
}

// nextUnconsumed returns the first index in idxs not yet marked consumed,
// which is how duplicate keys on both sides pair up deterministically: the
// state list's Nth occurrence of a key always pairs with the api list's Nth
// not-yet-consumed occurrence of that same key, in each side's own order.
func nextUnconsumed(idxs []int, consumed []bool) (int, bool) {
	for _, idx := range idxs {
		if !consumed[idx] {
			return idx, true
		}
	}
	return 0, false
}

// internal-ticket removed MergePrivacyPreferencesWithPriorState, identifierKey,
// mergePrivacyPreferenceItem, and mergeAppleEventsListWithPriorState (row
// #83 of the B16 audit). Unlike the system_extensions merges above, no
// PPPC reorder was ever confirmed live -- the gate citation was conditional
// ("if UEM reordered") and copied from the sysext contract -- so this was
// removed in full rather than split: mapAppleOsXPrivacyPreferences's result
// is now stored directly, with no keyed reorder and no PreserveNull against
// the prior state.

// internal-ticket removed MergeDiskEncryptionWithPriorState, mergeAirWatch, and
// mergeFileVault (rows #85/#86 of the B16 audit: never observed live, no
// source), and mergeNetworkItem (row #88, alongside MergeNetworkListWithPriorState
// above). disk_encryption and network_list sub-blocks are now stored exactly
// as UEM returned them: no sub-block is dropped because the user left it
// unconfigured, no field falls back to the prior state when the API sends
// null, and IdentityCertificate is no longer pinned to the prior state (see
// the EXPECTED DIFF RISK note above KeepStateString).

func credentialDisplayName(item profilemodels.CredentialItemModel, idx int) string {
	if key, ok := CredentialKey(item); ok {
		return key
	}
	return fmt.Sprintf("index_%d", idx)
}

// KeepStateList returns apiVal unless it is null/unknown, in which case the
// prior stateVal is preserved. Used for optional string-list attributes so
// fields the API omits don't show as drift.
func KeepStateList(apiVal, stateVal types.List) types.List {
	if apiVal.IsNull() || apiVal.IsUnknown() {
		return stateVal
	}
	return apiVal
}

// internal-ticket removed PreserveNullBool, PreserveNullString, PreserveNullInt64,
// and PreserveNullStringList along with their last callers (the credentials,
// system_extensions, PPPC, and restrictions merges above): nothing in this
// package discards an API value because the prior state had that field
// null anymore.

// stringSliceToTFList converts a []string from the SDK into a Terraform
// types.List. Returns ListNull when the slice is empty so no spurious diff
// is produced.
func stringSliceToTFList(in []string) types.List {
	if len(in) == 0 {
		return types.ListNull(types.StringType)
	}
	listVal, diags := types.ListValueFrom(context.Background(), types.StringType, in)
	if diags.HasError() {
		return types.ListNull(types.StringType)
	}
	return listVal
}

// MergeRestrictionsWithPriorState reconciles UEM's response with the user's
// prior config. internal-ticket removed almost all of what this used to do:
//   - dropping a sub-block the user didn't configure (row #93 of the B16
//     audit: never observed live, no source);
//   - restoring a prior sub-block the API omitted, for every sub-block
//     except Desktop (row #93; Desktop's restore is (a), kept below, per
//     R + GGS:43: Desktop is confirmed null on GET unless locked);
//   - PreserveNull on every leaf field, including the media/CD-DVD block
//     built from prior state (rows #94/#95: never observed live, and in
//     tension with the CD/DVD read_only=true validator, which the ruling
//     does not touch).
//
// mapAppleOsXRestrictions's result is now stored directly except for
// Desktop, which is restored from the prior state when the API omits it.
func MergeRestrictionsWithPriorState(api, state *profilemodels.RestrictionsModel) *profilemodels.RestrictionsModel {
	if api == nil {
		return nil
	}
	if state == nil {
		return api
	}
	if api.Desktop == nil && state.Desktop != nil {
		api.Desktop = state.Desktop
	}
	return api
}

// internal-ticket removed mergeRestrictionsApplications, mergeRestrictionsFunctionality,
// mergeRestrictionsICloud, mergeRestrictionsMedia, mergeMediaAccessPtr,
// mergeRestrictionsPreferences, and mergeRestrictionsSharing (rows #90/#93/
// #94/#95 of the B16 audit). Every restrictions leaf field is now the API
// value verbatim; nothing here falls back to the prior state, and the
// media/CD-DVD block is no longer fabricated from it (see the CD/DVD
// read_only=true validator this used to sit in tension with).

// EnsureComputedDefaults fills the General fields the provider has always
// defaulted when state holds null, EXCEPT for ProfileScope (removed):
// readGeneralV2IntoState and readGeneralV4IntoState now assign
// data.ProfileScope unconditionally from UEM's response (setDescriptionFromAPI's
// sibling for ProfileScope, added under internal-ticket), so after a real Read
// this field is always a concrete StringValue, never null or unknown —
// including "", which UEM returns live-confirmed 2026-09-25 on the 26.2 lab
// tenant (guarded B16 follow-up create) for a profile with no profile_scope
// configured. Keeping this line would have silently turned that faithful ""
// into "Production" the moment the field ever did arrive null/unknown (the
// General payload missing from the response entirely), masking a real gap
// instead of surfacing it.
//
// AssignmentType and IsActive are provider defaults now source-cited:
// UEM source: AirWatch API/AirWatch.ServiceModel/Profiles/V2/Resources/GeneralPayloadV2Entity.cs:691-707,140,148
// (canonical Q4). AssignmentType has no server default (an omitted value
// fails validation, so "Auto" is a provider-only convenience default);
// IsActive = true is confirmed as the server's own constructor default,
// unconditionally for every platform. UUID keeps its held default unchanged
// below (no canonical answer yet for that one — see B16 decision table row
// #40).
func EnsureComputedDefaults(data *profilemodels.ProfileResourceModel) {
	if data.AssignmentType.IsNull() || data.AssignmentType.IsUnknown() {
		data.AssignmentType = types.StringValue("Auto")
	}
	if data.IsActive.IsNull() || data.IsActive.IsUnknown() {
		data.IsActive = types.BoolValue(true)
	}
	if data.UUID.IsNull() || data.UUID.IsUnknown() {
		data.UUID = types.StringValue("")
	}
}
