package profile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceSchema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	commonerrors "github.com/euc-oss/terraform-provider-uem/internal/common/errors"
	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"
	profilestate "github.com/euc-oss/terraform-provider-uem/internal/profile/state"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

// --- NewProfileResource tests ---

func TestNewProfileResource(t *testing.T) {
	t.Parallel()
	r := NewProfileResource()
	if r == nil {
		t.Fatal("NewProfileResource() returned nil")
	}
}

// --- Metadata() tests ---

func TestProfileResourceMetadata(t *testing.T) {
	t.Parallel()
	r := &ProfileResource{}
	req := resource.MetadataRequest{ProviderTypeName: "uem"}
	var resp resource.MetadataResponse
	r.Metadata(context.Background(), req, &resp)

	if resp.TypeName != "uem_profile" {
		t.Errorf("expected TypeName 'uem_profile', got '%s'", resp.TypeName)
	}
}

// --- Schema() tests ---

func TestProfileResourceSchema(t *testing.T) {
	t.Parallel()
	r := &ProfileResource{}
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)

	s := resp.Schema

	// Verify all expected attributes exist.
	expectedAttrs := []string{
		"id", "name", "description", "platform", "org_group_id",
		"assignment_type", "profile_scope", "is_active",
		"lock_screen_message", "passcode", "custom_settings_list",
		"network_list", "credentials_list", "disk_encryption",
		"restrictions",
		"uuid", "profile_context",
	}
	for _, attr := range expectedAttrs {
		if _, ok := s.Attributes[attr]; !ok {
			t.Errorf("missing expected attribute: %s", attr)
		}
	}

	// Verify required attributes.
	requiredAttrs := []string{"name", "platform", "org_group_id"}
	for _, name := range requiredAttrs {
		attr, ok := s.Attributes[name]
		if !ok {
			continue
		}
		if sa, ok := attr.(resourceSchema.StringAttribute); ok {
			if !sa.Required {
				t.Errorf("expected attribute '%s' to be required", name)
			}
		}
	}

	// Verify computed attributes.
	computedAttrs := []string{"id", "uuid", "profile_context"}
	for _, name := range computedAttrs {
		attr, ok := s.Attributes[name]
		if !ok {
			continue
		}
		if sa, ok := attr.(resourceSchema.StringAttribute); ok {
			if !sa.Computed {
				t.Errorf("expected attribute '%s' to be computed", name)
			}
		}
	}

	// Verify platform has RequiresReplace plan modifier.
	platformAttr, ok := s.Attributes["platform"]
	if !ok {
		t.Fatal("missing 'platform' attribute")
	}
	if sa, ok := platformAttr.(resourceSchema.StringAttribute); ok {
		if len(sa.PlanModifiers) == 0 {
			t.Error("expected 'platform' to have plan modifiers (RequiresReplace)")
		}
	}
}

// TestProfileResourceSchema_TopLevelBlocksClearOnRemoval pins the u3a wiring:
// every managed top-level block carries exactly one plan modifier, the
// null-when-config-null one, so removing it from the configuration plans null
// (and the payload is deleted at apply) instead of keeping the prior state.
func TestProfileResourceSchema_TopLevelBlocksClearOnRemoval(t *testing.T) {
	t.Parallel()
	r := &ProfileResource{}
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	s := resp.Schema

	checkOnly := func(name string, mods []any, want any) {
		t.Helper()
		if len(mods) != 1 {
			t.Errorf("%s: want exactly 1 plan modifier (%T), got %d: %v", name, want, len(mods), mods)
			return
		}
		if gotT, wantT := fmt.Sprintf("%T", mods[0]), fmt.Sprintf("%T", want); gotT != wantT {
			t.Errorf("%s: plan modifier = %s, want %s", name, gotT, wantT)
		}
	}
	for _, name := range []string{"passcode", "disk_encryption", "gatekeeper", "restrictions"} {
		a, ok := s.Attributes[name].(resourceSchema.SingleNestedAttribute)
		if !ok {
			t.Fatalf("%s: not a SingleNestedAttribute", name)
		}
		mods := make([]any, 0, len(a.PlanModifiers))
		for _, m := range a.PlanModifiers {
			mods = append(mods, m)
		}
		checkOnly(name, mods, nullWhenConfigNullObjectModifier{})
	}
	for _, name := range []string{"custom_settings_list", "network_list", "credentials_list"} {
		a, ok := s.Attributes[name].(resourceSchema.ListNestedAttribute)
		if !ok {
			t.Fatalf("%s: not a ListNestedAttribute", name)
		}
		mods := make([]any, 0, len(a.PlanModifiers))
		for _, m := range a.PlanModifiers {
			mods = append(mods, m)
		}
		checkOnly(name, mods, nullWhenConfigNullListModifier{})
	}
	a, ok := s.Attributes["description"].(resourceSchema.StringAttribute)
	if !ok {
		t.Fatal("description: not a StringAttribute")
	}
	mods := make([]any, 0, len(a.PlanModifiers))
	for _, m := range a.PlanModifiers {
		mods = append(mods, m)
	}
	checkOnly("description", mods, descriptionPlanModifier{})
}

// --- credentials_list echoed-fields tests (B16 follow-up: faithful readback) ---

// credentialsListAttributes returns the credentials_list nested schema
// attributes, for pinning the certificate_authority/certificate_template/
// allow_access_to_all_applications/key_is_extractable wiring below.
func credentialsListAttributes(t *testing.T) map[string]resourceSchema.Attribute {
	t.Helper()
	r := &ProfileResource{}
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	credentialsList, ok := resp.Schema.Attributes["credentials_list"].(resourceSchema.ListNestedAttribute)
	if !ok {
		t.Fatalf("credentials_list is not a ListNestedAttribute: %T", resp.Schema.Attributes["credentials_list"])
	}
	return credentialsList.NestedObject.Attributes
}

// TestProfileResourceSchema_CredentialsListEchoedFieldsAreComputed pins the
// faithful-readback fix (B16 follow-up): certificate_authority,
// certificate_template, allow_access_to_all_applications and
// key_is_extractable are attributes UEM echoes back on every read
// (live-confirmed 2026-09-25: 0, 0, false, true when unset), so they must be
// Optional+Computed -- an unset config value plans unknown and takes UEM's
// readback, instead of being pinned to a hard-coded null (plain Optional,
// the pre-fix bug) or silently carried from prior state (which would hide
// real drift; see the reverted af93677f3). This test fails if Computed is
// ever reverted on any of the four.
func TestProfileResourceSchema_CredentialsListEchoedFieldsAreComputed(t *testing.T) {
	t.Parallel()
	nested := credentialsListAttributes(t)

	for _, name := range []string{
		"certificate_authority", "certificate_template",
		"allow_access_to_all_applications", "key_is_extractable",
	} {
		a, ok := nested[name]
		if !ok {
			t.Fatalf("%s: missing from credentials_list nested schema", name)
		}
		if !a.IsOptional() {
			t.Errorf("%s: want Optional = true, got false", name)
		}
		if !a.IsComputed() {
			t.Errorf("%s: want Computed = true, got false", name)
		}
	}
}

// TestProfileResourceSchema_CredentialsListEchoedFields_NoPerpetualDiffWhenUnset
// runs the actual schema-wired PlanModifiers for each of the four echoed
// fields with state holding UEM's previously-read-back values and config
// holding null (config omitted, matching every plan after the first), and
// asserts the plan comes out equal to state -- i.e. no change is planned.
// Within a list-nested attribute UseStateForUnknown on the leaves needs the
// resource to already have prior state for this attribute path (it does
// nothing on Create, where req.State.Raw is null); a minimal non-null
// tfsdk.State is enough to exercise that branch, since UseStateForUnknown
// only inspects req.State.Raw.IsNull(), req.PlanValue and req.ConfigValue.
func TestProfileResourceSchema_CredentialsListEchoedFields_NoPerpetualDiffWhenUnset(t *testing.T) {
	t.Parallel()
	nested := credentialsListAttributes(t)
	nonNullState := tfsdk.State{Raw: tftypes.NewValue(tftypes.Bool, true)}

	t.Run("certificate_authority stays at UEM's 0", func(t *testing.T) {
		a, ok := nested["certificate_authority"].(resourceSchema.Int64Attribute)
		if !ok {
			t.Fatalf("certificate_authority is not an Int64Attribute: %T", nested["certificate_authority"])
		}
		req := planmodifier.Int64Request{
			ConfigValue: types.Int64Null(),
			StateValue:  types.Int64Value(0),
			PlanValue:   types.Int64Unknown(),
			State:       nonNullState,
		}
		resp := &planmodifier.Int64Response{PlanValue: req.PlanValue}
		for _, m := range a.PlanModifiers {
			m.PlanModifyInt64(context.Background(), req, resp)
			req.PlanValue = resp.PlanValue
		}
		if !resp.PlanValue.Equal(types.Int64Value(0)) {
			t.Errorf("planned %v, want state's 0 (no perpetual diff)", resp.PlanValue)
		}
	})

	t.Run("certificate_template stays at UEM's 0", func(t *testing.T) {
		a, ok := nested["certificate_template"].(resourceSchema.Int64Attribute)
		if !ok {
			t.Fatalf("certificate_template is not an Int64Attribute: %T", nested["certificate_template"])
		}
		req := planmodifier.Int64Request{
			ConfigValue: types.Int64Null(),
			StateValue:  types.Int64Value(0),
			PlanValue:   types.Int64Unknown(),
			State:       nonNullState,
		}
		resp := &planmodifier.Int64Response{PlanValue: req.PlanValue}
		for _, m := range a.PlanModifiers {
			m.PlanModifyInt64(context.Background(), req, resp)
			req.PlanValue = resp.PlanValue
		}
		if !resp.PlanValue.Equal(types.Int64Value(0)) {
			t.Errorf("planned %v, want state's 0 (no perpetual diff)", resp.PlanValue)
		}
	})

	t.Run("allow_access_to_all_applications stays at UEM's false", func(t *testing.T) {
		a, ok := nested["allow_access_to_all_applications"].(resourceSchema.BoolAttribute)
		if !ok {
			t.Fatalf("allow_access_to_all_applications is not a BoolAttribute: %T", nested["allow_access_to_all_applications"])
		}
		req := planmodifier.BoolRequest{
			ConfigValue: types.BoolNull(),
			StateValue:  types.BoolValue(false),
			PlanValue:   types.BoolUnknown(),
			State:       nonNullState,
		}
		resp := &planmodifier.BoolResponse{PlanValue: req.PlanValue}
		for _, m := range a.PlanModifiers {
			m.PlanModifyBool(context.Background(), req, resp)
			req.PlanValue = resp.PlanValue
		}
		if !resp.PlanValue.Equal(types.BoolValue(false)) {
			t.Errorf("planned %v, want state's false (no perpetual diff)", resp.PlanValue)
		}
	})

	t.Run("key_is_extractable stays at UEM's true", func(t *testing.T) {
		a, ok := nested["key_is_extractable"].(resourceSchema.BoolAttribute)
		if !ok {
			t.Fatalf("key_is_extractable is not a BoolAttribute: %T", nested["key_is_extractable"])
		}
		req := planmodifier.BoolRequest{
			ConfigValue: types.BoolNull(),
			StateValue:  types.BoolValue(true),
			PlanValue:   types.BoolUnknown(),
			State:       nonNullState,
		}
		resp := &planmodifier.BoolResponse{PlanValue: req.PlanValue}
		for _, m := range a.PlanModifiers {
			m.PlanModifyBool(context.Background(), req, resp)
			req.PlanValue = resp.PlanValue
		}
		if !resp.PlanValue.Equal(types.BoolValue(true)) {
			t.Errorf("planned %v, want state's true (no perpetual diff)", resp.PlanValue)
		}
	})
}

// --- Configure() tests ---

func TestProfileResourceConfigure_NilProviderData(t *testing.T) {
	t.Parallel()
	r := &ProfileResource{}
	req := resource.ConfigureRequest{ProviderData: nil}
	var resp resource.ConfigureResponse
	r.Configure(context.Background(), req, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatal("unexpected error for nil ProviderData")
	}
	if r.client != nil {
		t.Error("expected client to remain nil")
	}
}

func TestProfileResourceConfigure_ValidClient(t *testing.T) {
	t.Parallel()
	c, server := createTestClient(t, func(w http.ResponseWriter, r *http.Request) {})
	defer server.Close()

	r := &ProfileResource{}
	req := resource.ConfigureRequest{ProviderData: c}
	var resp resource.ConfigureResponse
	r.Configure(context.Background(), req, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatal("unexpected error for valid client")
	}
	if r.client != c {
		t.Error("expected client to be set")
	}
}

func TestProfileResourceConfigure_ProviderResourceData(t *testing.T) {
	t.Parallel()
	c, server := createTestClient(t, func(w http.ResponseWriter, r *http.Request) {})
	defer server.Close()

	customFactoryCalled := false
	customFactory := func(ctx context.Context, c *sdk.Client) (profileServiceAPI, error) {
		customFactoryCalled = true
		return sdk.NewProfileServiceWithoutDiscovery(c), nil
	}

	r := &ProfileResource{}
	req := resource.ConfigureRequest{
		ProviderData: &resourceConfigData{
			client:            c,
			newProfileService: customFactory,
		},
	}
	var resp resource.ConfigureResponse
	r.Configure(context.Background(), req, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatal("unexpected error for provider resource data")
	}
	if r.client != c {
		t.Fatal("expected configured client to be set")
	}
	if r.newProfileService == nil {
		t.Fatal("expected profile service factory to be set")
	}
	_, err := r.profileService(context.Background())
	if err != nil {
		t.Fatalf("expected profileService init to succeed: %v", err)
	}
	if !customFactoryCalled {
		t.Fatal("expected custom factory to be used")
	}
}

func TestProfileResourceConfigure_WrongType(t *testing.T) {
	t.Parallel()
	r := &ProfileResource{}
	req := resource.ConfigureRequest{ProviderData: "wrong-type"}
	var resp resource.ConfigureResponse
	r.Configure(context.Background(), req, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for wrong ProviderData type")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if d.Summary() == "Unexpected Resource Configure Type" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'Unexpected Resource Configure Type' error")
	}
}

// --- ImportState() tests ---

func TestProfileResourceImportState_ValidPlatforms(t *testing.T) {
	t.Parallel()
	validPlatforms := []string{
		"Android", "Apple iOS", "AppleOsX",
		"Windows 10", "Windows_Rugged", "Linux",
	}

	for _, platform := range validPlatforms {
		t.Run(platform, func(t *testing.T) {
			t.Parallel()
			r := &ProfileResource{}
			ctx := context.Background()

			importID := "12345:" + platform
			req := resource.ImportStateRequest{ID: importID}
			resp := &resource.ImportStateResponse{
				State: emptyResourceState(t),
			}

			r.ImportState(ctx, req, resp)

			if resp.Diagnostics.HasError() {
				var msgs []string
				for _, d := range resp.Diagnostics.Errors() {
					msgs = append(msgs, d.Summary()+": "+d.Detail())
				}
				t.Fatalf("unexpected error for platform '%s': %v", platform, msgs)
			}

			// Verify ID was set.
			var id types.String
			resp.State.GetAttribute(ctx, path.Root("id"), &id)
			if id.ValueString() != "12345" {
				t.Errorf("expected id '12345', got '%s'", id.ValueString())
			}

			// Verify platform was set.
			var plat types.String
			resp.State.GetAttribute(ctx, path.Root("platform"), &plat)
			if plat.ValueString() != platform {
				t.Errorf("expected platform '%s', got '%s'", platform, plat.ValueString())
			}
		})
	}
}

func TestProfileResourceImportState_InvalidCases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		importID string
		errMsg   string
	}{
		{"no_colon", "12345", "Invalid Import ID"},
		{"empty", "", "Invalid Import ID"},
		{"invalid_platform", "12345:ChromeOS", "Invalid Platform"},
		{"extra_colons", "12345:Android:extra", "Invalid Import ID"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := &ProfileResource{}
			ctx := context.Background()

			req := resource.ImportStateRequest{ID: tt.importID}
			resp := &resource.ImportStateResponse{
				State: emptyResourceState(t),
			}

			r.ImportState(ctx, req, resp)

			if !resp.Diagnostics.HasError() {
				t.Fatal("expected error but got none")
			}

			found := false
			for _, d := range resp.Diagnostics.Errors() {
				if d.Summary() == tt.errMsg {
					found = true
					break
				}
			}
			if !found {
				var summaries []string
				for _, d := range resp.Diagnostics.Errors() {
					summaries = append(summaries, d.Summary())
				}
				t.Errorf("expected error '%s', got: %v", tt.errMsg, summaries)
			}
		})
	}
}

// --- Create() tests ---

func TestProfileResourceCreate(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && strings.Contains(r.URL.Path, "/create"):
			_ = json.NewEncoder(w).Encode(99999)
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/profiles/"):
			resp := map[string]interface{}{
				"General": map[string]interface{}{
					"ProfileId":              99999,
					"Name":                   "Test Profile",
					"Description":            "Test description",
					"AssignmentType":         "Auto",
					"ProfileScope":           "Production",
					"ManagedLocationGroupID": 14165,
					"IsActive":               true,
					"ProfileUuid":            "test-uuid-123",
					"ProfileContext":         "Device",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := &ProfileResource{client: c}
	ctx := context.Background()

	plan := createResourcePlan(t, map[string]tftypes.Value{
		"id":                   nullString(),
		"name":                 stringVal("Test Profile"),
		"description":          stringVal("Test description"),
		"platform":             stringVal("Android"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      nullString(),
		"profile_scope":        nullString(),
		"is_active":            nullBool(),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 nullString(),
		"profile_context":      nullString(),
	})

	req := resource.CreateRequest{Plan: plan}
	resp := &resource.CreateResponse{State: emptyResourceState(t)}

	res.Create(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors: %v", msgs)
	}

	// Verify the state was set.
	var model profilemodels.ProfileResourceModel
	resp.State.Get(ctx, &model)

	if model.ID.ValueString() != "99999" {
		t.Errorf("expected ID '99999', got '%s'", model.ID.ValueString())
	}
	if model.AssignmentType.ValueString() != "Auto" {
		t.Errorf("expected AssignmentType 'Auto', got '%s'", model.AssignmentType.ValueString())
	}
	if model.ProfileScope.ValueString() != "Production" {
		t.Errorf("expected ProfileScope 'Production', got '%s'", model.ProfileScope.ValueString())
	}
	if model.IsActive.ValueBool() != true {
		t.Errorf("expected IsActive true, got %v", model.IsActive.ValueBool())
	}
	if model.UUID.ValueString() != "test-uuid-123" {
		t.Errorf("expected UUID 'test-uuid-123', got '%s'", model.UUID.ValueString())
	}
	if model.ProfileContext.ValueString() != "Device" {
		t.Errorf("expected ProfileContext 'Device', got '%s'", model.ProfileContext.ValueString())
	}
}

func TestProfileResourceCreate_WithLockScreenMessage(t *testing.T) {
	t.Parallel()

	var capturedBody map[string]interface{}

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && strings.Contains(r.URL.Path, "/create"):
			_ = json.NewDecoder(r.Body).Decode(&capturedBody)
			_ = json.NewEncoder(w).Encode(88888)
		case r.Method == "GET":
			resp := map[string]interface{}{
				"General": map[string]interface{}{
					"ProfileId": 88888,
					"Name":      "Android Profile",
					"Uuid":      "uuid-android",
				},
				"AndroidForWorkCustomMessages": map[string]interface{}{
					"LockScreenMessage": "Company Device",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := &ProfileResource{client: c}
	ctx := context.Background()

	plan := createResourcePlan(t, map[string]tftypes.Value{
		"id":                   nullString(),
		"name":                 stringVal("Android Profile"),
		"description":          stringVal(""),
		"platform":             stringVal("Android"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      nullString(),
		"profile_scope":        nullString(),
		"is_active":            nullBool(),
		"lock_screen_message":  stringVal("Company Device"),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 nullString(),
		"profile_context":      nullString(),
	})

	req := resource.CreateRequest{Plan: plan}
	resp := &resource.CreateResponse{State: emptyResourceState(t)}

	res.Create(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors: %v", msgs)
	}

	// Verify the Android payload was included in the request.
	if capturedBody == nil {
		t.Fatal("expected request body to be captured")
	}
	androidPayload, ok := capturedBody["AndroidForWorkCustomMessages"]
	if !ok {
		t.Fatal("expected AndroidForWorkCustomMessages in request body")
	}
	payloadMap, ok := androidPayload.(map[string]interface{})
	if !ok {
		t.Fatal("expected AndroidForWorkCustomMessages to be a map")
	}
	if payloadMap["LockScreenMessage"] != "Company Device" {
		t.Errorf("expected LockScreenMessage 'Company Device', got '%v'", payloadMap["LockScreenMessage"])
	}
}

func TestProfileResourceCreate_WithPasscode(t *testing.T) {
	t.Parallel()

	var capturedBody map[string]interface{}

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && strings.Contains(r.URL.Path, "/create"):
			_ = json.NewDecoder(r.Body).Decode(&capturedBody)
			_ = json.NewEncoder(w).Encode(77777)
		case r.Method == "GET":
			resp := map[string]interface{}{
				"General": map[string]interface{}{
					"ProfileId":              77777,
					"Name":                   "macOS Passcode",
					"ProfileUuid":            "uuid-macos-passcode",
					"ProfileContext":         "Device",
					"ManagedLocationGroupID": 14165,
				},
				"Passcode": map[string]interface{}{
					"RequirePasscodeOnDevice":          true,
					"AllowSimpleValue":                 false,
					"RequireAlphanumericValue":         true,
					"MinimumPasscodeLength":            float64(8),
					"MinimumNumberOfComplexCharacters": "1",
					"MaximumPasscodeAge":               "90",
					"AutoLock":                         "5",
					"GracePeriod":                      float64(10),
					"MaxFailedAttempts":                float64(5),
					"pinHistory":                       float64(3),
					"minutesUntilFailedLoginReset":     float64(15),
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := &ProfileResource{client: c}
	ctx := context.Background()

	plan := createResourcePlan(t, map[string]tftypes.Value{
		"id":                  nullString(),
		"name":                stringVal("macOS Passcode"),
		"description":         stringVal("Passcode policy"),
		"platform":            stringVal("AppleOsX"),
		"org_group_id":        stringVal("14165"),
		"assignment_type":     nullString(),
		"profile_scope":       nullString(),
		"is_active":           nullBool(),
		"lock_screen_message": nullString(),
		"passcode": passcodeVal(map[string]tftypes.Value{
			"require_passcode_on_device":           boolVal(true),
			"allow_simple_value":                   boolVal(false),
			"require_alphanumeric_value":           boolVal(true),
			"minimum_passcode_length":              int64Val(8),
			"minimum_number_of_complex_characters": stringVal("1"),
			"maximum_passcode_age":                 stringVal("90"),
			"auto_lock":                            stringVal("5"),
			"grace_period":                         int64Val(10),
			"max_failed_attempts":                  stringVal("5"),
			"pin_history":                          stringVal("3"),
			"minutes_until_failed_login_reset":     int64Val(15),
		}),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 nullString(),
		"profile_context":      nullString(),
	})

	req := resource.CreateRequest{Plan: plan}
	resp := &resource.CreateResponse{State: emptyResourceState(t)}

	res.Create(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors: %v", msgs)
	}

	// Verify the Passcode payload was included in the request.
	if capturedBody == nil {
		t.Fatal("expected request body to be captured")
	}
	passcodePayload, ok := capturedBody["Passcode"]
	if !ok {
		t.Fatal("expected Passcode in request body")
	}
	payloadMap, ok := passcodePayload.(map[string]interface{})
	if !ok {
		t.Fatal("expected Passcode to be a map")
	}
	if payloadMap["RequirePasscodeOnDevice"] != true {
		t.Errorf("expected RequirePasscodeOnDevice true, got %v", payloadMap["RequirePasscodeOnDevice"])
	}
	if payloadMap["AllowSimpleValue"] != false {
		t.Errorf("expected AllowSimpleValue false, got %v", payloadMap["AllowSimpleValue"])
	}
	if payloadMap["MinimumPasscodeLength"] != float64(8) {
		t.Errorf("expected MinimumPasscodeLength 8, got %v", payloadMap["MinimumPasscodeLength"])
	}
	if payloadMap["MaxFailedAttempts"] != float64(5) {
		t.Errorf("expected MaxFailedAttempts 5, got %v", payloadMap["MaxFailedAttempts"])
	}

	// Verify the state has passcode fields populated from the read-back.
	var model profilemodels.ProfileResourceModel
	resp.State.Get(ctx, &model)

	if model.ID.ValueString() != "77777" {
		t.Errorf("expected ID '77777', got '%s'", model.ID.ValueString())
	}
	if model.Passcode == nil {
		t.Fatal("expected Passcode to be set in state after read-back")
	}
	if model.Passcode.RequirePasscodeOnDevice.ValueBool() != true {
		t.Errorf("expected RequirePasscodeOnDevice true, got %v", model.Passcode.RequirePasscodeOnDevice.ValueBool())
	}
	if model.Passcode.MaxFailedAttempts.ValueString() != "5" {
		t.Errorf("expected MaxFailedAttempts \"5\", got %q", model.Passcode.MaxFailedAttempts.ValueString())
	}
	if model.Passcode.PinHistory.ValueString() != "3" {
		t.Errorf("expected PinHistory \"3\", got %q", model.Passcode.PinHistory.ValueString())
	}
	if model.Passcode.MinutesUntilFailedLoginReset.ValueInt64() != 15 {
		t.Errorf("expected MinutesUntilFailedLoginReset 15, got %d", model.Passcode.MinutesUntilFailedLoginReset.ValueInt64())
	}
}

func TestNormalizePasscodeUnknownsToNull(t *testing.T) {
	t.Parallel()

	t.Run("all unknown becomes all null", func(t *testing.T) {
		t.Parallel()
		p := &profilemodels.PasscodeModel{
			RequirePasscodeOnDevice:          types.BoolUnknown(),
			AllowSimpleValue:                 types.BoolUnknown(),
			RequireAlphanumericValue:         types.BoolUnknown(),
			MinimumPasscodeLength:            types.Int64Unknown(),
			MinimumNumberOfComplexCharacters: types.StringUnknown(),
			MaximumPasscodeAge:               types.StringUnknown(),
			AutoLock:                         types.StringUnknown(),
			GracePeriod:                      types.Int64Unknown(),
			MaxFailedAttempts:                types.StringUnknown(),
			PinHistory:                       types.StringUnknown(),
			MinutesUntilFailedLoginReset:     types.Int64Unknown(),
		}
		profilestate.NormalizePasscodeUnknownsToNull(p)
		assertPasscodeHasNoUnknowns(t, p)
	})

	t.Run("preserves known values", func(t *testing.T) {
		t.Parallel()
		p := &profilemodels.PasscodeModel{
			RequirePasscodeOnDevice: types.BoolValue(true),
			AllowSimpleValue:        types.BoolUnknown(),
			MinimumPasscodeLength:   types.Int64Value(8),
			AutoLock:                types.StringUnknown(),
			MaximumPasscodeAge:      types.StringValue("30"),
		}
		profilestate.NormalizePasscodeUnknownsToNull(p)
		if !p.RequirePasscodeOnDevice.ValueBool() {
			t.Error("expected RequirePasscodeOnDevice to stay true")
		}
		if p.MinimumPasscodeLength.ValueInt64() != 8 {
			t.Errorf("expected MinimumPasscodeLength 8, got %d", p.MinimumPasscodeLength.ValueInt64())
		}
		if p.MaximumPasscodeAge.ValueString() != "30" {
			t.Errorf("expected MaximumPasscodeAge 30, got %s", p.MaximumPasscodeAge.ValueString())
		}
		if !p.AllowSimpleValue.IsNull() || !p.AutoLock.IsNull() {
			t.Error("expected unknown fields normalized to null")
		}
	})
}

func assertPasscodeHasNoUnknowns(t *testing.T, p *profilemodels.PasscodeModel) {
	t.Helper()
	if p == nil {
		t.Fatal("passcode is nil")
	}
	if p.RequirePasscodeOnDevice.IsUnknown() {
		t.Error("RequirePasscodeOnDevice still unknown")
	}
	if p.AllowSimpleValue.IsUnknown() {
		t.Error("AllowSimpleValue still unknown")
	}
	if p.RequireAlphanumericValue.IsUnknown() {
		t.Error("RequireAlphanumericValue still unknown")
	}
	if p.MinimumPasscodeLength.IsUnknown() {
		t.Error("MinimumPasscodeLength still unknown")
	}
	if p.MinimumNumberOfComplexCharacters.IsUnknown() {
		t.Error("MinimumNumberOfComplexCharacters still unknown")
	}
	if p.MaximumPasscodeAge.IsUnknown() {
		t.Error("MaximumPasscodeAge still unknown")
	}
	if p.AutoLock.IsUnknown() {
		t.Error("AutoLock still unknown")
	}
	if p.GracePeriod.IsUnknown() {
		t.Error("GracePeriod still unknown")
	}
	if p.MaxFailedAttempts.IsUnknown() {
		t.Error("MaxFailedAttempts still unknown")
	}
	if p.PinHistory.IsUnknown() {
		t.Error("PinHistory still unknown")
	}
	if p.MinutesUntilFailedLoginReset.IsUnknown() {
		t.Error("MinutesUntilFailedLoginReset still unknown")
	}
}

// TestProfileResourceCreate_PostCreateGetFails_PasscodeUnknownsResolved covers the
// path where Create succeeds but the follow-up Get fails: passcode fields that were
// unknown in the plan must be written to state as known null, not unknown.
func TestProfileResourceCreate_PostCreateGetFails_PasscodeUnknownsResolved(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && strings.Contains(r.URL.Path, "/create"):
			_ = json.NewEncoder(w).Encode(88888)
		case r.Method == "GET":
			// Use 404 so the client does not spend several seconds retrying 5xx responses.
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "not found"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := newProfileResourceWithRegistry(t, c)
	ctx := context.Background()

	ub := tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue)
	un := tftypes.NewValue(tftypes.Number, tftypes.UnknownValue)
	us := tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	pt := passcodeObjectType()
	passcodePlan := tftypes.NewValue(pt, map[string]tftypes.Value{
		"require_passcode_on_device":           tftypes.NewValue(tftypes.Bool, true),
		"allow_simple_value":                   ub,
		"require_alphanumeric_value":           ub,
		"minimum_passcode_length":              un,
		"minimum_number_of_complex_characters": us,
		"maximum_passcode_age":                 us,
		"auto_lock":                            us,
		"grace_period":                         un,
		"max_failed_attempts":                  us,
		"pin_history":                          us,
		"minutes_until_failed_login_reset":     un,
	})

	plan := createResourcePlan(t, map[string]tftypes.Value{
		"id":                   nullString(),
		"name":                 stringVal("macOS Passcode"),
		"description":          stringVal("Passcode policy"),
		"platform":             stringVal("AppleOsX"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      nullString(),
		"profile_scope":        nullString(),
		"is_active":            nullBool(),
		"lock_screen_message":  nullString(),
		"passcode":             passcodePlan,
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 nullString(),
		"profile_context":      nullString(),
	})

	req := resource.CreateRequest{Plan: plan}
	resp := &resource.CreateResponse{State: emptyResourceState(t)}

	res.Create(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors after Create (state must not contain unknown passcode values): %v", msgs)
	}

	var model profilemodels.ProfileResourceModel
	resp.State.Get(ctx, &model)
	if model.Passcode == nil {
		t.Fatal("expected Passcode block in state")
	}
	assertPasscodeHasNoUnknowns(t, model.Passcode)
}

func TestProfileResourceCreate_APIError(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "server error"})
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := &ProfileResource{client: c}
	ctx := context.Background()

	plan := createResourcePlan(t, map[string]tftypes.Value{
		"id":                   nullString(),
		"name":                 stringVal("Test Profile"),
		"description":          stringVal("desc"),
		"platform":             stringVal("Android"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      nullString(),
		"profile_scope":        nullString(),
		"is_active":            nullBool(),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 nullString(),
		"profile_context":      nullString(),
	})

	req := resource.CreateRequest{Plan: plan}
	resp := &resource.CreateResponse{State: emptyResourceState(t)}

	res.Create(ctx, req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for API failure")
	}
}

// --- Read() tests ---

func TestProfileResourceRead(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"General": map[string]interface{}{
				"ProfileId":              12345,
				"Name":                   "Read Profile",
				"Description":            "Read description",
				"AssignmentType":         "Optional",
				"ProfileScope":           "Staging",
				"ManagedLocationGroupID": 14165,
				"IsActive":               false,
				"ProfileUuid":            "read-uuid",
				"ProfileContext":         "User",
			},
			"AndroidForWorkCustomMessages": map[string]interface{}{
				"LockScreenMessage": "Read message",
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := newProfileResourceWithRegistry(t, c, discoveryEntry(12345, sdk.PlatformAndroid))
	ctx := context.Background()

	state := createResourceState(t, map[string]tftypes.Value{
		"id":                   stringVal("12345"),
		"name":                 stringVal("Old Name"),
		"description":          stringVal("Old description"),
		"platform":             stringVal("Android"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      stringVal("Auto"),
		"profile_scope":        stringVal("Production"),
		"is_active":            boolVal(true),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 stringVal("old-uuid"),
		"profile_context":      stringVal("Device"),
	})

	req := resource.ReadRequest{State: state}
	resp := &resource.ReadResponse{State: state}

	res.Read(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors: %v", msgs)
	}

	// Verify the state was updated.
	var model profilemodels.ProfileResourceModel
	resp.State.Get(ctx, &model)

	if model.Name.ValueString() != "Read Profile" {
		t.Errorf("expected Name 'Read Profile', got '%s'", model.Name.ValueString())
	}
	if model.Description.ValueString() != "Read description" {
		t.Errorf("expected Description 'Read description', got '%s'", model.Description.ValueString())
	}
	if model.AssignmentType.ValueString() != "Optional" {
		t.Errorf("expected AssignmentType 'Optional', got '%s'", model.AssignmentType.ValueString())
	}
	if model.IsActive.ValueBool() != false {
		t.Errorf("expected IsActive false, got %v", model.IsActive.ValueBool())
	}
	if model.UUID.ValueString() != "read-uuid" {
		t.Errorf("expected UUID 'read-uuid', got '%s'", model.UUID.ValueString())
	}
	if model.LockScreenMessage.ValueString() != "Read message" {
		t.Errorf("expected LockScreenMessage 'Read message', got '%s'", model.LockScreenMessage.ValueString())
	}
}

func TestProfileResourceRead_WithPasscode(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"General": map[string]interface{}{
				"ProfileId":              77777,
				"Name":                   "macOS Passcode",
				"Description":            "Passcode policy",
				"AssignmentType":         "Auto",
				"ProfileScope":           "Production",
				"ManagedLocationGroupID": 14165,
				"IsActive":               true,
				"ProfileUuid":            "uuid-macos-read",
				"ProfileContext":         "Device",
			},
			"Passcode": map[string]interface{}{
				"RequirePasscodeOnDevice":          true,
				"AllowSimpleValue":                 false,
				"RequireAlphanumericValue":         true,
				"MinimumPasscodeLength":            float64(6),
				"MinimumNumberOfComplexCharacters": "2",
				"MaximumPasscodeAge":               "30",
				"AutoLock":                         "1",
				"GracePeriod":                      float64(5),
				"MaxFailedAttempts":                float64(10),
				"pinHistory":                       float64(5),
				"minutesUntilFailedLoginReset":     float64(30),
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := newProfileResourceWithRegistry(t, c, discoveryEntry(77777, sdk.PlatformAppleOsX))
	ctx := context.Background()

	state := createResourceState(t, map[string]tftypes.Value{
		"id":                   stringVal("77777"),
		"name":                 stringVal("Old Name"),
		"description":          stringVal("Old description"),
		"platform":             stringVal("AppleOsX"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      stringVal("Auto"),
		"profile_scope":        stringVal("Production"),
		"is_active":            boolVal(true),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 stringVal("old-uuid"),
		"profile_context":      stringVal("Device"),
	})

	req := resource.ReadRequest{State: state}
	resp := &resource.ReadResponse{State: state}

	res.Read(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors: %v", msgs)
	}

	var model profilemodels.ProfileResourceModel
	resp.State.Get(ctx, &model)

	if model.Name.ValueString() != "macOS Passcode" {
		t.Errorf("expected Name 'macOS Passcode', got '%s'", model.Name.ValueString())
	}
	if model.Passcode == nil {
		t.Fatal("expected Passcode to be populated from API response")
	}
	if model.Passcode.RequirePasscodeOnDevice.ValueBool() != true {
		t.Errorf("expected RequirePasscodeOnDevice true, got %v", model.Passcode.RequirePasscodeOnDevice.ValueBool())
	}
	if model.Passcode.AllowSimpleValue.ValueBool() != false {
		t.Errorf("expected AllowSimpleValue false, got %v", model.Passcode.AllowSimpleValue.ValueBool())
	}
	if model.Passcode.MinimumPasscodeLength.ValueInt64() != 6 {
		t.Errorf("expected MinimumPasscodeLength 6, got %d", model.Passcode.MinimumPasscodeLength.ValueInt64())
	}
	if model.Passcode.MinimumNumberOfComplexCharacters.ValueString() != "2" {
		t.Errorf("expected MinimumNumberOfComplexCharacters '2', got '%s'", model.Passcode.MinimumNumberOfComplexCharacters.ValueString())
	}
	if model.Passcode.MaximumPasscodeAge.ValueString() != "30" {
		t.Errorf("expected MaximumPasscodeAge '30', got '%s'", model.Passcode.MaximumPasscodeAge.ValueString())
	}
	if model.Passcode.AutoLock.ValueString() != "1" {
		t.Errorf("expected AutoLock '1', got '%s'", model.Passcode.AutoLock.ValueString())
	}
	if model.Passcode.GracePeriod.ValueInt64() != 5 {
		t.Errorf("expected GracePeriod 5, got %d", model.Passcode.GracePeriod.ValueInt64())
	}
	if model.Passcode.MaxFailedAttempts.ValueString() != "10" {
		t.Errorf("expected MaxFailedAttempts \"10\", got %q", model.Passcode.MaxFailedAttempts.ValueString())
	}
	if model.Passcode.PinHistory.ValueString() != "5" {
		t.Errorf("expected PinHistory \"5\", got %q", model.Passcode.PinHistory.ValueString())
	}
	if model.Passcode.MinutesUntilFailedLoginReset.ValueInt64() != 30 {
		t.Errorf("expected MinutesUntilFailedLoginReset 30, got %d", model.Passcode.MinutesUntilFailedLoginReset.ValueInt64())
	}
}

// TestProfileResourceRead_PasscodeFieldsPreservedFromPriorState verifies that
// when the UEM API omits AutoLock, MaximumPasscodeAge, and
// MinimumNumberOfComplexCharacters from the GET response, the Read function
// preserves those values from the prior Terraform state instead of zeroing
// them out and causing a perpetual plan diff.
// internal-ticket removed mergePasscodeWithPriorState (row #62 of the B16
// audit): AutoLock/MaximumPasscodeAge/MinimumNumberOfComplexCharacters no
// longer fall back to the prior state when UEM's response omits them.
// Renamed from TestProfileResourceRead_PasscodeFieldsPreservedFromPriorState,
// which asserted the removed fallback behavior.
func TestProfileResourceRead_PasscodeFieldsOmittedByAPIAreNull(t *testing.T) {
	t.Parallel()

	// API returns a Passcode block but omits the three fields the UEM API
	// does not echo back after creation.
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"General": map[string]interface{}{
				"ProfileId":              77777,
				"Name":                   "macOS Passcode",
				"Description":            "Passcode policy",
				"AssignmentType":         "Auto",
				"ProfileScope":           "Production",
				"ManagedLocationGroupID": 14165,
				"IsActive":               true,
				"ProfileUuid":            "uuid-macos-read",
				"ProfileContext":         "Device",
			},
			"Passcode": map[string]interface{}{
				"RequirePasscodeOnDevice":      true,
				"AllowSimpleValue":             false,
				"RequireAlphanumericValue":     true,
				"MinimumPasscodeLength":        float64(8),
				"GracePeriod":                  float64(10),
				"MaxFailedAttempts":            float64(5),
				"pinHistory":                   float64(3),
				"minutesUntilFailedLoginReset": float64(15),
				// AutoLock, MaximumPasscodeAge, MinimumNumberOfComplexCharacters
				// are intentionally absent — this is the documented UEM API bug.
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := newProfileResourceWithRegistry(t, c, discoveryEntry(77777, sdk.PlatformAppleOsX))
	ctx := context.Background()

	// Prior state contains the values that were sent during Create.
	state := createResourceState(t, map[string]tftypes.Value{
		"id":                  stringVal("77777"),
		"name":                stringVal("macOS Passcode"),
		"description":         stringVal("Passcode policy"),
		"platform":            stringVal("AppleOsX"),
		"org_group_id":        stringVal("14165"),
		"assignment_type":     stringVal("Auto"),
		"profile_scope":       stringVal("Production"),
		"is_active":           boolVal(true),
		"lock_screen_message": nullString(),
		"passcode": passcodeVal(map[string]tftypes.Value{
			"require_passcode_on_device":           boolVal(true),
			"allow_simple_value":                   boolVal(false),
			"require_alphanumeric_value":           boolVal(true),
			"minimum_passcode_length":              int64Val(8),
			"minimum_number_of_complex_characters": stringVal("2"),
			"maximum_passcode_age":                 stringVal("90"),
			"auto_lock":                            stringVal("5"),
			"grace_period":                         int64Val(10),
			"max_failed_attempts":                  stringVal("5"),
			"pin_history":                          stringVal("3"),
			"minutes_until_failed_login_reset":     int64Val(15),
		}),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 stringVal("uuid-macos-read"),
		"profile_context":      stringVal("Device"),
	})

	req := resource.ReadRequest{State: state}
	resp := &resource.ReadResponse{State: state}

	res.Read(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors: %v", msgs)
	}

	var model profilemodels.ProfileResourceModel
	resp.State.Get(ctx, &model)

	if model.Passcode == nil {
		t.Fatal("expected Passcode to be populated in state after read")
	}

	// Fields returned by the API should reflect the API response.
	if model.Passcode.RequirePasscodeOnDevice.ValueBool() != true {
		t.Errorf("RequirePasscodeOnDevice: got %v, want true", model.Passcode.RequirePasscodeOnDevice.ValueBool())
	}
	if model.Passcode.MinimumPasscodeLength.ValueInt64() != 8 {
		t.Errorf("MinimumPasscodeLength: got %d, want 8", model.Passcode.MinimumPasscodeLength.ValueInt64())
	}
	if model.Passcode.MaxFailedAttempts.ValueString() != "5" {
		t.Errorf("MaxFailedAttempts: got %q, want \"5\"", model.Passcode.MaxFailedAttempts.ValueString())
	}

	// Fields omitted by the API (the three fields documented as not returned
	// by the UEM GET endpoint) are now null, not preserved from prior state.
	if !model.Passcode.AutoLock.IsNull() {
		t.Errorf("AutoLock: got %q, want null (no prior-state fallback)", model.Passcode.AutoLock.ValueString())
	}
	if !model.Passcode.MaximumPasscodeAge.IsNull() {
		t.Errorf("MaximumPasscodeAge: got %q, want null (no prior-state fallback)", model.Passcode.MaximumPasscodeAge.ValueString())
	}
	if !model.Passcode.MinimumNumberOfComplexCharacters.IsNull() {
		t.Errorf("MinimumNumberOfComplexCharacters: got %q, want null (no prior-state fallback)", model.Passcode.MinimumNumberOfComplexCharacters.ValueString())
	}
}

// TestProfileResourceRead_PasscodeFieldsReturnedByAPI verifies that when the
// UEM API does include AutoLock, MaximumPasscodeAge, and
// MinimumNumberOfComplexCharacters in the response, those API values take
// precedence over prior state (normal case when the API is fixed or returns
// them for other platforms).
func TestProfileResourceRead_PasscodeFieldsReturnedByAPI(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"General": map[string]interface{}{
				"ProfileId":              77777,
				"Name":                   "macOS Passcode",
				"ManagedLocationGroupID": 14165,
				"ProfileUuid":            "uuid-macos-read",
				"ProfileContext":         "Device",
			},
			"Passcode": map[string]interface{}{
				"RequirePasscodeOnDevice":          true,
				"MinimumPasscodeLength":            float64(6),
				"MinimumNumberOfComplexCharacters": "3",
				"MaximumPasscodeAge":               "60",
				"AutoLock":                         "10",
				"GracePeriod":                      float64(5),
				"MaxFailedAttempts":                float64(10),
				"pinHistory":                       float64(5),
				"minutesUntilFailedLoginReset":     float64(30),
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := newProfileResourceWithRegistry(t, c, discoveryEntry(77777, sdk.PlatformAppleOsX))
	ctx := context.Background()

	// Prior state has different values for the three fields.
	state := createResourceState(t, map[string]tftypes.Value{
		"id":                  stringVal("77777"),
		"name":                stringVal("macOS Passcode"),
		"description":         nullString(),
		"platform":            stringVal("AppleOsX"),
		"org_group_id":        stringVal("14165"),
		"assignment_type":     nullString(),
		"profile_scope":       nullString(),
		"is_active":           boolVal(true),
		"lock_screen_message": nullString(),
		"passcode": passcodeVal(map[string]tftypes.Value{
			"minimum_number_of_complex_characters": stringVal("1"),
			"maximum_passcode_age":                 stringVal("30"),
			"auto_lock":                            stringVal("2"),
		}),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 stringVal("uuid-macos-read"),
		"profile_context":      stringVal("Device"),
	})

	req := resource.ReadRequest{State: state}
	resp := &resource.ReadResponse{State: state}

	res.Read(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors: %v", msgs)
	}

	var model profilemodels.ProfileResourceModel
	resp.State.Get(ctx, &model)

	if model.Passcode == nil {
		t.Fatal("expected Passcode to be populated in state after read")
	}

	// When the API returns these fields, API values must win over prior state.
	if model.Passcode.AutoLock.ValueString() != "10" {
		t.Errorf("AutoLock: got %q, want %q (API value should take precedence)", model.Passcode.AutoLock.ValueString(), "10")
	}
	if model.Passcode.MaximumPasscodeAge.ValueString() != "60" {
		t.Errorf("MaximumPasscodeAge: got %q, want %q (API value should take precedence)", model.Passcode.MaximumPasscodeAge.ValueString(), "60")
	}
	if model.Passcode.MinimumNumberOfComplexCharacters.ValueString() != "3" {
		t.Errorf("MinimumNumberOfComplexCharacters: got %q, want %q (API value should take precedence)", model.Passcode.MinimumNumberOfComplexCharacters.ValueString(), "3")
	}
}

// TestProfileResourceRead_PasscodeNoPriorState verifies that when the prior
// state has no passcode block at all (e.g. first Read after import), the Read
// function does not panic and returns whatever the API provides.
func TestProfileResourceRead_PasscodeNoPriorState(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"General": map[string]interface{}{
				"ProfileId":              77777,
				"Name":                   "macOS Passcode",
				"ManagedLocationGroupID": 14165,
				"ProfileUuid":            "uuid-macos-read",
				"ProfileContext":         "Device",
			},
			"Passcode": map[string]interface{}{
				"RequirePasscodeOnDevice": true,
				"MinimumPasscodeLength":   float64(8),
				"GracePeriod":             float64(10),
				// AutoLock, MaximumPasscodeAge, MinimumNumberOfComplexCharacters absent.
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := newProfileResourceWithRegistry(t, c, discoveryEntry(77777, sdk.PlatformAppleOsX))
	ctx := context.Background()

	// Prior state has passcode = null (e.g., just after import).
	state := createResourceState(t, map[string]tftypes.Value{
		"id":                   stringVal("77777"),
		"name":                 stringVal("macOS Passcode"),
		"description":          nullString(),
		"platform":             stringVal("AppleOsX"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      nullString(),
		"profile_scope":        nullString(),
		"is_active":            nullBool(),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 nullString(),
		"profile_context":      nullString(),
	})

	req := resource.ReadRequest{State: state}
	resp := &resource.ReadResponse{State: state}

	res.Read(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors: %v", msgs)
	}

	var model profilemodels.ProfileResourceModel
	resp.State.Get(ctx, &model)

	if model.Passcode == nil {
		t.Fatal("expected Passcode to be set from API response even when prior state was null")
	}
	if model.Passcode.RequirePasscodeOnDevice.ValueBool() != true {
		t.Errorf("RequirePasscodeOnDevice: got %v, want true", model.Passcode.RequirePasscodeOnDevice.ValueBool())
	}
	// Fields absent from API and from prior state should be null/zero — not panic.
	if !model.Passcode.AutoLock.IsNull() && model.Passcode.AutoLock.ValueString() != "" {
		t.Errorf("AutoLock: expected null or empty when absent from API and prior state, got %q", model.Passcode.AutoLock.ValueString())
	}
}

func TestProfileResourceRead_APIError(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "server error"})
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := &ProfileResource{client: c}
	ctx := context.Background()

	state := createResourceState(t, map[string]tftypes.Value{
		"id":                   stringVal("12345"),
		"name":                 stringVal("Test"),
		"description":          nullString(),
		"platform":             stringVal("Android"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      stringVal("Auto"),
		"profile_scope":        stringVal("Production"),
		"is_active":            boolVal(true),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 nullString(),
		"profile_context":      nullString(),
	})

	req := resource.ReadRequest{State: state}
	resp := &resource.ReadResponse{State: state}

	res.Read(ctx, req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for API failure")
	}
}

func TestProfileResourceRead_InvalidID(t *testing.T) {
	t.Parallel()

	res := &ProfileResource{client: nil}
	ctx := context.Background()

	state := createResourceState(t, map[string]tftypes.Value{
		"id":                   stringVal("not-a-number"),
		"name":                 stringVal("Test"),
		"description":          nullString(),
		"platform":             stringVal("Android"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      stringVal("Auto"),
		"profile_scope":        stringVal("Production"),
		"is_active":            boolVal(true),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 nullString(),
		"profile_context":      nullString(),
	})

	req := resource.ReadRequest{State: state}
	resp := &resource.ReadResponse{State: state}

	res.Read(ctx, req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for invalid profile ID")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if d.Summary() == "Parse Error" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'Parse Error' diagnostic")
	}
}

// --- Update() tests ---

func TestProfileResourceUpdate(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && strings.Contains(r.URL.Path, "/update"):
			// Update returns a profile (resource ignores it).
			_ = json.NewEncoder(w).Encode(map[string]interface{}{})
		case r.Method == "GET":
			resp := map[string]interface{}{
				"General": map[string]interface{}{
					"ProfileId":      12345,
					"Name":           "Updated Profile",
					"ProfileUuid":    "updated-uuid",
					"ProfileContext": "Device",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := &ProfileResource{client: c}
	ctx := context.Background()

	plan := createResourcePlan(t, map[string]tftypes.Value{
		"id":                   stringVal("12345"),
		"name":                 stringVal("Updated Profile"),
		"description":          stringVal("Updated description"),
		"platform":             stringVal("Android"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      stringVal("Optional"),
		"profile_scope":        stringVal("Staging"),
		"is_active":            boolVal(false),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 stringVal("old-uuid"),
		"profile_context":      stringVal("Device"),
	})

	req := resource.UpdateRequest{Plan: plan, State: emptyResourceState(t)}
	resp := &resource.UpdateResponse{State: emptyResourceState(t)}

	res.Update(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors: %v", msgs)
	}

	// Verify computed fields were updated from read-back.
	var model profilemodels.ProfileResourceModel
	resp.State.Get(ctx, &model)

	if model.UUID.ValueString() != "updated-uuid" {
		t.Errorf("expected UUID 'updated-uuid', got '%s'", model.UUID.ValueString())
	}
}

func TestProfileResourceUpdate_PreservesExistingPayloads(t *testing.T) {
	// zf1 part 1: Update is now a read-modify-write against UEM's
	// full-replace Update endpoint. The live entity is fetched, and only
	// the sections this provider models are overlaid from the plan-built
	// entity; unmodeled sections returned by Get (e.g.
	// AndroidForWorkApplicationControl, which this provider's schema does
	// not expose) MUST be echoed back unchanged in the Update payload, or
	// UEM would silently wipe them server-side. We still also verify the
	// modeled-but-null invariant: AndroidForWorkCustomMessages is omitted
	// from the sent entity when lock_screen_message is null in the plan.
	t.Parallel()

	var capturedBody map[string]interface{}

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && strings.Contains(r.URL.Path, "/update"):
			_ = json.NewDecoder(r.Body).Decode(&capturedBody)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{})
		case r.Method == "GET":
			resp := map[string]interface{}{
				"General": map[string]interface{}{
					"ProfileId":      12345,
					"Name":           "Updated Profile",
					"ProfileUuid":    "updated-uuid",
					"ProfileContext": "Device",
				},
				// Unmodeled Android section — must survive Update unchanged.
				"AndroidForWorkApplicationControl": map[string]interface{}{
					"DisableAccessToBlacklistedApps": true,
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := &ProfileResource{client: c}
	ctx := context.Background()

	plan := createResourcePlan(t, map[string]tftypes.Value{
		"id":                   stringVal("12345"),
		"name":                 stringVal("Updated Profile"),
		"description":          stringVal("Updated description"),
		"platform":             stringVal("Android"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      stringVal("Optional"),
		"profile_scope":        stringVal("Staging"),
		"is_active":            boolVal(false),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 stringVal("old-uuid"),
		"profile_context":      stringVal("Device"),
	})

	req := resource.UpdateRequest{Plan: plan, State: emptyResourceState(t)}
	resp := &resource.UpdateResponse{State: emptyResourceState(t)}

	res.Update(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors: %v", msgs)
	}

	if capturedBody == nil {
		t.Fatal("expected request body to be captured")
	}
	if _, ok := capturedBody["AndroidForWorkCustomMessages"]; ok {
		t.Fatal("did not expect AndroidForWorkCustomMessages when lock_screen_message is null")
	}
	unmodeled, ok := capturedBody["AndroidForWorkApplicationControl"]
	if !ok {
		t.Fatal("expected unmodeled AndroidForWorkApplicationControl section to be preserved from the live entity")
	}
	unmodeledMap, ok := unmodeled.(map[string]interface{})
	if !ok {
		t.Fatalf("expected AndroidForWorkApplicationControl to be a map, got %T", unmodeled)
	}
	if v, ok := unmodeledMap["DisableAccessToBlacklistedApps"].(bool); !ok || !v {
		t.Fatalf("expected AndroidForWorkApplicationControl.DisableAccessToBlacklistedApps to survive unchanged, got %v", unmodeledMap["DisableAccessToBlacklistedApps"])
	}
}

func TestProfileResourceUpdate_WithLockScreenMessage(t *testing.T) {
	t.Parallel()

	var capturedBody map[string]interface{}

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && strings.Contains(r.URL.Path, "/update"):
			_ = json.NewDecoder(r.Body).Decode(&capturedBody)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{})
		case r.Method == "GET":
			resp := map[string]interface{}{
				"General": map[string]interface{}{
					"ProfileId": 12345,
					"Name":      "Updated",
					"Uuid":      "uuid-updated",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := &ProfileResource{client: c}
	ctx := context.Background()

	plan := createResourcePlan(t, map[string]tftypes.Value{
		"id":                   stringVal("12345"),
		"name":                 stringVal("Updated"),
		"description":          stringVal(""),
		"platform":             stringVal("Android"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      stringVal("Auto"),
		"profile_scope":        stringVal("Production"),
		"is_active":            boolVal(true),
		"lock_screen_message":  stringVal("Updated Lock Message"),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 stringVal("old-uuid"),
		"profile_context":      stringVal("Device"),
	})

	req := resource.UpdateRequest{Plan: plan, State: emptyResourceState(t)}
	resp := &resource.UpdateResponse{State: emptyResourceState(t)}

	res.Update(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors: %v", msgs)
	}

	// Verify Android payload was included.
	if capturedBody == nil {
		t.Fatal("expected request body to be captured")
	}
	androidPayload, ok := capturedBody["AndroidForWorkCustomMessages"]
	if !ok {
		t.Fatal("expected AndroidForWorkCustomMessages in update request")
	}
	payloadMap, ok := androidPayload.(map[string]interface{})
	if !ok {
		t.Fatal("expected AndroidForWorkCustomMessages to be a map")
	}
	if payloadMap["LockScreenMessage"] != "Updated Lock Message" {
		t.Errorf("expected LockScreenMessage 'Updated Lock Message', got '%v'", payloadMap["LockScreenMessage"])
	}

	// Verify ProfileId was included in General section.
	general, ok := capturedBody["General"]
	if !ok {
		t.Fatal("expected General in update request")
	}
	generalMap, ok := general.(map[string]interface{})
	if !ok {
		t.Fatal("expected General to be a map")
	}
	if generalMap["ProfileId"] != float64(12345) {
		t.Errorf("expected ProfileId 12345, got %v", generalMap["ProfileId"])
	}
}

func TestProfileResourceUpdate_WithPasscode(t *testing.T) {
	t.Parallel()

	var capturedBody map[string]interface{}

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && strings.Contains(r.URL.Path, "/update"):
			_ = json.NewDecoder(r.Body).Decode(&capturedBody)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{})
		case r.Method == "GET":
			resp := map[string]interface{}{
				"General": map[string]interface{}{
					"ProfileId":      77777,
					"Name":           "Updated Passcode",
					"ProfileUuid":    "uuid-updated-passcode",
					"ProfileContext": "Device",
				},
				"Passcode": map[string]interface{}{
					"RequirePasscodeOnDevice": true,
					"MinimumPasscodeLength":   float64(10),
					"MaxFailedAttempts":       float64(3),
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := &ProfileResource{client: c}
	ctx := context.Background()

	plan := createResourcePlan(t, map[string]tftypes.Value{
		"id":                  stringVal("77777"),
		"name":                stringVal("Updated Passcode"),
		"description":         stringVal("Updated policy"),
		"platform":            stringVal("AppleOsX"),
		"org_group_id":        stringVal("14165"),
		"assignment_type":     stringVal("Auto"),
		"profile_scope":       stringVal("Production"),
		"is_active":           boolVal(true),
		"lock_screen_message": nullString(),
		"passcode": passcodeVal(map[string]tftypes.Value{
			"require_passcode_on_device": boolVal(true),
			"minimum_passcode_length":    int64Val(10),
			"max_failed_attempts":        stringVal("3"),
		}),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 stringVal("old-uuid"),
		"profile_context":      stringVal("Device"),
	})

	req := resource.UpdateRequest{Plan: plan, State: emptyResourceState(t)}
	resp := &resource.UpdateResponse{State: emptyResourceState(t)}

	res.Update(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors: %v", msgs)
	}

	// Verify the Passcode payload was included in the update request.
	if capturedBody == nil {
		t.Fatal("expected request body to be captured")
	}
	passcodePayload, ok := capturedBody["Passcode"]
	if !ok {
		t.Fatal("expected Passcode in update request body")
	}
	payloadMap, ok := passcodePayload.(map[string]interface{})
	if !ok {
		t.Fatal("expected Passcode to be a map")
	}
	if payloadMap["RequirePasscodeOnDevice"] != true {
		t.Errorf("expected RequirePasscodeOnDevice true, got %v", payloadMap["RequirePasscodeOnDevice"])
	}
	if payloadMap["MinimumPasscodeLength"] != float64(10) {
		t.Errorf("expected MinimumPasscodeLength 10, got %v", payloadMap["MinimumPasscodeLength"])
	}
	if payloadMap["MaxFailedAttempts"] != float64(3) {
		t.Errorf("expected MaxFailedAttempts 3, got %v", payloadMap["MaxFailedAttempts"])
	}

	// Verify ProfileId was included in General section.
	general, ok := capturedBody["General"]
	if !ok {
		t.Fatal("expected General in update request")
	}
	generalMap, ok := general.(map[string]interface{})
	if !ok {
		t.Fatal("expected General to be a map")
	}
	if generalMap["ProfileId"] != float64(77777) {
		t.Errorf("expected ProfileId 77777, got %v", generalMap["ProfileId"])
	}
	// Verify ProfileContext is set for AppleOsX.
	if generalMap["ProfileContext"] != "Device" {
		t.Errorf("expected ProfileContext 'Device', got %v", generalMap["ProfileContext"])
	}
}

func TestProfileResourceCreate_WithCustomSettings(t *testing.T) {
	t.Parallel()

	var (
		mu           sync.Mutex
		capturedBody map[string]interface{}
	)

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && strings.Contains(r.URL.Path, "/create"):
			mu.Lock()
			_ = json.NewDecoder(r.Body).Decode(&capturedBody)
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(66666)
		case r.Method == "GET":
			resp := map[string]interface{}{
				"General": map[string]interface{}{
					"ProfileId":      66666,
					"Name":           "macOS Custom Settings",
					"ProfileUuid":    "uuid-custom-settings",
					"ProfileContext": "Device",
				},
				"CustomSettingsList": []interface{}{
					map[string]interface{}{"CustomSettings": "<dict><key>PayloadType</key><string>com.apple.screensaver</string></dict>"},
					map[string]interface{}{"CustomSettings": "<dict><key>PayloadType</key><string>com.apple.loginwindow</string></dict>"},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := &ProfileResource{client: c}
	ctx := context.Background()

	plan := createResourcePlan(t, map[string]tftypes.Value{
		"id":                  nullString(),
		"name":                stringVal("macOS Custom Settings"),
		"description":         stringVal("Custom settings profile"),
		"platform":            stringVal("AppleOsX"),
		"org_group_id":        stringVal("14165"),
		"assignment_type":     nullString(),
		"profile_scope":       nullString(),
		"is_active":           nullBool(),
		"lock_screen_message": nullString(),
		"passcode":            nullPasscode(),
		"custom_settings_list": customSettingsListVal([]string{
			"<dict><key>PayloadType</key><string>com.apple.screensaver</string></dict>",
			"<dict><key>PayloadType</key><string>com.apple.loginwindow</string></dict>",
		}),
		"network_list":     nullNetworkList(),
		"credentials_list": nullCredentialsList(),
		"disk_encryption":  nullDiskEncryption(),
		"gatekeeper":       nullGatekeeper(),
		"restrictions":     nullRestrictions(),
		"uuid":             nullString(),
		"profile_context":  nullString(),
	})

	req := resource.CreateRequest{Plan: plan}
	resp := &resource.CreateResponse{State: emptyResourceState(t)}

	res.Create(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors: %v", msgs)
	}

	mu.Lock()
	defer mu.Unlock()

	if capturedBody == nil {
		t.Fatal("expected request body to be captured")
	}
	csList, ok := capturedBody["CustomSettingsList"]
	if !ok {
		t.Fatal("expected CustomSettingsList in request body")
	}
	csItems, ok := csList.([]interface{})
	if !ok {
		t.Fatal("expected CustomSettingsList to be an array")
	}
	if len(csItems) != 2 {
		t.Fatalf("expected 2 custom settings items, got %d", len(csItems))
	}
	firstItem, ok := csItems[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected first item to be a map")
	}
	if firstItem["CustomSettings"] != "<dict><key>PayloadType</key><string>com.apple.screensaver</string></dict>" {
		t.Errorf("unexpected first CustomSettings value: %v", firstItem["CustomSettings"])
	}

	var model profilemodels.ProfileResourceModel
	resp.State.Get(ctx, &model)

	if model.ID.ValueString() != "66666" {
		t.Errorf("expected ID '66666', got '%s'", model.ID.ValueString())
	}
}

func TestProfileResourceRead_WithCustomSettings(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"General": map[string]interface{}{
				"ProfileId":              66666,
				"Name":                   "macOS Custom Settings",
				"Description":            "Custom settings",
				"AssignmentType":         "Auto",
				"ProfileScope":           "Production",
				"ManagedLocationGroupID": 14165,
				"IsActive":               true,
				"ProfileUuid":            "uuid-cs-read",
				"ProfileContext":         "Device",
			},
			"CustomSettingsList": []interface{}{
				map[string]interface{}{"CustomSettings": "<dict><key>example</key><true/></dict>"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := newProfileResourceWithRegistry(t, c, discoveryEntry(66666, sdk.PlatformAppleOsX))
	ctx := context.Background()

	state := createResourceState(t, map[string]tftypes.Value{
		"id":                   stringVal("66666"),
		"name":                 stringVal("Old Name"),
		"description":          stringVal("Old"),
		"platform":             stringVal("AppleOsX"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      stringVal("Auto"),
		"profile_scope":        stringVal("Production"),
		"is_active":            boolVal(true),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 stringVal("old-uuid"),
		"profile_context":      stringVal("Device"),
	})

	req := resource.ReadRequest{State: state}
	resp := &resource.ReadResponse{State: state}

	res.Read(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors: %v", msgs)
	}

	var model profilemodels.ProfileResourceModel
	resp.State.Get(ctx, &model)

	if len(model.CustomSettingsList) != 1 {
		t.Fatalf("expected 1 custom settings item, got %d", len(model.CustomSettingsList))
	}
	if model.CustomSettingsList[0].CustomSettings.ValueString() != "<dict><key>example</key><true/></dict>" {
		t.Errorf("unexpected CustomSettings value: %s", model.CustomSettingsList[0].CustomSettings.ValueString())
	}
}

func TestProfileResourceUpdate_WithCustomSettings(t *testing.T) {
	t.Parallel()

	var (
		mu           sync.Mutex
		capturedBody map[string]interface{}
	)

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && strings.Contains(r.URL.Path, "/update"):
			mu.Lock()
			_ = json.NewDecoder(r.Body).Decode(&capturedBody)
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]interface{}{})
		case r.Method == "GET":
			resp := map[string]interface{}{
				"General": map[string]interface{}{
					"ProfileId":      66666,
					"Name":           "Updated CS",
					"ProfileUuid":    "uuid-cs-updated",
					"ProfileContext": "Device",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := &ProfileResource{client: c}
	ctx := context.Background()

	plan := createResourcePlan(t, map[string]tftypes.Value{
		"id":                  stringVal("66666"),
		"name":                stringVal("Updated CS"),
		"description":         stringVal("Updated custom settings"),
		"platform":            stringVal("AppleOsX"),
		"org_group_id":        stringVal("14165"),
		"assignment_type":     stringVal("Auto"),
		"profile_scope":       stringVal("Production"),
		"is_active":           boolVal(true),
		"lock_screen_message": nullString(),
		"passcode":            nullPasscode(),
		"custom_settings_list": customSettingsListVal([]string{
			"<dict><key>updated</key><true/></dict>",
		}),
		"network_list":     nullNetworkList(),
		"credentials_list": nullCredentialsList(),
		"disk_encryption":  nullDiskEncryption(),
		"gatekeeper":       nullGatekeeper(),
		"restrictions":     nullRestrictions(),
		"uuid":             stringVal("old-uuid"),
		"profile_context":  stringVal("Device"),
	})

	req := resource.UpdateRequest{Plan: plan, State: emptyResourceState(t)}
	resp := &resource.UpdateResponse{State: emptyResourceState(t)}

	res.Update(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors: %v", msgs)
	}

	mu.Lock()
	defer mu.Unlock()

	if capturedBody == nil {
		t.Fatal("expected request body to be captured")
	}
	csList, ok := capturedBody["CustomSettingsList"]
	if !ok {
		t.Fatal("expected CustomSettingsList in update request body")
	}
	csItems, ok := csList.([]interface{})
	if !ok {
		t.Fatal("expected CustomSettingsList to be an array")
	}
	if len(csItems) != 1 {
		t.Fatalf("expected 1 custom settings item, got %d", len(csItems))
	}
	firstItem, ok := csItems[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected first item to be a map")
	}
	if firstItem["CustomSettings"] != "<dict><key>updated</key><true/></dict>" {
		t.Errorf("unexpected CustomSettings value: %v", firstItem["CustomSettings"])
	}
}

func TestProfileResourceCreate_WithNetworkList(t *testing.T) {
	t.Parallel()

	var capturedBody map[string]interface{}

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && strings.Contains(r.URL.Path, "/create"):
			_ = json.NewDecoder(r.Body).Decode(&capturedBody)
			_ = json.NewEncoder(w).Encode(55555)
		case r.Method == "GET":
			resp := map[string]interface{}{
				"General": map[string]interface{}{
					"ProfileId":      55555,
					"Name":           "macOS Network",
					"ProfileUuid":    "uuid-network",
					"ProfileContext": "Device",
				},
				"NetworkList": []interface{}{
					map[string]interface{}{
						"NetworkInterface":     "BuiltInWireless",
						"ServiceSetIdentifier": "Corp-WiFi",
						"HiddenNetwork":        false,
						"AutoJoin":             true,
						"SecurityType":         "WPA2",
						"ProxyType":            "None",
						"ProxyServerPort":      float64(0),
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := &ProfileResource{client: c}
	ctx := context.Background()

	plan := createResourcePlan(t, map[string]tftypes.Value{
		"id":                   nullString(),
		"name":                 stringVal("macOS Network"),
		"description":          stringVal("Network profile"),
		"platform":             stringVal("AppleOsX"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      nullString(),
		"profile_scope":        nullString(),
		"is_active":            nullBool(),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list": networkListVal([]map[string]tftypes.Value{
			{
				"network_interface":      stringVal("BuiltInWireless"),
				"service_set_identifier": stringVal("Corp-WiFi"),
				"hidden_network":         boolVal(false),
				"auto_join":              boolVal(true),
				"security_type":          stringVal("WPA2"),
				"proxy_type":             stringVal("None"),
			},
		}),
		"credentials_list": nullCredentialsList(),
		"disk_encryption":  nullDiskEncryption(),
		"gatekeeper":       nullGatekeeper(),
		"restrictions":     nullRestrictions(),
		"uuid":             nullString(),
		"profile_context":  nullString(),
	})

	req := resource.CreateRequest{Plan: plan}
	resp := &resource.CreateResponse{State: emptyResourceState(t)}

	res.Create(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors: %v", msgs)
	}

	if capturedBody == nil {
		t.Fatal("expected request body to be captured")
	}
	netList, ok := capturedBody["NetworkList"]
	if !ok {
		t.Fatal("expected NetworkList in request body")
	}
	netItems, ok := netList.([]interface{})
	if !ok {
		t.Fatal("expected NetworkList to be an array")
	}
	if len(netItems) != 1 {
		t.Fatalf("expected 1 network item, got %d", len(netItems))
	}
	firstItem, ok := netItems[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected first item to be a map")
	}
	if firstItem["ServiceSetIdentifier"] != "Corp-WiFi" {
		t.Errorf("expected SSID 'Corp-WiFi', got %v", firstItem["ServiceSetIdentifier"])
	}
	if firstItem["AutoJoin"] != true {
		t.Errorf("expected AutoJoin true, got %v", firstItem["AutoJoin"])
	}

	var model profilemodels.ProfileResourceModel
	resp.State.Get(ctx, &model)
	if model.ID.ValueString() != "55555" {
		t.Errorf("expected ID '55555', got '%s'", model.ID.ValueString())
	}
}

func TestProfileResourceRead_WithNetworkList(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"General": map[string]interface{}{
				"ProfileId":              55555,
				"Name":                   "macOS Network",
				"Description":            "Network profile",
				"AssignmentType":         "Auto",
				"ProfileScope":           "Production",
				"ManagedLocationGroupID": 14165,
				"IsActive":               true,
				"ProfileUuid":            "uuid-net-read",
				"ProfileContext":         "Device",
			},
			"NetworkList": []interface{}{
				map[string]interface{}{
					"NetworkInterface":     "BuiltInWireless",
					"ServiceSetIdentifier": "Office-WiFi",
					"HiddenNetwork":        true,
					"AutoJoin":             true,
					"SecurityType":         "WPA3",
					"ProxyType":            "Manual",
					"ProxyServer":          "proxy.corp.com",
					"ProxyServerPort":      float64(8080),
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := newProfileResourceWithRegistry(t, c, discoveryEntry(55555, sdk.PlatformAppleOsX))
	ctx := context.Background()

	state := createResourceState(t, map[string]tftypes.Value{
		"id":                   stringVal("55555"),
		"name":                 stringVal("Old Name"),
		"description":          stringVal("Old"),
		"platform":             stringVal("AppleOsX"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      stringVal("Auto"),
		"profile_scope":        stringVal("Production"),
		"is_active":            boolVal(true),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 stringVal("old-uuid"),
		"profile_context":      stringVal("Device"),
	})

	req := resource.ReadRequest{State: state}
	resp := &resource.ReadResponse{State: state}

	res.Read(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors: %v", msgs)
	}

	var model profilemodels.ProfileResourceModel
	resp.State.Get(ctx, &model)

	if len(model.NetworkList) != 1 {
		t.Fatalf("expected 1 network item, got %d", len(model.NetworkList))
	}
	net := model.NetworkList[0]
	if net.ServiceSetIdentifier.ValueString() != "Office-WiFi" {
		t.Errorf("expected SSID 'Office-WiFi', got '%s'", net.ServiceSetIdentifier.ValueString())
	}
	if net.HiddenNetwork.ValueBool() != true {
		t.Errorf("expected HiddenNetwork true, got %v", net.HiddenNetwork.ValueBool())
	}
	if net.ProxyServer.ValueString() != "proxy.corp.com" {
		t.Errorf("expected ProxyServer 'proxy.corp.com', got '%s'", net.ProxyServer.ValueString())
	}
	if net.ProxyServerPort.ValueInt64() != 8080 {
		t.Errorf("expected ProxyServerPort 8080, got %d", net.ProxyServerPort.ValueInt64())
	}
}

func TestProfileResourceUpdate_WithNetworkList(t *testing.T) {
	t.Parallel()

	var capturedBody map[string]interface{}

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && strings.Contains(r.URL.Path, "/update"):
			_ = json.NewDecoder(r.Body).Decode(&capturedBody)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{})
		case r.Method == "GET":
			resp := map[string]interface{}{
				"General": map[string]interface{}{
					"ProfileId":      55555,
					"Name":           "Updated Network",
					"ProfileUuid":    "uuid-net-updated",
					"ProfileContext": "Device",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := &ProfileResource{client: c}
	ctx := context.Background()

	plan := createResourcePlan(t, map[string]tftypes.Value{
		"id":                   stringVal("55555"),
		"name":                 stringVal("Updated Network"),
		"description":          stringVal("Updated network profile"),
		"platform":             stringVal("AppleOsX"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      stringVal("Auto"),
		"profile_scope":        stringVal("Production"),
		"is_active":            boolVal(true),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list": networkListVal([]map[string]tftypes.Value{
			{
				"network_interface":      stringVal("BuiltInWireless"),
				"service_set_identifier": stringVal("Updated-WiFi"),
				"auto_join":              boolVal(false),
				"security_type":          stringVal("WPA3"),
			},
		}),
		"credentials_list": nullCredentialsList(),
		"disk_encryption":  nullDiskEncryption(),
		"gatekeeper":       nullGatekeeper(),
		"restrictions":     nullRestrictions(),
		"uuid":             stringVal("old-uuid"),
		"profile_context":  stringVal("Device"),
	})

	req := resource.UpdateRequest{Plan: plan, State: emptyResourceState(t)}
	resp := &resource.UpdateResponse{State: emptyResourceState(t)}

	res.Update(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors: %v", msgs)
	}

	if capturedBody == nil {
		t.Fatal("expected request body to be captured")
	}
	netList, ok := capturedBody["NetworkList"]
	if !ok {
		t.Fatal("expected NetworkList in update request body")
	}
	netItems, ok := netList.([]interface{})
	if !ok {
		t.Fatal("expected NetworkList to be an array")
	}
	if len(netItems) != 1 {
		t.Fatalf("expected 1 network item, got %d", len(netItems))
	}
	firstItem, ok := netItems[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected first item to be a map")
	}
	if firstItem["ServiceSetIdentifier"] != "Updated-WiFi" {
		t.Errorf("expected SSID 'Updated-WiFi', got %v", firstItem["ServiceSetIdentifier"])
	}
	if firstItem["AutoJoin"] != false {
		t.Errorf("expected AutoJoin false, got %v", firstItem["AutoJoin"])
	}
}

func TestProfileResourceUpdate_APIError(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "server error"})
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := &ProfileResource{client: c}
	ctx := context.Background()

	plan := createResourcePlan(t, map[string]tftypes.Value{
		"id":                   stringVal("12345"),
		"name":                 stringVal("Test"),
		"description":          nullString(),
		"platform":             stringVal("Android"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      stringVal("Auto"),
		"profile_scope":        stringVal("Production"),
		"is_active":            boolVal(true),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 nullString(),
		"profile_context":      nullString(),
	})

	req := resource.UpdateRequest{Plan: plan, State: emptyResourceState(t)}
	resp := &resource.UpdateResponse{State: emptyResourceState(t)}

	res.Update(ctx, req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for API failure")
	}
}

func TestProfileResourceUpdate_InvalidID(t *testing.T) {
	t.Parallel()

	res := &ProfileResource{client: nil}
	ctx := context.Background()

	plan := createResourcePlan(t, map[string]tftypes.Value{
		"id":                   stringVal("not-a-number"),
		"name":                 stringVal("Test"),
		"description":          nullString(),
		"platform":             stringVal("Android"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      stringVal("Auto"),
		"profile_scope":        stringVal("Production"),
		"is_active":            boolVal(true),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 nullString(),
		"profile_context":      nullString(),
	})

	req := resource.UpdateRequest{Plan: plan, State: emptyResourceState(t)}
	resp := &resource.UpdateResponse{State: emptyResourceState(t)}

	res.Update(ctx, req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for invalid profile ID")
	}
}

// --- Delete() tests ---

func TestProfileResourceDelete(t *testing.T) {
	t.Parallel()

	var sawDelete bool
	handler := func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/profiles/search"):
			// Layer 2 discovery: an empty registry is fine; Delete calls
			// DELETE directly without a registry lookup.
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ProfileList":[]}`))
		case r.Method == http.MethodDelete:
			sawDelete = true
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotImplemented)
		}
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := &ProfileResource{client: c}
	ctx := context.Background()

	state := createResourceState(t, map[string]tftypes.Value{
		"id":                   stringVal("12345"),
		"name":                 stringVal("Delete Me"),
		"description":          nullString(),
		"platform":             stringVal("Android"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      stringVal("Auto"),
		"profile_scope":        stringVal("Production"),
		"is_active":            boolVal(true),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 nullString(),
		"profile_context":      nullString(),
	})

	req := resource.DeleteRequest{State: state}
	var resp resource.DeleteResponse

	res.Delete(ctx, req, &resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors: %v", msgs)
	}
	if !sawDelete {
		t.Error("expected DELETE request to be issued")
	}
}

func TestProfileResourceDelete_APIError(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/profiles/search"):
			_, _ = w.Write([]byte(`{"ProfileList":[]}`))
		default:
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "server error"})
		}
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := &ProfileResource{client: c}
	ctx := context.Background()

	state := createResourceState(t, map[string]tftypes.Value{
		"id":                   stringVal("12345"),
		"name":                 stringVal("Test"),
		"description":          nullString(),
		"platform":             stringVal("Android"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      stringVal("Auto"),
		"profile_scope":        stringVal("Production"),
		"is_active":            boolVal(true),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 nullString(),
		"profile_context":      nullString(),
	})

	req := resource.DeleteRequest{State: state}
	var resp resource.DeleteResponse

	res.Delete(ctx, req, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for API failure")
	}
}

// TestProfileResourceDelete_NotFound verifies that a 404 from the DELETE
// endpoint is treated as success (the profile is already gone), making
// `terraform destroy` idempotent.
func TestProfileResourceDelete_NotFound(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/profiles/search"):
			_, _ = w.Write([]byte(`{"ProfileList":[]}`))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "not found"})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotImplemented)
		}
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := &ProfileResource{client: c}
	ctx := context.Background()

	state := createResourceState(t, map[string]tftypes.Value{
		"id":                   stringVal("99999"),
		"name":                 stringVal("Already Gone"),
		"description":          nullString(),
		"platform":             stringVal("Android"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      stringVal("Auto"),
		"profile_scope":        stringVal("Production"),
		"is_active":            boolVal(true),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 nullString(),
		"profile_context":      nullString(),
	})

	req := resource.DeleteRequest{State: state}
	var resp resource.DeleteResponse

	res.Delete(ctx, req, &resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("expected no error for 404 on delete, got: %v", msgs)
	}
}

func TestProfileResourceDelete_InvalidID(t *testing.T) {
	t.Parallel()

	res := &ProfileResource{client: nil}
	ctx := context.Background()

	state := createResourceState(t, map[string]tftypes.Value{
		"id":                   stringVal("not-a-number"),
		"name":                 stringVal("Test"),
		"description":          nullString(),
		"platform":             stringVal("Android"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      stringVal("Auto"),
		"profile_scope":        stringVal("Production"),
		"is_active":            boolVal(true),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 nullString(),
		"profile_context":      nullString(),
	})

	req := resource.DeleteRequest{State: state}
	var resp resource.DeleteResponse

	res.Delete(ctx, req, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for invalid profile ID, but got none")
	}
}

func TestProfileResourceCreate_WithDiskEncryption(t *testing.T) {
	t.Parallel()

	var capturedBody map[string]interface{}

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && strings.Contains(r.URL.Path, "/create"):
			_ = json.NewDecoder(r.Body).Decode(&capturedBody)
			_ = json.NewEncoder(w).Encode(33333)
		case r.Method == "GET":
			resp := map[string]interface{}{
				"General": map[string]interface{}{
					"ProfileId":              33333,
					"Name":                   "macOS Disk Encryption",
					"ProfileUuid":            "uuid-macos-disk-enc",
					"ProfileContext":         "Device",
					"ManagedLocationGroupID": 14165,
				},
				"DiskEncryption": map[string]interface{}{
					"DiskEncryptionAirWatch": map[string]interface{}{
						"StoreKey":                              true,
						"RotateKeyAfter":                        float64(90),
						"UseIntelligentHub":                     true,
						"NotifyUserForEncryption":               true,
						"EncryptionMaxNotifyAttempts":           float64(3),
						"EncryptionActionAfterLastNotification": "ForceLogout",
					},
					"DiskEncryptionFileVault2": map[string]interface{}{
						"Enable":                     true,
						"ShowRecoveryKey":            true,
						"RecoveryType":               float64(1),
						"FileVaultUser":              "CurrentOrNextLoginUser",
						"PromptToEnableFileVaultAt":  "BothLoginAndLogout",
						"NumberOfTimesUserCanBypass": float64(3),
					},
					"DiskEncryptionMCX": map[string]interface{}{
						"DestroyFVKeyOnStandby": true,
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := &ProfileResource{client: c}
	ctx := context.Background()

	plan := createResourcePlan(t, map[string]tftypes.Value{
		"id":                   nullString(),
		"name":                 stringVal("macOS Disk Encryption"),
		"description":          stringVal("Disk encryption policy"),
		"platform":             stringVal("AppleOsX"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      nullString(),
		"profile_scope":        nullString(),
		"is_active":            nullBool(),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption": diskEncryptionVal(map[string]tftypes.Value{
			"airwatch": airwatchVal(map[string]tftypes.Value{
				"store_key":                                 boolVal(true),
				"rotate_key_after":                          int64Val(90),
				"use_intelligent_hub":                       boolVal(true),
				"notify_user_for_encryption":                boolVal(true),
				"encryption_max_notify_attempts":            int64Val(3),
				"encryption_action_after_last_notification": int64Val(1),
			}),
			"filevault2": filevaultVal(map[string]tftypes.Value{
				"enable":                          boolVal(true),
				"show_recovery_key":               boolVal(true),
				"recovery_type":                   int64Val(1),
				"filevault_user":                  int64Val(1),
				"prompt_to_enable_filevault_at":   int64Val(1),
				"number_of_times_user_can_bypass": int64Val(3),
			}),
			"mcx": mcxVal(map[string]tftypes.Value{
				"destroy_fv_key_on_standby": boolVal(true),
			}),
		}),
		"gatekeeper":      nullGatekeeper(),
		"restrictions":    nullRestrictions(),
		"uuid":            nullString(),
		"profile_context": nullString(),
	})

	req := resource.CreateRequest{Plan: plan}
	resp := &resource.CreateResponse{State: emptyResourceState(t)}

	res.Create(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors: %v", msgs)
	}

	if capturedBody == nil {
		t.Fatal("expected request body to be captured")
	}
	deTop, ok := capturedBody["DiskEncryption"].(map[string]interface{})
	if !ok {
		t.Fatal("expected DiskEncryption in request body")
	}
	aw, ok := deTop["DiskEncryptionAirWatch"].(map[string]interface{})
	if !ok {
		t.Fatal("expected DiskEncryptionAirWatch in request body")
	}
	if aw["StoreKey"] != true {
		t.Errorf("expected StoreKey true, got %v", aw["StoreKey"])
	}
	if aw["RotateKeyAfter"] != float64(90) {
		t.Errorf("expected RotateKeyAfter 90, got %v", aw["RotateKeyAfter"])
	}
	if aw["UseIntelligentHub"] != true {
		t.Errorf("expected UseIntelligentHub true, got %v", aw["UseIntelligentHub"])
	}
	if aw["NotifyUserForEncryption"] != true {
		t.Errorf("expected NotifyUserForEncryption true, got %v", aw["NotifyUserForEncryption"])
	}
	if aw["EncryptionMaxNotifyAttempts"] != float64(3) {
		t.Errorf("expected EncryptionMaxNotifyAttempts 3, got %v", aw["EncryptionMaxNotifyAttempts"])
	}
	if aw["EncryptionActionAfterLastNotification"] != "ForceLogout" {
		t.Errorf("expected EncryptionActionAfterLastNotification \"ForceLogout\", got %v", aw["EncryptionActionAfterLastNotification"])
	}

	fv, ok := deTop["DiskEncryptionFileVault2"].(map[string]interface{})
	if !ok {
		t.Fatal("expected DiskEncryptionFileVault2 in request body")
	}
	if fv["Enable"] != true {
		t.Errorf("expected Enable true, got %v", fv["Enable"])
	}
	if fv["ShowRecoveryKey"] != true {
		t.Errorf("expected ShowRecoveryKey true, got %v", fv["ShowRecoveryKey"])
	}
	if fv["RecoveryType"] != float64(1) {
		t.Errorf("expected RecoveryType 1, got %v", fv["RecoveryType"])
	}
	if fv["FileVaultUser"] != "CurrentOrNextLoginUser" {
		t.Errorf("expected FileVaultUser \"CurrentOrNextLoginUser\", got %v", fv["FileVaultUser"])
	}
	if fv["PromptToEnableFileVaultAt"] != "BothLoginAndLogout" {
		t.Errorf("expected PromptToEnableFileVaultAt \"BothLoginAndLogout\", got %v", fv["PromptToEnableFileVaultAt"])
	}
	if fv["NumberOfTimesUserCanBypass"] != float64(3) {
		t.Errorf("expected NumberOfTimesUserCanBypass 3, got %v", fv["NumberOfTimesUserCanBypass"])
	}

	mcx, ok := deTop["DiskEncryptionMCX"].(map[string]interface{})
	if !ok {
		t.Fatal("expected DiskEncryptionMCX in request body")
	}
	if mcx["DestroyFVKeyOnStandby"] != true {
		t.Errorf("expected DestroyFVKeyOnStandby true, got %v", mcx["DestroyFVKeyOnStandby"])
	}

	var model profilemodels.ProfileResourceModel
	resp.State.Get(ctx, &model)

	if model.ID.ValueString() != "33333" {
		t.Errorf("expected ID '33333', got '%s'", model.ID.ValueString())
	}
	if model.DiskEncryption == nil {
		t.Fatal("expected DiskEncryption to be set in state after read-back")
	}
	if model.DiskEncryption.AirWatch == nil {
		t.Fatal("expected AirWatch in state")
	}
	if model.DiskEncryption.AirWatch.StoreKey.ValueBool() != true {
		t.Errorf("expected StoreKey true, got %v", model.DiskEncryption.AirWatch.StoreKey.ValueBool())
	}
	if model.DiskEncryption.AirWatch.RotateKeyAfter.ValueInt64() != 90 {
		t.Errorf("expected RotateKeyAfter 90, got %d", model.DiskEncryption.AirWatch.RotateKeyAfter.ValueInt64())
	}
	if model.DiskEncryption.FileVault == nil {
		t.Fatal("expected FileVault in state")
	}
	if !model.DiskEncryption.FileVault.Enable.ValueBool() {
		t.Error("expected FileVault Enable true")
	}
	if model.DiskEncryption.FileVault.NumberOfTimesUserCanBypass.ValueInt64() != 3 {
		t.Errorf("expected NumberOfTimesUserCanBypass 3, got %d", model.DiskEncryption.FileVault.NumberOfTimesUserCanBypass.ValueInt64())
	}
	if model.DiskEncryption.MCX.IsNull() {
		t.Fatal("expected MCX in state")
	}
	if got := mcxDestroyFVKeyOnStandby(t, model.DiskEncryption.MCX); got.ValueBool() != true {
		t.Errorf("expected DestroyFVKeyOnStandby true, got %v", got.ValueBool())
	}
}

func TestProfileResourceRead_WithDiskEncryption(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"General": map[string]interface{}{
				"ProfileId":              33333,
				"Name":                   "macOS Disk Encryption",
				"Description":            "Policy",
				"AssignmentType":         "Auto",
				"ProfileScope":           "Production",
				"ManagedLocationGroupID": 14165,
				"IsActive":               true,
				"ProfileUuid":            "uuid-disk-read",
				"ProfileContext":         "Device",
			},
			"DiskEncryption": map[string]interface{}{
				"DiskEncryptionAirWatch": map[string]interface{}{
					"StoreKey":                              true,
					"RotateKeyAfter":                        float64(90),
					"UseIntelligentHub":                     true,
					"NotifyUserForEncryption":               true,
					"EncryptionMaxNotifyAttempts":           float64(3),
					"EncryptionActionAfterLastNotification": "ForceLogout",
				},
				"DiskEncryptionFileVault2": map[string]interface{}{
					"Enable":                     true,
					"ShowRecoveryKey":            true,
					"RecoveryType":               float64(1),
					"FileVaultUser":              "CurrentOrNextLoginUser",
					"PromptToEnableFileVaultAt":  "BothLoginAndLogout",
					"NumberOfTimesUserCanBypass": float64(3),
				},
				"DiskEncryptionMCX": map[string]interface{}{
					"DestroyFVKeyOnStandby": true,
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := newProfileResourceWithRegistry(t, c, discoveryEntry(33333, sdk.PlatformAppleOsX))
	ctx := context.Background()

	state := createResourceState(t, map[string]tftypes.Value{
		"id":                   stringVal("33333"),
		"name":                 stringVal("Old Name"),
		"description":          stringVal("Old"),
		"platform":             stringVal("AppleOsX"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      stringVal("Auto"),
		"profile_scope":        stringVal("Production"),
		"is_active":            boolVal(true),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 stringVal("old-uuid"),
		"profile_context":      stringVal("Device"),
	})

	req := resource.ReadRequest{State: state}
	resp := &resource.ReadResponse{State: state}

	res.Read(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors: %v", msgs)
	}

	var model profilemodels.ProfileResourceModel
	resp.State.Get(ctx, &model)

	if model.Name.ValueString() != "macOS Disk Encryption" {
		t.Errorf("expected Name 'macOS Disk Encryption', got '%s'", model.Name.ValueString())
	}
	if model.DiskEncryption == nil {
		t.Fatal("expected DiskEncryption to be populated from API response")
	}
	if model.DiskEncryption.AirWatch == nil {
		t.Fatal("expected AirWatch")
	}
	if model.DiskEncryption.AirWatch.StoreKey.ValueBool() != true {
		t.Errorf("StoreKey: got %v", model.DiskEncryption.AirWatch.StoreKey.ValueBool())
	}
	if model.DiskEncryption.AirWatch.RotateKeyAfter.ValueInt64() != 90 {
		t.Errorf("RotateKeyAfter: got %d, want 90", model.DiskEncryption.AirWatch.RotateKeyAfter.ValueInt64())
	}
	if model.DiskEncryption.AirWatch.UseIntelligentHub.ValueBool() != true {
		t.Errorf("UseIntelligentHub: got %v", model.DiskEncryption.AirWatch.UseIntelligentHub.ValueBool())
	}
	if model.DiskEncryption.AirWatch.NotifyUserForEncryption.ValueBool() != true {
		t.Errorf("NotifyUserForEncryption: got %v", model.DiskEncryption.AirWatch.NotifyUserForEncryption.ValueBool())
	}
	if model.DiskEncryption.AirWatch.EncryptionMaxNotifyAttempts.ValueInt64() != 3 {
		t.Errorf("EncryptionMaxNotifyAttempts: got %d", model.DiskEncryption.AirWatch.EncryptionMaxNotifyAttempts.ValueInt64())
	}
	if model.DiskEncryption.AirWatch.EncryptionActionAfterLastNotification.ValueInt64() != 1 {
		t.Errorf("EncryptionActionAfterLastNotification: got %d", model.DiskEncryption.AirWatch.EncryptionActionAfterLastNotification.ValueInt64())
	}
	if model.DiskEncryption.FileVault == nil {
		t.Fatal("expected FileVault")
	}
	if model.DiskEncryption.FileVault.Enable.ValueBool() != true {
		t.Errorf("FileVault Enable: got %v", model.DiskEncryption.FileVault.Enable.ValueBool())
	}
	if model.DiskEncryption.FileVault.ShowRecoveryKey.ValueBool() != true {
		t.Errorf("ShowRecoveryKey: got %v", model.DiskEncryption.FileVault.ShowRecoveryKey.ValueBool())
	}
	if model.DiskEncryption.FileVault.RecoveryType.ValueInt64() != 1 {
		t.Errorf("RecoveryType: got %d", model.DiskEncryption.FileVault.RecoveryType.ValueInt64())
	}
	if model.DiskEncryption.FileVault.FileVaultUser.ValueInt64() != 1 {
		t.Errorf("FileVaultUser: got %d", model.DiskEncryption.FileVault.FileVaultUser.ValueInt64())
	}
	if model.DiskEncryption.FileVault.PromptToEnableFileVaultAt.ValueInt64() != 1 {
		t.Errorf("PromptToEnableFileVaultAt: got %d", model.DiskEncryption.FileVault.PromptToEnableFileVaultAt.ValueInt64())
	}
	if model.DiskEncryption.FileVault.NumberOfTimesUserCanBypass.ValueInt64() != 3 {
		t.Errorf("NumberOfTimesUserCanBypass: got %d", model.DiskEncryption.FileVault.NumberOfTimesUserCanBypass.ValueInt64())
	}
	if model.DiskEncryption.MCX.IsNull() {
		t.Fatal("expected MCX")
	}
	if got := mcxDestroyFVKeyOnStandby(t, model.DiskEncryption.MCX); got.ValueBool() != true {
		t.Errorf("DestroyFVKeyOnStandby: got %v", got.ValueBool())
	}
}

func TestProfileResourceUpdate_WithDiskEncryption(t *testing.T) {
	t.Parallel()

	var capturedBody map[string]interface{}

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && strings.Contains(r.URL.Path, "/update"):
			_ = json.NewDecoder(r.Body).Decode(&capturedBody)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{})
		case r.Method == "GET":
			resp := map[string]interface{}{
				"General": map[string]interface{}{
					"ProfileId":      44444,
					"Name":           "Updated Disk Encryption",
					"ProfileUuid":    "uuid-updated-disk-enc",
					"ProfileContext": "Device",
				},
				"DiskEncryption": map[string]interface{}{
					"DiskEncryptionAirWatch": map[string]interface{}{
						"StoreKey":                              true,
						"RotateKeyAfter":                        float64(90),
						"UseIntelligentHub":                     true,
						"NotifyUserForEncryption":               true,
						"EncryptionMaxNotifyAttempts":           float64(3),
						"EncryptionActionAfterLastNotification": "ForceLogout",
					},
					"DiskEncryptionFileVault2": map[string]interface{}{
						"Enable":                     true,
						"ShowRecoveryKey":            true,
						"RecoveryType":               float64(1),
						"FileVaultUser":              "CurrentOrNextLoginUser",
						"PromptToEnableFileVaultAt":  "BothLoginAndLogout",
						"NumberOfTimesUserCanBypass": float64(3),
					},
					"DiskEncryptionMCX": map[string]interface{}{
						"DestroyFVKeyOnStandby": true,
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := &ProfileResource{client: c}
	ctx := context.Background()

	plan := createResourcePlan(t, map[string]tftypes.Value{
		"id":                   stringVal("44444"),
		"name":                 stringVal("Updated Disk Encryption"),
		"description":          stringVal("Updated policy"),
		"platform":             stringVal("AppleOsX"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      stringVal("Auto"),
		"profile_scope":        stringVal("Production"),
		"is_active":            boolVal(true),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption": diskEncryptionVal(map[string]tftypes.Value{
			"airwatch": airwatchVal(map[string]tftypes.Value{
				"store_key":                                 boolVal(true),
				"rotate_key_after":                          int64Val(90),
				"use_intelligent_hub":                       boolVal(true),
				"notify_user_for_encryption":                boolVal(true),
				"encryption_max_notify_attempts":            int64Val(3),
				"encryption_action_after_last_notification": int64Val(1),
			}),
			"filevault2": filevaultVal(map[string]tftypes.Value{
				"enable":                          boolVal(true),
				"show_recovery_key":               boolVal(true),
				"recovery_type":                   int64Val(1),
				"filevault_user":                  int64Val(1),
				"prompt_to_enable_filevault_at":   int64Val(1),
				"number_of_times_user_can_bypass": int64Val(3),
			}),
			"mcx": mcxVal(map[string]tftypes.Value{
				"destroy_fv_key_on_standby": boolVal(true),
			}),
		}),
		"gatekeeper":      nullGatekeeper(),
		"restrictions":    nullRestrictions(),
		"uuid":            stringVal("old-uuid"),
		"profile_context": stringVal("Device"),
	})

	req := resource.UpdateRequest{Plan: plan, State: emptyResourceState(t)}
	resp := &resource.UpdateResponse{State: emptyResourceState(t)}

	res.Update(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors: %v", msgs)
	}

	if capturedBody == nil {
		t.Fatal("expected request body to be captured")
	}
	deTop, ok := capturedBody["DiskEncryption"].(map[string]interface{})
	if !ok {
		t.Fatal("expected DiskEncryption in update request body")
	}
	aw, ok := deTop["DiskEncryptionAirWatch"].(map[string]interface{})
	if !ok {
		t.Fatal("expected DiskEncryptionAirWatch in update request body")
	}
	if aw["StoreKey"] != true {
		t.Errorf("expected StoreKey true, got %v", aw["StoreKey"])
	}
	if aw["RotateKeyAfter"] != float64(90) {
		t.Errorf("expected RotateKeyAfter 90, got %v", aw["RotateKeyAfter"])
	}
	if aw["EncryptionMaxNotifyAttempts"] != float64(3) {
		t.Errorf("expected EncryptionMaxNotifyAttempts 3, got %v", aw["EncryptionMaxNotifyAttempts"])
	}

	fv, ok := deTop["DiskEncryptionFileVault2"].(map[string]interface{})
	if !ok {
		t.Fatal("expected DiskEncryptionFileVault2 in update request body")
	}
	if fv["Enable"] != true || fv["NumberOfTimesUserCanBypass"] != float64(3) {
		t.Errorf("unexpected FileVault2 payload: %#v", fv)
	}

	mcx, ok := deTop["DiskEncryptionMCX"].(map[string]interface{})
	if !ok {
		t.Fatal("expected DiskEncryptionMCX in update request body")
	}
	if mcx["DestroyFVKeyOnStandby"] != true {
		t.Errorf("expected DestroyFVKeyOnStandby true, got %v", mcx["DestroyFVKeyOnStandby"])
	}

	general, ok := capturedBody["General"].(map[string]interface{})
	if !ok {
		t.Fatal("expected General in update request")
	}
	if general["ProfileId"] != float64(44444) {
		t.Errorf("expected ProfileId 44444, got %v", general["ProfileId"])
	}
	if general["ProfileContext"] != "Device" {
		t.Errorf("expected ProfileContext Device, got %v", general["ProfileContext"])
	}

	var model profilemodels.ProfileResourceModel
	resp.State.Get(ctx, &model)

	if model.DiskEncryption == nil {
		t.Fatal("expected DiskEncryption in state after read-back")
	}
	if model.DiskEncryption.AirWatch == nil || model.DiskEncryption.AirWatch.RotateKeyAfter.ValueInt64() != 90 {
		t.Errorf("expected AirWatch.RotateKeyAfter 90 in state")
	}
	if model.DiskEncryption.FileVault == nil || model.DiskEncryption.FileVault.RecoveryType.ValueInt64() != 1 {
		t.Errorf("expected FileVault.RecoveryType 1 in state")
	}
	if model.DiskEncryption.MCX.IsNull() || !mcxDestroyFVKeyOnStandby(t, model.DiskEncryption.MCX).ValueBool() {
		t.Errorf("expected MCX.DestroyFVKeyOnStandby true in state")
	}
}

func TestProfileResourceCreate_WithGatekeeper(t *testing.T) {
	t.Parallel()

	var capturedBody map[string]interface{}

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && strings.Contains(r.URL.Path, "/create"):
			_ = json.NewDecoder(r.Body).Decode(&capturedBody)
			_ = json.NewEncoder(w).Encode(55555)
		case r.Method == "GET":
			resp := map[string]interface{}{
				"General": map[string]interface{}{
					"ProfileId":              55555,
					"Name":                   "macOS Gatekeeper",
					"ProfileUuid":            "uuid-macos-gatekeeper",
					"ProfileContext":         "Device",
					"ManagedLocationGroupID": 14165,
				},
				"GateKeeper": map[string]interface{}{
					"AllowAutoUnlock":              false,
					"AllowFingerprintForUnlock":    true,
					"AllowHandoff":                 false,
					"AllowScreenCapture":           true,
					"EnableAppSoftwareUpdateDelay": true,
					"EnableSoftwareUpdateDelay":    true,
					"EnforcedSoftwareUpdateDelay":  float64(60),
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := &ProfileResource{client: c}
	ctx := context.Background()

	plan := createResourcePlan(t, map[string]tftypes.Value{
		"id":                   nullString(),
		"name":                 stringVal("macOS Gatekeeper"),
		"description":          stringVal("Gatekeeper policy"),
		"platform":             stringVal("AppleOsX"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      nullString(),
		"profile_scope":        nullString(),
		"is_active":            nullBool(),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper": gatekeeperVal(map[string]tftypes.Value{
			"allow_auto_unlock":                boolVal(false),
			"allow_fingerprint_for_unlock":     boolVal(true),
			"allow_handoff":                    boolVal(false),
			"allow_screen_capture":             boolVal(true),
			"enable_app_software_update_delay": boolVal(true),
			"enable_software_update_delay":     boolVal(true),
			"enforced_software_update_delay":   int64Val(60),
		}),
		"restrictions":    nullRestrictions(),
		"uuid":            nullString(),
		"profile_context": nullString(),
	})

	req := resource.CreateRequest{Plan: plan}
	resp := &resource.CreateResponse{State: emptyResourceState(t)}

	res.Create(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors: %v", msgs)
	}

	if capturedBody == nil {
		t.Fatal("expected request body to be captured")
	}
	gk, ok := capturedBody["GateKeeper"].(map[string]interface{})
	if !ok {
		t.Fatal("expected GateKeeper in request body")
	}
	if gk["AllowAutoUnlock"] != false {
		t.Errorf("expected AllowAutoUnlock false, got %v", gk["AllowAutoUnlock"])
	}
	if gk["AllowFingerprintForUnlock"] != true {
		t.Errorf("expected AllowFingerprintForUnlock true, got %v", gk["AllowFingerprintForUnlock"])
	}
	if gk["AllowHandoff"] != false {
		t.Errorf("expected AllowHandoff false, got %v", gk["AllowHandoff"])
	}
	if gk["AllowScreenCapture"] != true {
		t.Errorf("expected AllowScreenCapture true, got %v", gk["AllowScreenCapture"])
	}
	if gk["EnableAppSoftwareUpdateDelay"] != true {
		t.Errorf("expected EnableAppSoftwareUpdateDelay true, got %v", gk["EnableAppSoftwareUpdateDelay"])
	}
	if gk["EnableSoftwareUpdateDelay"] != true {
		t.Errorf("expected EnableSoftwareUpdateDelay true, got %v", gk["EnableSoftwareUpdateDelay"])
	}
	if gk["EnforcedSoftwareUpdateDelay"] != float64(60) {
		t.Errorf("expected EnforcedSoftwareUpdateDelay 60, got %v", gk["EnforcedSoftwareUpdateDelay"])
	}

	var model profilemodels.ProfileResourceModel
	resp.State.Get(ctx, &model)

	if model.ID.ValueString() != "55555" {
		t.Errorf("expected ID '55555', got '%s'", model.ID.ValueString())
	}
	if model.Gatekeeper == nil {
		t.Fatal("expected Gatekeeper to be set in state after read-back")
	}
	if model.Gatekeeper.AllowAutoUnlock.ValueBool() != false {
		t.Errorf("expected AllowAutoUnlock false, got %v", model.Gatekeeper.AllowAutoUnlock.ValueBool())
	}
	if model.Gatekeeper.AllowFingerprintForUnlock.ValueBool() != true {
		t.Errorf("expected AllowFingerprintForUnlock true, got %v", model.Gatekeeper.AllowFingerprintForUnlock.ValueBool())
	}
	if model.Gatekeeper.AllowHandoff.ValueBool() != false {
		t.Errorf("expected AllowHandoff false, got %v", model.Gatekeeper.AllowHandoff.ValueBool())
	}
	if model.Gatekeeper.AllowScreenCapture.ValueBool() != true {
		t.Errorf("expected AllowScreenCapture true, got %v", model.Gatekeeper.AllowScreenCapture.ValueBool())
	}
	if model.Gatekeeper.EnableAppSoftwareUpdateDelay.ValueBool() != true {
		t.Errorf("expected EnableAppSoftwareUpdateDelay true, got %v", model.Gatekeeper.EnableAppSoftwareUpdateDelay.ValueBool())
	}
	if model.Gatekeeper.EnableSoftwareUpdateDelay.ValueBool() != true {
		t.Errorf("expected EnableSoftwareUpdateDelay true, got %v", model.Gatekeeper.EnableSoftwareUpdateDelay.ValueBool())
	}
	if model.Gatekeeper.EnforcedSoftwareUpdateDelay.ValueInt64() != 60 {
		t.Errorf("expected EnforcedSoftwareUpdateDelay 60, got %d", model.Gatekeeper.EnforcedSoftwareUpdateDelay.ValueInt64())
	}
}

func TestProfileResourceRead_WithGatekeeper(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"General": map[string]interface{}{
				"ProfileId":              55555,
				"Name":                   "macOS Gatekeeper",
				"Description":            "Policy",
				"AssignmentType":         "Auto",
				"ProfileScope":           "Production",
				"ManagedLocationGroupID": 14165,
				"IsActive":               true,
				"ProfileUuid":            "uuid-gatekeeper-read",
				"ProfileContext":         "Device",
			},
			"GateKeeper": map[string]interface{}{
				"AllowAutoUnlock":              false,
				"AllowFingerprintForUnlock":    true,
				"AllowHandoff":                 false,
				"AllowScreenCapture":           true,
				"EnableAppSoftwareUpdateDelay": true,
				"EnableSoftwareUpdateDelay":    true,
				"EnforcedSoftwareUpdateDelay":  float64(60),
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := newProfileResourceWithRegistry(t, c, discoveryEntry(55555, sdk.PlatformAppleOsX))
	ctx := context.Background()

	state := createResourceState(t, map[string]tftypes.Value{
		"id":                   stringVal("55555"),
		"name":                 stringVal("Old Name"),
		"description":          stringVal("Old"),
		"platform":             stringVal("AppleOsX"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      stringVal("Auto"),
		"profile_scope":        stringVal("Production"),
		"is_active":            boolVal(true),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 stringVal("old-uuid"),
		"profile_context":      stringVal("Device"),
	})

	req := resource.ReadRequest{State: state}
	resp := &resource.ReadResponse{State: state}

	res.Read(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors: %v", msgs)
	}

	var model profilemodels.ProfileResourceModel
	resp.State.Get(ctx, &model)

	if model.Name.ValueString() != "macOS Gatekeeper" {
		t.Errorf("expected Name 'macOS Gatekeeper', got '%s'", model.Name.ValueString())
	}
	if model.Gatekeeper == nil {
		t.Fatal("expected Gatekeeper to be populated from API response")
	}
	if model.Gatekeeper.AllowAutoUnlock.ValueBool() != false {
		t.Errorf("AllowAutoUnlock: got %v", model.Gatekeeper.AllowAutoUnlock.ValueBool())
	}
	if model.Gatekeeper.AllowFingerprintForUnlock.ValueBool() != true {
		t.Errorf("AllowFingerprintForUnlock: got %v", model.Gatekeeper.AllowFingerprintForUnlock.ValueBool())
	}
	if model.Gatekeeper.AllowHandoff.ValueBool() != false {
		t.Errorf("AllowHandoff: got %v", model.Gatekeeper.AllowHandoff.ValueBool())
	}
	if model.Gatekeeper.AllowScreenCapture.ValueBool() != true {
		t.Errorf("AllowScreenCapture: got %v", model.Gatekeeper.AllowScreenCapture.ValueBool())
	}
	if model.Gatekeeper.EnableAppSoftwareUpdateDelay.ValueBool() != true {
		t.Errorf("EnableAppSoftwareUpdateDelay: got %v", model.Gatekeeper.EnableAppSoftwareUpdateDelay.ValueBool())
	}
	if model.Gatekeeper.EnableSoftwareUpdateDelay.ValueBool() != true {
		t.Errorf("EnableSoftwareUpdateDelay: got %v", model.Gatekeeper.EnableSoftwareUpdateDelay.ValueBool())
	}
	if model.Gatekeeper.EnforcedSoftwareUpdateDelay.ValueInt64() != 60 {
		t.Errorf("EnforcedSoftwareUpdateDelay: got %d, want 60", model.Gatekeeper.EnforcedSoftwareUpdateDelay.ValueInt64())
	}
}

func TestProfileResourceUpdate_WithGatekeeper(t *testing.T) {
	t.Parallel()

	var capturedBody map[string]interface{}

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && strings.Contains(r.URL.Path, "/update"):
			_ = json.NewDecoder(r.Body).Decode(&capturedBody)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{})
		case r.Method == "GET":
			resp := map[string]interface{}{
				"General": map[string]interface{}{
					"ProfileId":      66666,
					"Name":           "Updated Gatekeeper",
					"ProfileUuid":    "uuid-updated-gatekeeper",
					"ProfileContext": "Device",
				},
				"GateKeeper": map[string]interface{}{
					"AllowAutoUnlock":              false,
					"AllowFingerprintForUnlock":    true,
					"AllowHandoff":                 false,
					"AllowScreenCapture":           true,
					"EnableAppSoftwareUpdateDelay": true,
					"EnableSoftwareUpdateDelay":    true,
					"EnforcedSoftwareUpdateDelay":  float64(60),
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := &ProfileResource{client: c}
	ctx := context.Background()

	plan := createResourcePlan(t, map[string]tftypes.Value{
		"id":                   stringVal("66666"),
		"name":                 stringVal("Updated Gatekeeper"),
		"description":          stringVal("Updated policy"),
		"platform":             stringVal("AppleOsX"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      stringVal("Auto"),
		"profile_scope":        stringVal("Production"),
		"is_active":            boolVal(true),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper": gatekeeperVal(map[string]tftypes.Value{
			"allow_auto_unlock":                boolVal(false),
			"allow_fingerprint_for_unlock":     boolVal(true),
			"allow_handoff":                    boolVal(false),
			"allow_screen_capture":             boolVal(true),
			"enable_app_software_update_delay": boolVal(true),
			"enable_software_update_delay":     boolVal(true),
			"enforced_software_update_delay":   int64Val(60),
		}),
		"restrictions":    nullRestrictions(),
		"uuid":            stringVal("old-uuid"),
		"profile_context": stringVal("Device"),
	})

	req := resource.UpdateRequest{Plan: plan, State: emptyResourceState(t)}
	resp := &resource.UpdateResponse{State: emptyResourceState(t)}

	res.Update(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors: %v", msgs)
	}

	if capturedBody == nil {
		t.Fatal("expected request body to be captured")
	}
	gk, ok := capturedBody["GateKeeper"].(map[string]interface{})
	if !ok {
		t.Fatal("expected GateKeeper in update request body")
	}
	if gk["AllowScreenCapture"] != true {
		t.Errorf("expected AllowScreenCapture true, got %v", gk["AllowScreenCapture"])
	}
	if gk["EnforcedSoftwareUpdateDelay"] != float64(60) {
		t.Errorf("expected EnforcedSoftwareUpdateDelay 60, got %v", gk["EnforcedSoftwareUpdateDelay"])
	}

	general, ok := capturedBody["General"].(map[string]interface{})
	if !ok {
		t.Fatal("expected General in update request")
	}
	if general["ProfileId"] != float64(66666) {
		t.Errorf("expected ProfileId 66666, got %v", general["ProfileId"])
	}

	var model profilemodels.ProfileResourceModel
	resp.State.Get(ctx, &model)

	if model.Gatekeeper == nil {
		t.Fatal("expected Gatekeeper in state after read-back")
	}
	if model.Gatekeeper.AllowScreenCapture.ValueBool() != true {
		t.Errorf("expected Gatekeeper.AllowScreenCapture true in state")
	}
	if model.Gatekeeper.EnforcedSoftwareUpdateDelay.ValueInt64() != 60 {
		t.Errorf("expected Gatekeeper.EnforcedSoftwareUpdateDelay 60 in state")
	}
}

// ---------------------------------------------------------------------------
// Unit tests for credential helper functions
// ---------------------------------------------------------------------------

// internal-ticket removed AnyNetworkReferencesCredential along with the
// credentials_list-drop it gated in PrepareCredentialsForPayload (row #97 of
// the B16 audit); TestAnyNetworkReferencesCredential is removed with it.

func TestUploadCertificatesTyped_ReusesPriorIDForUnchangedPayload(t *testing.T) {
	t.Parallel()

	items := []profilemodels.CredentialItemModel{
		{
			CredentialName:     types.StringValue("cert-a"),
			CredentialSource:   types.StringValue("Upload"),
			CertificatePayload: types.StringValue("payload-a"),
		},
	}
	prior := []profilemodels.CredentialItemModel{
		{
			CredentialName:     types.StringValue("cert-a"),
			CredentialSource:   types.StringValue("Upload"),
			CertificatePayload: types.StringValue("payload-a"),
			CertificateID:      types.Int64Value(1234),
		},
	}

	out, err := profilestate.UploadCertificatesTyped(context.Background(), nil, items, prior)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 credential, got %d", len(out))
	}
	if out[0].CertificateID.IsNull() || out[0].CertificateID.ValueInt64() != 1234 {
		t.Fatalf("expected CertificateID 1234, got %v", out[0].CertificateID)
	}
}

func TestUploadCertificatesTyped_ErrorsWhenUploadRequired(t *testing.T) {
	t.Parallel()

	items := []profilemodels.CredentialItemModel{
		{
			CredentialName:     types.StringValue("cert-new"),
			CredentialSource:   types.StringValue("Upload"),
			CertificatePayload: types.StringValue("new-payload"),
		},
	}

	_, err := profilestate.UploadCertificatesTyped(context.Background(), nil, items, nil)
	if err == nil {
		t.Fatal("expected error when upload is required")
	}
	if !errors.Is(err, commonerrors.ErrCertificateUploadUnsupported) {
		t.Fatalf("expected unsupported upload error, got: %v", err)
	}
}

func TestUploadCertificatesTyped_AllowsExplicitCertificateID(t *testing.T) {
	t.Parallel()

	items := []profilemodels.CredentialItemModel{
		{
			CredentialName:     types.StringValue("cert-existing"),
			CredentialSource:   types.StringValue("Upload"),
			CertificatePayload: types.StringValue("payload"),
			CertificateID:      types.Int64Value(555),
		},
	}

	out, err := profilestate.UploadCertificatesTyped(context.Background(), nil, items, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 credential, got %d", len(out))
	}
	if out[0].CertificateID.IsNull() || out[0].CertificateID.ValueInt64() != 555 {
		t.Fatalf("expected CertificateID 555, got %v", out[0].CertificateID)
	}
}

// internal-ticket removed MergeCredentialsListWithPriorState in full (row #87 of
// the B16 audit): credentials_list is now exactly what
// mapAppleOsXCredentialsList returned, with no name/index-keyed pinning of
// write-only fields, CredentialSource, or CredentialName back onto the API
// response. TestMergeCredentialsListWithPriorState_ByName/_NilAPI are
// removed along with it.

func TestCredentialKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		item    profilemodels.CredentialItemModel
		wantKey string
		wantOK  bool
	}{
		{"valid name", profilemodels.CredentialItemModel{CredentialName: types.StringValue("cert-a")}, "cert-a", true},
		{"empty name", profilemodels.CredentialItemModel{CredentialName: types.StringValue("")}, "", false},
		{"whitespace name", profilemodels.CredentialItemModel{CredentialName: types.StringValue("  ")}, "", false},
		{"null name", profilemodels.CredentialItemModel{}, "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key, ok := profilestate.CredentialKey(tc.item)
			if key != tc.wantKey || ok != tc.wantOK {
				t.Errorf("profilestate.CredentialKey() = (%q, %v), want (%q, %v)", key, ok, tc.wantKey, tc.wantOK)
			}
		})
	}
}

// --- nullWhenConfigNull* top-level modifier tests (internal-task) ---

// TestNullWhenConfigNullObjectModifier_PlanModifyObject covers the three
// cases for the new top-level Object modifier that replaced
// objectplanmodifier.UseStateForUnknown()+useStateForNull(): removing a
// managed block from config must plan it as null (not copy prior state)
// so the RMW Update actually clears it on apply; unconfigured-on-create
// must also become null, never unknown; a configured value is untouched.
func TestNullWhenConfigNullObjectModifier_PlanModifyObject(t *testing.T) {
	t.Parallel()

	attrTypes := map[string]attr.Type{"field": types.StringType}

	// unknown config (e.g. a block fed by a not-yet-known variable) must stay
	// unknown: planning null here would make core reject the plan as invalid.
	t.Run("unknown config leaves plan unknown", func(t *testing.T) {
		t.Parallel()
		req := planmodifier.ObjectRequest{
			ConfigValue: types.ObjectUnknown(attrTypes),
			StateValue:  types.ObjectNull(attrTypes),
			PlanValue:   types.ObjectUnknown(attrTypes),
		}
		resp := &planmodifier.ObjectResponse{PlanValue: req.PlanValue}
		nullWhenConfigNullObjectModifier{}.PlanModifyObject(context.Background(), req, resp)
		if !resp.PlanValue.IsUnknown() {
			t.Errorf("expected plan to stay unknown for unknown config, got %v", resp.PlanValue)
		}
	})

	buildObj := func(t *testing.T, v map[string]attr.Value) types.Object {
		t.Helper()
		obj, diags := types.ObjectValue(attrTypes, v)
		if diags.HasError() {
			t.Fatalf("building object value: %v", diags)
		}
		return obj
	}

	// null config + non-null state (block removed from HCL) → plan is null,
	// NOT the prior state value. This is the removal-clears fix itself.
	t.Run("null config with non-null state plans null (removal)", func(t *testing.T) {
		t.Parallel()
		stateVal := buildObj(t, map[string]attr.Value{"field": types.StringValue("from-state")})
		req := planmodifier.ObjectRequest{
			ConfigValue: types.ObjectNull(attrTypes),
			StateValue:  stateVal,
			PlanValue:   types.ObjectUnknown(attrTypes),
		}
		resp := &planmodifier.ObjectResponse{PlanValue: req.PlanValue}
		nullWhenConfigNullObjectModifier{}.PlanModifyObject(context.Background(), req, resp)
		if !resp.PlanValue.IsNull() {
			t.Errorf("expected null plan on removal, got %v", resp.PlanValue)
		}
	})

	// null config + null state (Create without config) → plan is null, never
	// unknown, avoiding "cannot handle unknown values" on decode.
	t.Run("null config with null state plans null, never unknown (create)", func(t *testing.T) {
		t.Parallel()
		req := planmodifier.ObjectRequest{
			ConfigValue: types.ObjectNull(attrTypes),
			StateValue:  types.ObjectNull(attrTypes),
			PlanValue:   types.ObjectUnknown(attrTypes),
		}
		resp := &planmodifier.ObjectResponse{PlanValue: req.PlanValue}
		nullWhenConfigNullObjectModifier{}.PlanModifyObject(context.Background(), req, resp)
		if resp.PlanValue.IsUnknown() {
			t.Errorf("expected plan to never be unknown on create, got %v", resp.PlanValue)
		}
		if !resp.PlanValue.IsNull() {
			t.Errorf("expected null plan on create with no config, got %v", resp.PlanValue)
		}
	})

	// non-null config → modifier is a no-op.
	t.Run("non-null config leaves plan unchanged", func(t *testing.T) {
		t.Parallel()
		planVal := buildObj(t, map[string]attr.Value{"field": types.StringValue("configured")})
		req := planmodifier.ObjectRequest{
			ConfigValue: planVal,
			StateValue:  types.ObjectNull(attrTypes),
			PlanValue:   planVal,
		}
		resp := &planmodifier.ObjectResponse{PlanValue: req.PlanValue}
		nullWhenConfigNullObjectModifier{}.PlanModifyObject(context.Background(), req, resp)
		if !resp.PlanValue.Equal(planVal) {
			t.Errorf("expected plan unchanged when config is set, got %v", resp.PlanValue)
		}
	})
}

// TestNullWhenConfigNullListModifier_PlanModifyList is the List-typed
// equivalent of TestNullWhenConfigNullObjectModifier_PlanModifyObject,
// covering custom_settings_list/network_list/credentials_list.
func TestNullWhenConfigNullListModifier_PlanModifyList(t *testing.T) {
	t.Parallel()

	elemType := types.StringType

	// unknown config must stay unknown (see the Object test).
	t.Run("unknown config leaves plan unknown", func(t *testing.T) {
		t.Parallel()
		req := planmodifier.ListRequest{
			ConfigValue: types.ListUnknown(elemType),
			StateValue:  types.ListNull(elemType),
			PlanValue:   types.ListUnknown(elemType),
		}
		resp := &planmodifier.ListResponse{PlanValue: req.PlanValue}
		nullWhenConfigNullListModifier{}.PlanModifyList(context.Background(), req, resp)
		if !resp.PlanValue.IsUnknown() {
			t.Errorf("expected plan to stay unknown for unknown config, got %v", resp.PlanValue)
		}
	})

	t.Run("null config with non-null state plans null (removal)", func(t *testing.T) {
		t.Parallel()
		stateVal, diags := types.ListValue(elemType, []attr.Value{types.StringValue("from-state")})
		if diags.HasError() {
			t.Fatalf("building list value: %v", diags)
		}
		req := planmodifier.ListRequest{
			ConfigValue: types.ListNull(elemType),
			StateValue:  stateVal,
			PlanValue:   types.ListUnknown(elemType),
		}
		resp := &planmodifier.ListResponse{PlanValue: req.PlanValue}
		nullWhenConfigNullListModifier{}.PlanModifyList(context.Background(), req, resp)
		if !resp.PlanValue.IsNull() {
			t.Errorf("expected null plan on removal, got %v", resp.PlanValue)
		}
	})

	t.Run("null config with null state plans null, never unknown (create)", func(t *testing.T) {
		t.Parallel()
		req := planmodifier.ListRequest{
			ConfigValue: types.ListNull(elemType),
			StateValue:  types.ListNull(elemType),
			PlanValue:   types.ListUnknown(elemType),
		}
		resp := &planmodifier.ListResponse{PlanValue: req.PlanValue}
		nullWhenConfigNullListModifier{}.PlanModifyList(context.Background(), req, resp)
		if resp.PlanValue.IsUnknown() {
			t.Errorf("expected plan to never be unknown on create, got %v", resp.PlanValue)
		}
		if !resp.PlanValue.IsNull() {
			t.Errorf("expected null plan on create with no config, got %v", resp.PlanValue)
		}
	})

	t.Run("non-null config leaves plan unchanged", func(t *testing.T) {
		t.Parallel()
		planVal, diags := types.ListValue(elemType, []attr.Value{types.StringValue("configured")})
		if diags.HasError() {
			t.Fatalf("building list value: %v", diags)
		}
		req := planmodifier.ListRequest{
			ConfigValue: planVal,
			StateValue:  types.ListNull(elemType),
			PlanValue:   planVal,
		}
		resp := &planmodifier.ListResponse{PlanValue: req.PlanValue}
		nullWhenConfigNullListModifier{}.PlanModifyList(context.Background(), req, resp)
		if !resp.PlanValue.Equal(planVal) {
			t.Errorf("expected plan unchanged when config is set, got %v", resp.PlanValue)
		}
	})
}

// TestNullWhenConfigNullStringModifier_PlanModifyString is the String-typed
// equivalent, covering the top-level description attribute.
func TestDescriptionPlanModifier_PlanModifyString(t *testing.T) {
	t.Parallel()

	// unknown config must stay unknown (see the Object test).
	t.Run("unknown config leaves plan unknown", func(t *testing.T) {
		t.Parallel()
		req := planmodifier.StringRequest{
			ConfigValue: types.StringUnknown(),
			StateValue:  types.StringNull(),
			PlanValue:   types.StringUnknown(),
		}
		resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
		descriptionPlanModifier{}.PlanModifyString(context.Background(), req, resp)
		if !resp.PlanValue.IsUnknown() {
			t.Errorf("expected plan to stay unknown for unknown config, got %v", resp.PlanValue)
		}
	})

	t.Run("null config with non-empty state plans empty (removal clears)", func(t *testing.T) {
		t.Parallel()
		req := planmodifier.StringRequest{
			ConfigValue: types.StringNull(),
			StateValue:  types.StringValue("from-state"),
			PlanValue:   types.StringUnknown(),
		}
		resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
		descriptionPlanModifier{}.PlanModifyString(context.Background(), req, resp)
		if resp.PlanValue.IsNull() || resp.PlanValue.IsUnknown() || resp.PlanValue.ValueString() != "" {
			t.Errorf("expected \"\" plan on removal, got %v", resp.PlanValue)
		}
	})

	t.Run("null config with empty state keeps empty (no diff after UEM stored empty)", func(t *testing.T) {
		t.Parallel()
		req := planmodifier.StringRequest{
			ConfigValue: types.StringNull(),
			StateValue:  types.StringValue(""),
			PlanValue:   types.StringUnknown(),
		}
		resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
		descriptionPlanModifier{}.PlanModifyString(context.Background(), req, resp)
		if resp.PlanValue.IsUnknown() || resp.PlanValue.ValueString() != "" {
			t.Errorf("expected \"\" plan when state is empty, got %v", resp.PlanValue)
		}
	})

	t.Run("null config with null state stays unknown on create", func(t *testing.T) {
		t.Parallel()
		req := planmodifier.StringRequest{
			ConfigValue: types.StringNull(),
			StateValue:  types.StringNull(),
			PlanValue:   types.StringUnknown(),
		}
		resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
		descriptionPlanModifier{}.PlanModifyString(context.Background(), req, resp)
		if !resp.PlanValue.IsUnknown() {
			t.Errorf("expected unknown plan on create with no config, so UEM's stored value is accepted; got %v", resp.PlanValue)
		}
	})

	t.Run("non-null config leaves plan unchanged", func(t *testing.T) {
		t.Parallel()
		req := planmodifier.StringRequest{
			ConfigValue: types.StringValue("configured"),
			StateValue:  types.StringNull(),
			PlanValue:   types.StringValue("configured"),
		}
		resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
		descriptionPlanModifier{}.PlanModifyString(context.Background(), req, resp)
		if resp.PlanValue.ValueString() != "configured" {
			t.Errorf("expected plan unchanged when config is set, got %v", resp.PlanValue)
		}
	})
}

// TestCredentialSourceCanonicalizeModifier_PlanModifyString covers b16
// decision table row #52 (CORRECT): UEM's API only ever returns/expects
// "DefinedCA" for a CA-backed credential (canonical Q7), so a plan carrying
// the legacy "DefinedCertificateAuthority" spelling must be canonicalized to
// "DefinedCA" to avoid a perpetual diff against UEM's own GET readback on a
// Required (non-Computed) attribute.
func TestCredentialSourceCanonicalizeModifier_PlanModifyString(t *testing.T) {
	t.Parallel()

	t.Run("legacy spelling is canonicalized to DefinedCA", func(t *testing.T) {
		t.Parallel()
		req := planmodifier.StringRequest{
			PlanValue: types.StringValue("DefinedCertificateAuthority"),
		}
		resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
		credentialSourceCanonicalizeModifier{}.PlanModifyString(context.Background(), req, resp)
		if resp.PlanValue.ValueString() != "DefinedCA" {
			t.Errorf(`expected "DefinedCA", got %v`, resp.PlanValue)
		}
	})

	t.Run("canonical spelling passes through unchanged", func(t *testing.T) {
		t.Parallel()
		req := planmodifier.StringRequest{
			PlanValue: types.StringValue("DefinedCA"),
		}
		resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
		credentialSourceCanonicalizeModifier{}.PlanModifyString(context.Background(), req, resp)
		if resp.PlanValue.ValueString() != "DefinedCA" {
			t.Errorf(`expected "DefinedCA" unchanged, got %v`, resp.PlanValue)
		}
	})

	t.Run("Upload passes through unchanged", func(t *testing.T) {
		t.Parallel()
		req := planmodifier.StringRequest{
			PlanValue: types.StringValue("Upload"),
		}
		resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
		credentialSourceCanonicalizeModifier{}.PlanModifyString(context.Background(), req, resp)
		if resp.PlanValue.ValueString() != "Upload" {
			t.Errorf(`expected "Upload" unchanged, got %v`, resp.PlanValue)
		}
	})

	t.Run("unknown plan value is left untouched", func(t *testing.T) {
		t.Parallel()
		req := planmodifier.StringRequest{
			PlanValue: types.StringUnknown(),
		}
		resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
		credentialSourceCanonicalizeModifier{}.PlanModifyString(context.Background(), req, resp)
		if !resp.PlanValue.IsUnknown() {
			t.Errorf("expected plan to stay unknown, got %v", resp.PlanValue)
		}
	})

	t.Run("null plan value is left untouched", func(t *testing.T) {
		t.Parallel()
		req := planmodifier.StringRequest{
			PlanValue: types.StringNull(),
		}
		resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
		credentialSourceCanonicalizeModifier{}.PlanModifyString(context.Background(), req, resp)
		if !resp.PlanValue.IsNull() {
			t.Errorf("expected plan to stay null, got %v", resp.PlanValue)
		}
	})
}

// --- useStateForNull{List,String,Bool,Int64} nested-attribute modifier
// tests (unchanged usage: they still back nested attrs like network_list
// items and credentials_list items, so their existing "copy state on null
// config" behavior remains correct there and is exercised here directly). ---

func TestUseStateForNullListModifier_PlanModifyList(t *testing.T) {
	t.Parallel()

	elemType := types.StringType

	t.Run("null config with prior state copies state to plan", func(t *testing.T) {
		t.Parallel()
		stateVal, diags := types.ListValue(elemType, []attr.Value{types.StringValue("from-state")})
		if diags.HasError() {
			t.Fatalf("building list value: %v", diags)
		}
		req := planmodifier.ListRequest{
			ConfigValue: types.ListNull(elemType),
			StateValue:  stateVal,
			PlanValue:   stateVal,
		}
		resp := &planmodifier.ListResponse{PlanValue: req.PlanValue}
		useStateForNullListModifier{}.PlanModifyList(context.Background(), req, resp)
		if !resp.PlanValue.Equal(stateVal) {
			t.Errorf("expected state value in plan, got %v", resp.PlanValue)
		}
	})

	t.Run("null config and state with unknown plan becomes null on Create", func(t *testing.T) {
		t.Parallel()
		req := planmodifier.ListRequest{
			ConfigValue: types.ListNull(elemType),
			StateValue:  types.ListNull(elemType),
			PlanValue:   types.ListUnknown(elemType),
		}
		resp := &planmodifier.ListResponse{PlanValue: req.PlanValue}
		useStateForNullListModifier{}.PlanModifyList(context.Background(), req, resp)
		if !resp.PlanValue.IsNull() {
			t.Errorf("expected null plan on Create with no config, got %v", resp.PlanValue)
		}
	})
}

func TestUseStateForNullStringModifier_PlanModifyString(t *testing.T) {
	t.Parallel()

	t.Run("null config with prior state copies state to plan", func(t *testing.T) {
		t.Parallel()
		req := planmodifier.StringRequest{
			ConfigValue: types.StringNull(),
			StateValue:  types.StringValue("from-state"),
			PlanValue:   types.StringUnknown(),
		}
		resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
		useStateForNullStringModifier{}.PlanModifyString(context.Background(), req, resp)
		if resp.PlanValue.ValueString() != "from-state" {
			t.Errorf("expected state value in plan, got %v", resp.PlanValue)
		}
	})
}

// TestNullWhenConfigNullModifiers_ImportShapedConfigIsANoOp builds state
// from a realistic fetched macOS entity via the real read mappers
// (profilestate.ReadProfileIntoState — the same call path Read/Create/
// Update use), simulating an import. It then builds a generator-style
// config that mirrors state exactly, attribute-for-attribute (the same
// value where state is non-null, null where state is null) — what
// `terraform plan -generate-config-out` produces after import — and
// verifies each of the 8 nullWhenConfigNull* top-level modifiers leaves the
// plan completely unchanged: an empty plan at the modifier level, because
// config == state always lands in the "non-null config, no-op" branch.
func TestNullWhenConfigNullModifiers_ImportShapedConfigIsANoOp(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	var data profilemodels.ProfileResourceModel
	profilestate.ReadProfileIntoState(ctx, &data, &sdk.ProfileResult{AppleOsX: liveAppleOsXEntityAllBlocks()})

	var schemaResp resource.SchemaResponse
	(&ProfileResource{}).Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	sch := schemaResp.Schema

	checkObject := func(name string, modelValue any) {
		t.Helper()
		objType, ok := sch.Attributes[name].GetType().(types.ObjectType)
		if !ok {
			t.Fatalf("%s: expected an Object-typed attribute", name)
		}
		val, diags := types.ObjectValueFrom(ctx, objType.AttrTypes, modelValue)
		if diags.HasError() {
			t.Fatalf("%s: building object value: %v", name, diags)
		}
		req := planmodifier.ObjectRequest{ConfigValue: val, StateValue: val, PlanValue: val}
		resp := &planmodifier.ObjectResponse{PlanValue: req.PlanValue}
		nullWhenConfigNullObjectModifier{}.PlanModifyObject(ctx, req, resp)
		if !resp.PlanValue.Equal(val) {
			t.Errorf("%s: expected plan unchanged for import-shaped config, got %v (want %v)", name, resp.PlanValue, val)
		}
	}

	checkList := func(name string, modelValue any) {
		t.Helper()
		listType, ok := sch.Attributes[name].GetType().(types.ListType)
		if !ok {
			t.Fatalf("%s: expected a List-typed attribute", name)
		}
		val, diags := types.ListValueFrom(ctx, listType.ElemType, modelValue)
		if diags.HasError() {
			t.Fatalf("%s: building list value: %v", name, diags)
		}
		req := planmodifier.ListRequest{ConfigValue: val, StateValue: val, PlanValue: val}
		resp := &planmodifier.ListResponse{PlanValue: req.PlanValue}
		nullWhenConfigNullListModifier{}.PlanModifyList(ctx, req, resp)
		if !resp.PlanValue.Equal(val) {
			t.Errorf("%s: expected plan unchanged for import-shaped config, got %v (want %v)", name, resp.PlanValue, val)
		}
	}

	checkObject("passcode", data.Passcode)
	checkObject("disk_encryption", data.DiskEncryption)
	checkObject("gatekeeper", data.Gatekeeper)
	checkObject("restrictions", data.Restrictions)
	checkList("custom_settings_list", data.CustomSettingsList)
	checkList("network_list", data.NetworkList)
	checkList("credentials_list", data.CredentialsList)

	descReq := planmodifier.StringRequest{ConfigValue: data.Description, StateValue: data.Description, PlanValue: data.Description}
	descResp := &planmodifier.StringResponse{PlanValue: descReq.PlanValue}
	descriptionPlanModifier{}.PlanModifyString(ctx, descReq, descResp)
	if !descResp.PlanValue.Equal(data.Description) {
		t.Errorf("description: expected plan unchanged for import-shaped config, got %v (want %v)", descResp.PlanValue, data.Description)
	}

	// Sanity: this test is only meaningful if the fetched entity actually
	// populated every block; otherwise the assertions above would trivially
	// pass on already-null values without ever exercising the non-null
	// "leave it alone" branch this test exists to cover.
	if data.Passcode == nil || data.DiskEncryption == nil || data.Gatekeeper == nil ||
		data.Restrictions == nil || len(data.CustomSettingsList) == 0 ||
		len(data.NetworkList) == 0 || len(data.CredentialsList) == 0 || data.Description.IsNull() {
		t.Fatal("test setup: liveAppleOsXEntityAllBlocks() must populate every one of the 8 attributes for this test to be meaningful")
	}
}

// --- Description Optional+Computed tests ---

// TestProfileResourceCreate_DescriptionReadBackFromAPI verifies that when the
// user omits description in config (unknown in plan from the framework),
// Create reads it back from the API GET response and saves the known value
// to state, satisfying the "no unknown values after apply" contract.
func TestProfileResourceCreate_DescriptionReadBackFromAPI(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && strings.Contains(r.URL.Path, "/create"):
			_ = json.NewEncoder(w).Encode(11111)
		case r.Method == "GET":
			resp := map[string]interface{}{
				"General": map[string]interface{}{
					"ProfileId":              11111,
					"Name":                   "No-Desc Profile",
					"Description":            "Returned by UEM",
					"AssignmentType":         "Auto",
					"ProfileScope":           "Production",
					"ManagedLocationGroupID": 14165,
					"IsActive":               true,
					"Uuid":                   "uuid-nodesc",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := &ProfileResource{client: c}
	ctx := context.Background()

	// description has an unknown value — the framework sets this when the
	// attribute is Optional+Computed and the user didn't include it in config.
	plan := createResourcePlan(t, map[string]tftypes.Value{
		"id":                   nullString(),
		"name":                 stringVal("No-Desc Profile"),
		"description":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"platform":             stringVal("Android"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      nullString(),
		"profile_scope":        nullString(),
		"is_active":            nullBool(),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 nullString(),
		"profile_context":      nullString(),
	})

	req := resource.CreateRequest{Plan: plan}
	resp := &resource.CreateResponse{State: emptyResourceState(t)}
	res.Create(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors: %v", msgs)
	}

	var model profilemodels.ProfileResourceModel
	resp.State.Get(ctx, &model)

	if model.Description.IsUnknown() {
		t.Fatal("Description must not be unknown after Create")
	}
	if model.Description.ValueString() != "Returned by UEM" {
		t.Errorf("expected Description 'Returned by UEM', got %q", model.Description.ValueString())
	}
}

// TestProfileResourceCreate_PostCreateGetFails_DescriptionResolved verifies
// that when Create succeeds but the follow-up Get fails, an unknown description
// is resolved to a known empty string (safety net) rather than left unknown,
// which would cause Terraform to fail with "provider returned unknown value".
func TestProfileResourceCreate_PostCreateGetFails_DescriptionResolved(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/create") {
			_ = json.NewEncoder(w).Encode(22222)
			return
		}
		// 404 keeps the client from retrying 5xx responses.
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "not found"})
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := newProfileResourceWithRegistry(t, c)
	ctx := context.Background()

	plan := createResourcePlan(t, map[string]tftypes.Value{
		"id":                   nullString(),
		"name":                 stringVal("Profile"),
		"description":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"platform":             stringVal("Android"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      nullString(),
		"profile_scope":        nullString(),
		"is_active":            nullBool(),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 nullString(),
		"profile_context":      nullString(),
	})

	req := resource.CreateRequest{Plan: plan}
	resp := &resource.CreateResponse{State: emptyResourceState(t)}
	res.Create(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors after Create (description must not stay unknown): %v", msgs)
	}

	var model profilemodels.ProfileResourceModel
	resp.State.Get(ctx, &model)

	if model.Description.IsUnknown() {
		t.Fatal("Description must not be unknown after Create even when Get fails")
	}
	// Safety net resolves the unknown to empty string.
	if model.Description.ValueString() != "" {
		t.Errorf("expected empty Description when Get fails, got %q", model.Description.ValueString())
	}
}

// TestProfileResourceCreate_NoBlocksConfigured verifies that Create for a
// macOS profile with every one of the 8 nullWhenConfigNull*-modified
// attributes null (exactly what those modifiers plan for an unconfigured
// Optional+Computed attribute on Create — never unknown, per internal-task)
// decodes and completes without a "cannot handle unknown values" error.
func TestProfileResourceCreate_NoBlocksConfigured(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && strings.Contains(r.URL.Path, "/create"):
			_ = json.NewEncoder(w).Encode(66666)
		case r.Method == "GET":
			resp := map[string]interface{}{
				"General": map[string]interface{}{
					"ProfileId":              66666,
					"Name":                   "No Blocks",
					"ProfileUuid":            "uuid-noblocks",
					"ProfileContext":         "Device",
					"ManagedLocationGroupID": 14165,
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := &ProfileResource{client: c}
	ctx := context.Background()

	plan := createResourcePlan(t, map[string]tftypes.Value{
		"id":                   nullString(),
		"name":                 stringVal("No Blocks"),
		"description":          nullString(),
		"platform":             stringVal("AppleOsX"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      nullString(),
		"profile_scope":        nullString(),
		"is_active":            nullBool(),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 nullString(),
		"profile_context":      nullString(),
	})

	req := resource.CreateRequest{Plan: plan}
	resp := &resource.CreateResponse{State: emptyResourceState(t)}
	res.Create(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors creating with no blocks configured: %v", msgs)
	}

	var model profilemodels.ProfileResourceModel
	resp.State.Get(ctx, &model)
	if model.Description.IsUnknown() {
		t.Error("Description must not be unknown after Create")
	}
	// internal-ticket removed setDescriptionFromAPI's null defaulting (row #57 of
	// the B16 audit): the server's omitted Description now stores as "" (the
	// SDK can't distinguish "omitted" from an explicit "" on this
	// non-pointer field), not null.
	if model.Description.IsNull() || model.Description.ValueString() != "" {
		t.Errorf("expected Description \"\" (server value as-is, no null defaulting), got %v", model.Description)
	}
	if model.Passcode != nil {
		t.Errorf("expected Passcode nil, got %+v", model.Passcode)
	}
	if model.DiskEncryption != nil {
		t.Errorf("expected DiskEncryption nil, got %+v", model.DiskEncryption)
	}
	if model.Gatekeeper != nil {
		t.Errorf("expected Gatekeeper nil, got %+v", model.Gatekeeper)
	}
	if model.Restrictions != nil {
		t.Errorf("expected Restrictions nil, got %+v", model.Restrictions)
	}
}

// --- Restrictions (macOS) tests: representative subset across sub-blocks ---

func TestProfileResourceCreate_WithRestrictions(t *testing.T) {
	t.Parallel()

	var capturedBody map[string]interface{}

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && strings.Contains(r.URL.Path, "/create"):
			_ = json.NewDecoder(r.Body).Decode(&capturedBody)
			_ = json.NewEncoder(w).Encode(55555)
		case r.Method == "GET":
			resp := map[string]interface{}{
				"General": map[string]interface{}{
					"ProfileId":              55555,
					"Name":                   "macOS Restrictions",
					"ProfileUuid":            "uuid-restrictions",
					"ProfileContext":         "Device",
					"ManagedLocationGroupID": 14165,
				},
				"Restrictions": map[string]interface{}{
					"Applications": map[string]interface{}{
						"Camera": map[string]interface{}{
							"AllowUseOfBuiltInCamera": false,
						},
					},
					"Preferences": map[string]interface{}{
						"AppStore":           true,
						"PreferenceBehavior": "allow",
					},
					"Widgets": map[string]interface{}{
						"AllowOnlyConfiguredWidgets": true,
						"AllowedWidgets":             []interface{}{"com.apple.widget.weather"},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := &ProfileResource{client: c}
	ctx := context.Background()

	widgetsListType := tftypes.List{ElementType: tftypes.String}
	widgetsList := tftypes.NewValue(widgetsListType, []tftypes.Value{stringVal("com.apple.widget.weather")})

	plan := createResourcePlan(t, map[string]tftypes.Value{
		"id":                   nullString(),
		"name":                 stringVal("macOS Restrictions"),
		"description":          stringVal("Restrictions policy"),
		"platform":             stringVal("AppleOsX"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      nullString(),
		"profile_scope":        nullString(),
		"is_active":            nullBool(),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions": restrictionsVal(map[string]tftypes.Value{
			"applications": restrictionsApplicationsVal(map[string]tftypes.Value{
				"camera": restrictionsCameraVal(map[string]tftypes.Value{
					"allow_use_of_built_in_camera": boolVal(false),
				}),
			}),
			"preferences": restrictionsPreferencesVal(map[string]tftypes.Value{
				"app_store":           boolVal(true),
				"preference_behavior": stringVal("allow"),
			}),
			"widgets": restrictionsWidgetsVal(map[string]tftypes.Value{
				"allow_only_configured_widgets": boolVal(true),
				"allowed_widgets":               widgetsList,
			}),
		}),
		"uuid":            nullString(),
		"profile_context": nullString(),
	})

	req := resource.CreateRequest{Plan: plan}
	resp := &resource.CreateResponse{State: emptyResourceState(t)}

	res.Create(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors: %v", msgs)
	}

	if capturedBody == nil {
		t.Fatal("expected request body to be captured")
	}
	r, ok := capturedBody["Restrictions"].(map[string]interface{})
	if !ok {
		t.Fatal("expected Restrictions in request body")
	}
	apps, ok := r["Applications"].(map[string]interface{})
	if !ok {
		t.Fatal("expected Applications in Restrictions")
	}
	cam, ok := apps["Camera"].(map[string]interface{})
	if !ok {
		t.Fatal("expected Camera in Applications")
	}
	if cam["AllowUseOfBuiltInCamera"] != false {
		t.Errorf("expected AllowUseOfBuiltInCamera=false, got %v", cam["AllowUseOfBuiltInCamera"])
	}
	prefs, ok := r["Preferences"].(map[string]interface{})
	if !ok {
		t.Fatal("expected Preferences in Restrictions")
	}
	if prefs["AppStore"] != true {
		t.Errorf("expected Preferences.AppStore=true, got %v", prefs["AppStore"])
	}
	if prefs["PreferenceBehavior"] != "allow" {
		t.Errorf("expected PreferenceBehavior=allow, got %v", prefs["PreferenceBehavior"])
	}
	widgets, ok := r["Widgets"].(map[string]interface{})
	if !ok {
		t.Fatal("expected Widgets in Restrictions")
	}
	if widgets["AllowOnlyConfiguredWidgets"] != true {
		t.Errorf("expected AllowOnlyConfiguredWidgets=true, got %v", widgets["AllowOnlyConfiguredWidgets"])
	}
	aw, ok := widgets["AllowedWidgets"].([]interface{})
	if !ok || len(aw) != 1 || aw[0] != "com.apple.widget.weather" {
		t.Errorf("expected AllowedWidgets=[com.apple.widget.weather], got %v", widgets["AllowedWidgets"])
	}

	var model profilemodels.ProfileResourceModel
	resp.State.Get(ctx, &model)

	if model.ID.ValueString() != "55555" {
		t.Errorf("expected ID '55555', got '%s'", model.ID.ValueString())
	}
	if model.Restrictions == nil {
		t.Fatal("expected Restrictions populated after read-back")
	}
	if model.Restrictions.Applications == nil || model.Restrictions.Applications.Camera == nil {
		t.Fatal("expected Applications.Camera populated after read-back")
	}
	if model.Restrictions.Applications.Camera.AllowUseOfBuiltInCamera.ValueBool() != false {
		t.Errorf("expected AllowUseOfBuiltInCamera=false in state")
	}
	if model.Restrictions.Preferences == nil || model.Restrictions.Preferences.AppStore.ValueBool() != true {
		t.Errorf("expected Preferences.AppStore=true in state")
	}
	if model.Restrictions.Preferences.PreferenceBehavior.ValueString() != "allow" {
		t.Errorf("expected PreferenceBehavior=allow in state, got %q", model.Restrictions.Preferences.PreferenceBehavior.ValueString())
	}
	if model.Restrictions.Widgets == nil {
		t.Fatal("expected Widgets populated after read-back")
	}
	if model.Restrictions.Widgets.AllowOnlyConfiguredWidgets.ValueBool() != true {
		t.Errorf("expected AllowOnlyConfiguredWidgets=true in state")
	}
	if model.Restrictions.Widgets.AllowedWidgets.IsNull() {
		t.Error("expected AllowedWidgets populated in state")
	}
}

func TestProfileResourceRead_WithRestrictions(t *testing.T) {
	t.Parallel()

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"General": map[string]interface{}{
				"ProfileId":              55555,
				"Name":                   "macOS Restrictions",
				"Description":            "Policy",
				"AssignmentType":         "Auto",
				"ProfileScope":           "Production",
				"ManagedLocationGroupID": 14165,
				"IsActive":               true,
				"ProfileUuid":            "uuid-restrictions-read",
				"ProfileContext":         "Device",
			},
			"Restrictions": map[string]interface{}{
				"Functionality": map[string]interface{}{
					"ICloud": map[string]interface{}{
						"AllowIcloudMail":      false,
						"AllowIcloudBookmarks": false,
					},
				},
				"Sharing": map[string]interface{}{
					"Twitter":  false,
					"Facebook": false,
				},
				"Media": map[string]interface{}{
					"AutoEjectMedia": true,
					"DiskMediaCDs": map[string]interface{}{
						"Allow":     false,
						"Read-Only": true,
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := newProfileResourceWithRegistry(t, c, discoveryEntry(55555, sdk.PlatformAppleOsX))
	ctx := context.Background()

	state := createResourceState(t, map[string]tftypes.Value{
		"id":                   stringVal("55555"),
		"name":                 stringVal("Old Name"),
		"description":          stringVal("Old"),
		"platform":             stringVal("AppleOsX"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      stringVal("Auto"),
		"profile_scope":        stringVal("Production"),
		"is_active":            boolVal(true),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 stringVal("old-uuid"),
		"profile_context":      stringVal("Device"),
	})

	req := resource.ReadRequest{State: state}
	resp := &resource.ReadResponse{State: state}

	res.Read(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors: %v", msgs)
	}

	var model profilemodels.ProfileResourceModel
	resp.State.Get(ctx, &model)

	if model.Restrictions == nil {
		t.Fatal("expected Restrictions populated from API response")
	}
	if model.Restrictions.Functionality == nil || model.Restrictions.Functionality.ICloud == nil {
		t.Fatal("expected Functionality.ICloud populated")
	}
	if model.Restrictions.Functionality.ICloud.AllowIcloudMail.ValueBool() != false {
		t.Error("expected AllowIcloudMail=false")
	}
	if model.Restrictions.Functionality.ICloud.AllowIcloudBookmarks.ValueBool() != false {
		t.Error("expected AllowIcloudBookmarks=false")
	}
	if model.Restrictions.Sharing == nil {
		t.Fatal("expected Sharing populated")
	}
	if model.Restrictions.Sharing.Twitter.ValueBool() != false {
		t.Error("expected Sharing.Twitter=false")
	}
	if model.Restrictions.Media == nil {
		t.Fatal("expected Media populated")
	}
	if model.Restrictions.Media.AutoEjectMedia.ValueBool() != true {
		t.Error("expected Media.AutoEjectMedia=true")
	}
	if model.Restrictions.Media.DiskMediaCDs == nil {
		t.Fatal("expected Media.DiskMediaCDs populated")
	}
	if model.Restrictions.Media.DiskMediaCDs.Allow.ValueBool() != false {
		t.Error("expected DiskMediaCDs.Allow=false")
	}
	if model.Restrictions.Media.DiskMediaCDs.ReadOnly.ValueBool() != true {
		t.Error("expected DiskMediaCDs.ReadOnly=true")
	}
}

func TestProfileResourceUpdate_WithRestrictions(t *testing.T) {
	t.Parallel()

	var capturedBody map[string]interface{}

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && strings.Contains(r.URL.Path, "/update"):
			_ = json.NewDecoder(r.Body).Decode(&capturedBody)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{})
		case r.Method == "GET":
			resp := map[string]interface{}{
				"General": map[string]interface{}{
					"ProfileId":      66666,
					"Name":           "Updated Restrictions",
					"ProfileUuid":    "uuid-updated-restr",
					"ProfileContext": "Device",
				},
				"Restrictions": map[string]interface{}{
					"Applications": map[string]interface{}{
						"Camera": map[string]interface{}{
							"AllowUseOfBuiltInCamera": true,
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}

	c, server := createTestClient(t, handler)
	defer server.Close()

	res := newProfileResourceWithRegistry(t, c, discoveryEntry(66666, sdk.PlatformAppleOsX))
	ctx := context.Background()

	plan := createResourcePlan(t, map[string]tftypes.Value{
		"id":                   stringVal("66666"),
		"name":                 stringVal("Updated Restrictions"),
		"description":          stringVal("Updated"),
		"platform":             stringVal("AppleOsX"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      stringVal("Auto"),
		"profile_scope":        stringVal("Production"),
		"is_active":            boolVal(true),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions": restrictionsVal(map[string]tftypes.Value{
			"applications": restrictionsApplicationsVal(map[string]tftypes.Value{
				"camera": restrictionsCameraVal(map[string]tftypes.Value{
					"allow_use_of_built_in_camera": boolVal(true),
				}),
			}),
		}),
		"uuid":            stringVal("uuid-restr-old"),
		"profile_context": stringVal("Device"),
	})
	priorState := createResourceState(t, map[string]tftypes.Value{
		"id":                   stringVal("66666"),
		"name":                 stringVal("Old Restrictions"),
		"description":          stringVal("Old"),
		"platform":             stringVal("AppleOsX"),
		"org_group_id":         stringVal("14165"),
		"assignment_type":      stringVal("Auto"),
		"profile_scope":        stringVal("Production"),
		"is_active":            boolVal(true),
		"lock_screen_message":  nullString(),
		"passcode":             nullPasscode(),
		"custom_settings_list": nullCustomSettingsList(),
		"network_list":         nullNetworkList(),
		"credentials_list":     nullCredentialsList(),
		"disk_encryption":      nullDiskEncryption(),
		"gatekeeper":           nullGatekeeper(),
		"restrictions":         nullRestrictions(),
		"uuid":                 stringVal("uuid-restr-old"),
		"profile_context":      stringVal("Device"),
	})

	req := resource.UpdateRequest{Plan: plan, State: priorState}
	resp := &resource.UpdateResponse{State: priorState}

	res.Update(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		var msgs []string
		for _, d := range resp.Diagnostics.Errors() {
			msgs = append(msgs, d.Summary()+": "+d.Detail())
		}
		t.Fatalf("unexpected errors: %v", msgs)
	}

	if capturedBody == nil {
		t.Fatal("expected update request body to be captured")
	}
	r, ok := capturedBody["Restrictions"].(map[string]interface{})
	if !ok {
		t.Fatal("expected Restrictions in update request body")
	}
	apps, ok := r["Applications"].(map[string]interface{})
	if !ok {
		t.Fatal("expected Applications in update request")
	}
	cam, ok := apps["Camera"].(map[string]interface{})
	if !ok {
		t.Fatal("expected Camera in update request")
	}
	if cam["AllowUseOfBuiltInCamera"] != true {
		t.Errorf("expected AllowUseOfBuiltInCamera=true after update, got %v", cam["AllowUseOfBuiltInCamera"])
	}

	var model profilemodels.ProfileResourceModel
	resp.State.Get(ctx, &model)
	if model.Restrictions == nil || model.Restrictions.Applications == nil || model.Restrictions.Applications.Camera == nil {
		t.Fatal("expected Applications.Camera populated in state after update")
	}
	if model.Restrictions.Applications.Camera.AllowUseOfBuiltInCamera.ValueBool() != true {
		t.Errorf("expected Camera.AllowUseOfBuiltInCamera=true in state")
	}
}

// TestProfileResourceSchema_ProfileScopeKeepsStateWhenUnset proves an unset
// profile_scope keeps its stored value on update instead of planning a
// change on every update.
func TestProfileResourceSchema_ProfileScopeKeepsStateWhenUnset(t *testing.T) {
	r := &ProfileResource{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	a, ok := resp.Schema.Attributes["profile_scope"].(resourceSchema.StringAttribute)
	if !ok {
		t.Fatal("profile_scope: not a StringAttribute")
	}
	want := stringplanmodifier.UseStateForUnknown().Description(context.Background())
	for _, m := range a.PlanModifiers {
		if m.Description(context.Background()) == want {
			return
		}
	}
	t.Fatal("profile_scope must keep its stored value when unset (UseStateForUnknown), so an update does not replan it")
}
