package smartgroup

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/euc-oss/terraform-provider-uem/internal/providerdata"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/euc-oss/terraform-sdk-uem/v26/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// --- schema/config helpers (mirroring internal/organizationgroup's style:
// every attribute defaults to null except explicit overrides, so adding a
// new top-level attribute never breaks every existing test's config
// builder). ---

func getDSSchema(t *testing.T) datasource.SchemaResponse {
	t.Helper()
	ds := &SmartGroupsDataSource{}
	var resp datasource.SchemaResponse
	ds.Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	return resp
}

// buildConfig constructs a tfsdk.Config for the data source's schema with
// every attribute defaulted to null except the overrides given.
func buildConfig(t *testing.T, s datasource.SchemaResponse, overrides map[string]tftypes.Value) tfsdk.Config {
	t.Helper()
	ctx := context.Background()
	objType, ok := s.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatal("schema type is not an Object")
	}
	values := map[string]tftypes.Value{}
	for name, at := range objType.AttributeTypes {
		values[name] = tftypes.NewValue(at, nil)
	}
	for name, v := range overrides {
		values[name] = v
	}
	return tfsdk.Config{Schema: s.Schema, Raw: tftypes.NewValue(objType, values)}
}

func emptyDSState(t *testing.T) tfsdk.State {
	t.Helper()
	s := getDSSchema(t)
	ctx := context.Background()
	objType, ok := s.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatal("schema type is not an Object")
	}
	values := map[string]tftypes.Value{}
	for name, at := range objType.AttributeTypes {
		values[name] = tftypes.NewValue(at, nil)
	}
	return tfsdk.State{Schema: s.Schema, Raw: tftypes.NewValue(objType, values)}
}

// --- fakes implementing the narrow injected interfaces ---

// fakeSmartGroupSearch answers SearchAsync from a fixed set of pages, keyed
// by the requested page number. It also supports simulating a server that
// ignores the page parameter (always answers page 0), a hard error, and --
// when sequence is set -- answering strictly in CALL order rather than by
// requested page number, so a test can make the two independent walks a
// double-walk performs see different responses (mirrors
// internal/organizationgroup's fakeSearch).
type fakeSmartGroupSearch struct {
	pages      map[int]*sdk.SmartGroupSearchResultV1
	sequence   []*sdk.SmartGroupSearchResultV1
	ignorePage bool
	err        error

	// nilForMissing, when true, answers a page with no map entry with a nil
	// result (nil, nil, nil) instead of an empty-but-non-nil struct -- the
	// live-confirmed HTTP-204 out-of-range shape on both as<internal-env> and
	// paul-2609 (GET /api/mdm/smartgroups/search), which the SDK surfaces as
	// a nil *SmartGroupSearchResultV1 with a nil error.
	nilForMissing bool

	pagesRequested []int
	lastOpts       *sdk.SmartGroupsSearchAsyncOptions
}

func (f *fakeSmartGroupSearch) SearchAsync(_ context.Context, opts *sdk.SmartGroupsSearchAsyncOptions) (http.Header, *sdk.SmartGroupSearchResultV1, error) {
	page := 0
	if opts != nil && opts.Page != nil {
		page = *opts.Page
	}
	call := len(f.pagesRequested)
	f.pagesRequested = append(f.pagesRequested, page)
	f.lastOpts = opts
	if f.err != nil {
		return nil, nil, f.err
	}
	if f.sequence != nil {
		if call >= len(f.sequence) {
			return nil, &sdk.SmartGroupSearchResultV1{}, nil
		}
		return nil, f.sequence[call], nil
	}
	if f.ignorePage {
		page = 0
	}
	res, ok := f.pages[page]
	if !ok {
		if f.nilForMissing {
			return nil, nil, nil
		}
		return nil, &sdk.SmartGroupSearchResultV1{}, nil
	}
	return nil, res, nil
}

// fakeSmartGroupLoader answers LoadSmartGroupAsync with a fixed group or
// error.
type fakeSmartGroupLoader struct {
	group  *sdk.SmartGroupV1
	err    error
	idSeen int
}

func (f *fakeSmartGroupLoader) LoadSmartGroupAsync(_ context.Context, id int) (http.Header, *sdk.SmartGroupV1, error) {
	f.idSeen = id
	if f.err != nil {
		return nil, nil, f.err
	}
	return nil, f.group, nil
}

func intPtr(v int) *int { return &v }

// sg builds a minimal SmartGroupSearchModelV1 fixture -- everything the
// paging/dedup/uuid-match logic needs.
func sg(id int64, name, uuid string) sdk.SmartGroupSearchModelV1 {
	i := int(id)
	return sdk.SmartGroupSearchModelV1{SmartGroupID: &i, Name: name, SmartGroupUUID: uuid}
}

func searchResult(items []sdk.SmartGroupSearchModelV1, page, pageSize int, total *int) *sdk.SmartGroupSearchResultV1 {
	return &sdk.SmartGroupSearchResultV1{
		SmartGroups: items,
		Page:        intPtr(page),
		PageSize:    intPtr(pageSize),
		Total:       total,
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// --- Metadata / Schema / Configure ---

func TestSmartGroupsDataSourceMetadata(t *testing.T) {
	t.Parallel()
	ds := &SmartGroupsDataSource{}
	var resp datasource.MetadataResponse
	ds.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "uem"}, &resp)

	if resp.TypeName != "uem_smart_groups" {
		t.Errorf("expected TypeName 'uem_smart_groups', got '%s'", resp.TypeName)
	}
}

func TestSmartGroupsDataSourceSchema(t *testing.T) {
	t.Parallel()
	schemaResp := getDSSchema(t)
	s := schemaResp.Schema

	for _, name := range []string{"name", "organization_group_id", "smart_group_id", "smart_group_uuid", "smart_groups"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing expected attribute: %s", name)
		}
	}

	// internal-ticket (onboard smart-group closure): smart_groups' nested object
	// must expose the owning org group's id/uuid, not just its name — ws1-tf
	// onboard's owned-only filter and dependency closure need to compare
	// against a numeric/uuid owner, the same way every other onboardable
	// type's list data source already does.
	nested, ok := s.Attributes["smart_groups"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("smart_groups is not a ListNestedAttribute: %T", s.Attributes["smart_groups"])
	}
	for _, name := range []string{"managed_by_org_group_id", "managed_by_org_group_uuid", "managed_by_org_group_name"} {
		if _, ok := nested.NestedObject.Attributes[name]; !ok {
			t.Errorf("missing expected smart_groups[] attribute: %s", name)
		}
	}
}

// createTestClient creates a *client.Client backed by the given httptest.Server.
func createTestClient(t *testing.T, handler http.HandlerFunc) (*client.Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)

	cfg := &client.Config{
		InstanceURL: server.URL,
		TenantCode:  "test-tenant",
		AuthMethod:  "basic",
		Username:    "test-user",
		Password:    "test-pass",
	}

	c, err := client.NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create test client: %v", err)
	}
	return c, server
}

func TestSmartGroupsDataSourceConfigure_NilProviderData(t *testing.T) {
	t.Parallel()
	ds := &SmartGroupsDataSource{}
	req := datasource.ConfigureRequest{ProviderData: nil}
	var resp datasource.ConfigureResponse
	ds.Configure(context.Background(), req, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatal("unexpected error for nil ProviderData")
	}
}

func TestSmartGroupsDataSourceConfigure_ValidClient(t *testing.T) {
	t.Parallel()
	c, server := createTestClient(t, func(w http.ResponseWriter, r *http.Request) {})
	defer server.Close()

	ds := &SmartGroupsDataSource{}
	req := datasource.ConfigureRequest{ProviderData: &providerdata.ProviderData{Client: c}}
	var resp datasource.ConfigureResponse
	ds.Configure(context.Background(), req, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatal("unexpected error for valid client")
	}
	if ds.client != c {
		t.Error("expected client to be set")
	}
	if ds.search == nil || ds.loader == nil {
		t.Error("expected search and loader to be set from the client")
	}
}

func TestSmartGroupsDataSourceConfigure_WrongType(t *testing.T) {
	t.Parallel()
	ds := &SmartGroupsDataSource{}
	req := datasource.ConfigureRequest{ProviderData: "not-a-client"}
	var resp datasource.ConfigureResponse
	ds.Configure(context.Background(), req, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for wrong ProviderData type")
	}
}

// --- ValidateConfig: mutual exclusion ---

func TestValidateConfig_SingleModeIsValid(t *testing.T) {
	t.Parallel()
	s := getDSSchema(t)
	d := &SmartGroupsDataSource{}

	for _, tc := range []struct {
		name      string
		overrides map[string]tftypes.Value
	}{
		{"no mode (default search, unfiltered)", nil},
		{"name filter only", map[string]tftypes.Value{"name": tftypes.NewValue(tftypes.String, "foo")}},
		{"organization_group_id filter only", map[string]tftypes.Value{"organization_group_id": tftypes.NewValue(tftypes.String, "138883")}},
		{"smart_group_id only", map[string]tftypes.Value{"smart_group_id": tftypes.NewValue(tftypes.Number, 42001)}},
		{"smart_group_uuid only", map[string]tftypes.Value{"smart_group_uuid": tftypes.NewValue(tftypes.String, "9af645a8-fef3-3e6d-3408-5cc69e0937d4")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := datasource.ValidateConfigRequest{Config: buildConfig(t, s, tc.overrides)}
			resp := &datasource.ValidateConfigResponse{}
			d.ValidateConfig(context.Background(), req, resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("expected no error, got %v", resp.Diagnostics)
			}
		})
	}
}

// TestValidateConfig_MultipleModesNoLongerErrors proves B16 (b)-row #215
// removal: ValidateConfig no longer rejects setting more than one lookup
// mode. Precedence between the modes is enforced by Read instead (see the
// TestRead_*Precedence tests below), not by ValidateConfig.
func TestValidateConfig_MultipleModesNoLongerErrors(t *testing.T) {
	t.Parallel()
	s := getDSSchema(t)
	d := &SmartGroupsDataSource{}

	for _, tc := range []struct {
		name      string
		overrides map[string]tftypes.Value
	}{
		{"smart_group_id and name", map[string]tftypes.Value{
			"smart_group_id": tftypes.NewValue(tftypes.Number, 1),
			"name":           tftypes.NewValue(tftypes.String, "foo"),
		}},
		{"smart_group_uuid and organization_group_id", map[string]tftypes.Value{
			"smart_group_uuid":      tftypes.NewValue(tftypes.String, "9af645a8-fef3-3e6d-3408-5cc69e0937d4"),
			"organization_group_id": tftypes.NewValue(tftypes.String, "138883"),
		}},
		{"smart_group_id and smart_group_uuid", map[string]tftypes.Value{
			"smart_group_id":   tftypes.NewValue(tftypes.Number, 1),
			"smart_group_uuid": tftypes.NewValue(tftypes.String, "9af645a8-fef3-3e6d-3408-5cc69e0937d4"),
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := datasource.ValidateConfigRequest{Config: buildConfig(t, s, tc.overrides)}
			resp := &datasource.ValidateConfigResponse{}
			d.ValidateConfig(context.Background(), req, resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("expected no error with more than one mode set, got %v", resp.Diagnostics)
			}
		})
	}
}

func TestValidateConfig_UnknownValueDoesNotConflict(t *testing.T) {
	t.Parallel()
	s := getDSSchema(t)
	d := &SmartGroupsDataSource{}

	req := datasource.ValidateConfigRequest{Config: buildConfig(t, s, map[string]tftypes.Value{
		"name":           tftypes.NewValue(tftypes.String, "foo"),
		"smart_group_id": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})}
	resp := &datasource.ValidateConfigResponse{}
	d.ValidateConfig(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error when the conflicting attribute is unknown, got %v", resp.Diagnostics)
	}
}

// --- ValidateConfig: numeric organization_group_id ---
//
// TestValidateConfig_NonNumericOrgGroupIDErrors is the fail-on-revert pin for
// the silent-drop bug fix: against the old Read-time `if id, err :=
// strconv.Atoi(...); err == nil { opts.OrganizationGroupID = &id }`, a
// non-numeric value was silently ignored and the search ran unfiltered. If
// ValidateConfig's numeric check is ever removed or weakened to not report an
// error, this test fails.
func TestValidateConfig_NonNumericOrgGroupIDErrors(t *testing.T) {
	t.Parallel()
	s := getDSSchema(t)
	d := &SmartGroupsDataSource{}

	req := datasource.ValidateConfigRequest{Config: buildConfig(t, s, map[string]tftypes.Value{
		"organization_group_id": tftypes.NewValue(tftypes.String, "not-a-number"),
	})}
	resp := &datasource.ValidateConfigResponse{}
	d.ValidateConfig(context.Background(), req, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for a non-numeric organization_group_id")
	}
}

func TestValidateConfig_NumericOrgGroupIDIsValid(t *testing.T) {
	t.Parallel()
	s := getDSSchema(t)
	d := &SmartGroupsDataSource{}

	req := datasource.ValidateConfigRequest{Config: buildConfig(t, s, map[string]tftypes.Value{
		"organization_group_id": tftypes.NewValue(tftypes.String, "138883"),
	})}
	resp := &datasource.ValidateConfigResponse{}
	d.ValidateConfig(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error for a numeric organization_group_id, got %v", resp.Diagnostics)
	}
}

// --- ValidateConfig: smart_group_uuid shape (B16 (b)-row #214 removal) ---

// TestValidateConfig_MalformedSmartGroupUUIDNoLongerErrors proves the
// UUID-shape regex is gone: a malformed smart_group_uuid is no longer
// rejected at plan time. TestRead_SmartGroupUUID_MalformedFindsNoMatch below
// proves it instead just finds no match at Read.
func TestValidateConfig_MalformedSmartGroupUUIDNoLongerErrors(t *testing.T) {
	t.Parallel()
	s := getDSSchema(t)
	d := &SmartGroupsDataSource{}

	req := datasource.ValidateConfigRequest{Config: buildConfig(t, s, map[string]tftypes.Value{
		"smart_group_uuid": tftypes.NewValue(tftypes.String, "not-a-uuid"),
	})}
	resp := &datasource.ValidateConfigResponse{}
	d.ValidateConfig(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no plan-time error for a malformed smart_group_uuid, got %v", resp.Diagnostics)
	}
}

func TestValidateConfig_WellFormedSmartGroupUUIDIsValid(t *testing.T) {
	t.Parallel()
	s := getDSSchema(t)
	d := &SmartGroupsDataSource{}

	req := datasource.ValidateConfigRequest{Config: buildConfig(t, s, map[string]tftypes.Value{
		"smart_group_uuid": tftypes.NewValue(tftypes.String, "9af645a8-fef3-3e6d-3408-5cc69e0937d4"),
	})}
	resp := &datasource.ValidateConfigResponse{}
	d.ValidateConfig(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error for a well-formed (uppercase) uuid, got %v", resp.Diagnostics)
	}
}

// --- Read: smart_group_id exact-lookup mode ---

func TestRead_SmartGroupID_Found(t *testing.T) {
	t.Parallel()
	s := getDSSchema(t)
	id := 42001
	fl := &fakeSmartGroupLoader{group: &sdk.SmartGroupV1{
		SmartGroupID:                   &id,
		Name:                           "example-smartgroup",
		SmartGroupUUID:                 "9af645a8-fef3-3e6d-3408-5cc69e0937d4",
		ManagedByOrganizationGroupName: "Example Org Group",
		ManagedByOrganizationGroupID:   "571",
		ManagedByOrganizationGroupUUID: "666ff6cc-aa5b-3c07-feaa-3a95d3a4bd2c",
	}}
	d := &SmartGroupsDataSource{loader: fl, search: &fakeSmartGroupSearch{}}

	req := datasource.ReadRequest{Config: buildConfig(t, s, map[string]tftypes.Value{
		"smart_group_id": tftypes.NewValue(tftypes.Number, 42001),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(t)}
	d.Read(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", resp.Diagnostics)
	}

	var model SmartGroupsDataSourceModel
	resp.Diagnostics.Append(resp.State.Get(context.Background(), &model)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("state Get diagnostics: %v", resp.Diagnostics)
	}
	if len(model.SmartGroups) != 1 {
		t.Fatalf("expected exactly 1 smart group, got %d", len(model.SmartGroups))
	}
	g := model.SmartGroups[0]
	if g.SmartGroupID.ValueInt64() != 42001 || g.Name.ValueString() != "example-smartgroup" {
		t.Fatalf("unexpected result: %+v", g)
	}
	// internal-ticket (onboard smart-group closure): managed_by_org_group_id/uuid
	// must round-trip from LoadSmartGroupAsync's response — ws1-tf onboard's
	// owned-only filter and dependency closure compare against these.
	if g.ManagedByOrgGroupID.ValueString() != "571" {
		t.Fatalf("expected managed_by_org_group_id %q, got %q", "571", g.ManagedByOrgGroupID.ValueString())
	}
	if g.ManagedByOrgGroupUUID.ValueString() != "666ff6cc-aa5b-3c07-feaa-3a95d3a4bd2c" {
		t.Fatalf("expected managed_by_org_group_uuid %q, got %q", "666ff6cc-aa5b-3c07-feaa-3a95d3a4bd2c", g.ManagedByOrgGroupUUID.ValueString())
	}
	if fl.idSeen != 42001 {
		t.Fatalf("expected LoadSmartGroupAsync(42001), got %d", fl.idSeen)
	}
}

// TestRead_SmartGroupID_NotFound_ReturnsEmptyListNotError pins the not-found
// contract: client.IsNotFound recognises the smart-group-specific 400 shape,
// and Read must translate that into an EMPTY list, never a diagnostic error.
func TestRead_SmartGroupID_NotFound_ReturnsEmptyListNotError(t *testing.T) {
	t.Parallel()
	s := getDSSchema(t)
	notFoundErr := &sdk.APIError{
		StatusCode: http.StatusBadRequest,
		ErrorCode:  "6",
		Message:    "Smart Group not found with Id: 999999999 or User does not have access to it.",
	}
	fl := &fakeSmartGroupLoader{err: notFoundErr}
	d := &SmartGroupsDataSource{loader: fl, search: &fakeSmartGroupSearch{}}

	req := datasource.ReadRequest{Config: buildConfig(t, s, map[string]tftypes.Value{
		"smart_group_id": tftypes.NewValue(tftypes.Number, 999999999),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(t)}
	d.Read(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error for a not-found smart_group_id, got %v", resp.Diagnostics)
	}

	var model SmartGroupsDataSourceModel
	resp.Diagnostics.Append(resp.State.Get(context.Background(), &model)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("state Get diagnostics: %v", resp.Diagnostics)
	}
	if len(model.SmartGroups) != 0 {
		t.Fatalf("expected an empty list for a not-found smart_group_id, got %d entries", len(model.SmartGroups))
	}
}

// TestRead_SmartGroupID_GenericAPIErrorFails proves a REAL (non-not-found)
// API error still fails the read -- the not-found translation must not
// swallow every error.
func TestRead_SmartGroupID_GenericAPIErrorFails(t *testing.T) {
	t.Parallel()
	s := getDSSchema(t)
	fl := &fakeSmartGroupLoader{err: &sdk.APIError{StatusCode: http.StatusInternalServerError, Message: "boom"}}
	d := &SmartGroupsDataSource{loader: fl, search: &fakeSmartGroupSearch{}}

	req := datasource.ReadRequest{Config: buildConfig(t, s, map[string]tftypes.Value{
		"smart_group_id": tftypes.NewValue(tftypes.Number, 1),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(t)}
	d.Read(context.Background(), req, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error for a generic (non-not-found) API error")
	}
}

func TestRead_SmartGroupID_PlainErrorFails(t *testing.T) {
	t.Parallel()
	s := getDSSchema(t)
	fl := &fakeSmartGroupLoader{err: fmt.Errorf("boom")}
	d := &SmartGroupsDataSource{loader: fl, search: &fakeSmartGroupSearch{}}

	req := datasource.ReadRequest{Config: buildConfig(t, s, map[string]tftypes.Value{
		"smart_group_id": tftypes.NewValue(tftypes.Number, 1),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(t)}
	d.Read(context.Background(), req, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when the loader returns a plain (non-APIError) error")
	}
}

// --- Read: smart_group_uuid exact-lookup mode ---

// TestRead_SmartGroupUUID_ExactMatchVsNearMiss is the fail-on-revert pin for
// the uuid-match logic: the fake search page holds a near-miss (differs by
// one trailing character) and an exact (case-differing) match. Only the exact
// match may be returned; a substring/prefix match would wrongly also return
// the near-miss.
func TestRead_SmartGroupUUID_ExactMatchVsNearMiss(t *testing.T) {
	t.Parallel()
	s := getDSSchema(t)
	total := 3
	fs := &fakeSmartGroupSearch{pages: map[int]*sdk.SmartGroupSearchResultV1{
		0: searchResult([]sdk.SmartGroupSearchModelV1{
			sg(1, "near-miss", "f30f49fb-2b15-d5c6-098f-94c60a0b6c9e"), // differs in the last hex digit
			sg(2, "example-smartgroup", "9af645a8-fef3-3e6d-3408-5cc69e0937d4"),
			sg(3, "superstring", "{9af645a8-fef3-3e6d-3408-5cc69e0937d4}"), // contains the query; must not match
		}, 0, 500, &total),
	}}
	d := &SmartGroupsDataSource{search: fs, loader: &fakeSmartGroupLoader{}}

	req := datasource.ReadRequest{Config: buildConfig(t, s, map[string]tftypes.Value{
		"smart_group_uuid": tftypes.NewValue(tftypes.String, "9af645a8-fef3-3e6d-3408-5cc69e0937d4"),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(t)}
	d.Read(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", resp.Diagnostics)
	}

	var model SmartGroupsDataSourceModel
	resp.Diagnostics.Append(resp.State.Get(context.Background(), &model)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("state Get diagnostics: %v", resp.Diagnostics)
	}
	if len(model.SmartGroups) != 1 {
		t.Fatalf("expected exactly 1 exact match (case-insensitive), got %d: %+v", len(model.SmartGroups), model.SmartGroups)
	}
	if model.SmartGroups[0].SmartGroupID.ValueInt64() != 2 {
		t.Fatalf("expected the exact match (id 2), got %+v", model.SmartGroups[0])
	}
}

func TestRead_SmartGroupUUID_NoMatch_ReturnsEmptyList(t *testing.T) {
	t.Parallel()
	s := getDSSchema(t)
	total := 1
	fs := &fakeSmartGroupSearch{pages: map[int]*sdk.SmartGroupSearchResultV1{
		0: searchResult([]sdk.SmartGroupSearchModelV1{sg(1, "other", "bafde89c-041e-1756-082b-933aaf16cad8")}, 0, 500, &total),
	}}
	d := &SmartGroupsDataSource{search: fs, loader: &fakeSmartGroupLoader{}}

	req := datasource.ReadRequest{Config: buildConfig(t, s, map[string]tftypes.Value{
		"smart_group_uuid": tftypes.NewValue(tftypes.String, "9af645a8-fef3-3e6d-3408-5cc69e0937d4"),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(t)}
	d.Read(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", resp.Diagnostics)
	}

	var model SmartGroupsDataSourceModel
	resp.Diagnostics.Append(resp.State.Get(context.Background(), &model)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("state Get diagnostics: %v", resp.Diagnostics)
	}
	if len(model.SmartGroups) != 0 {
		t.Fatalf("expected an empty list, got %d entries", len(model.SmartGroups))
	}
}

func TestRead_SmartGroupUUID_SearchAPIErrorFails(t *testing.T) {
	t.Parallel()
	s := getDSSchema(t)
	fs := &fakeSmartGroupSearch{err: fmt.Errorf("boom")}
	d := &SmartGroupsDataSource{search: fs, loader: &fakeSmartGroupLoader{}}

	req := datasource.ReadRequest{Config: buildConfig(t, s, map[string]tftypes.Value{
		"smart_group_uuid": tftypes.NewValue(tftypes.String, "9af645a8-fef3-3e6d-3408-5cc69e0937d4"),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(t)}
	d.Read(context.Background(), req, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected an error when the underlying search fails")
	}
}

// TestRead_SmartGroupUUID_MalformedFindsNoMatch proves B16 (b)-row #214
// removal end-to-end: a malformed smart_group_uuid is not rejected at plan
// time (see TestValidateConfig_MalformedSmartGroupUUIDNoLongerErrors) and
// simply matches nothing at Read, the same as any other non-matching value.
func TestRead_SmartGroupUUID_MalformedFindsNoMatch(t *testing.T) {
	t.Parallel()
	s := getDSSchema(t)
	total := 1
	fs := &fakeSmartGroupSearch{pages: map[int]*sdk.SmartGroupSearchResultV1{
		0: searchResult([]sdk.SmartGroupSearchModelV1{sg(1, "other", "bafde89c-041e-1756-082b-933aaf16cad8")}, 0, 500, &total),
	}}
	d := &SmartGroupsDataSource{search: fs, loader: &fakeSmartGroupLoader{}}

	req := datasource.ReadRequest{Config: buildConfig(t, s, map[string]tftypes.Value{
		"smart_group_uuid": tftypes.NewValue(tftypes.String, "not-a-uuid"),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(t)}
	d.Read(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error for a malformed smart_group_uuid at Read, got %v", resp.Diagnostics)
	}

	var model SmartGroupsDataSourceModel
	resp.Diagnostics.Append(resp.State.Get(context.Background(), &model)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("state Get diagnostics: %v", resp.Diagnostics)
	}
	if len(model.SmartGroups) != 0 {
		t.Fatalf("expected an empty list for a malformed smart_group_uuid, got %d entries", len(model.SmartGroups))
	}
}

// --- Read: precedence when more than one mode is set (B16 (b)-row #215
// removal) ---

// TestRead_ModePrecedence_SmartGroupIDWinsOverUUIDAndSearchFilters proves
// smart_group_id takes precedence: when smart_group_id, smart_group_uuid,
// and name are all set, Read uses the loader (exact-id lookup) and never
// calls the search API at all.
func TestRead_ModePrecedence_SmartGroupIDWinsOverUUIDAndSearchFilters(t *testing.T) {
	t.Parallel()
	s := getDSSchema(t)
	id := 42001
	fl := &fakeSmartGroupLoader{group: &sdk.SmartGroupV1{SmartGroupID: &id, Name: "winner"}}
	fs := &fakeSmartGroupSearch{}
	d := &SmartGroupsDataSource{loader: fl, search: fs}

	req := datasource.ReadRequest{Config: buildConfig(t, s, map[string]tftypes.Value{
		"smart_group_id":   tftypes.NewValue(tftypes.Number, 42001),
		"smart_group_uuid": tftypes.NewValue(tftypes.String, "9af645a8-fef3-3e6d-3408-5cc69e0937d4"),
		"name":             tftypes.NewValue(tftypes.String, "foo"),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(t)}
	d.Read(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", resp.Diagnostics)
	}

	if fl.idSeen != 42001 {
		t.Fatalf("expected LoadSmartGroupAsync(42001), got %d", fl.idSeen)
	}
	if len(fs.pagesRequested) != 0 {
		t.Fatalf("expected the search API never to be called when smart_group_id is set, got %d calls", len(fs.pagesRequested))
	}

	var model SmartGroupsDataSourceModel
	resp.Diagnostics.Append(resp.State.Get(context.Background(), &model)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("state Get diagnostics: %v", resp.Diagnostics)
	}
	if len(model.SmartGroups) != 1 || model.SmartGroups[0].Name.ValueString() != "winner" {
		t.Fatalf("expected the smart_group_id result to win, got %+v", model.SmartGroups)
	}
}

// TestRead_ModePrecedence_SmartGroupUUIDWinsOverSearchFilters proves
// smart_group_uuid takes precedence over the search filters: when
// smart_group_uuid and organization_group_id are both set (and
// smart_group_id is not), Read walks the search UNFILTERED (ignoring
// organization_group_id) and keeps only the exact uuid match.
func TestRead_ModePrecedence_SmartGroupUUIDWinsOverSearchFilters(t *testing.T) {
	t.Parallel()
	s := getDSSchema(t)
	total := 1
	fs := &fakeSmartGroupSearch{pages: map[int]*sdk.SmartGroupSearchResultV1{
		0: searchResult([]sdk.SmartGroupSearchModelV1{
			sg(1, "example-smartgroup", "9af645a8-fef3-3e6d-3408-5cc69e0937d4"),
		}, 0, 500, &total),
	}}
	d := &SmartGroupsDataSource{search: fs, loader: &fakeSmartGroupLoader{}}

	req := datasource.ReadRequest{Config: buildConfig(t, s, map[string]tftypes.Value{
		"smart_group_uuid":      tftypes.NewValue(tftypes.String, "9af645a8-fef3-3e6d-3408-5cc69e0937d4"),
		"organization_group_id": tftypes.NewValue(tftypes.String, "138883"),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(t)}
	d.Read(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", resp.Diagnostics)
	}

	if fs.lastOpts == nil || fs.lastOpts.OrganizationGroupID != nil {
		t.Fatalf("expected the search to run unfiltered (organization_group_id ignored), got opts %+v", fs.lastOpts)
	}

	var model SmartGroupsDataSourceModel
	resp.Diagnostics.Append(resp.State.Get(context.Background(), &model)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("state Get diagnostics: %v", resp.Diagnostics)
	}
	if len(model.SmartGroups) != 1 || model.SmartGroups[0].Name.ValueString() != "example-smartgroup" {
		t.Fatalf("expected the uuid match, got %+v", model.SmartGroups)
	}
}

// --- Read: default search mode ---

func TestRead_NumericOrgGroupID_IsSent(t *testing.T) {
	t.Parallel()
	s := getDSSchema(t)
	total := 0
	fs := &fakeSmartGroupSearch{pages: map[int]*sdk.SmartGroupSearchResultV1{
		0: searchResult(nil, 0, 500, &total),
	}}
	d := &SmartGroupsDataSource{search: fs, loader: &fakeSmartGroupLoader{}}

	req := datasource.ReadRequest{Config: buildConfig(t, s, map[string]tftypes.Value{
		"organization_group_id": tftypes.NewValue(tftypes.String, "12345"),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(t)}
	d.Read(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if fs.lastOpts == nil || fs.lastOpts.OrganizationGroupID == nil || *fs.lastOpts.OrganizationGroupID != 12345 {
		t.Fatalf("expected organizationgroupid=12345 to be sent, got opts %+v", fs.lastOpts)
	}
}

func TestRead_NameFilter_IsSent(t *testing.T) {
	t.Parallel()
	s := getDSSchema(t)
	total := 0
	fs := &fakeSmartGroupSearch{pages: map[int]*sdk.SmartGroupSearchResultV1{
		0: searchResult(nil, 0, 500, &total),
	}}
	d := &SmartGroupsDataSource{search: fs, loader: &fakeSmartGroupLoader{}}

	req := datasource.ReadRequest{Config: buildConfig(t, s, map[string]tftypes.Value{
		"name": tftypes.NewValue(tftypes.String, "All Devices"),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(t)}
	d.Read(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}
	if fs.lastOpts == nil || fs.lastOpts.Name == nil || *fs.lastOpts.Name != "All Devices" {
		t.Fatalf("expected name filter to be sent, got opts %+v", fs.lastOpts)
	}
}

func TestRead_EmptyResult_EmitsEmptyNotNullList(t *testing.T) {
	t.Parallel()
	s := getDSSchema(t)
	total := 0
	fs := &fakeSmartGroupSearch{pages: map[int]*sdk.SmartGroupSearchResultV1{
		0: searchResult(nil, 0, 500, &total),
	}}
	d := &SmartGroupsDataSource{search: fs, loader: &fakeSmartGroupLoader{}}

	req := datasource.ReadRequest{Config: buildConfig(t, s, nil)}
	resp := &datasource.ReadResponse{State: emptyDSState(t)}
	d.Read(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error: %v", resp.Diagnostics)
	}

	var obj map[string]tftypes.Value
	if err := resp.State.Raw.As(&obj); err != nil {
		t.Fatalf("decode state as object: %v", err)
	}
	if obj["smart_groups"].IsNull() {
		t.Fatal("expected smart_groups to be a known EMPTY list on a zero-result search, got NULL")
	}
}

func TestRead_SearchAPIError_ReturnsDiagnostic(t *testing.T) {
	t.Parallel()
	s := getDSSchema(t)
	d := &SmartGroupsDataSource{search: &fakeSmartGroupSearch{err: fmt.Errorf("server error")}, loader: &fakeSmartGroupLoader{}}

	req := datasource.ReadRequest{Config: buildConfig(t, s, nil)}
	resp := &datasource.ReadResponse{State: emptyDSState(t)}
	d.Read(context.Background(), req, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error diagnostic for API failure")
	}
}

// TestRead_TransientServerError_IsRetried is the one app-level test that pays
// the SDK's real retry backoff (RetryWaitMin is fixed at 1s), using a real
// *client.Client (via Configure) rather than the fake interfaces, so it
// exercises the real HTTP retry path.
func TestRead_TransientServerError_IsRetried(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprintln(w, `{"message": "transient"}`)
			return
		}
		_ = json.NewEncoder(w).Encode(sdk.SmartGroupSearchResultV1{
			SmartGroups: nil,
			Total:       intPtr(0),
		})
	}))
	defer server.Close()

	c, err := client.NewClient(&client.Config{
		InstanceURL: server.URL,
		TenantCode:  "test-tenant",
		AuthMethod:  "basic",
		Username:    "test-user",
		Password:    "test-pass",
		MaxRetries:  1,
	})
	if err != nil {
		t.Fatalf("failed to create test client: %v", err)
	}

	ds := &SmartGroupsDataSource{client: c}
	s := getDSSchema(t)
	req := datasource.ReadRequest{Config: buildConfig(t, s, nil)}
	resp := &datasource.ReadResponse{State: emptyDSState(t)}

	ds.Read(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected the retry to succeed, got: %v", resp.Diagnostics.Errors())
	}
	// 500 then 200 (page 0, empty/Total 0) then 200 (page 1, the base probe's
	// fallback fetch, also empty) -- the base probe always tries both page 0
	// and page 1 before confirming zero results; see internal/pagewalk's
	// probeBase.
	if got := calls.Load(); got != 3 {
		t.Fatalf("expected 3 requests (500, then 200 on page 0, then 200 on the page-1 base probe), got %d", got)
	}
}

// --- listAllSearchRaw (pagination hardening) ---
//
// These pin the walkSmartGroupSearch/double-walk contract, mirroring
// internal/organizationgroup's listAllSearch tests. The old "Total is only a
// backstop, stop once len(allGroups) >= Total" behaviour is gone: Total must
// match the walk's own unique-id count exactly, never merely bound it.

func TestListAllSearchRaw_SingleShortPageStopsImmediately(t *testing.T) {
	t.Parallel()
	total := 2
	fs := &fakeSmartGroupSearch{pages: map[int]*sdk.SmartGroupSearchResultV1{
		0: searchResult([]sdk.SmartGroupSearchModelV1{sg(1, "A", "uuid-1"), sg(2, "B", "uuid-2")}, 0, 3, &total),
	}}
	d := &SmartGroupsDataSource{search: fs, pageSize: 3}

	got, err := d.listAllSearchRaw(context.Background(), fs, nil, nil)
	if err != nil {
		t.Fatalf("listAllSearchRaw: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(got))
	}
	if !equalInts(fs.pagesRequested, []int{0}) {
		t.Fatalf("expected exactly one request for page 0 (a single-page walk is never repeated), got %v", fs.pagesRequested)
	}
}

func TestListAllSearchRaw_MultiPageHappyPathTriggersSecondWalk(t *testing.T) {
	t.Parallel()
	total := 5
	fs := &fakeSmartGroupSearch{pages: map[int]*sdk.SmartGroupSearchResultV1{
		0: searchResult([]sdk.SmartGroupSearchModelV1{sg(1, "A", "u1"), sg(2, "B", "u2")}, 0, 2, &total),
		1: searchResult([]sdk.SmartGroupSearchModelV1{sg(3, "C", "u3"), sg(4, "D", "u4")}, 1, 2, &total),
		2: searchResult([]sdk.SmartGroupSearchModelV1{sg(5, "E", "u5")}, 2, 2, &total),
	}}
	d := &SmartGroupsDataSource{search: fs, pageSize: 2}

	got, err := d.listAllSearchRaw(context.Background(), fs, nil, nil)
	if err != nil {
		t.Fatalf("listAllSearchRaw: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("expected 5 groups, got %d", len(got))
	}
	if !equalInts(fs.pagesRequested, []int{0, 1, 2, 0, 1, 2}) {
		t.Fatalf("expected the 3-page walk to run twice (double-walk), got %v", fs.pagesRequested)
	}
}

// TestListAllSearchRaw_OneBasedMultiPage_ProbeFallsBackToPageOne is the
// fail-on-revert pin for the base probe itself (internal-ticket, release blocker
// B12): the smart group search walk previously hard-coded page base 0,
// copied from the org-groups data source and never independently verified
// live against the SmartGroups search endpoint. Page 0 here has no entry (the
// live-confirmed HTTP-204/empty out-of-range shape), so a walk that still
// hard-codes base 0 would see an empty/zero-item first page and either wrongly
// report zero results or error, never reaching the 2 real groups on page 1.
// If the base probe is ever reverted to a hard-coded 0, this test fails.
func TestListAllSearchRaw_OneBasedMultiPage_ProbeFallsBackToPageOne(t *testing.T) {
	t.Parallel()
	total := 3
	fs := &fakeSmartGroupSearch{
		nilForMissing: true,
		pages: map[int]*sdk.SmartGroupSearchResultV1{
			// no entry for page 0: forces the fallback probe to page 1.
			1: searchResult([]sdk.SmartGroupSearchModelV1{sg(1, "A", "u1"), sg(2, "B", "u2")}, 1, 2, &total),
			2: searchResult([]sdk.SmartGroupSearchModelV1{sg(3, "C", "u3")}, 2, 2, &total), // short: 1 < pageSize
		},
	}
	d := &SmartGroupsDataSource{search: fs, pageSize: 2}

	got, err := d.listAllSearchRaw(context.Background(), fs, nil, nil)
	if err != nil {
		t.Fatalf("listAllSearchRaw: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(got))
	}
	// Multi-page (2 pages: 1 and 2), so the walk is repeated once, each time
	// re-probing from page 0.
	if !equalInts(fs.pagesRequested, []int{0, 1, 2, 0, 1, 2}) {
		t.Fatalf("expected the probe to try page 0 then fall back to page 1, walked twice, got %v", fs.pagesRequested)
	}
}

// TestListAllSearchRaw_OutOfRangePageReturns204Shape pins the live-confirmed
// out-of-range shape (HTTP 204, surfaced by the SDK as a nil result with a
// nil error) observed on both as<internal-env> and paul-2609: a page-size-aligned
// result's confirming page must be treated as end-of-results, not an error.
func TestListAllSearchRaw_OutOfRangePageReturns204Shape(t *testing.T) {
	t.Parallel()
	total := 2
	fs := &fakeSmartGroupSearch{
		nilForMissing: true,
		pages: map[int]*sdk.SmartGroupSearchResultV1{
			0: searchResult([]sdk.SmartGroupSearchModelV1{sg(1, "A", "u1"), sg(2, "B", "u2")}, 0, 2, &total),
			// no entry for page 1: SearchAsync returns (nil, nil, nil), the HTTP 204 shape.
		},
	}
	d := &SmartGroupsDataSource{search: fs, pageSize: 2}

	got, err := d.listAllSearchRaw(context.Background(), fs, nil, nil)
	if err != nil {
		t.Fatalf("listAllSearchRaw: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(got))
	}
	// Multi-page (page 0 full + confirming page 1), so walked twice.
	if !equalInts(fs.pagesRequested, []int{0, 1, 0, 1}) {
		t.Fatalf("expected page 0 then a confirming (204) page 1, walked twice, got %v", fs.pagesRequested)
	}
}

func TestListAllSearchRaw_TotalExactMultipleOfPageSizeFetchesConfirmingEmptyPage(t *testing.T) {
	t.Parallel()
	total := 4
	fs := &fakeSmartGroupSearch{pages: map[int]*sdk.SmartGroupSearchResultV1{
		0: searchResult([]sdk.SmartGroupSearchModelV1{sg(1, "A", "u1"), sg(2, "B", "u2")}, 0, 2, &total),
		1: searchResult([]sdk.SmartGroupSearchModelV1{sg(3, "C", "u3"), sg(4, "D", "u4")}, 1, 2, &total),
		2: searchResult(nil, 2, 2, &total),
	}}
	d := &SmartGroupsDataSource{search: fs, pageSize: 2}

	got, err := d.listAllSearchRaw(context.Background(), fs, nil, nil)
	if err != nil {
		t.Fatalf("listAllSearchRaw: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 groups, got %d", len(got))
	}
	if !equalInts(fs.pagesRequested, []int{0, 1, 2, 0, 1, 2}) {
		t.Fatalf("expected the confirming empty page 2 to be fetched, and the whole 3-page walk repeated, got %v", fs.pagesRequested)
	}
}

func TestListAllSearchRaw_EmptyPageZeroReturnsEmptyResult(t *testing.T) {
	t.Parallel()
	fs := &fakeSmartGroupSearch{pages: map[int]*sdk.SmartGroupSearchResultV1{}}
	d := &SmartGroupsDataSource{search: fs, pageSize: 2}

	got, err := d.listAllSearchRaw(context.Background(), fs, nil, nil)
	if err != nil {
		t.Fatalf("listAllSearchRaw: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 groups, got %d", len(got))
	}
	// The base probe tries page 0 first, then falls back to page 1 before
	// confirming zero results -- see internal/pagewalk's probeBase. Both come
	// back empty here, so both are requested.
	if !equalInts(fs.pagesRequested, []int{0, 1}) {
		t.Fatalf("expected both page 0 and page 1 to be probed before confirming zero, got %v", fs.pagesRequested)
	}
}

func TestListAllSearchRaw_DedupsRepeatedIDAcrossPages(t *testing.T) {
	t.Parallel()
	total := 4
	fs := &fakeSmartGroupSearch{pages: map[int]*sdk.SmartGroupSearchResultV1{
		0: searchResult([]sdk.SmartGroupSearchModelV1{sg(1, "A", "u1"), sg(2, "B", "u2")}, 0, 2, &total),
		1: searchResult([]sdk.SmartGroupSearchModelV1{sg(2, "B", "u2"), sg(3, "C", "u3")}, 1, 2, &total),
		2: searchResult([]sdk.SmartGroupSearchModelV1{sg(4, "D", "u4")}, 2, 2, &total),
	}}
	d := &SmartGroupsDataSource{search: fs, pageSize: 2}

	got, err := d.listAllSearchRaw(context.Background(), fs, nil, nil)
	if err != nil {
		t.Fatalf("listAllSearchRaw: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 deduplicated groups, got %d", len(got))
	}
}

// TestListAllSearchRaw_TotalTooSmallErrors is a fail-on-revert pin for the
// "unique > total is an error" paging rule.
func TestListAllSearchRaw_TotalTooSmallErrors(t *testing.T) {
	t.Parallel()
	total := 2
	fs := &fakeSmartGroupSearch{pages: map[int]*sdk.SmartGroupSearchResultV1{
		0: searchResult([]sdk.SmartGroupSearchModelV1{sg(1, "A", "u1"), sg(2, "B", "u2"), sg(3, "C", "u3")}, 0, 3, &total),
	}}
	d := &SmartGroupsDataSource{search: fs, pageSize: 3}

	got, err := d.listAllSearchRaw(context.Background(), fs, nil, nil)
	if err == nil {
		t.Fatalf("expected an error when more distinct groups exist than Total claims, got %d groups", len(got))
	}
	if !strings.Contains(err.Error(), "reported total") {
		t.Fatalf("expected a total-related error, got: %v", err)
	}
}

// TestListAllSearchRaw_TotalTooLargeWithEmptyPageErrors is a fail-on-revert
// pin for the "ran out early" paging rule: this is exactly the bug the old
// `len(allGroups) >= total` stop could never detect, since it only ever
// checked >=, never noticed the list running out before reaching Total.
func TestListAllSearchRaw_TotalTooLargeWithEmptyPageErrors(t *testing.T) {
	t.Parallel()
	total := 10
	fs := &fakeSmartGroupSearch{pages: map[int]*sdk.SmartGroupSearchResultV1{
		0: searchResult([]sdk.SmartGroupSearchModelV1{sg(1, "A", "u1"), sg(2, "B", "u2"), sg(3, "C", "u3")}, 0, 3, &total),
		1: searchResult(nil, 1, 3, &total),
	}}
	d := &SmartGroupsDataSource{search: fs, pageSize: 3}

	_, err := d.listAllSearchRaw(context.Background(), fs, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "empty page") {
		t.Fatalf("expected an empty-page-before-Total-reached error, got %v", err)
	}
}

func TestListAllSearchRaw_TotalChangesBetweenNonEmptyPagesErrors(t *testing.T) {
	t.Parallel()
	total4, total5 := 4, 5
	fs := &fakeSmartGroupSearch{pages: map[int]*sdk.SmartGroupSearchResultV1{
		0: searchResult([]sdk.SmartGroupSearchModelV1{sg(1, "A", "u1"), sg(2, "B", "u2")}, 0, 2, &total4),
		1: searchResult([]sdk.SmartGroupSearchModelV1{sg(3, "C", "u3"), sg(4, "D", "u4")}, 1, 2, &total5),
	}}
	d := &SmartGroupsDataSource{search: fs, pageSize: 2}

	_, err := d.listAllSearchRaw(context.Background(), fs, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("expected a Total-changed-during-the-walk error, got %v", err)
	}
}

func TestListAllSearchRaw_FullPageWithNoNewIDsErrors(t *testing.T) {
	t.Parallel()
	total := 4 // never reachable via duplicates alone
	fs := &fakeSmartGroupSearch{
		pages: map[int]*sdk.SmartGroupSearchResultV1{
			0: searchResult([]sdk.SmartGroupSearchModelV1{sg(1, "A", "u1"), sg(2, "B", "u2")}, 0, 2, &total),
		},
		ignorePage: true,
	}
	d := &SmartGroupsDataSource{search: fs, pageSize: 2}

	_, err := d.listAllSearchRaw(context.Background(), fs, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "no progress") {
		t.Fatalf("expected a no-progress error, got %v", err)
	}
}

func TestListAllSearchRaw_FullPageAfterTotalReachedErrors(t *testing.T) {
	t.Parallel()
	total := 2
	fs := &fakeSmartGroupSearch{pages: map[int]*sdk.SmartGroupSearchResultV1{
		0: searchResult([]sdk.SmartGroupSearchModelV1{sg(1, "A", "u1"), sg(2, "B", "u2")}, 0, 2, &total),
		1: searchResult([]sdk.SmartGroupSearchModelV1{sg(1, "A", "u1"), sg(2, "B", "u2")}, 1, 2, &total),
	}}
	d := &SmartGroupsDataSource{search: fs, pageSize: 2}

	_, err := d.listAllSearchRaw(context.Background(), fs, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "already seen") {
		t.Fatalf("expected a full-page-after-total-reached error, got %v", err)
	}
}

func TestListAllSearchRaw_MaxPagesCapErrorsNeverTruncates(t *testing.T) {
	t.Parallel()
	total := maxSmartGroupSearchPages + 1
	pages := map[int]*sdk.SmartGroupSearchResultV1{}
	for i := 0; i < maxSmartGroupSearchPages; i++ {
		pages[i] = searchResult([]sdk.SmartGroupSearchModelV1{sg(int64(i)+1, fmt.Sprintf("G%d", i), fmt.Sprintf("u%d", i))}, i, 1, &total)
	}
	fs := &fakeSmartGroupSearch{pages: pages}
	d := &SmartGroupsDataSource{search: fs, pageSize: 1}

	got, err := d.listAllSearchRaw(context.Background(), fs, nil, nil)
	if err == nil {
		t.Fatalf("expected an error once the walk hit maxSmartGroupSearchPages, got %d groups with no error", len(got))
	}
	if !strings.Contains(err.Error(), "aborting rather than silently returning a truncated result set") {
		t.Fatalf("expected the page-cap error, got: %v", err)
	}
	if len(fs.pagesRequested) != maxSmartGroupSearchPages {
		t.Fatalf("expected exactly %d page requests, got %d", maxSmartGroupSearchPages, len(fs.pagesRequested))
	}
}

func TestListAllSearchRaw_NilTotalOnNonEmptyPageErrors(t *testing.T) {
	t.Parallel()
	fs := &fakeSmartGroupSearch{pages: map[int]*sdk.SmartGroupSearchResultV1{
		0: searchResult([]sdk.SmartGroupSearchModelV1{sg(1, "A", "u1"), sg(2, "B", "u2")}, 0, 2, nil),
	}}
	d := &SmartGroupsDataSource{search: fs, pageSize: 2}

	_, err := d.listAllSearchRaw(context.Background(), fs, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "no reported total") {
		t.Fatalf("expected a no-reported-total error, got %v", err)
	}
}

func TestListAllSearchRaw_SearchErrorPropagates(t *testing.T) {
	t.Parallel()
	fs := &fakeSmartGroupSearch{err: fmt.Errorf("boom")}
	d := &SmartGroupsDataSource{search: fs, pageSize: 2}

	if _, err := d.listAllSearchRaw(context.Background(), fs, nil, nil); err == nil {
		t.Fatal("expected the search error to propagate")
	}
}

// TestListAllSearchRaw_SecondWalkDifferentIDSetErrors is a fail-on-revert pin
// for the double-walk consistency check: both walks report the same Total
// and count, but the actual id sets differ.
func TestListAllSearchRaw_SecondWalkDifferentIDSetErrors(t *testing.T) {
	t.Parallel()
	fs := &fakeSmartGroupSearch{sequence: []*sdk.SmartGroupSearchResultV1{
		searchResult([]sdk.SmartGroupSearchModelV1{sg(1, "A", "u1"), sg(2, "B", "u2")}, 0, 2, intPtr(3)),
		searchResult([]sdk.SmartGroupSearchModelV1{sg(3, "C", "u3")}, 1, 2, intPtr(3)),
		searchResult([]sdk.SmartGroupSearchModelV1{sg(1, "A", "u1"), sg(2, "B", "u2")}, 0, 2, intPtr(3)),
		searchResult([]sdk.SmartGroupSearchModelV1{sg(4, "D", "u4")}, 1, 2, intPtr(3)),
	}}
	d := &SmartGroupsDataSource{search: fs, pageSize: 2}

	_, err := d.listAllSearchRaw(context.Background(), fs, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "different set of ids") {
		t.Fatalf("expected a different-id-set error, got %v", err)
	}
	if len(fs.pagesRequested) != 4 {
		t.Fatalf("expected exactly 4 calls (2 per walk), got %d", len(fs.pagesRequested))
	}
}

func TestListAllSearchRaw_SecondWalkTotalDiffersErrors(t *testing.T) {
	t.Parallel()
	fs := &fakeSmartGroupSearch{sequence: []*sdk.SmartGroupSearchResultV1{
		searchResult([]sdk.SmartGroupSearchModelV1{sg(1, "A", "u1")}, 0, 1, intPtr(2)),
		searchResult([]sdk.SmartGroupSearchModelV1{sg(2, "B", "u2")}, 1, 1, intPtr(2)),
		searchResult(nil, 2, 1, intPtr(2)),
		searchResult([]sdk.SmartGroupSearchModelV1{sg(1, "A", "u1")}, 0, 1, intPtr(2)),
		searchResult([]sdk.SmartGroupSearchModelV1{sg(2, "B", "u2")}, 1, 1, intPtr(5)),
	}}
	d := &SmartGroupsDataSource{search: fs, pageSize: 1}

	_, err := d.listAllSearchRaw(context.Background(), fs, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "second walk") {
		t.Fatalf("expected the second walk's internal Total inconsistency to fail the overall call, got %v", err)
	}
	if len(fs.pagesRequested) != 5 {
		t.Fatalf("expected exactly 5 calls (3 for walk 1, 2 for walk 2 before it errors), got %d", len(fs.pagesRequested))
	}
}

func TestMetadataTypeName(t *testing.T) {
	t.Parallel()
	var resp datasource.MetadataResponse
	NewDataSource().Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "uem"}, &resp)
	if resp.TypeName != "uem_smart_groups" {
		t.Fatalf("TypeName = %q", resp.TypeName)
	}
}

// An empty page's Total must never replace the Total learned from a non-empty
// page: here page 1 is empty and claims Total=3, which would make the 3 groups
// already seen look complete and return a truncated list.
func TestListAllSearchRaw_EmptyPageTotalNeverOverridesNonEmptyTotal(t *testing.T) {
	t.Parallel()
	total5, total3 := 5, 3
	fs := &fakeSmartGroupSearch{pages: map[int]*sdk.SmartGroupSearchResultV1{
		0: searchResult([]sdk.SmartGroupSearchModelV1{sg(1, "A", "u1"), sg(2, "B", "u2"), sg(3, "C", "u3")}, 0, 3, &total5),
		1: searchResult(nil, 1, 3, &total3),
	}}
	d := &SmartGroupsDataSource{search: fs, pageSize: 3}

	got, err := d.listAllSearchRaw(context.Background(), fs, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "empty page") {
		t.Fatalf("expected an empty-page-before-Total-reached error, got %v (%d groups)", err, len(got))
	}
	if got != nil {
		t.Fatalf("expected a nil (not partial) result, got %d groups", len(got))
	}
}

// A non-numeric organization_group_id that reaches Read (e.g. unknown at
// plan time, so ValidateConfig could not check it) must fail, never fall back
// to an unfiltered search.
func TestRead_NonNumericOrgGroupID_ErrorsNotUnfiltered(t *testing.T) {
	t.Parallel()
	s := getDSSchema(t)
	total := 1
	fs := &fakeSmartGroupSearch{pages: map[int]*sdk.SmartGroupSearchResultV1{
		0: searchResult([]sdk.SmartGroupSearchModelV1{sg(1, "A", "u1")}, 0, 500, &total),
	}}
	d := &SmartGroupsDataSource{search: fs, loader: &fakeSmartGroupLoader{}}

	req := datasource.ReadRequest{Config: buildConfig(t, s, map[string]tftypes.Value{
		"organization_group_id": tftypes.NewValue(tftypes.String, "12x"),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(t)}
	d.Read(context.Background(), req, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected an error for a non-numeric organization_group_id at Read")
	}
	if fs.lastOpts != nil {
		t.Fatalf("expected no search call, got opts %+v", fs.lastOpts)
	}
}
