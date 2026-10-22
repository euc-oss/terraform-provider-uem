package profile

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestOrgGroupIDApplyTimeGuard covers internal-task's apply-time defense: Create
// and Update must never silently omit General.ManagedLocationGroupID (a
// non-nullable field per canonical rules general-ogid Q1/Q2) by reaching the
// builder with a null or unknown org_group_id.
func TestOrgGroupIDApplyTimeGuard(t *testing.T) {
	t.Parallel()

	t.Run("known non-null value passes", func(t *testing.T) {
		t.Parallel()
		if diag := orgGroupIDApplyTimeGuard(types.StringValue("138883")); diag != nil {
			t.Errorf("expected no diagnostic for a known org_group_id, got: %s", diag.Summary())
		}
	})

	t.Run("null value is rejected", func(t *testing.T) {
		t.Parallel()
		diag := orgGroupIDApplyTimeGuard(types.StringNull())
		if diag == nil {
			t.Fatal("expected a diagnostic for a null org_group_id at apply time, got none")
		}
		if diag.Summary() == "" {
			t.Error("expected a non-empty diagnostic summary")
		}
	})

	t.Run("unknown value is rejected", func(t *testing.T) {
		t.Parallel()
		diag := orgGroupIDApplyTimeGuard(types.StringUnknown())
		if diag == nil {
			t.Fatal("expected a diagnostic for an unknown org_group_id at apply time, got none")
		}
	})
}
