package profile

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure ProfileResource satisfies the plan-time config validation interface.
var _ resource.ResourceWithValidateConfig = &ProfileResource{}

// allowedAssignmentTypes lists the 5 canonical `General.AssignmentType`
// names UEM's server (DeviceProfileAssignmentType, release/26.2.0.0) parses
// case-insensitively. See internal-task.
var allowedAssignmentTypes = []string{"Auto", "Custom", "Optional", "Interactive", "Compliance"}

// allowedProfileScopes lists the 3 canonical `General.ProfileScope` names.
// "Staging" is the real second value — NOT "Test" (see internal-task bug 1).
//
// UEM source: AirWatch API/AirWatch.ServiceModel/Profiles/V1/Resources/ProfileScopeType.cs:18-39,
// AirWatch API/AirWatch.ServiceModel/Profiles/V2/Resources/GeneralPayloadV2Entity.cs:96-119 (canonical Q1).
// On macOS, General has no server-side Validate() allow-list for ProfileScope; the server accepts any
// enum name via case-insensitive Enum.Parse. This 3-name list is the provider's own mirror of the
// enum, not a server allow-list — the "" acceptance below (an empty ProfileScope, live-confirmed on
// as<internal-env> 26.2 and paul-2609 26.9) does not conflict with it.
var allowedProfileScopes = []string{"Production", "Staging", "Both"}

// winRTAssignmentTypeExclusion is the assignment_type value the WinRT
// (Windows 10 desktop) platform rejects server-side, even though every
// other supported platform accepts it. Confirmed against the C# server
// source, release/26.2.0.0 (see internal-task).
const winRTAssignmentTypeExclusion = "Custom"

// canonicalizeEnum case-insensitively matches value against allowed and
// returns the canonically-cased member and true on a match, or ("", false)
// if value doesn't match any allowed member.
func canonicalizeEnum(value string, allowed []string) (string, bool) {
	for _, a := range allowed {
		if strings.EqualFold(value, a) {
			return a, true
		}
	}
	return "", false
}

// ValidateConfig implements resource.ResourceWithValidateConfig. It runs at
// `terraform plan`/`validate` time, before any API call, so an invalid
// assignment_type or profile_scope value is rejected with a clear message
// instead of surfacing later as an opaque 400 from UEM at apply time.
//
// It deliberately does NOT decode the whole config into
// profilemodels.ProfileResourceModel via req.Config.Get, because the
// framework's reflection-based decode raises a "Value Conversion Error" when
// any pointer-to-struct field (e.g. passcode, restrictions, disk_encryption,
// gatekeeper) or list field (e.g. custom_settings_list, network_list) is
// wholly UNKNOWN, as happens routinely at plan time when the value is
// derived from a not-yet-created resource or attribute. An unknown value
// must never fail validate/plan, so ValidateConfig instead reads only the
// individual attributes it needs via req.Config.GetAttribute, using the
// framework's own attr.Value-based types (e.g. types.String, types.Object),
// which represent "unknown" natively instead of failing to decode it.
// Each check gates only on the diagnostics of its own GetAttribute call, so an
// invalid value in one attribute never hides the findings of the others.
func (r *ProfileResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var platformAttr types.String
	dPlatform := req.Config.GetAttribute(ctx, path.Root("platform"), &platformAttr)
	resp.Diagnostics.Append(dPlatform...)
	if dPlatform.HasError() {
		return
	}
	platform := platformAttr.ValueString()

	var orgGroupID types.String
	dOrgGroupID := req.Config.GetAttribute(ctx, path.Root("org_group_id"), &orgGroupID)
	resp.Diagnostics.Append(dOrgGroupID...)
	if !dOrgGroupID.HasError() && !orgGroupID.IsNull() && !orgGroupID.IsUnknown() {
		if _, err := strconv.Atoi(orgGroupID.ValueString()); err != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("org_group_id"),
				"org_group_id must be numeric",
				fmt.Sprintf(
					"UEM's General.ManagedLocationGroupID is a non-nullable int (canonical rules general-ogid Q1: "+
						"AirWatch.ServiceModel GeneralPayloadV2Entity.RootLocationGroupId, serialized as "+
						"ManagedLocationGroupID) that is sent unconditionally on every profile write, for every "+
						"platform (Q2/Q3: no platform guard in the GET/create/update mapper). A non-numeric "+
						"org_group_id cannot be converted to that wire value, so the provider rejects it here "+
						"instead of silently omitting General.ManagedLocationGroupID from the request. Got %q.",
					orgGroupID.ValueString(),
				),
			)
		}
	}

	var assignmentType types.String
	dAssignmentType := req.Config.GetAttribute(ctx, path.Root("assignment_type"), &assignmentType)
	resp.Diagnostics.Append(dAssignmentType...)

	if !dAssignmentType.HasError() && !assignmentType.IsNull() && !assignmentType.IsUnknown() {
		raw := assignmentType.ValueString()
		canon, ok := canonicalizeEnum(raw, allowedAssignmentTypes)
		switch {
		case !ok:
			resp.Diagnostics.AddAttributeError(
				path.Root("assignment_type"),
				"Invalid assignment_type",
				fmt.Sprintf(
					"assignment_type must be one of (case-insensitive): %s. Got %q.",
					strings.Join(allowedAssignmentTypes, ", "), raw,
				),
			)
		case canon == winRTAssignmentTypeExclusion && platform == sdk.PlatformWindows10:
			resp.Diagnostics.AddAttributeError(
				path.Root("assignment_type"),
				"assignment_type \"Custom\" is not supported on Windows 10",
				fmt.Sprintf(
					"UEM rejects assignment_type %q for the %q platform. Use one of: Auto, Compliance, Interactive, Optional.",
					winRTAssignmentTypeExclusion, sdk.PlatformWindows10,
				),
			)
		}
	}

	var profileScope types.String
	dProfileScope := req.Config.GetAttribute(ctx, path.Root("profile_scope"), &profileScope)
	resp.Diagnostics.Append(dProfileScope...)

	if !dProfileScope.HasError() && !profileScope.IsNull() && !profileScope.IsUnknown() {
		raw := profileScope.ValueString()
		// "" is accepted as well: UEM itself stores an empty ProfileScope.
		// Live-confirmed 2026-09-25 on as<internal-env> 26.2 and paul-2609 26.9: every
		// owned profile read in the B16 acceptance (5 of 5) returned
		// ProfileScope "", so an imported profile faithfully carries
		// profile_scope = "" and must validate.
		if _, ok := canonicalizeEnum(raw, allowedProfileScopes); raw != "" && !ok {
			resp.Diagnostics.AddAttributeError(
				path.Root("profile_scope"),
				"Invalid profile_scope",
				fmt.Sprintf(
					"profile_scope must be one of (case-insensitive): %s, or empty. Got %q.",
					strings.Join(allowedProfileScopes, ", "), raw,
				),
			)
		}
	}

	var diskEncryption types.Object
	dDiskEncryption := req.Config.GetAttribute(ctx, path.Root("disk_encryption"), &diskEncryption)
	resp.Diagnostics.Append(dDiskEncryption...)
	if !dDiskEncryption.HasError() {
		var credentialsList types.List
		dCredentialsList := req.Config.GetAttribute(ctx, path.Root("credentials_list"), &credentialsList)
		resp.Diagnostics.Append(dCredentialsList...)

		// Controller-level rules (ProfilesV2Controller.ValidateDiskEncryptionPayload):
		// fire whenever disk_encryption is present, regardless of individual
		// sub-block presence or enable's value.
		validateDiskEncryptionEnableMustBeTrue(platform, diskEncryption, resp)
		validateFileVaultRequiresFileVaultUser(platform, diskEncryption, resp)
		validateDiskEncryptionRequiresPromptToEnableFileVaultAt(platform, diskEncryption, resp)
		validateDiskEncryptionRequiresUseIntelligentHub(platform, diskEncryption, resp)

		// RecoveryType cross-field rules (parent Validate()).
		validateFileVaultRequiresRecoveryType(platform, diskEncryption, resp)
		validateFileVaultRecoveryTypeRequiresShowRecoveryKeyAndStoreKey(platform, diskEncryption, resp)
		validateFileVaultRecoveryTypeCorporateForbidsShowRecoveryKeyAndStoreKey(platform, diskEncryption, resp)
		validateFileVaultRecoveryTypeRequiresCertificate(platform, diskEncryption, resp)
		validateFileVaultRecoveryTypePersonalForbidsCertificate(platform, diskEncryption, resp)
		if !dCredentialsList.HasError() {
			validateFileVaultCertificateReferencesCredential(platform, diskEncryption, credentialsList, resp)
		}

		// FileVaultUser / prompt cross-field rules.
		validateFileVaultUserSpecificRequiresUsername(platform, diskEncryption, resp)
		validateFileVaultUserNotSpecificForbidsUsername(platform, diskEncryption, resp)
		validateFileVaultPromptRequiresNumberOfTimesUserCanBypass(platform, diskEncryption, resp)
		validateFileVaultPromptLogoutOnlyForbidsBypass(platform, diskEncryption, resp)

		// AirWatch gating clusters.
		validateDiskEncryptionUseIntelligentHubGatesHubFlags(platform, diskEncryption, resp)
		validateDiskEncryptionNotifyGatesEncryptionNotificationFields(platform, diskEncryption, resp)
		validateDiskEncryptionEnableRecoveryKeyGatesRecoveryKeyFields(platform, diskEncryption, resp)
		validateDiskEncryptionEnableRecoveryKeyRequiresStoreKey(platform, diskEncryption, resp)

		// Drift traps: configurations that pass validation today but that UEM's
		// save-time business layer (MacOsDiskEncryptionProcessor) silently
		// rewrites, producing a permanent plan-time diff. Caught here instead so
		// they never reach apply.
		validateFileVaultRecoveryTypeCorporateDriftTrapEnableRecoveryKey(platform, diskEncryption, resp)
	}

	var networkList types.List
	dNetworkList := req.Config.GetAttribute(ctx, path.Root("network_list"), &networkList)
	resp.Diagnostics.Append(dNetworkList...)
	if !dNetworkList.HasError() {
		validateNetworkListWirelessRequiresSecurityType(platform, networkList, resp)
		validateNetworkListNetworkInterfaceOneOf(platform, networkList, resp)
		validateNetworkListInnerIdentityOneOf(platform, networkList, resp)
		validateNetworkListTTLSRequiresInnerIdentity(platform, networkList, resp)
	}

	var restrictions types.Object
	dRestrictions := req.Config.GetAttribute(ctx, path.Root("restrictions"), &restrictions)
	resp.Diagnostics.Append(dRestrictions...)
	if !dRestrictions.HasError() {
		validateRestrictionsMediaReadOnlyUnsupported(platform, restrictions, resp)
		validateRestrictionsDesktopPictureRequiresLock(platform, restrictions, resp)
		validateRestrictionsApplicationsAndWidgetsRequireParentFlag(platform, restrictions, resp)
	}
}

// validateFileVaultRequiresRecoveryType implements the plan-time half of the
// parent Validate() RecoveryType range check (canonical 26.2 rules Q1/Q2):
// RecoveryType is a plain, non-nullable int defaulting to 0, and UEM rejects
// anything outside [1, 3] with a 422 (Invalid RecoveryType, must be either
// 1-RecoveryTypePersonal, 2-RecoveryTypeCorporate or
// 3-RecoveryTypePersonalAndCorporate). The builder omits a null
// recovery_type (omitempty), so the server sees its 0 default. The check
// therefore fires whenever disk_encryption is present, whatever enable's
// value, and an absent filevault2 sub-block counts as recovery_type unset
// (childObjectAttrsOrEmpty). Originally live-verified with enable = true.
//
// The recovery_type must be known and null to fire; an unknown filevault2 block
// or recovery_type is skipped, and any known non-null value is left to the
// schema-level int64validator.OneOf(1, 2, 3).
func validateFileVaultRequiresRecoveryType(platform string, diskEncryption types.Object, resp *resource.ValidateConfigResponse) {
	if platform != sdk.PlatformAppleOsX {
		return
	}
	if diskEncryption.IsNull() || diskEncryption.IsUnknown() {
		return
	}

	fvAttrs, ok := childObjectAttrsOrEmpty(diskEncryption, "filevault2")
	if !ok {
		return
	}

	recoveryType := fvAttrs["recovery_type"]
	if isAttrUnknown(recoveryType) || isAttrKnownSet(recoveryType) {
		return
	}

	resp.Diagnostics.AddAttributeError(
		path.Root("disk_encryption").AtName("filevault2").AtName("recovery_type"),
		"recovery_type is required when disk_encryption is configured",
		"UEM rejects a DiskEncryption payload with no recovery_type at apply time "+
			"(422: Invalid RecoveryType, must be either 1-RecoveryTypePersonal, 2-RecoveryTypeCorporate, "+
			"or 3-RecoveryTypePersonalAndCorporate). Set disk_encryption.filevault2.recovery_type to 1, 2, or 3.",
	)
}

// isKnownNullBool reports whether v is a types.Bool that is known (not
// unknown) and null. An unknown value, a known non-null value (true or
// false — either counts as "set"), or a value of the wrong type all report
// false, so a caller treating true as "missing, fire the diagnostic" never
// fires on something that isn't decidably absent yet.
func isKnownNullBool(v attr.Value) bool {
	b, ok := v.(types.Bool)
	return ok && !b.IsUnknown() && b.IsNull()
}

// validateFileVaultRecoveryTypeRequiresShowRecoveryKeyAndStoreKey implements
// the plan-time half of a second live finding (as<internal-env> tenant, discovered
// live-proving validateFileVaultRequiresRecoveryType above): a macOS
// (AppleOsX) uem_profile with disk_encryption.filevault2.recovery_type = 1
// (Personal) or 3 (PersonalAndCorporate) is accepted by `terraform plan` but
// rejected by UEM at apply time with a 422
// (deviceProfile.DiskEncryption.DiskEncryptionFileVault2.ShowRecoveryKey,
// DiskEncryptionAirWatch.StoreKey ... Must provide the ShowRecoveryKey and
// StoreKey when the RecoveryType is Personal or PersonalAndCorporate) unless
// filevault2.show_recovery_key AND disk_encryption.airwatch.store_key are
// both set. Recovery type 2 (Corporate) does not trigger this — live-
// confirmed only for 1 and 3, matching the API's own message.
//
// Gated the same way as validateFileVaultRequiresRecoveryType (macOS-only:
// buildAppleOsXDiskEncryptionFileVault2Entity/buildAppleOsXDiskEncryptionAirWatchEntity
// are the only callers that ever send these blocks). The recovery_type must be
// known and exactly 1 or 3 to fire; unknown or any other value (including a
// null recovery_type, which validateFileVaultRequiresRecoveryType already
// covers on its own) never fires here. Both show_recovery_key and store_key are
// each checked independently and each gets its own diagnostic at its own
// attribute path: an unknown value is skipped (not yet decidable), a known
// non-null value (true or false) counts as "set", and a known-null value —
// including the field being entirely absent because the whole airwatch
// block is null — fires. An unknown airwatch block skips the store_key
// check entirely, since whether it will end up null isn't decided yet.
func validateFileVaultRecoveryTypeRequiresShowRecoveryKeyAndStoreKey(platform string, diskEncryption types.Object, resp *resource.ValidateConfigResponse) {
	if platform != sdk.PlatformAppleOsX {
		return
	}
	if diskEncryption.IsNull() || diskEncryption.IsUnknown() {
		return
	}
	deAttrs := diskEncryption.Attributes()

	fileVault, ok := deAttrs["filevault2"].(types.Object)
	if !ok || fileVault.IsNull() || fileVault.IsUnknown() {
		return
	}
	fvAttrs := fileVault.Attributes()

	recoveryType, ok := fvAttrs["recovery_type"].(types.Int64)
	if !ok || recoveryType.IsUnknown() || recoveryType.IsNull() {
		return
	}
	rt := recoveryType.ValueInt64()
	if rt != 1 && rt != 3 {
		return
	}

	if isKnownNullBool(fvAttrs["show_recovery_key"]) {
		resp.Diagnostics.AddAttributeError(
			path.Root("disk_encryption").AtName("filevault2").AtName("show_recovery_key"),
			"show_recovery_key is required when recovery_type is Personal or PersonalAndCorporate",
			"UEM rejects a FileVault 2 payload with recovery_type = 1 or 3 and no show_recovery_key at apply "+
				"time (422: Must provide the ShowRecoveryKey and StoreKey when the RecoveryType is Personal or "+
				"PersonalAndCorporate). Set disk_encryption.filevault2.show_recovery_key.",
		)
	}

	airwatch, ok := deAttrs["airwatch"].(types.Object)
	storeKeyMissing := false
	switch {
	case !ok || airwatch.IsUnknown():
		// Can't decide yet: leave storeKeyMissing false.
	case airwatch.IsNull():
		// The whole airwatch block is absent, so store_key is definitely
		// not set.
		storeKeyMissing = true
	default:
		storeKeyMissing = isKnownNullBool(airwatch.Attributes()["store_key"])
	}
	if storeKeyMissing {
		resp.Diagnostics.AddAttributeError(
			path.Root("disk_encryption").AtName("airwatch").AtName("store_key"),
			"disk_encryption.airwatch.store_key is required when filevault2.recovery_type is Personal or PersonalAndCorporate",
			"UEM rejects a FileVault 2 payload with recovery_type = 1 or 3 and no disk_encryption.airwatch.store_key "+
				"at apply time (422: Must provide the ShowRecoveryKey and StoreKey when the RecoveryType is Personal "+
				"or PersonalAndCorporate). Set disk_encryption.airwatch.store_key.",
		)
	}
}

// validateFileVaultRequiresFileVaultUser implements the plan-time half of
// the controller's unconditional FileVaultUser check
// (ProfilesV2Controller.ValidateDiskEncryptionPayload, canonical rules Q2
// controller table): whenever disk_encryption is present on a macOS
// (AppleOsX) uem_profile, UEM requires filevault2.filevault_user regardless
// of enable's value, rejecting an apply with a 400 (Invalid input -
// FileVaultUser) otherwise. Originally live-verified narrower (only when
// enable = true); broadened here to match the report, since UEM's parent
// ctor always instantiates a non-null FileVault2 child with FileVaultUser
// defaulting to null (canonical rules Q1), so an entirely absent filevault2
// sub-block is indistinguishable server-side from one with every field
// unset -- childObjectAttrsOrEmpty models that equivalence.
//
// The filevault_user must be known and unset (null, or the whole filevault2
// sub-block absent) to fire; an unknown filevault2 block or filevault_user
// value is "not yet known" and is skipped, and any known non-null value is
// left to the schema-level int64validator.OneOf(1, 2) on filevault_user
// itself.
func validateFileVaultRequiresFileVaultUser(platform string, diskEncryption types.Object, resp *resource.ValidateConfigResponse) {
	if platform != sdk.PlatformAppleOsX {
		return
	}
	if diskEncryption.IsNull() || diskEncryption.IsUnknown() {
		return
	}

	fvAttrs, ok := childObjectAttrsOrEmpty(diskEncryption, "filevault2")
	if !ok {
		return
	}

	fileVaultUser := fvAttrs["filevault_user"]
	if isAttrUnknown(fileVaultUser) || isAttrKnownSet(fileVaultUser) {
		return
	}

	resp.Diagnostics.AddAttributeError(
		path.Root("disk_encryption").AtName("filevault2").AtName("filevault_user"),
		"filevault_user is required when disk_encryption is configured",
		"UEM rejects a DiskEncryption payload with no filevault_user at apply time (400: Invalid input - "+
			"FileVaultUser). Set disk_encryption.filevault2.filevault_user to 1 (Current or Next Login User) or 2 "+
			"(Specific User).",
	)
}

// validateDiskEncryptionRequiresPromptToEnableFileVaultAt implements the
// plan-time half of the controller's unconditional PromptToEnableFileVaultAt
// check (canonical rules Q2 controller table): whenever disk_encryption is
// present, UEM requires filevault2.prompt_to_enable_filevault_at regardless
// of enable's value, rejecting an apply with a 400 (Invalid input -
// PromptToEnableFileVaultAt) otherwise. Gated the same way as
// validateFileVaultRequiresFileVaultUser, including the absent-filevault2
// equivalence via childObjectAttrsOrEmpty.
func validateDiskEncryptionRequiresPromptToEnableFileVaultAt(platform string, diskEncryption types.Object, resp *resource.ValidateConfigResponse) {
	if platform != sdk.PlatformAppleOsX {
		return
	}
	if diskEncryption.IsNull() || diskEncryption.IsUnknown() {
		return
	}

	fvAttrs, ok := childObjectAttrsOrEmpty(diskEncryption, "filevault2")
	if !ok {
		return
	}

	prompt := fvAttrs["prompt_to_enable_filevault_at"]
	if isAttrUnknown(prompt) || isAttrKnownSet(prompt) {
		return
	}

	resp.Diagnostics.AddAttributeError(
		path.Root("disk_encryption").AtName("filevault2").AtName("prompt_to_enable_filevault_at"),
		"prompt_to_enable_filevault_at is required when disk_encryption is configured",
		"UEM rejects a DiskEncryption payload with no prompt_to_enable_filevault_at at apply time (400: Invalid "+
			"input - PromptToEnableFileVaultAt). Set disk_encryption.filevault2.prompt_to_enable_filevault_at to 1 "+
			"(Both Login and Logout), 2 (Logout Only), or 3 (Login Only).",
	)
}

// validateDiskEncryptionRequiresUseIntelligentHub implements the plan-time
// half of the controller's unconditional UseIntelligentHub check (canonical
// rules Q2 controller table): whenever disk_encryption is present, UEM
// requires disk_encryption.airwatch.use_intelligent_hub, rejecting an apply
// with a 400 (Invalid input - UseIntelligentHub) otherwise. Gated on
// disk_encryption alone (not filevault2), since the parent ctor always
// instantiates a non-null AirWatch child too (canonical rules Q1): an
// entirely absent airwatch sub-block is indistinguishable server-side from
// one with every field unset.
func validateDiskEncryptionRequiresUseIntelligentHub(platform string, diskEncryption types.Object, resp *resource.ValidateConfigResponse) {
	if platform != sdk.PlatformAppleOsX {
		return
	}
	if diskEncryption.IsNull() || diskEncryption.IsUnknown() {
		return
	}

	awAttrs, ok := childObjectAttrsOrEmpty(diskEncryption, "airwatch")
	if !ok {
		return
	}

	hub := awAttrs["use_intelligent_hub"]
	if isAttrUnknown(hub) || isAttrKnownSet(hub) {
		return
	}

	resp.Diagnostics.AddAttributeError(
		path.Root("disk_encryption").AtName("airwatch").AtName("use_intelligent_hub"),
		"use_intelligent_hub is required when disk_encryption is configured",
		"UEM rejects a DiskEncryption payload with no disk_encryption.airwatch.use_intelligent_hub at apply time "+
			"(400: Invalid input - UseIntelligentHub). Set disk_encryption.airwatch.use_intelligent_hub.",
	)
}

// validateDiskEncryptionEnableMustBeTrue implements the plan-time half of
// the controller's Enable check (canonical rules Q2 controller table): UEM
// rejects an apply with filevault2.enable explicitly false (422: Enable
// must be true when configuring DiskEncryption payload).
//
// Only an explicit, known false fires. FileVault2.Enable is a non-nullable
// bool defaulting to true in UEM's ctor (canonical rules Q1), and that ctor
// default survives whenever the filevault2 sub-block, or just its enable
// field, is omitted from the request -- so an absent filevault2 sub-block,
// or one with enable left null/unknown in config, never requires this
// check to fire (it's already effectively true server-side).
func validateDiskEncryptionEnableMustBeTrue(platform string, diskEncryption types.Object, resp *resource.ValidateConfigResponse) {
	if platform != sdk.PlatformAppleOsX {
		return
	}
	if diskEncryption.IsNull() || diskEncryption.IsUnknown() {
		return
	}

	fileVault, ok := diskEncryption.Attributes()["filevault2"].(types.Object)
	if !ok || fileVault.IsNull() || fileVault.IsUnknown() {
		return
	}

	enable, ok := fileVault.Attributes()["enable"].(types.Bool)
	if !ok || enable.IsUnknown() || enable.IsNull() || enable.ValueBool() {
		return
	}

	resp.Diagnostics.AddAttributeError(
		path.Root("disk_encryption").AtName("filevault2").AtName("enable"),
		"enable must be true when disk_encryption is configured",
		"UEM rejects a DiskEncryption payload with filevault2.enable = false at apply time (422: Enable must be "+
			"true when configuring DiskEncryption payload). Set disk_encryption.filevault2.enable = true, or "+
			"remove the disk_encryption block entirely.",
	)
}

// validateFileVaultUserSpecificRequiresUsername requires
// disk_encryption.filevault2.username whenever filevault_user is known and
// equal to 2 (Specific User). Gated the same way as
// validateFileVaultRequiresFileVaultUser: macOS-only, since
// buildAppleOsXDiskEncryptionFileVault2Entity is the only caller that ever
// sends this block.
//
// The filevault_user must be known and exactly 2 to fire; unknown or any other
// value (including a null filevault_user, which
// validateFileVaultRequiresFileVaultUser already covers on its own when
// enable is true) never fires here. A known, non-null username (empty
// string excluded) counts as "set"; a known-null or empty username fires.
func validateFileVaultUserSpecificRequiresUsername(platform string, diskEncryption types.Object, resp *resource.ValidateConfigResponse) {
	if platform != sdk.PlatformAppleOsX {
		return
	}
	if diskEncryption.IsNull() || diskEncryption.IsUnknown() {
		return
	}

	fileVault, ok := diskEncryption.Attributes()["filevault2"].(types.Object)
	if !ok || fileVault.IsNull() || fileVault.IsUnknown() {
		return
	}
	attrs := fileVault.Attributes()

	fileVaultUser, ok := attrs["filevault_user"].(types.Int64)
	if !ok || fileVaultUser.IsUnknown() || fileVaultUser.IsNull() {
		return
	}
	if fileVaultUser.ValueInt64() != 2 {
		return
	}

	// UEM rejects a null or empty Username ("Must provide the Username when
	// FileVaultUser is SpecificUser"), so an empty string counts as unset.
	username, ok := attrs["username"].(types.String)
	if !ok || username.IsUnknown() || (!username.IsNull() && username.ValueString() != "") {
		return
	}

	resp.Diagnostics.AddAttributeError(
		path.Root("disk_encryption").AtName("filevault2").AtName("username"),
		"username is required when filevault_user is 2 (Specific User)",
		"UEM requires an Open Directory username when disk_encryption.filevault2.filevault_user is 2 "+
			"(Specific User). Set disk_encryption.filevault2.username.",
	)
}

// validateFileVaultPromptRequiresNumberOfTimesUserCanBypass implements the
// plan-time half of a live finding (as<internal-env> tenant, disk-encryption-filevault2
// live-e2e proof, 2026-09-25): a macOS (AppleOsX) uem_profile with
// disk_encryption.filevault2.prompt_to_enable_filevault_at = 1 (Both Login
// and Logout) or 3 (Login Only) is accepted by `terraform plan` but rejected
// by UEM at apply time with a 422
// (deviceProfile.DiskEncryption.DiskEncryptionFileVault2.NumberOfTimesUserCanBypass
// ... Must provide the NumberOfTimesUserCanBypass when PromptToEnableFileVaultAt
// is BothLoginAndLogout or LoginOnly) unless
// filevault2.number_of_times_user_can_bypass is also set. The prompt_to_enable_filevault_at
// = 2 (Logout Only) does not trigger this.
//
// Gated the same way as the other filevault2 checks: macOS-only, since
// buildAppleOsXDiskEncryptionFileVault2Entity is the only builder that ever
// sends this block. The prompt_to_enable_filevault_at must be known and exactly 1
// or 3 to fire; unknown or any other value (including a null
// prompt_to_enable_filevault_at) never fires here. The number_of_times_user_can_bypass
// must be known and null (unset) to fire; an unknown value is "not yet
// known" and is skipped, and any known non-null value (including 0) counts
// as "set".
func validateFileVaultPromptRequiresNumberOfTimesUserCanBypass(platform string, diskEncryption types.Object, resp *resource.ValidateConfigResponse) {
	if platform != sdk.PlatformAppleOsX {
		return
	}
	if diskEncryption.IsNull() || diskEncryption.IsUnknown() {
		return
	}

	fileVault, ok := diskEncryption.Attributes()["filevault2"].(types.Object)
	if !ok || fileVault.IsNull() || fileVault.IsUnknown() {
		return
	}
	attrs := fileVault.Attributes()

	prompt, ok := attrs["prompt_to_enable_filevault_at"].(types.Int64)
	if !ok || prompt.IsUnknown() || prompt.IsNull() {
		return
	}
	p := prompt.ValueInt64()
	if p != 1 && p != 3 {
		return
	}

	bypass, ok := attrs["number_of_times_user_can_bypass"].(types.Int64)
	if !ok || bypass.IsUnknown() {
		return
	}
	if !bypass.IsNull() {
		return
	}

	resp.Diagnostics.AddAttributeError(
		path.Root("disk_encryption").AtName("filevault2").AtName("number_of_times_user_can_bypass"),
		"number_of_times_user_can_bypass is required when prompt_to_enable_filevault_at is 1 or 3",
		"UEM rejects a FileVault 2 payload with prompt_to_enable_filevault_at = 1 (Both Login and Logout) or 3 "+
			"(Login Only) and no number_of_times_user_can_bypass at apply time (422: Must provide the "+
			"NumberOfTimesUserCanBypass when PromptToEnableFileVaultAt is BothLoginAndLogout or LoginOnly). "+
			"Set disk_encryption.filevault2.number_of_times_user_can_bypass (0-10).",
	)
}

// --- Additional FileVault2/AirWatch cross-field rules (canonical rules
// 2026-09-25-canonical-262-macos-diskencryption, Q2/Q5) ------------------------------

// childObjectAttrsOrEmpty returns the attributes of the disk_encryption
// child object named name (e.g. "filevault2", "airwatch"), treating a
// known-null child the same as an empty attribute set: every lookup against
// the returned map for a field that isn't there yields a nil attr.Value,
// which isAttrUnknown/isAttrKnownSet/isAttrKnownUnset all treat as "known,
// unset" rather than "unknown". This models UEM's own behavior: the parent
// AppleOsXDiskEncryptionPayloadEntity ctor always instantiates non-null
// FileVault2 and AirWatch child objects (canonical rules Q1), so a sub-block
// the caller never configured is indistinguishable server-side from one
// with every field explicitly unset. The ok result is false only when the child is
// wholly unknown (not yet decided) or the parent doesn't expose it as a
// types.Object at all.
func childObjectAttrsOrEmpty(parent types.Object, name string) (attrs map[string]attr.Value, ok bool) {
	child, isObj := parent.Attributes()[name].(types.Object)
	if !isObj {
		return nil, false
	}
	if child.IsUnknown() {
		return nil, false
	}
	if child.IsNull() {
		return map[string]attr.Value{}, true
	}
	return child.Attributes(), true
}

// isAttrUnknown reports whether v is a not-yet-known attr.Value. A nil v
// (the field is absent from a childObjectAttrsOrEmpty map) is never
// unknown -- it's decided-absent.
func isAttrUnknown(v attr.Value) bool {
	return v != nil && v.IsUnknown()
}

// isAttrKnownSet reports whether v is a known, "set" value: a non-null
// bool/int64 (any value, including false/0), or a non-null, non-empty
// string. A nil, unknown, or null/empty-string v is not set.
func isAttrKnownSet(v attr.Value) bool {
	if v == nil || v.IsUnknown() || v.IsNull() {
		return false
	}
	if s, ok := v.(types.String); ok {
		return s.ValueString() != ""
	}
	return true
}

// isAttrKnownUnset reports whether v is known and unset: absent (nil, e.g.
// from an absent parent sub-block), explicitly null, or an empty string.
// An unknown v is never reported as unset (it isn't decided yet).
func isAttrKnownUnset(v attr.Value) bool {
	if v == nil {
		return true
	}
	if v.IsUnknown() {
		return false
	}
	if v.IsNull() {
		return true
	}
	if s, ok := v.(types.String); ok {
		return s.ValueString() == ""
	}
	return false
}

// isAttrKnownTrueBool reports whether v is a known, true types.Bool.
func isAttrKnownTrueBool(v attr.Value) bool {
	b, ok := v.(types.Bool)
	return ok && !b.IsUnknown() && !b.IsNull() && b.ValueBool()
}

// isAttrKnownFalseBool reports whether v is a known, false types.Bool.
func isAttrKnownFalseBool(v attr.Value) bool {
	b, ok := v.(types.Bool)
	return ok && !b.IsUnknown() && !b.IsNull() && !b.ValueBool()
}

// validateFileVaultRecoveryTypeCorporateForbidsShowRecoveryKeyAndStoreKey
// implements the mirror image of
// validateFileVaultRecoveryTypeRequiresShowRecoveryKeyAndStoreKey: recovery_type
// = 2 (Corporate) forbids both show_recovery_key and store_key (canonical
// rules Q2 parent Validate() table: "Cannot set ShowRecoveryKey or StoreKey
// when the RecoveryType is Corporate").
//
// The recovery_type must be known and exactly 2 to fire. Each field is checked
// independently at its own attribute path; a known-unknown field is left
// alone, and only a known, "set" value (true or false -- either counts,
// same as the live-verified require-side check) fires.
func validateFileVaultRecoveryTypeCorporateForbidsShowRecoveryKeyAndStoreKey(platform string, diskEncryption types.Object, resp *resource.ValidateConfigResponse) {
	if platform != sdk.PlatformAppleOsX {
		return
	}
	if diskEncryption.IsNull() || diskEncryption.IsUnknown() {
		return
	}

	fvAttrs, ok := childObjectAttrsOrEmpty(diskEncryption, "filevault2")
	if !ok {
		return
	}
	recoveryType, ok := fvAttrs["recovery_type"].(types.Int64)
	if !ok || recoveryType.IsUnknown() || recoveryType.IsNull() || recoveryType.ValueInt64() != 2 {
		return
	}

	if isAttrKnownSet(fvAttrs["show_recovery_key"]) {
		resp.Diagnostics.AddAttributeError(
			path.Root("disk_encryption").AtName("filevault2").AtName("show_recovery_key"),
			"show_recovery_key must not be set when recovery_type is Corporate",
			"UEM rejects a FileVault 2 payload with recovery_type = 2 (Corporate) and show_recovery_key set at "+
				"apply time (422: Cannot set ShowRecoveryKey or StoreKey when the RecoveryType is Corporate). "+
				"Remove disk_encryption.filevault2.show_recovery_key, or change recovery_type.",
		)
	}

	awAttrs, ok := childObjectAttrsOrEmpty(diskEncryption, "airwatch")
	if ok && isAttrKnownSet(awAttrs["store_key"]) {
		resp.Diagnostics.AddAttributeError(
			path.Root("disk_encryption").AtName("airwatch").AtName("store_key"),
			"store_key must not be set when filevault2.recovery_type is Corporate",
			"UEM rejects a FileVault 2 payload with recovery_type = 2 (Corporate) and disk_encryption.airwatch."+
				"store_key set at apply time (422: Cannot set ShowRecoveryKey or StoreKey when the RecoveryType is "+
				"Corporate). Remove disk_encryption.airwatch.store_key, or change recovery_type.",
		)
	}
}

// validateFileVaultRecoveryTypeRequiresCertificate implements the plan-time
// half of a live-shaped finding (canonical rules Q2 parent Validate() table):
// recovery_type = 2 (Corporate) or 3 (PersonalAndCorporate) requires
// filevault_enterprise_certificate ("Must provide the FileVaultEnterpriseCertificate
// when the RecoveryType is Corporate or PersonalAndCorporate").
//
// The recovery_type must be known and exactly 2 or 3 to fire.
// The filevault_enterprise_certificate must be known and unset (null or empty)
// to fire; an unknown certificate is skipped.
func validateFileVaultRecoveryTypeRequiresCertificate(platform string, diskEncryption types.Object, resp *resource.ValidateConfigResponse) {
	if platform != sdk.PlatformAppleOsX {
		return
	}
	if diskEncryption.IsNull() || diskEncryption.IsUnknown() {
		return
	}

	fvAttrs, ok := childObjectAttrsOrEmpty(diskEncryption, "filevault2")
	if !ok {
		return
	}
	recoveryType, ok := fvAttrs["recovery_type"].(types.Int64)
	if !ok || recoveryType.IsUnknown() || recoveryType.IsNull() {
		return
	}
	rt := recoveryType.ValueInt64()
	if rt != 2 && rt != 3 {
		return
	}

	cert := fvAttrs["filevault_enterprise_certificate"]
	if isAttrUnknown(cert) || isAttrKnownSet(cert) {
		return
	}

	resp.Diagnostics.AddAttributeError(
		path.Root("disk_encryption").AtName("filevault2").AtName("filevault_enterprise_certificate"),
		"filevault_enterprise_certificate is required when recovery_type is Corporate or PersonalAndCorporate",
		"UEM rejects a FileVault 2 payload with recovery_type = 2 or 3 and no filevault_enterprise_certificate at "+
			"apply time (422: Must provide the FileVaultEnterpriseCertificate when the RecoveryType is Corporate "+
			"or PersonalAndCorporate). Set disk_encryption.filevault2.filevault_enterprise_certificate to the "+
			"credential_name of a credentials_list entry.",
	)
}

// validateFileVaultRecoveryTypePersonalForbidsCertificate implements the
// mirror image of validateFileVaultRecoveryTypeRequiresCertificate:
// recovery_type = 1 (Personal) forbids filevault_enterprise_certificate
// (canonical rules Q2 parent Validate() table: "Cannot set
// FileVaultEnterpriseCertificate when the RecoveryType is Personal").
func validateFileVaultRecoveryTypePersonalForbidsCertificate(platform string, diskEncryption types.Object, resp *resource.ValidateConfigResponse) {
	if platform != sdk.PlatformAppleOsX {
		return
	}
	if diskEncryption.IsNull() || diskEncryption.IsUnknown() {
		return
	}

	fvAttrs, ok := childObjectAttrsOrEmpty(diskEncryption, "filevault2")
	if !ok {
		return
	}
	recoveryType, ok := fvAttrs["recovery_type"].(types.Int64)
	if !ok || recoveryType.IsUnknown() || recoveryType.IsNull() || recoveryType.ValueInt64() != 1 {
		return
	}

	cert := fvAttrs["filevault_enterprise_certificate"]
	if isAttrUnknown(cert) || !isAttrKnownSet(cert) {
		return
	}

	resp.Diagnostics.AddAttributeError(
		path.Root("disk_encryption").AtName("filevault2").AtName("filevault_enterprise_certificate"),
		"filevault_enterprise_certificate must not be set when recovery_type is Personal",
		"UEM rejects a FileVault 2 payload with recovery_type = 1 (Personal) and filevault_enterprise_certificate "+
			"set at apply time (422: Cannot set FileVaultEnterpriseCertificate when the RecoveryType is Personal). "+
			"Remove disk_encryption.filevault2.filevault_enterprise_certificate, or change recovery_type.",
	)
}

// validateFileVaultCertificateReferencesCredential implements the plan-time
// half of the controller's FileVaultEnterpriseCertificate cross-reference
// check (canonical rules Q2 controller table): a non-empty
// filevault_enterprise_certificate must match the credential_name of a
// credentials_list entry in the same profile, case-insensitively, or UEM
// rejects the apply with a 400 (Invalid input - FileVaultEnterpriseCertificate).
//
// The filevault_enterprise_certificate must be known and non-empty to fire. If
// credentials_list itself is unknown, the check is skipped entirely. A known
// credentials_list is treated as empty when null. Any single entry whose
// credential_name is unknown makes the whole "not found" conclusion
// undecidable, so the check is skipped rather than risking a false
// positive; an entry that's null/unknown as a whole, or whose
// credential_name is null, is simply not a match.
func validateFileVaultCertificateReferencesCredential(platform string, diskEncryption types.Object, credentialsList types.List, resp *resource.ValidateConfigResponse) {
	if platform != sdk.PlatformAppleOsX {
		return
	}
	if diskEncryption.IsNull() || diskEncryption.IsUnknown() {
		return
	}

	fvAttrs, ok := childObjectAttrsOrEmpty(diskEncryption, "filevault2")
	if !ok {
		return
	}
	cert, ok := fvAttrs["filevault_enterprise_certificate"].(types.String)
	if !ok || cert.IsUnknown() || cert.IsNull() || cert.ValueString() == "" {
		return
	}
	certName := cert.ValueString()

	if credentialsList.IsUnknown() {
		return
	}

	found := false
	anyUnknownName := false
	if !credentialsList.IsNull() {
		for _, el := range credentialsList.Elements() {
			item, isObj := el.(types.Object)
			if !isObj || item.IsUnknown() || item.IsNull() {
				continue
			}
			name, isStr := item.Attributes()["credential_name"].(types.String)
			if !isStr {
				continue
			}
			switch {
			case name.IsUnknown():
				anyUnknownName = true
			case !name.IsNull() && strings.EqualFold(name.ValueString(), certName):
				found = true
			}
		}
	}
	if found || anyUnknownName {
		return
	}

	resp.Diagnostics.AddAttributeError(
		path.Root("disk_encryption").AtName("filevault2").AtName("filevault_enterprise_certificate"),
		"filevault_enterprise_certificate does not match any credentials_list entry",
		fmt.Sprintf(
			"UEM rejects filevault_enterprise_certificate %q unless it matches the credential_name of a "+
				"credentials_list entry in the same profile (400: Invalid input - FileVaultEnterpriseCertificate). "+
				"Add a credentials_list entry with credential_name = %q, or reference an existing one.",
			certName, certName,
		),
	)
}

// validateFileVaultUserNotSpecificForbidsUsername implements the mirror
// image of validateFileVaultUserSpecificRequiresUsername: filevault_user !=
// 2 (SpecificUser) forbids username (canonical rules Q2 parent Validate()
// table: "Cannot set the value of Username when FileVaultUser isn't
// SpecificUser").
//
// The filevault_user must be known, non-null, and not equal to 2 to fire (that
// covers 1, the only other schema-valid value; an out-of-range value is
// left to the schema-level OneOf(1, 2) validator). The username must be known
// and set (non-empty) to fire.
func validateFileVaultUserNotSpecificForbidsUsername(platform string, diskEncryption types.Object, resp *resource.ValidateConfigResponse) {
	if platform != sdk.PlatformAppleOsX {
		return
	}
	if diskEncryption.IsNull() || diskEncryption.IsUnknown() {
		return
	}

	fvAttrs, ok := childObjectAttrsOrEmpty(diskEncryption, "filevault2")
	if !ok {
		return
	}
	fileVaultUser, ok := fvAttrs["filevault_user"].(types.Int64)
	if !ok || fileVaultUser.IsUnknown() || fileVaultUser.IsNull() || fileVaultUser.ValueInt64() == 2 {
		return
	}

	username := fvAttrs["username"]
	if isAttrUnknown(username) || !isAttrKnownSet(username) {
		return
	}

	resp.Diagnostics.AddAttributeError(
		path.Root("disk_encryption").AtName("filevault2").AtName("username"),
		"username must not be set unless filevault_user is 2 (Specific User)",
		"UEM rejects disk_encryption.filevault2.username unless filevault_user is 2 (Specific User) (422: Cannot "+
			"set the value of Username when FileVaultUser isn't SpecificUser). Remove username, or set "+
			"filevault_user = 2.",
	)
}

// validateFileVaultPromptLogoutOnlyForbidsBypass implements the mirror image
// of validateFileVaultPromptRequiresNumberOfTimesUserCanBypass:
// prompt_to_enable_filevault_at = 2 (LogoutOnly) forbids
// number_of_times_user_can_bypass (canonical rules Q2 parent Validate()
// table: "Cannot set the value of NumberOfTimesUserCanBypass when
// PromptToEnableFileVaultAt is LogoutOnly").
func validateFileVaultPromptLogoutOnlyForbidsBypass(platform string, diskEncryption types.Object, resp *resource.ValidateConfigResponse) {
	if platform != sdk.PlatformAppleOsX {
		return
	}
	if diskEncryption.IsNull() || diskEncryption.IsUnknown() {
		return
	}

	fvAttrs, ok := childObjectAttrsOrEmpty(diskEncryption, "filevault2")
	if !ok {
		return
	}
	prompt, ok := fvAttrs["prompt_to_enable_filevault_at"].(types.Int64)
	if !ok || prompt.IsUnknown() || prompt.IsNull() || prompt.ValueInt64() != 2 {
		return
	}

	bypass := fvAttrs["number_of_times_user_can_bypass"]
	if isAttrUnknown(bypass) || !isAttrKnownSet(bypass) {
		return
	}

	resp.Diagnostics.AddAttributeError(
		path.Root("disk_encryption").AtName("filevault2").AtName("number_of_times_user_can_bypass"),
		"number_of_times_user_can_bypass must not be set when prompt_to_enable_filevault_at is 2 (Logout Only)",
		"UEM rejects a FileVault 2 payload with prompt_to_enable_filevault_at = 2 (Logout Only) and "+
			"number_of_times_user_can_bypass set at apply time (422: Cannot set the value of "+
			"NumberOfTimesUserCanBypass when PromptToEnableFileVaultAt is LogoutOnly). Remove "+
			"number_of_times_user_can_bypass, or change prompt_to_enable_filevault_at.",
	)
}

// diskEncryptionHubFlags are the two disk_encryption.airwatch boolean flags
// use_intelligent_hub gates (canonical rules Q2 AirWatch table rows
// 98-100, Q5): notify_user_for_encryption and enable_recovery_key.
var diskEncryptionHubFlags = []string{"notify_user_for_encryption", "enable_recovery_key"}

// validateDiskEncryptionUseIntelligentHubGatesHubFlags implements all three
// use_intelligent_hub gating rules in one pass (canonical rules Q2 AirWatch
// table rows 98-100):
//  1. use_intelligent_hub false/null forbids both notify_user_for_encryption
//     and enable_recovery_key being set ("UseIntelligentHub must be true to
//     set the value").
//  2. use_intelligent_hub = true requires both to be set ("Must provide the
//     value of NotifyUserForEncryption and EnableRecoveryKey when
//     UseIntelligentHub is true").
//  3. use_intelligent_hub = true with both false is rejected ("Must set
//     either NotifyUserForEncryption or EnableRecoveryKey as true to enable
//     UseIntelligentHub"). This is also SCOPE's drift-trap #2: UEM's
//     save-time MacOsDiskEncryptionProcessor forces use_intelligent_hub back
//     to false for this exact shape, so it's caught here at plan time
//     before it would ever reach that save-time rewrite -- one check serves
//     both the validation-error and the drift-trap requirement, rather than
//     duplicating it.
func validateDiskEncryptionUseIntelligentHubGatesHubFlags(platform string, diskEncryption types.Object, resp *resource.ValidateConfigResponse) {
	if platform != sdk.PlatformAppleOsX {
		return
	}
	if diskEncryption.IsNull() || diskEncryption.IsUnknown() {
		return
	}

	awAttrs, ok := childObjectAttrsOrEmpty(diskEncryption, "airwatch")
	if !ok {
		return
	}

	hub := awAttrs["use_intelligent_hub"]
	if isAttrUnknown(hub) {
		return
	}

	if !isAttrKnownTrueBool(hub) {
		for _, f := range diskEncryptionHubFlags {
			v := awAttrs[f]
			if isAttrUnknown(v) {
				continue
			}
			if isAttrKnownSet(v) {
				resp.Diagnostics.AddAttributeError(
					path.Root("disk_encryption").AtName("airwatch").AtName(f),
					fmt.Sprintf("%s requires airwatch.use_intelligent_hub = true", f),
					fmt.Sprintf(
						"UEM rejects disk_encryption.airwatch.%s unless use_intelligent_hub is true (422: "+
							"UseIntelligentHub must be true to set the value). Set "+
							"disk_encryption.airwatch.use_intelligent_hub = true, or remove %s.",
						f, f,
					),
				)
			}
		}
		return
	}

	missingAny := false
	for _, f := range diskEncryptionHubFlags {
		v := awAttrs[f]
		if isAttrUnknown(v) {
			missingAny = true
			continue
		}
		if isAttrKnownUnset(v) {
			resp.Diagnostics.AddAttributeError(
				path.Root("disk_encryption").AtName("airwatch").AtName(f),
				fmt.Sprintf("%s is required when use_intelligent_hub is true", f),
				fmt.Sprintf(
					"UEM rejects a FileVault 2 payload with use_intelligent_hub = true and no %s at apply time "+
						"(422: Must provide the value of NotifyUserForEncryption and EnableRecoveryKey when "+
						"UseIntelligentHub is true). Set disk_encryption.airwatch.%s.",
					f, f,
				),
			)
			missingAny = true
		}
	}
	if missingAny {
		return
	}

	if isAttrKnownFalseBool(awAttrs["notify_user_for_encryption"]) && isAttrKnownFalseBool(awAttrs["enable_recovery_key"]) {
		resp.Diagnostics.AddAttributeError(
			path.Root("disk_encryption").AtName("airwatch").AtName("use_intelligent_hub"),
			"use_intelligent_hub requires notify_user_for_encryption or enable_recovery_key to be true",
			"UEM rejects a FileVault 2 payload with use_intelligent_hub = true and both "+
				"notify_user_for_encryption and enable_recovery_key false at apply time (422: Must set either "+
				"NotifyUserForEncryption or EnableRecoveryKey as true to enable UseIntelligentHub). This is also a "+
				"drift trap: if it ever reached save, MacOsDiskEncryptionProcessor forces use_intelligent_hub back "+
				"to false for this exact shape. Set notify_user_for_encryption or enable_recovery_key to true.",
		)
	}
}

// notifyUserForEncryptionGatedFields lists the disk_encryption.airwatch
// fields UEM only honors when notify_user_for_encryption = true (canonical
// rules Q2 AirWatch table rows 101-102): the full encryption-notification
// cluster.
var notifyUserForEncryptionGatedFields = []string{
	"encryption_notification_title",
	"encryption_notification_message",
	"encryption_max_notify_attempts",
	"encryption_notification_retry_interval_in_hours",
	"encryption_action_after_last_notification",
}

// validateDiskEncryptionNotifyGatesEncryptionNotificationFields implements
// both notify_user_for_encryption gating rules in one pass (canonical rules
// Q2 AirWatch table rows 101-102): the cluster is forbidden when
// notify_user_for_encryption is false/null ("NotifyUserForEncryption must be
// true to set the value"), and every field in it is required when
// notify_user_for_encryption is true ("Value cannot be null or empty when
// NotifyUserForEncryption is true").
func validateDiskEncryptionNotifyGatesEncryptionNotificationFields(platform string, diskEncryption types.Object, resp *resource.ValidateConfigResponse) {
	if platform != sdk.PlatformAppleOsX {
		return
	}
	if diskEncryption.IsNull() || diskEncryption.IsUnknown() {
		return
	}

	awAttrs, ok := childObjectAttrsOrEmpty(diskEncryption, "airwatch")
	if !ok {
		return
	}

	notify := awAttrs["notify_user_for_encryption"]
	if isAttrUnknown(notify) {
		return
	}

	if !isAttrKnownTrueBool(notify) {
		for _, f := range notifyUserForEncryptionGatedFields {
			v := awAttrs[f]
			if isAttrUnknown(v) {
				continue
			}
			if isAttrKnownSet(v) {
				resp.Diagnostics.AddAttributeError(
					path.Root("disk_encryption").AtName("airwatch").AtName(f),
					fmt.Sprintf("%s requires airwatch.notify_user_for_encryption = true", f),
					fmt.Sprintf(
						"UEM rejects disk_encryption.airwatch.%s unless notify_user_for_encryption is true (422: "+
							"NotifyUserForEncryption must be true to set the value). Set "+
							"disk_encryption.airwatch.notify_user_for_encryption = true, or remove %s.",
						f, f,
					),
				)
			}
		}
		return
	}

	for _, f := range notifyUserForEncryptionGatedFields {
		v := awAttrs[f]
		if isAttrUnknown(v) {
			continue
		}
		if isAttrKnownUnset(v) {
			resp.Diagnostics.AddAttributeError(
				path.Root("disk_encryption").AtName("airwatch").AtName(f),
				fmt.Sprintf("%s is required when notify_user_for_encryption is true", f),
				fmt.Sprintf(
					"UEM rejects a FileVault 2 payload with notify_user_for_encryption = true and no %s at apply "+
						"time (422: Value cannot be null or empty when NotifyUserForEncryption is true). Set "+
						"disk_encryption.airwatch.%s.",
					f, f,
				),
			)
		}
	}
}

// enableRecoveryKeyGatedFields lists the disk_encryption.airwatch fields UEM
// only honors when enable_recovery_key = true.
//
// UEM source: AirWatch API/AirWatch.ServiceModel/Profiles/V2/Resources/AppleOsX/AppleOsXDiskEncryptionPayloadEntity.cs:323-346
// (canonical Q2). Q2's single yield-return clause requires 9 string/int fields
// (the 9 below) OR RecoveryKeyMaxFailureCount to be non-null when
// enable_recovery_key is true; this slice matches the 9 string/int fields
// exactly. RecoveryKeyMaxFailureCount (modeled as
// recovery_key_max_failure_count) is not gated by this slice — that remains
// an open gap, out of scope for this pass. The separate StoreKey requirement
// from Q2's second clause is enforced immediately below by
// validateDiskEncryptionEnableRecoveryKeyRequiresStoreKey.
var enableRecoveryKeyGatedFields = []string{
	"recovery_key_notification_title",
	"recovery_key_notification_message",
	"recovery_key_notification_retry_interval_in_hours",
	"recovery_key_prompt_title",
	"recovery_key_prompt_message",
	"recovery_key_success_title",
	"recovery_key_success_message",
	"recovery_key_error_title",
	"recovery_key_error_message",
}

// validateDiskEncryptionEnableRecoveryKeyGatesRecoveryKeyFields implements
// both enable_recovery_key gating rules in one pass (UEM source:
// AirWatch API/AirWatch.ServiceModel/Profiles/V2/Resources/AppleOsX/AppleOsXDiskEncryptionPayloadEntity.cs:323-346,
// canonical Q2), mirroring
// validateDiskEncryptionNotifyGatesEncryptionNotificationFields exactly:
// the cluster is forbidden when enable_recovery_key is false/null
// ("EnableRecoveryKey must be true to set the value"), and every field in
// it is required when enable_recovery_key is true ("Value cannot be null or
// empty when EnableRecoveryKey is true").
func validateDiskEncryptionEnableRecoveryKeyGatesRecoveryKeyFields(platform string, diskEncryption types.Object, resp *resource.ValidateConfigResponse) {
	if platform != sdk.PlatformAppleOsX {
		return
	}
	if diskEncryption.IsNull() || diskEncryption.IsUnknown() {
		return
	}

	awAttrs, ok := childObjectAttrsOrEmpty(diskEncryption, "airwatch")
	if !ok {
		return
	}

	enableRecoveryKey := awAttrs["enable_recovery_key"]
	if isAttrUnknown(enableRecoveryKey) {
		return
	}

	if !isAttrKnownTrueBool(enableRecoveryKey) {
		for _, f := range enableRecoveryKeyGatedFields {
			v := awAttrs[f]
			if isAttrUnknown(v) {
				continue
			}
			if isAttrKnownSet(v) {
				resp.Diagnostics.AddAttributeError(
					path.Root("disk_encryption").AtName("airwatch").AtName(f),
					fmt.Sprintf("%s requires airwatch.enable_recovery_key = true", f),
					fmt.Sprintf(
						"UEM rejects disk_encryption.airwatch.%s unless enable_recovery_key is true (422: "+
							"EnableRecoveryKey must be true to set the value). Set "+
							"disk_encryption.airwatch.enable_recovery_key = true, or remove %s.",
						f, f,
					),
				)
			}
		}
		return
	}

	for _, f := range enableRecoveryKeyGatedFields {
		v := awAttrs[f]
		if isAttrUnknown(v) {
			continue
		}
		if isAttrKnownUnset(v) {
			resp.Diagnostics.AddAttributeError(
				path.Root("disk_encryption").AtName("airwatch").AtName(f),
				fmt.Sprintf("%s is required when enable_recovery_key is true", f),
				fmt.Sprintf(
					"UEM rejects a FileVault 2 payload with enable_recovery_key = true and no %s at apply time "+
						"(422: Value cannot be null or empty when EnableRecoveryKey is true). Set "+
						"disk_encryption.airwatch.%s.",
					f, f,
				),
			)
		}
	}
}

// validateDiskEncryptionEnableRecoveryKeyRequiresStoreKey implements the
// plan-time half of the parent Validate() cross-field rule (UEM source:
// AirWatch API/AirWatch.ServiceModel/Profiles/V2/Resources/AppleOsX/AppleOsXDiskEncryptionPayloadEntity.cs:323-346,
// canonical Q2): enable_recovery_key = true requires store_key
// = true ("StoreKey must be true to set EnableRecoveryKey as true").
func validateDiskEncryptionEnableRecoveryKeyRequiresStoreKey(platform string, diskEncryption types.Object, resp *resource.ValidateConfigResponse) {
	if platform != sdk.PlatformAppleOsX {
		return
	}
	if diskEncryption.IsNull() || diskEncryption.IsUnknown() {
		return
	}

	awAttrs, ok := childObjectAttrsOrEmpty(diskEncryption, "airwatch")
	if !ok {
		return
	}

	enableRecoveryKey := awAttrs["enable_recovery_key"]
	if !isAttrKnownTrueBool(enableRecoveryKey) {
		return
	}

	storeKey := awAttrs["store_key"]
	if isAttrUnknown(storeKey) || isAttrKnownTrueBool(storeKey) {
		return
	}

	resp.Diagnostics.AddAttributeError(
		path.Root("disk_encryption").AtName("airwatch").AtName("store_key"),
		"store_key must be true when enable_recovery_key is true",
		"UEM rejects a FileVault 2 payload with enable_recovery_key = true and store_key not also true at apply "+
			"time (422: StoreKey must be true to set EnableRecoveryKey as true). Set "+
			"disk_encryption.airwatch.store_key = true.",
	)
}

// validateFileVaultRecoveryTypeCorporateDriftTrapEnableRecoveryKey
// implements SCOPE's drift-trap #1: recovery_type = 2 (Corporate) with
// enable_recovery_key = true. This combination is already structurally
// unreachable through valid config once
// validateFileVaultRecoveryTypeCorporateForbidsShowRecoveryKeyAndStoreKey
// (forbids store_key when recovery_type = 2) and
// validateDiskEncryptionEnableRecoveryKeyRequiresStoreKey (requires
// store_key when enable_recovery_key = true) both hold -- store_key can't
// be simultaneously unset and true -- but that leaves the user with an
// indirect, confusing error about store_key instead of a direct one
// explaining the actual risk: MacOsDiskEncryptionProcessor silently forces
// disk_encryption.airwatch.enable_recovery_key back to false whenever
// recovery_type is Corporate (canonical rules Q2 business-layer table), so
// this exact shape is also a save-time drift trap in its own right if ever
// reached. Implemented as its own direct check for a clearer diagnostic.
func validateFileVaultRecoveryTypeCorporateDriftTrapEnableRecoveryKey(platform string, diskEncryption types.Object, resp *resource.ValidateConfigResponse) {
	if platform != sdk.PlatformAppleOsX {
		return
	}
	if diskEncryption.IsNull() || diskEncryption.IsUnknown() {
		return
	}

	fvAttrs, ok := childObjectAttrsOrEmpty(diskEncryption, "filevault2")
	if !ok {
		return
	}
	recoveryType, ok := fvAttrs["recovery_type"].(types.Int64)
	if !ok || recoveryType.IsUnknown() || recoveryType.IsNull() || recoveryType.ValueInt64() != 2 {
		return
	}

	awAttrs, ok := childObjectAttrsOrEmpty(diskEncryption, "airwatch")
	if !ok {
		return
	}
	if !isAttrKnownTrueBool(awAttrs["enable_recovery_key"]) {
		return
	}

	resp.Diagnostics.AddAttributeError(
		path.Root("disk_encryption").AtName("airwatch").AtName("enable_recovery_key"),
		"enable_recovery_key silently reset when recovery_type is Corporate",
		"UEM's save-time processor forces disk_encryption.airwatch.enable_recovery_key back to false whenever "+
			"filevault2.recovery_type is 2 (Corporate), so this configuration would apply cleanly but then drift "+
			"permanently out of sync with state on every subsequent plan. Set recovery_type to 1 or 3, or remove "+
			"enable_recovery_key.",
	)
}

// validateNetworkListWirelessRequiresSecurityType implements the plan-time
// half of the live finding (as<internal-env> tenant): a macOS (AppleOsX) uem_profile
// network_list entry with network_interface = "BuiltInWireless" and no
// security_type is accepted by `terraform plan` but rejected by UEM at apply
// time with a 422 (NetworkList[0].AppleOsXNetworkPayloadEntity.SecurityType:
// it cannot be null when NetworkInterface is BuiltInWireless). The builder
// (buildAppleOsXNetworkListEntity) is the only caller that ever sends this
// list, and it's macOS-only, so the check is gated the same way.
//
// Each element is checked independently: an unknown or null element is
// skipped (not yet decidable / no entry to check), a known network_interface
// value other than exactly "BuiltInWireless" never requires security_type,
// and an unknown network_interface or unknown security_type is left alone.
// Only a known network_interface == "BuiltInWireless" together with a known,
// null security_type fires, at that element's own security_type path.
func validateNetworkListWirelessRequiresSecurityType(platform string, networkList types.List, resp *resource.ValidateConfigResponse) {
	if platform != sdk.PlatformAppleOsX {
		return
	}
	if networkList.IsNull() || networkList.IsUnknown() {
		return
	}

	for i, el := range networkList.Elements() {
		item, ok := el.(types.Object)
		if !ok || item.IsNull() || item.IsUnknown() {
			continue
		}
		attrs := item.Attributes()

		iface, ok := attrs["network_interface"].(types.String)
		if !ok || iface.IsUnknown() || iface.IsNull() || iface.ValueString() != "BuiltInWireless" {
			continue
		}

		securityType, ok := attrs["security_type"].(types.String)
		if !ok || securityType.IsUnknown() || !securityType.IsNull() {
			continue
		}

		resp.Diagnostics.AddAttributeError(
			path.Root("network_list").AtListIndex(i).AtName("security_type"),
			"security_type is required when network_interface is \"BuiltInWireless\"",
			"UEM rejects a network_list entry with network_interface = \"BuiltInWireless\" and no security_type "+
				"at apply time (422: SecurityType cannot be null when NetworkInterface is BuiltInWireless). "+
				"Set security_type (e.g. \"WPA2\", \"WPA3\", or \"None\" for an open network).",
		)
	}
}

// allowedNetworkInterfaces lists the 8 `NetworkInterface` picklist names
// UEM's server (AppleOsXNetworkPayloadEntity, release/26.2.0.0) accepts for
// a macOS network_list entry — the exact wire strings, not an enum of
// friendly labels. Source: the canonical UEM 26.2 report, "Q5 —
// NetworkList / NetworkInterface / InnerIdentity", NetworkInterface table
// ("Valid values" row, citing DevicePlatformSettingOption.seed.sql:1165–1172).
// Confirmed live (as<internal-env> tenant): "Wi-Fi" is rejected with a 422 ("Invalid
// NetworkInterface Value"); "BuiltInWireless" is accepted. Matching is exact
// (case-sensitive) — these are the literal names UEM parses, not a
// case-insensitive label like assignment_type/profile_scope.
var allowedNetworkInterfaces = []string{
	"BuiltInWireless",
	"FirstActiveEthernet",
	"SecondActiveEthernet",
	"ThirdActiveEthernet",
	"FirstEthernet",
	"SecondEthernet",
	"ThirdEthernet",
	"AnyEthernet",
}

// allowedInnerIdentities lists the 4 `InnerIdentity` (TTLSInnerAuthentication)
// picklist names UEM's server accepts for a macOS network_list entry.
// Source: the canonical UEM 26.2 report, "Q5 — NetworkList / NetworkInterface
// / InnerIdentity", InnerIdentity table ("Valid values" row, picklist on
// setting TTLSInnerAuthentication, seed options on setting 101 / 3172).
// Matching is exact (case-sensitive), same rationale as
// allowedNetworkInterfaces.
var allowedInnerIdentities = []string{"PAP", "CHAP", "MSCHAP", "MSCHAPv2"}

// exactMatch reports whether value equals (case-sensitively) any member of
// allowed.
func exactMatch(value string, allowed []string) bool {
	for _, a := range allowed {
		if value == a {
			return true
		}
	}
	return false
}

// validateNetworkListNetworkInterfaceOneOf implements the plan-time check
// for network_list[].network_interface: UEM only accepts the 8 exact wire
// names in allowedNetworkInterfaces (see its doc comment for the report
// citation and the live-verified "Wi-Fi" rejection). This is a macOS-only
// ValidateConfig rule rather than a schema-level stringvalidator.OneOf
// because network_list is a single schema-level attribute shared across
// every platform (the schema does not gate it by platform — see
// validateNetworkListWirelessRequiresSecurityType's doc comment); only
// buildAppleOsXNetworkListEntity ever builds or sends it, so the check is
// gated the same way to avoid rejecting a hypothetical non-macOS use of the
// same attribute name.
//
// Each element is checked independently: an unknown or null element, or an
// unknown or null network_interface, is skipped (not yet decidable / no
// value to check). A known, non-null value that doesn't exactly match one of
// the 8 names fires at that element's own network_interface path.
func validateNetworkListNetworkInterfaceOneOf(platform string, networkList types.List, resp *resource.ValidateConfigResponse) {
	if platform != sdk.PlatformAppleOsX {
		return
	}
	if networkList.IsNull() || networkList.IsUnknown() {
		return
	}

	for i, el := range networkList.Elements() {
		item, ok := el.(types.Object)
		if !ok || item.IsNull() || item.IsUnknown() {
			continue
		}
		attrs := item.Attributes()

		iface, ok := attrs["network_interface"].(types.String)
		if !ok || iface.IsUnknown() || iface.IsNull() {
			continue
		}
		if exactMatch(iface.ValueString(), allowedNetworkInterfaces) {
			continue
		}

		resp.Diagnostics.AddAttributeError(
			path.Root("network_list").AtListIndex(i).AtName("network_interface"),
			"Invalid network_interface",
			fmt.Sprintf(
				"network_interface must be exactly one of: %s. Got %q. UEM rejects any other value "+
					"at apply time with a 422 (Invalid NetworkInterface Value) — e.g. \"Wi-Fi\" is rejected; "+
					"use \"BuiltInWireless\".",
				strings.Join(allowedNetworkInterfaces, ", "), iface.ValueString(),
			),
		)
	}
}

// validateNetworkListInnerIdentityOneOf implements the plan-time check for
// network_list[].inner_identity: UEM only accepts the 4 exact wire names in
// allowedInnerIdentities (see its doc comment for the report citation). This
// is a macOS-only ValidateConfig rule for the same reason as
// validateNetworkListNetworkInterfaceOneOf.
//
// Each element is checked independently: an unknown or null element, or an
// unknown or null inner_identity, is skipped. A known, non-null value that
// doesn't exactly match one of the 4 names fires at that element's own
// inner_identity path.
func validateNetworkListInnerIdentityOneOf(platform string, networkList types.List, resp *resource.ValidateConfigResponse) {
	if platform != sdk.PlatformAppleOsX {
		return
	}
	if networkList.IsNull() || networkList.IsUnknown() {
		return
	}

	for i, el := range networkList.Elements() {
		item, ok := el.(types.Object)
		if !ok || item.IsNull() || item.IsUnknown() {
			continue
		}
		attrs := item.Attributes()

		inner, ok := attrs["inner_identity"].(types.String)
		if !ok || inner.IsUnknown() || inner.IsNull() {
			continue
		}
		if exactMatch(inner.ValueString(), allowedInnerIdentities) {
			continue
		}

		resp.Diagnostics.AddAttributeError(
			path.Root("network_list").AtListIndex(i).AtName("inner_identity"),
			"Invalid inner_identity",
			fmt.Sprintf(
				"inner_identity must be exactly one of: %s. Got %q.",
				strings.Join(allowedInnerIdentities, ", "), inner.ValueString(),
			),
		)
	}
}

// validateNetworkListTTLSRequiresInnerIdentity implements the plan-time
// check for network_list[].inner_identity being required when TTLS is
// enabled. Source: the canonical UEM 26.2 report, "Q5 — NetworkList /
// NetworkInterface / InnerIdentity", InnerIdentity table ("Required when"
// row: "TTLS enabled", citing AppleOsXNetworkPayloadEntity.cs:628–629). In
// our schema TTLS is the network_list[].ttls bool (the EAP protocol
// booleans, per the report, are separate from network_interface/
// inner_identity). This is a macOS-only ValidateConfig rule for the same
// reason as validateNetworkListNetworkInterfaceOneOf.
//
// Each element is checked independently: an unknown or null element is
// skipped. An unknown or null/false ttls never requires inner_identity. Only
// a known ttls == true together with a known, null inner_identity fires, at
// that element's own inner_identity path; an unknown inner_identity is left
// alone (not yet decidable). This check only enforces presence — a present
// but invalid inner_identity value is separately caught by
// validateNetworkListInnerIdentityOneOf.
func validateNetworkListTTLSRequiresInnerIdentity(platform string, networkList types.List, resp *resource.ValidateConfigResponse) {
	if platform != sdk.PlatformAppleOsX {
		return
	}
	if networkList.IsNull() || networkList.IsUnknown() {
		return
	}

	for i, el := range networkList.Elements() {
		item, ok := el.(types.Object)
		if !ok || item.IsNull() || item.IsUnknown() {
			continue
		}
		attrs := item.Attributes()

		ttls, ok := attrs["ttls"].(types.Bool)
		if !ok || ttls.IsUnknown() || ttls.IsNull() || !ttls.ValueBool() {
			continue
		}

		inner, ok := attrs["inner_identity"].(types.String)
		if !ok || inner.IsUnknown() || !inner.IsNull() {
			continue
		}

		resp.Diagnostics.AddAttributeError(
			path.Root("network_list").AtListIndex(i).AtName("inner_identity"),
			"inner_identity is required when ttls is enabled",
			fmt.Sprintf(
				"UEM requires network_list[%d].inner_identity when ttls = true (TTLSInnerAuthentication is required "+
					"whenever TTLS is enabled). Set inner_identity to one of: %s.",
				i, strings.Join(allowedInnerIdentities, ", "),
			),
		)
	}
}

// restrictionsUnsupportedReadOnlyMedia lists the restrictions.media entries
// whose read_only flag the V2 restrictions API never maps (canonical
// source, 2026-09-25: DiskMediaCDs.ReadOnly and DiskMediaDVDs.ReadOnly are
// parsed by the SDK model but never sent to UEM). Every other media entry
// sharing restrictionsMediaAccessSchema (external_hard_disk_media_access,
// hard_disk_dvd_ram, hard_disk_images, internal_hard_disk_media_access,
// recordable_disc.burn_support) does support read_only, so this check is
// scoped to exactly these two field names rather than the whole media block.
var restrictionsUnsupportedReadOnlyMedia = []string{"disk_media_cds", "disk_media_dvds"}

// validateRestrictionsMediaReadOnlyUnsupported rejects restrictions.media
// entries whose read_only the API silently ignores (see
// restrictionsUnsupportedReadOnlyMedia), instead of letting them apply
// cleanly and mask as an accepted-but-inert setting. A null/unknown
// restrictions or media block, or a null/unknown/false read_only on either
// field, produces no diagnostic.
func validateRestrictionsMediaReadOnlyUnsupported(platform string, restrictions types.Object, resp *resource.ValidateConfigResponse) {
	if platform != sdk.PlatformAppleOsX {
		return
	}
	if restrictions.IsNull() || restrictions.IsUnknown() {
		return
	}

	media, ok := restrictions.Attributes()["media"].(types.Object)
	if !ok || media.IsNull() || media.IsUnknown() {
		return
	}
	mediaAttrs := media.Attributes()

	for _, field := range restrictionsUnsupportedReadOnlyMedia {
		entry, ok := mediaAttrs[field].(types.Object)
		if !ok || entry.IsNull() || entry.IsUnknown() {
			continue
		}
		readOnly, ok := entry.Attributes()["read_only"].(types.Bool)
		if !ok || readOnly.IsUnknown() || readOnly.IsNull() || !readOnly.ValueBool() {
			continue
		}
		resp.Diagnostics.AddAttributeError(
			path.Root("restrictions").AtName("media").AtName(field).AtName("read_only"),
			fmt.Sprintf("restrictions.media.%s.read_only is not supported", field),
			"UEM's V2 restrictions API never maps this field: it is accepted at apply time but has no effect, "+
				"which would otherwise mask as a silently-inert setting. Remove it, or set it to false.",
		)
	}
}

// validateRestrictionsDesktopPictureRequiresLock implements the plan-time
// half of the canonical finding (2026-09-25 source read, confirmed against
// the live preservenull-masking-audit round trip): UEM only returns
// restrictions.desktop.desktop_picture_path on Read when
// lock_desktop_picture = true. With lock_desktop_picture false or unset, the
// path is accepted at apply time but omitted on the very next Read, so
// PreserveNullString masks it -- Terraform never reports the drift. A known,
// non-null desktop_picture_path requires a known, true lock_desktop_picture;
// unknown values on either side are not yet decidable and are skipped.
func validateRestrictionsDesktopPictureRequiresLock(platform string, restrictions types.Object, resp *resource.ValidateConfigResponse) {
	if platform != sdk.PlatformAppleOsX {
		return
	}
	if restrictions.IsNull() || restrictions.IsUnknown() {
		return
	}

	desktop, ok := restrictions.Attributes()["desktop"].(types.Object)
	if !ok || desktop.IsNull() || desktop.IsUnknown() {
		return
	}
	attrs := desktop.Attributes()

	desktopPicturePath, ok := attrs["desktop_picture_path"].(types.String)
	if !ok || desktopPicturePath.IsUnknown() || desktopPicturePath.IsNull() {
		return
	}

	lock, ok := attrs["lock_desktop_picture"].(types.Bool)
	if !ok || lock.IsUnknown() {
		return
	}
	if !lock.IsNull() && lock.ValueBool() {
		return
	}

	resp.Diagnostics.AddAttributeError(
		path.Root("restrictions").AtName("desktop").AtName("lock_desktop_picture"),
		"lock_desktop_picture is required when desktop_picture_path is set",
		"UEM only returns restrictions.desktop.desktop_picture_path on Read when lock_desktop_picture = true. "+
			"With it false or unset, the path applies but is silently dropped on the next read, masking as "+
			"configuration drift instead of surfacing it. Set restrictions.desktop.lock_desktop_picture = true.",
	)
}

// validateRestrictionsApplicationsAndWidgetsRequireParentFlag implements the
// plan-time half of the canonical finding (2026-09-25 source read): UEM
// clears restrictions.applications.allow_application unless
// restrict_which_applications_are_allowed_to_launch = true, and clears
// restrictions.widgets.allowed_widgets unless
// allow_only_configured_widgets = true. Each list only needs to be known and
// non-empty to require its parent flag; a null, unknown, or empty list never
// fires, and the parent flag must be known (unknown is not yet decidable)
// and either null or false to fire.
func validateRestrictionsApplicationsAndWidgetsRequireParentFlag(platform string, restrictions types.Object, resp *resource.ValidateConfigResponse) {
	if platform != sdk.PlatformAppleOsX {
		return
	}
	if restrictions.IsNull() || restrictions.IsUnknown() {
		return
	}
	restrAttrs := restrictions.Attributes()

	if applications, ok := restrAttrs["applications"].(types.Object); ok && !applications.IsNull() && !applications.IsUnknown() {
		appAttrs := applications.Attributes()
		if listRequiresParentFlag(appAttrs["allow_application"]) {
			flag, ok := appAttrs["restrict_which_applications_are_allowed_to_launch"].(types.Bool)
			if ok && !flag.IsUnknown() && (flag.IsNull() || !flag.ValueBool()) {
				resp.Diagnostics.AddAttributeError(
					path.Root("restrictions").AtName("applications").AtName("restrict_which_applications_are_allowed_to_launch"),
					"restrict_which_applications_are_allowed_to_launch is required when allow_application is set",
					"UEM clears restrictions.applications.allow_application unless "+
						"restrict_which_applications_are_allowed_to_launch = true, so the list applies at apply time "+
						"and is silently dropped on the next read. Set restrict_which_applications_are_allowed_to_launch = true.",
				)
			}
		}
	}

	if widgets, ok := restrAttrs["widgets"].(types.Object); ok && !widgets.IsNull() && !widgets.IsUnknown() {
		widgetAttrs := widgets.Attributes()
		if listRequiresParentFlag(widgetAttrs["allowed_widgets"]) {
			flag, ok := widgetAttrs["allow_only_configured_widgets"].(types.Bool)
			if ok && !flag.IsUnknown() && (flag.IsNull() || !flag.ValueBool()) {
				resp.Diagnostics.AddAttributeError(
					path.Root("restrictions").AtName("widgets").AtName("allow_only_configured_widgets"),
					"allow_only_configured_widgets is required when allowed_widgets is set",
					"UEM clears restrictions.widgets.allowed_widgets unless allow_only_configured_widgets = true, "+
						"so the list applies at apply time and is silently dropped on the next read. Set "+
						"allow_only_configured_widgets = true.",
				)
			}
		}
	}
}

// listRequiresParentFlag reports whether v is a known, non-null, non-empty
// types.List -- the only shape that needs its parent flag checked. An
// unknown or null list, a wrong-typed value, or a known empty list all
// report false.
func listRequiresParentFlag(v attr.Value) bool {
	l, ok := v.(types.List)
	if !ok || l.IsUnknown() || l.IsNull() {
		return false
	}
	return len(l.Elements()) > 0
}
