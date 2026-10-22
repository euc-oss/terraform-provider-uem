package state

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/types"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/internal-app/mac/models"
)

// SetMinimalState populates the identity fields after a successful
// GET/Create response: id always, uuid only when UEM actually resolved one.
//
// UEM source: AirWatch API/AW.Mam.Api/AW.Mam.Api.Model.Mappers/InternalAppModelMapper.cs:179-180,
// AirWatch API/AW.Mam.Api/AW.Mam.Api.Model/Apps/InternalAppModel.cs:413-416
// (canonical Q10): Id is always set on a successful GET, but Uuid is
// resolved separately (ResolveUuidsForInternalAppModel) and can legitimately
// come back null/blank if that resolution fails — "not always both
// populated". B16 decision table row #102 (CORRECT): this previously
// hard-failed the ENTIRE refresh (Read or Create's readback) whenever
// applicationUUID was blank or didn't parse as 8-4-4-4-12 hex, which is
// wrong — only id is a reliable-enough identity signal to fail on. A
// blank/malformed uuid is now tolerated: the prior state's uuid is kept
// (chosen over forcing null) rather than erroring, since id remains the
// authoritative identity either way and clearing uuid to null would only
// produce a spurious diff/replacement risk for no benefit.
func SetMinimalState(
	data *tf.MacApplicationResourceModel,
	id int,
	applicationUUID string) error {

	if _, err := tf.ValidateID(id); err != nil {
		return fmt.Errorf("invalid application id: %w", err)
	}
	data.ID = types.Int32Value(int32(id))

	if _, err := tf.ValidateAppUUID(applicationUUID); err == nil {
		data.UUID = types.StringValue(applicationUUID)
	}
	// else: UEM did not resolve a uuid for this application (canonical Q10)
	// — data.UUID is left as whatever it already was (the prior state on
	// Read, or the already-planned/empty value on Create), never cleared to
	// null and never failing the refresh.

	return nil
}
