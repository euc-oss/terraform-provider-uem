package profile

import (
	"context"
	"encoding/json"
	"fmt"

	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"
	profileplatform "github.com/euc-oss/terraform-provider-uem/internal/profile/platform"
	profilestate "github.com/euc-oss/terraform-provider-uem/internal/profile/state"
	sdk "github.com/euc-oss/terraform-sdk-uem"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// createTypedProfile routes Create to platform-specific builders.
func (r *ProfileResource) createTypedProfile(ctx context.Context, data *profilemodels.ProfileResourceModel, platform string) (int, error) {
	svc, err := r.profileService(ctx)
	if err != nil {
		return 0, err
	}

	var entity interface{}
	switch platform {
	case sdk.PlatformAppleOsX:
		// macOS is the only platform with credentials_list — upload any
		// Upload-sourced certificates and drop orphaned credential data
		// before building the entity.
		if err := profilestate.PrepareCredentialsForPayload(ctx, r.client, data, nil); err != nil {
			return 0, err
		}
		ent, err := profileplatform.BuildAppleOsXCreateEntity(data)
		if err != nil {
			return 0, err
		}
		entity = ent
	case sdk.PlatformAndroid:
		entity = profileplatform.BuildAndroidCreateEntity(data)
	case sdk.PlatformAppleiOS:
		entity = profileplatform.BuildAppleiOSCreateEntity(data)
	case sdk.PlatformWindows10:
		entity = profileplatform.BuildWindows10CreateEntity(data)
	case sdk.PlatformWindowsRugged:
		entity = profileplatform.BuildWindowsRuggedCreateEntity(data)
	case sdk.PlatformLinux:
		entity = profileplatform.BuildLinuxCreateEntity(data)
	default:
		return 0, fmt.Errorf("unsupported platform %q", platform)
	}

	if body, mErr := json.Marshal(entity); mErr == nil {
		tflog.Debug(ctx, "Create payload", map[string]interface{}{"platform": platform, "body": string(body)})
	}
	id, err := svc.Create(ctx, platform, entity)
	if err != nil {
		tflog.Error(ctx, "Create failed", map[string]interface{}{"platform": platform, "error": err.Error()})
		return 0, err
	}
	svc.RegisterEntry(id, platform)
	return id, nil
}

// updateTypedProfile routes Update to platform-specific builders.
func (r *ProfileResource) updateTypedProfile(ctx context.Context, data *profilemodels.ProfileResourceModel, priorState *profilemodels.ProfileResourceModel, profileID int, platform string) error {
	if platform == sdk.PlatformAppleOsX {
		return r.updateAppleOsXProfile(ctx, data, priorState, profileID)
	}

	svc, err := r.profileService(ctx)
	if err != nil {
		return err
	}
	svc.RegisterEntry(profileID, platform)

	var currentV2 *int
	var currentV4 *int
	if existing, gErr := svc.Get(ctx, profileID); gErr == nil {
		switch {
		case existing.Android != nil && existing.Android.General != nil:
			currentV2 = existing.Android.General.Version
		case existing.AppleiOS != nil && existing.AppleiOS.General != nil:
			currentV2 = existing.AppleiOS.General.Version
		case existing.Windows10 != nil && existing.Windows10.General != nil:
			currentV2 = existing.Windows10.General.Version
		case existing.WindowsRugged != nil && existing.WindowsRugged.General != nil:
			currentV2 = existing.WindowsRugged.General.Version
		case existing.Linux != nil && existing.Linux.General != nil:
			currentV4 = existing.Linux.General.Version
		}
	} else {
		tflog.Warn(ctx, "Unable to read existing profile before update", map[string]any{
			"profile_id": profileID, "platform": platform, "error": gErr.Error(),
		})
	}

	pid := profileID
	var entity interface{}
	switch platform {
	case sdk.PlatformAndroid:
		ent := profileplatform.BuildAndroidCreateEntity(data)
		profileplatform.StampGeneralV2(ent.General, pid, currentV2)
		entity = ent
	case sdk.PlatformAppleiOS:
		ent := profileplatform.BuildAppleiOSCreateEntity(data)
		profileplatform.StampGeneralV2(ent.General, pid, currentV2)
		entity = ent
	case sdk.PlatformWindows10:
		ent := profileplatform.BuildWindows10CreateEntity(data)
		profileplatform.StampGeneralV2(ent.General, pid, currentV2)
		entity = ent
	case sdk.PlatformWindowsRugged:
		ent := profileplatform.BuildWindowsRuggedCreateEntity(data)
		profileplatform.StampGeneralV2(ent.General, pid, currentV2)
		entity = ent
	case sdk.PlatformLinux:
		ent := profileplatform.BuildLinuxCreateEntity(data)
		profileplatform.StampGeneralV4(ent.General, pid, currentV4)
		entity = ent
	default:
		return fmt.Errorf("unsupported platform %q", platform)
	}

	if body, mErr := json.Marshal(entity); mErr == nil {
		tflog.Debug(ctx, "Update payload", map[string]interface{}{"platform": platform, "body": string(body)})
	}
	if err := svc.Update(ctx, profileID, entity); err != nil {
		tflog.Error(ctx, "Update failed", map[string]interface{}{"platform": platform, "error": err.Error()})
		return err
	}
	return nil
}
