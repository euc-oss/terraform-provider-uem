package state

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/purchased-app/assignment/models"
)

func TestReadAPIIntoState_Description_PriorNullServerEmpty_KeepsNull(t *testing.T) {
	got := readSingleAssignment(t, priorRule(t, types.StringNull(), nil), apiRule("", nil))
	if !got.Distribution.Description.IsNull() {
		t.Fatalf("expected null description, got %v", got.Distribution.Description)
	}
}

func TestReadAPIIntoState_Description_NoPriorServerEmpty_KeepsNull(t *testing.T) {
	got := readSingleAssignment(t, &tf.PurchasedAppAssignmentRuleModel{}, apiRule("", nil))
	if !got.Distribution.Description.IsNull() {
		t.Fatalf("expected null description on import of empty server value, got %v", got.Distribution.Description)
	}
}

func TestReadAPIIntoState_Description_PriorNonNullServerEmpty_TakesEmpty(t *testing.T) {
	got := readSingleAssignment(t, priorRule(t, types.StringValue(""), nil), apiRule("", nil))
	if got.Distribution.Description.IsNull() || got.Distribution.Description.ValueString() != "" {
		t.Fatalf("expected empty-string description, got %v", got.Distribution.Description)
	}
}

func TestReadAPIIntoState_Description_PriorNonNullServerText_TakesServer(t *testing.T) {
	got := readSingleAssignment(t, priorRule(t, types.StringValue("old"), nil), apiRule("some text", nil))
	if got.Distribution.Description.ValueString() != "some text" {
		t.Fatalf("expected server description, got %v", got.Distribution.Description)
	}
}

func TestReadAPIIntoState_Description_PriorNullServerText_SurfacesDrift(t *testing.T) {
	got := readSingleAssignment(t, priorRule(t, types.StringNull(), nil), apiRule("set out of band", nil))
	if got.Distribution.Description.ValueString() != "set out of band" {
		t.Fatalf("expected server description drift to surface, got %v", got.Distribution.Description)
	}
}
