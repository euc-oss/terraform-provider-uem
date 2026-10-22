package profile

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// enableRecoveryKeyDriftPath is the attribute path the modifier under test
// is registered at.
func enableRecoveryKeyDriftPath() path.Path {
	return path.Root("disk_encryption").AtName("airwatch").AtName("enable_recovery_key")
}

// enableRecoveryKeyDriftRequest builds a planmodifier.BoolRequest whose
// Config carries disk_encryption.filevault2.recovery_type = recoveryType
// (nil means null; recoveryTypeUnknown overrides both to wholly unknown),
// mirroring how the framework would present the sibling attribute at plan
// time -- everything else in the resource config is left null.
func enableRecoveryKeyDriftRequest(t *testing.T, configValue, stateValue, planValue types.Bool, recoveryType *int64, recoveryTypeUnknown bool) (planmodifier.BoolRequest, *planmodifier.BoolResponse) {
	t.Helper()

	var rt tftypes.Value
	switch {
	case recoveryTypeUnknown:
		rt = tftypes.NewValue(tftypes.Number, tftypes.UnknownValue)
	case recoveryType == nil:
		rt = tftypes.NewValue(tftypes.Number, nil)
	default:
		rt = int64Val(*recoveryType)
	}

	schemaResp := getResourceSchema(t)
	ctx := context.Background()
	configType := schemaResp.Schema.Type().TerraformType(ctx)
	values := map[string]tftypes.Value{
		"disk_encryption": diskEncryptionVal(map[string]tftypes.Value{
			"filevault2": filevaultVal(map[string]tftypes.Value{
				"recovery_type": rt,
			}),
		}),
	}
	raw := tftypes.NewValue(configType, fillMissingAttrs(t, configType, values))

	req := planmodifier.BoolRequest{
		Path: enableRecoveryKeyDriftPath(),
		Config: tfsdk.Config{
			Schema: schemaResp.Schema,
			Raw:    raw,
		},
		ConfigValue: configValue,
		StateValue:  stateValue,
		PlanValue:   planValue,
	}
	resp := &planmodifier.BoolResponse{PlanValue: req.PlanValue}
	return req, resp
}

func TestEnableRecoveryKeyCorporateDriftModifier_ConfigNullRecoveryTypeCorporate_PlansFalse(t *testing.T) {
	t.Parallel()

	rt := int64(2)
	req, resp := enableRecoveryKeyDriftRequest(t, types.BoolNull(), types.BoolNull(), types.BoolUnknown(), &rt, false)

	enableRecoveryKeyCorporateDriftModifier{}.PlanModifyBool(context.Background(), req, resp)

	if resp.PlanValue.IsUnknown() || resp.PlanValue.IsNull() {
		t.Fatalf("expected a known, non-null plan value, got %v", resp.PlanValue)
	}
	if resp.PlanValue.ValueBool() != false {
		t.Errorf("expected plan value false for recovery_type = 2 (Corporate), got %v", resp.PlanValue.ValueBool())
	}
}

func TestEnableRecoveryKeyCorporateDriftModifier_ConfigNullRecoveryTypePersonal_PlansNull(t *testing.T) {
	t.Parallel()

	for _, rt := range []int64{1, 3} {
		t.Run(fmt.Sprintf("recovery_type=%d", rt), func(t *testing.T) {
			t.Parallel()
			req, resp := enableRecoveryKeyDriftRequest(t, types.BoolNull(), types.BoolNull(), types.BoolUnknown(), &rt, false)

			enableRecoveryKeyCorporateDriftModifier{}.PlanModifyBool(context.Background(), req, resp)

			if !resp.PlanValue.IsNull() {
				t.Errorf("expected null plan value for recovery_type = %d, got %v", rt, resp.PlanValue)
			}
		})
	}
}

func TestEnableRecoveryKeyCorporateDriftModifier_ConfigNullRecoveryTypeUnknown_FallsBackToState(t *testing.T) {
	t.Parallel()

	t.Run("prior state known: use it", func(t *testing.T) {
		t.Parallel()
		req, resp := enableRecoveryKeyDriftRequest(t, types.BoolNull(), types.BoolValue(false), types.BoolUnknown(), nil, true)

		enableRecoveryKeyCorporateDriftModifier{}.PlanModifyBool(context.Background(), req, resp)

		if resp.PlanValue.IsNull() || resp.PlanValue.IsUnknown() || resp.PlanValue.ValueBool() != false {
			t.Errorf("expected plan value to fall back to prior state (false), got %v", resp.PlanValue)
		}
	})

	t.Run("prior state null: leaves the framework's proposed value alone", func(t *testing.T) {
		t.Parallel()
		req, resp := enableRecoveryKeyDriftRequest(t, types.BoolNull(), types.BoolNull(), types.BoolUnknown(), nil, true)

		enableRecoveryKeyCorporateDriftModifier{}.PlanModifyBool(context.Background(), req, resp)

		if !resp.PlanValue.IsUnknown() {
			t.Errorf("expected plan value to remain unknown on create with unknown recovery_type, got %v", resp.PlanValue)
		}
	})
}

func TestEnableRecoveryKeyCorporateDriftModifier_ExplicitConfigValuePassesThrough(t *testing.T) {
	t.Parallel()

	for _, v := range []bool{true, false} {
		t.Run(fmt.Sprintf("value=%v", v), func(t *testing.T) {
			t.Parallel()
			rt := int64(2)
			req, resp := enableRecoveryKeyDriftRequest(t, types.BoolValue(v), types.BoolNull(), types.BoolValue(v), &rt, false)

			enableRecoveryKeyCorporateDriftModifier{}.PlanModifyBool(context.Background(), req, resp)

			if resp.PlanValue.IsNull() || resp.PlanValue.IsUnknown() || resp.PlanValue.ValueBool() != v {
				t.Errorf("expected explicit config value %v to pass through unchanged, got %v", v, resp.PlanValue)
			}
		})
	}
}
