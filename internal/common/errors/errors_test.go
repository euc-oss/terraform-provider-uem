package errors_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	commonerrors "github.com/euc-oss/terraform-provider-uem/internal/common/errors"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

func apiErr(status int, errorCode, message string) error {
	return &sdk.APIError{StatusCode: status, ErrorCode: errorCode, Message: message}
}

func TestIsNotFoundAPIError_Matches404(t *testing.T) {
	t.Parallel()
	if !commonerrors.IsNotFoundAPIError(apiErr(http.StatusNotFound, "", "not found")) {
		t.Fatal("expected 404 to match IsNotFoundAPIError")
	}
}

func TestIsNotFoundAPIError_RejectsOtherStatuses(t *testing.T) {
	t.Parallel()
	if commonerrors.IsNotFoundAPIError(apiErr(http.StatusBadRequest, "", "Invalid Profile 1.")) {
		t.Fatal("expected 400 not to match IsNotFoundAPIError")
	}
}

func TestIsAPIErrorMatch_StatusAndMessagePrefix(t *testing.T) {
	t.Parallel()
	err := apiErr(http.StatusBadRequest, "400", "Invalid Profile 12345.")
	if !commonerrors.IsAPIErrorMatch(err, http.StatusBadRequest, "", "Invalid Profile") {
		t.Fatal("expected status+prefix match")
	}
}

func TestIsAPIErrorMatch_DifferentMessage_NoMatch(t *testing.T) {
	t.Parallel()
	err := apiErr(http.StatusBadRequest, "400", "Name is required.")
	if commonerrors.IsAPIErrorMatch(err, http.StatusBadRequest, "", "Invalid Profile") {
		t.Fatal("a different 400 message must not be misclassified as not-found")
	}
}

func TestIsAPIErrorMatch_DifferentStatus_NoMatch(t *testing.T) {
	t.Parallel()
	err := apiErr(http.StatusUnauthorized, "", "Invalid Profile 12345.")
	if commonerrors.IsAPIErrorMatch(err, http.StatusBadRequest, "", "Invalid Profile") {
		t.Fatal("a matching message at the wrong status must not match")
	}
}

func TestIsAPIErrorMatch_StatusAndErrorCode(t *testing.T) {
	t.Parallel()
	err := apiErr(http.StatusUnauthorized, "7000", "Application not found or user does not have access to it.")
	if !commonerrors.IsAPIErrorMatch(err, http.StatusUnauthorized, "7000", "") {
		t.Fatal("expected status+errorCode match")
	}
}

func TestIsAPIErrorMatch_DifferentErrorCode_NoMatch(t *testing.T) {
	t.Parallel()
	err := apiErr(http.StatusUnauthorized, "9999", "Token expired.")
	if commonerrors.IsAPIErrorMatch(err, http.StatusUnauthorized, "7000", "") {
		t.Fatal("a 401 with a different errorCode must not match a 7000-specific matcher")
	}
}

func TestIsAPIErrorMatch_PlainUnauthorized_NoErrorCode_NoMatch(t *testing.T) {
	t.Parallel()
	// A genuine expired-token 401 with no errorCode at all must not match.
	err := apiErr(http.StatusUnauthorized, "", "Unauthorized.")
	if commonerrors.IsAPIErrorMatch(err, http.StatusUnauthorized, "7000", "") {
		t.Fatal("a plain 401 with no errorCode must not match a 7000-specific matcher")
	}
}

func TestIsAPIErrorMatch_NeverBlanketOnStatusAlone(t *testing.T) {
	t.Parallel()
	// Calling with both errorCode and messagePrefix empty must never match
	// anything — this is the guard against "any 400/401 = not-found".
	err := apiErr(http.StatusBadRequest, "400", "anything at all")
	if commonerrors.IsAPIErrorMatch(err, http.StatusBadRequest, "", "") {
		t.Fatal("IsAPIErrorMatch must never match on status alone")
	}
}

func TestIsAPIErrorMatch_NonAPIError_NoMatch(t *testing.T) {
	t.Parallel()
	if commonerrors.IsAPIErrorMatch(errors.New("boom"), http.StatusBadRequest, "", "Invalid Profile") {
		t.Fatal("a non-APIError must never match")
	}
}

func TestIsAPIErrorMatch_WrappedError(t *testing.T) {
	t.Parallel()
	err := fmt.Errorf("wrapped: %w", apiErr(http.StatusBadRequest, "400", "Invalid Profile 1."))
	if !commonerrors.IsAPIErrorMatch(err, http.StatusBadRequest, "", "Invalid Profile") {
		t.Fatal("expected match through a wrapped error chain")
	}
}

func TestIsAPIErrorExactMatch_ExactMessage(t *testing.T) {
	t.Parallel()
	err := apiErr(http.StatusBadRequest, "400", "Invalid Profile 12345")
	if !commonerrors.IsAPIErrorExactMatch(err, http.StatusBadRequest, "Invalid Profile 12345") {
		t.Fatal("expected an exact message match with no trailing period")
	}
}

func TestIsAPIErrorExactMatch_TrailingPeriodTolerant(t *testing.T) {
	t.Parallel()
	err := apiErr(http.StatusBadRequest, "400", "Invalid Profile 12345.")
	if !commonerrors.IsAPIErrorExactMatch(err, http.StatusBadRequest, "Invalid Profile 12345") {
		t.Fatal("expected the actual message's trailing period to be trim-tolerant")
	}
	err2 := apiErr(http.StatusBadRequest, "400", "Invalid Profile 12345")
	if !commonerrors.IsAPIErrorExactMatch(err2, http.StatusBadRequest, "Invalid Profile 12345.") {
		t.Fatal("expected the expected message's trailing period to be trim-tolerant too")
	}
}

func TestIsAPIErrorExactMatch_WhitespaceTolerant(t *testing.T) {
	t.Parallel()
	err := apiErr(http.StatusBadRequest, "400", "  Invalid Profile 12345.  ")
	if !commonerrors.IsAPIErrorExactMatch(err, http.StatusBadRequest, "Invalid Profile 12345") {
		t.Fatal("expected surrounding whitespace on the actual message to be trimmed")
	}
}

func TestIsAPIErrorExactMatch_UnrelatedPrefixOnly_NoMatch(t *testing.T) {
	t.Parallel()
	// This is the exact shape of the bug being fixed: a message that
	// starts with the same words but is a genuinely different error.
	err := apiErr(http.StatusBadRequest, "400", "Invalid ProfileScope value.")
	if commonerrors.IsAPIErrorExactMatch(err, http.StatusBadRequest, "Invalid Profile 12345") {
		t.Fatal("a message that merely shares a prefix must not exact-match")
	}
}

func TestIsAPIErrorExactMatch_DifferentID_NoMatch(t *testing.T) {
	t.Parallel()
	err := apiErr(http.StatusBadRequest, "400", "Invalid Profile 123.")
	if commonerrors.IsAPIErrorExactMatch(err, http.StatusBadRequest, "Invalid Profile 456") {
		t.Fatal("a gone-message for a different id must not exact-match")
	}
}

func TestIsAPIErrorExactMatch_TrailingExtraText_NoMatch(t *testing.T) {
	t.Parallel()
	err := apiErr(http.StatusBadRequest, "400", "Invalid Profile 12345 extra text")
	if commonerrors.IsAPIErrorExactMatch(err, http.StatusBadRequest, "Invalid Profile 12345") {
		t.Fatal("extra trailing text beyond the optional period must not exact-match")
	}
}

func TestIsAPIErrorExactMatch_DifferentStatus_NoMatch(t *testing.T) {
	t.Parallel()
	err := apiErr(http.StatusUnauthorized, "400", "Invalid Profile 12345.")
	if commonerrors.IsAPIErrorExactMatch(err, http.StatusBadRequest, "Invalid Profile 12345") {
		t.Fatal("a matching message at the wrong status must not exact-match")
	}
}

func TestIsAPIErrorExactMatch_NonAPIError_NoMatch(t *testing.T) {
	t.Parallel()
	if commonerrors.IsAPIErrorExactMatch(errors.New("boom"), http.StatusBadRequest, "Invalid Profile 12345") {
		t.Fatal("a non-APIError must never exact-match")
	}
}
