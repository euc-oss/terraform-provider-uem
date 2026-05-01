package smartgroup

import "github.com/hashicorp/terraform-plugin-framework/types"

type SmartGroupsDataSourceModel struct {
	Name                types.String      `tfsdk:"name"`
	OrganizationGroupID types.String      `tfsdk:"organization_group_id"`
	SmartGroups         []SmartGroupModel `tfsdk:"smart_groups"`
}

type SmartGroupModel struct {
	SmartGroupID          types.Int64  `tfsdk:"smart_group_id"`
	Name                  types.String `tfsdk:"name"`
	SmartGroupUUID        types.String `tfsdk:"smart_group_uuid"`
	ManagedByOrgGroupName types.String `tfsdk:"managed_by_org_group_name"`
}
