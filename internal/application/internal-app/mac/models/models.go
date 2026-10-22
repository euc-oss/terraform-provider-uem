package models

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// uuidPattern matches the standard 8-4-4-4-12 hex UUID format. ValidateAppUUID
// enforces this because appUUID is used to build a filesystem path
// (filepath.Join(appBinaryStoragePath, appUUID)) during import — an
// unconstrained value (e.g. containing "..", "/", or "\") could otherwise
// write outside the configured storage directory.
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type MacApplicationResourceModel struct {
	ID                 types.Int32     `tfsdk:"id"`
	UUID               types.String    `tfsdk:"uuid"`
	OrgGroupID         types.Int32     `tfsdk:"org_group_id"`
	DMGFilePath        types.String    `tfsdk:"dmg_file_path"`
	PlistFilePath      types.String    `tfsdk:"plist_file_path"`
	AppVersion         AppVersionValue `tfsdk:"app_version"`
	IncludeContent     types.Bool      `tfsdk:"include_content"`
	DMGContentBase64   types.String    `tfsdk:"dmg_content_base64"`
	PlistContentBase64 types.String    `tfsdk:"plist_content_base64"`
	DMGFileSHA256      types.String    `tfsdk:"dmg_file_sha256"`
	PlistFileSHA256    types.String    `tfsdk:"plist_file_sha256"`
	IconFilePath       types.String    `tfsdk:"icon_file_path"`
	IconFileSHA256     types.String    `tfsdk:"icon_file_sha256"`

	// The read-only application record (F9), see record_state.go.
	ActualFileVersion                  types.String `tfsdk:"actual_file_version"`
	AppIDValue                         types.String `tfsdk:"app_id"`
	AppProvisioningProfileUUID         types.String `tfsdk:"app_provisioning_profile_uuid"`
	AppSizeInKB                        types.Int64  `tfsdk:"app_size_in_kb"`
	ApplicationFileHash                types.String `tfsdk:"application_file_hash"`
	ApplicationName                    types.String `tfsdk:"application_name"`
	ApplicationURL                     types.String `tfsdk:"application_url"`
	AssumeManagementOfUserInstalledApp types.String `tfsdk:"assume_management_of_user_installed_app"`
	BuildVersion                       types.String `tfsdk:"build_version"`
	CategoryList                       types.List   `tfsdk:"category_list"`
	ChangeLog                          types.String `tfsdk:"change_log"`
	Comments                           types.String `tfsdk:"comments"`
	DisplayName                        types.String `tfsdk:"display_name"`
	LargeIconBlobGUID                  types.String `tfsdk:"large_icon_blob_guid"`
	LaunchCommand                      types.String `tfsdk:"launch_command"`
	LaunchType                         types.String `tfsdk:"launch_type"`
	MacOsSoftwareDeploymentSummary     types.Object `tfsdk:"mac_os_software_deployment_summary"`
	ManagedBy                          types.String `tfsdk:"managed_by"`
	ManagedByUUID                      types.String `tfsdk:"managed_by_uuid"`
	MediumIconBlobGUID                 types.String `tfsdk:"medium_icon_blob_guid"`
	MinimumOperatingSystem             types.String `tfsdk:"minimum_operating_system"`
	Platform                           types.String `tfsdk:"platform"`
	Rating                             types.Int64  `tfsdk:"rating"`
	Sdk                                types.String `tfsdk:"sdk"`
	SdkProfileID                       types.Int64  `tfsdk:"sdk_profile_id"`
	SdkProfileUUID                     types.String `tfsdk:"sdk_profile_uuid"`
	SmallIconBlobGUID                  types.String `tfsdk:"small_icon_blob_guid"`
	Status                             types.String `tfsdk:"status"`
	SupportedModels                    types.List   `tfsdk:"supported_models"`
	SupportedModelsName                types.List   `tfsdk:"supported_models_name"`
}

func validateRequiredString(fieldName, value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("%s must not be empty", fieldName)
	}
	return trimmed, nil
}

func ValidateAppUUID(appUUID string) (string, error) {
	trimmed, err := validateRequiredString("uuid", appUUID)
	if err != nil {
		return "", err
	}
	if !uuidPattern.MatchString(trimmed) {
		return "", fmt.Errorf("uuid must be a valid UUID (format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx), got: %q", trimmed)
	}
	return trimmed, nil
}

// ValidateID requires a known, positive org_group_id/id. UEM source:
// AirWatch API/AW.Mam.Api/AW.Mam.Api/Controllers/macOS/Apps/V1/MacOsAppsV1Controller.cs:263-268
// (canonical Q8): the organization group is the route parameter and is
// validated via LocationGroupHelper.ValidateOrganizationGroup (known-OG
// check; failure is a 400), but the server has no explicit `id > 0` check
// on this route. The ">0" half of this function is therefore stricter than
// UEM itself — harmless defensive input hygiene, kept as-is — while the
// "must be known" half now matches a confirmed server-side check.
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

// ValidateAppVersion trims app_version for the create request. UEM source:
// AirWatch API/AW.Mam.Api/AW.Mam.Api/Controllers/macOS/Apps/V1/MacOsAppsV1Controller.cs:118-121
// (canonical Q9): version is optional on MacOsCreateApplicationRequestV1Model
// — no FluentValidation rule requires it, and create only validates the
// value when it is non-blank — and the server does not trim it itself.
// Trimming here is provider-side hygiene only; it is deliberately NOT a
// basis for rejecting a blank value (see FetchValidAppVersion).
func ValidateAppVersion(appVersion string) string {
	return strings.TrimSpace(appVersion)
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

// FetchOptionalAppVersion returns the trimmed app_version to send at create,
// or "" when the configuration omits it (null or unknown). It never errors:
// B16 decision table row #99 (CORRECT) — canonical Q9 confirms app_version
// is optional server-side, so a blank/omitted value must not fail Create.
func (m MacApplicationResourceModel) FetchOptionalAppVersion() string {
	if m.AppVersion.IsNull() || m.AppVersion.IsUnknown() {
		return ""
	}
	return ValidateAppVersion(m.GetAppVersion())
}
