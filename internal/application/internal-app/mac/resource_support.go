package macapplication

import (
	"bytes"
	"context"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	sdk "github.com/euc-oss/terraform-sdk-uem/v26"

	commonerrors "github.com/euc-oss/terraform-provider-uem/internal/common/errors"
	"github.com/euc-oss/terraform-provider-uem/internal/httpclient"
)

// artifactDir is the single, shared place that computes where a given
// imported application's on-disk artifacts live:
// <storageRoot>/<uuid>/. ImportState uses it for the downloaded
// app.pkg/app.dmg blob; it is also the directory F9's plist/icon import
// writes importedPlistFileName/importedIconFileName into — both features
// must agree on this directory, so it is computed in exactly one function
// rather than each call site re-deriving filepath.Join(storageRoot, uuid).
//
// It relies on filepath.Join already calling filepath.Clean on its result,
// so this never needs a separate Clean call; it also never calls
// filepath.Abs — storageRoot may be relative (see the provider's
// app_binary_storage_path default), and this function preserves that,
// resolving later only against whatever process cwd is in effect when the
// file is actually opened/written.
func artifactDir(storageRoot, uuid string) string {
	return filepath.Join(storageRoot, uuid)
}

// displayAppName is the human-facing name ImportState's B10 log lines and
// stall-diagnostic use: the application's real name when known, falling
// back to its UUID (still unambiguous, just less friendly) when the details
// lookup left ApplicationName empty.
func displayAppName(name, appUUID string) string {
	if name != "" {
		return name
	}
	return appUUID
}

// stallDiagnosticMessage renders the required user-facing diagnostic for a
// download that stalled mid-transfer (B10): "download of <name> stalled
// after N of M MB; check the network and re-run onboard" when the total size
// is known, or "...stalled after N MB; ..." when it is not (e.g. the server
// omitted Content-Length).
func stallDiagnosticMessage(name string, e *httpclient.StallError) string {
	readMB := float64(e.BytesRead) / (1024 * 1024)
	if e.TotalBytes >= 0 {
		totalMB := float64(e.TotalBytes) / (1024 * 1024)
		return fmt.Sprintf("download of %s stalled after %.1f of %.1f MB; check the network and re-run onboard", name, readMB, totalMB)
	}
	return fmt.Sprintf("download of %s stalled after %.1f MB; check the network and re-run onboard", name, readMB)
}

type appBlobV1ResourceServiceAPI interface {
	UploadBlobAsync(
		ctx context.Context,
		request []byte,
		opts *sdk.BlobsV1UploadBlobAsyncOptions,
	) (http.Header, *sdk.EntityV1Model, error)
}

type appBlobV2ResourceServiceAPI interface {
	Delete(
		ctx context.Context,
		BlobID string,
	) (http.Header, error)

	Get(
		ctx context.Context,
		BlobID string,
	) (http.Header, []byte, error)
}

// internalAppUUIDLookupAPI resolves an internal application's details by
// UUID. It is the narrow seam used by sdkBlobDownloader to translate an
// imported application's UUID into its application file blob GUID.
type internalAppUUIDLookupAPI interface {
	GetInternalAppByUuid(
		ctx context.Context,
		UUID string,
	) (http.Header, *sdk.InternalAppModelV2, error)
}

type macAppResourceServiceAPI interface {
	CreateMacOSApplication(
		ctx context.Context,
		ID int,
		request *sdk.MacOsCreateApplicationRequestV1Model,
	) (http.Header, error)
}

type InternalAppsV1ServiceAPI interface {
	DeleteInternalAppAsync(
		ctx context.Context,
		ApplicationID int,
	) (http.Header, error)

	GetInternalAppByIdAsync(
		ctx context.Context,
		ApplicationID int,
	) (http.Header, *sdk.InternalAppModelV1, error)

	// no update
}

type resourceConfigData struct {
	client                        *sdk.Client
	newBlobV1ResourceService      func(c *sdk.Client) appBlobV1ResourceServiceAPI
	newBlobV2ResourceService      func(c *sdk.Client) appBlobV2ResourceServiceAPI
	newMacAppResourceService      func(c *sdk.Client) macAppResourceServiceAPI
	newInternalAppResourceService func(c *sdk.Client) InternalAppsV1ServiceAPI
}

func defaultBlobV1ResourceServiceFactory(c *sdk.Client) appBlobV1ResourceServiceAPI {
	return sdk.NewBlobsV1Service(c)
}

func defaultBlobV2ResourceServiceFactory(c *sdk.Client) appBlobV2ResourceServiceAPI {
	return sdk.NewBlobsV2Service(c)
}

func defaultMacAppResourceServiceFactory(c *sdk.Client) macAppResourceServiceAPI {
	return sdk.NewMacOsAppsV1Service(c)
}

func defaultInternalAppResourceServiceFactory(c *sdk.Client) InternalAppsV1ServiceAPI {
	return sdk.NewInternalAppsV1Service(c)
}

func isNotFoundAPIError(err error) bool {
	return commonerrors.IsNotFoundAPIError(err)
}

// xarMagic is the 4-byte header of a xar archive ("xar!"), the on-disk
// format of a macOS flat package (.pkg). See
// https://github.com/apple-oss-distributions/xar (xar/lib/xar.h,
// XAR_HEADER_MAGIC) for the format; UEM stores a flat package's bytes
// unchanged, so this signature survives the round trip through the blob
// download.
var xarMagic = []byte("xar!")

// blobFileName picks the on-disk filename ImportState saves a downloaded
// application blob under, sniffed from its first bytes: a flat package
// (xar archive) is named app.pkg, everything else (a real DMG, or any input
// too short to tell) is named app.dmg, preserving the pre-existing name for
// the common case.
func blobFileName(firstBytes []byte) string {
	if bytes.HasPrefix(firstBytes, xarMagic) {
		return "app.pkg"
	}
	return "app.dmg"
}

// contentDispositionFileName extracts the "filename" parameter from a
// Content-Disposition header value (e.g. `attachment; filename=icon.png`).
// It returns "" when headers is nil, the header is absent or empty, or it
// doesn't parse as a valid media-type parameter list — never an error, since
// this is best-effort metadata and every caller already has a working
// fallback for "unknown".
func contentDispositionFileName(headers http.Header) string {
	if headers == nil {
		return ""
	}
	v := headers.Get("Content-Disposition")
	if v == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(v)
	if err != nil {
		return ""
	}
	return params["filename"]
}

// usableFileExt returns name's extension, lowercased and including its
// leading dot (e.g. ".png"), when it looks like a genuine file extension: 1
// to 5 ASCII letters/digits right after the final dot, with at least one
// character before that dot. It returns "" for a name with no dot, a name
// ending in a bare dot, or a suffix containing anything but letters/digits
// (e.g. a trailing "(1)", a version-looking ".1.2", or a name that is
// entirely an extension like ".png" with nothing preceding it) — none of
// which this provider has confirmed UEM would treat as a real extension, so
// they fall through to content sniffing instead of being trusted blind.
func usableFileExt(name string) string {
	ext := filepath.Ext(name)
	if len(ext) < 2 || len(ext) > 6 || len(ext) == len(name) {
		return ""
	}
	for _, r := range ext[1:] {
		isDigit := r >= '0' && r <= '9'
		isLetter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		if !isDigit && !isLetter {
			return ""
		}
	}
	return strings.ToLower(ext)
}

// iconFileName picks the on-disk filename ImportState saves a downloaded
// application icon under (B47).
//
// UEM's own filename for the icon blob — uemFileName, from the
// Content-Disposition header on its download (contentDispositionFileName) —
// is preferred when it carries a usable extension (usableFileExt): that
// preserves whatever type UEM itself considers the icon, including one this
// function's own sniffing below doesn't recognize (e.g. a non-raster
// .icns). Failing that, firstBytes is sniffed with net/http.DetectContentType;
// only PNG/JPEG/GIF are turned into an extension, since those are the only
// icon types this provider has confirmed handling for. Anything else keeps
// the extension-less importedIconFileName ("icon") — the CLI's onboard
// render must then warn that the extension needs to be set by hand before
// the app is re-created elsewhere, since UEM types a re-uploaded blob by its
// filename's extension and an extension-less name fails with "Invalid blob
// type" (the bug this function fixes).
func iconFileName(firstBytes []byte, uemFileName string) string {
	if ext := usableFileExt(uemFileName); ext != "" {
		return importedIconFileName + ext
	}
	switch http.DetectContentType(firstBytes) {
	case "image/png":
		return importedIconFileName + ".png"
	case "image/jpeg":
		return importedIconFileName + ".jpg"
	case "image/gif":
		return importedIconFileName + ".gif"
	default:
		return importedIconFileName
	}
}

func resourceTypeErrorDetail(got any) string {
	return fmt.Sprintf(
		"Expected *resourceConfigData or *sdk.Client, got: %T. Please report this issue to the provider developers.",
		got,
	)
}

func CreateBlobUploadOptions(
	fileName string,
	isApplication bool,
	orgGroupID int) *sdk.BlobsV1UploadBlobAsyncOptions {

	opts := &sdk.BlobsV1UploadBlobAsyncOptions{
		FileName:            fileName,
		OrganizationGroupID: orgGroupID,
	}

	applicationModuleType := "Application"
	if isApplication {
		opts.ModuleType = &applicationModuleType
	}

	return opts
}

func CreateMacOSApplicationRequest(
	dmgBlobID int,
	plistBlobID int,
	iconBlobID *int,
	version string,
) *sdk.MacOsCreateApplicationRequestV1Model {
	return &sdk.MacOsCreateApplicationRequestV1Model{
		ApplicationBlobID: &dmgBlobID,
		ApplicationIconID: iconBlobID,
		PkgInfoBlobID:     &plistBlobID,
		Version:           version,
	}
}

func parseAppIDFromLocation(header http.Header) (int, error) {
	location := header.Get("Location")
	location = strings.TrimSpace(location)
	if location == "" {
		return 0, fmt.Errorf("location is empty")
	}

	parts := strings.Split(strings.TrimRight(location, "/"), "/")
	if len(parts) == 0 {
		return 0, fmt.Errorf("invalid location format")
	}

	appID, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return 0, fmt.Errorf("invalid application ID format: %w", err)
	}

	return appID, nil
}
