package profile

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

func liveWithMaskedVPN() *sdk.ProfileResult {
	return &sdk.ProfileResult{AppleOsX: &sdk.AppleOsXDeviceProfileEntityV2{
		General: &sdk.GeneralPayloadV2Entity{ProfileID: intPtr(999), Version: intPtr(1)},
		VpnList: []sdk.AppleOsXVpnPayloadEntityV2{{ConnectionName: "v", ConnectionType: "AirwatchTunnel", Server: "s", Proxy: "None", Password: "*****"}},
	}}
}

// vpnPlanValues is an update plan whose single vpn_list entry leaves every
// secret null (an onboarded profile with a blank secrets.json), unless
// password is given.
func vpnPlanValues(t *testing.T, password *string) map[string]tftypes.Value {
	t.Helper()
	objType, _ := getResourceSchema(t).Schema.Type().TerraformType(context.Background()).(tftypes.Object)
	listType, _ := objType.AttributeTypes["vpn_list"].(tftypes.List)
	elemType, _ := listType.ElementType.(tftypes.Object)
	elem := map[string]tftypes.Value{}
	for name, typ := range elemType.AttributeTypes {
		elem[name] = tftypes.NewValue(typ, nil)
	}
	elem["connection_name"] = tftypes.NewValue(tftypes.String, "v")
	elem["connection_type"] = tftypes.NewValue(tftypes.String, "AirwatchTunnel")
	elem["server"] = tftypes.NewValue(tftypes.String, "s2")
	if password != nil {
		elem["password"] = tftypes.NewValue(tftypes.String, *password)
	}
	return mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{
		"id":       stringVal("999"),
		"vpn_list": tftypes.NewValue(listType, []tftypes.Value{tftypes.NewValue(elemType, elem)}),
	})
}

// runModifyPlan runs ModifyPlan for an update from a state with the same
// vpn_list but server "s" (so the plan, server "s2", is a real change),
// or, with noChange, from a state equal to the plan.
func runModifyPlan(t *testing.T, fake *fakeProfileService, values map[string]tftypes.Value, noChange ...bool) *resource.ModifyPlanResponse {
	t.Helper()
	res := newRMWTestResource(t, fake)
	plan := createResourcePlan(t, values)
	state := tfsdk.State(plan)
	if len(noChange) == 0 {
		prior := map[string]tftypes.Value{}
		for k, v := range values {
			prior[k] = v
		}
		prior["description"] = stringVal("before")
		state = tfsdk.State(createResourcePlan(t, prior))
	}
	resp := &resource.ModifyPlanResponse{Plan: plan}
	res.ModifyPlan(context.Background(), resource.ModifyPlanRequest{Plan: plan, State: state}, resp)
	return resp
}

// Plan time: an update leaving out a VPN secret UEM holds is an error; the
// same update with the secret set is not.
func TestModifyPlan_RefusesOmittedVPNSecret(t *testing.T) {
	resp := runModifyPlan(t, &fakeProfileService{getResult: liveWithMaskedVPN()}, vpnPlanValues(t, nil))
	if !resp.Diagnostics.HasError() || !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), `vpn_list["v"].password`) {
		t.Fatalf("want a plan error naming the secret, got %v", resp.Diagnostics)
	}
	p := "set"
	if resp := runModifyPlan(t, &fakeProfileService{getResult: liveWithMaskedVPN()}, vpnPlanValues(t, &p)); resp.Diagnostics.HasError() {
		t.Errorf("secret set: unexpected error %v", resp.Diagnostics)
	}
}

// Apply time: Update refuses before sending anything.
func TestUpdate_RefusesOmittedVPNSecretBeforeWrite(t *testing.T) {
	fake := &fakeProfileService{getResult: liveWithMaskedVPN()}
	resp := runRMWUpdate(t, fake, vpnPlanValues(t, nil))
	if !resp.Diagnostics.HasError() || fake.updateCalls != 0 {
		t.Fatalf("want an error and no Update call; diags %v, updateCalls %d", resp.Diagnostics, fake.updateCalls)
	}
}

// A no-op plan (onboard's own post-import check, blank secrets) sends no
// update, so it must never be refused.
func TestModifyPlan_NoChangeIsNotRefused(t *testing.T) {
	if resp := runModifyPlan(t, &fakeProfileService{getResult: liveWithMaskedVPN()}, vpnPlanValues(t, nil), true); resp.Diagnostics.HasError() {
		t.Fatalf("no-op plan refused: %v", resp.Diagnostics)
	}
}
