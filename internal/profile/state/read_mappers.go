package state

import (
	"context"
	"strconv"
	"strings"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"
)

type ProfileResourceModel = profilemodels.ProfileResourceModel
type PasscodeModel = profilemodels.PasscodeModel
type CustomSettingsItemModel = profilemodels.CustomSettingsItemModel
type NetworkItemModel = profilemodels.NetworkItemModel
type CredentialItemModel = profilemodels.CredentialItemModel
type DiskEncryptionModel = profilemodels.DiskEncryptionModel
type DiskEncryptionAirWatchModel = profilemodels.DiskEncryptionAirWatchModel
type DiskEncryptionFileVaultModel = profilemodels.DiskEncryptionFileVaultModel
type GatekeeperModel = profilemodels.GatekeeperModel
type SystemExtensionsModel = profilemodels.SystemExtensionsModel
type AllowedSystemExtensionTypeModel = profilemodels.AllowedSystemExtensionTypeModel
type AllowedSystemExtensionModel = profilemodels.AllowedSystemExtensionModel
type PrivacyPreferenceModel = profilemodels.PrivacyPreferenceModel
type AppleEventModel = profilemodels.AppleEventModel

func ReadProfileIntoState(ctx context.Context, data *ProfileResourceModel, result *sdk.ProfileResult) {
	switch {
	case result.AppleOsX != nil:
		readAppleOsXIntoState(ctx, data, result.AppleOsX)
	case result.AppleiOS != nil:
		readAppleiOSIntoState(ctx, data, result.AppleiOS)
	case result.Android != nil:
		readAndroidIntoState(ctx, data, result.Android)
	case result.Windows10 != nil:
		readGeneralV2IntoState(data, result.Windows10.General)
	case result.WindowsRugged != nil:
		readGeneralV2IntoState(data, result.WindowsRugged.General)
	case result.Linux != nil:
		readGeneralV4IntoState(ctx, data, result.Linux.General)
	}
}

// preserveEnumCase keeps whatever case the caller (config/prior state)
// already had for assignment_type when it case-insensitively matches what
// the server returned, instead of always taking the server's canonical
// casing verbatim. This is what actually fixes internal-task's "Provider
// produced invalid plan" bug (real Terraform core forbids a plan modifier
// from overriding a value the user configured — see
// normalizeEnumCaseModifier's removal). Import/no-prior-value case:
// existing is null/unknown, so the server's canonical value is used as-is.
// Drift case: existing doesn't EqualFold match the server's value, so the
// server's (real, changed) value wins and is reflected in state/diff.
//
// internal-ticket: profile_scope no longer goes through this helper. internal-task
// only covers AssignmentType casing (Enum.TryParse ignoreCase:true); there
// is no equivalent source citation for ProfileScope, so profile_scope now
// stores UEM's response verbatim instead of preserving prior casing.
func preserveEnumCase(existing types.String, serverValue string) string {
	if !existing.IsNull() && !existing.IsUnknown() && strings.EqualFold(existing.ValueString(), serverValue) {
		return existing.ValueString()
	}
	return serverValue
}

// setDescriptionFromAPI maps the API's Description string onto
// data.Description verbatim. UEM's Description field is a non-pointer
// string with omitempty, so "the field was omitted" and "the field was
// sent as an explicit empty string" are indistinguishable on the wire; both
// decode to "". internal-ticket removed the prior-state-dependent null/""
// disambiguation here (row #57 of the B16 audit: never observed live, no
// source) in favor of storing exactly what the SDK decoded, with no
// conditioning on the previous state value.
//
// EXPECTED DIFF RISK: resource.go's nullWhenConfigNullStringModifier plans
// description as null when it is removed from HCL. On the post-apply
// readback after such a removal, UEM's wire value is "" and this function
// now stores "" (not null), which can trip Terraform's post-apply
// consistency check ("was cty.NullVal, but now cty.StringVal(\"\")"). No
// user config can avoid this because the SDK cannot represent the
// omitted/empty distinction; see the CHANGELOG entry.
func setDescriptionFromAPI(data *ProfileResourceModel, apiDescription string) {
	data.Description = types.StringValue(apiDescription)
}

func readGeneralV2IntoState(data *ProfileResourceModel, g *sdk.GeneralPayloadV2Entity) {
	if g == nil {
		return
	}
	data.Name = types.StringValue(g.Name)
	setDescriptionFromAPI(data, g.Description)
	// internal-ticket: the "only assign when non-empty" guards used to leave the
	// prior AssignmentType/ProfileScope value in place whenever UEM's wire
	// value was "" (row #58 of the B16 audit: never observed live). Both are
	// now assigned unconditionally so an empty wire value is reflected as an
	// empty string in state rather than silently keeping a stale value.
	data.AssignmentType = types.StringValue(preserveEnumCase(data.AssignmentType, g.AssignmentType))
	data.ProfileScope = types.StringValue(g.ProfileScope)
	if g.IsActive != nil {
		data.IsActive = types.BoolValue(*g.IsActive)
	}
	if g.ProfileUUID != "" {
		data.UUID = types.StringValue(g.ProfileUUID)
	}
	// internal-ticket: no longer requires ProfileContext to be "User" or
	// "Device" before storing it (row #59: never observed live that UEM
	// sends anything else, no source for silently ignoring an unrecognized
	// value and keeping the prior one).
	if g.ProfileContext != "" {
		data.ProfileContext = types.StringValue(g.ProfileContext)
	}
	if g.ManagedLocationGroupID != nil {
		data.OrgGroupID = types.StringValue(strconv.Itoa(*g.ManagedLocationGroupID))
	}
	// Map assigned/excluded smart groups from API to state. internal-ticket:
	// entries whose SmartGroupID is nil are no longer dropped (row #61); they
	// are stored as a null list element so the response's entry count is
	// preserved.
	data.AssignedSmartGroups = nil
	if len(g.AssignedSmartGroups) > 0 {
		data.AssignedSmartGroups = make([]types.String, 0, len(g.AssignedSmartGroups))
		for _, sg := range g.AssignedSmartGroups {
			if sg.SmartGroupID != nil {
				data.AssignedSmartGroups = append(data.AssignedSmartGroups, types.StringValue(strconv.Itoa(*sg.SmartGroupID)))
			} else {
				data.AssignedSmartGroups = append(data.AssignedSmartGroups, types.StringNull())
			}
		}
	}
	data.ExcludedSmartGroups = nil
	if len(g.ExcludedSmartGroups) > 0 {
		data.ExcludedSmartGroups = make([]types.String, 0, len(g.ExcludedSmartGroups))
		for _, sg := range g.ExcludedSmartGroups {
			if sg.SmartGroupID != nil {
				data.ExcludedSmartGroups = append(data.ExcludedSmartGroups, types.StringValue(strconv.Itoa(*sg.SmartGroupID)))
			} else {
				data.ExcludedSmartGroups = append(data.ExcludedSmartGroups, types.StringNull())
			}
		}
	}
}

func readGeneralV4IntoState(ctx context.Context, data *ProfileResourceModel, g *sdk.GeneralPayloadV4Entity) {
	if g == nil {
		return
	}
	data.Name = types.StringValue(g.Name)
	setDescriptionFromAPI(data, g.Description)
	if g.AssignmentType != "" {
		data.AssignmentType = types.StringValue(preserveEnumCase(data.AssignmentType, g.AssignmentType))
	}
	if g.ProfileScope != nil {
		if name, ok := profileScopeNameFromWireValue(*g.ProfileScope); ok {
			data.ProfileScope = types.StringValue(preserveEnumCase(data.ProfileScope, name))
		} else {
			tflog.Warn(ctx, "Unmapped Linux profile_scope wire value; leaving prior state unchanged", map[string]any{"wire_value": *g.ProfileScope})
		}
	}
	if g.IsActive != nil {
		data.IsActive = types.BoolValue(*g.IsActive)
	}
	if g.ProfileUUID != "" {
		data.UUID = types.StringValue(g.ProfileUUID)
	} else if g.UUID != "" {
		data.UUID = types.StringValue(g.UUID)
	}
	if g.ProfileContext != "" && IsValidProfileContext(g.ProfileContext) {
		data.ProfileContext = types.StringValue(g.ProfileContext)
	}
	if g.ManagedLocationGroupID != nil {
		data.OrgGroupID = types.StringValue(strconv.Itoa(*g.ManagedLocationGroupID))
	}
}

// profileScopeWireNames maps the V4 (Linux) wire integer ProfileScope value
// back to its canonical name, mirroring platform.profileScopeWireValues.
// See internal-task: Production=1 (live-confirmed against as<internal-env>.eng.example.com),
// Staging=2, Both=3 (UNVERIFIED live — no Linux profile existed in the
// accessible org group scope to confirm against a real server response;
// these follow the canonical enum ordering from the C# server source,
// release/26.2.0.0, per doctrine-quirk-17).
var profileScopeWireNames = map[int]string{
	1: "Production",
	2: "Staging",
	3: "Both",
}

func profileScopeNameFromWireValue(v int) (string, bool) {
	name, ok := profileScopeWireNames[v]
	return name, ok
}

func readAppleOsXIntoState(ctx context.Context, data *ProfileResourceModel, ent *sdk.AppleOsXDeviceProfileEntityV2) {
	readGeneralV2IntoState(data, ent.General)

	// internal-ticket: Passcode/NetworkList/CredentialsList/DiskEncryption/
	// Gatekeeper/PrivacyPreferences are stored straight from what UEM
	// returned, with no fallback to the prior state. See the B16 audit rows
	// #62-64, #66-67, #69-70, #74, #85-88, #97.
	data.Passcode = mapAppleOsXPasscode(ent.Passcode)

	data.CustomSettingsList = mapCustomSettingsListAppleOsX(data.CustomSettingsList, ent.CustomSettingsList)

	data.NetworkList = carryNetworkPasswords(mapAppleOsXNetworkList(ent.NetworkList), data.NetworkList)

	data.CredentialsList = carryCredentialSecrets(mapAppleOsXCredentialsList(ent.CredentialsList), data.CredentialsList)

	data.DiskEncryption = mapAppleOsXDiskEncryption(ent.DiskEncryption)

	data.Gatekeeper = mapAppleOsXGatekeeper(ent.GateKeeper)

	priorR := data.Restrictions
	data.Restrictions = MergeRestrictionsWithPriorState(mapAppleOsXRestrictions(ent.Restrictions), priorR)

	priorSE := data.SystemExtensions
	data.SystemExtensions = MergeSystemExtensionsWithPriorState(mapAppleOsXSystemExtensions(ent.SystemExtensions, priorSE), priorSE)

	data.PrivacyPreferences = mapAppleOsXPrivacyPreferences(ent.PrivacyPreferences)

	// Stored straight from what UEM returned, like the other lists above.
	data.ScepList = mapAppleOsXScepList(ent.ScepList)
	data.WebClipsList = mapAppleOsXWebClipsList(ent.WebClipsList)
	data.VpnList = carryVPNSecrets(mapVPNItemList(ent.VpnList), data.VpnList)
	data.EasMicrosoftOutlook = carryEasMicrosoftOutlookPassword(mapEasMicrosoftOutlookPtr(ent.EasMicrosoftOutlook), data.EasMicrosoftOutlook)
	data.CustomAttributes = mapCustomAttributeList(ent.CustomAttributes)
	data.KernelExtension = mapKernelExtensionPtr(ent.KernelExtension)
	_ = ctx
}

// internal-ticket: no longer gated on appleOsXPasscodeHasContent (row #63 of the
// B16 audit: never observed live that UEM omits this block, no source) and
// no longer merged with the prior state afterward (row #62: never observed
// live that UEM sends null for AutoLock/MaximumPasscodeAge/
// MinimumNumberOfComplexCharacters, no source). The block is now present in
// state whenever UEM's response includes the pointer, with each field
// mapped from the server value as-is.
//
// EXPECTED DIFF RISK: if UEM always returns this struct non-nil (even for a
// profile with no configured passcode policy), every macOS/iOS profile will
// now show an empty `passcode {}` block in state instead of a nil one. This
// was not confirmed either way by the audit.
func mapAppleOsXPasscode(p *sdk.AppleOsXPasscodePayloadEntityV2) *PasscodeModel {
	if p == nil {
		return nil
	}
	return &PasscodeModel{
		RequirePasscodeOnDevice:          boolPtrToTF(p.RequirePasscodeOnDevice),
		AllowSimpleValue:                 boolPtrToTF(p.AllowSimpleValue),
		RequireAlphanumericValue:         boolPtrToTF(p.RequireAlphanumericValue),
		MinimumPasscodeLength:            int64PtrToTF(p.MinimumPasscodeLength),
		MinimumNumberOfComplexCharacters: stringToTF(p.MinimumNumberOfComplexCharacters),
		MaximumPasscodeAge:               stringToTF(p.MaximumPasscodeAge),
		AutoLock:                         stringToTF(p.AutoLock),
		GracePeriod:                      int64PtrToTF(p.GracePeriod),
		MaxFailedAttempts:                intPtrToNumericString(p.MaxFailedAttempts),
		PinHistory:                       intPtrToNumericString(p.PinHistory),
		MinutesUntilFailedLoginReset:     int64PtrToTF(p.MinutesUntilFailedLoginReset),
	}
}

func mapCustomSettingsListAppleOsX(prior []CustomSettingsItemModel, items []sdk.AppleOsXCustomSettingsPayloadEntityV2) []CustomSettingsItemModel {
	if prior == nil && len(items) == 0 {
		return nil
	}
	result := make([]CustomSettingsItemModel, 0, len(items))
	for _, it := range items {
		if it.CustomSettings == "" {
			continue
		}
		result = append(result, CustomSettingsItemModel{CustomSettings: types.StringValue(it.CustomSettings)})
	}
	if len(result) == 0 {
		return make([]CustomSettingsItemModel, 0)
	}
	return result
}

func mapAppleOsXNetworkList(items []sdk.AppleOsXNetworkPayloadEntityV2) []NetworkItemModel {
	if len(items) == 0 {
		return nil
	}
	result := make([]NetworkItemModel, 0, len(items))
	for i := range items {
		n := &items[i]
		out := NetworkItemModel{TrustedCertificates: types.ListNull(types.StringType)}
		if n.NetworkInterface != "" {
			out.NetworkInterface = types.StringValue(n.NetworkInterface)
		}
		if n.ServiceSetIdentifier != "" {
			out.ServiceSetIdentifier = types.StringValue(n.ServiceSetIdentifier)
		}
		setTrueBool(&out.HiddenNetwork, n.HiddenNetwork)
		setTrueBool(&out.AutoJoin, n.AutoJoin)
		if n.SecurityType != "" {
			out.SecurityType = types.StringValue(n.SecurityType)
		}
		setTrueBool(&out.UseAsLoginWindowConfiguration, n.UseAsLoginWindowConfiguration)
		setTrueBool(&out.UseDirectoryAuthentication, n.UseDirectoryAuthentication)
		setTrueBool(&out.TLS, n.TLS)
		setTrueBool(&out.TTLS, n.TTLS)
		setTrueBool(&out.LEAP, n.LEAP)
		setTrueBool(&out.PEAP, n.PEAP)
		setTrueBool(&out.EAPFAST, n.EAPFAST)
		setTrueBool(&out.EAPSIM, n.EAPSIM)
		setTrueBool(&out.EAPAKA, n.EAPAKA)
		if n.TLSMinimumVersion != "" {
			out.TLSMinimumVersion = types.StringValue(n.TLSMinimumVersion)
		}
		if n.TLSMaximumVersion != "" {
			out.TLSMaximumVersion = types.StringValue(n.TLSMaximumVersion)
		}
		setTrueBool(&out.DisableAssociationMACRandomization, n.DisableAssociationMACRandomization)
		if n.UserName != "" {
			out.UserName = types.StringValue(n.UserName)
		}
		if n.IdentityCertificate != "" {
			out.IdentityCertificate = types.StringValue(n.IdentityCertificate)
		}
		if n.InnerIdentity != "" {
			out.InnerIdentity = types.StringValue(n.InnerIdentity)
		}
		if n.OuterIdentity != "" {
			out.OuterIdentity = types.StringValue(n.OuterIdentity)
		}
		// Password / UserPassword / ProxyPassword are write-only: they are
		// not read from UEM. carryNetworkPasswords puts back the user's
		// configured values from prior state after mapping.
		// Live-confirmed 2026-09-25 on the 26.2 lab tenant (uem_profile
		// AppleOsX network_list, WPA2 create): UEM does not echo the
		// configured PSK, and reading it made apply fail with
		// "inconsistent values for sensitive attribute". Evidence:
		// internal-design-doc
		setTrueBool(&out.UsePAC, n.UsePAC)
		setTrueBool(&out.AllowTwoRANDs, n.AllowTwoRANDs)
		if len(n.TrustedCertificates) > 0 {
			listVal, diags := types.ListValueFrom(context.Background(), types.StringType, n.TrustedCertificates)
			if !diags.HasError() {
				out.TrustedCertificates = listVal
			}
		}
		setTrueBool(&out.AllowTrustExceptions, n.AllowTrustExceptions)
		if n.ProxyType != "" {
			out.ProxyType = types.StringValue(n.ProxyType)
		}
		if n.ProxyServer != "" {
			out.ProxyServer = types.StringValue(n.ProxyServer)
		}
		// internal-ticket: a server-returned 0 is no longer treated as "unset"
		// (row #67 of the B16 audit).
		if n.ProxyServerPort != nil {
			out.ProxyServerPort = types.Int64Value(int64(*n.ProxyServerPort))
		}
		if n.ProxyUsername != "" {
			out.ProxyUsername = types.StringValue(n.ProxyUsername)
		}
		if n.ProxyURL != "" {
			out.ProxyUrl = types.StringValue(n.ProxyURL)
		}
		setTrueBool(&out.PacFallback, n.PacFallback)
		result = append(result, out)
	}
	return result
}

func mapAppleOsXCredentialsList(items []sdk.AppleOsXCredentialPayloadEntityV2) []CredentialItemModel {
	if len(items) == 0 {
		return nil
	}
	result := make([]CredentialItemModel, 0, len(items))
	for i := range items {
		c := &items[i]
		out := CredentialItemModel{}
		if c.CredentialSource != "" {
			out.CredentialSource = types.StringValue(c.CredentialSource)
		}
		// Two SDK fields map to credential_name: `CredentialName` is the
		// request-side / user-supplied label (Upload credentials), and `Name`
		// is UEM's auto-generated display name (e.g. "Certificate #1"
		// returned for DefinedCertificateAuthority). Prefer CredentialName so
		// Upload credentials keep the user's label, and fall back to Name for
		// DefinedCA where CredentialName comes back empty.
		switch {
		case c.CredentialName != "":
			out.CredentialName = types.StringValue(c.CredentialName)
		case c.Name != "":
			out.CredentialName = types.StringValue(c.Name)
		}
		if c.CertificateID != nil {
			out.CertificateID = types.Int64Value(int64(*c.CertificateID))
		}
		// internal-ticket: a server-returned 0 is no longer treated as "unset"
		// for CA/template (row #69 of the B16 audit).
		if c.CertificateAuthority != nil {
			out.CertificateAuthority = types.Int64Value(int64(*c.CertificateAuthority))
		}
		if c.CertificateTemplate != nil {
			out.CertificateTemplate = types.Int64Value(int64(*c.CertificateTemplate))
		}
		if c.AllowAccessToAllApplications != nil {
			out.AllowAccessToAllApplications = types.BoolValue(*c.AllowAccessToAllApplications)
		}
		if c.KeyIsExtractable != nil {
			out.KeyIsExtractable = types.BoolValue(*c.KeyIsExtractable)
		}
		result = append(result, out)
	}
	return result
}

func mapAppleOsXDiskEncryption(de *sdk.AppleOsXDiskEncryptionPayloadEntityV2) *DiskEncryptionModel {
	if de == nil {
		return nil
	}
	aw := mapAppleOsXDiskEncryptionAirWatch(de.DiskEncryptionAirWatch)
	fv := mapAppleOsXDiskEncryptionFileVault(de.DiskEncryptionFileVault2)
	mcx := mapAppleOsXDiskEncryptionMCX(de.DiskEncryptionMCX)
	if aw == nil && fv == nil && mcx.IsNull() {
		return nil
	}
	return &DiskEncryptionModel{AirWatch: aw, FileVault: fv, MCX: mcx}
}

// internal-ticket: no longer collapses an all-zero-fields struct to nil (row #70
// of the B16 audit); only a nil pointer maps to nil now.
func mapAppleOsXDiskEncryptionAirWatch(aw *sdk.AppleOsXDiskEncryptionAirWatchPayloadEntityV2) *DiskEncryptionAirWatchModel {
	if aw == nil {
		return nil
	}
	return &DiskEncryptionAirWatchModel{
		StoreKey:                                    boolPtrToTF(aw.StoreKey),
		RotateKeyAfter:                              int64PtrToTF(aw.RotateKeyAfter),
		UseIntelligentHub:                           boolPtrToTF(aw.UseIntelligentHub),
		NotifyUserForEncryption:                     boolPtrToTF(aw.NotifyUserForEncryption),
		EncryptionNotificationTitle:                 stringToTF(aw.EncryptionNotificationTitle),
		EncryptionNotificationMessage:               stringToTF(aw.EncryptionNotificationMessage),
		EncryptionMaxNotifyAttempts:                 int64PtrToTF(aw.EncryptionMaxNotifyAttempts),
		EncryptionNotificationRetryIntervalInHours:  int64PtrToTF(aw.EncryptionNotificationRetryIntervalInHours),
		EncryptionActionAfterLastNotification:       encryptionActionAfterLastNotificationToInt64TF(aw.EncryptionActionAfterLastNotification),
		EnableRecoveryKey:                           boolPtrToTF(aw.EnableRecoveryKey),
		RecoveryKeyNotificationTitle:                stringToTF(aw.RecoveryKeyNotificationTitle),
		RecoveryKeyNotificationMessage:              stringToTF(aw.RecoveryKeyNotificationMessage),
		RecoveryKeyNotificationRetryIntervalInHours: int64PtrToTF(aw.RecoveryKeyNotificationRetryIntervalInHours),
		RecoveryKeyPromptTitle:                      stringToTF(aw.RecoveryKeyPromptTitle),
		RecoveryKeyPromptMessage:                    stringToTF(aw.RecoveryKeyPromptMessage),
		RecoveryKeySuccessTitle:                     stringToTF(aw.RecoveryKeySuccessTitle),
		RecoveryKeySuccessMessage:                   stringToTF(aw.RecoveryKeySuccessMessage),
		RecoveryKeyErrorTitle:                       stringToTF(aw.RecoveryKeyErrorTitle),
		RecoveryKeyErrorMessage:                     stringToTF(aw.RecoveryKeyErrorMessage),
		RecoveryKeyMaxFailureCount:                  int64PtrToTF(aw.RecoveryKeyMaxFailureCount),
	}
}

// internal-ticket: no longer collapses an all-zero-fields struct to nil (row #70
// of the B16 audit); only a nil pointer maps to nil now.
func mapAppleOsXDiskEncryptionFileVault(fv *sdk.AppleOsXDiskEncryptionFileVault2PayloadEntityV2) *DiskEncryptionFileVaultModel {
	if fv == nil {
		return nil
	}
	return &DiskEncryptionFileVaultModel{
		Enable:                         boolPtrToTF(fv.Enable),
		ShowRecoveryKey:                boolPtrToTF(fv.ShowRecoveryKey),
		RecoveryType:                   int64PtrToTF(fv.RecoveryType),
		FileVaultEnterpriseCertificate: stringToTF(fv.FileVaultEnterpriseCertificate),
		FileVaultUser:                  fileVaultUserToInt64TF(fv.FileVaultUser),
		Username:                       stringToTF(fv.Username),
		PromptToEnableFileVaultAt:      promptToEnableFileVaultAtToInt64TF(fv.PromptToEnableFileVaultAt),
		NumberOfTimesUserCanBypass:     int64PtrToTF(fv.NumberOfTimesUserCanBypass),
	}
}

// mapAppleOsXDiskEncryptionMCX maps UEM's DiskEncryptionMCX into the mcx
// types.Object. A nil API value (UEM genuinely sent no MCX section) maps to
// ObjectNull. A non-nil API value always maps to a known ObjectValueMust,
// even when its own DestroyFVKeyOnStandby pointer is nil -- the leaf then
// maps to BoolNull, verbatim, rather than collapsing the whole object away
// (B42 follow-up: collapsing a present-but-partial object to nil is what
// let a genuinely unknown create-time plan get confused with an
// API-omitted section; the two are now kept distinct).
func mapAppleOsXDiskEncryptionMCX(mcx *sdk.AppleOsXDiskEncryptionMCXPayloadEntityV2) types.Object {
	if mcx == nil {
		return types.ObjectNull(profilemodels.DiskEncryptionMCXAttrTypes)
	}
	return types.ObjectValueMust(profilemodels.DiskEncryptionMCXAttrTypes, map[string]attr.Value{
		"destroy_fv_key_on_standby": boolPtrToTF(mcx.DestroyFVKeyOnStandby),
	})
}

// mapAppleOsXGatekeeper hydrates a GatekeeperModel from the parent value-typed
// SDK field. internal-ticket: no longer gated on appleOsXGatekeeperHasContent,
// and the caller no longer runs the result through
// MergeGatekeeperWithPriorState (both row #74 of the B16 audit: never
// observed live, no source). A nil pointer maps to nil; otherwise every
// field UEM returned is stored, including any server-side defaults for
// fields the caller never configured.
func mapAppleOsXGatekeeper(g *sdk.MacOsGatekeeperPayloadV2Entity) *GatekeeperModel {
	if g == nil {
		return nil
	}
	return &GatekeeperModel{
		AllowAutoUnlock:              boolPtrToTF(g.AllowAutoUnlock),
		AllowFingerprintForUnlock:    boolPtrToTF(g.AllowFingerprintForUnlock),
		AllowHandoff:                 boolPtrToTF(g.AllowHandoff),
		AllowScreenCapture:           boolPtrToTF(g.AllowScreenCapture),
		EnableAppSoftwareUpdateDelay: boolPtrToTF(g.EnableAppSoftwareUpdateDelay),
		EnableSoftwareUpdateDelay:    boolPtrToTF(g.EnableSoftwareUpdateDelay),
		EnforcedSoftwareUpdateDelay:  int64PtrToTF(g.EnforcedSoftwareUpdateDelay),
	}
}

// mapAppleOsXSystemExtensions hydrates a SystemExtensionsModel from the
// parent value-typed SDK field. Confirmed live 2026-09-25 (as<internal-env>):
// UEM populates this block once any system_extensions config is applied, and
// always injects a "*" (global team identifier) entry into
// AllowedSystemExtensionTypes representing the implicit deny-all-others
// default policy — even when the user never configured one. Prior carries
// the caller's own prior state/config so mapAllowedSystemExtensionTypes can
// tell a genuine user-configured "*" entry apart from that server default.
func mapAppleOsXSystemExtensions(s *sdk.MacOsSystemExtensionsPayloadV2Model, prior *SystemExtensionsModel) *SystemExtensionsModel {
	if s == nil || !appleOsXSystemExtensionsHasContent(s) {
		return nil
	}
	var priorTypes []AllowedSystemExtensionTypeModel
	if prior != nil {
		priorTypes = prior.AllowedSystemExtensionTypes
	}
	return &SystemExtensionsModel{
		AllowUserOverrides:          boolPtrToTF(s.AllowUserOverrides),
		AllowedSystemExtensionTypes: mapAllowedSystemExtensionTypes(s.AllowedSystemExtensionTypes, priorTypes),
		AllowedSystemExtensions:     mapAllowedSystemExtensions(s.AllowedSystemExtensions),
	}
}

func appleOsXSystemExtensionsHasContent(s *sdk.MacOsSystemExtensionsPayloadV2Model) bool {
	return s.AllowUserOverrides != nil ||
		len(s.AllowedSystemExtensionTypes) > 0 ||
		len(s.AllowedSystemExtensions) > 0
}

// mapAllowedSystemExtensionTypes drops UEM's server-synthesized "*" entry
// (TeamIdentifier "*" with every type denied, the implicit deny-all-others
// default UEM injects alongside any explicit team entry) unless prior already
// carries an explicit "*" entry of its own, i.e. the user configured one.
// Dropping it keeps state matching config so plans stay clean; keeping a
// user-configured one preserves round-trip fidelity for that entry. A "*"
// entry that allows any type is never the server default, so it is always
// kept: on import (no prior) dropping it would leave it out of the onboarded
// HCL, and the next update would then silently delete it from the profile.
func mapAllowedSystemExtensionTypes(items []sdk.MacOsAllowedSystemExtensionTypesV2Model, prior []AllowedSystemExtensionTypeModel) []AllowedSystemExtensionTypeModel {
	keepWildcard := hasExplicitWildcardTypeEntry(prior)
	result := make([]AllowedSystemExtensionTypeModel, 0, len(items))
	for i := range items {
		it := &items[i]
		if !keepWildcard && isServerDefaultWildcardType(it) {
			continue
		}
		result = append(result, AllowedSystemExtensionTypeModel{
			TeamIdentifier:                     stringToTF(it.TeamIdentifier),
			AllowDriverExtensionType:           boolPtrToTF(it.AllowDriverExtensionType),
			AllowEndpointSecurityExtensionType: boolPtrToTF(it.AllowEndpointSecurityExtensionType),
			AllowNetworkExtensionType:          boolPtrToTF(it.AllowNetworkExtensionType),
		})
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// isServerDefaultWildcardType reports whether it has the shape of UEM's
// synthetic entry: TeamIdentifier "*" and no extension type allowed.
func isServerDefaultWildcardType(it *sdk.MacOsAllowedSystemExtensionTypesV2Model) bool {
	return it.TeamIdentifier == "*" &&
		!boolPtrTrue(it.AllowDriverExtensionType) &&
		!boolPtrTrue(it.AllowEndpointSecurityExtensionType) &&
		!boolPtrTrue(it.AllowNetworkExtensionType)
}

func boolPtrTrue(b *bool) bool { return b != nil && *b }

func hasExplicitWildcardTypeEntry(items []AllowedSystemExtensionTypeModel) bool {
	for _, it := range items {
		if !it.TeamIdentifier.IsNull() && it.TeamIdentifier.ValueString() == "*" {
			return true
		}
	}
	return false
}

// mapAllowedSystemExtensions needs no wildcard-drop treatment: confirmed live
// 2026-09-25 (as<internal-env>) that UEM does NOT inject a synthetic entry into this
// list the way it does for AllowedSystemExtensionTypes -- it echoed back
// exactly the one entry that was configured, nothing more.
func mapAllowedSystemExtensions(items []sdk.MacOsAllowedSystemExtensionV2Model) []AllowedSystemExtensionModel {
	if len(items) == 0 {
		return nil
	}
	result := make([]AllowedSystemExtensionModel, 0, len(items))
	for i := range items {
		it := &items[i]
		result = append(result, AllowedSystemExtensionModel{
			BundleIdentifier: stringToTF(it.BundleIdentifier),
			TeamIdentifier:   stringToTF(it.TeamIdentifier),
		})
	}
	return result
}

func readAppleiOSIntoState(_ context.Context, data *ProfileResourceModel, ent *sdk.AppleDeviceProfileV2Entity) {
	readGeneralV2IntoState(data, ent.General)
	data.Passcode = mapAppleiOSPasscode(ent.Passcode)
	data.CustomSettingsList = mapCustomSettingsListAppleiOS(data.CustomSettingsList, ent.CustomSettingsList)
}

// internal-ticket: see mapAppleOsXPasscode's doc comment -- the same
// hasContent-gate and prior-state merge removal (rows #62, #63) applies
// here for iOS.
func mapAppleiOSPasscode(p *sdk.ApplePasscodePayloadV2Entity) *PasscodeModel {
	if p == nil {
		return nil
	}
	return &PasscodeModel{
		RequirePasscodeOnDevice:          boolPtrToTF(p.RequirePasscodeOnDevice),
		AllowSimpleValue:                 boolPtrToTF(p.AllowSimpleValue),
		RequireAlphanumericValue:         boolPtrToTF(p.RequireAlphanumericValue),
		MinimumPasscodeLength:            int64PtrToTF(p.MinimumPasscodeLength),
		MinimumNumberOfComplexCharacters: intPtrToNumericString(p.MinimumNumberOfComplexCharacters),
		MaximumPasscodeAge:               stringToTF(p.MaximumPasscodeAge),
		AutoLock:                         stringToTF(p.AutoLock),
		GracePeriod:                      int64PtrToTF(p.GracePeriodForDeviceLock),
		MaxFailedAttempts:                stringToTF(p.MaximumNumberOfFailedAttempts),
		PinHistory:                       stringToTF(p.PasscodeHistory),
		MinutesUntilFailedLoginReset:     types.Int64Null(),
	}
}

func mapCustomSettingsListAppleiOS(prior []CustomSettingsItemModel, items []sdk.AppleCustomSettingsPayloadV2Entity) []CustomSettingsItemModel {
	if prior == nil && len(items) == 0 {
		return nil
	}
	result := make([]CustomSettingsItemModel, 0, len(items))
	for _, it := range items {
		if it.CustomSettings == "" {
			continue
		}
		result = append(result, CustomSettingsItemModel{CustomSettings: types.StringValue(it.CustomSettings)})
	}
	if len(result) == 0 {
		return make([]CustomSettingsItemModel, 0)
	}
	return result
}

func readAndroidIntoState(_ context.Context, data *ProfileResourceModel, ent *sdk.AndroidDeviceProfileV2Entity) {
	readGeneralV2IntoState(data, ent.General)
	if ent.AndroidForWorkCustomMessages != nil && ent.AndroidForWorkCustomMessages.LockScreenMessage != "" {
		data.LockScreenMessage = types.StringValue(ent.AndroidForWorkCustomMessages.LockScreenMessage)
	}
	data.CustomSettingsList = mapCustomSettingsListAndroid(data.CustomSettingsList, ent.CustomSettingsList)
}

func mapCustomSettingsListAndroid(prior []CustomSettingsItemModel, items []sdk.AndroidCustomSettingsPayloadV2Entity) []CustomSettingsItemModel {
	if prior == nil && len(items) == 0 {
		return nil
	}
	result := make([]CustomSettingsItemModel, 0, len(items))
	for _, it := range items {
		if it.CustomSettings == "" {
			continue
		}
		result = append(result, CustomSettingsItemModel{CustomSettings: types.StringValue(it.CustomSettings)})
	}
	if len(result) == 0 {
		return make([]CustomSettingsItemModel, 0)
	}
	return result
}

func boolPtrToTF(p *bool) types.Bool {
	if p == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*p)
}

func int64PtrToTF(p *int) types.Int64 {
	if p == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*p))
}

func stringToTF(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

func intPtrToNumericString(p *int) types.String {
	if p == nil {
		return types.StringNull()
	}
	return types.StringValue(strconv.Itoa(*p))
}

// fileVaultUserNumbers is the read-side inverse of the builder's
// fileVaultUserNames map (platform/builders.go): the complete set of
// MacOsFileVaultUser enum name strings UEM's GET response returns (canonical
// rules Q3/Q4, StringEnumConverter), each mapped back to the schema's Int64
// encoding.
var fileVaultUserNumbers = map[string]int64{
	"CurrentOrNextLoginUser": 1,
	"SpecificUser":           2,
}

// promptToEnableFileVaultAtNumbers is the read-side inverse of the
// builder's promptToEnableFileVaultAtNames map: the complete set of
// MacOsTimeOfPromptToEnableFileVault enum name strings.
var promptToEnableFileVaultAtNumbers = map[string]int64{
	"BothLoginAndLogout": 1,
	"LogoutOnly":         2,
	"LoginOnly":          3,
}

// encryptionActionAfterLastNotificationNumbers is the read-side inverse of
// the builder's encryptionActionAfterLastNotificationNames map: the complete
// set of MacOsEncryptionAction enum name strings.
var encryptionActionAfterLastNotificationNumbers = map[string]int64{
	"ForceLogout": 1,
	"DoNothing":   2,
}

// enumNameToInt64TF is a reader for the three StringEnumConverter-backed
// FileVault2/AirWatch fields (filevault_user, prompt_to_enable_filevault_at,
// encryption_action_after_last_notification). UEM's GET response returns the
// enum name string (e.g. "SpecificUser"), matched via names; an unmapped or
// unrecognized name maps to null. internal-ticket: no longer also accepts a raw
// numeric string (row #73 of the B16 audit: the tolerant fallback was for
// defensive/pre-fix stored state, not confirmed live UEM behavior).
func enumNameToInt64TF(s string, names map[string]int64) types.Int64 {
	if n, ok := names[s]; ok {
		return types.Int64Value(n)
	}
	return types.Int64Null()
}

// fileVaultUserToInt64TF maps disk_encryption.filevault2.filevault_user from
// one of the SDK's live-validated MacOsFileVaultUser enum name strings. Any
// other value maps to null.
func fileVaultUserToInt64TF(s string) types.Int64 {
	return enumNameToInt64TF(s, fileVaultUserNumbers)
}

// promptToEnableFileVaultAtToInt64TF maps
// disk_encryption.filevault2.prompt_to_enable_filevault_at from one of the
// SDK's live-validated MacOsTimeOfPromptToEnableFileVault enum name strings.
// Any other value maps to null.
func promptToEnableFileVaultAtToInt64TF(s string) types.Int64 {
	return enumNameToInt64TF(s, promptToEnableFileVaultAtNumbers)
}

// encryptionActionAfterLastNotificationToInt64TF maps
// disk_encryption.airwatch.encryption_action_after_last_notification from
// one of the SDK's live-validated MacOsEncryptionAction enum name strings.
// Any other value maps to null.
func encryptionActionAfterLastNotificationToInt64TF(s string) types.Int64 {
	return enumNameToInt64TF(s, encryptionActionAfterLastNotificationNumbers)
}

// setTrueBool writes the API-supplied boolean into state when present.
// Despite the legacy name, it round-trips both true and false so that
// fields the API explicitly returns as false are reflected in state
// (preventing permanent diffs when config sets the field to false).
// A nil src is left untouched (caller's zero/null value is preserved).
func setTrueBool(dst *types.Bool, src *bool) {
	if src != nil {
		*dst = types.BoolValue(*src)
	}
}

// ----- Restrictions (macOS) -----

// mapAppleOsXRestrictions and every sub-mapper below it in this file map a
// nil pointer to nil, and otherwise return a populated struct -- even one
// whose fields are all null/zero. internal-ticket removed the "collapse an
// empty/all-zero result back to nil" post-hoc checks that used to run after
// mapping (row #90 of the B16 audit: never observed live that UEM omits a
// sub-block whose pointer it sends, no source), so block presence is now
// determined solely by whether UEM's response included the pointer. The
// Widgets sub-mapper is the one exception in this tree: it keeps its
// empty-collapse check, which is (a)-confirmed by GGS:44 and out of scope
// here.
func mapAppleOsXRestrictions(r *sdk.AppleOsXRestrictionsPayloadEntityV2) *profilemodels.RestrictionsModel {
	if r == nil {
		return nil
	}
	return &profilemodels.RestrictionsModel{
		Applications:  mapAppleOsXRestrictionsApplications(r.Applications),
		Desktop:       mapAppleOsXRestrictionsDesktop(r.Desktop),
		Functionality: mapAppleOsXRestrictionsFunctionality(r.Functionality),
		Media:         mapAppleOsXRestrictionsMedia(r.Media),
		Preferences:   mapAppleOsXRestrictionsPreferences(r.Preferences),
		Sharing:       mapAppleOsXRestrictionsSharing(r.Sharing),
		Widgets:       mapAppleOsXRestrictionsWidgets(r.Widgets),
	}
}

func mapAppleOsXRestrictionsApplications(a *sdk.AppleOsXRestrictionApplicationsPayloadEntityV2) *profilemodels.RestrictionsApplicationsModel {
	if a == nil {
		return nil
	}
	return &profilemodels.RestrictionsApplicationsModel{
		AllowApplication: stringSliceToTFList(a.AllowApplication),
		AllowFolders:     stringSliceToTFList(a.AllowFolders),
		DisallowFolders:  stringSliceToTFList(a.DisallowFolders),
		RestrictWhichApplicationsAreAllowedToLaunch: boolPtrToTF(a.RestrictWhichApplicationsAreAllowedToLaunch),
		AppStore:   mapAppleOsXRestrictionsAppStore(a.AppStore),
		AppleMusic: mapAppleOsXRestrictionsAppleMusic(a.AppleMusic),
		Camera:     mapAppleOsXRestrictionsCamera(a.Camera),
		GameCentre: mapAppleOsXRestrictionsGameCentre(a.GameCentre),
		Safari:     mapAppleOsXRestrictionsSafari(a.Safari),
	}
}

func mapAppleOsXRestrictionsAppStore(a *sdk.AppleOsXRestrictionAppStorePayloadEntityV2) *profilemodels.RestrictionsAppStoreModel {
	if a == nil {
		return nil
	}
	return &profilemodels.RestrictionsAppStoreModel{
		AllowAppStoreAppAdoption:                 boolPtrToTF(a.AllowAppStoreAppAdoption),
		RequireAdminPasswordToInstallOrUpdateApp: boolPtrToTF(a.RequireAdminPasswordToInstallOrUpdateApp),
		RestrictAppStoreToSoftwareUpdatesOnly:    boolPtrToTF(a.RestrictAppStoreToSoftwareUpdatesOnly),
	}
}

func mapAppleOsXRestrictionsAppleMusic(a *sdk.AppleOsXRestrictionAppleMusicPayloadEntityV2) *profilemodels.RestrictionsAppleMusicModel {
	if a == nil {
		return nil
	}
	return &profilemodels.RestrictionsAppleMusicModel{
		AllowMusicService: boolPtrToTF(a.AllowMusicService),
	}
}

func mapAppleOsXRestrictionsCamera(a *sdk.AppleOsXRestrictionCameraPayloadEntityV2) *profilemodels.RestrictionsCameraModel {
	if a == nil {
		return nil
	}
	return &profilemodels.RestrictionsCameraModel{
		AllowUseOfBuiltInCamera: boolPtrToTF(a.AllowUseOfBuiltInCamera),
	}
}

func mapAppleOsXRestrictionsGameCentre(a *sdk.AppleOsXRestrictionGameCentrePayloadEntityV2) *profilemodels.RestrictionsGameCentreModel {
	if a == nil {
		return nil
	}
	return &profilemodels.RestrictionsGameCentreModel{
		AllowAddingGameCenterFriends: boolPtrToTF(a.AllowAddingGameCenterFriends),
		AllowGameCenterModification:  boolPtrToTF(a.AllowGameCenterModification),
		AllowMultiplayerGaming:       boolPtrToTF(a.AllowMultiplayerGaming),
		AllowUseOfGameCenter:         boolPtrToTF(a.AllowUseOfGameCenter),
	}
}

func mapAppleOsXRestrictionsSafari(a *sdk.AppleOsXRestrictionSafariPayloadEntityV2) *profilemodels.RestrictionsSafariModel {
	if a == nil {
		return nil
	}
	return &profilemodels.RestrictionsSafariModel{
		AllowDeprecatedWebKitTls: boolPtrToTF(a.AllowDeprecatedWebKitTls),
		AllowSafariAutoFill:      boolPtrToTF(a.AllowSafariAutoFill),
	}
}

func mapAppleOsXRestrictionsDesktop(a *sdk.AppleOsXRestrictionDesktopPayloadEntityV2) *profilemodels.RestrictionsDesktopModel {
	if a == nil {
		return nil
	}
	return &profilemodels.RestrictionsDesktopModel{
		DesktopPicturePath: stringToTF(a.DesktopPicturePath),
		LockDesktopPicture: boolPtrToTF(a.LockDesktopPicture),
	}
}

func mapAppleOsXRestrictionsFunctionality(a *sdk.AppleOsXRestrictionFunctionalityPayloadEntityV2) *profilemodels.RestrictionsFunctionalityModel {
	if a == nil {
		return nil
	}
	return &profilemodels.RestrictionsFunctionalityModel{
		AirPrint:       mapAppleOsXRestrictionsAirPrint(a.AirPrint),
		ContentCaching: mapAppleOsXRestrictionsContentCaching(a.ContentCaching),
		ICloud:         mapAppleOsXRestrictionsICloud(a.ICloud),
		Passwords:      mapAppleOsXRestrictionsPasswords(a.Passwords),
		Spotlight:      mapAppleOsXRestrictionsSpotlight(a.Spotlight),
	}
}

func mapAppleOsXRestrictionsAirPrint(a *sdk.AppleOsXRestrictionAirPrintPayloadEntityV2) *profilemodels.RestrictionsAirPrintModel {
	if a == nil {
		return nil
	}
	return &profilemodels.RestrictionsAirPrintModel{
		AllowAirPrint:                      boolPtrToTF(a.AllowAirPrint),
		AllowAirPrintiBeaconDiscovery:      boolPtrToTF(a.AllowAirPrintiBeaconDiscovery),
		ForceAirPrintTrustedTLSRequirement: boolPtrToTF(a.ForceAirPrintTrustedTLSRequirement),
	}
}

func mapAppleOsXRestrictionsContentCaching(a *sdk.AppleOsXRestrictionContentCachingPayloadEntityV2) *profilemodels.RestrictionsContentCachingModel {
	if a == nil {
		return nil
	}
	return &profilemodels.RestrictionsContentCachingModel{
		AllowContentCaching: boolPtrToTF(a.AllowContentCaching),
	}
}

func mapAppleOsXRestrictionsICloud(a *sdk.AppleOsXRestrictionICloudPayloadEntityV2) *profilemodels.RestrictionsICloudModel {
	if a == nil {
		return nil
	}
	return &profilemodels.RestrictionsICloudModel{
		AllowAirPrint:                          boolPtrToTF(a.AllowAirPrint),
		AllowAirPrintiBeaconDiscovery:          boolPtrToTF(a.AllowAirPrintiBeaconDiscovery),
		AllowCloudDesktopAndDocuments:          boolPtrToTF(a.AllowCloudDesktopAndDocuments),
		AllowDeprecatedWebKitTls:               boolPtrToTF(a.AllowDeprecatedWebKitTls),
		AllowICloudFMM:                         boolPtrToTF(a.AllowICloudFMM),
		AllowIcloudAddressBook:                 boolPtrToTF(a.AllowIcloudAddressBook),
		AllowIcloudBTMM:                        boolPtrToTF(a.AllowIcloudBTMM),
		AllowIcloudBookmarks:                   boolPtrToTF(a.AllowIcloudBookmarks),
		AllowIcloudCalendar:                    boolPtrToTF(a.AllowIcloudCalendar),
		AllowIcloudDocumentsAndData:            boolPtrToTF(a.AllowIcloudDocumentsAndData),
		AllowIcloudKeychainSync:                boolPtrToTF(a.AllowIcloudKeychainSync),
		AllowIcloudMail:                        boolPtrToTF(a.AllowIcloudMail),
		AllowIcloudNotes:                       boolPtrToTF(a.AllowIcloudNotes),
		AllowIcloudReminders:                   boolPtrToTF(a.AllowIcloudReminders),
		AllowPasswordAutoFill:                  boolPtrToTF(a.AllowPasswordAutoFill),
		AllowPasswordProximityRequests:         boolPtrToTF(a.AllowPasswordProximityRequests),
		AllowPasswordSharing:                   boolPtrToTF(a.AllowPasswordSharing),
		AllowUseIcloudPasswordForLocalAccounts: boolPtrToTF(a.AllowUseIcloudPasswordForLocalAccounts),
		ForceAirPrintTrustedTLSRequirement:     boolPtrToTF(a.ForceAirPrintTrustedTLSRequirement),
	}
}

func mapAppleOsXRestrictionsPasswords(a *sdk.AppleOsXRestrictionPasswordsPayloadEntityV2) *profilemodels.RestrictionsPasswordsModel {
	if a == nil {
		return nil
	}
	return &profilemodels.RestrictionsPasswordsModel{
		AllowPasswordAutoFill:          boolPtrToTF(a.AllowPasswordAutoFill),
		AllowPasswordProximityRequests: boolPtrToTF(a.AllowPasswordProximityRequests),
		AllowPasswordSharing:           boolPtrToTF(a.AllowPasswordSharing),
	}
}

func mapAppleOsXRestrictionsSpotlight(a *sdk.AppleOsXRestrictionSpotlightPayloadEntityV2) *profilemodels.RestrictionsSpotlightModel {
	if a == nil {
		return nil
	}
	return &profilemodels.RestrictionsSpotlightModel{
		AllowSpotlightSuggestions: boolPtrToTF(a.AllowSpotlightSuggestions),
	}
}

func mapAppleOsXMediaAccess(a *sdk.AppleOsXMediaAccessEntityV2) *profilemodels.RestrictionsMediaAccessModel {
	if a == nil {
		return nil
	}
	return &profilemodels.RestrictionsMediaAccessModel{
		Allow:        boolPtrToTF(a.Allow),
		Authenticate: boolPtrToTF(a.Authenticate),
		ReadOnly:     boolPtrToTF(a.ReadOnly),
	}
}

func mapAppleOsXRestrictionsMedia(a *sdk.AppleOsXRestrictionMediaPayloadEntityV2) *profilemodels.RestrictionsMediaModel {
	if a == nil {
		return nil
	}
	m := &profilemodels.RestrictionsMediaModel{
		AutoEjectMedia:              boolPtrToTF(a.AutoEjectMedia),
		DiskMediaCDs:                mapAppleOsXMediaAccess(a.DiskMediaCDs),
		DiskMediaDVDs:               mapAppleOsXMediaAccess(a.DiskMediaDVDs),
		ExternalHardDiskMediaAccess: mapAppleOsXMediaAccess(a.ExternalHardDiskMediaAccess),
		HardDiskDvdRam:              mapAppleOsXMediaAccess(a.HardDiskDvdRam),
		HardDiskImages:              mapAppleOsXMediaAccess(a.HardDiskImages),
		InternalHardDiskMediaAccess: mapAppleOsXMediaAccess(a.InternalHardDiskMediaAccess),
	}
	if a.NetworkAccess != nil {
		m.NetworkAccess = &profilemodels.RestrictionsNetworkAccessModel{
			AirDrop: boolPtrToTF(a.NetworkAccess.AirDrop),
		}
	}
	if a.RecordableDisc != nil {
		m.RecordableDisc = &profilemodels.RestrictionsBurnSupportModel{
			BurnSupport: mapAppleOsXMediaAccess(a.RecordableDisc.BurnSupport),
		}
	}
	return m
}

func mapAppleOsXRestrictionsPreferences(a *sdk.AppleOsXRestrictionPreferencesPayloadEntityV2) *profilemodels.RestrictionsPreferencesModel {
	if a == nil {
		return nil
	}
	m := &profilemodels.RestrictionsPreferencesModel{
		Accessibility:          boolPtrToTF(a.Accessibility),
		AppStore:               boolPtrToTF(a.AppStore),
		Bluetooth:              boolPtrToTF(a.Bluetooth),
		CDsAndDVDs:             boolPtrToTF(a.CDsAndDVDs),
		DateAndTime:            boolPtrToTF(a.DateAndTime),
		DesktopAndScreenSaver:  boolPtrToTF(a.DesktopAndScreenSaver),
		DictationAndSpeech:     boolPtrToTF(a.DictationAndSpeech),
		Displays:               boolPtrToTF(a.Displays),
		Dock:                   boolPtrToTF(a.Dock),
		EnabledPreferencePanes: boolPtrToTF(a.EnabledPreferencePanes),
		EnergySaver:            boolPtrToTF(a.EnergySaver),
		Extensions:             boolPtrToTF(a.Extensions),
		FibreChannel:           boolPtrToTF(a.FibreChannel),
		FlashPlayer:            boolPtrToTF(a.FlashPlayer),
		General:                boolPtrToTF(a.General),
		Ink:                    boolPtrToTF(a.Ink),
		InternetAccounts:       boolPtrToTF(a.InternetAccounts),
		Keyboard:               boolPtrToTF(a.Keyboard),
		LanguageAndText:        boolPtrToTF(a.LanguageAndText),
		MissionControl:         boolPtrToTF(a.MissionControl),
		MobileMe:               boolPtrToTF(a.MobileMe),
		Mouse:                  boolPtrToTF(a.Mouse),
		Network:                boolPtrToTF(a.Network),
		Notifications:          boolPtrToTF(a.Notifications),
		ParentalControls:       boolPtrToTF(a.ParentalControls),
		PreferenceBehavior:     stringToTF(a.PreferenceBehavior),
		PrintAndScan:           boolPtrToTF(a.PrintAndScan),
		Profiles:               boolPtrToTF(a.Profiles),
		SecurityAndPrivacy:     boolPtrToTF(a.SecurityAndPrivacy),
		Sharing:                boolPtrToTF(a.Sharing),
		SoftwareUpdate:         boolPtrToTF(a.SoftwareUpdate),
		Sound:                  boolPtrToTF(a.Sound),
		Spotlight:              boolPtrToTF(a.Spotlight),
		StartupDisk:            boolPtrToTF(a.StartupDisk),
		TimeMachine:            boolPtrToTF(a.TimeMachine),
		Trackpad:               boolPtrToTF(a.Trackpad),
		UsersAndGroups:         boolPtrToTF(a.UsersAndGroups),
		Xsan:                   boolPtrToTF(a.Xsan),
		ICloud:                 boolPtrToTF(a.ICloud),
	}
	return m
}

func mapAppleOsXRestrictionsSharing(a *sdk.AppleOsXRestrictionSharingPayloadEntityV2) *profilemodels.RestrictionsSharingModel {
	if a == nil {
		return nil
	}
	return &profilemodels.RestrictionsSharingModel{
		AddtoAperture:                          boolPtrToTF(a.AddtoAperture),
		AddtoReadingList:                       boolPtrToTF(a.AddtoReadingList),
		AddtoiPhoto:                            boolPtrToTF(a.AddtoiPhoto),
		AirDrop:                                boolPtrToTF(a.AirDrop),
		AutomaticallyEnableNewSharingServices:  boolPtrToTF(a.AutomaticallyEnableNewSharingServices),
		Facebook:                               boolPtrToTF(a.Facebook),
		Mail:                                   boolPtrToTF(a.Mail),
		Messages:                               boolPtrToTF(a.Messages),
		RestrictWhichSharingServicesAreEnabled: boolPtrToTF(a.RestrictWhichSharingServicesAreEnabled),
		SinaWeibo:                              boolPtrToTF(a.SinaWeibo),
		Twitter:                                boolPtrToTF(a.Twitter),
		VideoServices:                          boolPtrToTF(a.VideoServices),
	}
}

// mapAppleOsXPrivacyPreferences hydrates the PrivacyPreferences (PPPC) list
// from the parent value-typed SDK field. Identities is already omitempty on
// the wire, so an absent block and an empty list are the same nil case --
// unlike GateKeeper/SystemExtensions there is no separate "hasContent" gate
// needed here.
func mapAppleOsXPrivacyPreferences(p *sdk.MacOsPrivacyPreferencesPayloadV2Model) []PrivacyPreferenceModel {
	if p == nil || len(p.Identities) == 0 {
		return nil
	}
	result := make([]PrivacyPreferenceModel, 0, len(p.Identities))
	for i := range p.Identities {
		result = append(result, mapPrivacyPreferenceIdentity(&p.Identities[i]))
	}
	return result
}

func mapPrivacyPreferenceIdentity(it *sdk.MacOsPrivacyPreferencesV2Model) PrivacyPreferenceModel {
	return PrivacyPreferenceModel{
		Identifier:                   stringToTF(it.Identifier),
		IdentifierType:               stringToTF(it.IdentifierType),
		CodeRequirement:              stringToTF(it.CodeRequirement),
		Comment:                      stringToTF(it.Comment),
		AppleEventsList:              mapAppleEventsList(it.AppleEventsList),
		StaticCode:                   boolPtrToTF(it.StaticCode),
		Accessibility:                stringToTF(it.Accessibility),
		AddressBook:                  stringToTF(it.AddressBook),
		Calendar:                     stringToTF(it.Calendar),
		Camera:                       stringToTF(it.Camera),
		FileProviderPresence:         stringToTF(it.FileProviderPresence),
		ListenEvent:                  stringToTF(it.ListenEvent),
		MediaLibrary:                 stringToTF(it.MediaLibrary),
		Microphone:                   stringToTF(it.Microphone),
		Photos:                       stringToTF(it.Photos),
		PostEvent:                    stringToTF(it.PostEvent),
		Reminders:                    stringToTF(it.Reminders),
		ScreenCapture:                stringToTF(it.ScreenCapture),
		SpeechRecognition:            stringToTF(it.SpeechRecognition),
		SystemPolicyAllFiles:         stringToTF(it.SystemPolicyAllFiles),
		SystemPolicyDesktopFolder:    stringToTF(it.SystemPolicyDesktopFolder),
		SystemPolicyDocumentsFolder:  stringToTF(it.SystemPolicyDocumentsFolder),
		SystemPolicyDownloadsFolder:  stringToTF(it.SystemPolicyDownloadsFolder),
		SystemPolicyNetworkVolumes:   stringToTF(it.SystemPolicyNetworkVolumes),
		SystemPolicyRemovableVolumes: stringToTF(it.SystemPolicyRemovableVolumes),
		SystemPolicySysAdminFiles:    stringToTF(it.SystemPolicySysAdminFiles),
	}
}

func mapAppleEventsList(items []sdk.AppleEventV2) []AppleEventModel {
	if len(items) == 0 {
		return nil
	}
	result := make([]AppleEventModel, 0, len(items))
	for i := range items {
		it := &items[i]
		result = append(result, AppleEventModel{
			CodeRequirement: stringToTF(it.CodeRequirement),
			Identifier:      stringToTF(it.Identifier),
			IdentifierType:  stringToTF(it.IdentifierType),
			Permission:      stringToTF(it.Permission),
		})
	}
	return result
}

func mapAppleOsXRestrictionsWidgets(a *sdk.AppleOsXRestrictionWidgetPayloadEntityV2) *profilemodels.RestrictionsWidgetsModel {
	if a == nil {
		return nil
	}
	m := &profilemodels.RestrictionsWidgetsModel{
		AllowOnlyConfiguredWidgets: boolPtrToTF(a.AllowOnlyConfiguredWidgets),
		AllowedWidgets:             stringSliceToTFList(a.AllowedWidgets),
	}
	if m.AllowOnlyConfiguredWidgets.IsNull() && m.AllowedWidgets.IsNull() {
		return nil
	}
	return m
}

// carryNetworkPasswords copies the write-only network secrets (password,
// user_password, proxy_password) from prior state into the freshly mapped
// list, matched by list position, because UEM never returns them (see the
// evidence note in mapAppleOsXNetworkList). Only these three fields are
// carried; every other network field is stored exactly as UEM returns it.
// With no prior state (import) they stay null, so imported config omits
// them and plans clean.
func carryNetworkPasswords(apiList, prior []NetworkItemModel) []NetworkItemModel {
	for i := range apiList {
		if i >= len(prior) {
			break
		}
		// knownOrNull: on create the prior is the plan, where an unset
		// (Computed) password is unknown.
		apiList[i].Password = knownOrNull(prior[i].Password)
		apiList[i].UserPassword = knownOrNull(prior[i].UserPassword)
		apiList[i].ProxyPassword = knownOrNull(prior[i].ProxyPassword)
	}
	return apiList
}

// carryCredentialSecrets copies the write-only credential secrets
// (certificate_payload, certificate_password) from prior state, because UEM
// never returns them at all. It matches by credential_name first and falls
// back to list position. Every other field -- including
// certificate_authority, certificate_template,
// allow_access_to_all_applications and key_is_extractable -- is stored
// exactly as UEM returns it: UEM does echo those four (live-confirmed
// 2026-09-25: 0, 0, false, true when unset), so per the project's
// faithfulness rule (carry only what UEM does not echo; everything else is
// read back as-is) they belong in the schema's own Optional+Computed
// UseStateForUnknown handling (resource.go), not in this carry function.
// Carrying them here would hide real drift on those four fields. With no
// prior state (import) the two secrets stay null.
//
// Live-confirmed 2026-09-25 on the 26.2 lab tenant (uem_profile AppleOsX
// create with one Upload credential): apply failed with
// "credentials_list: inconsistent values for sensitive attribute" while the
// secrets were not carried. Evidence: the guarded B16 follow-up live
// run on 2026-09-25 (see the B16 gate records).
func carryCredentialSecrets(apiList, prior []CredentialItemModel) []CredentialItemModel {
	byName := make(map[string]CredentialItemModel, len(prior))
	for _, p := range prior {
		if key, ok := CredentialKey(p); ok {
			byName[key] = p
		}
	}
	for i := range apiList {
		p, ok := CredentialItemModel{}, false
		if key, keyed := CredentialKey(apiList[i]); keyed {
			p, ok = byName[key]
		}
		if !ok && i < len(prior) {
			p, ok = prior[i], true
		}
		if !ok {
			continue
		}
		apiList[i].CertificatePayload = p.CertificatePayload
		apiList[i].CertificatePassword = p.CertificatePassword
	}
	return apiList
}

// mapAppleOsXScepList hydrates scep_list from UEM's read, field for field.
func mapAppleOsXScepList(items []sdk.AppleOsXScepPayloadEntityV2) []profilemodels.ScepItemModel {
	if len(items) == 0 {
		return nil
	}
	result := make([]profilemodels.ScepItemModel, 0, len(items))
	for i := range items {
		it := &items[i]
		out := profilemodels.ScepItemModel{
			Name:                    stringToTF(it.Name),
			CredentialSource:        stringToTF(it.CredentialSource),
			CertificateAuthorityID:  int64PtrToTF(it.CertificateAuthorityID),
			CertificateTemplateID:   int64PtrToTF(it.CertificateTemplateID),
			AllowExportFromKeyChain: boolPtrToTF(it.AllowExportFromKeyChain),
		}
		if it.IdentityPreference != nil && len(it.IdentityPreference.Names) > 0 {
			out.IdentityPreference = &profilemodels.ScepIdentityPreferenceModel{Names: stringSliceToTFList(it.IdentityPreference.Names)}
		}
		result = append(result, out)
	}
	return result
}

// mapAppleOsXWebClipsList hydrates web_clips_list from UEM's read, field for
// field.
func mapAppleOsXWebClipsList(items []sdk.MacOsWebClipsPayloadV2Entity) []profilemodels.WebClipItemModel {
	if len(items) == 0 {
		return nil
	}
	result := make([]profilemodels.WebClipItemModel, 0, len(items))
	for i := range items {
		it := &items[i]
		result = append(result, profilemodels.WebClipItemModel{
			Label:            stringToTF(it.Label),
			URL:              stringToTF(it.URL),
			ShowInAppCatalog: boolPtrToTF(it.ShowInAppCatalog),
			Icon:             int64PtrToTF(it.Icon),
		})
	}
	return result
}
