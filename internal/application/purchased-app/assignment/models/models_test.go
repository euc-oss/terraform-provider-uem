package models

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestValidateAppUUID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "valid uuid", input: "ABC-123", want: "abc-123"},
		{name: "trim whitespace", input: "  ABC-123  ", want: "abc-123"},
		{name: "empty", input: "", wantErr: true},
		{name: "whitespace only", input: "   ", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateAppUUID(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("unexpected result: got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeUUID(t *testing.T) {
	if got := NormalizeUUID("  ABC-DEF  "); got != "abc-def" {
		t.Fatalf("unexpected normalized uuid: %q", got)
	}
}

func TestFetchValidAppUUID(t *testing.T) {
	model := PurchasedAppAssignmentRuleModel{ApplicationUUID: types.StringValue("  APP-UUID  ")}
	got, err := model.FetchValidAppUUID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "app-uuid" {
		t.Fatalf("unexpected result: got %q", got)
	}
}

func TestValidateTargeting(t *testing.T) {
	ctx := context.Background()

	licenseUsage, luDiags := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: map[string]attr.Type{
		"smart_group_uuid": types.StringType,
		"allocated":        types.Int64Type,
		"redeemed":         types.Int64Type,
	}}, []VppLicenseUsageModel{{
		SmartGroupUUID: types.StringValue("sg-1"),
		Allocated:      types.Int64Value(2),
		Redeemed:       types.Int64Null(),
	}})
	if luDiags.HasError() {
		t.Fatalf("failed to build license usage: %v", luDiags)
	}

	withVPP := PurchasedAppAssignmentModel{
		Distribution: PurchasedAppAssignmentDistributionModel{
			VppAppDetails: VppAppDetailsModel{LicenseUsage: licenseUsage},
		},
	}
	if err := withVPP.ValidateTargeting(ctx); err != nil {
		t.Fatalf("expected VPP targeting to be valid: %v", err)
	}

	smartGroups, sgDiags := types.ListValueFrom(ctx, types.StringType, []string{"sg-1"})
	if sgDiags.HasError() {
		t.Fatalf("failed to build smart groups: %v", sgDiags)
	}
	withSG := PurchasedAppAssignmentModel{
		Distribution: PurchasedAppAssignmentDistributionModel{
			SmartGroups: smartGroups,
		},
	}
	err := withSG.ValidateTargeting(ctx)
	if err == nil || !strings.Contains(err.Error(), "use vpp_app_details.license_usage") {
		t.Fatalf("expected smart group targeting to be rejected with license_usage guidance, got %v", err)
	}

	withBoth := PurchasedAppAssignmentModel{
		Distribution: PurchasedAppAssignmentDistributionModel{
			SmartGroups:   smartGroups,
			VppAppDetails: VppAppDetailsModel{LicenseUsage: licenseUsage},
		},
	}
	err = withBoth.ValidateTargeting(ctx)
	if err == nil || !strings.Contains(err.Error(), "use vpp_app_details.license_usage") {
		t.Fatalf("expected smart_groups alongside license_usage to be rejected, got %v", err)
	}

	empty := PurchasedAppAssignmentModel{
		Distribution: PurchasedAppAssignmentDistributionModel{},
	}
	if err := empty.ValidateTargeting(ctx); err == nil {
		t.Fatal("expected targeting validation error for empty assignment")
	}
}
