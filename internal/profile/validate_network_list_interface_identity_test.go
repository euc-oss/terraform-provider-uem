package profile

import (
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Tests in this file cover validateNetworkListNetworkInterfaceOneOf,
// validateNetworkListInnerIdentityOneOf, and
// validateNetworkListTTLSRequiresInnerIdentity in internal/profile/validate.go.
//
// Source: the canonical UEM 26.2 report, "Q5 — NetworkList /
// NetworkInterface / InnerIdentity". network_interface's 8 allowed values and
// inner_identity's 4 allowed values are the exact wire names UEM's server
// parses (AppleOsXNetworkPayloadEntity, release/26.2.0.0); inner_identity is
// additionally required whenever ttls = true
// (AppleOsXNetworkPayloadEntity.cs:628–629). Live-verified (as<internal-env> tenant):
// "Wi-Fi" is rejected with a 422 ("Invalid NetworkInterface Value");
// "BuiltInWireless" is accepted.
//
// runValidateConfigWithNetworkList and networkListVal/networkListItemType
// are shared with validate_network_list_wireless_security_test.go and
// testutils_test.go.

const networkInterfaceInvalidSummary = "Invalid network_interface"
const innerIdentityInvalidSummary = "Invalid inner_identity"
const ttlsRequiresInnerIdentitySummary = "inner_identity is required when ttls is enabled"

// --- network_interface OneOf -----------------------------------------------

func TestValidateConfig_NetworkInterface_AllAllowedValues_Accepted(t *testing.T) {
	t.Parallel()

	for _, v := range allowedNetworkInterfaces {
		t.Run(v, func(t *testing.T) {
			t.Parallel()
			nl := networkListVal([]map[string]tftypes.Value{
				{
					"network_interface": stringVal(v),
					// Avoid tripping the sibling BuiltInWireless-requires-
					// security_type rule for that one member.
					"security_type": stringVal("WPA2"),
				},
			})
			resp := runValidateConfigWithNetworkList(t, sdk.PlatformAppleOsX, nl)

			if errs := diagErrorsAtPath(resp, "network_list[0].network_interface"); len(errs) != 0 {
				t.Fatalf("expected no error for network_interface = %q, got: %v", v, errs)
			}
		})
	}
}

func TestValidateConfig_NetworkInterface_NearMisses_Rejected(t *testing.T) {
	t.Parallel()

	for _, v := range []string{"Wi-Fi", "builtinwireless", "", "Ethernet", "BUILTINWIRELESS"} {
		t.Run(v, func(t *testing.T) {
			t.Parallel()
			nl := networkListVal([]map[string]tftypes.Value{
				{"network_interface": stringVal(v)},
			})
			resp := runValidateConfigWithNetworkList(t, sdk.PlatformAppleOsX, nl)

			errs := diagErrorsAtPath(resp, "network_list[0].network_interface")
			if len(errs) != 1 {
				t.Fatalf("expected exactly 1 error for network_interface = %q, got %d: %s", v, len(errs), diagSummaries(resp))
			}
			if errs[0] != networkInterfaceInvalidSummary {
				t.Errorf("expected summary %q, got %q", networkInterfaceInvalidSummary, errs[0])
			}
		})
	}
}

func TestValidateConfig_NetworkInterface_Unknown_NoError(t *testing.T) {
	t.Parallel()

	nl := networkListVal([]map[string]tftypes.Value{
		{"network_interface": tftypes.NewValue(tftypes.String, tftypes.UnknownValue)},
	})
	resp := runValidateConfigWithNetworkList(t, sdk.PlatformAppleOsX, nl)

	if errs := diagErrorsAtPath(resp, "network_list[0].network_interface"); len(errs) != 0 {
		t.Fatalf("expected no error for unknown network_interface, got: %v", errs)
	}
}

func TestValidateConfig_NetworkInterface_Null_NoError(t *testing.T) {
	t.Parallel()

	nl := networkListVal([]map[string]tftypes.Value{
		{"service_set_identifier": stringVal("some-ssid")}, // network_interface left null
	})
	resp := runValidateConfigWithNetworkList(t, sdk.PlatformAppleOsX, nl)

	if errs := diagErrorsAtPath(resp, "network_list[0].network_interface"); len(errs) != 0 {
		t.Fatalf("expected no error for null network_interface, got: %v", errs)
	}
}

func TestValidateConfig_NetworkInterface_OtherPlatform_NoError(t *testing.T) {
	t.Parallel()

	nl := networkListVal([]map[string]tftypes.Value{
		{"network_interface": stringVal("Wi-Fi")},
	})
	resp := runValidateConfigWithNetworkList(t, sdk.PlatformAppleiOS, nl)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error on %q, got: %s", sdk.PlatformAppleiOS, diagSummaries(resp))
	}
}

// --- inner_identity OneOf ---------------------------------------------------

func TestValidateConfig_InnerIdentity_AllAllowedValues_Accepted(t *testing.T) {
	t.Parallel()

	for _, v := range allowedInnerIdentities {
		t.Run(v, func(t *testing.T) {
			t.Parallel()
			nl := networkListVal([]map[string]tftypes.Value{
				{"inner_identity": stringVal(v)},
			})
			resp := runValidateConfigWithNetworkList(t, sdk.PlatformAppleOsX, nl)

			if errs := diagErrorsAtPath(resp, "network_list[0].inner_identity"); len(errs) != 0 {
				t.Fatalf("expected no error for inner_identity = %q, got: %v", v, errs)
			}
		})
	}
}

func TestValidateConfig_InnerIdentity_NearMisses_Rejected(t *testing.T) {
	t.Parallel()

	for _, v := range []string{"Wi-Fi", "builtinwireless", "", "pap", "MSCHAPV2", "PAP2"} {
		t.Run(v, func(t *testing.T) {
			t.Parallel()
			nl := networkListVal([]map[string]tftypes.Value{
				{"inner_identity": stringVal(v)},
			})
			resp := runValidateConfigWithNetworkList(t, sdk.PlatformAppleOsX, nl)

			errs := diagErrorsAtPath(resp, "network_list[0].inner_identity")
			if len(errs) != 1 {
				t.Fatalf("expected exactly 1 error for inner_identity = %q, got %d: %s", v, len(errs), diagSummaries(resp))
			}
			if errs[0] != innerIdentityInvalidSummary {
				t.Errorf("expected summary %q, got %q", innerIdentityInvalidSummary, errs[0])
			}
		})
	}
}

func TestValidateConfig_InnerIdentity_Unknown_NoError(t *testing.T) {
	t.Parallel()

	nl := networkListVal([]map[string]tftypes.Value{
		{"inner_identity": tftypes.NewValue(tftypes.String, tftypes.UnknownValue)},
	})
	resp := runValidateConfigWithNetworkList(t, sdk.PlatformAppleOsX, nl)

	if errs := diagErrorsAtPath(resp, "network_list[0].inner_identity"); len(errs) != 0 {
		t.Fatalf("expected no error for unknown inner_identity, got: %v", errs)
	}
}

func TestValidateConfig_InnerIdentity_Null_NoError(t *testing.T) {
	t.Parallel()

	nl := networkListVal([]map[string]tftypes.Value{
		{"service_set_identifier": stringVal("some-ssid")}, // inner_identity left null
	})
	resp := runValidateConfigWithNetworkList(t, sdk.PlatformAppleOsX, nl)

	if errs := diagErrorsAtPath(resp, "network_list[0].inner_identity"); len(errs) != 0 {
		t.Fatalf("expected no error for null inner_identity, got: %v", errs)
	}
}

func TestValidateConfig_InnerIdentity_OtherPlatform_NoError(t *testing.T) {
	t.Parallel()

	nl := networkListVal([]map[string]tftypes.Value{
		{"inner_identity": stringVal("bogus")},
	})
	resp := runValidateConfigWithNetworkList(t, sdk.PlatformAppleiOS, nl)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error on %q, got: %s", sdk.PlatformAppleiOS, diagSummaries(resp))
	}
}

// --- ttls requires inner_identity -------------------------------------------

func TestValidateConfig_TTLS_InnerIdentityNull_Rejected(t *testing.T) {
	t.Parallel()

	nl := networkListVal([]map[string]tftypes.Value{
		{"ttls": boolVal(true)},
	})
	resp := runValidateConfigWithNetworkList(t, sdk.PlatformAppleOsX, nl)

	wantPath := "network_list[0].inner_identity"
	errs := diagErrorsAtPath(resp, wantPath)
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error at %s, got %d: %s", wantPath, len(errs), diagSummaries(resp))
	}
	if errs[0] != ttlsRequiresInnerIdentitySummary {
		t.Errorf("expected summary %q, got %q", ttlsRequiresInnerIdentitySummary, errs[0])
	}
}

func TestValidateConfig_TTLS_InnerIdentitySet_Accepted(t *testing.T) {
	t.Parallel()

	for _, v := range allowedInnerIdentities {
		t.Run(v, func(t *testing.T) {
			t.Parallel()
			nl := networkListVal([]map[string]tftypes.Value{
				{
					"ttls":           boolVal(true),
					"inner_identity": stringVal(v),
				},
			})
			resp := runValidateConfigWithNetworkList(t, sdk.PlatformAppleOsX, nl)

			if errs := diagErrorsAtPath(resp, "network_list[0].inner_identity"); len(errs) != 0 {
				t.Fatalf("expected no error with ttls=true and inner_identity = %q, got: %v", v, errs)
			}
		})
	}
}

func TestValidateConfig_TTLS_FalseOrNull_InnerIdentityNull_NoError(t *testing.T) {
	t.Parallel()

	for name, ttls := range map[string]tftypes.Value{
		"false": boolVal(false),
		"null":  nullBool(),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			nl := networkListVal([]map[string]tftypes.Value{
				{"ttls": ttls},
			})
			resp := runValidateConfigWithNetworkList(t, sdk.PlatformAppleOsX, nl)

			if errs := diagErrorsAtPath(resp, "network_list[0].inner_identity"); len(errs) != 0 {
				t.Fatalf("expected no error with ttls=%s and inner_identity null, got: %v", name, errs)
			}
		})
	}
}

// TestValidateConfig_TTLS_Unknown_NoError proves an unknown ttls (not yet
// decidable) never fires the requires-inner_identity rule, even with
// inner_identity null.
func TestValidateConfig_TTLS_Unknown_NoError(t *testing.T) {
	t.Parallel()

	nl := networkListVal([]map[string]tftypes.Value{
		{"ttls": tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue)},
	})
	resp := runValidateConfigWithNetworkList(t, sdk.PlatformAppleOsX, nl)

	if errs := diagErrorsAtPath(resp, "network_list[0].inner_identity"); len(errs) != 0 {
		t.Fatalf("expected no error with ttls unknown, got: %v", errs)
	}
}

// TestValidateConfig_TTLS_InnerIdentityUnknown_NoError proves an unknown
// inner_identity (not yet decidable) is left alone even with ttls = true.
func TestValidateConfig_TTLS_InnerIdentityUnknown_NoError(t *testing.T) {
	t.Parallel()

	nl := networkListVal([]map[string]tftypes.Value{
		{
			"ttls":           boolVal(true),
			"inner_identity": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		},
	})
	resp := runValidateConfigWithNetworkList(t, sdk.PlatformAppleOsX, nl)

	if errs := diagErrorsAtPath(resp, "network_list[0].inner_identity"); len(errs) != 0 {
		t.Fatalf("expected no error with ttls=true and inner_identity unknown, got: %v", errs)
	}
}

// TestValidateConfig_TTLS_OtherPlatform_NoError proves the rule is
// macOS-only: buildAppleOsXNetworkListEntity is the only builder that ever
// sends this list, so an equivalent config on another platform is never
// flagged.
func TestValidateConfig_TTLS_OtherPlatform_NoError(t *testing.T) {
	t.Parallel()

	nl := networkListVal([]map[string]tftypes.Value{
		{"ttls": boolVal(true)},
	})
	resp := runValidateConfigWithNetworkList(t, sdk.PlatformAppleiOS, nl)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error on %q, got: %s", sdk.PlatformAppleiOS, diagSummaries(resp))
	}
}
