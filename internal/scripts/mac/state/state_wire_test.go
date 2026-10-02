package state

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	tf "github.com/euc-oss/terraform-provider-uem/internal/scripts/mac/models"
)

// TestAllowedInCatalog_alwaysOnTheWire pins the marshaled create and update request
// bodies: allowed_in_catalog must be present with the configured value in every case,
// including false and a null attribute (which maps to false). The pre-26.2 SDK used
// *bool with omitempty, and this provider always sent a non-nil pointer, so false was
// already explicit on the wire. The 26.2 SDK's CreateScriptV1 uses a plain bool with no
// omitempty, while UpdateScriptV1 keeps *bool with omitempty, so the update path relies
// on the provider still passing a non-nil pointer.
func TestAllowedInCatalog_alwaysOnTheWire(t *testing.T) {
	cases := []struct {
		name string
		in   types.Bool
		want bool
	}{
		{"true", types.BoolValue(true), true},
		{"false", types.BoolValue(false), false},
		{"null", types.BoolNull(), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := &tf.MacScriptResourceModel{
				OrganizationGroupUUID: types.StringValue("90cd82d2-e5f5-ce88-e288-e50bdbf629b8"),
				Name:                  types.StringValue("wire"),
				Platform:              types.StringValue("APPLE_OSX"),
				ScriptType:            types.StringValue("BASH"),
				ScriptData:            types.StringValue("eA=="),
				AllowedInCatalog:      tc.in,
			}

			createReq, diags := ToCreateScriptV1(model)
			if diags.HasError() {
				t.Fatalf("ToCreateScriptV1: %v", diags)
			}
			assertAllowedInCatalogOnWire(t, "create", createReq, tc.want)

			updateReq, diags := ToUpdateScriptV1(model, "05d17100-b346-c29d-6760-a0fdedcf8623")
			if diags.HasError() {
				t.Fatalf("ToUpdateScriptV1: %v", diags)
			}
			assertAllowedInCatalogOnWire(t, "update", updateReq, tc.want)
		})
	}
}

func assertAllowedInCatalogOnWire(t *testing.T, label string, req any, want bool) {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("%s: marshal: %v", label, err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatalf("%s: unmarshal: %v", label, err)
	}
	raw, ok := fields["allowed_in_catalog"]
	if !ok {
		t.Fatalf("%s: allowed_in_catalog missing from body %s", label, body)
	}
	var got bool
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("%s: allowed_in_catalog is not a bool: %s", label, raw)
	}
	if got != want {
		t.Fatalf("%s: allowed_in_catalog = %v, want %v", label, got, want)
	}
}
