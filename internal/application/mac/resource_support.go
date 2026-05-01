package macapplication

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	sdk "github.com/euc-oss/terraform-sdk-uem"

	commonerrors "github.com/euc-oss/terraform-provider-uem/internal/common/errors"
)

type appBlobResourceServiceAPI interface {
	Delete(
		ctx context.Context,
		BlobID string,
	) (http.Header, error)

	UploadBlobAsync(
		ctx context.Context,
		request []byte,
		opts *sdk.BlobsV2UploadBlobAsyncOptions,
	) (http.Header, *sdk.EntityV1ModelV2, error)
}

type macAppResourceServiceAPI interface {
	CreateMacOSApplication(
		ctx context.Context,
		ID int,
		request *sdk.MacOsCreateApplicationRequestV1Model,
	) (http.Header, int, error)
}

type InternalAppsV1ServiceAPI interface {
	DeleteInternalAppAsync(
		ctx context.Context,
		ApplicationID int,
	) (http.Header, error)

	GetInternalAppByIdAsync(
		ctx context.Context,
		ApplicationID int,
	) (http.Header, *sdk.InternalAppModelV1, error)

	// no update
}

type resourceConfigData struct {
	client                        *sdk.Client
	newBlobResourceService        func(c *sdk.Client) appBlobResourceServiceAPI
	newMacAppResourceService      func(c *sdk.Client) macAppResourceServiceAPI
	newInternalAppResourceService func(c *sdk.Client) InternalAppsV1ServiceAPI
}

func defaultBlobResourceServiceFactory(c *sdk.Client) appBlobResourceServiceAPI {
	return sdk.NewBlobsV2Service(c)
}

func defaultMacAppResourceServiceFactory(c *sdk.Client) macAppResourceServiceAPI {
	return sdk.NewMacOsAppsV1Service(c)
}

func defaultInternalAppResourceServiceFactory(c *sdk.Client) InternalAppsV1ServiceAPI {
	return sdk.NewInternalAppsV1Service(c)
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

func CreateBlobUploadOptions(
	fileName string,
	isApplication bool,
	orgGroupID int) *sdk.BlobsV2UploadBlobAsyncOptions {

	opts := &sdk.BlobsV2UploadBlobAsyncOptions{
		FileName:            fileName,
		OrganizationGroupID: orgGroupID,
	}

	applicationModuleType := "Application"
	if isApplication {
		opts.ModuleType = &applicationModuleType
	}

	return opts
}

func CreateMacOSApplicationRequest(
	dmgBlobID int,
	plistBlobID int,
	version string,
) *sdk.MacOsCreateApplicationRequestV1Model {
	return &sdk.MacOsCreateApplicationRequestV1Model{
		ApplicationBlobID: &dmgBlobID,
		PkgInfoBlobID:     &plistBlobID,
		Version:           version,
	}
}

func parseAppIDFromLocation(header http.Header) (int, error) {
	location := header.Get("Location")
	location = strings.TrimSpace(location)
	if location == "" {
		return 0, fmt.Errorf("location is empty")
	}

	parts := strings.Split(strings.TrimRight(location, "/"), "/")
	if len(parts) == 0 {
		return 0, fmt.Errorf("invalid location format")
	}

	appID, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return 0, fmt.Errorf("invalid application ID format: %w", err)
	}

	return appID, nil
}
