// Package smartgroup implements the uem_smart_groups Terraform data source.
//
// uem_smart_groups looks up smart groups in THREE modes, backed by two
// different SDK calls (SmartGroupsService.SearchAsync and
// SmartGroupsService.LoadSmartGroupAsync). The modes are no longer validated
// as mutually exclusive (B16 (b)-row #215 removal: that check was copied from
// uem_organization_groups and never independently live-proven for smart
// groups). Instead, when more than one mode's fields are set, Read applies
// them by PRECEDENCE, from most to least specific -- the same order the
// dispatch switch below already used before this change, now made the
// documented contract instead of an unreachable path:
//
//  1. smart_group_id, if set: exact lookup by id. Any set name,
//     organization_group_id, or smart_group_uuid is ignored.
//  2. smart_group_uuid, if set (and smart_group_id is not): exact lookup by
//     uuid via an unfiltered search walk. Any set name or
//     organization_group_id is ignored.
//  3. Otherwise: the search filters (name/organization_group_id), applied
//     together (AND) when both are set, or unfiltered when neither is set.
//
// The three underlying operations:
//
//   - search (default): SearchAsync, optionally filtered by name (substring)
//     and/or organization_group_id (exact match), at PageSize 500. The
//     page-index BASE is PROBED per call (never hard-coded, never cached
//     across calls/tenants) and the walk is hardened via internal/pagewalk
//     (internal-ticket, release blocker B12), the same shared implementation
//     used by uem_scripts and uem_applications/uem_mac_applications:
//     progress is counted in unique numeric SmartGroupIDs, Total is trusted
//     only from non-empty pages and must not change between them, the walk
//     stops only on a short/empty page with unique == total (fetching one
//     confirming page when total is reached on a full page), and a
//     multi-page result is walked twice and the two id sets, totals and
//     bases compared. This replaces a prior hard-coded page-base-0 walk that
//     was copied from the org-groups data source and never independently
//     verified live against the SmartGroups search endpoint.
//
//     Live-confirmed (internal-ticket): GET /api/mdm/smartgroups/search is
//     0-indexed on both tenant as<internal-env> (UEM 26.2) and tenant paul-2609 (UEM
//     26.9), pagesize is honoured exactly (verified at pagesize 2 and 500),
//     and an out-of-range page returns HTTP 204 (a nil result) on both.
//     Total is reported under the "Total" field and is constant across
//     pages on both tenants.
//
//     B16 decision table row #218 (KEEP+cite): the canonical batch's Q29
//     (audit-local paging question for this exact hardening) is one of the
//     three missing sections in that batch (Q5/Q29/Q32 never got written),
//     but this in-code live-confirmed evidence above already closes that
//     gap independently -- no further action needed pending Q29 ever being
//     answered.
//
//   - smart_group_id: LoadSmartGroupAsync(id) (GET /api/mdm/smartgroups/{id}),
//     returning exactly that one group. A not-found answer (client.IsNotFound
//     recognises UEM's HTTP 400 "Smart Group not found with Id: <id> or User
//     does not have access to it." shape for this endpoint, alongside a
//     generic 404) is reported as an EMPTY list, not an error -- the same
//     "no match" contract a name/organization_group_id filter has. Any other
//     error is a real error.
//
//   - smart_group_uuid: the search API has no uuid query parameter, so this
//     mode walks the same hardened search unfiltered and keeps only exact,
//     case-insensitive uuid matches. smart_group_uuid is no longer validated
//     against a UUID shape at plan time (B16 (b)-row #214 removal: that regex
//     asserted a shape with no evidence, and the API has no uuid parameter to
//     reject anyway); a malformed value simply matches nothing, the same as
//     any other value with no match, and Read returns an empty list.
package smartgroup

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/euc-oss/terraform-provider-uem/internal/providerdata"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/euc-oss/terraform-sdk-uem/v26/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &SmartGroupsDataSource{}
var _ datasource.DataSourceWithConfigure = &SmartGroupsDataSource{}
var _ datasource.DataSourceWithValidateConfig = &SmartGroupsDataSource{}

// defaultSmartGroupSearchPageSize is the page size requested from
// SearchAsync. 500 matches the convention already confirmed live for the
// organization group and sensor search endpoints, and is honoured exactly by
// the SmartGroups search endpoint (live-confirmed, internal-ticket: the server
// echoes back the requested PageSize on both as<internal-env> and paul-2609).
const defaultSmartGroupSearchPageSize = 500

// maxSmartGroupSearchPages is a last-resort guard against an endless walk: a
// server that keeps advancing pages and keeps returning new smart groups but
// never runs out. It is not the defense against a server that ignores the
// page parameter; that is internal/pagewalk's per-page progress check, which
// fails on a full page that adds no new id. Exceeding it is an error, never a
// truncated result.
const maxSmartGroupSearchPages = 1000

// smartGroupSearchAPI is the narrow surface of SmartGroupsService's search
// operation this data source depends on, so tests can inject a fake without a
// real SDK client (mirroring organizationGroupsSearchAPI).
type smartGroupSearchAPI interface {
	SearchAsync(ctx context.Context, opts *sdk.SmartGroupsSearchAsyncOptions) (http.Header, *sdk.SmartGroupSearchResultV1, error)
}

// smartGroupLoadAPI is the narrow surface backing the smart_group_id exact
// lookup mode.
type smartGroupLoadAPI interface {
	LoadSmartGroupAsync(ctx context.Context, id int) (http.Header, *sdk.SmartGroupV1, error)
}

func NewDataSource() datasource.DataSource {
	return &SmartGroupsDataSource{}
}

type SmartGroupsDataSource struct {
	client   *sdk.Client
	search   smartGroupSearchAPI
	loader   smartGroupLoadAPI
	pageSize int // 0 means defaultSmartGroupSearchPageSize; overridden by tests
}

func (d *SmartGroupsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_smart_groups"
}

func (d *SmartGroupsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up smart groups in Workspace ONE UEM in three modes, applied by PRECEDENCE " +
			"when more than one is set (B16 (b)-row #215 removal: the modes are no longer rejected as mutually " +
			"exclusive). From most to least specific: " +
			"(1) `smart_group_id`: exact lookup of one smart group by its numeric id; a smart group that does not " +
			"exist returns an EMPTY list, not an error. Sending any other filter alongside it has no effect. " +
			"(2) `smart_group_uuid` (used when `smart_group_id` is not set): exact lookup of one smart group by " +
			"its UUID (the search API has no uuid parameter, so this walks the full search and keeps only exact, " +
			"case-insensitive matches; a malformed value simply matches nothing). Sending `name`/" +
			"`organization_group_id` alongside it has no effect. " +
			"(3) Otherwise: `name` (substring match) and/or `organization_group_id` (exact match, must be " +
			"numeric) narrow the search, applied together (AND) when both are set, or unfiltered when neither " +
			"is set.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Search mode only: filter smart groups by name (substring match). Ignored " +
					"when `smart_group_id` or `smart_group_uuid` is set (see the data source description for " +
					"precedence).",
				Optional: true,
			},
			"organization_group_id": schema.StringAttribute{
				MarkdownDescription: "Search mode only: filter smart groups by organization group ID. Must be " +
					"numeric (validated at plan time). Ignored when `smart_group_id` or `smart_group_uuid` is set " +
					"(see the data source description for precedence).",
				Optional: true,
			},
			"smart_group_id": schema.Int64Attribute{
				MarkdownDescription: "Exact-id mode: numeric smart group identifier to look up directly " +
					"(GET /api/mdm/smartgroups/{id}). A smart group that does not exist returns an EMPTY " +
					"`smart_groups` list, not an error. Takes precedence over the search filters " +
					"(`name`/`organization_group_id`) and over `smart_group_uuid` when more than one is set.",
				Optional: true,
			},
			"smart_group_uuid": schema.StringAttribute{
				MarkdownDescription: "Exact-uuid mode: UUID of the smart group to look up. The search API has no " +
					"uuid parameter, so this mode walks the full search and keeps only an exact, case-insensitive " +
					"match; a malformed value is not rejected at plan time, it simply matches nothing (B16 " +
					"(b)-row #214 removal). Takes precedence over the search filters (`name`/" +
					"`organization_group_id`) but not over `smart_group_id` when more than one is set." +
					" This mode cannot see an organization group's own smart group (the one named after the organization group, used when assigning to the whole group): UEM's search omits it and UEM has no by-UUID read (live-confirmed). Look such a group up with `smart_group_id` instead.",
				Optional: true,
			},
			"smart_groups": schema.ListNestedAttribute{
				MarkdownDescription: "List of smart groups matching the active mode (at most one entry in " +
					"`smart_group_id`/`smart_group_uuid` mode).",
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"smart_group_id": schema.Int64Attribute{
							MarkdownDescription: "Smart group identifier",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Smart group name",
							Computed:            true,
						},
						"smart_group_uuid": schema.StringAttribute{
							MarkdownDescription: "Smart group UUID",
							Computed:            true,
						},
						"managed_by_org_group_name": schema.StringAttribute{
							MarkdownDescription: "Name of the organization group that manages this smart group",
							Computed:            true,
						},
						"managed_by_org_group_id": schema.StringAttribute{
							MarkdownDescription: "Numeric id of the organization group that manages this smart group",
							Computed:            true,
						},
						"managed_by_org_group_uuid": schema.StringAttribute{
							MarkdownDescription: "UUID of the organization group that manages this smart group",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (d *SmartGroupsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*providerdata.ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Configure Type", fmt.Sprintf("Expected *providerdata.ProviderData, got: %T", req.ProviderData))
		return
	}
	if pd == nil || pd.Client == nil {
		resp.Diagnostics.AddError("Provider Not Fully Configured", "The provider's data holder or its client is nil; this indicates a bug in provider Configure().")
		return
	}
	d.client = pd.Client
	svc := sdk.NewSmartGroupsService(pd.Client)
	d.search = svc
	d.loader = svc
}

// isKnownNonEmptyString reports whether v is a non-null, non-unknown,
// non-empty string -- i.e. whether the caller actually configured it (an
// unknown value is treated as "not (yet) set", never as a conflict), matching
// internal/organizationgroup's identically-named helper.
func isKnownNonEmptyString(v types.String) bool {
	return !v.IsNull() && !v.IsUnknown() && v.ValueString() != ""
}

// isKnownInt64 reports whether v is a non-null, non-unknown Int64 -- see
// isKnownNonEmptyString for why unknown is treated as "not set".
func isKnownInt64(v types.Int64) bool {
	return !v.IsNull() && !v.IsUnknown()
}

// ValidateConfig rejects a non-numeric organization_group_id (previously
// silently dropped by Read's strconv.Atoi error check, so a typo'd filter
// silently searched unfiltered instead of failing).
//
// It no longer rejects setting more than one lookup mode, and no longer
// validates smart_group_uuid against a UUID shape (B16 (b)-rows #214/#215
// removal: both checks were copied from uem_organization_groups and never
// independently live-proven for smart groups). See the package doc comment
// and the smart_groups data source's MarkdownDescription for the precedence
// Read now applies when more than one mode is set, and for what a malformed
// smart_group_uuid does (matches nothing, rather than failing at plan time).
func (d *SmartGroupsDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var config SmartGroupsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if isKnownNonEmptyString(config.OrganizationGroupID) {
		if _, err := strconv.Atoi(config.OrganizationGroupID.ValueString()); err != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("organization_group_id"),
				"Invalid Organization Group ID",
				fmt.Sprintf("organization_group_id must be a numeric organization group id, got %q: %s",
					config.OrganizationGroupID.ValueString(), err),
			)
		}
	}
}

func (d *SmartGroupsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config SmartGroupsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	search, loader := d.services()

	// Initialised to an EMPTY (non-nil) slice so a zero-result read stores a
	// known empty list rather than a null one (same reasoning as
	// uem_organization_groups/uem_purchased_applications/uem_update_deployments).
	config.SmartGroups = []SmartGroupModel{}

	switch {
	case isKnownInt64(config.SmartGroupID):
		id := config.SmartGroupID.ValueInt64()
		item, found, err := d.loadByID(ctx, loader, id)
		if err != nil {
			resp.Diagnostics.AddError("Smart Group Lookup Failed", fmt.Sprintf("Unable to read smart group %d: %s", id, err))
			return
		}
		if found {
			config.SmartGroups = []SmartGroupModel{item}
		}

	case isKnownNonEmptyString(config.SmartGroupUUID):
		uuid := config.SmartGroupUUID.ValueString()
		matches, err := d.searchByUUID(ctx, search, uuid)
		if err != nil {
			resp.Diagnostics.AddError("Smart Group Search Failed", err.Error())
			return
		}
		config.SmartGroups = matches

	default:
		var namePtr *string
		var orgGroupIDPtr *int
		if isKnownNonEmptyString(config.Name) {
			v := config.Name.ValueString()
			namePtr = &v
		}
		if isKnownNonEmptyString(config.OrganizationGroupID) {
			// ValidateConfig has already rejected a non-numeric value at plan
			// time, so this Atoi cannot fail in practice; the zero-value
			// fallback on error is defensive only.
			// ValidateConfig rejects a non-numeric value at plan time, but only
			// when the value is known then; one that was unknown at plan time is
			// checked here and fails rather than silently running an unfiltered
			// (broad) search.
			id, err := strconv.Atoi(config.OrganizationGroupID.ValueString())
			if err != nil {
				resp.Diagnostics.AddAttributeError(
					path.Root("organization_group_id"),
					"Invalid Organization Group ID",
					fmt.Sprintf("organization_group_id must be a numeric organization group id, got %q: %s",
						config.OrganizationGroupID.ValueString(), err),
				)
				return
			}
			orgGroupIDPtr = &id
		}
		items, err := d.listAllSearch(ctx, search, namePtr, orgGroupIDPtr)
		if err != nil {
			resp.Diagnostics.AddError("Smart Group Search Failed", err.Error())
			return
		}
		config.SmartGroups = items
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// services returns the search/load API to use for a Read: the injected test
// doubles if set (d.search/d.loader, set directly by tests or by Configure),
// falling back to a service constructed from d.client. This lets tests that
// only ever set d.client (as internal/smartgroup's original tests did)
// continue to work unchanged, while paging/lookup-hardening tests can inject
// fakes directly, mirroring organizationGroupsDataSource's Configure pattern.
func (d *SmartGroupsDataSource) services() (smartGroupSearchAPI, smartGroupLoadAPI) {
	search, loader := d.search, d.loader
	if search == nil || loader == nil {
		svc := sdk.NewSmartGroupsService(d.client)
		if search == nil {
			search = svc
		}
		if loader == nil {
			loader = svc
		}
	}
	return search, loader
}

// loadByID calls LoadSmartGroupAsync(id) and projects the result into the
// item model. A not-found answer (client.IsNotFound, which recognises UEM's
// HTTP 400 "Smart Group not found with Id: <id> or User does not have access
// to it." shape for this endpoint, alongside a generic 404) is reported as
// (zero value, false, nil) -- an empty result, never an error -- because a
// smart_group_id filter naming an id that does not exist is exactly like a
// search filter that matches nothing. Any other error is a real error.
func (d *SmartGroupsDataSource) loadByID(ctx context.Context, loader smartGroupLoadAPI, id int64) (SmartGroupModel, bool, error) {
	_, sg, err := loader.LoadSmartGroupAsync(ctx, int(id))
	if err != nil {
		if client.IsNotFound(err) {
			return SmartGroupModel{}, false, nil
		}
		return SmartGroupModel{}, false, err
	}
	if sg == nil {
		return SmartGroupModel{}, false, fmt.Errorf("smart group %d lookup returned no data", id)
	}
	return toItemFromDetail(*sg), true, nil
}

// searchByUUID walks the hardened smart group search (unfiltered: uuid mode
// is mutually exclusive with name/organization_group_id, see ValidateConfig)
// and keeps only exact, case-insensitive uuid matches. There is normally at
// most one match (UUIDs are unique), but this returns every match found
// rather than assuming that, since nothing here enforces server-side
// uniqueness.
func (d *SmartGroupsDataSource) searchByUUID(ctx context.Context, search smartGroupSearchAPI, uuid string) ([]SmartGroupModel, error) {
	raw, err := findSmartGroupsByUUID(ctx, search, uuid, d.pageSize)
	if err != nil {
		return nil, err
	}
	out := []SmartGroupModel{}
	for _, sg := range raw {
		out = append(out, toItemFromSearch(sg))
	}
	return out, nil
}

// listAllSearch walks every page of the smart group search for the given
// filters and projects each result into the item model.
func (d *SmartGroupsDataSource) listAllSearch(ctx context.Context, search smartGroupSearchAPI, name *string, orgGroupID *int) ([]SmartGroupModel, error) {
	raw, err := d.listAllSearchRaw(ctx, search, name, orgGroupID)
	if err != nil {
		return nil, err
	}
	out := make([]SmartGroupModel, 0, len(raw))
	for _, sg := range raw {
		out = append(out, toItemFromSearch(sg))
	}
	return out, nil
}

// listAllSearchRaw walks every page of the smart group search for the given
// filters via the shared hardened walk (searchAllSmartGroups in search.go,
// itself built on internal/pagewalk.WalkTwiceVerified -- internal-ticket,
// release blocker B12), at this data source's page size.
func (d *SmartGroupsDataSource) listAllSearchRaw(ctx context.Context, search smartGroupSearchAPI, name *string, orgGroupID *int) ([]sdk.SmartGroupSearchModelV1, error) {
	return searchAllSmartGroups(ctx, search, name, orgGroupID, d.pageSize)
}

// toItemFromSearch projects one SearchAsync result row into the item model.
func toItemFromSearch(sg sdk.SmartGroupSearchModelV1) SmartGroupModel {
	item := SmartGroupModel{
		Name:                  types.StringValue(sg.Name),
		SmartGroupUUID:        types.StringValue(sg.SmartGroupUUID),
		ManagedByOrgGroupName: types.StringValue(sg.ManagedByOrganizationGroupName),
		ManagedByOrgGroupID:   types.StringValue(sg.ManagedByOrganizationGroupID),
		ManagedByOrgGroupUUID: types.StringValue(sg.ManagedByOrganizationGroupUUID),
		SmartGroupID:          types.Int64Null(),
	}
	if sg.SmartGroupID != nil {
		item.SmartGroupID = types.Int64Value(int64(*sg.SmartGroupID))
	}
	return item
}

// toItemFromDetail projects a LoadSmartGroupAsync result into the item model.
func toItemFromDetail(sg sdk.SmartGroupV1) SmartGroupModel {
	item := SmartGroupModel{
		Name:                  types.StringValue(sg.Name),
		SmartGroupUUID:        types.StringValue(sg.SmartGroupUUID),
		ManagedByOrgGroupName: types.StringValue(sg.ManagedByOrganizationGroupName),
		ManagedByOrgGroupID:   types.StringValue(sg.ManagedByOrganizationGroupID),
		ManagedByOrgGroupUUID: types.StringValue(sg.ManagedByOrganizationGroupUUID),
		SmartGroupID:          types.Int64Null(),
	}
	if sg.SmartGroupID != nil {
		item.SmartGroupID = types.Int64Value(int64(*sg.SmartGroupID))
	}
	return item
}
