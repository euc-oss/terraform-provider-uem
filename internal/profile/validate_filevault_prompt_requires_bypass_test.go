package profile

import (
	"context"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Tests in this file cover validateFileVaultPromptRequiresNumberOfTimesUserCanBypass
// in internal/profile/validate.go, plus the schema-level
// int64validator.OneOf(1, 2, 3) on disk_encryption.filevault2.prompt_to_enable_filevault_at.
//
// Live-verified behavior being guarded against (as<internal-env> tenant, macOS/AppleOsX,
// disk-encryption-filevault2 live-e2e proof, 2026-09-25): a uem_profile with
// disk_encryption.filevault2.prompt_to_enable_filevault_at = 1 (Both Login and
// Logout) or 3 (Login Only) and no number_of_times_user_can_bypass is
// accepted by `terraform plan` but rejected by UEM at apply time with a 422
// (deviceProfile.DiskEncryption.DiskEncryptionFileVault2.NumberOfTimesUserCanBypass
// ... Must provide the NumberOfTimesUserCanBypass when PromptToEnableFileVaultAt
// is BothLoginAndLogout or LoginOnly).

const fileVaultPromptRequiresBypassSummary = "number_of_times_user_can_bypass is required when prompt_to_enable_filevault_at is 1 or 3"

func TestValidateConfig_FileVaultPrompt_BothLoginAndLogout_BypassNull_Rejected(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(map[string]tftypes.Value{
			"prompt_to_enable_filevault_at": int64Val(1),
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	wantPath := "disk_encryption.filevault2.number_of_times_user_can_bypass"
	errs := diagErrorsAtPath(resp, wantPath)
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error at %s, got %d: %s", wantPath, len(errs), diagSummaries(resp))
	}
	if errs[0] != fileVaultPromptRequiresBypassSummary {
		t.Errorf("expected summary %q, got %q", fileVaultPromptRequiresBypassSummary, errs[0])
	}
}

func TestValidateConfig_FileVaultPrompt_LoginOnly_BypassNull_Rejected(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(map[string]tftypes.Value{
			"prompt_to_enable_filevault_at": int64Val(3),
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	wantPath := "disk_encryption.filevault2.number_of_times_user_can_bypass"
	errs := diagErrorsAtPath(resp, wantPath)
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error at %s, got %d: %s", wantPath, len(errs), diagSummaries(resp))
	}
	if errs[0] != fileVaultPromptRequiresBypassSummary {
		t.Errorf("expected summary %q, got %q", fileVaultPromptRequiresBypassSummary, errs[0])
	}
}

func TestValidateConfig_FileVaultPrompt_BypassSet_NoError(t *testing.T) {
	t.Parallel()

	for _, p := range []int64{1, 3} {
		t.Run("", func(t *testing.T) {
			t.Parallel()
			de := diskEncryptionVal(map[string]tftypes.Value{
				"filevault2": filevaultVal(map[string]tftypes.Value{
					"prompt_to_enable_filevault_at":   int64Val(p),
					"number_of_times_user_can_bypass": int64Val(-1),
				}),
			})
			resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

			if errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.number_of_times_user_can_bypass"); len(errs) != 0 {
				t.Fatalf("expected no error with prompt = %d and bypass set, got: %v", p, errs)
			}
		})
	}
}

func TestValidateConfig_FileVaultPrompt_Unknown_NoError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(map[string]tftypes.Value{
			"prompt_to_enable_filevault_at": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
			// number_of_times_user_can_bypass left null: if prompt's
			// unknown-ness weren't respected, this would otherwise fire.
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	if errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.number_of_times_user_can_bypass"); len(errs) != 0 {
		t.Fatalf("expected no error with prompt_to_enable_filevault_at unknown, got: %v", errs)
	}
}

func TestValidateConfig_FileVaultPrompt_BypassUnknown_NoError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(map[string]tftypes.Value{
			"prompt_to_enable_filevault_at":   int64Val(1),
			"number_of_times_user_can_bypass": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	if errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.number_of_times_user_can_bypass"); len(errs) != 0 {
		t.Fatalf("expected no error with number_of_times_user_can_bypass unknown, got: %v", errs)
	}
}

// TestValidateConfig_FileVaultPrompt_LogoutOnly_NotRequired proves
// prompt_to_enable_filevault_at = 2 (Logout Only) never requires
// number_of_times_user_can_bypass, even with it null.
func TestValidateConfig_FileVaultPrompt_LogoutOnly_NotRequired(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(map[string]tftypes.Value{
			"prompt_to_enable_filevault_at": int64Val(2),
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	if errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.number_of_times_user_can_bypass"); len(errs) != 0 {
		t.Fatalf("expected no error with prompt_to_enable_filevault_at = 2, got: %v", errs)
	}
}

// TestValidateConfig_FileVaultPrompt_OtherPlatform_NoError proves the check
// is macOS-only, matching the other filevault2 checks.
func TestValidateConfig_FileVaultPrompt_OtherPlatform_NoError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(map[string]tftypes.Value{
			"prompt_to_enable_filevault_at": int64Val(1),
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleiOS, de)

	if errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.number_of_times_user_can_bypass"); len(errs) != 0 {
		t.Fatalf("expected no error on %q, got: %v", sdk.PlatformAppleiOS, errs)
	}
}

// --- schema-level int64validator.OneOf(1, 2, 3) on prompt_to_enable_filevault_at ---

func promptToEnableFileVaultAtValidators(t *testing.T) []validator.Int64 {
	t.Helper()

	schemaResp := getResourceSchema(t)

	deAttr, ok := schemaResp.Schema.Attributes["disk_encryption"]
	if !ok {
		t.Fatal("schema has no \"disk_encryption\" attribute")
	}
	deNested, ok := deAttr.(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("\"disk_encryption\" is not a schema.SingleNestedAttribute, got %T", deAttr)
	}

	fvAttr, ok := deNested.Attributes["filevault2"]
	if !ok {
		t.Fatal("disk_encryption schema has no \"filevault2\" attribute")
	}
	fvNested, ok := fvAttr.(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("\"filevault2\" is not a schema.SingleNestedAttribute, got %T", fvAttr)
	}

	ptAttr, ok := fvNested.Attributes["prompt_to_enable_filevault_at"]
	if !ok {
		t.Fatal("filevault2 schema has no \"prompt_to_enable_filevault_at\" attribute")
	}
	ptInt64, ok := ptAttr.(schema.Int64Attribute)
	if !ok {
		t.Fatalf("\"prompt_to_enable_filevault_at\" is not a schema.Int64Attribute, got %T", ptAttr)
	}

	return ptInt64.Validators
}

func runPromptToEnableFileVaultAtValidators(t *testing.T, n int64) []validator.Int64Response {
	t.Helper()

	validators := promptToEnableFileVaultAtValidators(t)
	responses := make([]validator.Int64Response, 0, len(validators))
	for _, v := range validators {
		req := validator.Int64Request{
			Path:        path.Root("disk_encryption").AtName("filevault2").AtName("prompt_to_enable_filevault_at"),
			ConfigValue: types.Int64Value(n),
		}
		resp := &validator.Int64Response{}
		v.ValidateInt64(context.Background(), req, resp)
		responses = append(responses, *resp)
	}
	return responses
}

func TestPromptToEnableFileVaultAtAttribute_PlanTimeValidation_RejectsOutOfRange(t *testing.T) {
	t.Parallel()

	for _, n := range []int64{0, 4, -1} {
		responses := runPromptToEnableFileVaultAtValidators(t, n)
		hasError := false
		for _, r := range responses {
			if r.Diagnostics.HasError() {
				hasError = true
			}
		}
		if !hasError {
			t.Errorf("expected a plan-time validation error for prompt_to_enable_filevault_at = %d, got none", n)
		}
	}
}

func TestPromptToEnableFileVaultAtAttribute_PlanTimeValidation_AcceptsDocumentedValues(t *testing.T) {
	t.Parallel()

	for _, n := range []int64{1, 2, 3} {
		responses := runPromptToEnableFileVaultAtValidators(t, n)
		for _, r := range responses {
			if r.Diagnostics.HasError() {
				t.Errorf("expected no plan-time validation error for prompt_to_enable_filevault_at = %d, got: %v", n, r.Diagnostics)
			}
		}
	}
}
