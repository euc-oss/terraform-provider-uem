package profile

// is_active plan tests (internal-ticket).
//
// is_active is Optional+Computed. When a plan changes anything else and the
// config leaves is_active unset, the framework marks it unknown, which Terraform
// prints as "is_active: (known after apply)" on every update. UseStateForUnknown
// carries the refreshed state value forward instead. These tests drive
// PlanResourceChange through the real framework server (providerserver ->
// fwserver, so the schema's plan modifiers run exactly as in production), then
// feed the planned value into the real ProfileResource.Update.

import (
	"context"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

type isActiveTestProvider struct{}

func (isActiveTestProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "uem"
}
func (isActiveTestProvider) Schema(context.Context, provider.SchemaRequest, *provider.SchemaResponse) {
}
func (isActiveTestProvider) Configure(context.Context, provider.ConfigureRequest, *provider.ConfigureResponse) {
}
func (isActiveTestProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{func() resource.Resource { return &ProfileResource{} }}
}
func (isActiveTestProvider) DataSources(context.Context) []func() datasource.DataSource { return nil }

// The planProfile helper runs PlanResourceChange for uem_profile. The prior
// state may be a genuine null (create). ProposedNewState mirrors Terraform core
// for this schema: a non-null configured value wins, otherwise the prior value
// is carried.
func planProfile(t *testing.T, prior, config tftypes.Value) map[string]tftypes.Value {
	t.Helper()
	planned := planProfileValue(t, prior, config)
	var attrs map[string]tftypes.Value
	if err := planned.As(&attrs); err != nil {
		t.Fatalf("planned.As: %v", err)
	}
	return attrs
}

// planProfileValue is planProfile returning the whole planned object, for
// tests that feed it straight into Create or Update.
func planProfileValue(t *testing.T, prior, config tftypes.Value) tftypes.Value {
	t.Helper()
	server, err := providerserver.NewProtocol6WithError(isActiveTestProvider{})()
	if err != nil {
		t.Fatalf("NewProtocol6WithError: %v", err)
	}
	proposed := config
	if !prior.IsNull() {
		var priorAttrs, configAttrs map[string]tftypes.Value
		if err := prior.As(&priorAttrs); err != nil {
			t.Fatalf("prior.As: %v", err)
		}
		if err := config.As(&configAttrs); err != nil {
			t.Fatalf("config.As: %v", err)
		}
		out := make(map[string]tftypes.Value, len(priorAttrs))
		for name, pv := range priorAttrs {
			if cv := configAttrs[name]; !cv.IsNull() {
				out[name] = cv
			} else {
				out[name] = pv
			}
		}
		proposed = tftypes.NewValue(prior.Type(), out)
	}
	dv := func(v tftypes.Value) *tfprotov6.DynamicValue {
		d, err := tfprotov6.NewDynamicValue(v.Type(), v)
		if err != nil {
			t.Fatalf("NewDynamicValue: %v", err)
		}
		return &d
	}
	resp, err := server.PlanResourceChange(context.Background(), &tfprotov6.PlanResourceChangeRequest{
		TypeName:         "uem_profile",
		PriorState:       dv(prior),
		ProposedNewState: dv(proposed),
		Config:           dv(config),
	})
	if err != nil {
		t.Fatalf("PlanResourceChange: %v", err)
	}
	for _, d := range resp.Diagnostics {
		if d.Severity == tfprotov6.DiagnosticSeverityError {
			t.Fatalf("PlanResourceChange error: %s: %s", d.Summary, d.Detail)
		}
	}
	planned, err := resp.PlannedState.Unmarshal(prior.Type())
	if err != nil {
		t.Fatalf("Unmarshal planned state: %v", err)
	}
	return planned
}

func isActiveStateAndConfig(t *testing.T, stateActive bool, configIsActive tftypes.Value) (prior, config tftypes.Value) {
	t.Helper()
	stateVals := mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{
		"is_active": boolVal(stateActive),
	})
	// The config renames the profile (the "something else changed" that makes
	// the framework mark unconfigured computed attributes unknown); the
	// computed attributes the user does not set stay null in the config.
	configVals := map[string]tftypes.Value{
		"name":         stringVal("Renamed"),
		"platform":     stringVal(sdk.PlatformAppleOsX),
		"org_group_id": stringVal("14165"),
		"is_active":    configIsActive,
	}
	return createResourceState(t, stateVals).Raw, createResourceConfig(t, configVals).Raw
}

// TestProfilePlan_IsActiveUnset_KeepsStateValue is the fix: an update that
// leaves is_active unset plans the refreshed state value, not "known after
// apply", whichever way it is set.
func TestProfilePlan_IsActiveUnset_KeepsStateValue(t *testing.T) {
	t.Parallel()
	for _, stateActive := range []bool{true, false} {
		prior, config := isActiveStateAndConfig(t, stateActive, nullBool())
		planned := planProfile(t, prior, config)
		got := planned["is_active"]
		if !got.IsKnown() {
			t.Fatalf("state is_active=%v: planned is_active is unknown (\"known after apply\"), want it carried from state", stateActive)
		}
		var v bool
		if err := got.As(&v); err != nil || v != stateActive {
			t.Fatalf("planned is_active = %v (err %v), want %v", got, err, stateActive)
		}
		if name := planned["name"]; name.IsNull() || !name.IsKnown() {
			t.Fatalf("test setup: the rename must be part of the plan, got name=%v", name)
		}
	}
}

// TestProfilePlan_IsActiveConfigured_Wins proves the modifier never overrides
// a configured value.
func TestProfilePlan_IsActiveConfigured_Wins(t *testing.T) {
	t.Parallel()
	prior, config := isActiveStateAndConfig(t, true, boolVal(false))
	planned := planProfile(t, prior, config)
	var v bool
	if err := planned["is_active"].As(&v); err != nil || v {
		t.Fatalf("planned is_active = %v (err %v), want the configured false", planned["is_active"], err)
	}
}

// TestProfilePlan_Create_IsActiveStaysUnknown proves create is unchanged: with
// no prior state there is nothing to carry, so is_active is still unknown at
// plan time (the provider sends true and stores what UEM reports).
func TestProfilePlan_Create_IsActiveStaysUnknown(t *testing.T) {
	t.Parallel()
	_, config := isActiveStateAndConfig(t, true, nullBool())
	prior := nullResourceState(t).Raw
	planned := planProfile(t, prior, config)
	if planned["is_active"].IsKnown() {
		t.Fatalf("create: planned is_active = %v, want unknown", planned["is_active"])
	}
}

// TestProfilePlanThenUpdate_InactiveProfileStaysInactive runs the planned
// value through the real Update: an inactive profile whose is_active is unset
// in the config is sent back as inactive. Before the fix the unknown planned
// value fell through to the provider default and the update sent true,
// re-activating the profile.
func TestProfilePlanThenUpdate_InactiveProfileStaysInactive(t *testing.T) {
	t.Parallel()
	prior, config := isActiveStateAndConfig(t, false, nullBool())
	planned := planProfile(t, prior, config)

	// UEM holds the profile inactive and reports that on the read-back.
	live := liveAppleOsXEntity()
	live.General.IsActive = sdk.BoolPtr(false)
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{AppleOsX: live}}
	planValues := mergeValues(rmwBasePlanValues(sdk.PlatformAppleOsX), map[string]tftypes.Value{
		"name":      stringVal("Renamed"),
		"is_active": planned["is_active"],
	})
	resp := runRMWUpdate(t, fake, planValues)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics.Errors())
	}
	sent, ok := fake.updateEntity.(*sdk.AppleOsXDeviceProfileEntityV2)
	if !ok {
		t.Fatalf("expected *sdk.AppleOsXDeviceProfileEntityV2, got %T", fake.updateEntity)
	}
	if sent.General == nil || sent.General.IsActive == nil {
		t.Fatalf("sent General.IsActive missing: %+v", sent.General)
	}
	if *sent.General.IsActive {
		t.Fatal("update re-activated an inactive profile: sent IsActive=true, want false")
	}
	// The state written after the apply equals the planned value, so Terraform
	// accepts the result (no "inconsistent result after apply").
	var stateActive bool
	if diags := resp.State.GetAttribute(context.Background(), path.Root("is_active"), &stateActive); diags.HasError() {
		t.Fatalf("reading is_active from the new state: %v", diags)
	}
	if stateActive {
		t.Fatal("new state is_active = true, want false (the planned value)")
	}
}
