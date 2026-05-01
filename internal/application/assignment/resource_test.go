package assignment

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func TestAssignmentMetadata(t *testing.T) {
	r := &applicationAssignmentResource{}
	var resp resource.MetadataResponse
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "uem"}, &resp)

	if resp.TypeName != "uem_application_assignment" {
		t.Fatalf("unexpected type name: %q", resp.TypeName)
	}
}

func TestAssignmentSchemaContainsCoreAttributes(t *testing.T) {
	r := &applicationAssignmentResource{}
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)

	attrs := resp.Schema.Attributes
	for _, key := range []string{"id", "application_uuid", "excluded_smart_groups", "assignments"} {
		if _, ok := attrs[key]; !ok {
			t.Fatalf("missing schema attribute %q", key)
		}
	}
}

func TestAssignmentServiceNilClient(t *testing.T) {
	r := &applicationAssignmentResource{}
	_, err := r.appAssignmentService(context.Background())
	if err == nil {
		t.Fatal("expected nil client error")
	}
	if !strings.Contains(err.Error(), "SDK client is nil") {
		t.Fatalf("unexpected error: %v", err)
	}
}
