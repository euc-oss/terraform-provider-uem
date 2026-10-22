// Package updates implements the uem_update_deployments Terraform data
// source as a hand-written concept, matching the pattern already used by
// internal/profile/data_source.go and internal/smartgroup/data_source.go:
// one self-contained file (schema + model structs + Metadata/Schema/
// Configure/Read), its own locally-scoped SDK interface, no generated code.
//
// uem_update_deployments enumerates DEPLOYMENTS (not the update catalog) via
// a two-level fetch:
//
//  1. List catalog updates (paginated) via
//     UpdatesV1Service.GetDeviceUpdatesBySearchParameters — each entry's UUID
//     identifies one update in the catalog.
//  2. For EACH update UUID, fetch its deployments via
//     UpdatesV1Service.GetDeploymentsByDeviceUpdate, and flatten all
//     deployments across all updates into the final output list.
//
// This repurposes update_deployments_data_source_gen.go /
// update_deployments_hooks.go IN PLACE — that generated pair wrongly
// enumerated the update catalog (available_for_install/description/
// external_key, no UUID field at all) rather than deployments. Confirmed not
// consumed anywhere in production (only a registration test + the
// composition-map entry referenced it), so repurposing in place carries no
// migration risk.
package updates

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/euc-oss/terraform-provider-uem/internal/pagewalk"
	"github.com/euc-oss/terraform-provider-uem/internal/providerdata"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &updateDataSource{}
var _ datasource.DataSourceWithConfigure = &updateDataSource{}

// maxCatalogPages bounds the level-1 catalog pagination loop in Read. At
// pageSize=50 this allows walking up to 50,000 catalog entries before
// erroring out — far beyond any real tenant's update catalog size — and
// exists purely as a termination guard against a pathological/misbehaving
// API response (e.g. one that never returns a short/empty page and never
// reports a Total), not as a limit expected to be hit in practice.
const maxCatalogPages = 1000

// updatesV1SearchAPI is the narrow surface of sdk.UpdatesV1Service this data
// source depends on, so tests can inject a fake without a real SDK client.
// Mirrors profile's profilesV2SearchAPI / application's appsV2SearchAPI
// pattern. Deliberately NOT the same interface as
// internal/updates/deployment/service.go's UpdatesV1API — that one is the
// RESOURCE's narrow CRUD-only interface (Create/Get/Update/Delete-by-UUID)
// and is untouched by this data-source work.
type updatesV1SearchAPI interface {
	GetDeviceUpdatesBySearchParameters(ctx context.Context, opts *sdk.UpdatesV1GetDeviceUpdatesBySearchParametersOptions) (http.Header, *sdk.DeviceUpdatePagedSearchResultsV1Model, error)
	GetDeploymentsByDeviceUpdate(ctx context.Context, updateUUID string, opts *sdk.UpdatesV1GetDeploymentsByDeviceUpdateOptions) (http.Header, *[]sdk.DeploymentV1Model, error)
	// GetDeviceUpdateDeploymentDetails is the level-3, per-deployment detail lookup added to
	// resolve each deployment's OWNING organization group. DeploymentV1Model (the level-2
	// list model returned by GetDeploymentsByDeviceUpdate above) has NO owner field at all;
	// only the single-item detail model (DeviceUpdateDeploymentV1Model) carries
	// OrganizationGroupUUID, so this is the only way to obtain it here.
	GetDeviceUpdateDeploymentDetails(ctx context.Context, uuid string) (http.Header, *sdk.DeviceUpdateDeploymentV1Model, error)
}

// updateDataSource implements the uem_update_deployments Terraform data
// source. NewUpdateDataSource is the exact constructor name
// internal/provider/provider.go already registers — preserved unchanged so
// provider.go needs no edit.
type updateDataSource struct {
	search updatesV1SearchAPI
}

func NewUpdateDataSource() datasource.DataSource {
	return &updateDataSource{}
}

// UpdateDataSourceModel is the Terraform state model for
// uem_update_deployments.
type UpdateDataSourceModel struct {
	OrganizationGroupUuid types.String        `tfsdk:"organization_group_uuid"`
	Platform              types.String        `tfsdk:"platform"`
	UpdateName            types.String        `tfsdk:"update_name"`
	UpdateDeployments     []DeploymentSummary `tfsdk:"update_deployments"`
}

// DeploymentSummary is one flattened deployment entry — the shape returned
// by UpdatesV1Service.GetDeploymentsByDeviceUpdate for a single update in
// the catalog, projected into Terraform state.
type DeploymentSummary struct {
	UUID                  types.String `tfsdk:"uuid"`
	Name                  types.String `tfsdk:"name"`
	DeploymentType        types.String `tfsdk:"deployment_type"`
	DeploymentStartTime   types.String `tfsdk:"deployment_start_time"`
	Ranking               types.Int64  `tfsdk:"ranking"`
	SmartGroupCount       types.Int64  `tfsdk:"smart_group_count"`
	OrganizationGroupUuid types.String `tfsdk:"organization_group_uuid"`
}

func (d *updateDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_update_deployments"
}

func (d *updateDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Enumerates update deployments in Workspace ONE UEM by walking every update " +
			"in the catalog for the given organization group and flattening each update's deployments into " +
			"a single list. Use this data source to look up deployment UUIDs and names for import. " +
			"Note: resolving each deployment's owning organization group requires one additional GET per " +
			"deployment (UpdatesV1Service.GetDeviceUpdateDeploymentDetails), on top of the one GET per " +
			"catalog update already required to list its deployments.",
		Attributes: map[string]schema.Attribute{
			"organization_group_uuid": schema.StringAttribute{
				MarkdownDescription: "Organization group UUID to search within (required)",
				Required:            true,
			},
			"platform": schema.StringAttribute{
				MarkdownDescription: "Filter catalog updates by platform (e.g. Apple, AppleOSX, AppleTV)",
				Optional:            true,
			},
			"update_name": schema.StringAttribute{
				MarkdownDescription: "Filter catalog updates by name",
				Optional:            true,
			},
			"update_deployments": schema.ListNestedAttribute{
				MarkdownDescription: "List of deployments across every matching update in the catalog",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"uuid": schema.StringAttribute{
							MarkdownDescription: "Deployment UUID",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Deployment name",
							Computed:            true,
						},
						"deployment_type": schema.StringAttribute{
							MarkdownDescription: "Deployment type",
							Computed:            true,
						},
						"deployment_start_time": schema.StringAttribute{
							MarkdownDescription: "Deployment start time",
							Computed:            true,
						},
						"ranking": schema.Int64Attribute{
							MarkdownDescription: "Ranking of the deployment (1 is highest)",
							Computed:            true,
						},
						"smart_group_count": schema.Int64Attribute{
							MarkdownDescription: "Count of smart groups associated with the deployment",
							Computed:            true,
						},
						"organization_group_uuid": schema.StringAttribute{
							MarkdownDescription: "UUID of the organization group that OWNS this deployment, resolved via " +
								"one additional per-deployment detail GET (UpdatesV1Service.GetDeviceUpdateDeploymentDetails).",
							Computed: true,
						},
					},
				},
			},
		},
	}
}

func (d *updateDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
	d.search = sdk.NewUpdatesV1Service(pd.Client)
}

func (d *updateDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config UpdateDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ogUUID := config.OrganizationGroupUuid.ValueString()
	if ogUUID == "" {
		resp.Diagnostics.AddError("Invalid Configuration", "organization_group_uuid is required and must not be empty")
		return
	}

	opts := &sdk.UpdatesV1GetDeviceUpdatesBySearchParametersOptions{OrganizationGroupUUID: ogUUID}
	if !config.Platform.IsNull() && !config.Platform.IsUnknown() {
		v := config.Platform.ValueString()
		opts.Platform = &v
	}
	if !config.UpdateName.IsNull() && !config.UpdateName.IsUnknown() {
		v := config.UpdateName.ValueString()
		opts.UpdateName = &v
	}

	// Level 1: paginated catalog fetch, hardened via internal/pagewalk (B13,
	// mirroring B12's applications/scripts/mac_applications walks):
	// GetDeviceUpdatesBySearchParameters paginates via Page/PageSize request
	// fields + Page/PageSize/Total response fields (no cursor) — confirmed
	// via a live fixture test: Total=173, len(UpdateList)=50 on page 1.
	// Every page MUST be walked before level 2 runs, or deployments for
	// updates past the first page would silently never be fetched.
	//
	// The page-index BASE is 1 for this endpoint, FIXED rather than probed
	// (live-confirmed: requesting page=0 doesn't come back empty/HTTP-204
	// like every other pagewalk-hardened endpoint's out-of-range page --
	// GetDeviceUpdatesBySearchParameters instead returns a hard HTTP 422
	// "Page must be a positive numeric value" validation error against
	// tenant as<internal-env>. Using pagewalk.WalkTwiceVerified's own page-0 probe
	// here would fail the walk before ever reaching real data, so this uses
	// pagewalk.WalkTwiceVerifiedFromBase instead — same hardening, base
	// given directly rather than probed.
	//
	// pagewalk trusts Total only from non-empty pages, requires it to stay
	// identical across pages, and — critically — only ends the walk once the
	// DEDUPED unique count reaches it, never merely because an individual
	// page came back short of the requested page size. This matters because
	// the server is free to cap the effective page size below the requested
	// PageSize (gate finding, reproduced: server caps pages at 20 even
	// though PageSize=50 was requested, with Total=120) — a naive
	// "len(page) < pageSize" check would have stopped after the very first
	// capped page at 20, silently dropping the remaining 100; pagewalk's
	// "unique == total && n < pageSize" rule only fires once the count
	// genuinely matches Total, so a short-relative-to-requested page that
	// ISN'T the last page correctly keeps paging.
	//
	// Progress is counted in unique (deduped) UUIDs, never raw items per
	// page: a server that ignores the requested Page param and returns the
	// same page forever, or claims more distinct items exist than its own
	// Total, is a pagewalk error (never a silently truncated or stuck
	// result) — this is the "strict total check" pagewalk adds beyond the
	// old hand-rolled loop, which only compared the deduped count against
	// Total to decide when to STOP, never treated "more unique items than
	// Total claims" as an anomaly in its own right. maxCatalogPages bounds
	// the walk (each independent walk, since a multi-page result is walked
	// twice) so a pathological API response can't spin forever.
	catalogPageSize := 50
	fetch := func(ctx context.Context, page, pageSize int) (pagewalk.Page[sdk.DeviceUpdateDetailsDeploymentsV1Model], error) {
		pOpts := *opts
		p, sz := page, pageSize
		pOpts.Page = &p
		pOpts.PageSize = &sz
		_, res, err := d.search.GetDeviceUpdatesBySearchParameters(ctx, &pOpts)
		if err != nil {
			return pagewalk.Page[sdk.DeviceUpdateDetailsDeploymentsV1Model]{}, err
		}
		if res == nil {
			return pagewalk.Page[sdk.DeviceUpdateDetailsDeploymentsV1Model]{}, nil
		}
		return pagewalk.Page[sdk.DeviceUpdateDetailsDeploymentsV1Model]{Items: res.UpdateList, Total: res.Total}, nil
	}
	key := func(item sdk.DeviceUpdateDetailsDeploymentsV1Model) (string, error) {
		if item.UUID == "" {
			return "", fmt.Errorf("update catalog entry %q came back from search without a uuid; cannot page or dedupe it", item.Name)
		}
		return strings.ToLower(item.UUID), nil
	}

	const catalogPageBase = 1
	catalogResult, err := pagewalk.WalkTwiceVerifiedFromBase(ctx, fetch, key, catalogPageSize, maxCatalogPages, catalogPageBase, "update catalog search")
	if err != nil {
		resp.Diagnostics.AddError("Update Catalog Search Failed", fmt.Sprintf("Unable to search device updates: %s", err))
		return
	}
	updateUUIDs := make([]string, 0, len(catalogResult.Items))
	for _, item := range catalogResult.Items {
		updateUUIDs = append(updateUUIDs, item.UUID)
	}

	// Level 2: per-update-UUID deployment fetch, flattened into a single
	// list. GetDeploymentsByDeviceUpdate returns a clean nil/empty slice on
	// HTTP 204 (an update with zero deployments) — confirmed via a real
	// fixture-backed SDK test, no error, no 404 risk for that case, so no
	// special-casing is needed here beyond a nil-safe range.
	//
	// Initialised to an EMPTY (non-nil) slice, not left as a nil var: a nil
	// slice marshals as a NULL list attribute in state, which Terraform then
	// drops from `terraform output` entirely on a zero-result run (gate
	// finding, live e2e against org group 138883 with zero deployments —
	// `ws1-tf onboard --type update_deployment` hard-failed with
	// `terraform output "update_deployments" not found` instead of the
	// intended "No update deployments found" clean exit). Mirrors
	// internal/sensors' data source, which correctly emits `sensors = []`
	// for the same reason. Every exit path that can leave this empty (an
	// empty catalog, every update returning 204/nil) is covered by starting
	// from an empty slice up front rather than only allocating on the first
	// append.
	deployments := []DeploymentSummary{}
	deploymentOpts := &sdk.UpdatesV1GetDeploymentsByDeviceUpdateOptions{OrganizationGroupUUID: ogUUID}
	for _, updateUUID := range updateUUIDs {
		_, deps, err := d.search.GetDeploymentsByDeviceUpdate(ctx, updateUUID, deploymentOpts)
		if err != nil {
			// A level-2 (per-update deployment fetch) error fails the WHOLE Read closed — no partial list is ever returned — deliberately consistent with how a level-1 (catalog) error is also handled above.
			resp.Diagnostics.AddError("Deployment Lookup Failed", fmt.Sprintf("Unable to fetch deployments for update %s: %s", updateUUID, err))
			return
		}
		if deps == nil {
			continue
		}
		for _, dep := range *deps {
			summary := DeploymentSummary{
				UUID:           types.StringValue(dep.UUID),
				Name:           types.StringValue(dep.Name),
				DeploymentType: types.StringValue(dep.DeploymentType),
			}
			if !dep.DeploymentStartTime.IsZero() {
				summary.DeploymentStartTime = types.StringValue(dep.DeploymentStartTime.String())
			} else {
				summary.DeploymentStartTime = types.StringNull()
			}
			if dep.Ranking != nil {
				summary.Ranking = types.Int64Value(int64(*dep.Ranking))
			}
			if dep.SmartGroupCount != nil {
				summary.SmartGroupCount = types.Int64Value(int64(*dep.SmartGroupCount))
			}

			// Level 3: per-deployment detail fetch to resolve the OWNING organization
			// group. DeploymentV1Model (dep, above) has no owner field at all; only the
			// single-item detail model carries OrganizationGroupUUID. This is a SERIAL
			// per-deployment GET (no goroutines/concurrency) -- one extra GET per
			// deployment, in addition to the existing one GET per catalog update above.
			// B16 decision table row #200 (KEEP+cite, upgraded confidence).
			// UEM source: AirWatch API/AW.Mdm.Api/AW.Mdm.Api/Controllers/OsUpdates/V1/UpdatesV1Controller.cs:575-590,987-991
			// (canonical Q28): GetDeviceUpdateDeploymentDetailsImplAsync returns
			// 200 with a model or 404 -- there is no 204 path for the
			// single-deployment details endpoint -- and an inaccessible OG at
			// that endpoint surfaces as 404 (ValidateOrganizationGroup failure),
			// by design, NOT as an empty organization_group_uuid. A 404 here
			// therefore reaches the fail-closed detailErr branch immediately
			// below (aborting the whole Read), not the skip-with-warning branch
			// underneath it. That skip-with-warning branch instead guards a
			// narrower, DB-data-integrity scenario Q28 also confirms: a loaded
			// deployment whose OrganizationGroupUuid defaults to Guid.Empty in
			// the DB when genuinely missing -- unrelated to OG access.
			_, detail, detailErr := d.search.GetDeviceUpdateDeploymentDetails(ctx, dep.UUID)
			if detailErr != nil {
				// Fail closed: a detail lookup error aborts the WHOLE Read immediately --
				// no partial list is ever returned -- deliberately consistent with how
				// level-1/level-2 errors are handled above.
				resp.Diagnostics.AddError("Deployment Detail Lookup Failed", fmt.Sprintf("Unable to fetch owning organization group for deployment %s: %s", dep.UUID, detailErr))
				return
			}
			if detail == nil || detail.OrganizationGroupUUID == "" {
				// A nil detail (never actually observed against the pinned SDK --
				// GetDeviceUpdateDeploymentDetails always returns a non-nil struct,
				// even for a 204: handleResponse skips decoding an empty body and
				// leaves every field at its zero value) OR an empty
				// OrganizationGroupUUID (what a REAL 204 yields: a non-nil struct
				// whose owner field never got populated) are both treated the same
				// way the level-2 loop treats a 204/nil deployments response for an
				// update: skip the item. Checking the empty-string case explicitly
				// closes a gap a nil-only check would miss entirely: without it, a
				// 204 detail would flow through with owner "" and be reported as
				// "owned by ()" -- and --include-inherited would import a deployment
				// whose owner was never actually resolved. This is surfaced as a
				// warning (not silent) since it means one deployment's owning org
				// group could not be resolved.
				resp.Diagnostics.AddWarning(
					"Deployment Detail Not Found",
					fmt.Sprintf("Deployment %s returned no detail record (nil/empty response); skipping it from the result set -- its owning organization group could not be resolved.", dep.UUID),
				)
				continue
			}
			summary.OrganizationGroupUuid = types.StringValue(detail.OrganizationGroupUUID)

			deployments = append(deployments, summary)
		}
	}

	config.UpdateDeployments = deployments
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
