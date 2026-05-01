package profile

import (
	"context"
	"fmt"
	"sync"

	sdk "github.com/euc-oss/terraform-sdk-uem"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure resource types satisfy framework interfaces.
var _ resource.Resource = &ProfileResource{}
var _ resource.ResourceWithImportState = &ProfileResource{}

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

// useStateForNullModifier copies the prior state value into the plan when
// the configuration value is null. This allows Optional+Computed nested
// attributes to be preserved during import and subsequent plans when the
// user hasn't explicitly configured the block.
type useStateForNullModifier struct{}

func (m useStateForNullModifier) Description(_ context.Context) string {
	return "Use the prior state value when the configuration value is null."
}

func (m useStateForNullModifier) MarkdownDescription(_ context.Context) string {
	return "Use the prior state value when the configuration value is null."
}

func (m useStateForNullModifier) PlanModifyObject(ctx context.Context, req planmodifier.ObjectRequest, resp *planmodifier.ObjectResponse) {
	if req.ConfigValue.IsNull() && !req.StateValue.IsNull() {
		resp.PlanValue = req.StateValue
		return
	}
	// During create (state is null) and config is null, the framework marks
	// Computed object attributes as unknown. Convert unknown → null so that
	// plan decoding into a *Model pointer field does not fail with
	// "cannot handle unknown values". Symmetric with useStateForNullListModifier.
	if req.ConfigValue.IsNull() && resp.PlanValue.IsUnknown() {
		resp.PlanValue = types.ObjectNull(req.PlanValue.AttributeTypes(ctx))
	}
}

func useStateForNull() planmodifier.Object {
	return useStateForNullModifier{}
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

// restrictionsBoolAttr returns a standard optional bool attribute used by the
// restrictions payload (preferences/sharing panes are all uniform bool
// toggles).
func restrictionsBoolAttr(desc string) schema.BoolAttribute {
	return schema.BoolAttribute{
		MarkdownDescription: desc,
		Optional:            true,
	}
}

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
		Version:             1,
		MarkdownDescription: "Manages a Workspace ONE UEM Profile",

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
				MarkdownDescription: "Profile description",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					useStateForNullString(),
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
				MarkdownDescription: "Organization Group ID",
				Required:            true,
			},
			"assignment_type": schema.StringAttribute{
				MarkdownDescription: "Assignment type: Auto or Optional. Defaults to Auto",
				Optional:            true,
				Computed:            true,
			},
			"profile_scope": schema.StringAttribute{
				MarkdownDescription: "Profile scope: Production or Test. Defaults to Production",
				Optional:            true,
				Computed:            true,
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
					objectplanmodifier.UseStateForUnknown(),
					useStateForNull(),
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
					listplanmodifier.UseStateForUnknown(),
					useStateForNullList(),
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
					listplanmodifier.UseStateForUnknown(),
					useStateForNullList(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"network_interface": schema.StringAttribute{
							MarkdownDescription: "Network interface: Wi-Fi or Ethernet",
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
							MarkdownDescription: "Encryption type of the Wi-Fi network (e.g. WPA2, WPA3, WEP, None)",
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
							MarkdownDescription: "Inner authentication method used by TTLS",
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
					"Certificate Authority (credential_source = \"DefinedCertificateAuthority\"). " +
					"Reference a credential in a network_list entry via identity_certificate = <credential_name>.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
					useStateForNullList(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"credential_source": schema.StringAttribute{
							MarkdownDescription: `Source of the credential. Valid values: "Upload" (base64 certificate file) or "DefinedCertificateAuthority" (pre-configured CA in UEM).`,
							Required:            true,
						},
						"credential_name": schema.StringAttribute{
							MarkdownDescription: "Human-readable name for this credential, referenced by identity_certificate in network payloads. " +
								"Required when credential_source is \"Upload\" so network_list entries can join to it; " +
								"optional for DefinedCertificateAuthority credentials, which are identified by certificate_authority + certificate_template.",
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
							MarkdownDescription: "Numeric certificate ID assigned by UEM after upload. Computed automatically when credential_source is \"Upload\"; leave unset for DefinedCertificateAuthority.",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Int64{
								int64planmodifier.UseStateForUnknown(),
								useStateForNullInt64(),
							},
						},
						"certificate_authority": schema.Int64Attribute{
							MarkdownDescription: "Numeric ID of the Certificate Authority in UEM. Required when credential_source is \"DefinedCertificateAuthority\".",
							Optional:            true,
						},
						"certificate_template": schema.Int64Attribute{
							MarkdownDescription: "Numeric ID of the certificate template within the CA. Required when credential_source is \"DefinedCertificateAuthority\".",
							Optional:            true,
						},
						"allow_access_to_all_applications": schema.BoolAttribute{
							MarkdownDescription: "When true, all apps on the device can access the private key in the Keychain.",
							Optional:            true,
						},
						"key_is_extractable": schema.BoolAttribute{
							MarkdownDescription: "Allow export of the private key from the Keychain (macOS 10.15+). Corresponds to \"Allow export of private key from Keychain\" in the UI.",
							Optional:            true,
						},
					},
				},
			},
			"disk_encryption": schema.SingleNestedAttribute{
				MarkdownDescription: "Disk encryption (FileVault 2) settings (macOS only)",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.UseStateForUnknown(),
					useStateForNull(),
				},
				Attributes: map[string]schema.Attribute{
					"airwatch": schema.SingleNestedAttribute{
						MarkdownDescription: "AirWatch disk encryption enforcement settings",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"store_key": schema.BoolAttribute{
								MarkdownDescription: "Store the recovery key in AirWatch",
								Optional:            true,
							},
							"rotate_key_after": schema.Int64Attribute{
								MarkdownDescription: "Days until the recovery key rotates",
								Optional:            true,
							},
							"use_intelligent_hub": schema.BoolAttribute{
								MarkdownDescription: "Use Intelligent Hub for enforcement",
								Optional:            true,
							},
							"notify_user_for_encryption": schema.BoolAttribute{
								MarkdownDescription: "Notify user if encryption is disabled",
								Optional:            true,
							},
							"encryption_notification_title": schema.StringAttribute{
								MarkdownDescription: "Title for the encryption notification (max 29 chars)",
								Optional:            true,
							},
							"encryption_notification_message": schema.StringAttribute{
								MarkdownDescription: "Message for the encryption notification",
								Optional:            true,
							},
							"encryption_max_notify_attempts": schema.Int64Attribute{
								MarkdownDescription: "Maximum times to show the encryption notification",
								Optional:            true,
							},
							"encryption_notification_retry_interval_in_hours": schema.Int64Attribute{
								MarkdownDescription: "Retry interval in hours between notification dismissals",
								Optional:            true,
							},
							"encryption_action_after_last_notification": schema.Int64Attribute{
								MarkdownDescription: "Action after last notification dismissal: 1 = Force Logout, 2 = Do Nothing",
								Optional:            true,
							},
							"enable_recovery_key": schema.BoolAttribute{
								MarkdownDescription: "Prompt for user password to rotate recovery key",
								Optional:            true,
							},
							"recovery_key_notification_title": schema.StringAttribute{
								MarkdownDescription: "Title for rotating recovery key notification (max 29 chars)",
								Optional:            true,
							},
							"recovery_key_notification_message": schema.StringAttribute{
								MarkdownDescription: "Message for rotating recovery key notification",
								Optional:            true,
							},
							"recovery_key_notification_retry_interval_in_hours": schema.Int64Attribute{
								MarkdownDescription: "Retry interval in hours for recovery key notification",
								Optional:            true,
							},
							"recovery_key_prompt_title": schema.StringAttribute{
								MarkdownDescription: "Prompt title for rotating recovery key (max 50 chars)",
								Optional:            true,
							},
							"recovery_key_prompt_message": schema.StringAttribute{
								MarkdownDescription: "Prompt message for rotating recovery key (max 150 chars)",
								Optional:            true,
							},
							"recovery_key_success_title": schema.StringAttribute{
								MarkdownDescription: "Success title after recovery key rotation (max 50 chars)",
								Optional:            true,
							},
							"recovery_key_success_message": schema.StringAttribute{
								MarkdownDescription: "Success message after recovery key rotation (max 150 chars)",
								Optional:            true,
							},
							"recovery_key_error_title": schema.StringAttribute{
								MarkdownDescription: "Error title for recovery key rotation failure (max 50 chars)",
								Optional:            true,
							},
							"recovery_key_error_message": schema.StringAttribute{
								MarkdownDescription: "Error message for recovery key rotation failure (max 150 chars)",
								Optional:            true,
							},
							"recovery_key_max_failure_count": schema.Int64Attribute{
								MarkdownDescription: "Maximum times to retry recovery key rotation on failure",
								Optional:            true,
							},
						},
					},
					"filevault2": schema.SingleNestedAttribute{
						MarkdownDescription: "FileVault 2 encryption settings",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"enable": schema.BoolAttribute{
								MarkdownDescription: "Enforce disk encryption",
								Optional:            true,
							},
							"show_recovery_key": schema.BoolAttribute{
								MarkdownDescription: "Show the personal recovery key to the user",
								Optional:            true,
							},
							"recovery_type": schema.Int64Attribute{
								MarkdownDescription: "Recovery type: 1 = Personal, 2 = Institutional, 3 = Personal and Corporate",
								Optional:            true,
							},
							"filevault_enterprise_certificate": schema.StringAttribute{
								MarkdownDescription: "Certificate name from credential payload for institutional recovery",
								Optional:            true,
							},
							"filevault_user": schema.Int64Attribute{
								MarkdownDescription: "User type added to FileVault: 1 = Current or Next Login User, 2 = Specific User",
								Optional:            true,
							},
							"username": schema.StringAttribute{
								MarkdownDescription: "Open Directory username for FileVault (when filevault_user = 2)",
								Optional:            true,
							},
							"prompt_to_enable_filevault_at": schema.Int64Attribute{
								MarkdownDescription: "When to prompt: 1 = Both Login and Logout, 2 = Logout Only, 3 = Login Only",
								Optional:            true,
							},
							"number_of_times_user_can_bypass": schema.Int64Attribute{
								MarkdownDescription: "Number of times user can bypass the FileVault prompt (-1 for unlimited)",
								Optional:            true,
							},
						},
					},
					"mcx": schema.SingleNestedAttribute{
						MarkdownDescription: "MCX disk encryption settings",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"destroy_fv_key_on_standby": schema.BoolAttribute{
								MarkdownDescription: "Require user to unlock disk after hibernation",
								Optional:            true,
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
					objectplanmodifier.UseStateForUnknown(),
					useStateForNull(),
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
					objectplanmodifier.UseStateForUnknown(),
					useStateForNull(),
				},
				Attributes: map[string]schema.Attribute{
					"applications": schema.SingleNestedAttribute{
						MarkdownDescription: "Application restrictions",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"allow_application": schema.ListAttribute{
								MarkdownDescription: "List of applications to be allowed",
								Optional:            true,
								ElementType:         types.StringType,
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
								MarkdownDescription: "Allowed Widgets",
								Optional:            true,
								ElementType:         types.StringType,
							},
						},
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
				MarkdownDescription: "Profile context: Device or User. Required for AppleOsX profiles (defaults to Device)",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}
