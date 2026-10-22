package state

import (
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// B42: creating a uem_profile with disk_encryption.filevault2 set and
// disk_encryption.mcx left unset failed apply with "Provider produced
// inconsistent result after apply: .disk_encryption.mcx: was null, but now
// {\"destroy_fv_key_on_standby\":false}" (live, as<internal-env> UEM 26.2) -- UEM's
// DiskEncryption ctor always instantiates a non-null MCX sub-object with its
// own default, even when the whole block is left out of the request.
//
// The mapper, mapAppleOsXDiskEncryptionMCX, must map UEM's
// DestroyFVKeyOnStandby value verbatim, including the live-observed false
// default, so the schema fix (mcx and its leaf now Optional+Computed with
// UseStateForUnknown, and the model field now a types.Object rather than a
// *DiskEncryptionMCXModel pointer -- see the follow-up fix for this same issue)
// has a faithful value to land in state.
func TestMapAppleOsXDiskEncryptionMCX_FalseValueMapsVerbatim(t *testing.T) {
	t.Parallel()

	mcx := &sdk.AppleOsXDiskEncryptionMCXPayloadEntityV2{
		DestroyFVKeyOnStandby: sdk.BoolPtr(false),
	}

	got := mapAppleOsXDiskEncryptionMCX(mcx)

	if got.IsNull() || got.IsUnknown() {
		t.Fatalf("expected a known, non-null MCX object for a non-nil API value, got %v", got)
	}
	destroy, ok := got.Attributes()["destroy_fv_key_on_standby"].(types.Bool)
	if !ok {
		t.Fatal("expected destroy_fv_key_on_standby attribute of type types.Bool")
	}
	if destroy.IsNull() || destroy.IsUnknown() {
		t.Fatalf("DestroyFVKeyOnStandby = %v, want a known, non-null false", destroy)
	}
	if destroy.ValueBool() != false {
		t.Errorf("DestroyFVKeyOnStandby = %v, want false (UEM's live-observed default)", destroy.ValueBool())
	}
}

func TestMapAppleOsXDiskEncryptionMCX_TrueValueMapsVerbatim(t *testing.T) {
	t.Parallel()

	mcx := &sdk.AppleOsXDiskEncryptionMCXPayloadEntityV2{
		DestroyFVKeyOnStandby: sdk.BoolPtr(true),
	}

	got := mapAppleOsXDiskEncryptionMCX(mcx)

	destroy, ok := got.Attributes()["destroy_fv_key_on_standby"].(types.Bool)
	if !ok {
		t.Fatal("expected destroy_fv_key_on_standby attribute of type types.Bool")
	}
	if destroy.ValueBool() != true {
		t.Errorf("DestroyFVKeyOnStandby = %v, want true", destroy.ValueBool())
	}
}

// mapAppleOsXDiskEncryptionMCX(nil) -- UEM sent no MCX section at all -- maps
// to ObjectNull. A present-but-partial API value (its own
// DestroyFVKeyOnStandby pointer nil) now maps to a KNOWN object with a null
// leaf, not to an overall null: only "the API genuinely sent nothing" is
// ObjectNull now. Collapsing the two together (the pre-B42-follow-up
// behavior) is what made a genuinely unknown create-time plan indistinguishable
// from "UEM sent an empty section", which is not the same thing.
func TestMapAppleOsXDiskEncryptionMCX_Nil(t *testing.T) {
	t.Parallel()

	got := mapAppleOsXDiskEncryptionMCX(nil)
	if !got.IsNull() {
		t.Errorf("mapAppleOsXDiskEncryptionMCX(nil) = %v, want ObjectNull", got)
	}

	gotEmpty := mapAppleOsXDiskEncryptionMCX(&sdk.AppleOsXDiskEncryptionMCXPayloadEntityV2{})
	if gotEmpty.IsNull() {
		t.Fatalf("mapAppleOsXDiskEncryptionMCX(empty struct) = %v, want a known object with a null leaf, not ObjectNull", gotEmpty)
	}
	destroy, ok := gotEmpty.Attributes()["destroy_fv_key_on_standby"].(types.Bool)
	if !ok {
		t.Fatal("expected destroy_fv_key_on_standby attribute of type types.Bool")
	}
	if !destroy.IsNull() {
		t.Errorf("mapAppleOsXDiskEncryptionMCX(empty struct).destroy_fv_key_on_standby = %v, want null (DestroyFVKeyOnStandby unset)", destroy)
	}
}

// mapAppleOsXDiskEncryption still collapses the whole DiskEncryptionModel to
// nil when airwatch, filevault2, and mcx are ALL absent -- mcx.IsNull() now
// stands in for the old "mcx == nil" pointer check.
func TestMapAppleOsXDiskEncryption_AllAbsentCollapsesToNil(t *testing.T) {
	t.Parallel()

	got := mapAppleOsXDiskEncryption(&sdk.AppleOsXDiskEncryptionPayloadEntityV2{})
	if got != nil {
		t.Errorf("mapAppleOsXDiskEncryption(all absent) = %+v, want nil", got)
	}
}

// mapAppleOsXDiskEncryption must not collapse to nil when mcx is the ONLY
// present section -- a present-but-partial mcx (per the Nil test above) is now
// a known, non-null object, so DiskEncryptionModel.MCX carries it even though
// AirWatch and FileVault are both nil.
func TestMapAppleOsXDiskEncryption_MCXOnlyIsNotCollapsed(t *testing.T) {
	t.Parallel()

	got := mapAppleOsXDiskEncryption(&sdk.AppleOsXDiskEncryptionPayloadEntityV2{
		DiskEncryptionMCX: &sdk.AppleOsXDiskEncryptionMCXPayloadEntityV2{
			DestroyFVKeyOnStandby: sdk.BoolPtr(false),
		},
	})
	if got == nil {
		t.Fatal("expected a non-nil DiskEncryptionModel when mcx is present")
	}
	if got.MCX.IsNull() {
		t.Error("expected a known, non-null MCX")
	}
}
