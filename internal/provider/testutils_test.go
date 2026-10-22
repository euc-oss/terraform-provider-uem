package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// getProviderSchema returns the provider schema for testing.
func getProviderSchema(t *testing.T) provider.SchemaResponse {
	t.Helper()
	p := &WorkspaceOneProvider{version: "test"}
	var resp provider.SchemaResponse
	p.Schema(context.Background(), provider.SchemaRequest{}, &resp)
	return resp
}

// createProviderConfig creates a tfsdk.Config for provider testing.
func createProviderConfig(t *testing.T, values map[string]tftypes.Value) tfsdk.Config {
	t.Helper()
	schemaResp := getProviderSchema(t)
	ctx := context.Background()
	configType := schemaResp.Schema.Type().TerraformType(ctx)
	configValue := tftypes.NewValue(configType, values)
	return tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw:    configValue,
	}
}

func nullString() tftypes.Value {
	return tftypes.NewValue(tftypes.String, nil)
}

func stringVal(s string) tftypes.Value {
	return tftypes.NewValue(tftypes.String, s)
}

func allNullProviderConfig() map[string]tftypes.Value {
	return map[string]tftypes.Value{
		"instance_url":            nullString(),
		"tenant_code":             nullString(),
		"auth_method":             nullString(),
		"username":                nullString(),
		"password":                nullString(),
		"client_id":               nullString(),
		"client_secret":           nullString(),
		"oauth2_token_url":        nullString(),
		"app_binary_storage_path": nullString(),
	}
}
