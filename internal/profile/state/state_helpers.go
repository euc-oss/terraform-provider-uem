package state

import (
	"context"
	"fmt"
	"strings"

	commonerrors "github.com/euc-oss/terraform-provider-uem/internal/common/errors"
	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"
	sdk "github.com/euc-oss/terraform-sdk-uem"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func NormalizePasscodeUnknownsToNull(p *profilemodels.PasscodeModel) {
	if p == nil {
		return
	}
	if p.RequirePasscodeOnDevice.IsUnknown() {
		p.RequirePasscodeOnDevice = types.BoolNull()
	}
	if p.AllowSimpleValue.IsUnknown() {
		p.AllowSimpleValue = types.BoolNull()
	}
	if p.RequireAlphanumericValue.IsUnknown() {
		p.RequireAlphanumericValue = types.BoolNull()
	}
	if p.MinimumPasscodeLength.IsUnknown() {
		p.MinimumPasscodeLength = types.Int64Null()
	}
	if p.MinimumNumberOfComplexCharacters.IsUnknown() {
		p.MinimumNumberOfComplexCharacters = types.StringNull()
	}
	if p.MaximumPasscodeAge.IsUnknown() {
		p.MaximumPasscodeAge = types.StringNull()
	}
	if p.AutoLock.IsUnknown() {
		p.AutoLock = types.StringNull()
	}
	if p.GracePeriod.IsUnknown() {
		p.GracePeriod = types.Int64Null()
	}
	if p.MaxFailedAttempts.IsUnknown() {
		p.MaxFailedAttempts = types.StringNull()
	}
	if p.PinHistory.IsUnknown() {
		p.PinHistory = types.StringNull()
	}
	if p.MinutesUntilFailedLoginReset.IsUnknown() {
		p.MinutesUntilFailedLoginReset = types.Int64Null()
	}
}

// NormalizeNetworkUnknownsToNull replaces any Unknown attribute on each
// NetworkItemModel with its typed null. Required after Create because the
// resource schema marks every network attribute Optional+Computed; UEM only
// returns the attributes the caller actually configured, so the rest stay
// Unknown after the post-create readback. Terraform's post-apply consistency
// check then rejects the response. Mirrors NormalizePasscodeUnknownsToNull.
func NormalizeNetworkUnknownsToNull(items []profilemodels.NetworkItemModel) {
	for i := range items {
		n := &items[i]
		if n.NetworkInterface.IsUnknown() {
			n.NetworkInterface = types.StringNull()
		}
		if n.ServiceSetIdentifier.IsUnknown() {
			n.ServiceSetIdentifier = types.StringNull()
		}
		if n.HiddenNetwork.IsUnknown() {
			n.HiddenNetwork = types.BoolNull()
		}
		if n.AutoJoin.IsUnknown() {
			n.AutoJoin = types.BoolNull()
		}
		if n.SecurityType.IsUnknown() {
			n.SecurityType = types.StringNull()
		}
		if n.Password.IsUnknown() {
			n.Password = types.StringNull()
		}
		if n.UseAsLoginWindowConfiguration.IsUnknown() {
			n.UseAsLoginWindowConfiguration = types.BoolNull()
		}
		if n.UseDirectoryAuthentication.IsUnknown() {
			n.UseDirectoryAuthentication = types.BoolNull()
		}
		if n.TLS.IsUnknown() {
			n.TLS = types.BoolNull()
		}
		if n.TTLS.IsUnknown() {
			n.TTLS = types.BoolNull()
		}
		if n.LEAP.IsUnknown() {
			n.LEAP = types.BoolNull()
		}
		if n.PEAP.IsUnknown() {
			n.PEAP = types.BoolNull()
		}
		if n.EAPFAST.IsUnknown() {
			n.EAPFAST = types.BoolNull()
		}
		if n.EAPSIM.IsUnknown() {
			n.EAPSIM = types.BoolNull()
		}
		if n.EAPAKA.IsUnknown() {
			n.EAPAKA = types.BoolNull()
		}
		if n.TLSMinimumVersion.IsUnknown() {
			n.TLSMinimumVersion = types.StringNull()
		}
		if n.TLSMaximumVersion.IsUnknown() {
			n.TLSMaximumVersion = types.StringNull()
		}
		if n.DisableAssociationMACRandomization.IsUnknown() {
			n.DisableAssociationMACRandomization = types.BoolNull()
		}
		if n.UserName.IsUnknown() {
			n.UserName = types.StringNull()
		}
		if n.UserPassword.IsUnknown() {
			n.UserPassword = types.StringNull()
		}
		if n.IdentityCertificate.IsUnknown() {
			n.IdentityCertificate = types.StringNull()
		}
		if n.InnerIdentity.IsUnknown() {
			n.InnerIdentity = types.StringNull()
		}
		if n.OuterIdentity.IsUnknown() {
			n.OuterIdentity = types.StringNull()
		}
		if n.UsePAC.IsUnknown() {
			n.UsePAC = types.BoolNull()
		}
		if n.AllowTwoRANDs.IsUnknown() {
			n.AllowTwoRANDs = types.BoolNull()
		}
		if n.TrustedCertificates.IsUnknown() {
			n.TrustedCertificates = types.ListNull(types.StringType)
		}
		if n.AllowTrustExceptions.IsUnknown() {
			n.AllowTrustExceptions = types.BoolNull()
		}
		if n.ProxyType.IsUnknown() {
			n.ProxyType = types.StringNull()
		}
		if n.ProxyServer.IsUnknown() {
			n.ProxyServer = types.StringNull()
		}
		if n.ProxyServerPort.IsUnknown() {
			n.ProxyServerPort = types.Int64Null()
		}
		if n.ProxyUsername.IsUnknown() {
			n.ProxyUsername = types.StringNull()
		}
		if n.ProxyPassword.IsUnknown() {
			n.ProxyPassword = types.StringNull()
		}
		if n.ProxyUrl.IsUnknown() {
			n.ProxyUrl = types.StringNull()
		}
		if n.PacFallback.IsUnknown() {
			n.PacFallback = types.BoolNull()
		}
	}
}

// NormalizeCredentialsUnknownsToNull mirrors NormalizeNetworkUnknownsToNull
// for credentials_list entries.
func NormalizeCredentialsUnknownsToNull(items []profilemodels.CredentialItemModel) {
	for i := range items {
		c := &items[i]
		if c.CredentialSource.IsUnknown() {
			c.CredentialSource = types.StringNull()
		}
		if c.CredentialName.IsUnknown() {
			c.CredentialName = types.StringNull()
		}
		if c.CertificatePayload.IsUnknown() {
			c.CertificatePayload = types.StringNull()
		}
		if c.CertificatePassword.IsUnknown() {
			c.CertificatePassword = types.StringNull()
		}
		if c.CertificateID.IsUnknown() {
			c.CertificateID = types.Int64Null()
		}
		if c.CertificateAuthority.IsUnknown() {
			c.CertificateAuthority = types.Int64Null()
		}
		if c.CertificateTemplate.IsUnknown() {
			c.CertificateTemplate = types.Int64Null()
		}
		if c.AllowAccessToAllApplications.IsUnknown() {
			c.AllowAccessToAllApplications = types.BoolNull()
		}
		if c.KeyIsExtractable.IsUnknown() {
			c.KeyIsExtractable = types.BoolNull()
		}
	}
}

func IsApplePlatform(platform string) bool {
	return platform == sdk.PlatformAppleOsX || platform == sdk.PlatformAppleiOS
}

func IsValidProfileContext(v string) bool {
	return v == "User" || v == "Device"
}

func EnsureComputedDefaults(data *profilemodels.ProfileResourceModel) {
	if data.ProfileScope.IsNull() || data.ProfileScope.IsUnknown() {
		data.ProfileScope = types.StringValue("Production")
	}
	if data.AssignmentType.IsNull() || data.AssignmentType.IsUnknown() {
		data.AssignmentType = types.StringValue("Auto")
	}
	if data.IsActive.IsNull() || data.IsActive.IsUnknown() {
		data.IsActive = types.BoolValue(true)
	}
	if data.UUID.IsNull() || data.UUID.IsUnknown() {
		data.UUID = types.StringValue("")
	}
	if data.Description.IsNull() || data.Description.IsUnknown() {
		data.Description = types.StringValue("")
	}
}

func UploadCertificatesTyped(ctx context.Context, client *sdk.Client, items, priorCreds []profilemodels.CredentialItemModel) ([]profilemodels.CredentialItemModel, error) {
	priorByName := make(map[string]profilemodels.CredentialItemModel, len(priorCreds))
	for _, p := range priorCreds {
		if key, ok := CredentialKey(p); ok {
			priorByName[key] = p
		}
	}

	out := make([]profilemodels.CredentialItemModel, len(items))
	copy(out, items)
	var uploadSvc *sdk.ProfilesV1Service
	for i, c := range out {
		if c.CredentialSource.ValueString() != "Upload" {
			continue
		}
		if c.CertificatePayload.IsNull() || c.CertificatePayload.IsUnknown() || c.CertificatePayload.ValueString() == "" {
			continue
		}

		if key, ok := CredentialKey(c); ok {
			if prior, found := priorByName[key]; found {
				samePayload := !prior.CertificatePayload.IsNull() &&
					prior.CertificatePayload.ValueString() == c.CertificatePayload.ValueString()
				hasID := !prior.CertificateID.IsNull() && prior.CertificateID.ValueInt64() != 0
				if samePayload && hasID {
					out[i].CertificateID = prior.CertificateID
					continue
				}
			}
		}

		if !c.CertificateID.IsNull() && !c.CertificateID.IsUnknown() && c.CertificateID.ValueInt64() != 0 {
			continue
		}

		name := credentialDisplayName(c, i)

		if client == nil {
			return nil, fmt.Errorf("credential %q requires certificate upload but no SDK client was provided: %w", name, commonerrors.ErrCertificateUploadUnsupported)
		}

		if uploadSvc == nil {
			uploadSvc = sdk.NewProfilesV1Service(client)
		}

		_, resp, err := uploadSvc.UploadCertificate(ctx, &sdk.CertificateV1{
			CertificatePayload: c.CertificatePayload.ValueString(),
			Password:           c.CertificatePassword.ValueString(),
		})
		if err != nil {
			return nil, fmt.Errorf("upload certificate %q: %w", name, err)
		}
		if resp == nil || resp.Value == nil {
			return nil, fmt.Errorf("upload certificate %q: empty response (no certificate ID)", name)
		}
		out[i].CertificateID = types.Int64Value(*resp.Value)
	}
	return out, nil
}

func PrepareCredentialsForPayload(ctx context.Context, client *sdk.Client, data *profilemodels.ProfileResourceModel, priorCreds []profilemodels.CredentialItemModel) error {
	if len(data.CredentialsList) == 0 {
		return nil
	}
	if !AnyNetworkReferencesCredential(data.NetworkList, data.CredentialsList) {
		data.CredentialsList = nil
		return nil
	}
	updated, err := UploadCertificatesTyped(ctx, client, data.CredentialsList, priorCreds)
	if err != nil {
		return err
	}
	data.CredentialsList = updated
	return nil
}

func AnyNetworkReferencesCredential(networks []profilemodels.NetworkItemModel, creds []profilemodels.CredentialItemModel) bool {
	credNames := make(map[string]struct{}, len(creds))
	for _, c := range creds {
		if !c.CredentialName.IsNull() && !c.CredentialName.IsUnknown() {
			credNames[c.CredentialName.ValueString()] = struct{}{}
		}
	}
	for _, n := range networks {
		if !n.IdentityCertificate.IsNull() && !n.IdentityCertificate.IsUnknown() {
			if _, ok := credNames[n.IdentityCertificate.ValueString()]; ok {
				return true
			}
		}
	}
	return false
}

func CredentialKey(item profilemodels.CredentialItemModel) (string, bool) {
	if item.CredentialName.IsNull() || item.CredentialName.IsUnknown() {
		return "", false
	}
	name := strings.TrimSpace(item.CredentialName.ValueString())
	if name == "" {
		return "", false
	}
	return name, true
}

func MergeCredentialsListWithPriorState(apiList, stateList []profilemodels.CredentialItemModel) []profilemodels.CredentialItemModel {
	if apiList == nil {
		return nil
	}

	stateByName := make(map[string]profilemodels.CredentialItemModel, len(stateList))
	for _, s := range stateList {
		if key, ok := CredentialKey(s); ok {
			stateByName[key] = s
		}
	}

	for i := range apiList {
		var (
			s       profilemodels.CredentialItemModel
			matched bool
		)

		// Try name-keyed match first (stable across reorderings).
		if key, ok := CredentialKey(apiList[i]); ok {
			s, matched = stateByName[key]
		}

		// Fall back to index match. Required for Upload credentials whose
		// CredentialName UEM auto-rewrites to "Certificate #N" — the live
		// API name no longer matches the user-supplied prior-state name, so
		// the keyed lookup misses. Position-by-index recovers them.
		if !matched && i < len(stateList) {
			s = stateList[i]
			matched = true
		}

		if !matched {
			continue
		}

		if apiList[i].CertificatePayload.IsNull() && !s.CertificatePayload.IsNull() {
			apiList[i].CertificatePayload = s.CertificatePayload
		}
		if apiList[i].CertificatePassword.IsNull() && !s.CertificatePassword.IsNull() {
			apiList[i].CertificatePassword = s.CertificatePassword
		}
		// CredentialSource and CredentialName are user-owned identifiers that
		// UEM may normalize ("DefinedCA" → "DefinedCertificateAuthority") or
		// auto-rename ("Certificate #N"). Pin to prior state when the user
		// previously set them — same pattern as IdentityCertificate in the
		// network merge — otherwise the post-apply consistency check fails
		// with "inconsistent values for sensitive attribute" because
		// credentials_list contains sensitive child fields.
		if !s.CredentialSource.IsNull() {
			apiList[i].CredentialSource = s.CredentialSource
		} else {
			apiList[i].CredentialSource = KeepStateString(apiList[i].CredentialSource, s.CredentialSource)
		}
		if !s.CredentialName.IsNull() {
			apiList[i].CredentialName = s.CredentialName
		} else {
			apiList[i].CredentialName = KeepStateString(apiList[i].CredentialName, s.CredentialName)
		}
		// These are Optional (not Computed) on the schema, so when the user
		// leaves them null the plan keeps them null. UEM returns concrete
		// values for unset fields (false for the bools, 0 for the ints), and
		// KeepStateBool/Int64 would let those win — flipping plan(null) →
		// state(false/0) and tripping the post-apply consistency check
		// ("inconsistent values for sensitive attribute" because the
		// containing list has Sensitive children). PreserveNull* keeps the
		// user-null semantics: state was null → final null; otherwise let
		// the API value through.
		apiList[i].AllowAccessToAllApplications = PreserveNullBool(apiList[i].AllowAccessToAllApplications, s.AllowAccessToAllApplications)
		apiList[i].KeyIsExtractable = PreserveNullBool(apiList[i].KeyIsExtractable, s.KeyIsExtractable)
		apiList[i].CertificateAuthority = PreserveNullInt64(apiList[i].CertificateAuthority, s.CertificateAuthority)
		apiList[i].CertificateTemplate = PreserveNullInt64(apiList[i].CertificateTemplate, s.CertificateTemplate)
	}
	return apiList
}

func MergeNetworkListWithPriorState(apiList, stateList []profilemodels.NetworkItemModel) []profilemodels.NetworkItemModel {
	if apiList == nil {
		return nil
	}
	for i := range apiList {
		if i >= len(stateList) {
			break
		}
		mergeNetworkItem(&apiList[i], &stateList[i])
	}
	return apiList
}

func KeepStateString(apiVal, stateVal types.String) types.String {
	if apiVal.IsNull() || apiVal.IsUnknown() {
		return stateVal
	}
	return apiVal
}

func KeepStateBool(apiVal, stateVal types.Bool) types.Bool {
	if apiVal.IsNull() || apiVal.IsUnknown() {
		return stateVal
	}
	return apiVal
}

func KeepStateInt64(apiVal, stateVal types.Int64) types.Int64 {
	if apiVal.IsNull() || apiVal.IsUnknown() {
		return stateVal
	}
	return apiVal
}

// MergeGatekeeperWithPriorState reconciles UEM's response with the user's prior
// config. UEM always returns the GateKeeper block populated for macOS profiles
// — even fields the caller didn't send come back with API defaults. On import
// (state is nil) we surface every field so `terraform output | jq` reveals
// the full live policy and the user can round-trip it into tfvars. On normal
// reads (state non-nil) we suppress fields the user left null so each field
// stays managed independently.
func MergeGatekeeperWithPriorState(api, state *profilemodels.GatekeeperModel) *profilemodels.GatekeeperModel {
	if api == nil {
		return nil
	}
	if state == nil {
		return api
	}
	if state.AllowAutoUnlock.IsNull() {
		api.AllowAutoUnlock = types.BoolNull()
	}
	if state.AllowFingerprintForUnlock.IsNull() {
		api.AllowFingerprintForUnlock = types.BoolNull()
	}
	if state.AllowHandoff.IsNull() {
		api.AllowHandoff = types.BoolNull()
	}
	if state.AllowScreenCapture.IsNull() {
		api.AllowScreenCapture = types.BoolNull()
	}
	if state.EnableAppSoftwareUpdateDelay.IsNull() {
		api.EnableAppSoftwareUpdateDelay = types.BoolNull()
	}
	if state.EnableSoftwareUpdateDelay.IsNull() {
		api.EnableSoftwareUpdateDelay = types.BoolNull()
	}
	if state.EnforcedSoftwareUpdateDelay.IsNull() {
		api.EnforcedSoftwareUpdateDelay = types.Int64Null()
	}
	return api
}

func MergeDiskEncryptionWithPriorState(api, state *profilemodels.DiskEncryptionModel) *profilemodels.DiskEncryptionModel {
	if api == nil {
		return nil
	}
	if state == nil {
		return api
	}
	// Drop API-only sub-blocks that the user did not configure. The WS1 API
	// returns server-side defaults for some sub-blocks (e.g. DiskEncryptionMCX
	// with DestroyFVKeyOnStandby=false) even when the caller did not include
	// them in the request. Surfacing those defaults would conflict with a plan
	// where the corresponding nested attribute is null.
	if state.AirWatch == nil {
		api.AirWatch = nil
	}
	if state.FileVault == nil {
		api.FileVault = nil
	}
	if state.MCX == nil {
		api.MCX = nil
	}
	if api.AirWatch != nil && state.AirWatch != nil {
		mergeAirWatch(api.AirWatch, state.AirWatch)
	}
	if api.FileVault != nil && state.FileVault != nil {
		mergeFileVault(api.FileVault, state.FileVault)
	}
	if api.MCX != nil && state.MCX != nil {
		api.MCX.DestroyFVKeyOnStandby = KeepStateBool(api.MCX.DestroyFVKeyOnStandby, state.MCX.DestroyFVKeyOnStandby)
	}
	if api.AirWatch == nil && api.FileVault == nil && api.MCX == nil {
		return nil
	}
	return api
}

func mergeNetworkItem(api, state *profilemodels.NetworkItemModel) {
	api.NetworkInterface = KeepStateString(api.NetworkInterface, state.NetworkInterface)
	api.ServiceSetIdentifier = KeepStateString(api.ServiceSetIdentifier, state.ServiceSetIdentifier)
	api.HiddenNetwork = KeepStateBool(api.HiddenNetwork, state.HiddenNetwork)
	api.AutoJoin = KeepStateBool(api.AutoJoin, state.AutoJoin)
	api.SecurityType = KeepStateString(api.SecurityType, state.SecurityType)
	api.Password = KeepStateString(api.Password, state.Password)
	api.UseAsLoginWindowConfiguration = KeepStateBool(api.UseAsLoginWindowConfiguration, state.UseAsLoginWindowConfiguration)
	api.UseDirectoryAuthentication = KeepStateBool(api.UseDirectoryAuthentication, state.UseDirectoryAuthentication)
	api.TLS = KeepStateBool(api.TLS, state.TLS)
	api.TTLS = KeepStateBool(api.TTLS, state.TTLS)
	api.LEAP = KeepStateBool(api.LEAP, state.LEAP)
	api.PEAP = KeepStateBool(api.PEAP, state.PEAP)
	api.EAPFAST = KeepStateBool(api.EAPFAST, state.EAPFAST)
	api.EAPSIM = KeepStateBool(api.EAPSIM, state.EAPSIM)
	api.EAPAKA = KeepStateBool(api.EAPAKA, state.EAPAKA)
	api.TLSMinimumVersion = KeepStateString(api.TLSMinimumVersion, state.TLSMinimumVersion)
	api.TLSMaximumVersion = KeepStateString(api.TLSMaximumVersion, state.TLSMaximumVersion)
	api.DisableAssociationMACRandomization = KeepStateBool(api.DisableAssociationMACRandomization, state.DisableAssociationMACRandomization)
	api.UserName = KeepStateString(api.UserName, state.UserName)
	api.UserPassword = KeepStateString(api.UserPassword, state.UserPassword)
	// IdentityCertificate is the join key into credentials_list. UEM
	// auto-rewrites the value to its server-side credential name (e.g.
	// "Certificate #1") whenever the credential is reissued, even when the
	// caller sent a stable user-chosen name. Pin to prior state when the
	// user previously set one — otherwise the join breaks and Terraform
	// shows a permanent in-place diff on every plan.
	if !state.IdentityCertificate.IsNull() {
		api.IdentityCertificate = state.IdentityCertificate
	} else {
		api.IdentityCertificate = KeepStateString(api.IdentityCertificate, state.IdentityCertificate)
	}
	api.InnerIdentity = KeepStateString(api.InnerIdentity, state.InnerIdentity)
	api.OuterIdentity = KeepStateString(api.OuterIdentity, state.OuterIdentity)
	api.UsePAC = KeepStateBool(api.UsePAC, state.UsePAC)
	api.AllowTwoRANDs = KeepStateBool(api.AllowTwoRANDs, state.AllowTwoRANDs)
	api.AllowTrustExceptions = KeepStateBool(api.AllowTrustExceptions, state.AllowTrustExceptions)
	api.ProxyType = KeepStateString(api.ProxyType, state.ProxyType)
	api.ProxyServer = KeepStateString(api.ProxyServer, state.ProxyServer)
	api.ProxyServerPort = KeepStateInt64(api.ProxyServerPort, state.ProxyServerPort)
	api.ProxyUsername = KeepStateString(api.ProxyUsername, state.ProxyUsername)
	api.ProxyPassword = KeepStateString(api.ProxyPassword, state.ProxyPassword)
	api.ProxyUrl = KeepStateString(api.ProxyUrl, state.ProxyUrl)
	api.PacFallback = KeepStateBool(api.PacFallback, state.PacFallback)
	if api.TrustedCertificates.IsNull() && !state.TrustedCertificates.IsNull() {
		api.TrustedCertificates = state.TrustedCertificates
	}
}

func mergeAirWatch(api, state *profilemodels.DiskEncryptionAirWatchModel) {
	api.StoreKey = KeepStateBool(api.StoreKey, state.StoreKey)
	api.RotateKeyAfter = KeepStateInt64(api.RotateKeyAfter, state.RotateKeyAfter)
	api.UseIntelligentHub = KeepStateBool(api.UseIntelligentHub, state.UseIntelligentHub)
	api.NotifyUserForEncryption = KeepStateBool(api.NotifyUserForEncryption, state.NotifyUserForEncryption)
	api.EncryptionNotificationTitle = KeepStateString(api.EncryptionNotificationTitle, state.EncryptionNotificationTitle)
	api.EncryptionNotificationMessage = KeepStateString(api.EncryptionNotificationMessage, state.EncryptionNotificationMessage)
	api.EncryptionMaxNotifyAttempts = KeepStateInt64(api.EncryptionMaxNotifyAttempts, state.EncryptionMaxNotifyAttempts)
	api.EncryptionNotificationRetryIntervalInHours = KeepStateInt64(api.EncryptionNotificationRetryIntervalInHours, state.EncryptionNotificationRetryIntervalInHours)
	api.EncryptionActionAfterLastNotification = KeepStateInt64(api.EncryptionActionAfterLastNotification, state.EncryptionActionAfterLastNotification)
	api.EnableRecoveryKey = KeepStateBool(api.EnableRecoveryKey, state.EnableRecoveryKey)
	api.RecoveryKeyNotificationTitle = KeepStateString(api.RecoveryKeyNotificationTitle, state.RecoveryKeyNotificationTitle)
	api.RecoveryKeyNotificationMessage = KeepStateString(api.RecoveryKeyNotificationMessage, state.RecoveryKeyNotificationMessage)
	api.RecoveryKeyNotificationRetryIntervalInHours = KeepStateInt64(api.RecoveryKeyNotificationRetryIntervalInHours, state.RecoveryKeyNotificationRetryIntervalInHours)
	api.RecoveryKeyPromptTitle = KeepStateString(api.RecoveryKeyPromptTitle, state.RecoveryKeyPromptTitle)
	api.RecoveryKeyPromptMessage = KeepStateString(api.RecoveryKeyPromptMessage, state.RecoveryKeyPromptMessage)
	api.RecoveryKeySuccessTitle = KeepStateString(api.RecoveryKeySuccessTitle, state.RecoveryKeySuccessTitle)
	api.RecoveryKeySuccessMessage = KeepStateString(api.RecoveryKeySuccessMessage, state.RecoveryKeySuccessMessage)
	api.RecoveryKeyErrorTitle = KeepStateString(api.RecoveryKeyErrorTitle, state.RecoveryKeyErrorTitle)
	api.RecoveryKeyErrorMessage = KeepStateString(api.RecoveryKeyErrorMessage, state.RecoveryKeyErrorMessage)
	api.RecoveryKeyMaxFailureCount = KeepStateInt64(api.RecoveryKeyMaxFailureCount, state.RecoveryKeyMaxFailureCount)
}

func mergeFileVault(api, state *profilemodels.DiskEncryptionFileVaultModel) {
	api.Enable = KeepStateBool(api.Enable, state.Enable)
	api.ShowRecoveryKey = KeepStateBool(api.ShowRecoveryKey, state.ShowRecoveryKey)
	api.RecoveryType = KeepStateInt64(api.RecoveryType, state.RecoveryType)
	api.FileVaultEnterpriseCertificate = KeepStateString(api.FileVaultEnterpriseCertificate, state.FileVaultEnterpriseCertificate)
	api.FileVaultUser = KeepStateInt64(api.FileVaultUser, state.FileVaultUser)
	api.Username = KeepStateString(api.Username, state.Username)
	api.PromptToEnableFileVaultAt = KeepStateInt64(api.PromptToEnableFileVaultAt, state.PromptToEnableFileVaultAt)
	api.NumberOfTimesUserCanBypass = KeepStateInt64(api.NumberOfTimesUserCanBypass, state.NumberOfTimesUserCanBypass)
}

func credentialDisplayName(item profilemodels.CredentialItemModel, idx int) string {
	if key, ok := CredentialKey(item); ok {
		return key
	}
	return fmt.Sprintf("index_%d", idx)
}

// KeepStateList returns apiVal unless it is null/unknown, in which case the
// prior stateVal is preserved. Used for optional string-list attributes so
// fields the API omits don't show as drift.
func KeepStateList(apiVal, stateVal types.List) types.List {
	if apiVal.IsNull() || apiVal.IsUnknown() {
		return stateVal
	}
	return apiVal
}

// PreserveNullBool / PreserveNullString / PreserveNullInt64 / PreserveNullStringList
// implement "user-didn't-configure-it" semantics for Optional (non-Computed)
// nested attributes. If the prior state value was null the caller never
// managed that field, so server-side defaults from the API must be discarded
// (returning null), otherwise Terraform's post-apply consistency check fails
// with "was null, but now cty.False" / etc. When the caller did set a value
// in state, the API value wins so genuine drift is surfaced on Read.
func PreserveNullBool(apiVal, stateVal types.Bool) types.Bool {
	if stateVal.IsNull() {
		return types.BoolNull()
	}
	if apiVal.IsNull() || apiVal.IsUnknown() {
		return stateVal
	}
	return apiVal
}

func PreserveNullString(apiVal, stateVal types.String) types.String {
	if stateVal.IsNull() {
		return types.StringNull()
	}
	if apiVal.IsNull() || apiVal.IsUnknown() {
		return stateVal
	}
	return apiVal
}

func PreserveNullInt64(apiVal, stateVal types.Int64) types.Int64 {
	if stateVal.IsNull() {
		return types.Int64Null()
	}
	if apiVal.IsNull() || apiVal.IsUnknown() {
		return stateVal
	}
	return apiVal
}

// PreserveNullStringList is the string-list variant of the PreserveNull*
// helpers. The element type is hardcoded to types.StringType; callers needing
// other element types must derive the null value themselves rather than reuse
// this helper.
func PreserveNullStringList(apiVal, stateVal types.List) types.List {
	if stateVal.IsNull() {
		return types.ListNull(types.StringType)
	}
	if apiVal.IsNull() || apiVal.IsUnknown() {
		return stateVal
	}
	return apiVal
}

// stringSliceToTFList converts a []string from the SDK into a Terraform
// types.List. Returns ListNull when the slice is empty so no spurious diff
// is produced.
func stringSliceToTFList(in []string) types.List {
	if len(in) == 0 {
		return types.ListNull(types.StringType)
	}
	listVal, diags := types.ListValueFrom(context.Background(), types.StringType, in)
	if diags.HasError() {
		return types.ListNull(types.StringType)
	}
	return listVal
}

// MergeRestrictionsWithPriorState folds API-derived restrictions values into
// the prior Terraform state so that server-side defaults for fields the user
// never configured don't show up as drift on subsequent plans. Mirrors
// MergeDiskEncryptionWithPriorState.
func MergeRestrictionsWithPriorState(api, state *profilemodels.RestrictionsModel) *profilemodels.RestrictionsModel {
	if api == nil {
		return nil
	}
	if state == nil {
		return api
	}
	if state.Applications == nil {
		api.Applications = nil
	}
	if state.Desktop == nil {
		api.Desktop = nil
	}
	if state.Functionality == nil {
		api.Functionality = nil
	}
	if state.Media == nil {
		api.Media = nil
	}
	if state.Preferences == nil {
		api.Preferences = nil
	}
	if state.Sharing == nil {
		api.Sharing = nil
	}
	if state.Widgets == nil {
		api.Widgets = nil
	}
	// Inverse: if the API omits a sub-block the user previously configured
	// (mappers may collapse empty payloads to nil), keep the prior-state
	// sub-block so a Read doesn't silently drop user config and produce a
	// perpetual diff.
	if api.Applications == nil && state.Applications != nil {
		api.Applications = state.Applications
	}
	if api.Desktop == nil && state.Desktop != nil {
		api.Desktop = state.Desktop
	}
	if api.Functionality == nil && state.Functionality != nil {
		api.Functionality = state.Functionality
	}
	if api.Media == nil && state.Media != nil {
		api.Media = state.Media
	}
	if api.Preferences == nil && state.Preferences != nil {
		api.Preferences = state.Preferences
	}
	if api.Sharing == nil && state.Sharing != nil {
		api.Sharing = state.Sharing
	}
	if api.Widgets == nil && state.Widgets != nil {
		api.Widgets = state.Widgets
	}
	if api.Applications != nil && state.Applications != nil {
		mergeRestrictionsApplications(api.Applications, state.Applications)
	}
	if api.Desktop != nil && state.Desktop != nil {
		api.Desktop.DesktopPicturePath = PreserveNullString(api.Desktop.DesktopPicturePath, state.Desktop.DesktopPicturePath)
		api.Desktop.LockDesktopPicture = PreserveNullBool(api.Desktop.LockDesktopPicture, state.Desktop.LockDesktopPicture)
	}
	if api.Functionality != nil && state.Functionality != nil {
		mergeRestrictionsFunctionality(api.Functionality, state.Functionality)
	}
	if api.Media != nil && state.Media != nil {
		mergeRestrictionsMedia(api.Media, state.Media)
	}
	if api.Preferences != nil && state.Preferences != nil {
		mergeRestrictionsPreferences(api.Preferences, state.Preferences)
	}
	if api.Sharing != nil && state.Sharing != nil {
		mergeRestrictionsSharing(api.Sharing, state.Sharing)
	}
	if api.Widgets != nil && state.Widgets != nil {
		api.Widgets.AllowOnlyConfiguredWidgets = PreserveNullBool(api.Widgets.AllowOnlyConfiguredWidgets, state.Widgets.AllowOnlyConfiguredWidgets)
		api.Widgets.AllowedWidgets = PreserveNullStringList(api.Widgets.AllowedWidgets, state.Widgets.AllowedWidgets)
	}
	if api.Applications == nil && api.Desktop == nil && api.Functionality == nil &&
		api.Media == nil && api.Preferences == nil && api.Sharing == nil && api.Widgets == nil {
		return nil
	}
	return api
}

func mergeRestrictionsApplications(api, state *profilemodels.RestrictionsApplicationsModel) {
	api.AllowApplication = PreserveNullStringList(api.AllowApplication, state.AllowApplication)
	api.AllowFolders = PreserveNullStringList(api.AllowFolders, state.AllowFolders)
	api.DisallowFolders = PreserveNullStringList(api.DisallowFolders, state.DisallowFolders)
	api.RestrictWhichApplicationsAreAllowedToLaunch = PreserveNullBool(api.RestrictWhichApplicationsAreAllowedToLaunch, state.RestrictWhichApplicationsAreAllowedToLaunch)
	if state.AppStore == nil {
		api.AppStore = nil
	} else if api.AppStore != nil {
		api.AppStore.AllowAppStoreAppAdoption = PreserveNullBool(api.AppStore.AllowAppStoreAppAdoption, state.AppStore.AllowAppStoreAppAdoption)
		api.AppStore.RequireAdminPasswordToInstallOrUpdateApp = PreserveNullBool(api.AppStore.RequireAdminPasswordToInstallOrUpdateApp, state.AppStore.RequireAdminPasswordToInstallOrUpdateApp)
		api.AppStore.RestrictAppStoreToSoftwareUpdatesOnly = PreserveNullBool(api.AppStore.RestrictAppStoreToSoftwareUpdatesOnly, state.AppStore.RestrictAppStoreToSoftwareUpdatesOnly)
	}
	if state.AppleMusic == nil {
		api.AppleMusic = nil
	} else if api.AppleMusic != nil {
		api.AppleMusic.AllowMusicService = PreserveNullBool(api.AppleMusic.AllowMusicService, state.AppleMusic.AllowMusicService)
	}
	if state.Camera == nil {
		api.Camera = nil
	} else if api.Camera != nil {
		api.Camera.AllowUseOfBuiltInCamera = PreserveNullBool(api.Camera.AllowUseOfBuiltInCamera, state.Camera.AllowUseOfBuiltInCamera)
	}
	if state.GameCentre == nil {
		api.GameCentre = nil
	} else if api.GameCentre != nil {
		api.GameCentre.AllowAddingGameCenterFriends = PreserveNullBool(api.GameCentre.AllowAddingGameCenterFriends, state.GameCentre.AllowAddingGameCenterFriends)
		api.GameCentre.AllowGameCenterModification = PreserveNullBool(api.GameCentre.AllowGameCenterModification, state.GameCentre.AllowGameCenterModification)
		api.GameCentre.AllowMultiplayerGaming = PreserveNullBool(api.GameCentre.AllowMultiplayerGaming, state.GameCentre.AllowMultiplayerGaming)
		api.GameCentre.AllowUseOfGameCenter = PreserveNullBool(api.GameCentre.AllowUseOfGameCenter, state.GameCentre.AllowUseOfGameCenter)
	}
	if state.Safari == nil {
		api.Safari = nil
	} else if api.Safari != nil {
		api.Safari.AllowDeprecatedWebKitTls = PreserveNullBool(api.Safari.AllowDeprecatedWebKitTls, state.Safari.AllowDeprecatedWebKitTls)
		api.Safari.AllowSafariAutoFill = PreserveNullBool(api.Safari.AllowSafariAutoFill, state.Safari.AllowSafariAutoFill)
	}
}

func mergeRestrictionsFunctionality(api, state *profilemodels.RestrictionsFunctionalityModel) {
	if state.AirPrint == nil {
		api.AirPrint = nil
	} else if api.AirPrint != nil {
		api.AirPrint.AllowAirPrint = PreserveNullBool(api.AirPrint.AllowAirPrint, state.AirPrint.AllowAirPrint)
		api.AirPrint.AllowAirPrintiBeaconDiscovery = PreserveNullBool(api.AirPrint.AllowAirPrintiBeaconDiscovery, state.AirPrint.AllowAirPrintiBeaconDiscovery)
		api.AirPrint.ForceAirPrintTrustedTLSRequirement = PreserveNullBool(api.AirPrint.ForceAirPrintTrustedTLSRequirement, state.AirPrint.ForceAirPrintTrustedTLSRequirement)
	}
	if state.ContentCaching == nil {
		api.ContentCaching = nil
	} else if api.ContentCaching != nil {
		api.ContentCaching.AllowContentCaching = PreserveNullBool(api.ContentCaching.AllowContentCaching, state.ContentCaching.AllowContentCaching)
	}
	if state.ICloud == nil {
		api.ICloud = nil
	} else if api.ICloud != nil {
		mergeRestrictionsICloud(api.ICloud, state.ICloud)
	}
	if state.Passwords == nil {
		api.Passwords = nil
	} else if api.Passwords != nil {
		api.Passwords.AllowPasswordAutoFill = PreserveNullBool(api.Passwords.AllowPasswordAutoFill, state.Passwords.AllowPasswordAutoFill)
		api.Passwords.AllowPasswordProximityRequests = PreserveNullBool(api.Passwords.AllowPasswordProximityRequests, state.Passwords.AllowPasswordProximityRequests)
		api.Passwords.AllowPasswordSharing = PreserveNullBool(api.Passwords.AllowPasswordSharing, state.Passwords.AllowPasswordSharing)
	}
	if state.Spotlight == nil {
		api.Spotlight = nil
	} else if api.Spotlight != nil {
		api.Spotlight.AllowSpotlightSuggestions = PreserveNullBool(api.Spotlight.AllowSpotlightSuggestions, state.Spotlight.AllowSpotlightSuggestions)
	}
}

func mergeRestrictionsICloud(api, state *profilemodels.RestrictionsICloudModel) {
	api.AllowAirPrint = PreserveNullBool(api.AllowAirPrint, state.AllowAirPrint)
	api.AllowAirPrintiBeaconDiscovery = PreserveNullBool(api.AllowAirPrintiBeaconDiscovery, state.AllowAirPrintiBeaconDiscovery)
	api.AllowCloudDesktopAndDocuments = PreserveNullBool(api.AllowCloudDesktopAndDocuments, state.AllowCloudDesktopAndDocuments)
	api.AllowDeprecatedWebKitTls = PreserveNullBool(api.AllowDeprecatedWebKitTls, state.AllowDeprecatedWebKitTls)
	api.AllowICloudFMM = PreserveNullBool(api.AllowICloudFMM, state.AllowICloudFMM)
	api.AllowIcloudAddressBook = PreserveNullBool(api.AllowIcloudAddressBook, state.AllowIcloudAddressBook)
	api.AllowIcloudBTMM = PreserveNullBool(api.AllowIcloudBTMM, state.AllowIcloudBTMM)
	api.AllowIcloudBookmarks = PreserveNullBool(api.AllowIcloudBookmarks, state.AllowIcloudBookmarks)
	api.AllowIcloudCalendar = PreserveNullBool(api.AllowIcloudCalendar, state.AllowIcloudCalendar)
	api.AllowIcloudDocumentsAndData = PreserveNullBool(api.AllowIcloudDocumentsAndData, state.AllowIcloudDocumentsAndData)
	api.AllowIcloudKeychainSync = PreserveNullBool(api.AllowIcloudKeychainSync, state.AllowIcloudKeychainSync)
	api.AllowIcloudMail = PreserveNullBool(api.AllowIcloudMail, state.AllowIcloudMail)
	api.AllowIcloudNotes = PreserveNullBool(api.AllowIcloudNotes, state.AllowIcloudNotes)
	api.AllowIcloudReminders = PreserveNullBool(api.AllowIcloudReminders, state.AllowIcloudReminders)
	api.AllowPasswordAutoFill = PreserveNullBool(api.AllowPasswordAutoFill, state.AllowPasswordAutoFill)
	api.AllowPasswordProximityRequests = PreserveNullBool(api.AllowPasswordProximityRequests, state.AllowPasswordProximityRequests)
	api.AllowPasswordSharing = PreserveNullBool(api.AllowPasswordSharing, state.AllowPasswordSharing)
	api.AllowUseIcloudPasswordForLocalAccounts = PreserveNullBool(api.AllowUseIcloudPasswordForLocalAccounts, state.AllowUseIcloudPasswordForLocalAccounts)
	api.ForceAirPrintTrustedTLSRequirement = PreserveNullBool(api.ForceAirPrintTrustedTLSRequirement, state.ForceAirPrintTrustedTLSRequirement)
}

func mergeRestrictionsMedia(api, state *profilemodels.RestrictionsMediaModel) {
	api.AutoEjectMedia = PreserveNullBool(api.AutoEjectMedia, state.AutoEjectMedia)
	mergeMediaAccessPtr(&api.DiskMediaCDs, state.DiskMediaCDs)
	mergeMediaAccessPtr(&api.DiskMediaDVDs, state.DiskMediaDVDs)
	mergeMediaAccessPtr(&api.ExternalHardDiskMediaAccess, state.ExternalHardDiskMediaAccess)
	mergeMediaAccessPtr(&api.HardDiskDvdRam, state.HardDiskDvdRam)
	mergeMediaAccessPtr(&api.HardDiskImages, state.HardDiskImages)
	mergeMediaAccessPtr(&api.InternalHardDiskMediaAccess, state.InternalHardDiskMediaAccess)
	if state.NetworkAccess == nil {
		api.NetworkAccess = nil
	} else if api.NetworkAccess != nil {
		api.NetworkAccess.AirDrop = PreserveNullBool(api.NetworkAccess.AirDrop, state.NetworkAccess.AirDrop)
	}
	if state.RecordableDisc == nil {
		api.RecordableDisc = nil
	} else {
		if api.RecordableDisc == nil {
			api.RecordableDisc = &profilemodels.RestrictionsBurnSupportModel{}
		}
		mergeMediaAccessPtr(&api.RecordableDisc.BurnSupport, state.RecordableDisc.BurnSupport)
	}
}

func mergeMediaAccessPtr(apiPtr **profilemodels.RestrictionsMediaAccessModel, state *profilemodels.RestrictionsMediaAccessModel) {
	if state == nil {
		*apiPtr = nil
		return
	}
	if *apiPtr == nil {
		*apiPtr = &profilemodels.RestrictionsMediaAccessModel{
			Allow:        state.Allow,
			Authenticate: state.Authenticate,
			ReadOnly:     state.ReadOnly,
		}
		return
	}
	(*apiPtr).Allow = PreserveNullBool((*apiPtr).Allow, state.Allow)
	(*apiPtr).Authenticate = PreserveNullBool((*apiPtr).Authenticate, state.Authenticate)
	(*apiPtr).ReadOnly = PreserveNullBool((*apiPtr).ReadOnly, state.ReadOnly)
}

func mergeRestrictionsPreferences(api, state *profilemodels.RestrictionsPreferencesModel) {
	api.Accessibility = PreserveNullBool(api.Accessibility, state.Accessibility)
	api.AppStore = PreserveNullBool(api.AppStore, state.AppStore)
	api.Bluetooth = PreserveNullBool(api.Bluetooth, state.Bluetooth)
	api.CDsAndDVDs = PreserveNullBool(api.CDsAndDVDs, state.CDsAndDVDs)
	api.DateAndTime = PreserveNullBool(api.DateAndTime, state.DateAndTime)
	api.DesktopAndScreenSaver = PreserveNullBool(api.DesktopAndScreenSaver, state.DesktopAndScreenSaver)
	api.DictationAndSpeech = PreserveNullBool(api.DictationAndSpeech, state.DictationAndSpeech)
	api.Displays = PreserveNullBool(api.Displays, state.Displays)
	api.Dock = PreserveNullBool(api.Dock, state.Dock)
	api.EnabledPreferencePanes = PreserveNullBool(api.EnabledPreferencePanes, state.EnabledPreferencePanes)
	api.EnergySaver = PreserveNullBool(api.EnergySaver, state.EnergySaver)
	api.Extensions = PreserveNullBool(api.Extensions, state.Extensions)
	api.FibreChannel = PreserveNullBool(api.FibreChannel, state.FibreChannel)
	api.FlashPlayer = PreserveNullBool(api.FlashPlayer, state.FlashPlayer)
	api.General = PreserveNullBool(api.General, state.General)
	api.Ink = PreserveNullBool(api.Ink, state.Ink)
	api.InternetAccounts = PreserveNullBool(api.InternetAccounts, state.InternetAccounts)
	api.Keyboard = PreserveNullBool(api.Keyboard, state.Keyboard)
	api.LanguageAndText = PreserveNullBool(api.LanguageAndText, state.LanguageAndText)
	api.MissionControl = PreserveNullBool(api.MissionControl, state.MissionControl)
	api.MobileMe = PreserveNullBool(api.MobileMe, state.MobileMe)
	api.Mouse = PreserveNullBool(api.Mouse, state.Mouse)
	api.Network = PreserveNullBool(api.Network, state.Network)
	api.Notifications = PreserveNullBool(api.Notifications, state.Notifications)
	api.ParentalControls = PreserveNullBool(api.ParentalControls, state.ParentalControls)
	api.PreferenceBehavior = PreserveNullString(api.PreferenceBehavior, state.PreferenceBehavior)
	api.PrintAndScan = PreserveNullBool(api.PrintAndScan, state.PrintAndScan)
	api.Profiles = PreserveNullBool(api.Profiles, state.Profiles)
	api.SecurityAndPrivacy = PreserveNullBool(api.SecurityAndPrivacy, state.SecurityAndPrivacy)
	api.Sharing = PreserveNullBool(api.Sharing, state.Sharing)
	api.SoftwareUpdate = PreserveNullBool(api.SoftwareUpdate, state.SoftwareUpdate)
	api.Sound = PreserveNullBool(api.Sound, state.Sound)
	api.Spotlight = PreserveNullBool(api.Spotlight, state.Spotlight)
	api.StartupDisk = PreserveNullBool(api.StartupDisk, state.StartupDisk)
	api.TimeMachine = PreserveNullBool(api.TimeMachine, state.TimeMachine)
	api.Trackpad = PreserveNullBool(api.Trackpad, state.Trackpad)
	api.UsersAndGroups = PreserveNullBool(api.UsersAndGroups, state.UsersAndGroups)
	api.Xsan = PreserveNullBool(api.Xsan, state.Xsan)
	api.ICloud = PreserveNullBool(api.ICloud, state.ICloud)
}

func mergeRestrictionsSharing(api, state *profilemodels.RestrictionsSharingModel) {
	api.AddtoAperture = PreserveNullBool(api.AddtoAperture, state.AddtoAperture)
	api.AddtoReadingList = PreserveNullBool(api.AddtoReadingList, state.AddtoReadingList)
	api.AddtoiPhoto = PreserveNullBool(api.AddtoiPhoto, state.AddtoiPhoto)
	api.AirDrop = PreserveNullBool(api.AirDrop, state.AirDrop)
	api.AutomaticallyEnableNewSharingServices = PreserveNullBool(api.AutomaticallyEnableNewSharingServices, state.AutomaticallyEnableNewSharingServices)
	api.Facebook = PreserveNullBool(api.Facebook, state.Facebook)
	api.Mail = PreserveNullBool(api.Mail, state.Mail)
	api.Messages = PreserveNullBool(api.Messages, state.Messages)
	api.RestrictWhichSharingServicesAreEnabled = PreserveNullBool(api.RestrictWhichSharingServicesAreEnabled, state.RestrictWhichSharingServicesAreEnabled)
	api.SinaWeibo = PreserveNullBool(api.SinaWeibo, state.SinaWeibo)
	api.Twitter = PreserveNullBool(api.Twitter, state.Twitter)
	api.VideoServices = PreserveNullBool(api.VideoServices, state.VideoServices)
}
