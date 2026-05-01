package state

import (
	"context"
	"strconv"

	sdk "github.com/euc-oss/terraform-sdk-uem"
	"github.com/hashicorp/terraform-plugin-framework/types"

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
type DiskEncryptionMCXModel = profilemodels.DiskEncryptionMCXModel
type GatekeeperModel = profilemodels.GatekeeperModel

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
		readGeneralV4IntoState(data, result.Linux.General)
	}
}

func readGeneralV2IntoState(data *ProfileResourceModel, g *sdk.GeneralPayloadV2Entity) {
	if g == nil {
		return
	}
	data.Name = types.StringValue(g.Name)
	data.Description = types.StringValue(g.Description)
	if g.AssignmentType != "" {
		data.AssignmentType = types.StringValue(g.AssignmentType)
	}
	if g.ProfileScope != "" {
		data.ProfileScope = types.StringValue(g.ProfileScope)
	}
	if g.IsActive != nil {
		data.IsActive = types.BoolValue(*g.IsActive)
	}
	if g.ProfileUUID != "" {
		data.UUID = types.StringValue(g.ProfileUUID)
	}
	if g.ProfileContext != "" && IsValidProfileContext(g.ProfileContext) {
		data.ProfileContext = types.StringValue(g.ProfileContext)
	}
	if g.ManagedLocationGroupID != nil {
		data.OrgGroupID = types.StringValue(strconv.Itoa(*g.ManagedLocationGroupID))
	}
	// Map assigned/excluded smart groups from API to state
	data.AssignedSmartGroups = nil
	if len(g.AssignedSmartGroups) > 0 {
		data.AssignedSmartGroups = make([]types.String, 0, len(g.AssignedSmartGroups))
		for _, sg := range g.AssignedSmartGroups {
			if sg.SmartGroupID != nil {
				data.AssignedSmartGroups = append(data.AssignedSmartGroups, types.StringValue(strconv.Itoa(*sg.SmartGroupID)))
			}
		}
	}
	data.ExcludedSmartGroups = nil
	if len(g.ExcludedSmartGroups) > 0 {
		data.ExcludedSmartGroups = make([]types.String, 0, len(g.ExcludedSmartGroups))
		for _, sg := range g.ExcludedSmartGroups {
			if sg.SmartGroupID != nil {
				data.ExcludedSmartGroups = append(data.ExcludedSmartGroups, types.StringValue(strconv.Itoa(*sg.SmartGroupID)))
			}
		}
	}
}

func readGeneralV4IntoState(data *ProfileResourceModel, g *sdk.GeneralPayloadV4Entity) {
	if g == nil {
		return
	}
	data.Name = types.StringValue(g.Name)
	data.Description = types.StringValue(g.Description)
	if g.AssignmentType != "" {
		data.AssignmentType = types.StringValue(g.AssignmentType)
	}
	if g.ProfileScope != "" {
		data.ProfileScope = types.StringValue(g.ProfileScope)
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

func readAppleOsXIntoState(ctx context.Context, data *ProfileResourceModel, ent *sdk.AppleOsXDeviceProfileEntityV2) {
	readGeneralV2IntoState(data, ent.General)

	priorPasscode := data.Passcode
	data.Passcode = mapAppleOsXPasscode(ent.Passcode)
	mergePasscodeWithPriorState(data.Passcode, priorPasscode)

	data.CustomSettingsList = mapCustomSettingsListAppleOsX(data.CustomSettingsList, ent.CustomSettingsList)

	priorNetworks := data.NetworkList
	mappedNetworks := mapAppleOsXNetworkList(ent.NetworkList)
	if mappedNetworks == nil && priorNetworks != nil {
		mappedNetworks = priorNetworks[:0]
	}
	data.NetworkList = MergeNetworkListWithPriorState(mappedNetworks, priorNetworks)

	priorCreds := data.CredentialsList
	mappedCreds := mapAppleOsXCredentialsList(ent.CredentialsList)
	if mappedCreds == nil && priorCreds != nil {
		mappedCreds = priorCreds[:0]
	}
	data.CredentialsList = MergeCredentialsListWithPriorState(mappedCreds, priorCreds)

	priorDE := data.DiskEncryption
	data.DiskEncryption = MergeDiskEncryptionWithPriorState(mapAppleOsXDiskEncryption(ent.DiskEncryption), priorDE)

	priorGK := data.Gatekeeper
	data.Gatekeeper = MergeGatekeeperWithPriorState(mapAppleOsXGatekeeper(ent.GateKeeper), priorGK)

	priorR := data.Restrictions
	data.Restrictions = MergeRestrictionsWithPriorState(mapAppleOsXRestrictions(ent.Restrictions), priorR)
	_ = ctx
}

func mapAppleOsXPasscode(p *sdk.AppleOsXPasscodePayloadEntityV2) *PasscodeModel {
	if p == nil || !appleOsXPasscodeHasContent(p) {
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

func appleOsXPasscodeHasContent(p *sdk.AppleOsXPasscodePayloadEntityV2) bool {
	return p.RequirePasscodeOnDevice != nil ||
		p.AllowSimpleValue != nil ||
		p.RequireAlphanumericValue != nil ||
		p.MinimumPasscodeLength != nil ||
		p.MinimumNumberOfComplexCharacters != "" ||
		p.MaximumPasscodeAge != "" ||
		p.AutoLock != "" ||
		p.GracePeriod != nil ||
		p.MaxFailedAttempts != nil ||
		p.PinHistory != nil ||
		p.MinutesUntilFailedLoginReset != nil
}

func mergePasscodeWithPriorState(api, state *PasscodeModel) {
	if api == nil || state == nil {
		return
	}
	if (api.AutoLock.IsNull() || api.AutoLock.IsUnknown()) && !state.AutoLock.IsUnknown() {
		api.AutoLock = state.AutoLock
	}
	if (api.MaximumPasscodeAge.IsNull() || api.MaximumPasscodeAge.IsUnknown()) && !state.MaximumPasscodeAge.IsUnknown() {
		api.MaximumPasscodeAge = state.MaximumPasscodeAge
	}
	if (api.MinimumNumberOfComplexCharacters.IsNull() || api.MinimumNumberOfComplexCharacters.IsUnknown()) && !state.MinimumNumberOfComplexCharacters.IsUnknown() {
		api.MinimumNumberOfComplexCharacters = state.MinimumNumberOfComplexCharacters
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
		// Password / UserPassword / ProxyPassword are intentionally not read
		// from the API. UEM either omits them, echoes back an obfuscated
		// placeholder, or returns the original value; any of those breaks
		// Terraform's post-apply consistency check ("inconsistent values for
		// sensitive attribute") because the planned config value would no
		// longer match the response. Leave them null here and let
		// MergeNetworkListWithPriorState (KeepStateString) carry the user's
		// plan/state value forward unchanged.
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
		if n.ProxyServerPort != nil && *n.ProxyServerPort != 0 {
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
		// CA + template come back as 0 for Upload credentials (the field is
		// only meaningful for DefinedCertificateAuthority). Treat 0 as
		// "unset" so the consumer's null config doesn't perpetually diff.
		if c.CertificateAuthority != nil && *c.CertificateAuthority != 0 {
			out.CertificateAuthority = types.Int64Value(int64(*c.CertificateAuthority))
		}
		if c.CertificateTemplate != nil && *c.CertificateTemplate != 0 {
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
	if aw == nil && fv == nil && mcx == nil {
		return nil
	}
	return &DiskEncryptionModel{AirWatch: aw, FileVault: fv, MCX: mcx}
}

func mapAppleOsXDiskEncryptionAirWatch(aw *sdk.AppleOsXDiskEncryptionAirWatchPayloadEntityV2) *DiskEncryptionAirWatchModel {
	if aw == nil {
		return nil
	}
	empty := *aw == (sdk.AppleOsXDiskEncryptionAirWatchPayloadEntityV2{})
	if empty {
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
		EncryptionActionAfterLastNotification:       numericStringToInt64TF(aw.EncryptionActionAfterLastNotification),
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

func mapAppleOsXDiskEncryptionFileVault(fv *sdk.AppleOsXDiskEncryptionFileVault2PayloadEntityV2) *DiskEncryptionFileVaultModel {
	if fv == nil {
		return nil
	}
	empty := *fv == (sdk.AppleOsXDiskEncryptionFileVault2PayloadEntityV2{})
	if empty {
		return nil
	}
	return &DiskEncryptionFileVaultModel{
		Enable:                         boolPtrToTF(fv.Enable),
		ShowRecoveryKey:                boolPtrToTF(fv.ShowRecoveryKey),
		RecoveryType:                   int64PtrToTF(fv.RecoveryType),
		FileVaultEnterpriseCertificate: stringToTF(fv.FileVaultEnterpriseCertificate),
		FileVaultUser:                  numericStringToInt64TF(fv.FileVaultUser),
		Username:                       stringToTF(fv.Username),
		PromptToEnableFileVaultAt:      numericStringToInt64TF(fv.PromptToEnableFileVaultAt),
		NumberOfTimesUserCanBypass:     int64PtrToTF(fv.NumberOfTimesUserCanBypass),
	}
}

func mapAppleOsXDiskEncryptionMCX(mcx *sdk.AppleOsXDiskEncryptionMCXPayloadEntityV2) *DiskEncryptionMCXModel {
	if mcx == nil || mcx.DestroyFVKeyOnStandby == nil {
		return nil
	}
	return &DiskEncryptionMCXModel{DestroyFVKeyOnStandby: types.BoolValue(*mcx.DestroyFVKeyOnStandby)}
}

// mapAppleOsXGatekeeper hydrates a GatekeeperModel from the parent value-typed
// SDK field. UEM always returns this block populated for macOS profiles, even
// when the caller never sent it; the caller is expected to filter that out via
// MergeGatekeeperWithPriorState so unmanaged fields don't leak into state.
func mapAppleOsXGatekeeper(g *sdk.MacOsGatekeeperPayloadV2Entity) *GatekeeperModel {
	if g == nil || !appleOsXGatekeeperHasContent(g) {
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

func appleOsXGatekeeperHasContent(g *sdk.MacOsGatekeeperPayloadV2Entity) bool {
	return g.AllowAutoUnlock != nil ||
		g.AllowFingerprintForUnlock != nil ||
		g.AllowHandoff != nil ||
		g.AllowScreenCapture != nil ||
		g.EnableAppSoftwareUpdateDelay != nil ||
		g.EnableSoftwareUpdateDelay != nil ||
		g.EnforcedSoftwareUpdateDelay != nil
}

func readAppleiOSIntoState(_ context.Context, data *ProfileResourceModel, ent *sdk.AppleDeviceProfileV2Entity) {
	readGeneralV2IntoState(data, ent.General)
	priorPasscode := data.Passcode
	data.Passcode = mapAppleiOSPasscode(ent.Passcode)
	mergePasscodeWithPriorState(data.Passcode, priorPasscode)
	data.CustomSettingsList = mapCustomSettingsListAppleiOS(data.CustomSettingsList, ent.CustomSettingsList)
}

func mapAppleiOSPasscode(p *sdk.ApplePasscodePayloadV2Entity) *PasscodeModel {
	if p == nil || !appleiOSPasscodeHasContent(p) {
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

func appleiOSPasscodeHasContent(p *sdk.ApplePasscodePayloadV2Entity) bool {
	return p.RequirePasscodeOnDevice != nil ||
		p.AllowSimpleValue != nil ||
		p.RequireAlphanumericValue != nil ||
		p.MinimumPasscodeLength != nil ||
		p.MinimumNumberOfComplexCharacters != nil ||
		p.MaximumPasscodeAge != "" ||
		p.AutoLock != "" ||
		p.GracePeriodForDeviceLock != nil ||
		p.MaximumNumberOfFailedAttempts != "" ||
		p.PasscodeHistory != ""
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

func numericStringToInt64TF(s string) types.Int64 {
	if s == "" {
		return types.Int64Null()
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return types.Int64Null()
	}
	return types.Int64Value(n)
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

func mapAppleOsXRestrictions(r *sdk.AppleOsXRestrictionsPayloadEntityV2) *profilemodels.RestrictionsModel {
	if r == nil {
		return nil
	}
	out := &profilemodels.RestrictionsModel{
		Applications:  mapAppleOsXRestrictionsApplications(r.Applications),
		Desktop:       mapAppleOsXRestrictionsDesktop(r.Desktop),
		Functionality: mapAppleOsXRestrictionsFunctionality(r.Functionality),
		Media:         mapAppleOsXRestrictionsMedia(r.Media),
		Preferences:   mapAppleOsXRestrictionsPreferences(r.Preferences),
		Sharing:       mapAppleOsXRestrictionsSharing(r.Sharing),
		Widgets:       mapAppleOsXRestrictionsWidgets(r.Widgets),
	}
	if out.Applications == nil && out.Desktop == nil && out.Functionality == nil &&
		out.Media == nil && out.Preferences == nil && out.Sharing == nil && out.Widgets == nil {
		return nil
	}
	return out
}

func mapAppleOsXRestrictionsApplications(a *sdk.AppleOsXRestrictionApplicationsPayloadEntityV2) *profilemodels.RestrictionsApplicationsModel {
	if a == nil {
		return nil
	}
	m := &profilemodels.RestrictionsApplicationsModel{
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
	if m.AllowApplication.IsNull() && m.AllowFolders.IsNull() && m.DisallowFolders.IsNull() &&
		m.RestrictWhichApplicationsAreAllowedToLaunch.IsNull() &&
		m.AppStore == nil && m.AppleMusic == nil && m.Camera == nil &&
		m.GameCentre == nil && m.Safari == nil {
		return nil
	}
	return m
}

func mapAppleOsXRestrictionsAppStore(a *sdk.AppleOsXRestrictionAppStorePayloadEntityV2) *profilemodels.RestrictionsAppStoreModel {
	if a == nil || *a == (sdk.AppleOsXRestrictionAppStorePayloadEntityV2{}) {
		return nil
	}
	return &profilemodels.RestrictionsAppStoreModel{
		AllowAppStoreAppAdoption:                 boolPtrToTF(a.AllowAppStoreAppAdoption),
		RequireAdminPasswordToInstallOrUpdateApp: boolPtrToTF(a.RequireAdminPasswordToInstallOrUpdateApp),
		RestrictAppStoreToSoftwareUpdatesOnly:    boolPtrToTF(a.RestrictAppStoreToSoftwareUpdatesOnly),
	}
}

func mapAppleOsXRestrictionsAppleMusic(a *sdk.AppleOsXRestrictionAppleMusicPayloadEntityV2) *profilemodels.RestrictionsAppleMusicModel {
	if a == nil || a.AllowMusicService == nil {
		return nil
	}
	return &profilemodels.RestrictionsAppleMusicModel{
		AllowMusicService: boolPtrToTF(a.AllowMusicService),
	}
}

func mapAppleOsXRestrictionsCamera(a *sdk.AppleOsXRestrictionCameraPayloadEntityV2) *profilemodels.RestrictionsCameraModel {
	if a == nil || a.AllowUseOfBuiltInCamera == nil {
		return nil
	}
	return &profilemodels.RestrictionsCameraModel{
		AllowUseOfBuiltInCamera: boolPtrToTF(a.AllowUseOfBuiltInCamera),
	}
}

func mapAppleOsXRestrictionsGameCentre(a *sdk.AppleOsXRestrictionGameCentrePayloadEntityV2) *profilemodels.RestrictionsGameCentreModel {
	if a == nil || *a == (sdk.AppleOsXRestrictionGameCentrePayloadEntityV2{}) {
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
	if a == nil || *a == (sdk.AppleOsXRestrictionSafariPayloadEntityV2{}) {
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
	m := &profilemodels.RestrictionsDesktopModel{
		DesktopPicturePath: stringToTF(a.DesktopPicturePath),
		LockDesktopPicture: boolPtrToTF(a.LockDesktopPicture),
	}
	if m.DesktopPicturePath.IsNull() && m.LockDesktopPicture.IsNull() {
		return nil
	}
	return m
}

func mapAppleOsXRestrictionsFunctionality(a *sdk.AppleOsXRestrictionFunctionalityPayloadEntityV2) *profilemodels.RestrictionsFunctionalityModel {
	if a == nil {
		return nil
	}
	m := &profilemodels.RestrictionsFunctionalityModel{
		AirPrint:       mapAppleOsXRestrictionsAirPrint(a.AirPrint),
		ContentCaching: mapAppleOsXRestrictionsContentCaching(a.ContentCaching),
		ICloud:         mapAppleOsXRestrictionsICloud(a.ICloud),
		Passwords:      mapAppleOsXRestrictionsPasswords(a.Passwords),
		Spotlight:      mapAppleOsXRestrictionsSpotlight(a.Spotlight),
	}
	if m.AirPrint == nil && m.ContentCaching == nil && m.ICloud == nil &&
		m.Passwords == nil && m.Spotlight == nil {
		return nil
	}
	return m
}

func mapAppleOsXRestrictionsAirPrint(a *sdk.AppleOsXRestrictionAirPrintPayloadEntityV2) *profilemodels.RestrictionsAirPrintModel {
	if a == nil || *a == (sdk.AppleOsXRestrictionAirPrintPayloadEntityV2{}) {
		return nil
	}
	return &profilemodels.RestrictionsAirPrintModel{
		AllowAirPrint:                      boolPtrToTF(a.AllowAirPrint),
		AllowAirPrintiBeaconDiscovery:      boolPtrToTF(a.AllowAirPrintiBeaconDiscovery),
		ForceAirPrintTrustedTLSRequirement: boolPtrToTF(a.ForceAirPrintTrustedTLSRequirement),
	}
}

func mapAppleOsXRestrictionsContentCaching(a *sdk.AppleOsXRestrictionContentCachingPayloadEntityV2) *profilemodels.RestrictionsContentCachingModel {
	if a == nil || a.AllowContentCaching == nil {
		return nil
	}
	return &profilemodels.RestrictionsContentCachingModel{
		AllowContentCaching: boolPtrToTF(a.AllowContentCaching),
	}
}

func mapAppleOsXRestrictionsICloud(a *sdk.AppleOsXRestrictionICloudPayloadEntityV2) *profilemodels.RestrictionsICloudModel {
	if a == nil || *a == (sdk.AppleOsXRestrictionICloudPayloadEntityV2{}) {
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
	if a == nil || *a == (sdk.AppleOsXRestrictionPasswordsPayloadEntityV2{}) {
		return nil
	}
	return &profilemodels.RestrictionsPasswordsModel{
		AllowPasswordAutoFill:          boolPtrToTF(a.AllowPasswordAutoFill),
		AllowPasswordProximityRequests: boolPtrToTF(a.AllowPasswordProximityRequests),
		AllowPasswordSharing:           boolPtrToTF(a.AllowPasswordSharing),
	}
}

func mapAppleOsXRestrictionsSpotlight(a *sdk.AppleOsXRestrictionSpotlightPayloadEntityV2) *profilemodels.RestrictionsSpotlightModel {
	if a == nil || a.AllowSpotlightSuggestions == nil {
		return nil
	}
	return &profilemodels.RestrictionsSpotlightModel{
		AllowSpotlightSuggestions: boolPtrToTF(a.AllowSpotlightSuggestions),
	}
}

func mapAppleOsXMediaAccess(a *sdk.AppleOsXMediaAccessEntityV2) *profilemodels.RestrictionsMediaAccessModel {
	if a == nil || *a == (sdk.AppleOsXMediaAccessEntityV2{}) {
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
	if a.NetworkAccess != nil && a.NetworkAccess.AirDrop != nil {
		m.NetworkAccess = &profilemodels.RestrictionsNetworkAccessModel{
			AirDrop: boolPtrToTF(a.NetworkAccess.AirDrop),
		}
	}
	if a.RecordableDisc != nil {
		if burn := mapAppleOsXMediaAccess(a.RecordableDisc.BurnSupport); burn != nil {
			m.RecordableDisc = &profilemodels.RestrictionsBurnSupportModel{BurnSupport: burn}
		}
	}
	if m.AutoEjectMedia.IsNull() && m.DiskMediaCDs == nil && m.DiskMediaDVDs == nil &&
		m.ExternalHardDiskMediaAccess == nil && m.HardDiskDvdRam == nil &&
		m.HardDiskImages == nil && m.InternalHardDiskMediaAccess == nil &&
		m.NetworkAccess == nil && m.RecordableDisc == nil {
		return nil
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
	if preferencesModelIsEmpty(m) {
		return nil
	}
	return m
}

func preferencesModelIsEmpty(m *profilemodels.RestrictionsPreferencesModel) bool {
	return m.Accessibility.IsNull() && m.AppStore.IsNull() && m.Bluetooth.IsNull() &&
		m.CDsAndDVDs.IsNull() && m.DateAndTime.IsNull() && m.DesktopAndScreenSaver.IsNull() &&
		m.DictationAndSpeech.IsNull() && m.Displays.IsNull() && m.Dock.IsNull() &&
		m.EnabledPreferencePanes.IsNull() && m.EnergySaver.IsNull() && m.Extensions.IsNull() &&
		m.FibreChannel.IsNull() && m.FlashPlayer.IsNull() && m.General.IsNull() &&
		m.Ink.IsNull() && m.InternetAccounts.IsNull() && m.Keyboard.IsNull() &&
		m.LanguageAndText.IsNull() && m.MissionControl.IsNull() && m.MobileMe.IsNull() &&
		m.Mouse.IsNull() && m.Network.IsNull() && m.Notifications.IsNull() &&
		m.ParentalControls.IsNull() && m.PreferenceBehavior.IsNull() && m.PrintAndScan.IsNull() &&
		m.Profiles.IsNull() && m.SecurityAndPrivacy.IsNull() && m.Sharing.IsNull() &&
		m.SoftwareUpdate.IsNull() && m.Sound.IsNull() && m.Spotlight.IsNull() &&
		m.StartupDisk.IsNull() && m.TimeMachine.IsNull() && m.Trackpad.IsNull() &&
		m.UsersAndGroups.IsNull() && m.Xsan.IsNull() && m.ICloud.IsNull()
}

func mapAppleOsXRestrictionsSharing(a *sdk.AppleOsXRestrictionSharingPayloadEntityV2) *profilemodels.RestrictionsSharingModel {
	if a == nil || *a == (sdk.AppleOsXRestrictionSharingPayloadEntityV2{}) {
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
