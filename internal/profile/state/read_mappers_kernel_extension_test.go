package state

import (
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// B48 (live, as<internal-env>): creating a uem_profile with kernel_extension set and
// allowed_team_identifiers omitted echoes back AllowedTeamIdentifiers: [""]
// (a single degenerate empty-string entry, not an empty/absent list). The
// mapper, mapKernelExtension, must map that value verbatim -- [""] stays
// [""], never collapsed to null or to an empty list -- so the schema fix
// (allowed_team_identifiers now carries only listplanmodifier.UseStateForUnknown,
// with the create-time null-conversion useStateForNullList removed) has a
// faithful value to land in state.
func TestMapKernelExtension_AllowedTeamIdentifiersEmptyStringEchoesVerbatim(t *testing.T) {
	t.Parallel()

	got := mapKernelExtension(&sdk.MacOsKernelExtensionPayloadV2Entity{
		AllowedTeamIdentifiers: []string{""},
	})

	if got.AllowedTeamIdentifiers.IsNull() || got.AllowedTeamIdentifiers.IsUnknown() {
		t.Fatalf("expected a known, non-null AllowedTeamIdentifiers, got %v", got.AllowedTeamIdentifiers)
	}
	elems := got.AllowedTeamIdentifiers.Elements()
	if len(elems) != 1 {
		t.Fatalf("expected AllowedTeamIdentifiers = [\"\"], got %v", elems)
	}
	s, ok := elems[0].(types.String)
	if !ok || s.ValueString() != "" {
		t.Errorf("expected AllowedTeamIdentifiers[0] = \"\" (UEM's echoed value), got %v", elems[0])
	}
}
