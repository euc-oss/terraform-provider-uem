package assignment

import (
	"context"
	"errors"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	tf "github.com/euc-oss/terraform-provider-uem/internal/scripts/assignment/models"
)

func TestRead_PopulatesOrgGroupFromParentScript(t *testing.T) {
	s := &fakeScriptService{script: &sdk.ScriptResourceV1{OrganizationGroupUUID: testOrgGroupUUID}}
	resp := runRead(t, newTestResource(&fakeAssignmentService{getResult: &sdk.ScriptAssignmentsSearchResultV1{}}, s), "")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if got := stateOrgGroup(t, resp.State); got == nil || *got != testOrgGroupUUID {
		t.Fatalf("expected organization_group_uuid %q, got %v", testOrgGroupUUID, got)
	}
	if s.getCalls != 1 {
		t.Fatalf("expected 1 script GET, got %d", s.getCalls)
	}
}

func TestRead_NilBody_PopulatesOrgGroup(t *testing.T) {
	s := &fakeScriptService{script: &sdk.ScriptResourceV1{OrganizationGroupUUID: testOrgGroupUUID}}
	resp := runRead(t, newTestResource(&fakeAssignmentService{}, s), "")
	if got := stateOrgGroup(t, resp.State); got == nil || *got != testOrgGroupUUID {
		t.Fatalf("expected organization_group_uuid %q, got %v", testOrgGroupUUID, got)
	}
}

func TestRead_ScriptLookupFails_KeepsPriorOrgGroup(t *testing.T) {
	const prior = "51492f1d-2d0e-edff-e525-ec254753e1c5"
	cases := map[string]*fakeScriptService{
		"error":    {scriptErr: errors.New("boom")},
		"empty og": {script: &sdk.ScriptResourceV1{}},
	}
	for name, s := range cases {
		t.Run(name, func(t *testing.T) {
			resp := runRead(t, newTestResource(&fakeAssignmentService{getResult: &sdk.ScriptAssignmentsSearchResultV1{}}, s), prior)
			if resp.Diagnostics.HasError() {
				t.Fatalf("lookup failure must not fail Read: %v", resp.Diagnostics)
			}
			if got := stateOrgGroup(t, resp.State); got == nil || *got != prior {
				t.Fatalf("expected prior organization_group_uuid %q kept, got %v", prior, got)
			}
		})
	}
}

func TestRead_ScriptLookupFails_NoPriorStaysNull(t *testing.T) {
	s := &fakeScriptService{scriptErr: errors.New("boom")}
	resp := runRead(t, newTestResource(&fakeAssignmentService{getResult: &sdk.ScriptAssignmentsSearchResultV1{}}, s), "")
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if got := stateOrgGroup(t, resp.State); got != nil {
		t.Fatalf("expected null organization_group_uuid, got %q", *got)
	}
}

func TestConfigure_ResourceConfigData_WiresScriptServiceFactory(t *testing.T) {
	r := &scriptAssignmentResource{}
	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: &resourceConfigData{client: &sdk.Client{}}}, resp)
	if resp.Diagnostics.HasError() || r.newScriptService == nil {
		t.Fatalf("expected the default script service factory to be set: %v", resp.Diagnostics)
	}
}

// TestRefreshIntoState_UnknownOrgGroupBecomesKnown covers Create/Update: the
// planned value is unknown, and state after apply must hold a known value
// whether or not the parent script lookup succeeds.
func TestRefreshIntoState_UnknownOrgGroupBecomesKnown(t *testing.T) {
	cases := map[string]struct {
		s    *fakeScriptService
		want *string
	}{
		"lookup ok":     {s: &fakeScriptService{script: &sdk.ScriptResourceV1{OrganizationGroupUUID: testOrgGroupUUID}}, want: ptr(testOrgGroupUUID)},
		"lookup failed": {s: &fakeScriptService{scriptErr: errors.New("boom")}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			r := newTestResource(&fakeAssignmentService{getResult: &sdk.ScriptAssignmentsSearchResultV1{}}, tc.s)
			state := seededState(t, "")
			data := tf.ScriptAssignmentRuleModel{
				ScriptUUID:            types.StringValue(testScriptUUID),
				OrganizationGroupUUID: types.StringUnknown(),
				Assignments:           types.ListNull(types.ObjectType{}),
			}
			var diags diag.Diagnostics
			svc, _ := r.scriptAssignmentService(ctx)
			r.refreshIntoState(ctx, svc, testScriptUUID, &data, &state, &diags)
			if data.OrganizationGroupUUID.IsUnknown() {
				t.Fatal("organization_group_uuid must not stay unknown after apply")
			}
			got := data.OrganizationGroupUUID.ValueStringPointer()
			if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
				t.Fatalf("organization_group_uuid = %v, want %v", got, tc.want)
			}
		})
	}
}

func ptr(s string) *string { return &s }
