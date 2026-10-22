package macsensor

import (
	"context"
	"fmt"

	"github.com/euc-oss/terraform-provider-uem/internal/common/notfound"
	"github.com/euc-oss/terraform-provider-uem/internal/providerdata"
	tf "github.com/euc-oss/terraform-provider-uem/internal/sensors/mac/models"
	sensorstate "github.com/euc-oss/terraform-provider-uem/internal/sensors/mac/state"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

func (r *macsensorResource) Configure(
	ctx context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
) {
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
		if data.newDeviceSensorsV2API != nil {
			r.newDeviceSensorsV2API = data.newDeviceSensorsV2API
		} else {
			r.newDeviceSensorsV2API = defaultDeviceSensorsV2Factory
		}
	case *providerdata.ProviderData:
		if data == nil || data.Client == nil {
			resp.Diagnostics.AddError("Unexpected Resource Configure Type", "Provider resource data was nil or missing a configured SDK client.")
			return
		}
		r.client = data.Client
		r.newDeviceSensorsV2API = defaultDeviceSensorsV2Factory
	case *sdk.Client:
		r.client = data
		r.newDeviceSensorsV2API = defaultDeviceSensorsV2Factory
	default:
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			resourceTypeErrorDetail(req.ProviderData))
		return
	}

	tflog.Trace(ctx, "configure a macOS device sensor resource")
}

func (r *macsensorResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var data tf.MacSensorResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq, mapDiags := sensorstate.ToCreateDeviceSensorV2(&data)
	resp.Diagnostics.Append(mapDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	sensorUUID, err := r.createMacSensor(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create macOS device sensor: %s", err))
		return
	}

	fetchResponse, err := r.fetchMacSensorDetails(ctx, sensorUUID)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read macOS device sensor after create: %s", err))
		return
	}

	r.refreshIntoState(ctx, sensorUUID, fetchResponse, &data, &resp.State, &resp.Diagnostics)
	tflog.Trace(ctx, "created a macOS device sensor resource", map[string]any{
		"sensor_uuid": sensorUUID,
	})
}

func (r *macsensorResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var data tf.MacSensorResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sensorUUID, err := data.FetchValidSensorUUID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid sensor id", err.Error())
		return
	}

	fetchResponse, err := r.fetchMacSensorDetails(ctx, sensorUUID)
	if err != nil {
		if !isNotFoundAPIError(err) {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to fetch device sensor details: %s", err))
			return
		}
		confirmed, ok := r.confirmNotFoundRead(ctx, sensorUUID, resp)
		if !ok {
			return
		}
		fetchResponse = confirmed
	}

	r.refreshIntoState(ctx, sensorUUID, fetchResponse, &data, &resp.State, &resp.Diagnostics)
	tflog.Trace(ctx, "read a macOS device sensor resource")
}

// confirmNotFoundRead runs notfound.Confirm for a Read not-found
// classification (see internal/common/notfound: this protects against a
// single flaky not-found response dropping real Terraform state). It
// returns (confirmed fetch result, true) when Read should continue exactly
// as it would the original success path; it returns (nil, false) once it
// has already fully handled the response itself (RemoveResource or a
// Diagnostics error), in which case the caller must return immediately.
func (r *macsensorResource) confirmNotFoundRead(
	ctx context.Context,
	sensorUUID string,
	resp *resource.ReadResponse,
) (*sdk.DeviceSensorResponseV2Model, bool) {
	confirmed, stillNotFound, confirmErr := notfound.Confirm(ctx, isNotFoundAPIError, func(ctx context.Context) (*sdk.DeviceSensorResponseV2Model, error) {
		return r.fetchMacSensorDetails(ctx, sensorUUID)
	})
	if confirmErr != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to fetch device sensor details: %s", confirmErr))
		return nil, false
	}
	if stillNotFound {
		resp.State.RemoveResource(ctx)
		return nil, false
	}
	tflog.Warn(ctx, fmt.Sprintf("UEM returned not-found then found for macOS device sensor %s; kept in state", sensorUUID), map[string]any{
		"sensor_uuid": sensorUUID,
	})
	return confirmed, true
}

func (r *macsensorResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var data tf.MacSensorResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sensorUUID, err := data.FetchValidSensorUUID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid sensor id", err.Error())
		return
	}

	updateReq, mapDiags := sensorstate.ToUpdateDeviceSensorV2(&data, sensorUUID)
	resp.Diagnostics.Append(mapDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err = r.updateMacSensor(ctx, sensorUUID, updateReq)
	if err != nil {
		if isNotFoundAPIError(err) {
			r.confirmNotFoundUpdate(ctx, sensorUUID, resp)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update macOS device sensor: %s", err))
		return
	}

	fetchResponse, err := r.fetchMacSensorDetails(ctx, sensorUUID)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read macOS device sensor after update: %s", err))
		return
	}

	r.refreshIntoState(ctx, sensorUUID, fetchResponse, &data, &resp.State, &resp.Diagnostics)
	tflog.Trace(ctx, "updated a macOS device sensor resource", map[string]any{
		"sensor_uuid": sensorUUID,
	})
}

// confirmNotFoundUpdate runs notfound.Confirm for an Update not-found
// classification (see internal/common/notfound: this protects against a
// single flaky not-found response dropping real Terraform state). Unlike
// Read's confirmNotFoundRead, Update never resumes inline: the write that
// just failed not-found may or may not have already applied on UEM's side,
// and there is no way to tell from here, so blindly re-issuing the same
// write is not something this package can call "clean" -- it risks
// double-applying the update against a live tenant. If the confirming
// re-GET finds the sensor gone, this drops state exactly as it would
// without notfound.Confirm; if it finds the sensor present after all, this
// surfaces a clear "re-run apply" error and leaves state untouched (the
// sensor is confirmed to still exist, so removing it from state would be
// wrong).
func (r *macsensorResource) confirmNotFoundUpdate(
	ctx context.Context,
	sensorUUID string,
	resp *resource.UpdateResponse,
) {
	_, stillNotFound, confirmErr := notfound.Confirm(ctx, isNotFoundAPIError, func(ctx context.Context) (*sdk.DeviceSensorResponseV2Model, error) {
		return r.fetchMacSensorDetails(ctx, sensorUUID)
	})
	if confirmErr != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update macOS device sensor: %s", confirmErr))
		return
	}
	if stillNotFound {
		resp.State.RemoveResource(ctx)
		return
	}
	tflog.Warn(ctx, fmt.Sprintf("UEM returned not-found then found for macOS device sensor %s during update; re-run apply", sensorUUID), map[string]any{
		"sensor_uuid": sensorUUID,
	})
	resp.Diagnostics.AddError("Client Error", "UEM returned not-found then found during update; re-run apply")
}

func (r *macsensorResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var data tf.MacSensorResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sensorUUID, err := data.FetchValidSensorUUID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid sensor id", err.Error())
		return
	}

	orgGroupUUID, err := data.FetchValidOrganizationGroupUUID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid organization_group_uuid", err.Error())
		return
	}

	err = r.deleteMacSensor(ctx, orgGroupUUID, sensorUUID)
	if err != nil {
		if isNotFoundAPIError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete macOS device sensor: %s", err))
		return
	}

	tflog.Trace(ctx, "deleted a macOS device sensor resource")
}

func (r *macsensorResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	sensorUUID, err := tf.ValidateSensorUUID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Sensor UUID", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), sensorUUID)...)
}
