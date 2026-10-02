package datasource

import (
	"context"
	"fmt"
	"net/http"

	"github.com/euc-oss/terraform-provider-uem/internal/providerdata"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &ApplicationsDataSource{}
var _ datasource.DataSourceWithConfigure = &ApplicationsDataSource{}

// appsV2SearchAPI is the narrow surface of sdk.AppsV2Service this data
// source depends on, so tests can inject a fake without a real SDK client.
type appsV2SearchAPI interface {
	Search(ctx context.Context, opts *sdk.AppsV2SearchOptions) (http.Header, *sdk.ApplicationSearchV2Model, error)
}

func NewDataSource() datasource.DataSource {
	return &ApplicationsDataSource{}
}

// ApplicationsDataSource implements the uem_applications Terraform data source.
type ApplicationsDataSource struct {
	search appsV2SearchAPI

	// maxPages, when non-zero, overrides maxApplicationSearchPages for this
	// instance. It exists purely so tests can inject a tiny cap and prove the
	// termination guard fires against a pathological fake that never signals
	// end-of-results, without waiting out the full production cap. Production
	// code always leaves this unset (zero value) and falls back to
	// maxApplicationSearchPages.
	maxPages int
}

// ApplicationsDataSourceModel is the Terraform state model for uem_applications.
type ApplicationsDataSourceModel struct {
	Name         types.String         `tfsdk:"name"`
	Platform     types.String         `tfsdk:"platform"`
	Applications []ApplicationSummary `tfsdk:"applications"`
}

// ApplicationSummary is one entry in the applications list returned by uem_applications.
type ApplicationSummary struct {
	ID       types.Int64  `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	Type     types.String `tfsdk:"type"`
	BundleID types.String `tfsdk:"bundle_id"`
}

func (d *ApplicationsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_applications"
}

func (d *ApplicationsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Searches for applications in Workspace ONE UEM. " +
			"Use this data source to look up application IDs, types, and bundle identifiers.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Filter applications by name (search text match)",
				Optional:            true,
			},
			"platform": schema.StringAttribute{
				MarkdownDescription: "Filter applications by platform",
				Optional:            true,
			},
			"applications": schema.ListNestedAttribute{
				MarkdownDescription: "List of applications matching the filter criteria",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.Int64Attribute{
							MarkdownDescription: "Application identifier",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Application name",
							Computed:            true,
						},
						"type": schema.StringAttribute{
							MarkdownDescription: "Application type",
							Computed:            true,
						},
						"bundle_id": schema.StringAttribute{
							MarkdownDescription: "Bundle identifier of the application",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (d *ApplicationsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
	d.search = sdk.NewAppsV2Service(pd.Client)
}

func (d *ApplicationsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ApplicationsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	opts := &sdk.AppsV2SearchOptions{}
	if !config.Name.IsNull() && !config.Name.IsUnknown() {
		name := config.Name.ValueString()
		opts.Name = &name
	}
	if !config.Platform.IsNull() && !config.Platform.IsUnknown() {
		platform := config.Platform.ValueString()
		opts.Platform = &platform
	}

	apps, err := walkApplications(ctx, d.search, opts, d.maxPages, "application search")
	if err != nil {
		wrapped := fmt.Errorf("searching applications: %w", err)
		resp.Diagnostics.AddError("Application Search Failed", wrapped.Error())
		return
	}

	applications := []ApplicationSummary{}
	for _, a := range apps {
		item := ApplicationSummary{
			Name:     types.StringValue(a.ApplicationName),
			Type:     types.StringValue(a.ApplicationType),
			BundleID: types.StringValue(a.BundleID),
		}
		if a.ID != nil {
			item.ID = types.Int64Value(int64(*a.ID))
		}
		applications = append(applications, item)
	}

	config.Applications = applications
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
