package macscript

import (
	"context"
	"fmt"

	"github.com/euc-oss/terraform-provider-uem/internal/common/notfound"
	"github.com/euc-oss/terraform-provider-uem/internal/providerdata"
	tf "github.com/euc-oss/terraform-provider-uem/internal/scripts/mac/models"
	scriptstate "github.com/euc-oss/terraform-provider-uem/internal/scripts/mac/state"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

func (r *macscriptResource) Configure(
	ctx context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse) {
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

		if data.newScriptServiceV1API != nil {
			r.newScriptServiceV1API = data.newScriptServiceV1API
		} else {
			r.newScriptServiceV1API = defaulScriptServiceV1Factory
		}
	case *providerdata.ProviderData:
		if data == nil || data.Client == nil {
			resp.Diagnostics.AddError("Unexpected Resource Configure Type", "Provider resource data was nil or missing a configured SDK client.")
			return
		}
		r.client = data.Client
		r.newScriptServiceV1API = defaulScriptServiceV1Factory
	case *sdk.Client:
		// Backward-compatible path for direct unit tests that still pass *sdk.Client.
		r.client = data
		r.newScriptServiceV1API = defaulScriptServiceV1Factory
	default:
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			resourceTypeErrorDetail(req.ProviderData))
		return
	}

	tflog.Trace(ctx, "configure a macOS script resource")
}

func (r *macscriptResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse) {

	var data tf.MacScriptResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	orgGroupUUID, err := data.FetchValidOrganizationGroupUUID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid organization_group_uuid", err.Error())
		return
	}

	createReq, mapDiags := scriptstate.ToCreateScriptV1(&data)
	resp.Diagnostics.Append(mapDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	scriptUUID, err := r.createMacScript(ctx, orgGroupUUID, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create macOS script: %s", err))
		return
	}

	fetchResponse, err := r.fetchMacscriptDetails(ctx, scriptUUID)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read macOS script after create: %s", err))
		return
	}

	r.refreshIntoState(ctx, scriptUUID, fetchResponse, &data, &resp.State, &resp.Diagnostics)
	tflog.Trace(ctx, "created a macOS script resource", map[string]any{
		"script_uuid": scriptUUID,
	})
}

func (r *macscriptResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse) {

	var data tf.MacScriptResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	scriptUUID, err := data.FetchValidScriptUUID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid script id", err.Error())
		return
	}

	fetchResponse, err := r.fetchMacscriptDetails(ctx, scriptUUID)
	if err != nil {
		switch {
		case isNotFoundAPIError(err):
			confirmed, ok := r.confirmNotFoundRead(ctx, isNotFoundAPIError, scriptUUID, resp)
			if !ok {
				return
			}
			fetchResponse = confirmed
		case isAmbiguousGoneAPIError(err) && r.confirmScriptGone(ctx, &data, scriptUUID):
			// A deleted script's GET answers 500/1000, which is ambiguous; the
			// OG-scoped list has already confirmed it gone above. Confirm's
			// wait+re-GET goes here, at the point we're about to actually drop
			// state, and re-checks the SAME classification that got us here (a
			// 500/1000 the list also still confirms absent). A plain 404 on the
			// confirming re-GET does not classify here, so it surfaces as an
			// error and state is kept (fail safe).
			classify := func(e error) bool {
				return isAmbiguousGoneAPIError(e) && r.confirmScriptGone(ctx, &data, scriptUUID)
			}
			confirmed, ok := r.confirmNotFoundRead(ctx, classify, scriptUUID, resp)
			if !ok {
				return
			}
			fetchResponse = confirmed
		default:
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to fetch script details: %s", err))
			return
		}
	}

	r.refreshIntoState(ctx, scriptUUID, fetchResponse, &data, &resp.State, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "read a macOS script resource")
}

// confirmNotFoundRead runs notfound.Confirm for a Read not-found
// classification (see internal/common/notfound: this protects against a
// single flaky not-found response dropping real Terraform state). It
// returns (confirmed fetch result, true) when Read should continue exactly
// as it would the original success path; it returns (nil, false) once it
// has already fully handled the response itself (RemoveResource or a
// Diagnostics error), in which case the caller must return immediately.
func (r *macscriptResource) confirmNotFoundRead(
	ctx context.Context,
	classify func(error) bool,
	scriptUUID string,
	resp *resource.ReadResponse,
) (*sdk.ScriptResourceV1, bool) {
	confirmed, stillNotFound, confirmErr := notfound.Confirm(ctx, classify, func(ctx context.Context) (*sdk.ScriptResourceV1, error) {
		return r.fetchMacscriptDetails(ctx, scriptUUID)
	})
	if confirmErr != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to fetch script details: %s", confirmErr))
		return nil, false
	}
	if stillNotFound {
		resp.State.RemoveResource(ctx)
		return nil, false
	}
	tflog.Warn(ctx, fmt.Sprintf("UEM returned not-found then found for macOS script %s; kept in state", scriptUUID), map[string]any{
		"script_uuid": scriptUUID,
	})
	return confirmed, true
}

func (r *macscriptResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse) {

	var data tf.MacScriptResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	scriptUUID, err := data.FetchValidScriptUUID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid script id", err.Error())
		return
	}

	updateReq, mapDiags := scriptstate.ToUpdateScriptV1(&data, scriptUUID)
	resp.Diagnostics.Append(mapDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err = r.updateMacScript(ctx, scriptUUID, updateReq)
	if err != nil {
		if isNotFoundAPIError(err) {
			r.confirmNotFoundUpdate(ctx, scriptUUID, resp)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update macOS script: %s", err))
		return
	}

	fetchResponse, err := r.fetchMacscriptDetails(ctx, scriptUUID)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read macOS script after update: %s", err))
		return
	}

	r.refreshIntoState(ctx, scriptUUID, fetchResponse, &data, &resp.State, &resp.Diagnostics)
	tflog.Trace(ctx, "updated a macOS script resource", map[string]any{
		"script_uuid": scriptUUID,
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
// re-GET finds the script gone, this drops state exactly as it would
// without notfound.Confirm; if it finds the script present after all, this
// surfaces a clear "re-run apply" error and leaves state untouched (the
// script is confirmed to still exist, so removing it from state would be
// wrong).
//
// This uses the same plain isNotFoundAPIError classifier Update already
// used before this change, not Read's combined ambiguous-gone classifier:
// Update never treated the 500/1000 "ambiguous gone" shape as not-found, so
// this wiring does not change that.
func (r *macscriptResource) confirmNotFoundUpdate(
	ctx context.Context,
	scriptUUID string,
	resp *resource.UpdateResponse,
) {
	_, stillNotFound, confirmErr := notfound.Confirm(ctx, isNotFoundAPIError, func(ctx context.Context) (*sdk.ScriptResourceV1, error) {
		return r.fetchMacscriptDetails(ctx, scriptUUID)
	})
	if confirmErr != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update macOS script: %s", confirmErr))
		return
	}
	if stillNotFound {
		resp.State.RemoveResource(ctx)
		return
	}
	tflog.Warn(ctx, fmt.Sprintf("UEM returned not-found then found for macOS script %s during update; re-run apply", scriptUUID), map[string]any{
		"script_uuid": scriptUUID,
	})
	resp.Diagnostics.AddError("Client Error", "UEM returned not-found then found during update; re-run apply")
}

func (r *macscriptResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse) {
	var data tf.MacScriptResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	scriptUUID, err := data.FetchValidScriptUUID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid script id", err.Error())
		return
	}

	orgGroupUUID, err := data.FetchValidOrganizationGroupUUID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid organization_group_uuid", err.Error())
		return
	}

	err = r.deleteMacScript(ctx, orgGroupUUID, scriptUUID)
	if err != nil {
		if isNotFoundAPIError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		// Already gone: an ambiguous 500/1000 confirmed absent by the list.
		if isAmbiguousGoneAPIError(err) && r.confirmScriptGone(ctx, &data, scriptUUID) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete macOS script, got error: %s", err))
		return
	}

	tflog.Trace(ctx, "deleted a macOS script resource")
}

func (r *macscriptResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse) {
	scriptUUID, err := tf.ValidateScriptUUID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Script UUID", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), scriptUUID)...)
}
