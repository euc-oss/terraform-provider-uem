package macscript

import (
	"context"
	"fmt"

	tf "github.com/euc-oss/terraform-provider-uem/internal/scripts/mac/models"
	scriptstate "github.com/euc-oss/terraform-provider-uem/internal/scripts/mac/state"
	"github.com/euc-oss/terraform-provider-uem/internal/scripts/scriptlookup"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

func (r *macscriptResource) createMacScript(
	ctx context.Context,
	orgGroupUUID string,
	createReq *sdk.CreateScriptV1,
) (string, error) {
	svc, err := r.MacScriptAppService(ctx)
	if err != nil {
		return "", err
	}

	headers, err := svc.CreateScriptAsync(ctx, orgGroupUUID, createReq)
	if err != nil {
		return "", fmt.Errorf("unable to create mac script: %w", err)
	}

	scriptUUID, err := sdk.ParseLocationID(headers.Get("Location"))
	if err != nil {
		return "", fmt.Errorf("unable to parse script UUID from Location header: %w", err)
	}

	return scriptUUID, nil
}

func (r *macscriptResource) fetchMacscriptDetails(
	ctx context.Context,
	scriptUUID string,
) (*sdk.ScriptResourceV1, error) {
	svc, err := r.MacScriptAppService(ctx)
	if err != nil {
		return nil, err
	}

	_, script, err := svc.GetScriptAsync(ctx, scriptUUID)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch mac script %s: %w", scriptUUID, err)
	}

	return script, nil
}

func (r *macscriptResource) updateMacScript(
	ctx context.Context,
	scriptUUID string,
	updateReq *sdk.UpdateScriptV1,
) error {
	svc, err := r.MacScriptAppService(ctx)
	if err != nil {
		return err
	}

	_, err = svc.ReplaceScriptDefinitionAsync(ctx, scriptUUID, updateReq)
	if err != nil {
		return fmt.Errorf("unable to update mac script %s: %w", scriptUUID, err)
	}

	return nil
}

func (r *macscriptResource) deleteMacScript(
	ctx context.Context,
	orgGroupUUID string,
	scriptUUID string,
) error {
	svc, err := r.MacScriptAppService(ctx)
	if err != nil {
		return err
	}

	uuids := []string{scriptUUID}
	_, _, err = svc.ScriptBulkDeleteAsync(ctx, orgGroupUUID, &uuids)
	if err != nil {
		return fmt.Errorf("unable to delete mac script %s: %w", scriptUUID, err)
	}

	return nil
}

func (r *macscriptResource) refreshIntoState(
	ctx context.Context,
	scriptUUID string,
	api *sdk.ScriptResourceV1,
	data *tf.MacScriptResourceModel,
	state *tfsdk.State,
	diags *diag.Diagnostics,
) {
	diags.Append(scriptstate.ReadAPIIntoState(ctx, data, api, scriptUUID)...)
	if diags.HasError() {
		return
	}
	diags.Append(state.Set(ctx, data)...)
}

// scriptListPageSize is the page size used when paging the OG-scoped script
// list. It is a var only so tests can exercise multi-page paging cheaply.
var scriptListPageSize = scriptlookup.DefaultPageSize

// scriptListMaxPages is the last-resort cap on pages walked; see
// scriptlookup.DefaultMaxPages. It is a var only so tests can shrink it.
var scriptListMaxPages = scriptlookup.DefaultMaxPages

// scriptAbsentFromOrgGroup reports whether scriptUUID is confirmed absent
// from the organization group's script list. It delegates to
// scriptlookup.AbsentFromOrgGroup, which documents the termination rule;
// only (true, nil) means absent.
func scriptAbsentFromOrgGroup(
	ctx context.Context,
	svc ScriptServiceV1API,
	orgGroupUUID string,
	scriptUUID string,
) (bool, error) {
	return scriptlookup.AbsentFromOrgGroup(ctx, svc, orgGroupUUID, scriptUUID, scriptListPageSize, scriptListMaxPages)
}

// confirmScriptGone runs the confirming list lookup after an ambiguous
// 500/1000 and reports whether the script is confirmed absent. It fails
// closed: an invalid organization group, a lookup error or a found script
// all return false.
func (r *macscriptResource) confirmScriptGone(
	ctx context.Context,
	data *tf.MacScriptResourceModel,
	scriptUUID string,
) bool {
	orgGroupUUID, err := data.FetchValidOrganizationGroupUUID()
	if err != nil {
		tflog.Debug(ctx, "cannot confirm macOS script absence: no valid organization_group_uuid in state", map[string]any{
			"script_uuid": scriptUUID,
		})
		return false
	}
	svc, err := r.MacScriptAppService(ctx)
	if err != nil {
		return false
	}
	absent, err := scriptAbsentFromOrgGroup(ctx, svc, orgGroupUUID, scriptUUID)
	tflog.Debug(ctx, "confirming lookup for macOS script after 500/1000", map[string]any{
		"script_uuid":             scriptUUID,
		"organization_group_uuid": orgGroupUUID,
		"absent":                  absent,
		"lookup_error":            fmt.Sprint(err),
	})
	return err == nil && absent
}
