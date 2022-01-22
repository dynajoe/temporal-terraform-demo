package workflows

import (
	"go.temporal.io/sdk/worker"
)

func Register(w worker.Worker) {
	w.RegisterWorkflow(CreateDemoNetworkWorkflow)
	w.RegisterActivity(CreateVPCActivity)
	w.RegisterActivity(CreateSubnetsActivity)

	w.RegisterWorkflow(DestroyDemoNetworkWorkflow)
	w.RegisterActivity(DestroyVPCActivity)
	w.RegisterActivity(DestroySubnetsActivity)
}
