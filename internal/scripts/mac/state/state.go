package state

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/euc-oss/terraform-sdk-uem/v26/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	tf "github.com/euc-oss/terraform-provider-uem/internal/scripts/mac/models"
)

// ToCreateScriptV1 maps the Terraform plan model into an SDK create request.
func ToCreateScriptV1(m *tf.MacScriptResourceModel) (*sdk.CreateScriptV1, diag.Diagnostics) {
	var diags diag.Diagnostics
	if m == nil {
		return nil, diags
	}

	api := &sdk.CreateScriptV1{
		Name:       m.Name.ValueString(),
		Platform:   m.Platform.ValueString(),
		ScriptType: m.ScriptType.ValueString(),
		ScriptData: m.ScriptData.ValueString(),
	}

	setStringIfKnown(&api.Description, m.Description)
	setStringIfKnown(&api.ExecutionContext, m.ExecutionContext)
	setPlatformArchitectureIfKnown(&api.PlatformArchitecture, m.PlatformArchitecture)

	if !m.Timeout.IsNull() && !m.Timeout.IsUnknown() {
		timeout := int(m.Timeout.ValueInt64())
		api.Timeout = &timeout
	}

	if len(m.ScriptVariables) > 0 {
		api.ScriptVariables = scriptVariablesToAPI(m.ScriptVariables)
	}

	api.AllowedInCatalog = m.AllowedInCatalog.ValueBool()

	// B16 decision table row #162 (KEEP+cite, partial). UEM source:
	// BusinessImpl/src/BusinessImplSln/AirWatch.UEM.Scripts/Contract/Application/Model/V1/Validators/ModelValidatorExtensions.cs:69-74,
	// BusinessImpl/src/BusinessImplSln/AirWatch.UEM.Scripts/Contract/Types/Scripts/CatalogDisplay.cs:81-88
	// (canonical Q24): confirms catalog_display's required string fields
	// (display_name, display_desc, pre_action_text, post_action_text,
	// action_type, catalog_icon_url -- NOT categories, see B35 below) are
	// required when allowed_in_catalog (CustomizeForCatalog) is true; that
	// half is now source-confirmed (see row #163 below). Q24 does not
	// address the other half — whether the whole block should be silently
	// dropped when allowed_in_catalog is false, as this does — so that half
	// remains unconfirmed either way.
	if m.CatalogDisplay != nil {
		cd, d := catalogDisplayToAPI(m.CatalogDisplay)
		diags.Append(d...)
		if diags.HasError() {
			return nil, diags
		}
		if api.AllowedInCatalog {
			api.CatalogDisplay = cd
		}
	}

	if !m.UserInteraction.IsNull() && !m.UserInteraction.IsUnknown() {
		userInteraction := m.UserInteraction.ValueBool()
		api.UserInteraction = &userInteraction
	}

	return api, diags
}

// ToUpdateScriptV1 maps the Terraform plan model into an SDK update request.
// ScriptUUID must match the path parameter passed to ReplaceScriptDefinitionAsync.
func ToUpdateScriptV1(m *tf.MacScriptResourceModel, scriptUUID string) (*sdk.UpdateScriptV1, diag.Diagnostics) {
	createReq, diags := ToCreateScriptV1(m)
	if diags.HasError() {
		return nil, diags
	}

	orgGroupUUID, err := m.FetchValidOrganizationGroupUUID()
	if err != nil {
		diags.AddError("Invalid organization_group_uuid", err.Error())
		return nil, diags
	}

	// Faithful pass-through, exactly like Create: a null/unknown plan value
	// omits Timeout from the request rather than substituting a fabricated
	// default. There is no UEM source, and nothing live-confirmed, showing
	// that a PUT requires Timeout or that omitting it resets the value to
	// 30; that claim (and the force-send below) was never cited. Separately,
	// since B34 timeout is Optional+Computed with UseStateForUnknown, so a
	// real plan that omits timeout against known state carries the prior
	// value forward as a known, non-null value -- this branch is not reached
	// by a real Terraform plan at all, only by a literal `timeout = null`.
	// user_interaction is not Optional+Computed's UseStateForUnknown-carried
	// case here: it is passed straight through from createReq like every
	// other omittable field.

	return &sdk.UpdateScriptV1{
		AllowedInCatalog:      &createReq.AllowedInCatalog,
		CatalogDisplay:        createReq.CatalogDisplay,
		Description:           createReq.Description,
		ExecutionContext:      createReq.ExecutionContext,
		Name:                  createReq.Name,
		OrganizationGroupUUID: orgGroupUUID,
		Platform:              createReq.Platform,
		PlatformArchitecture:  createReq.PlatformArchitecture,
		ScriptData:            createReq.ScriptData,
		ScriptType:            createReq.ScriptType,
		ScriptUUID:            scriptUUID,
		ScriptVariables:       createReq.ScriptVariables,
		Timeout:               createReq.Timeout,
		UserInteraction:       createReq.UserInteraction,
	}, diags
}

// timeoutFromAPI maps the API's Timeout onto the Optional+Computed timeout
// attribute, verbatim: UEM's response is authoritative, so an explicit
// server default of 30 reads back as 30 exactly like any other
// value. This used to collapse an API-default 30 to null whenever the prior
// value (prior state on refresh, the planned value on the post-apply
// readback, or -- always -- null on import) was itself null, on the theory
// that a null prior meant "configuration omitted timeout". That theory does
// not hold on import: ImportState only ever sets id (internal/scripts/mac/
// crud.go), so the prior is unconditionally null there regardless of what
// UEM actually holds, and the collapse silently discarded a real, present
// value that happened to equal UEM's default (B34). The schema's Computed +
// UseStateForUnknown plan modifier is what now keeps a configuration that
// omits timeout from showing a diff once a value is in state, so this
// function no longer needs to fabricate null to get the same effect.
func timeoutFromAPI(api *int) types.Int64 {
	if api == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*api))
}

// userInteractionFromAPI maps the API's UserInteraction onto the
// Optional+Computed user_interaction attribute, verbatim: UEM's response is
// authoritative, so an explicit false reads back as false exactly like any
// other value. This used to collapse an API-default false to null whenever
// the prior value was itself null, on the theory that a null prior meant
// "configuration omitted user_interaction". That theory does not hold on
// import: ImportState only ever sets id (internal/scripts/mac/crud.go), so
// the prior is unconditionally null there regardless of what UEM actually
// holds, and the collapse silently discarded a real, present value that
// happened to equal UEM's default (B34). The schema's Computed +
// UseStateForUnknown plan modifier is what now keeps a configuration that
// omits user_interaction from showing a diff once a value is in state, so
// this function no longer needs to fabricate null to get the same effect.
// Server-default behaviour live-confirmed 2026-09-24 on as<internal-env> 26.2 (1bp
// gate, scope item C1). Evidence:
// internal-design-doc
func userInteractionFromAPI(api *bool) types.Bool {
	if api == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*api)
}

// platformArchitectureFromAPI maps the API's PlatformArchitecture onto the
// Optional-only platform_architecture attribute. QUIRK-11: the server returns
// the int 0 when the architecture is unset and a named string (e.g. "LEGACY")
// when set. Only that int 0 "unset" default maps to null (the provider only
// ever writes the string form); any other int keeps its string form (e.g.
// "2") and a string is taken as-is, so drift shows. Live-confirmed
// 2026-09-24 on as<internal-env> 26.2 (1bp gate, scope item C3); the int-0-only mapping
// was itself a gate follow-up fix (commit 340a76f73, 2026-09-24). Evidence:
// internal-design-doc
func platformArchitectureFromAPI(api *client.IntOrString) types.String {
	if api == nil || (!api.IsStr && api.IntVal == 0) {
		return types.StringNull()
	}
	return types.StringValue(api.String())
}

// ReadAPIIntoState maps a live GetScriptAsync response into the Terraform state model.
// ScriptUUID is written to data.ID; callers may pass api.ScriptUUID when present.
func ReadAPIIntoState(ctx context.Context, data *tf.MacScriptResourceModel, api *sdk.ScriptResourceV1, scriptUUID string) diag.Diagnostics {
	var diags diag.Diagnostics
	if data == nil || api == nil {
		return diags
	}

	if scriptUUID == "" {
		scriptUUID = api.ScriptUUID
	}
	data.ID = types.StringValue(scriptUUID)
	// The API is authoritative: live GET /api/mdm/scripts/{uuid} always returns
	// organization_group_uuid (v1n.4.1, verified on 26.2), so an import-time Read
	// fills this Required attribute and the post-import plan is a no-op.
	data.OrganizationGroupUUID = types.StringValue(api.OrganizationGroupUUID)
	data.Name = types.StringValue(api.Name)
	// B16 (b)-row #161 removal: store the server value exactly as returned,
	// with no prior-based disambiguation. UEM's wire model represents "no
	// description" as "" (a non-pointer string), so a script created/updated
	// with description unset reads back as "" here, not null -- see the B16
	// report's EXPECTED DIFF RISK for this row.
	data.Description = types.StringValue(api.Description)
	data.Platform = types.StringValue(api.Platform)
	data.ScriptType = types.StringValue(api.ScriptType)
	data.ExecutionContext = types.StringValue(api.ExecutionContext)
	data.PlatformArchitecture = platformArchitectureFromAPI(api.PlatformArchitecture)
	data.ScriptData = types.StringValue(api.ScriptData)

	data.Timeout = timeoutFromAPI(api.Timeout)

	data.ScriptVariables = scriptVariablesFromAPI(api.ScriptVariables)

	if api.AllowedInCatalog != nil {
		data.AllowedInCatalog = types.BoolValue(*api.AllowedInCatalog)
	} else {
		data.AllowedInCatalog = types.BoolNull()
	}

	data.CatalogDisplay = catalogDisplayFromAPI(api.CatalogDisplay)

	data.UserInteraction = userInteractionFromAPI(api.UserInteraction)

	return diags
}

// scriptVariablesToAPI maps script_variables into the create/update request.
//
// B16 decision table row #164 (REMOVE — dead code cleanup, not a UEM-
// fidelity question). This previously skipped any entry whose name was null
// or unknown, but internal/scripts/mac/schema.go's script_variables.name is
// schema-Required, so the framework itself already rejects a config with a
// null name before this function ever runs, and a Required attribute is
// always known by apply time — that branch was unreachable. Every entry is
// now passed through as-is.
func scriptVariablesToAPI(vars []tf.MacScriptVariableModel) []sdk.ScriptVariablesV1 {
	out := make([]sdk.ScriptVariablesV1, 0, len(vars))
	for _, v := range vars {
		out = append(out, sdk.ScriptVariablesV1{
			Name:  v.Name.ValueString(),
			Value: v.Value.ValueString(),
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func scriptVariablesFromAPI(vars []sdk.ScriptVariablesV1) []tf.MacScriptVariableModel {
	if len(vars) == 0 {
		return nil
	}
	out := make([]tf.MacScriptVariableModel, len(vars))
	for i, v := range vars {
		out[i] = tf.MacScriptVariableModel{
			Name:  types.StringValue(v.Name),
			Value: types.StringValue(v.Value),
		}
	}
	return out
}

func catalogDisplayToAPI(m *tf.MacCatalogDisplayModel) (*sdk.CatalogDisplayV1, diag.Diagnostics) {
	var diags diag.Diagnostics
	if m == nil {
		return nil, diags
	}

	api := &sdk.CatalogDisplayV1{}

	var d diag.Diagnostic
	api.DisplayName, d = requireCatalogString("display_name", m.DisplayName)
	diags.Append(d)
	api.DisplayDesc, d = requireCatalogString("display_desc", m.DisplayDesc)
	diags.Append(d)
	api.PreActionText, d = requireCatalogString("pre_action_text", m.PreActionText)
	diags.Append(d)
	api.PostActionText, d = requireCatalogString("post_action_text", m.PostActionText)
	diags.Append(d)
	api.ActionType, d = requireCatalogString("action_type", m.ActionType)
	diags.Append(d)
	api.CatalogIconURL, d = requireCatalogString("catalog_icon_url", m.CatalogIconURL)
	diags.Append(d)

	if diags.HasError() {
		return nil, diags
	}

	// B35/uix: categories is Optional+Computed (see schema.go and the B16
	// comment above; Q24 never mentions categories) with
	// UseStateForUnknown, so the model field is a types.List, not a Go
	// slice: an omitted config plans Unknown, which req.Plan.Get cannot
	// decode into a slice. Null or unknown (config omitted it, or this is
	// a create with no prior state to carry forward) sends nothing, so an
	// unset categories leaves whatever UEM already has; a known (possibly
	// empty) list is sent as given. NOTE: the SDK's CatalogDisplayV1.Categories
	// field is tagged `json:"categories,omitempty"`, so encoding/json drops it
	// from the request body whenever the built slice has zero elements even
	// if it is non-nil here -- an explicit `categories = []` in config cannot
	// currently be distinguished on the wire from omitting categories
	// entirely. Fixing that requires an SDK change, out of scope here.
	if !m.Categories.IsNull() && !m.Categories.IsUnknown() {
		categories, d := categoryStringsToAPI(m.Categories)
		diags.Append(d...)
		if diags.HasError() {
			return nil, diags
		}
		api.Categories = categories
	}

	return api, diags
}

func catalogDisplayFromAPI(api *sdk.CatalogDisplayV1) *tf.MacCatalogDisplayModel {
	if api == nil {
		return nil
	}

	out := &tf.MacCatalogDisplayModel{
		DisplayName:    types.StringValue(api.DisplayName),
		DisplayDesc:    types.StringValue(api.DisplayDesc),
		PreActionText:  types.StringValue(api.PreActionText),
		PostActionText: types.StringValue(api.PostActionText),
		ActionType:     types.StringValue(api.ActionType),
		CatalogIconURL: types.StringValue(api.CatalogIconURL),
	}

	// B35/uix: read back verbatim. A nil api.Categories (the field absent
	// from UEM's response) becomes types.ListNull, which Terraform
	// serializes as null. A non-nil api.Categories -- including a non-nil,
	// zero-length one, which is exactly what UEM returns for a script that
	// is in the catalog with no categories assigned (live-confirmed) --
	// becomes a known (possibly empty) types.List via ListValueMust, never
	// null. Previously this only ran when len(api.Categories) > 0, so a live
	// "categories": [] silently became null, tripping the (now-removed)
	// Required schema constraint.
	if api.Categories != nil {
		values := make([]attr.Value, 0, len(api.Categories))
		for _, c := range api.Categories {
			if c == nil {
				continue
			}
			values = append(values, types.StringValue(strconv.Itoa(*c)))
		}
		out.Categories = types.ListValueMust(types.StringType, values)
	} else {
		out.Categories = types.ListNull(types.StringType)
	}

	return out
}

// categoryStringsToAPI converts a known categories list config value into the
// SDK's []*int shape. It is only ever called (see catalogDisplayToAPI above)
// when the caller's categories is itself known (neither null nor unknown), so
// it always returns a non-nil slice -- including a non-nil, zero-length one
// for a config that sets categories = [] -- rather than collapsing an empty
// result to nil; see catalogDisplayToAPI's SDK omitempty caveat for why that
// distinction currently can't survive onto the wire regardless.
func categoryStringsToAPI(categories types.List) ([]*int, diag.Diagnostics) {
	var diags diag.Diagnostics
	elems := categories.Elements()
	out := make([]*int, 0, len(elems))
	for _, e := range elems {
		c, ok := e.(types.String)
		if !ok {
			diags.AddError(
				"Invalid catalog category",
				fmt.Sprintf("catalog_display.categories element has unexpected type %T", e),
			)
			continue
		}
		if c.IsNull() || c.IsUnknown() {
			continue
		}
		n, err := strconv.Atoi(c.ValueString())
		if err != nil {
			diags.AddError(
				"Invalid catalog category",
				fmt.Sprintf("catalog_display.categories value %q must be a whole number: %s", c.ValueString(), err),
			)
			continue
		}
		nCopy := n
		out = append(out, &nCopy)
	}
	return out, diags
}

func setStringIfKnown(dst *string, s types.String) {
	if s.IsNull() || s.IsUnknown() {
		return
	}
	*dst = s.ValueString()
}

// setPlatformArchitectureIfKnown constructs a client.IntOrString from a known
// Terraform string value. PlatformArchitecture is a QUIRK-11 polymorphic field
// (see client.IntOrString doc) that the wire API returns as int 0 when unset or
// a named string (e.g. "LEGACY") when set; the provider only ever writes the
// string form.
func setPlatformArchitectureIfKnown(dst **client.IntOrString, s types.String) {
	if s.IsNull() || s.IsUnknown() {
		return
	}
	*dst = &client.IntOrString{IsStr: true, StrVal: s.ValueString()}
}

// requireCatalogString requires each catalog_display field named at its call
// sites above (display_name, display_desc, pre_action_text, post_action_text,
// action_type, catalog_icon_url) to be a known, non-empty (post-trim) string.
//
// B16 decision table row #163 (KEEP+cite). UEM source:
// BusinessImpl/src/BusinessImplSln/AirWatch.UEM.Scripts/Contract/Types/Scripts/CatalogDisplay.cs:81-88
// (canonical Q24): CatalogDisplay.IsValid() requires DisplayName, ActionType,
// PostActionText, and PreActionText to be non-empty, plus CatalogIconPath
// non-empty UNLESS UseDefaultIcon is true. This provider's schema has no
// use_default_icon attribute, so catalog_icon_url is required
// unconditionally here (stricter than IsValid()'s icon-path-or-default-icon
// exemption); display_desc is also required unconditionally here, though
// IsValid() does not list DisplayDesc as required at all. Trimming itself
// has no server citation but is harmless client hygiene.
func requireCatalogString(field string, s types.String) (string, diag.Diagnostic) {
	if s.IsNull() || s.IsUnknown() {
		return "", diag.NewErrorDiagnostic(
			"Missing catalog display attribute",
			fmt.Sprintf("catalog_display.%s is required when catalog_display is set", field),
		)
	}
	value := strings.TrimSpace(s.ValueString())
	if value == "" {
		return "", diag.NewErrorDiagnostic(
			"Invalid catalog display attribute",
			fmt.Sprintf("catalog_display.%s must not be empty", field),
		)
	}
	return value, nil
}
