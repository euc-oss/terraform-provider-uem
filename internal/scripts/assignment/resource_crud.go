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
	tf "github.com/euc-oss/terraform-provider-uem/internal/scripts/assignment/models"
	assignmentState "github.com/euc-oss/terraform-provider-uem/internal/scripts/assignment/state"
)

func (r *scriptAssignmentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
		if data.newScriptAssignmentService != nil {
			r.newScriptAssignmentService = data.newScriptAssignmentService
		} else {
			r.newScriptAssignmentService = defaultScriptAssignmentServiceFactory
		}
		if data.newScriptService != nil {
			r.newScriptService = data.newScriptService
		} else {
			r.newScriptService = defaultScriptServiceFactory
		}
	case *providerdata.ProviderData:
		if data == nil || data.Client == nil {
			resp.Diagnostics.AddError("Unexpected Resource Configure Type", "Provider resource data was nil or missing a configured SDK client.")
			return
		}
		r.client = data.Client
		r.newScriptAssignmentService = defaultScriptAssignmentServiceFactory
		r.newScriptService = defaultScriptServiceFactory
	case *sdk.Client:
		r.client = data
		r.newScriptAssignmentService = defaultScriptAssignmentServiceFactory
		r.newScriptService = defaultScriptServiceFactory
	default:
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", resourceTypeErrorDetail(req.ProviderData))
		return
	}

	tflog.Trace(ctx, "configure script assignment resource")
}

func (r *scriptAssignmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data tf.ScriptAssignmentRuleModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	scriptUUID, err := data.FetchValidScriptUUID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid script_uuid", err.Error())
		return
	}

	svc, err := r.scriptAssignmentService(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to initialize script assignment service: %s", err))
		return
	}

	apiBody, diags := assignmentState.ScriptAssignmentRuleToAPI(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err = svc.BulkUpdateScriptAssignmentsAsync(ctx, scriptUUID, apiBody)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create script assignments, got error: %s", err))
		return
	}

	r.refreshIntoState(ctx, svc, scriptUUID, &data, &resp.State, &resp.Diagnostics)
	tflog.Trace(ctx, "created script assignment resource")
}

func (r *scriptAssignmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data tf.ScriptAssignmentRuleModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	scriptUUID, err := data.FetchValidScriptUUID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid script_uuid", err.Error())
		return
	}

	svc, err := r.scriptAssignmentService(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to initialize script assignment service: %s", err))
		return
	}

	_, result, err := svc.GetScriptAssignmentsAsync(ctx, scriptUUID)
	if err != nil {
		confirmed, ok := r.readWithNotFoundConfirm(ctx, svc, err, &data, scriptUUID, resp)
		if !ok {
			return
		}
		result = confirmed
	}
	if result == nil {
		tflog.Warn(ctx, "script assignment read returned no body; preserving minimal state", map[string]any{
			"script_uuid": scriptUUID,
		})
		assignmentState.SetMinimalState(&data, scriptUUID)
		r.refreshOrgGroupUUID(ctx, scriptUUID, &data)
		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
		return
	}

	assignmentState.SetMinimalState(&data, scriptUUID)
	resp.Diagnostics.Append(assignmentState.ReadAPIIntoState(ctx, &data, result)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.refreshOrgGroupUUID(ctx, scriptUUID, &data)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	tflog.Trace(ctx, "read script assignment resource")
}

// readWithNotFoundConfirm handles a GetScriptAssignmentsAsync error from
// Read. A not-found classification (plain 404, or an ambiguous 500/1000
// that resolveAmbiguousGone has already confirmed absent via the parent
// script's OG-scoped list) gets one confirming re-GET via notfound.Confirm
// before RemoveResource actually runs, so a single flaky not-found response
// doesn't drop real Terraform state (see internal/common/notfound and the
// 7fx incident it documents). This wraps the drop points AFTER
// resolveAmbiguousGone has already decided "gone" — it does not change
// resolveAmbiguousGone's own logic.
//
// It returns (confirmed result, true) when Read should continue exactly as
// it would the original success path; it returns (nil, false) once it has
// already fully handled the response itself (RemoveResource or a
// Diagnostics error), in which case the caller must return immediately.
func (r *scriptAssignmentResource) readWithNotFoundConfirm(
	ctx context.Context,
	svc scriptAssignmentServiceAPI,
	err error,
	data *tf.ScriptAssignmentRuleModel,
	scriptUUID string,
	resp *resource.ReadResponse,
) (*sdk.ScriptAssignmentsSearchResultV1, bool) {
	refetch := func(ctx context.Context) (*sdk.ScriptAssignmentsSearchResultV1, error) {
		_, result, err := svc.GetScriptAssignmentsAsync(ctx, scriptUUID)
		return result, err
	}
	logKept := func() {
		tflog.Warn(ctx, fmt.Sprintf("UEM returned not-found then found for script assignment %s; kept in state", scriptUUID), map[string]any{
			"script_uuid": scriptUUID,
		})
	}

	if isNotFoundAPIError(err) {
		confirmed, stillNotFound, confirmErr := notfound.Confirm(ctx, isNotFoundAPIError, refetch)
		if confirmErr != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read script assignments, got error: %s", confirmErr))
			return nil, false
		}
		if stillNotFound {
			resp.State.RemoveResource(ctx)
			return nil, false
		}
		logKept()
		return confirmed, true
	}

	detail := fmt.Sprintf("Unable to read script assignments, got error: %s", err)
	if isAmbiguousGoneAPIError(err) {
		gone, hint := r.resolveAmbiguousGone(ctx, data, scriptUUID)
		if gone {
			classify := func(e error) bool {
				if !isAmbiguousGoneAPIError(e) {
					return false
				}
				g, _ := r.resolveAmbiguousGone(ctx, data, scriptUUID)
				return g
			}
			confirmed, stillNotFound, confirmErr := notfound.Confirm(ctx, classify, refetch)
			if confirmErr != nil {
				resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read script assignments, got error: %s", confirmErr))
				return nil, false
			}
			if stillNotFound {
				tflog.Info(ctx, "parent script confirmed absent after 500/1000; removing script assignment from state", map[string]any{
					"script_uuid": scriptUUID,
				})
				resp.State.RemoveResource(ctx)
				return nil, false
			}
			logKept()
			return confirmed, true
		}
		detail = appendHint(detail, hint)
	}
	resp.Diagnostics.AddError("Client Error", detail)
	return nil, false
}

func (r *scriptAssignmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data tf.ScriptAssignmentRuleModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	scriptUUID, err := data.FetchValidScriptUUID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid script_uuid", err.Error())
		return
	}

	svc, err := r.scriptAssignmentService(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to initialize script assignment service: %s", err))
		return
	}

	apiBody, diags := assignmentState.ScriptAssignmentRuleToAPI(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err = svc.BulkUpdateScriptAssignmentsAsync(ctx, scriptUUID, apiBody)
	if err != nil {
		if isNotFoundAPIError(err) {
			r.confirmNotFoundUpdate(ctx, svc, scriptUUID, resp)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update script assignments, got error: %s", err))
		return
	}

	r.refreshIntoState(ctx, svc, scriptUUID, &data, &resp.State, &resp.Diagnostics)
	tflog.Trace(ctx, "updated script assignment resource")
}

// confirmNotFoundUpdate runs notfound.Confirm for an Update not-found
// classification (see internal/common/notfound: this protects against a
// single flaky not-found response dropping real Terraform state). Unlike
// Read's readWithNotFoundConfirm, Update never resumes inline: the write
// that just failed not-found may or may not have already applied on UEM's
// side, and there is no way to tell from here, so blindly re-issuing the
// same write is not something this package can call "clean" -- it risks
// double-applying the update against a live tenant. If the confirming
// re-GET finds the script's assignments gone, this drops state exactly as
// it would without notfound.Confirm; if it finds them present after all,
// this surfaces a clear "re-run apply" error and leaves state untouched
// (the assignments are confirmed to still exist, so removing state would be
// wrong).
//
// This uses the same plain isNotFoundAPIError classifier Update already
// used before this change, not Read's combined ambiguous-gone classifier:
// Update never treated the 500/1000 "ambiguous gone" shape as not-found,
// this wiring does not change that.
func (r *scriptAssignmentResource) confirmNotFoundUpdate(
	ctx context.Context,
	svc scriptAssignmentServiceAPI,
	scriptUUID string,
	resp *resource.UpdateResponse,
) {
	_, stillNotFound, confirmErr := notfound.Confirm(ctx, isNotFoundAPIError, func(ctx context.Context) (*sdk.ScriptAssignmentsSearchResultV1, error) {
		_, result, err := svc.GetScriptAssignmentsAsync(ctx, scriptUUID)
		return result, err
	})
	if confirmErr != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update script assignments, got error: %s", confirmErr))
		return
	}
	if stillNotFound {
		resp.State.RemoveResource(ctx)
		return
	}
	tflog.Warn(ctx, fmt.Sprintf("UEM returned not-found then found for script assignment %s during update; re-run apply", scriptUUID), map[string]any{
		"script_uuid": scriptUUID,
	})
	resp.Diagnostics.AddError("Client Error", "UEM returned not-found then found during update; re-run apply")
}

func (r *scriptAssignmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data tf.ScriptAssignmentRuleModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	scriptUUID, err := data.FetchValidScriptUUID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid script_uuid", err.Error())
		return
	}

	svc, err := r.scriptAssignmentService(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to initialize script assignment service: %s", err))
		return
	}

	apiBody := assignmentState.EmptyScriptAssignmentRuleToAPI()

	_, err = svc.BulkUpdateScriptAssignmentsAsync(ctx, scriptUUID, apiBody)
	if err != nil {
		if isNotFoundAPIError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		detail := fmt.Sprintf("Unable to delete script assignments, got error: %s", err)
		if isAmbiguousGoneAPIError(err) {
			gone, hint := r.resolveAmbiguousGone(ctx, &data, scriptUUID)
			if gone {
				tflog.Info(ctx, "parent script confirmed absent after 500/1000; treating script assignment as deleted", map[string]any{
					"script_uuid": scriptUUID,
				})
				resp.State.RemoveResource(ctx)
				return
			}
			detail = appendHint(detail, hint)
		}
		resp.Diagnostics.AddError("Client Error", detail)
		return
	}

	tflog.Trace(ctx, "deleted script assignment resource")
}

func (r *scriptAssignmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	scriptUUID, err := tf.ValidateScriptUUID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Script UUID", err.Error())
		return
	}

	// Only script_uuid is set here; the Read that follows import fills
	// organization_group_uuid through refreshOrgGroupUUID.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("script_uuid"), scriptUUID)...)
}

func (r *scriptAssignmentResource) refreshIntoState(
	ctx context.Context,
	svc scriptAssignmentServiceAPI,
	scriptUUID string,
	data *tf.ScriptAssignmentRuleModel,
	state *tfsdk.State,
	diags *diag.Diagnostics,
) {
	_, result, err := svc.GetScriptAssignmentsAsync(ctx, scriptUUID)
	if err != nil {
		tflog.Warn(ctx, "post-write refresh failed; state may be stale", map[string]any{
			"script_uuid": scriptUUID,
			"error":       err.Error(),
		})
		assignmentState.SetMinimalState(data, scriptUUID)
		r.refreshOrgGroupUUID(ctx, scriptUUID, data)
		diags.Append(state.Set(ctx, data)...)
		return
	}
	if result != nil {
		diags.Append(assignmentState.ReadAPIIntoState(ctx, data, result)...)
		if diags.HasError() {
			return
		}
	}
	assignmentState.SetMinimalState(data, scriptUUID)
	r.refreshOrgGroupUUID(ctx, scriptUUID, data)
	diags.Append(state.Set(ctx, data)...)
}
