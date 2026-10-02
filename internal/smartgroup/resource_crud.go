package smartgroup

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/euc-oss/terraform-provider-uem/internal/common/notfound"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/euc-oss/terraform-sdk-uem/v26/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// numericImportIDPattern matches the numeric-id import form.
var numericImportIDPattern = regexp.MustCompile(`^[0-9]+$`)

// smartGroupUUIDPattern matches a canonical 8-4-4-4-12 hex UUID. Used only to
// dispatch an ImportState id between the numeric and UUID import forms below;
// unlike the data source's removed plan-time smart_group_uuid shape check
// (B16 (b)-row #214 removal -- that regex rejected a config value with no
// live evidence for the shape), a raw import id that matches neither pattern
// here is not silently treated as "no match": ImportState errors out, since
// there is no other way to tell the two id forms apart.
var smartGroupUUIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func (r *SmartGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider Not Configured", "The smart group service is not configured.")
		return
	}
	var plan SmartGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body, diags := buildEditModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, created, err := r.svc.CreateSmartGroupAsync(ctx, body)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to create smart group: %s", err))
		return
	}
	if created == nil || created.Value == nil {
		resp.Diagnostics.AddError("Client Error", "Smart group create response did not include the new smart group id.")
		return
	}
	id := int(*created.Value)

	_, sg, err := r.svc.LoadSmartGroupAsync(ctx, id)
	if err == nil && sg == nil {
		err = fmt.Errorf("smart group %d lookup returned no data", id)
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read smart group %d after create: %s", id, err))
		return
	}
	r.setState(ctx, sg, &plan, &resp.State, &resp.Diagnostics)
	tflog.Trace(ctx, "created a smart group resource", map[string]any{"smart_group_id": id})
}

func (r *SmartGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider Not Configured", "The smart group service is not configured.")
		return
	}
	var state SmartGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := parseSmartGroupID(state.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Smart Group ID", err.Error())
		return
	}

	sg, found, err := r.loadConfirmingNotFound(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read smart group %d: %s", id, err))
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	if len(sg.OEMAndModels) > 0 {
		resp.Diagnostics.AddWarning(
			"Unmanaged OEM and Model Criteria",
			fmt.Sprintf("Smart group %d has Manufacturer/OEM and Model criteria (OEMAndModels) set in UEM. uem_smart_group does "+
				"not support that criterion yet, so it was not imported into state and is not managed by Terraform. "+
				"Updates keep the server's current value.", id),
		)
	}
	r.setState(ctx, sg, &state, &resp.State, &resp.Diagnostics)
}

func (r *SmartGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider Not Configured", "The smart group service is not configured.")
		return
	}
	var plan, state SmartGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := parseSmartGroupID(state.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Smart Group ID", err.Error())
		return
	}

	// Read the current group first: the PUT replaces the whole object, and
	// OEMAndModels is not modelled, so its server value is carried through
	// rather than dropped.
	current, found, err := r.loadConfirmingNotFound(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read smart group %d before update: %s", id, err))
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	body, diags := buildEditModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	body.OEMAndModels = current.OEMAndModels

	if _, err := r.svc.UpdateSmartGroupAsync(ctx, id, body); err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to update smart group %d: %s", id, err))
		return
	}

	_, sg, err := r.svc.LoadSmartGroupAsync(ctx, id)
	if err == nil && sg == nil {
		err = fmt.Errorf("smart group %d lookup returned no data", id)
	}
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read smart group %d after update: %s", id, err))
		return
	}
	r.setState(ctx, sg, &plan, &resp.State, &resp.Diagnostics)
	tflog.Trace(ctx, "updated a smart group resource", map[string]any{"smart_group_id": id})
}

func (r *SmartGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.svc == nil {
		resp.Diagnostics.AddError("Provider Not Configured", "The smart group service is not configured.")
		return
	}
	var state SmartGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := parseSmartGroupID(state.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Smart Group ID", err.Error())
		return
	}
	if _, err := r.svc.DeleteAsync(ctx, id); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete smart group %d: %s", id, err))
	}
}

// ImportState accepts a bare numeric smart group id, or a smart group UUID
// resolved to its id through the shared hardened search walk
// (findSmartGroupsByUUID: exact, case-insensitive). Read then fills the rest.
func (r *SmartGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	raw := strings.TrimSpace(req.ID)
	switch {
	case numericImportIDPattern.MatchString(raw):
		if _, err := strconv.Atoi(raw); err != nil {
			resp.Diagnostics.AddError("Invalid Import ID", fmt.Sprintf("smart group id %q is out of range: %s", raw, err))
			return
		}
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), raw)...)
	case smartGroupUUIDPattern.MatchString(raw):
		id, diags := r.resolveUUID(ctx, raw)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), strconv.FormatInt(id, 10))...)
	default:
		resp.Diagnostics.AddError("Invalid Import ID",
			fmt.Sprintf("Expected a numeric smart group id or a smart group UUID (8-4-4-4-12 hex digits), got %q.", req.ID))
	}
}

// resolveUUID finds the numeric id of the one smart group whose UUID matches.
func (r *SmartGroupResource) resolveUUID(ctx context.Context, uuid string) (int64, diag.Diagnostics) {
	var diags diag.Diagnostics
	if r.svc == nil {
		diags.AddError("Provider Not Configured", "The smart group service is not configured.")
		return 0, diags
	}
	matches, err := findSmartGroupsByUUID(ctx, r.svc, uuid, r.pageSize)
	if err != nil {
		diags.AddError("Smart Group Search Failed", err.Error())
		return 0, diags
	}
	switch len(matches) {
	case 0:
		diags.AddError("Smart Group Not Found", fmt.Sprintf(
			"No smart group with UUID %q was found by UEM's smart group search. UEM can only resolve a UUID through "+
				"that search, and the search omits an organization group's own smart group (the one named after the "+
				"organization group, used when assigning to the whole group). UEM has no by-UUID read "+
				"(GET /api/mdm/smartgroups/{uuid} returns 404). If this is such a group, import it by its numeric id "+
				"instead: terraform import <address> <smart_group_id>.", uuid))
		return 0, diags
	case 1:
	default:
		diags.AddError("Ambiguous Smart Group UUID", fmt.Sprintf("%d smart groups have UUID %q; import by numeric id instead.", len(matches), uuid))
		return 0, diags
	}
	id, _ := smartGroupID(matches[0]) // the walk rejects groups without a numeric id
	return id, diags
}

// loadConfirmingNotFound loads the group, re-checking a not-found answer once
// with notfound.Confirm so a single flaky response cannot drop state. It
// returns found=false only when both answers were not-found.
func (r *SmartGroupResource) loadConfirmingNotFound(ctx context.Context, id int) (*sdk.SmartGroupV1, bool, error) {
	load := func(ctx context.Context) (*sdk.SmartGroupV1, error) {
		_, sg, err := r.svc.LoadSmartGroupAsync(ctx, id)
		if err == nil && sg == nil {
			err = fmt.Errorf("smart group %d lookup returned no data", id)
		}
		return sg, err
	}
	sg, err := load(ctx)
	if err == nil {
		return sg, true, nil
	}
	if !client.IsNotFound(err) {
		return nil, false, err
	}
	confirmed, stillNotFound, err := notfound.Confirm(ctx, client.IsNotFound, load)
	if err != nil {
		return nil, false, err
	}
	if stillNotFound {
		return nil, false, nil
	}
	tflog.Warn(ctx, fmt.Sprintf("UEM returned not-found then found for smart group %d; kept in state", id))
	return confirmed, true, nil
}

func (r *SmartGroupResource) setState(ctx context.Context, sg *sdk.SmartGroupV1, prior *SmartGroupResourceModel, state *tfsdk.State, diags *diag.Diagnostics) {
	m, d := smartGroupToModel(ctx, sg, prior)
	diags.Append(d...)
	if diags.HasError() {
		return
	}
	diags.Append(state.Set(ctx, &m)...)
}
