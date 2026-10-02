package profile

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	commonerrors "github.com/euc-oss/terraform-provider-uem/internal/common/errors"
	"github.com/euc-oss/terraform-provider-uem/internal/common/notfound"
	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"
	profilestate "github.com/euc-oss/terraform-provider-uem/internal/profile/state"
	"github.com/euc-oss/terraform-provider-uem/internal/providerdata"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// lastPayloadMandatoryMessage is the message UEM 26.2 uses to reject a
// macOS (AppleOsX) profile Create/Update whose entity carries zero payload
// sections — API error 422 (errorCode "1012"). internal-task's removal-clears
// fix (nullWhenConfigNullObjectModifier/List/String in resource.go) makes
// it newly possible for a user to configure a macOS uem_profile with every
// managed block removed, which now plans and sends a genuinely empty
// payload instead of silently keeping the last applied value — so this
// previously theoretical UEM rejection is now reachable from ordinary use
// and needs an actionable hint rather than a bare passthrough of UEM's
// error text.
const lastPayloadMandatoryMessage = "Atleast one Payload is mandatory to create/update a AppleOsX Device Profile"

// lastPayloadMandatoryHint is appended to the diagnostic detail when err's
// chain matches lastPayloadMandatoryMessage.
const lastPayloadMandatoryHint = "a macOS profile must keep at least one payload; removing the last managed block isn't possible"

// isLastPayloadMandatoryError reports whether err (directly, wrapped, or as
// an *sdk.APIError) carries UEM's "Atleast one Payload is mandatory"
// rejection. It checks the typed *sdk.APIError's Message first (the
// precise, intended shape) and falls back to a substring check against the
// error's own text, so the hint still surfaces if the SDK ever stops
// wrapping this response as an *sdk.APIError.
func isLastPayloadMandatoryError(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *sdk.APIError
	if errors.As(err, &apiErr) {
		return strings.Contains(apiErr.Message, lastPayloadMandatoryMessage)
	}
	return strings.Contains(err.Error(), lastPayloadMandatoryMessage)
}

// profileWriteErrorDetail formats the diagnostic detail for a failed
// Create/Update, appending lastPayloadMandatoryHint when err is UEM's
// last-payload-mandatory rejection. Action is "create" or "update".
func profileWriteErrorDetail(action string, err error) string {
	detail := fmt.Sprintf("Unable to %s profile, got error: %s", action, err)
	if isLastPayloadMandatoryError(err) {
		detail = fmt.Sprintf("%s\n\n%s", detail, lastPayloadMandatoryHint)
	}
	return detail
}

// isProfileGoneAPIError reports whether err is the shape UEM 26.2 answers
// a GET (v2) for an already-deleted (or never-existing) profile with:
// HTTP 400 whose message is exactly "Invalid Profile <id>" (live-confirmed
// for both a never-existed id and a recently-deleted id, e.g. "Invalid
// Profile 99999999."; the trailing "." is optional/trim-tolerant, handled
// by commonerrors.IsAPIErrorExactMatch). DELETE does NOT answer with this
// shape; see isProfileDeleteGoneAPIError. The match is pinned to the
// specific profileID being read/deleted, not just the "Invalid Profile"
// prefix: an unanchored prefix match would also catch an unrelated
// validation error that happens to start with the same words (e.g.
// "Invalid ProfileScope value.") or a "gone" message for a DIFFERENT id,
// either of which would misclassify a genuine error as not-found. The
// response's errorCode field is populated but always mirrors the HTTP
// status ("400"), so it carries no extra specificity here and is
// deliberately not part of the match.
func isProfileGoneAPIError(err error, profileID int) bool {
	expected := fmt.Sprintf("Invalid Profile %d", profileID)
	return commonerrors.IsAPIErrorExactMatch(err, http.StatusBadRequest, expected)
}

// profileDeleteGoneMessage is the exact message UEM 26.2 returns for a
// DELETE of an already-deleted profile.
const profileDeleteGoneMessage = "Profile not found or User does not have access to the Profile."

// isProfileDeleteGoneAPIError reports whether err is the shape UEM 26.2
// answers a DELETE (v1, the only version the route is registered at, and
// the version the pinned SDK sends) for an already-deleted profile with:
// HTTP 400 whose message is exactly profileDeleteGoneMessage
// (live-confirmed on this pin; also the SDK's live-captured
// profiles_delete_not_found.json fixture). GET answers the same condition
// with a different shape ("Invalid Profile <id>.", isProfileGoneAPIError),
// so this matcher is used by Delete ONLY. It is an exact full-message
// match (trailing "." and whitespace tolerant): any other 400 from
// DELETE, e.g. a validation or "still assigned" error, stays an error.
// The message carries no id, so it cannot be pinned to profileID.
func isProfileDeleteGoneAPIError(err error) bool {
	return commonerrors.IsAPIErrorExactMatch(err, http.StatusBadRequest, profileDeleteGoneMessage)
}

// orgGroupIDApplyTimeGuard returns a diagnostic when orgGroupID is not known
// and non-null at apply time (Create/Update), or nil when it's safe to build
// General from. Since org_group_id is a Required, non-Computed schema
// attribute, Terraform Core resolves it to a known, non-null value before
// ever calling Create/Update -- ValidateConfig (internal-task, general-ogid canonical rules Q1)
// already rejects a known-but-non-numeric value at plan time, and Terraform
// Core itself rejects a null Required attribute in config before the
// provider is invoked at all. Reaching Create/Update with orgGroupID still
// null or unknown therefore indicates a framework-level inconsistency (e.g.
// an upstream dependency that didn't resolve as promised), not a normal user
// error -- so this fails loudly instead of letting
// platform.BuildGeneralV2Create/BuildGeneralV4Create silently omit
// General.ManagedLocationGroupID, which UEM's server model treats as a
// non-nullable field that must be sent on every write (canonical rules
// general-ogid Q1/Q2).
func orgGroupIDApplyTimeGuard(orgGroupID types.String) diag.Diagnostic {
	if !orgGroupID.IsNull() && !orgGroupID.IsUnknown() {
		return nil
	}
	return diag.NewAttributeErrorDiagnostic(
		path.Root("org_group_id"),
		"org_group_id is not known at apply time",
		"org_group_id is Required and not Computed, so Terraform should have resolved it to a known, non-null "+
			"value before Create/Update ran. UEM's General.ManagedLocationGroupID is a non-nullable field the "+
			"provider must send on every profile write (canonical rules general-ogid Q1/Q2), so it refuses to "+
			"silently omit it rather than send an incomplete request. This indicates a Terraform Core or provider "+
			"inconsistency; please report it.",
	)
}

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
	case *providerdata.ProviderData:
		if data == nil || data.Client == nil {
			resp.Diagnostics.AddError("Unexpected Resource Configure Type", "Provider resource data was nil or missing a configured SDK client.")
			return
		}
		r.client = data.Client
		r.newProfileService = defaultProfileServiceFactory
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

	if diag := orgGroupIDApplyTimeGuard(data.OrgGroupID); diag != nil {
		resp.Diagnostics.Append(diag)
		return
	}

	// Normalize computed defaults on the plan model so Create sees stable
	// General values regardless of omitted optional attributes.
	//
	// profile_scope is deliberately NOT defaulted here: the
	// schema's plain Optional+Computed declaration (no plan modifier) already
	// plans it as unknown when configuration omits it on Create, and
	// data.ProfileScope.ValueString() sends "" for an unknown value, which
	// UEM's own GeneralPayloadV2Entity/V4Entity json:"omitempty" tag omits
	// from the wire — letting UEM apply its own default instead of this
	// provider forcing "Production" (live-confirmed 2026-09-25 on the 26.2
	// lab tenant, guarded B16 follow-up create: an unsent profile_scope came
	// back as "", not "Production", so filling "Production" here produced
	// "was cty.StringVal(\"Production\"), but now cty.StringVal(\"\")").
	platform := data.Platform.ValueString()
	sdkPlatform := platformForSDK(platform)

	// assignment_type: provider-only default, not a UEM mirror. UEM's
	// AssignmentType is a non-nullable server field with no default of its
	// own — an omitted value fails validation server-side ("AssignmentType
	// cannot be null"). "Auto" avoids a guaranteed apply-time failure.
	// UEM source: AirWatch API/AirWatch.ServiceModel/Profiles/V2/Resources/GeneralPayloadV2Entity.cs:691-707,148
	// (canonical Q4).
	assignmentType := "Auto"
	if !data.AssignmentType.IsNull() && !data.AssignmentType.IsUnknown() {
		assignmentType = data.AssignmentType.ValueString()
	}
	// is_active: UEM source: AirWatch API/AirWatch.ServiceModel/Profiles/V2/Resources/GeneralPayloadV2Entity.cs:691-707,140
	// (canonical Q4) — GeneralPayloadV2Entity()'s constructor sets
	// IsActive = true unconditionally for every platform (not Android-only,
	// as previously suspected). This default mirrors that server-side
	// constructor default.
	isActive := true
	if !data.IsActive.IsNull() && !data.IsActive.IsUnknown() {
		isActive = data.IsActive.ValueBool()
	}
	profileContext := ""
	if !data.ProfileContext.IsNull() && !data.ProfileContext.IsUnknown() {
		profileContext = data.ProfileContext.ValueString()
	} else if profilestate.IsApplePlatform(platform) {
		// Provider convenience, kept by owner decision: UEM requires
		// ProfileContext (User or Device) and has no server default
		// (canonical Q4, GeneralPayloadV2Entity.cs:691-707), so the provider
		// supplies Device when it is unset.
		profileContext = "Device"
	}

	data.AssignmentType = types.StringValue(assignmentType)
	data.IsActive = types.BoolValue(isActive)
	if profileContext != "" {
		data.ProfileContext = types.StringValue(profileContext)
	}

	var profileID int

	id, err := r.createTypedProfile(ctx, &data, sdkPlatform)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", profileWriteErrorDetail("create", err))
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
	if data.ProfileScope.IsUnknown() {
		// The readback above (svc.Get) normally resolves this from the live
		// profile; this is only reached if that readback failed, and mirrors
		// UEM's own observed default for an unsent profile_scope (see the
		// comment above at the top of Create).
		data.ProfileScope = types.StringValue("")
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
		classify := func(e error) bool { return isNotFoundAPIError(e) || isProfileGoneAPIError(e, profileID) }
		if !classify(err) {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read profile, got error: %s", err))
			return
		}
		// Confirm protects against a single flaky not-found response during a
		// live-tenant refresh dropping real Terraform state (see the 7fx
		// incident documented on internal/common/notfound): wait briefly and
		// re-GET once before actually dropping state.
		confirmed, stillNotFound, confirmErr := notfound.Confirm(ctx, classify, func(ctx context.Context) (*sdk.ProfileResult, error) {
			return svc.Get(ctx, profileID)
		})
		if confirmErr != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read profile, got error: %s", confirmErr))
			return
		}
		if stillNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		tflog.Warn(ctx, fmt.Sprintf("UEM returned not-found then found for profile %d; kept in state", profileID), map[string]any{
			"profile_id": profileID,
		})
		result = confirmed
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

	if diag := orgGroupIDApplyTimeGuard(data.OrgGroupID); diag != nil {
		resp.Diagnostics.Append(diag)
		return
	}

	profileID, err := strconv.Atoi(data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Parse Error", fmt.Sprintf("Unable to parse profile ID: %s", err))
		return
	}

	platform := data.Platform.ValueString()
	sdkPlatform := platformForSDK(platform)

	// profile_scope is deliberately NOT defaulted here: see
	// the matching comment in Create. On Update, the plan already carries the
	// prior state value forward when configuration omits profile_scope
	// (Optional+Computed with no plan modifier), so re-defaulting it to
	// "Production" here would discard that correctly-planned value.
	// assignment_type: provider-only default, not a UEM mirror. UEM's
	// AssignmentType is a non-nullable server field with no default of its
	// own — an omitted value fails validation server-side ("AssignmentType
	// cannot be null"). "Auto" avoids a guaranteed apply-time failure.
	// UEM source: AirWatch API/AirWatch.ServiceModel/Profiles/V2/Resources/GeneralPayloadV2Entity.cs:691-707,148
	// (canonical Q4).
	assignmentType := "Auto"
	if !data.AssignmentType.IsNull() && !data.AssignmentType.IsUnknown() {
		assignmentType = data.AssignmentType.ValueString()
	}
	// is_active: UEM source: AirWatch API/AirWatch.ServiceModel/Profiles/V2/Resources/GeneralPayloadV2Entity.cs:691-707,140
	// (canonical Q4) — GeneralPayloadV2Entity()'s constructor sets
	// IsActive = true unconditionally for every platform (not Android-only,
	// as previously suspected). This default mirrors that server-side
	// constructor default.
	isActive := true
	if !data.IsActive.IsNull() && !data.IsActive.IsUnknown() {
		isActive = data.IsActive.ValueBool()
	}
	profileContext := ""
	if !data.ProfileContext.IsNull() && !data.ProfileContext.IsUnknown() {
		profileContext = data.ProfileContext.ValueString()
	} else if profilestate.IsApplePlatform(platform) {
		// Provider convenience, kept by owner decision: UEM requires
		// ProfileContext (User or Device) and has no server default
		// (canonical Q4, GeneralPayloadV2Entity.cs:691-707), so the provider
		// supplies Device when it is unset.
		profileContext = "Device"
	}

	data.AssignmentType = types.StringValue(assignmentType)
	data.IsActive = types.BoolValue(isActive)
	if profileContext != "" {
		data.ProfileContext = types.StringValue(profileContext)
	}

	if err := r.updateTypedProfile(ctx, &data, &priorState, profileID, sdkPlatform); err != nil {
		resp.Diagnostics.AddError("Client Error", profileWriteErrorDetail("update", err))
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
	if data.ProfileScope.IsUnknown() {
		data.ProfileScope = types.StringValue("")
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
		if isNotFoundAPIError(err) || isProfileGoneAPIError(err, profileID) || isProfileDeleteGoneAPIError(err) {
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
			fmt.Sprintf("Expected import ID in format '<profile_id>:<platform>', got: %s\nExample: 12345:Android", req.ID),
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
