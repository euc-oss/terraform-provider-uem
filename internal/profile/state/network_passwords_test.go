package state

import (
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// UEM does not return network secrets faithfully, so the read path must
// never store what UEM sends for them; only the user's prior value is kept.
func TestMapAppleOsXNetworkList_DoesNotReadPasswords(t *testing.T) {
	got := mapAppleOsXNetworkList([]sdk.AppleOsXNetworkPayloadEntityV2{{
		ServiceSetIdentifier: "ssid",
		Password:             "value-from-uem",
		UserPassword:         "user-from-uem",
		ProxyPassword:        "proxy-from-uem",
	}})
	if len(got) != 1 {
		t.Fatalf("expected 1 item, got %d", len(got))
	}
	if !got[0].Password.IsNull() || !got[0].UserPassword.IsNull() || !got[0].ProxyPassword.IsNull() {
		t.Fatalf("expected passwords not read from UEM (null), got %v / %v / %v", got[0].Password, got[0].UserPassword, got[0].ProxyPassword)
	}
}

func TestCarryNetworkPasswords_KeepsPriorValues(t *testing.T) {
	api := []NetworkItemModel{{Password: types.StringNull(), UserPassword: types.StringNull(), ProxyPassword: types.StringNull()}}
	prior := []NetworkItemModel{{Password: types.StringValue("psk"), UserPassword: types.StringValue("u"), ProxyPassword: types.StringValue("p")}}

	got := carryNetworkPasswords(api, prior)
	if got[0].Password.ValueString() != "psk" || got[0].UserPassword.ValueString() != "u" || got[0].ProxyPassword.ValueString() != "p" {
		t.Fatalf("expected prior passwords carried, got %v / %v / %v", got[0].Password, got[0].UserPassword, got[0].ProxyPassword)
	}
}

func TestCarryNetworkPasswords_ImportLeavesNull(t *testing.T) {
	api := []NetworkItemModel{{Password: types.StringNull()}}
	got := carryNetworkPasswords(api, nil)
	if !got[0].Password.IsNull() {
		t.Fatalf("expected null password on import (no prior state), got %v", got[0].Password)
	}
}

func TestCarryCredentialSecrets_ByNameThenIndex(t *testing.T) {
	api := []CredentialItemModel{
		{CredentialName: types.StringValue("b")},
		{CredentialName: types.StringValue("Certificate #1")},
	}
	prior := []CredentialItemModel{
		{CredentialName: types.StringValue("a"), CertificatePayload: types.StringValue("pa"), CertificatePassword: types.StringValue("wa")},
		{CredentialName: types.StringValue("b"), CertificatePayload: types.StringValue("pb"), CertificatePassword: types.StringValue("wb")},
	}
	got := carryCredentialSecrets(api, prior)
	if got[0].CertificatePayload.ValueString() != "pb" || got[0].CertificatePassword.ValueString() != "wb" {
		t.Fatalf("name match: got %v / %v", got[0].CertificatePayload, got[0].CertificatePassword)
	}
	// "Certificate #1" has no name match, so it falls back to index 1.
	if got[1].CertificatePayload.ValueString() != "pb" {
		t.Fatalf("index fallback: got %v", got[1].CertificatePayload)
	}
}

func TestCarryCredentialSecrets_ImportLeavesNull(t *testing.T) {
	got := carryCredentialSecrets([]CredentialItemModel{{CredentialName: types.StringValue("x")}}, nil)
	if !got[0].CertificatePayload.IsNull() || !got[0].CertificatePassword.IsNull() {
		t.Fatalf("expected null secrets on import, got %v / %v", got[0].CertificatePayload, got[0].CertificatePassword)
	}
}
