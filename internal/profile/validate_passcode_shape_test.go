package profile

import (
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestValidateConfig_PasscodeMacOSRequireFalse_NoDiagnostics proves the
// provider is faithful to UEM rather than inferring an unwritten
// business rule: on macOS (AppleOsX), a passcode block with
// require_passcode_on_device = false, allow_simple_value = true,
// require_alphanumeric_value = false, and minimum_passcode_length = 0 (the
// shape a real imported console profile produced, live-verified on
// paul-2609/26.9) must produce no diagnostics at all. ValidateConfig no
// longer applies a validator inferred from live observation instead of UEM
// source; the provider now sends and reads passcode fields as-is.
func TestValidateConfig_PasscodeMacOSRequireFalse_NoDiagnostics(t *testing.T) {
	t.Parallel()

	resp := runValidateConfigWithPasscode(t, sdk.PlatformAppleOsX, passcodeVal(map[string]tftypes.Value{
		"require_passcode_on_device": boolVal(false),
		"allow_simple_value":         boolVal(true),
		"require_alphanumeric_value": boolVal(false),
		"minimum_passcode_length":    int64Val(0),
	}))

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no diagnostics for a faithful macOS passcode import shape (require_passcode_on_device=false, allow_simple_value=true, require_alphanumeric_value=false, minimum_passcode_length=0), got: %s", diagSummaries(resp))
	}
	if len(resp.Diagnostics) != 0 {
		t.Fatalf("expected zero diagnostics (not even a warning) for this shape, got: %v", resp.Diagnostics)
	}
}
