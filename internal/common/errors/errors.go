package errors

import (
	stderrors "errors"
	"net/http"
	"strings"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

var ErrCertificateUploadUnsupported = stderrors.New("certificate uploads are not yet supported by terraform-sdk-uem")

func IsNotFoundAPIError(err error) bool {
	var apiErr *sdk.APIError
	return stderrors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

// IsAPIErrorMatch reports whether err is an *sdk.APIError whose StatusCode
// equals status AND that also matches the caller-supplied errorCode and/or
// messagePrefix, whichever the caller passes non-empty. It exists so each
// resource type can classify its OWN non-404 "this thing is actually gone"
// response shape (e.g. UEM 26.2 answering a deleted profile's GET with 400
// "Invalid Profile", or a deleted internal app's GET with 401 errorCode
// 7000) without ever treating status alone as a signal: at least one of
// errorCode or messagePrefix must be supplied, or this always returns
// false. This deliberately keeps "any 400 is not-found" or "any 401 is
// not-found" impossible to express through this helper.
//
//   - errorCode, if non-empty, must equal apiErr.ErrorCode exactly.
//   - messagePrefix, if non-empty, must be a prefix of apiErr.Message.
//   - If both are supplied, both must match.
func IsAPIErrorMatch(err error, status int, errorCode string, messagePrefix string) bool {
	if errorCode == "" && messagePrefix == "" {
		return false
	}
	var apiErr *sdk.APIError
	if !stderrors.As(err, &apiErr) || apiErr.StatusCode != status {
		return false
	}
	if errorCode != "" && apiErr.ErrorCode != errorCode {
		return false
	}
	if messagePrefix != "" && !strings.HasPrefix(apiErr.Message, messagePrefix) {
		return false
	}
	return true
}

// IsAPIErrorExactMatch reports whether err is an *sdk.APIError whose
// StatusCode equals status and whose Message, after trimming surrounding
// whitespace and an optional single trailing ".", equals expectedMessage
// (normalized the same way). Unlike IsAPIErrorMatch's messagePrefix,
// which only requires err's message to START WITH the given text, this
// requires the FULL normalized message to match — needed when a caller
// must pin the match to a specific value embedded in the message (e.g. a
// resource id), so a genuinely different validation error that merely
// happens to share the same leading words (e.g. "Invalid ProfileScope
// value." vs. "Invalid Profile 12345.") is never misclassified as the
// same "gone" shape.
func IsAPIErrorExactMatch(err error, status int, expectedMessage string) bool {
	var apiErr *sdk.APIError
	if !stderrors.As(err, &apiErr) || apiErr.StatusCode != status {
		return false
	}
	return normalizeAPIErrorMessage(apiErr.Message) == normalizeAPIErrorMessage(expectedMessage)
}

func normalizeAPIErrorMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	msg = strings.TrimSuffix(msg, ".")
	return msg
}
