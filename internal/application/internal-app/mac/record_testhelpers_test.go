package macapplication

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/types"

	tf "github.com/euc-oss/terraform-provider-uem/internal/application/internal-app/mac/models"
)

// withTypedRecordNulls gives the record's composite attributes typed null
// values, so a model built as a literal in a test can be Set into state.
func withTypedRecordNulls(m tf.MacApplicationResourceModel) tf.MacApplicationResourceModel {
	named := types.ObjectType{AttrTypes: namedRefAttrTypes}
	if m.CategoryList.ElementType(context.Background()) == nil {
		m.CategoryList = types.ListNull(named)
	}
	if m.SupportedModels.ElementType(context.Background()) == nil {
		m.SupportedModels = types.ListNull(named)
	}
	if m.SupportedModelsName.ElementType(context.Background()) == nil {
		m.SupportedModelsName = types.ListNull(types.StringType)
	}
	if len(m.MacOsSoftwareDeploymentSummary.AttributeTypes(context.Background())) == 0 {
		m.MacOsSoftwareDeploymentSummary = types.ObjectNull(deploymentSummaryAttrTypes)
	}
	return m
}
