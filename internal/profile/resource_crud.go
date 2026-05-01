package profile

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"
	profilestate "github.com/euc-oss/terraform-provider-uem/internal/profile/state"
	sdk "github.com/euc-oss/terraform-sdk-uem"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

func (r *ProfileResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
		if data.newProfileService != nil {
			r.newProfileService = data.newProfileService
		} else {
			r.newProfileService = defaultProfileServiceFactory
		}
	case *sdk.Client:
		// Backward-compatible path for direct unit tests that still pass *sdk.Client.
		if data == nil {
			resp.Diagnostics.AddError("Unexpected Resource Configure Type", "Provider resource data was nil or missing a configured SDK client.")
			return
		}
		r.client = data
		r.newProfileService = defaultProfileServiceFactory
	default:
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", resourceTypeErrorDetail(req.ProviderData))
		return
	}
}

func (r *ProfileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data profilemodels.ProfileResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Normalize computed defaults on the plan model so Create sees stable
	// General values regardless of omitted optional attributes.
	platform := data.Platform.ValueString()
	sdkPlatform := platformForSDK(platform)

	assignmentType := "Auto"
	if !data.AssignmentType.IsNull() && !data.AssignmentType.IsUnknown() {
		assignmentType = data.AssignmentType.ValueString()
	}
	profileScope := "Production"
	if !data.ProfileScope.IsNull() && !data.ProfileScope.IsUnknown() {
		profileScope = data.ProfileScope.ValueString()
	}
	isActive := true
	if !data.IsActive.IsNull() && !data.IsActive.IsUnknown() {
		isActive = data.IsActive.ValueBool()
	}
	profileContext := ""
	if !data.ProfileContext.IsNull() && !data.ProfileContext.IsUnknown() {
		profileContext = data.ProfileContext.ValueString()
	} else if profilestate.IsApplePlatform(platform) {
		profileContext = "Device"
	}

	data.AssignmentType = types.StringValue(assignmentType)
	data.ProfileScope = types.StringValue(profileScope)
	data.IsActive = types.BoolValue(isActive)
	if profileContext != "" {
		data.ProfileContext = types.StringValue(profileContext)
	}

	var profileID int

	id, err := r.createTypedProfile(ctx, &data, sdkPlatform)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create profile, got error: %s", err))
		return
	}
	profileID = id

	data.ID = types.StringValue(strconv.Itoa(profileID))

	if data.UUID.IsUnknown() {
		data.UUID = types.StringValue("")
	}
	if data.ProfileContext.IsUnknown() {
		data.ProfileContext = types.StringValue(profileContext)
	}

	svc, err := r.profileService(ctx)
	if err == nil {
		svc.RegisterEntry(profileID, sdkPlatform)
		if result, getErr := svc.Get(ctx, profileID); getErr == nil {
			profilestate.ReadProfileIntoState(ctx, &data, result)
		} else {
			tflog.Warn(ctx, "Unable to read back created profile", map[string]any{"profile_id": profileID, "error": getErr.Error()})
		}
	} else {
		tflog.Warn(ctx, "Unable to initialize profile service for readback", map[string]any{"profile_id": profileID, "error": err.Error()})
	}

	if data.Passcode != nil {
		profilestate.NormalizePasscodeUnknownsToNull(data.Passcode)
	}
	profilestate.NormalizeNetworkUnknownsToNull(data.NetworkList)
	profilestate.NormalizeCredentialsUnknownsToNull(data.CredentialsList)
	if data.Description.IsUnknown() {
		data.Description = types.StringValue("")
	}

	tflog.Trace(ctx, "created a profile resource")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ProfileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data profilemodels.ProfileResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	profileID, err := strconv.Atoi(data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Parse Error", fmt.Sprintf("Unable to parse profile ID: %s", err))
		return
	}

	svc, err := r.profileService(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to initialize profile service: %s", err))
		return
	}

	if platform := data.Platform.ValueString(); platform != "" {
		svc.RegisterEntry(profileID, platformForSDK(platform))
	}

	result, err := svc.Get(ctx, profileID)
	if err != nil {
		if isNotFoundAPIError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read profile, got error: %s", err))
		return
	}

	profilestate.ReadProfileIntoState(ctx, &data, result)
	profilestate.EnsureComputedDefaults(&data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ProfileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data profilemodels.ProfileResourceModel
	var priorState profilemodels.ProfileResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &priorState)...)
	if resp.Diagnostics.HasError() {
		return
	}

	profileID, err := strconv.Atoi(data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Parse Error", fmt.Sprintf("Unable to parse profile ID: %s", err))
		return
	}

	platform := data.Platform.ValueString()
	sdkPlatform := platformForSDK(platform)

	assignmentType := "Auto"
	if !data.AssignmentType.IsNull() && !data.AssignmentType.IsUnknown() {
		assignmentType = data.AssignmentType.ValueString()
	}
	profileScope := "Production"
	if !data.ProfileScope.IsNull() && !data.ProfileScope.IsUnknown() {
		profileScope = data.ProfileScope.ValueString()
	}
	isActive := true
	if !data.IsActive.IsNull() && !data.IsActive.IsUnknown() {
		isActive = data.IsActive.ValueBool()
	}
	profileContext := ""
	if !data.ProfileContext.IsNull() && !data.ProfileContext.IsUnknown() {
		profileContext = data.ProfileContext.ValueString()
	} else if profilestate.IsApplePlatform(platform) {
		profileContext = "Device"
	}

	data.AssignmentType = types.StringValue(assignmentType)
	data.ProfileScope = types.StringValue(profileScope)
	data.IsActive = types.BoolValue(isActive)
	if profileContext != "" {
		data.ProfileContext = types.StringValue(profileContext)
	}

	if err := r.updateTypedProfile(ctx, &data, &priorState, profileID, sdkPlatform); err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update profile, got error: %s", err))
		return
	}

	svc, err := r.profileService(ctx)
	if err == nil {
		svc.RegisterEntry(profileID, sdkPlatform)
		if result, getErr := svc.Get(ctx, profileID); getErr == nil {
			profilestate.ReadProfileIntoState(ctx, &data, result)
		} else {
			tflog.Warn(ctx, "Unable to read back updated profile", map[string]any{"profile_id": profileID, "error": getErr.Error()})
		}
	} else {
		tflog.Warn(ctx, "Unable to initialize profile service for readback", map[string]any{"profile_id": profileID, "error": err.Error()})
	}

	if data.Passcode != nil {
		profilestate.NormalizePasscodeUnknownsToNull(data.Passcode)
	}
	profilestate.NormalizeNetworkUnknownsToNull(data.NetworkList)
	profilestate.NormalizeCredentialsUnknownsToNull(data.CredentialsList)
	if data.Description.IsUnknown() {
		data.Description = types.StringValue("")
	}
	if data.UUID.IsUnknown() {
		data.UUID = types.StringValue("")
	}
	if data.ProfileContext.IsUnknown() {
		data.ProfileContext = types.StringValue(profileContext)
	}

	tflog.Trace(ctx, "updated a profile resource")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ProfileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data profilemodels.ProfileResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	profileID, err := strconv.Atoi(data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Parse Error", fmt.Sprintf("Unable to parse profile ID: %s", err))
		return
	}

	svc, err := r.profileService(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Profile Service Init Failed",
			fmt.Sprintf("Unable to initialize the UEM profile service: %s", err),
		)
		return
	}

	if platform := data.Platform.ValueString(); platform != "" {
		svc.RegisterEntry(profileID, platformForSDK(platform))
	}

	if err := svc.Delete(ctx, profileID); err != nil {
		if isNotFoundAPIError(err) {
			tflog.Trace(ctx, "profile already absent on delete", map[string]interface{}{"profile_id": profileID})
		} else {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete profile, got error: %s", err))
			return
		}
	}

	tflog.Trace(ctx, "deleted a profile resource")
}

func (r *ProfileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, ":")
	if len(parts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Expected import ID in format '<profile_id>:<platform>', got: %s\nExample: 57996:Android", req.ID),
		)
		return
	}

	profileID := parts[0]
	platform := parts[1]

	if !isValidImportPlatform(platform) {
		resp.Diagnostics.AddError(
			"Invalid Platform",
			fmt.Sprintf("Platform must be one of: %v, got: %s", supportedImportPlatforms, platform),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), profileID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("platform"), platform)...)
}
