package platform

import (
	"testing"

	profilemodels "github.com/euc-oss/terraform-provider-uem/internal/profile/models"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The three StringEnumConverter-backed disk-encryption fields go on the wire
// as enum NAMES (SDK maintainer report
// 2026-09-25-canonical-262-macos-diskencryption, Q3/Q4). These tables are
// the report's enum definitions; every member is pinned so a swapped or
// missing name fails. The read side pins the same tables in
// internal/profile/state/read_mappers_filevault_user_test.go.

func TestBuildFileVault2Entity_SendsEveryEnumName(t *testing.T) {
	t.Parallel()

	for n, want := range map[int64]string{1: "CurrentOrNextLoginUser", 2: "SpecificUser"} {
		m := &profilemodels.DiskEncryptionFileVaultModel{FileVaultUser: types.Int64Value(n)}
		if got := buildAppleOsXDiskEncryptionFileVault2Entity(m).FileVaultUser; got != want {
			t.Errorf("filevault_user %d: FileVaultUser = %q, want %q", n, got, want)
		}
	}
	for n, want := range map[int64]string{1: "BothLoginAndLogout", 2: "LogoutOnly", 3: "LoginOnly"} {
		m := &profilemodels.DiskEncryptionFileVaultModel{PromptToEnableFileVaultAt: types.Int64Value(n)}
		if got := buildAppleOsXDiskEncryptionFileVault2Entity(m).PromptToEnableFileVaultAt; got != want {
			t.Errorf("prompt_to_enable_filevault_at %d: PromptToEnableFileVaultAt = %q, want %q", n, got, want)
		}
	}
}

func TestBuildAirWatchEntity_SendsEveryEncryptionActionName(t *testing.T) {
	t.Parallel()

	for n, want := range map[int64]string{1: "ForceLogout", 2: "DoNothing"} {
		m := &profilemodels.DiskEncryptionAirWatchModel{EncryptionActionAfterLastNotification: types.Int64Value(n)}
		if got := buildAppleOsXDiskEncryptionAirWatchEntity(m).EncryptionActionAfterLastNotification; got != want {
			t.Errorf("encryption_action_after_last_notification %d: got %q, want %q", n, got, want)
		}
	}
}
