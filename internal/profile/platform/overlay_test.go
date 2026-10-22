package platform

// zf1 part 2: mutation-closing tests for the RMW overlay functions in
// overlay.go.
//
// These are direct, package-internal table tests against the unexported
// overlayGeneralV2Fields/overlayGeneralV4Fields helpers and the exported
// Overlay*UpdateEntity functions. Each table below has two rows:
//
//   "clear"  - live has every modeled field/section set to a non-zero
//              value, planned has them all nil/zero -> the result must
//              clear (nil/zero) every modeled field/section.
//   "change" - live has one set of values, planned has a DIFFERENT set of
//              non-zero values -> the result must equal planned's values,
//              not live's.
//
// Both rows also assert at least one unmodeled field/section from live
// survives untouched, so a mutation that stops overlaying (assigns from
// live instead of planned) or a mutation that drops an assignment line
// entirely both fail a table row.

import (
	"reflect"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

func intPtr(i int) *int { return &i }

// --- overlayGeneralV2Fields: every modeled field, including smart groups ---

func TestOverlayGeneralV2Fields_ModeledFields(t *testing.T) {
	t.Parallel()

	liveGeneral := func() *sdk.GeneralPayloadV2Entity {
		return &sdk.GeneralPayloadV2Entity{
			Name:                   "Live Name",
			Description:            "Live Description",
			AssignmentType:         "Manual",
			ProfileScope:           "Staging",
			AssignedSmartGroups:    []sdk.SmartGroupEntityV2{{Name: "live-asg", SmartGroupID: intPtr(1)}},
			ExcludedSmartGroups:    []sdk.SmartGroupEntityV2{{Name: "live-esg", SmartGroupID: intPtr(2)}},
			IsActive:               boolPtr(true),
			ProfileContext:         "Device",
			ManagedLocationGroupID: intPtr(10),
			// unmodeled - must survive every row unchanged.
			Password: "live-password",
		}
	}

	t.Run("clear: planned nil/zero clears every modeled field, unmodeled survives", func(t *testing.T) {
		t.Parallel()
		live := liveGeneral()
		planned := &sdk.GeneralPayloadV2Entity{}

		out := overlayGeneralV2Fields(live, planned)

		if out.Name != "" {
			t.Errorf("Name = %q, want cleared", out.Name)
		}
		if out.Description != "" {
			t.Errorf("Description = %q, want cleared", out.Description)
		}
		if out.AssignmentType != "" {
			t.Errorf("AssignmentType = %q, want cleared", out.AssignmentType)
		}
		if out.ProfileScope != "" {
			t.Errorf("ProfileScope = %q, want cleared", out.ProfileScope)
		}
		if out.AssignedSmartGroups != nil {
			t.Errorf("AssignedSmartGroups = %+v, want nil (cleared)", out.AssignedSmartGroups)
		}
		if out.ExcludedSmartGroups != nil {
			t.Errorf("ExcludedSmartGroups = %+v, want nil (cleared)", out.ExcludedSmartGroups)
		}
		if out.IsActive != nil {
			t.Errorf("IsActive = %v, want nil (cleared)", out.IsActive)
		}
		if out.ProfileContext != "" {
			t.Errorf("ProfileContext = %q, want cleared", out.ProfileContext)
		}
		if out.ManagedLocationGroupID != nil {
			t.Errorf("ManagedLocationGroupID = %v, want nil (cleared)", out.ManagedLocationGroupID)
		}
		if out.Password != "live-password" {
			t.Errorf("unmodeled Password = %q, want survive as %q", out.Password, "live-password")
		}
	})

	t.Run("change: planned's different non-zero values win, unmodeled survives", func(t *testing.T) {
		t.Parallel()
		live := liveGeneral()
		planned := &sdk.GeneralPayloadV2Entity{
			Name:                   "Planned Name",
			Description:            "Planned Description",
			AssignmentType:         "Auto",
			ProfileScope:           "Production",
			AssignedSmartGroups:    []sdk.SmartGroupEntityV2{{Name: "planned-asg", SmartGroupID: intPtr(3)}},
			ExcludedSmartGroups:    []sdk.SmartGroupEntityV2{{Name: "planned-esg", SmartGroupID: intPtr(4)}},
			IsActive:               boolPtr(false),
			ProfileContext:         "User",
			ManagedLocationGroupID: intPtr(20),
		}

		out := overlayGeneralV2Fields(live, planned)

		if out.Name != "Planned Name" {
			t.Errorf("Name = %q, want %q", out.Name, "Planned Name")
		}
		if out.Description != "Planned Description" {
			t.Errorf("Description = %q, want %q", out.Description, "Planned Description")
		}
		if out.AssignmentType != "Auto" {
			t.Errorf("AssignmentType = %q, want %q", out.AssignmentType, "Auto")
		}
		if out.ProfileScope != "Production" {
			t.Errorf("ProfileScope = %q, want %q", out.ProfileScope, "Production")
		}
		if !reflect.DeepEqual(out.AssignedSmartGroups, planned.AssignedSmartGroups) {
			t.Errorf("AssignedSmartGroups = %+v, want %+v", out.AssignedSmartGroups, planned.AssignedSmartGroups)
		}
		if !reflect.DeepEqual(out.ExcludedSmartGroups, planned.ExcludedSmartGroups) {
			t.Errorf("ExcludedSmartGroups = %+v, want %+v", out.ExcludedSmartGroups, planned.ExcludedSmartGroups)
		}
		if out.IsActive == nil || *out.IsActive != false {
			t.Errorf("IsActive = %v, want false", out.IsActive)
		}
		if out.ProfileContext != "User" {
			t.Errorf("ProfileContext = %q, want %q", out.ProfileContext, "User")
		}
		if out.ManagedLocationGroupID == nil || *out.ManagedLocationGroupID != 20 {
			t.Errorf("ManagedLocationGroupID = %v, want 20", out.ManagedLocationGroupID)
		}
		if out.Password != "live-password" {
			t.Errorf("unmodeled Password = %q, want survive as %q", out.Password, "live-password")
		}
	})
}

// --- overlayGeneralV4Fields: every modeled field; smart groups NOT overlaid ---

func TestOverlayGeneralV4Fields_ModeledFields(t *testing.T) {
	t.Parallel()

	liveGeneral := func() *sdk.GeneralPayloadV4Entity {
		return &sdk.GeneralPayloadV4Entity{
			Name:                   "Live Name",
			Description:            "Live Description",
			AssignmentType:         "Manual",
			ProfileScope:           intPtr(1),
			IsActive:               boolPtr(true),
			ProfileContext:         "Device",
			ManagedLocationGroupID: intPtr(10),
			// unmodeled by V4 overlay - must survive every row unchanged.
			Password:            "live-password",
			AssignedSmartGroups: []sdk.SmartGroupEntity1V4{{Name: "live-asg", SmartGroupID: intPtr(1)}},
		}
	}

	t.Run("clear: planned nil/zero clears every modeled field, unmodeled (incl. smart groups) survives", func(t *testing.T) {
		t.Parallel()
		live := liveGeneral()
		planned := &sdk.GeneralPayloadV4Entity{}

		out := overlayGeneralV4Fields(live, planned)

		if out.Name != "" {
			t.Errorf("Name = %q, want cleared", out.Name)
		}
		if out.Description != "" {
			t.Errorf("Description = %q, want cleared", out.Description)
		}
		if out.AssignmentType != "" {
			t.Errorf("AssignmentType = %q, want cleared", out.AssignmentType)
		}
		if out.ProfileScope != nil {
			t.Errorf("ProfileScope = %v, want cleared", out.ProfileScope)
		}
		if out.IsActive != nil {
			t.Errorf("IsActive = %v, want nil (cleared)", out.IsActive)
		}
		if out.ProfileContext != "" {
			t.Errorf("ProfileContext = %q, want cleared", out.ProfileContext)
		}
		if out.ManagedLocationGroupID != nil {
			t.Errorf("ManagedLocationGroupID = %v, want nil (cleared)", out.ManagedLocationGroupID)
		}
		if out.Password != "live-password" {
			t.Errorf("unmodeled Password = %q, want survive", out.Password)
		}
		// V4 General does not model smart groups: BuildGeneralV4Create never
		// populates them, so overlayGeneralV4Fields must leave live's value
		// untouched even though planned has none.
		if len(out.AssignedSmartGroups) != 1 || out.AssignedSmartGroups[0].Name != "live-asg" {
			t.Errorf("AssignedSmartGroups = %+v, want unmodeled survive as live's value", out.AssignedSmartGroups)
		}
	})

	t.Run("change: planned's different non-zero values win, smart groups still not overlaid", func(t *testing.T) {
		t.Parallel()
		live := liveGeneral()
		planned := &sdk.GeneralPayloadV4Entity{
			Name:                   "Planned Name",
			Description:            "Planned Description",
			AssignmentType:         "Auto",
			ProfileScope:           intPtr(2),
			IsActive:               boolPtr(false),
			ProfileContext:         "User",
			ManagedLocationGroupID: intPtr(20),
			AssignedSmartGroups:    []sdk.SmartGroupEntity1V4{{Name: "planned-asg", SmartGroupID: intPtr(3)}},
		}

		out := overlayGeneralV4Fields(live, planned)

		if out.Name != "Planned Name" {
			t.Errorf("Name = %q, want %q", out.Name, "Planned Name")
		}
		if out.Description != "Planned Description" {
			t.Errorf("Description = %q, want %q", out.Description, "Planned Description")
		}
		if out.AssignmentType != "Auto" {
			t.Errorf("AssignmentType = %q, want %q", out.AssignmentType, "Auto")
		}
		if out.ProfileScope == nil || *out.ProfileScope != 2 {
			t.Errorf("ProfileScope = %v, want 2", out.ProfileScope)
		}
		if out.IsActive == nil || *out.IsActive != false {
			t.Errorf("IsActive = %v, want false", out.IsActive)
		}
		if out.ProfileContext != "User" {
			t.Errorf("ProfileContext = %q, want %q", out.ProfileContext, "User")
		}
		if out.ManagedLocationGroupID == nil || *out.ManagedLocationGroupID != 20 {
			t.Errorf("ManagedLocationGroupID = %v, want 20", out.ManagedLocationGroupID)
		}
		if out.Password != "live-password" {
			t.Errorf("unmodeled Password = %q, want survive", out.Password)
		}
		// planned's smart groups must be ignored - live's must remain.
		if len(out.AssignedSmartGroups) != 1 || out.AssignedSmartGroups[0].Name != "live-asg" {
			t.Errorf("AssignedSmartGroups = %+v, want live's unmodeled value to survive", out.AssignedSmartGroups)
		}
	})
}

// --- OverlayAppleOsXUpdateEntity: every modeled section ---

func liveAppleOsXFull() *sdk.AppleOsXDeviceProfileEntityV2 {
	return &sdk.AppleOsXDeviceProfileEntityV2{
		General: &sdk.GeneralPayloadV2Entity{Name: "Live Name"},
		Passcode: &sdk.AppleOsXPasscodePayloadEntityV2{
			AllowSimpleValue: boolPtr(true),
		},
		CustomSettingsList: []sdk.AppleOsXCustomSettingsPayloadEntityV2{{CustomSettings: "live-settings"}},
		NetworkList:        []sdk.AppleOsXNetworkPayloadEntityV2{{ServiceSetIdentifier: "live-ssid"}},
		CredentialsList:    []sdk.AppleOsXCredentialPayloadEntityV2{{CredentialName: "live-cred"}},
		DiskEncryption: &sdk.AppleOsXDiskEncryptionPayloadEntityV2{
			DiskEncryptionAirWatch: &sdk.AppleOsXDiskEncryptionAirWatchPayloadEntityV2{EnableRecoveryKey: boolPtr(true)},
		},
		GateKeeper: &sdk.MacOsGatekeeperPayloadV2Entity{AllowAutoUnlock: boolPtr(true)},
		Restrictions: &sdk.AppleOsXRestrictionsPayloadEntityV2{
			Desktop: &sdk.AppleOsXRestrictionDesktopPayloadEntityV2{LockDesktopPicture: boolPtr(true)},
		},
		PrivacyPreferences: &sdk.MacOsPrivacyPreferencesPayloadV2Model{
			Identities: []sdk.MacOsPrivacyPreferencesV2Model{{Identifier: "com.example.live", IdentifierType: "bundleID"}},
		},
		// unmodeled - must survive every row unchanged.
		SsoExtensionList: []sdk.MacOsSsoExtensionPayloadV2Entity{{CertificateName: "live-sso-cert"}},
	}
}

func TestOverlayAppleOsXUpdateEntity_ModeledSections(t *testing.T) {
	t.Parallel()

	t.Run("clear: planned nil clears every modeled section, unmodeled survives", func(t *testing.T) {
		t.Parallel()
		live := liveAppleOsXFull()
		planned := &sdk.AppleOsXDeviceProfileEntityV2{General: &sdk.GeneralPayloadV2Entity{}}

		out := OverlayAppleOsXUpdateEntity(live, planned)

		if out.Passcode != nil {
			t.Errorf("Passcode = %+v, want nil (cleared)", out.Passcode)
		}
		if out.CustomSettingsList != nil {
			t.Errorf("CustomSettingsList = %+v, want nil (cleared)", out.CustomSettingsList)
		}
		if out.NetworkList != nil {
			t.Errorf("NetworkList = %+v, want nil (cleared)", out.NetworkList)
		}
		if out.CredentialsList != nil {
			t.Errorf("CredentialsList = %+v, want nil (cleared)", out.CredentialsList)
		}
		if out.DiskEncryption != nil {
			t.Errorf("DiskEncryption = %+v, want nil (cleared)", out.DiskEncryption)
		}
		if out.GateKeeper != nil {
			t.Errorf("GateKeeper = %+v, want nil (cleared)", out.GateKeeper)
		}
		if out.Restrictions != nil {
			t.Errorf("Restrictions = %+v, want nil (cleared)", out.Restrictions)
		}
		if out.PrivacyPreferences != nil {
			t.Errorf("PrivacyPreferences = %+v, want nil (cleared)", out.PrivacyPreferences)
		}
		if out.General.Name != "" {
			t.Errorf("General.Name = %q, want cleared", out.General.Name)
		}
		if len(out.SsoExtensionList) != 1 || out.SsoExtensionList[0].CertificateName != "live-sso-cert" {
			t.Errorf("unmodeled SsoExtensionList = %+v, want survive", out.SsoExtensionList)
		}
	})

	t.Run("change: planned's different sections win, unmodeled survives", func(t *testing.T) {
		t.Parallel()
		live := liveAppleOsXFull()
		planned := &sdk.AppleOsXDeviceProfileEntityV2{
			General:            &sdk.GeneralPayloadV2Entity{Name: "Planned Name"},
			Passcode:           &sdk.AppleOsXPasscodePayloadEntityV2{AllowSimpleValue: boolPtr(false)},
			CustomSettingsList: []sdk.AppleOsXCustomSettingsPayloadEntityV2{{CustomSettings: "planned-settings"}},
			NetworkList:        []sdk.AppleOsXNetworkPayloadEntityV2{{ServiceSetIdentifier: "planned-ssid"}},
			CredentialsList:    []sdk.AppleOsXCredentialPayloadEntityV2{{CredentialName: "planned-cred"}},
			DiskEncryption: &sdk.AppleOsXDiskEncryptionPayloadEntityV2{
				DiskEncryptionAirWatch: &sdk.AppleOsXDiskEncryptionAirWatchPayloadEntityV2{EnableRecoveryKey: boolPtr(false)},
			},
			GateKeeper: &sdk.MacOsGatekeeperPayloadV2Entity{AllowAutoUnlock: boolPtr(false)},
			Restrictions: &sdk.AppleOsXRestrictionsPayloadEntityV2{
				Desktop: &sdk.AppleOsXRestrictionDesktopPayloadEntityV2{LockDesktopPicture: boolPtr(false)},
			},
			PrivacyPreferences: &sdk.MacOsPrivacyPreferencesPayloadV2Model{
				Identities: []sdk.MacOsPrivacyPreferencesV2Model{{Identifier: "com.example.planned", IdentifierType: "bundleID"}},
			},
		}

		out := OverlayAppleOsXUpdateEntity(live, planned)

		if out.PrivacyPreferences == nil || len(out.PrivacyPreferences.Identities) != 1 ||
			out.PrivacyPreferences.Identities[0].Identifier != "com.example.planned" {
			t.Errorf("PrivacyPreferences = %+v, want planned's value", out.PrivacyPreferences)
		}

		if out.Passcode == nil || out.Passcode.AllowSimpleValue == nil || *out.Passcode.AllowSimpleValue != false {
			t.Errorf("Passcode = %+v, want planned's value", out.Passcode)
		}
		if len(out.CustomSettingsList) != 1 || out.CustomSettingsList[0].CustomSettings != "planned-settings" {
			t.Errorf("CustomSettingsList = %+v, want planned's value", out.CustomSettingsList)
		}
		if len(out.NetworkList) != 1 || out.NetworkList[0].ServiceSetIdentifier != "planned-ssid" {
			t.Errorf("NetworkList = %+v, want planned's value", out.NetworkList)
		}
		if len(out.CredentialsList) != 1 || out.CredentialsList[0].CredentialName != "planned-cred" {
			t.Errorf("CredentialsList = %+v, want planned's value", out.CredentialsList)
		}
		if out.DiskEncryption == nil || out.DiskEncryption.DiskEncryptionAirWatch == nil ||
			out.DiskEncryption.DiskEncryptionAirWatch.EnableRecoveryKey == nil ||
			*out.DiskEncryption.DiskEncryptionAirWatch.EnableRecoveryKey != false {
			t.Errorf("DiskEncryption = %+v, want planned's value", out.DiskEncryption)
		}
		if out.GateKeeper == nil || out.GateKeeper.AllowAutoUnlock == nil || *out.GateKeeper.AllowAutoUnlock != false {
			t.Errorf("GateKeeper = %+v, want planned's value", out.GateKeeper)
		}
		if out.Restrictions == nil || out.Restrictions.Desktop == nil || out.Restrictions.Desktop.LockDesktopPicture == nil ||
			*out.Restrictions.Desktop.LockDesktopPicture != false {
			t.Errorf("Restrictions = %+v, want planned's value", out.Restrictions)
		}
		if out.General.Name != "Planned Name" {
			t.Errorf("General.Name = %q, want %q", out.General.Name, "Planned Name")
		}
		if len(out.SsoExtensionList) != 1 || out.SsoExtensionList[0].CertificateName != "live-sso-cert" {
			t.Errorf("unmodeled SsoExtensionList = %+v, want survive", out.SsoExtensionList)
		}
	})
}

// --- OverlayAndroidUpdateEntity: every modeled section ---

func liveAndroidFull() *sdk.AndroidDeviceProfileV2Entity {
	return &sdk.AndroidDeviceProfileV2Entity{
		General: &sdk.GeneralPayloadV2Entity{Name: "Live Name"},
		AndroidForWorkCustomMessages: &sdk.AndroidForWorkCustomMessagesPayloadV2Entity{
			LockScreenMessage: "live-message",
		},
		CustomSettingsList: []sdk.AndroidCustomSettingsPayloadV2Entity{{CustomSettings: "live-settings"}},
		// unmodeled - must survive every row unchanged.
		AndroidForWorkApplicationControl: &sdk.AndroidForWorkApplicationControlPayloadV2Entity{
			DisableAccessToBlacklistedApps: boolPtr(true),
		},
	}
}

func TestOverlayAndroidUpdateEntity_ModeledSections(t *testing.T) {
	t.Parallel()

	t.Run("clear: planned nil clears every modeled section, unmodeled survives", func(t *testing.T) {
		t.Parallel()
		live := liveAndroidFull()
		planned := &sdk.AndroidDeviceProfileV2Entity{General: &sdk.GeneralPayloadV2Entity{}}

		out := OverlayAndroidUpdateEntity(live, planned)

		if out.AndroidForWorkCustomMessages != nil {
			t.Errorf("AndroidForWorkCustomMessages = %+v, want nil (cleared)", out.AndroidForWorkCustomMessages)
		}
		if out.CustomSettingsList != nil {
			t.Errorf("CustomSettingsList = %+v, want nil (cleared)", out.CustomSettingsList)
		}
		if out.AndroidForWorkApplicationControl == nil ||
			out.AndroidForWorkApplicationControl.DisableAccessToBlacklistedApps == nil ||
			!*out.AndroidForWorkApplicationControl.DisableAccessToBlacklistedApps {
			t.Errorf("unmodeled AndroidForWorkApplicationControl = %+v, want survive", out.AndroidForWorkApplicationControl)
		}
		if out.General.Name != "" {
			t.Errorf("General.Name = %q, want cleared", out.General.Name)
		}
	})

	t.Run("change: planned's different sections win, unmodeled survives", func(t *testing.T) {
		t.Parallel()
		live := liveAndroidFull()
		planned := &sdk.AndroidDeviceProfileV2Entity{
			General: &sdk.GeneralPayloadV2Entity{Name: "Planned Name"},
			AndroidForWorkCustomMessages: &sdk.AndroidForWorkCustomMessagesPayloadV2Entity{
				LockScreenMessage: "planned-message",
			},
			CustomSettingsList: []sdk.AndroidCustomSettingsPayloadV2Entity{{CustomSettings: "planned-settings"}},
		}

		out := OverlayAndroidUpdateEntity(live, planned)

		if out.AndroidForWorkCustomMessages == nil || out.AndroidForWorkCustomMessages.LockScreenMessage != "planned-message" {
			t.Errorf("AndroidForWorkCustomMessages = %+v, want planned's value", out.AndroidForWorkCustomMessages)
		}
		if len(out.CustomSettingsList) != 1 || out.CustomSettingsList[0].CustomSettings != "planned-settings" {
			t.Errorf("CustomSettingsList = %+v, want planned's value", out.CustomSettingsList)
		}
		if out.AndroidForWorkApplicationControl == nil ||
			out.AndroidForWorkApplicationControl.DisableAccessToBlacklistedApps == nil ||
			!*out.AndroidForWorkApplicationControl.DisableAccessToBlacklistedApps {
			t.Errorf("unmodeled AndroidForWorkApplicationControl = %+v, want survive", out.AndroidForWorkApplicationControl)
		}
		if out.General.Name != "Planned Name" {
			t.Errorf("General.Name = %q, want %q", out.General.Name, "Planned Name")
		}
	})
}

// --- OverlayAppleiOSUpdateEntity: every modeled section ---

func liveAppleiOSFull() *sdk.AppleDeviceProfileV2Entity {
	return &sdk.AppleDeviceProfileV2Entity{
		General:            &sdk.GeneralPayloadV2Entity{Name: "Live Name"},
		Passcode:           &sdk.ApplePasscodePayloadV2Entity{AllowSimpleValue: boolPtr(true)},
		CustomSettingsList: []sdk.AppleCustomSettingsPayloadV2Entity{{CustomSettings: "live-settings"}},
		// unmodeled - must survive every row unchanged.
		Restrictions: &sdk.AppleRestrictionsPayloadV2Entity{AcceptCookies: "2"},
	}
}

func TestOverlayAppleiOSUpdateEntity_ModeledSections(t *testing.T) {
	t.Parallel()

	t.Run("clear: planned nil clears every modeled section, unmodeled survives", func(t *testing.T) {
		t.Parallel()
		live := liveAppleiOSFull()
		planned := &sdk.AppleDeviceProfileV2Entity{General: &sdk.GeneralPayloadV2Entity{}}

		out := OverlayAppleiOSUpdateEntity(live, planned)

		if out.Passcode != nil {
			t.Errorf("Passcode = %+v, want nil (cleared)", out.Passcode)
		}
		if out.CustomSettingsList != nil {
			t.Errorf("CustomSettingsList = %+v, want nil (cleared)", out.CustomSettingsList)
		}
		if out.Restrictions == nil || out.Restrictions.AcceptCookies != "2" {
			t.Errorf("unmodeled Restrictions = %+v, want survive", out.Restrictions)
		}
		if out.General.Name != "" {
			t.Errorf("General.Name = %q, want cleared", out.General.Name)
		}
	})

	t.Run("change: planned's different sections win, unmodeled survives", func(t *testing.T) {
		t.Parallel()
		live := liveAppleiOSFull()
		planned := &sdk.AppleDeviceProfileV2Entity{
			General:            &sdk.GeneralPayloadV2Entity{Name: "Planned Name"},
			Passcode:           &sdk.ApplePasscodePayloadV2Entity{AllowSimpleValue: boolPtr(false)},
			CustomSettingsList: []sdk.AppleCustomSettingsPayloadV2Entity{{CustomSettings: "planned-settings"}},
		}

		out := OverlayAppleiOSUpdateEntity(live, planned)

		if out.Passcode == nil || out.Passcode.AllowSimpleValue == nil || *out.Passcode.AllowSimpleValue != false {
			t.Errorf("Passcode = %+v, want planned's value", out.Passcode)
		}
		if len(out.CustomSettingsList) != 1 || out.CustomSettingsList[0].CustomSettings != "planned-settings" {
			t.Errorf("CustomSettingsList = %+v, want planned's value", out.CustomSettingsList)
		}
		if out.Restrictions == nil || out.Restrictions.AcceptCookies != "2" {
			t.Errorf("unmodeled Restrictions = %+v, want survive", out.Restrictions)
		}
		if out.General.Name != "Planned Name" {
			t.Errorf("General.Name = %q, want %q", out.General.Name, "Planned Name")
		}
	})
}

// --- General-only overlays (Windows10, WindowsRugged): direct assignment-line coverage ---

func TestOverlayWindows10UpdateEntity_Direct(t *testing.T) {
	t.Parallel()
	live := &sdk.WinRTDeviceProfileV2Entity{
		General:   &sdk.GeneralPayloadV2Entity{Name: "Live Name"},
		AntiVirus: &sdk.WindowsDesktopAntivirusPayloadEntityV2{ArchiveScanning: boolPtr(true)},
	}
	planned := &sdk.WinRTDeviceProfileV2Entity{General: &sdk.GeneralPayloadV2Entity{Name: "Planned Name"}}

	out := OverlayWindows10UpdateEntity(live, planned)

	if out.General.Name != "Planned Name" {
		t.Errorf("General.Name = %q, want %q", out.General.Name, "Planned Name")
	}
	if out.AntiVirus == nil || out.AntiVirus.ArchiveScanning == nil || !*out.AntiVirus.ArchiveScanning {
		t.Errorf("unmodeled AntiVirus = %+v, want survive", out.AntiVirus)
	}
}

func TestOverlayWindowsRuggedUpdateEntity_Direct(t *testing.T) {
	t.Parallel()
	live := &sdk.QnxDeviceProfileEntityV2{
		General: &sdk.GeneralPayloadV2Entity{Name: "Live Name"},
		CustomAttributePayload: &sdk.QnxCustomAttributePayloadEntityV2{
			CustomAttributes: []sdk.QnxProfileCustomAttributeEntityV2{{Name: "live-attr", Value: "live-value"}},
		},
	}
	planned := &sdk.QnxDeviceProfileEntityV2{General: &sdk.GeneralPayloadV2Entity{Name: "Planned Name"}}

	out := OverlayWindowsRuggedUpdateEntity(live, planned)

	if out.General.Name != "Planned Name" {
		t.Errorf("General.Name = %q, want %q", out.General.Name, "Planned Name")
	}
	if out.CustomAttributePayload == nil || len(out.CustomAttributePayload.CustomAttributes) != 1 ||
		out.CustomAttributePayload.CustomAttributes[0].Name != "live-attr" {
		t.Errorf("unmodeled CustomAttributePayload = %+v, want survive", out.CustomAttributePayload)
	}
}
