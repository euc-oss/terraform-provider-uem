package profile

// Server-filled attributes (internal-ticket).
//
// UEM fills a value for fields a configuration leaves out, and for whole
// sibling sub-blocks once any one field of a payload is set (seen live,
// a UEM 26.4 lab tenant: setting one restrictions.applications.camera leaf
// makes UEM return every other restrictions sub-block fully populated). An
// Optional-only attribute plans null for those, the read-back is not null,
// and Terraform core rejects the apply. The model used here is the one the
// repo already uses for server-defaulted payload fields (disk_encryption.mcx,
// filevault2.enable, kernel_extension, scep_list): Optional+Computed, UEM's
// value is stored as returned, and an unset field carries its prior state
// value on later plans. Nothing is defaulted or normalised by the provider,
// and a value that IS configured is still compared with what UEM returns.

import (
	"context"
	"fmt"
	"reflect"

	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

const useStateWhenKnownDescription = "Once set, the value of this attribute in state will not change; while the state value is null the plan stays unknown."

// The modifiers below copy the prior state value into an unknown plan, but only
// when that state value is itself non-null. The framework's own
// UseStateForUnknown only checks whether the whole resource has a prior state,
// so an attribute inside a block that is ADDED to an existing profile (null in
// state) was planned null instead of unknown and failed the same way as on
// create. A configured value is never touched.

type useStateWhenKnownBoolModifier struct{}

func (useStateWhenKnownBoolModifier) Description(context.Context) string {
	return useStateWhenKnownDescription
}
func (m useStateWhenKnownBoolModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}
func (useStateWhenKnownBoolModifier) PlanModifyBool(_ context.Context, req planmodifier.BoolRequest, resp *planmodifier.BoolResponse) {
	if req.State.Raw.IsNull() || req.StateValue.IsNull() || !req.PlanValue.IsUnknown() || req.ConfigValue.IsUnknown() {
		return
	}
	resp.PlanValue = req.StateValue
}

func useStateWhenKnownBool() planmodifier.Bool { return useStateWhenKnownBoolModifier{} }

type useStateWhenKnownInt64Modifier struct{}

func (useStateWhenKnownInt64Modifier) Description(context.Context) string {
	return useStateWhenKnownDescription
}
func (m useStateWhenKnownInt64Modifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}
func (useStateWhenKnownInt64Modifier) PlanModifyInt64(_ context.Context, req planmodifier.Int64Request, resp *planmodifier.Int64Response) {
	if req.State.Raw.IsNull() || req.StateValue.IsNull() || !req.PlanValue.IsUnknown() || req.ConfigValue.IsUnknown() {
		return
	}
	resp.PlanValue = req.StateValue
}

func useStateWhenKnownInt64() planmodifier.Int64 { return useStateWhenKnownInt64Modifier{} }

type useStateWhenKnownStringModifier struct{}

func (useStateWhenKnownStringModifier) Description(context.Context) string {
	return useStateWhenKnownDescription
}
func (m useStateWhenKnownStringModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}
func (useStateWhenKnownStringModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.State.Raw.IsNull() || req.StateValue.IsNull() || !req.PlanValue.IsUnknown() || req.ConfigValue.IsUnknown() {
		return
	}
	resp.PlanValue = req.StateValue
}

func useStateWhenKnownString() planmodifier.String { return useStateWhenKnownStringModifier{} }

type useStateWhenKnownObjectModifier struct{}

func (useStateWhenKnownObjectModifier) Description(context.Context) string {
	return useStateWhenKnownDescription
}
func (m useStateWhenKnownObjectModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}
func (useStateWhenKnownObjectModifier) PlanModifyObject(_ context.Context, req planmodifier.ObjectRequest, resp *planmodifier.ObjectResponse) {
	if req.State.Raw.IsNull() || req.StateValue.IsNull() || !req.PlanValue.IsUnknown() || req.ConfigValue.IsUnknown() {
		return
	}
	resp.PlanValue = req.StateValue
}

func useStateWhenKnownObject() planmodifier.Object { return useStateWhenKnownObjectModifier{} }

// serverFilledAttributes returns attrs with every bool, int64, string and
// nested-object attribute made Optional+Computed with the state-carrying
// modifier above. List and other attributes are returned unchanged. Used for
// the restrictions sub-blocks (see applyServerFilledRestrictions).
func serverFilledAttributes(attrs map[string]schema.Attribute) map[string]schema.Attribute {
	out := make(map[string]schema.Attribute, len(attrs))
	for name, a := range attrs {
		switch v := a.(type) {
		case schema.BoolAttribute:
			v.Computed = true
			v.PlanModifiers = append(append([]planmodifier.Bool{}, v.PlanModifiers...), useStateWhenKnownBool())
			out[name] = v
		case schema.Int64Attribute:
			v.Computed = true
			v.PlanModifiers = append(append([]planmodifier.Int64{}, v.PlanModifiers...), useStateWhenKnownInt64())
			out[name] = v
		case schema.StringAttribute:
			v.Computed = true
			v.PlanModifiers = append(append([]planmodifier.String{}, v.PlanModifiers...), useStateWhenKnownString())
			out[name] = v
		case schema.SingleNestedAttribute:
			v.Computed = true
			v.PlanModifiers = append(append([]planmodifier.Object{}, v.PlanModifiers...), useStateWhenKnownObject())
			v.Attributes = serverFilledAttributes(v.Attributes)
			out[name] = v
		default:
			out[name] = a
		}
	}
	return out
}

// applyServerFilledRestrictions makes every restrictions sub-block and leaf
// server-filled (live: UEM returns all sibling sub-blocks populated with
// defaults once one leaf is set). The restrictions attribute itself keeps its
// own modifiers.
func applyServerFilledRestrictions(s *schema.Schema) {
	r, ok := s.Attributes["restrictions"].(schema.SingleNestedAttribute)
	if !ok {
		return
	}
	r.Attributes = serverFilledAttributes(r.Attributes)
	s.Attributes["restrictions"] = r
}

// getPlan reads the plan into target. The restrictions sub-blocks are Go
// pointer structs, which cannot hold the unknown a Computed sub-block plans
// as on create (or when its parent block is added to an existing profile), so
// an unknown object under restrictions is read as null: nothing is sent for
// it, and UEM's filled value lands in state after the read-back. Leaves are
// types.Bool/String/Int64 and hold unknown fine.
func getPlan(ctx context.Context, plan tfsdk.Plan, target any) diag.Diagnostics {
	raw, err := tftypes.Transform(plan.Raw, func(p *tftypes.AttributePath, v tftypes.Value) (tftypes.Value, error) {
		steps := p.Steps()
		if len(steps) < 2 || v.IsKnown() || !v.Type().Is(tftypes.Object{}) {
			return v, nil
		}
		if name, ok := steps[0].(tftypes.AttributeName); ok && string(name) == "restrictions" {
			return tftypes.NewValue(v.Type(), nil), nil
		}
		return v, nil
	})
	if err != nil {
		var d diag.Diagnostics
		d.AddError("Plan Conversion Error", err.Error())
		return d
	}
	plan.Raw = raw
	return plan.Get(ctx, target)
}

// readBackFailedWarning is the warning added when a write succeeded but the
// final GET did not.
func readBackFailedWarning(profileID int) string {
	return fmt.Sprintf("Profile %d was written, but reading it back from UEM failed, so values UEM fills in for settings left out of the configuration could not be read. They are stored as null for now; run `terraform refresh` or the next `terraform plan` to reconcile them with UEM.", profileID)
}

// resolveServerFilledUnknowns is used when the read-back after a successful
// write failed. It turns every value still unknown in the server-filled blocks
// (gatekeeper, system_extensions, privacy_preferences and restrictions) into
// null, so the applied state holds no unknown. A known planned value is never
// changed.
func resolveServerFilledUnknowns(data *profilemodels.ProfileResourceModel) {
	nullUnknowns(reflect.ValueOf(data.Gatekeeper))
	nullUnknowns(reflect.ValueOf(data.SystemExtensions))
	nullUnknowns(reflect.ValueOf(data.Restrictions))
	nullUnknowns(reflect.ValueOf(data.PrivacyPreferences))
}

var attrValueType = reflect.TypeOf((*attr.Value)(nil)).Elem()

// nullUnknowns walks pointers, structs and slices, and sets every settable
// attr.Value that is unknown to its zero value, which is null for the framework
// value types.
func nullUnknowns(v reflect.Value) {
	switch v.Kind() {
	case reflect.Ptr:
		if !v.IsNil() {
			nullUnknowns(v.Elem())
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			nullUnknowns(v.Index(i))
		}
	case reflect.Struct:
		if v.Type().Implements(attrValueType) {
			if av, ok := v.Interface().(attr.Value); ok && av.IsUnknown() && v.CanSet() {
				v.Set(reflect.Zero(v.Type()))
			}
			return
		}
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				nullUnknowns(v.Field(i))
			}
		}
	}
}
