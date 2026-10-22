package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/euc-oss/terraform-provider-uem/internal/providerdata"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

const missingTenantCodeSummary = "Missing Tenant Code"

// configureTenantCode runs the real Configure with the given config values and
// environment and returns the response. The auth probe is stubbed by TestMain,
// so no request is made; the SDK still builds the clients, which is where it
// applies its own per-auth-method TenantCode rule.
func configureTenantCode(t *testing.T, cfg map[string]tftypes.Value, env map[string]string) *provider.ConfigureResponse {
	t.Helper()
	clearProviderEnvVars(t)
	for k, v := range env {
		t.Setenv(k, v)
	}
	p := &WorkspaceOneProvider{version: "test"}
	resp := &provider.ConfigureResponse{}
	p.Configure(context.Background(), provider.ConfigureRequest{Config: createProviderConfig(t, cfg)}, resp)
	return resp
}

func tenantCodeDiag(resp *provider.ConfigureResponse) (detail string, found bool) {
	for _, d := range resp.Diagnostics.Errors() {
		if d.Summary() == missingTenantCodeSummary {
			return d.Detail(), true
		}
	}
	return "", false
}

func basicConfig(tenant tftypes.Value) map[string]tftypes.Value {
	v := allNullProviderConfig()
	v["instance_url"] = stringVal("https://example.invalid")
	v["tenant_code"] = tenant
	v["auth_method"] = stringVal("basic")
	v["username"] = stringVal("user")
	v["password"] = stringVal("pass")
	return v
}

func oauth2Config(tenant tftypes.Value) map[string]tftypes.Value {
	v := allNullProviderConfig()
	v["instance_url"] = stringVal("https://example.invalid")
	v["tenant_code"] = tenant
	v["auth_method"] = stringVal("oauth2")
	v["client_id"] = stringVal("client")
	v["client_secret"] = stringVal("secret")
	return v
}

func TestConfigure_TenantCode_RequiredForBasicOnly(t *testing.T) {
	unknown := tftypes.NewValue(tftypes.String, tftypes.UnknownValue)

	tests := []struct {
		name        string
		cfg         map[string]tftypes.Value
		env         map[string]string
		wantMissing bool
		wantDetail  string
	}{
		{
			name:        "basic_without_tenant_code_fails",
			cfg:         basicConfig(nullString()),
			wantMissing: true,
			wantDetail:  "tenant_code is required when auth_method is basic",
		},
		{
			name:        "basic_with_empty_tenant_code_fails",
			cfg:         basicConfig(stringVal("")),
			wantMissing: true,
			wantDetail:  "tenant_code is required when auth_method is basic",
		},
		{
			name: "basic_inferred_from_credentials_without_tenant_code_fails",
			cfg: func() map[string]tftypes.Value {
				v := basicConfig(nullString())
				v["auth_method"] = nullString()
				return v
			}(),
			wantMissing: true,
			wantDetail:  "tenant_code is required when auth_method is basic",
		},
		{
			name: "basic_auth_method_from_env_without_tenant_code_fails",
			cfg: func() map[string]tftypes.Value {
				v := basicConfig(nullString())
				v["auth_method"] = nullString()
				return v
			}(),
			env:         map[string]string{"UEM_AUTH_METHOD": "basic"},
			wantMissing: true,
			wantDetail:  "tenant_code is required when auth_method is basic",
		},
		{
			name:        "basic_with_unknown_tenant_code_and_no_env_is_absent_like_other_fields",
			cfg:         basicConfig(unknown),
			wantMissing: true,
			wantDetail:  "tenant_code is required when auth_method is basic",
		},
		{
			name: "basic_with_tenant_code_passes",
			cfg:  basicConfig(stringVal("TENANT1")),
		},
		{
			name: "basic_with_tenant_code_from_env_passes",
			cfg:  basicConfig(nullString()),
			env:  map[string]string{"UEM_TENANT_CODE": "TENANT1"},
		},
		{
			name: "basic_with_unknown_tenant_code_falls_back_to_env",
			cfg:  basicConfig(unknown),
			env:  map[string]string{"UEM_TENANT_CODE": "TENANT1"},
		},
		{
			name: "oauth2_without_tenant_code_passes_validation",
			cfg:  oauth2Config(nullString()),
		},
		{
			name: "oauth2_with_empty_tenant_code_passes_validation",
			cfg:  oauth2Config(stringVal("")),
		},
		{
			name: "oauth2_with_unknown_tenant_code_passes_validation",
			cfg:  oauth2Config(unknown),
		},
		{
			name: "oauth2_inferred_from_credentials_without_tenant_code_passes_validation",
			cfg: func() map[string]tftypes.Value {
				v := oauth2Config(nullString())
				v["auth_method"] = nullString()
				return v
			}(),
		},
		{
			// Explicit basic with only oauth2 credentials: the tenant_code
			// requirement follows auth_method, not just the credential sets.
			name: "explicit_basic_with_only_oauth2_credentials_requires_tenant_code",
			cfg: func() map[string]tftypes.Value {
				v := oauth2Config(nullString())
				v["auth_method"] = stringVal("basic")
				return v
			}(),
			wantMissing: true,
			wantDetail:  "tenant_code is required when auth_method is basic",
		},
		{
			// A partial basic set never builds a basic client.
			name: "oauth2_with_username_only_does_not_require_tenant_code",
			cfg: func() map[string]tftypes.Value {
				v := oauth2Config(nullString())
				v["username"] = stringVal("user")
				return v
			}(),
		},
		{
			// Basic is the effective method, so the code is required even
			// though a complete oauth2 set is present too.
			name: "explicit_basic_with_both_credential_sets_requires_tenant_code",
			cfg: func() map[string]tftypes.Value {
				v := basicConfig(nullString())
				v["client_id"] = stringVal("client")
				v["client_secret"] = stringVal("secret")
				return v
			}(),
			wantMissing: true,
			wantDetail:  "tenant_code is required when auth_method is basic",
		},
		{
			name: "oauth2_with_tenant_code_passes",
			cfg:  oauth2Config(stringVal("TENANT1")),
		},
		{
			name: "oauth2_primary_with_basic_credentials_alongside_and_tenant_code_passes",
			cfg: func() map[string]tftypes.Value {
				v := oauth2Config(stringVal("TENANT1"))
				v["username"] = stringVal("user")
				v["password"] = stringVal("pass")
				return v
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := configureTenantCode(t, tt.cfg, tt.env)
			detail, found := tenantCodeDiag(resp)
			if found != tt.wantMissing {
				t.Fatalf("Missing Tenant Code diagnostic present = %v, want %v; errors: %v", found, tt.wantMissing, resp.Diagnostics.Errors())
			}
			if !tt.wantMissing && resp.Diagnostics.HasError() {
				// The SDK builds the clients without a tenant code for oauth2, so
				// a passing case has no error at all, not just no tenant error.
				t.Fatalf("expected no error diagnostics, got: %v", resp.Diagnostics.Errors())
			}
			if !tt.wantMissing && resp.ResourceData == nil {
				t.Fatal("expected ResourceData to be set when Configure succeeds")
			}
			if tt.wantMissing {
				if !strings.Contains(detail, tt.wantDetail) {
					t.Errorf("detail %q does not contain %q", detail, tt.wantDetail)
				}
				if !strings.Contains(detail, "UEM_TENANT_CODE") || !strings.Contains(detail, "tenant_code attribute") {
					t.Errorf("detail %q should name the attribute and the environment variable", detail)
				}
			}
		})
	}
}

// An explicit tenant_code attribute wins over UEM_TENANT_CODE; with the
// attribute set, the env var is never needed.
func TestConfigure_TenantCode_AttributeWinsOverEnv(t *testing.T) {
	cfg := basicConfig(stringVal("TENANT1"))
	resp := configureTenantCode(t, cfg, map[string]string{"UEM_TENANT_CODE": "TENANT2"})
	if _, found := tenantCodeDiag(resp); found {
		t.Fatalf("unexpected Missing Tenant Code diagnostic: %v", resp.Diagnostics.Errors())
	}
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics.Errors())
	}
	data, ok := resp.ResourceData.(*providerdata.ProviderData)
	if !ok || data == nil || data.Client == nil {
		t.Fatalf("ResourceData = %T, want *providerdata.ProviderData with a client", resp.ResourceData)
	}
	if got := clientHTTPClientConfig(t, data.Client).TenantCode; got != "TENANT1" {
		t.Errorf("primary client TenantCode = %q, want TENANT1 (the attribute, not UEM_TENANT_CODE)", got)
	}
}

// An absent tenant_code reaches the SDK client config as the empty string; the
// SDK (client.go validateConfig and the header code) decides what that means.
func TestBuildSDKClientConfig_AbsentTenantCodeIsEmpty(t *testing.T) {
	cfg := buildSDKClientConfig("https://example.invalid", "", "oauth2", nil, "ua")
	if cfg.TenantCode != "" {
		t.Fatalf("TenantCode = %q, want empty", cfg.TenantCode)
	}
}

// recordedRequest is one request the fake UEM server saw.
type recordedRequest struct {
	path string
	// tenantCode is the aw-tenant-code header value; hasTenantCode is false when
	// the header was not sent at all (as opposed to sent empty).
	tenantCode    string
	hasTenantCode bool
}

// newRecordingUEM starts a fake UEM server that answers the OAuth2 token
// endpoint and every other path with an empty JSON object, and records every
// request. It is plain http on 127.0.0.1, which the provider accepts.
func newRecordingUEM(t *testing.T) (*httptest.Server, func() []recordedRequest) {
	t.Helper()
	var mu sync.Mutex
	var seen []recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vals, has := r.Header["Aw-Tenant-Code"]
		rec := recordedRequest{path: r.URL.Path, hasTenantCode: has}
		if has && len(vals) > 0 {
			rec.tenantCode = vals[0]
		}
		mu.Lock()
		seen = append(seen, rec)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/oauth/token") {
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "token_type": "Bearer", "expires_in": 3600})
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	return srv, func() []recordedRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]recordedRequest(nil), seen...)
	}
}

// useRealAuthVerification lets Configure run the provider's real auth probe
// (a SearchProfiles call) against the fake server instead of the TestMain stub,
// so the test sees the request the production path sends.
func useRealAuthVerification(t *testing.T) {
	t.Helper()
	orig := verifyClientAuth
	t.Cleanup(func() { verifyClientAuth = orig })
	verifyClientAuth = defaultVerifyClientAuth
}

// OAuth2 credentials without any tenant_code (no attribute, no env) configure
// the provider with no error at all, build the oauth2 client with an empty
// TenantCode, and send no aw-tenant-code header on the token call or the API
// call. The basic and oauth2-with-code controls show the header is still sent
// when a code is set.
func TestConfigure_OAuth2WithoutTenantCode_BuildsClient(t *testing.T) {
	useRealAuthVerification(t)

	t.Run("oauth2_without_tenant_code", func(t *testing.T) {
		srv, requests := newRecordingUEM(t)
		cfg := oauth2Config(nullString())
		cfg["instance_url"] = stringVal(srv.URL)
		cfg["oauth2_token_url"] = stringVal(srv.URL + "/oauth/token")

		resp := configureTenantCode(t, cfg, nil)
		if resp.Diagnostics.HasError() {
			t.Fatalf("expected zero error diagnostics, got: %v", resp.Diagnostics.Errors())
		}
		data, ok := resp.ResourceData.(*providerdata.ProviderData)
		if !ok || data == nil {
			t.Fatalf("ResourceData = %T, want non-nil *providerdata.ProviderData", resp.ResourceData)
		}
		if data.Client == nil {
			t.Fatal("expected a non-nil primary client")
		}
		if data.SecondaryClient != nil {
			t.Error("expected no secondary client: no basic credentials were set")
		}
		if got := clientAuthMethod(t, data.Client); got != "oauth2" {
			t.Errorf("primary client auth method = %q, want oauth2", got)
		}
		if got := clientHTTPClientConfig(t, data.Client).TenantCode; got != "" {
			t.Errorf("oauth2 client TenantCode = %q, want empty", got)
		}

		var api, token int
		for _, r := range requests() {
			if r.hasTenantCode {
				t.Errorf("%s carried an aw-tenant-code header (%q); want it absent", r.path, r.tenantCode)
			}
			if strings.HasSuffix(r.path, "/oauth/token") {
				token++
			} else {
				api++
			}
		}
		if token == 0 || api == 0 {
			t.Fatalf("expected a token request and an API request, saw %d token and %d api: %+v", token, api, requests())
		}
	})

	t.Run("oauth2_with_tenant_code_control_sends_header", func(t *testing.T) {
		srv, requests := newRecordingUEM(t)
		cfg := oauth2Config(stringVal("TENANT1"))
		cfg["instance_url"] = stringVal(srv.URL)
		cfg["oauth2_token_url"] = stringVal(srv.URL + "/oauth/token")

		resp := configureTenantCode(t, cfg, nil)
		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected errors: %v", resp.Diagnostics.Errors())
		}
		assertAPIRequestsCarryTenantCode(t, requests(), "TENANT1")
	})

	t.Run("basic_with_tenant_code_control_sends_header", func(t *testing.T) {
		srv, requests := newRecordingUEM(t)
		cfg := basicConfig(stringVal("TENANT1"))
		cfg["instance_url"] = stringVal(srv.URL)

		resp := configureTenantCode(t, cfg, nil)
		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected errors: %v", resp.Diagnostics.Errors())
		}
		data, ok := resp.ResourceData.(*providerdata.ProviderData)
		if !ok || data == nil || data.Client == nil {
			t.Fatalf("ResourceData = %T, want *providerdata.ProviderData with a client", resp.ResourceData)
		}
		if got := clientHTTPClientConfig(t, data.Client).TenantCode; got != "TENANT1" {
			t.Errorf("basic client TenantCode = %q, want TENANT1", got)
		}
		assertAPIRequestsCarryTenantCode(t, requests(), "TENANT1")
	})
}

func assertAPIRequestsCarryTenantCode(t *testing.T, requests []recordedRequest, want string) {
	t.Helper()
	var api int
	for _, r := range requests {
		if strings.HasSuffix(r.path, "/oauth/token") {
			continue
		}
		api++
		if !r.hasTenantCode || r.tenantCode != want {
			t.Errorf("%s: aw-tenant-code = %q (sent=%v), want %q", r.path, r.tenantCode, r.hasTenantCode, want)
		}
	}
	if api == 0 {
		t.Fatalf("expected at least one API request, saw %+v", requests)
	}
}

func warningSummaries(resp *provider.ConfigureResponse) []string {
	var out []string
	for _, d := range resp.Diagnostics.Warnings() {
		out = append(out, d.Summary())
	}
	return out
}

const basicIgnoredSummary = "Basic Credentials Ignored"

// Distinctive fake basic credentials, so a test can tell if a diagnostic echoes them.
const (
	ignoredUser = "BASICUSER1"
	ignoredPass = "BASICPASS1"
)

func withBasicCreds(v map[string]tftypes.Value) map[string]tftypes.Value {
	v["username"] = stringVal(ignoredUser)
	v["password"] = stringVal(ignoredPass)
	return v
}

// OAuth2 as the effective method, a complete basic set and no tenant code: no
// error, exactly one warning, only the oauth2 client is built (a basic client
// needs a tenant code and nothing uses it anyway). With a tenant code both
// clients are built and nothing is reported.
func TestConfigure_OAuth2WithCompleteBasicSet_TenantCodeDecidesSecondaryClient(t *testing.T) {
	unknown := tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	tests := []struct {
		name string
		cfg  map[string]tftypes.Value
		env  map[string]string
		// wantBasic is true when the basic (secondary) client must be built.
		wantBasic bool
	}{
		{name: "explicit_oauth2_no_code_ignores_basic", cfg: withBasicCreds(oauth2Config(nullString()))},
		{name: "empty_code_ignores_basic", cfg: withBasicCreds(oauth2Config(stringVal("")))},
		{name: "unknown_code_and_no_env_ignores_basic", cfg: withBasicCreds(oauth2Config(unknown))},
		{
			name: "inferred_oauth2_no_code_ignores_basic",
			cfg: func() map[string]tftypes.Value {
				v := withBasicCreds(oauth2Config(nullString()))
				v["auth_method"] = nullString()
				return v
			}(),
		},
		{
			name: "basic_creds_from_env_no_code_ignores_basic",
			cfg:  oauth2Config(nullString()),
			env:  map[string]string{"UEM_USERNAME": ignoredUser, "UEM_PASSWORD": ignoredPass},
		},
		{
			name: "oauth2_method_from_env_no_code_ignores_basic",
			cfg: func() map[string]tftypes.Value {
				v := withBasicCreds(oauth2Config(nullString()))
				v["auth_method"] = nullString()
				return v
			}(),
			env: map[string]string{"UEM_AUTH_METHOD": "oauth2"},
		},
		{name: "code_in_config_builds_both", cfg: withBasicCreds(oauth2Config(stringVal("TENANT1"))), wantBasic: true},
		{
			name:      "code_from_env_builds_both",
			cfg:       withBasicCreds(oauth2Config(nullString())),
			env:       map[string]string{"UEM_TENANT_CODE": "TENANT1"},
			wantBasic: true,
		},
		{
			name:      "unknown_code_falls_back_to_env_and_builds_both",
			cfg:       withBasicCreds(oauth2Config(unknown)),
			env:       map[string]string{"UEM_TENANT_CODE": "TENANT1"},
			wantBasic: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := configureTenantCode(t, tt.cfg, tt.env)
			if resp.Diagnostics.HasError() {
				t.Fatalf("expected zero error diagnostics, got: %v", resp.Diagnostics.Errors())
			}
			data, ok := resp.ResourceData.(*providerdata.ProviderData)
			if !ok || data == nil || data.Client == nil {
				t.Fatalf("ResourceData = %T, want *providerdata.ProviderData with a client", resp.ResourceData)
			}
			if got := clientAuthMethod(t, data.Client); got != "oauth2" {
				t.Errorf("primary client auth method = %q, want oauth2", got)
			}
			warnings := resp.Diagnostics.Warnings()
			if tt.wantBasic {
				if data.SecondaryClient == nil {
					t.Fatal("expected the basic client as secondary when a tenant code is set")
				}
				if got := clientAuthMethod(t, data.SecondaryClient); got != "basic" {
					t.Errorf("secondary client auth method = %q, want basic", got)
				}
				if len(warnings) != 0 {
					t.Errorf("expected no warnings, got: %v", warningSummaries(resp))
				}
				return
			}
			if data.SecondaryClient != nil {
				t.Error("expected no basic client: it needs a tenant code and none is set")
			}
			if len(warnings) != 1 {
				t.Fatalf("expected exactly one warning, got %d: %v", len(warnings), warningSummaries(resp))
			}
			w := warnings[0]
			if w.Summary() != basicIgnoredSummary {
				t.Errorf("warning summary = %q, want %q", w.Summary(), basicIgnoredSummary)
			}
			for _, want := range []string{
				"basic credentials are ignored",
				"a tenant code is required for basic authentication",
				"using oauth2",
				"tenant_code",
				"UEM_TENANT_CODE",
			} {
				if !strings.Contains(w.Detail(), want) {
					t.Errorf("warning detail %q does not contain %q", w.Detail(), want)
				}
			}
			// The warning names no credential value.
			for _, secret := range []string{ignoredUser, ignoredPass} {
				if strings.Contains(w.Detail(), secret) {
					t.Errorf("warning detail %q echoes a credential value", w.Detail())
				}
			}
		})
	}
}

// Cases where nothing is ignored, so no warning: partial or absent basic set,
// and basic as the effective method.
func TestConfigure_BasicIgnoredWarning_NotEmittedWhenNothingIsIgnored(t *testing.T) {
	tests := []struct {
		name          string
		cfg           map[string]tftypes.Value
		env           map[string]string
		wantTenantErr bool
		wantErrDetail string
	}{
		{name: "oauth2_only", cfg: oauth2Config(nullString())},
		{
			name: "oauth2_with_username_only",
			cfg: func() map[string]tftypes.Value {
				v := oauth2Config(nullString())
				v["username"] = stringVal("user")
				return v
			}(),
		},
		{
			name: "oauth2_with_password_only",
			cfg: func() map[string]tftypes.Value {
				v := oauth2Config(nullString())
				v["password"] = stringVal("pass")
				return v
			}(),
		},
		{
			name: "basic_with_both_sets_and_no_code_still_errors_without_a_warning",
			cfg: func() map[string]tftypes.Value {
				v := withBasicCreds(oauth2Config(nullString()))
				v["auth_method"] = stringVal("basic")
				return v
			}(),
			wantTenantErr: true,
		},
		{
			name: "basic_from_env_method_with_both_sets_and_no_code_errors",
			cfg: func() map[string]tftypes.Value {
				v := withBasicCreds(oauth2Config(nullString()))
				v["auth_method"] = nullString()
				return v
			}(),
			env:           map[string]string{"UEM_AUTH_METHOD": "basic"},
			wantTenantErr: true,
		},
		{
			name: "inferred_basic_only_no_code_errors",
			cfg: func() map[string]tftypes.Value {
				v := basicConfig(nullString())
				v["auth_method"] = nullString()
				return v
			}(),
			wantTenantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := configureTenantCode(t, tt.cfg, tt.env)
			if len(resp.Diagnostics.Warnings()) != 0 {
				t.Errorf("expected no warnings, got: %v", warningSummaries(resp))
			}
			_, found := tenantCodeDiag(resp)
			if found != tt.wantTenantErr {
				t.Fatalf("Missing Tenant Code present = %v, want %v; errors: %v", found, tt.wantTenantErr, resp.Diagnostics.Errors())
			}
			if !tt.wantTenantErr && resp.Diagnostics.HasError() {
				t.Fatalf("expected no errors, got: %v", resp.Diagnostics.Errors())
			}
		})
	}
}

// Explicit oauth2 with only a basic set and no tenant code: the basic set is
// not turned into a tenant-code error (oauth2 is the method), the user gets
// the missing-credentials error for the selected method and no warning.
func TestConfigure_ExplicitOAuth2WithOnlyBasicCreds_ReportsMissingOAuth2Credentials(t *testing.T) {
	cfg := basicConfig(nullString())
	cfg["auth_method"] = stringVal("oauth2")
	resp := configureTenantCode(t, cfg, nil)
	if _, found := tenantCodeDiag(resp); found {
		t.Errorf("unexpected Missing Tenant Code: %v", resp.Diagnostics.Errors())
	}
	var missingCreds bool
	for _, d := range resp.Diagnostics.Errors() {
		if d.Summary() == "Missing Credentials for Selected auth_method" {
			missingCreds = true
		}
	}
	if !missingCreds {
		t.Errorf("expected Missing Credentials for Selected auth_method, got: %v", resp.Diagnostics.Errors())
	}
	if len(resp.Diagnostics.Warnings()) != 0 {
		t.Errorf("expected no warnings, got: %v", warningSummaries(resp))
	}
}

// An invalid auth_method is reported as such; the ignored-basic warning is
// not stacked on top of it.
func TestConfigure_InvalidAuthMethodWithBothSetsAndNoCode_NoIgnoredWarning(t *testing.T) {
	cfg := withBasicCreds(oauth2Config(nullString()))
	cfg["auth_method"] = stringVal("bogus")
	resp := configureTenantCode(t, cfg, nil)
	var invalid bool
	for _, d := range resp.Diagnostics.Errors() {
		if d.Summary() == "Invalid Auth Method" {
			invalid = true
		}
	}
	if !invalid {
		t.Fatalf("expected Invalid Auth Method, got: %v", resp.Diagnostics.Errors())
	}
	if len(resp.Diagnostics.Warnings()) != 0 {
		t.Errorf("expected no warnings, got: %v", warningSummaries(resp))
	}
}

// An explicit empty tenant_code attribute wins over UEM_TENANT_CODE, like every
// other field, so the basic set is ignored with the warning.
func TestConfigure_EmptyTenantCodeAttributeShadowsEnv_IgnoresBasic(t *testing.T) {
	resp := configureTenantCode(t, withBasicCreds(oauth2Config(stringVal(""))), map[string]string{"UEM_TENANT_CODE": "TENANT1"})
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected errors: %v", resp.Diagnostics.Errors())
	}
	if got := warningSummaries(resp); len(got) != 1 || got[0] != basicIgnoredSummary {
		t.Errorf("warnings = %v, want exactly [%s]", got, basicIgnoredSummary)
	}
}
