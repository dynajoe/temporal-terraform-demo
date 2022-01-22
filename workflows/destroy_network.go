package workflows

import (
	"context"
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/dynajoe/temporal-terraform-demo/config/awsconfig"
	"github.com/dynajoe/temporal-terraform-demo/terraform"
	"github.com/dynajoe/temporal-terraform-demo/tfactivity"
	"github.com/dynajoe/temporal-terraform-demo/tfexec"
	"github.com/dynajoe/temporal-terraform-demo/tfworkspace"
)

type DestroyDemoNetworkInput struct {
	Name   string
	Region string
}

func DestroyDemoNetworkWorkflow(ctx workflow.Context, input DestroyDemoNetworkInput) error {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: time.Hour,
		HeartbeatTimeout:    time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    10 * time.Second,
			BackoffCoefficient: 1.3,
			MaximumInterval:    2 * time.Minute,
		},
	})

	if err := workflow.ExecuteActivity(ctx, DestroySubnetsActivity, input).Get(ctx, nil); err != nil {
		return err
	}

	if err := workflow.ExecuteActivity(ctx, DestroyVPCActivity, input).Get(ctx, nil); err != nil {
		return err
	}

	return nil
}

func DestroyVPCActivity(ctx context.Context, input DestroyDemoNetworkInput) error {
	awsConfig := awsconfig.LoadConfig()

	tfa := tfactivity.New(tfworkspace.Config{
		TerraformPath: "aws/vpc",
		TerraformFS:   terraform.FS,
		S3Backend: tfexec.S3BackendConfig{
			Credentials: awsConfig.Credentials,
			Region:      "us-west-2",
			Bucket:      "temporal-terraform-demo-state",
			Key:         fmt.Sprintf("vpc-%s.tfstate", input.Name),
		},
	})

	if err := tfa.Destroy(ctx, tfworkspace.DestroyInput{
		AwsCredentials: awsConfig.Credentials,
		Env: map[string]string{
			"AWS_REGION": input.Region,
		},
	}); err != nil {
		return err
	}
	return nil
}

func DestroySubnetsActivity(ctx context.Context, input DestroyDemoNetworkInput) error {
	awsConfig := awsconfig.LoadConfig()

	tfa := tfactivity.New(tfworkspace.Config{
		TerraformPath: "aws/subnet",
		TerraformFS:   terraform.FS,
		S3Backend: tfexec.S3BackendConfig{
			Credentials: awsConfig.Credentials,
			Region:      "us-west-2",
			Bucket:      "temporal-terraform-demo-state",
			Key:         fmt.Sprintf("subnets-%s.tfstate", input.Name),
		},
	})

	if err := tfa.Destroy(ctx, tfworkspace.DestroyInput{
		AwsCredentials: awsConfig.Credentials,
		Env: map[string]string{
			"AWS_REGION": input.Region,
		},
	}); err != nil {
		return err
	}
	return nil
}
