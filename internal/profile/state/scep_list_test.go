package state

import (
	"context"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// A live-shaped SCEP read (placeholder ids) maps field for field; an empty
// or missing value is null, and a second entry keeps its own values.
func TestMapAppleOsXScepList(t *testing.T) {
	got := mapAppleOsXScepList([]sdk.AppleOsXScepPayloadEntityV2{
		{Name: "SCEP #1", CredentialSource: "DefinedCA", CertificateAuthorityID: sdk.IntPtr(1001), CertificateTemplateID: sdk.IntPtr(2002), AllowExportFromKeyChain: sdk.BoolPtr(true)},
		{Name: "SCEP #2", CertificateAuthorityID: sdk.IntPtr(0), AllowExportFromKeyChain: sdk.BoolPtr(false),
			IdentityPreference: &sdk.MacOsScepIdentityPreferencePayloadV2Model{Names: []string{"a", "b"}}},
	})
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
	a, b := got[0], got[1]
	if a.Name.ValueString() != "SCEP #1" || a.CredentialSource.ValueString() != "DefinedCA" ||
		a.CertificateAuthorityID.ValueInt64() != 1001 || a.CertificateTemplateID.ValueInt64() != 2002 ||
		!a.AllowExportFromKeyChain.ValueBool() || a.IdentityPreference != nil {
		t.Errorf("entry 0 = %+v", a)
	}
	if !b.CredentialSource.IsNull() || !b.CertificateTemplateID.IsNull() {
		t.Errorf("missing values must be null: %+v", b)
	}
	if b.CertificateAuthorityID.IsNull() || b.CertificateAuthorityID.ValueInt64() != 0 {
		t.Errorf("a returned 0 is a value, not null: %v", b.CertificateAuthorityID)
	}
	if b.AllowExportFromKeyChain.IsNull() || b.AllowExportFromKeyChain.ValueBool() {
		t.Errorf("a returned false is a value: %v", b.AllowExportFromKeyChain)
	}
	var names []string
	if b.IdentityPreference == nil || b.IdentityPreference.Names.ElementsAs(context.Background(), &names, false).HasError() || len(names) != 2 {
		t.Errorf("identity_preference.names = %+v", b.IdentityPreference)
	}
	if mapAppleOsXScepList(nil) != nil {
		t.Error("no SCEP payload must map to null")
	}
}

// readAppleOsXIntoState fills scep_list from the read.
func TestReadAppleOsXIntoState_ScepList(t *testing.T) {
	data := &ProfileResourceModel{}
	readAppleOsXIntoState(context.Background(), data, &sdk.AppleOsXDeviceProfileEntityV2{
		ScepList: []sdk.AppleOsXScepPayloadEntityV2{{Name: "SCEP #1"}},
	})
	if len(data.ScepList) != 1 || data.ScepList[0].Name != types.StringValue("SCEP #1") {
		t.Errorf("scep_list = %+v", data.ScepList)
	}
}
