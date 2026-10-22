package assignment

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"

	commonerrors "github.com/euc-oss/terraform-provider-uem/internal/common/errors"
)

type sensorAssignmentServiceAPI interface {
	AddDeviceSensorAssignmentAsync(
		ctx context.Context,
		SensorUUID string,
		request *sdk.DeviceSensorAssignmentRequestV1ModelV2,
	) (http.Header, *sdk.BaseModelV2, error)

	GetDeviceSensorAssignmentsAsync(
		ctx context.Context,
		SensorUUID string,
	) (http.Header, *[]sdk.DeviceSensorAssignmentResponseV1ModelV2, error)

	GetDeviceSensorAssignmentAsync(
		ctx context.Context,
		AssignmentUUID string,
	) (http.Header, *sdk.DeviceSensorAssignmentResponseV1ModelV2, error)

	UpdateDeviceSensorAssignmentAsync(
		ctx context.Context,
		AssignmentUUID string,
		request *sdk.DeviceSensorAssignmentRequestV1ModelV2,
	) (http.Header, *sdk.BaseExceptionModelV2, error)

	DeleteDeviceSensorAssignmentAsync(
		ctx context.Context,
		AssignmentUUID string,
	) (http.Header, error)

	BulkUpdateDeviceSensorAssignmentRankingsAsync(
		ctx context.Context,
		SensorUUID string,
		request *[]sdk.DeviceSensorAssignmentRankingV1ModelV2,
		opts *sdk.DeviceSensorsV2BulkUpdateDeviceSensorAssignmentRankingsAsyncOptions,
	) (http.Header, error)
}

type resourceConfigData struct {
	client                     *sdk.Client
	newSensorAssignmentService func(c *sdk.Client) sensorAssignmentServiceAPI
}

func defaultSensorAssignmentServiceFactory(c *sdk.Client) sensorAssignmentServiceAPI {
	return sdk.NewDeviceSensorsV2Service(c)
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

func parseAssignmentUUID(headers http.Header, body *sdk.BaseModelV2) (string, error) {
	if body != nil && strings.TrimSpace(body.UUID) != "" {
		return strings.TrimSpace(body.UUID), nil
	}
	return sdk.ParseLocationID(headers.Get("Location"))
}
