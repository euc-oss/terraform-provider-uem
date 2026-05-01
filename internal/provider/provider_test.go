package provider

import (
	"context"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	providerSchema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// --- getConfigValue tests ---

func TestGetConfigValue_ConfigSet(t *testing.T) {
	t.Parallel()
	got := getConfigValue(types.StringValue("test-value"), "UNUSED_ENV_VAR_XYZ")
	if got != "test-value" {
		t.Errorf("expected 'test-value', got '%s'", got)
	}
}

func TestGetConfigValue_NullFallsBackToEnv(t *testing.T) {
	t.Setenv("TEST_PROVIDER_CFG_1", "env-value")
	got := getConfigValue(types.StringNull(), "TEST_PROVIDER_CFG_1")
	if got != "env-value" {
		t.Errorf("expected 'env-value', got '%s'", got)
	}
}

func TestGetConfigValue_UnknownFallsBackToEnv(t *testing.T) {
	t.Setenv("TEST_PROVIDER_CFG_2", "env-value-2")
	got := getConfigValue(types.StringUnknown(), "TEST_PROVIDER_CFG_2")
	if got != "env-value-2" {
		t.Errorf("expected 'env-value-2', got '%s'", got)
	}
}

func TestGetConfigValue_BothUnset(t *testing.T) {
	t.Parallel()
	got := getConfigValue(types.StringNull(), "NONEXISTENT_ENV_VAR_12345")
	if got != "" {
		t.Errorf("expected empty string, got '%s'", got)
	}
}

// --- New() tests ---

func TestNew(t *testing.T) {
	t.Parallel()
	factory := New("1.0.0")
	if factory == nil {
		t.Fatal("New() returned nil")
	}
	p := factory()
	if p == nil {
		t.Fatal("factory() returned nil provider")
	}
}

// --- Metadata() tests ---

func TestProviderMetadata(t *testing.T) {
	t.Parallel()
	p := &WorkspaceOneProvider{version: "1.2.3"}
	var resp provider.MetadataResponse
	p.Metadata(context.Background(), provider.MetadataRequest{}, &resp)

	if resp.TypeName != "uem" {
		t.Errorf("expected TypeName 'uem', got '%s'", resp.TypeName)
	}
	if resp.Version != "1.2.3" {
		t.Errorf("expected Version '1.2.3', got '%s'", resp.Version)
	}
}

// --- Schema() tests ---

func TestProviderSchema(t *testing.T) {
	t.Parallel()
	p := &WorkspaceOneProvider{version: "test"}
	var resp provider.SchemaResponse
	p.Schema(context.Background(), provider.SchemaRequest{}, &resp)

	s := resp.Schema

	// Verify all expected attributes exist.
	expectedAttrs := []string{
		"instance_url", "tenant_code", "auth_method",
		"username", "password",
		"client_id", "client_secret", "oauth2_token_url",
	}
	for _, attr := range expectedAttrs {
		if _, ok := s.Attributes[attr]; !ok {
			t.Errorf("missing expected attribute: %s", attr)
		}
	}

	// Verify sensitive attributes.
	sensitiveAttrs := []string{"tenant_code", "password", "client_secret"}
	for _, name := range sensitiveAttrs {
		attr, ok := s.Attributes[name]
		if !ok {
			continue
		}
		if sa, ok := attr.(providerSchema.StringAttribute); ok {
			if !sa.Sensitive {
				t.Errorf("expected attribute '%s' to be sensitive", name)
			}
		}
	}

	// Verify all attributes are optional.
	for name, attr := range s.Attributes {
		if sa, ok := attr.(providerSchema.StringAttribute); ok {
			if !sa.Optional {
				t.Errorf("expected attribute '%s' to be optional", name)
			}
		}
	}
}

// --- Resources(), DataSources(), Functions() tests ---

func TestProviderResources(t *testing.T) {
	t.Parallel()
	p := &WorkspaceOneProvider{version: "test"}
	resources := p.Resources(context.Background())
	if len(resources) != 3 {
		t.Errorf("expected 3 resources, got %d", len(resources))
	}
}

func TestProviderDataSources(t *testing.T) {
	t.Parallel()
	p := &WorkspaceOneProvider{version: "test"}
	dataSources := p.DataSources(context.Background())
	if len(dataSources) != 1 {
		t.Errorf("expected 1 data source, got %d", len(dataSources))
	}
}

func TestProviderFunctions(t *testing.T) {
	t.Parallel()
	p := &WorkspaceOneProvider{version: "test"}
	functions := p.Functions(context.Background())
	if len(functions) != 0 {
		t.Errorf("expected 0 functions, got %d", len(functions))
	}
}

// --- Configure() tests ---

func TestProviderConfigure(t *testing.T) {
	// Clear all provider env vars for all subtests to prevent interference.
	providerEnvVars := []string{
		"UEM_INSTANCE_URL", "UEM_TENANT_CODE", "UEM_AUTH_METHOD",
		"UEM_USERNAME", "UEM_PASSWORD",
		"UEM_CLIENT_ID", "UEM_CLIENT_SECRET", "UEM_OAUTH2_TOKEN_URL",
	}

	tests := []struct {
		name         string
		configValues map[string]tftypes.Value
		envVars      map[string]string
		expectError  bool
		errorSummary string
	}{
		{
			name:         "missing_instance_url",
			configValues: allNullProviderConfig(),
			expectError:  true,
			errorSummary: "Missing Instance URL",
		},
		{
			name: "missing_tenant_code",
			configValues: func() map[string]tftypes.Value {
				v := allNullProviderConfig()
				v["instance_url"] = stringVal("https://test.awmdm.com")
				return v
			}(),
			expectError:  true,
			errorSummary: "Missing Tenant Code",
		},
		{
			name: "basic_auth_missing_both_credentials",
			configValues: func() map[string]tftypes.Value {
				v := allNullProviderConfig()
				v["instance_url"] = stringVal("https://test.awmdm.com")
				v["tenant_code"] = stringVal("test-tenant")
				v["auth_method"] = stringVal("basic")
				return v
			}(),
			expectError:  true,
			errorSummary: "Missing Basic Auth Credentials",
		},
		{
			name: "basic_auth_missing_password",
			configValues: func() map[string]tftypes.Value {
				v := allNullProviderConfig()
				v["instance_url"] = stringVal("https://test.awmdm.com")
				v["tenant_code"] = stringVal("test-tenant")
				v["auth_method"] = stringVal("basic")
				v["username"] = stringVal("user")
				return v
			}(),
			expectError:  true,
			errorSummary: "Missing Basic Auth Credentials",
		},
		{
			name: "oauth2_missing_both_credentials",
			configValues: func() map[string]tftypes.Value {
				v := allNullProviderConfig()
				v["instance_url"] = stringVal("https://test.awmdm.com")
				v["tenant_code"] = stringVal("test-tenant")
				v["auth_method"] = stringVal("oauth2")
				return v
			}(),
			expectError:  true,
			errorSummary: "Missing OAuth2 Credentials",
		},
		{
			name: "oauth2_missing_client_secret",
			configValues: func() map[string]tftypes.Value {
				v := allNullProviderConfig()
				v["instance_url"] = stringVal("https://test.awmdm.com")
				v["tenant_code"] = stringVal("test-tenant")
				v["auth_method"] = stringVal("oauth2")
				v["client_id"] = stringVal("cid")
				return v
			}(),
			expectError:  true,
			errorSummary: "Missing OAuth2 Credentials",
		},
		{
			name: "invalid_auth_method",
			configValues: func() map[string]tftypes.Value {
				v := allNullProviderConfig()
				v["instance_url"] = stringVal("https://test.awmdm.com")
				v["tenant_code"] = stringVal("test-tenant")
				v["auth_method"] = stringVal("invalid")
				return v
			}(),
			expectError:  true,
			errorSummary: "Invalid Auth Method",
		},
		{
			name: "valid_basic_auth",
			configValues: func() map[string]tftypes.Value {
				v := allNullProviderConfig()
				v["instance_url"] = stringVal("https://test.awmdm.com")
				v["tenant_code"] = stringVal("test-tenant")
				v["auth_method"] = stringVal("basic")
				v["username"] = stringVal("user")
				v["password"] = stringVal("pass")
				return v
			}(),
			expectError: false,
		},
		{
			name: "valid_oauth2",
			configValues: func() map[string]tftypes.Value {
				v := allNullProviderConfig()
				v["instance_url"] = stringVal("https://test.awmdm.com")
				v["tenant_code"] = stringVal("test-tenant")
				v["auth_method"] = stringVal("oauth2")
				v["client_id"] = stringVal("cid")
				v["client_secret"] = stringVal("csecret")
				v["oauth2_token_url"] = stringVal("https://auth.example.com/token")
				return v
			}(),
			expectError: false,
		},
		{
			name: "default_auth_method_to_basic",
			configValues: func() map[string]tftypes.Value {
				v := allNullProviderConfig()
				v["instance_url"] = stringVal("https://test.awmdm.com")
				v["tenant_code"] = stringVal("test-tenant")
				// auth_method is null -> defaults to "basic"
				v["username"] = stringVal("user")
				v["password"] = stringVal("pass")
				return v
			}(),
			expectError: false,
		},
		{
			name:         "env_vars_fallback",
			configValues: allNullProviderConfig(),
			envVars: map[string]string{
				"UEM_INSTANCE_URL": "https://test.awmdm.com",
				"UEM_TENANT_CODE":  "test-tenant",
				"UEM_AUTH_METHOD":  "basic",
				"UEM_USERNAME":     "user",
				"UEM_PASSWORD":     "pass",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all provider env vars.
			for _, envVar := range providerEnvVars {
				t.Setenv(envVar, "")
			}
			// Set test-specific env vars.
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			p := &WorkspaceOneProvider{version: "test"}
			ctx := context.Background()

			config := createProviderConfig(t, tt.configValues)
			req := provider.ConfigureRequest{Config: config}
			var resp provider.ConfigureResponse

			p.Configure(ctx, req, &resp)

			if tt.expectError {
				if !resp.Diagnostics.HasError() {
					t.Fatal("expected error but got none")
				}
				found := false
				for _, d := range resp.Diagnostics.Errors() {
					if d.Summary() == tt.errorSummary {
						found = true
						break
					}
				}
				if !found {
					var summaries []string
					for _, d := range resp.Diagnostics.Errors() {
						summaries = append(summaries, d.Summary())
					}
					t.Errorf("expected error summary '%s', got: %v", tt.errorSummary, summaries)
				}
			} else {
				if resp.Diagnostics.HasError() {
					var msgs []string
					for _, d := range resp.Diagnostics.Errors() {
						msgs = append(msgs, d.Summary()+": "+d.Detail())
					}
					t.Fatalf("unexpected errors: %v", msgs)
				}
				if resp.ResourceData == nil {
					t.Error("expected ResourceData to be set")
				}
				if resp.DataSourceData == nil {
					t.Error("expected DataSourceData to be set")
				}
				if _, ok := resp.ResourceData.(*sdk.Client); !ok {
					t.Errorf("expected ResourceData type *sdk.Client, got %T", resp.ResourceData)
				}
			}
		})
	}
}
