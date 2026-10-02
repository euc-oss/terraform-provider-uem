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

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/purchased-app/assignment/models"
	purchasedState "github.com/euc-oss/terraform-provider-uem/internal/application/purchased-app/assignment/state"
	"github.com/euc-oss/terraform-provider-uem/internal/common/notfound"
	"github.com/euc-oss/terraform-provider-uem/internal/providerdata"
)

func (r *purchasedApplicationAssignmentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
		if data.newPurchasedAppAssignmentService != nil {
			r.newPurchasedAppAssignmentService = data.newPurchasedAppAssignmentService
		} else {
			r.newPurchasedAppAssignmentService = defaultPurchasedAppAssignmentServiceFactory
		}
		if data.newVppPlatformLookup != nil {
			r.newVppPlatformLookup = data.newVppPlatformLookup
		} else {
			r.newVppPlatformLookup = defaultVppPlatformLookupFactory
		}
	case *providerdata.ProviderData:
		if data == nil || data.Client == nil {
			resp.Diagnostics.AddError("Unexpected Resource Configure Type", "Provider resource data was nil or missing a configured SDK client.")
			return
		}
		r.client = data.Client
		r.newPurchasedAppAssignmentService = defaultPurchasedAppAssignmentServiceFactory
		r.newVppPlatformLookup = defaultVppPlatformLookupFactory
	case *sdk.Client:
		r.client = data
		r.newPurchasedAppAssignmentService = defaultPurchasedAppAssignmentServiceFactory
		r.newVppPlatformLookup = defaultVppPlatformLookupFactory
	default:
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", resourceTypeErrorDetail(req.ProviderData))
		return
	}

	tflog.Trace(ctx, "configure purchased application assignment resource")
}

func (r *purchasedApplicationAssignmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data tf.PurchasedAppAssignmentRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	applicationUUID, err := data.FetchValidAppUUID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid application_uuid", err.Error())
		return
	}

	resp.Diagnostics.Append(tf.ValidateAssignmentsTargeting(ctx, data.Assignments)...)
	if resp.Diagnostics.HasError() {
		return
	}

	svc, err := r.purchasedAppAssignmentService(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to initialize purchased application assignment service: %s", err))
		return
	}

	apiBody, diags := purchasedState.PurchasedAppAssignmentRuleToAPI(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err = svc.UpdateAssignmentRuleAsync(ctx, applicationUUID, apiBody)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create purchased application assignment rule, got error: %s", err))
		return
	}

	r.refreshIntoState(ctx, svc, applicationUUID, &data, &resp.State, &resp.Diagnostics)
	tflog.Trace(ctx, "created purchased application assignment resource")
}

func (r *purchasedApplicationAssignmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data tf.PurchasedAppAssignmentRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	applicationUUID, err := data.FetchValidAppUUID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid application_uuid", err.Error())
		return
	}

	svc, err := r.purchasedAppAssignmentService(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to initialize purchased application assignment service: %s", err))
		return
	}

	_, result, err := svc.GetAssignmentRuleAsync(ctx, applicationUUID)
	if err != nil {
		if !isNotFoundAPIError(err) {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read purchased application assignment rule, got error: %s", err))
			return
		}
		confirmed, ok := r.confirmNotFoundRead(ctx, svc, applicationUUID, resp)
		if !ok {
			return
		}
		result = confirmed
	}
	if result == nil {
		purchasedState.SetMinimalState(&data, applicationUUID)
		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
		return
	}

	purchasedState.SetMinimalState(&data, applicationUUID)
	resp.Diagnostics.Append(purchasedState.ReadAPIIntoState(ctx, &data, result)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// confirmNotFoundRead runs notfound.Confirm for a Read not-found
// classification (see internal/common/notfound: this protects against a
// single flaky not-found response dropping real Terraform state). It
// returns (confirmed result, true) when Read should continue exactly as it
// would the original success path; it returns (nil, false) once it has
// already fully handled the response itself (RemoveResource or a
// Diagnostics error), in which case the caller must return immediately.
func (r *purchasedApplicationAssignmentResource) confirmNotFoundRead(
	ctx context.Context,
	svc purchasedAppAssignmentServiceAPI,
	applicationUUID string,
	resp *resource.ReadResponse,
) (*sdk.AppAssignmentRuleV2Model, bool) {
	confirmed, stillNotFound, confirmErr := notfound.Confirm(ctx, isNotFoundAPIError, func(ctx context.Context) (*sdk.AppAssignmentRuleV2Model, error) {
		_, result, err := svc.GetAssignmentRuleAsync(ctx, applicationUUID)
		return result, err
	})
	if confirmErr != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read purchased application assignment rule, got error: %s", confirmErr))
		return nil, false
	}
	if stillNotFound {
		resp.State.RemoveResource(ctx)
		return nil, false
	}
	tflog.Warn(ctx, fmt.Sprintf("UEM returned not-found then found for purchased application assignment %s; kept in state", applicationUUID), map[string]any{
		"application_uuid": applicationUUID,
	})
	return confirmed, true
}

func (r *purchasedApplicationAssignmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data tf.PurchasedAppAssignmentRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	applicationUUID, err := data.FetchValidAppUUID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid application_uuid", err.Error())
		return
	}

	resp.Diagnostics.Append(tf.ValidateAssignmentsTargeting(ctx, data.Assignments)...)
	if resp.Diagnostics.HasError() {
		return
	}

	svc, err := r.purchasedAppAssignmentService(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to initialize purchased application assignment service: %s", err))
		return
	}

	apiBody, diags := purchasedState.PurchasedAppAssignmentRuleToAPI(ctx, &data)
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
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update purchased application assignment rule, got error: %s", err))
		return
	}

	r.refreshIntoState(ctx, svc, applicationUUID, &data, &resp.State, &resp.Diagnostics)
	tflog.Trace(ctx, "updated purchased application assignment resource")
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
func (r *purchasedApplicationAssignmentResource) confirmNotFoundUpdate(
	ctx context.Context,
	svc purchasedAppAssignmentServiceAPI,
	applicationUUID string,
	resp *resource.UpdateResponse,
) {
	_, stillNotFound, confirmErr := notfound.Confirm(ctx, isNotFoundAPIError, func(ctx context.Context) (*sdk.AppAssignmentRuleV2Model, error) {
		_, result, err := svc.GetAssignmentRuleAsync(ctx, applicationUUID)
		return result, err
	})
	if confirmErr != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update purchased application assignment rule, got error: %s", confirmErr))
		return
	}
	if stillNotFound {
		resp.State.RemoveResource(ctx)
		return
	}
	tflog.Warn(ctx, fmt.Sprintf("UEM returned not-found then found for purchased application assignment %s during update; re-run apply", applicationUUID), map[string]any{
		"application_uuid": applicationUUID,
	})
	resp.Diagnostics.AddError("Client Error", "UEM returned not-found then found during update; re-run apply")
}

func (r *purchasedApplicationAssignmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data tf.PurchasedAppAssignmentRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	applicationUUID, err := data.FetchValidAppUUID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid application_uuid", err.Error())
		return
	}

	svc, err := r.purchasedAppAssignmentService(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to initialize purchased application assignment service: %s", err))
		return
	}

	_, err = svc.UpdateAssignmentRuleAsync(ctx, applicationUUID, purchasedState.EmptyPurchasedAppAssignmentRuleToAPI())
	if err != nil {
		if isNotFoundAPIError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete purchased application assignment rule, got error: %s", err))
		return
	}

	tflog.Trace(ctx, "deleted purchased application assignment resource")
}

func (r *purchasedApplicationAssignmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	applicationUUID, err := tf.ValidateAppUUID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Application UUID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("application_uuid"), applicationUUID)...)
}

func (r *purchasedApplicationAssignmentResource) refreshIntoState(
	ctx context.Context,
	svc purchasedAppAssignmentServiceAPI,
	applicationUUID string,
	data *tf.PurchasedAppAssignmentRuleModel,
	state *tfsdk.State,
	diags *diag.Diagnostics,
) {
	// Snapshot the planned assignments before the readback overwrites
	// data.Assignments, so the post-readback backstop below can compare what
	// was requested against what UEM actually stored.
	var plannedAssignments []tf.PurchasedAppAssignmentModel
	if !data.Assignments.IsNull() && !data.Assignments.IsUnknown() {
		var elemDiags diag.Diagnostics
		elemDiags.Append(data.Assignments.ElementsAs(ctx, &plannedAssignments, false)...)
		if elemDiags.HasError() {
			plannedAssignments = nil
		}
	}

	_, result, err := svc.GetAssignmentRuleAsync(ctx, applicationUUID)
	if err != nil {
		tflog.Warn(ctx, "post-write refresh failed; state may be stale", map[string]any{
			"application_uuid": applicationUUID,
			"error":            err.Error(),
		})
		purchasedState.SetMinimalState(data, applicationUUID)
		diags.Append(state.Set(ctx, data)...)
		return
	}
	if result != nil {
		diags.Append(purchasedState.ReadAPIIntoState(ctx, data, result)...)
		if diags.HasError() {
			return
		}

		// Backstop: UEM silently ignores some VPP restriction flags depending
		// on the app's platform (internal-task), which the provider cannot know in
		// advance. Compare the plan against what was actually stored and
		// surface a clear error naming each dropped flag, while still
		// setting state to the honest, read-back values below.
		if plannedAssignments != nil {
			var actualAssignments []tf.PurchasedAppAssignmentModel
			if !data.Assignments.IsNull() && !data.Assignments.IsUnknown() {
				var elemDiags diag.Diagnostics
				elemDiags.Append(data.Assignments.ElementsAs(ctx, &actualAssignments, false)...)
				if !elemDiags.HasError() {
					if mismatches := compareRestrictionReadback(plannedAssignments, actualAssignments); len(mismatches) > 0 {
						diags.AddError(restrictionBackstopSummary, formatRestrictionMismatchDetail(mismatches))
					}
				}
			}
		}
	}
	purchasedState.SetMinimalState(data, applicationUUID)
	diags.Append(state.Set(ctx, data)...)
}
