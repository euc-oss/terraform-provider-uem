package state

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// UEM stores "" for an unset profile scope, so the held General defaults
// must no longer turn a null or unknown scope into "Production".
func TestEnsureComputedDefaults_DoesNotDefaultProfileScope(t *testing.T) {
	for name, in := range map[string]types.String{
		"null":    types.StringNull(),
		"unknown": types.StringUnknown(),
	} {
		data := &ProfileResourceModel{ProfileScope: in}
		EnsureComputedDefaults(data)
		if data.ProfileScope.ValueString() == "Production" {
			t.Errorf("%s profile_scope was defaulted to Production", name)
		}
	}
}
