package models

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

type MacApplicationResourceModel struct {
	ID            types.Int32  `tfsdk:"id"`
	UUID          types.String `tfsdk:"uuid"`
	OrgGroupID    types.Int32  `tfsdk:"org_group_id"`
	DMGFilePath   types.String `tfsdk:"dmg_file_path"`
	PlistFilePath types.String `tfsdk:"plist_file_path"`
	AppVersion    types.String `tfsdk:"app_version"`
}

func validateRequiredString(fieldName, value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("%s must not be empty", fieldName)
	}
	return trimmed, nil
}

func ValidateAppUUID(appUUID string) (string, error) {
	return validateRequiredString("uuid", appUUID)
}

func ValidateID(id int) (int, error) {
	if id <= 0 {
		return 0, fmt.Errorf("id must be a positive integer")
	}
	return id, nil
}

func ValidateDMGFilePath(dmgFilePath string) (string, error) {
	return validateRequiredString("dmg_file_path", dmgFilePath)
}

func ValidatePlistFilePath(plistFilePath string) (string, error) {
	return validateRequiredString("plist_file_path", plistFilePath)
}

func ValidateAppVersion(appVersion string) (string, error) {
	return validateRequiredString("app_version", appVersion)
}

func (m MacApplicationResourceModel) GetAppUUID() string {
	return m.UUID.ValueString()
}

func (m MacApplicationResourceModel) GetOrgGroupID() int {
	return int(m.OrgGroupID.ValueInt32())
}

func (m MacApplicationResourceModel) GetDMGFilePath() string {
	return m.DMGFilePath.ValueString()
}

func (m MacApplicationResourceModel) GetPlistFilePath() string {
	return m.PlistFilePath.ValueString()
}

func (m MacApplicationResourceModel) GetAppVersion() string {
	return m.AppVersion.ValueString()
}

func (m MacApplicationResourceModel) FetchValidAppUUID() (string, error) {
	if m.ID.IsNull() || m.ID.IsUnknown() {
		return "", fmt.Errorf("id must be set and known")
	}
	return m.GetAppUUID(), nil
}

func (m MacApplicationResourceModel) FetchValidAppID() (int, error) {
	if m.ID.IsNull() || m.ID.IsUnknown() {
		return 0, fmt.Errorf("id must be set and known")
	}
	return ValidateID(int(m.ID.ValueInt32()))
}

func (m MacApplicationResourceModel) FetchValidOrgGroupID() (int, error) {
	if m.OrgGroupID.IsNull() || m.OrgGroupID.IsUnknown() {
		return 0, fmt.Errorf("org_group_id must be set and known")
	}
	return ValidateID(m.GetOrgGroupID())
}

func (m MacApplicationResourceModel) FetchValidDMGFilePath() (string, error) {
	if m.DMGFilePath.IsNull() || m.DMGFilePath.IsUnknown() {
		return "", fmt.Errorf("dmg_file_path must be set and known")
	}
	return ValidateDMGFilePath(m.GetDMGFilePath())
}

func (m MacApplicationResourceModel) FetchValidPlistFilePath() (string, error) {
	if m.PlistFilePath.IsNull() || m.PlistFilePath.IsUnknown() {
		return "", fmt.Errorf("plist_file_path must be set and known")
	}
	return ValidatePlistFilePath(m.GetPlistFilePath())
}

func (m MacApplicationResourceModel) FetchValidAppVersion() (string, error) {
	if m.AppVersion.IsNull() || m.AppVersion.IsUnknown() {
		return "", fmt.Errorf("app_version must be set and known")
	}
	return ValidateAppVersion(m.GetAppVersion())
}
