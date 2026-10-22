package macscript

import (
	"context"
	"fmt"
	"net/http"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"

	commonerrors "github.com/euc-oss/terraform-provider-uem/internal/common/errors"
)

type ScriptServiceV1API interface {
	CreateScriptAsync(
		ctx context.Context,
		OrganizationGroupUUID string,
		request *sdk.CreateScriptV1,
	) (http.Header, error)

	GetScriptAsync(
		ctx context.Context,
		ScriptUUID string,
	) (http.Header, *sdk.ScriptResourceV1, error)

	ScriptBulkDeleteAsync(
		ctx context.Context,
		OrganizationGroupUUID string,
		request *[]string,
	) (http.Header, *sdk.DeleteScriptResourceV1, error)

	ReplaceScriptDefinitionAsync(
		ctx context.Context,
		ScriptUUID string,
		request *sdk.UpdateScriptV1,
	) (http.Header, error)

	GetScriptsByOrganizationGroupAsync(
		ctx context.Context,
		OrganizationGroupUUID string,
		opts *sdk.ScriptsV1GetScriptsByOrganizationGroupAsyncOptions,
	) (http.Header, *sdk.ScriptsSearchResultV1, error)
}

type resourceConfigData struct {
	client                *sdk.Client
	newScriptServiceV1API func(c *sdk.Client) ScriptServiceV1API
}

func defaulScriptServiceV1Factory(c *sdk.Client) ScriptServiceV1API {
	return sdk.NewScriptsV1Service(c)
}

func isNotFoundAPIError(err error) bool {
	return commonerrors.IsNotFoundAPIError(err)
}

// isAmbiguousGoneAPIError reports whether err is the exact HTTP 500 with
// errorCode "1000" that UEM 26.2 returns for a GET (and assignments GET) of
// a deleted script. The same shape can also be a genuine server fault, so
// it is ambiguous: callers must confirm absence with scriptAbsentFromOrgGroup
// before treating it as not-found. Other 500s never match.
func isAmbiguousGoneAPIError(err error) bool {
	return commonerrors.IsAPIErrorMatch(err, http.StatusInternalServerError, "1000", "")
}

func resourceTypeErrorDetail(got any) string {
	return fmt.Sprintf(
		"Expected *resourceConfigData or *sdk.Client, got: %T. Please report this issue to the provider developers.",
		got,
	)
}
