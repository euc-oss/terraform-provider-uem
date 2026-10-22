package assignment

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/euc-oss/terraform-provider-uem/internal/pagewalk"
	sdk "github.com/euc-oss/terraform-sdk-uem/v26"
)

// B24: this file previously resolved an app's bundle id/org group via the V2
// GetPurchasedApplicationAndAssignments GET (/api/mam/apps/purchased/{uuid})
// before searching V1 by bundle id. That V2 GET's authorization does NOT
// walk up the org group hierarchy: live-confirmed on paul-2609, it returns
// HTTP 200 for an app owned DIRECTLY by the authenticated org group (e.g.
// "Evernote", owned by 402666 itself) but HTTP 404 errorCode 6 ("is not
// found or user doesn't have access to it") for an app owned by an ANCESTOR
// org group (e.g. "Google"/"WhatsApp Messenger", owned by 68381, 402666's
// parent) — even though that same app is perfectly visible, with its
// Platform field already populated, via an UNFILTERED V1
// /api/mam/apps/purchased/search (curl-verified: page 0 of an unfiltered
// search returned all 19 of the tenant's purchased apps, "Google" included,
// with "Uuid" and "Platform" both present on the entity). So there is no
// need for the V2 GET at all: LookupVppPlatform below walks the unfiltered
// V1 search via internal/pagewalk.WalkTwiceVerified (the same hardened,
// base-probing, double-verified walk B12/B1 use for uem_scripts,
// uem_applications, uem_mac_applications) and reads Platform off the
// matching entity — the same "another field" the old code got from V2 plus
// a second V1 call.
//
// B21 had already hardened the old, hand-rolled loop to report page-cap
// exhaustion and a no-progress page as errors instead of a silent Unknown.
// Switching to pagewalk.WalkTwiceVerified keeps both of those error
// semantics (and adds base-probing, a strict "more distinct items than the
// reported total" check, and a full independent second walk to catch a
// catalog that shifted mid-walk) for free — it is the same hardening B21
// wanted, generalized instead of hand-rolled a second time.

// vppPlatform is the VPP app platform as reported by UEM, mapped from the
// numeric Platform code returned by the V1 purchased-app search (internal-task).
// UEM applies restriction flags differently per platform (see
// resource_modify_plan.go), and the resource's own state never tracks
// platform, so it has to be looked up separately.
type vppPlatform int

const (
	vppPlatformUnknown vppPlatform = iota
	vppPlatformIOS
	vppPlatformMacOS
)

// vppPlatformLookupAPI resolves the platform of a purchased VPP application
// by its assignment-resource application_uuid. Unknown/no-match is reported
// as (vppPlatformUnknown, nil) only when a COMPLETE walk of the catalog
// finished without finding appUUID at all — a genuine "this app is not (or
// no longer) in the purchased-app catalog" outcome. Any walk that could not
// complete (page-cap hit, no progress, a total that changed mid-walk, or the
// two independent walks disagreeing) is a hard error, never silently
// downgraded to Unknown — see LookupVppPlatform. ModifyPlan still downgrades
// a genuine error to a warning and proceeds either way; the distinction here
// is only about what LookupVppPlatform itself is allowed to paper over.
type vppPlatformLookupAPI interface {
	LookupVppPlatform(ctx context.Context, appUUID string) (vppPlatform, error)
}

// purchasedAppSearchAPI is the narrow V1 dependency of
// defaultVppPlatformLookup: page through purchased apps and match by uuid to
// recover the Platform field (B24: unfiltered — no bundle id or org group
// needed, see the file-level comment above for why).
type purchasedAppSearchAPI interface {
	VppAppSearchAsync(
		ctx context.Context,
		opts *sdk.PurchasedAppsV1VppAppSearchAsyncOptions,
	) (http.Header, *sdk.PurchasedApplicationSearchResultV1, error)
}

// defaultVppPlatformLookup implements vppPlatformLookupAPI against the real
// SDK: walk the V1 search (unfiltered, base-probed, double-verified via
// pagewalk.WalkTwiceVerified) and match the result back to appUUID directly
// (B24 — no V2 GET involved).
type defaultVppPlatformLookup struct {
	search purchasedAppSearchAPI
}

func newDefaultVppPlatformLookup(c *sdk.Client) vppPlatformLookupAPI {
	return &defaultVppPlatformLookup{
		search: sdk.NewPurchasedAppsV1Service(c),
	}
}

// defaultVppPlatformLookupFactory binds the production vppPlatformLookupAPI
// to the real SDK client. Mirrors defaultPurchasedAppAssignmentServiceFactory
// in resource_support.go.
func defaultVppPlatformLookupFactory(c *sdk.Client) vppPlatformLookupAPI {
	return newDefaultVppPlatformLookup(c)
}

// vppSearchPageSize is the page size requested for VppAppSearchAsync
// pagination (internal-task minor 3). UEM's own default when PageSize is unset is
// 500 (live-verified); requesting it explicitly keeps pagewalk's
// short-page-ends-pagination rule correct regardless of whether that default
// ever changes server-side.
const vppSearchPageSize = 500

// vppSearchMaxPages hard-caps LookupVppPlatform's pagewalk.WalkTwiceVerified
// walk (internal-task minor 3) so a misbehaving server (e.g. one that always
// returns a full page and never runs out of results) cannot loop forever.
// Chosen generously relative to any realistic purchased-app catalog size;
// hitting it is an error (B21/B24), never a silent vppPlatformUnknown.
const vppSearchMaxPages = 20

// vppAppSearchOptions builds the VppAppSearchAsync query for one page of the
// UNFILTERED purchased-app catalog (B24 — no bundle id or org group filter;
// see the file-level comment above for why), paged per the live-verified
// contract for /api/mam/apps/purchased/search (internal-task minor 3): pages are
// 0-indexed, and the response's Total field is the count of items in that
// page — never the overall total — so pagewalk trusts it only across
// non-empty pages and stops on a short (or empty) page instead.
func vppAppSearchOptions(page, pageSize int) *sdk.PurchasedAppsV1VppAppSearchAsyncOptions {
	return &sdk.PurchasedAppsV1VppAppSearchAsyncOptions{
		Page:     &page,
		PageSize: &pageSize,
	}
}

// mapVppPlatform maps the V1 search entity's numeric Platform code to a
// vppPlatform. 2 = Apple/iOS, 10 = AppleOSX/macOS (live-verified, internal-task);
// anything else, including a nil Platform, is unknown.
func mapVppPlatform(platform *int) vppPlatform {
	if platform == nil {
		return vppPlatformUnknown
	}
	switch *platform {
	case 2:
		return vppPlatformIOS
	case 10:
		return vppPlatformMacOS
	default:
		return vppPlatformUnknown
	}
}

// LookupVppPlatform walks the ENTIRE unfiltered purchased-app catalog via
// pagewalk.WalkTwiceVerified (base-probed, deduped by lowercased uuid,
// strict-total-checked, and independently walked twice end-to-end) and then
// searches the complete, verified result for appUUID. Any error from the
// walk itself — page-cap exhaustion, a page that made no progress, a total
// that changed mid-walk, or the two independent walks disagreeing —
// propagates as-is: LookupVppPlatform reports vppPlatformUnknown with no
// error ONLY when the walk completed successfully and appUUID simply was not
// among the items it found (internal-task / B21 / B24).
func (l *defaultVppPlatformLookup) LookupVppPlatform(ctx context.Context, appUUID string) (vppPlatform, error) {
	fetch := func(ctx context.Context, page, pageSize int) (pagewalk.Page[sdk.PurchasedApplicationEntityV1], error) {
		_, result, err := l.search.VppAppSearchAsync(ctx, vppAppSearchOptions(page, pageSize))
		if err != nil {
			return pagewalk.Page[sdk.PurchasedApplicationEntityV1]{}, err
		}
		if result == nil {
			return pagewalk.Page[sdk.PurchasedApplicationEntityV1]{}, nil
		}
		return pagewalk.Page[sdk.PurchasedApplicationEntityV1]{Items: result.Application, Total: result.Total}, nil
	}

	key := func(entity sdk.PurchasedApplicationEntityV1) (string, error) {
		if entity.UUID == "" {
			return "", fmt.Errorf("purchased app search returned an entity with no uuid; cannot page or dedupe it")
		}
		return strings.ToLower(entity.UUID), nil
	}

	result, err := pagewalk.WalkTwiceVerified(ctx, fetch, key, vppSearchPageSize, vppSearchMaxPages, "purchased app search")
	if err != nil {
		return vppPlatformUnknown, err
	}

	for _, entity := range result.Items {
		if strings.EqualFold(entity.UUID, appUUID) {
			return mapVppPlatform(entity.Platform), nil
		}
	}
	return vppPlatformUnknown, nil
}
