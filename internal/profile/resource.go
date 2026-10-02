package profile

import (
	"context"
	"fmt"
	"regexp"
	"sync"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure resource types satisfy framework interfaces.
var _ resource.Resource = &ProfileResource{}
var _ resource.ResourceWithImportState = &ProfileResource{}

// diskEncryptionNotificationTextRegex is the charset UEM allows in the
// disk_encryption.airwatch notification/prompt title and message fields
// (canonical rules Q2 DataAnnotations table:
// AppleOsXDiskEncryptionAirWatchPayloadEntity's [RegularExpression(...)]
// attributes on EncryptionNotificationTitle/Message and every
// RecoveryKey*Title/Message field -- all ten fields share this exact
// pattern). Space, newline, letters, digits, and the literal set
// _#,;:'?."!@{}+-.
var diskEncryptionNotificationTextRegex = regexp.MustCompile(`^[ \na-zA-Z0-9_#,;:'?."!@{}+-]*$`)

const diskEncryptionNotificationTextRegexMessage = "must contain only spaces, newlines, letters, digits, and the characters _ # , ; : ' ? . \" ! @ { } + -"

func NewResource() resource.Resource {
	return &ProfileResource{}
}

func NewProfileResource() resource.Resource {
	return NewResource()
}

// ProfileResource defines the resource implementation.
type ProfileResource struct {
	client *sdk.Client
	// newProfileService builds a Layer 2 profile service from the configured client.
	newProfileService func(ctx context.Context, c *sdk.Client) (profileServiceAPI, error)

	// profileSvc is the lazily initialized Layer 2 service used by CRUD paths.
	profileSvc     profileServiceAPI
	profileSvcOnce sync.Once
	profileSvcErr  error
}

// profileService returns the Layer 2 ProfileService, constructing it lazily on
// first use via the configured factory. Subsequent calls return the cached
// service — or the cached error, if the first attempt failed.
func (r *ProfileResource) profileService(ctx context.Context) (profileServiceAPI, error) {
	r.profileSvcOnce.Do(func() {
		if r.client == nil {
			r.profileSvcErr = fmt.Errorf("profile resource is not configured: SDK client is nil")
			return
		}
		factory := r.newProfileService
		if factory == nil {
			factory = defaultProfileServiceFactory
		}
		r.profileSvc, r.profileSvcErr = factory(ctx, r.client)
	})
	return r.profileSvc, r.profileSvcErr
}

// nullWhenConfigNullObjectModifier plans a top-level Optional+Computed object
// attribute as an explicit typed null whenever the configuration omits it —
// whether or not prior state held a value. This is what makes removing a
// managed top-level block from HCL plan as NULL (design (b) for internal-task):
// the Update read-modify-write (internal/profile/platform/overlay.go) treats
// a nil planned section as "clear this on the live entity", so the null
// plan value is what actually clears the block on apply. Unlike the old
// useStateForNullModifier, this modifier never copies the prior state value
// into the plan — copying state would repeat the previous "removal is a
// no-op" bug. During Create (state also null), forcing null instead of
// leaving the framework's default Computed "unknown" also avoids a "cannot
// handle unknown values" error when the plan is decoded into a *Model
// pointer field.
type nullWhenConfigNullObjectModifier struct{}

func (m nullWhenConfigNullObjectModifier) Description(_ context.Context) string {
	return "Plans this attribute as null whenever it is omitted from configuration, so removing it from HCL clears it on apply instead of preserving the prior value."
}

func (m nullWhenConfigNullObjectModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m nullWhenConfigNullObjectModifier) PlanModifyObject(ctx context.Context, req planmodifier.ObjectRequest, resp *planmodifier.ObjectResponse) {
	if req.ConfigValue.IsNull() {
		resp.PlanValue = types.ObjectNull(req.PlanValue.AttributeTypes(ctx))
	}
}

func nullWhenConfigNullObject() planmodifier.Object {
	return nullWhenConfigNullObjectModifier{}
}

// useStateForNullListModifier is the List-typed equivalent of useStateForNullModifier.
type useStateForNullListModifier struct{}

func (m useStateForNullListModifier) Description(_ context.Context) string {
	return "Use the prior state value when the configuration value is null."
}

func (m useStateForNullListModifier) MarkdownDescription(_ context.Context) string {
	return "Use the prior state value when the configuration value is null."
}

func (m useStateForNullListModifier) PlanModifyList(ctx context.Context, req planmodifier.ListRequest, resp *planmodifier.ListResponse) {
	if req.ConfigValue.IsNull() && !req.StateValue.IsNull() {
		resp.PlanValue = req.StateValue
		return
	}
	// During create (state is null) and config is null, the framework marks
	// Computed lists as unknown. Convert unknown → null so that plan decoding
	// into []T struct fields does not fail with "cannot handle unknown values".
	if req.ConfigValue.IsNull() && resp.PlanValue.IsUnknown() {
		resp.PlanValue = types.ListNull(req.PlanValue.ElementType(ctx))
	}
}

func useStateForNullList() planmodifier.List {
	return useStateForNullListModifier{}
}

// nullWhenConfigNullListModifier is the List-typed equivalent of
// nullWhenConfigNullObjectModifier — see its doc comment for the removal
// semantics (design (b) for internal-task). Used only for the three top-level
// ListNested attributes (custom_settings_list, network_list,
// credentials_list); nested list attributes keep useStateForNullListModifier.
type nullWhenConfigNullListModifier struct{}

func (m nullWhenConfigNullListModifier) Description(_ context.Context) string {
	return "Plans this attribute as null whenever it is omitted from configuration, so removing it from HCL clears it on apply instead of preserving the prior value."
}

func (m nullWhenConfigNullListModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m nullWhenConfigNullListModifier) PlanModifyList(ctx context.Context, req planmodifier.ListRequest, resp *planmodifier.ListResponse) {
	if req.ConfigValue.IsNull() {
		resp.PlanValue = types.ListNull(req.PlanValue.ElementType(ctx))
	}
}

func nullWhenConfigNullList() planmodifier.List {
	return nullWhenConfigNullListModifier{}
}

// useStateForNullStringModifier is the String-typed equivalent of useStateForNullModifier.
type useStateForNullStringModifier struct{}

func (m useStateForNullStringModifier) Description(_ context.Context) string {
	return "Use the prior state value when the configuration value is null."
}

func (m useStateForNullStringModifier) MarkdownDescription(_ context.Context) string {
	return "Use the prior state value when the configuration value is null."
}

func (m useStateForNullStringModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.ConfigValue.IsNull() && !req.StateValue.IsNull() {
		resp.PlanValue = req.StateValue
	}
}

func useStateForNullString() planmodifier.String {
	return useStateForNullStringModifier{}
}

// descriptionPlanModifier implements the Create/Update
// semantics for the top-level description attribute, replacing the old
// nullWhenConfigNullStringModifier. That modifier always planned null
// whenever configuration omitted description, on both Create and Update —
// correct for a genuine removal-clears update, but wrong on Create, where
// forcing null (instead of leaving the framework's own Computed default of
// unknown) is what produced the live-confirmed 2026-09-25 apply failure on
// the 26.2 lab tenant (guarded B16 follow-up create, AppleOsX profile, no
// description configured): ".description: was null, but now
// cty.StringVal(\"\")". UEM stores "" for an omitted Description; it never
// leaves the attribute genuinely unset, so a plan of null can never survive
// the post-apply read.
//
// Behaviour when the configuration omits description (ConfigValue is null):
//   - An explicit configuration value, including "", is never touched here
//     (ConfigValue non-null returns immediately) and is planned as-is.
//   - Create (no prior state, so StateValue is also null): this modifier
//     does nothing, leaving the plan at the framework's own default for a
//     Computed attribute with no config value — unknown, already resolved by
//     MarkComputedNilsAsUnknown before any AttributePlanModifier runs. UEM's
//     "" readback then matches "unknown", not "null".
//   - Update, prior state null or "": this modifier does nothing. The
//     framework's own proposed-new-value already carries the unchanged
//     prior value forward for a Computed attribute the config didn't set,
//     so the plan is already null or "" — no diff, nothing to clear.
//   - Update, prior state non-empty (the user removed a previously-set
//     description from HCL): plan "" so it actually clears on apply. This is
//     the one case where the framework's default (silently keep the stale
//     non-empty value) must be overridden — kept from the u3a "removal
//     clears" fix, and now also consistent with the readback, since UEM
//     reports "" for both "never set" and "explicitly cleared".
type descriptionPlanModifier struct{}

func (m descriptionPlanModifier) Description(_ context.Context) string {
	return "Plans description as unknown on create, and clears it to \"\" on update when it is removed from a previously non-empty configuration, so the plan matches UEM's own empty-string default for an omitted description instead of tripping the post-apply consistency check."
}

func (m descriptionPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m descriptionPlanModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	// Configuration omits description on an update (state is known): plan
	// "" so an empty stored value stays "" with no noisy "known after
	// apply", and a previously non-empty value is cleared. On create the
	// state is null and the plan stays unknown.
	if req.ConfigValue.IsNull() && !req.StateValue.IsNull() && !req.StateValue.IsUnknown() {
		resp.PlanValue = types.StringValue("")
	}
}

func descriptionModifier() planmodifier.String {
	return descriptionPlanModifier{}
}

// useStateForNullBoolModifier is the Bool-typed equivalent of useStateForNullStringModifier.
type useStateForNullBoolModifier struct{}

func (m useStateForNullBoolModifier) Description(_ context.Context) string {
	return "Use the prior state value when the configuration value is null."
}

func (m useStateForNullBoolModifier) MarkdownDescription(_ context.Context) string {
	return "Use the prior state value when the configuration value is null."
}

func (m useStateForNullBoolModifier) PlanModifyBool(_ context.Context, req planmodifier.BoolRequest, resp *planmodifier.BoolResponse) {
	if req.ConfigValue.IsNull() && !req.StateValue.IsNull() {
		resp.PlanValue = req.StateValue
	}
}

func useStateForNullBool() planmodifier.Bool {
	return useStateForNullBoolModifier{}
}

// useStateForNullInt64Modifier is the Int64-typed equivalent of useStateForNullStringModifier.
type useStateForNullInt64Modifier struct{}

func (m useStateForNullInt64Modifier) Description(_ context.Context) string {
	return "Use the prior state value when the configuration value is null."
}

func (m useStateForNullInt64Modifier) MarkdownDescription(_ context.Context) string {
	return "Use the prior state value when the configuration value is null."
}

func (m useStateForNullInt64Modifier) PlanModifyInt64(_ context.Context, req planmodifier.Int64Request, resp *planmodifier.Int64Response) {
	if req.ConfigValue.IsNull() && !req.StateValue.IsNull() {
		resp.PlanValue = req.StateValue
	}
}

func useStateForNullInt64() planmodifier.Int64 {
	return useStateForNullInt64Modifier{}
}

// credentialSourceLegacySpelling is the older credential_source spelling
// this provider's own docs used to recommend for a CA-backed credential.
// UEM source: Database/AirWatchDB/AirWatchDB/deviceProfile/Seeds/deviceProfile.DevicePlatformSettingOption.seed.sql:1133-1142,
// AirWatch API/AirWatch.Api.Entity/PickListItem.cs:47 (canonical Q7):
// "DefinedCA" is the only value UEM's picklist Key (and its API/wire
// CredentialSource) ever actually uses; "DefinedCertificateAuthority" is a
// display/localization label, never a distinct stored/API value.
// Accepting the legacy spelling and mapping it to "DefinedCA" is a documented
// provider convenience (owner decision), so configs written against this
// provider's older docs don't plan a diff forever. It is not a UEM rule.
const credentialSourceLegacySpelling = "DefinedCertificateAuthority"

// credentialSourceCanonicalCA is the canonical CA-backed credential_source
// value: the only one UEM's API ever returns or is confirmed to accept
// (canonical Q7 — see credentialSourceLegacySpelling).
const credentialSourceCanonicalCA = "DefinedCA"

// credentialSourceCanonicalizeModifier canonicalizes a planned
// credential_source of credentialSourceLegacySpelling
// ("DefinedCertificateAuthority") to credentialSourceCanonicalCA
// ("DefinedCA"). Without this, a configuration written with the legacy
// spelling (this provider's own docs recommended it before canonical Q7 was
// answered) would perpetually diff on every plan: UEM's GET always echoes
// "DefinedCA" (canonical Q7), and credential_source is Required (not
// Computed), so a config value that never normalizes to match would show a
// permanent drift. Any other value (including the canonical spelling
// itself, and "Upload") passes through unchanged.
type credentialSourceCanonicalizeModifier struct{}

func (m credentialSourceCanonicalizeModifier) Description(_ context.Context) string {
	return `Canonicalizes credential_source "DefinedCertificateAuthority" to "DefinedCA", the only value UEM's API returns.`
}

func (m credentialSourceCanonicalizeModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m credentialSourceCanonicalizeModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.PlanValue.IsNull() || req.PlanValue.IsUnknown() {
		return
	}
	if req.PlanValue.ValueString() == credentialSourceLegacySpelling {
		resp.PlanValue = types.StringValue(credentialSourceCanonicalCA)
	}
}

func credentialSourceCanonicalize() planmodifier.String {
	return credentialSourceCanonicalizeModifier{}
}

// enableRecoveryKeyCorporateDriftModifier plans
// disk_encryption.airwatch.enable_recovery_key as false whenever the
// configuration omits it AND disk_encryption.filevault2.recovery_type is
// known and equal to 2 (Corporate). This mirrors a documented UEM save-time
// rewrite (canonical rules 2026-09-25-canonical-262-macos-diskencryption,
// Q2 business-layer table / Q5: "Corporate recovery type auto-disables
// EnableRecoveryKey on save", MacOsDiskEncryptionProcessor.ProcessOnSaveAsync
// lines 72-77): whenever recovery_type is Corporate, UEM forces
// EnableRecoveryKey to false server-side regardless of what (if anything)
// was sent, so the field's true value is never actually absent -- it's
// false. Without this modifier, an omitted enable_recovery_key would plan as
// null (config's own value, since the attribute is Optional-not-Computed by
// default) forever, while Read faithfully reports the true false value UEM
// holds, producing a permanent "false -> null" diff on every subsequent
// plan that no apply can ever resolve (recovery_type = 2 with
// enable_recovery_key = true is already rejected at plan time by
// validateFileVaultRecoveryTypeCorporateDriftTrapEnableRecoveryKey, so this
// is the one remaining unfaithful combination: recovery_type = 2 with
// enable_recovery_key omitted).
//
// This requires the attribute to be Optional+Computed (see the schema),
// which changes today's default-when-omitted behavior for every recovery_type,
// not just Corporate -- so every other branch below is written to reproduce
// the exact prior Optional-only behavior (plan as null when config is null),
// changing outcomes only for the documented recovery_type = 2 case.
type enableRecoveryKeyCorporateDriftModifier struct{}

func (m enableRecoveryKeyCorporateDriftModifier) Description(_ context.Context) string {
	return "Plans enable_recovery_key as false when omitted and filevault2.recovery_type is 2 (Corporate), matching UEM's save-time rewrite."
}

func (m enableRecoveryKeyCorporateDriftModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m enableRecoveryKeyCorporateDriftModifier) PlanModifyBool(ctx context.Context, req planmodifier.BoolRequest, resp *planmodifier.BoolResponse) {
	if !req.ConfigValue.IsNull() {
		// Explicit config value (true or false): pass it through unchanged.
		// (recovery_type = 2 with an explicit true is already rejected at
		// plan time by a ValidateConfig check; an explicit false already
		// matches UEM's forced value.)
		resp.PlanValue = req.ConfigValue
		return
	}

	var recoveryType types.Int64
	diags := req.Config.GetAttribute(ctx, path.Root("disk_encryption").AtName("filevault2").AtName("recovery_type"), &recoveryType)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	switch {
	case recoveryType.IsUnknown():
		// Not yet decidable. Fall back to the prior state value (mirrors
		// useStateForNullBoolModifier) rather than committing to a value
		// that apply might contradict.
		if !req.StateValue.IsNull() {
			resp.PlanValue = req.StateValue
		}
	case !recoveryType.IsNull() && recoveryType.ValueInt64() == 2:
		resp.PlanValue = types.BoolValue(false)
	default:
		// recovery_type is null (already an error on its own via
		// validateFileVaultRequiresRecoveryType) or 1/3: no documented
		// server-side rewrite applies, so reproduce the pre-existing
		// Optional-only behavior of planning null when config is null.
		resp.PlanValue = types.BoolNull()
	}
}

func enableRecoveryKeyCorporateDrift() planmodifier.Bool {
	return enableRecoveryKeyCorporateDriftModifier{}
}

// restrictionsBoolAttr returns a standard optional bool attribute used by the
// restrictions payload (preferences/sharing panes are all uniform bool
// toggles).
func restrictionsBoolAttr(desc string) schema.BoolAttribute {
	return schema.BoolAttribute{
		MarkdownDescription: desc,
		Optional:            true,
	}
}

// picklistIDRegex matches the numeric picklist ID UEM's V2 restrictions API
// expects for restrictions.applications.allow_application and
// restrictions.widgets.allowed_widgets entries (canonical source,
// 2026-09-25): both fields take the numeric ID of a pre-existing UEM
// picklist item, not a bundle identifier or a widget name.
var picklistIDRegex = regexp.MustCompile(`^[0-9]+$`)

// restrictionsMediaAccessSchema returns the shared nested schema used for the
// MediaAccess sub-blocks of the restrictions.media payload.
func restrictionsMediaAccessSchema(desc string) schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: desc,
		Optional:            true,
		Attributes: map[string]schema.Attribute{
			"allow": schema.BoolAttribute{
				MarkdownDescription: "If false, the media will not be mounted",
				Optional:            true,
			},
			"authenticate": schema.BoolAttribute{
				MarkdownDescription: "If true, the user will be authenticated before the media is mounted",
				Optional:            true,
			},
			"read_only": schema.BoolAttribute{
				MarkdownDescription: "If true, the media will be mounted as read-only",
				Optional:            true,
			},
		},
	}
}

func (r *ProfileResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_profile"
}

func (r *ProfileResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version: 1,
		MarkdownDescription: "Manages a Workspace ONE UEM Profile\n\n" +
			"## Update behaviour\n\n" +
			"An update reads the live profile first and changes only what this resource manages. " +
			"Payload sections and General settings the schema does not cover (for example Email and SSO payloads, or the removal password) are kept as they are in UEM.\n\n" +
			"- The configuration is the source of truth for the blocks and lists this resource manages (`description`, `passcode`, `restrictions`, `disk_encryption`, `gatekeeper`, `network_list`, `credentials_list`, `custom_settings_list`, `system_extensions`, `privacy_preferences`, `scep_list`, `web_clips_list`, `vpn_list`, `eas_microsoft_outlook`, `kernel_extension`, `custom_attributes`). Removing a block or list from the configuration deletes that payload from the profile at the next apply; removing an entry from a configured list removes that entry.\n" +
			"- A macOS profile must keep at least one payload: UEM rejects an update that would leave it with none, so the last managed block cannot be removed while the profile has no other payloads.\n" +
			"- Exceptions kept from the live profile: the Android long and short support messages; and for each `credentials_list` entry that matches exactly one live credential of the same source type (by certificate ID, else by certificate authority and template, else by name), that credential's certificate and identity preferences, plus its certificate metadata when the certificate ID is unchanged. Unmatched or ambiguous entries keep nothing from the live profile.\n" +
			"- UEM returns encrypted settings as `*****`. If a setting this resource does not manage comes back masked, the update is refused, because sending it would overwrite the secret with the mask. The check covers every live credential's kept settings, including credentials the configuration removes. A setting whose real value is literally `*****` is also refused, since it cannot be told apart from a masked secret. Update such profiles in the UEM console.\n" +
			"- If the live profile cannot be read, the update is aborted.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Profile ID",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Profile name",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Profile description. Leaving it unconfigured plans as unknown on create (UEM stores \"\" when none is sent, live-confirmed 2026-09-25 on the 26.2 lab tenant, guarded B16 follow-up create) and, on update, keeps the prior value when it was null or empty but clears to \"\" when a previously non-empty description is removed from configuration.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					descriptionModifier(),
				},
			},
			"platform": schema.StringAttribute{
				MarkdownDescription: "Platform: Android, Apple iOS, AppleOsX, Windows 10, Windows_Rugged, or Linux",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"org_group_id": schema.StringAttribute{
				MarkdownDescription: "Organization Group ID. Must be numeric: UEM's General.ManagedLocationGroupID is a non-nullable field sent on every profile write, for every platform, so a non-numeric value is rejected at plan time rather than silently omitted from the request.",
				Required:            true,
			},
			"assignment_type": schema.StringAttribute{
				MarkdownDescription: "Assignment type: Auto, Custom, Optional, Interactive, or Compliance (case-insensitive). Defaults to Auto. Note: the Windows 10 (WinRT) platform rejects \"Custom\" — use one of the other 4 values there. Note: for Android profiles with provisioning enabled, UEM silently forces this to \"Auto\" and clears smart groups regardless of what is configured here; this is existing server behavior, not a provider validation.",
				Optional:            true,
				Computed:            true,
			},
			"profile_scope": schema.StringAttribute{
				MarkdownDescription: "Profile scope: Production, Staging, or Both (case-insensitive), or empty. UEM stores an empty value on some profiles, and an imported profile carries it as-is. Leaving it unconfigured no longer sends a filled-in \"Production\" default: on create it plans as unknown and nothing is sent, so UEM's own default applies (live-confirmed 2026-09-25 on the 26.2 lab tenant, guarded B16 follow-up create: an unset profile_scope came back as \"\", not \"Production\"); on update it keeps the prior state value unchanged. An explicit value is always sent and planned as-is. If your platform requires a non-empty profile_scope on create (Android is documented to), configure one explicitly. Unlike assignment_type, the casing you configure is not preserved across a refresh: state always reflects UEM's own casing for this field.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"is_active": schema.BoolAttribute{
				MarkdownDescription: "Whether the profile is active. Defaults to true",
				Optional:            true,
				Computed:            true,
			},
			"assigned_smart_groups": schema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "List of SmartGroup IDs to assign to this profile.",
			},
			"excluded_smart_groups": schema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				MarkdownDescription: "List of SmartGroup IDs to exclude from this profile.",
			},
			"lock_screen_message": schema.StringAttribute{
				MarkdownDescription: "Lock screen message (Android only)",
				Optional:            true,
			},
			"passcode": schema.SingleNestedAttribute{
				MarkdownDescription: "Passcode policy settings (macOS and iOS)",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Object{
					nullWhenConfigNullObject(),
				},
				Attributes: map[string]schema.Attribute{
					"require_passcode_on_device": schema.BoolAttribute{
						MarkdownDescription: "Require passcode on the device",
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.Bool{
							boolplanmodifier.UseStateForUnknown(),
						},
					},
					"allow_simple_value": schema.BoolAttribute{
						MarkdownDescription: "Allow simple passcode values",
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.Bool{
							boolplanmodifier.UseStateForUnknown(),
						},
					},
					"require_alphanumeric_value": schema.BoolAttribute{
						MarkdownDescription: "Require alphanumeric passcode",
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.Bool{
							boolplanmodifier.UseStateForUnknown(),
						},
					},
					"minimum_passcode_length": schema.Int64Attribute{
						MarkdownDescription: "Minimum passcode length",
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.Int64{
							int64planmodifier.UseStateForUnknown(),
						},
					},
					"minimum_number_of_complex_characters": schema.StringAttribute{
						MarkdownDescription: "Minimum number of complex characters",
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
						},
					},
					"maximum_passcode_age": schema.StringAttribute{
						MarkdownDescription: "Maximum passcode age (in days)",
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
						},
					},
					"auto_lock": schema.StringAttribute{
						MarkdownDescription: "Auto-lock timeout (in minutes)",
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
						},
					},
					"grace_period": schema.Int64Attribute{
						MarkdownDescription: "Grace period before requiring passcode (in minutes)",
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.Int64{
							int64planmodifier.UseStateForUnknown(),
						},
					},
					"max_failed_attempts": schema.StringAttribute{
						MarkdownDescription: "Maximum number of failed passcode attempts. iOS accepts the sentinel \"None\" meaning no limit; both iOS and macOS accept numeric strings in the documented range.",
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
						},
					},
					"pin_history": schema.StringAttribute{
						MarkdownDescription: "Number of previous PINs to remember (numeric string, 1-50). Stored as a string to match the iOS canonical API type.",
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
						},
					},
					"minutes_until_failed_login_reset": schema.Int64Attribute{
						MarkdownDescription: "Minutes until failed login attempt counter resets",
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.Int64{
							int64planmodifier.UseStateForUnknown(),
						},
					},
				},
			},
			"custom_settings_list": schema.ListNestedAttribute{
				MarkdownDescription: "Custom settings payloads (macOS, iOS, and Android). Each item contains an XML/plist custom settings string.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.List{
					nullWhenConfigNullList(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"custom_settings": schema.StringAttribute{
							MarkdownDescription: "Custom settings XML/plist content",
							Required:            true,
						},
					},
				},
			},
			"network_list": schema.ListNestedAttribute{
				MarkdownDescription: "Network (Wi-Fi / Ethernet) payloads for macOS profiles. Each item configures one network connection.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.List{
					nullWhenConfigNullList(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"network_interface": schema.StringAttribute{
							MarkdownDescription: "Network interface. Must be one of: \"BuiltInWireless\", \"FirstActiveEthernet\", \"SecondActiveEthernet\", \"ThirdActiveEthernet\", \"FirstEthernet\", \"SecondEthernet\", \"ThirdEthernet\", \"AnyEthernet\" (exact match; UEM rejects any other value, e.g. \"Wi-Fi\", with a 422). When \"BuiltInWireless\", security_type must also be set.",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							},
						},
						"service_set_identifier": schema.StringAttribute{
							MarkdownDescription: "SSID of the Wi-Fi network",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							},
						},
						"hidden_network": schema.BoolAttribute{
							MarkdownDescription: "Whether the network is hidden (not broadcasting SSID)",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
								useStateForNullBool(),
							},
						},
						"auto_join": schema.BoolAttribute{
							MarkdownDescription: "Automatically join this network",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
								useStateForNullBool(),
							},
						},
						"security_type": schema.StringAttribute{
							MarkdownDescription: "Encryption type of the Wi-Fi network (e.g. WPA2, WPA3, WEP, None). Required when network_interface = \"BuiltInWireless\".",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							},
						},
						"password": schema.StringAttribute{
							MarkdownDescription: "Pre-shared key (PSK) password for the network",
							Optional:            true,
							Computed:            true,
							Sensitive:           true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							},
						},
						"use_as_login_window_configuration": schema.BoolAttribute{
							MarkdownDescription: "Allow user to authenticate to the network at login (macOS)",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
								useStateForNullBool(),
							},
						},
						"use_directory_authentication": schema.BoolAttribute{
							MarkdownDescription: "Use the machine's directory credentials for authentication (macOS)",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
								useStateForNullBool(),
							},
						},
						"tls": schema.BoolAttribute{
							MarkdownDescription: "Use TLS for EAP authentication",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
								useStateForNullBool(),
							},
						},
						"ttls": schema.BoolAttribute{
							MarkdownDescription: "Use TTLS for EAP authentication",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
								useStateForNullBool(),
							},
						},
						"leap": schema.BoolAttribute{
							MarkdownDescription: "Use LEAP for EAP authentication",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
								useStateForNullBool(),
							},
						},
						"peap": schema.BoolAttribute{
							MarkdownDescription: "Use PEAP for EAP authentication",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
								useStateForNullBool(),
							},
						},
						"eap_fast": schema.BoolAttribute{
							MarkdownDescription: "Use EAP-FAST for authentication",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
								useStateForNullBool(),
							},
						},
						"eap_sim": schema.BoolAttribute{
							MarkdownDescription: "Use EAP-SIM for authentication",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
								useStateForNullBool(),
							},
						},
						"eap_aka": schema.BoolAttribute{
							MarkdownDescription: "Use EAP-AKA for authentication",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
								useStateForNullBool(),
							},
						},
						"tls_minimum_version": schema.StringAttribute{
							MarkdownDescription: "Minimum TLS version for EAP-TLS (default 1.0)",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							},
						},
						"tls_maximum_version": schema.StringAttribute{
							MarkdownDescription: "Maximum TLS version for EAP-TLS (default 1.2)",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							},
						},
						"disable_association_mac_randomization": schema.BoolAttribute{
							MarkdownDescription: "Disable MAC address randomization when associating with this network",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
								useStateForNullBool(),
							},
						},
						"user_name": schema.StringAttribute{
							MarkdownDescription: "Username for EAP authentication",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							},
						},
						"user_password": schema.StringAttribute{
							MarkdownDescription: "Password for EAP authentication",
							Optional:            true,
							Computed:            true,
							Sensitive:           true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							},
						},
						"identity_certificate": schema.StringAttribute{
							MarkdownDescription: "Name of the credential from credentials_list to use as the EAP identity certificate. Must match the credential_name of a credentials_list entry.",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							},
						},
						"inner_identity": schema.StringAttribute{
							MarkdownDescription: "Inner authentication method used by TTLS. Must be one of: \"PAP\", \"CHAP\", \"MSCHAP\", \"MSCHAPv2\" (exact match). Required when ttls = true.",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							},
						},
						"outer_identity": schema.StringAttribute{
							MarkdownDescription: "Outer identity for TTLS, PEAP, and EAP-FAST (hides real identity)",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							},
						},
						"use_pac": schema.BoolAttribute{
							MarkdownDescription: "Use an existing PAC (Protected Access Credential) if present",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
								useStateForNullBool(),
							},
						},
						"allow_two_rands": schema.BoolAttribute{
							MarkdownDescription: "Allow two RANDs for EAP-SIM authentication",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
								useStateForNullBool(),
							},
						},
						"trusted_certificates": schema.ListAttribute{
							MarkdownDescription: "List of trusted certificate names for EAP authentication",
							Optional:            true,
							Computed:            true,
							ElementType:         types.StringType,
							PlanModifiers: []planmodifier.List{
								listplanmodifier.UseStateForUnknown(),
								useStateForNullList(),
							},
						},
						"allow_trust_exceptions": schema.BoolAttribute{
							MarkdownDescription: "Allow dynamic trust decisions by the user",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
								useStateForNullBool(),
							},
						},
						"proxy_type": schema.StringAttribute{
							MarkdownDescription: "Proxy type (e.g. None, Manual, Auto)",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							},
						},
						"proxy_server": schema.StringAttribute{
							MarkdownDescription: "Hostname of the HTTP proxy server",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							},
						},
						"proxy_server_port": schema.Int64Attribute{
							MarkdownDescription: "Port number of the HTTP proxy server",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Int64{
								int64planmodifier.UseStateForUnknown(),
								useStateForNullInt64(),
							},
						},
						"proxy_username": schema.StringAttribute{
							MarkdownDescription: "Username for proxy authentication",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							},
						},
						"proxy_password": schema.StringAttribute{
							MarkdownDescription: "Password for proxy authentication",
							Optional:            true,
							Computed:            true,
							Sensitive:           true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							},
						},
						"proxy_url": schema.StringAttribute{
							MarkdownDescription: "URL of the proxy auto-configuration (PAC) file",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							},
						},
						"pac_fallback": schema.BoolAttribute{
							MarkdownDescription: "Allow direct connection if the PAC file is unreachable",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
								useStateForNullBool(),
							},
						},
					},
				},
			},
			"credentials_list": schema.ListNestedAttribute{
				MarkdownDescription: "Credential payloads for macOS profiles. Each entry either uploads a " +
					"certificate file (credential_source = \"Upload\") or references a pre-configured " +
					"Certificate Authority (credential_source = \"DefinedCA\"). " +
					"Reference a credential in a network_list entry via identity_certificate = <credential_name>.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.List{
					nullWhenConfigNullList(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"credential_source": schema.StringAttribute{
							MarkdownDescription: `Source of the credential. Valid values: "Upload" (base64 certificate file) or "DefinedCA" (pre-configured CA in UEM) — the only value UEM's API returns for a CA-backed credential. As a provider convenience, the legacy spelling "DefinedCertificateAuthority" (from older versions of these docs) is also accepted and mapped to "DefinedCA"; UEM itself only uses "DefinedCA".`,
							Required:            true,
							PlanModifiers: []planmodifier.String{
								credentialSourceCanonicalize(),
							},
						},
						"credential_name": schema.StringAttribute{
							MarkdownDescription: "Human-readable name for this credential, referenced by identity_certificate in network payloads. " +
								"Required when credential_source is \"Upload\" so network_list entries can join to it; " +
								"optional for DefinedCA credentials, which are identified by certificate_authority + certificate_template.",
							Optional: true,
							Computed: true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							},
						},
						"certificate_payload": schema.StringAttribute{
							MarkdownDescription: "Base64-encoded certificate file (.pfx, .p12, or .cer). Required when credential_source is \"Upload\". Use filebase64(\"path/to/cert.pfx\") to load from disk.",
							Optional:            true,
							Sensitive:           true,
						},
						"certificate_password": schema.StringAttribute{
							MarkdownDescription: "Password protecting the certificate file. Required for encrypted .pfx/.p12 files.",
							Optional:            true,
							Sensitive:           true,
						},
						"certificate_id": schema.Int64Attribute{
							MarkdownDescription: "Numeric certificate ID assigned by UEM after upload. Computed automatically when credential_source is \"Upload\"; leave unset for DefinedCA.",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Int64{
								int64planmodifier.UseStateForUnknown(),
								useStateForNullInt64(),
							},
						},
						"certificate_authority": schema.Int64Attribute{
							MarkdownDescription: "Numeric ID of the Certificate Authority in UEM. Required when credential_source is \"DefinedCA\". UEM echoes this back (0 when unset), so it is read back from UEM rather than left null.",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Int64{
								int64planmodifier.UseStateForUnknown(),
							},
						},
						"certificate_template": schema.Int64Attribute{
							MarkdownDescription: "Numeric ID of the certificate template within the CA. Required when credential_source is \"DefinedCA\". UEM echoes this back (0 when unset), so it is read back from UEM rather than left null.",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Int64{
								int64planmodifier.UseStateForUnknown(),
							},
						},
						"allow_access_to_all_applications": schema.BoolAttribute{
							MarkdownDescription: "When true, all apps on the device can access the private key in the Keychain. UEM echoes this back (false when unset), so it is read back from UEM rather than left null.",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
							},
						},
						"key_is_extractable": schema.BoolAttribute{
							MarkdownDescription: "Allow export of the private key from the Keychain (macOS 10.15+). Corresponds to \"Allow export of private key from Keychain\" in the UI. UEM echoes this back (true when unset), so it is read back from UEM rather than left null.",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
							},
						},
					},
				},
			},
			"disk_encryption": schema.SingleNestedAttribute{
				MarkdownDescription: "Disk encryption (FileVault 2) settings (macOS only)",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Object{
					nullWhenConfigNullObject(),
				},
				Attributes: map[string]schema.Attribute{
					"airwatch": schema.SingleNestedAttribute{
						MarkdownDescription: "AirWatch disk encryption enforcement settings. Required whenever disk_encryption is configured.",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"store_key": schema.BoolAttribute{
								MarkdownDescription: "Store the recovery key in AirWatch. Required when filevault2.recovery_type is 1 (Personal) or 3 (Personal and Corporate); forbidden when recovery_type is 2 (Corporate). Required (true) when enable_recovery_key is true.",
								Optional:            true,
							},
							"rotate_key_after": schema.Int64Attribute{
								MarkdownDescription: "Days until the recovery key rotates. Not validated by UEM.",
								Optional:            true,
							},
							"use_intelligent_hub": schema.BoolAttribute{
								MarkdownDescription: "Use Intelligent Hub for enforcement. Required whenever disk_encryption is configured. When true, at least one of notify_user_for_encryption or enable_recovery_key must also be true (both must be set, and not both false).",
								Optional:            true,
							},
							"notify_user_for_encryption": schema.BoolAttribute{
								MarkdownDescription: "Notify user if encryption is disabled. Requires use_intelligent_hub = true. Gates the encryption_notification_*/encryption_max_notify_attempts/encryption_action_after_last_notification fields: forbidden when this is not true, all required together when this is true.",
								Optional:            true,
							},
							"encryption_notification_title": schema.StringAttribute{
								MarkdownDescription: "Title for the encryption notification (max 29 chars). Requires notify_user_for_encryption = true.",
								Optional:            true,
								Validators: []validator.String{
									stringvalidator.LengthAtMost(29),
									stringvalidator.RegexMatches(diskEncryptionNotificationTextRegex, diskEncryptionNotificationTextRegexMessage),
								},
							},
							"encryption_notification_message": schema.StringAttribute{
								MarkdownDescription: "Message for the encryption notification. Requires notify_user_for_encryption = true.",
								Optional:            true,
								Validators: []validator.String{
									stringvalidator.RegexMatches(diskEncryptionNotificationTextRegex, diskEncryptionNotificationTextRegexMessage),
								},
							},
							"encryption_max_notify_attempts": schema.Int64Attribute{
								MarkdownDescription: "Maximum times to show the encryption notification (0-100). Requires notify_user_for_encryption = true.",
								Optional:            true,
								Validators: []validator.Int64{
									int64validator.Between(0, 100),
								},
							},
							"encryption_notification_retry_interval_in_hours": schema.Int64Attribute{
								MarkdownDescription: "Retry interval in hours between notification dismissals (1-168). Requires notify_user_for_encryption = true.",
								Optional:            true,
								Validators: []validator.Int64{
									int64validator.Between(1, 168),
								},
							},
							"encryption_action_after_last_notification": schema.Int64Attribute{
								MarkdownDescription: "Action after last notification dismissal: 1 = Force Logout, 2 = Do Nothing. Requires notify_user_for_encryption = true.",
								Optional:            true,
								Validators: []validator.Int64{
									int64validator.OneOf(1, 2),
								},
							},
							"enable_recovery_key": schema.BoolAttribute{
								MarkdownDescription: "Prompt for user password to rotate recovery key. Requires use_intelligent_hub = true and store_key = true. Gates the recovery_key_notification_*/recovery_key_prompt_*/recovery_key_success_*/recovery_key_error_* fields: forbidden when this is not true, all required together when this is true. UEM silently resets this to false on save whenever filevault2.recovery_type is 2 (Corporate) -- if this is left unset, the provider plans it as false to match, so an omitted value and an explicit false behave identically here.",
								Optional:            true,
								Computed:            true,
								PlanModifiers: []planmodifier.Bool{
									enableRecoveryKeyCorporateDrift(),
								},
							},
							"recovery_key_notification_title": schema.StringAttribute{
								MarkdownDescription: "Title for rotating recovery key notification (max 29 chars). Requires enable_recovery_key = true.",
								Optional:            true,
								Validators: []validator.String{
									stringvalidator.LengthAtMost(29),
									stringvalidator.RegexMatches(diskEncryptionNotificationTextRegex, diskEncryptionNotificationTextRegexMessage),
								},
							},
							"recovery_key_notification_message": schema.StringAttribute{
								MarkdownDescription: "Message for rotating recovery key notification. Requires enable_recovery_key = true.",
								Optional:            true,
								Validators: []validator.String{
									stringvalidator.RegexMatches(diskEncryptionNotificationTextRegex, diskEncryptionNotificationTextRegexMessage),
								},
							},
							"recovery_key_notification_retry_interval_in_hours": schema.Int64Attribute{
								MarkdownDescription: "Retry interval in hours for recovery key notification (1-168). Requires enable_recovery_key = true.",
								Optional:            true,
								Validators: []validator.Int64{
									int64validator.Between(1, 168),
								},
							},
							"recovery_key_prompt_title": schema.StringAttribute{
								MarkdownDescription: "Prompt title for rotating recovery key (max 50 chars). Requires enable_recovery_key = true.",
								Optional:            true,
								Validators: []validator.String{
									stringvalidator.LengthAtMost(50),
									stringvalidator.RegexMatches(diskEncryptionNotificationTextRegex, diskEncryptionNotificationTextRegexMessage),
								},
							},
							"recovery_key_prompt_message": schema.StringAttribute{
								MarkdownDescription: "Prompt message for rotating recovery key (max 150 chars). Requires enable_recovery_key = true.",
								Optional:            true,
								Validators: []validator.String{
									stringvalidator.LengthAtMost(150),
									stringvalidator.RegexMatches(diskEncryptionNotificationTextRegex, diskEncryptionNotificationTextRegexMessage),
								},
							},
							"recovery_key_success_title": schema.StringAttribute{
								MarkdownDescription: "Success title after recovery key rotation (max 50 chars). Requires enable_recovery_key = true.",
								Optional:            true,
								Validators: []validator.String{
									stringvalidator.LengthAtMost(50),
									stringvalidator.RegexMatches(diskEncryptionNotificationTextRegex, diskEncryptionNotificationTextRegexMessage),
								},
							},
							"recovery_key_success_message": schema.StringAttribute{
								MarkdownDescription: "Success message after recovery key rotation (max 150 chars). Requires enable_recovery_key = true.",
								Optional:            true,
								Validators: []validator.String{
									stringvalidator.LengthAtMost(150),
									stringvalidator.RegexMatches(diskEncryptionNotificationTextRegex, diskEncryptionNotificationTextRegexMessage),
								},
							},
							"recovery_key_error_title": schema.StringAttribute{
								MarkdownDescription: "Error title for recovery key rotation failure (max 50 chars). Requires enable_recovery_key = true.",
								Optional:            true,
								Validators: []validator.String{
									stringvalidator.LengthAtMost(50),
									stringvalidator.RegexMatches(diskEncryptionNotificationTextRegex, diskEncryptionNotificationTextRegexMessage),
								},
							},
							"recovery_key_error_message": schema.StringAttribute{
								MarkdownDescription: "Error message for recovery key rotation failure (max 150 chars). Requires enable_recovery_key = true.",
								Optional:            true,
								Validators: []validator.String{
									stringvalidator.LengthAtMost(150),
									stringvalidator.RegexMatches(diskEncryptionNotificationTextRegex, diskEncryptionNotificationTextRegexMessage),
								},
							},
							"recovery_key_max_failure_count": schema.Int64Attribute{
								MarkdownDescription: "Maximum times to retry recovery key rotation on failure (1-5).",
								Optional:            true,
								Validators: []validator.Int64{
									int64validator.Between(1, 5),
								},
							},
						},
					},
					"filevault2": schema.SingleNestedAttribute{
						MarkdownDescription: "FileVault 2 encryption settings",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"enable": schema.BoolAttribute{
								MarkdownDescription: "Enforce disk encryption. Must be true whenever disk_encryption is configured; UEM rejects an explicit false at apply time. UEM's FileVault2 ctor defaults this to true (a non-nullable bool) whenever it is left unset, so an omitted value takes that default rather than planning null. Removing it from the configuration keeps its current value rather than clearing it, so to change it set it explicitly.",
								Optional:            true,
								Computed:            true,
								PlanModifiers: []planmodifier.Bool{
									// UseStateForUnknown alone already gives the
									// right shape on both paths: create (no
									// prior state) leaves the plan genuinely
									// unknown, so UEM's echoed default (true)
									// is accepted as a change from unknown;
									// update (prior state present) carries the
									// state value forward when config omits
									// it. useStateForNullBool used to run
									// alongside this and was a no-op on
									// create (it only acts when state is
									// non-null) but is removed here because an
									// earlier, mistaken belief that Computed
									// attributes are exempt from the
									// plan/actual consistency check when
									// config is null led to the sibling mcx
									// object modifier forcing create's
									// unknown to an explicit null -- which
									// Terraform core does NOT exempt, and
									// which broke live create (B42 follow-up,
									// live as<internal-env> UEM 26.2). See
									// useStateForNullBoolModifier's remaining
									// call sites for attributes without a
									// UEM-echoed non-null default.
									boolplanmodifier.UseStateForUnknown(),
								},
							},
							"show_recovery_key": schema.BoolAttribute{
								MarkdownDescription: "Show the personal recovery key to the user. Required when recovery_type is 1 (Personal) or 3 (Personal and Corporate); forbidden when recovery_type is 2 (Corporate).",
								Optional:            true,
							},
							"recovery_type": schema.Int64Attribute{
								MarkdownDescription: "Recovery type: 1 = Personal, 2 = Corporate, 3 = Personal and Corporate. Required whenever disk_encryption is configured. 2 or 3 require filevault_enterprise_certificate; 1 forbids it.",
								Optional:            true,
								Validators: []validator.Int64{
									int64validator.OneOf(1, 2, 3),
								},
							},
							"filevault_enterprise_certificate": schema.StringAttribute{
								MarkdownDescription: "credential_name of a credentials_list entry in this same profile, used for institutional recovery. Required when recovery_type is 2 (Corporate) or 3 (Personal and Corporate); forbidden when recovery_type is 1 (Personal).",
								Optional:            true,
							},
							"filevault_user": schema.Int64Attribute{
								MarkdownDescription: "User type added to FileVault: 1 = Current or Next Login User, 2 = Specific User. Required whenever disk_encryption is configured.",
								Optional:            true,
								Validators: []validator.Int64{
									int64validator.OneOf(1, 2),
								},
							},
							"username": schema.StringAttribute{
								MarkdownDescription: "Open Directory username for FileVault. Required when filevault_user = 2 (Specific User); forbidden otherwise.",
								Optional:            true,
							},
							"prompt_to_enable_filevault_at": schema.Int64Attribute{
								MarkdownDescription: "When to prompt: 1 = Both Login and Logout, 2 = Logout Only, 3 = Login Only. Required whenever disk_encryption is configured. 1 and 3 require number_of_times_user_can_bypass; 2 forbids it.",
								Optional:            true,
								Validators: []validator.Int64{
									int64validator.OneOf(1, 2, 3),
								},
							},
							"number_of_times_user_can_bypass": schema.Int64Attribute{
								MarkdownDescription: "Number of times user can bypass the FileVault prompt (0-10). Required when prompt_to_enable_filevault_at is 1 (Both Login and Logout) or 3 (Login Only); forbidden when prompt_to_enable_filevault_at is 2 (Logout Only).",
								Optional:            true,
								Validators: []validator.Int64{
									int64validator.Between(0, 10),
								},
							},
						},
					},
					"mcx": schema.SingleNestedAttribute{
						MarkdownDescription: "MCX disk encryption settings. UEM's DiskEncryption ctor always instantiates a non-null MCX sub-object with its own defaults, even when this whole block is left out of the configuration, so a field you leave unset takes the value UEM reports. Removing this block from the configuration keeps its current value rather than clearing it, so to change a field set it explicitly.",
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.Object{
							// UseStateForUnknown only: on create (no prior
							// state) this leaves the plan genuinely unknown,
							// which is what lets UEM's echoed non-null
							// default land in state without tripping "was
							// null, but now {...}" (Terraform core only
							// allows the applied value to differ from the
							// plan when the plan was unknown, never when it
							// was null). A prior useStateForNullObject
							// modifier forced this to an explicit null on
							// create -- added because the model field used
							// to be a *DiskEncryptionMCXModel pointer, which
							// cannot decode an unknown plan value -- and that
							// forced null is exactly what live create
							// rejected (B42 follow-up, live as<internal-env> UEM 26.2).
							// The model field is now types.Object, which
							// holds unknown fine, so the object-level
							// conversion is no longer needed and has been
							// removed entirely (no other attribute used it).
							objectplanmodifier.UseStateForUnknown(),
						},
						Attributes: map[string]schema.Attribute{
							"destroy_fv_key_on_standby": schema.BoolAttribute{
								MarkdownDescription: "Require user to unlock disk after hibernation. Left unset, UEM reports its own default (live-confirmed: false) rather than leaving the field genuinely absent.",
								Optional:            true,
								Computed:            true,
								PlanModifiers: []planmodifier.Bool{
									// See the "enable" modifier comment above:
									// UseStateForUnknown alone is correct here
									// too, and useStateForNullBool was
									// removed for the same reason.
									boolplanmodifier.UseStateForUnknown(),
								},
							},
						},
					},
				},
			},
			"gatekeeper": schema.SingleNestedAttribute{
				MarkdownDescription: "macOS Security & Privacy payload (called GateKeeper in the WS1 API). Controls auto-unlock, Touch ID, Handoff, screen capture, and software update deferral. Applies to macOS only.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Object{
					nullWhenConfigNullObject(),
				},
				Attributes: map[string]schema.Attribute{
					"allow_auto_unlock": schema.BoolAttribute{
						MarkdownDescription: "If false, disallows macOS auto unlock with Apple Watch (defaults to true).",
						Optional:            true,
					},
					"allow_fingerprint_for_unlock": schema.BoolAttribute{
						MarkdownDescription: "If false, prevents Touch ID from unlocking a device.",
						Optional:            true,
					},
					"allow_handoff": schema.BoolAttribute{
						MarkdownDescription: "If false, disables activity continuation features in macOS.",
						Optional:            true,
					},
					"allow_screen_capture": schema.BoolAttribute{
						MarkdownDescription: "If false, users can't save a screenshot of the display and are prevented from capturing a screen recording; it also prevents the Classroom app from observing remote screens.",
						Optional:            true,
					},
					"enable_app_software_update_delay": schema.BoolAttribute{
						MarkdownDescription: "Enable delay for non-OS app software updates.",
						Optional:            true,
					},
					"enable_software_update_delay": schema.BoolAttribute{
						MarkdownDescription: "Enable delay for OS software updates.",
						Optional:            true,
					},
					"enforced_software_update_delay": schema.Int64Attribute{
						MarkdownDescription: "Number of days a software update on the device is delayed (default 30 once a delay toggle is enabled).",
						Optional:            true,
					},
				},
			},
			"restrictions": schema.SingleNestedAttribute{
				MarkdownDescription: "Restrictions settings (macOS only)",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Object{
					nullWhenConfigNullObject(),
				},
				Attributes: map[string]schema.Attribute{
					"applications": schema.SingleNestedAttribute{
						MarkdownDescription: "Application restrictions",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"allow_application": schema.ListAttribute{
								MarkdownDescription: "List of applications to be allowed, as UEM picklist IDs (numeric strings) -- NOT bundle identifiers. Requires restrict_which_applications_are_allowed_to_launch = true, otherwise UEM clears it.",
								Optional:            true,
								ElementType:         types.StringType,
								Validators: []validator.List{
									listvalidator.ValueStringsAre(
										stringvalidator.RegexMatches(picklistIDRegex, "must be a numeric UEM picklist ID, not a bundle identifier"),
									),
								},
							},
							"allow_folders": schema.ListAttribute{
								MarkdownDescription: "List of folders to be allowed",
								Optional:            true,
								ElementType:         types.StringType,
							},
							"disallow_folders": schema.ListAttribute{
								MarkdownDescription: "List of folders to be disallowed",
								Optional:            true,
								ElementType:         types.StringType,
							},
							"restrict_which_applications_are_allowed_to_launch": schema.BoolAttribute{
								MarkdownDescription: "Whether to enable the Family Controls",
								Optional:            true,
							},
							"app_store": schema.SingleNestedAttribute{
								MarkdownDescription: "App Store restrictions",
								Optional:            true,
								Attributes: map[string]schema.Attribute{
									"allow_app_store_app_adoption": schema.BoolAttribute{
										MarkdownDescription: "Allow the App Store App Adoption",
										Optional:            true,
									},
									"require_admin_password_to_install_or_update_app": schema.BoolAttribute{
										MarkdownDescription: "Require an admin's password to install or update apps",
										Optional:            true,
									},
									"restrict_app_store_to_software_updates_only": schema.BoolAttribute{
										MarkdownDescription: "Restrict App Store to software updates only",
										Optional:            true,
									},
								},
							},
							"apple_music": schema.SingleNestedAttribute{
								MarkdownDescription: "Apple Music restrictions",
								Optional:            true,
								Attributes: map[string]schema.Attribute{
									"allow_music_service": schema.BoolAttribute{
										MarkdownDescription: "If false, Music service is disabled and Music app reverts to classic mode on macOS 10.12+",
										Optional:            true,
									},
								},
							},
							"camera": schema.SingleNestedAttribute{
								MarkdownDescription: "Camera restrictions",
								Optional:            true,
								Attributes: map[string]schema.Attribute{
									"allow_use_of_built_in_camera": schema.BoolAttribute{
										MarkdownDescription: "If false, disables the built-in camera",
										Optional:            true,
									},
								},
							},
							"game_centre": schema.SingleNestedAttribute{
								MarkdownDescription: "Game Center restrictions",
								Optional:            true,
								Attributes: map[string]schema.Attribute{
									"allow_adding_game_center_friends": schema.BoolAttribute{
										MarkdownDescription: "If false, disables adding Game Center friends",
										Optional:            true,
									},
									"allow_game_center_modification": schema.BoolAttribute{
										MarkdownDescription: "If false, disables account modifications",
										Optional:            true,
									},
									"allow_multiplayer_gaming": schema.BoolAttribute{
										MarkdownDescription: "If false, disables multiplayer gaming",
										Optional:            true,
									},
									"allow_use_of_game_center": schema.BoolAttribute{
										MarkdownDescription: "If false, disables the Game Center",
										Optional:            true,
									},
								},
							},
							"safari": schema.SingleNestedAttribute{
								MarkdownDescription: "Safari restrictions",
								Optional:            true,
								Attributes: map[string]schema.Attribute{
									"allow_deprecated_web_kit_tls": schema.BoolAttribute{
										MarkdownDescription: "Allow deprecated TLS 1.0/1.1 behavior in Safari (macOS 10.15.4+)",
										Optional:            true,
									},
									"allow_safari_auto_fill": schema.BoolAttribute{
										MarkdownDescription: "If false, Safari auto-fill is disabled",
										Optional:            true,
									},
								},
							},
						},
					},
					"desktop": schema.SingleNestedAttribute{
						MarkdownDescription: "Desktop restrictions",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"desktop_picture_path": schema.StringAttribute{
								MarkdownDescription: "Path for the desktop picture. Blank path locks the current desktop picture",
								Optional:            true,
							},
							"lock_desktop_picture": schema.BoolAttribute{
								MarkdownDescription: "If true, prevents changing the desktop picture",
								Optional:            true,
							},
						},
					},
					"functionality": schema.SingleNestedAttribute{
						MarkdownDescription: "Functionality restrictions",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"air_print": schema.SingleNestedAttribute{
								MarkdownDescription: "AirPrint restrictions",
								Optional:            true,
								Attributes: map[string]schema.Attribute{
									"allow_air_print": schema.BoolAttribute{
										MarkdownDescription: "If false, disallows AirPrint on macOS 10.13+",
										Optional:            true,
									},
									"allow_air_print_ibeacon_discovery": schema.BoolAttribute{
										MarkdownDescription: "If false, disables iBeacon discovery of AirPrint printers on macOS 10.13+",
										Optional:            true,
									},
									"force_air_print_trusted_tls_requirement": schema.BoolAttribute{
										MarkdownDescription: "If true, requires trusted certificates for TLS printing communication on macOS 10.13+",
										Optional:            true,
									},
								},
							},
							"content_caching": schema.SingleNestedAttribute{
								MarkdownDescription: "Content Caching restrictions",
								Optional:            true,
								Attributes: map[string]schema.Attribute{
									"allow_content_caching": schema.BoolAttribute{
										MarkdownDescription: "If false, disallows Content Caching on macOS 10.13+",
										Optional:            true,
									},
								},
							},
							"icloud": schema.SingleNestedAttribute{
								MarkdownDescription: "iCloud restrictions",
								Optional:            true,
								Attributes: map[string]schema.Attribute{
									"allow_air_print": schema.BoolAttribute{
										MarkdownDescription: "If false, disallows AirPrint on macOS 10.13+",
										Optional:            true,
									},
									"allow_air_print_ibeacon_discovery": schema.BoolAttribute{
										MarkdownDescription: "If false, disables iBeacon discovery of AirPrint printers on macOS 10.13+",
										Optional:            true,
									},
									"allow_cloud_desktop_and_documents": schema.BoolAttribute{
										MarkdownDescription: "Allow cloud desktop and documents",
										Optional:            true,
									},
									"allow_deprecated_web_kit_tls": schema.BoolAttribute{
										MarkdownDescription: "Allow deprecated TLS 1.0/1.1 behavior in Safari (macOS 10.15.4+)",
										Optional:            true,
									},
									"allow_icloud_fmm": schema.BoolAttribute{
										MarkdownDescription: "If false, disallows macOS Find My Mac iCloud service",
										Optional:            true,
									},
									"allow_icloud_address_book": schema.BoolAttribute{
										MarkdownDescription: "If false, disallows macOS iCloud Address Book services",
										Optional:            true,
									},
									"allow_icloud_btmm": schema.BoolAttribute{
										MarkdownDescription: "If false, disallows macOS Back to My Mac iCloud service",
										Optional:            true,
									},
									"allow_icloud_bookmarks": schema.BoolAttribute{
										MarkdownDescription: "If false, disallows macOS iCloud Bookmark sync",
										Optional:            true,
									},
									"allow_icloud_calendar": schema.BoolAttribute{
										MarkdownDescription: "If false, disallows macOS iCloud Calendar services",
										Optional:            true,
									},
									"allow_icloud_documents_and_data": schema.BoolAttribute{
										MarkdownDescription: "If false, disables document and key-value syncing to iCloud",
										Optional:            true,
									},
									"allow_icloud_keychain_sync": schema.BoolAttribute{
										MarkdownDescription: "If false, disables iCloud keychain synchronization",
										Optional:            true,
									},
									"allow_icloud_mail": schema.BoolAttribute{
										MarkdownDescription: "If false, disallows macOS Mail iCloud services",
										Optional:            true,
									},
									"allow_icloud_notes": schema.BoolAttribute{
										MarkdownDescription: "If false, disallows macOS iCloud Notes services",
										Optional:            true,
									},
									"allow_icloud_reminders": schema.BoolAttribute{
										MarkdownDescription: "If false, disallows iCloud Reminder services",
										Optional:            true,
									},
									"allow_password_auto_fill": schema.BoolAttribute{
										MarkdownDescription: "Allow auto filling of passwords",
										Optional:            true,
									},
									"allow_password_proximity_requests": schema.BoolAttribute{
										MarkdownDescription: "Allow requesting passwords from nearby devices",
										Optional:            true,
									},
									"allow_password_sharing": schema.BoolAttribute{
										MarkdownDescription: "Allow sharing of Wi-Fi passwords",
										Optional:            true,
									},
									"allow_use_icloud_password_for_local_accounts": schema.BoolAttribute{
										MarkdownDescription: "If false, disables use of iCloud password for local accounts",
										Optional:            true,
									},
									"force_air_print_trusted_tls_requirement": schema.BoolAttribute{
										MarkdownDescription: "If true, requires trusted certificates for TLS printing communication",
										Optional:            true,
									},
								},
							},
							"passwords": schema.SingleNestedAttribute{
								MarkdownDescription: "Password restrictions",
								Optional:            true,
								Attributes: map[string]schema.Attribute{
									"allow_password_auto_fill": schema.BoolAttribute{
										MarkdownDescription: "Allow auto filling of passwords",
										Optional:            true,
									},
									"allow_password_proximity_requests": schema.BoolAttribute{
										MarkdownDescription: "Allow requesting passwords from nearby devices",
										Optional:            true,
									},
									"allow_password_sharing": schema.BoolAttribute{
										MarkdownDescription: "Allow sharing of Wi-Fi passwords",
										Optional:            true,
									},
								},
							},
							"spotlight": schema.SingleNestedAttribute{
								MarkdownDescription: "Spotlight restrictions",
								Optional:            true,
								Attributes: map[string]schema.Attribute{
									"allow_spotlight_suggestions": schema.BoolAttribute{
										MarkdownDescription: "If false, Spotlight will not return Internet search results",
										Optional:            true,
									},
								},
							},
						},
					},
					"media": schema.SingleNestedAttribute{
						MarkdownDescription: "Media restrictions",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"auto_eject_media": schema.BoolAttribute{
								MarkdownDescription: "If true, media will be ejected on logout",
								Optional:            true,
							},
							"disk_media_cds":                  restrictionsMediaAccessSchema("Disk Media CDs access"),
							"disk_media_dvds":                 restrictionsMediaAccessSchema("Disk Media DVDs access"),
							"external_hard_disk_media_access": restrictionsMediaAccessSchema("External hard disk media access"),
							"hard_disk_dvd_ram":               restrictionsMediaAccessSchema("Hard disk DVD-RAM access"),
							"hard_disk_images":                restrictionsMediaAccessSchema("Hard disk image access"),
							"internal_hard_disk_media_access": restrictionsMediaAccessSchema("Internal hard disk media access"),
							"network_access": schema.SingleNestedAttribute{
								MarkdownDescription: "Network access restrictions (AirDrop)",
								Optional:            true,
								Attributes: map[string]schema.Attribute{
									"air_drop": schema.BoolAttribute{
										MarkdownDescription: "If true, AirDrop is disabled",
										Optional:            true,
									},
								},
							},
							"recordable_disc": schema.SingleNestedAttribute{
								MarkdownDescription: "Recordable disc burn support",
								Optional:            true,
								Attributes: map[string]schema.Attribute{
									"burn_support": restrictionsMediaAccessSchema("Burn support media access"),
								},
							},
						},
					},
					"preferences": schema.SingleNestedAttribute{
						MarkdownDescription: "System Preferences pane restrictions",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"accessibility":            restrictionsBoolAttr("Enable Accessibility preference pane"),
							"app_store":                restrictionsBoolAttr("Enable App Store preference pane"),
							"bluetooth":                restrictionsBoolAttr("Enable Bluetooth preference pane"),
							"cds_and_dvds":             restrictionsBoolAttr("Enable CDs and DVDs preference pane"),
							"date_and_time":            restrictionsBoolAttr("Enable Date and Time preference pane"),
							"desktop_and_screen_saver": restrictionsBoolAttr("Enable Desktop and Screen Saver preference pane"),
							"dictation_and_speech":     restrictionsBoolAttr("Enable Dictation and Speech preference pane"),
							"displays":                 restrictionsBoolAttr("Enable Displays preference pane"),
							"dock":                     restrictionsBoolAttr("Enable Dock preference pane"),
							"enabled_preference_panes": restrictionsBoolAttr("Enable System Preference Panes"),
							"energy_saver":             restrictionsBoolAttr("Enable Energy Saver preference pane"),
							"extensions":               restrictionsBoolAttr("Enable Extensions preference pane"),
							"fibre_channel":            restrictionsBoolAttr("Enable Fibre Channel preference pane"),
							"flash_player":             restrictionsBoolAttr("Enable Flash Player preference pane"),
							"general":                  restrictionsBoolAttr("Enable General preference pane"),
							"ink":                      restrictionsBoolAttr("Enable Ink preference pane"),
							"internet_accounts":        restrictionsBoolAttr("Enable Internet Accounts preference pane"),
							"keyboard":                 restrictionsBoolAttr("Enable Keyboard preference pane"),
							"language_and_text":        restrictionsBoolAttr("Enable Language and Text preference pane"),
							"mission_control":          restrictionsBoolAttr("Enable Mission Control preference pane"),
							"mobile_me":                restrictionsBoolAttr("Enable MobileMe preference pane"),
							"mouse":                    restrictionsBoolAttr("Enable Mouse preference pane"),
							"network":                  restrictionsBoolAttr("Enable Network preference pane"),
							"notifications":            restrictionsBoolAttr("Enable Notifications preference pane"),
							"parental_controls":        restrictionsBoolAttr("Enable Parental Controls preference pane"),
							"preference_behavior": schema.StringAttribute{
								MarkdownDescription: "Behavior of selected items (e.g. allow/disallow)",
								Optional:            true,
							},
							"print_and_scan":       restrictionsBoolAttr("Enable Print and Scan preference pane"),
							"profiles":             restrictionsBoolAttr("Enable Profiles preference pane"),
							"security_and_privacy": restrictionsBoolAttr("Enable Security and Privacy preference pane"),
							"sharing":              restrictionsBoolAttr("Enable Sharing preference pane"),
							"software_update":      restrictionsBoolAttr("Enable Software Update preference pane"),
							"sound":                restrictionsBoolAttr("Enable Sound preference pane"),
							"spotlight":            restrictionsBoolAttr("Enable Spotlight preference pane"),
							"startup_disk":         restrictionsBoolAttr("Enable Startup Disk preference pane"),
							"time_machine":         restrictionsBoolAttr("Enable Time Machine preference pane"),
							"trackpad":             restrictionsBoolAttr("Enable Trackpad preference pane"),
							"users_and_groups":     restrictionsBoolAttr("Enable Users and Groups preference pane"),
							"xsan":                 restrictionsBoolAttr("Enable Xsan preference pane"),
							"icloud":               restrictionsBoolAttr("Enable iCloud preference pane"),
						},
					},
					"sharing": schema.SingleNestedAttribute{
						MarkdownDescription: "Sharing services restrictions",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"add_to_aperture":     restrictionsBoolAttr("Enable Add to Aperture"),
							"add_to_reading_list": restrictionsBoolAttr("Enable Add to Reading List"),
							"add_to_iphoto":       restrictionsBoolAttr("Enable Add to iPhoto"),
							"air_drop":            restrictionsBoolAttr("Enable AirDrop"),
							"automatically_enable_new_sharing_services": restrictionsBoolAttr("Allow new sharing services automatically"),
							"facebook": restrictionsBoolAttr("Enable Facebook"),
							"mail":     restrictionsBoolAttr("Enable Mail"),
							"messages": restrictionsBoolAttr("Enable Messages"),
							"restrict_which_sharing_services_are_enabled": restrictionsBoolAttr("Enable restrictions for sharing services"),
							"sina_weibo":     restrictionsBoolAttr("Enable Sina Weibo"),
							"twitter":        restrictionsBoolAttr("Enable Twitter"),
							"video_services": restrictionsBoolAttr("Enable Video Services"),
						},
					},
					"widgets": schema.SingleNestedAttribute{
						MarkdownDescription: "Widgets restrictions",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"allow_only_configured_widgets": schema.BoolAttribute{
								MarkdownDescription: "Allow only configured Widgets",
								Optional:            true,
							},
							"allowed_widgets": schema.ListAttribute{
								MarkdownDescription: "Allowed Widgets, as UEM picklist IDs (numeric strings) -- NOT widget names. Requires allow_only_configured_widgets = true, otherwise UEM clears it.",
								Optional:            true,
								ElementType:         types.StringType,
								Validators: []validator.List{
									listvalidator.ValueStringsAre(
										stringvalidator.RegexMatches(picklistIDRegex, "must be a numeric UEM picklist ID, not a widget name"),
									),
								},
							},
						},
					},
				},
			},
			"system_extensions": schema.SingleNestedAttribute{
				MarkdownDescription: "macOS System Extensions payload. Controls whether users may approve additional system extensions on their own, plus two independent allow-lists: extension TYPES permitted for a team identifier, and specific extensions (by bundle/team identifier) always approved. Distinct from the older kernel extension payload (`kernel_extension`). Applies to macOS only.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Object{
					nullWhenConfigNullObject(),
				},
				Attributes: map[string]schema.Attribute{
					"allow_user_overrides": schema.BoolAttribute{
						MarkdownDescription: "If false, restricts users from approving additional system extensions on their own.",
						Optional:            true,
					},
					"allowed_system_extension_types": schema.ListNestedAttribute{
						MarkdownDescription: "Extension TYPES (driver, endpoint security, network) allowed for a given team identifier, without naming a specific extension.",
						Optional:            true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"team_identifier": schema.StringAttribute{
									MarkdownDescription: `Apple Developer Team ID this rule applies to, or "*" for the global team identifier.`,
									Required:            true,
								},
								"allow_driver_extension_type": schema.BoolAttribute{
									MarkdownDescription: "Allow driver extension type for this team identifier.",
									Optional:            true,
								},
								"allow_endpoint_security_extension_type": schema.BoolAttribute{
									MarkdownDescription: "Allow endpoint security extension type for this team identifier.",
									Optional:            true,
								},
								"allow_network_extension_type": schema.BoolAttribute{
									MarkdownDescription: "Allow network extension type for this team identifier.",
									Optional:            true,
								},
							},
						},
					},
					"allowed_system_extensions": schema.ListNestedAttribute{
						MarkdownDescription: "Specific system extensions, by bundle and/or team identifier, that are always approved on the machine.",
						Optional:            true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"bundle_identifier": schema.StringAttribute{
									MarkdownDescription: "Bundle identifier of the system extension to always approve.",
									Optional:            true,
								},
								"team_identifier": schema.StringAttribute{
									MarkdownDescription: "Apple Developer Team ID of the system extension to always approve.",
									Optional:            true,
								},
							},
						},
					},
				},
			},
			"privacy_preferences": schema.ListNestedAttribute{
				MarkdownDescription: "macOS Privacy Preferences Policy Control (PPPC) payload. Each entry is one identity/app-scoped rule granting or denying that binary access to privacy-sensitive resources (camera, microphone, calendar, and so on) plus an optional list of Apple Events it may send to other processes. Applies to macOS only.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.List{
					nullWhenConfigNullList(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"identifier": schema.StringAttribute{
							MarkdownDescription: "Bundle ID or install path of the target binary this rule applies to.",
							Required:            true,
						},
						"identifier_type": schema.StringAttribute{
							MarkdownDescription: `How to interpret identifier. Must be either "bundleID" or "path".`,
							Required:            true,
							Validators: []validator.String{
								stringvalidator.OneOf("bundleID", "path"),
							},
						},
						"code_requirement": schema.StringAttribute{
							MarkdownDescription: "Code signing requirement for identifier (the output of `codesign -d -r-`).",
							Optional:            true,
						},
						"comment": schema.StringAttribute{
							MarkdownDescription: "Freeform note about this rule.",
							Optional:            true,
						},
						"apple_events_list": schema.ListNestedAttribute{
							MarkdownDescription: "Specific Apple Event receivers this identity is allowed (or denied) to send events to. UEM folds an identity-level Apple Events receiver into this list on write and never returns it in the older identity-level form, so this is the only supported way to configure Apple Events for a rule.",
							Optional:            true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"code_requirement": schema.StringAttribute{
										MarkdownDescription: "Code signing requirement for this Apple Event receiver.",
										Optional:            true,
									},
									"identifier": schema.StringAttribute{
										MarkdownDescription: "Bundle ID or install path of this Apple Event receiver.",
										Optional:            true,
									},
									"identifier_type": schema.StringAttribute{
										MarkdownDescription: `How to interpret identifier. Must be either "bundleID" or "path".`,
										Optional:            true,
										Validators: []validator.String{
											stringvalidator.OneOf("bundleID", "path"),
										},
									},
									"permission": schema.StringAttribute{
										MarkdownDescription: "Whether this identity may send Apple Events to this receiver. Accepted values are not documented by the SDK and unconfirmed live; left unvalidated.",
										Optional:            true,
									},
								},
							},
						},
						"static_code": schema.BoolAttribute{
							MarkdownDescription: "Whether identifier's code requirement should be evaluated statically rather than at runtime.",
							Optional:            true,
						},
						"accessibility": schema.StringAttribute{
							MarkdownDescription: `Access to control the computer via Accessibility APIs. Must be either "Allow" or "Disallow".`,
							Optional:            true,
							Validators: []validator.String{
								stringvalidator.OneOf("Allow", "Disallow"),
							},
						},
						"address_book": schema.StringAttribute{
							MarkdownDescription: `Access to Contacts. Must be either "Allow" or "Disallow".`,
							Optional:            true,
							Validators: []validator.String{
								stringvalidator.OneOf("Allow", "Disallow"),
							},
						},
						"calendar": schema.StringAttribute{
							MarkdownDescription: `Access to Calendar. Must be either "Allow" or "Disallow".`,
							Optional:            true,
							Validators: []validator.String{
								stringvalidator.OneOf("Allow", "Disallow"),
							},
						},
						"camera": schema.StringAttribute{
							MarkdownDescription: `Access to the camera. Can only be "Disallow" -- access cannot be granted by a profile.`,
							Optional:            true,
							Validators: []validator.String{
								stringvalidator.OneOf("Disallow"),
							},
						},
						"file_provider_presence": schema.StringAttribute{
							MarkdownDescription: "Access to File Provider presence. Accepted values are not restated by the SDK doc comment and unconfirmed live; left unvalidated.",
							Optional:            true,
						},
						"listen_event": schema.StringAttribute{
							MarkdownDescription: `Access to listen for system-wide keyboard/mouse events. Can only be "Disallow".`,
							Optional:            true,
							Validators: []validator.String{
								stringvalidator.OneOf("Disallow"),
							},
						},
						"media_library": schema.StringAttribute{
							MarkdownDescription: "Access to the Media Library. Accepted values are not restated by the SDK doc comment and unconfirmed live; left unvalidated.",
							Optional:            true,
						},
						"microphone": schema.StringAttribute{
							MarkdownDescription: `Access to the microphone. Can only be "Disallow" -- access cannot be granted by a profile.`,
							Optional:            true,
							Validators: []validator.String{
								stringvalidator.OneOf("Disallow"),
							},
						},
						"photos": schema.StringAttribute{
							MarkdownDescription: `Access to Photos. Must be either "Allow" or "Disallow".`,
							Optional:            true,
							Validators: []validator.String{
								stringvalidator.OneOf("Allow", "Disallow"),
							},
						},
						"post_event": schema.StringAttribute{
							MarkdownDescription: `Access to post synthesized system-wide keyboard/mouse events. Must be either "Allow" or "Disallow".`,
							Optional:            true,
							Validators: []validator.String{
								stringvalidator.OneOf("Allow", "Disallow"),
							},
						},
						"reminders": schema.StringAttribute{
							MarkdownDescription: `Access to Reminders. Must be either "Allow" or "Disallow".`,
							Optional:            true,
							Validators: []validator.String{
								stringvalidator.OneOf("Allow", "Disallow"),
							},
						},
						"screen_capture": schema.StringAttribute{
							MarkdownDescription: `Access to capture the screen. Can only be "Disallow".`,
							Optional:            true,
							Validators: []validator.String{
								stringvalidator.OneOf("Disallow"),
							},
						},
						"speech_recognition": schema.StringAttribute{
							MarkdownDescription: "Access to Speech Recognition. Accepted values are not restated by the SDK doc comment and unconfirmed live; left unvalidated.",
							Optional:            true,
						},
						"system_policy_all_files": schema.StringAttribute{
							MarkdownDescription: `Full Disk Access. Must be either "Allow" or "Disallow".`,
							Optional:            true,
							Validators: []validator.String{
								stringvalidator.OneOf("Allow", "Disallow"),
							},
						},
						"system_policy_desktop_folder": schema.StringAttribute{
							MarkdownDescription: "Access to the Desktop folder. Accepted values are not restated by the SDK doc comment and unconfirmed live; left unvalidated.",
							Optional:            true,
						},
						"system_policy_documents_folder": schema.StringAttribute{
							MarkdownDescription: "Access to the Documents folder. Accepted values are not restated by the SDK doc comment and unconfirmed live; left unvalidated.",
							Optional:            true,
						},
						"system_policy_downloads_folder": schema.StringAttribute{
							MarkdownDescription: "Access to the Downloads folder. Accepted values are not restated by the SDK doc comment and unconfirmed live; left unvalidated.",
							Optional:            true,
						},
						"system_policy_network_volumes": schema.StringAttribute{
							MarkdownDescription: "Access to network-mounted volumes. Accepted values are not restated by the SDK doc comment and unconfirmed live; left unvalidated.",
							Optional:            true,
						},
						"system_policy_removable_volumes": schema.StringAttribute{
							MarkdownDescription: "Access to removable volumes. Accepted values are not restated by the SDK doc comment and unconfirmed live; left unvalidated.",
							Optional:            true,
						},
						"system_policy_sys_admin_files": schema.StringAttribute{
							MarkdownDescription: `Access to administer the system (files normally requiring elevated privileges). Must be either "Allow" or "Disallow".`,
							Optional:            true,
							Validators: []validator.String{
								stringvalidator.OneOf("Allow", "Disallow"),
							},
						},
					},
				},
			},
			"scep_list": schema.ListNestedAttribute{
				MarkdownDescription: "macOS SCEP payloads (UEM `ScepList`). Each entry requests a certificate from a " +
					"certificate authority UEM already knows (`certificate_authority_id` and `certificate_template_id` " +
					"are that CA's and template's ids in UEM; this provider doesn't manage them). Values are passed to " +
					"UEM and read back as-is. Applies to macOS only. A field you leave unset takes the value UEM reports (UEM fills a default for most fields on create); removing a field from the configuration keeps its current value rather than clearing it, so to change a field set it explicitly.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.List{
					nullWhenConfigNullList(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							MarkdownDescription: "Name of the SCEP payload (UEM `Name`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							}},
						"credential_source": schema.StringAttribute{
							MarkdownDescription: "Where the certificate comes from (UEM `CredentialSource`, e.g. `DefinedCA`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							}},
						"certificate_authority_id": schema.Int64Attribute{
							MarkdownDescription: "UEM id of the certificate authority (UEM `CertificateAuthorityId`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Int64{
								int64planmodifier.UseStateForUnknown(),
								useStateForNullInt64(),
							}},
						"certificate_template_id": schema.Int64Attribute{
							MarkdownDescription: "UEM id of the certificate template (UEM `CertificateTemplateId`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Int64{
								int64planmodifier.UseStateForUnknown(),
								useStateForNullInt64(),
							}},
						"allow_export_from_key_chain": schema.BoolAttribute{
							MarkdownDescription: "Allow the private key to be exported from the keychain (UEM `AllowExportFromKeyChain`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
								useStateForNullBool(),
							}},
						"identity_preference": schema.SingleNestedAttribute{
							MarkdownDescription: "Identity preference (UEM `IdentityPreference`).",
							Optional:            true,
							Attributes: map[string]schema.Attribute{
								"names": schema.ListAttribute{
									MarkdownDescription: "Identity preference names (UEM `Names`).",
									ElementType:         types.StringType,
									Optional:            true,
									Computed:            true,
									PlanModifiers: []planmodifier.List{
										listplanmodifier.UseStateForUnknown(),
										useStateForNullList(),
									}},
							},
						},
					},
				},
			},
			"web_clips_list": schema.ListNestedAttribute{
				MarkdownDescription: "macOS Web Clips (UEM `WebClipsList`): links to web pages placed on the device. " +
					"Values are passed to UEM and read back as-is. Applies to macOS only. A field you leave unset takes the value UEM reports (UEM fills a default for most fields on create); removing a field from the configuration keeps its current value rather than clearing it, so to change a field set it explicitly.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.List{
					nullWhenConfigNullList(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"label": schema.StringAttribute{
							MarkdownDescription: "Name shown under the Web Clip icon (UEM `Label`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							}},
						"url": schema.StringAttribute{
							MarkdownDescription: "URL the Web Clip opens (UEM `URL`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							}},
						"show_in_app_catalog": schema.BoolAttribute{
							MarkdownDescription: "Show the Web Clip in the app catalog (UEM `ShowInAppCatalog`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
								useStateForNullBool(),
							}},
						"icon": schema.Int64Attribute{
							MarkdownDescription: "UEM id of an icon image already uploaded to UEM (UEM `Icon`). This provider doesn't upload icons.",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Int64{
								int64planmodifier.UseStateForUnknown(),
								useStateForNullInt64(),
							}},
					},
				},
			},
			"vpn_list": schema.ListNestedAttribute{
				MarkdownDescription: "macOS VPN payloads (UEM `VpnList`). Values are passed to UEM and read back as-is, except " +
					"`password`, `shared_secret` and `vpn_password`: UEM returns those masked, so they are write-only and never read back. UEM clears one of these secrets when an update leaves it out, so a plan that would leave out a secret UEM holds is refused: set it in the configuration first. Applies to macOS only. A field you leave unset takes the value UEM reports (UEM fills a default for most fields on create); removing a field from the configuration keeps its current value rather than clearing it, so to change a field set it explicitly.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.List{
					nullWhenConfigNullList(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"account": schema.StringAttribute{
							MarkdownDescription: "Account (UEM `Account`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							}},
						"app_mapping": schema.BoolAttribute{
							MarkdownDescription: "App mapping (UEM `AppMapping`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
								useStateForNullBool(),
							}},
						"application_bundle_id": schema.ListAttribute{
							MarkdownDescription: "Application bundle id (UEM `ApplicationBundleId`).",
							ElementType:         types.StringType,
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.List{
								listplanmodifier.UseStateForUnknown(),
								useStateForNullList(),
							}},
						"associated_domains": schema.ListAttribute{
							MarkdownDescription: "Associated domains (UEM `AssociatedDomains`).",
							ElementType:         types.StringType,
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.List{
								listplanmodifier.UseStateForUnknown(),
								useStateForNullList(),
							}},
						"calendar_domains": schema.ListAttribute{
							MarkdownDescription: "Calendar domains (UEM `CalendarDomains`).",
							ElementType:         types.StringType,
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.List{
								listplanmodifier.UseStateForUnknown(),
								useStateForNullList(),
							}},
						"connect_automatically": schema.BoolAttribute{
							MarkdownDescription: "Connect automatically (UEM `ConnectAutomatically`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
								useStateForNullBool(),
							}},
						"connection_name": schema.StringAttribute{
							MarkdownDescription: "Connection name (UEM `ConnectionName`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							}},
						"connection_type": schema.StringAttribute{
							MarkdownDescription: "Connection type (UEM `ConnectionType`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							}},
						"contacts_domains": schema.ListAttribute{
							MarkdownDescription: "Contacts domains (UEM `ContactsDomains`).",
							ElementType:         types.StringType,
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.List{
								listplanmodifier.UseStateForUnknown(),
								useStateForNullList(),
							}},
						"custom_datas": schema.ListNestedAttribute{
							MarkdownDescription: "Custom datas (UEM `CustomDatas`).",
							Optional:            true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"key": schema.StringAttribute{
										MarkdownDescription: "Key (UEM `Key`).",
										Optional:            true,
										Computed:            true,
										PlanModifiers: []planmodifier.String{
											stringplanmodifier.UseStateForUnknown(),
											useStateForNullString(),
										}},
									"value": schema.StringAttribute{
										MarkdownDescription: "Value (UEM `Value`).",
										Optional:            true,
										Computed:            true,
										PlanModifiers: []planmodifier.String{
											stringplanmodifier.UseStateForUnknown(),
											useStateForNullString(),
										}},
								},
							},
						},
						"enable_safari_domains": schema.BoolAttribute{
							MarkdownDescription: "Enable safari domains (UEM `EnableSafariDomains`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
								useStateForNullBool(),
							}},
						"enable_vpn_on_demand": schema.BoolAttribute{
							MarkdownDescription: "Enable vpn on demand (UEM `EnableVPNOnDemand`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
								useStateForNullBool(),
							}},
						"encryption_level": schema.Int64Attribute{
							MarkdownDescription: "Encryption level (UEM `EncryptionLevel`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Int64{
								int64planmodifier.UseStateForUnknown(),
								useStateForNullInt64(),
							}},
						"exclude_local_networks": schema.BoolAttribute{
							MarkdownDescription: "Exclude local networks (UEM `ExcludeLocalNetworks`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
								useStateForNullBool(),
							}},
						"excluded_domains": schema.ListAttribute{
							MarkdownDescription: "Excluded domains (UEM `ExcludedDomains`).",
							ElementType:         types.StringType,
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.List{
								listplanmodifier.UseStateForUnknown(),
								useStateForNullList(),
							}},
						"group_name": schema.StringAttribute{
							MarkdownDescription: "Group name (UEM `GroupName`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							}},
						"identity_certificate": schema.StringAttribute{
							MarkdownDescription: "Identity certificate (UEM `IdentityCertificate`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							}},
						"include_all_networks": schema.BoolAttribute{
							MarkdownDescription: "Include all networks (UEM `IncludeAllNetworks`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
								useStateForNullBool(),
							}},
						"include_user_pin": schema.BoolAttribute{
							MarkdownDescription: "Include user pin (UEM `IncludeUserPIN`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
								useStateForNullBool(),
							}},
						"machine_authentication": schema.Int64Attribute{
							MarkdownDescription: "Machine authentication (UEM `MachineAuthentication`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Int64{
								int64planmodifier.UseStateForUnknown(),
								useStateForNullInt64(),
							}},
						"mail_domains": schema.ListAttribute{
							MarkdownDescription: "Mail domains (UEM `MailDomains`).",
							ElementType:         types.StringType,
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.List{
								listplanmodifier.UseStateForUnknown(),
								useStateForNullList(),
							}},
						"mdm_assigned_id": schema.StringAttribute{
							MarkdownDescription: "Mdm assigned id (UEM `MdmAssignedId`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							}},
						"mdm_device_serial_number": schema.StringAttribute{
							MarkdownDescription: "Mdm device serial number (UEM `MdmDeviceSerialNumber`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							}},
						"mdm_device_unique_id": schema.StringAttribute{
							MarkdownDescription: "Mdm device unique id (UEM `MdmDeviceUniqueId`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							}},
						"mdm_device_wifi_mac_address": schema.StringAttribute{
							MarkdownDescription: "Mdm device wifi mac address (UEM `MdmDeviceWifiMacAddress`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							}},
						"password": schema.StringAttribute{
							MarkdownDescription: "Password (UEM `Password`). Write-only: UEM returns it masked, so it is never read back.",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							},
							Sensitive: true},
						"per_app_vpn": schema.BoolAttribute{
							MarkdownDescription: "Per app vpn (UEM `PerAppVpn`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
								useStateForNullBool(),
							}},
						"port": schema.Int64Attribute{
							MarkdownDescription: "Port (UEM `Port`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Int64{
								int64planmodifier.UseStateForUnknown(),
								useStateForNullInt64(),
							}},
						"prompt_for_password": schema.BoolAttribute{
							MarkdownDescription: "Prompt for password (UEM `PromptForPassword`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
								useStateForNullBool(),
							}},
						"provider_designated_requirement": schema.StringAttribute{
							MarkdownDescription: "Provider designated requirement (UEM `ProviderDesignatedRequirement`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							}},
						"provider_type": schema.StringAttribute{
							MarkdownDescription: "Provider type (UEM `ProviderType`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							}},
						"proxy": schema.StringAttribute{
							MarkdownDescription: "Proxy type (UEM `Proxy`): `None`, `Manual` or `Automatic`. UEM requires a value on every create and update but leaves it out of reads when none is stored, so when unset the provider sends `None` and reads a missing value as `None`.",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"proxy_server": schema.StringAttribute{
							MarkdownDescription: "Proxy server (UEM `ProxyServer`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							}},
						"proxy_server_auto_config_url": schema.StringAttribute{
							MarkdownDescription: "Proxy server auto config url (UEM `ProxyServerAutoConfigURL`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							}},
						"safari_domains": schema.ListAttribute{
							MarkdownDescription: "Safari domains (UEM `SafariDomains`).",
							ElementType:         types.StringType,
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.List{
								listplanmodifier.UseStateForUnknown(),
								useStateForNullList(),
							}},
						"send_all_traffic": schema.BoolAttribute{
							MarkdownDescription: "Send all traffic (UEM `SendAllTraffic`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
								useStateForNullBool(),
							}},
						"server": schema.StringAttribute{
							MarkdownDescription: "Server (UEM `Server`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							}},
						"shared_secret": schema.StringAttribute{
							MarkdownDescription: "Shared secret (UEM `SharedSecret`). Write-only: UEM returns it masked, so it is never read back.",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							},
							Sensitive: true},
						"use_hybrid_authentication": schema.BoolAttribute{
							MarkdownDescription: "Use hybrid authentication (UEM `UseHybridAuthentication`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
								useStateForNullBool(),
							}},
						"user_authentication": schema.StringAttribute{
							MarkdownDescription: "User authentication (UEM `UserAuthentication`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							}},
						"user_name": schema.StringAttribute{
							MarkdownDescription: "User name (UEM `UserName`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							}},
						"vpn_on_demand": schema.ListNestedAttribute{
							MarkdownDescription: "Vpn on demand (UEM `VpnOnDemand`).",
							Optional:            true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"domain": schema.StringAttribute{
										MarkdownDescription: "Domain (UEM `Domain`).",
										Optional:            true,
										Computed:            true,
										PlanModifiers: []planmodifier.String{
											stringplanmodifier.UseStateForUnknown(),
											useStateForNullString(),
										}},
									"on_demand_action": schema.StringAttribute{
										MarkdownDescription: "On demand action (UEM `OnDemandAction`).",
										Optional:            true,
										Computed:            true,
										PlanModifiers: []planmodifier.String{
											stringplanmodifier.UseStateForUnknown(),
											useStateForNullString(),
										}},
								},
							},
						},
						"vpn_password": schema.StringAttribute{
							MarkdownDescription: "Vpn password (UEM `VpnPassword`). Write-only: UEM returns it masked, so it is never read back.",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							},
							Sensitive: true},
						"web_logon": schema.BoolAttribute{
							MarkdownDescription: "Web logon (UEM `WebLogon`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
								useStateForNullBool(),
							}},
					},
				},
			},
			"eas_microsoft_outlook": schema.SingleNestedAttribute{
				MarkdownDescription: "macOS Exchange (Microsoft Outlook) payload (UEM `EasMicrosoftOutlook`). Values are passed to UEM and read back as-is, " +
					"except `password`: UEM returns it masked, so it is write-only and never read back. UEM clears one of these secrets when an update leaves it out, so a plan that would leave out a secret UEM holds is refused: set it in the configuration first. Applies to macOS only. UEM rejects a profile that combines this payload with any other payload (HTTP 422 \"Cannot add multiple payloads when AppleOsXEasMicrosoftOutlookPayloadEntity is provided\", live-confirmed): put it in a profile of its own. A field you leave unset takes the value UEM reports (UEM fills a default for most fields on create); removing a field from the configuration keeps its current value rather than clearing it, so to change a field set it explicitly.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Object{
					nullWhenConfigNullObject(),
				},
				Attributes: map[string]schema.Attribute{
					"account_name": schema.StringAttribute{
						MarkdownDescription: "Account name (UEM `AccountName`).",
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
							useStateForNullString(),
						}},
					"directory_server": schema.StringAttribute{
						MarkdownDescription: "Directory server (UEM `DirectoryServer`).",
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
							useStateForNullString(),
						}},
					"directory_server_port": schema.StringAttribute{
						MarkdownDescription: "Directory server port (UEM `DirectoryServerPort`).",
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
							useStateForNullString(),
						}},
					"directory_server_requires_ssl": schema.BoolAttribute{
						MarkdownDescription: "Directory server requires ssl (UEM `DirectoryServerRequiresSSL`).",
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.Bool{
							boolplanmodifier.UseStateForUnknown(),
							useStateForNullBool(),
						}},
					"domain": schema.StringAttribute{
						MarkdownDescription: "Domain (UEM `Domain`).",
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
							useStateForNullString(),
						}},
					"email_address": schema.StringAttribute{
						MarkdownDescription: "Email address (UEM `EmailAddress`). UEM requires it when creating the profile (HTTP 422 on create when left out, live-confirmed).",
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
							useStateForNullString(),
						}},
					"exchange_host": schema.StringAttribute{
						MarkdownDescription: "Exchange host (UEM `ExchangeHost`).",
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
							useStateForNullString(),
						}},
					"exchange_port": schema.StringAttribute{
						MarkdownDescription: "Exchange port (UEM `ExchangePort`).",
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
							useStateForNullString(),
						}},
					"password": schema.StringAttribute{
						MarkdownDescription: "Password (UEM `Password`). Write-only: UEM returns it masked, so it is never read back.",
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
							useStateForNullString(),
						},
						Sensitive: true},
					"search_base": schema.StringAttribute{
						MarkdownDescription: "Search base (UEM `SearchBase`).",
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
							useStateForNullString(),
						}},
					"use_ssl": schema.BoolAttribute{
						MarkdownDescription: "Use ssl (UEM `UseSSL`).",
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.Bool{
							boolplanmodifier.UseStateForUnknown(),
							useStateForNullBool(),
						}},
					"user_name": schema.StringAttribute{
						MarkdownDescription: "User name (UEM `UserName`). UEM requires it when creating the profile (HTTP 422 on create when left out, live-confirmed).",
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
							useStateForNullString(),
						}},
				},
			},
			"kernel_extension": schema.SingleNestedAttribute{
				MarkdownDescription: "macOS Kernel Extensions payload (UEM `KernelExtension`): the legacy kernel extension allow-list. Values are passed to UEM and read back as-is. Applies to macOS only. A field you leave unset takes the value UEM reports (UEM fills a default for most fields on create); removing a field from the configuration keeps its current value rather than clearing it, so to change a field set it explicitly.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Object{
					nullWhenConfigNullObject(),
				},
				Attributes: map[string]schema.Attribute{
					"allow_user_overrides": schema.BoolAttribute{
						MarkdownDescription: "Allow user overrides (UEM `AllowUserOverrides`).",
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.Bool{
							boolplanmodifier.UseStateForUnknown(),
							useStateForNullBool(),
						}},
					"allowed_kernel_extensions": schema.ListNestedAttribute{
						MarkdownDescription: "Allowed kernel extensions (UEM `AllowedKernelExtensions`).",
						Optional:            true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"bundle_identifier": schema.StringAttribute{
									MarkdownDescription: "Bundle identifier (UEM `BundleIdentifier`).",
									Optional:            true,
									Computed:            true,
									PlanModifiers: []planmodifier.String{
										stringplanmodifier.UseStateForUnknown(),
										useStateForNullString(),
									}},
								"team_identifier": schema.StringAttribute{
									MarkdownDescription: "Team identifier (UEM `TeamIdentifier`).",
									Optional:            true,
									Computed:            true,
									PlanModifiers: []planmodifier.String{
										stringplanmodifier.UseStateForUnknown(),
										useStateForNullString(),
									}},
							},
						},
					},
					"allowed_team_identifiers": schema.ListAttribute{
						MarkdownDescription: "Allowed team identifiers (UEM `AllowedTeamIdentifiers`).",
						ElementType:         types.StringType,
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.List{
							// UseStateForUnknown only (B48, same reasoning as
							// disk_encryption.mcx/filevault2.enable in B42):
							// on create (no prior state) this leaves the plan
							// genuinely unknown, which is what lets UEM's
							// echoed non-null default ([""]) land in state
							// without tripping "was null, but now [...]"
							// (Terraform core only allows the applied value
							// to differ from the plan when the plan was
							// unknown, never when it was null).
							// useStateForNullList used to run alongside this
							// and forced create's unknown plan to an
							// explicit null -- Terraform core does not
							// exempt Computed attributes from the
							// plan/actual consistency check when config is
							// null, so that forced null is exactly what live
							// create rejected (B48, live as<internal-env>). The model
							// field is types.List, which holds unknown fine,
							// so no builder/read-mapper change is needed.
							listplanmodifier.UseStateForUnknown(),
						}},
				},
			},
			"custom_attributes": schema.ListNestedAttribute{
				MarkdownDescription: "macOS Custom Attributes payload (UEM `CustomAttributes`): scripts that report device attributes. `attribute_script` is the base64 script body. Values are passed to UEM and read back as-is. Applies to macOS only. A field you leave unset takes the value UEM reports (UEM fills a default for most fields on create); removing a field from the configuration keeps its current value rather than clearing it, so to change a field set it explicitly.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.List{
					nullWhenConfigNullList(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"attribute_name": schema.StringAttribute{
							MarkdownDescription: "Attribute name (UEM `AttributeName`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							}},
						"attribute_script": schema.StringAttribute{
							MarkdownDescription: "Attribute script (UEM `AttributeScript`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
								useStateForNullString(),
							}},
						"events": schema.ListAttribute{
							MarkdownDescription: "Events (UEM `Events`).",
							ElementType:         types.StringType,
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.List{
								listplanmodifier.UseStateForUnknown(),
								useStateForNullList(),
							}},
						"schedule": schema.Int64Attribute{
							MarkdownDescription: "Schedule (UEM `Schedule`).",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Int64{
								int64planmodifier.UseStateForUnknown(),
								useStateForNullInt64(),
							}},
					},
				},
			},
			"uuid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Profile UUID",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"profile_context": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Profile context: Device or User. Required for AppleOsX profiles (defaults to Device). UEM has no server default for this field and rejects a profile without it; the provider supplies Device when it is unset.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}
