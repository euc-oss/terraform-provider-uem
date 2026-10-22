package profile

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// f13Payloads are the uem_profile attributes F13 added from the SDK structs.
var f13Payloads = []string{"scep_list", "web_clips_list", "vpn_list", "eas_microsoft_outlook", "kernel_extension", "custom_attributes"}

// UEM echoes a value for every field a create left unset (live, as<internal-env>
// profile 3001: EncryptionLevel 0, Port 0, false bools). An Optional-only
// nested attribute plans null, so the echoed value fails Terraform's
// "inconsistent result after apply" check. Every nested scalar/list field of
// the F13 payloads must therefore be Optional+Computed with
// UseStateForUnknown, like network_list's.
func TestF13NestedAttributesAreComputed(t *testing.T) {
	var sr resource.SchemaResponse
	(&ProfileResource{}).Schema(context.Background(), resource.SchemaRequest{}, &sr)
	var check func(path string, attrs map[string]schema.Attribute)
	check = func(path string, attrs map[string]schema.Attribute) {
		for name, a := range attrs {
			p := path + "." + name
			switch v := a.(type) {
			case schema.StringAttribute:
				if !v.Optional || !v.Computed || len(v.PlanModifiers) == 0 {
					t.Errorf("%s: want Optional+Computed with plan modifiers", p)
				}
			case schema.BoolAttribute:
				if !v.Optional || !v.Computed || len(v.PlanModifiers) == 0 {
					t.Errorf("%s: want Optional+Computed with plan modifiers", p)
				}
			case schema.Int64Attribute:
				if !v.Optional || !v.Computed || len(v.PlanModifiers) == 0 {
					t.Errorf("%s: want Optional+Computed with plan modifiers", p)
				}
			case schema.ListAttribute:
				if !v.Optional || !v.Computed || len(v.PlanModifiers) == 0 {
					t.Errorf("%s: want Optional+Computed with plan modifiers", p)
				}
			case schema.ListNestedAttribute:
				check(p, v.NestedObject.Attributes)
			case schema.SingleNestedAttribute:
				check(p, v.Attributes)
			}
		}
	}
	for _, name := range f13Payloads {
		switch v := sr.Schema.Attributes[name].(type) {
		case schema.ListNestedAttribute:
			check(name, v.NestedObject.Attributes)
		case schema.SingleNestedAttribute:
			check(name, v.Attributes)
		default:
			t.Errorf("%s: unexpected attribute type %T", name, v)
		}
	}
}

// The live create shape: a VPN entry that sets only a few fields (the rest
// planned unknown, as Terraform plans Computed attributes left unset), and a
// UEM read that echoes defaults. The resulting state must hold UEM's values
// and agree with every known planned value (Terraform's consistency rule).
func TestProfileResourceCreate_VpnListEchoedDefaults(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && strings.Contains(r.URL.Path, "/create"):
			_ = json.NewEncoder(w).Encode(3001)
		case r.Method == "GET":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"General": map[string]interface{}{"ProfileId": 3001, "Name": "p", "ProfileUuid": "u", "ProfileContext": "Device", "ManagedLocationGroupID": 1001},
				"VpnList": []interface{}{map[string]interface{}{
					"ConnectionName": "v", "ConnectionType": "AirwatchTunnel", "Server": "vpn.example.invalid", "Proxy": "None",
					"EncryptionLevel": 0, "Port": 0, "SendAllTraffic": false, "PerAppVpn": false, "SafariDomains": []string{},
					"Password": "*****", "SharedSecret": "*****",
				}},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
	c, server := createTestClient(t, handler)
	defer server.Close()

	ctx := context.Background()
	schemaResp := getResourceSchema(t)
	objType, _ := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	vpnListType, _ := objType.AttributeTypes["vpn_list"].(tftypes.List)
	elemType, _ := vpnListType.ElementType.(tftypes.Object)
	set := map[string]tftypes.Value{
		"connection_name": tftypes.NewValue(tftypes.String, "v"),
		"connection_type": tftypes.NewValue(tftypes.String, "AirwatchTunnel"),
		"server":          tftypes.NewValue(tftypes.String, "vpn.example.invalid"),
		"password":        tftypes.NewValue(tftypes.String, "p"),
		"shared_secret":   tftypes.NewValue(tftypes.String, "s"),
	}
	elem := map[string]tftypes.Value{}
	for name, typ := range elemType.AttributeTypes {
		switch lt, isList := typ.(tftypes.List); {
		case set[name].Type() != nil:
			elem[name] = set[name]
		case isList && lt.ElementType.Is(tftypes.Object{}):
			// Nested object lists are not Computed: unset plans null.
			elem[name] = tftypes.NewValue(typ, nil)
		case vpnNestedComputed(schemaResp.Schema, name):
			// Terraform plans an unset Computed attribute as unknown.
			elem[name] = tftypes.NewValue(typ, tftypes.UnknownValue)
		default:
			// An unset Optional-only attribute plans null.
			elem[name] = tftypes.NewValue(typ, nil)
		}
	}
	plan := createResourcePlan(t, map[string]tftypes.Value{
		"name": stringVal("p"), "platform": stringVal("AppleOsX"), "org_group_id": stringVal("1001"),
		"vpn_list": tftypes.NewValue(vpnListType, []tftypes.Value{tftypes.NewValue(elemType, elem)}),
	})
	resp := &resource.CreateResponse{State: emptyResourceState(t)}
	(&ProfileResource{client: c}).Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create: %v", resp.Diagnostics)
	}
	var stateVal map[string]tftypes.Value
	if err := resp.State.Raw.As(&stateVal); err != nil {
		t.Fatal(err)
	}
	var list []tftypes.Value
	_ = stateVal["vpn_list"].As(&list)
	if len(list) != 1 {
		t.Fatalf("vpn_list in state = %v", stateVal["vpn_list"])
	}
	var got map[string]tftypes.Value
	_ = list[0].As(&got)
	for name, planned := range elem {
		if planned.IsKnown() && !planned.Equal(got[name]) {
			t.Errorf("%s: planned %v, state %v (inconsistent result after apply)", name, planned, got[name])
		}
	}
	for _, name := range []string{"encryption_level", "port", "send_all_traffic", "per_app_vpn", "proxy"} {
		if v := got[name]; !v.IsKnown() || v.IsNull() {
			t.Errorf("%s must hold UEM's echoed value, got %v", name, v)
		}
	}
}

// vpnNestedComputed reports whether vpn_list's nested attribute name is
// Computed in the schema.
func vpnNestedComputed(sch schema.Schema, name string) bool {
	l, ok := sch.Attributes["vpn_list"].(schema.ListNestedAttribute)
	if !ok {
		return false
	}
	a, ok := l.NestedObject.Attributes[name]
	return ok && a.IsComputed()
}
