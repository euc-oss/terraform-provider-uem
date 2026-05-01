package assignment

import (
	"context"
	"fmt"
	"net/http"

	sdk "github.com/euc-oss/terraform-sdk-uem"

	commonerrors "github.com/euc-oss/terraform-provider-uem/internal/common/errors"
)

type appAssignmentServiceAPI interface {
	GetAssignmentRuleAsync(
		ctx context.Context,
		ApplicationUUID string,
	) (http.Header, *sdk.AppAssignmentRuleV2Model, error)
	UpdateAssignmentRuleAsync(
		ctx context.Context,
		ApplicationUUID string,
		request *sdk.AppAssignmentRuleV2Model,
	) (http.Header, error)
}

type resourceConfigData struct {
	client                  *sdk.Client
	newAppAssignmentService func(c *sdk.Client) appAssignmentServiceAPI
}

func defaultAppAssignmentServiceFactory(c *sdk.Client) appAssignmentServiceAPI {
	return sdk.NewAppsV2Service(c)
}

func isNotFoundAPIError(err error) bool {
	return commonerrors.IsNotFoundAPIError(err)
}

func resourceTypeErrorDetail(got any) string {
	return fmt.Sprintf(
		"Expected *resourceConfigData or *sdk.Client, got: %T. Please report this issue to the provider developers.",
		got,
	)
}
