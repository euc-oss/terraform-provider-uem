package macapplication

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// maxPlistIdentityBytes bounds how much of a plist_file_path file is read at
// plan time. A pkginfo plist is a few KB; anything larger than this is not a
// plausible plist, so it fails safe (replace) instead of being read whole.
const maxPlistIdentityBytes = 16 << 20

// fileSHA256 is the dmg_file_path content identity: the plain SHA-256 of the
// file bytes (the same digest Create/ImportState record in dmg_file_sha256).
// It streams the file so a multi-GB DMG is never loaded into memory.
func fileSHA256(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close() //nolint:errcheck // read-only handle

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// plistFileCanonicalSHA256 is the plist_file_path content identity: the
// canonical-form SHA-256 (canonicalPlistSHA256). Files larger than
// maxPlistIdentityBytes are an error (fail safe: replace).
func plistFileCanonicalSHA256(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close() //nolint:errcheck // read-only handle

	b, err := io.ReadAll(io.LimitReader(f, maxPlistIdentityBytes+1))
	if err != nil {
		return "", err
	}
	if len(b) > maxPlistIdentityBytes {
		return "", fmt.Errorf("plist %s exceeds %d bytes", filePath, maxPlistIdentityBytes)
	}
	return canonicalPlistSHA256(b)
}

// pathContentChanged decides whether a changed *_file_path attribute forces
// replacement. It is only called by stringplanmodifier.RequiresReplaceIf, which
// skips create/destroy and unchanged paths. The resource is
// kept (an in-place Update that only persists the new path — no upload, no
// API write) only when the file at the planned path has the same content
// identity as the one recorded in state under shaAttr. Every other case
// replaces, failing safe:
//   - the planned path is unknown (or null);
//   - the recorded identity is null/empty — i.e. state predating the
//     identity attribute, which is exactly the pre-existing always-replace
//     behavior;
//   - the file is missing/unreadable, or its identity cannot be computed;
//   - the identities differ.
func pathContentChanged(ctx context.Context, req planmodifier.StringRequest, shaAttr string, identity func(filePath string) (string, error)) bool {
	if req.PlanValue.IsNull() || req.PlanValue.IsUnknown() {
		return true
	}

	var recorded types.String
	if diags := req.State.GetAttribute(ctx, path.Root(shaAttr), &recorded); diags.HasError() {
		return true
	}
	if recorded.IsNull() || recorded.IsUnknown() || recorded.ValueString() == "" {
		return true
	}

	got, err := identity(req.PlanValue.ValueString())
	if err != nil {
		return true
	}
	return !strings.EqualFold(got, recorded.ValueString())
}

// cleanEquivalentPathUseState is a LOCAL FILESYSTEM PATH comparison — not a
// UEM-observed rule of any kind — that treats two path strings as the same
// path when they are equal after filepath.Clean (e.g. "./../../x" and
// "../../x" both Clean to "../x"). When a configured *_file_path differs
// from the recorded state value only in this cosmetic way, it keeps the
// state value so Terraform plans no diff at all for the attribute. A
// genuinely different path (one that does not Clean-equal the recorded
// value) is left untouched, falling through unchanged to the
// content-identity RequiresReplaceIf modifiers below, which still resolve a
// same-content change to an in-place update and a different-content change
// to a replace, exactly as before this modifier existed.
type cleanEquivalentPathUseState struct{}

func (cleanEquivalentPathUseState) Description(context.Context) string {
	return "Keeps the recorded state path when the configured path is the same path as the recorded one after " +
		"filepath.Clean, so purely cosmetic path-string differences (e.g. \"./a\" vs \"a\") plan no diff."
}

func (m cleanEquivalentPathUseState) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (cleanEquivalentPathUseState) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.PlanValue.IsNull() || req.PlanValue.IsUnknown() {
		return
	}
	if req.StateValue.IsNull() || req.StateValue.IsUnknown() {
		return
	}
	planStr, stateStr := req.PlanValue.ValueString(), req.StateValue.ValueString()
	if planStr == stateStr {
		return
	}
	if filepath.Clean(planStr) == filepath.Clean(stateStr) {
		resp.PlanValue = req.StateValue
	}
}

// Live-confirmed 2026-09-23 on as<internal-env> 26.2: a dmg_file_path change to a file
// with the same content (sha256 matches dmg_file_sha256) is an in-place
// update with no upload; a real content change replaces. Evidence:
// internal-design-doc;
// commit c03a7267ea.
func dmgFilePathRequiresReplace() planmodifier.String {
	const desc = "Changing dmg_file_path forces replacement unless the file at the new path has the same " +
		"SHA-256 as the recorded dmg_file_sha256, in which case only the path is updated in place (no upload)."
	return stringplanmodifier.RequiresReplaceIf(
		func(ctx context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
			resp.RequiresReplace = pathContentChanged(ctx, req, "dmg_file_sha256", fileSHA256)
		},
		desc, desc,
	)
}

// Live-confirmed 2026-09-23 on as<internal-env> 26.2: the canonical plist sha stayed
// byte-stable across 4 live GETs (Pkginfo), so a same-identity
// plist_file_path change is an in-place update with no upload; the
// canonicalization rules themselves (sorted keys, dropped whitespace) are
// provider-local, not server-observed. Evidence:
// internal-design-doc;
// commit c03a7267ea.
func plistFilePathRequiresReplace() planmodifier.String {
	const desc = "Changing plist_file_path forces replacement unless the plist at the new path has the same " +
		"canonical SHA-256 as the recorded plist_file_sha256, in which case only the path is updated in place " +
		"(no upload). State without plist_file_sha256 (written before it existed) always replaces, as before."
	return stringplanmodifier.RequiresReplaceIf(
		func(ctx context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
			// A null plist_file_sha256 (pre-existing state, or a plist that
			// could not be canonicalized) replaces — identical to the old
			// unconditional RequiresReplace behavior.
			resp.RequiresReplace = pathContentChanged(ctx, req, "plist_file_sha256", plistFileCanonicalSHA256)
		},
		desc, desc,
	)
}

// iconFilePathRequiresReplace applies dmg_file_path's content-identity rule to
// the icon: the icon is a create-only input (applicationIconId) and UEM has no
// update for the record, so a content change replaces.
func iconFilePathRequiresReplace() planmodifier.String {
	const desc = "Changing icon_file_path forces replacement unless the file at the new path has the same " +
		"SHA-256 as the recorded icon_file_sha256, in which case only the path is updated in place (no upload)."
	return stringplanmodifier.RequiresReplaceIf(
		func(ctx context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
			resp.RequiresReplace = pathContentChanged(ctx, req, "icon_file_sha256", fileSHA256)
		},
		desc, desc,
	)
}
