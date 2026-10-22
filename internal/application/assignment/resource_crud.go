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

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/assignment/models"
	assignmentState "github.com/euc-oss/terraform-provider-uem/internal/application/assignment/state"
	"github.com/euc-oss/terraform-provider-uem/internal/common/notfound"
	"github.com/euc-oss/terraform-provider-uem/internal/providerdata"
)

func (r *applicationAssignmentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
		if data.newAppAssignmentService != nil {
			r.newAppAssignmentService = data.newAppAssignmentService
		} else {
			r.newAppAssignmentService = defaultAppAssignmentServiceFactory
		}
	case *providerdata.ProviderData:
		if data == nil || data.Client == nil {
			resp.Diagnostics.AddError("Unexpected Resource Configure Type", "Provider resource data was nil or missing a configured SDK client.")
			return
		}
		r.client = data.Client
		r.newAppAssignmentService = defaultAppAssignmentServiceFactory
	case *sdk.Client:
		// Backward-compatible path for direct unit tests that still pass *sdk.Client.
		r.client = data
		r.newAppAssignmentService = defaultAppAssignmentServiceFactory
	default:
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", resourceTypeErrorDetail(req.ProviderData))
		return
	}

	tflog.Trace(ctx, "configure application assignment resource")
}

func (r *applicationAssignmentResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse) {
	var data tf.AppAssignmentRuleModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	applicationUUID, err := data.FetchValidAppUUID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid application_uuid", err.Error())
		return
	}

	svc, err := r.appAssignmentService(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to initialize application assignment service: %s", err))
		return
	}

	apiBody, diags := assignmentState.AppAssignmentRuleToAPI(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err = svc.UpdateAssignmentRuleAsync(ctx, applicationUUID, apiBody)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create assignment rule, got error: %s", err))
		return
	}

	r.refreshIntoState(ctx, svc, applicationUUID, &data, &resp.State, &resp.Diagnostics)

	tflog.Trace(ctx, "created application assignment resource")
}

func (r *applicationAssignmentResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse) {

	var data tf.AppAssignmentRuleModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	applicationUUID, err := data.FetchValidAppUUID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid application_uuid", err.Error())
		return
	}

	svc, err := r.appAssignmentService(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to initialize application assignment service: %s", err))
		return
	}

	_, result, err := svc.GetAssignmentRuleAsync(ctx, applicationUUID)
	if err != nil {
		if !isNotFoundAPIError(err) {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read assignment rule, got error: %s", err))
			return
		}
		confirmed, ok := r.confirmNotFoundRead(ctx, svc, applicationUUID, resp)
		if !ok {
			return
		}
		result = confirmed
	}
	if result == nil {
		tflog.Warn(ctx, "assignment read returned no body; preserving minimal state", map[string]any{
			"application_uuid": applicationUUID,
		})
		assignmentState.SetMinimalState(&data, applicationUUID)
		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
		return
	}

	tflog.Trace(ctx, "after api call", map[string]any{
		"application_uuid": applicationUUID,
	})

	assignmentState.SetMinimalState(&data, applicationUUID)
	resp.Diagnostics.Append(assignmentState.ReadAPIIntoState(ctx, &data, result)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Warn(ctx, "state set", map[string]any{
		"application_uuid": applicationUUID,
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)

	tflog.Trace(ctx, "read application assignment resource")
}

// confirmNotFoundRead runs notfound.Confirm for a Read not-found
// classification (see internal/common/notfound: this protects against a
// single flaky not-found response dropping real Terraform state). It
// returns (confirmed result, true) when Read should continue exactly as it
// would the original success path; it returns (nil, false) once it has
// already fully handled the response itself (RemoveResource or a
// Diagnostics error), in which case the caller must return immediately.
func (r *applicationAssignmentResource) confirmNotFoundRead(
	ctx context.Context,
	svc appAssignmentServiceAPI,
	applicationUUID string,
	resp *resource.ReadResponse,
) (*sdk.AppAssignmentRuleV2Model, bool) {
	confirmed, stillNotFound, confirmErr := notfound.Confirm(ctx, isNotFoundAPIError, func(ctx context.Context) (*sdk.AppAssignmentRuleV2Model, error) {
		_, result, err := svc.GetAssignmentRuleAsync(ctx, applicationUUID)
		return result, err
	})
	if confirmErr != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read assignment rule, got error: %s", confirmErr))
		return nil, false
	}
	if stillNotFound {
		resp.State.RemoveResource(ctx)
		return nil, false
	}
	tflog.Warn(ctx, fmt.Sprintf("UEM returned not-found then found for application assignment %s; kept in state", applicationUUID), map[string]any{
		"application_uuid": applicationUUID,
	})
	return confirmed, true
}

func (r *applicationAssignmentResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse) {

	var data tf.AppAssignmentRuleModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	applicationUUID, err := data.FetchValidAppUUID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid application_uuid", err.Error())
		return
	}

	svc, err := r.appAssignmentService(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to initialize application assignment service: %s", err))
		return
	}

	apiBody, diags := assignmentState.AppAssignmentRuleToAPI(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err = svc.UpdateAssignmentRuleAsync(ctx, applicationUUID, apiBody)
	if err != nil {
		if isNotFoundAPIError(err) {
			r.confirmNotFoundUpdate(ctx, svc, applicationUUID, resp)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update assignment rule, got error: %s", err))
		return
	}

	r.refreshIntoState(
		ctx,
		svc,
		applicationUUID,
		&data,
		&resp.State,
		&resp.Diagnostics)

	tflog.Trace(ctx, "updated application assignment resource")
}

// confirmNotFoundUpdate runs notfound.Confirm for an Update not-found
// classification (see internal/common/notfound: this protects against a
// single flaky not-found response dropping real Terraform state). Unlike
// Read's confirmNotFoundRead, Update never resumes inline: the write that
// just failed not-found may or may not have already applied on UEM's side,
// and there is no way to tell from here, so blindly re-issuing the same
// write is not something this package can call "clean" -- it risks
// double-applying the update against a live tenant. If the confirming
// re-GET finds the assignment rule gone, this drops state exactly as it
// would without notfound.Confirm; if it finds the rule present after all,
// this surfaces a clear "re-run apply" error and leaves state untouched
// (the rule is confirmed to still exist, so removing it from state would be
// wrong).
func (r *applicationAssignmentResource) confirmNotFoundUpdate(
	ctx context.Context,
	svc appAssignmentServiceAPI,
	applicationUUID string,
	resp *resource.UpdateResponse,
) {
	_, stillNotFound, confirmErr := notfound.Confirm(ctx, isNotFoundAPIError, func(ctx context.Context) (*sdk.AppAssignmentRuleV2Model, error) {
		_, result, err := svc.GetAssignmentRuleAsync(ctx, applicationUUID)
		return result, err
	})
	if confirmErr != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update assignment rule, got error: %s", confirmErr))
		return
	}
	if stillNotFound {
		resp.State.RemoveResource(ctx)
		return
	}
	tflog.Warn(ctx, fmt.Sprintf("UEM returned not-found then found for application assignment %s during update; re-run apply", applicationUUID), map[string]any{
		"application_uuid": applicationUUID,
	})
	resp.Diagnostics.AddError("Client Error", "UEM returned not-found then found during update; re-run apply")
}

func (r *applicationAssignmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data tf.AppAssignmentRuleModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	applicationUUID, err := data.FetchValidAppUUID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid application_uuid", err.Error())
		return
	}

	svc, err := r.appAssignmentService(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to initialize application assignment service: %s", err))
		return
	}

	apiBody := assignmentState.EmptyAppAssignmentRuleToAPI()

	_, err = svc.UpdateAssignmentRuleAsync(ctx, applicationUUID, apiBody)
	if err != nil {
		if isNotFoundAPIError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update assignment rule, got error: %s", err))
		return
	}

	tflog.Trace(ctx, "deleted application resource")
}

func (r *applicationAssignmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	applicationUUID, err := tf.ValidateAppUUID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid Application UUID",
			err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("application_uuid"), applicationUUID)...)
}

// refreshIntoState calls GetAssignmentRuleAsync and writes the result directly
// into the provided state and diagnostics. Used by Create and Update to avoid
// the Go by-value copy trap when calling r.Read() inline.
func (r *applicationAssignmentResource) refreshIntoState(
	ctx context.Context,
	svc appAssignmentServiceAPI,
	applicationUUID string,
	data *tf.AppAssignmentRuleModel,
	state *tfsdk.State,
	diags *diag.Diagnostics,
) {
	_, result, err := svc.GetAssignmentRuleAsync(ctx, applicationUUID)
	if err != nil {
		// Non-fatal on post-write refresh; log but preserve the minimal state
		// that was already written so Terraform at least tracks the resource.
		tflog.Warn(ctx, "post-write refresh failed; state may be stale", map[string]any{
			"application_uuid": applicationUUID,
			"error":            err.Error(),
		})
		assignmentState.SetMinimalState(data, applicationUUID)
		diags.Append(state.Set(ctx, data)...)
		return
	}
	if result != nil {
		diags.Append(assignmentState.ReadAPIIntoState(ctx, data, result)...)
		if diags.HasError() {
			return
		}
	}
	assignmentState.SetMinimalState(data, applicationUUID)
	diags.Append(state.Set(ctx, data)...)
}
