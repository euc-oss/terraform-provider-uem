package assignment

import (
	"context"
	"net/http"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

const (
	testScriptUUID   = "9af645a8-fef3-3e6d-3408-5cc69e0937d4"
	testOrgGroupUUID = "666ff6cc-aa5b-3c07-feaa-3a95d3a4bd2c"
)

// fakeAssignmentService answers the assignments calls with fixed results.
type fakeAssignmentService struct {
	getResult *sdk.ScriptAssignmentsSearchResultV1
	getErr    error
	bulkErr   error
}

func (f *fakeAssignmentService) AddScriptAssignmentAsync(context.Context, string, *sdk.CreateScriptAssignmentV1) (http.Header, error) {
	return nil, nil
}

func (f *fakeAssignmentService) BulkUpdateScriptAssignmentsAsync(context.Context, string, *sdk.BulkUpdateScriptAssignmentV1) (http.Header, error) {
	return nil, f.bulkErr
}

func (f *fakeAssignmentService) GetScriptAssignmentAsync(context.Context, string) (http.Header, *sdk.ScriptAssignmentResourceV1, error) {
	return nil, nil, nil
}

func (f *fakeAssignmentService) GetScriptAssignmentsAsync(context.Context, string) (http.Header, *sdk.ScriptAssignmentsSearchResultV1, error) {
	return nil, f.getResult, f.getErr
}

// fakeScriptService answers the parent-script GET and the OG-scoped list,
// counting calls so tests can prove a lookup never happened.
type fakeScriptService struct {
	script    *sdk.ScriptResourceV1
	scriptErr error
	listFn    func(string, *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions) (*sdk.ScriptsSearchResultV1, error)
	getCalls  int
	listCalls int
}

func (f *fakeScriptService) GetScriptAsync(context.Context, string) (http.Header, *sdk.ScriptResourceV1, error) {
	f.getCalls++
	return nil, f.script, f.scriptErr
}

func (f *fakeScriptService) GetScriptsByOrganizationGroupAsync(
	_ context.Context,
	og string,
	opts *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions,
) (http.Header, *sdk.ScriptsSearchResultV1, error) {
	f.listCalls++
	if f.listFn == nil {
		return nil, nil, nil
	}
	res, err := f.listFn(og, opts)
	return nil, res, err
}

func newTestResource(a *fakeAssignmentService, s *fakeScriptService) *scriptAssignmentResource {
	return &scriptAssignmentResource{
		client:                     &sdk.Client{},
		newScriptAssignmentService: func(*sdk.Client) scriptAssignmentServiceAPI { return a },
		newScriptService:           func(*sdk.Client) scriptServiceAPI { return s },
	}
}

func testSchemaResponse(t *testing.T) *resource.SchemaResponse {
	t.Helper()
	resp := &resource.SchemaResponse{}
	(&scriptAssignmentResource{}).Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema: %v", resp.Diagnostics)
	}
	return resp
}

// seededState builds state holding id and script_uuid, plus
// organization_group_uuid when orgGroupUUID is non-empty.
func seededState(t *testing.T, orgGroupUUID string) tfsdk.State {
	t.Helper()
	ctx := context.Background()
	sch := testSchemaResponse(t).Schema
	state := tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	diags := state.SetAttribute(ctx, path.Root("id"), testScriptUUID)
	diags.Append(state.SetAttribute(ctx, path.Root("script_uuid"), testScriptUUID)...)
	if orgGroupUUID != "" {
		diags.Append(state.SetAttribute(ctx, path.Root("organization_group_uuid"), orgGroupUUID)...)
	}
	if diags.HasError() {
		t.Fatalf("failed to seed state: %v", diags)
	}
	return state
}

func runRead(t *testing.T, r *scriptAssignmentResource, orgGroupUUID string) *resource.ReadResponse {
	t.Helper()
	state := seededState(t, orgGroupUUID)
	resp := &resource.ReadResponse{State: state}
	r.Read(context.Background(), resource.ReadRequest{State: state}, resp)
	return resp
}

func runDelete(t *testing.T, r *scriptAssignmentResource, orgGroupUUID string) *resource.DeleteResponse {
	t.Helper()
	state := seededState(t, orgGroupUUID)
	resp := &resource.DeleteResponse{State: state}
	r.Delete(context.Background(), resource.DeleteRequest{State: state}, resp)
	return resp
}

func stateOrgGroup(t *testing.T, state tfsdk.State) *string {
	t.Helper()
	var v *string
	if diags := state.GetAttribute(context.Background(), path.Root("organization_group_uuid"), &v); diags.HasError() {
		t.Fatalf("get organization_group_uuid: %v", diags)
	}
	return v
}
