package deployment

import (
	"context"
	"fmt"
	"net/http"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"

	commonerrors "github.com/euc-oss/terraform-provider-uem/internal/common/errors"
)

type UpdatesV1API interface {
	CreateUpdateDeployment(
		ctx context.Context,
		UpdateUUID string,
		OrganizationGroupUUID string,
		request *sdk.DeviceUpdateDeploymentBaseV1Model,
	) (http.Header, *sdk.BaseModelV1, error)

	GetDeviceUpdateDeploymentDetails(
		ctx context.Context,
		UUID string,
	) (http.Header, *sdk.DeviceUpdateDeploymentV1Model, error)

	UpdateDeviceUpdateDeployment(
		ctx context.Context,
		UUID string,
		request *sdk.DeviceUpdateDeploymentUpdateV1Model,
	) (http.Header, error)

	DeleteDeviceUpdateDeployment(
		ctx context.Context,
		UUID string,
	) (http.Header, error)
}

type resourceConfigData struct {
	client       *sdk.Client
	newUpdatesV1 func(c *sdk.Client) UpdatesV1API
}

func defaultUpdatesV1Factory(c *sdk.Client) UpdatesV1API {
	return sdk.NewUpdatesV1Service(c)
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
