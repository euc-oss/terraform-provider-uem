package profile

import (
	"context"
	"fmt"
	"strings"

	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"
	profileplatform "github.com/euc-oss/terraform-provider-uem/internal/profile/platform"
	profilestate "github.com/euc-oss/terraform-provider-uem/internal/profile/state"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// refuseIfMaskedUnmodeled returns a non-nil error, formatted per internal-task's
// permanent masked-secret guard, if paths (from a platform's
// MaskedUnmodeled* detector in internal/profile/platform/masked.go) is
// non-empty, or if inspectErr is non-nil (the detector itself FAILED CLOSED
// because it could not fully inspect the live entity — that must also
// refuse the update, never be treated as "no masked secrets found").
// Callers must check this before calling svc.Update — see overlay.go's
// header comment for why the guard is required and permanent.
func refuseIfMaskedUnmodeled(profileID int, paths []string, inspectErr error) error {
	if inspectErr != nil {
		return fmt.Errorf("refusing to update profile %d: unable to check the live profile for masked secrets before update: %w", profileID, inspectErr)
	}
	if len(paths) == 0 {
		return nil
	}
	return fmt.Errorf("refusing to update profile %d: UEM returns masked secrets (\"*****\") for settings this provider does not manage (%s); sending the update would overwrite those secrets with the mask. Update this profile in the UEM console instead", profileID, strings.Join(paths, ", "))
}

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
		ent, err := profileplatform.BuildLinuxCreateEntity(data)
		if err != nil {
			return 0, err
		}
		entity = ent
	default:
		return 0, fmt.Errorf("unsupported platform %q", platform)
	}

	if sections := payloadSectionNames(entity); sections != nil {
		tflog.Debug(ctx, "Create payload", map[string]interface{}{"platform": platform, "sections": sections})
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
//
// Update is a read-modify-write against UEM's full-replace Update endpoint:
// the live entity is fetched, then the plan-built entity's modeled sections
// are overlaid onto it (see internal/profile/platform/overlay.go for the
// modeled-section list per platform), so payload sections and General
// fields the provider doesn't model survive the Update unchanged. The Get
// is a hard requirement (FAIL CLOSED) — if it errors, or the expected
// platform entity is missing from the result, the update is aborted before
// svc.Update is ever called.
func (r *ProfileResource) updateTypedProfile(ctx context.Context, data *profilemodels.ProfileResourceModel, priorState *profilemodels.ProfileResourceModel, profileID int, platform string) error {
	if platform == sdk.PlatformAppleOsX {
		return r.updateAppleOsXProfile(ctx, data, priorState, profileID)
	}

	svc, err := r.profileService(ctx)
	if err != nil {
		return err
	}
	svc.RegisterEntry(profileID, platform)

	existing, gErr := svc.Get(ctx, profileID)
	if gErr != nil {
		return fmt.Errorf("unable to read current profile before update: %w", gErr)
	}
	if existing == nil {
		return fmt.Errorf("unable to read current profile before update: empty result for profile %d", profileID)
	}

	pid := profileID
	var entity interface{}
	switch platform {
	case sdk.PlatformAndroid:
		if existing.Android == nil {
			return fmt.Errorf("unable to read current profile before update: no Android entity found for profile %d", profileID)
		}
		live := existing.Android
		paths, perr := profileplatform.MaskedUnmodeledAndroid(live)
		if err := refuseIfMaskedUnmodeled(profileID, paths, perr); err != nil {
			return err
		}
		var currentVersion *int
		if live.General != nil {
			currentVersion = live.General.Version
		}
		planned := profileplatform.BuildAndroidCreateEntity(data)
		ent := profileplatform.OverlayAndroidUpdateEntity(live, planned)
		profileplatform.StampGeneralV2(ent.General, pid, currentVersion)
		entity = ent
	case sdk.PlatformAppleiOS:
		if existing.AppleiOS == nil {
			return fmt.Errorf("unable to read current profile before update: no AppleiOS entity found for profile %d", profileID)
		}
		live := existing.AppleiOS
		paths, perr := profileplatform.MaskedUnmodeledAppleiOS(live)
		if err := refuseIfMaskedUnmodeled(profileID, paths, perr); err != nil {
			return err
		}
		var currentVersion *int
		if live.General != nil {
			currentVersion = live.General.Version
		}
		planned := profileplatform.BuildAppleiOSCreateEntity(data)
		ent := profileplatform.OverlayAppleiOSUpdateEntity(live, planned)
		profileplatform.StampGeneralV2(ent.General, pid, currentVersion)
		entity = ent
	case sdk.PlatformWindows10:
		if existing.Windows10 == nil {
			return fmt.Errorf("unable to read current profile before update: no Windows10 entity found for profile %d", profileID)
		}
		live := existing.Windows10
		paths, perr := profileplatform.MaskedUnmodeledWindows10(live)
		if err := refuseIfMaskedUnmodeled(profileID, paths, perr); err != nil {
			return err
		}
		var currentVersion *int
		if live.General != nil {
			currentVersion = live.General.Version
		}
		planned := profileplatform.BuildWindows10CreateEntity(data)
		ent := profileplatform.OverlayWindows10UpdateEntity(live, planned)
		profileplatform.StampGeneralV2(ent.General, pid, currentVersion)
		entity = ent
	case sdk.PlatformWindowsRugged:
		if existing.WindowsRugged == nil {
			return fmt.Errorf("unable to read current profile before update: no WindowsRugged entity found for profile %d", profileID)
		}
		live := existing.WindowsRugged
		paths, perr := profileplatform.MaskedUnmodeledWindowsRugged(live)
		if err := refuseIfMaskedUnmodeled(profileID, paths, perr); err != nil {
			return err
		}
		var currentVersion *int
		if live.General != nil {
			currentVersion = live.General.Version
		}
		planned := profileplatform.BuildWindowsRuggedCreateEntity(data)
		ent := profileplatform.OverlayWindowsRuggedUpdateEntity(live, planned)
		profileplatform.StampGeneralV2(ent.General, pid, currentVersion)
		entity = ent
	case sdk.PlatformLinux:
		if existing.Linux == nil {
			return fmt.Errorf("unable to read current profile before update: no Linux entity found for profile %d", profileID)
		}
		live := existing.Linux
		paths, perr := profileplatform.MaskedUnmodeledLinux(live)
		if err := refuseIfMaskedUnmodeled(profileID, paths, perr); err != nil {
			return err
		}
		var currentVersion *int
		if live.General != nil {
			currentVersion = live.General.Version
		}
		planned, err := profileplatform.BuildLinuxCreateEntity(data)
		if err != nil {
			return err
		}
		ent := profileplatform.OverlayLinuxUpdateEntity(live, planned)
		profileplatform.StampGeneralV4(ent.General, pid, currentVersion)
		entity = ent
	default:
		return fmt.Errorf("unsupported platform %q", platform)
	}

	if sections := payloadSectionNames(entity); sections != nil {
		tflog.Debug(ctx, "Update payload", map[string]interface{}{"platform": platform, "profile_id": profileID, "sections": sections})
	}
	if err := svc.Update(ctx, profileID, entity); err != nil {
		tflog.Error(ctx, "Update failed", map[string]interface{}{"platform": platform, "error": err.Error()})
		return err
	}
	return nil
}
