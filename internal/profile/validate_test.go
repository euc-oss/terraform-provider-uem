package profile

import (
	"context"
	"strings"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// runValidateConfig builds a tfsdk.Config with the given platform,
// assignment_type, and profile_scope values (everything else defaults to
// null) and runs it through ProfileResource.ValidateConfig, mirroring what
// terraform-plugin-framework does at `terraform plan`/`validate` time,
// before any Create/Update/apply-time logic runs.
func runValidateConfig(t *testing.T, platform, assignmentType, profileScope string) *resource.ValidateConfigResponse {
	t.Helper()

	values := map[string]tftypes.Value{
		"id":       nullString(),
		"name":     stringVal("Test Profile"),
		"platform": stringVal(platform),
	}
	if assignmentType == "" {
		values["assignment_type"] = nullString()
	} else {
		values["assignment_type"] = stringVal(assignmentType)
	}
	if profileScope == "" {
		values["profile_scope"] = nullString()
	} else {
		values["profile_scope"] = stringVal(profileScope)
	}

	cfg := createResourceConfig(t, values)

	r := &ProfileResource{}
	req := resource.ValidateConfigRequest{Config: cfg}
	resp := &resource.ValidateConfigResponse{}
	r.ValidateConfig(context.Background(), req, resp)
	return resp
}

// minimalFileVaultBaselineFields returns disk_encryption.filevault2 field
// values that satisfy every "required whenever disk_encryption is present"
// rule this package implements (filevault_user,
// prompt_to_enable_filevault_at) at their least-entangling values, for
// tests that want to isolate one OTHER filevault2/airwatch validator
// without tripping these. The prompt_to_enable_filevault_at is set to 2
// (LogoutOnly) specifically because it's the one value that never also
// requires number_of_times_user_can_bypass. Callers overlay/override
// individual fields with mergeValues as needed.
func minimalFileVaultBaselineFields() map[string]tftypes.Value {
	return map[string]tftypes.Value{
		"filevault_user":                int64Val(1),
		"prompt_to_enable_filevault_at": int64Val(2),
	}
}

// minimalAirWatchBaselineFields returns disk_encryption.airwatch field
// values that satisfy the "use_intelligent_hub required whenever
// disk_encryption is present" rule at its least-entangling value (false:
// known/set, so the requiredness check passes, but off, so none of the
// notify_user_for_encryption/enable_recovery_key gating rules are engaged).
func minimalAirWatchBaselineFields() map[string]tftypes.Value {
	return map[string]tftypes.Value{
		"use_intelligent_hub": boolVal(false),
	}
}

// runValidateConfigValues builds a tfsdk.Config from values (any attribute
// left unset defaults to null, per createResourceConfig/fillMissingAttrs)
// and runs it through ProfileResource.ValidateConfig. It's the low-level
// helper behind runValidateConfig and runValidateConfigWithPasscode, kept
// separate so tests that need to set an attribute runValidateConfig doesn't
// expose (e.g. an unknown platform value) can still share the same
// config-building/invocation path.
func runValidateConfigValues(t *testing.T, values map[string]tftypes.Value) *resource.ValidateConfigResponse {
	t.Helper()

	cfg := createResourceConfig(t, values)

	r := &ProfileResource{}
	req := resource.ValidateConfigRequest{Config: cfg}
	resp := &resource.ValidateConfigResponse{}
	r.ValidateConfig(context.Background(), req, resp)
	return resp
}

// runValidateConfigWithPasscode builds a tfsdk.Config with the given
// platform and passcode value (everything else defaults to null) and runs
// it through ProfileResource.ValidateConfig. Used by
// validate_unknown_blocks_test.go and validate_passcode_shape_test.go.
func runValidateConfigWithPasscode(t *testing.T, platform string, passcode tftypes.Value) *resource.ValidateConfigResponse {
	t.Helper()

	return runValidateConfigValues(t, map[string]tftypes.Value{
		"id":       nullString(),
		"name":     stringVal("Test Profile"),
		"platform": stringVal(platform),
		"passcode": passcode,
	})
}

// runValidateConfigWithRestrictions builds a tfsdk.Config with the given
// platform and restrictions value (everything else defaults to null) and
// runs it through ProfileResource.ValidateConfig. Used by
// validate_restrictions_canonical_test.go.
func runValidateConfigWithRestrictions(t *testing.T, platform string, restrictions tftypes.Value) *resource.ValidateConfigResponse {
	t.Helper()

	return runValidateConfigValues(t, map[string]tftypes.Value{
		"id":           nullString(),
		"name":         stringVal("Test Profile"),
		"platform":     stringVal(platform),
		"restrictions": restrictions,
	})
}

func diagSummaries(resp *resource.ValidateConfigResponse) string {
	var out []string
	for _, d := range resp.Diagnostics.Errors() {
		out = append(out, d.Summary()+": "+d.Detail())
	}
	return strings.Join(out, " | ")
}

// diagErrorsAtPath returns the summaries of every error diagnostic in resp
// whose attribute path renders (via path.Path.String()) exactly as
// wantPath — e.g. "passcode" or "passcode.require_alphanumeric_value".
func diagErrorsAtPath(resp *resource.ValidateConfigResponse, wantPath string) []string {
	var out []string
	for _, d := range resp.Diagnostics.Errors() {
		wp, ok := d.(diag.DiagnosticWithPath)
		if !ok {
			continue
		}
		if wp.Path().String() == wantPath {
			out = append(out, d.Summary())
		}
	}
	return out
}

// --- assignment_type -------------------------------------------------------

func TestValidateConfig_AssignmentType_InvalidValue_Rejected(t *testing.T) {
	t.Parallel()

	resp := runValidateConfig(t, sdk.PlatformAppleOsX, "Bogus", "")

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a plan-time error for assignment_type \"Bogus\", got none")
	}
	msg := diagSummaries(resp)
	for _, want := range []string{"Auto", "Custom", "Optional", "Interactive", "Compliance"} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected error message to name allowed value %q, got: %s", want, msg)
		}
	}
}

func TestValidateConfig_AssignmentType_AllValidValues_AcceptedOnNonWinRTPlatform(t *testing.T) {
	t.Parallel()

	for _, v := range allowedAssignmentTypes {
		t.Run(v, func(t *testing.T) {
			t.Parallel()
			resp := runValidateConfig(t, sdk.PlatformAppleOsX, v, "")
			if resp.Diagnostics.HasError() {
				t.Fatalf("expected assignment_type %q to be accepted on %q, got errors: %s", v, sdk.PlatformAppleOsX, diagSummaries(resp))
			}
		})
	}
}

func TestValidateConfig_AssignmentType_CustomRejectedOnWindows10(t *testing.T) {
	t.Parallel()

	resp := runValidateConfig(t, sdk.PlatformWindows10, "Custom", "")

	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected assignment_type \"Custom\" to be rejected for platform %q, got no error", sdk.PlatformWindows10)
	}
	msg := diagSummaries(resp)
	if !strings.Contains(msg, "Windows 10") {
		t.Errorf("expected error message to mention the Windows 10 platform, got: %s", msg)
	}
}

func TestValidateConfig_AssignmentType_CustomAcceptedOnOtherPlatforms(t *testing.T) {
	t.Parallel()

	resp := runValidateConfig(t, sdk.PlatformAppleOsX, "Custom", "")

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected assignment_type \"Custom\" to be accepted on %q, got errors: %s", sdk.PlatformAppleOsX, diagSummaries(resp))
	}
}

func TestValidateConfig_AssignmentType_CaseInsensitive(t *testing.T) {
	t.Parallel()

	resp := runValidateConfig(t, sdk.PlatformAppleOsX, "auto", "")

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected lowercase assignment_type \"auto\" to be accepted, got errors: %s", diagSummaries(resp))
	}
}

// --- profile_scope -----------------------------------------------------------

func TestValidateConfig_ProfileScope_InvalidValue_Rejected(t *testing.T) {
	t.Parallel()

	// "Test" is the old, incorrect value fixed in internal-task bug 1 — the real
	// second canonical value is "Staging".
	resp := runValidateConfig(t, sdk.PlatformAppleOsX, "", "Test")

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a plan-time error for profile_scope \"Test\", got none")
	}
	msg := diagSummaries(resp)
	for _, want := range []string{"Production", "Staging", "Both"} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected error message to name allowed value %q, got: %s", want, msg)
		}
	}
}

func TestValidateConfig_ProfileScope_AllValidValues_Accepted(t *testing.T) {
	t.Parallel()

	for _, v := range allowedProfileScopes {
		t.Run(v, func(t *testing.T) {
			t.Parallel()
			resp := runValidateConfig(t, sdk.PlatformAppleOsX, "", v)
			if resp.Diagnostics.HasError() {
				t.Fatalf("expected profile_scope %q to be accepted, got errors: %s", v, diagSummaries(resp))
			}
		})
	}
}

// TestValidateConfig_ProfileScope_EmptyAccepted pins that an explicit ""
// validates: UEM stores an empty ProfileScope on real profiles, so an
// imported profile carries profile_scope = "" and must plan.
func TestValidateConfig_ProfileScope_EmptyAccepted(t *testing.T) {
	t.Parallel()

	cfg := createResourceConfig(t, map[string]tftypes.Value{
		"id":              nullString(),
		"name":            stringVal("Test Profile"),
		"platform":        stringVal(sdk.PlatformAppleOsX),
		"assignment_type": nullString(),
		"profile_scope":   stringVal(""),
	})
	r := &ProfileResource{}
	resp := &resource.ValidateConfigResponse{}
	r.ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: cfg}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected profile_scope \"\" to be accepted, got errors: %s", diagSummaries(resp))
	}
}

func TestValidateConfig_ProfileScope_CaseInsensitive(t *testing.T) {
	t.Parallel()

	resp := runValidateConfig(t, sdk.PlatformAppleOsX, "", "production")

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected lowercase profile_scope \"production\" to be accepted, got errors: %s", diagSummaries(resp))
	}
}

// --- null/unset values are no-ops -------------------------------------------

func TestValidateConfig_NullValues_NoError(t *testing.T) {
	t.Parallel()

	resp := runValidateConfig(t, sdk.PlatformAppleOsX, "", "")

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected null assignment_type/profile_scope to produce no error, got: %s", diagSummaries(resp))
	}
}
