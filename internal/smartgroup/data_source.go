package smartgroup

import (
	"context"
	"fmt"
	"strconv"

	sdk "github.com/euc-oss/terraform-sdk-uem"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &SmartGroupsDataSource{}
var _ datasource.DataSourceWithConfigure = &SmartGroupsDataSource{}

func NewDataSource() datasource.DataSource {
	return &SmartGroupsDataSource{}
}

type SmartGroupsDataSource struct {
	client *sdk.Client
}

func (d *SmartGroupsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_smart_groups"
}

func (d *SmartGroupsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Searches for smart groups in Workspace ONE UEM. " +
			"Use this data source to look up smart group IDs for profile assignment.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				MarkdownDescription: "Filter smart groups by name (substring match)",
				Optional:            true,
			},
			"organization_group_id": schema.StringAttribute{
				MarkdownDescription: "Filter smart groups by organization group ID",
				Optional:            true,
			},
			"smart_groups": schema.ListNestedAttribute{
				MarkdownDescription: "List of smart groups matching the filter criteria",
				Computed:            true,
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
	client, ok := req.ProviderData.(*sdk.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *sdk.Client, got: %T", req.ProviderData),
		)
		return
	}
	if client == nil {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			"Provider data was nil or missing a configured SDK client.",
		)
		return
	}
	d.client = client
}

func (d *SmartGroupsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config SmartGroupsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	svc := sdk.NewSmartGroupsService(d.client)

	opts := &sdk.SmartGroupsSearchOptions{}
	if !config.Name.IsNull() && !config.Name.IsUnknown() {
		name := config.Name.ValueString()
		opts.Name = &name
	}
	if !config.OrganizationGroupID.IsNull() && !config.OrganizationGroupID.IsUnknown() {
		if id, err := strconv.Atoi(config.OrganizationGroupID.ValueString()); err == nil {
			opts.OrganizationGroupID = &id
		}
	}

	var allGroups []SmartGroupModel
	page := 0
	pageSize := 500
	opts.PageSize = &pageSize

	for {
		opts.Page = &page
		_, result, err := svc.Search(ctx, opts)
		if err != nil {
			resp.Diagnostics.AddError("Smart Group Search Failed",
				fmt.Sprintf("Unable to search smart groups: %s", err))
			return
		}
		if result == nil || len(result.SmartGroups) == 0 {
			break
		}
		for _, sg := range result.SmartGroups {
			item := SmartGroupModel{
				Name:                  types.StringValue(sg.Name),
				SmartGroupUUID:        types.StringValue(sg.SmartGroupUUID),
				ManagedByOrgGroupName: types.StringValue(sg.ManagedByOrganizationGroupName),
			}
			if sg.SmartGroupID != nil {
				item.SmartGroupID = types.Int64Value(int64(*sg.SmartGroupID))
			}
			allGroups = append(allGroups, item)
		}
		total := 0
		if result.Total != nil {
			total = *result.Total
		}
		if len(allGroups) >= total {
			break
		}
		page++
	}

	config.SmartGroups = allGroups
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
