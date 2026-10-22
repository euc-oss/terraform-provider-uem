package macsensor

import (
	"context"
	"fmt"
	"net/http"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"

	commonerrors "github.com/euc-oss/terraform-provider-uem/internal/common/errors"
)

type DeviceSensorsV2API interface {
	CreateDeviceSensorAsync(
		ctx context.Context,
		request *sdk.DeviceSensorRequestV2Model,
	) (http.Header, *sdk.BaseModelV2, error)

	GetDeviceSensorAsync(
		ctx context.Context,
		sensorUUID string,
	) (http.Header, *sdk.DeviceSensorResponseV2Model, error)

	UpdateDeviceSensorAsync(
		ctx context.Context,
		sensorUUID string,
		request *sdk.DeviceSensorUpdateV2Model,
	) (http.Header, error)
}

type DeviceSensorsV1DeleteAPI interface {
	BulkDeleteDeviceSensors(
		ctx context.Context,
		request *sdk.DeviceSensorsBulkDeleteRequestV1Model,
	) (http.Header, error)
}

type resourceConfigData struct {
	client                *sdk.Client
	newDeviceSensorsV2API func(c *sdk.Client) DeviceSensorsV2API
}

func defaultDeviceSensorsV2Factory(c *sdk.Client) DeviceSensorsV2API {
	return sdk.NewDeviceSensorsV2Service(c)
}

func defaultDeviceSensorsV1DeleteFactory(c *sdk.Client) DeviceSensorsV1DeleteAPI {
	return sdk.NewDeviceSensorsV1Service(c)
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
