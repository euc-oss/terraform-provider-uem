package profile

import (
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Tests in this file cover ValidateConfig's handling of wholly-unknown
// nested blocks and lists. A naive implementation that decodes the entire
// config with a single typed req.Config.Get(ctx, &data) into
// profilemodels.ProfileResourceModel breaks here: when any pointer-to-struct
// field (passcode, restrictions, disk_encryption, gatekeeper, ...) or list
// field (custom_settings_list, network_list, ...) is wholly UNKNOWN — as
// happens when the value is derived from a not-yet-created resource/attribute
// at plan time — terraform-plugin-framework's reflection-based decode raises
// a "Value Conversion Error" diagnostic and ValidateConfig returns early. An
// unknown value must never produce a validate/plan-time error.
//
// After the fix, ValidateConfig reads only the individual attributes it
// needs via req.Config.GetAttribute, so an unknown nested block/list never
// touches the whole-model reflection decode at all.

// TestValidateConfig_UnknownPasscodeBlock_NoDiagnostics proves a wholly
// unknown passcode block on AppleOsX produces no diagnostics whatsoever
// (not even the framework's own conversion error).
func TestValidateConfig_UnknownPasscodeBlock_NoDiagnostics(t *testing.T) {
	t.Parallel()

	resp := runValidateConfigWithPasscode(t, sdk.PlatformAppleOsX, tftypes.NewValue(passcodeObjectType(), tftypes.UnknownValue))

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no diagnostics for a wholly-unknown passcode block, got: %s", diagSummaries(resp))
	}
}

// TestValidateConfig_UnknownRestrictionsBlock_NoDiagnostics proves a wholly
// unknown restrictions block produces no diagnostics.
func TestValidateConfig_UnknownRestrictionsBlock_NoDiagnostics(t *testing.T) {
	t.Parallel()

	resp := runValidateConfigValues(t, map[string]tftypes.Value{
		"id":           nullString(),
		"name":         stringVal("Test Profile"),
		"platform":     stringVal(sdk.PlatformAppleOsX),
		"restrictions": tftypes.NewValue(restrictionsObjectType(), tftypes.UnknownValue),
	})

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no diagnostics for a wholly-unknown restrictions block, got: %s", diagSummaries(resp))
	}
}

// TestValidateConfig_UnknownCustomSettingsList_NoDiagnostics proves a wholly
// unknown list attribute (custom_settings_list) produces no diagnostics.
func TestValidateConfig_UnknownCustomSettingsList_NoDiagnostics(t *testing.T) {
	t.Parallel()

	resp := runValidateConfigValues(t, map[string]tftypes.Value{
		"id":                   nullString(),
		"name":                 stringVal("Test Profile"),
		"platform":             stringVal(sdk.PlatformAppleOsX),
		"custom_settings_list": tftypes.NewValue(tftypes.List{ElementType: customSettingsListItemType()}, tftypes.UnknownValue),
	})

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no diagnostics for a wholly-unknown custom_settings_list, got: %s", diagSummaries(resp))
	}
}

// TestValidateConfig_UnknownNetworkList_NoDiagnostics proves a wholly
// unknown list attribute (network_list) produces no diagnostics.
func TestValidateConfig_UnknownNetworkList_NoDiagnostics(t *testing.T) {
	t.Parallel()

	resp := runValidateConfigValues(t, map[string]tftypes.Value{
		"id":           nullString(),
		"name":         stringVal("Test Profile"),
		"platform":     stringVal(sdk.PlatformAppleOsX),
		"network_list": tftypes.NewValue(tftypes.List{ElementType: networkListItemType()}, tftypes.UnknownValue),
	})

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no diagnostics for a wholly-unknown network_list, got: %s", diagSummaries(resp))
	}
}

// TestValidateConfig_UnknownPlatform_NoDiagnostics proves a wholly unknown
// platform value alone (everything else null) produces no diagnostics.
func TestValidateConfig_UnknownPlatform_NoDiagnostics(t *testing.T) {
	t.Parallel()

	resp := runValidateConfigValues(t, map[string]tftypes.Value{
		"id":       nullString(),
		"name":     stringVal("Test Profile"),
		"platform": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no diagnostics for an unknown platform, got: %s", diagSummaries(resp))
	}
}
