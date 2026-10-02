package assignment

import (
	"strings"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

func TestAssignmentResourceTypeErrorDetail(t *testing.T) {
	msg := resourceTypeErrorDetail(123)
	if !strings.Contains(msg, "int") {
		t.Fatalf("expected type name in error detail, got: %q", msg)
	}
}

func TestAssignmentIsNotFoundAPIError(t *testing.T) {
	notFound := &sdk.APIError{StatusCode: 404, Message: "not found"}
	if !isNotFoundAPIError(notFound) {
		t.Fatal("expected 404 APIError to be recognized as not found")
	}

	serverErr := &sdk.APIError{StatusCode: 500, Message: "server error"}
	if isNotFoundAPIError(serverErr) {
		t.Fatal("did not expect 500 APIError to be recognized as not found")
	}
}
