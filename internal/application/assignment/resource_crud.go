package assignment

import (
	"context"
	"fmt"

	sdk "github.com/euc-oss/terraform-sdk-uem"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/assignment/models"
	assignmentState "github.com/euc-oss/terraform-provider-uem/internal/application/assignment/state"
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
		if isNotFoundAPIError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read assignment rule, got error: %s", err))
		return
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
			resp.State.RemoveResource(ctx)
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
