package profile

import (
	"context"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"

	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"
	profileplatform "github.com/euc-oss/terraform-provider-uem/internal/profile/platform"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

var _ resource.ResourceWithModifyPlan = &ProfileResource{}

// ModifyPlan refuses, at plan time, a macOS profile update that would leave
// out a VPN or Exchange (Microsoft Outlook) secret UEM holds: UEM clears such
// a secret instead of keeping it (see
// profileplatform.OmittedWipedSecrets for the live evidence). It reads the
// live profile only when the plan has a VPN or EAS secret left unset, so
// other plans cost no API call. Update repeats the check before writing.
func (r *ProfileResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() || r.client == nil {
		return // create, destroy, or not configured
	}
	if req.Plan.Raw.Equal(req.State.Raw) {
		return // no change: no update is sent, so nothing can be cleared
	}
	var plan profilemodels.ProfileResourceModel
	if diags := req.Plan.Get(ctx, &plan); diags.HasError() {
		return // the plan still has unknowns we can't decode; Update checks again
	}
	if plan.Platform.ValueString() != sdk.PlatformAppleOsX || !hasUnsetWipedSecret(&plan) {
		return
	}
	var state profilemodels.ProfileResourceModel
	if diags := req.State.Get(ctx, &state); diags.HasError() {
		return
	}
	id, err := strconv.Atoi(state.ID.ValueString())
	if err != nil {
		return
	}
	svc, err := r.profileService(ctx)
	if err != nil {
		return
	}
	svc.RegisterEntry(id, sdk.PlatformAppleOsX)
	live, err := svc.Get(ctx, id)
	if err != nil || live == nil || live.AppleOsX == nil {
		return // Update fails closed on the same read
	}
	planned, err := profileplatform.BuildAppleOsXCreateEntity(&plan)
	if err != nil {
		return
	}
	if missing := profileplatform.OmittedWipedSecrets(live.AppleOsX, planned); len(missing) > 0 {
		resp.Diagnostics.AddError("VPN or Exchange secret would be deleted",
			"UEM clears an omitted VPN or Exchange secret on update, and it holds a value for "+strings.Join(missing, ", ")+
				". Set it in the configuration (for an onboarded profile, fill it in secrets.json) before applying.")
	}
}

// hasUnsetWipedSecret reports whether any VPN or EAS secret in the plan is
// null or unknown, the only case OmittedWipedSecrets can flag.
func hasUnsetWipedSecret(m *profilemodels.ProfileResourceModel) bool {
	for _, v := range m.VpnList {
		for _, s := range []interface{ IsNull() bool }{v.Password, v.SharedSecret, v.VPNPassword} {
			if s.IsNull() {
				return true
			}
		}
		if v.Password.IsUnknown() || v.SharedSecret.IsUnknown() || v.VPNPassword.IsUnknown() {
			return true
		}
	}
	e := m.EasMicrosoftOutlook
	return e != nil && (e.Password.IsNull() || e.Password.IsUnknown())
}
