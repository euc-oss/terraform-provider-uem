package macapplication

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestMacMetadata(t *testing.T) {
	r := &macapplicationResource{}
	var resp resource.MetadataResponse
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "uem"}, &resp)

	if resp.TypeName != "uem_mac_application" {
		t.Fatalf("unexpected type name: %q", resp.TypeName)
	}
}

func TestMacSchemaContainsCoreAttributes(t *testing.T) {
	r := &macapplicationResource{}
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)

	attrs := resp.Schema.Attributes
	for _, key := range []string{"id", "uuid", "org_group_id", "dmg_file_path", "plist_file_path", "app_version"} {
		if _, ok := attrs[key]; !ok {
			t.Fatalf("missing schema attribute %q", key)
		}
	}
}

func TestSchema_ContentAttributes(t *testing.T) {
	r := &macapplicationResource{}
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)

	attrs := resp.Schema.Attributes

	for _, key := range []string{"dmg_file_path", "plist_file_path", "app_version"} {
		attr, ok := attrs[key]
		if !ok {
			t.Fatalf("missing schema attribute %q", key)
		}
		if attr.IsRequired() {
			t.Errorf("%s: expected Required == false", key)
		}
		if !attr.IsOptional() {
			t.Errorf("%s: expected Optional == true", key)
		}
		if !attr.IsComputed() {
			t.Errorf("%s: expected Computed == true", key)
		}
	}

	includeContent, ok := attrs["include_content"]
	if !ok {
		t.Fatalf("missing schema attribute %q", "include_content")
	}
	if _, ok := includeContent.(schema.BoolAttribute); !ok {
		t.Fatalf("include_content: expected schema.BoolAttribute, got %T", includeContent)
	}
	if !includeContent.IsOptional() {
		t.Errorf("include_content: expected Optional == true")
	}

	for _, key := range []string{"dmg_content_base64", "plist_content_base64", "dmg_file_sha256"} {
		attr, ok := attrs[key]
		if !ok {
			t.Fatalf("missing schema attribute %q", key)
		}
		if !attr.IsComputed() {
			t.Errorf("%s: expected Computed == true", key)
		}
	}
}

func TestMacServicesNilClient(t *testing.T) {
	r := &macapplicationResource{}
	if _, err := r.BlobV1ResourceService(context.Background()); err == nil || !strings.Contains(err.Error(), "SDK client is nil") {
		t.Fatalf("unexpected blob v1 service error: %v", err)
	}
	if _, err := r.BlobV2ResourceService(context.Background()); err == nil || !strings.Contains(err.Error(), "SDK client is nil") {
		t.Fatalf("unexpected blob v2 service error: %v", err)
	}
	if _, err := r.MacAppService(context.Background()); err == nil || !strings.Contains(err.Error(), "SDK client is nil") {
		t.Fatalf("unexpected mac app service error: %v", err)
	}
	if _, err := r.InternalAppService(context.Background()); err == nil || !strings.Contains(err.Error(), "SDK client is nil") {
		t.Fatalf("unexpected internal app service error: %v", err)
	}
}

// TestOrgGroupIDPlanModifiers_TypePresence proves org_group_id carries BOTH
// a UseStateForUnknown-shaped modifier and a requiresReplaceIfModifier-shaped
// modifier (the framework's unexported type backing both RequiresReplace()
// and RequiresReplaceIf()). This is a type-presence check only — it does not
// exercise an actual plan diff; see the behavioral tests below for that.
func TestOrgGroupIDPlanModifiers_TypePresence(t *testing.T) {
	r := &macapplicationResource{}
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)

	attr, ok := resp.Schema.Attributes["org_group_id"]
	if !ok {
		t.Fatal("missing schema attribute \"org_group_id\"")
	}

	int32Attr, ok := attr.(schema.Int32Attribute)
	if !ok {
		t.Fatalf("org_group_id is not a schema.Int32Attribute: %T", attr)
	}

	if int32Attr.Required {
		t.Errorf("org_group_id: expected Required == false")
	}
	if !int32Attr.Optional {
		t.Errorf("org_group_id: expected Optional == true")
	}
	if !int32Attr.Computed {
		t.Errorf("org_group_id: expected Computed == true")
	}

	var hasUseStateForUnknown, hasRequiresReplaceIf bool
	for _, modifier := range int32Attr.PlanModifiers {
		typeName := fmt.Sprintf("%T", modifier)
		if strings.Contains(typeName, "useStateForUnknownModifier") {
			hasUseStateForUnknown = true
		}
		if strings.Contains(typeName, "requiresReplaceIfModifier") {
			hasRequiresReplaceIf = true
		}
	}
	if !hasUseStateForUnknown {
		t.Fatalf("org_group_id PlanModifiers %v does not contain a UseStateForUnknown modifier", int32Attr.PlanModifiers)
	}
	if !hasRequiresReplaceIf {
		t.Fatalf("org_group_id PlanModifiers %v does not contain a RequiresReplaceIf modifier", int32Attr.PlanModifiers)
	}
}

// orgGroupIDPlanModifiers returns the org_group_id attribute's PlanModifiers
// slice, in schema order, for direct low-level modifier chaining in the
// behavioral tests below.
func orgGroupIDPlanModifiers(t *testing.T) []planmodifier.Int32 {
	t.Helper()
	r := &macapplicationResource{}
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)

	attr, ok := resp.Schema.Attributes["org_group_id"]
	if !ok {
		t.Fatal("missing schema attribute \"org_group_id\"")
	}
	int32Attr, ok := attr.(schema.Int32Attribute)
	if !ok {
		t.Fatalf("org_group_id is not a schema.Int32Attribute: %T", attr)
	}
	return int32Attr.PlanModifiers
}

// orgGroupIDTestSchema is a minimal single-attribute schema used to build
// tfsdk.Plan/tfsdk.State values whose Raw.IsNull() reflects overall
// create/destroy semantics, mirroring the pattern used by the framework's own
// TestRequiresReplaceIfModifierPlanModifyInt32
// (int32planmodifier/requires_replace_if_test.go).
var orgGroupIDTestSchema = schema.Schema{
	Attributes: map[string]schema.Attribute{
		"testattr": schema.Int32Attribute{},
	},
}

// nullStateRaw represents an overall-null state (resource creation: no prior
// state exists, so req.State.Raw.IsNull() is true).
func nullStateRaw(t *testing.T) tfsdk.State {
	t.Helper()
	return tfsdk.State{
		Schema: orgGroupIDTestSchema,
		Raw:    tftypes.NewValue(orgGroupIDTestSchema.Type().TerraformType(context.Background()), nil),
	}
}

func planWithValue(t *testing.T, value types.Int32) tfsdk.Plan {
	t.Helper()
	tfValue, err := value.ToTerraformValue(context.Background())
	if err != nil {
		t.Fatalf("ToTerraformValue error: %v", err)
	}
	return tfsdk.Plan{
		Schema: orgGroupIDTestSchema,
		Raw: tftypes.NewValue(
			orgGroupIDTestSchema.Type().TerraformType(context.Background()),
			map[string]tftypes.Value{"testattr": tfValue},
		),
	}
}

func stateWithValue(t *testing.T, value types.Int32) tfsdk.State {
	t.Helper()
	tfValue, err := value.ToTerraformValue(context.Background())
	if err != nil {
		t.Fatalf("ToTerraformValue error: %v", err)
	}
	return tfsdk.State{
		Schema: orgGroupIDTestSchema,
		Raw: tftypes.NewValue(
			orgGroupIDTestSchema.Type().TerraformType(context.Background()),
			map[string]tftypes.Value{"testattr": tfValue},
		),
	}
}

// chainPlanModifiers applies modifiers in schema order exactly as the
// framework applies a PlanModifiers slice: each modifier's response
// PlanValue feeds the next modifier's request PlanValue, and the last
// modifier's RequiresReplace wins (a false response never unsets a prior
// true, but neither UseStateForUnknown nor RequiresReplaceIf ever sets
// RequiresReplace back to false once true, so plain overwrite is faithful
// here for a two-modifier chain).
func chainPlanModifiers(t *testing.T, modifiers []planmodifier.Int32, req planmodifier.Int32Request) planmodifier.Int32Response {
	t.Helper()
	resp := planmodifier.Int32Response{PlanValue: req.PlanValue}
	for _, m := range modifiers {
		req.PlanValue = resp.PlanValue
		stepResp := planmodifier.Int32Response{PlanValue: req.PlanValue, RequiresReplace: resp.RequiresReplace}
		m.PlanModifyInt32(context.Background(), req, &stepResp)
		resp = stepResp
	}
	return resp
}

// TestOrgGroupID_BareUUIDImport_ProducesCleanPlan proves that a bare-uuid
// import (state has org_group_id == null; config omits the attribute so the
// incoming plan value starts unknown) resolves to a literally clean plan:
// UseStateForUnknown collapses the unknown plan value back to the prior null
// state BEFORE RequiresReplaceIf ever runs, so RequiresReplaceIf's own
// req.PlanValue.Equal(req.StateValue) short-circuit fires and its ifFunc is
// never invoked. Final PlanValue must be null (not unknown, not some other
// value) and RequiresReplace must be false.
func TestOrgGroupID_BareUUIDImport_ProducesCleanPlan(t *testing.T) {
	modifiers := orgGroupIDPlanModifiers(t)

	// State already holds the import-written null (org_group_id was never
	// SetAttribute'd during ImportState). Overall plan/state are both
	// non-null (an update-shaped plan against existing state, not a
	// create), and the config omits org_group_id so the incoming plan value
	// starts unknown.
	req := planmodifier.Int32Request{
		Plan:        planWithValue(t, types.Int32Unknown()),
		PlanValue:   types.Int32Unknown(),
		State:       stateWithValue(t, types.Int32Null()),
		StateValue:  types.Int32Null(),
		ConfigValue: types.Int32Null(),
	}

	resp := chainPlanModifiers(t, modifiers, req)

	if !resp.PlanValue.IsNull() {
		t.Fatalf("expected final PlanValue to be null, got %#v", resp.PlanValue)
	}
	if resp.RequiresReplace {
		t.Fatalf("expected RequiresReplace == false for a bare-uuid import, got true")
	}
}

// TestOrgGroupID_Create_SuppliedValuePassesThroughWithoutReplace proves that
// supplying org_group_id at Create (overall resource state is null) passes
// the plan value through unchanged and never forces replace — Create never
// replaces.
func TestOrgGroupID_Create_SuppliedValuePassesThroughWithoutReplace(t *testing.T) {
	modifiers := orgGroupIDPlanModifiers(t)

	req := planmodifier.Int32Request{
		State:       nullStateRaw(t), // req.State.Raw.IsNull() == true: resource creation
		StateValue:  types.Int32Null(),
		ConfigValue: types.Int32Value(5),
		Plan:        planWithValue(t, types.Int32Value(5)),
		PlanValue:   types.Int32Value(5),
	}

	resp := chainPlanModifiers(t, modifiers, req)

	if resp.PlanValue.IsNull() || resp.PlanValue.IsUnknown() || resp.PlanValue.ValueInt32() != 5 {
		t.Fatalf("expected final PlanValue to be 5, got %#v", resp.PlanValue)
	}
	if resp.RequiresReplace {
		t.Fatalf("expected RequiresReplace == false at Create, got true")
	}

	// Create-time validation is unweakened by this schema change: it lives
	// in models.MacApplicationResourceModel.FetchValidOrgGroupID, called
	// from Create() in resource_crud.go, entirely independent of the
	// schema's Required/Optional/Computed flags and untouched by this diff.
}

// TestOrgGroupID_GenuineEditToKnownValue_ForcesReplace proves the fix does
// NOT weaken today's destroy/recreate behavior for a genuine edit: changing
// an already-known org_group_id to a different known value must still force
// replace. Both values are already known, so UseStateForUnknown is not
// involved; this drives RequiresReplaceIf directly.
func TestOrgGroupID_GenuineEditToKnownValue_ForcesReplace(t *testing.T) {
	modifiers := orgGroupIDPlanModifiers(t)

	var requiresReplaceIf planmodifier.Int32
	for _, m := range modifiers {
		if strings.Contains(fmt.Sprintf("%T", m), "requiresReplaceIfModifier") {
			requiresReplaceIf = m
			break
		}
	}
	if requiresReplaceIf == nil {
		t.Fatal("org_group_id PlanModifiers does not contain a RequiresReplaceIf modifier")
	}

	req := planmodifier.Int32Request{
		State:       stateWithValue(t, types.Int32Value(5)),
		StateValue:  types.Int32Value(5),
		ConfigValue: types.Int32Value(10),
		Plan:        planWithValue(t, types.Int32Value(10)),
		PlanValue:   types.Int32Value(10),
	}
	resp := planmodifier.Int32Response{PlanValue: req.PlanValue}
	requiresReplaceIf.PlanModifyInt32(context.Background(), req, &resp)

	if !resp.RequiresReplace {
		t.Fatalf("expected RequiresReplace == true for a genuine known-to-different-known edit, got false")
	}
}
