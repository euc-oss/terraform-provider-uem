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
	if got := KeepStateString(types.StringUnknown(), state); got.ValueString() != "state" {
		t.Fatalf("expected state fallback for unknown, got %q", got.ValueString())
	}
	if got := KeepStateString(types.StringValue("api"), state); got.ValueString() != "api" {
		t.Fatalf("expected api value, got %q", got.ValueString())
	}
}

func TestKeepStateBool(t *testing.T) {
	state := types.BoolValue(true)
	if got := KeepStateBool(types.BoolNull(), state); !got.ValueBool() {
		t.Fatal("expected state fallback for null")
	}
	if got := KeepStateBool(types.BoolUnknown(), state); !got.ValueBool() {
		t.Fatal("expected state fallback for unknown")
	}
	if got := KeepStateBool(types.BoolValue(false), state); got.ValueBool() {
		t.Fatal("expected api value false")
	}
}

func TestKeepStateInt64(t *testing.T) {
	state := types.Int64Value(9)
	if got := KeepStateInt64(types.Int64Null(), state); got.ValueInt64() != 9 {
		t.Fatalf("expected state fallback, got %d", got.ValueInt64())
	}
	if got := KeepStateInt64(types.Int64Unknown(), state); got.ValueInt64() != 9 {
		t.Fatalf("expected state fallback for unknown, got %d", got.ValueInt64())
	}
	if got := KeepStateInt64(types.Int64Value(3), state); got.ValueInt64() != 3 {
		t.Fatalf("expected api value, got %d", got.ValueInt64())
	}
}
