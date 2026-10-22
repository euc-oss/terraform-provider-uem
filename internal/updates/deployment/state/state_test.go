package state

import (
	"context"
	"testing"
	"time"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/euc-oss/terraform-sdk-uem/v26/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	tf "github.com/euc-oss/terraform-provider-uem/internal/updates/deployment/models"
)

func TestToCreateAPI_roundTripFields(t *testing.T) {
	ctx := context.Background()
	sg, diags := types.ListValueFrom(ctx, types.StringType, []string{"sg-1", "sg-2"})
	if diags.HasError() {
		t.Fatalf("smart groups: %v", diags)
	}

	model := &tf.UpdateDeploymentModel{
		UpdateUUID:            types.StringValue("update-uuid"),
		OrganizationGroupUUID: types.StringValue("og-uuid"),
		Name:                  types.StringValue("Pilot"),
		DeploymentType:        types.StringValue("download_and_install"),
		DeploymentStartTime:   types.StringValue("2026-07-28T16:00:00.000Z"),
		SmartGroupUUIDs:       sg,
		Notifications:         types.ListNull(notificationElemType),
	}

	api, diags := ToCreateAPI(ctx, model)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if api.Name != "Pilot" {
		t.Fatalf("name mismatch: %q", api.Name)
	}
	if api.DeploymentType != "DOWNLOAD_AND_INSTALL" {
		t.Fatalf("deployment type mismatch: %q", api.DeploymentType)
	}
	if len(api.SmartGroupUUIDs) != 2 {
		t.Fatalf("smart groups mismatch: %#v", api.SmartGroupUUIDs)
	}
	if api.DeploymentStartTime.IsZero() {
		t.Fatal("expected non-zero deployment start time")
	}
}

func TestToCreateAPI_rejectsEmptySmartGroups(t *testing.T) {
	ctx := context.Background()
	empty, _ := types.ListValue(types.StringType, []attr.Value{})
	model := &tf.UpdateDeploymentModel{
		Name:                types.StringValue("Pilot"),
		DeploymentType:      types.StringValue("DOWNLOAD_ONLY"),
		DeploymentStartTime: types.StringValue("2026-07-28T16:00:00.000Z"),
		SmartGroupUUIDs:     empty,
	}

	_, diags := ToCreateAPI(ctx, model)
	if !diags.HasError() {
		t.Fatal("expected diagnostics for empty smart_group_uuids")
	}
}

func TestToCreateAPI_rejectsInvalidDeploymentType(t *testing.T) {
	ctx := context.Background()
	sg, diags := types.ListValueFrom(ctx, types.StringType, []string{"sg-1"})
	if diags.HasError() {
		t.Fatalf("smart groups: %v", diags)
	}
	model := &tf.UpdateDeploymentModel{
		Name:                types.StringValue("Pilot"),
		DeploymentType:      types.StringValue("BOGUS"),
		DeploymentStartTime: types.StringValue("2026-07-28T16:00:00.000Z"),
		SmartGroupUUIDs:     sg,
	}

	_, diags = ToCreateAPI(ctx, model)
	if !diags.HasError() {
		t.Fatal("expected diagnostics for invalid deployment_type")
	}
}

func TestReadAPIIntoState_mapsFields(t *testing.T) {
	ctx := context.Background()
	templateID := 42
	api := &sdk.DeviceUpdateDeploymentV1Model{
		Name:                  "Pilot",
		DeploymentType:        "DOWNLOAD_AND_INSTALL",
		DeploymentStartTime:   client.NewUEMTime(time.Date(2026, 7, 28, 16, 0, 0, 0, time.UTC)),
		DeviceUpdateUUID:      "update-uuid",
		OrganizationGroupUUID: "og-uuid",
		SmartGroupUUIDs:       []string{"sg-1"},
		Notifications: []sdk.NotificationV1Model{
			{Action: "INSTALL_SUCCESS", Message: "done", MessageTemplateID: &templateID},
		},
	}

	data := &tf.UpdateDeploymentModel{}
	diags := ReadAPIIntoState(ctx, data, api, "deployment-uuid")
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if data.ID.ValueString() != "deployment-uuid" {
		t.Fatalf("id mismatch: %q", data.ID.ValueString())
	}
	if data.UpdateUUID.ValueString() != "update-uuid" {
		t.Fatalf("update_uuid mismatch: %q", data.UpdateUUID.ValueString())
	}
	if data.OrganizationGroupUUID.ValueString() != "og-uuid" {
		t.Fatalf("organization_group_uuid mismatch: %q", data.OrganizationGroupUUID.ValueString())
	}
	if data.Name.ValueString() != "Pilot" {
		t.Fatalf("name mismatch: %q", data.Name.ValueString())
	}
}

func notificationList(t *testing.T, items ...tf.NotificationModel) types.List {
	t.Helper()
	ctx := context.Background()
	list, diags := types.ListValueFrom(ctx, notificationElemType, items)
	if diags.HasError() {
		t.Fatalf("notifications list: %v", diags)
	}
	return list
}

func baseModelWithNotifications(t *testing.T, notifications types.List) *tf.UpdateDeploymentModel {
	t.Helper()
	ctx := context.Background()
	sg, diags := types.ListValueFrom(ctx, types.StringType, []string{"sg-1"})
	if diags.HasError() {
		t.Fatalf("smart groups: %v", diags)
	}
	return &tf.UpdateDeploymentModel{
		Name:                types.StringValue("Pilot"),
		DeploymentType:      types.StringValue("DOWNLOAD_AND_INSTALL"),
		DeploymentStartTime: types.StringValue("2026-07-28T16:00:00.000Z"),
		SmartGroupUUIDs:     sg,
		Notifications:       notifications,
	}
}

func TestToCreateAPI_rejectsMoreThanTwoNotifications(t *testing.T) {
	notifications := notificationList(t,
		tf.NotificationModel{Action: types.StringValue("DOWNLOAD_SUCCESS"), Message: types.StringValue("one")},
		tf.NotificationModel{Action: types.StringValue("INSTALL_SUCCESS"), Message: types.StringValue("two")},
		tf.NotificationModel{Action: types.StringValue("INSTALL_SUCCESS"), Message: types.StringValue("three")},
	)

	_, diags := ToCreateAPI(context.Background(), baseModelWithNotifications(t, notifications))
	if !diags.HasError() {
		t.Fatal("expected diagnostics for more than 2 notifications")
	}
}

func TestToCreateAPI_rejectsMessageAndTemplateTogether(t *testing.T) {
	notifications := notificationList(t, tf.NotificationModel{
		Action:            types.StringValue("INSTALL_SUCCESS"),
		Message:           types.StringValue("done"),
		MessageTemplateID: types.Int64Value(42),
	})

	_, diags := ToCreateAPI(context.Background(), baseModelWithNotifications(t, notifications))
	if !diags.HasError() {
		t.Fatal("expected diagnostics for message and message_template_id together")
	}
}

// TestToCreateAPI_sendsAllConfiguredNotifications proves B16 (b)-row #194
// removal: an all-empty notification entry is no longer silently dropped --
// every configured list element is sent through -- and action is sent
// exactly as configured, not uppercased.
func TestToCreateAPI_sendsAllConfiguredNotifications(t *testing.T) {
	notifications := notificationList(t,
		tf.NotificationModel{},
		tf.NotificationModel{
			Action:  types.StringValue("install_success"),
			Message: types.StringValue("done"),
		},
	)

	api, diags := ToCreateAPI(context.Background(), baseModelWithNotifications(t, notifications))
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(api.Notifications) != 2 {
		t.Fatalf("expected 2 notifications (no empty-entry drop), got %d", len(api.Notifications))
	}
	if api.Notifications[0].Action != "" {
		t.Fatalf("expected the empty entry's action to stay empty, got %q", api.Notifications[0].Action)
	}
	if api.Notifications[1].Action != "install_success" {
		t.Fatalf("expected action to pass through unchanged, got %q", api.Notifications[1].Action)
	}
	if api.Notifications[1].Message != "done" {
		t.Fatalf("message mismatch: %q", api.Notifications[1].Message)
	}
}

// TestReadAPIIntoState_nilListsStayNull proves B16 (b)-row #196 removal: a
// nil SmartGroupUUIDs/Notifications from the API is stored as a null list,
// not silently rewritten to an empty list.
func TestReadAPIIntoState_nilListsStayNull(t *testing.T) {
	ctx := context.Background()
	api := &sdk.DeviceUpdateDeploymentV1Model{
		Name:                  "Pilot",
		DeploymentType:        "DOWNLOAD_AND_INSTALL",
		DeploymentStartTime:   client.NewUEMTime(time.Date(2026, 7, 28, 16, 0, 0, 0, time.UTC)),
		DeviceUpdateUUID:      "update-uuid",
		OrganizationGroupUUID: "og-uuid",
		SmartGroupUUIDs:       nil,
		Notifications:         nil,
	}

	data := &tf.UpdateDeploymentModel{}
	diags := ReadAPIIntoState(ctx, data, api, "deployment-uuid")
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !data.SmartGroupUUIDs.IsNull() {
		t.Errorf("expected smart_group_uuids to be null for a nil API list, got %v", data.SmartGroupUUIDs)
	}
	if !data.Notifications.IsNull() {
		t.Errorf("expected notifications to be null for a nil API list, got %v", data.Notifications)
	}
}

// TestReadAPIIntoState_emptyListsStayEmpty proves the read path also
// preserves a non-nil empty API list as an empty (not null) list.
func TestReadAPIIntoState_emptyListsStayEmpty(t *testing.T) {
	ctx := context.Background()
	api := &sdk.DeviceUpdateDeploymentV1Model{
		Name:                  "Pilot",
		DeploymentType:        "DOWNLOAD_AND_INSTALL",
		DeploymentStartTime:   client.NewUEMTime(time.Date(2026, 7, 28, 16, 0, 0, 0, time.UTC)),
		DeviceUpdateUUID:      "update-uuid",
		OrganizationGroupUUID: "og-uuid",
		SmartGroupUUIDs:       []string{},
		Notifications:         []sdk.NotificationV1Model{},
	}

	data := &tf.UpdateDeploymentModel{}
	diags := ReadAPIIntoState(ctx, data, api, "deployment-uuid")
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if data.SmartGroupUUIDs.IsNull() || len(data.SmartGroupUUIDs.Elements()) != 0 {
		t.Errorf("expected smart_group_uuids to be an empty (non-null) list, got %v", data.SmartGroupUUIDs)
	}
	if data.Notifications.IsNull() || len(data.Notifications.Elements()) != 0 {
		t.Errorf("expected notifications to be an empty (non-null) list, got %v", data.Notifications)
	}
}
