package platform

import (
	"fmt"
	"strconv"
	"strings"

	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
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
	if data.SystemExtensions != nil {
		ent.SystemExtensions = buildAppleOsXSystemExtensionsEntity(data.SystemExtensions)
	}
	if items := buildAppleOsXScepListEntity(data.ScepList); len(items) > 0 {
		ent.ScepList = items
	}
	if items := buildAppleOsXWebClipsListEntity(data.WebClipsList); len(items) > 0 {
		ent.WebClipsList = items
	}
	if items := buildVPNItemList(data.VpnList); len(items) > 0 {
		ent.VpnList = items
	}
	if data.EasMicrosoftOutlook != nil {
		e := buildEasMicrosoftOutlook(data.EasMicrosoftOutlook)
		ent.EasMicrosoftOutlook = &e
	}
	if items := buildCustomAttributeList(data.CustomAttributes); len(items) > 0 {
		ent.CustomAttributes = items
	}
	if data.KernelExtension != nil {
		e := buildKernelExtension(data.KernelExtension)
		ent.KernelExtension = &e
	}
	if identities := buildAppleOsXPrivacyPreferencesIdentities(data.PrivacyPreferences); len(identities) > 0 {
		ent.PrivacyPreferences = &sdk.MacOsPrivacyPreferencesPayloadV2Model{Identities: identities}
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
	// ManagedLocationGroupID is non-nullable on every write (canonical OGID
	// answer); requiring or resolving it when unset is handled separately.
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

func BuildLinuxCreateEntity(data *profilemodels.ProfileResourceModel) (*sdk.LinuxDeviceProfileEntity1V4, error) {
	general, err := BuildGeneralV4Create(data)
	if err != nil {
		return nil, err
	}
	return &sdk.LinuxDeviceProfileEntity1V4{General: general}, nil
}

// profileScopeWireValues maps the canonical (case-normalized) profile_scope
// name to the wire integer value expected by the V4 (Linux) API. "Production"
// -> 1 is directly evidenced by request-template testdata
// (wifi-create-v4.json / wifi-update-v4.json and api_validation.notes:
// "General.ProfileScope must be an integer (1)") AND live-confirmed against
// as<internal-env>.eng.example.com (26.2 tenant) during internal-task. "Staging" -> 2 and
// "Both" -> 3 follow the confirmed canonical enum ordering
// (Production, Staging, Both) from the C# server source
// (DeviceProfileScope, release/26.2.0.0), sourced from doctrine-quirk-17 —
// but are UNVERIFIED live: no Linux profile existed in the accessible org
// group scope (138883) at the time of this check to confirm the Staging/Both
// wire ints against a real server response. Only Production=1 is
// live-confirmed; Staging=2/Both=3 rest on the C# source mapping only.
var profileScopeWireValues = map[string]int{
	"production": 1,
	"staging":    2,
	"both":       3,
}

// profileScopeWireValue maps the TF profile_scope string (case-insensitive)
// to the wire integer value expected by the V4 (Linux) API. An empty scope
// (unconfigured, planned unknown on create or carried forward as "" on
// update) returns (nil, nil): GeneralPayloadV4Entity.
// ProfileScope is a json:"omitempty" *int, so a nil pointer omits the field
// from the wire entirely, letting UEM apply its own default instead of this
// provider guessing one (see the matching V2 comment in resource_crud.go,
// live-confirmed 2026-09-25 on the 26.2 lab tenant, guarded B16 follow-up
// create). Any other value not in profileScopeWireValues is an explicit
// configuration error rather than a guessed mapping.
func profileScopeWireValue(scope string) (*int, error) {
	if scope == "" {
		return nil, nil
	}
	if v, ok := profileScopeWireValues[strings.ToLower(scope)]; ok {
		return sdk.IntPtr(v), nil
	}
	return nil, fmt.Errorf("profile_scope %q has no known wire mapping for Linux profiles: only \"Production\", \"Staging\", or \"Both\" are confirmed to map to a wire value", scope)
}

func BuildGeneralV4Create(data *profilemodels.ProfileResourceModel) (*sdk.GeneralPayloadV4Entity, error) {
	profileScope, err := profileScopeWireValue(data.ProfileScope.ValueString())
	if err != nil {
		return nil, err
	}
	g := &sdk.GeneralPayloadV4Entity{
		Name:           data.Name.ValueString(),
		Description:    data.Description.ValueString(),
		AssignmentType: data.AssignmentType.ValueString(),
		ProfileScope:   profileScope,
	}
	if !data.IsActive.IsNull() && !data.IsActive.IsUnknown() {
		g.IsActive = sdk.BoolPtr(data.IsActive.ValueBool())
	}
	if !data.ProfileContext.IsNull() && !data.ProfileContext.IsUnknown() && data.ProfileContext.ValueString() != "" {
		g.ProfileContext = data.ProfileContext.ValueString()
	}
	// ManagedLocationGroupID is non-nullable on every write (canonical OGID
	// answer); requiring or resolving it when unset is handled separately.
	if !data.OrgGroupID.IsNull() && !data.OrgGroupID.IsUnknown() {
		if n, err := strconv.Atoi(data.OrgGroupID.ValueString()); err == nil {
			g.ManagedLocationGroupID = sdk.IntPtr(n)
		}
	}
	return g, nil
}

// StampGeneralV2 always forces CreateNewVersion=true and bumps Version on
// every update. UEM source: AirWatch API/AW.Mdm.Api/AW.Mdm.Api/Helper/Profiles/ProfileServiceV2Helper.cs:1035-1041,3443-3448,1650-1654
// (canonical Q6) confirms CreateNewVersion's two semantics generally
// (true = rebuild from request only, omitted/null lists replace/clear;
// false = keep existing entity, General-only properties update, credentials
// and network lists left unchanged) and that this provider's update path
// always chooses the "true" (rebuild) semantics — matching
// OverlayAppleOsXUpdateEntity's full live-entity + plan overlay strategy in
// resource_update_osx.go — but does not itself require forcing true on
// every update rather than sometimes taking the false (General-only) path;
// this row is only partially answered.
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
	if mcx := buildAppleOsXDiskEncryptionMCXEntity(m.MCX); mcx != nil {
		out.DiskEncryptionMCX = mcx
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
		// MacOsEncryptionAction also carries StringEnumConverter (canonical
		// rules Q3/Q4): send the enum name string, not the raw int.
		if name, ok := encryptionActionAfterLastNotificationNames[m.EncryptionActionAfterLastNotification.ValueInt64()]; ok {
			out.EncryptionActionAfterLastNotification = name
		}
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

// fileVaultUserNames maps the schema's Int64 filevault_user encoding to the
// exact MacOsFileVaultUser enum name string UEM's StringEnumConverter
// expects on the wire (canonical rules Q3/Q4). RecoveryType stays a plain
// int both ways (Q3: "RecoveryType is int, not an enum") and is NOT in this
// map.
var fileVaultUserNames = map[int64]string{
	1: "CurrentOrNextLoginUser",
	2: "SpecificUser",
}

// promptToEnableFileVaultAtNames maps the schema's Int64
// prompt_to_enable_filevault_at encoding to the exact
// MacOsTimeOfPromptToEnableFileVault enum name string (canonical rules
// Q3/Q4).
var promptToEnableFileVaultAtNames = map[int64]string{
	1: "BothLoginAndLogout",
	2: "LogoutOnly",
	3: "LoginOnly",
}

// encryptionActionAfterLastNotificationNames maps the schema's Int64
// encryption_action_after_last_notification encoding to the exact
// MacOsEncryptionAction enum name string (canonical rules Q3/Q4).
var encryptionActionAfterLastNotificationNames = map[int64]string{
	1: "ForceLogout",
	2: "DoNothing",
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
		// UEM's FileVaultUser property carries [JsonConverter(typeof(StringEnumConverter))],
		// so the wire format is the enum name string, not the schema's raw int
		// (canonical rules Q3/Q4). An out-of-range value can't reach here: the
		// schema's int64validator.OneOf(1, 2) rejects it at plan time.
		if name, ok := fileVaultUserNames[m.FileVaultUser.ValueInt64()]; ok {
			out.FileVaultUser = name
		}
	}
	setStringIfKnown(&out.Username, m.Username)
	if !m.PromptToEnableFileVaultAt.IsNull() && !m.PromptToEnableFileVaultAt.IsUnknown() {
		// Same StringEnumConverter treatment as FileVaultUser above.
		if name, ok := promptToEnableFileVaultAtNames[m.PromptToEnableFileVaultAt.ValueInt64()]; ok {
			out.PromptToEnableFileVaultAt = name
		}
	}
	if !m.NumberOfTimesUserCanBypass.IsNull() && !m.NumberOfTimesUserCanBypass.IsUnknown() {
		out.NumberOfTimesUserCanBypass = sdk.IntPtr(int(m.NumberOfTimesUserCanBypass.ValueInt64()))
	}
	return out
}

// buildAppleOsXDiskEncryptionMCXEntity builds the DiskEncryptionMCX request
// entity from the mcx types.Object. A null or unknown object (config omitted
// mcx, including create's genuinely-unknown plan value -- see
// DiskEncryptionModel.MCX's doc comment) means omit DiskEncryptionMCX from
// the request entirely and let UEM apply its own default (B42 follow-up,
// live-confirmed as<internal-env> UEM 26.2). A known object is built from its leaf
// value; an unknown or null leaf is likewise omitted from the sub-entity so
// UEM fills its own default for that one field.
func buildAppleOsXDiskEncryptionMCXEntity(o types.Object) *sdk.AppleOsXDiskEncryptionMCXPayloadEntityV2 {
	if o.IsNull() || o.IsUnknown() {
		return nil
	}
	destroy, ok := o.Attributes()["destroy_fv_key_on_standby"].(types.Bool)
	if !ok {
		destroy = types.BoolNull()
	}
	return &sdk.AppleOsXDiskEncryptionMCXPayloadEntityV2{
		DestroyFVKeyOnStandby: boolPtrFromTF(destroy),
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

func buildAppleOsXSystemExtensionsEntity(m *profilemodels.SystemExtensionsModel) *sdk.MacOsSystemExtensionsPayloadV2Model {
	out := &sdk.MacOsSystemExtensionsPayloadV2Model{}
	out.AllowUserOverrides = boolPtrFromTF(m.AllowUserOverrides)
	if len(m.AllowedSystemExtensionTypes) > 0 {
		allowedTypes := make([]sdk.MacOsAllowedSystemExtensionTypesV2Model, 0, len(m.AllowedSystemExtensionTypes))
		for _, t := range m.AllowedSystemExtensionTypes {
			var item sdk.MacOsAllowedSystemExtensionTypesV2Model
			setStringIfKnown(&item.TeamIdentifier, t.TeamIdentifier)
			item.AllowDriverExtensionType = boolPtrFromTF(t.AllowDriverExtensionType)
			item.AllowEndpointSecurityExtensionType = boolPtrFromTF(t.AllowEndpointSecurityExtensionType)
			item.AllowNetworkExtensionType = boolPtrFromTF(t.AllowNetworkExtensionType)
			allowedTypes = append(allowedTypes, item)
		}
		out.AllowedSystemExtensionTypes = allowedTypes
	}
	if len(m.AllowedSystemExtensions) > 0 {
		exts := make([]sdk.MacOsAllowedSystemExtensionV2Model, 0, len(m.AllowedSystemExtensions))
		for _, e := range m.AllowedSystemExtensions {
			var item sdk.MacOsAllowedSystemExtensionV2Model
			setStringIfKnown(&item.BundleIdentifier, e.BundleIdentifier)
			setStringIfKnown(&item.TeamIdentifier, e.TeamIdentifier)
			exts = append(exts, item)
		}
		out.AllowedSystemExtensions = exts
	}
	return out
}

func buildAppleOsXPrivacyPreferencesIdentities(items []profilemodels.PrivacyPreferenceModel) []sdk.MacOsPrivacyPreferencesV2Model {
	if len(items) == 0 {
		return nil
	}
	result := make([]sdk.MacOsPrivacyPreferencesV2Model, 0, len(items))
	for _, m := range items {
		result = append(result, buildAppleOsXPrivacyPreferenceIdentity(&m))
	}
	return result
}

func buildAppleOsXPrivacyPreferenceIdentity(m *profilemodels.PrivacyPreferenceModel) sdk.MacOsPrivacyPreferencesV2Model {
	var it sdk.MacOsPrivacyPreferencesV2Model
	setStringIfKnown(&it.Identifier, m.Identifier)
	setStringIfKnown(&it.IdentifierType, m.IdentifierType)
	setStringIfKnown(&it.CodeRequirement, m.CodeRequirement)
	setStringIfKnown(&it.Comment, m.Comment)
	// AEReceiverIdentifier/AEReceiverIdentifierType/AEReceiverCodeRequirement
	// and the coarse-grained AppleEvents field are never set here: UEM folds
	// an identity-level Apple Events receiver into AppleEventsList on write
	// (storing 2 identical entries when both forms are sent) and never
	// repopulates the identity-level fields on read (canonical answer from
	// the SDK team, MacOsPrivacyPreferencesV2Model.cs:274-283). Only
	// AppleEventsList is supported.
	if events := buildAppleEventsList(m.AppleEventsList); len(events) > 0 {
		it.AppleEventsList = events
	}
	it.StaticCode = boolPtrFromTF(m.StaticCode)
	setStringIfKnown(&it.Accessibility, m.Accessibility)
	setStringIfKnown(&it.AddressBook, m.AddressBook)
	setStringIfKnown(&it.Calendar, m.Calendar)
	setStringIfKnown(&it.Camera, m.Camera)
	setStringIfKnown(&it.FileProviderPresence, m.FileProviderPresence)
	setStringIfKnown(&it.ListenEvent, m.ListenEvent)
	setStringIfKnown(&it.MediaLibrary, m.MediaLibrary)
	setStringIfKnown(&it.Microphone, m.Microphone)
	setStringIfKnown(&it.Photos, m.Photos)
	setStringIfKnown(&it.PostEvent, m.PostEvent)
	setStringIfKnown(&it.Reminders, m.Reminders)
	setStringIfKnown(&it.ScreenCapture, m.ScreenCapture)
	setStringIfKnown(&it.SpeechRecognition, m.SpeechRecognition)
	setStringIfKnown(&it.SystemPolicyAllFiles, m.SystemPolicyAllFiles)
	setStringIfKnown(&it.SystemPolicyDesktopFolder, m.SystemPolicyDesktopFolder)
	setStringIfKnown(&it.SystemPolicyDocumentsFolder, m.SystemPolicyDocumentsFolder)
	setStringIfKnown(&it.SystemPolicyDownloadsFolder, m.SystemPolicyDownloadsFolder)
	setStringIfKnown(&it.SystemPolicyNetworkVolumes, m.SystemPolicyNetworkVolumes)
	setStringIfKnown(&it.SystemPolicyRemovableVolumes, m.SystemPolicyRemovableVolumes)
	setStringIfKnown(&it.SystemPolicySysAdminFiles, m.SystemPolicySysAdminFiles)
	return it
}

func buildAppleEventsList(items []profilemodels.AppleEventModel) []sdk.AppleEventV2 {
	if len(items) == 0 {
		return nil
	}
	result := make([]sdk.AppleEventV2, 0, len(items))
	for _, e := range items {
		var item sdk.AppleEventV2
		setStringIfKnown(&item.CodeRequirement, e.CodeRequirement)
		setStringIfKnown(&item.Identifier, e.Identifier)
		setStringIfKnown(&item.IdentifierType, e.IdentifierType)
		setStringIfKnown(&item.Permission, e.Permission)
		result = append(result, item)
	}
	return result
}

func buildAppleOsXScepListEntity(items []profilemodels.ScepItemModel) []sdk.AppleOsXScepPayloadEntityV2 {
	if len(items) == 0 {
		return nil
	}
	result := make([]sdk.AppleOsXScepPayloadEntityV2, 0, len(items))
	for i := range items {
		it := &items[i]
		var out sdk.AppleOsXScepPayloadEntityV2
		setStringIfKnown(&out.Name, it.Name)
		setStringIfKnown(&out.CredentialSource, it.CredentialSource)
		if !it.CertificateAuthorityID.IsNull() && !it.CertificateAuthorityID.IsUnknown() {
			out.CertificateAuthorityID = sdk.IntPtr(int(it.CertificateAuthorityID.ValueInt64()))
		}
		if !it.CertificateTemplateID.IsNull() && !it.CertificateTemplateID.IsUnknown() {
			out.CertificateTemplateID = sdk.IntPtr(int(it.CertificateTemplateID.ValueInt64()))
		}
		out.AllowExportFromKeyChain = boolPtrFromTF(it.AllowExportFromKeyChain)
		if it.IdentityPreference != nil {
			out.IdentityPreference = &sdk.MacOsScepIdentityPreferencePayloadV2Model{Names: stringSliceFromTFList(it.IdentityPreference.Names)}
		}
		result = append(result, out)
	}
	return result
}

func buildAppleOsXWebClipsListEntity(items []profilemodels.WebClipItemModel) []sdk.MacOsWebClipsPayloadV2Entity {
	if len(items) == 0 {
		return nil
	}
	result := make([]sdk.MacOsWebClipsPayloadV2Entity, 0, len(items))
	for i := range items {
		it := &items[i]
		var out sdk.MacOsWebClipsPayloadV2Entity
		setStringIfKnown(&out.Label, it.Label)
		setStringIfKnown(&out.URL, it.URL)
		out.ShowInAppCatalog = boolPtrFromTF(it.ShowInAppCatalog)
		if !it.Icon.IsNull() && !it.Icon.IsUnknown() {
			out.Icon = sdk.IntPtr(int(it.Icon.ValueInt64()))
		}
		result = append(result, out)
	}
	return result
}
