package macapplication

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	sdk "github.com/euc-oss/terraform-sdk-uem"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/mac/models"
	appState "github.com/euc-oss/terraform-provider-uem/internal/application/mac/state"
)

func (r *macapplicationResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	switch data := req.ProviderData.(type) {
	case *resourceConfigData:
		if data == nil || data.client == nil {
			resp.Diagnostics.AddError("Unexpected Resource Configure Type", "Provider resource data was nil or missing a configured SDK client.")
			return
		}
		r.client = data.client
		if data.newBlobResourceService != nil {
			r.newBlobResourceService = data.newBlobResourceService
		} else {
			r.newBlobResourceService = defaultBlobResourceServiceFactory
		}

		if data.newMacAppResourceService != nil {
			r.newMacAppResourceService = data.newMacAppResourceService
		} else {
			r.newMacAppResourceService = defaultMacAppResourceServiceFactory
		}

		if data.newInternalAppResourceService != nil {
			r.newInternalAppResourceService = data.newInternalAppResourceService
		} else {
			r.newInternalAppResourceService = defaultInternalAppResourceServiceFactory
		}
	case *sdk.Client:
		// Backward-compatible path for direct unit tests that still pass *sdk.Client.
		r.client = data
		r.newBlobResourceService = defaultBlobResourceServiceFactory
		r.newMacAppResourceService = defaultMacAppResourceServiceFactory
		r.newInternalAppResourceService = defaultInternalAppResourceServiceFactory
	default:
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", resourceTypeErrorDetail(req.ProviderData))
		return
	}

	tflog.Trace(ctx, "configure a macOS application resource")
}

func (r *macapplicationResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse) {

	var data tf.MacApplicationResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	orgGroupID, err := data.FetchValidOrgGroupID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid org_group_id", err.Error())
		return
	}

	dmgFilePath, err := data.FetchValidDMGFilePath()
	if err != nil {
		resp.Diagnostics.AddError("Invalid dmg_file_path", err.Error())
		return
	}

	plistFilePath, err := data.FetchValidPlistFilePath()
	if err != nil {
		resp.Diagnostics.AddError("Invalid plist_file_path", err.Error())
		return
	}

	appversion, err := data.FetchValidAppVersion()
	if err != nil {
		resp.Diagnostics.AddError("Invalid app_version", err.Error())
		return
	}

	appUploadResponse, err := r.uploadBlob(ctx, dmgFilePath, orgGroupID, true)
	if err != nil {
		resp.Diagnostics.AddError("Blob Upload Error", fmt.Sprintf("Failed to upload DMG binary: %s", err))
		return
	}

	plistUploadResponse, err := r.uploadBlob(ctx, plistFilePath, orgGroupID, false)
	if err != nil {
		r.cleanupBlobs(ctx, *appUploadResponse.Value)
		resp.Diagnostics.AddError("Blob Upload Error", fmt.Sprintf("Failed to upload plist file: %s", err))
		return
	}

	appCreateResponse, appId, err := r.createMacOSApp(
		ctx,
		orgGroupID,
		*appUploadResponse.Value,
		*plistUploadResponse.Value,
		appversion)

	if err != nil {
		tflog.Trace(ctx, "app creating failed, deleting the blob.")
		r.cleanupBlobs(ctx, *appUploadResponse.Value, *plistUploadResponse.Value)
		resp.Diagnostics.AddError("App Creation Failed", fmt.Sprintf("Failed to create macOS application: %s", err))
		return
	}

	r.refreshIntoState(ctx, appId, appCreateResponse, &data, &resp.State, &resp.Diagnostics)
	tflog.Trace(ctx, "created a macOS application resource")
}

func (r *macapplicationResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse) {

	var data tf.MacApplicationResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	applicationId, err := data.FetchValidAppID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid application id", err.Error())
		return
	}

	fetchResponse, err := r.fetchMacApplicationDetails(ctx, applicationId)
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to fetch application details: %s", err))
		return
	}

	tflog.Warn(ctx, "state set", map[string]any{
		"application_id": applicationId,
	})

	r.refreshIntoState(ctx, *fetchResponse.ID, fetchResponse, &data, &resp.State, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Warn(ctx, "read end", map[string]any{
		"application_id": applicationId,
	})

	tflog.Trace(ctx, "read a macOS application resource")
}

func (r *macapplicationResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse) {

	// this is no update? delete and recreate.
	r.Delete(ctx, resource.DeleteRequest{
		State: req.State,
	}, &resource.DeleteResponse{
		Diagnostics: resp.Diagnostics,
	})

	r.Create(ctx, resource.CreateRequest{
		Plan: req.Plan,
	}, &resource.CreateResponse{
		State:       resp.State,
		Diagnostics: resp.Diagnostics,
	})

	tflog.Trace(ctx, "updated a macOS application resource")
}

func (r *macapplicationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data tf.MacApplicationResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	applicationId, err := data.FetchValidAppID()
	if err != nil {
		resp.Diagnostics.AddError("Invalid application id", err.Error())
		return
	}

	_, err = r.deleteMacOSApp(ctx, applicationId)
	if err != nil {
		if isNotFoundAPIError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete macOS application, got error: %s", err))
		return
	}

	tflog.Trace(ctx, "deleted a macOS application resource")
}

func (r *macapplicationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.AddError(
		"Import Not Supported",
		"Import is not supported for mac applications because required configuration attributes such as dmg_file_path, plist_file_path, and app_version cannot be reconstructed from the API. Recreate the resource in configuration and apply instead of importing it.")
}

func (r *macapplicationResource) uploadBlob(
	ctx context.Context,
	filePath string,
	orgGroupID int,
	isApplication bool) (*sdk.EntityV1ModelV2, error) {

	blobSvc, err := r.BlobResourceService(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize blob resource service: %w", err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	fileName := filepath.Base(filePath)

	sizeMB := float64(len(data)) / (1024 * 1024)
	tflog.Info(ctx, "Uploading blob", map[string]interface{}{
		"file":    fileName,
		"size_mb": fmt.Sprintf("%.1f", sizeMB),
	})

	opts := CreateBlobUploadOptions(fileName, isApplication, orgGroupID)
	_, response, err := blobSvc.UploadBlobAsync(ctx, data, opts)

	if err != nil {
		return nil, fmt.Errorf("blob upload failed for %s (%.1f MB): %w", fileName, sizeMB, err)
	}

	if response == nil ||
		response.Value == nil || *response.Value == 0 ||
		response.UUID == "" {
		return nil, fmt.Errorf("blob upload returned invalid ID for file %s", fileName)
	}

	tflog.Info(ctx, "Blob upload successful", map[string]interface{}{
		"file":                fileName,
		"size_mb":             fmt.Sprintf("%.1f", sizeMB),
		"blob_id":             *response.Value,
		"blob_uuid":           response.UUID,
		"is_application_blob": isApplication,
		"org_group_id":        orgGroupID,
	})

	return response, nil
}

func (r *macapplicationResource) deleteBlob(
	ctx context.Context,
	blobId int) error {

	blobSvc, err := r.BlobResourceService(ctx)
	if err != nil {
		return fmt.Errorf("unable to initialize blob resource service: %w", err)
	}

	_, err = blobSvc.Delete(ctx, strconv.Itoa(blobId))
	if err != nil {
		return fmt.Errorf("blob deletion failed for blob ID %d: %w", blobId, err)
	}

	return nil
}

func (r *macapplicationResource) cleanupBlobs(ctx context.Context, blobIDs ...int) {
	if len(blobIDs) == 0 {
		return
	}

	errCh := make(chan error, len(blobIDs))
	var wg sync.WaitGroup
	for _, id := range blobIDs {
		blobID := id
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.deleteBlob(ctx, blobID); err != nil {
				errCh <- err
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		tflog.Warn(ctx, "blob cleanup failed", map[string]any{"error": err.Error()})
	}
}

func (r *macapplicationResource) createMacOSApp(
	ctx context.Context,
	orgGroupID int,
	dmgBlobID int,
	plistBlobID int,
	version string) (*sdk.InternalAppModelV1, int, error) {

	macAppSvc, err := r.MacAppService(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("unable to initialize mac application resource service: %w", err)
	}

	header, _, err := macAppSvc.CreateMacOSApplication(
		ctx,
		orgGroupID,
		CreateMacOSApplicationRequest(dmgBlobID, plistBlobID, version))

	if err != nil {
		return nil, 0, fmt.Errorf("failed to create macOS application: %w", err)
	}

	appId, err := parseAppIDFromLocation(header)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to parse application ID from location header: %w", err)
	}

	fetchApplicationDetailsResponse, err := r.fetchMacApplicationDetails(ctx, appId)
	if err != nil {
		return nil, 0, err
	}

	return fetchApplicationDetailsResponse, appId, nil
}

func (r *macapplicationResource) fetchMacApplicationDetails(
	ctx context.Context,
	applicationID int) (*sdk.InternalAppModelV1, error) {
	internalAppSvc, err := r.InternalAppService(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize internal app resource service: %w", err)
	}

	_, response, err := internalAppSvc.GetInternalAppByIdAsync(ctx, applicationID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch internal app details: %w", err)
	}

	return response, nil
}

func (r *macapplicationResource) deleteMacOSApp(
	ctx context.Context,
	applicationID int) (http.Header, error) {

	internalAppSvc, err := r.InternalAppService(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize internal app resource service: %w", err)
	}

	header, err := internalAppSvc.DeleteInternalAppAsync(ctx, applicationID)
	if err != nil {
		return nil, fmt.Errorf("unable to delete mac internal app resource service: %w", err)
	}

	return header, nil
}

func (r *macapplicationResource) refreshIntoState(
	ctx context.Context,
	appId int,
	internalAppModel *sdk.InternalAppModelV1,
	data *tf.MacApplicationResourceModel,
	state *tfsdk.State,
	diags *diag.Diagnostics,
) {
	if err := appState.SetMinimalState(data, appId, internalAppModel.UUID); err != nil {
		diags.AddError(
			"Unable to refresh mac application state",
			fmt.Sprintf("Failed to populate minimal state for mac application %d: %s", appId, err),
		)
		return
	}
	diags.Append(state.Set(ctx, data)...)
}
