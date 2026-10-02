// Package organizationgroup implements the uem_organization_groups Terraform
// data source as a hand-written concept, matching the pattern already used by
// internal/application/purchased-app/purchased_applications_data_source.go:
// one self-contained file (schema + model structs + Metadata/Schema/
// Configure/ValidateConfig/Read), its own locally-scoped SDK interfaces, no
// generated code.
//
// Hand-written rather than gends-generated because this data source has
// FOUR mutually exclusive lookup modes backed by two different SDK calls
// (LocationGroupSearch and GetChildLocationGroups, the latter also used for
// GetParents) plus a per-result N+1 hydration fan-out (GetChildLocationGroups
// again) in the search mode -- the same class of shape the generator's
// one-call composition-map model cannot express, as already documented for
// uem_purchased_applications and uem_update_deployments.
//
// uem_organization_groups reads organization groups (aka location groups) via
// sdk.OrganizationGroupsService:
//
//   - search (default): LocationGroupSearch, optionally filtered by name
//     (substring), type, and/or group_id (exact match), paged 0-based at
//     PageSize 500. Search results come back WITHOUT uuid or parent
//     information, and GetAsync(id) cannot fill either: it returns no
//     ParentLocationGroup field at all and LgLevel 0 (live-verified). So
//     every matched group is instead hydrated with GetChildLocationGroups(id)
//     -- the same call parent_id mode uses -- whose result includes the
//     group itself alongside its descendants, and the self entry DOES carry
//     ParentLocationGroup/Uuid (live-verified). The self entry is picked out
//     by matching its numeric id against the one just searched. This means
//     search mode costs one extra children-lookup call per matched group on
//     top of the paged search itself, and for a high-level group that lookup
//     response includes its whole subtree, not just the one group being
//     hydrated. lg_level is set to null in this mode: the self entry's
//     LgLevel is relative to itself (always 0), which carries no information
//     about the group's real position in the hierarchy.
//   - id: exact lookup of one organization group by its numeric id. Backed by
//     the same GetChildLocationGroups(id) call as search-mode hydration and
//     parent_id mode; only the self entry (matched by numeric id, exactly as
//     search-mode hydration does) is kept, so `organization_groups` holds
//     exactly one entry with the same shape search mode produces (lg_level
//     null). A missing self entry, or any SDK error, fails the whole read.
//   - parent_id: GetChildLocationGroups(id). The result set INCLUDES the
//     parent group itself (self, LgLevel 0) plus every descendant
//     (live-verified) -- not just direct children.
//   - ancestors_of_uuid: GetParents(uuid). Returns only ancestor UUIDs (self
//     plus ancestors up to Global) as `ancestor_uuids`; `organization_groups`
//     is left empty in this mode because the SDK has no get-by-uuid
//     operation, so an ancestor UUID cannot be hydrated into a full
//     organization group entry.
package organizationgroup

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/euc-oss/terraform-provider-uem/internal/providerdata"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &organizationGroupsDataSource{}
var _ datasource.DataSourceWithConfigure = &organizationGroupsDataSource{}
var _ datasource.DataSourceWithValidateConfig = &organizationGroupsDataSource{}

// defaultSearchPageSize is the page size requested from LocationGroupSearch.
// 500 is also the server's own default page size (per the SDK's own
// documented default and live-verified behaviour).
const defaultSearchPageSize = 500

// maxSearchPages is a last-resort guard against an endless walk: a server
// that keeps advancing pages and keeps returning new organization groups but
// never runs out. It is not the defense against a server that ignores the
// page parameter; that is the per-page progress check in listAllSearch,
// which fails on a full page that adds no new id. Exceeding it is an error,
// never a truncated result; at pageSize=500 it allows 500,000 organization
// groups.
const maxSearchPages = 1000

// organizationGroupsSearchAPI is the narrow surface of
// sdk.OrganizationGroupsService's search operation this data source depends
// on, so tests can inject a fake without a real SDK client.
type organizationGroupsSearchAPI interface {
	LocationGroupSearch(ctx context.Context, opts *sdk.OrganizationGroupsLocationGroupSearchOptions) (http.Header, *sdk.LocationGroupSearchResultV1, error)
}

// organizationGroupsChildrenAPI is the narrow surface backing the parent_id
// (children) lookup mode.
type organizationGroupsChildrenAPI interface {
	GetChildLocationGroups(ctx context.Context, id int) (http.Header, *[]sdk.LocationGroupV1, error)
}

// organizationGroupsParentsAPI is the narrow surface backing the
// ancestors_of_uuid lookup mode.
type organizationGroupsParentsAPI interface {
	GetParents(ctx context.Context, uuid string) (http.Header, *sdk.OrganizationGroupCollectionV1Model, error)
}

type organizationGroupsDataSource struct {
	search   organizationGroupsSearchAPI
	children organizationGroupsChildrenAPI
	parents  organizationGroupsParentsAPI
	pageSize int // 0 means defaultSearchPageSize; overridden by tests
}

// NewDataSource is the constructor internal/provider/provider.go registers.
func NewDataSource() datasource.DataSource {
	return &organizationGroupsDataSource{}
}

// OrganizationGroupsDataSourceModel is the Terraform state model for
// uem_organization_groups.
type OrganizationGroupsDataSourceModel struct {
	// ID is a String, not an Int64, because the ws1-tf onboard generator
	// codes against a string-typed `id = "<numeric>"` attribute (maintainer
	// requirement, 2026-09-25); it is validated numeric at plan time (see
	// ValidateConfig) rather than being schema-typed as a number.
	ID                 types.String               `tfsdk:"id"`
	Name               types.String               `tfsdk:"name"`
	Type               types.String               `tfsdk:"type"`
	GroupID            types.String               `tfsdk:"group_id"`
	ParentID           types.Int64                `tfsdk:"parent_id"`
	AncestorsOfUUID    types.String               `tfsdk:"ancestors_of_uuid"`
	OrganizationGroups []OrganizationGroupSummary `tfsdk:"organization_groups"`
	AncestorUUIDs      []types.String             `tfsdk:"ancestor_uuids"`
}

// OrganizationGroupSummary is one organization group projected into
// Terraform state.
type OrganizationGroupSummary struct {
	ID         types.Int64  `tfsdk:"id"`
	UUID       types.String `tfsdk:"uuid"`
	Name       types.String `tfsdk:"name"`
	GroupID    types.String `tfsdk:"group_id"`
	Type       types.String `tfsdk:"type"`
	ParentID   types.Int64  `tfsdk:"parent_id"`
	ParentUUID types.String `tfsdk:"parent_uuid"`
	Country    types.String `tfsdk:"country"`
	Locale     types.String `tfsdk:"locale"`
	LgLevel    types.Int64  `tfsdk:"lg_level"`
}

func (d *organizationGroupsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_groups"
}

func (d *organizationGroupsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up organization groups (location groups) in four MUTUALLY EXCLUSIVE modes. " +
			"(1) Default/search: optionally filter by `name` (substring match), `type`, and/or `group_id` (exact match), " +
			"paging through every match. Search results come back without UUID or parent information, so EVERY matched " +
			"group is then hydrated with one additional children-lookup call (the same call `parent_id` mode uses) to " +
			"fill in `uuid`, `parent_id`, and `parent_uuid` from the self entry of that call's result " +
			"-- this mode costs one extra children-lookup call per matched group, on top of the paged search itself, " +
			"and for a high-level group that response includes its whole subtree, not just the one group being hydrated. " +
			"(2) `id`: exact lookup of one organization group by its numeric id, via the same children-lookup call, " +
			"keeping only the self entry -- `organization_groups` holds exactly that one entry (lg_level null, like " +
			"search mode); a missing self entry is an error. " +
			"(3) `parent_id`: lists a group's children; the result INCLUDES the parent group itself (self) plus every " +
			"descendant, not just direct children. No paging, no hydration. " +
			"(4) `ancestors_of_uuid`: lists a group's ancestors (self plus ancestors up to Global) as `ancestor_uuids` " +
			"only -- `organization_groups` is left empty in this mode, because the SDK has no get-by-uuid operation and " +
			"an ancestor UUID therefore cannot be hydrated into a full organization group entry.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Exact-id mode: numeric ID (as a string) of the single organization group to look " +
					"up. Must be numeric (validated at plan time). Mutually exclusive with the search filters " +
					"(`name`/`type`/`group_id`), `parent_id`, and `ancestors_of_uuid`. On a match, `organization_groups` " +
					"holds exactly that one entry (hydrated the same way search mode hydrates a match, with `lg_level` " +
					"null); a group that does not exist is an error.",
				Optional: true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Search mode only: substring filter on organization group name.",
				Optional:            true,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Search mode only: filter on organization group type (e.g. \"Container\", \"Customer\", \"Partner\").",
				Optional:            true,
			},
			"group_id": schema.StringAttribute{
				MarkdownDescription: "Search mode only: exact-match filter on the organization group identifier (activation code).",
				Optional:            true,
			},
			"parent_id": schema.Int64Attribute{
				MarkdownDescription: "Children mode: numeric ID of the organization group whose children (and itself) to list. " +
					"Mutually exclusive with the search filters (`name`/`type`/`group_id`) and with `ancestors_of_uuid`.",
				Optional: true,
			},
			"ancestors_of_uuid": schema.StringAttribute{
				MarkdownDescription: "Ancestors mode: UUID of the organization group whose ancestors to list (populates only " +
					"`ancestor_uuids`, not `organization_groups`). Mutually exclusive with the search filters " +
					"(`name`/`type`/`group_id`) and with `parent_id`.",
				Optional: true,
			},
			"organization_groups": schema.ListNestedAttribute{
				MarkdownDescription: "Organization groups found by the active mode (empty in `ancestors_of_uuid` mode).",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.Int64Attribute{
							MarkdownDescription: "Numeric organization group ID.",
							Computed:            true,
						},
						"uuid": schema.StringAttribute{
							MarkdownDescription: "Organization group UUID.",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Organization group name.",
							Computed:            true,
						},
						"group_id": schema.StringAttribute{
							MarkdownDescription: "Organization group identifier (activation code).",
							Computed:            true,
						},
						"type": schema.StringAttribute{
							MarkdownDescription: "Organization group type (e.g. \"Container\", \"Customer\", \"Partner\").",
							Computed:            true,
						},
						"parent_id": schema.Int64Attribute{
							MarkdownDescription: "Numeric ID of the parent organization group, or null if this group has none.",
							Computed:            true,
						},
						"parent_uuid": schema.StringAttribute{
							MarkdownDescription: "UUID of the parent organization group, or null if this group has none.",
							Computed:            true,
						},
						"country": schema.StringAttribute{
							MarkdownDescription: "Country associated with the organization group.",
							Computed:            true,
						},
						"locale": schema.StringAttribute{
							MarkdownDescription: "Locale associated with the organization group.",
							Computed:            true,
						},
						"lg_level": schema.Int64Attribute{
							MarkdownDescription: "Location group hierarchy level relative to the `parent_id` lookup root " +
								"(0 = self) in children mode; always null in search mode, since the only value available " +
								"there is the hydration lookup's self entry relative to itself (always 0), which carries " +
								"no information about the group's real position in the hierarchy.",
							Computed: true,
						},
					},
				},
			},
			"ancestor_uuids": schema.ListAttribute{
				MarkdownDescription: "Ancestors mode only: UUIDs of the organization group itself and its ancestors, up to Global.",
				Computed:            true,
				ElementType:         types.StringType,
			},
		},
	}
}

func (d *organizationGroupsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
	svc := sdk.NewOrganizationGroupsService(pd.Client)
	d.search = svc
	d.children = svc
	d.parents = svc
}

// isKnownNonEmptyString reports whether v is a non-null, non-unknown,
// non-empty string -- i.e. whether the caller actually configured it (an
// unknown value is treated as "not (yet) set", never as a conflict, so a
// value that depends on another not-yet-created resource does not
// spuriously trip the mutually-exclusive-mode check below).
func isKnownNonEmptyString(v types.String) bool {
	return !v.IsNull() && !v.IsUnknown() && v.ValueString() != ""
}

// isKnownInt64 reports whether v is a non-null, non-unknown Int64 -- see
// isKnownNonEmptyString for why unknown is treated as "not set".
func isKnownInt64(v types.Int64) bool {
	return !v.IsNull() && !v.IsUnknown()
}

// ValidateConfig rejects:
//   - a configuration that sets more than one of this data source's four
//     mutually exclusive lookup modes: the search filters (name/type/group_id,
//     treated as one mode), id, parent_id, and ancestors_of_uuid (setting none
//     of them is valid: an unfiltered search);
//   - a non-numeric id (id is String-typed, not Int64, because the ws1-tf
//     onboard generator codes against a string-typed `id = "<numeric>"`
//     attribute; its numeric shape is therefore validated here instead of by
//     the schema type).
func (d *organizationGroupsDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var config OrganizationGroupsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if isKnownNonEmptyString(config.ID) {
		if _, err := strconv.ParseInt(config.ID.ValueString(), 10, 64); err != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("id"),
				"Invalid Organization Group ID",
				fmt.Sprintf("id must be a numeric organization group id, got %q: %s", config.ID.ValueString(), err),
			)
		}
	}

	ancestorsSet := isKnownNonEmptyString(config.AncestorsOfUUID)
	parentSet := isKnownInt64(config.ParentID)
	idSet := isKnownNonEmptyString(config.ID)
	searchFilterSet := isKnownNonEmptyString(config.Name) || isKnownNonEmptyString(config.Type) || isKnownNonEmptyString(config.GroupID)

	var modes []string
	if searchFilterSet {
		modes = append(modes, "the search filters (name/type/group_id)")
	}
	if idSet {
		modes = append(modes, "id")
	}
	if parentSet {
		modes = append(modes, "parent_id")
	}
	if ancestorsSet {
		modes = append(modes, "ancestors_of_uuid")
	}

	if len(modes) > 1 {
		resp.Diagnostics.AddError(
			"Mutually Exclusive Organization Group Lookup Modes",
			fmt.Sprintf(
				"uem_organization_groups supports four mutually exclusive lookup modes: the search filters "+
					"(name/type/group_id), id, parent_id, and ancestors_of_uuid. This configuration set more than one: %s. "+
					"Remove all but one.",
				strings.Join(modes, ", "),
			),
		)
	}
}

func (d *organizationGroupsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config OrganizationGroupsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Initialised to EMPTY (non-nil) slices so a zero-result read stores
	// empty lists rather than null ones, which Terraform would drop from
	// `terraform output` entirely (same reasoning as uem_purchased_applications
	// / uem_update_deployments).
	config.OrganizationGroups = []OrganizationGroupSummary{}
	config.AncestorUUIDs = []types.String{}

	switch {
	case isKnownNonEmptyString(config.ID):
		// ValidateConfig has already rejected a non-numeric id at plan time;
		// this ParseInt is defensive (e.g. a direct Read call in a test that
		// bypasses ValidateConfig) rather than a path expected to fail live.
		id, err := strconv.ParseInt(config.ID.ValueString(), 10, 64)
		if err != nil {
			resp.Diagnostics.AddError("Invalid Organization Group ID", fmt.Sprintf("id must be numeric, got %q: %s", config.ID.ValueString(), err))
			return
		}
		summary, err := d.fetchSelfSummary(ctx, id)
		if err != nil {
			resp.Diagnostics.AddError("Organization Group Lookup Failed", err.Error())
			return
		}
		config.OrganizationGroups = []OrganizationGroupSummary{summary}

	case isKnownNonEmptyString(config.AncestorsOfUUID):
		uuid := config.AncestorsOfUUID.ValueString()
		_, coll, err := d.parents.GetParents(ctx, uuid)
		if err != nil {
			resp.Diagnostics.AddError("Organization Group Ancestor Lookup Failed", fmt.Sprintf("Unable to read ancestors of organization group %s: %s", uuid, err))
			return
		}
		if coll != nil {
			for _, u := range coll.Items {
				config.AncestorUUIDs = append(config.AncestorUUIDs, types.StringValue(u))
			}
		}

	case isKnownInt64(config.ParentID):
		parentID := config.ParentID.ValueInt64()
		_, children, err := d.children.GetChildLocationGroups(ctx, int(parentID))
		if err != nil {
			resp.Diagnostics.AddError("Organization Group Children Lookup Failed", fmt.Sprintf("Unable to read child organization groups of %d: %s", parentID, err))
			return
		}
		if children != nil {
			for _, lg := range *children {
				config.OrganizationGroups = append(config.OrganizationGroups, toSummary(lg))
			}
		}

	default:
		// B16 decision table row #203 (KEEP+cite). UEM source:
		// Database/AirWatchDB/AirWatchDB/LocationGroup/Stored Procedures/API_LocationGroupSearch.sql:144
		// (canonical Q31): the org-group search stored proc's name filter
		// only applies when @LocationGroupName IS NULL is false -- an absent
		// (SQL NULL) name matches every row, but an EXPLICIT EMPTY STRING is
		// NOT NULL either, so it also becomes a `LIKE '%%'` wildcard that
		// matches every row. Treating an empty configured name as "unset"
		// here (isKnownNonEmptyString) is behaviorally equivalent to sending
		// it through, even though the mechanism differs: this client
		// achieves "no filter" by omitting the parameter; the server would
		// have achieved the same "no filter" outcome by wildcard-matching
		// it. Q31's second half confirms the same equivalence for smart
		// group search's name filter.
		var namePtr, typePtr, groupIDPtr *string
		if isKnownNonEmptyString(config.Name) {
			v := config.Name.ValueString()
			namePtr = &v
		}
		if isKnownNonEmptyString(config.Type) {
			v := config.Type.ValueString()
			typePtr = &v
		}
		if isKnownNonEmptyString(config.GroupID) {
			v := config.GroupID.ValueString()
			groupIDPtr = &v
		}

		summaries, err := d.searchAndHydrate(ctx, namePtr, typePtr, groupIDPtr)
		if err != nil {
			resp.Diagnostics.AddError("Organization Group Search Failed", err.Error())
			return
		}
		config.OrganizationGroups = summaries
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// searchAndHydrate walks every page of LocationGroupSearch matching the given
// filters and then hydrates each result so uuid/parent_id/parent_uuid are
// populated (search results alone omit them, live-verified). GetAsync cannot
// fill them (it returns no ParentLocationGroup field at all, live-verified),
// so hydration instead calls GetChildLocationGroups(id) -- the same call
// parent_id mode uses -- whose result includes the group itself (the self
// entry, matched by numeric id) alongside its descendants, and the self
// entry DOES carry ParentLocationGroup/Uuid (live-verified). Hydration is
// fail-closed: a children-lookup error, a nil result, or a result with no
// self entry aborts the whole call (never a partial list), matching the "any
// SDK error -> whole read fails" rule for this data source. LgLevel from the
// self entry is discarded (it is relative to itself, always 0, and carries
// no real hierarchy information); search-mode results always report
// lg_level as null.
func (d *organizationGroupsDataSource) searchAndHydrate(ctx context.Context, name, ogType, groupID *string) ([]OrganizationGroupSummary, error) {
	raw, err := d.listAllSearch(ctx, name, ogType, groupID)
	if err != nil {
		return nil, err
	}
	out := make([]OrganizationGroupSummary, 0, len(raw))
	for _, lg := range raw {
		id, ok := locationGroupID(lg)
		if !ok {
			return nil, fmt.Errorf("organization group %q came back from search without a numeric id; cannot hydrate it", lg.Name)
		}
		summary, err := d.fetchSelfSummary(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, summary)
	}
	return out, nil
}

// fetchSelfSummary calls GetChildLocationGroups(id) and returns the self
// entry (matched by numeric id, never by list position -- see findSelf)
// projected into the Terraform state model, with lg_level forced null (the
// self entry's LgLevel is relative to itself, always 0, and carries no real
// hierarchy information). Used by both search-mode hydration
// (searchAndHydrate) and id mode. Fail-closed: an SDK error, a nil result, or
// a result with no self entry is reported as an error, never a zero-value
// summary.
func (d *organizationGroupsDataSource) fetchSelfSummary(ctx context.Context, id int64) (OrganizationGroupSummary, error) {
	_, children, err := d.children.GetChildLocationGroups(ctx, int(id))
	if err != nil {
		return OrganizationGroupSummary{}, fmt.Errorf("unable to read organization group %d: %w", id, err)
	}
	if children == nil {
		return OrganizationGroupSummary{}, fmt.Errorf("children lookup for organization group %d returned no data", id)
	}
	self, ok := findSelf(*children, id)
	if !ok {
		return OrganizationGroupSummary{}, fmt.Errorf("organization group %d not found in its own children lookup result", id)
	}
	summary := toSummary(self)
	summary.LgLevel = types.Int64Null()
	return summary, nil
}

// findSelf returns the entry in children whose numeric id equals id -- the
// self entry that GetChildLocationGroups includes alongside the group's
// descendants (live-verified) -- reporting false when no entry matches.
func findSelf(children []sdk.LocationGroupV1, id int64) (sdk.LocationGroupV1, bool) {
	for _, c := range children {
		if cid, ok := locationGroupID(c); ok && cid == id {
			return c, true
		}
	}
	return sdk.LocationGroupV1{}, false
}

// listAllSearch walks every page of LocationGroupSearch for the given
// filters and returns every matched organization group, in first-seen order.
// It follows the same rules, for the same reasons, as
// internal/scripts/scriptlookup.AbsentFromOrgGroup (see that function's doc
// comment for the full reasoning); this doc summarizes the adaptation from
// "is one scriptUUID absent" to "collect every organization group".
//
// Progress is counted in unique numeric ids, never in raw items per page, so
// a server that repeats a page, or a list that shifts under the walk, cannot
// reach Total through duplicates. A group with no numeric id is an error at
// once: without an id it can neither be deduplicated nor later hydrated, so
// there is no useful way to carry it forward.
//
// Total is the grand total only on pages that hold items. The live server
// answers a page past the end with no items, so an empty page never
// overwrites a total learned from an earlier non-empty page; an empty page 0
// (no earlier total) uses its own Total (nil is read as 0, meaning "no
// groups"). A nil Total on a NON-EMPTY page is an error: unlike an empty
// page, a page with items gives no other way to know how many groups exist
// in total. Per page, with n items, unique the distinct ids seen so far and
// total the last trusted Total:
//
//   - unique > total: error. More distinct groups exist than the server
//     claims, so Total cannot be trusted to bound the walk.
//   - unique == total and n < the effective page size (a short or empty
//     page): done. Every group the server counts has been seen, and the
//     server has signalled there is no more data.
//   - unique == total and a full page that reached total: fetch one more
//     page to confirm the end (a Total that is an exact multiple of the page
//     size is legitimate and ends on a full page).
//   - unique == total and a full page when total was already reached before
//     it: error. The server keeps returning full pages past the end.
//   - unique < total and an empty page: error (the list ran out early).
//   - unique < total and a page that added no new id: error. The server is
//     not advancing (for example ignoring the page parameter); continuing
//     would only re-read the same items.
//
// The effective page size for the "short page" check is the one the server
// echoes back in PageSize when present, falling back to the requested size.
//
// A list that changes during the walk shifts the offsets under it, so a
// later page can skip a group that still exists. This is guarded the same
// two ways as AbsentFromOrgGroup: once a non-empty page has set the total,
// every later non-empty page must report the same Total (a disagreement is
// an error); and a walk that fetched more than one page is repeated as a
// second, independent walk from page 0, requiring the same set of ids (same
// count, same ids) and the same total -- any disagreement, including an
// error in the second walk, is an error. A single-page walk (any filter
// matching at most one page of groups) makes exactly one server call and has
// no window for the list to shift between calls, so it is not repeated.
//
// The maxSearchPages cap is a last-resort guard against an endless walk;
// exceeding it is an error, never a truncated result.
func (d *organizationGroupsDataSource) listAllSearch(ctx context.Context, name, ogType, groupID *string) ([]sdk.LocationGroupV1, error) {
	pageSize := d.pageSize
	if pageSize <= 0 {
		pageSize = defaultSearchPageSize
	}
	out, total, pages, err := d.walkSearch(ctx, name, ogType, groupID, pageSize)
	if err != nil || pages <= 1 {
		return out, err
	}
	again, againTotal, _, err := d.walkSearch(ctx, name, ogType, groupID, pageSize)
	switch {
	case err != nil:
		return nil, fmt.Errorf("second walk of the organization group search disagrees with the first, which found %d groups: %w", len(out), err)
	case !sameIDSet(out, again):
		return nil, fmt.Errorf("organization group search changed between two walks: the first found %d groups, the second found %d with a different set of ids", len(out), len(again))
	case againTotal != total:
		return nil, fmt.Errorf("organization group search changed between two walks: Total was %d then %d; refusing to return a possibly inconsistent result", total, againTotal)
	}
	return out, nil
}

// walkSearch is one independent walk of the organization group search, from
// page 0, under the rules documented on listAllSearch. It returns the
// matched groups in first-seen order, the trusted total, and how many pages
// it fetched.
func (d *organizationGroupsDataSource) walkSearch(ctx context.Context, name, ogType, groupID *string, pageSize int) ([]sdk.LocationGroupV1, int, int, error) {
	var out []sdk.LocationGroupV1
	seen := make(map[int64]bool)
	reached := false
	total, haveTotal, totalPage := 0, false, -1
	for page := 0; page < maxSearchPages; page++ {
		fetched := page + 1
		p, ps := page, pageSize
		_, res, err := d.search.LocationGroupSearch(ctx, &sdk.OrganizationGroupsLocationGroupSearchOptions{
			Name:     name,
			Type:     ogType,
			Groupid:  groupID,
			Page:     &p,
			PageSize: &ps,
		})
		if err != nil {
			return nil, 0, fetched, fmt.Errorf("unable to search organization groups (page %d): %w", page, err)
		}
		if res == nil {
			res = &sdk.LocationGroupSearchResultV1{}
		}
		n := len(res.LocationGroups)

		switch {
		case n > 0 && res.Total == nil:
			return nil, 0, fetched, fmt.Errorf("organization group search response (page %d) has %d groups but is missing Total", page, n)
		case n > 0 && totalPage >= 0 && *res.Total != total:
			return nil, 0, fetched, fmt.Errorf("organization group search Total changed from %d to %d between page %d and page %d; the list changed during the walk", total, *res.Total, totalPage, page)
		case n > 0:
			total, haveTotal = *res.Total, true
			if totalPage < 0 {
				totalPage = page
			}
		case !haveTotal:
			t := 0
			if res.Total != nil {
				t = *res.Total
			}
			total, haveTotal = t, true
		}

		added := 0
		for _, g := range res.LocationGroups {
			id, ok := locationGroupID(g)
			if !ok {
				return nil, 0, fetched, fmt.Errorf("organization group %q came back from search without a numeric id; cannot page or hydrate it", g.Name)
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, g)
			added++
		}
		unique := len(seen)

		effective := pageSize
		if res.PageSize != nil && *res.PageSize > 0 {
			effective = *res.PageSize
		}

		switch {
		case unique > total:
			return nil, 0, fetched, fmt.Errorf("organization group search returned %d distinct groups by page %d but Total is %d", unique, page, total)
		case unique == total && n < effective:
			return out, total, fetched, nil
		case unique == total && reached:
			return nil, 0, fetched, fmt.Errorf("organization group search returned a full page %d after all %d groups were already seen", page, total)
		case unique == total:
			reached = true
		case n == 0:
			return nil, 0, fetched, fmt.Errorf("organization group search returned an empty page %d after %d of %d groups", page, unique, total)
		case added == 0:
			return nil, 0, fetched, fmt.Errorf("organization group search did not advance on page %d: page was full but contained only already-seen groups", page)
		}
	}
	return nil, 0, maxSearchPages, fmt.Errorf("organization group search walked %d pages (maxSearchPages) without reaching a short or empty page; aborting rather than returning a truncated result set", maxSearchPages)
}

// sameIDSet reports whether a and b contain exactly the same numeric ids
// (same count, same ids; order does not matter). Both slices are the output
// of walkSearch, so every entry is guaranteed to have a numeric id.
func sameIDSet(a, b []sdk.LocationGroupV1) bool {
	if len(a) != len(b) {
		return false
	}
	ids := make(map[int64]bool, len(a))
	for _, g := range a {
		id, _ := locationGroupID(g)
		ids[id] = true
	}
	for _, g := range b {
		id, _ := locationGroupID(g)
		if !ids[id] {
			return false
		}
	}
	return true
}

// locationGroupID extracts the numeric id from a LocationGroupV1, reporting
// false when the SDK did not populate one.
func locationGroupID(lg sdk.LocationGroupV1) (int64, bool) {
	if lg.ID != nil && lg.ID.Value != nil {
		return *lg.ID.Value, true
	}
	return 0, false
}

// toSummary projects an SDK LocationGroupV1 into the Terraform state model,
// leaving parent_id and parent_uuid null when the group has no parent (or the
// SDK omitted one), matching the schema's "null if none" contract.
//
// B16 decision table row #210 (KEEP+cite). UEM source:
// AirWatch API/AW.Core.Api/AW.Core.Api/Controllers/OrganizationGroups/OrganizationGroupsController.cs:560,
// AirWatch API/AirWatch.Data/Groups/RetrieveChildLg.cs:79 (canonical Q30): for
// `{id}/children` (the GetChildLocationGroups call backing parent_id mode and
// search-mode hydration, per the package doc above), ParentLocationGroup is
// excluded from the response entirely via `<excludeProperty>`; separately,
// the root row's ParentLocationGroupID==0 maps to a null ParentLocationGroup
// reference (not an empty-UUID object) rather than to zero-value fields.
// This corroborates the live-verified in-code evidence above (lines 21-27)
// specifically for the `/children` case; it does not newly confirm the
// search/self-hydration path Q30 itself.
func toSummary(lg sdk.LocationGroupV1) OrganizationGroupSummary {
	s := OrganizationGroupSummary{
		UUID:       types.StringValue(lg.UUID),
		Name:       types.StringValue(lg.Name),
		GroupID:    types.StringValue(lg.GroupID),
		Type:       types.StringValue(lg.LocationGroupType),
		Country:    types.StringValue(lg.Country),
		Locale:     types.StringValue(lg.Locale),
		ID:         types.Int64Null(),
		ParentID:   types.Int64Null(),
		ParentUUID: types.StringNull(),
		LgLevel:    types.Int64Null(),
	}
	if lg.ID != nil && lg.ID.Value != nil {
		s.ID = types.Int64Value(*lg.ID.Value)
	}
	if lg.LgLevel != nil {
		s.LgLevel = types.Int64Value(int64(*lg.LgLevel))
	}
	if p := lg.ParentLocationGroup; p != nil {
		if p.ID != nil && p.ID.Value != nil {
			s.ParentID = types.Int64Value(*p.ID.Value)
		}
		if p.UUID != "" {
			s.ParentUUID = types.StringValue(p.UUID)
		}
	}
	return s
}
