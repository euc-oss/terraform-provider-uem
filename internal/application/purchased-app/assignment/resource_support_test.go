package assignment

import (
	"strings"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

func TestPurchasedAssignmentResourceTypeErrorDetail(t *testing.T) {
	msg := resourceTypeErrorDetail(123)
	if !strings.Contains(msg, "int") {
		t.Fatalf("expected type name in error detail, got: %q", msg)
	}
}

func TestPurchasedAssignmentIsNotFoundAPIError(t *testing.T) {
	notFound := &sdk.APIError{StatusCode: 404, Message: "not found"}
	if !isNotFoundAPIError(notFound) {
		t.Fatal("expected 404 APIError to be recognized as not found")
	}
}
