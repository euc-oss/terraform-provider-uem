package state

import (
	"context"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	tf "github.com/euc-oss/terraform-provider-uem/internal/sensors/mac/models"
)

func ToCreateDeviceSensorV2(m *tf.MacSensorResourceModel) (*sdk.DeviceSensorRequestV2Model, diag.Diagnostics) {
	var diags diag.Diagnostics
	if m == nil {
		return nil, diags
	}

	orgGroupUUID, err := m.FetchValidOrganizationGroupUUID()
	if err != nil {
		diags.AddError("Invalid organization_group_uuid", err.Error())
		return nil, diags
	}

	name, err := tf.ValidateSensorName(m.Name.ValueString())
	if err != nil {
		diags.AddError("Invalid name", err.Error())
		return nil, diags
	}

	api := &sdk.DeviceSensorRequestV2Model{
		Name:                  name,
		OrganizationGroupUUID: orgGroupUUID,
		Platform:              tf.MacSensorPlatform,
		QueryType:             m.Language.ValueString(),
		QueryResponseType:     m.ResponseDataType.ValueString(),
		ExecutionContext:      m.ExecutionContext.ValueString(),
		ExecutionArchitecture: m.ExecutionArchitecture.ValueString(),
		ScriptData:            m.Code.ValueString(),
	}

	setStringIfKnown(&api.Description, m.Description)

	return api, diags
}

func ToUpdateDeviceSensorV2(m *tf.MacSensorResourceModel, sensorUUID string) (*sdk.DeviceSensorUpdateV2Model, diag.Diagnostics) {
	var diags diag.Diagnostics
	if m == nil {
		return nil, diags
	}

	api := &sdk.DeviceSensorUpdateV2Model{
		UUID:                  sensorUUID,
		Platform:              tf.MacSensorPlatform,
		QueryType:             m.Language.ValueString(),
		ExecutionContext:      m.ExecutionContext.ValueString(),
		ExecutionArchitecture: m.ExecutionArchitecture.ValueString(),
		ScriptData:            m.Code.ValueString(),
	}

	setStringIfKnown(&api.Description, m.Description)

	return api, diags
}

func ReadAPIIntoState(_ context.Context, data *tf.MacSensorResourceModel, api *sdk.DeviceSensorResponseV2Model, sensorUUID string) diag.Diagnostics {
	var diags diag.Diagnostics
	if data == nil || api == nil {
		return diags
	}

	if sensorUUID == "" {
		sensorUUID = api.UUID
	}
	data.ID = types.StringValue(sensorUUID)

	// The API is authoritative: live GET .../devicesensors/{uuid} always returns
	// organization_group_uuid (verified live against readonly_fixtures on as<internal-env>),
	// so an import-time Read fills this Required attribute and the post-import
	// plan is a no-op.
	data.OrganizationGroupUUID = types.StringValue(api.OrganizationGroupUUID)
	data.Name = types.StringValue(api.Name)
	// B16 (b)-row #179 removal: store the server value exactly as returned,
	// with no prior-based disambiguation (this was transplanted from
	// uem_profile's setDescriptionFromAPI rule per its own prior comment).
	// UEM's wire model represents "no description" as "" (a non-pointer
	// string), so a sensor created/updated with description unset reads back
	// as "" here, not null -- see the B16 report's EXPECTED DIFF RISK for
	// this row.
	data.Description = types.StringValue(api.Description)
	data.Language = types.StringValue(api.QueryType)
	data.ResponseDataType = types.StringValue(api.QueryResponseType)
	data.ExecutionContext = types.StringValue(api.ExecutionContext)
	// The API omits execution_architecture on some responses; never write an
	// empty string into state, or it would perpetually diff against the
	// schema's static default (which only applies to a null config value at
	// plan time, not to a Read-populated state). Live-confirmed 2026-09-23 on
	// as<internal-env> 26.2: "Read keeps the default if the API omits the value, to
	// avoid a perpetual diff." Evidence:
	// internal-design-doc
	executionArchitecture := api.ExecutionArchitecture
	if executionArchitecture == "" {
		executionArchitecture = tf.ExecutionArchitectureEitherOr
	}
	data.ExecutionArchitecture = types.StringValue(executionArchitecture)
	data.Code = types.StringValue(api.ScriptData)

	if api.IsReadOnly != nil {
		data.IsReadOnly = types.BoolValue(*api.IsReadOnly)
	} else {
		data.IsReadOnly = types.BoolNull()
	}

	return diags
}

func setStringIfKnown(dst *string, s types.String) {
	if s.IsNull() || s.IsUnknown() {
		return
	}
	*dst = s.ValueString()
}
