package profile

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Ensure ProfileResource implements the upgrade-state interface.
var _ resource.ResourceWithUpgradeState = &ProfileResource{}

// UpgradeState returns the state-upgrade functions for prior schema versions.
//
// Schema v0 → v1 (see CHANGELOG):
//   - passcode.max_failed_attempts: Int64 → String.
//     On iOS this field carries the sentinel "None", which cannot be
//     represented as an integer. Values stored in v0 state are numeric.
//   - passcode.pin_history: Int64 → String.
//     On iOS the canonical API key is PasscodeHistory and it is serialised as
//     a string. We harmonise the Terraform type with the iOS wire shape so
//     both platforms can round-trip without silently dropping the value.
func (r *ProfileResource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: {
			StateUpgrader: upgradePasscodeIntToStringV0toV1,
		},
	}
}

// upgradePasscodeIntToStringV0toV1 coerces the v0 passcode.max_failed_attempts
// and passcode.pin_history values from JSON numbers to JSON strings before
// decoding into the v1 schema. PriorSchema is intentionally left nil so we
// can do a narrow raw-JSON rewrite instead of duplicating the full schema.
func upgradePasscodeIntToStringV0toV1(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	if req.RawState == nil || len(req.RawState.JSON) == 0 {
		return
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(req.RawState.JSON, &raw); err != nil {
		resp.Diagnostics.AddError(
			"State upgrade: cannot decode v0 state JSON",
			err.Error(),
		)
		return
	}

	if passcode, ok := raw["passcode"].(map[string]interface{}); ok {
		for _, key := range []string{"max_failed_attempts", "pin_history"} {
			switch v := passcode[key].(type) {
			case float64:
				passcode[key] = strconv.FormatFloat(v, 'f', -1, 64)
			case json.Number:
				passcode[key] = v.String()
			case int:
				passcode[key] = strconv.Itoa(v)
			case int64:
				passcode[key] = strconv.FormatInt(v, 10)
				// string or nil: leave as-is.
			}
		}
	}

	newJSON, err := json.Marshal(raw)
	if err != nil {
		resp.Diagnostics.AddError(
			"State upgrade: cannot re-encode coerced state",
			err.Error(),
		)
		return
	}

	newType := resp.State.Schema.Type().TerraformType(ctx)
	newVal, err := (&tfprotov6.RawState{JSON: newJSON}).UnmarshalWithOpts(newType, tfprotov6.UnmarshalOpts{
		ValueFromJSONOpts: tftypes.ValueFromJSONOpts{
			IgnoreUndefinedAttributes: true,
		},
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"State upgrade: cannot decode coerced state into v1 schema",
			err.Error(),
		)
		return
	}

	resp.State.Raw = newVal
}
