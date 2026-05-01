package platform

import (
	"fmt"
	"strconv"

	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"
	sdk "github.com/euc-oss/terraform-sdk-uem"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func BuildAppleOsXCreateEntity(data *profilemodels.ProfileResourceModel) (*sdk.AppleOsXDeviceProfileEntityV2, error) {
	ent := &sdk.AppleOsXDeviceProfileEntityV2{
		General: BuildGeneralV2Create(data),
	}

	if data.Passcode != nil {
		pc, err := buildAppleOsXPasscodeEntity(data.Passcode)
		if err != nil {
			return nil, err
		}
		ent.Passcode = pc
	}
	if items := buildAppleOsXCustomSettingsList(data.CustomSettingsList); len(items) > 0 {
		ent.CustomSettingsList = items
	}
	if items := buildAppleOsXNetworkListEntity(data.NetworkList); len(items) > 0 {
		ent.NetworkList = items
	}
	if items := buildAppleOsXCredentialsListEntity(data.CredentialsList); len(items) > 0 {
		ent.CredentialsList = items
	}
	if data.DiskEncryption != nil {
		ent.DiskEncryption = buildAppleOsXDiskEncryptionEntity(data.DiskEncryption)
	}
	if data.Gatekeeper != nil {
		ent.GateKeeper = buildAppleOsXGatekeeperEntity(data.Gatekeeper)
	}
	if data.Restrictions != nil {
		ent.Restrictions = buildAppleOsXRestrictionsEntity(data.Restrictions)
	}
	return ent, nil
}

func BuildGeneralV2Create(data *profilemodels.ProfileResourceModel) *sdk.GeneralPayloadV2Entity {
	g := &sdk.GeneralPayloadV2Entity{
		Name:           data.Name.ValueString(),
		Description:    data.Description.ValueString(),
		AssignmentType: data.AssignmentType.ValueString(),
		ProfileScope:   data.ProfileScope.ValueString(),
	}

	// Map assigned/excluded smart groups if present
	if len(data.AssignedSmartGroups) > 0 {
		g.AssignedSmartGroups = make([]sdk.SmartGroupEntityV2, 0, len(data.AssignedSmartGroups))
		for _, sg := range data.AssignedSmartGroups {
			if !sg.IsNull() && !sg.IsUnknown() && sg.ValueString() != "" {
				if n, err := strconv.Atoi(sg.ValueString()); err == nil {
					g.AssignedSmartGroups = append(g.AssignedSmartGroups, sdk.SmartGroupEntityV2{SmartGroupID: sdk.IntPtr(n)})
				}
			}
		}
	}
	if len(data.ExcludedSmartGroups) > 0 {
		g.ExcludedSmartGroups = make([]sdk.SmartGroupEntityV2, 0, len(data.ExcludedSmartGroups))
		for _, sg := range data.ExcludedSmartGroups {
			if !sg.IsNull() && !sg.IsUnknown() && sg.ValueString() != "" {
				if n, err := strconv.Atoi(sg.ValueString()); err == nil {
					g.ExcludedSmartGroups = append(g.ExcludedSmartGroups, sdk.SmartGroupEntityV2{SmartGroupID: sdk.IntPtr(n)})
				}
			}
		}
	}
	if !data.IsActive.IsNull() && !data.IsActive.IsUnknown() {
		g.IsActive = sdk.BoolPtr(data.IsActive.ValueBool())
	}
	if !data.ProfileContext.IsNull() && !data.ProfileContext.IsUnknown() && data.ProfileContext.ValueString() != "" {
		g.ProfileContext = data.ProfileContext.ValueString()
	}
	if !data.OrgGroupID.IsNull() && !data.OrgGroupID.IsUnknown() {
		if n, err := strconv.Atoi(data.OrgGroupID.ValueString()); err == nil {
			g.ManagedLocationGroupID = sdk.IntPtr(n)
		}
	}
	return g
}

func BuildAndroidCreateEntity(data *profilemodels.ProfileResourceModel) *sdk.AndroidDeviceProfileV2Entity {
	ent := &sdk.AndroidDeviceProfileV2Entity{
		General: BuildGeneralV2Create(data),
	}
	if !data.LockScreenMessage.IsNull() && !data.LockScreenMessage.IsUnknown() {
		ent.AndroidForWorkCustomMessages = &sdk.AndroidForWorkCustomMessagesPayloadV2Entity{
			LockScreenMessage: data.LockScreenMessage.ValueString(),
		}
	}
	if items := buildAndroidCustomSettingsList(data.CustomSettingsList); len(items) > 0 {
		ent.CustomSettingsList = items
	}
	return ent
}

func BuildAppleiOSCreateEntity(data *profilemodels.ProfileResourceModel) *sdk.AppleDeviceProfileV2Entity {
	ent := &sdk.AppleDeviceProfileV2Entity{
		General: BuildGeneralV2Create(data),
	}
	if data.Passcode != nil {
		ent.Passcode = buildAppleiOSPasscodeEntity(data.Passcode)
	}
	if items := buildAppleCustomSettingsList(data.CustomSettingsList); len(items) > 0 {
		ent.CustomSettingsList = items
	}
	return ent
}

func BuildWindows10CreateEntity(data *profilemodels.ProfileResourceModel) *sdk.WinRTDeviceProfileV2Entity {
	return &sdk.WinRTDeviceProfileV2Entity{General: BuildGeneralV2Create(data)}
}

func BuildWindowsRuggedCreateEntity(data *profilemodels.ProfileResourceModel) *sdk.QnxDeviceProfileEntityV2 {
	return &sdk.QnxDeviceProfileEntityV2{General: BuildGeneralV2Create(data)}
}

func BuildLinuxCreateEntity(data *profilemodels.ProfileResourceModel) *sdk.LinuxDeviceProfileEntity1V4 {
	return &sdk.LinuxDeviceProfileEntity1V4{General: BuildGeneralV4Create(data)}
}

func BuildGeneralV4Create(data *profilemodels.ProfileResourceModel) *sdk.GeneralPayloadV4Entity {
	g := &sdk.GeneralPayloadV4Entity{
		Name:           data.Name.ValueString(),
		Description:    data.Description.ValueString(),
		AssignmentType: data.AssignmentType.ValueString(),
		ProfileScope:   data.ProfileScope.ValueString(),
	}
	if !data.IsActive.IsNull() && !data.IsActive.IsUnknown() {
		g.IsActive = sdk.BoolPtr(data.IsActive.ValueBool())
	}
	if !data.ProfileContext.IsNull() && !data.ProfileContext.IsUnknown() && data.ProfileContext.ValueString() != "" {
		g.ProfileContext = data.ProfileContext.ValueString()
	}
	if !data.OrgGroupID.IsNull() && !data.OrgGroupID.IsUnknown() {
		if n, err := strconv.Atoi(data.OrgGroupID.ValueString()); err == nil {
			g.ManagedLocationGroupID = sdk.IntPtr(n)
		}
	}
	return g
}

func StampGeneralV2(g *sdk.GeneralPayloadV2Entity, profileID int, currentVersion *int) {
	if g == nil {
		return
	}
	pid := profileID
	g.ProfileID = &pid
	g.CreateNewVersion = sdk.BoolPtr(true)
	if currentVersion != nil {
		next := *currentVersion + 1
		g.Version = &next
	}
}

func StampGeneralV4(g *sdk.GeneralPayloadV4Entity, profileID int, currentVersion *int) {
	if g == nil {
		return
	}
	pid := profileID
	g.ProfileID = &pid
	g.CreateNewVersion = sdk.BoolPtr(true)
	if currentVersion != nil {
		next := *currentVersion + 1
		g.Version = &next
	}
}

func buildAndroidCustomSettingsList(items []profilemodels.CustomSettingsItemModel) []sdk.AndroidCustomSettingsPayloadV2Entity {
	if len(items) == 0 {
		return nil
	}
	out := make([]sdk.AndroidCustomSettingsPayloadV2Entity, 0, len(items))
	for _, it := range items {
		if it.CustomSettings.IsNull() || it.CustomSettings.IsUnknown() {
			continue
		}
		out = append(out, sdk.AndroidCustomSettingsPayloadV2Entity{CustomSettings: it.CustomSettings.ValueString()})
	}
	return out
}

func buildAppleCustomSettingsList(items []profilemodels.CustomSettingsItemModel) []sdk.AppleCustomSettingsPayloadV2Entity {
	if len(items) == 0 {
		return nil
	}
	out := make([]sdk.AppleCustomSettingsPayloadV2Entity, 0, len(items))
	for _, it := range items {
		if it.CustomSettings.IsNull() || it.CustomSettings.IsUnknown() {
			continue
		}
		out = append(out, sdk.AppleCustomSettingsPayloadV2Entity{CustomSettings: it.CustomSettings.ValueString()})
	}
	return out
}

func buildAppleiOSPasscodeEntity(p *profilemodels.PasscodeModel) *sdk.ApplePasscodePayloadV2Entity {
	out := &sdk.ApplePasscodePayloadV2Entity{}
	if !p.RequirePasscodeOnDevice.IsNull() && !p.RequirePasscodeOnDevice.IsUnknown() {
		out.RequirePasscodeOnDevice = sdk.BoolPtr(p.RequirePasscodeOnDevice.ValueBool())
	}
	if !p.AllowSimpleValue.IsNull() && !p.AllowSimpleValue.IsUnknown() {
		out.AllowSimpleValue = sdk.BoolPtr(p.AllowSimpleValue.ValueBool())
	}
	if !p.RequireAlphanumericValue.IsNull() && !p.RequireAlphanumericValue.IsUnknown() {
		out.RequireAlphanumericValue = sdk.BoolPtr(p.RequireAlphanumericValue.ValueBool())
	}
	if !p.MinimumPasscodeLength.IsNull() && !p.MinimumPasscodeLength.IsUnknown() {
		out.MinimumPasscodeLength = sdk.IntPtr(int(p.MinimumPasscodeLength.ValueInt64()))
	}
	if !p.MinimumNumberOfComplexCharacters.IsNull() && !p.MinimumNumberOfComplexCharacters.IsUnknown() {
		if n, err := strconv.Atoi(p.MinimumNumberOfComplexCharacters.ValueString()); err == nil {
			out.MinimumNumberOfComplexCharacters = sdk.IntPtr(n)
		}
	}
	if !p.MaximumPasscodeAge.IsNull() && !p.MaximumPasscodeAge.IsUnknown() {
		out.MaximumPasscodeAge = p.MaximumPasscodeAge.ValueString()
	}
	if !p.AutoLock.IsNull() && !p.AutoLock.IsUnknown() {
		out.AutoLock = p.AutoLock.ValueString()
	}
	if !p.GracePeriod.IsNull() && !p.GracePeriod.IsUnknown() {
		out.GracePeriodForDeviceLock = sdk.IntPtr(int(p.GracePeriod.ValueInt64()))
	}
	if !p.MaxFailedAttempts.IsNull() && !p.MaxFailedAttempts.IsUnknown() {
		out.MaximumNumberOfFailedAttempts = p.MaxFailedAttempts.ValueString()
	}
	if !p.PinHistory.IsNull() && !p.PinHistory.IsUnknown() {
		out.PasscodeHistory = p.PinHistory.ValueString()
	}
	return out
}

func buildAppleOsXPasscodeEntity(p *profilemodels.PasscodeModel) (*sdk.AppleOsXPasscodePayloadEntityV2, error) {
	out := &sdk.AppleOsXPasscodePayloadEntityV2{}
	if !p.RequirePasscodeOnDevice.IsNull() && !p.RequirePasscodeOnDevice.IsUnknown() {
		out.RequirePasscodeOnDevice = sdk.BoolPtr(p.RequirePasscodeOnDevice.ValueBool())
	}
	if !p.AllowSimpleValue.IsNull() && !p.AllowSimpleValue.IsUnknown() {
		out.AllowSimpleValue = sdk.BoolPtr(p.AllowSimpleValue.ValueBool())
	}
	if !p.RequireAlphanumericValue.IsNull() && !p.RequireAlphanumericValue.IsUnknown() {
		out.RequireAlphanumericValue = sdk.BoolPtr(p.RequireAlphanumericValue.ValueBool())
	}
	if !p.MinimumPasscodeLength.IsNull() && !p.MinimumPasscodeLength.IsUnknown() {
		out.MinimumPasscodeLength = sdk.IntPtr(int(p.MinimumPasscodeLength.ValueInt64()))
	}
	if !p.MinimumNumberOfComplexCharacters.IsNull() && !p.MinimumNumberOfComplexCharacters.IsUnknown() {
		out.MinimumNumberOfComplexCharacters = p.MinimumNumberOfComplexCharacters.ValueString()
	}
	if !p.MaximumPasscodeAge.IsNull() && !p.MaximumPasscodeAge.IsUnknown() {
		out.MaximumPasscodeAge = p.MaximumPasscodeAge.ValueString()
	}
	if !p.AutoLock.IsNull() && !p.AutoLock.IsUnknown() {
		out.AutoLock = p.AutoLock.ValueString()
	}
	if !p.GracePeriod.IsNull() && !p.GracePeriod.IsUnknown() {
		out.GracePeriod = sdk.IntPtr(int(p.GracePeriod.ValueInt64()))
	}
	if !p.MaxFailedAttempts.IsNull() && !p.MaxFailedAttempts.IsUnknown() {
		s := p.MaxFailedAttempts.ValueString()
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil, fmt.Errorf("passcode.max_failed_attempts (macOS): must be a whole number, got %q", s)
		}
		out.MaxFailedAttempts = sdk.IntPtr(n)
	}
	if !p.PinHistory.IsNull() && !p.PinHistory.IsUnknown() {
		s := p.PinHistory.ValueString()
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil, fmt.Errorf("passcode.pin_history (macOS): must be a whole number, got %q", s)
		}
		out.PinHistory = sdk.IntPtr(n)
	}
	if !p.MinutesUntilFailedLoginReset.IsNull() && !p.MinutesUntilFailedLoginReset.IsUnknown() {
		out.MinutesUntilFailedLoginReset = sdk.IntPtr(int(p.MinutesUntilFailedLoginReset.ValueInt64()))
	}
	return out, nil
}

func buildAppleOsXCustomSettingsList(items []profilemodels.CustomSettingsItemModel) []sdk.AppleOsXCustomSettingsPayloadEntityV2 {
	if len(items) == 0 {
		return nil
	}
	result := make([]sdk.AppleOsXCustomSettingsPayloadEntityV2, 0, len(items))
	for _, it := range items {
		if it.CustomSettings.IsNull() || it.CustomSettings.IsUnknown() {
			continue
		}
		result = append(result, sdk.AppleOsXCustomSettingsPayloadEntityV2{CustomSettings: it.CustomSettings.ValueString()})
	}
	return result
}

func buildAppleOsXNetworkListEntity(items []profilemodels.NetworkItemModel) []sdk.AppleOsXNetworkPayloadEntityV2 {
	if len(items) == 0 {
		return nil
	}
	result := make([]sdk.AppleOsXNetworkPayloadEntityV2, 0, len(items))
	for i := range items {
		n := &items[i]
		out := sdk.AppleOsXNetworkPayloadEntityV2{}

		setStringIfKnown(&out.NetworkInterface, n.NetworkInterface)
		setStringIfKnown(&out.ServiceSetIdentifier, n.ServiceSetIdentifier)
		out.HiddenNetwork = boolPtrFromTF(n.HiddenNetwork)
		out.AutoJoin = boolPtrFromTF(n.AutoJoin)
		setStringIfKnown(&out.SecurityType, n.SecurityType)
		setStringIfKnown(&out.Password, n.Password)
		out.UseAsLoginWindowConfiguration = boolPtrFromTF(n.UseAsLoginWindowConfiguration)
		out.UseDirectoryAuthentication = boolPtrFromTF(n.UseDirectoryAuthentication)
		out.TLS = boolPtrFromTF(n.TLS)
		out.TTLS = boolPtrFromTF(n.TTLS)
		out.LEAP = boolPtrFromTF(n.LEAP)
		out.PEAP = boolPtrFromTF(n.PEAP)
		out.EAPFAST = boolPtrFromTF(n.EAPFAST)
		out.EAPSIM = boolPtrFromTF(n.EAPSIM)
		out.EAPAKA = boolPtrFromTF(n.EAPAKA)
		setStringIfKnown(&out.TLSMinimumVersion, n.TLSMinimumVersion)
		setStringIfKnown(&out.TLSMaximumVersion, n.TLSMaximumVersion)
		out.DisableAssociationMACRandomization = boolPtrFromTF(n.DisableAssociationMACRandomization)
		setStringIfKnown(&out.UserName, n.UserName)
		setStringIfKnown(&out.UserPassword, n.UserPassword)
		setStringIfKnown(&out.IdentityCertificate, n.IdentityCertificate)
		setStringIfKnown(&out.InnerIdentity, n.InnerIdentity)
		setStringIfKnown(&out.OuterIdentity, n.OuterIdentity)
		out.UsePAC = boolPtrFromTF(n.UsePAC)
		out.AllowTwoRANDs = boolPtrFromTF(n.AllowTwoRANDs)
		if !n.TrustedCertificates.IsNull() && !n.TrustedCertificates.IsUnknown() {
			certs := make([]string, 0, len(n.TrustedCertificates.Elements()))
			for _, elem := range n.TrustedCertificates.Elements() {
				if sv, ok := elem.(types.String); ok {
					certs = append(certs, sv.ValueString())
				}
			}
			if len(certs) > 0 {
				out.TrustedCertificates = certs
			}
		}
		out.AllowTrustExceptions = boolPtrFromTF(n.AllowTrustExceptions)
		setStringIfKnown(&out.ProxyType, n.ProxyType)
		setStringIfKnown(&out.ProxyServer, n.ProxyServer)
		if !n.ProxyServerPort.IsNull() && !n.ProxyServerPort.IsUnknown() {
			out.ProxyServerPort = sdk.IntPtr(int(n.ProxyServerPort.ValueInt64()))
		}
		setStringIfKnown(&out.ProxyUsername, n.ProxyUsername)
		setStringIfKnown(&out.ProxyPassword, n.ProxyPassword)
		setStringIfKnown(&out.ProxyURL, n.ProxyUrl)
		out.PacFallback = boolPtrFromTF(n.PacFallback)

		result = append(result, out)
	}
	return result
}

func buildAppleOsXCredentialsListEntity(items []profilemodels.CredentialItemModel) []sdk.AppleOsXCredentialPayloadEntityV2 {
	if len(items) == 0 {
		return nil
	}
	result := make([]sdk.AppleOsXCredentialPayloadEntityV2, 0, len(items))
	for i := range items {
		c := &items[i]
		out := sdk.AppleOsXCredentialPayloadEntityV2{}
		setStringIfKnown(&out.CredentialSource, c.CredentialSource)
		if !c.CredentialName.IsNull() && !c.CredentialName.IsUnknown() {
			out.CredentialName = c.CredentialName.ValueString()
			out.Name = c.CredentialName.ValueString()
		}
		if !c.CertificateID.IsNull() && !c.CertificateID.IsUnknown() {
			out.CertificateID = sdk.IntPtr(int(c.CertificateID.ValueInt64()))
		}
		if !c.CertificateAuthority.IsNull() && !c.CertificateAuthority.IsUnknown() {
			out.CertificateAuthority = sdk.IntPtr(int(c.CertificateAuthority.ValueInt64()))
		}
		if !c.CertificateTemplate.IsNull() && !c.CertificateTemplate.IsUnknown() {
			out.CertificateTemplate = sdk.IntPtr(int(c.CertificateTemplate.ValueInt64()))
		}
		if !c.AllowAccessToAllApplications.IsNull() && !c.AllowAccessToAllApplications.IsUnknown() {
			out.AllowAccessToAllApplications = sdk.BoolPtr(c.AllowAccessToAllApplications.ValueBool())
		}
		if !c.KeyIsExtractable.IsNull() && !c.KeyIsExtractable.IsUnknown() {
			out.KeyIsExtractable = sdk.BoolPtr(c.KeyIsExtractable.ValueBool())
		}
		result = append(result, out)
	}
	return result
}

func buildAppleOsXDiskEncryptionEntity(m *profilemodels.DiskEncryptionModel) *sdk.AppleOsXDiskEncryptionPayloadEntityV2 {
	out := &sdk.AppleOsXDiskEncryptionPayloadEntityV2{}
	if m.AirWatch != nil {
		out.DiskEncryptionAirWatch = buildAppleOsXDiskEncryptionAirWatchEntity(m.AirWatch)
	}
	if m.FileVault != nil {
		out.DiskEncryptionFileVault2 = buildAppleOsXDiskEncryptionFileVault2Entity(m.FileVault)
	}
	if m.MCX != nil {
		out.DiskEncryptionMCX = buildAppleOsXDiskEncryptionMCXEntity(m.MCX)
	}
	return out
}

func buildAppleOsXDiskEncryptionAirWatchEntity(m *profilemodels.DiskEncryptionAirWatchModel) *sdk.AppleOsXDiskEncryptionAirWatchPayloadEntityV2 {
	out := &sdk.AppleOsXDiskEncryptionAirWatchPayloadEntityV2{}
	out.StoreKey = boolPtrFromTF(m.StoreKey)
	if !m.RotateKeyAfter.IsNull() && !m.RotateKeyAfter.IsUnknown() {
		out.RotateKeyAfter = sdk.IntPtr(int(m.RotateKeyAfter.ValueInt64()))
	}
	out.UseIntelligentHub = boolPtrFromTF(m.UseIntelligentHub)
	out.NotifyUserForEncryption = boolPtrFromTF(m.NotifyUserForEncryption)
	setStringIfKnown(&out.EncryptionNotificationTitle, m.EncryptionNotificationTitle)
	setStringIfKnown(&out.EncryptionNotificationMessage, m.EncryptionNotificationMessage)
	if !m.EncryptionMaxNotifyAttempts.IsNull() && !m.EncryptionMaxNotifyAttempts.IsUnknown() {
		out.EncryptionMaxNotifyAttempts = sdk.IntPtr(int(m.EncryptionMaxNotifyAttempts.ValueInt64()))
	}
	if !m.EncryptionNotificationRetryIntervalInHours.IsNull() && !m.EncryptionNotificationRetryIntervalInHours.IsUnknown() {
		out.EncryptionNotificationRetryIntervalInHours = sdk.IntPtr(int(m.EncryptionNotificationRetryIntervalInHours.ValueInt64()))
	}
	if !m.EncryptionActionAfterLastNotification.IsNull() && !m.EncryptionActionAfterLastNotification.IsUnknown() {
		out.EncryptionActionAfterLastNotification = strconv.FormatInt(m.EncryptionActionAfterLastNotification.ValueInt64(), 10)
	}
	out.EnableRecoveryKey = boolPtrFromTF(m.EnableRecoveryKey)
	setStringIfKnown(&out.RecoveryKeyNotificationTitle, m.RecoveryKeyNotificationTitle)
	setStringIfKnown(&out.RecoveryKeyNotificationMessage, m.RecoveryKeyNotificationMessage)
	if !m.RecoveryKeyNotificationRetryIntervalInHours.IsNull() && !m.RecoveryKeyNotificationRetryIntervalInHours.IsUnknown() {
		out.RecoveryKeyNotificationRetryIntervalInHours = sdk.IntPtr(int(m.RecoveryKeyNotificationRetryIntervalInHours.ValueInt64()))
	}
	setStringIfKnown(&out.RecoveryKeyPromptTitle, m.RecoveryKeyPromptTitle)
	setStringIfKnown(&out.RecoveryKeyPromptMessage, m.RecoveryKeyPromptMessage)
	setStringIfKnown(&out.RecoveryKeySuccessTitle, m.RecoveryKeySuccessTitle)
	setStringIfKnown(&out.RecoveryKeySuccessMessage, m.RecoveryKeySuccessMessage)
	setStringIfKnown(&out.RecoveryKeyErrorTitle, m.RecoveryKeyErrorTitle)
	setStringIfKnown(&out.RecoveryKeyErrorMessage, m.RecoveryKeyErrorMessage)
	if !m.RecoveryKeyMaxFailureCount.IsNull() && !m.RecoveryKeyMaxFailureCount.IsUnknown() {
		out.RecoveryKeyMaxFailureCount = sdk.IntPtr(int(m.RecoveryKeyMaxFailureCount.ValueInt64()))
	}
	return out
}

func buildAppleOsXDiskEncryptionFileVault2Entity(m *profilemodels.DiskEncryptionFileVaultModel) *sdk.AppleOsXDiskEncryptionFileVault2PayloadEntityV2 {
	out := &sdk.AppleOsXDiskEncryptionFileVault2PayloadEntityV2{}
	out.Enable = boolPtrFromTF(m.Enable)
	out.ShowRecoveryKey = boolPtrFromTF(m.ShowRecoveryKey)
	if !m.RecoveryType.IsNull() && !m.RecoveryType.IsUnknown() {
		out.RecoveryType = sdk.IntPtr(int(m.RecoveryType.ValueInt64()))
	}
	setStringIfKnown(&out.FileVaultEnterpriseCertificate, m.FileVaultEnterpriseCertificate)
	if !m.FileVaultUser.IsNull() && !m.FileVaultUser.IsUnknown() {
		out.FileVaultUser = strconv.FormatInt(m.FileVaultUser.ValueInt64(), 10)
	}
	setStringIfKnown(&out.Username, m.Username)
	if !m.PromptToEnableFileVaultAt.IsNull() && !m.PromptToEnableFileVaultAt.IsUnknown() {
		out.PromptToEnableFileVaultAt = strconv.FormatInt(m.PromptToEnableFileVaultAt.ValueInt64(), 10)
	}
	if !m.NumberOfTimesUserCanBypass.IsNull() && !m.NumberOfTimesUserCanBypass.IsUnknown() {
		out.NumberOfTimesUserCanBypass = sdk.IntPtr(int(m.NumberOfTimesUserCanBypass.ValueInt64()))
	}
	return out
}

func buildAppleOsXDiskEncryptionMCXEntity(m *profilemodels.DiskEncryptionMCXModel) *sdk.AppleOsXDiskEncryptionMCXPayloadEntityV2 {
	return &sdk.AppleOsXDiskEncryptionMCXPayloadEntityV2{
		DestroyFVKeyOnStandby: boolPtrFromTF(m.DestroyFVKeyOnStandby),
	}
}

func buildAppleOsXRestrictionsEntity(m *profilemodels.RestrictionsModel) *sdk.AppleOsXRestrictionsPayloadEntityV2 {
	out := &sdk.AppleOsXRestrictionsPayloadEntityV2{}
	if m.Applications != nil {
		out.Applications = buildAppleOsXRestrictionsApplicationsEntity(m.Applications)
	}
	if m.Desktop != nil {
		out.Desktop = buildAppleOsXRestrictionsDesktopEntity(m.Desktop)
	}
	if m.Functionality != nil {
		out.Functionality = buildAppleOsXRestrictionsFunctionalityEntity(m.Functionality)
	}
	if m.Media != nil {
		out.Media = buildAppleOsXRestrictionsMediaEntity(m.Media)
	}
	if m.Preferences != nil {
		out.Preferences = buildAppleOsXRestrictionsPreferencesEntity(m.Preferences)
	}
	if m.Sharing != nil {
		out.Sharing = buildAppleOsXRestrictionsSharingEntity(m.Sharing)
	}
	if m.Widgets != nil {
		out.Widgets = buildAppleOsXRestrictionsWidgetsEntity(m.Widgets)
	}
	return out
}

func buildAppleOsXRestrictionsApplicationsEntity(m *profilemodels.RestrictionsApplicationsModel) *sdk.AppleOsXRestrictionApplicationsPayloadEntityV2 {
	out := &sdk.AppleOsXRestrictionApplicationsPayloadEntityV2{
		AllowApplication: stringSliceFromTFList(m.AllowApplication),
		AllowFolders:     stringSliceFromTFList(m.AllowFolders),
		DisallowFolders:  stringSliceFromTFList(m.DisallowFolders),
	}
	out.RestrictWhichApplicationsAreAllowedToLaunch = boolPtrFromTF(m.RestrictWhichApplicationsAreAllowedToLaunch)
	if m.AppStore != nil {
		out.AppStore = &sdk.AppleOsXRestrictionAppStorePayloadEntityV2{
			AllowAppStoreAppAdoption:                 boolPtrFromTF(m.AppStore.AllowAppStoreAppAdoption),
			RequireAdminPasswordToInstallOrUpdateApp: boolPtrFromTF(m.AppStore.RequireAdminPasswordToInstallOrUpdateApp),
			RestrictAppStoreToSoftwareUpdatesOnly:    boolPtrFromTF(m.AppStore.RestrictAppStoreToSoftwareUpdatesOnly),
		}
	}
	if m.AppleMusic != nil {
		out.AppleMusic = &sdk.AppleOsXRestrictionAppleMusicPayloadEntityV2{
			AllowMusicService: boolPtrFromTF(m.AppleMusic.AllowMusicService),
		}
	}
	if m.Camera != nil {
		out.Camera = &sdk.AppleOsXRestrictionCameraPayloadEntityV2{
			AllowUseOfBuiltInCamera: boolPtrFromTF(m.Camera.AllowUseOfBuiltInCamera),
		}
	}
	if m.GameCentre != nil {
		out.GameCentre = &sdk.AppleOsXRestrictionGameCentrePayloadEntityV2{
			AllowAddingGameCenterFriends: boolPtrFromTF(m.GameCentre.AllowAddingGameCenterFriends),
			AllowGameCenterModification:  boolPtrFromTF(m.GameCentre.AllowGameCenterModification),
			AllowMultiplayerGaming:       boolPtrFromTF(m.GameCentre.AllowMultiplayerGaming),
			AllowUseOfGameCenter:         boolPtrFromTF(m.GameCentre.AllowUseOfGameCenter),
		}
	}
	if m.Safari != nil {
		out.Safari = &sdk.AppleOsXRestrictionSafariPayloadEntityV2{
			AllowDeprecatedWebKitTls: boolPtrFromTF(m.Safari.AllowDeprecatedWebKitTls),
			AllowSafariAutoFill:      boolPtrFromTF(m.Safari.AllowSafariAutoFill),
		}
	}
	return out
}

func buildAppleOsXRestrictionsDesktopEntity(m *profilemodels.RestrictionsDesktopModel) *sdk.AppleOsXRestrictionDesktopPayloadEntityV2 {
	out := &sdk.AppleOsXRestrictionDesktopPayloadEntityV2{
		LockDesktopPicture: boolPtrFromTF(m.LockDesktopPicture),
	}
	setStringIfKnown(&out.DesktopPicturePath, m.DesktopPicturePath)
	return out
}

func buildAppleOsXRestrictionsFunctionalityEntity(m *profilemodels.RestrictionsFunctionalityModel) *sdk.AppleOsXRestrictionFunctionalityPayloadEntityV2 {
	out := &sdk.AppleOsXRestrictionFunctionalityPayloadEntityV2{}
	if m.AirPrint != nil {
		out.AirPrint = &sdk.AppleOsXRestrictionAirPrintPayloadEntityV2{
			AllowAirPrint:                      boolPtrFromTF(m.AirPrint.AllowAirPrint),
			AllowAirPrintiBeaconDiscovery:      boolPtrFromTF(m.AirPrint.AllowAirPrintiBeaconDiscovery),
			ForceAirPrintTrustedTLSRequirement: boolPtrFromTF(m.AirPrint.ForceAirPrintTrustedTLSRequirement),
		}
	}
	if m.ContentCaching != nil {
		out.ContentCaching = &sdk.AppleOsXRestrictionContentCachingPayloadEntityV2{
			AllowContentCaching: boolPtrFromTF(m.ContentCaching.AllowContentCaching),
		}
	}
	if m.ICloud != nil {
		out.ICloud = &sdk.AppleOsXRestrictionICloudPayloadEntityV2{
			AllowAirPrint:                          boolPtrFromTF(m.ICloud.AllowAirPrint),
			AllowAirPrintiBeaconDiscovery:          boolPtrFromTF(m.ICloud.AllowAirPrintiBeaconDiscovery),
			AllowCloudDesktopAndDocuments:          boolPtrFromTF(m.ICloud.AllowCloudDesktopAndDocuments),
			AllowDeprecatedWebKitTls:               boolPtrFromTF(m.ICloud.AllowDeprecatedWebKitTls),
			AllowICloudFMM:                         boolPtrFromTF(m.ICloud.AllowICloudFMM),
			AllowIcloudAddressBook:                 boolPtrFromTF(m.ICloud.AllowIcloudAddressBook),
			AllowIcloudBTMM:                        boolPtrFromTF(m.ICloud.AllowIcloudBTMM),
			AllowIcloudBookmarks:                   boolPtrFromTF(m.ICloud.AllowIcloudBookmarks),
			AllowIcloudCalendar:                    boolPtrFromTF(m.ICloud.AllowIcloudCalendar),
			AllowIcloudDocumentsAndData:            boolPtrFromTF(m.ICloud.AllowIcloudDocumentsAndData),
			AllowIcloudKeychainSync:                boolPtrFromTF(m.ICloud.AllowIcloudKeychainSync),
			AllowIcloudMail:                        boolPtrFromTF(m.ICloud.AllowIcloudMail),
			AllowIcloudNotes:                       boolPtrFromTF(m.ICloud.AllowIcloudNotes),
			AllowIcloudReminders:                   boolPtrFromTF(m.ICloud.AllowIcloudReminders),
			AllowPasswordAutoFill:                  boolPtrFromTF(m.ICloud.AllowPasswordAutoFill),
			AllowPasswordProximityRequests:         boolPtrFromTF(m.ICloud.AllowPasswordProximityRequests),
			AllowPasswordSharing:                   boolPtrFromTF(m.ICloud.AllowPasswordSharing),
			AllowUseIcloudPasswordForLocalAccounts: boolPtrFromTF(m.ICloud.AllowUseIcloudPasswordForLocalAccounts),
			ForceAirPrintTrustedTLSRequirement:     boolPtrFromTF(m.ICloud.ForceAirPrintTrustedTLSRequirement),
		}
	}
	if m.Passwords != nil {
		out.Passwords = &sdk.AppleOsXRestrictionPasswordsPayloadEntityV2{
			AllowPasswordAutoFill:          boolPtrFromTF(m.Passwords.AllowPasswordAutoFill),
			AllowPasswordProximityRequests: boolPtrFromTF(m.Passwords.AllowPasswordProximityRequests),
			AllowPasswordSharing:           boolPtrFromTF(m.Passwords.AllowPasswordSharing),
		}
	}
	if m.Spotlight != nil {
		out.Spotlight = &sdk.AppleOsXRestrictionSpotlightPayloadEntityV2{
			AllowSpotlightSuggestions: boolPtrFromTF(m.Spotlight.AllowSpotlightSuggestions),
		}
	}
	return out
}

func buildAppleOsXMediaAccessEntity(m *profilemodels.RestrictionsMediaAccessModel) *sdk.AppleOsXMediaAccessEntityV2 {
	if m == nil {
		return nil
	}
	return &sdk.AppleOsXMediaAccessEntityV2{
		Allow:        boolPtrFromTF(m.Allow),
		Authenticate: boolPtrFromTF(m.Authenticate),
		ReadOnly:     boolPtrFromTF(m.ReadOnly),
	}
}

func buildAppleOsXRestrictionsMediaEntity(m *profilemodels.RestrictionsMediaModel) *sdk.AppleOsXRestrictionMediaPayloadEntityV2 {
	out := &sdk.AppleOsXRestrictionMediaPayloadEntityV2{
		AutoEjectMedia:              boolPtrFromTF(m.AutoEjectMedia),
		DiskMediaCDs:                buildAppleOsXMediaAccessEntity(m.DiskMediaCDs),
		DiskMediaDVDs:               buildAppleOsXMediaAccessEntity(m.DiskMediaDVDs),
		ExternalHardDiskMediaAccess: buildAppleOsXMediaAccessEntity(m.ExternalHardDiskMediaAccess),
		HardDiskDvdRam:              buildAppleOsXMediaAccessEntity(m.HardDiskDvdRam),
		HardDiskImages:              buildAppleOsXMediaAccessEntity(m.HardDiskImages),
		InternalHardDiskMediaAccess: buildAppleOsXMediaAccessEntity(m.InternalHardDiskMediaAccess),
	}
	if m.NetworkAccess != nil {
		out.NetworkAccess = &sdk.AppleOsXNetworkAccessAirDropPayloadEntityV2{
			AirDrop: boolPtrFromTF(m.NetworkAccess.AirDrop),
		}
	}
	if m.RecordableDisc != nil {
		out.RecordableDisc = &sdk.AppleOsXRestrictionBurnsupportPayloadEntityV2{
			BurnSupport: buildAppleOsXMediaAccessEntity(m.RecordableDisc.BurnSupport),
		}
	}
	return out
}

func buildAppleOsXRestrictionsPreferencesEntity(m *profilemodels.RestrictionsPreferencesModel) *sdk.AppleOsXRestrictionPreferencesPayloadEntityV2 {
	out := &sdk.AppleOsXRestrictionPreferencesPayloadEntityV2{
		Accessibility:          boolPtrFromTF(m.Accessibility),
		AppStore:               boolPtrFromTF(m.AppStore),
		Bluetooth:              boolPtrFromTF(m.Bluetooth),
		CDsAndDVDs:             boolPtrFromTF(m.CDsAndDVDs),
		DateAndTime:            boolPtrFromTF(m.DateAndTime),
		DesktopAndScreenSaver:  boolPtrFromTF(m.DesktopAndScreenSaver),
		DictationAndSpeech:     boolPtrFromTF(m.DictationAndSpeech),
		Displays:               boolPtrFromTF(m.Displays),
		Dock:                   boolPtrFromTF(m.Dock),
		EnabledPreferencePanes: boolPtrFromTF(m.EnabledPreferencePanes),
		EnergySaver:            boolPtrFromTF(m.EnergySaver),
		Extensions:             boolPtrFromTF(m.Extensions),
		FibreChannel:           boolPtrFromTF(m.FibreChannel),
		FlashPlayer:            boolPtrFromTF(m.FlashPlayer),
		General:                boolPtrFromTF(m.General),
		Ink:                    boolPtrFromTF(m.Ink),
		InternetAccounts:       boolPtrFromTF(m.InternetAccounts),
		Keyboard:               boolPtrFromTF(m.Keyboard),
		LanguageAndText:        boolPtrFromTF(m.LanguageAndText),
		MissionControl:         boolPtrFromTF(m.MissionControl),
		MobileMe:               boolPtrFromTF(m.MobileMe),
		Mouse:                  boolPtrFromTF(m.Mouse),
		Network:                boolPtrFromTF(m.Network),
		Notifications:          boolPtrFromTF(m.Notifications),
		ParentalControls:       boolPtrFromTF(m.ParentalControls),
		PrintAndScan:           boolPtrFromTF(m.PrintAndScan),
		Profiles:               boolPtrFromTF(m.Profiles),
		SecurityAndPrivacy:     boolPtrFromTF(m.SecurityAndPrivacy),
		Sharing:                boolPtrFromTF(m.Sharing),
		SoftwareUpdate:         boolPtrFromTF(m.SoftwareUpdate),
		Sound:                  boolPtrFromTF(m.Sound),
		Spotlight:              boolPtrFromTF(m.Spotlight),
		StartupDisk:            boolPtrFromTF(m.StartupDisk),
		TimeMachine:            boolPtrFromTF(m.TimeMachine),
		Trackpad:               boolPtrFromTF(m.Trackpad),
		UsersAndGroups:         boolPtrFromTF(m.UsersAndGroups),
		Xsan:                   boolPtrFromTF(m.Xsan),
		ICloud:                 boolPtrFromTF(m.ICloud),
	}
	setStringIfKnown(&out.PreferenceBehavior, m.PreferenceBehavior)
	return out
}

func buildAppleOsXRestrictionsSharingEntity(m *profilemodels.RestrictionsSharingModel) *sdk.AppleOsXRestrictionSharingPayloadEntityV2 {
	return &sdk.AppleOsXRestrictionSharingPayloadEntityV2{
		AddtoAperture:                          boolPtrFromTF(m.AddtoAperture),
		AddtoReadingList:                       boolPtrFromTF(m.AddtoReadingList),
		AddtoiPhoto:                            boolPtrFromTF(m.AddtoiPhoto),
		AirDrop:                                boolPtrFromTF(m.AirDrop),
		AutomaticallyEnableNewSharingServices:  boolPtrFromTF(m.AutomaticallyEnableNewSharingServices),
		Facebook:                               boolPtrFromTF(m.Facebook),
		Mail:                                   boolPtrFromTF(m.Mail),
		Messages:                               boolPtrFromTF(m.Messages),
		RestrictWhichSharingServicesAreEnabled: boolPtrFromTF(m.RestrictWhichSharingServicesAreEnabled),
		SinaWeibo:                              boolPtrFromTF(m.SinaWeibo),
		Twitter:                                boolPtrFromTF(m.Twitter),
		VideoServices:                          boolPtrFromTF(m.VideoServices),
	}
}

func buildAppleOsXRestrictionsWidgetsEntity(m *profilemodels.RestrictionsWidgetsModel) *sdk.AppleOsXRestrictionWidgetPayloadEntityV2 {
	return &sdk.AppleOsXRestrictionWidgetPayloadEntityV2{
		AllowOnlyConfiguredWidgets: boolPtrFromTF(m.AllowOnlyConfiguredWidgets),
		AllowedWidgets:             stringSliceFromTFList(m.AllowedWidgets),
	}
}

func buildAppleOsXGatekeeperEntity(m *profilemodels.GatekeeperModel) *sdk.MacOsGatekeeperPayloadV2Entity {
	out := &sdk.MacOsGatekeeperPayloadV2Entity{}
	out.AllowAutoUnlock = boolPtrFromTF(m.AllowAutoUnlock)
	out.AllowFingerprintForUnlock = boolPtrFromTF(m.AllowFingerprintForUnlock)
	out.AllowHandoff = boolPtrFromTF(m.AllowHandoff)
	out.AllowScreenCapture = boolPtrFromTF(m.AllowScreenCapture)
	out.EnableAppSoftwareUpdateDelay = boolPtrFromTF(m.EnableAppSoftwareUpdateDelay)
	out.EnableSoftwareUpdateDelay = boolPtrFromTF(m.EnableSoftwareUpdateDelay)
	if !m.EnforcedSoftwareUpdateDelay.IsNull() && !m.EnforcedSoftwareUpdateDelay.IsUnknown() {
		out.EnforcedSoftwareUpdateDelay = sdk.IntPtr(int(m.EnforcedSoftwareUpdateDelay.ValueInt64()))
	}
	return out
}
