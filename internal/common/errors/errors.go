package errors

import (
	stderrors "errors"
	"net/http"

	sdk "github.com/euc-oss/terraform-sdk-uem"
)

var ErrCertificateUploadUnsupported = stderrors.New("certificate uploads are not yet supported by terraform-sdk-uem")

func IsNotFoundAPIError(err error) bool {
	var apiErr *sdk.APIError
	return stderrors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}
