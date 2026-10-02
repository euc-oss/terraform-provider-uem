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

// Tests in this file cover validateFileVaultRequiresFileVaultUser in
// internal/profile/validate.go, plus the schema-level
// int64validator.OneOf(1, 2) on disk_encryption.filevault2.filevault_user.
//
// Originally live-verified narrower (as<internal-env> tenant, macOS/AppleOsX): a
// uem_profile with disk_encryption.filevault2.enable = true and
// filevault_user omitted is accepted by `terraform plan` but rejected by UEM
// at apply time with a 400 (Invalid input - FileVaultUser). Broadened per
// the canonical UEM 26.2/26.4 rules report (ProfilesV2Controller.
// ValidateDiskEncryptionPayload): the check fires whenever disk_encryption
// is present at all, regardless of enable's value.

const fileVaultRequiresFileVaultUserSummary = "filevault_user is required when disk_encryption is configured"

func TestValidateConfig_FileVault_EnableTrue_FileVaultUserNull_Rejected(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(map[string]tftypes.Value{
			"enable": boolVal(true),
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	wantPath := "disk_encryption.filevault2.filevault_user"
	errs := diagErrorsAtPath(resp, wantPath)
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error at %s, got %d: %s", wantPath, len(errs), diagSummaries(resp))
	}
	if errs[0] != fileVaultRequiresFileVaultUserSummary {
		t.Errorf("expected summary %q, got %q", fileVaultRequiresFileVaultUserSummary, errs[0])
	}
}

func TestValidateConfig_FileVault_EnableTrue_FileVaultUserSet_NoError(t *testing.T) {
	t.Parallel()

	for _, fu := range []int64{1, 2} {
		t.Run("", func(t *testing.T) {
			t.Parallel()
			de := diskEncryptionVal(map[string]tftypes.Value{
				"filevault2": filevaultVal(map[string]tftypes.Value{
					"enable":         boolVal(true),
					"filevault_user": int64Val(fu),
					// filevault_user = 2 also requires username; set it so
					// this test isolates the filevault_user check.
					"username": stringVal("od-user"),
				}),
			})
			resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

			if errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.filevault_user"); len(errs) != 0 {
				t.Fatalf("expected no error with filevault_user = %d, got: %v", fu, errs)
			}
		})
	}
}

func TestValidateConfig_FileVault_EnableTrue_FileVaultUserUnknown_NoError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(map[string]tftypes.Value{
			"enable":         boolVal(true),
			"filevault_user": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	if errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.filevault_user"); len(errs) != 0 {
		t.Fatalf("expected no error with filevault_user unknown, got: %v", errs)
	}
}

// TestValidateConfig_FileVault_FileVaultUser_EnableUnknown_StillRejected
// proves the broadened rule doesn't gate on enable at all: even with enable
// unknown, a known-null filevault_user still fires, since disk_encryption
// (and filevault2) are both known and present regardless of what enable
// eventually resolves to.
func TestValidateConfig_FileVault_FileVaultUser_EnableUnknown_StillRejected(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(map[string]tftypes.Value{
			"enable": tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	if errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.filevault_user"); len(errs) != 1 {
		t.Fatalf("expected exactly 1 error with enable unknown, got %d: %s", len(errs), diagSummaries(resp))
	}
}

// TestValidateConfig_FileVault_EnableFalse_FileVaultUserNull_Rejected proves
// the broadened rule: even with enable = false, UEM's controller still
// requires filevault_user whenever disk_encryption is present at all (it's
// a separate, non-exclusive check from the enable = true FileVault2 rules).
func TestValidateConfig_FileVault_EnableFalse_FileVaultUserNull_Rejected(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(map[string]tftypes.Value{
			"enable": boolVal(false),
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.filevault_user")
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error with enable = false, got %d: %s", len(errs), diagSummaries(resp))
	}
	if errs[0] != fileVaultRequiresFileVaultUserSummary {
		t.Errorf("expected summary %q, got %q", fileVaultRequiresFileVaultUserSummary, errs[0])
	}
}

// TestValidateConfig_FileVault_EmptyFileVault2Block_FileVaultUserNull_Rejected
// proves the rule also fires when the filevault2 sub-block is entirely
// absent from config (disk_encryption present via another sub-block, e.g.
// mcx): UEM's parent ctor always instantiates a non-null FileVault2 child,
// so an absent sub-block is indistinguishable server-side from one with
// every field explicitly unset.
func TestValidateConfig_FileVault_EmptyFileVault2Block_FileVaultUserNull_Rejected(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"mcx": mcxVal(map[string]tftypes.Value{
			"destroy_fv_key_on_standby": boolVal(true),
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.filevault_user")
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error with filevault2 absent, got %d: %s", len(errs), diagSummaries(resp))
	}
}

// TestValidateConfig_FileVault_FileVaultUser_OtherPlatform_NoError proves the
// check is macOS-only: buildAppleOsXDiskEncryptionFileVault2Entity is the
// only builder that ever sends this block, so an equivalent config on
// another platform is never flagged.
func TestValidateConfig_FileVault_FileVaultUser_OtherPlatform_NoError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(map[string]tftypes.Value{
			"enable": boolVal(true),
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleiOS, de)

	if errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.filevault_user"); len(errs) != 0 {
		t.Fatalf("expected no error on %q, got: %v", sdk.PlatformAppleiOS, errs)
	}
}

// --- schema-level int64validator.OneOf(1, 2) on filevault_user ------------

func fileVaultUserValidators(t *testing.T) []validator.Int64 {
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

	fuAttr, ok := fvNested.Attributes["filevault_user"]
	if !ok {
		t.Fatal("filevault2 schema has no \"filevault_user\" attribute")
	}
	fuInt64, ok := fuAttr.(schema.Int64Attribute)
	if !ok {
		t.Fatalf("\"filevault_user\" is not a schema.Int64Attribute, got %T", fuAttr)
	}

	return fuInt64.Validators
}

func runFileVaultUserValidators(t *testing.T, n int64) []validator.Int64Response {
	t.Helper()

	validators := fileVaultUserValidators(t)
	responses := make([]validator.Int64Response, 0, len(validators))
	for _, v := range validators {
		req := validator.Int64Request{
			Path:        path.Root("disk_encryption").AtName("filevault2").AtName("filevault_user"),
			ConfigValue: types.Int64Value(n),
		}
		resp := &validator.Int64Response{}
		v.ValidateInt64(context.Background(), req, resp)
		responses = append(responses, *resp)
	}
	return responses
}

func TestFileVaultUserAttribute_PlanTimeValidation_RejectsOutOfRange(t *testing.T) {
	t.Parallel()

	for _, n := range []int64{0, 3, -1} {
		responses := runFileVaultUserValidators(t, n)
		hasError := false
		for _, r := range responses {
			if r.Diagnostics.HasError() {
				hasError = true
			}
		}
		if !hasError {
			t.Errorf("expected a plan-time validation error for filevault_user = %d, got none", n)
		}
	}
}

func TestFileVaultUserAttribute_PlanTimeValidation_AcceptsDocumentedValues(t *testing.T) {
	t.Parallel()

	for _, n := range []int64{1, 2} {
		responses := runFileVaultUserValidators(t, n)
		for _, r := range responses {
			if r.Diagnostics.HasError() {
				t.Errorf("expected no plan-time validation error for filevault_user = %d, got: %v", n, r.Diagnostics)
			}
		}
	}
}
