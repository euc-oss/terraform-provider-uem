package state

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestKeepStateString(t *testing.T) {
	state := types.StringValue("state")
	if got := KeepStateString(types.StringNull(), state); got.ValueString() != "state" {
		t.Fatalf("expected state fallback, got %q", got.ValueString())
	}
	if got := KeepStateString(types.StringValue("api"), state); got.ValueString() != "api" {
		t.Fatalf("expected api value, got %q", got.ValueString())
	}
}

func TestAppConfigTypeToAPIString(t *testing.T) {
	v, err := appConfigTypeToAPIString("boolean")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != "3" {
		t.Fatalf("expected BOOLEAN=\"3\", got %q", v)
	}
}

func TestAppConfigTypeFromAPIString(t *testing.T) {
	got := appConfigTypeFromAPIString("3")
	if got.ValueString() != "BOOLEAN" {
		t.Fatalf("expected BOOLEAN, got %q", got.ValueString())
	}
}
