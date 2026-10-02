package smartgroup

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/euc-oss/terraform-provider-uem/internal/providerdata"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &SmartGroupResource{}
var _ resource.ResourceWithConfigure = &SmartGroupResource{}
var _ resource.ResourceWithValidateConfig = &SmartGroupResource{}
var _ resource.ResourceWithModifyPlan = &SmartGroupResource{}
var _ resource.ResourceWithImportState = &SmartGroupResource{}

const (
	criteriaTypeAll        = "All"
	criteriaTypeUserDevice = "UserDevice"
)

// smartGroupResourceAPI is the SmartGroupsService surface the resource uses,
// so tests can inject a fake.
type smartGroupResourceAPI interface {
	smartGroupSearchAPI
	smartGroupLoadAPI
	CreateSmartGroupAsync(ctx context.Context, request *sdk.SmartGroupEditV1Model) (http.Header, *sdk.SmartGroupCreateResponseV1, error)
	UpdateSmartGroupAsync(ctx context.Context, id int, request *sdk.SmartGroupEditV1Model) (http.Header, error)
	DeleteAsync(ctx context.Context, id int) (http.Header, error)
}

func NewResource() resource.Resource {
	return &SmartGroupResource{}
}

// SmartGroupResource implements uem_smart_group, which manages one smart
// group (MDM API v1, /api/mdm/smartgroups). Create/Update send a whole SmartGroupEditV1Model
// built from the plan and then re-read the group with LoadSmartGroupAsync, so
// state always reflects the server's view. Import accepts a numeric id or a
// UUID; a UUID is resolved through the same hardened search walk the
// uem_smart_groups data source uses (findSmartGroupsByUUID in search.go).
//
// OEMAndModels (the "Manufacturer/OEM and Model" criterion) is deliberately
// NOT modelled: it is deferred. Read warns when a group has it set on the
// server, and Update carries the server's current value through unchanged so
// a whole-object PUT does not drop it.
type SmartGroupResource struct {
	svc      smartGroupResourceAPI
	pageSize int // 0 means defaultSmartGroupSearchPageSize; overridden by tests
}

func (r *SmartGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_smart_group"
}

func (r *SmartGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
	r.svc = sdk.NewSmartGroupsService(pd.Client)
}

// criteriaAttributes are the attributes UEM uses only with criteria_type
// "All"; the provider no longer enforces that pairing at plan time (removed:
// unconfirmed against live UEM behaviour, see ValidateConfig below), but
// still uses this list to decide whether an "All" smart group has any
// criteria configured, for the "matches every device" warning.
var criteriaAttributes = []string{
	"platforms", "ownerships", "models", "management_types", "enrollment_categories",
	"cpu_architectures", "organization_group_ids", "user_group_ids", "tag_ids", "operating_systems",
}

func stringSetAttribute(description string) schema.SetAttribute {
	return schema.SetAttribute{
		MarkdownDescription: description,
		ElementType:         types.StringType,
		Optional:            true,
	}
}

func (r *SmartGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	allOnly := " Used only with `criteria_type = \"All\"`; UEM validates the pairing."
	userDeviceOnly := " Used only with `criteria_type = \"UserDevice\"`; UEM validates the pairing."
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a smart group in Workspace ONE UEM (MDM API v1). A smart group either matches " +
			"devices by criteria (`criteria_type = \"All\"`) or lists users and devices explicitly " +
			"(`criteria_type = \"UserDevice\"`). The Manufacturer/OEM and Model criterion (`OEMAndModels` in the " +
			"UEM API) is not currently supported by this resource; it is deferred.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Numeric smart group identifier assigned by UEM.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"uuid": schema.StringAttribute{
				MarkdownDescription: "Smart group UUID assigned by UEM.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Smart group name.",
				Required:            true,
			},
			"managed_by_org_group_id": schema.StringAttribute{
				MarkdownDescription: "Numeric id of the organization group that manages the smart group. Must be " +
					"numeric (validated at plan time). Changing it replaces the smart group.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"managed_by_org_group_uuid": schema.StringAttribute{
				MarkdownDescription: "UUID of the managing organization group, as reported by UEM.",
				Computed:            true,
			},
			"managed_by_org_group_name": schema.StringAttribute{
				MarkdownDescription: "Name of the managing organization group, as reported by UEM.",
				Computed:            true,
			},
			"criteria_type": schema.StringAttribute{
				MarkdownDescription: "`All` (match devices by the criteria attributes) or `UserDevice` (explicit " +
					"`user_additions`/`device_additions`). Read back exactly as UEM returns it; UEM validates the value.",
				Required: true,
			},
			"platforms": stringSetAttribute("Device platforms to match (for example `AppleOsX`)." + allOnly),
			"ownerships": stringSetAttribute("Device ownership types to match (for example `CorporateDedicated`)." + allOnly +
				" Read back exactly as UEM returns it. When unset, UEM stores `allownerships`; that server default is read back as unset, so it causes no diff."),
			"models":                 stringSetAttribute("Device models to match (for example `iPad`)." + allOnly),
			"management_types":       stringSetAttribute("Management types to match (for example `MdmEnrolled`)." + allOnly),
			"enrollment_categories":  stringSetAttribute("Enrollment categories to match (for example `DepEnrolled`)." + allOnly),
			"cpu_architectures":      stringSetAttribute("Device CPU architectures to match." + allOnly),
			"organization_group_ids": stringSetAttribute("Numeric ids of organization groups whose devices match." + allOnly),
			"user_group_ids":         stringSetAttribute("Numeric ids of user groups whose devices match." + allOnly),
			"tag_ids":                stringSetAttribute("Numeric ids of tags whose devices match." + allOnly),
			"operating_systems": schema.SetNestedAttribute{
				MarkdownDescription: "Operating system version rules to match." + allOnly +
					" Every field is read back exactly as UEM returns it.",
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"device_type": schema.StringAttribute{
							MarkdownDescription: "Device type (platform) the rule applies to.",
							Required:            true,
						},
						"operator": schema.StringAttribute{
							MarkdownDescription: "Comparison operator (for example `GreaterThan`).",
							Required:            true,
						},
						"value": schema.StringAttribute{
							MarkdownDescription: "Operating system version compared against.",
							Required:            true,
						},
					},
				},
			},
			"user_additions":        stringSetAttribute("Ids of users added explicitly." + userDeviceOnly),
			"device_additions":      stringSetAttribute("Ids of devices added explicitly." + userDeviceOnly),
			"user_exclusions":       stringSetAttribute("Ids of users excluded. Valid with either `criteria_type`."),
			"device_exclusions":     stringSetAttribute("Ids of devices excluded. Valid with either `criteria_type`."),
			"user_group_exclusions": stringSetAttribute("Ids of user groups excluded. Valid with either `criteria_type`."),
			"devices_count": schema.Int64Attribute{
				MarkdownDescription: "Number of devices in the smart group, refreshed on every read.",
				Computed:            true,
			},
			"assignments_count": schema.Int64Attribute{
				MarkdownDescription: "Number of entities the smart group is assigned to, refreshed on every read.",
				Computed:            true,
			},
			"exclusions_count": schema.Int64Attribute{
				MarkdownDescription: "Number of entities the smart group is excluded from, refreshed on every read.",
				Computed:            true,
			},
		},
	}
}

// isConfiguredSet reports whether a set was configured with content: null
// and a known empty set both count as "not configured" (an empty set sends
// nothing, see buildEditModel). An unknown set counts as configured, so a
// value not known until apply is never waved through.
func isConfiguredSet(s types.Set) bool {
	if s.IsNull() {
		return false
	}
	if s.IsUnknown() {
		return true
	}
	return len(s.Elements()) > 0
}

// ValidateConfig enforces, at plan time:
//   - managed_by_org_group_id is numeric;
//   - a WARNING (not an error) for criteria_type "All" with no criteria,
//     because such a smart group matches every device in the managing
//     organization group.
//
// The cross-field rules that used to reject the criteria attributes with
// criteria_type "UserDevice", and user_additions/device_additions with
// "All", were removed: they were never live-confirmed against UEM's own
// validation, so the config is now passed straight through and UEM
// validates the pairing itself, at apply time.
//
// An unknown criteria_type skips the "matches every device" check.
func (r *SmartGroupResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg SmartGroupResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if isKnownNonEmptyString(cfg.ManagedByOrgGroupID) {
		if _, err := strconv.Atoi(cfg.ManagedByOrgGroupID.ValueString()); err != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("managed_by_org_group_id"),
				"Invalid Managed By Organization Group ID",
				fmt.Sprintf("managed_by_org_group_id must be a numeric organization group id, got %q", cfg.ManagedByOrgGroupID.ValueString()),
			)
		}
	}

	if cfg.CriteriaType.IsNull() || cfg.CriteriaType.IsUnknown() || cfg.CriteriaType.ValueString() != criteriaTypeAll {
		return
	}
	criteria := cfg.criteriaSets()
	anyCriteria := false
	for _, name := range criteriaAttributes {
		if isConfiguredSet(criteria[name]) {
			anyCriteria = true
			break
		}
	}
	if !anyCriteria {
		resp.Diagnostics.AddWarning(
			"Smart Group Matches Every Device",
			fmt.Sprintf("criteria_type = %q with no criteria configured: this smart group matches EVERY device in its managing "+
				"organization group. Add criteria (for example platforms or ownerships) to narrow it, or ignore this warning "+
				"if that is intended.", criteriaTypeAll),
		)
	}
}

// smartGroupSetAttributes lists every set-valued criterion, member, or
// exclusion attribute on uem_smart_group: the criteria attributes, the
// explicit membership attributes, and the exclusion attributes. ModifyPlan
// uses this to check every one of them for a non-empty-to-empty clear.
var smartGroupSetAttributes = append(append([]string{}, criteriaAttributes...),
	"user_additions", "device_additions", "user_exclusions", "device_exclusions", "user_group_exclusions")

// ModifyPlan errors, on an update, when any criterion/member/exclusion set
// attribute goes from non-empty in state to empty or null in the plan.
//
// Live-confirmed broken (live run 2): the SDK's edit model tags every slice
// field `omitempty` (see buildEditModel in resource_state.go), so an emptied
// set is left out of the PUT body entirely rather than sent as `[]`. UEM
// then keeps the attribute's existing value instead of clearing it --
// clearing enrollment_categories left it at ["DepEnrolled"] on UEM's side,
// and Terraform reported an inconsistent result after apply. There is no way
// to ask UEM to clear one of these sets through this whole-object PUT, so
// this is caught here, before Update ever runs, instead of surfacing as a
// confusing inconsistent-result error after the API call already happened.
func (r *SmartGroupResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Create (no prior state) and destroy (no plan) have nothing to clear.
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	for _, name := range smartGroupSetAttributes {
		var stateSet, planSet types.Set
		resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root(name), &stateSet)...)
		resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root(name), &planSet)...)
		if resp.Diagnostics.HasError() {
			return
		}
		stateNonEmpty := !stateSet.IsNull() && !stateSet.IsUnknown() && len(stateSet.Elements()) > 0
		planEmptyOrNull := !planSet.IsUnknown() && (planSet.IsNull() || len(planSet.Elements()) == 0)
		if stateNonEmpty && planEmptyOrNull {
			resp.Diagnostics.AddAttributeError(
				path.Root(name),
				"Cannot Clear A Smart Group Set Attribute",
				fmt.Sprintf("%s cannot be cleared by updating this resource: UEM keeps the attribute's existing value because an "+
					"empty list can't be sent to the whole-object update endpoint. To clear %s, remove it in the UEM console, or "+
					"recreate the resource (for example `terraform apply -replace=<this resource's address>`).", name, name),
			)
		}
	}
}

// criteriaSets maps each criteria attribute name to its value.
func (m *SmartGroupResourceModel) criteriaSets() map[string]types.Set {
	return map[string]types.Set{
		"platforms":              m.Platforms,
		"ownerships":             m.Ownerships,
		"models":                 m.Models,
		"management_types":       m.ManagementTypes,
		"enrollment_categories":  m.EnrollmentCategories,
		"cpu_architectures":      m.CPUArchitectures,
		"organization_group_ids": m.OrganizationGroupIDs,
		"user_group_ids":         m.UserGroupIDs,
		"tag_ids":                m.TagIDs,
		"operating_systems":      m.OperatingSystems,
	}
}

// parseSmartGroupID converts the state id into the int the SDK takes.
func parseSmartGroupID(id types.String) (int, error) {
	v := strings.TrimSpace(id.ValueString())
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("smart group id must be a positive number, got %q", v)
	}
	return n, nil
}
