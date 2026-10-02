package platform

import (
	"fmt"
	"sort"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

// OmittedWipedSecrets returns the write-only VPN and Exchange (Microsoft
// Outlook) secrets that UEM holds (its read shows the *****
// mask) but that the planned entity would omit. UEM clears such a secret on
// update rather than keeping it, live-confirmed on as<internal-env> on 2026-09-26: a
// VPN profile whose GET showed Password and SharedSecret as ***** read back
// null for both after an update that sent neither (only Server changed),
// and an EAS Outlook profile's ***** Password read back null after an update
// that changed only ExchangeHost. The network_list passwords and credentials_list
// certificates are kept in the same situation (F10 part 2; the credential
// overlay reuses the live certificate), so they are not checked here.
//
// VPN entries are checked per secret field, fail closed, because the update
// replaces the whole list: a planned entry that leaves a secret empty is
// safe only when it name-matches a live entry that holds no value for that
// field. Any other empty planned secret (unnamed, renamed, reordered, or
// added) is flagged whenever ANY live entry holds a value for that field.
// This errs toward asking for a secret that may not be needed rather than
// letting one be cleared silently. The result is sorted, for stable messages.
func OmittedWipedSecrets(live, planned *sdk.AppleOsXDeviceProfileEntityV2) []string {
	if live == nil || planned == nil {
		return nil
	}
	var out []string
	masked := func(v string) bool { return v == UEMMaskedSecret }
	fields := []struct {
		attr string
		get  func(sdk.AppleOsXVpnPayloadEntityV2) string
	}{
		{"password", func(v sdk.AppleOsXVpnPayloadEntityV2) string { return v.Password }},
		{"shared_secret", func(v sdk.AppleOsXVpnPayloadEntityV2) string { return v.SharedSecret }},
		{"vpn_password", func(v sdk.AppleOsXVpnPayloadEntityV2) string { return v.VpnPassword }},
	}
	for _, f := range fields {
		anyHeld := false
		heldByName := map[string]bool{} // named live entries holding this secret
		liveNames := map[string]bool{}  // every named live entry
		for _, lv := range live.VpnList {
			held := masked(f.get(lv))
			anyHeld = anyHeld || held
			if lv.ConnectionName != "" {
				liveNames[lv.ConnectionName] = true
				heldByName[lv.ConnectionName] = heldByName[lv.ConnectionName] || held
			}
		}
		if !anyHeld {
			continue
		}
		for i, pv := range planned.VpnList {
			if f.get(pv) != "" {
				continue
			}
			name := pv.ConnectionName
			if name != "" && liveNames[name] && !heldByName[name] {
				continue // matches a live entry that holds no value for it
			}
			if name == "" {
				out = append(out, fmt.Sprintf("vpn_list[%d].%s", i, f.attr))
			} else {
				out = append(out, fmt.Sprintf("vpn_list[%q].%s", name, f.attr))
			}
		}
	}
	if live.EasMicrosoftOutlook != nil && planned.EasMicrosoftOutlook != nil &&
		masked(live.EasMicrosoftOutlook.Password) && planned.EasMicrosoftOutlook.Password == "" {
		out = append(out, "eas_microsoft_outlook.password")
	}
	sort.Strings(out)
	return out
}
