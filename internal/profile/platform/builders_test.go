package platform

import (
	"testing"

	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"

	sdk "github.com/euc-oss/terraform-sdk-uem"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// --- General builders -------------------------------------------------------

func TestBuildGeneralV2Create_AllFieldsKnown(t *testing.T) {
	t.Parallel()

	data := &profilemodels.ProfileResourceModel{
		Name:           types.StringValue("Test Profile"),
		Description:    types.StringValue("desc"),
		AssignmentType: types.StringValue("Auto"),
		ProfileScope:   types.StringValue("Production"),
		IsActive:       types.BoolValue(true),
		ProfileContext: types.StringValue("Device"),
		OrgGroupID:     types.StringValue("14165"),
	}

	got := BuildGeneralV2Create(data)
	if got == nil {
		t.Fatal("got nil")
	}
	if got.Name != "Test Profile" {
		t.Errorf("Name = %q", got.Name)
	}
	if got.Description != "desc" {
		t.Errorf("Description = %q", got.Description)
	}
	if got.AssignmentType != "Auto" {
		t.Errorf("AssignmentType = %q", got.AssignmentType)
	}
	if got.ProfileScope != "Production" {
		t.Errorf("ProfileScope = %q", got.ProfileScope)
	}
	if got.IsActive == nil || *got.IsActive != true {
		t.Errorf("IsActive = %v", got.IsActive)
	}
	if got.ProfileContext != "Device" {
		t.Errorf("ProfileContext = %q", got.ProfileContext)
	}
	if got.ManagedLocationGroupID == nil || *got.ManagedLocationGroupID != 14165 {
		t.Errorf("ManagedLocationGroupID = %v", got.ManagedLocationGroupID)
	}
}

func TestBuildGeneralV2Create_NullsAndUnknownsOmitted(t *testing.T) {
	t.Parallel()

	data := &profilemodels.ProfileResourceModel{
		Name:           types.StringValue("Only Name"),
		Description:    types.StringValue(""),
		AssignmentType: types.StringValue(""),
		ProfileScope:   types.StringValue(""),
		IsActive:       types.BoolNull(),
		ProfileContext: types.StringNull(),
		OrgGroupID:     types.StringUnknown(),
	}

	got := BuildGeneralV2Create(data)
	if got.IsActive != nil {
		t.Errorf("IsActive = %v, want nil", *got.IsActive)
	}
	if got.ProfileContext != "" {
		t.Errorf("ProfileContext = %q, want empty", got.ProfileContext)
	}
	if got.ManagedLocationGroupID != nil {
		t.Errorf("ManagedLocationGroupID = %v, want nil", *got.ManagedLocationGroupID)
	}
}

func TestBuildGeneralV2Create_InvalidOrgGroupIDIgnored(t *testing.T) {
	t.Parallel()

	data := &profilemodels.ProfileResourceModel{
		Name:       types.StringValue("X"),
		OrgGroupID: types.StringValue("not-a-number"),
	}
	got := BuildGeneralV2Create(data)
	if got.ManagedLocationGroupID != nil {
		t.Errorf("ManagedLocationGroupID = %v, want nil for non-numeric input", *got.ManagedLocationGroupID)
	}
}

func TestStampGeneralV2(t *testing.T) {
	t.Parallel()

	t.Run("nil safe", func(t *testing.T) {
		t.Parallel()
		StampGeneralV2(nil, 1, nil)
	})

	t.Run("with current version increments", func(t *testing.T) {
		t.Parallel()
		g := &sdk.GeneralPayloadV2Entity{}
		cur := 4
		StampGeneralV2(g, 99, &cur)
		if g.ProfileID == nil || *g.ProfileID != 99 {
			t.Errorf("ProfileID = %v", g.ProfileID)
		}
		if g.CreateNewVersion == nil || *g.CreateNewVersion != true {
			t.Errorf("CreateNewVersion = %v", g.CreateNewVersion)
		}
		if g.Version == nil || *g.Version != 5 {
			t.Errorf("Version = %v, want 5", g.Version)
		}
	})

	t.Run("without current version omits Version", func(t *testing.T) {
		t.Parallel()
		g := &sdk.GeneralPayloadV2Entity{}
		StampGeneralV2(g, 7, nil)
		if g.Version != nil {
			t.Errorf("Version = %v, want nil", *g.Version)
		}
	})
}

// --- AppleOsX top-level entity ---------------------------------------------

func TestBuildAppleOsXCreateEntity_Minimal(t *testing.T) {
	t.Parallel()

	data := &profilemodels.ProfileResourceModel{
		Name:           types.StringValue("Minimal"),
		AssignmentType: types.StringValue("Auto"),
	}
	ent, err := BuildAppleOsXCreateEntity(data)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if ent == nil || ent.General == nil {
		t.Fatal("entity or general is nil")
	}
	if ent.Passcode != nil {
		t.Error("Passcode should be nil when not set")
	}
	if ent.Restrictions != nil {
		t.Error("Restrictions should be nil when not set")
	}
	if ent.DiskEncryption != nil {
		t.Error("DiskEncryption should be nil when not set")
	}
}

func TestBuildAppleOsXCreateEntity_WithRestrictions(t *testing.T) {
	t.Parallel()

	data := &profilemodels.ProfileResourceModel{
		Name: types.StringValue("WithRestrictions"),
		Restrictions: &profilemodels.RestrictionsModel{
			Applications: &profilemodels.RestrictionsApplicationsModel{
				AllowApplication: types.ListNull(types.StringType),
				AllowFolders:     types.ListNull(types.StringType),
				DisallowFolders:  types.ListNull(types.StringType),
				AppleMusic: &profilemodels.RestrictionsAppleMusicModel{
					AllowMusicService: types.BoolValue(false),
				},
			},
		},
	}
	ent, err := BuildAppleOsXCreateEntity(data)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if ent.Restrictions == nil || ent.Restrictions.Applications == nil {
		t.Fatal("Restrictions.Applications missing")
	}
	if ent.Restrictions.Applications.AppleMusic == nil ||
		ent.Restrictions.Applications.AppleMusic.AllowMusicService == nil ||
		*ent.Restrictions.Applications.AppleMusic.AllowMusicService != false {
		t.Errorf("AppleMusic.AllowMusicService not propagated")
	}
}

func TestBuildAppleOsXGatekeeperEntity_NullsOmitted(t *testing.T) {
	t.Parallel()

	g := &profilemodels.GatekeeperModel{
		AllowAutoUnlock:              types.BoolNull(),
		AllowFingerprintForUnlock:    types.BoolNull(),
		AllowHandoff:                 types.BoolNull(),
		AllowScreenCapture:           types.BoolNull(),
		EnableAppSoftwareUpdateDelay: types.BoolNull(),
		EnableSoftwareUpdateDelay:    types.BoolNull(),
		EnforcedSoftwareUpdateDelay:  types.Int64Null(),
	}
	out := buildAppleOsXGatekeeperEntity(g)
	if out == nil {
		t.Fatal("expected non-nil entity")
	}
	if out.AllowAutoUnlock != nil || out.AllowFingerprintForUnlock != nil ||
		out.AllowHandoff != nil || out.AllowScreenCapture != nil ||
		out.EnableAppSoftwareUpdateDelay != nil || out.EnableSoftwareUpdateDelay != nil ||
		out.EnforcedSoftwareUpdateDelay != nil {
		t.Errorf("expected all fields nil, got %+v", out)
	}
}

func TestBuildAppleOsXGatekeeperEntity_AllFieldsSet(t *testing.T) {
	t.Parallel()

	g := &profilemodels.GatekeeperModel{
		AllowAutoUnlock:              types.BoolValue(false),
		AllowFingerprintForUnlock:    types.BoolValue(true),
		AllowHandoff:                 types.BoolValue(false),
		AllowScreenCapture:           types.BoolValue(true),
		EnableAppSoftwareUpdateDelay: types.BoolValue(true),
		EnableSoftwareUpdateDelay:    types.BoolValue(true),
		EnforcedSoftwareUpdateDelay:  types.Int64Value(60),
	}
	out := buildAppleOsXGatekeeperEntity(g)
	if out == nil {
		t.Fatal("expected non-nil entity")
	}
	if out.AllowAutoUnlock == nil || *out.AllowAutoUnlock != false {
		t.Errorf("AllowAutoUnlock = %v, want false", out.AllowAutoUnlock)
	}
	if out.EnforcedSoftwareUpdateDelay == nil || *out.EnforcedSoftwareUpdateDelay != 60 {
		t.Errorf("EnforcedSoftwareUpdateDelay = %v, want 60", out.EnforcedSoftwareUpdateDelay)
	}
}

func TestBuildAppleOsXCreateEntity_WithGatekeeper(t *testing.T) {
	t.Parallel()

	data := &profilemodels.ProfileResourceModel{
		Name: types.StringValue("WithGatekeeper"),
		Gatekeeper: &profilemodels.GatekeeperModel{
			AllowScreenCapture:        types.BoolValue(false),
			EnableSoftwareUpdateDelay: types.BoolValue(true),
		},
	}
	ent, err := BuildAppleOsXCreateEntity(data)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if ent.GateKeeper == nil {
		t.Fatal("GateKeeper not propagated to entity")
	}
	if ent.GateKeeper.AllowScreenCapture == nil || *ent.GateKeeper.AllowScreenCapture != false {
		t.Errorf("AllowScreenCapture not propagated, got %v", ent.GateKeeper.AllowScreenCapture)
	}
}

// --- Restrictions sub-builders ---------------------------------------------

func TestBuildAppleOsXRestrictionsApplicationsEntity_NestedBlocksOmittedWhenNil(t *testing.T) {
	t.Parallel()

	m := &profilemodels.RestrictionsApplicationsModel{
		AllowApplication: types.ListNull(types.StringType),
		AllowFolders:     types.ListNull(types.StringType),
		DisallowFolders:  types.ListNull(types.StringType),
	}
	got := buildAppleOsXRestrictionsApplicationsEntity(m)
	if got == nil {
		t.Fatal("nil")
	}
	if got.AppStore != nil || got.AppleMusic != nil || got.Camera != nil ||
		got.GameCentre != nil || got.Safari != nil {
		t.Error("nested blocks should be nil when not configured")
	}
	if got.AllowApplication != nil || got.AllowFolders != nil || got.DisallowFolders != nil {
		t.Error("null lists should produce nil slices")
	}
}

func TestBuildAppleOsXRestrictionsApplicationsEntity_PopulatedLists(t *testing.T) {
	t.Parallel()

	m := &profilemodels.RestrictionsApplicationsModel{
		AllowApplication: mustStringList(t, "app1", "app2"),
		AllowFolders:     mustStringList(t, "/Applications"),
		DisallowFolders:  types.ListNull(types.StringType),
		Safari: &profilemodels.RestrictionsSafariModel{
			AllowSafariAutoFill: types.BoolValue(false),
		},
	}
	got := buildAppleOsXRestrictionsApplicationsEntity(m)
	if !equalStringSlices(got.AllowApplication, []string{"app1", "app2"}) {
		t.Errorf("AllowApplication = %v", got.AllowApplication)
	}
	if !equalStringSlices(got.AllowFolders, []string{"/Applications"}) {
		t.Errorf("AllowFolders = %v", got.AllowFolders)
	}
	if got.DisallowFolders != nil {
		t.Errorf("DisallowFolders = %v, want nil", got.DisallowFolders)
	}
	if got.Safari == nil || got.Safari.AllowSafariAutoFill == nil || *got.Safari.AllowSafariAutoFill != false {
		t.Errorf("Safari.AllowSafariAutoFill not propagated")
	}
	if got.Safari.AllowDeprecatedWebKitTls != nil {
		t.Errorf("AllowDeprecatedWebKitTls = %v, want nil", *got.Safari.AllowDeprecatedWebKitTls)
	}
}

func TestBuildAppleOsXRestrictionsMediaEntity_AirDropAndNestedAccess(t *testing.T) {
	t.Parallel()

	m := &profilemodels.RestrictionsMediaModel{
		AutoEjectMedia: types.BoolValue(true),
		DiskMediaCDs: &profilemodels.RestrictionsMediaAccessModel{
			Allow:        types.BoolValue(true),
			Authenticate: types.BoolValue(false),
			ReadOnly:     types.BoolNull(),
		},
		NetworkAccess: &profilemodels.RestrictionsNetworkAccessModel{
			AirDrop: types.BoolValue(false),
		},
	}
	got := buildAppleOsXRestrictionsMediaEntity(m)
	if got.AutoEjectMedia == nil || *got.AutoEjectMedia != true {
		t.Errorf("AutoEjectMedia = %v", got.AutoEjectMedia)
	}
	if got.DiskMediaCDs == nil ||
		got.DiskMediaCDs.Allow == nil || *got.DiskMediaCDs.Allow != true ||
		got.DiskMediaCDs.Authenticate == nil || *got.DiskMediaCDs.Authenticate != false {
		t.Errorf("DiskMediaCDs not mapped correctly: %+v", got.DiskMediaCDs)
	}
	if got.DiskMediaCDs.ReadOnly != nil {
		t.Errorf("ReadOnly = %v, want nil for null input", *got.DiskMediaCDs.ReadOnly)
	}
	if got.NetworkAccess == nil || got.NetworkAccess.AirDrop == nil || *got.NetworkAccess.AirDrop != false {
		t.Errorf("AirDrop not propagated: %+v", got.NetworkAccess)
	}
	if got.RecordableDisc != nil {
		t.Error("RecordableDisc should be nil when not set")
	}
}

func TestBuildAppleOsXRestrictionsPreferencesEntity_BehaviorAndPanes(t *testing.T) {
	t.Parallel()

	m := &profilemodels.RestrictionsPreferencesModel{
		PreferenceBehavior: types.StringValue("enabled"),
		Bluetooth:          types.BoolValue(true),
		AppStore:           types.BoolValue(false),
		Spotlight:          types.BoolNull(),
	}
	got := buildAppleOsXRestrictionsPreferencesEntity(m)
	if got.PreferenceBehavior != "enabled" {
		t.Errorf("PreferenceBehavior = %q", got.PreferenceBehavior)
	}
	if got.Bluetooth == nil || *got.Bluetooth != true {
		t.Errorf("Bluetooth = %v", got.Bluetooth)
	}
	if got.AppStore == nil || *got.AppStore != false {
		t.Errorf("AppStore = %v", got.AppStore)
	}
	if got.Spotlight != nil {
		t.Errorf("Spotlight = %v, want nil for null input", *got.Spotlight)
	}
}

func TestBuildAppleOsXRestrictionsWidgetsEntity_AllowedWidgets(t *testing.T) {
	t.Parallel()

	m := &profilemodels.RestrictionsWidgetsModel{
		AllowOnlyConfiguredWidgets: types.BoolValue(true),
		AllowedWidgets:             mustStringList(t, "com.example.widget"),
	}
	got := buildAppleOsXRestrictionsWidgetsEntity(m)
	if got.AllowOnlyConfiguredWidgets == nil || *got.AllowOnlyConfiguredWidgets != true {
		t.Errorf("AllowOnlyConfiguredWidgets = %v", got.AllowOnlyConfiguredWidgets)
	}
	if !equalStringSlices(got.AllowedWidgets, []string{"com.example.widget"}) {
		t.Errorf("AllowedWidgets = %v", got.AllowedWidgets)
	}
}

// --- Custom-settings list builder -------------------------------------------

func TestBuildAppleOsXCustomSettingsList_FiltersNull(t *testing.T) {
	t.Parallel()

	items := []profilemodels.CustomSettingsItemModel{
		{CustomSettings: types.StringValue("<plist>1</plist>")},
		{CustomSettings: types.StringNull()},
		{CustomSettings: types.StringValue("<plist>2</plist>")},
	}
	got := buildAppleOsXCustomSettingsList(items)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].CustomSettings != "<plist>1</plist>" || got[1].CustomSettings != "<plist>2</plist>" {
		t.Errorf("got = %+v", got)
	}
}

func TestBuildAppleOsXCustomSettingsList_EmptyReturnsNil(t *testing.T) {
	t.Parallel()

	if got := buildAppleOsXCustomSettingsList(nil); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}
