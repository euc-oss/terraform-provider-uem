package state

import (
	"testing"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/mac/models"
)

func TestSetMinimalStateSuccess(t *testing.T) {
	var data tf.MacApplicationResourceModel
	err := SetMinimalState(&data, 123, "uuid-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.ID.ValueInt32() != 123 {
		t.Fatalf("unexpected id: %d", data.ID.ValueInt32())
	}
	if data.UUID.ValueString() != "uuid-123" {
		t.Fatalf("unexpected uuid: %q", data.UUID.ValueString())
	}
}

func TestSetMinimalStateValidationErrors(t *testing.T) {
	var data tf.MacApplicationResourceModel
	if err := SetMinimalState(&data, 0, "uuid-123"); err == nil {
		t.Fatal("expected error for invalid id")
	}
	if err := SetMinimalState(&data, 1, "   "); err == nil {
		t.Fatal("expected error for invalid uuid")
	}
}
