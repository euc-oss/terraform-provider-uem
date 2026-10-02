package macsensor

import (
	"context"
	"fmt"
	"strings"

	tf "github.com/euc-oss/terraform-provider-uem/internal/sensors/mac/models"
	sensorstate "github.com/euc-oss/terraform-provider-uem/internal/sensors/mac/state"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

func (r *macsensorResource) createMacSensor(
	ctx context.Context,
	createReq *sdk.DeviceSensorRequestV2Model,
) (string, error) {
	svc, err := r.DeviceSensorsService(ctx)
	if err != nil {
		return "", err
	}

	headers, body, err := svc.CreateDeviceSensorAsync(ctx, createReq)
	if err != nil {
		return "", fmt.Errorf("unable to create mac sensor: %w", err)
	}

	if body != nil && strings.TrimSpace(body.UUID) != "" {
		return strings.TrimSpace(body.UUID), nil
	}

	sensorUUID, err := sdk.ParseLocationID(headers.Get("Location"))
	if err != nil {
		return "", fmt.Errorf("unable to parse sensor UUID from create response: %w", err)
	}

	return sensorUUID, nil
}

func (r *macsensorResource) fetchMacSensorDetails(
	ctx context.Context,
	sensorUUID string,
) (*sdk.DeviceSensorResponseV2Model, error) {
	svc, err := r.DeviceSensorsService(ctx)
	if err != nil {
		return nil, err
	}

	_, sensor, err := svc.GetDeviceSensorAsync(ctx, sensorUUID)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch mac sensor %s: %w", sensorUUID, err)
	}

	return sensor, nil
}

func (r *macsensorResource) updateMacSensor(
	ctx context.Context,
	sensorUUID string,
	updateReq *sdk.DeviceSensorUpdateV2Model,
) error {
	svc, err := r.DeviceSensorsService(ctx)
	if err != nil {
		return err
	}

	_, err = svc.UpdateDeviceSensorAsync(ctx, sensorUUID, updateReq)
	if err != nil {
		return fmt.Errorf("unable to update mac sensor %s: %w", sensorUUID, err)
	}

	return nil
}

func (r *macsensorResource) deleteMacSensor(
	ctx context.Context,
	orgGroupUUID string,
	sensorUUID string,
) error {
	// MDM V2 does not expose sensor delete; V1 bulk delete remains the supported path.
	svc := defaultDeviceSensorsV1DeleteFactory(r.client)

	_, err := svc.BulkDeleteDeviceSensors(ctx, &sdk.DeviceSensorsBulkDeleteRequestV1Model{
		OrganizationGroupUUID: orgGroupUUID,
		SensorUUIDs:           []string{sensorUUID},
	})
	if err != nil {
		return fmt.Errorf("unable to delete mac sensor %s: %w", sensorUUID, err)
	}

	return nil
}

func (r *macsensorResource) refreshIntoState(
	ctx context.Context,
	sensorUUID string,
	api *sdk.DeviceSensorResponseV2Model,
	data *tf.MacSensorResourceModel,
	state *tfsdk.State,
	diags *diag.Diagnostics,
) {
	diags.Append(sensorstate.ReadAPIIntoState(ctx, data, api, sensorUUID)...)
	if diags.HasError() {
		return
	}
	diags.Append(state.Set(ctx, data)...)
}
