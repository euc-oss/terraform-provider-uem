package platform

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"
)

// UEM requires Proxy on every VPN write (see profilemodels.VPNProxyNone):
// an unset proxy is sent as "None", an explicit value as-is. This also
// covers the update path, whose planned VpnList comes from the same builder.
func TestBuildVPNItem_ProxyDefaultsToNone(t *testing.T) {
	unset := buildVPNItem(&profilemodels.VPNItemModel{ConnectionName: types.StringValue("v"), Proxy: types.StringNull()})
	if unset.Proxy != "None" {
		t.Errorf("unset proxy sent as %q, want None", unset.Proxy)
	}
	unknown := buildVPNItem(&profilemodels.VPNItemModel{Proxy: types.StringUnknown()})
	if unknown.Proxy != "None" {
		t.Errorf("unknown proxy sent as %q, want None", unknown.Proxy)
	}
	manual := buildVPNItem(&profilemodels.VPNItemModel{Proxy: types.StringValue("Manual")})
	if manual.Proxy != "Manual" {
		t.Errorf("explicit proxy sent as %q", manual.Proxy)
	}
	ent, err := BuildAppleOsXCreateEntity(&profilemodels.ProfileResourceModel{
		Name: types.StringValue("p"), OrgGroupID: types.StringValue("1001"),
		VpnList: []profilemodels.VPNItemModel{{ConnectionName: types.StringValue("v")}},
	})
	if err != nil || len(ent.VpnList) != 1 || ent.VpnList[0].Proxy != "None" {
		t.Errorf("create entity VpnList = %+v, err %v", ent, err)
	}
}
