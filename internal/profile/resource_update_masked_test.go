package profile

// zf1b: full-Update-path tests for the permanent masked-secret guard (internal-task). These drive the same real ProfileResource.Update path as
// resource_update_rmw_test.go (reusing its fakeProfileService,
// newRMWTestResource, rmwBasePlanValues, runRMWUpdate, and live*Entity
// builders), asserting that:
//
//   - a "*****" left by UEM in an UNMODELED string leaf of the live entity
//     (section or General field) aborts the Update with an error
//     diagnostic naming the offending path, and svc.Update is never
//     called;
//   - the same unmodeled leaf with a real-looking value passes through
//     and Update proceeds;
//   - a "*****" coming from the PLAN into a MODELED field/section (a user
//     legitimately typing the literal asterisks) does not trip the guard;
//   - the error diagnostic never leaks any OTHER secret value present on
//     the live entity (the "canary" checks).

import (
	"errors"
	"strings"
	"testing"

	profileplatform "github.com/euc-oss/terraform-provider-uem/internal/profile/platform"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestRefuseIfMaskedUnmodeled_InspectErr_FailsClosed covers correction 1's
// signature change to refuseIfMaskedUnmodeled: a non-nil inspectErr (the
// MaskedUnmodeled* detector itself failed to inspect live) must ALWAYS
// produce an error, regardless of paths, naming the profile id and stating
// that the live profile could not be checked.
func TestRefuseIfMaskedUnmodeled_InspectErr_FailsClosed(t *testing.T) {
	t.Parallel()
	inspectErr := errors.New("boom: could not marshal live entity")

	err := refuseIfMaskedUnmodeled(4242, nil, inspectErr)
	if err == nil {
		t.Fatal("refuseIfMaskedUnmodeled with a non-nil inspectErr: got nil error, want non-nil (fail closed)")
	}
	msg := err.Error()
	if !strings.Contains(msg, "4242") {
		t.Errorf("refuseIfMaskedUnmodeled error %q does not mention the profile id", msg)
	}
	if !strings.Contains(msg, "unable to check") {
		t.Errorf("refuseIfMaskedUnmodeled error %q does not mention \"unable to check\"", msg)
	}
	if !errors.Is(err, inspectErr) {
		t.Errorf("refuseIfMaskedUnmodeled error does not wrap the original inspectErr: %v", err)
	}
}

func TestRefuseIfMaskedUnmodeled_NoPathsNoErr_OK(t *testing.T) {
	t.Parallel()
	if err := refuseIfMaskedUnmodeled(1, nil, nil); err != nil {
		t.Errorf("refuseIfMaskedUnmodeled(1, nil, nil) = %v, want nil", err)
	}
}

func TestRefuseIfMaskedUnmodeled_Paths_Refused(t *testing.T) {
	t.Parallel()
	err := refuseIfMaskedUnmodeled(7, []string{"General.Password"}, nil)
	if err == nil {
		t.Fatal("refuseIfMaskedUnmodeled with non-empty paths: got nil error, want non-nil")
	}
	if !strings.Contains(err.Error(), "General.Password") {
		t.Errorf("refuseIfMaskedUnmodeled error %q does not name the masked path", err.Error())
	}
}

const zf1bCanary = "zf1b-canary-real"

// --- macOS (AppleOsX) ---

func TestProfileResourceUpdate_AppleOsX_MaskedUnmodeled_Refused(t *testing.T) {
	t.Parallel()
	live := liveAppleOsXEntity()
	live.EmailList = []sdk.AppleOsXEmailPayloadEntityV2{{IncomingPassword: profileplatform.UEMMaskedSecret}}
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{AppleOsX: live}}
	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{"id": stringVal("999")})

	resp := runRMWUpdate(t, fake, planValues)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error diagnostic when live has a masked unmodeled secret")
	}
	if fake.updateCalls != 0 {
		t.Fatalf("expected Update not to be called, got %d calls", fake.updateCalls)
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Detail(), "EmailList[0].IncomingPassword") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected diagnostic to name EmailList[0].IncomingPassword, got %v", resp.Diagnostics.Errors())
	}
}

func TestProfileResourceUpdate_AppleOsX_MaskedGeneralPassword_Refused(t *testing.T) {
	t.Parallel()
	live := liveAppleOsXEntity()
	live.General.Password = profileplatform.UEMMaskedSecret
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{AppleOsX: live}}
	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{"id": stringVal("999")})

	resp := runRMWUpdate(t, fake, planValues)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error diagnostic when live General.Password is masked")
	}
	if fake.updateCalls != 0 {
		t.Fatalf("expected Update not to be called, got %d calls", fake.updateCalls)
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Detail(), "General.Password") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected diagnostic to name General.Password, got %v", resp.Diagnostics.Errors())
	}
}

func TestProfileResourceUpdate_AppleOsX_RealUnmodeledSecret_Proceeds(t *testing.T) {
	t.Parallel()
	live := liveAppleOsXEntity()
	live.EmailList = []sdk.AppleOsXEmailPayloadEntityV2{{IncomingPassword: "hunter2-real"}}
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{AppleOsX: live}}
	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{"id": stringVal("999")})

	resp := runRMWUpdate(t, fake, planValues)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error diagnostics: %v", resp.Diagnostics.Errors())
	}
	if fake.updateCalls != 1 {
		t.Fatalf("expected exactly 1 Update call, got %d", fake.updateCalls)
	}
	sent, ok := fake.updateEntity.(*sdk.AppleOsXDeviceProfileEntityV2)
	if !ok {
		t.Fatalf("expected *sdk.AppleOsXDeviceProfileEntityV2, got %T", fake.updateEntity)
	}
	if len(sent.EmailList) != 1 || sent.EmailList[0].IncomingPassword != "hunter2-real" {
		t.Errorf("expected real unmodeled secret to pass through, got %+v", sent.EmailList)
	}
}

func TestProfileResourceUpdate_AppleOsX_MaskedModeledFieldFromPlan_Proceeds(t *testing.T) {
	t.Parallel()
	live := liveAppleOsXEntity() // no masked unmodeled leaves
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{AppleOsX: live}}
	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{
		"id": stringVal("999"),
		"network_list": networkListVal([]map[string]tftypes.Value{
			{
				"service_set_identifier": stringVal("plan-ssid"),
				"password":               stringVal(profileplatform.UEMMaskedSecret), // user legitimately typed "*****"
			},
		}),
	})

	resp := runRMWUpdate(t, fake, planValues)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error diagnostics: %v", resp.Diagnostics.Errors())
	}
	if fake.updateCalls != 1 {
		t.Fatalf("expected exactly 1 Update call, got %d", fake.updateCalls)
	}
	sent, ok := fake.updateEntity.(*sdk.AppleOsXDeviceProfileEntityV2)
	if !ok {
		t.Fatalf("expected *sdk.AppleOsXDeviceProfileEntityV2, got %T", fake.updateEntity)
	}
	if len(sent.NetworkList) != 1 || sent.NetworkList[0].Password != profileplatform.UEMMaskedSecret {
		t.Errorf("expected planned network password %q to pass through unmodified, got %+v", profileplatform.UEMMaskedSecret, sent.NetworkList)
	}
}

func TestProfileResourceUpdate_AppleOsX_MaskedDiagnostic_NoCanaryLeak(t *testing.T) {
	t.Parallel()
	live := liveAppleOsXEntity()
	live.EmailList = []sdk.AppleOsXEmailPayloadEntityV2{
		{IncomingPassword: profileplatform.UEMMaskedSecret},
		{OutgoingPassword: zf1bCanary},
	}
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{AppleOsX: live}}
	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{"id": stringVal("999")})

	resp := runRMWUpdate(t, fake, planValues)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error diagnostic")
	}
	if fake.updateCalls != 0 {
		t.Fatalf("expected Update not to be called, got %d calls", fake.updateCalls)
	}
	var sawMaskedPath bool
	for _, d := range resp.Diagnostics.Errors() {
		text := d.Summary() + " " + d.Detail()
		if strings.Contains(text, zf1bCanary) {
			t.Fatalf("diagnostic leaked canary real secret: %q", text)
		}
		if strings.Contains(text, "EmailList[0].IncomingPassword") {
			sawMaskedPath = true
		}
	}
	if !sawMaskedPath {
		t.Error("expected diagnostic to name the masked path EmailList[0].IncomingPassword")
	}
}

// --- Android ---

func TestProfileResourceUpdate_Android_MaskedUnmodeled_Refused(t *testing.T) {
	t.Parallel()
	live := liveAndroidEntity()
	live.AndroidForWorkCredentialsList = []sdk.AndroidForWorkCredentialsPayloadV2Entity{
		{CertificatePassword: profileplatform.UEMMaskedSecret},
	}
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{Android: live}}
	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAndroid), map[string]tftypes.Value{"id": stringVal("999")})

	resp := runRMWUpdate(t, fake, planValues)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error diagnostic when live has a masked unmodeled secret")
	}
	if fake.updateCalls != 0 {
		t.Fatalf("expected Update not to be called, got %d calls", fake.updateCalls)
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Detail(), "AndroidForWorkCredentialsList[0].CertificatePassword") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected diagnostic to name AndroidForWorkCredentialsList[0].CertificatePassword, got %v", resp.Diagnostics.Errors())
	}
}

func TestProfileResourceUpdate_Android_RealUnmodeledSecret_Proceeds(t *testing.T) {
	t.Parallel()
	live := liveAndroidEntity()
	live.AndroidForWorkCredentialsList = []sdk.AndroidForWorkCredentialsPayloadV2Entity{
		{CertificatePassword: "hunter2-real"},
	}
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{Android: live}}
	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAndroid), map[string]tftypes.Value{"id": stringVal("999")})

	resp := runRMWUpdate(t, fake, planValues)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error diagnostics: %v", resp.Diagnostics.Errors())
	}
	if fake.updateCalls != 1 {
		t.Fatalf("expected exactly 1 Update call, got %d", fake.updateCalls)
	}
}

// --- Apple iOS ---

func TestProfileResourceUpdate_AppleiOS_MaskedUnmodeled_Refused(t *testing.T) {
	t.Parallel()
	live := liveAppleiOSEntity()
	live.EmailList = []sdk.AppleEmailPayloadV2Entity{{IncomingPassword: profileplatform.UEMMaskedSecret}}
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{AppleiOS: live}}
	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAppleiOS), map[string]tftypes.Value{"id": stringVal("999")})

	resp := runRMWUpdate(t, fake, planValues)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error diagnostic when live has a masked unmodeled secret")
	}
	if fake.updateCalls != 0 {
		t.Fatalf("expected Update not to be called, got %d calls", fake.updateCalls)
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Detail(), "EmailList[0].IncomingPassword") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected diagnostic to name EmailList[0].IncomingPassword, got %v", resp.Diagnostics.Errors())
	}
}

func TestProfileResourceUpdate_AppleiOS_RealUnmodeledSecret_Proceeds(t *testing.T) {
	t.Parallel()
	live := liveAppleiOSEntity()
	live.EmailList = []sdk.AppleEmailPayloadV2Entity{{IncomingPassword: "hunter2-real"}}
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{AppleiOS: live}}
	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAppleiOS), map[string]tftypes.Value{"id": stringVal("999")})

	resp := runRMWUpdate(t, fake, planValues)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error diagnostics: %v", resp.Diagnostics.Errors())
	}
	if fake.updateCalls != 1 {
		t.Fatalf("expected exactly 1 Update call, got %d", fake.updateCalls)
	}
}

// --- Windows 10 ---

func TestProfileResourceUpdate_Windows10_MaskedUnmodeled_Refused(t *testing.T) {
	t.Parallel()
	live := liveWindows10Entity()
	live.Credentials = &sdk.WindowsDesktopCredentialsPayloadEntityV2{CertificatePassword: profileplatform.UEMMaskedSecret}
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{Windows10: live}}
	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformWindows10), map[string]tftypes.Value{"id": stringVal("999")})

	resp := runRMWUpdate(t, fake, planValues)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error diagnostic when live has a masked unmodeled secret")
	}
	if fake.updateCalls != 0 {
		t.Fatalf("expected Update not to be called, got %d calls", fake.updateCalls)
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Detail(), "Credentials.certificatePassword") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected diagnostic to name Credentials.certificatePassword, got %v", resp.Diagnostics.Errors())
	}
}

func TestProfileResourceUpdate_Windows10_RealUnmodeledSecret_Proceeds(t *testing.T) {
	t.Parallel()
	live := liveWindows10Entity()
	live.Credentials = &sdk.WindowsDesktopCredentialsPayloadEntityV2{CertificatePassword: "hunter2-real"}
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{Windows10: live}}
	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformWindows10), map[string]tftypes.Value{"id": stringVal("999")})

	resp := runRMWUpdate(t, fake, planValues)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error diagnostics: %v", resp.Diagnostics.Errors())
	}
	if fake.updateCalls != 1 {
		t.Fatalf("expected exactly 1 Update call, got %d", fake.updateCalls)
	}
}

// --- Windows Rugged (QNX) ---

func TestProfileResourceUpdate_WindowsRugged_MaskedUnmodeled_Refused(t *testing.T) {
	t.Parallel()
	live := liveWindowsRuggedEntity()
	live.CustomAttributePayload.CustomAttributes[0].Value = profileplatform.UEMMaskedSecret
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{WindowsRugged: live}}
	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformWindowsRugged), map[string]tftypes.Value{"id": stringVal("999")})

	resp := runRMWUpdate(t, fake, planValues)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error diagnostic when live has a masked unmodeled secret")
	}
	if fake.updateCalls != 0 {
		t.Fatalf("expected Update not to be called, got %d calls", fake.updateCalls)
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Detail(), "CustomAttributePayload.CustomAttributes[0].Value") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected diagnostic to name CustomAttributePayload.CustomAttributes[0].Value, got %v", resp.Diagnostics.Errors())
	}
}

func TestProfileResourceUpdate_WindowsRugged_RealUnmodeledSecret_Proceeds(t *testing.T) {
	t.Parallel()
	live := liveWindowsRuggedEntity()
	live.CustomAttributePayload.CustomAttributes[0].Value = "hunter2-real"
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{WindowsRugged: live}}
	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformWindowsRugged), map[string]tftypes.Value{"id": stringVal("999")})

	resp := runRMWUpdate(t, fake, planValues)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error diagnostics: %v", resp.Diagnostics.Errors())
	}
	if fake.updateCalls != 1 {
		t.Fatalf("expected exactly 1 Update call, got %d", fake.updateCalls)
	}
}

// --- Linux (V4) ---

func TestProfileResourceUpdate_Linux_MaskedUnmodeled_Refused(t *testing.T) {
	t.Parallel()
	live := liveLinuxEntity()
	live.Wifis = []sdk.LinuxWifiPayloadEntityV4{{EnterprisePassword: profileplatform.UEMMaskedSecret}}
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{Linux: live}}
	planValues := mergeValues(rmwBasePlanValues(profileplatform.PlatformLinuxUser), map[string]tftypes.Value{
		"id": stringVal("999"), "profile_scope": stringVal("Production"),
	})

	resp := runRMWUpdate(t, fake, planValues)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error diagnostic when live has a masked unmodeled secret")
	}
	if fake.updateCalls != 0 {
		t.Fatalf("expected Update not to be called, got %d calls", fake.updateCalls)
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Detail(), "Wifis[0].EnterprisePassword") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected diagnostic to name Wifis[0].EnterprisePassword, got %v", resp.Diagnostics.Errors())
	}
}

func TestProfileResourceUpdate_Linux_MaskedGeneralPassword_Refused(t *testing.T) {
	t.Parallel()
	live := liveLinuxEntity()
	live.General.Password = profileplatform.UEMMaskedSecret
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{Linux: live}}
	planValues := mergeValues(rmwBasePlanValues(profileplatform.PlatformLinuxUser), map[string]tftypes.Value{
		"id": stringVal("999"), "profile_scope": stringVal("Production"),
	})

	resp := runRMWUpdate(t, fake, planValues)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error diagnostic when live General.Password is masked")
	}
	if fake.updateCalls != 0 {
		t.Fatalf("expected Update not to be called, got %d calls", fake.updateCalls)
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Detail(), "general.Password") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected diagnostic to name general.Password, got %v", resp.Diagnostics.Errors())
	}
}

func TestProfileResourceUpdate_Linux_RealUnmodeledSecret_Proceeds(t *testing.T) {
	t.Parallel()
	live := liveLinuxEntity()
	live.Wifis = []sdk.LinuxWifiPayloadEntityV4{{EnterprisePassword: "hunter2-real"}}
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{Linux: live}}
	planValues := mergeValues(rmwBasePlanValues(profileplatform.PlatformLinuxUser), map[string]tftypes.Value{
		"id": stringVal("999"), "profile_scope": stringVal("Production"),
	})

	resp := runRMWUpdate(t, fake, planValues)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error diagnostics: %v", resp.Diagnostics.Errors())
	}
	if fake.updateCalls != 1 {
		t.Fatalf("expected exactly 1 Update call, got %d", fake.updateCalls)
	}
}
