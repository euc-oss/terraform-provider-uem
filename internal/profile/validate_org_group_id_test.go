package profile

import (
	"context"
	"strings"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// runValidateConfigOrgGroupID builds a minimal tfsdk.Config for the given
// platform and org_group_id (empty string means null, mirroring an omitted
// attribute) and runs it through ProfileResource.ValidateConfig. internal-task:
// org_group_id must be numeric because UEM's General.ManagedLocationGroupID
// (canonical rules 2026-09-25-canonical-264-general-ogid Q1) is a
// non-nullable int sent unconditionally on every write, for every platform
// (Q2/Q3).
func runValidateConfigOrgGroupID(t *testing.T, platform, orgGroupID string) *resource.ValidateConfigResponse {
	t.Helper()

	values := map[string]tftypes.Value{
		"id":       nullString(),
		"name":     stringVal("Test Profile"),
		"platform": stringVal(platform),
	}
	if orgGroupID == "" {
		values["org_group_id"] = nullString()
	} else {
		values["org_group_id"] = stringVal(orgGroupID)
	}

	cfg := createResourceConfig(t, values)

	r := &ProfileResource{}
	req := resource.ValidateConfigRequest{Config: cfg}
	resp := &resource.ValidateConfigResponse{}
	r.ValidateConfig(context.Background(), req, resp)
	return resp
}

func TestValidateConfig_OrgGroupID_NonNumeric_Rejected(t *testing.T) {
	t.Parallel()

	resp := runValidateConfigOrgGroupID(t, sdk.PlatformAppleOsX, "not-a-number")

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a plan-time error for a non-numeric org_group_id, got none")
	}
	msg := diagSummaries(resp)
	if !strings.Contains(msg, "org_group_id") {
		t.Errorf("expected error message to mention org_group_id, got: %s", msg)
	}
	if !strings.Contains(msg, "ManagedLocationGroupID") {
		t.Errorf("expected error message to cite the canonical ManagedLocationGroupID rule, got: %s", msg)
	}
}

func TestValidateConfig_OrgGroupID_Numeric_Accepted(t *testing.T) {
	t.Parallel()

	for _, platform := range []string{sdk.PlatformAppleOsX, sdk.PlatformAndroid, sdk.PlatformAppleiOS, sdk.PlatformWindows10, sdk.PlatformWindowsRugged, sdk.PlatformLinux} {
		t.Run(platform, func(t *testing.T) {
			t.Parallel()
			resp := runValidateConfigOrgGroupID(t, platform, "138883")
			if resp.Diagnostics.HasError() {
				t.Fatalf("expected numeric org_group_id to be accepted on %q, got errors: %s", platform, diagSummaries(resp))
			}
		})
	}
}

func TestValidateConfig_OrgGroupID_Null_NoError(t *testing.T) {
	t.Parallel()

	// A null org_group_id can't reach ValidateConfig through normal
	// Terraform use (org_group_id is Required, so Terraform Core rejects a
	// missing/null value in config before the provider is ever invoked), but
	// ValidateConfig itself must not fire a false positive if it ever does
	// (e.g. a future schema change, or a test harness that omits it).
	resp := runValidateConfigOrgGroupID(t, sdk.PlatformAppleOsX, "")

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected null org_group_id to produce no error from this check, got: %s", diagSummaries(resp))
	}
}

func TestValidateConfig_OrgGroupID_Unknown_NoError(t *testing.T) {
	t.Parallel()

	values := map[string]tftypes.Value{
		"id":           nullString(),
		"name":         stringVal("Test Profile"),
		"platform":     stringVal(sdk.PlatformAppleOsX),
		"org_group_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	}
	cfg := createResourceConfig(t, values)

	r := &ProfileResource{}
	req := resource.ValidateConfigRequest{Config: cfg}
	resp := &resource.ValidateConfigResponse{}
	r.ValidateConfig(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected unknown org_group_id to produce no error at plan time, got: %s", diagSummaries(resp))
	}
}
