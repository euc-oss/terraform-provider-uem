package datasource

import (
	"context"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// These two tests are fail-on-revert coverage for the optional organization_group_uuid
// filter added to macApplicationSearch.List (mac_applications_data_source_hooks.go). They
// drive the companion against the SDK-level appsV2SearchAPI fake (fakeAppsV2Search,
// declared in data_source_test.go), capturing the *sdk.AppsV2SearchOptions the companion
// builds -- the same capture pattern used by
// TestMacApplicationSearch_List_AppliesNoAppFamilyFilter in
// mac_applications_data_source_gen_test.go.

// TestMacApplicationSearch_List_SetOrgGroupUuidReachesOpts proves that when the caller
// sets organization_group_uuid, the companion propagates it onto
// AppsV2SearchOptions.OrganizationGroupUUID.
func TestMacApplicationSearch_List_SetOrgGroupUuidReachesOpts(t *testing.T) {
	t.Parallel()

	fake := &fakeAppsV2Search{result: &sdk.ApplicationSearchV2Model{Total: intPtr(0)}}
	s := &macApplicationSearch{svc: fake}

	const wantUUID = "16fea16d-024d-1e8b-e41d-b5981759f00d"
	_, err := s.List(context.Background(), macApplicationFilters{
		OrganizationGroupUuid: types.StringValue(wantUUID),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fake.lastOpts == nil {
		t.Fatal("expected Search to be called with options")
	}
	if fake.lastOpts.OrganizationGroupUUID == nil {
		t.Fatal("expected OrganizationGroupUUID to be set on opts, got nil")
	}
	if *fake.lastOpts.OrganizationGroupUUID != wantUUID {
		t.Errorf("expected OrganizationGroupUUID %q, got %q", wantUUID, *fake.lastOpts.OrganizationGroupUUID)
	}
}

// TestMacApplicationSearch_List_UnsetOrgGroupUuidLeavesOptsEmpty proves that when the
// caller leaves organization_group_uuid null (the default/existing behavior for every
// caller that predates this filter), opts.OrganizationGroupUUID stays nil -- i.e. zero
// behavior change for existing callers.
func TestMacApplicationSearch_List_UnsetOrgGroupUuidLeavesOptsEmpty(t *testing.T) {
	t.Parallel()

	fake := &fakeAppsV2Search{result: &sdk.ApplicationSearchV2Model{Total: intPtr(0)}}
	s := &macApplicationSearch{svc: fake}

	_, err := s.List(context.Background(), macApplicationFilters{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fake.lastOpts == nil {
		t.Fatal("expected Search to be called with options")
	}
	if fake.lastOpts.OrganizationGroupUUID != nil {
		t.Errorf("expected OrganizationGroupUUID to remain nil, got %v", *fake.lastOpts.OrganizationGroupUUID)
	}
}

// TestMacApplicationSearch_List_EmptyStringOrgGroupUuidReachesOpts proves that a KNOWN,
// non-null value of "" -- e.g. from a Terraform variable defaulting to an empty string --
// is sent through to AppsV2SearchOptions.OrganizationGroupUUID exactly as configured
// (B16 (b)-row #111 removal). The provider does not distinguish a set empty string from
// unset: it sends what the caller set, faithfully, and lets UEM decide what an
// empty-string scope means.
func TestMacApplicationSearch_List_EmptyStringOrgGroupUuidReachesOpts(t *testing.T) {
	t.Parallel()

	fake := &fakeAppsV2Search{result: &sdk.ApplicationSearchV2Model{Total: intPtr(0)}}
	s := &macApplicationSearch{svc: fake}

	_, err := s.List(context.Background(), macApplicationFilters{
		OrganizationGroupUuid: types.StringValue(""),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fake.lastOpts == nil {
		t.Fatal("expected Search to be called with options")
	}
	if fake.lastOpts.OrganizationGroupUUID == nil {
		t.Fatal("expected OrganizationGroupUUID to be set to a pointer to \"\", got nil")
	}
	if *fake.lastOpts.OrganizationGroupUUID != "" {
		t.Errorf("expected OrganizationGroupUUID to be \"\", got %q", *fake.lastOpts.OrganizationGroupUUID)
	}
}
