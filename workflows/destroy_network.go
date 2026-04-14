package workflows

import (
	"fmt"

	"go.temporal.io/sdk/workflow"
)

type DestroyDemoNetworkInput struct {
	Name   string
	Region string
}

func DestroyDemoNetworkWorkflow(ctx workflow.Context, input DestroyDemoNetworkInput) error {
	// Destroy subnets first, then VPC (reverse order of creation)
	if err := DestroySubnetsWorkflow(ctx, DestroySubnetsInput{
		Name:   input.Name,
		Region: input.Region,
	}); err != nil {
		return err
	}

	if err := DestroyVPCWorkflow(ctx, DestroyVPCInput{
		Name:   input.Name,
		Region: input.Region,
	}); err != nil {
		return err
	}

	return nil
}

type DestroyVPCInput struct {
	Name   string
	Region string
}

func DestroyVPCWorkflow(ctx workflow.Context, input DestroyVPCInput) error {
	return TerraformPlanAndDestroyWorkflow(ctx, DestroyTerraformInput{
		TerraformPath: "aws/vpc",
		Env: map[string]string{
			"AWS_REGION": input.Region,
		},
		StateKey: fmt.Sprintf("vpc-%s.tfstate", input.Name),
	})
}

type DestroySubnetsInput struct {
	Name   string
	Region string
}

func DestroySubnetsWorkflow(ctx workflow.Context, input DestroySubnetsInput) error {
	return TerraformPlanAndDestroyWorkflow(ctx, DestroyTerraformInput{
		TerraformPath: "aws/subnet",
		Env: map[string]string{
			"AWS_REGION": input.Region,
		},
		StateKey: fmt.Sprintf("subnets-%s.tfstate", input.Name),
	})
}
