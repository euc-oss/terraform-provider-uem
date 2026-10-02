package assignment

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
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

// TestApplicationUUIDRequiresReplace proves that changing application_uuid
// in place would silently change the resource id (see
// internal/application/assignment/state/state.go SetMinimalState, which sets
// data.ID = data.ApplicationUUID). Without a RequiresReplace plan modifier on
// application_uuid, Terraform plans an in-place update whose id then changes
// during apply, which the framework rejects as an inconsistent provider
// result. This test asserts application_uuid carries a RequiresReplace-typed
// plan modifier.
func TestApplicationUUIDRequiresReplace(t *testing.T) {
	r := &applicationAssignmentResource{}
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)

	attr, ok := resp.Schema.Attributes["application_uuid"]
	if !ok {
		t.Fatal("missing schema attribute \"application_uuid\"")
	}

	strAttr, ok := attr.(schema.StringAttribute)
	if !ok {
		t.Fatalf("application_uuid is not a schema.StringAttribute: %T", attr)
	}

	found := false
	for _, modifier := range strAttr.PlanModifiers {
		if strings.Contains(fmt.Sprintf("%T", modifier), "requiresReplaceIfModifier") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("application_uuid PlanModifiers %v does not contain a RequiresReplace modifier", strAttr.PlanModifiers)
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
