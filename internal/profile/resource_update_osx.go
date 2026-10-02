package profile

// updateAppleOsXProfile performs a read-modify-write using the Layer 2
// ProfileService: Get the live entity (FAIL CLOSED — if the Get errors, or
// returns no AppleOsX entity, the update is aborted and svc.Update is never
// called), build a fresh entity from the plan via BuildAppleOsXCreateEntity,
// overlay the plan-built entity's modeled sections onto the live entity
// (see platform.OverlayAppleOsXUpdateEntity for the modeled-section list),
// stamp the resulting General block with ProfileID + incremented Version +
// CreateNewVersion=true, then call Update. The caller is responsible for
// the subsequent Get + state mapping (shared with Create's readback path).
//
// UEM source: AirWatch API/AW.Mdm.Api/AW.Mdm.Api/Helper/Profiles/ProfileServiceV2Helper.cs:1035-1041,3443-3448
// (canonical Q6) confirms CreateNewVersion=true rebuilds the profile from
// the request payload only — omitted/null lists contribute no payloads and
// so replace/clear rather than merge — which is exactly why this function
// must overlay the live entity's unmodeled sections onto the plan-built
// entity first: without that overlay, forcing CreateNewVersion=true would
// silently drop any UEM-side payload this provider doesn't model.

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

func (r *ProfileResource) updateAppleOsXProfile(ctx context.Context, data *profilemodels.ProfileResourceModel, priorState *profilemodels.ProfileResourceModel, profileID int) error {
	svc, err := r.profileService(ctx)
	if err != nil {
		return err
	}
	svc.RegisterEntry(profileID, sdk.PlatformAppleOsX)

	// Fetch the live entity. This is a hard requirement: without it we
	// cannot preserve unmodeled payload sections against UEM's full-replace
	// Update endpoint, and we cannot recover the current Version to bump.
	// Fail closed — never fall through to a plan-only Update.
	existing, gErr := svc.Get(ctx, profileID)
	if gErr != nil {
		return fmt.Errorf("unable to read current profile before update: %w", gErr)
	}
	if existing == nil || existing.AppleOsX == nil {
		return fmt.Errorf("unable to read current profile before update: no AppleOsX entity found for profile %d", profileID)
	}
	live := existing.AppleOsX

	// Permanent masked-secret guard (internal-task): refuse to echo a UEM
	// "*****" mask from an unmodeled part of the live entity back into
	// Update — see platform.MaskedUnmodeledAppleOsX and overlay.go's
	// header comment for why this is not optional.
	paths, perr := profileplatform.MaskedUnmodeledAppleOsX(live)
	if err := refuseIfMaskedUnmodeled(profileID, paths, perr); err != nil {
		return err
	}

	var currentVersion *int
	if live.General != nil {
		currentVersion = live.General.Version
	}

	// UEM clears a VPN or EAS Outlook secret an update leaves out (see
	// OmittedWipedSecrets). Refuse before any write, including the
	// certificate upload below, rather than delete it. The VPN and EAS
	// sections don't depend on the uploaded certificates, so a build from
	// the plan as-is is enough for this check.
	precheck, err := profileplatform.BuildAppleOsXCreateEntity(data)
	if err != nil {
		return err
	}
	if missing := profileplatform.OmittedWipedSecrets(live, precheck); len(missing) > 0 {
		return fmt.Errorf("profile %d: UEM clears an omitted VPN or Exchange secret on update, and it holds a value for %s; "+
			"set it in the configuration (for an onboarded profile, fill it in secrets.json) before applying", profileID, strings.Join(missing, ", "))
	}

	// Pending-upload step — mirrors Create. Deliberately runs AFTER the Get,
	// the fail-closed nil checks, and the masked-secret guard above: it
	// performs a real POST that uploads certificate blobs to UEM, so a Get
	// failure or a masked-secret refusal must abort before it ever runs (see
	// internal-task #1 — a Get failure or masked refusal must not still upload
	// certificates).
	if err := profilestate.PrepareCredentialsForPayload(ctx, r.client, data, priorState.CredentialsList); err != nil {
		return err
	}

	planned, err := profileplatform.BuildAppleOsXCreateEntity(data)
	if err != nil {
		return err
	}

	entity := profileplatform.OverlayAppleOsXUpdateEntity(live, planned)
	profileplatform.StampGeneralV2(entity.General, profileID, currentVersion)

	if sections := payloadSectionNames(entity); sections != nil {
		tflog.Debug(ctx, "macOS Update payload", map[string]interface{}{"profile_id": profileID, "sections": sections})
	}
	if err := svc.Update(ctx, profileID, entity); err != nil {
		tflog.Error(ctx, "macOS Update failed", map[string]interface{}{"error": err.Error()})
		return err
	}
	return nil
}
