package state

import (
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestReadProfileIntoState_AppleOsXCredentialsList_CreateReadback_EchoedFieldsReadBackSecretsCarried
// pins the faithful-readback fix for the credentials_list nested attributes
// (B16 follow-up): certificate_authority, certificate_template,
// allow_access_to_all_applications and key_is_extractable are attributes UEM
// does echo back on every read (live-confirmed 2026-09-25: 0, 0, false, true
// when unset), so per the project's faithfulness rule (carry only what UEM
// does not echo; everything else is read back as-is) they must land in state
// exactly as UEM returned them, not be forced back to whatever the plan
// carried forward. Only certificate_payload and certificate_password --
// which UEM never returns at all -- are carried from prior state by
// carryCredentialSecrets (internal/profile/state/read_mappers.go).
//
// A prior commit (af93677f3, reverted here) carried all six fields forward
// unconditionally, which would have hidden real drift on the four UEM does
// echo. The schema side of this fix (internal/profile/resource.go) makes
// those four Optional+Computed with UseStateForUnknown, so an unset
// configuration plans unknown and takes UEM's readback instead of a
// hard-coded null, avoiding the "inconsistent result after apply" failure
// this test's earlier form reproduced (see git history on this file for
// that reproduction and its evidence).
func TestReadProfileIntoState_AppleOsXCredentialsList_CreateReadback_EchoedFieldsReadBackSecretsCarried(t *testing.T) {
	t.Parallel()

	// "data" plays the exact role it has in ProfileResource.Create
	// (internal/profile/resource_crud.go:148,210,213): the just-planned
	// model, unmodified before the readback, passed as ReadProfileIntoState's
	// "data" argument whose CredentialsList becomes carryCredentialSecrets's
	// "prior".
	data := &ProfileResourceModel{
		CredentialsList: []CredentialItemModel{
			{
				CredentialSource:    types.StringValue("Upload"),
				CredentialName:      types.StringValue("b16np-cred-1"),
				CertificatePayload:  types.StringValue("throwaway-base64-payload"),
				CertificatePassword: types.StringValue("throwaway-pw"),
				CertificateID:       types.Int64Unknown(),
				// Optional+Computed, omitted from config: plans unknown, so
				// the plan modifiers/schema resolve it to UEM's readback.
				CertificateAuthority:         types.Int64Unknown(),
				CertificateTemplate:          types.Int64Unknown(),
				AllowAccessToAllApplications: types.BoolUnknown(),
				KeyIsExtractable:             types.BoolUnknown(),
			},
		},
	}

	// The GET readback UEM returns for the freshly created profile: an
	// Upload credential, live-confirmed to come back with concrete
	// placeholder values for all four attributes even though none was
	// configured.
	ent := &sdk.AppleOsXDeviceProfileEntityV2{
		CredentialsList: []sdk.AppleOsXCredentialPayloadEntityV2{
			{
				CredentialSource:             "Upload",
				CredentialName:               "b16np-cred-1",
				CertificateID:                sdk.IntPtr(439543),
				CertificateAuthority:         sdk.IntPtr(0),
				CertificateTemplate:          sdk.IntPtr(0),
				AllowAccessToAllApplications: sdk.BoolPtr(false),
				KeyIsExtractable:             sdk.BoolPtr(true),
			},
		},
	}

	ReadProfileIntoState(t.Context(), data, &sdk.ProfileResult{AppleOsX: ent})

	if len(data.CredentialsList) != 1 {
		t.Fatalf("CredentialsList len = %d, want 1", len(data.CredentialsList))
	}
	got := data.CredentialsList[0]

	// The secrets UEM never echoes stay carried from prior state.
	if got.CertificatePayload.ValueString() != "throwaway-base64-payload" {
		t.Errorf("CertificatePayload = %v, want carried from the plan", got.CertificatePayload)
	}
	if got.CertificatePassword.ValueString() != "throwaway-pw" {
		t.Errorf("CertificatePassword = %v, want carried from the plan", got.CertificatePassword)
	}

	// The faithful-readback fix: these four are echoed by UEM, so they must
	// land in state exactly as UEM returned them, not carried from prior
	// state and not left null.
	if got.CertificateAuthority.IsNull() || got.CertificateAuthority.ValueInt64() != 0 {
		t.Errorf("CertificateAuthority = %v, want 0 (read back from UEM)", got.CertificateAuthority)
	}
	if got.CertificateTemplate.IsNull() || got.CertificateTemplate.ValueInt64() != 0 {
		t.Errorf("CertificateTemplate = %v, want 0 (read back from UEM)", got.CertificateTemplate)
	}
	if got.AllowAccessToAllApplications.IsNull() || got.AllowAccessToAllApplications.ValueBool() != false {
		t.Errorf("AllowAccessToAllApplications = %v, want false (read back from UEM)", got.AllowAccessToAllApplications)
	}
	if got.KeyIsExtractable.IsNull() || got.KeyIsExtractable.ValueBool() != true {
		t.Errorf("KeyIsExtractable = %v, want true (read back from UEM)", got.KeyIsExtractable)
	}
}
