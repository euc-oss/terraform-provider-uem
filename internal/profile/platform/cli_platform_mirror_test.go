package platform

import (
	"os"
	"regexp"
	"sort"
	"testing"
)

// The ws1-tf CLI is a separate module and can't import this package, so it
// copies SupportedImportPlatforms into its own list to keep unsupported
// profiles out of `terraform import`. This test fails when the two drift.
func TestCLIImportableProfilePlatformsMirrorSupportedImportPlatforms(t *testing.T) {
	if _, err := os.Stat("../../../cli/go.mod"); os.IsNotExist(err) {
		t.Skip("no cli/ module in this checkout")
	}
	src, err := os.ReadFile("../../../cli/internal/onboard/profile_platforms.go")
	if err != nil {
		t.Fatalf("read the CLI's mirrored list: %v", err)
	}
	block := regexp.MustCompile(`(?s)var importableProfilePlatforms = map\[string\]struct\{\}\{(.*?)\n\}`).FindSubmatch(src)
	if block == nil {
		t.Fatal("importableProfilePlatforms not found in the CLI source")
	}
	var cli []string
	for _, m := range regexp.MustCompile(`"([^"]+)":\s*\{\}`).FindAllSubmatch(block[1], -1) {
		cli = append(cli, string(m[1]))
	}
	provider := append([]string(nil), SupportedImportPlatforms...)
	sort.Strings(cli)
	sort.Strings(provider)
	if len(cli) != len(provider) {
		t.Fatalf("CLI list %v != provider SupportedImportPlatforms %v", cli, provider)
	}
	for i := range cli {
		if cli[i] != provider[i] {
			t.Fatalf("CLI list %v != provider SupportedImportPlatforms %v", cli, provider)
		}
	}
}
