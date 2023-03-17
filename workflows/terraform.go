package workflows

import (
	"context"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/dynajoe/temporal-terraform-demo/config/awsconfig"
	"github.com/dynajoe/temporal-terraform-demo/heartbeat"
	"github.com/dynajoe/temporal-terraform-demo/terraform"
	"github.com/dynajoe/temporal-terraform-demo/tfworkspace"
)

type (
	TerraformInput struct {
		TerraformPath string
		Vars          map[string]any
		Env           map[string]string
		StateKey      string
	}

	InitInput struct {
		BundlePath string
	}

	PlanInput struct {
		BundlePath string
		Env        map[string]string
	}

	ApplyInput struct {
		BundlePath string
		PlanFile   string
		Env        map[string]string
	}

	BundleEmbeddedTerraformInput struct {
		TerraformPath string
		Vars          map[string]any
		StateKey      string
	}

	ApplyDecision string
)

func TerraformPlanAndApplyWorkflow(ctx workflow.Context, input TerraformInput) (tfworkspace.ApplyOutput, error) {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: time.Hour,
		HeartbeatTimeout:    time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    5 * time.Second,
			BackoffCoefficient: 1.3,
			MaximumInterval:    10 * time.Second,
		},
	})

	// Bundle a snapshot of terraform configuration and vars
	var bundlePath string
	if err := workflow.ExecuteActivity(ctx, TerraformBundleEmbeddedTerraformActivity, BundleEmbeddedTerraformInput{
		TerraformPath: input.TerraformPath,
		Vars:          input.Vars,
		StateKey:      input.StateKey,
	}).Get(ctx, &bundlePath); err != nil {
		return tfworkspace.ApplyOutput{}, err
	}

	for {
		// terraform plan
		planOutput := tfworkspace.PlanOutput{}
		if err := workflow.ExecuteActivity(ctx, TerraformPlanActivity, PlanInput{
			BundlePath: bundlePath,
			Env:        input.Env,
		}).Get(ctx, &planOutput); err != nil {
			return tfworkspace.ApplyOutput{}, err
		}

		// nothing to do!
		if !planOutput.HasChanges {
			return tfworkspace.ApplyOutput{}, nil
		}

		// prompt for user to approve, plan, or reject
		var decision string
		workflow.GetSignalChannel(ctx, "terraform-apply-signal").Receive(ctx, &decision)

		// plan again
		if decision == "plan" {
			continue
		}

		if decision == "reject" {
			return tfworkspace.ApplyOutput{}, nil
		}

		if decision == "approve" {
			// terraform apply
			applyOutput := tfworkspace.ApplyOutput{}
			if err := workflow.ExecuteActivity(ctx, TerraformApplyActivity, ApplyInput{
				PlanFile:   planOutput.PlanFile,
				BundlePath: bundlePath,
				Env:        input.Env,
			}).Get(ctx, &applyOutput); err != nil {
				return tfworkspace.ApplyOutput{}, err
			}
			return applyOutput, nil
		}
	}
}

func TerraformBundleEmbeddedTerraformActivity(ctx context.Context, input BundleEmbeddedTerraformInput) (string, error) {
	activityInfo := activity.GetInfo(ctx)
	return tfworkspace.NewBundleBuilder().
		Source(terraform.FS, input.TerraformPath).
		WithVars(input.Vars).
		WithS3Backend(tfworkspace.S3BackendConfig{
			Bucket:        "temporal-joe-terraform-demo-state",
			Key:           input.StateKey,
			Region:        "us-west-2",
			AssumeRoleArn: "",
			Profile:       "",
		}).
		WithMetadata(map[string]string{
			"workflowType": activityInfo.WorkflowType.Name,
			"workflowID":   activityInfo.WorkflowExecution.ID,
			"runID":        activityInfo.WorkflowExecution.RunID,
			"activityID":   activityInfo.ActivityID,
			"activityType": activityInfo.ActivityType.Name,
		}).
		BundleForApply()
}

func TerraformPlanActivity(ctx context.Context, input PlanInput) (tfworkspace.PlanOutput, error) {
	ctx, cancel := heartbeat.Begin(ctx, 10*time.Second)
	defer cancel()

	env, err := terraformEnv(ctx, input.Env)
	if err != nil {
		return tfworkspace.PlanOutput{}, err
	}

	return tfworkspace.NewFromBundle(input.BundlePath).Plan(ctx, env)
}

func TerraformApplyActivity(ctx context.Context, input ApplyInput) (tfworkspace.ApplyOutput, error) {
	ctx, cancel := heartbeat.Begin(ctx, 10*time.Second)
	defer cancel()

	env, err := terraformEnv(ctx, input.Env)
	if err != nil {
		return tfworkspace.ApplyOutput{}, err
	}

	return tfworkspace.NewFromBundle(input.BundlePath).Apply(ctx, env, input.PlanFile)
}

func terraformEnv(ctx context.Context, mergeEnv map[string]string) (map[string]string, error) {
	awsConfig := awsconfig.LoadConfig()
	creds, err := awsConfig.Credentials.Retrieve(ctx)
	if err != nil {
		return nil, err
	}
	secretEnv := map[string]string{
		"AWS_ACCESS_KEY_ID":     creds.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY": creds.SecretAccessKey,
		"AWS_SESSION_TOKEN":     creds.SessionToken,
	}
	for k, v := range mergeEnv {
		secretEnv[k] = v
	}
	return secretEnv, nil
}
