package profile

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// privacyPreferencesAttrs returns the privacy_preferences identity attributes
// and its apple_events_list entry attributes from the real resource schema.
func privacyPreferencesAttrs(t *testing.T) (identity, appleEvent map[string]schema.Attribute) {
	t.Helper()
	var resp resource.SchemaResponse
	(&ProfileResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	pp, ok := resp.Schema.Attributes["privacy_preferences"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatal("privacy_preferences is not a ListNestedAttribute")
	}
	identity = pp.NestedObject.Attributes
	ae, ok := identity["apple_events_list"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatal("apple_events_list is not a ListNestedAttribute")
	}
	return identity, ae.NestedObject.Attributes
}

func stringAttr(t *testing.T, attr schema.Attribute) schema.StringAttribute {
	t.Helper()
	sa, ok := attr.(schema.StringAttribute)
	if !ok {
		t.Fatalf("attribute %T is not a StringAttribute", attr)
	}
	return sa
}

func runStringValidators(t *testing.T, attr schema.Attribute, v types.String) bool {
	t.Helper()
	sa := stringAttr(t, attr)
	for _, val := range sa.Validators {
		resp := &validator.StringResponse{}
		val.ValidateString(context.Background(), validator.StringRequest{Path: path.Root("x"), ConfigValue: v}, resp)
		if resp.Diagnostics.HasError() {
			return false
		}
	}
	return true
}

// TestPrivacyPreferencesValidators pins each PPPC validator to its SDK doc
// constraint exactly: every documented value is accepted, a near-miss is
// rejected, and unknown/null skip. Fields whose SDK doc states no constraint
// must carry no validator at all.
func TestPrivacyPreferencesValidators(t *testing.T) {
	t.Parallel()

	identity, appleEvent := privacyPreferencesAttrs(t)
	allowDisallow := []string{"Allow", "Disallow"}
	disallowOnly := []string{"Disallow"}
	bundleOrPath := []string{"bundleID", "path"}
	cases := []struct {
		attrs map[string]schema.Attribute
		name  string
		valid []string
		bad   []string
	}{
		{identity, "identifier_type", bundleOrPath, []string{"bundleid", "Path", "teamID"}},
		{appleEvent, "identifier_type", bundleOrPath, []string{"bundleid", "Path"}},
		{identity, "accessibility", allowDisallow, []string{"allow", "Deny"}},
		{identity, "address_book", allowDisallow, []string{"allow", "Deny"}},
		{identity, "calendar", allowDisallow, []string{"allow", "Deny"}},
		{identity, "photos", allowDisallow, []string{"allow", "Deny"}},
		{identity, "post_event", allowDisallow, []string{"allow", "Deny"}},
		{identity, "reminders", allowDisallow, []string{"allow", "Deny"}},
		{identity, "system_policy_all_files", allowDisallow, []string{"allow", "Deny"}},
		{identity, "system_policy_sys_admin_files", allowDisallow, []string{"allow", "Deny"}},
		{identity, "camera", disallowOnly, []string{"Allow", "disallow"}},
		{identity, "microphone", disallowOnly, []string{"Allow", "disallow"}},
		{identity, "listen_event", disallowOnly, []string{"Allow", "disallow"}},
		{identity, "screen_capture", disallowOnly, []string{"Allow", "disallow"}},
	}
	for _, c := range cases {
		for _, v := range c.valid {
			if !runStringValidators(t, c.attrs[c.name], types.StringValue(v)) {
				t.Errorf("%s rejected documented value %q", c.name, v)
			}
		}
		for _, v := range c.bad {
			if runStringValidators(t, c.attrs[c.name], types.StringValue(v)) {
				t.Errorf("%s accepted undocumented value %q", c.name, v)
			}
		}
		if !runStringValidators(t, c.attrs[c.name], types.StringUnknown()) || !runStringValidators(t, c.attrs[c.name], types.StringNull()) {
			t.Errorf("%s must skip unknown/null", c.name)
		}
	}

	for _, name := range []string{
		"identifier", "code_requirement", "comment", "file_provider_presence", "media_library",
		"speech_recognition", "system_policy_desktop_folder", "system_policy_documents_folder",
		"system_policy_downloads_folder", "system_policy_network_volumes", "system_policy_removable_volumes",
	} {
		if n := len(stringAttr(t, identity[name]).Validators); n != 0 {
			t.Errorf("%s has %d validators, want 0 (SDK documents no constraint)", name, n)
		}
	}
	for _, name := range []string{"identifier", "code_requirement", "permission"} {
		if n := len(stringAttr(t, appleEvent[name]).Validators); n != 0 {
			t.Errorf("apple_events_list.%s has %d validators, want 0 (SDK documents no constraint)", name, n)
		}
	}
	for _, name := range []string{"apple_events", "ae_receiver_identifier", "ae_receiver_identifier_type", "ae_receiver_code_requirement"} {
		if _, ok := identity[name]; ok {
			t.Errorf("identity-level %s must not be in the schema (UEM folds it into apple_events_list)", name)
		}
	}
}
