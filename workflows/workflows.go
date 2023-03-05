package workflows

import (
	"go.temporal.io/sdk/worker"
)

func Register(w worker.Worker) {
	w.RegisterWorkflow(CreateDemoNetworkWorkflow)
	w.RegisterWorkflow(CreateVPCWorkflow)
	w.RegisterWorkflow(CreateSubnetsWorkflow)

	w.RegisterWorkflow(TerraformPlanAndApplyWorkflow)
	w.RegisterActivity(TerraformPlanActivity)
	w.RegisterActivity(TerraformApplyActivity)
	w.RegisterActivity(TerraformBundleEmbeddedTerraformActivity)
}
