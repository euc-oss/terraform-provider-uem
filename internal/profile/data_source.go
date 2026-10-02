package profile

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	profileplatform "github.com/euc-oss/terraform-provider-uem/internal/profile/platform"
	"github.com/euc-oss/terraform-provider-uem/internal/providerdata"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &ProfilesDataSource{}
var _ datasource.DataSourceWithConfigure = &ProfilesDataSource{}

// profileSearchPageSize is the page size requested on every call to
// SearchProfiles. Mirrors internal/smartgroup/data_source.go's existing
// paginated-search convention (pageSize := 500) so all hand-written
// paginated data sources in this codebase request the same page size.
const profileSearchPageSize = 500

// maxProfilePages bounds the pagination loop in Read. At pageSize=500 this
// allows walking up to 50,000 profiles before erroring out — far beyond any
// real tenant's profile count — and exists purely as a termination guard
// against a pathological/misbehaving API response (one that never returns a
// short/empty page and never reports a TotalResults), not as a limit
// expected to be hit in practice. Mirrors
// internal/updates/update_deployments_data_source.go's maxCatalogPages
// guard.
const maxProfilePages = 100

// profilesV2SearchAPI is the narrow surface of sdk.ProfilesV2Service this data
// source depends on, so tests can inject a fake without a real SDK client.
type profilesV2SearchAPI interface {
	SearchProfiles(ctx context.Context, opts *sdk.ProfilesV2SearchProfilesOptions) (http.Header, *sdk.ProfileSearchResultV2Entity, error)
}

func NewDataSource() datasource.DataSource {
	return &ProfilesDataSource{}
}

// ProfilesDataSource implements the uem_profiles Terraform data source.
type ProfilesDataSource struct {
	search profilesV2SearchAPI

	// maxPages, when non-zero, overrides maxProfilePages for this instance.
	// It exists purely so tests can inject a tiny cap and prove the
	// termination guard fires (and does so in well under a second) against a
	// pathological fake that never signals end-of-results, without waiting
	// out the full production cap of 100 iterations. Production code always
	// leaves this unset (zero value) and falls back to maxProfilePages.
	maxPages int
}

// ProfilesDataSourceModel is the Terraform state model for uem_profiles.
type ProfilesDataSourceModel struct {
	Name                types.String     `tfsdk:"name"`
	OrganizationGroupID types.String     `tfsdk:"organization_group_id"`
	Profiles            []ProfileSummary `tfsdk:"profiles"`
}

// ProfileSummary is one entry in the profiles list returned by uem_profiles.
type ProfileSummary struct {
	ProfileID             types.Int64  `tfsdk:"profile_id"`
	Name                  types.String `tfsdk:"name"`
	Platform              types.String `tfsdk:"platform"`
	OrganizationGroupUUID types.String `tfsdk:"organization_group_uuid"`
}

func (d *ProfilesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_profiles"
}

func (d *ProfilesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Searches for profiles in Workspace ONE UEM. " +
			"Use this data source to look up profile IDs and their platform/organization group. " +
			"Results can include profiles owned by ancestor and descendant org groups, not only the queried org group; " +
			"use each item's organization_group_uuid to identify its owner.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Filter profiles by name (search text match)",
				Optional:            true,
			},
			"organization_group_id": schema.StringAttribute{
				MarkdownDescription: "Filter profiles by organization group ID. " +
					"Results can include profiles owned by ancestor and descendant org groups, not only the queried org group; " +
					"use each item's organization_group_uuid to identify its owner.",
				Optional: true,
			},
			"profiles": schema.ListNestedAttribute{
				MarkdownDescription: "List of profiles matching the filter criteria",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"profile_id": schema.Int64Attribute{
							MarkdownDescription: "Profile identifier",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Profile name",
							Computed:            true,
						},
						"platform": schema.StringAttribute{
							MarkdownDescription: "Profile platform",
							Computed:            true,
						},
						"organization_group_uuid": schema.StringAttribute{
							MarkdownDescription: "UUID of the organization group that owns this profile",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (d *ProfilesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
	d.search = sdk.NewProfilesV2Service(pd.Client)
}

func (d *ProfilesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ProfilesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	opts := &sdk.ProfilesV2SearchProfilesOptions{}
	if !config.Name.IsNull() && !config.Name.IsUnknown() {
		name := config.Name.ValueString()
		opts.SearchText = &name
	}
	if !config.OrganizationGroupID.IsNull() && !config.OrganizationGroupID.IsUnknown() {
		id, err := strconv.Atoi(config.OrganizationGroupID.ValueString())
		if err != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("organization_group_id"),
				"Invalid Organization Group ID",
				fmt.Sprintf("expected a numeric ID, got %q: %s", config.OrganizationGroupID.ValueString(), err),
			)
			return
		}
		opts.OrganizationGroupID = &id
	}

	// Paginated fetch. SearchProfiles paginates via Page/PageSize request
	// fields + ProfileList/TotalResults response fields (no cursor). The
	// page-index BASE differs by UEM version: live-confirmed 1-INDEXED on
	// tenant as<internal-env> (UEM 26.2) but 0-INDEXED on tenant paul-2609 (UEM
	// 26.9) — page=0/1/2 at pagesize=200 there return 200/200/98 distinct
	// items, and page=1 at pagesize=500 returns an HTTP 204 with an empty
	// body once all results fit on page 0. Hard-coding either base breaks
	// the other version: hard-coding 1 reads zero results on 26.9 (internal-ticket,
	// release blocker B1), and — with more than 500 profiles on a 26.9 tenant — would
	// silently skip page 0 forever. The base is therefore PROBED per read
	// (never cached across reads/tenants) and the walk is hardened the same
	// way as internal/organizationgroup's and internal/smartgroup's search
	// walks (in turn mirroring internal/scripts/scriptlookup.AbsentFromOrgGroup):
	// progress counted in unique ProfileIDs, TotalResults trusted only from
	// non-empty pages and never allowed to change between them, no-progress
	// and page-cap are errors, and a multi-page result is walked twice
	// end-to-end as an independent confirmation. See listAllProfiles and
	// probeProfilePageBase for the details.
	profiles, err := d.listAllProfiles(ctx, opts)
	if err != nil {
		resp.Diagnostics.AddError("Profile Search Failed", err.Error())
		return
	}

	config.Profiles = profiles
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// listAllProfiles walks every page of the profile search for opts (name/
// organization_group_id already applied by the caller) and returns every
// matched profile, in first-seen order. A single-page result (the common
// case) is walked once; a result spanning more than one page is walked a
// SECOND, fully independent time (including its own page-base probe) and the
// two outcomes — the set of profile ids, the trusted TotalResults, and the
// detected page-index base — must agree, exactly mirroring
// internal/organizationgroup.(*organizationGroupsDataSource).listAllSearch
// and internal/smartgroup.(*SmartGroupsDataSource).listAllSearchRaw. This
// guards against a list that shifts (or a page-index base that somehow
// differs) between the two walks; a single-page walk makes exactly one round
// trip (plus its own probe) and has no window for that to happen, so it is
// not repeated.
func (d *ProfilesDataSource) listAllProfiles(ctx context.Context, opts *sdk.ProfilesV2SearchProfilesOptions) ([]ProfileSummary, error) {
	pageSize := profileSearchPageSize
	maxPages := maxProfilePages
	if d.maxPages > 0 {
		maxPages = d.maxPages
	}

	out, total, base, pages, err := d.walkProfiles(ctx, opts, pageSize, maxPages)
	if err != nil || pages <= 1 {
		return out, err
	}
	again, againTotal, againBase, _, err := d.walkProfiles(ctx, opts, pageSize, maxPages)
	switch {
	case err != nil:
		return nil, fmt.Errorf("second walk of the profile search disagrees with the first, which found %d profiles: %w", len(out), err)
	case !sameProfileIDSet(out, again):
		return nil, fmt.Errorf("profile search changed between two walks: the first found %d profiles, the second found %d with a different set of ids", len(out), len(again))
	case againTotal != total:
		return nil, fmt.Errorf("profile search changed between two walks: TotalResults was %d then %d; refusing to return a possibly inconsistent result", total, againTotal)
	case againBase != base:
		return nil, fmt.Errorf("profile search page-index base changed between two walks: %d then %d; refusing to return a possibly inconsistent result", base, againBase)
	}
	return out, nil
}

// walkProfiles is one independent walk of the profile search: it first probes
// the page-index base (see probeProfilePageBase), then pages forward from
// that base under the same hardened rules as
// internal/organizationgroup.(*organizationGroupsDataSource).walkSearch (see
// that function's doc comment for the full reasoning; this doc summarizes the
// adaptation to profiles). It returns the matched profiles in first-seen
// order, the trusted TotalResults, the detected base, and how many pages it
// fetched (including the probed first page, but not a second, empty probe of
// the OTHER base when the search is confirmed to have zero results).
//
// Progress is counted in unique numeric ProfileIDs, never in raw items per
// page, so a server that repeats a page, or a list that shifts under the
// walk, cannot reach TotalResults through duplicates. A profile with no
// numeric id is an error at once: without an id it can neither be deduped nor
// trusted to advance the walk.
//
// TotalResults is the grand total only on pages that hold items; an empty
// page never overwrites a total learned from an earlier non-empty page, and a
// non-empty page with a nil TotalResults is an error. Per page, with n items,
// unique the distinct ids seen so far and total the last trusted
// TotalResults:
//
//   - unique > total: error — more distinct profiles exist than the server
//     claims, so TotalResults cannot be trusted to bound the walk.
//   - unique == total and n < pageSize (a short or empty page): done — every
//     profile the server counts has been seen, and the server has signalled
//     there is no more data. The walk NEVER stops on the total alone; it
//     always requires this short/empty page too.
//   - unique == total and a full page that just reached total: fetch one
//     more page to confirm the end (a total that is an exact multiple of
//     pageSize is legitimate and ends on a full page — this is the shape
//     behind the live "page 1 at pagesize=500 returns HTTP 204" evidence on a
//     26.9 tenant whose profile count is itself pageSize-aligned).
//   - unique == total and a full page when total was already reached before
//     it: error — the server keeps returning full pages past the end.
//   - unique < total and an empty page: error — the list ran out early.
//   - unique < total and a page that added no new id: error — the server is
//     not advancing (for example ignoring the page parameter); continuing
//     would only re-read the same items.
//
// The maxPages cap is a last-resort guard against an endless walk; exceeding
// it is an error, never a truncated result.
func (d *ProfilesDataSource) walkProfiles(ctx context.Context, opts *sdk.ProfilesV2SearchProfilesOptions, pageSize, maxPages int) ([]ProfileSummary, int, int, int, error) {
	probeOpts := *opts
	base, firstList, firstTotal, err := probeProfilePageBase(ctx, d.search, &probeOpts, pageSize)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	if firstList == nil {
		// Confirmed zero results (see probeProfilePageBase): nothing to page
		// through. The reported base is meaningless here (there is no data to
		// have a base), but is still returned for the caller's bookkeeping.
		return []ProfileSummary{}, 0, base, 1, nil
	}

	var out []ProfileSummary
	seen := make(map[int]bool)
	reached := false
	total, haveTotal, totalPage := 0, false, -1

	// applyPage folds one page's results (already fetched) into the walk
	// state and reports whether the walk is done, or an error.
	applyPage := func(page int, list []sdk.ProfileDetailsV2Entity, totalResults *int) (bool, error) {
		n := len(list)
		switch {
		case n > 0 && totalResults == nil:
			return false, fmt.Errorf("profile search response (page %d) has %d profiles but is missing TotalResults", page, n)
		case n > 0 && totalPage >= 0 && *totalResults != total:
			return false, fmt.Errorf("profile search TotalResults changed from %d to %d between page %d and page %d; the list changed during the walk", total, *totalResults, totalPage, page)
		case n > 0:
			total, haveTotal = *totalResults, true
			if totalPage < 0 {
				totalPage = page
			}
		case !haveTotal:
			t := 0
			if totalResults != nil {
				t = *totalResults
			}
			total, haveTotal = t, true
		}

		added := 0
		for _, p := range list {
			if p.ProfileID == nil {
				return false, fmt.Errorf("profile %q came back from search without a numeric profile id; cannot page or dedupe it", p.ProfileName)
			}
			id := *p.ProfileID
			if seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, ProfileSummary{
				Name: types.StringValue(p.ProfileName),
				// p.Platform is short-form as returned by the search v2 endpoint
				// (e.g. "Apple", "WinRT"); normalize to long-form so the emitted
				// value passes ImportState's IsValidImportPlatform check unchanged.
				// See internal-ticket and internal/profile/platform/platform.go.
				Platform:              types.StringValue(profileplatform.NormalizeAPIPlatform(p.Platform)),
				OrganizationGroupUUID: types.StringValue(p.OrganizationGroupUUID),
				ProfileID:             types.Int64Value(int64(id)),
			})
			added++
		}
		unique := len(seen)

		switch {
		case unique > total:
			return false, fmt.Errorf("profile search returned %d distinct profiles by page %d but TotalResults is %d", unique, page, total)
		case unique == total && n < pageSize:
			return true, nil
		case unique == total && reached:
			return false, fmt.Errorf("profile search returned a full page %d after all %d profiles were already seen", page, total)
		case unique == total:
			reached = true
		case n == 0:
			return false, fmt.Errorf("profile search returned an empty page %d after %d of %d profiles", page, unique, total)
		case added == 0:
			return false, fmt.Errorf("profile search did not advance on page %d: page was full but contained only already-seen profiles", page)
		}
		return false, nil
	}

	pages := 1
	done, err := applyPage(base, firstList, firstTotal)
	if err != nil {
		return nil, 0, 0, pages, err
	}

	for !done {
		pages++
		if pages > maxPages {
			return nil, 0, 0, pages, fmt.Errorf(
				"walked %d profile search pages (max=%d) without reaching a short/empty page or the reported TotalResults; "+
					"aborting rather than silently returning a truncated result set (this likely indicates a misbehaving API response)",
				pages, maxPages,
			)
		}
		page := base + pages - 1
		pOpts := *opts
		p, sz := page, pageSize
		pOpts.Page = &p
		pOpts.PageSize = &sz
		_, result, err := d.search.SearchProfiles(ctx, &pOpts)
		if err != nil {
			return nil, 0, 0, pages, fmt.Errorf("searching profiles (page %d): %w", page, err)
		}
		var list []sdk.ProfileDetailsV2Entity
		var totalResults *int
		if result != nil {
			list = result.ProfileList
			totalResults = result.TotalResults
		}
		done, err = applyPage(page, list, totalResults)
		if err != nil {
			return nil, 0, 0, pages, err
		}
	}

	if out == nil {
		out = []ProfileSummary{}
	}
	return out, total, base, pages, nil
}

// probeProfilePageBase determines whether the profile search endpoint is
// 0-indexed (live-confirmed on tenant paul-2609, UEM 26.9) or 1-indexed
// (live-confirmed on tenant as<internal-env>, UEM 26.2) for THIS walk — it is never
// cached across walks/reads, since different tenants (potentially served by
// the same provider process, e.g. across two `uem` provider configurations)
// can disagree.
//
// It requests page 0 first. A non-empty response means the endpoint is
// 0-based, and that page's data is returned as the first page of the walk
// (so the caller never re-fetches it). An empty (or HTTP-204-shaped, i.e. nil
// ProfileList and nil TotalResults) response means page 0 is out of range —
// live-confirmed on a 26.9 tenant whose profiles all fit on page 0 at a
// larger page size — so page 1 is probed next; a non-empty response there
// means the endpoint is 1-based.
//
// If BOTH probes come back empty, the search is confirmed to have zero
// results ONLY when neither probe reported a non-zero TotalResults. A
// non-zero TotalResults with no items on either probe is a genuine
// silent-partial-result condition (the API claims profiles exist but neither
// candidate first page produced any), not a legitimate empty answer, and is
// reported as an error rather than silently treated as zero.
//
// Returns (base, nil, nil, nil) when the search is confirmed to have zero
// results — callers must check for a nil list, not an empty-but-non-nil one,
// to distinguish "zero results" from "page `base` happened to come back
// empty" (which cannot happen once a base is confirmed, but is kept explicit
// here for clarity).
func probeProfilePageBase(ctx context.Context, search profilesV2SearchAPI, opts *sdk.ProfilesV2SearchProfilesOptions, pageSize int) (int, []sdk.ProfileDetailsV2Entity, *int, error) {
	ps := pageSize
	opts.PageSize = &ps

	p0 := 0
	opts.Page = &p0
	_, res0, err := search.SearchProfiles(ctx, opts)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("probing profile search page 0: %w", err)
	}
	var list0 []sdk.ProfileDetailsV2Entity
	var total0 *int
	if res0 != nil {
		list0 = res0.ProfileList
		total0 = res0.TotalResults
	}
	if len(list0) > 0 {
		return 0, list0, total0, nil
	}

	p1 := 1
	opts.Page = &p1
	_, res1, err := search.SearchProfiles(ctx, opts)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("probing profile search page 1: %w", err)
	}
	var list1 []sdk.ProfileDetailsV2Entity
	var total1 *int
	if res1 != nil {
		list1 = res1.ProfileList
		total1 = res1.TotalResults
	}
	if len(list1) > 0 {
		return 1, list1, total1, nil
	}

	nonZero := func(t *int) bool { return t != nil && *t != 0 }
	if nonZero(total0) || nonZero(total1) {
		reported := total0
		if reported == nil {
			reported = total1
		}
		return 0, nil, nil, fmt.Errorf(
			"profile search reported TotalResults=%d but page 0 and page 1 both returned no profiles; "+
				"refusing to treat this as zero results",
			*reported,
		)
	}
	return 0, nil, nil, nil
}

// sameProfileIDSet reports whether a and b contain exactly the same numeric
// profile ids (same count, same ids; order does not matter). Both slices are
// the output of walkProfiles, so every entry is guaranteed to have a numeric
// ProfileID.
func sameProfileIDSet(a, b []ProfileSummary) bool {
	if len(a) != len(b) {
		return false
	}
	ids := make(map[int64]bool, len(a))
	for _, p := range a {
		ids[p.ProfileID.ValueInt64()] = true
	}
	for _, p := range b {
		if !ids[p.ProfileID.ValueInt64()] {
			return false
		}
	}
	return true
}
