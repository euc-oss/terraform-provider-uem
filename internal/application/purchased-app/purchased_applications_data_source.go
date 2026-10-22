// Package purchasedapp implements the uem_purchased_applications Terraform
// data source as a hand-written concept, matching the pattern already used by
// internal/updates/update_deployments_data_source.go: one self-contained file
// (schema + model structs + Metadata/Schema/Configure/Read), its own
// locally-scoped SDK interfaces, no generated code. The purchased-app
// assignment RESOURCE lives in the assignment subpackage, mirroring the
// internal/sensors + internal/sensors/assignment split.
//
// Hand-written rather than gends-generated because the generator's
// composition-map model (one operations.list call, result fields reflected
// 1:1 from the SDK item struct) cannot express this data source's
// per-application assignment_count, which comes from a SECOND SDK call
// (AppsV2Service.GetAssignmentRuleAsync) rather than a field of the search
// result item -- the same class of fan-out gap that already made
// uem_update_deployments hand-written.
//
// uem_purchased_applications enumerates purchased (VPP) applications via
// PurchasedAppsV1Service.VppAppSearchAsync (GET /api/mam/apps/purchased/search)
// and then, for each application, reads its assignment rules to report how
// many assignments it has.
package purchasedapp

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/euc-oss/terraform-provider-uem/internal/providerdata"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &purchasedApplicationsDataSource{}
var _ datasource.DataSourceWithConfigure = &purchasedApplicationsDataSource{}

// defaultSearchPageSize is the page size requested from VppAppSearchAsync.
// 500 is also the server's own default page size (live-observed: an
// unpaged request echoes PageSize=500).
const defaultSearchPageSize = 500

// maxSearchPages bounds the pagination loop in Read. It exists purely as a
// termination guard against a pathological API response (one that never
// returns a short/empty page), not as a limit expected to be hit in
// practice: at pageSize=500 it allows 500,000 applications.
const maxSearchPages = 1000

// purchasedAppsSearchAPI is the narrow surface of sdk.PurchasedAppsV1Service
// this data source depends on, so tests can inject a fake without a real SDK
// client.
type purchasedAppsSearchAPI interface {
	VppAppSearchAsync(ctx context.Context, opts *sdk.PurchasedAppsV1VppAppSearchAsyncOptions) (http.Header, *sdk.PurchasedApplicationSearchResultV1, error)
}

// assignmentRulesAPI is the narrow read-only surface of sdk.AppsV2Service
// used to count each application's assignments. It is the same
// GetAssignmentRuleAsync call (api_version=v2) the
// uem_purchased_application_assignment resource's Read uses, so
// assignment_count reflects exactly what an import of that resource would
// read back.
type assignmentRulesAPI interface {
	GetAssignmentRuleAsync(ctx context.Context, applicationUUID string) (http.Header, *sdk.AppAssignmentRuleV2Model, error)
}

type purchasedApplicationsDataSource struct {
	search   purchasedAppsSearchAPI
	rules    assignmentRulesAPI
	pageSize int // 0 means defaultSearchPageSize; overridden by tests
}

// NewPurchasedApplicationsDataSource is the constructor
// internal/provider/provider.go registers.
func NewPurchasedApplicationsDataSource() datasource.DataSource {
	return &purchasedApplicationsDataSource{}
}

// PurchasedApplicationsDataSourceModel is the Terraform state model for
// uem_purchased_applications.
type PurchasedApplicationsDataSourceModel struct {
	OrganizationGroupUuid types.String                  `tfsdk:"organization_group_uuid"`
	PurchasedApplications []PurchasedApplicationSummary `tfsdk:"purchased_applications"`
}

// PurchasedApplicationSummary is one purchased application projected into
// Terraform state.
type PurchasedApplicationSummary struct {
	ID                    types.Int64  `tfsdk:"id"`
	UUID                  types.String `tfsdk:"uuid"`
	Name                  types.String `tfsdk:"name"`
	BundleID              types.String `tfsdk:"bundle_id"`
	Platform              types.Int64  `tfsdk:"platform"`
	OrganizationGroupUuid types.String `tfsdk:"organization_group_uuid"`
	LocationGroupID       types.Int64  `tfsdk:"location_group_id"`
	AssignmentCount       types.Int64  `tfsdk:"assignment_count"`
}

func (d *purchasedApplicationsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_purchased_applications"
}

func (d *purchasedApplicationsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Enumerates purchased (VPP) applications visible to the given organization group. " +
			"Use this data source to look up application UUIDs for importing `uem_purchased_application_assignment`. " +
			"The result can include applications OWNED by an ancestor organization group and merely visible to the " +
			"requested one; compare each entry's `organization_group_uuid` against the requested group to tell them apart. " +
			"Note: `assignment_count` requires one additional GET per application (its assignment rules), on top of the " +
			"paged search itself.",
		Attributes: map[string]schema.Attribute{
			"organization_group_uuid": schema.StringAttribute{
				MarkdownDescription: "Organization group UUID to search within (required)",
				Required:            true,
			},
			"purchased_applications": schema.ListNestedAttribute{
				MarkdownDescription: "List of purchased applications visible to the organization group",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.Int64Attribute{
							MarkdownDescription: "Numeric application ID",
							Computed:            true,
						},
						"uuid": schema.StringAttribute{
							MarkdownDescription: "Application UUID (the import ID for `uem_purchased_application_assignment`)",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Application name",
							Computed:            true,
						},
						"bundle_id": schema.StringAttribute{
							MarkdownDescription: "Application bundle identifier (package ID)",
							Computed:            true,
						},
						"platform": schema.Int64Attribute{
							MarkdownDescription: "UEM device-type code of the platform the application targets (e.g. 2 = Apple iOS, 10 = macOS)",
							Computed:            true,
						},
						"organization_group_uuid": schema.StringAttribute{
							MarkdownDescription: "UUID of the organization group that OWNS this application",
							Computed:            true,
						},
						"location_group_id": schema.Int64Attribute{
							MarkdownDescription: "Numeric ID of the organization group that OWNS this application",
							Computed:            true,
						},
						"assignment_count": schema.Int64Attribute{
							MarkdownDescription: "Number of assignments in the application's assignment rules (0 = no assignment rules)",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (d *purchasedApplicationsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
	d.search = sdk.NewPurchasedAppsV1Service(pd.Client)
	d.rules = sdk.NewAppsV2Service(pd.Client)
}

func (d *purchasedApplicationsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config PurchasedApplicationsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ogUUID := config.OrganizationGroupUuid.ValueString()
	if ogUUID == "" {
		resp.Diagnostics.AddError("Invalid Configuration", "organization_group_uuid is required and must not be empty")
		return
	}

	apps, err := d.listAll(ctx, ogUUID)
	if err != nil {
		resp.Diagnostics.AddError("Purchased Application Search Failed", err.Error())
		return
	}

	// Initialised to an EMPTY (non-nil) slice so a zero-result read stores an
	// empty list rather than a NULL one, which Terraform would drop from
	// `terraform output` entirely (same reason as uem_update_deployments).
	out := []PurchasedApplicationSummary{}
	for _, a := range apps {
		if a.UUID == "" {
			resp.Diagnostics.AddWarning(
				"Purchased Application Without UUID",
				fmt.Sprintf("Purchased application %q came back without a UUID; skipping it from the result set -- it cannot be referenced or imported.", a.ApplicationName),
			)
			continue
		}
		// Fail closed: an assignment-rule read error aborts the WHOLE Read
		// (no partial list), consistent with a search error above.
		_, rule, err := d.rules.GetAssignmentRuleAsync(ctx, a.UUID)
		if err != nil {
			resp.Diagnostics.AddError("Assignment Rule Lookup Failed", fmt.Sprintf("Unable to read assignment rules for purchased application %s: %s", a.UUID, err))
			return
		}
		count := 0
		if rule != nil {
			count = len(rule.Assignments)
		}

		summary := PurchasedApplicationSummary{
			UUID:                  types.StringValue(a.UUID),
			Name:                  types.StringValue(a.ApplicationName),
			BundleID:              types.StringValue(a.BundleID),
			OrganizationGroupUuid: types.StringValue(a.OrganizationGroupUUID),
			AssignmentCount:       types.Int64Value(int64(count)),
			ID:                    types.Int64Null(),
			Platform:              types.Int64Null(),
			LocationGroupID:       types.Int64Null(),
		}
		if a.ID != nil && a.ID.Value != nil {
			summary.ID = types.Int64Value(*a.ID.Value)
		}
		if a.Platform != nil {
			summary.Platform = types.Int64Value(int64(*a.Platform))
		}
		if a.LocationGroupID != nil {
			summary.LocationGroupID = types.Int64Value(*a.LocationGroupID)
		}
		out = append(out, summary)
	}

	config.PurchasedApplications = out
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// listAll walks every page of VppAppSearchAsync for ogUUID, returning the
// applications deduplicated by (case-insensitive) UUID.
//
// The endpoint's paging contract was LIVE-VERIFIED (as<internal-env>, the throwaway
// write org group, 7 apps, page sizes 1/2/3/7/500/1000) before this loop was
// written, because
// it differs from the other paged endpoints in this provider:
//
//   - Pages are 0-BASED: page=0 is the first page (page=1 is the second).
//   - The response's Total is the number of items ON THAT PAGE, not the
//     grand total and not a page count (pagesize=2 over 7 apps returned
//     Total=2,2,2,1 for pages 0..3). A "stop once Total items collected"
//     loop would therefore stop after the first page and silently truncate,
//     so Total is deliberately IGNORED here.
//   - Past the last page the server answers HTTP 204 No Content, which the
//     SDK surfaces as a zero-value result (no Application entries).
//
// So the stop condition is: an empty page (incl. 204), or a page shorter
// than the effective page size. The effective page size is the one the
// server ECHOES back in PageSize when present (it was echoed faithfully
// live, up to 100000), falling back to the requested size; using the echo
// means a server that caps the page size below the request cannot make a
// full-but-capped page look short. A FULL page that adds no new UUID means
// the server is not advancing (e.g. ignoring the page parameter), which is
// reported as an error rather than looping or silently truncating;
// maxSearchPages bounds the loop regardless.
func (d *purchasedApplicationsDataSource) listAll(ctx context.Context, ogUUID string) ([]sdk.PurchasedApplicationEntityV1, error) {
	pageSize := d.pageSize
	if pageSize <= 0 {
		pageSize = defaultSearchPageSize
	}
	var out []sdk.PurchasedApplicationEntityV1
	seen := make(map[string]bool)
	for page := 0; ; page++ {
		if page >= maxSearchPages {
			return nil, fmt.Errorf("walked %d pages (maxSearchPages) without reaching a short or empty page; aborting rather than returning a truncated result set", maxSearchPages)
		}
		p, ps, og := page, pageSize, ogUUID
		_, res, err := d.search.VppAppSearchAsync(ctx, &sdk.PurchasedAppsV1VppAppSearchAsyncOptions{
			OrganizationGroupUUID: &og,
			Page:                  &p,
			PageSize:              &ps,
		})
		if err != nil {
			return nil, fmt.Errorf("unable to search purchased applications (page %d): %w", page, err)
		}
		if res == nil || len(res.Application) == 0 {
			return out, nil
		}
		added := 0
		// Dedup-by-lowercased-UUID (B16 decision table row #144, KEEP —
		// inconclusive, defensive). UEM source:
		// AirWatch API/AirWatch.Data/Applications/PurchasedApplicationSearch.cs:81-92
		// (canonical Q21): the in-memory SortApplications re-sort has no
		// tie-breaker, and whether a row can repeat across pages under
		// concurrent data changes is "Data unavailable" — neither confirmed
		// nor ruled out. This dedup is cheap insurance either way: harmless
		// if rows never repeat, protective if they do.
		for _, a := range res.Application {
			key := strings.ToLower(a.UUID)
			if key != "" {
				if seen[key] {
					continue
				}
				seen[key] = true
			}
			out = append(out, a)
			added++
		}
		effective := pageSize
		if res.PageSize != nil && *res.PageSize > 0 {
			effective = *res.PageSize
		}
		if len(res.Application) < effective {
			return out, nil
		}
		if added == 0 {
			return nil, fmt.Errorf("purchased application search did not advance: page %d was full but contained only already-seen applications", page)
		}
	}
}
