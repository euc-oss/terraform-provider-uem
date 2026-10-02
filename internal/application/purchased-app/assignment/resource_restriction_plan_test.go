package assignment

import (
	"context"
	"net/http"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/purchased-app/assignment/models"
)

// restrictionTestProvider exposes only the purchased assignment resource so
// PlanResourceChange / ApplyResourceChange run through the real framework
// server, including schema default handling.
type restrictionTestProvider struct {
	r *purchasedApplicationAssignmentResource
}

func (p *restrictionTestProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "uem"
}

func (p *restrictionTestProvider) Schema(context.Context, provider.SchemaRequest, *provider.SchemaResponse) {
}

func (p *restrictionTestProvider) Configure(context.Context, provider.ConfigureRequest, *provider.ConfigureResponse) {
}

func (p *restrictionTestProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{func() resource.Resource { return p.r }}
}

func (p *restrictionTestProvider) DataSources(context.Context) []func() datasource.DataSource {
	return nil
}

// serverDefaultingAssignmentService echoes the last PUT body on GET the way
// UEM does: every restriction flag the request omitted comes back false.
type serverDefaultingAssignmentService struct {
	last *sdk.AppAssignmentRuleV2Model
}

func (s *serverDefaultingAssignmentService) GetAssignmentRuleAsync(context.Context, string) (http.Header, *sdk.AppAssignmentRuleV2Model, error) {
	if s.last == nil {
		return nil, nil, nil
	}
	out := *s.last
	out.Assignments = make([]sdk.AppAssignmentV2Model, len(s.last.Assignments))
	for i, a := range s.last.Assignments {
		r := sdk.AppAssignmentRestrictionV1ModelV2{}
		if a.Restriction != nil {
			r = *a.Restriction
		}
		for _, p := range []**bool{
			&r.RemoveOnUnenroll, &r.PreventRemoval, &r.PreventApplicationBackup,
			&r.MakeAppMdmManaged, &r.ManagedAccess, &r.DesiredStateManagement,
		} {
			if *p == nil {
				f := false
				*p = &f
			}
		}
		a.Restriction = &r
		out.Assignments[i] = a
	}
	return nil, &out, nil
}

func (s *serverDefaultingAssignmentService) UpdateAssignmentRuleAsync(_ context.Context, _ string, request *sdk.AppAssignmentRuleV2Model) (http.Header, error) {
	s.last = request
	return nil, nil
}

// partialRestrictionConfig builds a resource config with one assignment whose
// restriction sets only remove_on_unenroll.
func partialRestrictionConfig(t *testing.T, s tfsdk.Plan) tftypes.Value {
	t.Helper()
	ctx := context.Background()

	listType, ok := s.Schema.GetAttributes()["assignments"].GetType().(types.ListType)
	if !ok {
		t.Fatal("assignments is not a list type")
	}
	elemType, ok := listType.ElemType.(types.ObjectType)
	if !ok {
		t.Fatal("assignments element is not an object type")
	}
	distType, ok := elemType.AttrTypes["distribution"].(types.ObjectType)
	if !ok {
		t.Fatal("distribution is not an object type")
	}
	vppType, ok := distType.AttrTypes["vpp_app_details"].(types.ObjectType)
	if !ok {
		t.Fatal("vpp_app_details is not an object type")
	}
	usageListType, ok := vppType.AttrTypes["license_usage"].(types.ListType)
	if !ok {
		t.Fatal("license_usage is not a list type")
	}
	usageElemType, ok := usageListType.ElemType.(types.ObjectType)
	if !ok {
		t.Fatal("license_usage element is not an object type")
	}
	appConfigListType, ok := elemType.AttrTypes["application_configuration"].(types.ListType)
	if !ok {
		t.Fatal("application_configuration is not a list type")
	}

	usage, diags := types.ListValue(usageElemType, []attr.Value{
		types.ObjectValueMust(usageElemType.AttrTypes, map[string]attr.Value{
			"smart_group_uuid": types.StringValue("cd9f26cd-b1a2-f80e-5961-be6b3839fbd7"),
			"allocated":        types.Int64Value(1),
			"redeemed":         types.Int64Null(),
		}),
	})
	if diags.HasError() {
		t.Fatalf("license_usage: %v", diags)
	}

	a := tf.PurchasedAppAssignmentModel{
		Priority: types.Int64Value(0),
		Distribution: tf.PurchasedAppAssignmentDistributionModel{
			Name:              types.StringValue("VPP"),
			Description:       types.StringNull(),
			SmartGroups:       types.ListNull(types.StringType),
			AppDeliveryMethod: types.StringNull(),
			EffectiveDate:     types.StringNull(),
			VppAppDetails:     tf.VppAppDetailsModel{LicenseUsage: usage},
		},
		Restriction: &tf.PurchasedAppAssignmentRestrictionModel{
			RemoveOnUnenroll:         types.BoolValue(true),
			PreventRemoval:           types.BoolNull(),
			PreventApplicationBackup: types.BoolNull(),
			MakeAppMdmManaged:        types.BoolNull(),
			ManagedAccess:            types.BoolNull(),
			DesiredStateManagement:   types.BoolNull(),
		},
		ApplicationConfiguration: types.ListNull(appConfigListType.ElemType),
		ApplicationAttributes:    types.ListNull(appConfigListType.ElemType),
		IsDynamicTemplateSaved:   types.BoolNull(),
	}
	assignments, diags := types.ListValueFrom(ctx, elemType, []tf.PurchasedAppAssignmentModel{a})
	if diags.HasError() {
		t.Fatalf("assignments: %v", diags)
	}

	model := tf.PurchasedAppAssignmentRuleModel{
		ID:                  types.StringNull(),
		ApplicationUUID:     types.StringValue("596b30c4-5fd8-f8a4-2f40-553c312b9f1a"),
		ExcludedSmartGroups: types.ListNull(types.StringType),
		Assignments:         assignments,
	}
	if diags := s.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set config: %v", diags)
	}
	return s.Raw
}

// restrictionLeaves decodes assignments[0].restriction from a resource object.
func restrictionLeaves(t *testing.T, v tftypes.Value) map[string]tftypes.Value {
	t.Helper()
	var root map[string]tftypes.Value
	if err := v.As(&root); err != nil {
		t.Fatalf("root: %v", err)
	}
	var list []tftypes.Value
	if err := root["assignments"].As(&list); err != nil || len(list) != 1 {
		t.Fatalf("assignments: %v (len %d)", err, len(list))
	}
	var assignment map[string]tftypes.Value
	if err := list[0].As(&assignment); err != nil {
		t.Fatalf("assignment: %v", err)
	}
	var restriction map[string]tftypes.Value
	if err := assignment["restriction"].As(&restriction); err != nil {
		t.Fatalf("restriction: %v", err)
	}
	return restriction
}

// TestPurchasedAssignmentPartialRestrictionPlansDefaults covers a restriction
// block that sets only remove_on_unenroll. The omitted prevent_removal and
// managed_access must plan as false, and the applied state (from a server that
// echoes omitted flags as false) must match the plan for all three, so
// Terraform does not report an inconsistent result on Create.
func TestPurchasedAssignmentPartialRestrictionPlansDefaults(t *testing.T) {
	ctx := context.Background()
	svc := &serverDefaultingAssignmentService{}
	r := &purchasedApplicationAssignmentResource{
		client: &sdk.Client{},
		newPurchasedAppAssignmentService: func(*sdk.Client) purchasedAppAssignmentServiceAPI {
			return svc
		},
	}
	server, err := providerserver.NewProtocol6WithError(&restrictionTestProvider{r: r})()
	if err != nil {
		t.Fatalf("NewProtocol6WithError: %v", err)
	}

	empty := emptyPurchasedAssignmentState(t)
	config := partialRestrictionConfig(t, tfsdk.Plan{Schema: empty.Schema, Raw: empty.Raw.Copy()})
	prior := tftypes.NewValue(config.Type(), nil)

	dv := func(v tftypes.Value) *tfprotov6.DynamicValue {
		d, err := tfprotov6.NewDynamicValue(v.Type(), v)
		if err != nil {
			t.Fatalf("NewDynamicValue: %v", err)
		}
		return &d
	}
	failOnError := func(what string, diags []*tfprotov6.Diagnostic) {
		for _, d := range diags {
			if d.Severity == tfprotov6.DiagnosticSeverityError {
				t.Fatalf("%s error diagnostic: %s: %s", what, d.Summary, d.Detail)
			}
		}
	}

	planResp, err := server.PlanResourceChange(ctx, &tfprotov6.PlanResourceChangeRequest{
		TypeName:         "uem_purchased_application_assignment",
		PriorState:       dv(prior),
		ProposedNewState: dv(config),
		Config:           dv(config),
	})
	if err != nil {
		t.Fatalf("PlanResourceChange: %v", err)
	}
	failOnError("plan", planResp.Diagnostics)
	planned, err := planResp.PlannedState.Unmarshal(config.Type())
	if err != nil {
		t.Fatalf("unmarshal planned: %v", err)
	}

	applyResp, err := server.ApplyResourceChange(ctx, &tfprotov6.ApplyResourceChangeRequest{
		TypeName:     "uem_purchased_application_assignment",
		PriorState:   dv(prior),
		PlannedState: planResp.PlannedState,
		Config:       dv(config),
	})
	if err != nil {
		t.Fatalf("ApplyResourceChange: %v", err)
	}
	failOnError("apply", applyResp.Diagnostics)
	applied, err := applyResp.NewState.Unmarshal(config.Type())
	if err != nil {
		t.Fatalf("unmarshal applied: %v", err)
	}

	want := map[string]bool{
		"remove_on_unenroll": true,
		"prevent_removal":    false,
		"managed_access":     false,
	}
	plannedLeaves := restrictionLeaves(t, planned)
	appliedLeaves := restrictionLeaves(t, applied)
	for name, wantVal := range want {
		expected := tftypes.NewValue(tftypes.Bool, wantVal)
		if !plannedLeaves[name].Equal(expected) {
			t.Errorf("planned restriction.%s = %s, want %t", name, plannedLeaves[name], wantVal)
		}
		if !appliedLeaves[name].Equal(plannedLeaves[name]) {
			t.Errorf("inconsistent result: restriction.%s planned %s, applied %s",
				name, plannedLeaves[name], appliedLeaves[name])
		}
	}
}
