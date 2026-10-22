package assignment

import (
	"context"
	"fmt"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/euc-oss/terraform-provider-uem/internal/common/notfound"
	"github.com/euc-oss/terraform-provider-uem/internal/providerdata"
	tf "github.com/euc-oss/terraform-provider-uem/internal/sensors/assignment/models"
	assignmentState "github.com/euc-oss/terraform-provider-uem/internal/sensors/assignment/state"
)

func (r *sensorAssignmentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	switch data := req.ProviderData.(type) {
	case *resourceConfigData:
		if data == nil || data.client == nil {
			resp.Diagnostics.AddError("Unexpected Resource Configure Type", "Provider resource data was nil or missing a configured SDK client.")
			return
		}
		r.client = data.client
		if data.newSensorAssignmentService != nil {
			r.newSensorAssignmentService = data.newSensorAssignmentService
		} else {
			r.newSensorAssignmentService = defaultSensorAssignmentServiceFactory
		}
	case *providerdata.ProviderData:
		if data == nil || data.Client == nil {
			resp.Diagnostics.AddError("Unexpected Resource Configure Type", "Provider resource data was nil or missing a configured SDK client.")
			return
		}
		r.client = data.Client
		r.newSensorAssignmentService = defaultSensorAssignmentServiceFactory
	case *sdk.Client:
		r.client = data
		r.newSensorAssignmentService = defaultSensorAssignmentServiceFactory
	default:
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", resourceTypeErrorDetail(req.ProviderData))
		return
	}

	tflog.Trace(ctx, "configure sensor assignment resource")
}

func (r *sensorAssignmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data tf.SensorAssignmentModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sensorUUID, err := data.FetchValidSensorUUID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid sensor_uuid", err.Error())
		return
	}

	svc, err := r.sensorAssignmentService(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to initialize sensor assignment service: %s", err))
		return
	}

	desired, diags := assignmentState.DesiredAssignments(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(reconcileAssignments(ctx, svc, sensorUUID, desired)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.refreshIntoState(ctx, svc, sensorUUID, &data, &resp.State, &resp.Diagnostics)
	tflog.Trace(ctx, "created sensor assignment resource")
}

func (r *sensorAssignmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data tf.SensorAssignmentModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sensorUUID, err := data.FetchValidSensorUUID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid sensor_uuid", err.Error())
		return
	}

	svc, err := r.sensorAssignmentService(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to initialize sensor assignment service: %s", err))
		return
	}

	_, result, err := svc.GetDeviceSensorAssignmentsAsync(ctx, sensorUUID)
	if err != nil {
		if !isNotFoundAPIError(err) {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read sensor assignments, got error: %s", err))
			return
		}
		confirmed, ok := r.confirmNotFoundRead(ctx, svc, sensorUUID, resp)
		if !ok {
			return
		}
		result = confirmed
	}
	if result == nil {
		tflog.Warn(ctx, "sensor assignment read returned no body; preserving minimal state", map[string]any{
			"sensor_uuid": sensorUUID,
		})
		assignmentState.SetMinimalState(&data, sensorUUID)
		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
		return
	}

	assignmentState.SetMinimalState(&data, sensorUUID)
	resp.Diagnostics.Append(assignmentState.ReadAPIIntoState(ctx, &data, *result)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	tflog.Trace(ctx, "read sensor assignment resource")
}

// confirmNotFoundRead runs notfound.Confirm for a Read not-found
// classification (see internal/common/notfound: this protects against a
// single flaky not-found response dropping real Terraform state). It
// returns (confirmed result, true) when Read should continue exactly as it
// would the original success path; it returns (nil, false) once it has
// already fully handled the response itself (RemoveResource or a
// Diagnostics error), in which case the caller must return immediately.
func (r *sensorAssignmentResource) confirmNotFoundRead(
	ctx context.Context,
	svc sensorAssignmentServiceAPI,
	sensorUUID string,
	resp *resource.ReadResponse,
) (*[]sdk.DeviceSensorAssignmentResponseV1ModelV2, bool) {
	confirmed, stillNotFound, confirmErr := notfound.Confirm(ctx, isNotFoundAPIError, func(ctx context.Context) (*[]sdk.DeviceSensorAssignmentResponseV1ModelV2, error) {
		_, result, err := svc.GetDeviceSensorAssignmentsAsync(ctx, sensorUUID)
		return result, err
	})
	if confirmErr != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read sensor assignments, got error: %s", confirmErr))
		return nil, false
	}
	if stillNotFound {
		resp.State.RemoveResource(ctx)
		return nil, false
	}
	tflog.Warn(ctx, fmt.Sprintf("UEM returned not-found then found for sensor assignment %s; kept in state", sensorUUID), map[string]any{
		"sensor_uuid": sensorUUID,
	})
	return confirmed, true
}

func (r *sensorAssignmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data tf.SensorAssignmentModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sensorUUID, err := data.FetchValidSensorUUID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid sensor_uuid", err.Error())
		return
	}

	svc, err := r.sensorAssignmentService(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to initialize sensor assignment service: %s", err))
		return
	}

	desired, diags := assignmentState.DesiredAssignments(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(reconcileAssignments(ctx, svc, sensorUUID, desired)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.refreshIntoState(ctx, svc, sensorUUID, &data, &resp.State, &resp.Diagnostics)
	tflog.Trace(ctx, "updated sensor assignment resource")
}

func (r *sensorAssignmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data tf.SensorAssignmentModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sensorUUID, err := data.FetchValidSensorUUID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid sensor_uuid", err.Error())
		return
	}

	svc, err := r.sensorAssignmentService(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to initialize sensor assignment service: %s", err))
		return
	}

	resp.Diagnostics.Append(deleteAllAssignments(ctx, svc, sensorUUID)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "deleted sensor assignment resource", map[string]any{
		"sensor_uuid": sensorUUID,
	})
}

func (r *sensorAssignmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	sensorUUID, err := tf.ValidateSensorUUID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Sensor UUID", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("sensor_uuid"), sensorUUID)...)
}

func (r *sensorAssignmentResource) refreshIntoState(
	ctx context.Context,
	svc sensorAssignmentServiceAPI,
	sensorUUID string,
	data *tf.SensorAssignmentModel,
	state *tfsdk.State,
	diags *diag.Diagnostics,
) {
	_, result, err := svc.GetDeviceSensorAssignmentsAsync(ctx, sensorUUID)
	if err != nil {
		tflog.Warn(ctx, "post-write refresh failed; state may be stale", map[string]any{
			"sensor_uuid": sensorUUID,
			"error":       err.Error(),
		})
		assignmentState.SetMinimalState(data, sensorUUID)
		diags.Append(state.Set(ctx, data)...)
		return
	}
	if result != nil {
		diags.Append(assignmentState.ReadAPIIntoState(ctx, data, *result)...)
		if diags.HasError() {
			return
		}
	}
	assignmentState.SetMinimalState(data, sensorUUID)
	diags.Append(state.Set(ctx, data)...)
}
