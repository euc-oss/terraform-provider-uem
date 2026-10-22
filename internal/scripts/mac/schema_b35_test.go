package macscript

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// B35: catalog_display.categories must be Optional, not Required. UEM
// returns "categories": [] for a real, in-catalog script with no categories
// assigned (live-confirmed); Required would make that script's state
// permanently unplannable (a Required attribute holding null is a
// provider-schema-consistency error), and nothing in the cited canonical Q24
// source (CatalogDisplay.IsValid()) requires categories at all.
//
// Revert check: restoring Required: true on categories (and dropping
// Optional: true) makes this test fail.
func TestCatalogDisplayCategoriesSchema_Optional(t *testing.T) {
	resp := resourceSchema(t)
	cd, ok := resp.Schema.Attributes["catalog_display"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("catalog_display is not a SingleNestedAttribute: %T", resp.Schema.Attributes["catalog_display"])
	}
	categories, ok := cd.Attributes["categories"]
	if !ok {
		t.Fatal("catalog_display has no categories attribute")
	}
	if categories.IsRequired() {
		t.Error("catalog_display.categories must not be Required")
	}
	if !categories.IsOptional() {
		t.Error("catalog_display.categories must be Optional")
	}
	// An omitted categories must take UEM's readback ([] or values), not plan
	// null: Computed avoids an inconsistent result after apply.
	if !categories.IsComputed() {
		t.Error("catalog_display.categories must be Computed")
	}
}
