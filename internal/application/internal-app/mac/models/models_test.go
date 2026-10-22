package models

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestValidateID(t *testing.T) {
	if _, err := ValidateID(0); err == nil {
		t.Fatal("expected error for non-positive id")
	}
	if got, err := ValidateID(7); err != nil || got != 7 {
		t.Fatalf("expected valid id 7, got %d, err=%v", got, err)
	}
}

func TestMacGetters(t *testing.T) {
	m := MacApplicationResourceModel{
		UUID:          types.StringValue("app-uuid"),
		OrgGroupID:    types.Int32Value(123),
		DMGFilePath:   types.StringValue("/tmp/app.dmg"),
		PlistFilePath: types.StringValue("/tmp/app.plist"),
		AppVersion:    NewAppVersionValue("1.0.0"),
	}

	if m.GetAppUUID() != "app-uuid" {
		t.Fatalf("unexpected app uuid: %q", m.GetAppUUID())
	}
	if m.GetOrgGroupID() != 123 {
		t.Fatalf("unexpected org group id: %d", m.GetOrgGroupID())
	}
	if m.GetDMGFilePath() != "/tmp/app.dmg" {
		t.Fatalf("unexpected dmg path: %q", m.GetDMGFilePath())
	}
	if m.GetPlistFilePath() != "/tmp/app.plist" {
		t.Fatalf("unexpected plist path: %q", m.GetPlistFilePath())
	}
	if m.GetAppVersion() != "1.0.0" {
		t.Fatalf("unexpected app version: %q", m.GetAppVersion())
	}
}

// TestValidateAppVersion_DoesNotRequireNonBlank covers B16 decision table
// row #99 (CORRECT): canonical Q9 confirms UEM's Version field is optional
// server-side (no FluentValidation rule requires it), so ValidateAppVersion
// must trim without rejecting a blank value.
func TestValidateAppVersion_DoesNotRequireNonBlank(t *testing.T) {
	if got := ValidateAppVersion(""); got != "" {
		t.Fatalf("expected blank app_version to pass through as \"\", got %q", got)
	}
	if got := ValidateAppVersion("   "); got != "" {
		t.Fatalf("expected whitespace-only app_version to trim to \"\", got %q", got)
	}
	if got := ValidateAppVersion("  1.0.0  "); got != "1.0.0" {
		t.Fatalf("expected app_version to be trimmed, got %q", got)
	}
}

// TestFetchOptionalAppVersion_NeverErrors covers the same row: omitting
// app_version entirely (null, as an unconfigured Optional+Computed
// attribute on Create) or leaving it unknown must not fail Create.
func TestFetchOptionalAppVersion_NeverErrors(t *testing.T) {
	if got := (MacApplicationResourceModel{AppVersion: NewAppVersionValue("")}).FetchOptionalAppVersion(); got != "" {
		t.Fatalf("expected \"\" for blank app_version, got %q", got)
	}
	if got := (MacApplicationResourceModel{AppVersion: NewAppVersionNull()}).FetchOptionalAppVersion(); got != "" {
		t.Fatalf("expected \"\" for null app_version, got %q", got)
	}
	if got := (MacApplicationResourceModel{AppVersion: AppVersionValue{StringValue: types.StringUnknown()}}).FetchOptionalAppVersion(); got != "" {
		t.Fatalf("expected \"\" for unknown app_version, got %q", got)
	}
	if got := (MacApplicationResourceModel{AppVersion: NewAppVersionValue("2.6.22")}).FetchOptionalAppVersion(); got != "2.6.22" {
		t.Fatalf("expected configured app_version to pass through, got %q", got)
	}
}

func TestFetchValidOrgGroupID(t *testing.T) {
	if _, err := (MacApplicationResourceModel{OrgGroupID: types.Int32Null()}).FetchValidOrgGroupID(); err == nil {
		t.Fatal("expected error for null org_group_id")
	}
	if _, err := (MacApplicationResourceModel{OrgGroupID: types.Int32Unknown()}).FetchValidOrgGroupID(); err == nil {
		t.Fatal("expected error for unknown org_group_id")
	}
	if _, err := (MacApplicationResourceModel{OrgGroupID: types.Int32Value(0)}).FetchValidOrgGroupID(); err == nil {
		t.Fatal("expected error for invalid org_group_id")
	}
	if got, err := (MacApplicationResourceModel{OrgGroupID: types.Int32Value(10)}).FetchValidOrgGroupID(); err != nil || got != 10 {
		t.Fatalf("expected org_group_id 10, got %d, err=%v", got, err)
	}
}
