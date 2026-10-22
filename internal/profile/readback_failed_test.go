package profile

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// The write succeeds and the final GET fails: the plan model is written to
// state, so every server-filled leaf that planned unknown must be resolved
// (null) or Terraform rejects the applied state. A known planned value must
// not change, the ID stays in state, and a warning tells the user to refresh.

func requireReadBackWarning(t *testing.T, warnings int, summary string) {
	t.Helper()
	if warnings != 1 || summary != "Profile read-back failed" {
		t.Fatalf("want one \"Profile read-back failed\" warning, got %d (%q)", warnings, summary)
	}
}

func requireStateResolved(t *testing.T, planned, applied tftypes.Value) {
	t.Helper()
	if !applied.IsFullyKnown() {
		t.Fatalf("applied state still holds an unknown value: %v", applied)
	}
	assertAppliedMatchesPlan(t, planned, applied)
	if id := objAttr(t, applied, "id"); !id.IsKnown() || id.IsNull() {
		t.Fatalf("resource id missing from state: %v", id)
	}
}

func TestCreate_ReadBackFails_ServerFilledLeavesResolved(t *testing.T) {
	t.Parallel()
	config := createConfigWith(t, map[string]tftypes.Value{
		"gatekeeper": partialGatekeeper(),
		"system_extensions": sysExtVal(t, map[string]tftypes.Value{
			"allowed_system_extension_types": sysExtList(t, sysExtTypeEntry(t, "TEAM1", nil)),
			"allowed_system_extensions":      allowedExtList(t),
		}),
		"privacy_preferences": privacyList(t, privacyEntry(t, nil)),
		"restrictions":        restrictionsCameraConfig(t, true),
	})
	planned := planProfileValue(t, nullResourceState(t).Raw, config)

	c, server := createTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/create") {
			_, _ = io.Copy(io.Discard, r.Body)
			_, _ = w.Write([]byte("55555"))
			return
		}
		w.WriteHeader(http.StatusBadRequest) // the read-back GET fails
	})
	t.Cleanup(server.Close)
	schemaResp := getResourceSchema(t)
	res := &ProfileResource{client: c}
	resp := &resource.CreateResponse{State: emptyResourceState(t)}
	res.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan{Schema: schemaResp.Schema, Raw: planned}}, resp)

	requireNoDiagErrors(t, resp)
	requireReadBackWarning(t, resp.Diagnostics.WarningsCount(), resp.Diagnostics.Warnings()[0].Summary())
	requireStateResolved(t, planned, resp.State.Raw)
	if id := boolOf(t, objAttr(t, objAttr(t, resp.State.Raw, "gatekeeper"), "allow_auto_unlock")); !id {
		t.Errorf("configured gatekeeper.allow_auto_unlock changed")
	}
	if got := objAttr(t, objAttr(t, resp.State.Raw, "gatekeeper"), "enable_software_update_delay"); !got.IsNull() {
		t.Errorf("unset gatekeeper.enable_software_update_delay = %v, want null", got)
	}
}

// failAfterUpdate serves the read-modify-write fetch before the update and
// fails the read-back after it.
type failAfterUpdate struct {
	updateFake
}

func (f *failAfterUpdate) Get(ctx context.Context, id int) (*sdk.ProfileResult, error) {
	if f.updateCalls > 0 {
		return nil, errors.New("read-back failed")
	}
	return f.updateFake.Get(ctx, id)
}

func runUpdateReadBackFails(t *testing.T, fake profileServiceAPI, prior, planned tftypes.Value) *resource.UpdateResponse {
	t.Helper()
	resp := runUpdate(t, fake, prior, planned)
	requireReadBackWarning(t, resp.Diagnostics.WarningsCount(), resp.Diagnostics.Warnings()[0].Summary())
	requireStateResolved(t, planned, resp.State.Raw)
	return resp
}

// An update that ADDS a partial gatekeeper block to a profile without one.
func TestUpdate_ReadBackFails_AddingPartialGatekeeperResolved(t *testing.T) {
	t.Parallel()
	prior, planned := gatekeeperUpdateFixture(t, nullGatekeeper(), partialGatekeeper())
	fake := &failAfterUpdate{}
	resp := runUpdateReadBackFails(t, fake, prior, planned)
	gk := objAttr(t, resp.State.Raw, "gatekeeper")
	for _, f := range []string{"enable_app_software_update_delay", "enable_software_update_delay", "enforced_software_update_delay"} {
		if got := objAttr(t, gk, f); !got.IsNull() {
			t.Errorf("gatekeeper.%s = %v, want null", f, got)
		}
	}
	if !boolOf(t, objAttr(t, gk, "allow_auto_unlock")) {
		t.Errorf("configured gatekeeper.allow_auto_unlock changed")
	}
}

// A rename with an unchanged privacy_preferences list whose static_code is unset.
func TestUpdate_ReadBackFails_RenameKeepsPrivacyListResolved(t *testing.T) {
	t.Parallel()
	statePrivacy := privacyList(t, privacyEntry(t, map[string]tftypes.Value{"static_code": boolVal(false)}))
	prior := createResourceState(t, mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{"privacy_preferences": statePrivacy})).Raw
	planned := planProfileValue(t, prior, updateConfigWith(t, map[string]tftypes.Value{"privacy_preferences": privacyList(t, privacyEntry(t, nil))}))
	fake := &failAfterUpdate{updateFake: updateFake{mutate: func(e *sdk.AppleOsXDeviceProfileEntityV2) {
		e.PrivacyPreferences = &sdk.MacOsPrivacyPreferencesPayloadV2Model{Identities: []sdk.MacOsPrivacyPreferencesV2Model{{
			Identifier: "com.example.app", IdentifierType: "bundleID", CodeRequirement: `identifier "com.example.app" and anchor apple`,
			Accessibility: "Allow", StaticCode: sdk.BoolPtr(false),
		}}}
	}}}
	resp := runUpdateReadBackFails(t, fake, prior, planned)
	if name := objAttr(t, resp.State.Raw, "name"); !name.IsKnown() {
		t.Errorf("name unknown")
	}
}
