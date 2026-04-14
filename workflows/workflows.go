package workflows

import (
	"go.temporal.io/sdk/worker"
)

func Register(w worker.Worker) {
	w.RegisterWorkflow(CreateDemoNetworkWorkflow)
	w.RegisterWorkflow(CreateVPCWorkflow)
	w.RegisterWorkflow(CreateSubnetsWorkflow)

	w.RegisterWorkflow(DestroyDemoNetworkWorkflow)
	w.RegisterWorkflow(DestroyVPCWorkflow)
	w.RegisterWorkflow(DestroySubnetsWorkflow)

	w.RegisterWorkflow(TerraformPlanAndApplyWorkflow)
	w.RegisterWorkflow(TerraformPlanAndDestroyWorkflow)

	w.RegisterActivity(TerraformPlanActivity)
	w.RegisterActivity(TerraformApplyActivity)
	w.RegisterActivity(TerraformDestroyPlanActivity)
	w.RegisterActivity(TerraformBundleEmbeddedTerraformActivity)
	w.RegisterActivity(TerraformBundleEmbeddedTerraformDestroyActivity)
}
