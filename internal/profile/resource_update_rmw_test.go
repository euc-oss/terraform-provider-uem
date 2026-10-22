package profile

// zf1 part 1: read-modify-write Update tests.
//
// These tests drive Update through the real ProfileResource.Update method,
// injecting a fake profileServiceAPI (via ProfileResource.newProfileService)
// so we can assert directly on the entity passed to svc.Update without
// going through an httptest server. For each of the 6 profile platforms we
// verify:
//
//   (a) unmodeled sections AND unmodeled General fields (Password,
//       AllowRemoval, ExpirationDate, AssignedSchedule) on the live entity
//       survive Update unchanged.
//   (b) modeled sections/fields reflect the plan — including a modeled
//       section present live but null in the plan being cleared (nil) in
//       the sent entity.
//   (c) a Get error, or a Get that returns no entity for the platform,
//       aborts before svc.Update is ever called (FAIL CLOSED).

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	profileplatform "github.com/euc-oss/terraform-provider-uem/internal/profile/platform"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// errGetFailed is the sentinel error the fake profileServiceAPI returns
// from Get in the "Get fails" branches below.
var errGetFailed = errors.New("simulated Get failure")

// --- fake profileServiceAPI ---

// fakeProfileService is an in-memory profileServiceAPI double. It records
// the entity passed to Update and how many times Update was called, and
// returns a configured Get result/error.
type fakeProfileService struct {
	getResult *sdk.ProfileResult
	getErr    error

	updateCalls  int
	updateEntity interface{}
	updateErr    error
}

func (f *fakeProfileService) RegisterEntry(id int, platform string) {}

func (f *fakeProfileService) Get(ctx context.Context, id int) (*sdk.ProfileResult, error) {
	return f.getResult, f.getErr
}

func (f *fakeProfileService) Create(ctx context.Context, platform string, profile interface{}) (int, error) {
	return 0, nil
}

func (f *fakeProfileService) Update(ctx context.Context, id int, profile interface{}) error {
	f.updateCalls++
	f.updateEntity = profile
	return f.updateErr
}

func (f *fakeProfileService) Delete(ctx context.Context, id int) error { return nil }

// newRMWTestResource builds a ProfileResource wired to the given fake
// service. The SDK client field just needs to be non-nil (profileService()
// errors on a nil client) — the fake factory ignores the client argument
// entirely, so no network traffic ever happens.
func newRMWTestResource(t *testing.T, fake *fakeProfileService) *ProfileResource {
	t.Helper()
	c, server := createTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected HTTP call to %s %s; the fake profileServiceAPI should have intercepted this", r.Method, r.URL.Path)
	})
	t.Cleanup(server.Close)
	return &ProfileResource{
		client: c,
		newProfileService: func(ctx context.Context, c *sdk.Client) (profileServiceAPI, error) {
			return fake, nil
		},
	}
}

// rmwBasePlanValues returns the full set of profile schema attributes with
// sane defaults for an Update plan, so each test only needs to override
// the fields it cares about.
func rmwBasePlanValues(platform string) map[string]tftypes.Value {
	return map[string]tftypes.Value{
		"id":                   stringVal("999"),
		"name":                 stringVal("Live Name"),
		"description":          stringVal("Live Description"),
		"platform":             stringVal(platform),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      stringVal("Auto"),
		"profile_scope":        stringVal("Production"),
		"is_active":            boolVal(true),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 stringVal("old-uuid"),
		"profile_context":      stringVal("Device"),
	}
}

func mergeValues(base map[string]tftypes.Value, overrides map[string]tftypes.Value) map[string]tftypes.Value {
	out := make(map[string]tftypes.Value, len(base))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overrides {
		out[k] = v
	}
	return out
}

// runRMWUpdate drives ProfileResource.Update with the given plan values and
// fake service, returning the response for assertion.
func runRMWUpdate(t *testing.T, fake *fakeProfileService, planValues map[string]tftypes.Value) *resource.UpdateResponse {
	t.Helper()
	res := newRMWTestResource(t, fake)
	ctx := context.Background()

	plan := createResourcePlan(t, planValues)
	req := resource.UpdateRequest{Plan: plan, State: emptyResourceState(t)}
	resp := &resource.UpdateResponse{State: emptyResourceState(t)}
	res.Update(ctx, req, resp)
	return resp
}

// --- macOS (AppleOsX) ---

func liveAppleOsXEntity() *sdk.AppleOsXDeviceProfileEntityV2 {
	return &sdk.AppleOsXDeviceProfileEntityV2{
		General: &sdk.GeneralPayloadV2Entity{
			Name:                "Live Name",
			Description:         "Live Description",
			ProfileID:           intPtr(999),
			Version:             intPtr(3),
			Password:            "unlock-secret",
			AllowRemoval:        "Forbidden",
			ExpirationDate:      "2030-01-01",
			AssignedSchedule:    []*int{intPtr(5)},
			AssignedSmartGroups: []sdk.SmartGroupEntityV2{{Name: "live-asg", SmartGroupID: intPtr(101)}},
			ExcludedSmartGroups: []sdk.SmartGroupEntityV2{{Name: "live-esg", SmartGroupID: intPtr(201)}},
		},
		SsoExtensionList: []sdk.MacOsSsoExtensionPayloadV2Entity{
			{CertificateName: "live-sso-cert"},
		},
		// VpnList is modelled (F13); SkipSetupAssistant stands in as an
		// unmodeled section that must pass through.
		SkipSetupAssistant: &sdk.MacOsSetupAssistantPayloadEntityV2{SkipSiri: sdk.BoolPtr(true)},
		Restrictions: &sdk.AppleOsXRestrictionsPayloadEntityV2{
			Desktop: &sdk.AppleOsXRestrictionDesktopPayloadEntityV2{
				LockDesktopPicture: sdk.BoolPtr(true),
			},
		},
	}
}

func TestProfileResourceUpdate_AppleOsX_RMW(t *testing.T) {
	t.Parallel()

	t.Run("preserves unmodeled sections and General fields, clears null-in-plan modeled section, reflects modeled changes", func(t *testing.T) {
		fake := &fakeProfileService{getResult: &sdk.ProfileResult{AppleOsX: liveAppleOsXEntity()}}

		planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{
			"id":          stringVal("999"),
			"name":        stringVal("New Name"),
			"description": stringVal("New Description"),
			// restrictions stays null in the plan -> modeled section must clear.
			"restrictions": nullRestrictions(),
			"passcode": passcodeVal(map[string]tftypes.Value{
				"require_passcode_on_device": boolVal(true),
				"minimum_passcode_length":    int64Val(6),
			}),
		})

		resp := runRMWUpdate(t, fake, planValues)
		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected errors: %v", resp.Diagnostics.Errors())
		}
		if fake.updateCalls != 1 {
			t.Fatalf("expected exactly 1 Update call, got %d", fake.updateCalls)
		}
		sent, ok := fake.updateEntity.(*sdk.AppleOsXDeviceProfileEntityV2)
		if !ok {
			t.Fatalf("expected *sdk.AppleOsXDeviceProfileEntityV2, got %T", fake.updateEntity)
		}

		// (a) unmodeled sections preserved unchanged.
		if len(sent.SsoExtensionList) != 1 || sent.SsoExtensionList[0].CertificateName != "live-sso-cert" {
			t.Errorf("expected SsoExtensionList to survive unchanged, got %+v", sent.SsoExtensionList)
		}
		if sent.SkipSetupAssistant == nil || sent.SkipSetupAssistant.SkipSiri == nil || !*sent.SkipSetupAssistant.SkipSiri {
			t.Errorf("expected SkipSetupAssistant to survive unchanged, got %+v", sent.SkipSetupAssistant)
		}

		// (a) unmodeled General fields survive unchanged.
		if sent.General.Password != "unlock-secret" {
			t.Errorf("expected General.Password to survive unchanged, got %q", sent.General.Password)
		}
		if sent.General.AllowRemoval != "Forbidden" {
			t.Errorf("expected General.AllowRemoval to survive unchanged, got %q", sent.General.AllowRemoval)
		}
		if sent.General.ExpirationDate != "2030-01-01" {
			t.Errorf("expected General.ExpirationDate to survive unchanged, got %q", sent.General.ExpirationDate)
		}
		if !reflect.DeepEqual(sent.General.AssignedSchedule, []*int{intPtr(5)}) {
			t.Errorf("expected General.AssignedSchedule to survive unchanged, got %+v", sent.General.AssignedSchedule)
		}

		// (b) modeled section null in plan -> cleared.
		if sent.Restrictions != nil {
			t.Errorf("expected Restrictions to be nil (cleared) when null in plan, got %+v", sent.Restrictions)
		}

		// (b) modeled section set in plan -> reflects plan.
		if sent.Passcode == nil || sent.Passcode.RequirePasscodeOnDevice == nil || !*sent.Passcode.RequirePasscodeOnDevice {
			t.Errorf("expected Passcode.RequirePasscodeOnDevice true from plan, got %+v", sent.Passcode)
		}

		// (b) modeled General fields reflect plan.
		if sent.General.Name != "New Name" || sent.General.Description != "New Description" {
			t.Errorf("expected General Name/Description to reflect plan, got %q/%q", sent.General.Name, sent.General.Description)
		}

		// version bump: live General.Version was 3 -> sent must be 4, with
		// CreateNewVersion true and ProfileID set to the profile id.
		if sent.General.Version == nil || *sent.General.Version != 4 {
			t.Errorf("expected General.Version 4 (bumped from live 3), got %v", sent.General.Version)
		}
		if sent.General.CreateNewVersion == nil || !*sent.General.CreateNewVersion {
			t.Errorf("expected General.CreateNewVersion true, got %v", sent.General.CreateNewVersion)
		}
		if sent.General.ProfileID == nil || *sent.General.ProfileID != 999 {
			t.Errorf("expected General.ProfileID 999, got %v", sent.General.ProfileID)
		}
	})

	t.Run("smart groups: null in plan clears live's non-empty groups", func(t *testing.T) {
		fake := &fakeProfileService{getResult: &sdk.ProfileResult{AppleOsX: liveAppleOsXEntity()}}
		planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{
			"id":                    stringVal("999"),
			"assigned_smart_groups": nullStringList(),
			"excluded_smart_groups": nullStringList(),
		})

		resp := runRMWUpdate(t, fake, planValues)
		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected errors: %v", resp.Diagnostics.Errors())
		}
		sent, ok := fake.updateEntity.(*sdk.AppleOsXDeviceProfileEntityV2)
		if !ok {
			t.Fatalf("expected *sdk.AppleOsXDeviceProfileEntityV2, got %T", fake.updateEntity)
		}
		if len(sent.General.AssignedSmartGroups) != 0 {
			t.Errorf("expected AssignedSmartGroups cleared, got %+v", sent.General.AssignedSmartGroups)
		}
		if len(sent.General.ExcludedSmartGroups) != 0 {
			t.Errorf("expected ExcludedSmartGroups cleared, got %+v", sent.General.ExcludedSmartGroups)
		}
	})

	t.Run("smart groups: plan change is reflected exactly", func(t *testing.T) {
		fake := &fakeProfileService{getResult: &sdk.ProfileResult{AppleOsX: liveAppleOsXEntity()}}
		planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{
			"id":                    stringVal("999"),
			"assigned_smart_groups": stringListVal("301", "302"),
			"excluded_smart_groups": stringListVal("401"),
		})

		resp := runRMWUpdate(t, fake, planValues)
		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected errors: %v", resp.Diagnostics.Errors())
		}
		sent, ok := fake.updateEntity.(*sdk.AppleOsXDeviceProfileEntityV2)
		if !ok {
			t.Fatalf("expected *sdk.AppleOsXDeviceProfileEntityV2, got %T", fake.updateEntity)
		}
		wantAssigned := []sdk.SmartGroupEntityV2{{SmartGroupID: intPtr(301)}, {SmartGroupID: intPtr(302)}}
		wantExcluded := []sdk.SmartGroupEntityV2{{SmartGroupID: intPtr(401)}}
		if !reflect.DeepEqual(sent.General.AssignedSmartGroups, wantAssigned) {
			t.Errorf("expected AssignedSmartGroups %+v, got %+v", wantAssigned, sent.General.AssignedSmartGroups)
		}
		if !reflect.DeepEqual(sent.General.ExcludedSmartGroups, wantExcluded) {
			t.Errorf("expected ExcludedSmartGroups %+v, got %+v", wantExcluded, sent.General.ExcludedSmartGroups)
		}
	})

	t.Run("Get error aborts before Update", func(t *testing.T) {
		fake := &fakeProfileService{getErr: errGetFailed}
		planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{"id": stringVal("999")})
		resp := runRMWUpdate(t, fake, planValues)
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected error diagnostic on Get failure")
		}
		if fake.updateCalls != 0 {
			t.Fatalf("expected Update not to be called, got %d calls", fake.updateCalls)
		}
	})

	t.Run("Get OK but nil AppleOsX entity aborts before Update", func(t *testing.T) {
		fake := &fakeProfileService{getResult: &sdk.ProfileResult{}}
		planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{"id": stringVal("999")})
		resp := runRMWUpdate(t, fake, planValues)
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected error diagnostic when AppleOsX entity is nil")
		}
		if fake.updateCalls != 0 {
			t.Fatalf("expected Update not to be called, got %d calls", fake.updateCalls)
		}
	})
}

// TestProfileResourceUpdate_LastPayloadMandatoryHint covers internal-task's 422
// hint: when svc.Update returns UEM's "Atleast one Payload is mandatory"
// rejection (now reachable because the removal-clears fix can send a
// macOS profile with every managed block cleared), the diagnostic must
// append an actionable hint on top of the original error text. A
// differently-shaped error must NOT get the hint.
func TestProfileResourceUpdate_LastPayloadMandatoryHint(t *testing.T) {
	t.Parallel()

	t.Run("matching 422 gets the hint appended", func(t *testing.T) {
		fake := &fakeProfileService{
			getResult: &sdk.ProfileResult{AppleOsX: liveAppleOsXEntity()},
			updateErr: &sdk.APIError{
				StatusCode: http.StatusUnprocessableEntity,
				ErrorCode:  "1012",
				Message:    "Atleast one Payload is mandatory to create/update a AppleOsX Device Profile",
			},
		}
		planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{"id": stringVal("999")})

		resp := runRMWUpdate(t, fake, planValues)
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected an error diagnostic")
		}
		var found bool
		for _, d := range resp.Diagnostics.Errors() {
			if strings.Contains(d.Detail(), "Atleast one Payload is mandatory") &&
				strings.Contains(d.Detail(), "a macOS profile must keep at least one payload; removing the last managed block isn't possible") {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected hint in diagnostics, got: %v", resp.Diagnostics.Errors())
		}
	})

	t.Run("a different error has no hint", func(t *testing.T) {
		fake := &fakeProfileService{
			getResult: &sdk.ProfileResult{AppleOsX: liveAppleOsXEntity()},
			updateErr: &sdk.APIError{
				StatusCode: http.StatusBadRequest,
				ErrorCode:  "400",
				Message:    "Name is required.",
			},
		}
		planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{"id": stringVal("999")})

		resp := runRMWUpdate(t, fake, planValues)
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected an error diagnostic")
		}
		for _, d := range resp.Diagnostics.Errors() {
			if strings.Contains(d.Detail(), "removing the last managed block isn't possible") {
				t.Fatalf("did not expect the last-payload hint on an unrelated error, got: %s", d.Detail())
			}
		}
	})
}

// liveAppleOsXEntityAllBlocks is liveAppleOsXEntity() plus every one of the
// 8 top-level attributes that got a nullWhenConfigNull* modifier (internal-task) populated with content, so TestProfileResourceUpdate_RemovalClears
// can verify each one is actually cleared (not just planned null and
// silently kept) when the plan sends it null.
func liveAppleOsXEntityAllBlocks() *sdk.AppleOsXDeviceProfileEntityV2 {
	ent := liveAppleOsXEntity()
	ent.Passcode = &sdk.AppleOsXPasscodePayloadEntityV2{
		RequirePasscodeOnDevice: sdk.BoolPtr(true),
	}
	ent.CustomSettingsList = []sdk.AppleOsXCustomSettingsPayloadEntityV2{
		{CustomSettings: "<plist>live</plist>"},
	}
	ent.NetworkList = []sdk.AppleOsXNetworkPayloadEntityV2{
		{NetworkInterface: "BuiltInWireless", ServiceSetIdentifier: "live-ssid"},
	}
	ent.CredentialsList = []sdk.AppleOsXCredentialPayloadEntityV2{
		{CredentialSource: "Upload", CredentialName: "live-cred", CertificateID: intPtr(555)},
	}
	ent.DiskEncryption = &sdk.AppleOsXDiskEncryptionPayloadEntityV2{
		DiskEncryptionFileVault2: &sdk.AppleOsXDiskEncryptionFileVault2PayloadEntityV2{
			Enable: sdk.BoolPtr(true),
		},
	}
	ent.GateKeeper = &sdk.MacOsGatekeeperPayloadV2Entity{
		AllowAutoUnlock: sdk.BoolPtr(false),
	}
	// Restrictions is already populated (Desktop.LockDesktopPicture) by
	// liveAppleOsXEntity().
	return ent
}

// TestProfileResourceUpdate_RemovalClears drives Update with every one of
// the 8 nullWhenConfigNull*-modified attributes null in the plan — exactly
// what the new top-level modifiers plan when a managed block is removed
// from HCL configuration — against a live entity where all 8 are
// populated, and asserts every corresponding entity section is cleared
// (nil/empty) in what's sent to svc.Update. It also asserts the unmodeled
// zf1/01o sections (SsoExtensionList, SkipSetupAssistant, General.Password/
// AllowRemoval/ExpirationDate/AssignedSchedule) still pass through
// unchanged in the very same Update call.
func TestProfileResourceUpdate_RemovalClears(t *testing.T) {
	t.Parallel()

	fake := &fakeProfileService{getResult: &sdk.ProfileResult{AppleOsX: liveAppleOsXEntityAllBlocks()}}

	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{
		"id": stringVal("999"),
		// description omitted from config -> planned null by
		// nullWhenConfigNullStringModifier.
		"description": nullString(),
		// The rest are already null in rmwBasePlanValues (passcode,
		// custom_settings_list, network_list, credentials_list,
		// disk_encryption, gatekeeper, restrictions) — exactly what the new
		// top-level modifiers plan when the block is removed from HCL.
	})

	resp := runRMWUpdate(t, fake, planValues)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics.Errors())
	}
	if fake.updateCalls != 1 {
		t.Fatalf("expected exactly 1 Update call, got %d", fake.updateCalls)
	}
	sent, ok := fake.updateEntity.(*sdk.AppleOsXDeviceProfileEntityV2)
	if !ok {
		t.Fatalf("expected *sdk.AppleOsXDeviceProfileEntityV2, got %T", fake.updateEntity)
	}

	// Each of the 8 attributes' corresponding entity section is cleared.
	if sent.General.Description != "" {
		t.Errorf("expected General.Description cleared, got %q", sent.General.Description)
	}
	if sent.Passcode != nil {
		t.Errorf("expected Passcode cleared, got %+v", sent.Passcode)
	}
	if len(sent.CustomSettingsList) != 0 {
		t.Errorf("expected CustomSettingsList cleared, got %+v", sent.CustomSettingsList)
	}
	if len(sent.NetworkList) != 0 {
		t.Errorf("expected NetworkList cleared, got %+v", sent.NetworkList)
	}
	if len(sent.CredentialsList) != 0 {
		t.Errorf("expected CredentialsList cleared, got %+v", sent.CredentialsList)
	}
	if sent.DiskEncryption != nil {
		t.Errorf("expected DiskEncryption cleared, got %+v", sent.DiskEncryption)
	}
	if sent.GateKeeper != nil {
		t.Errorf("expected GateKeeper cleared, got %+v", sent.GateKeeper)
	}
	if sent.Restrictions != nil {
		t.Errorf("expected Restrictions cleared, got %+v", sent.Restrictions)
	}

	// Unmodeled sections/fields still pass through unchanged in this same
	// Update call.
	if len(sent.SsoExtensionList) != 1 || sent.SsoExtensionList[0].CertificateName != "live-sso-cert" {
		t.Errorf("expected SsoExtensionList to survive unchanged, got %+v", sent.SsoExtensionList)
	}
	if sent.SkipSetupAssistant == nil || sent.SkipSetupAssistant.SkipSiri == nil || !*sent.SkipSetupAssistant.SkipSiri {
		t.Errorf("expected SkipSetupAssistant to survive unchanged, got %+v", sent.SkipSetupAssistant)
	}
	if sent.General.Password != "unlock-secret" {
		t.Errorf("expected General.Password to survive unchanged, got %q", sent.General.Password)
	}
	if sent.General.AllowRemoval != "Forbidden" {
		t.Errorf("expected General.AllowRemoval to survive unchanged, got %q", sent.General.AllowRemoval)
	}
	if sent.General.ExpirationDate != "2030-01-01" {
		t.Errorf("expected General.ExpirationDate to survive unchanged, got %q", sent.General.ExpirationDate)
	}
	if !reflect.DeepEqual(sent.General.AssignedSchedule, []*int{intPtr(5)}) {
		t.Errorf("expected General.AssignedSchedule to survive unchanged, got %+v", sent.General.AssignedSchedule)
	}
}

// --- Android ---

func liveAndroidEntity() *sdk.AndroidDeviceProfileV2Entity {
	return &sdk.AndroidDeviceProfileV2Entity{
		General: &sdk.GeneralPayloadV2Entity{
			Name:             "Live Name",
			Description:      "Live Description",
			ProfileID:        intPtr(999),
			Version:          intPtr(2),
			Password:         "android-unlock",
			AllowRemoval:     "Forbidden",
			ExpirationDate:   "2031-06-15",
			AssignedSchedule: []*int{intPtr(9)},
		},
		AndroidForWorkApplicationControl: &sdk.AndroidForWorkApplicationControlPayloadV2Entity{
			DisableAccessToBlacklistedApps: sdk.BoolPtr(true),
		},
	}
}

func TestProfileResourceUpdate_Android_RMW(t *testing.T) {
	t.Parallel()

	t.Run("preserves unmodeled section and General fields, reflects modeled changes", func(t *testing.T) {
		fake := &fakeProfileService{getResult: &sdk.ProfileResult{Android: liveAndroidEntity()}}
		planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAndroid), map[string]tftypes.Value{
			"id":                  stringVal("999"),
			"lock_screen_message": stringVal("Updated Lock Message"),
		})

		resp := runRMWUpdate(t, fake, planValues)
		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected errors: %v", resp.Diagnostics.Errors())
		}
		if fake.updateCalls != 1 {
			t.Fatalf("expected exactly 1 Update call, got %d", fake.updateCalls)
		}
		sent, ok := fake.updateEntity.(*sdk.AndroidDeviceProfileV2Entity)
		if !ok {
			t.Fatalf("expected *sdk.AndroidDeviceProfileV2Entity, got %T", fake.updateEntity)
		}

		// (a) unmodeled section preserved.
		if sent.AndroidForWorkApplicationControl == nil ||
			sent.AndroidForWorkApplicationControl.DisableAccessToBlacklistedApps == nil ||
			!*sent.AndroidForWorkApplicationControl.DisableAccessToBlacklistedApps {
			t.Errorf("expected AndroidForWorkApplicationControl to survive unchanged, got %+v", sent.AndroidForWorkApplicationControl)
		}

		// (a) unmodeled General fields survive unchanged.
		if sent.General.Password != "android-unlock" {
			t.Errorf("expected General.Password to survive unchanged, got %q", sent.General.Password)
		}
		if sent.General.AllowRemoval != "Forbidden" {
			t.Errorf("expected General.AllowRemoval to survive unchanged, got %q", sent.General.AllowRemoval)
		}
		if sent.General.ExpirationDate != "2031-06-15" {
			t.Errorf("expected General.ExpirationDate to survive unchanged, got %q", sent.General.ExpirationDate)
		}
		if !reflect.DeepEqual(sent.General.AssignedSchedule, []*int{intPtr(9)}) {
			t.Errorf("expected General.AssignedSchedule to survive unchanged, got %+v", sent.General.AssignedSchedule)
		}

		// (b) modeled section reflects plan.
		if sent.AndroidForWorkCustomMessages == nil || sent.AndroidForWorkCustomMessages.LockScreenMessage != "Updated Lock Message" {
			t.Errorf("expected AndroidForWorkCustomMessages.LockScreenMessage from plan, got %+v", sent.AndroidForWorkCustomMessages)
		}

		// version bump: live General.Version was 2 -> sent must be 3, with
		// CreateNewVersion true and ProfileID set to the profile id.
		if sent.General.Version == nil || *sent.General.Version != 3 {
			t.Errorf("expected General.Version 3 (bumped from live 2), got %v", sent.General.Version)
		}
		if sent.General.CreateNewVersion == nil || !*sent.General.CreateNewVersion {
			t.Errorf("expected General.CreateNewVersion true, got %v", sent.General.CreateNewVersion)
		}
		if sent.General.ProfileID == nil || *sent.General.ProfileID != 999 {
			t.Errorf("expected General.ProfileID 999, got %v", sent.General.ProfileID)
		}
	})

	t.Run("Get error aborts before Update", func(t *testing.T) {
		fake := &fakeProfileService{getErr: errGetFailed}
		planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAndroid), map[string]tftypes.Value{"id": stringVal("999")})
		resp := runRMWUpdate(t, fake, planValues)
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected error diagnostic on Get failure")
		}
		if fake.updateCalls != 0 {
			t.Fatalf("expected Update not to be called, got %d calls", fake.updateCalls)
		}
	})

	t.Run("Get OK but nil Android entity aborts before Update", func(t *testing.T) {
		fake := &fakeProfileService{getResult: &sdk.ProfileResult{}}
		planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAndroid), map[string]tftypes.Value{"id": stringVal("999")})
		resp := runRMWUpdate(t, fake, planValues)
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected error diagnostic when Android entity is nil")
		}
		if fake.updateCalls != 0 {
			t.Fatalf("expected Update not to be called, got %d calls", fake.updateCalls)
		}
	})
}

// --- Apple iOS ---

func liveAppleiOSEntity() *sdk.AppleDeviceProfileV2Entity {
	return &sdk.AppleDeviceProfileV2Entity{
		General: &sdk.GeneralPayloadV2Entity{
			Name:             "Live Name",
			Description:      "Live Description",
			ProfileID:        intPtr(999),
			Version:          intPtr(7),
			Password:         "ios-unlock",
			AllowRemoval:     "Forbidden",
			ExpirationDate:   "2032-02-02",
			AssignedSchedule: []*int{intPtr(11)},
		},
		Restrictions: &sdk.AppleRestrictionsPayloadV2Entity{
			AcceptCookies: "2",
		},
	}
}

func TestProfileResourceUpdate_AppleiOS_RMW(t *testing.T) {
	t.Parallel()

	t.Run("preserves unmodeled section and General fields, reflects modeled changes", func(t *testing.T) {
		fake := &fakeProfileService{getResult: &sdk.ProfileResult{AppleiOS: liveAppleiOSEntity()}}
		planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAppleiOS), map[string]tftypes.Value{
			"id": stringVal("999"),
			"passcode": passcodeVal(map[string]tftypes.Value{
				"require_passcode_on_device": boolVal(true),
				"minimum_passcode_length":    int64Val(8),
			}),
		})

		resp := runRMWUpdate(t, fake, planValues)
		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected errors: %v", resp.Diagnostics.Errors())
		}
		if fake.updateCalls != 1 {
			t.Fatalf("expected exactly 1 Update call, got %d", fake.updateCalls)
		}
		sent, ok := fake.updateEntity.(*sdk.AppleDeviceProfileV2Entity)
		if !ok {
			t.Fatalf("expected *sdk.AppleDeviceProfileV2Entity, got %T", fake.updateEntity)
		}

		// (a) unmodeled section preserved.
		if sent.Restrictions == nil || sent.Restrictions.AcceptCookies != "2" {
			t.Errorf("expected Restrictions to survive unchanged, got %+v", sent.Restrictions)
		}

		// (a) unmodeled General fields survive unchanged.
		if sent.General.Password != "ios-unlock" {
			t.Errorf("expected General.Password to survive unchanged, got %q", sent.General.Password)
		}
		if sent.General.AllowRemoval != "Forbidden" {
			t.Errorf("expected General.AllowRemoval to survive unchanged, got %q", sent.General.AllowRemoval)
		}
		if sent.General.ExpirationDate != "2032-02-02" {
			t.Errorf("expected General.ExpirationDate to survive unchanged, got %q", sent.General.ExpirationDate)
		}
		if !reflect.DeepEqual(sent.General.AssignedSchedule, []*int{intPtr(11)}) {
			t.Errorf("expected General.AssignedSchedule to survive unchanged, got %+v", sent.General.AssignedSchedule)
		}

		// (b) modeled section reflects plan.
		if sent.Passcode == nil || sent.Passcode.RequirePasscodeOnDevice == nil || !*sent.Passcode.RequirePasscodeOnDevice {
			t.Errorf("expected Passcode.RequirePasscodeOnDevice true from plan, got %+v", sent.Passcode)
		}

		// version bump: live General.Version was 7 -> sent must be 8, with
		// CreateNewVersion true and ProfileID set to the profile id.
		if sent.General.Version == nil || *sent.General.Version != 8 {
			t.Errorf("expected General.Version 8 (bumped from live 7), got %v", sent.General.Version)
		}
		if sent.General.CreateNewVersion == nil || !*sent.General.CreateNewVersion {
			t.Errorf("expected General.CreateNewVersion true, got %v", sent.General.CreateNewVersion)
		}
		if sent.General.ProfileID == nil || *sent.General.ProfileID != 999 {
			t.Errorf("expected General.ProfileID 999, got %v", sent.General.ProfileID)
		}
	})

	t.Run("Get error aborts before Update", func(t *testing.T) {
		fake := &fakeProfileService{getErr: errGetFailed}
		planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAppleiOS), map[string]tftypes.Value{"id": stringVal("999")})
		resp := runRMWUpdate(t, fake, planValues)
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected error diagnostic on Get failure")
		}
		if fake.updateCalls != 0 {
			t.Fatalf("expected Update not to be called, got %d calls", fake.updateCalls)
		}
	})

	t.Run("Get OK but nil AppleiOS entity aborts before Update", func(t *testing.T) {
		fake := &fakeProfileService{getResult: &sdk.ProfileResult{}}
		planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAppleiOS), map[string]tftypes.Value{"id": stringVal("999")})
		resp := runRMWUpdate(t, fake, planValues)
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected error diagnostic when AppleiOS entity is nil")
		}
		if fake.updateCalls != 0 {
			t.Fatalf("expected Update not to be called, got %d calls", fake.updateCalls)
		}
	})
}

// --- Windows 10 ---

func liveWindows10Entity() *sdk.WinRTDeviceProfileV2Entity {
	return &sdk.WinRTDeviceProfileV2Entity{
		General: &sdk.GeneralPayloadV2Entity{
			Name:             "Live Name",
			Description:      "Live Description",
			ProfileID:        intPtr(999),
			Version:          intPtr(4),
			Password:         "win10-unlock",
			AllowRemoval:     "Forbidden",
			ExpirationDate:   "2033-03-03",
			AssignedSchedule: []*int{intPtr(13)},
		},
		AntiVirus: &sdk.WindowsDesktopAntivirusPayloadEntityV2{
			ArchiveScanning: sdk.BoolPtr(true),
		},
	}
}

func TestProfileResourceUpdate_Windows10_RMW(t *testing.T) {
	t.Parallel()

	t.Run("preserves unmodeled section and General fields, reflects modeled changes", func(t *testing.T) {
		fake := &fakeProfileService{getResult: &sdk.ProfileResult{Windows10: liveWindows10Entity()}}
		planValues := mergeValues(rmwBasePlanValues(sdk.PlatformWindows10), map[string]tftypes.Value{
			"id":          stringVal("999"),
			"name":        stringVal("New Win10 Name"),
			"description": stringVal("New Win10 Description"),
		})

		resp := runRMWUpdate(t, fake, planValues)
		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected errors: %v", resp.Diagnostics.Errors())
		}
		if fake.updateCalls != 1 {
			t.Fatalf("expected exactly 1 Update call, got %d", fake.updateCalls)
		}
		sent, ok := fake.updateEntity.(*sdk.WinRTDeviceProfileV2Entity)
		if !ok {
			t.Fatalf("expected *sdk.WinRTDeviceProfileV2Entity, got %T", fake.updateEntity)
		}

		// (a) unmodeled section preserved.
		if sent.AntiVirus == nil || sent.AntiVirus.ArchiveScanning == nil || !*sent.AntiVirus.ArchiveScanning {
			t.Errorf("expected AntiVirus to survive unchanged, got %+v", sent.AntiVirus)
		}

		// (a) unmodeled General fields survive unchanged.
		if sent.General.Password != "win10-unlock" {
			t.Errorf("expected General.Password to survive unchanged, got %q", sent.General.Password)
		}
		if sent.General.AllowRemoval != "Forbidden" {
			t.Errorf("expected General.AllowRemoval to survive unchanged, got %q", sent.General.AllowRemoval)
		}
		if sent.General.ExpirationDate != "2033-03-03" {
			t.Errorf("expected General.ExpirationDate to survive unchanged, got %q", sent.General.ExpirationDate)
		}
		if !reflect.DeepEqual(sent.General.AssignedSchedule, []*int{intPtr(13)}) {
			t.Errorf("expected General.AssignedSchedule to survive unchanged, got %+v", sent.General.AssignedSchedule)
		}

		// (b) modeled General fields reflect plan.
		if sent.General.Name != "New Win10 Name" || sent.General.Description != "New Win10 Description" {
			t.Errorf("expected General Name/Description to reflect plan, got %q/%q", sent.General.Name, sent.General.Description)
		}

		// version bump: live General.Version was 4 -> sent must be 5, with
		// CreateNewVersion true and ProfileID set to the profile id.
		if sent.General.Version == nil || *sent.General.Version != 5 {
			t.Errorf("expected General.Version 5 (bumped from live 4), got %v", sent.General.Version)
		}
		if sent.General.CreateNewVersion == nil || !*sent.General.CreateNewVersion {
			t.Errorf("expected General.CreateNewVersion true, got %v", sent.General.CreateNewVersion)
		}
		if sent.General.ProfileID == nil || *sent.General.ProfileID != 999 {
			t.Errorf("expected General.ProfileID 999, got %v", sent.General.ProfileID)
		}
	})

	t.Run("Get error aborts before Update", func(t *testing.T) {
		fake := &fakeProfileService{getErr: errGetFailed}
		planValues := mergeValues(rmwBasePlanValues(sdk.PlatformWindows10), map[string]tftypes.Value{"id": stringVal("999")})
		resp := runRMWUpdate(t, fake, planValues)
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected error diagnostic on Get failure")
		}
		if fake.updateCalls != 0 {
			t.Fatalf("expected Update not to be called, got %d calls", fake.updateCalls)
		}
	})

	t.Run("Get OK but nil Windows10 entity aborts before Update", func(t *testing.T) {
		fake := &fakeProfileService{getResult: &sdk.ProfileResult{}}
		planValues := mergeValues(rmwBasePlanValues(sdk.PlatformWindows10), map[string]tftypes.Value{"id": stringVal("999")})
		resp := runRMWUpdate(t, fake, planValues)
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected error diagnostic when Windows10 entity is nil")
		}
		if fake.updateCalls != 0 {
			t.Fatalf("expected Update not to be called, got %d calls", fake.updateCalls)
		}
	})
}

// --- Windows Rugged (QNX) ---

func liveWindowsRuggedEntity() *sdk.QnxDeviceProfileEntityV2 {
	return &sdk.QnxDeviceProfileEntityV2{
		General: &sdk.GeneralPayloadV2Entity{
			Name:             "Live Name",
			Description:      "Live Description",
			ProfileID:        intPtr(999),
			Version:          intPtr(1),
			Password:         "qnx-unlock",
			AllowRemoval:     "Forbidden",
			ExpirationDate:   "2034-04-04",
			AssignedSchedule: []*int{intPtr(17)},
		},
		CustomAttributePayload: &sdk.QnxCustomAttributePayloadEntityV2{
			CustomAttributes: []sdk.QnxProfileCustomAttributeEntityV2{
				{Name: "live-attr", Value: "live-value"},
			},
		},
	}
}

func TestProfileResourceUpdate_WindowsRugged_RMW(t *testing.T) {
	t.Parallel()

	t.Run("preserves unmodeled section and General fields, reflects modeled changes", func(t *testing.T) {
		fake := &fakeProfileService{getResult: &sdk.ProfileResult{WindowsRugged: liveWindowsRuggedEntity()}}
		planValues := mergeValues(rmwBasePlanValues(sdk.PlatformWindowsRugged), map[string]tftypes.Value{
			"id":          stringVal("999"),
			"name":        stringVal("New Rugged Name"),
			"description": stringVal("New Rugged Description"),
		})

		resp := runRMWUpdate(t, fake, planValues)
		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected errors: %v", resp.Diagnostics.Errors())
		}
		if fake.updateCalls != 1 {
			t.Fatalf("expected exactly 1 Update call, got %d", fake.updateCalls)
		}
		sent, ok := fake.updateEntity.(*sdk.QnxDeviceProfileEntityV2)
		if !ok {
			t.Fatalf("expected *sdk.QnxDeviceProfileEntityV2, got %T", fake.updateEntity)
		}

		// (a) unmodeled section preserved.
		if sent.CustomAttributePayload == nil || len(sent.CustomAttributePayload.CustomAttributes) != 1 ||
			sent.CustomAttributePayload.CustomAttributes[0].Name != "live-attr" {
			t.Errorf("expected CustomAttributePayload to survive unchanged, got %+v", sent.CustomAttributePayload)
		}

		// (a) unmodeled General fields survive unchanged.
		if sent.General.Password != "qnx-unlock" {
			t.Errorf("expected General.Password to survive unchanged, got %q", sent.General.Password)
		}
		if sent.General.AllowRemoval != "Forbidden" {
			t.Errorf("expected General.AllowRemoval to survive unchanged, got %q", sent.General.AllowRemoval)
		}
		if sent.General.ExpirationDate != "2034-04-04" {
			t.Errorf("expected General.ExpirationDate to survive unchanged, got %q", sent.General.ExpirationDate)
		}
		if !reflect.DeepEqual(sent.General.AssignedSchedule, []*int{intPtr(17)}) {
			t.Errorf("expected General.AssignedSchedule to survive unchanged, got %+v", sent.General.AssignedSchedule)
		}

		// (b) modeled General fields reflect plan.
		if sent.General.Name != "New Rugged Name" || sent.General.Description != "New Rugged Description" {
			t.Errorf("expected General Name/Description to reflect plan, got %q/%q", sent.General.Name, sent.General.Description)
		}

		// version bump: live General.Version was 1 -> sent must be 2, with
		// CreateNewVersion true and ProfileID set to the profile id.
		if sent.General.Version == nil || *sent.General.Version != 2 {
			t.Errorf("expected General.Version 2 (bumped from live 1), got %v", sent.General.Version)
		}
		if sent.General.CreateNewVersion == nil || !*sent.General.CreateNewVersion {
			t.Errorf("expected General.CreateNewVersion true, got %v", sent.General.CreateNewVersion)
		}
		if sent.General.ProfileID == nil || *sent.General.ProfileID != 999 {
			t.Errorf("expected General.ProfileID 999, got %v", sent.General.ProfileID)
		}
	})

	t.Run("Get error aborts before Update", func(t *testing.T) {
		fake := &fakeProfileService{getErr: errGetFailed}
		planValues := mergeValues(rmwBasePlanValues(sdk.PlatformWindowsRugged), map[string]tftypes.Value{"id": stringVal("999")})
		resp := runRMWUpdate(t, fake, planValues)
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected error diagnostic on Get failure")
		}
		if fake.updateCalls != 0 {
			t.Fatalf("expected Update not to be called, got %d calls", fake.updateCalls)
		}
	})

	t.Run("Get OK but nil WindowsRugged entity aborts before Update", func(t *testing.T) {
		fake := &fakeProfileService{getResult: &sdk.ProfileResult{}}
		planValues := mergeValues(rmwBasePlanValues(sdk.PlatformWindowsRugged), map[string]tftypes.Value{"id": stringVal("999")})
		resp := runRMWUpdate(t, fake, planValues)
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected error diagnostic when WindowsRugged entity is nil")
		}
		if fake.updateCalls != 0 {
			t.Fatalf("expected Update not to be called, got %d calls", fake.updateCalls)
		}
	})
}

// --- Linux (V4) ---

func liveLinuxEntity() *sdk.LinuxDeviceProfileEntity1V4 {
	return &sdk.LinuxDeviceProfileEntity1V4{
		General: &sdk.GeneralPayloadV4Entity{
			Name:             "Live Name",
			Description:      "Live Description",
			ProfileID:        intPtr(999),
			Version:          intPtr(6),
			Password:         "linux-unlock",
			AllowRemoval:     "Forbidden",
			ExpirationDate:   "2035-05-05",
			AssignedSchedule: []*int{intPtr(19)},
		},
		Wifis: []sdk.LinuxWifiPayloadEntityV4{
			{Identity: "live-wifi-identity"},
		},
	}
}

func TestProfileResourceUpdate_Linux_RMW(t *testing.T) {
	t.Parallel()

	t.Run("preserves unmodeled section and General fields, reflects modeled changes", func(t *testing.T) {
		fake := &fakeProfileService{getResult: &sdk.ProfileResult{Linux: liveLinuxEntity()}}
		planValues := mergeValues(rmwBasePlanValues(profileplatform.PlatformLinuxUser), map[string]tftypes.Value{
			"id":            stringVal("999"),
			"name":          stringVal("New Linux Name"),
			"description":   stringVal("New Linux Description"),
			"profile_scope": stringVal("Production"),
		})

		resp := runRMWUpdate(t, fake, planValues)
		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected errors: %v", resp.Diagnostics.Errors())
		}
		if fake.updateCalls != 1 {
			t.Fatalf("expected exactly 1 Update call, got %d", fake.updateCalls)
		}
		sent, ok := fake.updateEntity.(*sdk.LinuxDeviceProfileEntity1V4)
		if !ok {
			t.Fatalf("expected *sdk.LinuxDeviceProfileEntity1V4, got %T", fake.updateEntity)
		}

		// (a) unmodeled section preserved.
		if len(sent.Wifis) != 1 || sent.Wifis[0].Identity != "live-wifi-identity" {
			t.Errorf("expected Wifis to survive unchanged, got %+v", sent.Wifis)
		}

		// (a) unmodeled General fields survive unchanged.
		if sent.General.Password != "linux-unlock" {
			t.Errorf("expected General.Password to survive unchanged, got %q", sent.General.Password)
		}
		if sent.General.AllowRemoval != "Forbidden" {
			t.Errorf("expected General.AllowRemoval to survive unchanged, got %q", sent.General.AllowRemoval)
		}
		if sent.General.ExpirationDate != "2035-05-05" {
			t.Errorf("expected General.ExpirationDate to survive unchanged, got %q", sent.General.ExpirationDate)
		}
		if !reflect.DeepEqual(sent.General.AssignedSchedule, []*int{intPtr(19)}) {
			t.Errorf("expected General.AssignedSchedule to survive unchanged, got %+v", sent.General.AssignedSchedule)
		}

		// (b) modeled General fields reflect plan.
		if sent.General.Name != "New Linux Name" || sent.General.Description != "New Linux Description" {
			t.Errorf("expected General Name/Description to reflect plan, got %q/%q", sent.General.Name, sent.General.Description)
		}

		// version bump: live General.Version was 6 -> sent must be 7, with
		// CreateNewVersion true and ProfileID set to the profile id.
		if sent.General.Version == nil || *sent.General.Version != 7 {
			t.Errorf("expected General.Version 7 (bumped from live 6), got %v", sent.General.Version)
		}
		if sent.General.CreateNewVersion == nil || !*sent.General.CreateNewVersion {
			t.Errorf("expected General.CreateNewVersion true, got %v", sent.General.CreateNewVersion)
		}
		if sent.General.ProfileID == nil || *sent.General.ProfileID != 999 {
			t.Errorf("expected General.ProfileID 999, got %v", sent.General.ProfileID)
		}
	})

	t.Run("Get error aborts before Update", func(t *testing.T) {
		fake := &fakeProfileService{getErr: errGetFailed}
		planValues := mergeValues(rmwBasePlanValues(profileplatform.PlatformLinuxUser), map[string]tftypes.Value{
			"id": stringVal("999"), "profile_scope": stringVal("Production"),
		})
		resp := runRMWUpdate(t, fake, planValues)
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected error diagnostic on Get failure")
		}
		if fake.updateCalls != 0 {
			t.Fatalf("expected Update not to be called, got %d calls", fake.updateCalls)
		}
	})

	t.Run("Get OK but nil Linux entity aborts before Update", func(t *testing.T) {
		fake := &fakeProfileService{getResult: &sdk.ProfileResult{}}
		planValues := mergeValues(rmwBasePlanValues(profileplatform.PlatformLinuxUser), map[string]tftypes.Value{
			"id": stringVal("999"), "profile_scope": stringVal("Production"),
		})
		resp := runRMWUpdate(t, fake, planValues)
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected error diagnostic when Linux entity is nil")
		}
		if fake.updateCalls != 0 {
			t.Fatalf("expected Update not to be called, got %d calls", fake.updateCalls)
		}
	})
}

// --- direct overlay function unit tests ---

func TestOverlayAppleOsXUpdateEntity_Direct(t *testing.T) {
	t.Parallel()
	live := liveAppleOsXEntity()
	planned := &sdk.AppleOsXDeviceProfileEntityV2{
		General: &sdk.GeneralPayloadV2Entity{Name: "Planned Name", Description: "Planned Description"},
		// Restrictions intentionally nil, mimicking a plan with no restrictions block.
	}

	out := profileplatform.OverlayAppleOsXUpdateEntity(live, planned)

	if out.General.Name != "Planned Name" || out.General.Description != "Planned Description" {
		t.Errorf("expected modeled General fields from planned, got %+v", out.General)
	}
	if out.General.Password != "unlock-secret" {
		t.Errorf("expected unmodeled General.Password to survive, got %q", out.General.Password)
	}
	if out.Restrictions != nil {
		t.Errorf("expected Restrictions to be cleared (nil), got %+v", out.Restrictions)
	}
	if len(out.SsoExtensionList) != 1 {
		t.Errorf("expected SsoExtensionList to survive unchanged, got %+v", out.SsoExtensionList)
	}
}

func TestOverlayLinuxUpdateEntity_Direct(t *testing.T) {
	t.Parallel()
	live := liveLinuxEntity()
	planned := &sdk.LinuxDeviceProfileEntity1V4{
		General: &sdk.GeneralPayloadV4Entity{Name: "Planned Linux Name"},
	}

	out := profileplatform.OverlayLinuxUpdateEntity(live, planned)

	if out.General.Name != "Planned Linux Name" {
		t.Errorf("expected modeled General.Name from planned, got %q", out.General.Name)
	}
	if out.General.Password != "linux-unlock" {
		t.Errorf("expected unmodeled General.Password to survive, got %q", out.General.Password)
	}
	if len(out.Wifis) != 1 {
		t.Errorf("expected Wifis to survive unchanged, got %+v", out.Wifis)
	}
}
