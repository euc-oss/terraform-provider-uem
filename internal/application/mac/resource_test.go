package macapplication

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func TestMacMetadata(t *testing.T) {
	r := &macapplicationResource{}
	var resp resource.MetadataResponse
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "uem"}, &resp)

	if resp.TypeName != "uem_mac_application" {
		t.Fatalf("unexpected type name: %q", resp.TypeName)
	}
}

func TestMacSchemaContainsCoreAttributes(t *testing.T) {
	r := &macapplicationResource{}
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)

	attrs := resp.Schema.Attributes
	for _, key := range []string{"id", "uuid", "org_group_id", "dmg_file_path", "plist_file_path", "app_version"} {
		if _, ok := attrs[key]; !ok {
			t.Fatalf("missing schema attribute %q", key)
		}
	}
}

func TestMacServicesNilClient(t *testing.T) {
	r := &macapplicationResource{}
	if _, err := r.BlobResourceService(context.Background()); err == nil || !strings.Contains(err.Error(), "SDK client is nil") {
		t.Fatalf("unexpected blob service error: %v", err)
	}
	if _, err := r.MacAppService(context.Background()); err == nil || !strings.Contains(err.Error(), "SDK client is nil") {
		t.Fatalf("unexpected mac app service error: %v", err)
	}
	if _, err := r.InternalAppService(context.Background()); err == nil || !strings.Contains(err.Error(), "SDK client is nil") {
		t.Fatalf("unexpected internal app service error: %v", err)
	}
}
