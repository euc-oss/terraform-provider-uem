package profile

import (
	"context"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Tests in this file cover the 2026-09-25 canonical-source Restrictions
// fixes in internal/profile/validate.go (items 1, 2 and 4 of
// internal-design-doc; item 3, the NetworkList
// OneOf validators, is bugfix/networklist-enums) and the accompanying
// allow_application/allowed_widgets numeric-picklist-ID schema validators in
// internal/profile/resource.go.

// --- item 1: disk_media_cds/disk_media_dvds.read_only unsupported ----------

func TestValidateConfig_RestrictionsMediaReadOnly_CDsAndDVDs_True_Rejected(t *testing.T) {
	t.Parallel()

	for _, field := range []string{"disk_media_cds", "disk_media_dvds"} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			restr := restrictionsVal(map[string]tftypes.Value{
				"media": restrictionsMediaVal(map[string]tftypes.Value{
					field: restrictionsMediaAccessVal(map[string]tftypes.Value{
						"read_only": boolVal(true),
					}),
				}),
			})
			resp := runValidateConfigWithRestrictions(t, sdk.PlatformAppleOsX, restr)

			wantPath := "restrictions.media." + field + ".read_only"
			errs := diagErrorsAtPath(resp, wantPath)
			if len(errs) != 1 {
				t.Fatalf("expected exactly 1 error at %s, got %d: %s", wantPath, len(errs), diagSummaries(resp))
			}
		})
	}
}

func TestValidateConfig_RestrictionsMediaReadOnly_CDsAndDVDs_FalseOrNull_NoError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		readOnly tftypes.Value
	}{
		{"false", boolVal(false)},
		{"null", nullBool()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			restr := restrictionsVal(map[string]tftypes.Value{
				"media": restrictionsMediaVal(map[string]tftypes.Value{
					"disk_media_cds": restrictionsMediaAccessVal(map[string]tftypes.Value{
						"read_only": tc.readOnly,
					}),
				}),
			})
			resp := runValidateConfigWithRestrictions(t, sdk.PlatformAppleOsX, restr)
			if resp.Diagnostics.HasError() {
				t.Fatalf("expected no error, got: %s", diagSummaries(resp))
			}
		})
	}
}

// TestValidateConfig_RestrictionsMediaReadOnly_OtherMediaTypes_NotRejected
// proves the check is scoped to exactly disk_media_cds/disk_media_dvds:
// every other MediaAccess entry sharing the same schema does support
// read_only, so read_only = true there must never error.
func TestValidateConfig_RestrictionsMediaReadOnly_OtherMediaTypes_NotRejected(t *testing.T) {
	t.Parallel()

	for _, field := range []string{
		"external_hard_disk_media_access", "hard_disk_dvd_ram",
		"hard_disk_images", "internal_hard_disk_media_access",
	} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			restr := restrictionsVal(map[string]tftypes.Value{
				"media": restrictionsMediaVal(map[string]tftypes.Value{
					field: restrictionsMediaAccessVal(map[string]tftypes.Value{
						"read_only": boolVal(true),
					}),
				}),
			})
			resp := runValidateConfigWithRestrictions(t, sdk.PlatformAppleOsX, restr)
			if resp.Diagnostics.HasError() {
				t.Fatalf("expected %s.read_only = true to be accepted, got: %s", field, diagSummaries(resp))
			}
		})
	}
}

func TestValidateConfig_RestrictionsMediaReadOnly_BurnSupport_True_NotRejected(t *testing.T) {
	t.Parallel()

	restr := restrictionsVal(map[string]tftypes.Value{
		"media": restrictionsMediaVal(map[string]tftypes.Value{
			"recordable_disc": tftypes.NewValue(restrictionsBurnSupportType(), map[string]tftypes.Value{
				"burn_support": restrictionsMediaAccessVal(map[string]tftypes.Value{
					"read_only": boolVal(true),
				}),
			}),
		}),
	})
	resp := runValidateConfigWithRestrictions(t, sdk.PlatformAppleOsX, restr)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected recordable_disc.burn_support.read_only = true to be accepted, got: %s", diagSummaries(resp))
	}
}

// --- item 2: desktop_picture_path requires lock_desktop_picture = true -----

func TestValidateConfig_RestrictionsDesktopPicture_PathSetWithoutLock_Rejected(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		lock tftypes.Value
	}{
		{"lock_false", boolVal(false)},
		{"lock_null", nullBool()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			restr := restrictionsVal(map[string]tftypes.Value{
				"desktop": restrictionsDesktopVal(map[string]tftypes.Value{
					"desktop_picture_path": stringVal("/Library/Desktop Pictures/Company.jpg"),
					"lock_desktop_picture": tc.lock,
				}),
			})
			resp := runValidateConfigWithRestrictions(t, sdk.PlatformAppleOsX, restr)

			wantPath := "restrictions.desktop.lock_desktop_picture"
			errs := diagErrorsAtPath(resp, wantPath)
			if len(errs) != 1 {
				t.Fatalf("expected exactly 1 error at %s, got %d: %s", wantPath, len(errs), diagSummaries(resp))
			}
		})
	}
}

func TestValidateConfig_RestrictionsDesktopPicture_PathSetWithLockTrue_NoError(t *testing.T) {
	t.Parallel()

	restr := restrictionsVal(map[string]tftypes.Value{
		"desktop": restrictionsDesktopVal(map[string]tftypes.Value{
			"desktop_picture_path": stringVal("/Library/Desktop Pictures/Company.jpg"),
			"lock_desktop_picture": boolVal(true),
		}),
	})
	resp := runValidateConfigWithRestrictions(t, sdk.PlatformAppleOsX, restr)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error, got: %s", diagSummaries(resp))
	}
}

func TestValidateConfig_RestrictionsDesktopPicture_PathNull_NoError(t *testing.T) {
	t.Parallel()

	restr := restrictionsVal(map[string]tftypes.Value{
		"desktop": restrictionsDesktopVal(map[string]tftypes.Value{
			"lock_desktop_picture": boolVal(false),
		}),
	})
	resp := runValidateConfigWithRestrictions(t, sdk.PlatformAppleOsX, restr)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error for a null desktop_picture_path, got: %s", diagSummaries(resp))
	}
}

func TestValidateConfig_RestrictionsDesktopPicture_EmptyPathWithoutLock_Rejected(t *testing.T) {
	t.Parallel()

	// An empty string is a known, non-null value with its own documented
	// meaning ("locks the current desktop picture"), so it must still
	// require lock_desktop_picture = true.
	restr := restrictionsVal(map[string]tftypes.Value{
		"desktop": restrictionsDesktopVal(map[string]tftypes.Value{
			"desktop_picture_path": stringVal(""),
		}),
	})
	resp := runValidateConfigWithRestrictions(t, sdk.PlatformAppleOsX, restr)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for an empty desktop_picture_path without lock_desktop_picture = true, got none")
	}
}

// --- item 4: allow_application/allowed_widgets require their parent flag ---

func TestValidateConfig_RestrictionsApplications_AllowApplicationWithoutFlag_Rejected(t *testing.T) {
	t.Parallel()

	restr := restrictionsVal(map[string]tftypes.Value{
		"applications": restrictionsApplicationsVal(map[string]tftypes.Value{
			"allow_application": stringListVal("12345"),
		}),
	})
	resp := runValidateConfigWithRestrictions(t, sdk.PlatformAppleOsX, restr)

	wantPath := "restrictions.applications.restrict_which_applications_are_allowed_to_launch"
	errs := diagErrorsAtPath(resp, wantPath)
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error at %s, got %d: %s", wantPath, len(errs), diagSummaries(resp))
	}
}

func TestValidateConfig_RestrictionsApplications_AllowApplicationWithFlagTrue_NoError(t *testing.T) {
	t.Parallel()

	restr := restrictionsVal(map[string]tftypes.Value{
		"applications": restrictionsApplicationsVal(map[string]tftypes.Value{
			"allow_application": stringListVal("12345"),
			"restrict_which_applications_are_allowed_to_launch": boolVal(true),
		}),
	})
	resp := runValidateConfigWithRestrictions(t, sdk.PlatformAppleOsX, restr)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error, got: %s", diagSummaries(resp))
	}
}

func TestValidateConfig_RestrictionsApplications_EmptyOrNullAllowApplication_NoError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		list tftypes.Value
	}{
		{"null", nullStringList()},
		{"empty", stringListVal()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			restr := restrictionsVal(map[string]tftypes.Value{
				"applications": restrictionsApplicationsVal(map[string]tftypes.Value{
					"allow_application": tc.list,
				}),
			})
			resp := runValidateConfigWithRestrictions(t, sdk.PlatformAppleOsX, restr)
			if resp.Diagnostics.HasError() {
				t.Fatalf("expected no error, got: %s", diagSummaries(resp))
			}
		})
	}
}

func TestValidateConfig_RestrictionsWidgets_AllowedWidgetsWithoutFlag_Rejected(t *testing.T) {
	t.Parallel()

	restr := restrictionsVal(map[string]tftypes.Value{
		"widgets": restrictionsWidgetsVal(map[string]tftypes.Value{
			"allowed_widgets": stringListVal("67890"),
		}),
	})
	resp := runValidateConfigWithRestrictions(t, sdk.PlatformAppleOsX, restr)

	wantPath := "restrictions.widgets.allow_only_configured_widgets"
	errs := diagErrorsAtPath(resp, wantPath)
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error at %s, got %d: %s", wantPath, len(errs), diagSummaries(resp))
	}
}

func TestValidateConfig_RestrictionsWidgets_AllowedWidgetsWithFlagTrue_NoError(t *testing.T) {
	t.Parallel()

	restr := restrictionsVal(map[string]tftypes.Value{
		"widgets": restrictionsWidgetsVal(map[string]tftypes.Value{
			"allowed_widgets":               stringListVal("67890"),
			"allow_only_configured_widgets": boolVal(true),
		}),
	})
	resp := runValidateConfigWithRestrictions(t, sdk.PlatformAppleOsX, restr)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error, got: %s", diagSummaries(resp))
	}
}

// --- item 4 (schema-level): numeric-picklist-ID validator -----------------

// listAttrValidators returns the validators attached to the
// restrictions.<block>.<field> schema.ListAttribute, so tests can exercise
// them the same way the framework does during ValidateResourceConfig,
// mirroring recoveryTypeValidators in validate_filevault_recovery_type_test.go.
func listAttrValidators(t *testing.T, block, field string) []validator.List {
	t.Helper()

	schemaResp := getResourceSchema(t)

	restrAttr, ok := schemaResp.Schema.Attributes["restrictions"]
	if !ok {
		t.Fatal("schema has no \"restrictions\" attribute")
	}
	restrNested, ok := restrAttr.(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("\"restrictions\" is not a schema.SingleNestedAttribute, got %T", restrAttr)
	}

	blockAttr, ok := restrNested.Attributes[block]
	if !ok {
		t.Fatalf("restrictions schema has no %q attribute", block)
	}
	blockNested, ok := blockAttr.(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("%q is not a schema.SingleNestedAttribute, got %T", block, blockAttr)
	}

	fieldAttr, ok := blockNested.Attributes[field]
	if !ok {
		t.Fatalf("%s schema has no %q attribute", block, field)
	}
	fieldList, ok := fieldAttr.(schema.ListAttribute)
	if !ok {
		t.Fatalf("%q is not a schema.ListAttribute, got %T", field, fieldAttr)
	}

	return fieldList.Validators
}

func runListAttrValidators(t *testing.T, block, field string, elements []string) []validator.ListResponse {
	t.Helper()

	validators := listAttrValidators(t, block, field)
	elemValues := make([]attr.Value, len(elements))
	for i, e := range elements {
		elemValues[i] = types.StringValue(e)
	}
	listVal, diags := types.ListValue(types.StringType, elemValues)
	if diags.HasError() {
		t.Fatalf("failed to build test list value: %v", diags)
	}

	responses := make([]validator.ListResponse, 0, len(validators))
	for _, v := range validators {
		req := validator.ListRequest{
			Path:        path.Root("restrictions").AtName(block).AtName(field),
			ConfigValue: listVal,
		}
		resp := &validator.ListResponse{}
		v.ValidateList(context.Background(), req, resp)
		responses = append(responses, *resp)
	}
	return responses
}

func TestRestrictionsApplicationsAllowApplication_PlanTimeValidation_RejectsNonNumeric(t *testing.T) {
	t.Parallel()

	for _, elements := range [][]string{{"com.example.app"}, {"12345", "not-a-number"}, {""}} {
		responses := runListAttrValidators(t, "applications", "allow_application", elements)
		hasError := false
		for _, r := range responses {
			if r.Diagnostics.HasError() {
				hasError = true
			}
		}
		if !hasError {
			t.Errorf("expected a plan-time validation error for allow_application = %v, got none", elements)
		}
	}
}

func TestRestrictionsApplicationsAllowApplication_PlanTimeValidation_AcceptsNumericIDs(t *testing.T) {
	t.Parallel()

	for _, elements := range [][]string{{"12345"}, {"1", "2", "3"}, {}} {
		responses := runListAttrValidators(t, "applications", "allow_application", elements)
		for _, r := range responses {
			if r.Diagnostics.HasError() {
				t.Errorf("expected no plan-time validation error for allow_application = %v, got: %v", elements, r.Diagnostics)
			}
		}
	}
}

func TestRestrictionsWidgetsAllowedWidgets_PlanTimeValidation_RejectsNonNumeric(t *testing.T) {
	t.Parallel()

	responses := runListAttrValidators(t, "widgets", "allowed_widgets", []string{"Weather"})
	hasError := false
	for _, r := range responses {
		if r.Diagnostics.HasError() {
			hasError = true
		}
	}
	if !hasError {
		t.Error("expected a plan-time validation error for allowed_widgets = [\"Weather\"], got none")
	}
}

func TestRestrictionsWidgetsAllowedWidgets_PlanTimeValidation_AcceptsNumericIDs(t *testing.T) {
	t.Parallel()

	responses := runListAttrValidators(t, "widgets", "allowed_widgets", []string{"67890"})
	for _, r := range responses {
		if r.Diagnostics.HasError() {
			t.Errorf("expected no plan-time validation error, got: %v", r.Diagnostics)
		}
	}
}

// --- null/unknown restrictions is a no-op across all 3 checks -------------

func TestValidateConfig_Restrictions_NullOrUnknown_NoDiagnostics(t *testing.T) {
	t.Parallel()

	t.Run("null", func(t *testing.T) {
		t.Parallel()
		resp := runValidateConfigWithRestrictions(t, sdk.PlatformAppleOsX, nullRestrictions())
		if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() != 0 {
			t.Fatalf("expected no diagnostics for null restrictions, got: %v", resp.Diagnostics)
		}
	})

	t.Run("unknown", func(t *testing.T) {
		t.Parallel()
		unknown := tftypes.NewValue(restrictionsObjectType(), tftypes.UnknownValue)
		resp := runValidateConfigWithRestrictions(t, sdk.PlatformAppleOsX, unknown)
		if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() != 0 {
			t.Fatalf("expected no diagnostics for an unknown restrictions object, got: %v", resp.Diagnostics)
		}
	})
}

// --- no false rejections: non-macOS platform, unknown parent values -------

// canonicalAllViolations returns a restrictions value that violates every
// canonical rule (items 1, 2 and 4) at once, with the parent flags/lock
// replaced by the given values.
func canonicalAllViolations(lock, launchFlag, widgetsFlag tftypes.Value) tftypes.Value {
	return restrictionsVal(map[string]tftypes.Value{
		"media": restrictionsMediaVal(map[string]tftypes.Value{
			"disk_media_cds": restrictionsMediaAccessVal(map[string]tftypes.Value{"read_only": boolVal(true)}),
		}),
		"desktop": restrictionsDesktopVal(map[string]tftypes.Value{
			"desktop_picture_path": stringVal("/Library/Desktop Pictures/Company.jpg"),
			"lock_desktop_picture": lock,
		}),
		"applications": restrictionsApplicationsVal(map[string]tftypes.Value{
			"allow_application": stringListVal("12345"),
			"restrict_which_applications_are_allowed_to_launch": launchFlag,
		}),
		"widgets": restrictionsWidgetsVal(map[string]tftypes.Value{
			"allowed_widgets":               stringListVal("67890"),
			"allow_only_configured_widgets": widgetsFlag,
		}),
	})
}

func TestValidateConfig_RestrictionsCanonical_NonMacOSPlatform_NoError(t *testing.T) {
	t.Parallel()

	restr := canonicalAllViolations(nullBool(), nullBool(), nullBool())
	// Control: the same value on macOS fires all four rules.
	if got := runValidateConfigWithRestrictions(t, sdk.PlatformAppleOsX, restr).Diagnostics.ErrorsCount(); got != 4 {
		t.Fatalf("control: expected 4 errors on macOS, got %d", got)
	}
	resp := runValidateConfigWithRestrictions(t, sdk.PlatformWindows10, restr)
	for _, p := range []string{
		"restrictions.media.disk_media_cds.read_only",
		"restrictions.desktop.lock_desktop_picture",
		"restrictions.applications.restrict_which_applications_are_allowed_to_launch",
		"restrictions.widgets.allow_only_configured_widgets",
	} {
		if errs := diagErrorsAtPath(resp, p); len(errs) != 0 {
			t.Errorf("expected no error at %s on a non-macOS platform, got %v", p, errs)
		}
	}
}

func TestValidateConfig_RestrictionsCanonical_UnknownParent_NoError(t *testing.T) {
	t.Parallel()

	unknownBool := tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue)
	restr := canonicalAllViolations(unknownBool, unknownBool, unknownBool)
	resp := runValidateConfigWithRestrictions(t, sdk.PlatformAppleOsX, restr)
	// Only the media rule (no parent) may fire; the three parent-gated
	// rules must defer while the parent is unknown.
	if got := resp.Diagnostics.ErrorsCount(); got != 1 {
		t.Fatalf("expected exactly 1 error (media), got %d: %s", got, diagSummaries(resp))
	}
}
