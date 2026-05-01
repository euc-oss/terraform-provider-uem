package profile

import (
	"context"
	"fmt"

	commonerrors "github.com/euc-oss/terraform-provider-uem/internal/common/errors"
	profileplatform "github.com/euc-oss/terraform-provider-uem/internal/profile/platform"
	sdk "github.com/euc-oss/terraform-sdk-uem"
)

var supportedImportPlatforms = profileplatform.SupportedImportPlatforms

type profileServiceAPI interface {
	RegisterEntry(id int, platform string)
	Get(ctx context.Context, id int) (*sdk.ProfileResult, error)
	Create(ctx context.Context, platform string, profile interface{}) (int, error)
	Update(ctx context.Context, id int, profile interface{}) error
	Delete(ctx context.Context, id int) error
}

type resourceConfigData struct {
	client            *sdk.Client
	newProfileService func(ctx context.Context, c *sdk.Client) (profileServiceAPI, error)
}

func defaultProfileServiceFactory(ctx context.Context, c *sdk.Client) (profileServiceAPI, error) {
	return sdk.NewProfileService(ctx, c)
}

func isNotFoundAPIError(err error) bool {
	return commonerrors.IsNotFoundAPIError(err)
}

func isValidImportPlatform(platform string) bool {
	return profileplatform.IsValidImportPlatform(platform)
}

func platformForSDK(platform string) string {
	return profileplatform.ForSDK(platform)
}

func resourceTypeErrorDetail(got any) string {
	return fmt.Sprintf(
		"Expected *resourceConfigData or *sdk.Client, got: %T. Please report this issue to the provider developers.",
		got,
	)
}
