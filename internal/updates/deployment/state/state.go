package state

import (
	"context"
	"fmt"
	"strings"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/euc-oss/terraform-sdk-uem/v26/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	tf "github.com/euc-oss/terraform-provider-uem/internal/updates/deployment/models"
)

var notificationElemType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"action":              types.StringType,
		"message":             types.StringType,
		"message_template_id": types.Int64Type,
	},
}

// ToCreateAPI maps Terraform plan to the create request body.
func ToCreateAPI(ctx context.Context, m *tf.UpdateDeploymentModel) (*sdk.DeviceUpdateDeploymentBaseV1Model, diag.Diagnostics) {
	var diags diag.Diagnostics
	if m == nil {
		return nil, diags
	}

	name, err := requireString("name", m.Name)
	if err != nil {
		diags.AddError("Invalid name", err.Error())
		return nil, diags
	}
	deploymentType, err := requireString("deployment_type", m.DeploymentType)
	if err != nil {
		diags.AddError("Invalid deployment_type", err.Error())
		return nil, diags
	}
	startTime, err := requireString("deployment_start_time", m.DeploymentStartTime)
	if err != nil {
		diags.AddError("Invalid deployment_start_time", err.Error())
		return nil, diags
	}
	parsedTime, err := client.ParseUEMTime(startTime)
	if err != nil {
		diags.AddError("Invalid deployment_start_time", err.Error())
		return nil, diags
	}

	smartGroups, sgDiags := stringListToAPI(ctx, m.SmartGroupUUIDs, "smart_group_uuids", true)
	diags.Append(sgDiags...)
	if diags.HasError() {
		return nil, diags
	}

	notifications, nDiags := notificationsToAPI(ctx, m.Notifications)
	diags.Append(nDiags...)
	if diags.HasError() {
		return nil, diags
	}

	// B16 decision table row #190 (KEEP+cite — direct match). UEM source:
	// Entity/src/EntitySln/WanderingWiFi.AirWatch.Entity/PatchManagement/Update/UpdateDeploymentType.cs:15-43,
	// AirWatch API/AW.Mdm.Api/AW.Mdm.Api.Model.Validators/Updates/V1/DeviceUpdateDeploymentBaseV1ModelValidator.cs:59-60
	// (canonical Q27): DeploymentType is serialized with StringEnumConverter
	// (case-sensitive default) against the three [EnumMember] strings below;
	// the validator uses Enum.IsDefined on the deserialized value.
	// Uppercasing here and rejecting anything outside these three names is a
	// direct match.
	deploymentTypeUpper := strings.ToUpper(deploymentType)
	if deploymentTypeUpper != "DOWNLOAD_AND_INSTALL" && deploymentTypeUpper != "DOWNLOAD_ONLY" && deploymentTypeUpper != "INSTALL_ONLY" {
		diags.AddError("Invalid deployment_type", "deployment_type must be one of DOWNLOAD_AND_INSTALL, DOWNLOAD_ONLY, INSTALL_ONLY")
		return nil, diags
	}

	return &sdk.DeviceUpdateDeploymentBaseV1Model{
		Name:                name,
		DeploymentType:      deploymentTypeUpper,
		DeploymentStartTime: parsedTime,
		SmartGroupUUIDs:     smartGroups,
		Notifications:       notifications,
	}, diags
}

// ToUpdateAPI maps Terraform plan to the update request body.
func ToUpdateAPI(ctx context.Context, m *tf.UpdateDeploymentModel) (*sdk.DeviceUpdateDeploymentUpdateV1Model, diag.Diagnostics) {
	create, diags := ToCreateAPI(ctx, m)
	if diags.HasError() || create == nil {
		return nil, diags
	}
	return &sdk.DeviceUpdateDeploymentUpdateV1Model{
		Name:                create.Name,
		DeploymentType:      create.DeploymentType,
		DeploymentStartTime: create.DeploymentStartTime,
		SmartGroupUUIDs:     create.SmartGroupUUIDs,
		Notifications:       create.Notifications,
	}, diags
}

// ReadAPIIntoState maps GET response into Terraform state.
func ReadAPIIntoState(ctx context.Context, data *tf.UpdateDeploymentModel, api *sdk.DeviceUpdateDeploymentV1Model, deploymentUUID string) diag.Diagnostics {
	var diags diag.Diagnostics
	if data == nil || api == nil {
		return diags
	}

	if deploymentUUID == "" {
		deploymentUUID = strings.TrimSpace(data.ID.ValueString())
	}
	data.ID = types.StringValue(deploymentUUID)

	if api.DeviceUpdateUUID != "" {
		data.UpdateUUID = types.StringValue(api.DeviceUpdateUUID)
	}
	if api.OrganizationGroupUUID != "" {
		data.OrganizationGroupUUID = types.StringValue(api.OrganizationGroupUUID)
	}

	data.Name = types.StringValue(api.Name)
	data.DeploymentType = types.StringValue(api.DeploymentType)
	if !api.DeploymentStartTime.IsZero() {
		data.DeploymentStartTime = types.StringValue(api.DeploymentStartTime.String())
	} else {
		data.DeploymentStartTime = types.StringNull()
	}

	// B16 (b)-row #196 removal: store the server value exactly as returned.
	// types.ListValueFrom already maps a nil Go slice to a null list and a
	// non-nil empty slice to an empty list -- the prior override collapsed
	// both into an empty list, hiding whether UEM actually omitted the field.
	sgList, sgDiags := types.ListValueFrom(ctx, types.StringType, api.SmartGroupUUIDs)
	diags.Append(sgDiags...)
	data.SmartGroupUUIDs = sgList

	// B16 (b)-row #196 removal: build a nil slice (not an empty non-nil one)
	// when the API returned no notifications, so ListValueFrom below stores a
	// null list, not an empty one -- the server value, as-is. EXPECTED DIFF
	// RISK: notifications is Optional (not Computed); if UEM ever returns a
	// nil Notifications list after a create/update that sent a non-nil empty
	// list (or vice versa), this can perpetually diff against config.
	var tfNotifications []tf.NotificationModel
	if api.Notifications != nil {
		tfNotifications = make([]tf.NotificationModel, 0, len(api.Notifications))
		for _, n := range api.Notifications {
			item := tf.NotificationModel{
				Action:  types.StringValue(n.Action),
				Message: types.StringValue(n.Message),
			}
			if n.MessageTemplateID != nil {
				item.MessageTemplateID = types.Int64Value(int64(*n.MessageTemplateID))
			} else {
				item.MessageTemplateID = types.Int64Null()
			}
			tfNotifications = append(tfNotifications, item)
		}
	}
	nList, nDiags := types.ListValueFrom(ctx, notificationElemType, tfNotifications)
	diags.Append(nDiags...)
	data.Notifications = nList

	return diags
}

// requireString requires each caller's field (name, deployment_type,
// deployment_start_time, and the org-group/update-uuid path fields) to be a
// known, non-empty (post-trim) string.
//
// B16 decision table row #191 (KEEP+cite, partial). UEM source:
// AirWatch API/AW.Mdm.Api/AW.Mdm.Api.Model.Validators/Updates/V1/DeviceUpdateDeploymentBaseV1ModelValidator.cs
// (canonical Q27, general validator range): required-field status is
// confirmed for these fields; trimming itself has no server citation but is
// harmless client-side hygiene.
func requireString(field string, v types.String) (string, error) {
	if v.IsNull() || v.IsUnknown() {
		return "", fmt.Errorf("%s must be set and known", field)
	}
	trimmed := strings.TrimSpace(v.ValueString())
	if trimmed == "" {
		return "", fmt.Errorf("%s must not be empty", field)
	}
	return trimmed, nil
}

func stringListToAPI(ctx context.Context, list types.List, field string, required bool) ([]string, diag.Diagnostics) {
	var diags diag.Diagnostics
	if list.IsNull() || list.IsUnknown() {
		if required {
			diags.AddError("Invalid "+field, field+" must be set and known")
		}
		return nil, diags
	}

	var elems []types.String
	diags.Append(list.ElementsAs(ctx, &elems, false)...)
	if diags.HasError() {
		return nil, diags
	}

	out := make([]string, 0, len(elems))
	for _, e := range elems {
		if e.IsNull() || e.IsUnknown() {
			continue
		}
		v := strings.TrimSpace(e.ValueString())
		if v != "" {
			out = append(out, v)
		}
	}
	if required && len(out) == 0 {
		diags.AddError("Invalid "+field, field+" must contain at least one UUID")
	}
	return out, diags
}

func notificationsToAPI(ctx context.Context, list types.List) ([]sdk.NotificationV1Model, diag.Diagnostics) {
	var diags diag.Diagnostics
	if list.IsNull() || list.IsUnknown() {
		return nil, diags
	}

	var elems []tf.NotificationModel
	diags.Append(list.ElementsAs(ctx, &elems, false)...)
	if diags.HasError() {
		return nil, diags
	}

	// B16 decision table row #192 (KEEP+cite — direct match). UEM source:
	// AirWatch API/AW.Mdm.Api/AW.Mdm.Api.Model.Validators/Updates/V1/DeviceUpdateDeploymentBaseV1ModelValidator.cs:65
	// (canonical Q27): Notifications.Count <= 2.
	if len(elems) > 2 {
		diags.AddError("Invalid notifications", "notifications may contain at most 2 items")
		return nil, diags
	}

	out := make([]sdk.NotificationV1Model, 0, len(elems))
	for i, e := range elems {
		// B16 (b)-row #194 removal: send action exactly as configured (no
		// uppercasing), and no longer silently drop an entry whose fields
		// are all empty -- every configured list element is sent through.
		action := ""
		if !e.Action.IsNull() && !e.Action.IsUnknown() {
			action = e.Action.ValueString()
		}

		message := ""
		hasMessage := false
		if !e.Message.IsNull() && !e.Message.IsUnknown() {
			message = e.Message.ValueString()
			hasMessage = strings.TrimSpace(message) != ""
		}

		var templateID *int
		hasTemplate := false
		if !e.MessageTemplateID.IsNull() && !e.MessageTemplateID.IsUnknown() {
			id := int(e.MessageTemplateID.ValueInt64())
			templateID = &id
			hasTemplate = true
		}

		// B16 decision table row #193 (KEEP+cite — direct match). UEM
		// source: AirWatch API/AW.Mdm.Api/AW.Mdm.Api.Model.Validators/Updates/V1/DeviceUpdateDeploymentBaseV1ModelValidator.cs:66-69
		// (canonical Q27): message and message_template_id are mutually
		// exclusive per notification, and exactly one is required. This
		// check enforces the mutual-exclusivity half (both set is
		// rejected); it does not separately reject neither being set (both
		// schema attributes are Optional with no such cross-check here), so
		// the "exactly one required" half of Q27 is not fully enforced by
		// this function.
		if hasMessage && hasTemplate {
			diags.AddError(
				"Invalid notifications",
				fmt.Sprintf("notifications[%d]: message and message_template_id are mutually exclusive", i),
			)
			return nil, diags
		}

		out = append(out, sdk.NotificationV1Model{
			Action:            action,
			Message:           message,
			MessageTemplateID: templateID,
		})
	}
	return out, diags
}
