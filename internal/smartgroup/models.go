package smartgroup

import "github.com/hashicorp/terraform-plugin-framework/types"

// SmartGroupsDataSourceModel is the Terraform state model for
// uem_smart_groups. Name/OrganizationGroupID are the search-mode filters;
// SmartGroupID and SmartGroupUUID are exact-lookup modes, mutually exclusive
// with the search filters and with each other (see ValidateConfig in
// data_source.go).
type SmartGroupsDataSourceModel struct {
	Name                types.String      `tfsdk:"name"`
	OrganizationGroupID types.String      `tfsdk:"organization_group_id"`
	SmartGroupID        types.Int64       `tfsdk:"smart_group_id"`
	SmartGroupUUID      types.String      `tfsdk:"smart_group_uuid"`
	SmartGroups         []SmartGroupModel `tfsdk:"smart_groups"`
}

type SmartGroupModel struct {
	SmartGroupID          types.Int64  `tfsdk:"smart_group_id"`
	Name                  types.String `tfsdk:"name"`
	SmartGroupUUID        types.String `tfsdk:"smart_group_uuid"`
	ManagedByOrgGroupName types.String `tfsdk:"managed_by_org_group_name"`
	// ManagedByOrgGroupID/UUID (internal-ticket, onboard smart-group closure):
	// the owning org group's numeric id/uuid, needed so ws1-tf onboard's
	// owned-only filter (and the dependency closure's owned-vs-lookup
	// decision) can compare a listed smart group's OWNER against the
	// onboarded org group, the same way every other onboardable type's list
	// data source already exposes its owner. UEM's SDK models
	// (SmartGroupSearchModelV1/SmartGroupV1) already carry both fields; only
	// the mapping into this data source's schema was missing.
	ManagedByOrgGroupID   types.String `tfsdk:"managed_by_org_group_id"`
	ManagedByOrgGroupUUID types.String `tfsdk:"managed_by_org_group_uuid"`
}

// SmartGroupResourceModel is the Terraform state model for uem_smart_group.
// Every list-like attribute is a set (UEM does not preserve order). The
// criteria sets (Platforms through OperatingSystems) are used only with
// criteria_type "All"; UserAdditions/DeviceAdditions only with "UserDevice";
// the exclusion sets with either. The provider does not enforce that
// pairing itself (removed from ValidateConfig in resource.go: unconfirmed
// against live UEM behaviour); UEM validates it at apply time instead.
type SmartGroupResourceModel struct {
	ID                    types.String `tfsdk:"id"`
	UUID                  types.String `tfsdk:"uuid"`
	Name                  types.String `tfsdk:"name"`
	ManagedByOrgGroupID   types.String `tfsdk:"managed_by_org_group_id"`
	ManagedByOrgGroupUUID types.String `tfsdk:"managed_by_org_group_uuid"`
	ManagedByOrgGroupName types.String `tfsdk:"managed_by_org_group_name"`
	CriteriaType          types.String `tfsdk:"criteria_type"`

	Platforms            types.Set `tfsdk:"platforms"`
	Ownerships           types.Set `tfsdk:"ownerships"`
	Models               types.Set `tfsdk:"models"`
	ManagementTypes      types.Set `tfsdk:"management_types"`
	EnrollmentCategories types.Set `tfsdk:"enrollment_categories"`
	CPUArchitectures     types.Set `tfsdk:"cpu_architectures"`
	OrganizationGroupIDs types.Set `tfsdk:"organization_group_ids"`
	UserGroupIDs         types.Set `tfsdk:"user_group_ids"`
	TagIDs               types.Set `tfsdk:"tag_ids"`
	OperatingSystems     types.Set `tfsdk:"operating_systems"`

	UserAdditions       types.Set `tfsdk:"user_additions"`
	DeviceAdditions     types.Set `tfsdk:"device_additions"`
	UserExclusions      types.Set `tfsdk:"user_exclusions"`
	DeviceExclusions    types.Set `tfsdk:"device_exclusions"`
	UserGroupExclusions types.Set `tfsdk:"user_group_exclusions"`

	DevicesCount     types.Int64 `tfsdk:"devices_count"`
	AssignmentsCount types.Int64 `tfsdk:"assignments_count"`
	ExclusionsCount  types.Int64 `tfsdk:"exclusions_count"`
}

// OperatingSystemModel is one element of uem_smart_group's
// operating_systems set.
type OperatingSystemModel struct {
	DeviceType types.String `tfsdk:"device_type"`
	Operator   types.String `tfsdk:"operator"`
	Value      types.String `tfsdk:"value"`
}
