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
		AppVersion:    types.StringValue("1.0.0"),
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
