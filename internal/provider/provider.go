package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/euc-oss/terraform-provider-uem/internal/httpclient"
	"github.com/euc-oss/terraform-provider-uem/internal/providerdata"
	"github.com/euc-oss/terraform-provider-uem/internal/smartgroup"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	sdkclient "github.com/euc-oss/terraform-sdk-uem/v26/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/euc-oss/terraform-provider-uem/internal/application/assignment"
	applicationdatasource "github.com/euc-oss/terraform-provider-uem/internal/application/datasource"
	macapplication "github.com/euc-oss/terraform-provider-uem/internal/application/internal-app/mac"
	purchasedappdatasource "github.com/euc-oss/terraform-provider-uem/internal/application/purchased-app"
	purchasedappassignment "github.com/euc-oss/terraform-provider-uem/internal/application/purchased-app/assignment"
	organizationgroupdatasource "github.com/euc-oss/terraform-provider-uem/internal/organizationgroup"
	profileresource "github.com/euc-oss/terraform-provider-uem/internal/profile"
	scriptsdatasource "github.com/euc-oss/terraform-provider-uem/internal/scripts"
	scriptassignment "github.com/euc-oss/terraform-provider-uem/internal/scripts/assignment"
	macscript "github.com/euc-oss/terraform-provider-uem/internal/scripts/mac"
	sensorsdatasource "github.com/euc-oss/terraform-provider-uem/internal/sensors"
	sensorassignment "github.com/euc-oss/terraform-provider-uem/internal/sensors/assignment"
	macsensor "github.com/euc-oss/terraform-provider-uem/internal/sensors/mac"
	updatesdatasource "github.com/euc-oss/terraform-provider-uem/internal/updates"
	updatedeployment "github.com/euc-oss/terraform-provider-uem/internal/updates/deployment"
)

// Ensure WorkspaceOneProvider satisfies various provider interfaces.
var _ provider.Provider = &WorkspaceOneProvider{}
var _ provider.ProviderWithFunctions = &WorkspaceOneProvider{}

// WorkspaceOneProvider defines the provider implementation.
type WorkspaceOneProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// WorkspaceOneProviderModel describes the provider data model.
type WorkspaceOneProviderModel struct {
	InstanceURL          types.String `tfsdk:"instance_url"`
	TenantCode           types.String `tfsdk:"tenant_code"`
	AuthMethod           types.String `tfsdk:"auth_method"`
	Username             types.String `tfsdk:"username"`
	Password             types.String `tfsdk:"password"`
	ClientID             types.String `tfsdk:"client_id"`
	ClientSecret         types.String `tfsdk:"client_secret"`
	OAuth2TokenURL       types.String `tfsdk:"oauth2_token_url"`
	AppBinaryStoragePath types.String `tfsdk:"app_binary_storage_path"`
}

func (p *WorkspaceOneProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "uem"
	resp.Version = p.version
}

func (p *WorkspaceOneProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Terraform provider for Omnissa Workspace ONE UEM",
		Attributes: map[string]schema.Attribute{
			"instance_url": schema.StringAttribute{
				MarkdownDescription: "Workspace ONE UEM instance URL (e.g., https://your-instance.awmdm.com). Can also be set via UEM_INSTANCE_URL environment variable.",
				Optional:            true,
			},
			"tenant_code": schema.StringAttribute{
				MarkdownDescription: "Workspace ONE UEM tenant code (API Key). Can also be set via UEM_TENANT_CODE environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
			"auth_method": schema.StringAttribute{
				MarkdownDescription: "Authentication method: 'basic' or 'oauth2'. Optional — can also be set via the UEM_AUTH_METHOD " +
					"environment variable. When left unset, the provider infers the method from whichever credential " +
					"set(s) are configured: oauth2 is preferred when both username+password and client_id+client_secret " +
					"are complete; otherwise whichever single set is complete is used. When explicitly set (config or env), " +
					"the explicit value wins and the corresponding credential set must be present or Configure fails. " +
					"Both credential sets may be supplied simultaneously; only the resolved/primary method's client is used " +
					"for resource and data source calls.",
				Optional: true,
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "Username for Basic Authentication. Can also be set via UEM_USERNAME environment variable.",
				Optional:            true,
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "Password for Basic Authentication. Can also be set via UEM_PASSWORD environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
			"client_id": schema.StringAttribute{
				MarkdownDescription: "Client ID for OAuth2 Authentication. Can also be set via UEM_CLIENT_ID environment variable.",
				Optional:            true,
			},
			"client_secret": schema.StringAttribute{
				MarkdownDescription: "Client Secret for OAuth2 Authentication. Can also be set via UEM_CLIENT_SECRET environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
			"oauth2_token_url": schema.StringAttribute{
				MarkdownDescription: "OAuth2 token URL. Defaults to https://na.uemauth.workspaceone.com/connect/token. Can also be set via UEM_OAUTH2_TOKEN_URL environment variable.",
				Optional:            true,
			},
			"app_binary_storage_path": schema.StringAttribute{
				MarkdownDescription: "Local directory where uem_mac_application binaries (DMG/plist) are stored for read/import round-tripping. " +
					"Can also be set via UEM_APP_BINARY_DIR environment variable. Defaults to uem-artifacts/app-binaries " +
					"(relative to the Terraform working directory) if unset — never a home directory.",
				Optional: true,
			},
		},
	}
}

// sdkHTTPClientTimeout is passed as sdkclient.Config.Timeout for every SDK
// client this provider builds (B10). The SDK's own NewClient defaults
// Config.Timeout to 30s whenever it is left zero and applies it as
// http.Client.Timeout — a wall-clock cap over the WHOLE request, body
// included, that has nothing to do with the caller-supplied HTTPClient's own
// Transport settings (see newStallAwareHTTPClient). Setting a long fixed
// value here just keeps that branch from ever substituting its own 30s cap
// back in; actual staleness protection comes entirely from
// newStallAwareHTTPClient's header/stall timeouts, not from this field.
const sdkHTTPClientTimeout = 24 * time.Hour

// newStallAwareHTTPClient builds the *http.Client passed as
// sdkclient.Config.HTTPClient for both the basic and oauth2 SDK clients
// (B10 — see internal/httpclient's package doc for the full root-cause
// writeup). The header/stall timeouts are resolved from
// UEM_HTTP_HEADER_TIMEOUT / UEM_HTTP_STALL_TIMEOUT (falling back to
// httpclient's defaults, with a warning logged via tflog on an invalid
// override).
func newStallAwareHTTPClient(ctx context.Context) *http.Client {
	logf := func(format string, args ...any) { tflog.Warn(ctx, fmt.Sprintf(format, args...)) }
	return httpclient.NewClient(httpclient.Config{
		HeaderTimeout: httpclient.ResolveHeaderTimeout(logf),
		StallTimeout:  httpclient.ResolveStallTimeout(logf),
	})
}

// buildSDKClientConfig assembles the shared fields of an sdkclient.Config for
// either credential set (authMethod "basic" or "oauth2"); the caller sets the
// method-specific credential fields (Username/Password or
// ClientID/ClientSecret/OAuth2TokenURL) on the returned value before calling
// sdkclient.NewClient. HttpClient is always wired through as HTTPClient
// (B10), together with sdkHTTPClientTimeout — factored out from Configure so
// a unit test can assert directly that a given *http.Client ends up on
// Config.HTTPClient without needing to reach into the SDK's own (unexported)
// client internals.
func buildSDKClientConfig(instanceURL, tenantCode, authMethod string, httpClient *http.Client) *sdkclient.Config {
	return &sdkclient.Config{
		InstanceURL: instanceURL,
		TenantCode:  tenantCode,
		AuthMethod:  authMethod,
		HTTPClient:  httpClient,
		Timeout:     sdkHTTPClientTimeout,
	}
}

func (p *WorkspaceOneProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data WorkspaceOneProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Get configuration values from provider config or environment variables
	instanceURL := getConfigValue(data.InstanceURL, "UEM_INSTANCE_URL")
	tenantCode := getConfigValue(data.TenantCode, "UEM_TENANT_CODE")
	authMethod := getConfigValue(data.AuthMethod, "UEM_AUTH_METHOD")
	username := getConfigValue(data.Username, "UEM_USERNAME")
	password := getConfigValue(data.Password, "UEM_PASSWORD")
	clientID := getConfigValue(data.ClientID, "UEM_CLIENT_ID")
	clientSecret := getConfigValue(data.ClientSecret, "UEM_CLIENT_SECRET")
	oauth2TokenURL := getConfigValue(data.OAuth2TokenURL, "UEM_OAUTH2_TOKEN_URL")
	appBinaryStoragePath := getConfigValue(data.AppBinaryStoragePath, "UEM_APP_BINARY_DIR")
	if appBinaryStoragePath == "" {
		// F10: the default lives in the repo, next to the Terraform working
		// directory (never a hidden folder under the user's home
		// directory). It is left relative on purpose — no filepath.Abs, no
		// os.UserHomeDir — so it resolves against whatever process cwd is in
		// effect at apply/import time (for CLI-driven usage, that's always
		// the Terraform root module dir; for a hand-written config run
		// directly, it's wherever `terraform` was invoked from).
		appBinaryStoragePath = filepath.Join("uem-artifacts", "app-binaries")
	}

	// Validate required fields
	if instanceURL == "" {
		resp.Diagnostics.AddError(
			"Missing Instance URL",
			"The provider requires an instance_url to be configured. "+
				"Set the instance_url attribute in the provider configuration or the UEM_INSTANCE_URL environment variable.",
		)
	} else if summary, detail := validateInstanceURLScheme(instanceURL); summary != "" {
		resp.Diagnostics.AddError(summary, detail)
	}

	if tenantCode == "" {
		resp.Diagnostics.AddError(
			"Missing Tenant Code",
			"The provider requires a tenant_code to be configured. "+
				"Set the tenant_code attribute in the provider configuration or the UEM_TENANT_CODE environment variable.",
		)
	}

	// Determine which credential sets are present, independently — a consumer
	// may supply both, since different UEM endpoints require different auth
	// methods (per-endpoint routing is a later change, gated on the SDK
	// auth-capability artifact; this validation only ensures at least one
	// usable set exists).
	hasBasicCreds := username != "" && password != ""
	hasOAuth2Creds := clientID != "" && clientSecret != ""

	// Resolve auth_method. An explicit value (config or UEM_AUTH_METHOD env)
	// always wins. When unset, infer from whichever credential set(s) are
	// complete: oauth2 is preferred when both are complete; otherwise
	// whichever single set is complete is used. When neither is complete,
	// authMethod is left empty — the missing-credentials error below fires
	// and the auth_method format check is skipped, since there is no
	// inferred value to validate.
	explicitAuthMethod := authMethod != ""
	if !explicitAuthMethod {
		switch {
		case hasOAuth2Creds:
			authMethod = "oauth2"
		case hasBasicCreds:
			authMethod = "basic"
		}
	}

	if !hasBasicCreds && !hasOAuth2Creds {
		resp.Diagnostics.AddError(
			"Missing Credentials",
			"The provider requires at least one complete credential set: "+
				"username+password (basic) or client_id+client_secret (oauth2). "+
				"Set them via the provider configuration (or the corresponding UEM_* environment variables): "+
				"either username and password for basic authentication, or client_id and client_secret for oauth2 authentication.",
		)
	}

	if oauth2TokenURL == "" {
		oauth2TokenURL = "https://na.uemauth.workspaceone.com/connect/token"
	}

	if authMethod != "" && authMethod != "basic" && authMethod != "oauth2" {
		resp.Diagnostics.AddError(
			"Invalid Auth Method",
			"auth_method must be either 'basic' or 'oauth2'. Got: "+authMethod,
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Build a client per complete credential set present. Up to two clients
	// are constructed; the primary serves every resource/data source call,
	// and a complete secondary is exposed inertly on
	// ProviderData.SecondaryClient for future per-endpoint routing.
	//
	// B10: every client gets its own stall-aware *http.Client (see
	// newStallAwareHTTPClient) rather than letting the SDK build its
	// default one. Without this, the pinned SDK's client/client.go applies
	// a flat 30s http.Client.Timeout over the WHOLE request (headers and
	// body), which truncates any uem_mac_application ImportState blob
	// download that legitimately takes longer than 30s to transfer. A
	// separate *http.Client per credential set (rather than one shared
	// pointer) avoids the two configs' Config.Timeout-driven mutation of
	// retryClient.HTTPClient.Timeout (see sdkHTTPClientTimeout below)
	// stepping on each other.
	var basicClient, oauth2Client *sdkclient.Client

	if hasBasicCreds {
		cfg := buildSDKClientConfig(instanceURL, tenantCode, "basic", newStallAwareHTTPClient(ctx))
		cfg.Username = username
		cfg.Password = password
		c, err := sdkclient.NewClient(cfg)
		if err != nil {
			resp.Diagnostics.AddError("Unable to Create Workspace ONE Client (basic)", "Error: "+err.Error())
			return
		}
		basicClient = c
	}

	if hasOAuth2Creds {
		cfg := buildSDKClientConfig(instanceURL, tenantCode, "oauth2", newStallAwareHTTPClient(ctx))
		cfg.ClientID = clientID
		cfg.ClientSecret = clientSecret
		cfg.OAuth2TokenURL = oauth2TokenURL
		c, err := sdkclient.NewClient(cfg)
		if err != nil {
			resp.Diagnostics.AddError("Unable to Create Workspace ONE Client (oauth2)", "Error: "+err.Error())
			return
		}
		oauth2Client = c
	}

	// #3/v1n.2: authMethod is now either explicit (config or
	// UEM_AUTH_METHOD env) or inferred above from whichever credential
	// set(s) are complete (oauth2 preferred when both are complete). Either
	// way it is treated as authoritative here: it alone selects which
	// client is primary, and a required-but-missing credential set for that
	// method is a hard error, not a silent fallback to the other method.
	var wsoneClient *sdkclient.Client
	selectedAuthMethod := authMethod
	switch authMethod {
	case "basic":
		if !hasBasicCreds {
			resp.Diagnostics.AddError(
				"Missing Credentials for Selected auth_method",
				"auth_method is 'basic' but username+password were not both provided. "+
					"Set username and password (or UEM_USERNAME/UEM_PASSWORD), or change auth_method to 'oauth2'.",
			)
			return
		}
		wsoneClient = basicClient
	case "oauth2":
		if !hasOAuth2Creds {
			resp.Diagnostics.AddError(
				"Missing Credentials for Selected auth_method",
				"auth_method is 'oauth2' but client_id+client_secret were not both provided. "+
					"Set client_id and client_secret (or UEM_CLIENT_ID/UEM_CLIENT_SECRET), or change auth_method to 'basic'.",
			)
			return
		}
		wsoneClient = oauth2Client
	}

	// When both credential sets are complete, keep the non-primary client
	// around as the secondary (inert for now; a future SDK-marked endpoint
	// will consume it for per-endpoint auth routing). No warning here — both
	// credential sets configured simultaneously is expected/supported, not
	// a misconfiguration.
	var secondaryClient *sdkclient.Client
	secondaryAuthMethod := ""
	if basicClient != nil && oauth2Client != nil {
		if wsoneClient == basicClient {
			secondaryClient = oauth2Client
			secondaryAuthMethod = "oauth2"
		} else {
			secondaryClient = basicClient
			secondaryAuthMethod = "basic"
		}
	}

	// Silent (log-only) record of the resolved auth method for operators
	// debugging via TF_LOG, never a diagnostic warning — this is expected,
	// not exceptional, behavior. Never logs credential values.
	explicitness := "inferred from configured credentials"
	if explicitAuthMethod {
		explicitness = "explicitly configured"
	}
	secondaryNote := "no secondary credential set configured"
	if secondaryClient != nil {
		secondaryNote = fmt.Sprintf("%s credentials also configured as secondary (not verified; unused)", secondaryAuthMethod)
	}
	tflog.Info(ctx, fmt.Sprintf(
		"uem provider: using %s authentication (%s; %s)",
		selectedAuthMethod, explicitness, secondaryNote,
	))

	// Fail fast: verify the selected client can actually authenticate before
	// handing it to resources/data sources. Without this, a bad credential set
	// that passes field-presence validation above would surface as a
	// mysterious failure on the first resource operation instead of here.
	//
	// #4: distinguish "credentials are invalid" from "credentials are valid
	// but this specific probe call was denied by RBAC". A 401 (or any error
	// that isn't a recognized *sdkclient.APIError, e.g. network/5xx/timeout)
	// means we can't trust the credentials at all, so it's a hard error. A
	// 403 means authentication itself succeeded — UEM rejected the request
	// for permissions, not credentials — so it's a warning only and the
	// provider still configures.
	if err := verifyClientAuth(ctx, wsoneClient); err != nil {
		var apiErr *sdkclient.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusForbidden {
			resp.Diagnostics.AddWarning(
				"Auth-Verify Probe Denied by Permissions",
				fmt.Sprintf(
					"The provider authenticated successfully with the %q client, but the verification probe "+
						"(ProfilesV2Service.SearchProfiles) was denied by RBAC (403): %s. The provider will "+
						"proceed, but confirm your credentials have the permissions your configuration needs.",
					selectedAuthMethod, err,
				),
			)
		} else {
			resp.Diagnostics.AddError(
				"Unable to Authenticate",
				fmt.Sprintf(
					"The provider built a Workspace ONE UEM client (%q) but could not authenticate with it: %s",
					selectedAuthMethod, err,
				),
			)
			return
		}
	}

	pd := &providerdata.ProviderData{
		Client:               wsoneClient,
		SecondaryClient:      secondaryClient,
		AppBinaryStoragePath: appBinaryStoragePath,
	}
	resp.DataSourceData = pd
	resp.ResourceData = pd
}

// verifyClientAuth performs a cheap, side-effect-free authenticated call to
// confirm the configured credentials actually work. It is a package-level
// var so tests can substitute a stub without making a real network call;
// production code always runs defaultVerifyClientAuth.
var verifyClientAuth = defaultVerifyClientAuth

// defaultVerifyClientAuth calls ProfilesV2Service.SearchProfiles with a
// page size of 1 as the connectivity/auth check. This is not a dedicated
// ping/whoami endpoint — the SDK has none — but it is the cheapest read-only,
// side-effect-free GET already proven to work against this SDK (the same
// call shape used by internal/profile's uem_profiles data source), so it
// requires no new API surface to trust.
func defaultVerifyClientAuth(ctx context.Context, client *sdkclient.Client) error {
	pageSize := 1
	svc := sdk.NewProfilesV2Service(client)
	_, _, err := svc.SearchProfiles(ctx, &sdk.ProfilesV2SearchProfilesOptions{PageSize: &pageSize})
	if err != nil {
		return err
	}
	return nil
}

// validateInstanceURLScheme fail-closed enforces the same underlying HTTPS
// policy as the CLI's uemapi.enforceHTTPS (cli/internal/uemapi/client.go):
// an explicit http:// scheme is rejected unless the host is localhost or
// 127.0.0.1 (dev/loopback use, e.g. httptest.Server-based tests), since
// Basic/Bearer credentials must never travel in the clear to a real UEM
// tenant. The provider is a separate Go module from the CLI, so this ports
// the policy rather than importing across module boundaries. It diverges
// from the CLI on one point: the CLI defaults a schemeless URL to https://
// before validating, but a provider enforcing security should reject rather
// than silently upgrade, so a schemeless instance_url is a hard error here.
//
// Returns ("", "") when instanceURL is acceptable. Otherwise returns a
// diagnostic (summary, detail) pair — "Invalid Instance URL" for a URL that
// fails to parse at all, "Insecure Instance URL" for one that parses but
// violates the HTTPS policy — so callers report each as what it actually is
// rather than reusing an insecurity summary for a syntax error.
func validateInstanceURLScheme(instanceURL string) (summary string, detail string) {
	parsed, err := url.Parse(instanceURL)
	if err != nil {
		return "Invalid Instance URL", fmt.Sprintf("instance_url %q could not be parsed: %s", instanceURL, err)
	}
	if parsed.Scheme == "" {
		return "Insecure Instance URL", fmt.Sprintf(
			"instance_url %q has no scheme; Workspace ONE UEM tenants must be accessed over an explicit https://", instanceURL,
		)
	}
	if parsed.Scheme != "http" {
		return "", ""
	}
	host := parsed.Hostname()
	if host == "localhost" || host == "127.0.0.1" {
		return "", ""
	}
	return "Insecure Instance URL", fmt.Sprintf(
		"instance_url %q uses plain http; Workspace ONE UEM tenants must be accessed over https "+
			"(plain http is only permitted for localhost/127.0.0.1)", instanceURL,
	)
}

// getConfigValue returns the value from the config if set, otherwise from the environment variable.
func getConfigValue(configValue types.String, envVar string) string {
	if !configValue.IsNull() && !configValue.IsUnknown() {
		return configValue.ValueString()
	}
	return os.Getenv(envVar)
}

func (p *WorkspaceOneProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		profileresource.NewResource,
		assignment.NewResource,
		purchasedappassignment.NewResource,
		macapplication.NewResource,
		macscript.NewResource,
		scriptassignment.NewResource,
		macsensor.NewResource,
		sensorassignment.NewResource,
		updatedeployment.NewResource,
		smartgroup.NewResource,
	}
}

func (p *WorkspaceOneProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		smartgroup.NewDataSource,
		profileresource.NewDataSource,
		applicationdatasource.NewDataSource,
		scriptsdatasource.NewScriptDataSource,
		sensorsdatasource.NewSensorDataSource,
		updatesdatasource.NewUpdateDataSource,
		applicationdatasource.NewMacApplicationDataSource,
		purchasedappdatasource.NewPurchasedApplicationsDataSource,
		organizationgroupdatasource.NewDataSource,
	}
}

func (p *WorkspaceOneProvider) Functions(ctx context.Context) []func() function.Function {
	return []func() function.Function{
		// Functions will be added here if needed
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &WorkspaceOneProvider{
			version: version,
		}
	}
}
