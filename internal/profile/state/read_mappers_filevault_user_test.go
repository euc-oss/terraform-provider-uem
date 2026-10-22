package state

import (
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

// Tests in this file cover fileVaultUserToInt64TF and
// promptToEnableFileVaultAtToInt64TF in read_mappers.go: the read mapping
// for disk_encryption.filevault2.filevault_user and
// .prompt_to_enable_filevault_at.
//
// UEM's FileVaultUser/PromptToEnableFileVaultAt/
// EncryptionActionAfterLastNotification properties carry
// [JsonConverter(typeof(StringEnumConverter))] (canonical rules Q3/Q4), so
// GET always returns the enum NAME string ("FileVaultUser":
// "CurrentOrNextLoginUser", "PromptToEnableFileVaultAt":
// "BothLoginAndLogout") -- live-confirmed for the first member of each enum
// in the filevault-user bugfix; the remaining members are mapped from the
// canonical rules report (Q3), not independently live-verified. internal-ticket
// removed the tolerant "also accept a raw numeric string" fallback (row #73
// of the B16 audit: defensive/pre-fix-state only, not confirmed live UEM
// behavior) -- both mappers now map every documented enum member and fall
// back to null (never crash) for any other unrecognized name, including a
// numeric string.

func TestFileVaultUserToInt64TF_NumericStringNoLongerAccepted(t *testing.T) {
	t.Parallel()

	for _, in := range []string{"1", "2"} {
		got := fileVaultUserToInt64TF(in)
		if !got.IsNull() {
			t.Errorf("fileVaultUserToInt64TF(%q) = %v, want null (numeric-string fallback removed)", in, got)
		}
	}
}

func TestFileVaultUserToInt64TF_MapsAllEnumMembers(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		in   string
		want int64
	}{
		{"CurrentOrNextLoginUser", 1},
		{"SpecificUser", 2},
	} {
		got := fileVaultUserToInt64TF(tc.in)
		if got.IsNull() || got.ValueInt64() != tc.want {
			t.Errorf("fileVaultUserToInt64TF(%q) = %v, want %d", tc.in, got, tc.want)
		}
	}
}

func TestFileVaultUserToInt64TF_UnmappedName_ReturnsNullWithoutCrash(t *testing.T) {
	t.Parallel()

	for _, in := range []string{"SomeOtherEnumName", "specificuser", ""} {
		got := fileVaultUserToInt64TF(in)
		if !got.IsNull() {
			t.Errorf("fileVaultUserToInt64TF(%q) = %v, want null", in, got)
		}
	}
}

func TestPromptToEnableFileVaultAtToInt64TF_NumericStringNoLongerAccepted(t *testing.T) {
	t.Parallel()

	for _, in := range []string{"1", "2", "3"} {
		got := promptToEnableFileVaultAtToInt64TF(in)
		if !got.IsNull() {
			t.Errorf("promptToEnableFileVaultAtToInt64TF(%q) = %v, want null (numeric-string fallback removed)", in, got)
		}
	}
}

func TestPromptToEnableFileVaultAtToInt64TF_MapsAllEnumMembers(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		in   string
		want int64
	}{
		{"BothLoginAndLogout", 1},
		{"LogoutOnly", 2},
		{"LoginOnly", 3},
	} {
		got := promptToEnableFileVaultAtToInt64TF(tc.in)
		if got.IsNull() || got.ValueInt64() != tc.want {
			t.Errorf("promptToEnableFileVaultAtToInt64TF(%q) = %v, want %d", tc.in, got, tc.want)
		}
	}
}

func TestPromptToEnableFileVaultAtToInt64TF_UnmappedName_ReturnsNullWithoutCrash(t *testing.T) {
	t.Parallel()

	for _, in := range []string{"SomeOtherEnumName", "loginonly", ""} {
		got := promptToEnableFileVaultAtToInt64TF(in)
		if !got.IsNull() {
			t.Errorf("promptToEnableFileVaultAtToInt64TF(%q) = %v, want null", in, got)
		}
	}
}

func TestEncryptionActionAfterLastNotificationToInt64TF_MapsAllEnumMembers(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		in   string
		want int64
	}{
		{"ForceLogout", 1},
		{"DoNothing", 2},
	} {
		got := encryptionActionAfterLastNotificationToInt64TF(tc.in)
		if got.IsNull() || got.ValueInt64() != tc.want {
			t.Errorf("encryptionActionAfterLastNotificationToInt64TF(%q) = %v, want %d", tc.in, got, tc.want)
		}
	}
}

func TestEncryptionActionAfterLastNotificationToInt64TF_UnmappedName_ReturnsNullWithoutCrash(t *testing.T) {
	t.Parallel()

	for _, in := range []string{"SomeOtherEnumName", "forcelogout", "", "1", "2"} {
		got := encryptionActionAfterLastNotificationToInt64TF(in)
		if !got.IsNull() {
			t.Errorf("encryptionActionAfterLastNotificationToInt64TF(%q) = %v, want null (numeric-string fallback removed)", in, got)
		}
	}
}

// TestMapAppleOsXDiskEncryptionFileVault_EnumNames_EndToEnd proves the whole
// mapAppleOsXDiskEncryptionFileVault mapper (not just the two helpers in
// isolation) hydrates filevault_user and prompt_to_enable_filevault_at
// correctly when UEM returns the live-validated enum names.
func TestMapAppleOsXDiskEncryptionFileVault_EnumNames_EndToEnd(t *testing.T) {
	t.Parallel()

	fv := &sdk.AppleOsXDiskEncryptionFileVault2PayloadEntityV2{
		Enable:                    sdk.BoolPtr(true),
		FileVaultUser:             "CurrentOrNextLoginUser",
		PromptToEnableFileVaultAt: "BothLoginAndLogout",
	}

	got := mapAppleOsXDiskEncryptionFileVault(fv)
	if got == nil {
		t.Fatal("mapAppleOsXDiskEncryptionFileVault returned nil")
	}
	if got.FileVaultUser.IsNull() || got.FileVaultUser.ValueInt64() != 1 {
		t.Errorf("FileVaultUser = %v, want 1", got.FileVaultUser)
	}
	if got.PromptToEnableFileVaultAt.IsNull() || got.PromptToEnableFileVaultAt.ValueInt64() != 1 {
		t.Errorf("PromptToEnableFileVaultAt = %v, want 1", got.PromptToEnableFileVaultAt)
	}
}
