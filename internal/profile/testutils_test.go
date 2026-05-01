package profile

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem"
	"github.com/euc-oss/terraform-sdk-uem/client"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// createTestClient creates a client pointing to a test HTTP server.
func createTestClient(t *testing.T, handler http.HandlerFunc) (*client.Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)

	cfg := &client.Config{
		InstanceURL: server.URL,
		TenantCode:  "test-tenant",
		AuthMethod:  "basic",
		Username:    "test-user",
		Password:    "test-pass",
	}

	c, err := client.NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create test client: %v", err)
	}

	return c, server
}

// profileDiscoveryEntry is a (profileID, platform) pair for withProfileDiscovery.
type profileDiscoveryEntry struct {
	id       int
	platform string
}

// discoveryEntry is a concise constructor for profileDiscoveryEntry.
func discoveryEntry(id int, platform string) profileDiscoveryEntry {
	return profileDiscoveryEntry{id: id, platform: platform}
}

// newProfileResourceWithRegistry constructs a ProfileResource wired with a
// Layer 2 ProfileService whose registry is pre-populated with the supplied
// (id, platform) entries, bypassing the eager /api/mdm/profiles/search
// discovery call. Use this in tests that exercise Read/Update via the
// Layer 2 ProfileService without wanting to stub the discovery endpoint.
func newProfileResourceWithRegistry(_ *testing.T, c *client.Client, entries ...profileDiscoveryEntry) *ProfileResource {
	svc := sdk.NewProfileServiceWithoutDiscovery(c)
	for _, e := range entries {
		svc.RegisterEntry(e.id, e.platform)
	}
	res := &ProfileResource{client: c}
	// Mark the sync.Once as fired while installing the pre-built service so
	// profileService(ctx) returns it without trying to run discovery itself.
	res.profileSvcOnce.Do(func() {
		res.profileSvc = svc
	})
	return res
}

// getResourceSchema returns the profile resource schema for testing.
func getResourceSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	r := &ProfileResource{}
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	return resp
}

// fillMissingAttrs returns a copy of values with every schema attribute
// present — any attribute not provided by the caller is set to a null
// tftypes.Value of the correct type. This lets test call sites specify
// only the fields they care about even as the schema grows.
func fillMissingAttrs(t *testing.T, schemaType tftypes.Type, values map[string]tftypes.Value) map[string]tftypes.Value {
	t.Helper()
	objType, ok := schemaType.(tftypes.Object)
	if !ok {
		t.Fatalf("schema type is not an Object: %T", schemaType)
	}
	out := make(map[string]tftypes.Value, len(objType.AttributeTypes))
	for name, attrType := range objType.AttributeTypes {
		if v, present := values[name]; present {
			out[name] = v
		} else {
			out[name] = tftypes.NewValue(attrType, nil)
		}
	}
	return out
}

// createResourcePlan creates a tfsdk.Plan for resource testing.
func createResourcePlan(t *testing.T, values map[string]tftypes.Value) tfsdk.Plan {
	t.Helper()
	schemaResp := getResourceSchema(t)
	ctx := context.Background()
	planType := schemaResp.Schema.Type().TerraformType(ctx)
	planValue := tftypes.NewValue(planType, fillMissingAttrs(t, planType, values))
	return tfsdk.Plan{
		Schema: schemaResp.Schema,
		Raw:    planValue,
	}
}

// createResourceState creates a tfsdk.State with provided values.
func createResourceState(t *testing.T, values map[string]tftypes.Value) tfsdk.State {
	t.Helper()
	schemaResp := getResourceSchema(t)
	ctx := context.Background()
	stateType := schemaResp.Schema.Type().TerraformType(ctx)
	stateValue := tftypes.NewValue(stateType, fillMissingAttrs(t, stateType, values))
	return tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    stateValue,
	}
}

// emptyResourceState creates a tfsdk.State with all null attribute values.
func emptyResourceState(t *testing.T) tfsdk.State {
	t.Helper()
	schemaResp := getResourceSchema(t)
	ctx := context.Background()
	schemaType := schemaResp.Schema.Type().TerraformType(ctx)
	objType, ok := schemaType.(tftypes.Object)
	if !ok {
		t.Fatal("schema type is not an Object")
	}
	values := make(map[string]tftypes.Value)
	for name, attrType := range objType.AttributeTypes {
		values[name] = tftypes.NewValue(attrType, nil)
	}
	return tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(schemaType, values),
	}
}

// Helper functions for creating tftypes values.

func nullString() tftypes.Value {
	return tftypes.NewValue(tftypes.String, nil)
}

func stringVal(s string) tftypes.Value {
	return tftypes.NewValue(tftypes.String, s)
}

func nullBool() tftypes.Value {
	return tftypes.NewValue(tftypes.Bool, nil)
}

func boolVal(b bool) tftypes.Value {
	return tftypes.NewValue(tftypes.Bool, b)
}

// passcodeObjectType returns the tftypes.Object type for the passcode nested attribute.
func passcodeObjectType() tftypes.Object {
	return tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"require_passcode_on_device":           tftypes.Bool,
			"allow_simple_value":                   tftypes.Bool,
			"require_alphanumeric_value":           tftypes.Bool,
			"minimum_passcode_length":              tftypes.Number,
			"minimum_number_of_complex_characters": tftypes.String,
			"maximum_passcode_age":                 tftypes.String,
			"auto_lock":                            tftypes.String,
			"grace_period":                         tftypes.Number,
			"max_failed_attempts":                  tftypes.String,
			"pin_history":                          tftypes.String,
			"minutes_until_failed_login_reset":     tftypes.Number,
		},
	}
}

// nullPasscode returns a null tftypes.Value for the passcode attribute.
func nullPasscode() tftypes.Value {
	return tftypes.NewValue(passcodeObjectType(), nil)
}

// passcodeVal returns a tftypes.Value for a passcode object with the given field values.
func passcodeVal(vals map[string]tftypes.Value) tftypes.Value {
	objType := passcodeObjectType()
	// Fill in defaults for any missing fields
	defaults := map[string]tftypes.Value{
		"require_passcode_on_device":           tftypes.NewValue(tftypes.Bool, nil),
		"allow_simple_value":                   tftypes.NewValue(tftypes.Bool, nil),
		"require_alphanumeric_value":           tftypes.NewValue(tftypes.Bool, nil),
		"minimum_passcode_length":              tftypes.NewValue(tftypes.Number, nil),
		"minimum_number_of_complex_characters": tftypes.NewValue(tftypes.String, nil),
		"maximum_passcode_age":                 tftypes.NewValue(tftypes.String, nil),
		"auto_lock":                            tftypes.NewValue(tftypes.String, nil),
		"grace_period":                         tftypes.NewValue(tftypes.Number, nil),
		"max_failed_attempts":                  tftypes.NewValue(tftypes.String, nil),
		"pin_history":                          tftypes.NewValue(tftypes.String, nil),
		"minutes_until_failed_login_reset":     tftypes.NewValue(tftypes.Number, nil),
	}
	for k, v := range vals {
		defaults[k] = v
	}
	return tftypes.NewValue(objType, defaults)
}

func int64Val(n int64) tftypes.Value {
	return tftypes.NewValue(tftypes.Number, n)
}

// customSettingsListItemType returns the tftypes type for a single custom_settings_list item.
func customSettingsListItemType() tftypes.Object {
	return tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"custom_settings": tftypes.String,
		},
	}
}

// nullCustomSettingsList returns a null tftypes.Value for custom_settings_list.
func nullCustomSettingsList() tftypes.Value {
	return tftypes.NewValue(tftypes.List{ElementType: customSettingsListItemType()}, nil)
}

// customSettingsListVal returns a tftypes.Value for a list of custom settings items.
func customSettingsListVal(items []string) tftypes.Value {
	itemType := customSettingsListItemType()
	listType := tftypes.List{ElementType: itemType}
	vals := make([]tftypes.Value, len(items))
	for i, item := range items {
		vals[i] = tftypes.NewValue(itemType, map[string]tftypes.Value{
			"custom_settings": stringVal(item),
		})
	}
	return tftypes.NewValue(listType, vals)
}

// networkListItemType returns the tftypes type for a single network_list item.
func networkListItemType() tftypes.Object {
	return tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"network_interface":                     tftypes.String,
			"service_set_identifier":                tftypes.String,
			"hidden_network":                        tftypes.Bool,
			"auto_join":                             tftypes.Bool,
			"security_type":                         tftypes.String,
			"password":                              tftypes.String,
			"use_as_login_window_configuration":     tftypes.Bool,
			"use_directory_authentication":          tftypes.Bool,
			"tls":                                   tftypes.Bool,
			"ttls":                                  tftypes.Bool,
			"leap":                                  tftypes.Bool,
			"peap":                                  tftypes.Bool,
			"eap_fast":                              tftypes.Bool,
			"eap_sim":                               tftypes.Bool,
			"eap_aka":                               tftypes.Bool,
			"tls_minimum_version":                   tftypes.String,
			"tls_maximum_version":                   tftypes.String,
			"disable_association_mac_randomization": tftypes.Bool,
			"user_name":                             tftypes.String,
			"user_password":                         tftypes.String,
			"identity_certificate":                  tftypes.String,
			"inner_identity":                        tftypes.String,
			"outer_identity":                        tftypes.String,
			"use_pac":                               tftypes.Bool,
			"allow_two_rands":                       tftypes.Bool,
			"trusted_certificates":                  tftypes.List{ElementType: tftypes.String},
			"allow_trust_exceptions":                tftypes.Bool,
			"proxy_type":                            tftypes.String,
			"proxy_server":                          tftypes.String,
			"proxy_server_port":                     tftypes.Number,
			"proxy_username":                        tftypes.String,
			"proxy_password":                        tftypes.String,
			"proxy_url":                             tftypes.String,
			"pac_fallback":                          tftypes.Bool,
		},
	}
}

// nullNetworkList returns a null tftypes.Value for network_list.
func nullNetworkList() tftypes.Value {
	return tftypes.NewValue(tftypes.List{ElementType: networkListItemType()}, nil)
}

// networkListVal builds a tftypes list from a slice of network item value maps.
func networkListVal(items []map[string]tftypes.Value) tftypes.Value {
	itemType := networkListItemType()
	listType := tftypes.List{ElementType: itemType}

	// Fill in null defaults for missing keys in each item.
	defaults := map[string]tftypes.Value{
		"network_interface":                     tftypes.NewValue(tftypes.String, nil),
		"service_set_identifier":                tftypes.NewValue(tftypes.String, nil),
		"hidden_network":                        tftypes.NewValue(tftypes.Bool, nil),
		"auto_join":                             tftypes.NewValue(tftypes.Bool, nil),
		"security_type":                         tftypes.NewValue(tftypes.String, nil),
		"password":                              tftypes.NewValue(tftypes.String, nil),
		"use_as_login_window_configuration":     tftypes.NewValue(tftypes.Bool, nil),
		"use_directory_authentication":          tftypes.NewValue(tftypes.Bool, nil),
		"tls":                                   tftypes.NewValue(tftypes.Bool, nil),
		"ttls":                                  tftypes.NewValue(tftypes.Bool, nil),
		"leap":                                  tftypes.NewValue(tftypes.Bool, nil),
		"peap":                                  tftypes.NewValue(tftypes.Bool, nil),
		"eap_fast":                              tftypes.NewValue(tftypes.Bool, nil),
		"eap_sim":                               tftypes.NewValue(tftypes.Bool, nil),
		"eap_aka":                               tftypes.NewValue(tftypes.Bool, nil),
		"tls_minimum_version":                   tftypes.NewValue(tftypes.String, nil),
		"tls_maximum_version":                   tftypes.NewValue(tftypes.String, nil),
		"disable_association_mac_randomization": tftypes.NewValue(tftypes.Bool, nil),
		"user_name":                             tftypes.NewValue(tftypes.String, nil),
		"user_password":                         tftypes.NewValue(tftypes.String, nil),
		"identity_certificate":                  tftypes.NewValue(tftypes.String, nil),
		"inner_identity":                        tftypes.NewValue(tftypes.String, nil),
		"outer_identity":                        tftypes.NewValue(tftypes.String, nil),
		"use_pac":                               tftypes.NewValue(tftypes.Bool, nil),
		"allow_two_rands":                       tftypes.NewValue(tftypes.Bool, nil),
		"trusted_certificates":                  tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
		"allow_trust_exceptions":                tftypes.NewValue(tftypes.Bool, nil),
		"proxy_type":                            tftypes.NewValue(tftypes.String, nil),
		"proxy_server":                          tftypes.NewValue(tftypes.String, nil),
		"proxy_server_port":                     tftypes.NewValue(tftypes.Number, nil),
		"proxy_username":                        tftypes.NewValue(tftypes.String, nil),
		"proxy_password":                        tftypes.NewValue(tftypes.String, nil),
		"proxy_url":                             tftypes.NewValue(tftypes.String, nil),
		"pac_fallback":                          tftypes.NewValue(tftypes.Bool, nil),
	}

	vals := make([]tftypes.Value, len(items))
	for i, item := range items {
		merged := make(map[string]tftypes.Value, len(defaults))
		for k, v := range defaults {
			merged[k] = v
		}
		for k, v := range item {
			merged[k] = v
		}
		vals[i] = tftypes.NewValue(itemType, merged)
	}
	return tftypes.NewValue(listType, vals)
}

// credentialsListItemType returns the tftypes type for a single credentials_list item.
func credentialsListItemType() tftypes.Object {
	return tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"credential_source":                tftypes.String,
			"credential_name":                  tftypes.String,
			"certificate_payload":              tftypes.String,
			"certificate_password":             tftypes.String,
			"certificate_id":                   tftypes.Number,
			"certificate_authority":            tftypes.Number,
			"certificate_template":             tftypes.Number,
			"allow_access_to_all_applications": tftypes.Bool,
			"key_is_extractable":               tftypes.Bool,
		},
	}
}

// nullCredentialsList returns a null tftypes.Value for credentials_list.
func nullCredentialsList() tftypes.Value {
	return tftypes.NewValue(tftypes.List{ElementType: credentialsListItemType()}, nil)
}

// diskEncryptionAirWatchType returns the tftypes.Object for disk_encryption.airwatch.
func diskEncryptionAirWatchType() tftypes.Object {
	return tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"store_key":                                         tftypes.Bool,
			"rotate_key_after":                                  tftypes.Number,
			"use_intelligent_hub":                               tftypes.Bool,
			"notify_user_for_encryption":                        tftypes.Bool,
			"encryption_notification_title":                     tftypes.String,
			"encryption_notification_message":                   tftypes.String,
			"encryption_max_notify_attempts":                    tftypes.Number,
			"encryption_notification_retry_interval_in_hours":   tftypes.Number,
			"encryption_action_after_last_notification":         tftypes.Number,
			"enable_recovery_key":                               tftypes.Bool,
			"recovery_key_notification_title":                   tftypes.String,
			"recovery_key_notification_message":                 tftypes.String,
			"recovery_key_notification_retry_interval_in_hours": tftypes.Number,
			"recovery_key_prompt_title":                         tftypes.String,
			"recovery_key_prompt_message":                       tftypes.String,
			"recovery_key_success_title":                        tftypes.String,
			"recovery_key_success_message":                      tftypes.String,
			"recovery_key_error_title":                          tftypes.String,
			"recovery_key_error_message":                        tftypes.String,
			"recovery_key_max_failure_count":                    tftypes.Number,
		},
	}
}

// diskEncryptionFileVaultType returns the tftypes.Object for disk_encryption.filevault2.
func diskEncryptionFileVaultType() tftypes.Object {
	return tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"enable":                           tftypes.Bool,
			"show_recovery_key":                tftypes.Bool,
			"recovery_type":                    tftypes.Number,
			"filevault_enterprise_certificate": tftypes.String,
			"filevault_user":                   tftypes.Number,
			"username":                         tftypes.String,
			"prompt_to_enable_filevault_at":    tftypes.Number,
			"number_of_times_user_can_bypass":  tftypes.Number,
		},
	}
}

// diskEncryptionMCXType returns the tftypes.Object for disk_encryption.mcx.
func diskEncryptionMCXType() tftypes.Object {
	return tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"destroy_fv_key_on_standby": tftypes.Bool,
		},
	}
}

// diskEncryptionObjectType returns the tftypes.Object for the disk_encryption attribute.
func diskEncryptionObjectType() tftypes.Object {
	return tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"airwatch":   diskEncryptionAirWatchType(),
			"filevault2": diskEncryptionFileVaultType(),
			"mcx":        diskEncryptionMCXType(),
		},
	}
}

// nullDiskEncryption returns a null tftypes.Value for the disk_encryption attribute.
func nullDiskEncryption() tftypes.Value {
	return tftypes.NewValue(diskEncryptionObjectType(), nil)
}

// diskEncryptionVal builds a tftypes.Value for disk_encryption from sub-object values.
func diskEncryptionVal(vals map[string]tftypes.Value) tftypes.Value {
	objType := diskEncryptionObjectType()
	defaults := map[string]tftypes.Value{
		"airwatch":   tftypes.NewValue(diskEncryptionAirWatchType(), nil),
		"filevault2": tftypes.NewValue(diskEncryptionFileVaultType(), nil),
		"mcx":        tftypes.NewValue(diskEncryptionMCXType(), nil),
	}
	for k, v := range vals {
		defaults[k] = v
	}
	return tftypes.NewValue(objType, defaults)
}

// airwatchVal builds a tftypes.Value for the airwatch sub-object.
func airwatchVal(vals map[string]tftypes.Value) tftypes.Value {
	objType := diskEncryptionAirWatchType()
	defaults := map[string]tftypes.Value{
		"store_key":                                         tftypes.NewValue(tftypes.Bool, nil),
		"rotate_key_after":                                  tftypes.NewValue(tftypes.Number, nil),
		"use_intelligent_hub":                               tftypes.NewValue(tftypes.Bool, nil),
		"notify_user_for_encryption":                        tftypes.NewValue(tftypes.Bool, nil),
		"encryption_notification_title":                     tftypes.NewValue(tftypes.String, nil),
		"encryption_notification_message":                   tftypes.NewValue(tftypes.String, nil),
		"encryption_max_notify_attempts":                    tftypes.NewValue(tftypes.Number, nil),
		"encryption_notification_retry_interval_in_hours":   tftypes.NewValue(tftypes.Number, nil),
		"encryption_action_after_last_notification":         tftypes.NewValue(tftypes.Number, nil),
		"enable_recovery_key":                               tftypes.NewValue(tftypes.Bool, nil),
		"recovery_key_notification_title":                   tftypes.NewValue(tftypes.String, nil),
		"recovery_key_notification_message":                 tftypes.NewValue(tftypes.String, nil),
		"recovery_key_notification_retry_interval_in_hours": tftypes.NewValue(tftypes.Number, nil),
		"recovery_key_prompt_title":                         tftypes.NewValue(tftypes.String, nil),
		"recovery_key_prompt_message":                       tftypes.NewValue(tftypes.String, nil),
		"recovery_key_success_title":                        tftypes.NewValue(tftypes.String, nil),
		"recovery_key_success_message":                      tftypes.NewValue(tftypes.String, nil),
		"recovery_key_error_title":                          tftypes.NewValue(tftypes.String, nil),
		"recovery_key_error_message":                        tftypes.NewValue(tftypes.String, nil),
		"recovery_key_max_failure_count":                    tftypes.NewValue(tftypes.Number, nil),
	}
	for k, v := range vals {
		defaults[k] = v
	}
	return tftypes.NewValue(objType, defaults)
}

// filevaultVal builds a tftypes.Value for the filevault2 sub-object.
func filevaultVal(vals map[string]tftypes.Value) tftypes.Value {
	objType := diskEncryptionFileVaultType()
	defaults := map[string]tftypes.Value{
		"enable":                           tftypes.NewValue(tftypes.Bool, nil),
		"show_recovery_key":                tftypes.NewValue(tftypes.Bool, nil),
		"recovery_type":                    tftypes.NewValue(tftypes.Number, nil),
		"filevault_enterprise_certificate": tftypes.NewValue(tftypes.String, nil),
		"filevault_user":                   tftypes.NewValue(tftypes.Number, nil),
		"username":                         tftypes.NewValue(tftypes.String, nil),
		"prompt_to_enable_filevault_at":    tftypes.NewValue(tftypes.Number, nil),
		"number_of_times_user_can_bypass":  tftypes.NewValue(tftypes.Number, nil),
	}
	for k, v := range vals {
		defaults[k] = v
	}
	return tftypes.NewValue(objType, defaults)
}

// mcxVal builds a tftypes.Value for the mcx sub-object.
func mcxVal(vals map[string]tftypes.Value) tftypes.Value {
	objType := diskEncryptionMCXType()
	defaults := map[string]tftypes.Value{
		"destroy_fv_key_on_standby": tftypes.NewValue(tftypes.Bool, nil),
	}
	for k, v := range vals {
		defaults[k] = v
	}
	return tftypes.NewValue(objType, defaults)
}

// --- Gatekeeper (Security & Privacy, macOS) test helpers ---

func gatekeeperObjectType() tftypes.Object {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"allow_auto_unlock":                tftypes.Bool,
		"allow_fingerprint_for_unlock":     tftypes.Bool,
		"allow_handoff":                    tftypes.Bool,
		"allow_screen_capture":             tftypes.Bool,
		"enable_app_software_update_delay": tftypes.Bool,
		"enable_software_update_delay":     tftypes.Bool,
		"enforced_software_update_delay":   tftypes.Number,
	}}
}

func nullGatekeeper() tftypes.Value {
	return tftypes.NewValue(gatekeeperObjectType(), nil)
}

// --- Restrictions (macOS) test helpers ---

func restrictionsAppStoreType() tftypes.Object {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"allow_app_store_app_adoption":                    tftypes.Bool,
		"require_admin_password_to_install_or_update_app": tftypes.Bool,
		"restrict_app_store_to_software_updates_only":     tftypes.Bool,
	}}
}

func restrictionsAppleMusicType() tftypes.Object {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{"allow_music_service": tftypes.Bool}}
}

func restrictionsCameraType() tftypes.Object {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{"allow_use_of_built_in_camera": tftypes.Bool}}
}

func restrictionsGameCentreType() tftypes.Object {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"allow_adding_game_center_friends": tftypes.Bool,
		"allow_game_center_modification":   tftypes.Bool,
		"allow_multiplayer_gaming":         tftypes.Bool,
		"allow_use_of_game_center":         tftypes.Bool,
	}}
}

func restrictionsSafariType() tftypes.Object {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"allow_deprecated_web_kit_tls": tftypes.Bool,
		"allow_safari_auto_fill":       tftypes.Bool,
	}}
}

func restrictionsApplicationsType() tftypes.Object {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"allow_application": tftypes.List{ElementType: tftypes.String},
		"allow_folders":     tftypes.List{ElementType: tftypes.String},
		"disallow_folders":  tftypes.List{ElementType: tftypes.String},
		"restrict_which_applications_are_allowed_to_launch": tftypes.Bool,
		"app_store":   restrictionsAppStoreType(),
		"apple_music": restrictionsAppleMusicType(),
		"camera":      restrictionsCameraType(),
		"game_centre": restrictionsGameCentreType(),
		"safari":      restrictionsSafariType(),
	}}
}

func restrictionsDesktopType() tftypes.Object {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"desktop_picture_path": tftypes.String,
		"lock_desktop_picture": tftypes.Bool,
	}}
}

func restrictionsAirPrintType() tftypes.Object {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"allow_air_print":                         tftypes.Bool,
		"allow_air_print_ibeacon_discovery":       tftypes.Bool,
		"force_air_print_trusted_tls_requirement": tftypes.Bool,
	}}
}

func restrictionsContentCachingType() tftypes.Object {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{"allow_content_caching": tftypes.Bool}}
}

func restrictionsICloudType() tftypes.Object {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"allow_air_print":                              tftypes.Bool,
		"allow_air_print_ibeacon_discovery":            tftypes.Bool,
		"allow_cloud_desktop_and_documents":            tftypes.Bool,
		"allow_deprecated_web_kit_tls":                 tftypes.Bool,
		"allow_icloud_fmm":                             tftypes.Bool,
		"allow_icloud_address_book":                    tftypes.Bool,
		"allow_icloud_btmm":                            tftypes.Bool,
		"allow_icloud_bookmarks":                       tftypes.Bool,
		"allow_icloud_calendar":                        tftypes.Bool,
		"allow_icloud_documents_and_data":              tftypes.Bool,
		"allow_icloud_keychain_sync":                   tftypes.Bool,
		"allow_icloud_mail":                            tftypes.Bool,
		"allow_icloud_notes":                           tftypes.Bool,
		"allow_icloud_reminders":                       tftypes.Bool,
		"allow_password_auto_fill":                     tftypes.Bool,
		"allow_password_proximity_requests":            tftypes.Bool,
		"allow_password_sharing":                       tftypes.Bool,
		"allow_use_icloud_password_for_local_accounts": tftypes.Bool,
		"force_air_print_trusted_tls_requirement":      tftypes.Bool,
	}}
}

func restrictionsPasswordsType() tftypes.Object {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"allow_password_auto_fill":          tftypes.Bool,
		"allow_password_proximity_requests": tftypes.Bool,
		"allow_password_sharing":            tftypes.Bool,
	}}
}

func restrictionsSpotlightType() tftypes.Object {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{"allow_spotlight_suggestions": tftypes.Bool}}
}

func restrictionsFunctionalityType() tftypes.Object {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"air_print":       restrictionsAirPrintType(),
		"content_caching": restrictionsContentCachingType(),
		"icloud":          restrictionsICloudType(),
		"passwords":       restrictionsPasswordsType(),
		"spotlight":       restrictionsSpotlightType(),
	}}
}

func restrictionsMediaAccessType() tftypes.Object {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"allow":        tftypes.Bool,
		"authenticate": tftypes.Bool,
		"read_only":    tftypes.Bool,
	}}
}

func restrictionsNetworkAccessType() tftypes.Object {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{"air_drop": tftypes.Bool}}
}

func restrictionsBurnSupportType() tftypes.Object {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{"burn_support": restrictionsMediaAccessType()}}
}

func restrictionsMediaType() tftypes.Object {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"auto_eject_media":                tftypes.Bool,
		"disk_media_cds":                  restrictionsMediaAccessType(),
		"disk_media_dvds":                 restrictionsMediaAccessType(),
		"external_hard_disk_media_access": restrictionsMediaAccessType(),
		"hard_disk_dvd_ram":               restrictionsMediaAccessType(),
		"hard_disk_images":                restrictionsMediaAccessType(),
		"internal_hard_disk_media_access": restrictionsMediaAccessType(),
		"network_access":                  restrictionsNetworkAccessType(),
		"recordable_disc":                 restrictionsBurnSupportType(),
	}}
}

func restrictionsPreferencesType() tftypes.Object {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"accessibility":            tftypes.Bool,
		"app_store":                tftypes.Bool,
		"bluetooth":                tftypes.Bool,
		"cds_and_dvds":             tftypes.Bool,
		"date_and_time":            tftypes.Bool,
		"desktop_and_screen_saver": tftypes.Bool,
		"dictation_and_speech":     tftypes.Bool,
		"displays":                 tftypes.Bool,
		"dock":                     tftypes.Bool,
		"enabled_preference_panes": tftypes.Bool,
		"energy_saver":             tftypes.Bool,
		"extensions":               tftypes.Bool,
		"fibre_channel":            tftypes.Bool,
		"flash_player":             tftypes.Bool,
		"general":                  tftypes.Bool,
		"ink":                      tftypes.Bool,
		"internet_accounts":        tftypes.Bool,
		"keyboard":                 tftypes.Bool,
		"language_and_text":        tftypes.Bool,
		"mission_control":          tftypes.Bool,
		"mobile_me":                tftypes.Bool,
		"mouse":                    tftypes.Bool,
		"network":                  tftypes.Bool,
		"notifications":            tftypes.Bool,
		"parental_controls":        tftypes.Bool,
		"preference_behavior":      tftypes.String,
		"print_and_scan":           tftypes.Bool,
		"profiles":                 tftypes.Bool,
		"security_and_privacy":     tftypes.Bool,
		"sharing":                  tftypes.Bool,
		"software_update":          tftypes.Bool,
		"sound":                    tftypes.Bool,
		"spotlight":                tftypes.Bool,
		"startup_disk":             tftypes.Bool,
		"time_machine":             tftypes.Bool,
		"trackpad":                 tftypes.Bool,
		"users_and_groups":         tftypes.Bool,
		"xsan":                     tftypes.Bool,
		"icloud":                   tftypes.Bool,
	}}
}

func restrictionsSharingType() tftypes.Object {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"add_to_aperture":     tftypes.Bool,
		"add_to_reading_list": tftypes.Bool,
		"add_to_iphoto":       tftypes.Bool,
		"air_drop":            tftypes.Bool,
		"automatically_enable_new_sharing_services": tftypes.Bool,
		"facebook": tftypes.Bool,
		"mail":     tftypes.Bool,
		"messages": tftypes.Bool,
		"restrict_which_sharing_services_are_enabled": tftypes.Bool,
		"sina_weibo":     tftypes.Bool,
		"twitter":        tftypes.Bool,
		"video_services": tftypes.Bool,
	}}
}

func restrictionsWidgetsType() tftypes.Object {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"allow_only_configured_widgets": tftypes.Bool,
		"allowed_widgets":               tftypes.List{ElementType: tftypes.String},
	}}
}

func restrictionsObjectType() tftypes.Object {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"applications":  restrictionsApplicationsType(),
		"desktop":       restrictionsDesktopType(),
		"functionality": restrictionsFunctionalityType(),
		"media":         restrictionsMediaType(),
		"preferences":   restrictionsPreferencesType(),
		"sharing":       restrictionsSharingType(),
		"widgets":       restrictionsWidgetsType(),
	}}
}

func nullRestrictions() tftypes.Value {
	return tftypes.NewValue(restrictionsObjectType(), nil)
}

// restrictionsVal builds a tftypes.Value for restrictions from sub-object values.
// Sub-blocks not provided default to null.
func restrictionsVal(vals map[string]tftypes.Value) tftypes.Value {
	defaults := map[string]tftypes.Value{
		"applications":  tftypes.NewValue(restrictionsApplicationsType(), nil),
		"desktop":       tftypes.NewValue(restrictionsDesktopType(), nil),
		"functionality": tftypes.NewValue(restrictionsFunctionalityType(), nil),
		"media":         tftypes.NewValue(restrictionsMediaType(), nil),
		"preferences":   tftypes.NewValue(restrictionsPreferencesType(), nil),
		"sharing":       tftypes.NewValue(restrictionsSharingType(), nil),
		"widgets":       tftypes.NewValue(restrictionsWidgetsType(), nil),
	}
	for k, v := range vals {
		defaults[k] = v
	}
	return tftypes.NewValue(restrictionsObjectType(), defaults)
}

// restrictionsApplicationsVal builds an applications sub-object with defaults.
func restrictionsApplicationsVal(vals map[string]tftypes.Value) tftypes.Value {
	defaults := map[string]tftypes.Value{
		"allow_application": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
		"allow_folders":     tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
		"disallow_folders":  tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
		"restrict_which_applications_are_allowed_to_launch": tftypes.NewValue(tftypes.Bool, nil),
		"app_store":   tftypes.NewValue(restrictionsAppStoreType(), nil),
		"apple_music": tftypes.NewValue(restrictionsAppleMusicType(), nil),
		"camera":      tftypes.NewValue(restrictionsCameraType(), nil),
		"game_centre": tftypes.NewValue(restrictionsGameCentreType(), nil),
		"safari":      tftypes.NewValue(restrictionsSafariType(), nil),
	}
	for k, v := range vals {
		defaults[k] = v
	}
	return tftypes.NewValue(restrictionsApplicationsType(), defaults)
}

// restrictionsCameraVal builds a camera sub-object with defaults.
func restrictionsCameraVal(vals map[string]tftypes.Value) tftypes.Value {
	defaults := map[string]tftypes.Value{"allow_use_of_built_in_camera": tftypes.NewValue(tftypes.Bool, nil)}
	for k, v := range vals {
		defaults[k] = v
	}
	return tftypes.NewValue(restrictionsCameraType(), defaults)
}

// restrictionsWidgetsVal builds a widgets sub-object with defaults.
func restrictionsWidgetsVal(vals map[string]tftypes.Value) tftypes.Value {
	defaults := map[string]tftypes.Value{
		"allow_only_configured_widgets": tftypes.NewValue(tftypes.Bool, nil),
		"allowed_widgets":               tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
	}
	for k, v := range vals {
		defaults[k] = v
	}
	return tftypes.NewValue(restrictionsWidgetsType(), defaults)
}

// restrictionsPreferencesVal builds a preferences sub-object with defaults.
func restrictionsPreferencesVal(vals map[string]tftypes.Value) tftypes.Value {
	defaults := map[string]tftypes.Value{}
	for name, t := range restrictionsPreferencesType().AttributeTypes {
		defaults[name] = tftypes.NewValue(t, nil)
	}
	for k, v := range vals {
		defaults[k] = v
	}
	return tftypes.NewValue(restrictionsPreferencesType(), defaults)
}
