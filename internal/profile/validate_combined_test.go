package profile

import (
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

// The ValidateConfig checks gate only on their own attribute read, so an
// invalid value in one attribute must not hide another attribute's error.

func TestValidateConfig_Combined_BadAssignmentTypeAndBadProfileScope_BothReported(t *testing.T) {
	t.Parallel()
	resp := runValidateConfig(t, sdk.PlatformAppleOsX, "Bogus", "Bogus")
	if len(diagErrorsAtPath(resp, "assignment_type")) == 0 {
		t.Fatalf("expected an assignment_type error, got: %s", diagSummaries(resp))
	}
	if len(diagErrorsAtPath(resp, "profile_scope")) == 0 {
		t.Fatalf("expected a profile_scope error, got: %s", diagSummaries(resp))
	}
}
