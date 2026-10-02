package assignment

import (
	"context"
	"fmt"
	"net/http"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"

	commonerrors "github.com/euc-oss/terraform-provider-uem/internal/common/errors"
)

type scriptAssignmentServiceAPI interface {
	AddScriptAssignmentAsync(
		ctx context.Context,
		ScriptUUID string,
		request *sdk.CreateScriptAssignmentV1,
	) (http.Header, error)
	BulkUpdateScriptAssignmentsAsync(
		ctx context.Context,
		ScriptUUID string,
		request *sdk.BulkUpdateScriptAssignmentV1,
	) (http.Header, error)
	GetScriptAssignmentAsync(
		ctx context.Context,
		AssignmentUUID string,
	) (http.Header, *sdk.ScriptAssignmentResourceV1, error)
	GetScriptAssignmentsAsync(
		ctx context.Context,
		ScriptUUID string,
	) (http.Header, *sdk.ScriptAssignmentsSearchResultV1, error)
}

// scriptServiceAPI is the subset of the scripts service the assignment
// resource needs: reading the parent script's organization group, and the
// OG-scoped list that confirms a script is gone after an ambiguous 500.
type scriptServiceAPI interface {
	GetScriptAsync(
		ctx context.Context,
		ScriptUUID string,
	) (http.Header, *sdk.ScriptResourceV1, error)
	GetScriptsByOrganizationGroupAsync(
		ctx context.Context,
		OrganizationGroupUUID string,
		opts *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions,
	) (http.Header, *sdk.ScriptsSearchResultV1, error)
}

type resourceConfigData struct {
	client                     *sdk.Client
	newScriptAssignmentService func(c *sdk.Client) scriptAssignmentServiceAPI
	newScriptService           func(c *sdk.Client) scriptServiceAPI
}

func defaultScriptAssignmentServiceFactory(c *sdk.Client) scriptAssignmentServiceAPI {
	return sdk.NewScriptAssignmentV1Service(c)
}

func defaultScriptServiceFactory(c *sdk.Client) scriptServiceAPI {
	return sdk.NewScriptsV1Service(c)
}

func isNotFoundAPIError(err error) bool {
	return commonerrors.IsNotFoundAPIError(err)
}

// isAmbiguousGoneAPIError reports whether err is the exact HTTP 500 with
// errorCode "1000" that UEM 26.2 returns for the assignments of a deleted
// script. The same shape can also be a genuine server fault, so callers must
// confirm absence with scriptAbsentFromOrgGroup before treating it as
// not-found. Other 500s never match. It deliberately duplicates the
// macscript package's classifier, matching the per-package helper
// convention used for isNotFoundAPIError.
func isAmbiguousGoneAPIError(err error) bool {
	return commonerrors.IsAPIErrorMatch(err, http.StatusInternalServerError, "1000", "")
}

func resourceTypeErrorDetail(got any) string {
	return fmt.Sprintf(
		"Expected *resourceConfigData or *sdk.Client, got: %T. Please report this issue to the provider developers.",
		got,
	)
}
