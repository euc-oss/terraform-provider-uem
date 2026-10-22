package deployment

import (
	"context"
	"fmt"

	"github.com/euc-oss/terraform-provider-uem/internal/common/notfound"
	"github.com/euc-oss/terraform-provider-uem/internal/providerdata"
	tf "github.com/euc-oss/terraform-provider-uem/internal/updates/deployment/models"
	deploymentstate "github.com/euc-oss/terraform-provider-uem/internal/updates/deployment/state"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

func (r *updateDeploymentResource) Configure(
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
		if data.newUpdatesV1 != nil {
			r.newUpdatesV1 = data.newUpdatesV1
		} else {
			r.newUpdatesV1 = defaultUpdatesV1Factory
		}
	case *providerdata.ProviderData:
		if data == nil || data.Client == nil {
			resp.Diagnostics.AddError("Unexpected Resource Configure Type", "Provider resource data was nil or missing a configured SDK client.")
			return
		}
		r.client = data.Client
		r.newUpdatesV1 = defaultUpdatesV1Factory
	case *sdk.Client:
		r.client = data
		r.newUpdatesV1 = defaultUpdatesV1Factory
	default:
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			resourceTypeErrorDetail(req.ProviderData))
		return
	}

	tflog.Trace(ctx, "configure an update deployment resource")
}

func (r *updateDeploymentResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var data tf.UpdateDeploymentModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateUUID, err := data.FetchValidUpdateUUID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid update_uuid", err.Error())
		return
	}
	orgGroupUUID, err := data.FetchValidOrganizationGroupUUID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid organization_group_uuid", err.Error())
		return
	}

	createReq, mapDiags := deploymentstate.ToCreateAPI(ctx, &data)
	resp.Diagnostics.Append(mapDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	deploymentUUID, err := r.createDeployment(ctx, updateUUID, orgGroupUUID, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create update deployment: %s", err))
		return
	}

	fetchResponse, err := r.fetchDeploymentDetails(ctx, deploymentUUID)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read update deployment after create: %s", err))
		return
	}

	r.refreshIntoState(ctx, deploymentUUID, fetchResponse, &data, &resp.State, &resp.Diagnostics)
	tflog.Trace(ctx, "created an update deployment resource", map[string]any{
		"deployment_uuid": deploymentUUID,
	})
}

func (r *updateDeploymentResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var data tf.UpdateDeploymentModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deploymentUUID, err := data.FetchValidID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid id", err.Error())
		return
	}

	fetchResponse, err := r.fetchDeploymentDetails(ctx, deploymentUUID)
	if err != nil {
		if !isNotFoundAPIError(err) {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to fetch update deployment details: %s", err))
			return
		}
		confirmed, ok := r.confirmNotFoundRead(ctx, deploymentUUID, resp)
		if !ok {
			return
		}
		fetchResponse = confirmed
	}

	r.refreshIntoState(ctx, deploymentUUID, fetchResponse, &data, &resp.State, &resp.Diagnostics)
	tflog.Trace(ctx, "read an update deployment resource")
}

// confirmNotFoundRead runs notfound.Confirm for a Read not-found
// classification (see internal/common/notfound: this protects against a
// single flaky not-found response dropping real Terraform state). It
// returns (confirmed fetch result, true) when Read should continue exactly
// as it would the original success path; it returns (nil, false) once it
// has already fully handled the response itself (RemoveResource or a
// Diagnostics error), in which case the caller must return immediately.
func (r *updateDeploymentResource) confirmNotFoundRead(
	ctx context.Context,
	deploymentUUID string,
	resp *resource.ReadResponse,
) (*sdk.DeviceUpdateDeploymentV1Model, bool) {
	confirmed, stillNotFound, confirmErr := notfound.Confirm(ctx, isNotFoundAPIError, func(ctx context.Context) (*sdk.DeviceUpdateDeploymentV1Model, error) {
		return r.fetchDeploymentDetails(ctx, deploymentUUID)
	})
	if confirmErr != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to fetch update deployment details: %s", confirmErr))
		return nil, false
	}
	if stillNotFound {
		resp.State.RemoveResource(ctx)
		return nil, false
	}
	tflog.Warn(ctx, fmt.Sprintf("UEM returned not-found then found for update deployment %s; kept in state", deploymentUUID), map[string]any{
		"deployment_uuid": deploymentUUID,
	})
	return confirmed, true
}

func (r *updateDeploymentResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var data tf.UpdateDeploymentModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deploymentUUID, err := data.FetchValidID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid id", err.Error())
		return
	}

	updateReq, mapDiags := deploymentstate.ToUpdateAPI(ctx, &data)
	resp.Diagnostics.Append(mapDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err = r.updateDeployment(ctx, deploymentUUID, updateReq)
	if err != nil {
		if isNotFoundAPIError(err) {
			r.confirmNotFoundUpdate(ctx, deploymentUUID, resp)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update update deployment: %s", err))
		return
	}

	fetchResponse, err := r.fetchDeploymentDetails(ctx, deploymentUUID)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read update deployment after update: %s", err))
		return
	}

	r.refreshIntoState(ctx, deploymentUUID, fetchResponse, &data, &resp.State, &resp.Diagnostics)
	tflog.Trace(ctx, "updated an update deployment resource", map[string]any{
		"deployment_uuid": deploymentUUID,
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
// re-GET finds the deployment gone, this drops state exactly as it would
// without notfound.Confirm; if it finds the deployment present after all,
// this surfaces a clear "re-run apply" error and leaves state untouched
// (the deployment is confirmed to still exist, so removing it from state
// would be wrong).
func (r *updateDeploymentResource) confirmNotFoundUpdate(
	ctx context.Context,
	deploymentUUID string,
	resp *resource.UpdateResponse,
) {
	_, stillNotFound, confirmErr := notfound.Confirm(ctx, isNotFoundAPIError, func(ctx context.Context) (*sdk.DeviceUpdateDeploymentV1Model, error) {
		return r.fetchDeploymentDetails(ctx, deploymentUUID)
	})
	if confirmErr != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update update deployment: %s", confirmErr))
		return
	}
	if stillNotFound {
		resp.State.RemoveResource(ctx)
		return
	}
	tflog.Warn(ctx, fmt.Sprintf("UEM returned not-found then found for update deployment %s during update; re-run apply", deploymentUUID), map[string]any{
		"deployment_uuid": deploymentUUID,
	})
	resp.Diagnostics.AddError("Client Error", "UEM returned not-found then found during update; re-run apply")
}

func (r *updateDeploymentResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var data tf.UpdateDeploymentModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deploymentUUID, err := data.FetchValidID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid id", err.Error())
		return
	}

	err = r.deleteDeployment(ctx, deploymentUUID)
	if err != nil {
		if isNotFoundAPIError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete update deployment: %s", err))
		return
	}

	tflog.Trace(ctx, "deleted an update deployment resource")
}

func (r *updateDeploymentResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	deploymentUUID, err := tf.ValidateDeploymentUUID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Deployment UUID", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), deploymentUUID)...)
}
