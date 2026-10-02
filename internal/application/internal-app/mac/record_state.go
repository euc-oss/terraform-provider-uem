package macapplication

import (
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/internal-app/mac/models"
)

// The application record UEM reports on read (F9). None of these fields is
// writable through the API: the only create body,
// sdk.MacOsCreateApplicationRequestV1Model, carries just the application,
// icon and pkginfo blob ids plus version, and the SDK has no update endpoint
// for the record. So every field here is Computed and passed through exactly
// as UEM returns it ("" stays "", a missing number is null).
//
// Deliberately not modelled: Assignments and ExcludedSmartGroup* (owned by
// uem_application_assignment), the Devices*Count counters (they change as
// devices check in), AirwatchAppVersion (already app_version),
// ApplicationFileBlobGUID (upload plumbing), and the Windows/iOS-only shapes
// DeploymentOptions, FilesOptions, MsiDeploymentParameters and RenewalDate,
// none of which a macOS app has been seen to return.

// recordStringField maps one string field of the V1 record to its attribute.
type recordStringField struct {
	name, jsonKey string
	get           func(*sdk.InternalAppModelV1) string
	set           func(*tf.MacApplicationResourceModel, types.String)
}

// recordIntField maps one *int field of the V1 record to its attribute.
type recordIntField struct {
	name, jsonKey string
	get           func(*sdk.InternalAppModelV1) *int
	set           func(*tf.MacApplicationResourceModel, types.Int64)
}

var recordStringFields = []recordStringField{
	{"actual_file_version", "ActualFileVersion", func(a *sdk.InternalAppModelV1) string { return a.ActualFileVersion }, func(m *tf.MacApplicationResourceModel, v types.String) { m.ActualFileVersion = v }},
	{"app_id", "AppId", func(a *sdk.InternalAppModelV1) string { return a.AppID }, func(m *tf.MacApplicationResourceModel, v types.String) { m.AppIDValue = v }},
	{"app_provisioning_profile_uuid", "AppProvisioningProfileUuid", func(a *sdk.InternalAppModelV1) string { return a.AppProvisioningProfileUUID }, func(m *tf.MacApplicationResourceModel, v types.String) { m.AppProvisioningProfileUUID = v }},
	{"application_file_hash", "ApplicationFileHash", func(a *sdk.InternalAppModelV1) string { return a.ApplicationFileHash }, func(m *tf.MacApplicationResourceModel, v types.String) { m.ApplicationFileHash = v }},
	{"application_name", "ApplicationName", func(a *sdk.InternalAppModelV1) string { return a.ApplicationName }, func(m *tf.MacApplicationResourceModel, v types.String) { m.ApplicationName = v }},
	{"application_url", "ApplicationUrl", func(a *sdk.InternalAppModelV1) string { return a.ApplicationURL }, func(m *tf.MacApplicationResourceModel, v types.String) { m.ApplicationURL = v }},
	{"assume_management_of_user_installed_app", "AssumeManagementOfUserInstalledApp", func(a *sdk.InternalAppModelV1) string { return a.AssumeManagementOfUserInstalledApp }, func(m *tf.MacApplicationResourceModel, v types.String) { m.AssumeManagementOfUserInstalledApp = v }},
	{"build_version", "BuildVersion", func(a *sdk.InternalAppModelV1) string { return a.BuildVersion }, func(m *tf.MacApplicationResourceModel, v types.String) { m.BuildVersion = v }},
	{"change_log", "ChangeLog", func(a *sdk.InternalAppModelV1) string { return a.ChangeLog }, func(m *tf.MacApplicationResourceModel, v types.String) { m.ChangeLog = v }},
	{"comments", "Comments", func(a *sdk.InternalAppModelV1) string { return a.Comments }, func(m *tf.MacApplicationResourceModel, v types.String) { m.Comments = v }},
	{"display_name", "DisplayName", func(a *sdk.InternalAppModelV1) string { return a.DisplayName }, func(m *tf.MacApplicationResourceModel, v types.String) { m.DisplayName = v }},
	{"large_icon_blob_guid", "LargeIconBlobGUID", func(a *sdk.InternalAppModelV1) string { return a.LargeIconBlobGUID }, func(m *tf.MacApplicationResourceModel, v types.String) { m.LargeIconBlobGUID = v }},
	{"launch_command", "LaunchCommand", func(a *sdk.InternalAppModelV1) string { return a.LaunchCommand }, func(m *tf.MacApplicationResourceModel, v types.String) { m.LaunchCommand = v }},
	{"launch_type", "LaunchType", func(a *sdk.InternalAppModelV1) string { return a.LaunchType }, func(m *tf.MacApplicationResourceModel, v types.String) { m.LaunchType = v }},
	{"managed_by", "ManagedBy", func(a *sdk.InternalAppModelV1) string { return a.ManagedBy }, func(m *tf.MacApplicationResourceModel, v types.String) { m.ManagedBy = v }},
	{"managed_by_uuid", "ManagedByUuid", func(a *sdk.InternalAppModelV1) string { return a.ManagedByUUID }, func(m *tf.MacApplicationResourceModel, v types.String) { m.ManagedByUUID = v }},
	{"medium_icon_blob_guid", "MediumIconBlobGUID", func(a *sdk.InternalAppModelV1) string { return a.MediumIconBlobGUID }, func(m *tf.MacApplicationResourceModel, v types.String) { m.MediumIconBlobGUID = v }},
	{"minimum_operating_system", "MinimumOperatingSystem", func(a *sdk.InternalAppModelV1) string { return a.MinimumOperatingSystem }, func(m *tf.MacApplicationResourceModel, v types.String) { m.MinimumOperatingSystem = v }},
	{"platform", "Platform", func(a *sdk.InternalAppModelV1) string { return a.Platform }, func(m *tf.MacApplicationResourceModel, v types.String) { m.Platform = v }},
	{"sdk", "Sdk", func(a *sdk.InternalAppModelV1) string { return a.Sdk }, func(m *tf.MacApplicationResourceModel, v types.String) { m.Sdk = v }},
	{"sdk_profile_uuid", "SdkProfileUuid", func(a *sdk.InternalAppModelV1) string { return a.SdkProfileUUID }, func(m *tf.MacApplicationResourceModel, v types.String) { m.SdkProfileUUID = v }},
	{"small_icon_blob_guid", "SmallIconBlobGUID", func(a *sdk.InternalAppModelV1) string { return a.SmallIconBlobGUID }, func(m *tf.MacApplicationResourceModel, v types.String) { m.SmallIconBlobGUID = v }},
	{"status", "Status", func(a *sdk.InternalAppModelV1) string { return a.Status }, func(m *tf.MacApplicationResourceModel, v types.String) { m.Status = v }},
}

var recordIntFields = []recordIntField{
	{"app_size_in_kb", "AppSizeInKB", func(a *sdk.InternalAppModelV1) *int { return a.AppSizeInKB }, func(m *tf.MacApplicationResourceModel, v types.Int64) { m.AppSizeInKB = v }},
	{"rating", "Rating", func(a *sdk.InternalAppModelV1) *int { return a.Rating }, func(m *tf.MacApplicationResourceModel, v types.Int64) { m.Rating = v }},
	{"sdk_profile_id", "SdkProfileId", func(a *sdk.InternalAppModelV1) *int { return a.SdkProfileID }, func(m *tf.MacApplicationResourceModel, v types.Int64) { m.SdkProfileID = v }},
}

// namedRefAttrTypes is one entry of category_list / supported_models
// (ApplicationCategoriesModelV1 / ApplicationSupportedModelsModelV1: Name,
// id, uuid).
var namedRefAttrTypes = map[string]attr.Type{
	"name": types.StringType,
	"id":   types.Int64Type,
	"uuid": types.StringType,
}

// deploymentSummaryAttrTypes is MacOsSoftwareDeploymentSummaryModelV1.
var deploymentSummaryAttrTypes = map[string]attr.Type{
	"is_managed": types.StringType,
	"pkginfo":    types.StringType,
}

func readOnlyDescription(jsonKey string) string {
	return "Read-only. Reported by UEM (`" + jsonKey + "`); not settable through the API, which accepts only the " +
		"application, icon and pkginfo files and the version at create and has no update for the record."
}

// recordSchemaAttributes returns the Computed attributes for the record.
func recordSchemaAttributes() map[string]schema.Attribute {
	out := map[string]schema.Attribute{}
	for _, f := range recordStringFields {
		out[f.name] = schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: readOnlyDescription(f.jsonKey),
			PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		}
	}
	for _, f := range recordIntFields {
		out[f.name] = schema.Int64Attribute{
			Computed:            true,
			MarkdownDescription: readOnlyDescription(f.jsonKey),
			PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
		}
	}
	namedRef := func(jsonKey string) schema.ListNestedAttribute {
		return schema.ListNestedAttribute{
			Computed:            true,
			MarkdownDescription: readOnlyDescription(jsonKey),
			PlanModifiers:       []planmodifier.List{listplanmodifier.UseStateForUnknown()},
			NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
				"name": schema.StringAttribute{Computed: true, MarkdownDescription: "`Name`"},
				"id":   schema.Int64Attribute{Computed: true, MarkdownDescription: "`id`"},
				"uuid": schema.StringAttribute{Computed: true, MarkdownDescription: "`uuid`"},
			}},
		}
	}
	out["category_list"] = namedRef("CategoryList")
	out["supported_models"] = namedRef("SupportedModels")
	out["supported_models_name"] = schema.ListAttribute{
		Computed:            true,
		ElementType:         types.StringType,
		MarkdownDescription: readOnlyDescription("SupportedModelsName"),
		PlanModifiers:       []planmodifier.List{listplanmodifier.UseStateForUnknown()},
	}
	out["mac_os_software_deployment_summary"] = schema.SingleNestedAttribute{
		Computed:            true,
		MarkdownDescription: readOnlyDescription("MacOsSoftwareDeploymentSummary"),
		PlanModifiers:       []planmodifier.Object{objectplanmodifier.UseStateForUnknown()},
		Attributes: map[string]schema.Attribute{
			"is_managed": schema.StringAttribute{Computed: true, MarkdownDescription: "`IsManaged`"},
			"pkginfo":    schema.StringAttribute{Computed: true, MarkdownDescription: "`Pkginfo`: the application's pkginfo plist XML, as UEM stores it."},
		},
	}
	return out
}

func intValue(p *int) types.Int64 {
	if p == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*p))
}

func namedRefList(n int, at func(i int) (string, *int, string)) (types.List, diag.Diagnostics) {
	elemType := types.ObjectType{AttrTypes: namedRefAttrTypes}
	elems := make([]attr.Value, 0, n)
	var diags diag.Diagnostics
	for i := 0; i < n; i++ {
		name, id, uuid := at(i)
		obj, d := types.ObjectValue(namedRefAttrTypes, map[string]attr.Value{
			"name": types.StringValue(name),
			"id":   intValue(id),
			"uuid": types.StringValue(uuid),
		})
		diags.Append(d...)
		elems = append(elems, obj)
	}
	list, d := types.ListValue(elemType, elems)
	diags.Append(d...)
	return list, diags
}

// setRecordFromV1 fills every record attribute from a V1 read of the app.
func setRecordFromV1(m *tf.MacApplicationResourceModel, a *sdk.InternalAppModelV1) diag.Diagnostics {
	var diags diag.Diagnostics
	for _, f := range recordStringFields {
		f.set(m, types.StringValue(f.get(a)))
	}
	for _, f := range recordIntFields {
		f.set(m, intValue(f.get(a)))
	}

	cats, d := namedRefList(len(a.CategoryList), func(i int) (string, *int, string) {
		c := a.CategoryList[i]
		return c.Name, c.ID, c.UUID
	})
	diags.Append(d...)
	m.CategoryList = cats

	models, d := namedRefList(len(a.SupportedModels), func(i int) (string, *int, string) {
		s := a.SupportedModels[i]
		return s.Name, s.ID, s.UUID
	})
	diags.Append(d...)
	m.SupportedModels = models

	names := make([]attr.Value, 0, len(a.SupportedModelsName))
	for _, n := range a.SupportedModelsName {
		names = append(names, types.StringValue(n))
	}
	nameList, d := types.ListValue(types.StringType, names)
	diags.Append(d...)
	m.SupportedModelsName = nameList

	if s := a.MacOsSoftwareDeploymentSummary; s != nil {
		obj, d := types.ObjectValue(deploymentSummaryAttrTypes, map[string]attr.Value{
			"is_managed": types.StringValue(s.IsManaged),
			"pkginfo":    types.StringValue(s.Pkginfo),
		})
		diags.Append(d...)
		m.MacOsSoftwareDeploymentSummary = obj
	} else {
		m.MacOsSoftwareDeploymentSummary = types.ObjectNull(deploymentSummaryAttrTypes)
	}
	return diags
}
