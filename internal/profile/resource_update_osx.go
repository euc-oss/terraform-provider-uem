package profile

// updateAppleOsXProfile performs a read-modify-write using the Layer 2
// ProfileService: Get the existing entity to recover the current profile
// version, build a fresh entity from the plan, stamp the General block
// with ProfileID + incremented Version + CreateNewVersion=true, then call
// Update. The caller is responsible for the subsequent Get + state
// mapping (shared with Create's readback path).

import (
	"context"
	"encoding/json"

	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"
	profileplatform "github.com/euc-oss/terraform-provider-uem/internal/profile/platform"
	profilestate "github.com/euc-oss/terraform-provider-uem/internal/profile/state"
	sdk "github.com/euc-oss/terraform-sdk-uem"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

func (r *ProfileResource) updateAppleOsXProfile(ctx context.Context, data *profilemodels.ProfileResourceModel, priorState *profilemodels.ProfileResourceModel, profileID int) error {
	// Pending-upload step — mirrors Create.
	if err := profilestate.PrepareCredentialsForPayload(ctx, r.client, data, priorState.CredentialsList); err != nil {
		return err
	}

	svc, err := r.profileService(ctx)
	if err != nil {
		return err
	}
	svc.RegisterEntry(profileID, sdk.PlatformAppleOsX)

	// Fetch the live entity to recover the current Version so we can bump
	// it. Soft-fail on the Get: without a version bump UEM will still
	// accept the PUT but the change won't publish. Warn loudly in that
	// case so we have a breadcrumb.
	var currentVersion *int
	if existing, gErr := svc.Get(ctx, profileID); gErr == nil && existing.AppleOsX != nil && existing.AppleOsX.General != nil {
		currentVersion = existing.AppleOsX.General.Version
	} else if gErr != nil {
		tflog.Warn(ctx, "Unable to read existing profile before update", map[string]any{
			"profile_id": profileID,
			"platform":   sdk.PlatformAppleOsX,
			"error":      gErr.Error(),
		})
	}

	entity, err := profileplatform.BuildAppleOsXCreateEntity(data)
	if err != nil {
		return err
	}
	profileplatform.StampGeneralV2(entity.General, profileID, currentVersion)

	if body, mErr := json.Marshal(entity); mErr == nil {
		tflog.Debug(ctx, "macOS Update payload", map[string]interface{}{"body": string(body)})
	}
	if err := svc.Update(ctx, profileID, entity); err != nil {
		tflog.Error(ctx, "macOS Update failed", map[string]interface{}{"error": err.Error()})
		return err
	}
	return nil
}
