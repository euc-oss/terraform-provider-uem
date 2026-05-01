package provider

import (
	"context"
	"os"

	smartgroupdatasource "github.com/euc-oss/terraform-provider-uem/internal/smartgroup"
	sdk "github.com/euc-oss/terraform-sdk-uem"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/euc-oss/terraform-provider-uem/internal/application/assignment"
	macapplication "github.com/euc-oss/terraform-provider-uem/internal/application/mac"
	profileresource "github.com/euc-oss/terraform-provider-uem/internal/profile"
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
	InstanceURL    types.String `tfsdk:"instance_url"`
	TenantCode     types.String `tfsdk:"tenant_code"`
	AuthMethod     types.String `tfsdk:"auth_method"`
	Username       types.String `tfsdk:"username"`
	Password       types.String `tfsdk:"password"`
	ClientID       types.String `tfsdk:"client_id"`
	ClientSecret   types.String `tfsdk:"client_secret"`
	OAuth2TokenURL types.String `tfsdk:"oauth2_token_url"`
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
				MarkdownDescription: "Authentication method: 'basic' or 'oauth2'. Defaults to 'basic'. Can also be set via UEM_AUTH_METHOD environment variable.",
				Optional:            true,
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
		},
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

	// Validate required fields
	if instanceURL == "" {
		resp.Diagnostics.AddError(
			"Missing Instance URL",
			"The provider requires an instance_url to be configured. "+
				"Set the instance_url attribute in the provider configuration or the UEM_INSTANCE_URL environment variable.",
		)
	}

	if tenantCode == "" {
		resp.Diagnostics.AddError(
			"Missing Tenant Code",
			"The provider requires a tenant_code to be configured. "+
				"Set the tenant_code attribute in the provider configuration or the UEM_TENANT_CODE environment variable.",
		)
	}

	// Default auth method to basic
	if authMethod == "" {
		authMethod = "basic"
	}

	// Validate auth method and required credentials
	switch authMethod {
	case "basic":
		if username == "" || password == "" {
			resp.Diagnostics.AddError(
				"Missing Basic Auth Credentials",
				"When using basic authentication, both username and password must be configured. "+
					"Set the username and password attributes in the provider configuration or the UEM_USERNAME and UEM_PASSWORD environment variables.",
			)
		}
	case "oauth2":
		if clientID == "" || clientSecret == "" {
			resp.Diagnostics.AddError(
				"Missing OAuth2 Credentials",
				"When using oauth2 authentication, both client_id and client_secret must be configured. "+
					"Set the client_id and client_secret attributes in the provider configuration or the UEM_CLIENT_ID and UEM_CLIENT_SECRET environment variables.",
			)
		}
		// Default OAuth2 token URL if not provided
		if oauth2TokenURL == "" {
			oauth2TokenURL = "https://na.uemauth.workspaceone.com/connect/token"
		}
	default:
		resp.Diagnostics.AddError(
			"Invalid Auth Method",
			"The auth_method must be either 'basic' or 'oauth2'. Got: "+authMethod,
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Build the auth provider based on the configured auth method.
	var (
		auth    sdk.AuthProvider
		authErr error
	)
	switch authMethod {
	case "oauth2":
		auth, authErr = sdk.NewOAuth2Auth(sdk.OAuth2Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			TokenURL:     oauth2TokenURL,
		})
		if authErr != nil {
			resp.Diagnostics.AddError(
				"Unable to Configure OAuth2 Authentication",
				"An error occurred while configuring OAuth2 authentication for the Workspace ONE client.\n\n"+
					"Error: "+authErr.Error(),
			)
			return
		}
	default:
		auth = sdk.NewBasicAuth(username, password)
	}

	// Create SDK client configuration
	cfg := sdk.Config{
		BaseURL:    instanceURL,
		TenantCode: tenantCode,
		Auth:       auth,
	}

	// Create SDK client
	wsoneClient, err := sdk.NewClient(cfg)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create Workspace ONE Client",
			"An unexpected error occurred when creating the Workspace ONE API client. "+
				"If the error is not clear, please contact the provider developers.\n\n"+
				"Error: "+err.Error(),
		)
		return
	}

	// Make configured SDK client available to resources.
	resp.DataSourceData = wsoneClient
	resp.ResourceData = wsoneClient
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
		macapplication.NewResource,
	}
}

func (p *WorkspaceOneProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		smartgroupdatasource.NewDataSource,
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
