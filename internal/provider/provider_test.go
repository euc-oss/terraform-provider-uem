package provider

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/euc-oss/terraform-provider-uem/internal/httpclient"
	"github.com/euc-oss/terraform-provider-uem/internal/providerdata"
	sdkclient "github.com/euc-oss/terraform-sdk-uem/v26/client"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	providerSchema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-log/tflogtest"
)

// clientAuthMethod reaches into *sdkclient.Client's unexported "config"
// field via reflect+unsafe to read back the AuthMethod ("basic"/"oauth2")
// the client was built with. The sdkclient.Client type exposes no public accessor for
// this, but it's the only reliable way to assert which credential path a
// given *sdkclient.Client actually came from (distinct from merely comparing
// pointer identity, which can't tell basic from oauth2).
func clientAuthMethod(t *testing.T, c *sdkclient.Client) string {
	t.Helper()
	if c == nil {
		t.Fatal("clientAuthMethod: nil client")
	}
	v := reflect.ValueOf(c).Elem().FieldByName("config")
	v = reflect.NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr())).Elem()
	cfg, ok := v.Interface().(*sdkclient.Config)
	if !ok || cfg == nil {
		t.Fatalf("clientAuthMethod: unexpected config field type/value: %#v", v.Interface())
	}
	return cfg.AuthMethod
}

// clientHTTPClientConfig is clientAuthMethod's sibling: it reaches into the
// same unexported "config" field to read back the whole *sdkclient.Config a
// *sdkclient.Client was built with, so a B10 test can assert on
// Config.HTTPClient/Config.Timeout — fields Configure() sets but that
// sdkclient.Client exposes no public accessor for.
func clientHTTPClientConfig(t *testing.T, c *sdkclient.Client) *sdkclient.Config {
	t.Helper()
	if c == nil {
		t.Fatal("clientHTTPClientConfig: nil client")
	}
	v := reflect.ValueOf(c).Elem().FieldByName("config")
	v = reflect.NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr())).Elem()
	cfg, ok := v.Interface().(*sdkclient.Config)
	if !ok || cfg == nil {
		t.Fatalf("clientHTTPClientConfig: unexpected config field type/value: %#v", v.Interface())
	}
	return cfg
}

// TestMain stubs out the real auth-verification SDK call for the whole test
// binary, so every pre-existing Configure() test (which builds a real
// *sdkclient.Client against a fake instance_url) keeps passing without
// making a network call. Tests that care about the verification behavior
// itself override the var locally and restore it via t.Cleanup.
func TestMain(m *testing.M) {
	verifyClientAuth = func(_ context.Context, _ *sdkclient.Client) error { return nil }
	os.Exit(m.Run())
}

// providerUEMEnvVars is every UEM_* env var Configure()'s env fallback
// reads (internal/provider/provider.go's getConfigValue call sites),
// enumerated from source rather than hardcoded from memory so this list
// can't silently drift out of sync with provider.go.
var providerUEMEnvVars = []string{
	"UEM_INSTANCE_URL", "UEM_TENANT_CODE", "UEM_AUTH_METHOD",
	"UEM_USERNAME", "UEM_PASSWORD",
	"UEM_CLIENT_ID", "UEM_CLIENT_SECRET", "UEM_OAUTH2_TOKEN_URL",
	"UEM_APP_BINARY_DIR",
}

// clearProviderEnvVars isolates a Configure() test from whatever ambient
// UEM_* env vars the shell happens to export (e.g. fleet-shell
// pre-prod-demo creds — see internal-ticket/52y.9). Calling t.Setenv(var, "") is
// equivalent to unset for getConfigValue's purposes: os.Getenv returns ""
// for both an empty-string and an absent var, and every Configure()
// missing-credential/missing-config check treats "" as not-set.
func clearProviderEnvVars(t *testing.T) {
	t.Helper()
	for _, envVar := range providerUEMEnvVars {
		t.Setenv(envVar, "")
	}
}

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
	if len(resources) != 10 {
		t.Errorf("expected 10 resources, got %d", len(resources))
	}
}

func TestProviderDataSources(t *testing.T) {
	t.Parallel()
	p := &WorkspaceOneProvider{version: "test"}
	dataSources := p.DataSources(context.Background())
	if len(dataSources) != 9 {
		t.Errorf("expected 9 data sources, got %d", len(dataSources))
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
			errorSummary: "Missing Credentials",
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
			errorSummary: "Missing Credentials",
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
			errorSummary: "Missing Credentials",
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
			errorSummary: "Missing Credentials",
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
		{
			name: "instance_url_http_non_localhost_rejected",
			configValues: func() map[string]tftypes.Value {
				v := allNullProviderConfig()
				v["instance_url"] = stringVal("http://test.awmdm.com")
				v["tenant_code"] = stringVal("test-tenant")
				v["auth_method"] = stringVal("basic")
				v["username"] = stringVal("user")
				v["password"] = stringVal("pass")
				return v
			}(),
			expectError:  true,
			errorSummary: "Insecure Instance URL",
		},
		{
			name: "instance_url_http_localhost_allowed",
			configValues: func() map[string]tftypes.Value {
				v := allNullProviderConfig()
				v["instance_url"] = stringVal("http://localhost:8080")
				v["tenant_code"] = stringVal("test-tenant")
				v["auth_method"] = stringVal("basic")
				v["username"] = stringVal("user")
				v["password"] = stringVal("pass")
				return v
			}(),
			expectError: false,
		},
		{
			name: "instance_url_http_loopback_ip_allowed",
			configValues: func() map[string]tftypes.Value {
				v := allNullProviderConfig()
				v["instance_url"] = stringVal("http://127.0.0.1:8080")
				v["tenant_code"] = stringVal("test-tenant")
				v["auth_method"] = stringVal("basic")
				v["username"] = stringVal("user")
				v["password"] = stringVal("pass")
				return v
			}(),
			expectError: false,
		},
		{
			name: "instance_url_https_non_localhost_allowed",
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
			// I1: a schemeless instance_url must be rejected (fail-closed),
			// not silently upgraded to https:// or silently accepted.
			name: "instance_url_schemeless_rejected",
			configValues: func() map[string]tftypes.Value {
				v := allNullProviderConfig()
				v["instance_url"] = stringVal("test.awmdm.com")
				v["tenant_code"] = stringVal("test-tenant")
				v["auth_method"] = stringVal("basic")
				v["username"] = stringVal("user")
				v["password"] = stringVal("pass")
				return v
			}(),
			expectError:  true,
			errorSummary: "Insecure Instance URL",
		},
		{
			// M3: IPv6 loopback is NOT exempted, matching the CLI reference
			// (which only exempts the literal "localhost"/"127.0.0.1").
			name: "instance_url_http_ipv6_loopback_not_exempted",
			configValues: func() map[string]tftypes.Value {
				v := allNullProviderConfig()
				v["instance_url"] = stringVal("http://[::1]:8080")
				v["tenant_code"] = stringVal("test-tenant")
				v["auth_method"] = stringVal("basic")
				v["username"] = stringVal("user")
				v["password"] = stringVal("pass")
				return v
			}(),
			expectError:  true,
			errorSummary: "Insecure Instance URL",
		},
		{
			// M3: an uppercase scheme must still be caught by the scheme
			// comparison, guarding against a future switch to naive
			// string-prefix matching that would miss this.
			name: "instance_url_uppercase_http_scheme_rejected",
			configValues: func() map[string]tftypes.Value {
				v := allNullProviderConfig()
				v["instance_url"] = stringVal("HTTP://test.awmdm.com")
				v["tenant_code"] = stringVal("test-tenant")
				v["auth_method"] = stringVal("basic")
				v["username"] = stringVal("user")
				v["password"] = stringVal("pass")
				return v
			}(),
			expectError:  true,
			errorSummary: "Insecure Instance URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearProviderEnvVars(t)
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
				pd, ok := resp.ResourceData.(*providerdata.ProviderData)
				if !ok {
					t.Errorf("expected ResourceData type *providerdata.ProviderData, got %T", resp.ResourceData)
				} else if pd.Client == nil {
					t.Error("expected ProviderData.Client to be non-nil")
				}
			}
		})
	}
}

// --- Additive credential validation tests (auth_method is no longer exclusive) ---

// TestConfigure_OAuth2CredsWithBasicAuthMethod_ErrorsMissingBasicCreds covers
// #3: auth_method is now authoritative for client selection. Presence of a
// complete oauth2 credential set no longer satisfies an explicit (or
// defaulted) auth_method = "basic" — the basic credential set itself must
// be present, or Configure hard-errors rather than silently switching to
// oauth2.
func TestConfigure_OAuth2CredsWithBasicAuthMethod_ErrorsMissingBasicCreds(t *testing.T) {
	clearProviderEnvVars(t)
	p := &WorkspaceOneProvider{}
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)
	configVal := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
		"instance_url":            tftypes.NewValue(tftypes.String, "https://example.awmdm.com"),
		"tenant_code":             tftypes.NewValue(tftypes.String, "tenant123"),
		"auth_method":             tftypes.NewValue(tftypes.String, "basic"),
		"username":                tftypes.NewValue(tftypes.String, nil),
		"password":                tftypes.NewValue(tftypes.String, nil),
		"client_id":               tftypes.NewValue(tftypes.String, "client1"),
		"client_secret":           tftypes.NewValue(tftypes.String, "secret1"),
		"oauth2_token_url":        tftypes.NewValue(tftypes.String, nil),
		"app_binary_storage_path": tftypes.NewValue(tftypes.String, nil),
	})
	req := provider.ConfigureRequest{Config: tfsdk.Config{Raw: configVal, Schema: schemaResp.Schema}}
	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error: auth_method=basic requires basic creds even when oauth2 creds are present")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if d.Summary() == "Missing Credentials for Selected auth_method" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'Missing Credentials for Selected auth_method' error summary, got: %v", resp.Diagnostics.Errors())
	}
}

// TestConfigure_BothBasicAndOAuth2Creds_NoError covers v1n.2 (c): both
// credential sets configured simultaneously (with auth_method explicitly set
// to "basic" here) must NOT produce the old "Multiple Credential Sets
// Configured" warning — dual credentials are expected/supported now, not a
// misconfiguration to flag. This replaces the pre-v1n.2 assertion that such a
// warning WAS present.
func TestConfigure_BothBasicAndOAuth2Creds_NoError(t *testing.T) {
	clearProviderEnvVars(t)
	p := &WorkspaceOneProvider{}
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)
	configVal := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
		"instance_url":            tftypes.NewValue(tftypes.String, "https://example.awmdm.com"),
		"tenant_code":             tftypes.NewValue(tftypes.String, "tenant123"),
		"auth_method":             tftypes.NewValue(tftypes.String, "basic"),
		"username":                tftypes.NewValue(tftypes.String, "user1"),
		"password":                tftypes.NewValue(tftypes.String, "pass1"),
		"client_id":               tftypes.NewValue(tftypes.String, "client1"),
		"client_secret":           tftypes.NewValue(tftypes.String, "secret1"),
		"oauth2_token_url":        tftypes.NewValue(tftypes.String, nil),
		"app_binary_storage_path": tftypes.NewValue(tftypes.String, nil),
	})
	req := provider.ConfigureRequest{Config: tfsdk.Config{Raw: configVal, Schema: schemaResp.Schema}}
	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error: both basic and oauth2 creds present, got: %v", resp.Diagnostics.Errors())
	}
	for _, w := range resp.Diagnostics.Warnings() {
		if w.Summary() == "Multiple Credential Sets Configured" {
			t.Fatalf("did not expect 'Multiple Credential Sets Configured' warning (removed in v1n.2), got: %v", resp.Diagnostics.Warnings())
		}
	}
}

// TestConfigure_AuthMethodUnset_OAuth2OnlyCreds_ResolvesOAuth2 is the
// regression test for the v1n.2 bug: auth_method unset used to
// unconditionally default to "basic" (provider.go's old lines 173-176),
// which meant oauth2-only credentials with no auth_method set would hard-fail
// with "Missing Credentials for Selected auth_method" instead of resolving to
// oauth2. This test fails under that old unconditional-default code and
// passes under the new inference logic.
func TestConfigure_AuthMethodUnset_OAuth2OnlyCreds_ResolvesOAuth2(t *testing.T) {
	clearProviderEnvVars(t)
	p := &WorkspaceOneProvider{}
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)
	configVal := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
		"instance_url":            tftypes.NewValue(tftypes.String, "https://example.awmdm.com"),
		"tenant_code":             tftypes.NewValue(tftypes.String, "tenant123"),
		"auth_method":             tftypes.NewValue(tftypes.String, nil),
		"username":                tftypes.NewValue(tftypes.String, nil),
		"password":                tftypes.NewValue(tftypes.String, nil),
		"client_id":               tftypes.NewValue(tftypes.String, "client1"),
		"client_secret":           tftypes.NewValue(tftypes.String, "secret1"),
		"oauth2_token_url":        tftypes.NewValue(tftypes.String, nil),
		"app_binary_storage_path": tftypes.NewValue(tftypes.String, nil),
	})
	req := provider.ConfigureRequest{Config: tfsdk.Config{Raw: configVal, Schema: schemaResp.Schema}}
	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error: auth_method unset with only oauth2 creds should resolve to oauth2, got: %v", resp.Diagnostics.Errors())
	}
	pd, ok := resp.ResourceData.(*providerdata.ProviderData)
	if !ok || pd.Client == nil {
		t.Fatalf("expected ResourceData to be *providerdata.ProviderData with a non-nil Client, got: %#v", resp.ResourceData)
	}
	if got := clientAuthMethod(t, pd.Client); got != "oauth2" {
		t.Errorf("expected resolved auth method 'oauth2', got %q", got)
	}
	if pd.SecondaryClient != nil {
		t.Errorf("expected no secondary client when only oauth2 creds are configured, got: %#v", pd.SecondaryClient)
	}
}

// TestConfigure_AuthMethodUnset_BasicOnlyCreds_ResolvesBasic covers the
// unset+basic-only-creds branch of the inference logic.
func TestConfigure_AuthMethodUnset_BasicOnlyCreds_ResolvesBasic(t *testing.T) {
	clearProviderEnvVars(t)
	p := &WorkspaceOneProvider{}
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)
	configVal := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
		"instance_url":            tftypes.NewValue(tftypes.String, "https://example.awmdm.com"),
		"tenant_code":             tftypes.NewValue(tftypes.String, "tenant123"),
		"auth_method":             tftypes.NewValue(tftypes.String, nil),
		"username":                tftypes.NewValue(tftypes.String, "user1"),
		"password":                tftypes.NewValue(tftypes.String, "pass1"),
		"client_id":               tftypes.NewValue(tftypes.String, nil),
		"client_secret":           tftypes.NewValue(tftypes.String, nil),
		"oauth2_token_url":        tftypes.NewValue(tftypes.String, nil),
		"app_binary_storage_path": tftypes.NewValue(tftypes.String, nil),
	})
	req := provider.ConfigureRequest{Config: tfsdk.Config{Raw: configVal, Schema: schemaResp.Schema}}
	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error: auth_method unset with only basic creds should resolve to basic, got: %v", resp.Diagnostics.Errors())
	}
	pd, ok := resp.ResourceData.(*providerdata.ProviderData)
	if !ok || pd.Client == nil {
		t.Fatalf("expected ResourceData to be *providerdata.ProviderData with a non-nil Client, got: %#v", resp.ResourceData)
	}
	if got := clientAuthMethod(t, pd.Client); got != "basic" {
		t.Errorf("expected resolved auth method 'basic', got %q", got)
	}
	if pd.SecondaryClient != nil {
		t.Errorf("expected no secondary client when only basic creds are configured, got: %#v", pd.SecondaryClient)
	}
}

// TestConfigure_AuthMethodUnset_BothCredsComplete_PrefersOAuth2AsPrimary
// covers v1n.2 (a)/(b)/(c): when auth_method is unset and both credential
// sets are complete, oauth2 must be primary (basic becomes the secondary),
// and no "Multiple Credential Sets Configured" warning fires. This fails if
// reverted to basic-primary (secondary would come back nil, since basic
// would be primary and nothing would distinguish it — instead this asserts
// SecondaryClient is non-nil AND distinct from Client, which only holds if
// oauth2 is primary given the old code always defaulted to basic first) and
// fails if the old warning is restored.
func TestConfigure_AuthMethodUnset_BothCredsComplete_PrefersOAuth2AsPrimary(t *testing.T) {
	clearProviderEnvVars(t)
	p := &WorkspaceOneProvider{}
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)
	configVal := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
		"instance_url":            tftypes.NewValue(tftypes.String, "https://example.awmdm.com"),
		"tenant_code":             tftypes.NewValue(tftypes.String, "tenant123"),
		"auth_method":             tftypes.NewValue(tftypes.String, nil),
		"username":                tftypes.NewValue(tftypes.String, "user1"),
		"password":                tftypes.NewValue(tftypes.String, "pass1"),
		"client_id":               tftypes.NewValue(tftypes.String, "client1"),
		"client_secret":           tftypes.NewValue(tftypes.String, "secret1"),
		"oauth2_token_url":        tftypes.NewValue(tftypes.String, nil),
		"app_binary_storage_path": tftypes.NewValue(tftypes.String, nil),
	})
	req := provider.ConfigureRequest{Config: tfsdk.Config{Raw: configVal, Schema: schemaResp.Schema}}
	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error: both creds complete, auth_method unset, got: %v", resp.Diagnostics.Errors())
	}
	for _, w := range resp.Diagnostics.Warnings() {
		if w.Summary() == "Multiple Credential Sets Configured" {
			t.Fatalf("did not expect 'Multiple Credential Sets Configured' warning (removed in v1n.2), got: %v", resp.Diagnostics.Warnings())
		}
	}
	pd, ok := resp.ResourceData.(*providerdata.ProviderData)
	if !ok || pd.Client == nil {
		t.Fatalf("expected ResourceData to be *providerdata.ProviderData with a non-nil Client, got: %#v", resp.ResourceData)
	}
	if pd.SecondaryClient == nil {
		t.Fatal("expected a non-nil SecondaryClient when both credential sets are complete")
	}
	if got := clientAuthMethod(t, pd.Client); got != "oauth2" {
		t.Errorf("expected primary Client auth method 'oauth2' (preferred when unset+both complete), got %q", got)
	}
	if got := clientAuthMethod(t, pd.SecondaryClient); got != "basic" {
		t.Errorf("expected SecondaryClient auth method 'basic', got %q", got)
	}
}

// TestConfigure_ExplicitAuthMethodBasic_BothCredsComplete_BasicIsPrimary
// covers v1n.2 (a): an explicit auth_method overrides the oauth2-when-both
// inference preference.
func TestConfigure_ExplicitAuthMethodBasic_BothCredsComplete_BasicIsPrimary(t *testing.T) {
	clearProviderEnvVars(t)
	p := &WorkspaceOneProvider{}
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)
	configVal := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
		"instance_url":            tftypes.NewValue(tftypes.String, "https://example.awmdm.com"),
		"tenant_code":             tftypes.NewValue(tftypes.String, "tenant123"),
		"auth_method":             tftypes.NewValue(tftypes.String, "basic"),
		"username":                tftypes.NewValue(tftypes.String, "user1"),
		"password":                tftypes.NewValue(tftypes.String, "pass1"),
		"client_id":               tftypes.NewValue(tftypes.String, "client1"),
		"client_secret":           tftypes.NewValue(tftypes.String, "secret1"),
		"oauth2_token_url":        tftypes.NewValue(tftypes.String, nil),
		"app_binary_storage_path": tftypes.NewValue(tftypes.String, nil),
	})
	req := provider.ConfigureRequest{Config: tfsdk.Config{Raw: configVal, Schema: schemaResp.Schema}}
	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error: explicit auth_method=basic with both creds complete, got: %v", resp.Diagnostics.Errors())
	}
	pd, ok := resp.ResourceData.(*providerdata.ProviderData)
	if !ok || pd.Client == nil {
		t.Fatalf("expected ResourceData to be *providerdata.ProviderData with a non-nil Client, got: %#v", resp.ResourceData)
	}
	if pd.SecondaryClient == nil {
		t.Fatal("expected a non-nil SecondaryClient (the oauth2 client) when both credential sets are complete")
	}
	if got := clientAuthMethod(t, pd.Client); got != "basic" {
		t.Errorf("expected primary Client auth method 'basic' (explicit auth_method overrides oauth2-when-both preference), got %q", got)
	}
	if got := clientAuthMethod(t, pd.SecondaryClient); got != "oauth2" {
		t.Errorf("expected SecondaryClient auth method 'oauth2', got %q", got)
	}
}

func TestConfigure_BasicOnlyCreds_NoError(t *testing.T) {
	clearProviderEnvVars(t)
	p := &WorkspaceOneProvider{}
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)
	configVal := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
		"instance_url":            tftypes.NewValue(tftypes.String, "https://example.awmdm.com"),
		"tenant_code":             tftypes.NewValue(tftypes.String, "tenant123"),
		"auth_method":             tftypes.NewValue(tftypes.String, "basic"),
		"username":                tftypes.NewValue(tftypes.String, "user1"),
		"password":                tftypes.NewValue(tftypes.String, "pass1"),
		"client_id":               tftypes.NewValue(tftypes.String, nil),
		"client_secret":           tftypes.NewValue(tftypes.String, nil),
		"oauth2_token_url":        tftypes.NewValue(tftypes.String, nil),
		"app_binary_storage_path": tftypes.NewValue(tftypes.String, nil),
	})
	req := provider.ConfigureRequest{Config: tfsdk.Config{Raw: configVal, Schema: schemaResp.Schema}}
	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error: basic-only creds present, got: %v", resp.Diagnostics.Errors())
	}
}

func TestConfigure_BothCreds_BuildsClient(t *testing.T) {
	clearProviderEnvVars(t)
	p := &WorkspaceOneProvider{}
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)
	configVal := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
		"instance_url":            tftypes.NewValue(tftypes.String, "https://example.awmdm.com"),
		"tenant_code":             tftypes.NewValue(tftypes.String, "tenant123"),
		"auth_method":             tftypes.NewValue(tftypes.String, "basic"),
		"username":                tftypes.NewValue(tftypes.String, "user1"),
		"password":                tftypes.NewValue(tftypes.String, "pass1"),
		"client_id":               tftypes.NewValue(tftypes.String, "client1"),
		"client_secret":           tftypes.NewValue(tftypes.String, "secret1"),
		"oauth2_token_url":        tftypes.NewValue(tftypes.String, nil),
		"app_binary_storage_path": tftypes.NewValue(tftypes.String, nil),
	})
	req := provider.ConfigureRequest{Config: tfsdk.Config{Raw: configVal, Schema: schemaResp.Schema}}
	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error: both basic and oauth2 creds present, got: %v", resp.Diagnostics.Errors())
	}
	data, ok := resp.ResourceData.(*providerdata.ProviderData)
	if !ok {
		t.Fatalf("expected ResourceData type *providerdata.ProviderData, got %T", resp.ResourceData)
	}
	if data == nil || data.Client == nil {
		t.Fatal("expected non-nil *providerdata.ProviderData with non-nil Client")
	}
}

func TestConfigure_NoCredsAtAll_Error(t *testing.T) {
	clearProviderEnvVars(t)
	p := &WorkspaceOneProvider{}
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)
	configVal := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
		"instance_url":            tftypes.NewValue(tftypes.String, "https://example.awmdm.com"),
		"tenant_code":             tftypes.NewValue(tftypes.String, "tenant123"),
		"auth_method":             tftypes.NewValue(tftypes.String, "basic"),
		"username":                tftypes.NewValue(tftypes.String, nil),
		"password":                tftypes.NewValue(tftypes.String, nil),
		"client_id":               tftypes.NewValue(tftypes.String, nil),
		"client_secret":           tftypes.NewValue(tftypes.String, nil),
		"oauth2_token_url":        tftypes.NewValue(tftypes.String, nil),
		"app_binary_storage_path": tftypes.NewValue(tftypes.String, nil),
	})
	req := provider.ConfigureRequest{Config: tfsdk.Config{Raw: configVal, Schema: schemaResp.Schema}}
	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected error when no credential set is present")
	}
}

// --- Auth verification (Configure fails fast on bad credentials) ---

func TestConfigure_AuthVerificationFails_ReturnsError(t *testing.T) {
	clearProviderEnvVars(t)
	orig := verifyClientAuth
	t.Cleanup(func() { verifyClientAuth = orig })
	verifyClientAuth = func(_ context.Context, _ *sdkclient.Client) error {
		return errors.New("401 unauthorized: invalid credentials")
	}

	p := &WorkspaceOneProvider{}
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)
	configVal := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
		"instance_url":            tftypes.NewValue(tftypes.String, "https://example.awmdm.com"),
		"tenant_code":             tftypes.NewValue(tftypes.String, "tenant123"),
		"auth_method":             tftypes.NewValue(tftypes.String, "basic"),
		"username":                tftypes.NewValue(tftypes.String, "user1"),
		"password":                tftypes.NewValue(tftypes.String, "wrong-password"),
		"client_id":               tftypes.NewValue(tftypes.String, nil),
		"client_secret":           tftypes.NewValue(tftypes.String, nil),
		"oauth2_token_url":        tftypes.NewValue(tftypes.String, nil),
		"app_binary_storage_path": tftypes.NewValue(tftypes.String, nil),
	})
	req := provider.ConfigureRequest{Config: tfsdk.Config{Raw: configVal, Schema: schemaResp.Schema}}
	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when auth verification fails, got none")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if d.Summary() == "Unable to Authenticate" {
			found = true
			if d.Detail() == "" {
				t.Error("expected error detail to name the underlying failure")
			}
		}
	}
	if !found {
		t.Errorf("expected 'Unable to Authenticate' error summary, got: %v", resp.Diagnostics.Errors())
	}
	if resp.ResourceData != nil || resp.DataSourceData != nil {
		t.Error("expected ResourceData/DataSourceData to remain unset when auth verification fails")
	}
}

func TestConfigure_AuthVerificationSucceeds_NoError(t *testing.T) {
	clearProviderEnvVars(t)
	orig := verifyClientAuth
	t.Cleanup(func() { verifyClientAuth = orig })
	verifyClientAuth = func(_ context.Context, _ *sdkclient.Client) error { return nil }

	p := &WorkspaceOneProvider{}
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)
	configVal := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
		"instance_url":            tftypes.NewValue(tftypes.String, "https://example.awmdm.com"),
		"tenant_code":             tftypes.NewValue(tftypes.String, "tenant123"),
		"auth_method":             tftypes.NewValue(tftypes.String, "basic"),
		"username":                tftypes.NewValue(tftypes.String, "user1"),
		"password":                tftypes.NewValue(tftypes.String, "pass1"),
		"client_id":               tftypes.NewValue(tftypes.String, nil),
		"client_secret":           tftypes.NewValue(tftypes.String, nil),
		"oauth2_token_url":        tftypes.NewValue(tftypes.String, nil),
		"app_binary_storage_path": tftypes.NewValue(tftypes.String, nil),
	})
	req := provider.ConfigureRequest{Config: tfsdk.Config{Raw: configVal, Schema: schemaResp.Schema}}
	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error when auth verification succeeds, got: %v", resp.Diagnostics.Errors())
	}
	if resp.ResourceData == nil || resp.DataSourceData == nil {
		t.Fatal("expected ResourceData/DataSourceData to be set when auth verification succeeds")
	}
}

// TestConfigure_InstanceURLUnparseable_HardFailsWithoutBuildingClient covers
// the fail-closed path on a malformed/unparseable instance_url (a
// url.Parse error): it must hard-error AND leave ResourceData/DataSourceData
// unset — this is the exact guarantee the HTTPS hardening protects, and
// without a test a future refactor could flip it silently. It also checks that
// a parse failure gets its own "Invalid Instance URL" summary, distinct
// from "Insecure Instance URL" (which is reserved for a URL that parses
// fine but violates the HTTPS policy).
func TestConfigure_InstanceURLUnparseable_HardFailsWithoutBuildingClient(t *testing.T) {
	clearProviderEnvVars(t)
	p := &WorkspaceOneProvider{}
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)
	configVal := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
		"instance_url":            tftypes.NewValue(tftypes.String, "http://%zz"),
		"tenant_code":             tftypes.NewValue(tftypes.String, "tenant123"),
		"auth_method":             tftypes.NewValue(tftypes.String, "basic"),
		"username":                tftypes.NewValue(tftypes.String, "user1"),
		"password":                tftypes.NewValue(tftypes.String, "pass1"),
		"client_id":               tftypes.NewValue(tftypes.String, nil),
		"client_secret":           tftypes.NewValue(tftypes.String, nil),
		"oauth2_token_url":        tftypes.NewValue(tftypes.String, nil),
		"app_binary_storage_path": tftypes.NewValue(tftypes.String, nil),
	})
	req := provider.ConfigureRequest{Config: tfsdk.Config{Raw: configVal, Schema: schemaResp.Schema}}
	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for an unparseable instance_url")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if d.Summary() == "Invalid Instance URL" {
			found = true
		}
		if d.Summary() == "Insecure Instance URL" {
			t.Errorf("parse failure must not be reported as 'Insecure Instance URL', got: %s: %s", d.Summary(), d.Detail())
		}
	}
	if !found {
		t.Errorf("expected 'Invalid Instance URL' error summary, got: %v", resp.Diagnostics.Errors())
	}
	if resp.ResourceData != nil || resp.DataSourceData != nil {
		t.Error("expected ResourceData/DataSourceData to remain unset for an unparseable instance_url")
	}
}

// TestConfigure_AuthVerification403_WarnsAndProceeds covers #4: a 403 from
// the verification probe means the credentials authenticated fine but the
// probe call itself was denied by RBAC — that should be a warning, and
// Configure should still succeed, unlike a hard auth failure (401/other).
func TestConfigure_AuthVerification403_WarnsAndProceeds(t *testing.T) {
	clearProviderEnvVars(t)
	orig := verifyClientAuth
	t.Cleanup(func() { verifyClientAuth = orig })
	verifyClientAuth = func(_ context.Context, _ *sdkclient.Client) error {
		return &sdkclient.APIError{StatusCode: 403, Message: "permission denied"}
	}

	p := &WorkspaceOneProvider{}
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)
	configVal := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
		"instance_url":            tftypes.NewValue(tftypes.String, "https://example.awmdm.com"),
		"tenant_code":             tftypes.NewValue(tftypes.String, "tenant123"),
		"auth_method":             tftypes.NewValue(tftypes.String, "basic"),
		"username":                tftypes.NewValue(tftypes.String, "user1"),
		"password":                tftypes.NewValue(tftypes.String, "pass1"),
		"client_id":               tftypes.NewValue(tftypes.String, nil),
		"client_secret":           tftypes.NewValue(tftypes.String, nil),
		"oauth2_token_url":        tftypes.NewValue(tftypes.String, nil),
		"app_binary_storage_path": tftypes.NewValue(tftypes.String, nil),
	})
	req := provider.ConfigureRequest{Config: tfsdk.Config{Raw: configVal, Schema: schemaResp.Schema}}
	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error on 403 (permission-denied, not auth failure), got: %v", resp.Diagnostics.Errors())
	}
	found := false
	for _, d := range resp.Diagnostics.Warnings() {
		if d.Summary() == "Auth-Verify Probe Denied by Permissions" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'Auth-Verify Probe Denied by Permissions' warning, got: %v", resp.Diagnostics.Warnings())
	}
	if resp.ResourceData == nil || resp.DataSourceData == nil {
		t.Fatal("expected ResourceData/DataSourceData to be set when the probe is only permission-denied")
	}
}

// TestConfigure_AuthVerification401APIError_HardFails covers #4's other
// branch: a 401 *sdkclient.APIError is a genuine credential failure and
// must remain a hard error.
func TestConfigure_AuthVerification401APIError_HardFails(t *testing.T) {
	clearProviderEnvVars(t)
	orig := verifyClientAuth
	t.Cleanup(func() { verifyClientAuth = orig })
	verifyClientAuth = func(_ context.Context, _ *sdkclient.Client) error {
		return &sdkclient.APIError{StatusCode: 401, Message: "invalid credentials"}
	}

	p := &WorkspaceOneProvider{}
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)
	configVal := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
		"instance_url":            tftypes.NewValue(tftypes.String, "https://example.awmdm.com"),
		"tenant_code":             tftypes.NewValue(tftypes.String, "tenant123"),
		"auth_method":             tftypes.NewValue(tftypes.String, "basic"),
		"username":                tftypes.NewValue(tftypes.String, "user1"),
		"password":                tftypes.NewValue(tftypes.String, "pass1"),
		"client_id":               tftypes.NewValue(tftypes.String, nil),
		"client_secret":           tftypes.NewValue(tftypes.String, nil),
		"oauth2_token_url":        tftypes.NewValue(tftypes.String, nil),
		"app_binary_storage_path": tftypes.NewValue(tftypes.String, nil),
	})
	req := provider.ConfigureRequest{Config: tfsdk.Config{Raw: configVal, Schema: schemaResp.Schema}}
	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a hard error on 401 APIError")
	}
	if resp.ResourceData != nil || resp.DataSourceData != nil {
		t.Error("expected ResourceData/DataSourceData to remain unset on 401")
	}
}

func TestConfigure_AppBinaryStoragePath_FromEnv(t *testing.T) {
	clearProviderEnvVars(t)
	t.Setenv("UEM_APP_BINARY_DIR", "/tmp/uem-app-binaries")
	p := &WorkspaceOneProvider{}
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)
	configVal := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
		"instance_url":            tftypes.NewValue(tftypes.String, "https://example.awmdm.com"),
		"tenant_code":             tftypes.NewValue(tftypes.String, "tenant123"),
		"auth_method":             tftypes.NewValue(tftypes.String, "basic"),
		"username":                tftypes.NewValue(tftypes.String, "user1"),
		"password":                tftypes.NewValue(tftypes.String, "pass1"),
		"client_id":               tftypes.NewValue(tftypes.String, nil),
		"client_secret":           tftypes.NewValue(tftypes.String, nil),
		"oauth2_token_url":        tftypes.NewValue(tftypes.String, nil),
		"app_binary_storage_path": tftypes.NewValue(tftypes.String, nil),
	})
	req := provider.ConfigureRequest{Config: tfsdk.Config{Raw: configVal, Schema: schemaResp.Schema}}
	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error, got: %v", resp.Diagnostics.Errors())
	}
	data, ok := resp.ResourceData.(*providerdata.ProviderData)
	if !ok {
		t.Fatalf("expected ResourceData type *providerdata.ProviderData, got %T", resp.ResourceData)
	}
	if data.AppBinaryStoragePath != "/tmp/uem-app-binaries" {
		t.Errorf("expected AppBinaryStoragePath '/tmp/uem-app-binaries', got '%s'", data.AppBinaryStoragePath)
	}
}

// TestConfigure_AppBinaryStoragePath_DefaultsToRepoRelative_NoHomeDir is F10:
// with neither app_binary_storage_path nor UEM_APP_BINARY_DIR set, the
// resolved default must be the cwd-relative "uem-artifacts/app-binaries" —
// never a path under the user's home directory. Setting HOME to an
// unmistakable sentinel value confirms os.UserHomeDir is never consulted for
// the default (a regression back to the old ~/.uem-provider behavior would
// make this test's own HOME override show up in the resolved path).
func TestConfigure_AppBinaryStoragePath_DefaultsToRepoRelative_NoHomeDir(t *testing.T) {
	clearProviderEnvVars(t)
	t.Setenv("HOME", "/no-such-home-should-be-used")
	p := &WorkspaceOneProvider{}
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)
	configVal := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
		"instance_url":            tftypes.NewValue(tftypes.String, "https://example.awmdm.com"),
		"tenant_code":             tftypes.NewValue(tftypes.String, "tenant123"),
		"auth_method":             tftypes.NewValue(tftypes.String, "basic"),
		"username":                tftypes.NewValue(tftypes.String, "user1"),
		"password":                tftypes.NewValue(tftypes.String, "pass1"),
		"client_id":               tftypes.NewValue(tftypes.String, nil),
		"client_secret":           tftypes.NewValue(tftypes.String, nil),
		"oauth2_token_url":        tftypes.NewValue(tftypes.String, nil),
		"app_binary_storage_path": tftypes.NewValue(tftypes.String, nil),
	})
	req := provider.ConfigureRequest{Config: tfsdk.Config{Raw: configVal, Schema: schemaResp.Schema}}
	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error, got: %v", resp.Diagnostics.Errors())
	}
	data, ok := resp.ResourceData.(*providerdata.ProviderData)
	if !ok {
		t.Fatalf("expected ResourceData type *providerdata.ProviderData, got %T", resp.ResourceData)
	}
	const wantDefault = "uem-artifacts/app-binaries"
	if data.AppBinaryStoragePath != wantDefault {
		t.Errorf("expected AppBinaryStoragePath %q, got %q", wantDefault, data.AppBinaryStoragePath)
	}
	if strings.Contains(data.AppBinaryStoragePath, "no-such-home-should-be-used") {
		t.Errorf("resolved default %q must never expand $HOME", data.AppBinaryStoragePath)
	}
	if filepath.IsAbs(data.AppBinaryStoragePath) {
		t.Errorf("resolved default %q must stay relative (cwd-relative, not absolutised)", data.AppBinaryStoragePath)
	}
}

// TestConfigure_AppBinaryStoragePath_ConfigWinsOverEnvAndDefault confirms the
// resolution order is unchanged by F10: an explicit config value still wins
// over both UEM_APP_BINARY_DIR and the new default.
func TestConfigure_AppBinaryStoragePath_ConfigWinsOverEnvAndDefault(t *testing.T) {
	clearProviderEnvVars(t)
	t.Setenv("UEM_APP_BINARY_DIR", "/tmp/env-should-lose")
	p := &WorkspaceOneProvider{}
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)
	configVal := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
		"instance_url":            tftypes.NewValue(tftypes.String, "https://example.awmdm.com"),
		"tenant_code":             tftypes.NewValue(tftypes.String, "tenant123"),
		"auth_method":             tftypes.NewValue(tftypes.String, "basic"),
		"username":                tftypes.NewValue(tftypes.String, "user1"),
		"password":                tftypes.NewValue(tftypes.String, "pass1"),
		"client_id":               tftypes.NewValue(tftypes.String, nil),
		"client_secret":           tftypes.NewValue(tftypes.String, nil),
		"oauth2_token_url":        tftypes.NewValue(tftypes.String, nil),
		"app_binary_storage_path": tftypes.NewValue(tftypes.String, "/config/wins/app-binaries"),
	})
	req := provider.ConfigureRequest{Config: tfsdk.Config{Raw: configVal, Schema: schemaResp.Schema}}
	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error, got: %v", resp.Diagnostics.Errors())
	}
	data, ok := resp.ResourceData.(*providerdata.ProviderData)
	if !ok {
		t.Fatalf("expected ResourceData type *providerdata.ProviderData, got %T", resp.ResourceData)
	}
	if data.AppBinaryStoragePath != "/config/wins/app-binaries" {
		t.Errorf("expected config value to win, got %q", data.AppBinaryStoragePath)
	}
}

// TestConfigure_LogsResolvedAuthMethod covers v1n.2 (f): Configure must
// tflog.Info the resolved auth method, whether it was inferred or
// explicitly configured, and whether a secondary credential set is also
// configured — without ever logging a credential value. Uses
// tflogtest.RootLogger (already vendored via the terraform-plugin-log
// module this provider already depends on) to capture log output into a
// buffer instead of stderr, so the assertion is on actual emitted log
// content, not a stub.
//
// Table-driven across the three wordings the log line can take:
//   - inferred, single credential set configured
//   - explicitly configured (auth_method set), single credential set
//   - inferred, both credential sets configured (secondary noted)
func TestConfigure_LogsResolvedAuthMethod(t *testing.T) {
	tests := []struct {
		name                string
		authMethod          tftypes.Value
		username            tftypes.Value
		password            tftypes.Value
		clientID            tftypes.Value
		clientSecret        tftypes.Value
		wantSubstrings      []string
		wantAbsentSubstring string
	}{
		{
			name:         "Inferred",
			authMethod:   tftypes.NewValue(tftypes.String, nil),
			username:     tftypes.NewValue(tftypes.String, nil),
			password:     tftypes.NewValue(tftypes.String, nil),
			clientID:     tftypes.NewValue(tftypes.String, "client1"),
			clientSecret: tftypes.NewValue(tftypes.String, "verysecretvalue"),
			wantSubstrings: []string{
				"uem provider: using oauth2 authentication",
				"inferred from configured credentials",
				"no secondary credential set configured",
			},
			wantAbsentSubstring: "also configured as secondary",
		},
		{
			name:         "ExplicitlyConfigured",
			authMethod:   tftypes.NewValue(tftypes.String, "oauth2"),
			username:     tftypes.NewValue(tftypes.String, nil),
			password:     tftypes.NewValue(tftypes.String, nil),
			clientID:     tftypes.NewValue(tftypes.String, "client1"),
			clientSecret: tftypes.NewValue(tftypes.String, "verysecretvalue"),
			wantSubstrings: []string{
				"uem provider: using oauth2 authentication",
				"explicitly configured",
				"no secondary credential set configured",
			},
			wantAbsentSubstring: "also configured as secondary",
		},
		{
			name:         "BothCredsComplete_SecondaryNoted",
			authMethod:   tftypes.NewValue(tftypes.String, nil),
			username:     tftypes.NewValue(tftypes.String, "someuser"),
			password:     tftypes.NewValue(tftypes.String, "verysecretpassword"),
			clientID:     tftypes.NewValue(tftypes.String, "client1"),
			clientSecret: tftypes.NewValue(tftypes.String, "verysecretvalue"),
			wantSubstrings: []string{
				"uem provider: using oauth2 authentication",
				"inferred from configured credentials",
				"basic credentials also configured as secondary (not verified; unused)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearProviderEnvVars(t)
			var logBuf bytes.Buffer
			ctx := tflogtest.RootLogger(context.Background(), &logBuf)

			p := &WorkspaceOneProvider{}
			schemaResp := &provider.SchemaResponse{}
			p.Schema(ctx, provider.SchemaRequest{}, schemaResp)
			configVal := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), map[string]tftypes.Value{
				"instance_url":            tftypes.NewValue(tftypes.String, "https://example.awmdm.com"),
				"tenant_code":             tftypes.NewValue(tftypes.String, "tenant123"),
				"auth_method":             tt.authMethod,
				"username":                tt.username,
				"password":                tt.password,
				"client_id":               tt.clientID,
				"client_secret":           tt.clientSecret,
				"oauth2_token_url":        tftypes.NewValue(tftypes.String, nil),
				"app_binary_storage_path": tftypes.NewValue(tftypes.String, nil),
			})
			req := provider.ConfigureRequest{Config: tfsdk.Config{Raw: configVal, Schema: schemaResp.Schema}}
			resp := &provider.ConfigureResponse{}
			p.Configure(ctx, req, resp)

			if resp.Diagnostics.HasError() {
				t.Fatalf("expected no error, got: %v", resp.Diagnostics.Errors())
			}
			for _, w := range resp.Diagnostics.Warnings() {
				t.Errorf("did not expect any diagnostic warning for the log-only path, got: %s: %s", w.Summary(), w.Detail())
			}

			entries, err := tflogtest.MultilineJSONDecode(&logBuf)
			if err != nil {
				t.Fatalf("failed to decode captured log output: %s\nraw: %s", err, logBuf.String())
			}
			var found map[string]interface{}
			for _, e := range entries {
				msg, _ := e["@message"].(string)
				if strings.Contains(msg, "uem provider: using") {
					found = e
					break
				}
			}
			if found == nil {
				t.Fatalf("expected a log entry naming the resolved auth method, got entries: %#v", entries)
			}
			msg, _ := found["@message"].(string)
			for _, want := range tt.wantSubstrings {
				if !strings.Contains(msg, want) {
					t.Errorf("expected log message to contain %q, got: %q", want, msg)
				}
			}
			if tt.wantAbsentSubstring != "" && strings.Contains(msg, tt.wantAbsentSubstring) {
				t.Errorf("expected log message not to contain %q, got: %q", tt.wantAbsentSubstring, msg)
			}
			if found["@level"] != "info" {
				t.Errorf("expected the resolved-auth-method entry to be logged at info level, got: %v", found["@level"])
			}
			if strings.Contains(msg, "verysecretvalue") || strings.Contains(logBuf.String(), "verysecretvalue") {
				t.Fatal("log output must never contain a credential value")
			}
			if strings.Contains(msg, "verysecretpassword") || strings.Contains(logBuf.String(), "verysecretpassword") {
				t.Fatal("log output must never contain a credential value")
			}
		})
	}
}

// TestConfigure_WiresStallAwareHTTPClient is B10's "Configure actually
// passes the HTTPClient" unit test: it asserts, end to end through the real
// Configure() call (not just by calling newStallAwareHTTPClient/
// buildSDKClientConfig directly), that both the basic and oauth2
// sdkclient.Config end up with a non-nil HTTPClient whose Transport is
// internal/httpclient's stall-aware *httpclient.Transport (never nil, never
// http.DefaultTransport / a bare *http.Transport), and Timeout ==
// sdkHTTPClientTimeout — the long fixed value that keeps the SDK's own
// zero-defaults-to-30s branch (client/client.go) from substituting a real
// wall-clock cap back in. It also confirms the two clients get DIFFERENT
// *http.Client instances (not one shared, mutable pointer).
func TestConfigure_WiresStallAwareHTTPClient(t *testing.T) {
	clearProviderEnvVars(t)
	p := &WorkspaceOneProvider{}
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)
	configVal := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
		"instance_url":            tftypes.NewValue(tftypes.String, "https://example.awmdm.com"),
		"tenant_code":             tftypes.NewValue(tftypes.String, "tenant123"),
		"auth_method":             tftypes.NewValue(tftypes.String, "basic"),
		"username":                tftypes.NewValue(tftypes.String, "user1"),
		"password":                tftypes.NewValue(tftypes.String, "pass1"),
		"client_id":               tftypes.NewValue(tftypes.String, "client1"),
		"client_secret":           tftypes.NewValue(tftypes.String, "secret1"),
		"oauth2_token_url":        tftypes.NewValue(tftypes.String, nil),
		"app_binary_storage_path": tftypes.NewValue(tftypes.String, nil),
	})
	req := provider.ConfigureRequest{Config: tfsdk.Config{Raw: configVal, Schema: schemaResp.Schema}}
	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error, got: %v", resp.Diagnostics.Errors())
	}
	data, ok := resp.ResourceData.(*providerdata.ProviderData)
	if !ok {
		t.Fatalf("expected ResourceData type *providerdata.ProviderData, got %T", resp.ResourceData)
	}
	if data.Client == nil || data.SecondaryClient == nil {
		t.Fatal("expected both a primary and secondary client (both credential sets were complete)")
	}

	primaryCfg := clientHTTPClientConfig(t, data.Client)
	secondaryCfg := clientHTTPClientConfig(t, data.SecondaryClient)

	for _, tc := range []struct {
		name string
		cfg  *sdkclient.Config
	}{
		{"primary (" + primaryCfg.AuthMethod + ")", primaryCfg},
		{"secondary (" + secondaryCfg.AuthMethod + ")", secondaryCfg},
	} {
		if tc.cfg.HTTPClient == nil {
			t.Fatalf("%s: Config.HTTPClient is nil", tc.name)
		}
		if _, ok := tc.cfg.HTTPClient.Transport.(*httpclient.Transport); !ok {
			t.Fatalf("%s: Config.HTTPClient.Transport is %T, want *httpclient.Transport", tc.name, tc.cfg.HTTPClient.Transport)
		}
		// sdkclient.NewClient itself sets HTTPClient.Timeout = Config.Timeout
		// in place (client/client.go) — that mutation of OUR *http.Client is
		// exactly the behavior newStallAwareHTTPClient/sdkHTTPClientTimeout
		// are designed around (see their doc comments): a long fixed value
		// here so it never substitutes a real cap, with the header/stall
		// timeouts on the Transport (asserted above) providing the actual
		// protection.
		if tc.cfg.HTTPClient.Timeout != sdkHTTPClientTimeout {
			t.Errorf("%s: Config.HTTPClient.Timeout = %s, want sdkHTTPClientTimeout (%s)", tc.name, tc.cfg.HTTPClient.Timeout, sdkHTTPClientTimeout)
		}
		if tc.cfg.Timeout != sdkHTTPClientTimeout {
			t.Errorf("%s: Config.Timeout = %s, want sdkHTTPClientTimeout (%s)", tc.name, tc.cfg.Timeout, sdkHTTPClientTimeout)
		}
	}

	if primaryCfg.HTTPClient == secondaryCfg.HTTPClient {
		t.Error("expected the basic and oauth2 clients to get distinct *http.Client instances, got the same pointer")
	}
}

// TestConfigure_HTTPTimeoutEnvOverrides_FlowThroughToTransport confirms
// UEM_HTTP_HEADER_TIMEOUT / UEM_HTTP_STALL_TIMEOUT reach the constructed
// client's Transport via Configure(), not just via
// httpclient.ResolveHeaderTimeout/ResolveStallTimeout in isolation. It
// reaches into the unexported Transport fields via the same
// reflect+unsafe technique clientAuthMethod/clientHTTPClientConfig already
// use for sdkclient.Client's own unexported field, since *httpclient.
// Transport intentionally exposes no public accessor either (only its type
// is exported, for the `_, ok := transport.(*httpclient.Transport)` check).
func TestConfigure_HTTPTimeoutEnvOverrides_FlowThroughToTransport(t *testing.T) {
	clearProviderEnvVars(t)
	t.Setenv("UEM_HTTP_HEADER_TIMEOUT", "17s")
	t.Setenv("UEM_HTTP_STALL_TIMEOUT", "23s")

	p := &WorkspaceOneProvider{}
	schemaResp := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, schemaResp)
	configVal := tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
		"instance_url":            tftypes.NewValue(tftypes.String, "https://example.awmdm.com"),
		"tenant_code":             tftypes.NewValue(tftypes.String, "tenant123"),
		"auth_method":             tftypes.NewValue(tftypes.String, "basic"),
		"username":                tftypes.NewValue(tftypes.String, "user1"),
		"password":                tftypes.NewValue(tftypes.String, "pass1"),
		"client_id":               tftypes.NewValue(tftypes.String, nil),
		"client_secret":           tftypes.NewValue(tftypes.String, nil),
		"oauth2_token_url":        tftypes.NewValue(tftypes.String, nil),
		"app_binary_storage_path": tftypes.NewValue(tftypes.String, nil),
	})
	req := provider.ConfigureRequest{Config: tfsdk.Config{Raw: configVal, Schema: schemaResp.Schema}}
	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error, got: %v", resp.Diagnostics.Errors())
	}
	data, ok := resp.ResourceData.(*providerdata.ProviderData)
	if !ok {
		t.Fatalf("expected ResourceData type *providerdata.ProviderData, got %T", resp.ResourceData)
	}
	cfg := clientHTTPClientConfig(t, data.Client)
	// The SDK copies Config.Timeout onto HTTPClient.Timeout; a real cap here
	// (e.g. the SDK's 30s default) would cut off a slow-but-healthy download.
	if cfg.HTTPClient.Timeout < time.Hour {
		t.Errorf("HTTPClient.Timeout = %s, want no effective wall-clock cap (>= 1h)", cfg.HTTPClient.Timeout)
	}
	transport, ok := cfg.HTTPClient.Transport.(*httpclient.Transport)
	if !ok {
		t.Fatalf("Config.HTTPClient.Transport is %T, want *httpclient.Transport", cfg.HTTPClient.Transport)
	}

	rv := reflect.ValueOf(transport).Elem()
	headerTimeout := reflect.NewAt(rv.FieldByName("wrapped").Type(), unsafe.Pointer(rv.FieldByName("wrapped").UnsafeAddr())).Elem()
	wrapped, ok := headerTimeout.Interface().(http.RoundTripper)
	if !ok {
		t.Fatalf("Transport.wrapped is not an http.RoundTripper: %#v", headerTimeout.Interface())
	}
	inner, ok := wrapped.(*http.Transport)
	if !ok {
		t.Fatalf("Transport.wrapped is %T, want *http.Transport", wrapped)
	}
	if inner.ResponseHeaderTimeout != 17*time.Second {
		t.Errorf("ResponseHeaderTimeout = %s, want 17s", inner.ResponseHeaderTimeout)
	}

	stallTimeoutField := rv.FieldByName("stallTimeout")
	stallTimeout := reflect.NewAt(stallTimeoutField.Type(), unsafe.Pointer(stallTimeoutField.UnsafeAddr())).Elem().Interface().(time.Duration) //nolint:forcetypeassert // field is always time.Duration
	if stallTimeout != 23*time.Second {
		t.Errorf("stallTimeout = %s, want 23s", stallTimeout)
	}
}
