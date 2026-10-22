package macapplication

import (
	"context"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/internal-app/mac/models"
)

func TestMacConfigureUnsupportedType(t *testing.T) {
	r := &macapplicationResource{}
	req := resource.ConfigureRequest{ProviderData: 42}
	resp := &resource.ConfigureResponse{}

	r.Configure(context.Background(), req, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected diagnostics error for unsupported provider data type")
	}
}

func TestMacConfigureWithSDKClient(t *testing.T) {
	r := &macapplicationResource{}
	req := resource.ConfigureRequest{ProviderData: &sdk.Client{}}
	resp := &resource.ConfigureResponse{}

	r.Configure(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("did not expect diagnostics error: %v", resp.Diagnostics)
	}
	if r.client == nil {
		t.Fatal("expected client to be set")
	}
	if r.newBlobV1ResourceService == nil || r.newBlobV2ResourceService == nil || r.newMacAppResourceService == nil || r.newInternalAppResourceService == nil {
		t.Fatal("expected all service factories to be set")
	}
}

// newMacApplicationPlan builds a tfsdk.Plan matching the resource schema and
// populated with data, mirroring emptyMacApplicationState's schema-derived
// construction (see resource_import_test.go) but for the Plan side.
func newMacApplicationPlan(t *testing.T, data tf.MacApplicationResourceModel) tfsdk.Plan {
	t.Helper()

	var schemaResp resource.SchemaResponse
	(&macapplicationResource{}).Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)

	ctx := context.Background()
	schemaType := schemaResp.Schema.Type().TerraformType(ctx)
	if _, ok := schemaType.(tftypes.Object); !ok {
		t.Fatal("schema type is not an Object")
	}

	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	data = withTypedRecordNulls(data)
	diags := plan.Set(ctx, &data)
	if diags.HasError() {
		t.Fatalf("unexpected error building test plan: %v", diags)
	}
	return plan
}

// TestMacApplicationUpdate_NoOpCopiesPlanIntoState guards against the prior
// Update implementation's defect: it delegated to Delete-then-Create using
// locally seeded DeleteResponse/CreateResponse copies, so their results
// never propagated to the real *resource.UpdateResponse. Every mutable
// attribute is RequiresReplace (see resource.go PlanModifiers), so the SDK
// has no reachable update path — Update must be a no-op that copies plan
// straight into state.
func TestMacApplicationUpdate_NoOpCopiesPlanIntoState(t *testing.T) {
	r := &macapplicationResource{}

	planData := tf.MacApplicationResourceModel{
		ID:            typesInt32(42),
		UUID:          typesString("9af645a8-fef3-3e6d-3408-5cc69e0937d4"),
		OrgGroupID:    typesInt32(123),
		DMGFilePath:   typesString("/tmp/app.dmg"),
		PlistFilePath: typesString("/tmp/app.plist"),
		AppVersion:    tf.NewAppVersionValue("1.0.0"),
		DMGFileSHA256: typesString("deadbeef"),
	}

	req := resource.UpdateRequest{Plan: newMacApplicationPlan(t, planData)}
	resp := &resource.UpdateResponse{State: emptyMacApplicationState()}

	r.Update(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics error: %v", resp.Diagnostics)
	}

	var afterUpdate tf.MacApplicationResourceModel
	diags := resp.State.Get(context.Background(), &afterUpdate)
	if diags.HasError() {
		t.Fatalf("unexpected error reading state after update: %v", diags)
	}

	if afterUpdate.ID.ValueInt32() != 42 {
		t.Fatalf("expected id 42 to propagate from plan to state, got %v", afterUpdate.ID)
	}
	if afterUpdate.AppVersion.ValueString() != "1.0.0" {
		t.Fatalf("expected app_version to propagate from plan to state, got %v", afterUpdate.AppVersion)
	}
	if afterUpdate.OrgGroupID.ValueInt32() != 123 {
		t.Fatalf("expected org_group_id to propagate from plan to state, got %v", afterUpdate.OrgGroupID)
	}
}

func typesInt32(v int32) types.Int32 {
	return types.Int32Value(v)
}

func typesString(v string) types.String {
	return types.StringValue(v)
}
