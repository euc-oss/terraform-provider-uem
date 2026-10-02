package state

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/internal-app/mac/models"
)

func TestSetMinimalStateSuccess(t *testing.T) {
	var data tf.MacApplicationResourceModel
	err := SetMinimalState(&data, 123, "9af645a8-fef3-3e6d-3408-5cc69e0937d4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.ID.ValueInt32() != 123 {
		t.Fatalf("unexpected id: %d", data.ID.ValueInt32())
	}
	if data.UUID.ValueString() != "9af645a8-fef3-3e6d-3408-5cc69e0937d4" {
		t.Fatalf("unexpected uuid: %q", data.UUID.ValueString())
	}
}

func TestSetMinimalStateValidationErrors(t *testing.T) {
	var data tf.MacApplicationResourceModel
	if err := SetMinimalState(&data, 0, "9af645a8-fef3-3e6d-3408-5cc69e0937d4"); err == nil {
		t.Fatal("expected error for invalid id")
	}
}

// TestSetMinimalStateTolerantOfUnresolvedUUID covers B16 decision table row
// #102 (CORRECT). UEM source: AirWatch API/AW.Mam.Api/AW.Mam.Api.Model.Mappers/InternalAppModelMapper.cs:179-180,
// AirWatch API/AW.Mam.Api/AW.Mam.Api.Model/Apps/InternalAppModel.cs:413-416
// (canonical Q10): Id is always set on a successful GET, but Uuid resolution
// can legitimately fail and come back null/blank — "not always both
// populated". A blank or malformed uuid must not fail the whole refresh;
// only id is required. This provider keeps the prior state's uuid (chosen
// over forcing null): id remains the authoritative identity either way, and
// clearing uuid to null would only produce a spurious diff/replacement risk
// for no benefit.
func TestSetMinimalStateTolerantOfUnresolvedUUID(t *testing.T) {
	t.Run("blank uuid: id still set, no error, prior uuid kept", func(t *testing.T) {
		data := tf.MacApplicationResourceModel{UUID: types.StringValue("prior-uuid")}
		if err := SetMinimalState(&data, 123, ""); err != nil {
			t.Fatalf("unexpected error for blank uuid: %v", err)
		}
		if data.ID.ValueInt32() != 123 {
			t.Fatalf("unexpected id: %d", data.ID.ValueInt32())
		}
		if data.UUID.ValueString() != "prior-uuid" {
			t.Fatalf("expected prior uuid to be kept, got %q", data.UUID.ValueString())
		}
	})

	t.Run("malformed uuid: id still set, no error, prior uuid kept", func(t *testing.T) {
		data := tf.MacApplicationResourceModel{UUID: types.StringValue("prior-uuid")}
		if err := SetMinimalState(&data, 123, "not-a-uuid"); err != nil {
			t.Fatalf("unexpected error for malformed uuid: %v", err)
		}
		if data.UUID.ValueString() != "prior-uuid" {
			t.Fatalf("expected prior uuid to be kept, got %q", data.UUID.ValueString())
		}
	})

	t.Run("blank uuid with no prior state: id still set, no error, uuid stays null", func(t *testing.T) {
		var data tf.MacApplicationResourceModel
		if err := SetMinimalState(&data, 123, ""); err != nil {
			t.Fatalf("unexpected error for blank uuid: %v", err)
		}
		if !data.UUID.IsNull() {
			t.Fatalf("expected uuid to stay null with no prior state, got %q", data.UUID.ValueString())
		}
	})

	t.Run("valid uuid still overwrites a prior uuid", func(t *testing.T) {
		data := tf.MacApplicationResourceModel{UUID: types.StringValue("prior-uuid")}
		if err := SetMinimalState(&data, 123, "9af645a8-fef3-3e6d-3408-5cc69e0937d4"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if data.UUID.ValueString() != "9af645a8-fef3-3e6d-3408-5cc69e0937d4" {
			t.Fatalf("expected resolved uuid to overwrite prior, got %q", data.UUID.ValueString())
		}
	})
}
