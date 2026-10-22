package profile

import (
	"context"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// The create-time half of the restrictions parent-flag checks: with no prior
// state to carry a stored flag, a dependent setting with a NULL parent flag is
// rejected exactly like an explicit false. On update a null flag passes (the
// stored value carries over) and an explicit false is still rejected.

func restrictionsFlagCases() []struct {
	name, errPath string
	build         func(flag tftypes.Value) tftypes.Value
} {
	return []struct {
		name, errPath string
		build         func(flag tftypes.Value) tftypes.Value
	}{
		{"allow_application", "restrictions.applications.restrict_which_applications_are_allowed_to_launch", func(flag tftypes.Value) tftypes.Value {
			return restrictionsVal(map[string]tftypes.Value{"applications": restrictionsApplicationsVal(map[string]tftypes.Value{
				"allow_application": stringListVal("12345"), "restrict_which_applications_are_allowed_to_launch": flag,
			})})
		}},
		{"allowed_widgets", "restrictions.widgets.allow_only_configured_widgets", func(flag tftypes.Value) tftypes.Value {
			return restrictionsVal(map[string]tftypes.Value{"widgets": restrictionsWidgetsVal(map[string]tftypes.Value{
				"allowed_widgets": stringListVal("67890"), "allow_only_configured_widgets": flag,
			})})
		}},
	}
}

// modifyPlanErrorsAt runs ModifyPlan for a config with the given prior state
// (null = create) and returns the number of errors at errPath.
func modifyPlanErrorsAt(t *testing.T, prior tftypes.Value, restr tftypes.Value, errPath string) int {
	t.Helper()
	config := createConfigWith(t, map[string]tftypes.Value{"restrictions": restr})
	sch := getResourceSchema(t).Schema
	resp := &resource.ModifyPlanResponse{}
	(&ProfileResource{}).ModifyPlan(context.Background(), resource.ModifyPlanRequest{
		Config: tfsdk.Config{Schema: sch, Raw: config},
		Plan:   tfsdk.Plan{Schema: sch, Raw: config},
		State:  tfsdk.State{Schema: sch, Raw: prior},
	}, resp)
	n := 0
	for _, d := range resp.Diagnostics.Errors() {
		if wp, ok := d.(interface{ Path() path.Path }); ok && wp.Path().String() == errPath {
			n++
		}
	}
	return n
}

func TestModifyPlan_Create_NullParentFlagWithDependentSettingRejected(t *testing.T) {
	t.Parallel()
	for _, tc := range restrictionsFlagCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if n := modifyPlanErrorsAt(t, nullResourceState(t).Raw, tc.build(nullBool()), tc.errPath); n != 1 {
				t.Fatalf("create with a null flag: want 1 error at %s, got %d", tc.errPath, n)
			}
		})
	}
}

func TestModifyPlan_Create_ExplicitFalseRejected(t *testing.T) {
	t.Parallel()
	for _, tc := range restrictionsFlagCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if n := modifyPlanErrorsAt(t, nullResourceState(t).Raw, tc.build(boolVal(false)), tc.errPath); n != 1 {
				t.Fatalf("create with explicit false: want 1 error at %s, got %d", tc.errPath, n)
			}
		})
	}
}

func TestModifyPlan_Create_TrueFlagAccepted(t *testing.T) {
	t.Parallel()
	for _, tc := range restrictionsFlagCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if n := modifyPlanErrorsAt(t, nullResourceState(t).Raw, tc.build(boolVal(true)), tc.errPath); n != 0 {
				t.Fatalf("create with true flag: want no error, got %d", n)
			}
		})
	}
}

// updateCarriedCases returns, per dependency, a builder for restrictions with
// the dependent setting (list or path) and a flag value, plus the path of the
// dependent attribute (where the update-time error is reported).
func updateCarriedCases() []struct {
	name, errPath string
	build         func(withDependent bool, flag tftypes.Value) tftypes.Value
} {
	return []struct {
		name, errPath string
		build         func(withDependent bool, flag tftypes.Value) tftypes.Value
	}{
		{"allow_application", "restrictions.applications.allow_application", func(dep bool, flag tftypes.Value) tftypes.Value {
			l := tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil)
			if dep {
				l = stringListVal("12345")
			}
			return restrictionsVal(map[string]tftypes.Value{"applications": restrictionsApplicationsVal(map[string]tftypes.Value{
				"allow_application": l, "restrict_which_applications_are_allowed_to_launch": flag,
			})})
		}},
		{"allowed_widgets", "restrictions.widgets.allowed_widgets", func(dep bool, flag tftypes.Value) tftypes.Value {
			l := tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil)
			if dep {
				l = stringListVal("67890")
			}
			return restrictionsVal(map[string]tftypes.Value{"widgets": restrictionsWidgetsVal(map[string]tftypes.Value{
				"allowed_widgets": l, "allow_only_configured_widgets": flag,
			})})
		}},
	}
}

// updateErrorsAt runs ModifyPlan for an update with a separate config and plan
// (the plan carries the stored flag when the config omits it) and counts the
// errors at errPath.
func updateErrorsAt(t *testing.T, prior, cfgRestr, planRestr tftypes.Value, errPath string) int {
	t.Helper()
	sch := getResourceSchema(t).Schema
	config := updateConfigWith(t, map[string]tftypes.Value{"restrictions": cfgRestr})
	plan := createResourcePlan(t, map[string]tftypes.Value{
		"name": stringVal("Renamed"), "platform": stringVal(sdk.PlatformAppleOsX),
		"org_group_id": stringVal("14165"), "restrictions": planRestr,
	}).Raw
	resp := &resource.ModifyPlanResponse{}
	(&ProfileResource{}).ModifyPlan(context.Background(), resource.ModifyPlanRequest{
		Config: tfsdk.Config{Schema: sch, Raw: config},
		Plan:   tfsdk.Plan{Schema: sch, Raw: plan},
		State:  tfsdk.State{Schema: sch, Raw: prior},
	}, resp)
	n := 0
	for _, d := range resp.Diagnostics.Errors() {
		if wp, ok := d.(interface{ Path() path.Path }); ok && wp.Path().String() == errPath {
			n++
		}
	}
	return n
}

func priorWithRestrictions(t *testing.T, restr tftypes.Value) tftypes.Value {
	t.Helper()
	return createResourceState(t, mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{"restrictions": restr})).Raw
}

// Update that removes a previously TRUE flag (list kept) passes: the plan
// carries the stored true flag. The prior state really holds the flag true.
func TestUpdate_RemovingPreviouslyTrueParentFlagPasses(t *testing.T) {
	t.Parallel()
	for _, tc := range updateCarriedCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			prior := priorWithRestrictions(t, tc.build(true, boolVal(true)))
			cfg, plan := tc.build(true, nullBool()), tc.build(true, boolVal(true))
			if n := updateErrorsAt(t, prior, cfg, plan, tc.errPath); n != 0 {
				t.Fatalf("ModifyPlan on update: want no error, got %d", n)
			}
			if resp := runValidateConfigWithRestrictions(t, sdk.PlatformAppleOsX, cfg); resp.Diagnostics.HasError() {
				t.Fatalf("ValidateConfig: %s", diagSummaries(resp))
			}
		})
	}
}

// Prior flag false, config adds the dependent setting and omits the flag: the
// plan carries false, so ModifyPlan rejects it at the dependent attribute.
func TestUpdate_AddingDependentWithCarriedFalseFlagRejected(t *testing.T) {
	t.Parallel()
	for _, tc := range updateCarriedCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			prior := priorWithRestrictions(t, tc.build(false, boolVal(false)))
			cfg, plan := tc.build(true, nullBool()), tc.build(true, boolVal(false))
			if n := updateErrorsAt(t, prior, cfg, plan, tc.errPath); n != 1 {
				t.Fatalf("want 1 error at %s, got %d", tc.errPath, n)
			}
		})
	}
}

// Prior flag true, config adds or keeps the dependent setting with the flag
// omitted: the plan carries true and the update passes.
func TestUpdate_AddingDependentWithCarriedTrueFlagPasses(t *testing.T) {
	t.Parallel()
	for _, tc := range updateCarriedCases() {
		for _, hadDependent := range []bool{false, true} {
			name := tc.name + "/added"
			if hadDependent {
				name = tc.name + "/kept"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				prior := priorWithRestrictions(t, tc.build(hadDependent, boolVal(true)))
				cfg, plan := tc.build(true, nullBool()), tc.build(true, boolVal(true))
				if n := updateErrorsAt(t, prior, cfg, plan, tc.errPath); n != 0 {
					t.Fatalf("want no error, got %d", n)
				}
			})
		}
	}
}

// Prior flag false, config sets the flag explicitly true: passes.
func TestUpdate_ExplicitTrueFlagOverridesStoredFalsePasses(t *testing.T) {
	t.Parallel()
	for _, tc := range updateCarriedCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			prior := priorWithRestrictions(t, tc.build(false, boolVal(false)))
			cfg := tc.build(true, boolVal(true))
			if n := updateErrorsAt(t, prior, cfg, cfg, tc.errPath); n != 0 {
				t.Fatalf("want no error, got %d", n)
			}
		})
	}
}

// An explicit false with a dependent setting is rejected by ValidateConfig,
// which runs for create and update alike.
func TestValidateConfig_ExplicitFalseRejectedOnCreateAndUpdate(t *testing.T) {
	t.Parallel()
	for _, tc := range restrictionsFlagCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			resp := runValidateConfigWithRestrictions(t, sdk.PlatformAppleOsX, tc.build(boolVal(false)))
			if errs := diagErrorsAtPath(resp, tc.errPath); len(errs) != 1 {
				t.Fatalf("want 1 error at %s, got %d: %s", tc.errPath, len(errs), diagSummaries(resp))
			}
		})
	}
}

// desktop_picture_path is rejected on create and on update, with or without
// lock_desktop_picture = true (UEM 26.2/26.4 never stores it); the lock alone
// still passes.
func TestModifyPlanAndValidate_DesktopPicturePathRejected(t *testing.T) {
	t.Parallel()
	const errPath = "restrictions.desktop.desktop_picture_path"
	desk := func(p, lock tftypes.Value) tftypes.Value {
		return restrictionsVal(map[string]tftypes.Value{"desktop": restrictionsDesktopVal(map[string]tftypes.Value{
			"desktop_picture_path": p, "lock_desktop_picture": lock,
		})})
	}
	pic := stringVal("/Library/Desktop Pictures/Company.jpg")
	for name, lock := range map[string]tftypes.Value{"lock_true": boolVal(true), "lock_false": boolVal(false), "lock_omitted": nullBool()} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			restr := desk(pic, lock)
			if errs := diagErrorsAtPath(runValidateConfigWithRestrictions(t, sdk.PlatformAppleOsX, restr), errPath); len(errs) != 1 {
				t.Fatalf("ValidateConfig (create or update): want 1 error at %s, got %d", errPath, len(errs))
			}
		})
	}
	t.Run("empty_path", func(t *testing.T) {
		t.Parallel()
		restr := desk(stringVal(""), nullBool())
		if errs := diagErrorsAtPath(runValidateConfigWithRestrictions(t, sdk.PlatformAppleOsX, restr), errPath); len(errs) != 1 {
			t.Fatalf("empty desktop_picture_path: want 1 error at %s, got %d", errPath, len(errs))
		}
	})
	t.Run("lock_alone_passes", func(t *testing.T) {
		t.Parallel()
		for _, lock := range []tftypes.Value{boolVal(true), boolVal(false)} {
			resp := runValidateConfigWithRestrictions(t, sdk.PlatformAppleOsX, desk(nullString(), lock))
			if resp.Diagnostics.HasError() {
				t.Fatalf("lock alone must pass: %s", diagSummaries(resp))
			}
		}
	})
}
