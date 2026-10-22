package profile

import (
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Tests in this file cover validateNetworkListWirelessRequiresSecurityType in
// internal/profile/validate.go.
//
// Live-verified behavior being guarded against (as<internal-env> tenant, macOS/AppleOsX):
// a network_list entry with network_interface = "BuiltInWireless" and no
// security_type is accepted by `terraform plan` but rejected by UEM at apply
// time with a 422 (NetworkList[0].AppleOsXNetworkPayloadEntity.SecurityType
// ... cannot be null when NetworkInterface is BuiltInWireless).

const networkListWirelessSecuritySummary = "security_type is required when network_interface is \"BuiltInWireless\""

// runValidateConfigWithNetworkList builds a tfsdk.Config with the given
// platform and network_list value (everything else defaults to null) and
// runs it through ProfileResource.ValidateConfig.
func runValidateConfigWithNetworkList(t *testing.T, platform string, networkList tftypes.Value) *resource.ValidateConfigResponse {
	t.Helper()
	return runValidateConfigValues(t, map[string]tftypes.Value{
		"id":           nullString(),
		"name":         stringVal("Test Profile"),
		"platform":     stringVal(platform),
		"network_list": networkList,
	})
}

func TestValidateConfig_NetworkList_BuiltInWireless_SecurityTypeNull_Rejected(t *testing.T) {
	t.Parallel()

	nl := networkListVal([]map[string]tftypes.Value{
		{"network_interface": stringVal("BuiltInWireless")},
	})
	resp := runValidateConfigWithNetworkList(t, sdk.PlatformAppleOsX, nl)

	wantPath := "network_list[0].security_type"
	errs := diagErrorsAtPath(resp, wantPath)
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error at %s, got %d: %s", wantPath, len(errs), diagSummaries(resp))
	}
	if errs[0] != networkListWirelessSecuritySummary {
		t.Errorf("expected summary %q, got %q", networkListWirelessSecuritySummary, errs[0])
	}
}

func TestValidateConfig_NetworkList_BuiltInWireless_SecurityTypeSet_NoError(t *testing.T) {
	t.Parallel()

	for _, st := range []string{"WPA2", "WPA3", "None"} {
		t.Run(st, func(t *testing.T) {
			t.Parallel()
			nl := networkListVal([]map[string]tftypes.Value{
				{
					"network_interface": stringVal("BuiltInWireless"),
					"security_type":     stringVal(st),
				},
			})
			resp := runValidateConfigWithNetworkList(t, sdk.PlatformAppleOsX, nl)

			if errs := diagErrorsAtPath(resp, "network_list[0].security_type"); len(errs) != 0 {
				t.Fatalf("expected no error with security_type = %q, got: %v", st, errs)
			}
		})
	}
}

func TestValidateConfig_NetworkList_BuiltInWireless_SecurityTypeUnknown_NoError(t *testing.T) {
	t.Parallel()

	nl := networkListVal([]map[string]tftypes.Value{
		{
			"network_interface": stringVal("BuiltInWireless"),
			"security_type":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		},
	})
	resp := runValidateConfigWithNetworkList(t, sdk.PlatformAppleOsX, nl)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error with security_type unknown, got: %s", diagSummaries(resp))
	}
}

func TestValidateConfig_NetworkList_NetworkInterfaceUnknown_NoError(t *testing.T) {
	t.Parallel()

	nl := networkListVal([]map[string]tftypes.Value{
		{
			"network_interface": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
			// security_type left null: if network_interface's unknown-ness
			// weren't respected, this would otherwise fire.
		},
	})
	resp := runValidateConfigWithNetworkList(t, sdk.PlatformAppleOsX, nl)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error with network_interface unknown, got: %s", diagSummaries(resp))
	}
}

// TestValidateConfig_NetworkList_OtherInterface_NoError proves the rule only
// fires for exactly "BuiltInWireless": a different (e.g. wired) interface
// with no security_type is never flagged. "FirstEthernet" is one of the 8
// wire names in allowedNetworkInterfaces (see validate.go), so this doesn't
// also trip the network_interface OneOf check.
func TestValidateConfig_NetworkList_OtherInterface_NoError(t *testing.T) {
	t.Parallel()

	nl := networkListVal([]map[string]tftypes.Value{
		{"network_interface": stringVal("FirstEthernet")},
	})
	resp := runValidateConfigWithNetworkList(t, sdk.PlatformAppleOsX, nl)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error for network_interface = \"FirstEthernet\", got: %s", diagSummaries(resp))
	}
}

// TestValidateConfig_NetworkList_OtherPlatform_NoError proves the check is
// macOS-only: buildAppleOsXNetworkListEntity is the only builder that ever
// sends this list, so an equivalent config on another platform is never
// flagged.
func TestValidateConfig_NetworkList_OtherPlatform_NoError(t *testing.T) {
	t.Parallel()

	nl := networkListVal([]map[string]tftypes.Value{
		{"network_interface": stringVal("BuiltInWireless")},
	})
	resp := runValidateConfigWithNetworkList(t, sdk.PlatformAppleiOS, nl)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error on %q, got: %s", sdk.PlatformAppleiOS, diagSummaries(resp))
	}
}

// TestValidateConfig_NetworkList_UnknownList_NoError proves a wholly unknown
// network_list produces no diagnostics (also covered generically in
// validate_unknown_blocks_test.go; repeated here for this rule's own
// documentation trail).
func TestValidateConfig_NetworkList_UnknownList_NoError(t *testing.T) {
	t.Parallel()

	resp := runValidateConfigWithNetworkList(t, sdk.PlatformAppleOsX, tftypes.NewValue(tftypes.List{ElementType: networkListItemType()}, tftypes.UnknownValue))

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error for a wholly-unknown network_list, got: %s", diagSummaries(resp))
	}
}

// TestValidateConfig_NetworkList_UnknownElement_NoError proves a wholly
// unknown element within an otherwise-known network_list produces no
// diagnostics for that element.
func TestValidateConfig_NetworkList_UnknownElement_NoError(t *testing.T) {
	t.Parallel()

	listType := tftypes.List{ElementType: networkListItemType()}
	nl := tftypes.NewValue(listType, []tftypes.Value{
		tftypes.NewValue(networkListItemType(), tftypes.UnknownValue),
	})
	resp := runValidateConfigWithNetworkList(t, sdk.PlatformAppleOsX, nl)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error for a wholly-unknown network_list element, got: %s", diagSummaries(resp))
	}
}

// TestValidateConfig_NetworkList_MultipleEntries_OnlyOffendingOneFlagged
// proves each element is checked independently and the diagnostic lands on
// the specific offending element's own path.
func TestValidateConfig_NetworkList_MultipleEntries_OnlyOffendingOneFlagged(t *testing.T) {
	t.Parallel()

	nl := networkListVal([]map[string]tftypes.Value{
		{
			"network_interface": stringVal("BuiltInWireless"),
			"security_type":     stringVal("WPA2"),
		},
		{"network_interface": stringVal("BuiltInWireless")},
	})
	resp := runValidateConfigWithNetworkList(t, sdk.PlatformAppleOsX, nl)

	if errs := diagErrorsAtPath(resp, "network_list[0].security_type"); len(errs) != 0 {
		t.Fatalf("expected no error at network_list[0].security_type, got: %v", errs)
	}
	if errs := diagErrorsAtPath(resp, "network_list[1].security_type"); len(errs) != 1 {
		t.Fatalf("expected exactly 1 error at network_list[1].security_type, got %d: %s", len(errs), diagSummaries(resp))
	}
}
