package state

import (
	"testing"

	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Regression: when state declared recordable_disc.burn_support but the UEM
// server omitted RecordableDisc from its GET response, the merge previously
// left api.RecordableDisc as nil — Terraform then raised "inconsistent result
// after apply". The merge must rehydrate the block from prior state.
func TestMergeRestrictionsMedia_RehydratesRecordableDiscFromState(t *testing.T) {
	state := &profilemodels.RestrictionsMediaModel{
		RecordableDisc: &profilemodels.RestrictionsBurnSupportModel{
			BurnSupport: &profilemodels.RestrictionsMediaAccessModel{
				Allow:        types.BoolValue(false),
				Authenticate: types.BoolValue(false),
				ReadOnly:     types.BoolValue(false),
			},
		},
	}
	api := &profilemodels.RestrictionsMediaModel{} // server dropped RecordableDisc

	mergeRestrictionsMedia(api, state)

	if api.RecordableDisc == nil {
		t.Fatal("api.RecordableDisc was not rehydrated from state")
	}
	if api.RecordableDisc.BurnSupport == nil {
		t.Fatal("api.RecordableDisc.BurnSupport was not rehydrated from state")
	}
	bs := api.RecordableDisc.BurnSupport
	if bs.Allow.IsNull() || bs.Allow.ValueBool() != false {
		t.Errorf("Allow: want false, got %v", bs.Allow)
	}
	if bs.Authenticate.IsNull() || bs.Authenticate.ValueBool() != false {
		t.Errorf("Authenticate: want false, got %v", bs.Authenticate)
	}
	if bs.ReadOnly.IsNull() || bs.ReadOnly.ValueBool() != false {
		t.Errorf("ReadOnly: want false, got %v", bs.ReadOnly)
	}
}

// State-nil must continue to clear the api block (user removed the setting).
func TestMergeRestrictionsMedia_StateNilClearsRecordableDisc(t *testing.T) {
	state := &profilemodels.RestrictionsMediaModel{}
	api := &profilemodels.RestrictionsMediaModel{
		RecordableDisc: &profilemodels.RestrictionsBurnSupportModel{
			BurnSupport: &profilemodels.RestrictionsMediaAccessModel{
				Allow: types.BoolValue(true),
			},
		},
	}

	mergeRestrictionsMedia(api, state)

	if api.RecordableDisc != nil {
		t.Errorf("expected RecordableDisc cleared when state is nil, got %+v", api.RecordableDisc)
	}
}

// mergeMediaAccessPtr must rehydrate a dropped media-access struct from state.
// Same class of bug as recordable_disc, one level shallower.
func TestMergeMediaAccessPtr_RehydratesFromState(t *testing.T) {
	state := &profilemodels.RestrictionsMediaAccessModel{
		Allow:        types.BoolValue(true),
		Authenticate: types.BoolValue(false),
		ReadOnly:     types.BoolValue(true),
	}
	var api *profilemodels.RestrictionsMediaAccessModel

	mergeMediaAccessPtr(&api, state)

	if api == nil {
		t.Fatal("api was not rehydrated from state")
	}
	if api.Allow.ValueBool() != true {
		t.Errorf("Allow: want true, got %v", api.Allow)
	}
	if api.Authenticate.ValueBool() != false {
		t.Errorf("Authenticate: want false, got %v", api.Authenticate)
	}
	if api.ReadOnly.ValueBool() != true {
		t.Errorf("ReadOnly: want true, got %v", api.ReadOnly)
	}
}

func TestMergeMediaAccessPtr_StateNilClearsApi(t *testing.T) {
	api := &profilemodels.RestrictionsMediaAccessModel{Allow: types.BoolValue(true)}
	mergeMediaAccessPtr(&api, nil)
	if api != nil {
		t.Errorf("expected api cleared when state is nil, got %+v", api)
	}
}
