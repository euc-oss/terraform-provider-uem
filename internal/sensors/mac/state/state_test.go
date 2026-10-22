package state

import (
	"context"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/types"

	tf "github.com/euc-oss/terraform-provider-uem/internal/sensors/mac/models"
)

func TestToCreateDeviceSensorV2_roundTripFields(t *testing.T) {
	model := &tf.MacSensorResourceModel{
		OrganizationGroupUUID: types.StringValue("90cd82d2-e5f5-ce88-e288-e50bdbf629b8"),
		Name:                  types.StringValue("disk_free"),
		Description:           types.StringValue("Reports free disk space"),
		Language:              types.StringValue("BASH"),
		ResponseDataType:      types.StringValue("STRING"),
		ExecutionContext:      types.StringValue("SYSTEM"),
		Code:                  types.StringValue("ZWNobyBoZWxsbw=="),
	}

	api, diags := ToCreateDeviceSensorV2(model)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if api.Name != "disk_free" || api.Platform != tf.MacSensorPlatform {
		t.Fatalf("unexpected create payload: %+v", api)
	}
	if api.QueryType != "BASH" || api.QueryResponseType != "STRING" {
		t.Fatalf("unexpected language/response type: %+v", api)
	}
	if api.OrganizationGroupUUID != "90cd82d2-e5f5-ce88-e288-e50bdbf629b8" {
		t.Fatalf("organization group mismatch: %q", api.OrganizationGroupUUID)
	}
}

func TestToCreateDeviceSensorV2_setsExecutionArchitecture(t *testing.T) {
	model := &tf.MacSensorResourceModel{
		OrganizationGroupUUID: types.StringValue("90cd82d2-e5f5-ce88-e288-e50bdbf629b8"),
		Name:                  types.StringValue("disk_free"),
		Language:              types.StringValue("BASH"),
		ResponseDataType:      types.StringValue("STRING"),
		ExecutionContext:      types.StringValue("SYSTEM"),
		ExecutionArchitecture: types.StringValue(tf.ExecutionArchitectureEitherOr),
		Code:                  types.StringValue("ZWNobyBoZWxsbw=="),
	}

	api, diags := ToCreateDeviceSensorV2(model)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if api.ExecutionArchitecture != tf.ExecutionArchitectureEitherOr {
		t.Fatalf("ExecutionArchitecture = %q, want %q", api.ExecutionArchitecture, tf.ExecutionArchitectureEitherOr)
	}
}

func TestToUpdateDeviceSensorV2_setsExecutionArchitecture(t *testing.T) {
	model := &tf.MacSensorResourceModel{
		Language:              types.StringValue("BASH"),
		ExecutionContext:      types.StringValue("SYSTEM"),
		ExecutionArchitecture: types.StringValue(tf.ExecutionArchitectureEitherOr),
		Code:                  types.StringValue("ZWNobyBoZWxsbw=="),
	}

	api, diags := ToUpdateDeviceSensorV2(model, "sensor-uuid")
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if api.ExecutionArchitecture != tf.ExecutionArchitectureEitherOr {
		t.Fatalf("ExecutionArchitecture = %q, want %q", api.ExecutionArchitecture, tf.ExecutionArchitectureEitherOr)
	}
}

func TestToCreateDeviceSensorV2_rejectsInvalidName(t *testing.T) {
	model := &tf.MacSensorResourceModel{
		OrganizationGroupUUID: types.StringValue("90cd82d2-e5f5-ce88-e288-e50bdbf629b8"),
		Name:                  types.StringValue("Invalid Name"),
		Language:              types.StringValue("BASH"),
		ResponseDataType:      types.StringValue("STRING"),
		ExecutionContext:      types.StringValue("SYSTEM"),
		Code:                  types.StringValue("ZWNobyBoZWxsbw=="),
	}

	_, diags := ToCreateDeviceSensorV2(model)
	if !diags.HasError() {
		t.Fatal("expected diagnostics error for invalid sensor name")
	}
}

func TestReadAPIIntoState_mapsComputedFields(t *testing.T) {
	readOnly := false
	api := &sdk.DeviceSensorResponseV2Model{
		UUID:                  "sensor-uuid",
		Name:                  "disk_free",
		Description:           "Reports free disk space",
		OrganizationGroupUUID: "org-uuid",
		Platform:              tf.MacSensorPlatform,
		QueryType:             "BASH",
		QueryResponseType:     "STRING",
		ExecutionContext:      "SYSTEM",
		ScriptData:            "ZWNobyBoZWxsbw==",
		IsReadOnly:            &readOnly,
	}

	data := &tf.MacSensorResourceModel{}
	diags := ReadAPIIntoState(context.Background(), data, api, "sensor-uuid")
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if data.ID.ValueString() != "sensor-uuid" {
		t.Fatalf("id mismatch: %q", data.ID.ValueString())
	}
	if data.Language.ValueString() != "BASH" {
		t.Fatalf("language mismatch: %q", data.Language.ValueString())
	}
}

// ReadAPIIntoState must populate execution_architecture from the API
// response so an existing/imported resource refreshes to the real value
// (rather than leaving it null and relying solely on the schema Default,
// which only fires when config is unset).
func TestReadAPIIntoState_executionArchitectureFromAPI(t *testing.T) {
	api := &sdk.DeviceSensorResponseV2Model{
		UUID:                  "sensor-uuid",
		Name:                  "disk_free",
		OrganizationGroupUUID: "org-uuid",
		Platform:              tf.MacSensorPlatform,
		QueryType:             "BASH",
		ExecutionContext:      "SYSTEM",
		ExecutionArchitecture: tf.ExecutionArchitectureEitherOr,
		ScriptData:            "ZWNobyBoZWxsbw==",
	}

	data := &tf.MacSensorResourceModel{}
	diags := ReadAPIIntoState(context.Background(), data, api, "sensor-uuid")
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got := data.ExecutionArchitecture.ValueString(); got != tf.ExecutionArchitectureEitherOr {
		t.Fatalf("ExecutionArchitecture = %q, want %q", got, tf.ExecutionArchitectureEitherOr)
	}
}

// ReadAPIIntoState must never leave execution_architecture as an empty
// string in state: unlike organization_group_uuid (below), an empty API
// value here must resolve to the known default constant, not to the raw
// empty value, or every subsequent plan would show a perpetual diff against
// the schema's static default.
func TestReadAPIIntoState_executionArchitectureEmptyFallsBackToDefault(t *testing.T) {
	api := &sdk.DeviceSensorResponseV2Model{
		UUID:                  "sensor-uuid",
		Name:                  "disk_free",
		OrganizationGroupUUID: "org-uuid",
		Platform:              tf.MacSensorPlatform,
		QueryType:             "BASH",
		ExecutionContext:      "SYSTEM",
		ExecutionArchitecture: "",
		ScriptData:            "ZWNobyBoZWxsbw==",
	}

	data := &tf.MacSensorResourceModel{}
	diags := ReadAPIIntoState(context.Background(), data, api, "sensor-uuid")
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got := data.ExecutionArchitecture.ValueString(); got != tf.ExecutionArchitectureEitherOr {
		t.Fatalf("ExecutionArchitecture = %q, want default %q", got, tf.ExecutionArchitectureEitherOr)
	}
}

// organization_group_uuid comes from the API on every Read. An import-time
// Read (null prior state) must populate it, and a Read must never keep a
// stale prior value the API no longer reports.
func TestReadAPIIntoState_organizationGroupUUIDFromAPI(t *testing.T) {
	const ogUUID = "e91c83f7-97b0-825a-dea5-b30cfda2ce05"

	var imported tf.MacSensorResourceModel
	api := &sdk.DeviceSensorResponseV2Model{UUID: "bafde89c-041e-1756-082b-933aaf16cad8", OrganizationGroupUUID: ogUUID}
	if diags := ReadAPIIntoState(context.Background(), &imported, api, api.UUID); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got := imported.OrganizationGroupUUID.ValueString(); got != ogUUID {
		t.Errorf("import Read: OrganizationGroupUUID = %q, want %q", got, ogUUID)
	}

	stale := tf.MacSensorResourceModel{OrganizationGroupUUID: types.StringValue("stale-og-uuid")}
	api.OrganizationGroupUUID = ""
	if diags := ReadAPIIntoState(context.Background(), &stale, api, api.UUID); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got := stale.OrganizationGroupUUID.ValueString(); got != "" {
		t.Errorf("Read kept stale prior OrganizationGroupUUID %q; want the API value %q", got, "")
	}
}

// am3 (B16 (b)-row #179 removal): description is Optional-only, but Read no
// longer disambiguates "" using the prior value -- that rule was transplanted
// from uem_profile's setDescriptionFromAPI with no evidence for this
// endpoint. Read now stores whatever the API returned, faithfully, regardless
// of what the prior value was. EXPECTED DIFF RISK: a sensor created/updated
// with description unset reads back as "" (UEM's wire model cannot represent
// "no description" any other way), not null, which can perpetually diff
// against an omitted description in config.
func TestReadAPIIntoState_descriptionPassthrough(t *testing.T) {
	for _, tc := range []struct {
		name  string
		prior types.String
		api   string
		want  types.String
	}{
		{"null prior, empty API -> empty (pass-through)", types.StringNull(), "", types.StringValue("")},
		{"empty prior, empty API -> empty", types.StringValue(""), "", types.StringValue("")},
		{"null prior, non-empty API -> API value", types.StringNull(), "x", types.StringValue("x")},
		{"non-empty prior, empty API -> empty (pass-through, prior ignored)", types.StringValue("old"), "", types.StringValue("")},
		{"empty prior, non-empty API -> API value", types.StringValue(""), "x", types.StringValue("x")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := tf.MacSensorResourceModel{Description: tc.prior}
			api := &sdk.DeviceSensorResponseV2Model{UUID: "bafde89c-041e-1756-082b-933aaf16cad8", Description: tc.api}
			if diags := ReadAPIIntoState(context.Background(), &data, api, api.UUID); diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if !data.Description.Equal(tc.want) {
				t.Errorf("Description = %v, want %v", data.Description, tc.want)
			}
		})
	}
}
