package state

import (
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

// internal-ticket removed mergeFileVault (row #86 of the B16 audit): FileVaultUser
// and PromptToEnableFileVaultAt no longer fall back to the prior state's
// value when UEM returns an unmapped enum name. TestMapAppleOsXDiskEncryptionFileVault_UnmappedEnumNameIsNull
// asserts the new pass-through contract directly against the read mapper
// instead.
func TestMapAppleOsXDiskEncryptionFileVault_UnmappedEnumNameIsNull(t *testing.T) {
	t.Parallel()

	fv := &sdk.AppleOsXDiskEncryptionFileVault2PayloadEntityV2{
		FileVaultUser:             "SomeUnmappedEnumName",
		PromptToEnableFileVaultAt: "AlsoUnmapped",
	}

	got := mapAppleOsXDiskEncryptionFileVault(fv)
	if got == nil {
		t.Fatal("expected non-nil DiskEncryptionFileVaultModel")
	}
	if !got.FileVaultUser.IsNull() {
		t.Errorf("FileVaultUser = %v, want null (unmapped enum name, no prior-state fallback)", got.FileVaultUser)
	}
	if !got.PromptToEnableFileVaultAt.IsNull() {
		t.Errorf("PromptToEnableFileVaultAt = %v, want null (unmapped enum name, no prior-state fallback)", got.PromptToEnableFileVaultAt)
	}
}

// TestMapAppleOsXDiskEncryptionFileVault_KnownEnumNameMaps proves a
// recognized enum name still maps to its schema int.
func TestMapAppleOsXDiskEncryptionFileVault_KnownEnumNameMaps(t *testing.T) {
	t.Parallel()

	fv := &sdk.AppleOsXDiskEncryptionFileVault2PayloadEntityV2{
		FileVaultUser:             "SpecificUser",
		PromptToEnableFileVaultAt: "LoginOnly",
	}

	got := mapAppleOsXDiskEncryptionFileVault(fv)
	if got == nil {
		t.Fatal("expected non-nil DiskEncryptionFileVaultModel")
	}
	if got.FileVaultUser.ValueInt64() != 2 {
		t.Errorf("FileVaultUser = %v, want 2", got.FileVaultUser)
	}
	if got.PromptToEnableFileVaultAt.ValueInt64() != 3 {
		t.Errorf("PromptToEnableFileVaultAt = %v, want 3", got.PromptToEnableFileVaultAt)
	}
}
