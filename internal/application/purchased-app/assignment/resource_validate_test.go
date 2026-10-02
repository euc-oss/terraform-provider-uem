package assignment

import (
	"context"
	"math/big"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/purchased-app/assignment/models"
)

const licenseUsageGuidance = "use vpp_app_details.license_usage"

// validateAssignmentSpec describes one assignment in a validation test config.
// A nil smartGroups leaves distribution.smart_groups unset.
type validateAssignmentSpec struct {
	smartGroups   []string
	licenseUsage  bool
	effectiveDate string
	// effectiveDateEmptyString requests an explicit types.StringValue("")
	// for distribution.effective_date, distinct from leaving it unset
	// (types.StringNull()). effectiveDate is ignored when this is set.
	effectiveDateEmptyString bool
	// desiredStateManagement, when true, sets restriction.desired_state_management
	// = true (internal-task minor 1: never-honoured check moved from ModifyPlan to
	// ValidateConfig).
	desiredStateManagement bool
	// isDynamicTemplateSaved, when true, sets is_dynamic_template_saved = true
	// (internal-task minor 1, same as above).
	isDynamicTemplateSaved bool
}

// validationConfig builds a resource config with one assignment per spec.
func validationConfig(t *testing.T, specs ...validateAssignmentSpec) tfsdk.Config {
	t.Helper()
	ctx := context.Background()

	empty := emptyPurchasedAssignmentState(t)
	plan := tfsdk.Plan{Schema: empty.Schema, Raw: empty.Raw.Copy()}

	listType, ok := plan.Schema.GetAttributes()["assignments"].GetType().(types.ListType)
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

	models := make([]tf.PurchasedAppAssignmentModel, 0, len(specs))
	for i, spec := range specs {
		smartGroups := types.ListNull(types.StringType)
		if spec.smartGroups != nil {
			elems := make([]attr.Value, len(spec.smartGroups))
			for j, g := range spec.smartGroups {
				elems[j] = types.StringValue(g)
			}
			smartGroups = types.ListValueMust(types.StringType, elems)
		}
		usage := types.ListNull(usageElemType)
		if spec.licenseUsage {
			usage = types.ListValueMust(usageElemType, []attr.Value{
				types.ObjectValueMust(usageElemType.AttrTypes, map[string]attr.Value{
					"smart_group_uuid": types.StringValue("cd9f26cd-b1a2-f80e-5961-be6b3839fbd7"),
					"allocated":        types.Int64Value(1),
					"redeemed":         types.Int64Null(),
				}),
			})
		}
		effectiveDate := types.StringNull()
		if spec.effectiveDateEmptyString {
			effectiveDate = types.StringValue("")
		} else if spec.effectiveDate != "" {
			effectiveDate = types.StringValue(spec.effectiveDate)
		}
		var restriction *tf.PurchasedAppAssignmentRestrictionModel
		if spec.desiredStateManagement {
			restriction = &tf.PurchasedAppAssignmentRestrictionModel{
				DesiredStateManagement: types.BoolValue(true),
			}
		}
		isDynamicTemplateSaved := types.BoolNull()
		if spec.isDynamicTemplateSaved {
			isDynamicTemplateSaved = types.BoolValue(true)
		}
		models = append(models, tf.PurchasedAppAssignmentModel{
			Priority: types.Int64Value(int64(i)),
			Distribution: tf.PurchasedAppAssignmentDistributionModel{
				Name:              types.StringValue("VPP"),
				Description:       types.StringNull(),
				SmartGroups:       smartGroups,
				AppDeliveryMethod: types.StringNull(),
				EffectiveDate:     effectiveDate,
				VppAppDetails:     tf.VppAppDetailsModel{LicenseUsage: usage},
			},
			Restriction:              restriction,
			ApplicationConfiguration: types.ListNull(appConfigListType.ElemType),
			ApplicationAttributes:    types.ListNull(appConfigListType.ElemType),
			IsDynamicTemplateSaved:   isDynamicTemplateSaved,
		})
	}
	assignments, diags := types.ListValueFrom(ctx, elemType, models)
	if diags.HasError() {
		t.Fatalf("assignments: %v", diags)
	}

	model := tf.PurchasedAppAssignmentRuleModel{
		ID:                  types.StringNull(),
		ApplicationUUID:     types.StringValue("596b30c4-5fd8-f8a4-2f40-553c312b9f1a"),
		ExcludedSmartGroups: types.ListNull(types.StringType),
		Assignments:         assignments,
	}
	if diags := plan.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set config: %v", diags)
	}
	return tfsdk.Config(plan)
}

func runValidateConfig(t *testing.T, config tfsdk.Config) *resource.ValidateConfigResponse {
	t.Helper()
	r := &purchasedApplicationAssignmentResource{}
	resp := &resource.ValidateConfigResponse{}
	r.ValidateConfig(context.Background(), resource.ValidateConfigRequest{Config: config}, resp)
	return resp
}

// TestValidateConfigRejectsDistributionSmartGroups proves a non-empty
// distribution.smart_groups is rejected at plan time with guidance to use
// license_usage, attributed to the offending assignment only.
func TestValidateConfigRejectsDistributionSmartGroups(t *testing.T) {
	config := validationConfig(t,
		validateAssignmentSpec{licenseUsage: true},
		validateAssignmentSpec{smartGroups: []string{"cd9f26cd-b1a2-f80e-5961-be6b3839fbd7"}, licenseUsage: true},
	)
	resp := runValidateConfig(t, config)

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 {
		t.Fatalf("expected exactly one error, got %d: %v", len(errs), resp.Diagnostics)
	}
	if !strings.Contains(errs[0].Detail(), licenseUsageGuidance) {
		t.Fatalf("error detail %q does not contain %q", errs[0].Detail(), licenseUsageGuidance)
	}
	if !strings.Contains(errs[0].Detail(), "distribution.smart_groups") {
		t.Fatalf("error detail %q does not name distribution.smart_groups", errs[0].Detail())
	}
	withPath, ok := errs[0].(diag.DiagnosticWithPath)
	if !ok {
		t.Fatalf("error is not attribute-scoped: %v", errs[0])
	}
	if got := withPath.Path().String(); got != "assignments[1].distribution.smart_groups" {
		t.Fatalf("error path = %q, want assignments[1].distribution.smart_groups", got)
	}
}

// TestValidateConfigAllowsEmptyOrUnsetSmartGroups proves only a non-empty list
// trips the check: unset and empty smart_groups both pass.
func TestValidateConfigAllowsEmptyOrUnsetSmartGroups(t *testing.T) {
	for name, spec := range map[string]validateAssignmentSpec{
		"unset": {licenseUsage: true},
		"empty": {smartGroups: []string{}, licenseUsage: true},
	} {
		t.Run(name, func(t *testing.T) {
			resp := runValidateConfig(t, validationConfig(t, spec))
			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected error: %v", resp.Diagnostics)
			}
		})
	}
}

// TestValidateConfigAcceptsLicenseUsageOnly proves the supported shape
// (groups only under vpp_app_details.license_usage) validates cleanly.
func TestValidateConfigAcceptsLicenseUsageOnly(t *testing.T) {
	resp := runValidateConfig(t, validationConfig(t,
		validateAssignmentSpec{licenseUsage: true},
		validateAssignmentSpec{licenseUsage: true},
	))
	if len(resp.Diagnostics) != 0 {
		t.Fatalf("expected no diagnostics, got %v", resp.Diagnostics)
	}
}

// TestValidateConfigSkipsUnknownAssignments proves an unknown assignments list
// (for example one derived from a resource not yet created) is not rejected.
func TestValidateConfigSkipsUnknownAssignments(t *testing.T) {
	config := validationConfig(t, validateAssignmentSpec{licenseUsage: true})
	objType, ok := config.Raw.Type().(tftypes.Object)
	if !ok {
		t.Fatal("config type is not an object")
	}
	values := map[string]tftypes.Value{}
	if err := config.Raw.As(&values); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	values["assignments"] = tftypes.NewValue(objType.AttributeTypes["assignments"], tftypes.UnknownValue)
	config.Raw = tftypes.NewValue(objType, values)

	resp := runValidateConfig(t, config)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
}

// TestValidateConfigRejectsDistributionEffectiveDate proves a set
// distribution.effective_date is rejected at plan time on a VPP assignment
// (internal-task item 4): UEM ignores effective_date for VPP, so honoring a
// configured value would silently do nothing.
func TestValidateConfigRejectsDistributionEffectiveDate(t *testing.T) {
	config := validationConfig(t,
		validateAssignmentSpec{licenseUsage: true},
		validateAssignmentSpec{licenseUsage: true, effectiveDate: "2026-04-23T00:00:00Z"},
	)
	resp := runValidateConfig(t, config)

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 {
		t.Fatalf("expected exactly one error, got %d: %v", len(errs), resp.Diagnostics)
	}
	if !strings.Contains(errs[0].Summary(), "effective_date") {
		t.Fatalf("error summary %q does not name effective_date", errs[0].Summary())
	}
	withPath, ok := errs[0].(diag.DiagnosticWithPath)
	if !ok {
		t.Fatalf("error is not attribute-scoped: %v", errs[0])
	}
	if got := withPath.Path().String(); got != "assignments[1].distribution.effective_date" {
		t.Fatalf("error path = %q, want assignments[1].distribution.effective_date", got)
	}
}

// TestValidateConfigAllowsNullOrEmptyEffectiveDate proves an unset
// effective_date passes validation (only a null/unknown value is allowed).
func TestValidateConfigAllowsNullOrEmptyEffectiveDate(t *testing.T) {
	for name, spec := range map[string]validateAssignmentSpec{
		"unset": {licenseUsage: true},
	} {
		t.Run(name, func(t *testing.T) {
			resp := runValidateConfig(t, validationConfig(t, spec))
			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected error: %v", resp.Diagnostics)
			}
		})
	}
}

// TestValidateConfigRejectsExplicitEmptyEffectiveDate proves an explicit
// empty-string distribution.effective_date is rejected just like a
// meaningfully-set date (internal-task item 5): UEM would silently ignore either
// one for a VPP assignment.
func TestValidateConfigRejectsExplicitEmptyEffectiveDate(t *testing.T) {
	config := validationConfig(t,
		validateAssignmentSpec{licenseUsage: true},
		validateAssignmentSpec{licenseUsage: true, effectiveDateEmptyString: true},
	)
	resp := runValidateConfig(t, config)

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 {
		t.Fatalf("expected exactly one error, got %d: %v", len(errs), resp.Diagnostics)
	}
	if !strings.Contains(errs[0].Summary(), "effective_date") {
		t.Fatalf("error summary %q does not name effective_date", errs[0].Summary())
	}
	withPath, ok := errs[0].(diag.DiagnosticWithPath)
	if !ok {
		t.Fatalf("error is not attribute-scoped: %v", errs[0])
	}
	if got := withPath.Path().String(); got != "assignments[1].distribution.effective_date" {
		t.Fatalf("error path = %q, want assignments[1].distribution.effective_date", got)
	}
}

// TestValidateConfigValidPrioritiesNoDiags proves a config whose priorities
// are already 0..n-1 in order produces no diagnostics (internal-task).
func TestValidateConfigValidPrioritiesNoDiags(t *testing.T) {
	resp := runValidateConfig(t, validationConfig(t,
		validateAssignmentSpec{licenseUsage: true},
		validateAssignmentSpec{licenseUsage: true},
		validateAssignmentSpec{licenseUsage: true},
	))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
}

// TestValidateConfigPriorityGapReportsAtOffendingIndex proves a priority gap
// is reported as a (a)-rule error at the offending index (internal-task).
// Note: validationConfig assigns priorities 0..n-1 in list order by
// default, so the config priorities are overwritten directly to force
// [0, 2].
func TestValidateConfigPriorityGapReportsAtOffendingIndex(t *testing.T) {
	config := validationConfig(t,
		validateAssignmentSpec{licenseUsage: true},
		validateAssignmentSpec{licenseUsage: true},
	)
	config = setPurchasedAssignmentPriorities(t, config, 0, 2)
	resp := runValidateConfig(t, config)

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 {
		t.Fatalf("expected exactly one error, got %d: %v", len(errs), resp.Diagnostics)
	}
	withPath, ok := errs[0].(diag.DiagnosticWithPath)
	if !ok {
		t.Fatalf("error is not attribute-scoped: %v", errs[0])
	}
	if got := withPath.Path().String(); got != "assignments[1].priority" {
		t.Fatalf("error path = %q, want assignments[1].priority", got)
	}
	if !strings.Contains(errs[0].Detail(), "UEM requires the priorities") {
		t.Fatalf("error detail %q does not describe the (a) rule", errs[0].Detail())
	}
}

// TestValidateConfigPriorityOutOfOrderReportsAtFirstMismatch proves a
// valid-but-out-of-order priority set ([1, 0]) is reported as a (b)-rule
// error at the first index where priority != position (internal-task).
func TestValidateConfigPriorityOutOfOrderReportsAtFirstMismatch(t *testing.T) {
	config := validationConfig(t,
		validateAssignmentSpec{licenseUsage: true},
		validateAssignmentSpec{licenseUsage: true},
	)
	config = setPurchasedAssignmentPriorities(t, config, 1, 0)
	resp := runValidateConfig(t, config)

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 {
		t.Fatalf("expected exactly one error, got %d: %v", len(errs), resp.Diagnostics)
	}
	withPath, ok := errs[0].(diag.DiagnosticWithPath)
	if !ok {
		t.Fatalf("error is not attribute-scoped: %v", errs[0])
	}
	if got := withPath.Path().String(); got != "assignments[0].priority" {
		t.Fatalf("error path = %q, want assignments[0].priority", got)
	}
	if !strings.Contains(errs[0].Detail(), "listed at position") {
		t.Fatalf("error detail %q does not describe the (b) rule", errs[0].Detail())
	}
}

// TestValidateConfigUnknownPrioritySkipsCheck proves an unknown priority
// (fed by a not-yet-known variable) suppresses the whole priority check
// rather than failing (internal-task).
func TestValidateConfigUnknownPrioritySkipsCheck(t *testing.T) {
	config := validationConfig(t,
		validateAssignmentSpec{licenseUsage: true},
		validateAssignmentSpec{licenseUsage: true},
	)
	config = setPurchasedAssignmentPriorities(t, config, 0, 2)
	config = setPurchasedAssignmentPriorityUnknown(t, config, 1)
	resp := runValidateConfig(t, config)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
}

// setPurchasedAssignmentPriorities overwrites the priority of each
// assignment element in config, in list order.
func setPurchasedAssignmentPriorities(t *testing.T, config tfsdk.Config, priorities ...int64) tfsdk.Config {
	t.Helper()
	objType, ok := config.Raw.Type().(tftypes.Object)
	if !ok {
		t.Fatal("config type is not an object")
	}
	rootValues := map[string]tftypes.Value{}
	if err := config.Raw.As(&rootValues); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	assignmentsListType, ok := objType.AttributeTypes["assignments"].(tftypes.List)
	if !ok {
		t.Fatal("assignments attribute is not a list type")
	}
	var elements []tftypes.Value
	if err := rootValues["assignments"].As(&elements); err != nil {
		t.Fatalf("decode assignments: %v", err)
	}
	elemObjType, ok := assignmentsListType.ElementType.(tftypes.Object)
	if !ok {
		t.Fatal("assignments element is not an object type")
	}
	if len(priorities) != len(elements) {
		t.Fatalf("priorities length %d != assignments length %d", len(priorities), len(elements))
	}
	for i, p := range priorities {
		elemValues := map[string]tftypes.Value{}
		if err := elements[i].As(&elemValues); err != nil {
			t.Fatalf("decode assignment[%d]: %v", i, err)
		}
		elemValues["priority"] = tftypes.NewValue(elemObjType.AttributeTypes["priority"], big.NewFloat(float64(p)))
		elements[i] = tftypes.NewValue(elemObjType, elemValues)
	}
	rootValues["assignments"] = tftypes.NewValue(assignmentsListType, elements)
	config.Raw = tftypes.NewValue(objType, rootValues)
	return config
}

// setPurchasedAssignmentPriorityUnknown marks the priority of the assignment
// at index i as unknown.
func setPurchasedAssignmentPriorityUnknown(t *testing.T, config tfsdk.Config, index int) tfsdk.Config {
	t.Helper()
	objType, ok := config.Raw.Type().(tftypes.Object)
	if !ok {
		t.Fatal("config type is not an object")
	}
	rootValues := map[string]tftypes.Value{}
	if err := config.Raw.As(&rootValues); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	assignmentsListType, ok := objType.AttributeTypes["assignments"].(tftypes.List)
	if !ok {
		t.Fatal("assignments attribute is not a list type")
	}
	var elements []tftypes.Value
	if err := rootValues["assignments"].As(&elements); err != nil {
		t.Fatalf("decode assignments: %v", err)
	}
	elemObjType, ok := assignmentsListType.ElementType.(tftypes.Object)
	if !ok {
		t.Fatal("assignments element is not an object type")
	}
	elemValues := map[string]tftypes.Value{}
	if err := elements[index].As(&elemValues); err != nil {
		t.Fatalf("decode assignment[%d]: %v", index, err)
	}
	elemValues["priority"] = tftypes.NewValue(elemObjType.AttributeTypes["priority"], tftypes.UnknownValue)
	elements[index] = tftypes.NewValue(elemObjType, elemValues)
	rootValues["assignments"] = tftypes.NewValue(assignmentsListType, elements)
	config.Raw = tftypes.NewValue(objType, rootValues)
	return config
}

// TestValidateConfigWiredThroughProviderServer proves the framework actually
// invokes ValidateConfig for this resource via ValidateResourceConfig.
func TestValidateConfigWiredThroughProviderServer(t *testing.T) {
	server, err := providerserver.NewProtocol6WithError(&restrictionTestProvider{r: &purchasedApplicationAssignmentResource{}})()
	if err != nil {
		t.Fatalf("NewProtocol6WithError: %v", err)
	}
	config := validationConfig(t, validateAssignmentSpec{smartGroups: []string{"cd9f26cd-b1a2-f80e-5961-be6b3839fbd7"}})
	dv, err := tfprotov6.NewDynamicValue(config.Raw.Type(), config.Raw)
	if err != nil {
		t.Fatalf("NewDynamicValue: %v", err)
	}
	resp, err := server.ValidateResourceConfig(context.Background(), &tfprotov6.ValidateResourceConfigRequest{
		TypeName: "uem_purchased_application_assignment",
		Config:   &dv,
	})
	if err != nil {
		t.Fatalf("ValidateResourceConfig: %v", err)
	}
	for _, d := range resp.Diagnostics {
		if d.Severity == tfprotov6.DiagnosticSeverityError && strings.Contains(d.Detail, licenseUsageGuidance) {
			return
		}
	}
	t.Fatalf("expected an error diagnostic containing %q, got %+v", licenseUsageGuidance, resp.Diagnostics)
}

// TestValidateConfigDesiredStateManagementTrue_Errors proves
// restriction.desired_state_management = true is rejected at ValidateConfig
// time, with no lookup possible (ValidateConfig never performs one) — internal-task minor 1: this check moved here from ModifyPlan.
func TestValidateConfigDesiredStateManagementTrue_Errors(t *testing.T) {
	config := validationConfig(t, validateAssignmentSpec{licenseUsage: true, desiredStateManagement: true})
	resp := runValidateConfig(t, config)

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(errs), resp.Diagnostics)
	}
	if errs[0].Summary() != neverHonouredSummary {
		t.Fatalf("unexpected summary: %q", errs[0].Summary())
	}
	withPath, ok := errs[0].(diag.DiagnosticWithPath)
	if !ok {
		t.Fatalf("error is not attribute-scoped: %v", errs[0])
	}
	if got := withPath.Path().String(); got != "assignments[0].restriction.desired_state_management" {
		t.Fatalf("error path = %q, want assignments[0].restriction.desired_state_management", got)
	}
}

// TestValidateConfigIsDynamicTemplateSavedTrue_Errors proves
// is_dynamic_template_saved = true is rejected at ValidateConfig time (internal-task minor 1: moved here from ModifyPlan).
func TestValidateConfigIsDynamicTemplateSavedTrue_Errors(t *testing.T) {
	config := validationConfig(t, validateAssignmentSpec{licenseUsage: true, isDynamicTemplateSaved: true})
	resp := runValidateConfig(t, config)

	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(errs), resp.Diagnostics)
	}
	if errs[0].Summary() != neverHonouredSummary {
		t.Fatalf("unexpected summary: %q", errs[0].Summary())
	}
	withPath, ok := errs[0].(diag.DiagnosticWithPath)
	if !ok {
		t.Fatalf("error is not attribute-scoped: %v", errs[0])
	}
	if got := withPath.Path().String(); got != "assignments[0].is_dynamic_template_saved" {
		t.Fatalf("error path = %q, want assignments[0].is_dynamic_template_saved", got)
	}
}

// TestValidateConfigDesiredStateManagementUnknown_SkipsCheck proves an
// unknown restriction object suppresses the desired_state_management check
// rather than failing (mirrors TestValidateConfigUnknownPrioritySkipsCheck).
func TestValidateConfigDesiredStateManagementUnknown_SkipsCheck(t *testing.T) {
	config := validationConfig(t, validateAssignmentSpec{licenseUsage: true})
	config = setPurchasedAssignmentRestrictionUnknown(t, config, 0)
	resp := runValidateConfig(t, config)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
}

// setPurchasedAssignmentRestrictionUnknown marks the restriction object of
// the assignment at index i as unknown.
func setPurchasedAssignmentRestrictionUnknown(t *testing.T, config tfsdk.Config, index int) tfsdk.Config {
	t.Helper()
	objType, ok := config.Raw.Type().(tftypes.Object)
	if !ok {
		t.Fatal("config type is not an object")
	}
	rootValues := map[string]tftypes.Value{}
	if err := config.Raw.As(&rootValues); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	assignmentsListType, ok := objType.AttributeTypes["assignments"].(tftypes.List)
	if !ok {
		t.Fatal("assignments attribute is not a list type")
	}
	var elements []tftypes.Value
	if err := rootValues["assignments"].As(&elements); err != nil {
		t.Fatalf("decode assignments: %v", err)
	}
	elemObjType, ok := assignmentsListType.ElementType.(tftypes.Object)
	if !ok {
		t.Fatal("assignments element is not an object type")
	}
	elemValues := map[string]tftypes.Value{}
	if err := elements[index].As(&elemValues); err != nil {
		t.Fatalf("decode assignment[%d]: %v", index, err)
	}
	elemValues["restriction"] = tftypes.NewValue(elemObjType.AttributeTypes["restriction"], tftypes.UnknownValue)
	elements[index] = tftypes.NewValue(elemObjType, elemValues)
	rootValues["assignments"] = tftypes.NewValue(assignmentsListType, elements)
	config.Raw = tftypes.NewValue(objType, rootValues)
	return config
}
