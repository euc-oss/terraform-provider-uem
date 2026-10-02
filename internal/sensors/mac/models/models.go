package models

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// MacSensorPlatform is hard-coded rather than user-configurable: the SDK's
// DeviceSensor Platform enum is exactly WIN_RT and APPLE_OSX, and this
// resource is macOS-only. Live-confirmed 2026-09-23 on as<internal-env> 26.2: the data
// source showed macOS sensors as APPLE_OSX and a Windows sensor as WIN_RT.
// Evidence: internal-design-doc
const MacSensorPlatform = "APPLE_OSX"

// ExecutionArchitectureEitherOr is the only execution_architecture value
// live-confirmed valid for APPLE_OSX device sensors. "64BIT" and "32BIT" are
// accepted by the SDK's type definition but are rejected by the UEM API with
// an HTTP 400 for this platform. It is the single source of truth for both
// the schema Default and the OneOf validator.
const ExecutionArchitectureEitherOr = "EITHER64OR32BIT"

// SensorNamePattern is the single source of truth for valid mac sensor
// names. It is used both here (apply-time validation in the Create builder)
// and by the resource schema (plan-time validation via stringvalidator).
//
// B16 decision table row #177 (KEEP+cite). UEM source:
// AirWatch API/AW.Mdm.Api/AW.Mdm.Api.Model.Validators/DeviceSensors/V2/DeviceSensorRequestV2ModelValidator.cs:23-30
// (canonical Q25): FluentValidation requires Length(0, 64).Matches("^[a-z][a-z0-9_]+$").
// Combined, the effective bound is length 2-64 (Matches' `+` requires at
// least one character after the first, and Length caps the total at 64),
// first character `[a-z]`, remaining characters `[a-z0-9_]` repeated. This
// pattern is functionally identical to that combined bound.
var SensorNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,63}$`)

func validateRequiredString(fieldName, value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("%s must not be empty", fieldName)
	}
	return trimmed, nil
}

// MacSensorResourceModel represents a macOS device sensor in Terraform state.
type MacSensorResourceModel struct {
	ID                    types.String `tfsdk:"id"`
	OrganizationGroupUUID types.String `tfsdk:"organization_group_uuid"`
	Name                  types.String `tfsdk:"name"`
	Description           types.String `tfsdk:"description"`
	Language              types.String `tfsdk:"language"`
	ExecutionContext      types.String `tfsdk:"execution_context"`
	ExecutionArchitecture types.String `tfsdk:"execution_architecture"`
	ResponseDataType      types.String `tfsdk:"response_data_type"`
	Code                  types.String `tfsdk:"code"`
	IsReadOnly            types.Bool   `tfsdk:"is_read_only"`
}

func (m MacSensorResourceModel) FetchValidOrganizationGroupUUID() (string, error) {
	if m.OrganizationGroupUUID.IsNull() || m.OrganizationGroupUUID.IsUnknown() {
		return "", fmt.Errorf("organization_group_uuid must be set and known")
	}
	return validateRequiredString("organization_group_uuid", m.OrganizationGroupUUID.ValueString())
}

func ValidateSensorName(name string) (string, error) {
	value, err := validateRequiredString("name", name)
	if err != nil {
		return "", err
	}
	if !SensorNamePattern.MatchString(value) {
		return "", fmt.Errorf("name must start with a lowercase letter, use only lowercase letters, digits, and underscores, and be 2-64 characters")
	}
	return value, nil
}

func ValidateSensorUUID(sensorUUID string) (string, error) {
	return validateRequiredString("id", sensorUUID)
}

func (m MacSensorResourceModel) FetchValidSensorUUID() (string, error) {
	if m.ID.IsNull() || m.ID.IsUnknown() {
		return "", fmt.Errorf("id must be set and known")
	}
	return ValidateSensorUUID(m.ID.ValueString())
}
