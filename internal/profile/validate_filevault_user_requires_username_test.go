package profile

import (
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Tests in this file cover validateFileVaultUserSpecificRequiresUsername in
// internal/profile/validate.go: disk_encryption.filevault2.filevault_user =
// 2 (Specific User) requires disk_encryption.filevault2.username.

const fileVaultUserRequiresUsernameSummary = "username is required when filevault_user is 2 (Specific User)"

func TestValidateConfig_FileVaultUserSpecific_UsernameNull_Rejected(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(map[string]tftypes.Value{
			"filevault_user": int64Val(2),
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	wantPath := "disk_encryption.filevault2.username"
	errs := diagErrorsAtPath(resp, wantPath)
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error at %s, got %d: %s", wantPath, len(errs), diagSummaries(resp))
	}
	if errs[0] != fileVaultUserRequiresUsernameSummary {
		t.Errorf("expected summary %q, got %q", fileVaultUserRequiresUsernameSummary, errs[0])
	}
}

func TestValidateConfig_FileVaultUserSpecific_UsernameSet_NoError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(map[string]tftypes.Value{
			"filevault_user": int64Val(2),
			"username":       stringVal("od-user"),
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	if errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.username"); len(errs) != 0 {
		t.Fatalf("expected no error with username set, got: %v", errs)
	}
}

func TestValidateConfig_FileVaultUserSpecific_FileVaultUserUnknown_NoError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(map[string]tftypes.Value{
			"filevault_user": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
			// username left null: if filevault_user's unknown-ness weren't
			// respected, this would otherwise fire.
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	if errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.username"); len(errs) != 0 {
		t.Fatalf("expected no error with filevault_user unknown, got: %v", errs)
	}
}

// TestValidateConfig_FileVaultUserSpecific_OtherPlatform_NoError proves the
// check is macOS-only, matching the other filevault2 checks.
func TestValidateConfig_FileVaultUserSpecific_OtherPlatform_NoError(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(map[string]tftypes.Value{
			"filevault_user": int64Val(2),
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleiOS, de)

	if errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.username"); len(errs) != 0 {
		t.Fatalf("expected no error on %q, got: %v", sdk.PlatformAppleiOS, errs)
	}
}

// TestValidateConfig_FileVaultUserCurrentOrNext_UsernameNotRequired proves
// filevault_user = 1 (Current or Next Login User, the "condition false"
// case) never requires username, even with it null.
func TestValidateConfig_FileVaultUserCurrentOrNext_UsernameNotRequired(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(map[string]tftypes.Value{
			"filevault_user": int64Val(1),
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	if errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.username"); len(errs) != 0 {
		t.Fatalf("expected no error with filevault_user = 1, got: %v", errs)
	}
}

// TestValidateConfig_FileVaultUserSpecific_UsernameEmpty_Rejected proves an
// empty username counts as unset: UEM rejects a null or empty Username when
// FileVaultUser is SpecificUser (canonical 26.2 rules Q2).
func TestValidateConfig_FileVaultUserSpecific_UsernameEmpty_Rejected(t *testing.T) {
	t.Parallel()

	de := diskEncryptionVal(map[string]tftypes.Value{
		"filevault2": filevaultVal(map[string]tftypes.Value{
			"filevault_user": int64Val(2),
			"username":       stringVal(""),
		}),
	})
	resp := runValidateConfigWithDiskEncryption(t, sdk.PlatformAppleOsX, de)

	errs := diagErrorsAtPath(resp, "disk_encryption.filevault2.username")
	if len(errs) != 1 || errs[0] != fileVaultUserRequiresUsernameSummary {
		t.Fatalf("expected exactly 1 %q error, got %v", fileVaultUserRequiresUsernameSummary, errs)
	}
}
