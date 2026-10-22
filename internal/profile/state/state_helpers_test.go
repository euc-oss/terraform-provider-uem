package state

import (
	"testing"

	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// internal-ticket removed mergeRestrictionsMedia and mergeMediaAccessPtr (row #95
// of the B16 audit): UEM's response for Media/RecordableDisc/BurnSupport is
// no longer rehydrated from the prior state when the API omits it -- that
// fabricated a media-access block (including a CD/DVD read_only value) out
// of thin air, in tension with the CD/DVD read_only=true validator. The
// tests below assert the new pass-through contract on
// MergeRestrictionsWithPriorState directly: only Desktop still gets
// restored from prior state when the API omits it (row #92, kept, (a)).

// A server that drops Media/RecordableDisc entirely (nil) is no longer
// rehydrated from the prior state -- this used to be
// TestMergeRestrictionsMedia_RehydratesRecordableDiscFromState, asserting
// the opposite of the current contract.
func TestMergeRestrictionsWithPriorState_MediaNotRehydratedFromState(t *testing.T) {
	t.Parallel()

	state := &profilemodels.RestrictionsModel{
		Media: &profilemodels.RestrictionsMediaModel{
			RecordableDisc: &profilemodels.RestrictionsBurnSupportModel{
				BurnSupport: &profilemodels.RestrictionsMediaAccessModel{
					Allow: types.BoolValue(false),
				},
			},
		},
	}
	api := &profilemodels.RestrictionsModel{} // server omitted Media entirely

	got := MergeRestrictionsWithPriorState(api, state)

	if got != nil && got.Media != nil {
		t.Errorf("Media = %+v, want nil (no longer fabricated from prior state)", got.Media)
	}
}

// Desktop is the one sub-block still restored from the prior state when the
// API omits it (R + GGS:43, confirmed live: Desktop is null on GET unless
// locked).
func TestMergeRestrictionsWithPriorState_DesktopStillRehydratedFromState(t *testing.T) {
	t.Parallel()

	state := &profilemodels.RestrictionsModel{
		Desktop: &profilemodels.RestrictionsDesktopModel{
			LockDesktopPicture: types.BoolValue(true),
		},
	}
	api := &profilemodels.RestrictionsModel{Applications: &profilemodels.RestrictionsApplicationsModel{}}

	got := MergeRestrictionsWithPriorState(api, state)

	if got == nil || got.Desktop == nil {
		t.Fatal("Desktop was not restored from prior state")
	}
	if got.Desktop.LockDesktopPicture.ValueBool() != true {
		t.Errorf("Desktop.LockDesktopPicture = %v, want true (restored from prior state)", got.Desktop.LockDesktopPicture)
	}
}

// When the API does return Media, it is stored exactly as returned -- no
// merge against the prior state's RecordableDisc/BurnSupport values.
func TestMergeRestrictionsWithPriorState_MediaPassesThroughAPIValue(t *testing.T) {
	t.Parallel()

	state := &profilemodels.RestrictionsModel{
		Media: &profilemodels.RestrictionsMediaModel{
			RecordableDisc: &profilemodels.RestrictionsBurnSupportModel{
				BurnSupport: &profilemodels.RestrictionsMediaAccessModel{
					Allow: types.BoolValue(false),
				},
			},
		},
	}
	api := &profilemodels.RestrictionsModel{
		Media: &profilemodels.RestrictionsMediaModel{
			AutoEjectMedia: types.BoolValue(true),
		},
	}

	got := MergeRestrictionsWithPriorState(api, state)

	if got == nil || got.Media == nil {
		t.Fatal("expected Media to pass through")
	}
	if got.Media.AutoEjectMedia.ValueBool() != true {
		t.Errorf("AutoEjectMedia = %v, want true (API value)", got.Media.AutoEjectMedia)
	}
	if got.Media.RecordableDisc != nil {
		t.Errorf("RecordableDisc = %+v, want nil (not fabricated from prior state)", got.Media.RecordableDisc)
	}
}
