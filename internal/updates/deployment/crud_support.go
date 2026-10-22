package deployment

import (
	"context"
	"fmt"
	"strings"

	tf "github.com/euc-oss/terraform-provider-uem/internal/updates/deployment/models"
	deploymentstate "github.com/euc-oss/terraform-provider-uem/internal/updates/deployment/state"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

func (r *updateDeploymentResource) createDeployment(
	ctx context.Context,
	updateUUID string,
	orgGroupUUID string,
	createReq *sdk.DeviceUpdateDeploymentBaseV1Model,
) (string, error) {
	svc, err := r.UpdatesService(ctx)
	if err != nil {
		return "", err
	}

	headers, body, err := svc.CreateUpdateDeployment(ctx, updateUUID, orgGroupUUID, createReq)
	if err != nil {
		return "", fmt.Errorf("unable to create update deployment: %w", err)
	}

	if body != nil && strings.TrimSpace(body.UUID) != "" {
		return strings.TrimSpace(body.UUID), nil
	}

	deploymentUUID, err := sdk.ParseLocationID(headers.Get("Location"))
	if err != nil {
		return "", fmt.Errorf("unable to parse deployment UUID from create response: %w", err)
	}
	return deploymentUUID, nil
}

func (r *updateDeploymentResource) fetchDeploymentDetails(
	ctx context.Context,
	deploymentUUID string,
) (*sdk.DeviceUpdateDeploymentV1Model, error) {
	svc, err := r.UpdatesService(ctx)
	if err != nil {
		return nil, err
	}

	_, details, err := svc.GetDeviceUpdateDeploymentDetails(ctx, deploymentUUID)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch update deployment %s: %w", deploymentUUID, err)
	}
	return details, nil
}

func (r *updateDeploymentResource) updateDeployment(
	ctx context.Context,
	deploymentUUID string,
	updateReq *sdk.DeviceUpdateDeploymentUpdateV1Model,
) error {
	svc, err := r.UpdatesService(ctx)
	if err != nil {
		return err
	}

	_, err = svc.UpdateDeviceUpdateDeployment(ctx, deploymentUUID, updateReq)
	if err != nil {
		return fmt.Errorf("unable to update update deployment %s: %w", deploymentUUID, err)
	}
	return nil
}

func (r *updateDeploymentResource) deleteDeployment(
	ctx context.Context,
	deploymentUUID string,
) error {
	svc, err := r.UpdatesService(ctx)
	if err != nil {
		return err
	}

	_, err = svc.DeleteDeviceUpdateDeployment(ctx, deploymentUUID)
	if err != nil {
		return fmt.Errorf("unable to delete update deployment %s: %w", deploymentUUID, err)
	}
	return nil
}

func (r *updateDeploymentResource) refreshIntoState(
	ctx context.Context,
	deploymentUUID string,
	api *sdk.DeviceUpdateDeploymentV1Model,
	data *tf.UpdateDeploymentModel,
	state *tfsdk.State,
	diags *diag.Diagnostics,
) {
	diags.Append(deploymentstate.ReadAPIIntoState(ctx, data, api, deploymentUUID)...)
	if diags.HasError() {
		return
	}
	diags.Append(state.Set(ctx, data)...)
}
