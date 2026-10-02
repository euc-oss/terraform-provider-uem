package assignment

import (
	"context"
	"net/http"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

type mockPurchasedAppAssignmentService struct {
	lastUpdate *sdk.AppAssignmentRuleV2Model
}

func (m *mockPurchasedAppAssignmentService) GetAssignmentRuleAsync(ctx context.Context, applicationUUID string) (http.Header, *sdk.AppAssignmentRuleV2Model, error) {
	return nil, m.lastUpdate, nil
}

func (m *mockPurchasedAppAssignmentService) UpdateAssignmentRuleAsync(ctx context.Context, applicationUUID string, request *sdk.AppAssignmentRuleV2Model) (http.Header, error) {
	m.lastUpdate = request
	return nil, nil
}

func TestPurchasedAssignmentConfigureUnsupportedType(t *testing.T) {
	r := &purchasedApplicationAssignmentResource{}
	req := resource.ConfigureRequest{ProviderData: 42}
	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), req, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected diagnostics error for unsupported provider data type")
	}
}

func TestPurchasedAssignmentConfigureWithSDKClient(t *testing.T) {
	r := &purchasedApplicationAssignmentResource{}
	req := resource.ConfigureRequest{ProviderData: &sdk.Client{}}
	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("did not expect diagnostics error: %v", resp.Diagnostics)
	}
	if r.client == nil {
		t.Fatal("expected client to be set")
	}
	if r.newPurchasedAppAssignmentService == nil {
		t.Fatal("expected purchased app assignment service factory to be set")
	}
}

func TestMockPurchasedAppAssignmentServiceCapturesUpdateBody(t *testing.T) {
	mockSvc := &mockPurchasedAppAssignmentService{}
	allocated := 2
	body := &sdk.AppAssignmentRuleV2Model{
		Assignments: []sdk.AppAssignmentV2Model{{
			Priority: 0,
			Distribution: sdk.AppAssignmentDistributionV2Model{
				Name: "VPP",
				VppAppDetails: &sdk.AppAssignmentVppV1ModelV2{
					LicenseUsage: []sdk.AssignmentLicenseUsageV1ModelV2{{
						SmartGroupUUID: "cd9f26cd-b1a2-f80e-5961-be6b3839fbd7",
						Allocated:      &allocated,
					}},
				},
			},
		}},
	}
	_, err := mockSvc.UpdateAssignmentRuleAsync(context.Background(), "app-uuid", body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mockSvc.lastUpdate == nil || mockSvc.lastUpdate.Assignments[0].Distribution.VppAppDetails == nil {
		t.Fatal("expected mock to capture VPP update body")
	}
}

func emptyPurchasedAssignmentState(t *testing.T) tfsdk.State {
	t.Helper()

	r := &purchasedApplicationAssignmentResource{}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, schemaResp)

	ctx := context.Background()
	schemaType := schemaResp.Schema.Type().TerraformType(ctx)
	objType, ok := schemaType.(tftypes.Object)
	if !ok {
		t.Fatal("schema type is not an Object")
	}

	values := make(map[string]tftypes.Value, len(objType.AttributeTypes))
	for name, attrType := range objType.AttributeTypes {
		values[name] = tftypes.NewValue(attrType, nil)
	}

	return tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(schemaType, values),
	}
}

func TestPurchasedAssignmentImportState(t *testing.T) {
	ctx := context.Background()

	t.Run("valid application UUID", func(t *testing.T) {
		r := &purchasedApplicationAssignmentResource{}
		req := resource.ImportStateRequest{
			ID: "c4a174cd-4eae-e2e8-ac9f-75a2f24649d9",
		}
		resp := &resource.ImportStateResponse{State: emptyPurchasedAssignmentState(t)}

		r.ImportState(ctx, req, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
		}

		var applicationUUID types.String
		resp.Diagnostics.Append(resp.State.GetAttribute(ctx, path.Root("application_uuid"), &applicationUUID)...)
		if resp.Diagnostics.HasError() {
			t.Fatalf("failed to read application_uuid from state: %v", resp.Diagnostics)
		}
		if applicationUUID.ValueString() != "c4a174cd-4eae-e2e8-ac9f-75a2f24649d9" {
			t.Fatalf("application_uuid mismatch: got %q", applicationUUID.ValueString())
		}
	})

	t.Run("empty import ID", func(t *testing.T) {
		r := &purchasedApplicationAssignmentResource{}
		req := resource.ImportStateRequest{ID: ""}
		resp := &resource.ImportStateResponse{State: emptyPurchasedAssignmentState(t)}

		r.ImportState(ctx, req, resp)
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected diagnostics error for empty import ID")
		}
	})
}
