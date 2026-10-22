package platform

import (
	"reflect"
	"testing"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

func vpn(name, password string) sdk.AppleOsXVpnPayloadEntityV2 {
	return sdk.AppleOsXVpnPayloadEntityV2{ConnectionName: name, Password: password}
}

func osx(items ...sdk.AppleOsXVpnPayloadEntityV2) *sdk.AppleOsXDeviceProfileEntityV2 {
	return &sdk.AppleOsXDeviceProfileEntityV2{VpnList: items}
}

func checkWiped(t *testing.T, name string, live, planned *sdk.AppleOsXDeviceProfileEntityV2, want []string) {
	t.Helper()
	if got := OmittedWipedSecrets(live, planned); !reflect.DeepEqual(got, want) {
		t.Errorf("%s: got %v, want %v", name, got, want)
	}
}

// Matched entry omitting a held secret is flagged; a matched entry whose
// live counterpart holds no value for a field is safe; EAS is checked too.
func TestOmittedWipedSecrets(t *testing.T) {
	live := &sdk.AppleOsXDeviceProfileEntityV2{
		VpnList:             []sdk.AppleOsXVpnPayloadEntityV2{{ConnectionName: "a", Password: "*****", SharedSecret: "*****"}},
		EasMicrosoftOutlook: &sdk.AppleOsXEasMicrosoftOutlookPayloadEntityV2{Password: "*****"},
	}
	planned := &sdk.AppleOsXDeviceProfileEntityV2{
		// password sent, shared_secret omitted (held), vpn_password omitted (not held: safe)
		VpnList:             []sdk.AppleOsXVpnPayloadEntityV2{{ConnectionName: "a", Password: "p"}},
		EasMicrosoftOutlook: &sdk.AppleOsXEasMicrosoftOutlookPayloadEntityV2{AccountName: "x"},
	}
	checkWiped(t, "matched", live, planned, []string{"eas_microsoft_outlook.password", `vpn_list["a"].shared_secret`})
	planned.VpnList[0].SharedSecret = "s"
	planned.EasMicrosoftOutlook.Password = "e"
	checkWiped(t, "all sent", live, planned, nil)
	checkWiped(t, "nothing held", osx(vpn("a", "")), osx(vpn("a", "")), nil)
}

// Gate probe 1: a renamed entry omitting a held secret.
func TestOmittedWipedSecrets_RenamedEntry(t *testing.T) {
	checkWiped(t, "rename", osx(vpn("Corp VPN", "*****")), osx(vpn("Corp VPN 2", "")), []string{`vpn_list["Corp VPN 2"].password`})
}

// Gate probe 2: an earlier entry removed and an unnamed entry kept. Position
// matching would compare the wrong entries; any empty planned secret that
// is not name-matched to a live entry without that secret is flagged.
func TestOmittedWipedSecrets_UnnamedAfterRemoval(t *testing.T) {
	checkWiped(t, "unnamed", osx(vpn("A", "*****"), vpn("", "*****")), osx(vpn("", "")), []string{"vpn_list[0].password"})
}

func TestOmittedWipedSecrets_RenamedAndReordered(t *testing.T) {
	live := osx(vpn("A", "*****"), vpn("B", "*****"))
	checkWiped(t, "secrets sent", live, osx(vpn("B", "b"), vpn("A2", "a")), nil)
	checkWiped(t, "renamed omits", live, osx(vpn("B", "b"), vpn("A2", "")), []string{`vpn_list["A2"].password`})
	checkWiped(t, "matched omits", live, osx(vpn("B", ""), vpn("A2", "a")), []string{`vpn_list["B"].password`})
}

// Removing an entry sends nothing for it: nothing to refuse.
func TestOmittedWipedSecrets_RemovedEntry(t *testing.T) {
	checkWiped(t, "removal", osx(vpn("A", "p"), vpn("B", "*****")), osx(vpn("A", "p")), nil)
}

// An added entry without a secret is flagged when any live entry holds that
// secret (fail closed; a false positive only asks the user to set it), and
// is fine when no live entry holds one.
func TestOmittedWipedSecrets_AddedEntry(t *testing.T) {
	checkWiped(t, "added, secret held elsewhere", osx(vpn("A", "*****")), osx(vpn("New", ""), vpn("A", "p")), []string{`vpn_list["New"].password`})
	checkWiped(t, "added, none held", osx(vpn("A", "")), osx(vpn("New", ""), vpn("A", "")), nil)
}
