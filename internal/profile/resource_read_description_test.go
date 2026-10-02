package profile

import (
	"context"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// runProfileRead drives the real ProfileResource.Read against a fake service
// whose Get returns ent, starting from a prior state built from stateValues,
// and returns the resulting description.
func runProfileRead(t *testing.T, ent *sdk.AppleOsXDeviceProfileEntityV2, stateValues map[string]tftypes.Value) types.String {
	t.Helper()
	fake := &fakeProfileService{getResult: &sdk.ProfileResult{AppleOsX: ent}}
	res := newRMWTestResource(t, fake)
	ctx := context.Background()

	state := createResourceState(t, stateValues)
	resp := &resource.ReadResponse{State: state}
	res.Read(ctx, resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", resp.Diagnostics)
	}
	var desc types.String
	resp.Diagnostics.Append(resp.State.GetAttribute(ctx, path.Root("description"), &desc)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("GetAttribute(description): %v", resp.Diagnostics)
	}
	return desc
}

// TestProfileResourceRead_DescriptionPassesThroughServerValue replaces
// TestProfileResourceRead_DescriptionRemovalConverges. internal-ticket removed
// setDescriptionFromAPI's prior-state-dependent null/"" disambiguation (row
// #57 of the B16 audit: never observed live, no source). UEM's Description
// field is a non-pointer string with omitempty, so "omitted" and "sent as
// an explicit empty string" are indistinguishable on the wire; Read now
// always stores that value as "" regardless of what was there before.
//
// EXPECTED DIFF RISK: resource.go's nullWhenConfigNullStringModifier still
// plans description as null when it's removed from HCL. Because Read now
// stores "" (not null) on the post-apply readback after such a removal,
// real Terraform can report "Provider produced inconsistent result after
// apply" (planned null, got ""). This unit harness calls Read/Update
// directly and so cannot reproduce that RPC-boundary consistency check;
// the sub-tests below assert only the mapper-level "" result. See the
// CHANGELOG entry.
func TestProfileResourceRead_DescriptionPassesThroughServerValue(t *testing.T) {
	t.Parallel()

	liveNoDescription := func() *sdk.AppleOsXDeviceProfileEntityV2 {
		ent := liveAppleOsXEntity()
		ent.General.Description = ""
		return ent
	}

	t.Run("prior null (removed from config), API empty -> empty string", func(t *testing.T) {
		t.Parallel()
		got := runProfileRead(t, liveNoDescription(), mergeValues(rmwBasePlanValues("AppleOsX"), map[string]tftypes.Value{
			"description": nullString(),
		}))
		if got.IsNull() || got.ValueString() != "" {
			t.Fatalf("description = %s, want \"\" (server value as-is)", got)
		}
	})

	t.Run("prior explicit empty string, API empty -> stays empty string", func(t *testing.T) {
		t.Parallel()
		got := runProfileRead(t, liveNoDescription(), mergeValues(rmwBasePlanValues("AppleOsX"), map[string]tftypes.Value{
			"description": stringVal(""),
		}))
		if got.IsNull() || got.ValueString() != "" {
			t.Fatalf("description = %s, want \"\"", got)
		}
	})

	t.Run("prior non-empty, API empty (cleared in console) -> empty string", func(t *testing.T) {
		t.Parallel()
		got := runProfileRead(t, liveNoDescription(), rmwBasePlanValues("AppleOsX"))
		if got.IsNull() || got.ValueString() != "" {
			t.Fatalf("description = %s, want \"\" (server value as-is, drift now visible as a change to \"\" rather than staying null)", got)
		}
	})

	t.Run("update removing description, then refresh -> empty string both times", func(t *testing.T) {
		t.Parallel()
		fake := &fakeProfileService{getResult: &sdk.ProfileResult{AppleOsX: liveNoDescription()}}
		res := newRMWTestResource(t, fake)
		ctx := context.Background()

		planValues := mergeValues(rmwBasePlanValues("AppleOsX"), map[string]tftypes.Value{
			"description": nullString(),
		})
		upResp := &resource.UpdateResponse{State: emptyResourceState(t)}
		res.Update(ctx, resource.UpdateRequest{Plan: createResourcePlan(t, planValues), State: emptyResourceState(t)}, upResp)
		if upResp.Diagnostics.HasError() {
			t.Fatalf("Update diagnostics: %v", upResp.Diagnostics)
		}
		var afterApply types.String
		upResp.Diagnostics.Append(upResp.State.GetAttribute(ctx, path.Root("description"), &afterApply)...)
		if afterApply.IsNull() || afterApply.ValueString() != "" {
			t.Fatalf("after apply: description = %s, want \"\" (server value as-is; real Terraform can reject this apply -- see the EXPECTED DIFF RISK note above)", afterApply)
		}

		readResp := &resource.ReadResponse{State: upResp.State}
		res.Read(ctx, resource.ReadRequest{State: upResp.State}, readResp)
		if readResp.Diagnostics.HasError() {
			t.Fatalf("Read diagnostics: %v", readResp.Diagnostics)
		}
		var afterRefresh types.String
		readResp.Diagnostics.Append(readResp.State.GetAttribute(ctx, path.Root("description"), &afterRefresh)...)
		if afterRefresh.IsNull() || afterRefresh.ValueString() != "" {
			t.Fatalf("after refresh: description = %s, want \"\"", afterRefresh)
		}
	})
}
