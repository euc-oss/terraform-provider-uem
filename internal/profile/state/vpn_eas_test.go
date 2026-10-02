package state

import (
	"context"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/types"

	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"
)

// A live-shaped VPN/EAS read (placeholder values; UEM masks the secrets as
// *****): settings map field for field, the masked secrets never reach
// state, and on import (no prior state) they stay null.
func TestReadAppleOsX_VpnAndEasImport(t *testing.T) {
	data := &ProfileResourceModel{}
	readAppleOsXIntoState(context.Background(), data, &sdk.AppleOsXDeviceProfileEntityV2{
		VpnList: []sdk.AppleOsXVpnPayloadEntityV2{{
			ConnectionName: "VPN", ConnectionType: "AirwatchTunnel", Server: "tcp://vpn.example.invalid:8443",
			EncryptionLevel: sdk.IntPtr(1), SendAllTraffic: sdk.BoolPtr(true), PerAppVpn: sdk.BoolPtr(false),
			MdmDeviceUniqueID: "{DeviceUid}", SafariDomains: []string{"a.example"},
			VpnOnDemand: []sdk.AppleOsXVpnOnDemandEntityV2{{Domain: "d.example", OnDemandAction: "Connect"}},
			Password:    "*****", SharedSecret: "*****", VpnPassword: "*****",
		}},
		EasMicrosoftOutlook: &sdk.AppleOsXEasMicrosoftOutlookPayloadEntityV2{
			AccountName: "EWS", ExchangeHost: "mail.example.invalid", UseSSL: sdk.BoolPtr(false), Domain: "{EmailDomain}", Password: "*****",
		},
	})
	if len(data.VpnList) != 1 {
		t.Fatalf("vpn_list = %+v", data.VpnList)
	}
	v := data.VpnList[0]
	if v.ConnectionName.ValueString() != "VPN" || v.EncryptionLevel.ValueInt64() != 1 || !v.SendAllTraffic.ValueBool() ||
		v.PerAppVPN.ValueBool() || v.PerAppVPN.IsNull() || v.MdmDeviceUniqueID.ValueString() != "{DeviceUid}" ||
		len(v.SafariDomains.Elements()) != 1 || len(v.VPNOnDemand) != 1 || v.VPNOnDemand[0].OnDemandAction.ValueString() != "Connect" {
		t.Errorf("vpn settings = %+v", v)
	}
	if !v.Password.IsNull() || !v.SharedSecret.IsNull() || !v.VPNPassword.IsNull() {
		t.Errorf("masked secrets must not reach state: %v %v %v", v.Password, v.SharedSecret, v.VPNPassword)
	}
	e := data.EasMicrosoftOutlook
	if e == nil || e.AccountName.ValueString() != "EWS" || e.Domain.ValueString() != "{EmailDomain}" || !e.Password.IsNull() {
		t.Errorf("eas = %+v", e)
	}
}

// On a normal read the configured secrets are carried from prior state.
func TestReadAppleOsX_VpnAndEasCarrySecrets(t *testing.T) {
	data := &ProfileResourceModel{
		VpnList:             []profilemodels.VPNItemModel{{Password: types.StringValue("p"), SharedSecret: types.StringValue("s"), VPNPassword: types.StringValue("v")}},
		EasMicrosoftOutlook: &profilemodels.EasMicrosoftOutlookModel{Password: types.StringValue("e")},
	}
	readAppleOsXIntoState(context.Background(), data, &sdk.AppleOsXDeviceProfileEntityV2{
		VpnList:             []sdk.AppleOsXVpnPayloadEntityV2{{ConnectionName: "VPN", Password: "*****", SharedSecret: "*****", VpnPassword: "*****"}},
		EasMicrosoftOutlook: &sdk.AppleOsXEasMicrosoftOutlookPayloadEntityV2{AccountName: "EWS", Password: "*****"},
	})
	v := data.VpnList[0]
	if v.Password.ValueString() != "p" || v.SharedSecret.ValueString() != "s" || v.VPNPassword.ValueString() != "v" {
		t.Errorf("vpn secrets not carried: %+v", v)
	}
	if data.EasMicrosoftOutlook.Password.ValueString() != "e" {
		t.Errorf("eas password not carried")
	}
	// Payload removed in UEM: nothing to carry into.
	readAppleOsXIntoState(context.Background(), data, &sdk.AppleOsXDeviceProfileEntityV2{})
	if data.VpnList != nil || data.EasMicrosoftOutlook != nil {
		t.Errorf("absent payloads must be null: %+v %+v", data.VpnList, data.EasMicrosoftOutlook)
	}
}

// A read with no Proxy (UEM omits it when unset) maps to "None"; a stored
// value is kept.
func TestMapVPNItem_ProxyAbsentReadsAsNone(t *testing.T) {
	got := mapVPNItemList([]sdk.AppleOsXVpnPayloadEntityV2{{ConnectionName: "a"}, {ConnectionName: "b", Proxy: "Automatic"}})
	if got[0].Proxy.ValueString() != "None" || got[1].Proxy.ValueString() != "Automatic" {
		t.Errorf("proxy = %v, %v", got[0].Proxy, got[1].Proxy)
	}
}

// On create the prior is the plan: a secret the configuration leaves unset
// is unknown there, and must be null (never unknown) in state.
func TestReadAppleOsX_UnsetSecretOnCreateIsNull(t *testing.T) {
	data := &ProfileResourceModel{
		VpnList:             []profilemodels.VPNItemModel{{Password: types.StringValue("p"), SharedSecret: types.StringValue("s"), VPNPassword: types.StringUnknown()}},
		EasMicrosoftOutlook: &profilemodels.EasMicrosoftOutlookModel{Password: types.StringUnknown()},
	}
	readAppleOsXIntoState(context.Background(), data, &sdk.AppleOsXDeviceProfileEntityV2{
		VpnList:             []sdk.AppleOsXVpnPayloadEntityV2{{ConnectionName: "VPN", Password: "*****", SharedSecret: "*****"}},
		EasMicrosoftOutlook: &sdk.AppleOsXEasMicrosoftOutlookPayloadEntityV2{AccountName: "EWS"},
	})
	v := data.VpnList[0]
	if v.Password.ValueString() != "p" || v.SharedSecret.ValueString() != "s" {
		t.Errorf("configured secrets must carry: %+v", v)
	}
	if v.VPNPassword.IsUnknown() || !v.VPNPassword.IsNull() {
		t.Errorf("unset vpn_password must be null, got %v", v.VPNPassword)
	}
	if p := data.EasMicrosoftOutlook.Password; p.IsUnknown() || !p.IsNull() {
		t.Errorf("unset eas password must be null, got %v", p)
	}
}

// Same latent pattern for network_list: an unset (Computed) password is
// unknown in the plan on create and must become null in state.
func TestCarryNetworkPasswords_UnknownPriorIsNull(t *testing.T) {
	got := carryNetworkPasswords([]NetworkItemModel{{}}, []NetworkItemModel{{
		Password: types.StringValue("p"), UserPassword: types.StringUnknown(), ProxyPassword: types.StringUnknown(),
	}})
	if got[0].Password.ValueString() != "p" || !got[0].UserPassword.IsNull() || !got[0].ProxyPassword.IsNull() {
		t.Errorf("got %+v", got[0])
	}
}
