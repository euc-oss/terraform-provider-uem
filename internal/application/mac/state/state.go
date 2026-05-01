package state

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/types"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/mac/models"
)

func SetMinimalState(
	data *tf.MacApplicationResourceModel,
	id int,
	applicationUUID string) error {

	_, err := tf.ValidateAppUUID(applicationUUID)
	if err != nil {
		return fmt.Errorf("invalid application uuid: %w", err)
	}

	_, err = tf.ValidateID(id)
	if err != nil {
		return fmt.Errorf("invalid application id: %w", err)
	}
	data.ID = types.Int32Value(int32(id))
	data.UUID = types.StringValue(applicationUUID)
	return nil
}
