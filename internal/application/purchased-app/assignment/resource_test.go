package assignment

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func TestPurchasedAssignmentMetadata(t *testing.T) {
	r := &purchasedApplicationAssignmentResource{}
	var resp resource.MetadataResponse
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "uem"}, &resp)
	if resp.TypeName != "uem_purchased_application_assignment" {
		t.Fatalf("unexpected type name: %q", resp.TypeName)
	}
}

func TestPurchasedAssignmentSchemaContainsCoreAttributes(t *testing.T) {
	r := &purchasedApplicationAssignmentResource{}
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)

	attrs := resp.Schema.Attributes
	for _, key := range []string{"id", "application_uuid", "excluded_smart_groups", "assignments"} {
		if _, ok := attrs[key]; !ok {
			t.Fatalf("missing schema attribute %q", key)
		}
	}
}

func TestPurchasedAssignmentServiceNilClient(t *testing.T) {
	r := &purchasedApplicationAssignmentResource{}
	_, err := r.purchasedAppAssignmentService(context.Background())
	if err == nil {
		t.Fatal("expected nil client error")
	}
	if !strings.Contains(err.Error(), "SDK client is nil") {
		t.Fatalf("unexpected error: %v", err)
	}
}
