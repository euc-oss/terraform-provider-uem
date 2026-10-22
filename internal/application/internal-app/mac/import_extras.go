package macapplication

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// File names the import writes next to the downloaded application binary,
// inside the same per-application directory (destDir). Kept as constants so
// storage-layout code and docs can reference them.
const (
	importedPlistFileName = "app.plist"
	importedIconFileName  = "icon"
)

// appIconDownloader is an optional extension of appBlobDownloader: a
// downloader that can also fetch the application's icon. Downloaders that
// don't implement it simply import no icon.
type appIconDownloader interface {
	// DownloadAppIcon returns the icon bytes UEM holds for appUUID, or nil
	// (and no error) when the application has no icon. uemFileName (B47) is
	// the icon's own filename as UEM reported it on the blob download (see
	// contentDispositionFileName) — "" when UEM returned no such header, or
	// none survives parsing.
	DownloadAppIcon(ctx context.Context, appUUID string) (icon []byte, uemFileName string, err error)
}

// DownloadAppIcon implements appIconDownloader. UEM reports up to three icon
// blobs (LargeIconBlobGUID, MediumIconBlobGUID, SmallIconBlobGUID) but the
// create body takes a single applicationIconId; the largest one reported is
// downloaded.
//
// B47: BlobsV2Service.Get's response carries a Content-Disposition header
// naming the blob's originally-uploaded filename. Live, as<internal-env> 26.2, GET
// /api/mam/blobs/{guid} at version=2 on an app icon blob returned
// "attachment; filename=<app name>-<stamp>.png; size=40" (version=1 is 404).
// The SDK's mock (testdata/mock-responses/blobs/get_blob_v2.json) shows the
// same shape. The filename is passed to the caller and its extension is used
// when usable; otherwise iconFileName falls back to sniffing the bytes.
func (d *sdkBlobDownloader) DownloadAppIcon(ctx context.Context, appUUID string) ([]byte, string, error) {
	if d.internalAppLookup == nil || d.blobService == nil {
		return nil, "", fmt.Errorf("blob downloader is not configured")
	}
	_, details, err := d.internalAppLookup.GetInternalAppByUuid(ctx, appUUID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch internal app details for uuid %s: %w", appUUID, err)
	}
	if details == nil {
		return nil, "", nil
	}
	guid := firstNonEmpty(details.LargeIconBlobGUID, details.MediumIconBlobGUID, details.SmallIconBlobGUID)
	if guid == "" {
		return nil, "", nil
	}
	headers, icon, err := d.blobService.Get(ctx, guid)
	if err != nil {
		return nil, "", fmt.Errorf("failed to download icon blob %s: %w", guid, err)
	}
	return icon, contentDispositionFileName(headers), nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// writeImportedPlist writes the pkginfo UEM returned on import to
// destDir/app.plist (B32) and returns its path. An empty pkginfo writes
// nothing and returns "".
func writeImportedPlist(destDir, pkginfo string) (string, error) {
	if pkginfo == "" {
		return "", nil
	}
	p := filepath.Join(destDir, importedPlistFileName)
	if err := os.WriteFile(p, []byte(pkginfo), 0o644); err != nil {
		return "", fmt.Errorf("unable to write %s: %w", p, err)
	}
	return p, nil
}

// writeImportedIcon writes icon bytes to destDir/<iconFileName(icon,
// uemFileName)> (B47) and returns its path and SHA-256. Nil or empty bytes
// write nothing and return "", "".
func writeImportedIcon(destDir string, icon []byte, uemFileName string) (string, string, error) {
	if len(icon) == 0 {
		return "", "", nil
	}
	p := filepath.Join(destDir, iconFileName(icon, uemFileName))
	if err := os.WriteFile(p, icon, 0o644); err != nil {
		return "", "", fmt.Errorf("unable to write %s: %w", p, err)
	}
	sum := sha256.Sum256(icon)
	return p, hex.EncodeToString(sum[:]), nil
}
