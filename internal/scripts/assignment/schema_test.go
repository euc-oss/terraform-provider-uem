package assignment

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// TestSchema_OrganizationGroupUUIDComputedOnly pins organization_group_uuid
// as read-only, which also keeps it out of ws1-tf onboard's generated HCL.
func TestSchema_OrganizationGroupUUIDComputedOnly(t *testing.T) {
	a, ok := testSchemaResponse(t).Schema.Attributes["organization_group_uuid"].(schema.StringAttribute)
	if !ok {
		t.Fatal("organization_group_uuid missing or not a string attribute")
	}
	if !a.Computed || a.Optional || a.Required {
		t.Fatalf("want Computed-only, got computed=%v optional=%v required=%v", a.Computed, a.Optional, a.Required)
	}
	if len(a.PlanModifiers) != 1 {
		t.Fatalf("want exactly UseStateForUnknown, got %d plan modifiers", len(a.PlanModifiers))
	}
}
