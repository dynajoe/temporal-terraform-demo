package workflows

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"

	"github.com/dynajoe/temporal-terraform-demo/tfworkspace"
)

func TestTerraformPlanAndApplyWorkflow_Approve(t *testing.T) {
	s := testsuite.WorkflowTestSuite{}
	env := s.NewTestWorkflowEnvironment()

	bundlePath := "/tmp/test-bundle.zip"
	planFile := "/tmp/test.tf-plan"

	env.OnActivity(TerraformBundleEmbeddedTerraformActivity, mock.Anything, mock.Anything).
		Return(bundlePath, nil)
	env.OnActivity(TerraformPlanActivity, mock.Anything, PlanInput{
		BundlePath: bundlePath,
		Env:        map[string]string{"AWS_REGION": "us-west-2"},
	}).Return(tfworkspace.PlanOutput{
		PlanFile:   planFile,
		HasChanges: true,
	}, nil)
	env.OnActivity(TerraformApplyActivity, mock.Anything, ApplyInput{
		BundlePath: bundlePath,
		PlanFile:   planFile,
		Env:        map[string]string{"AWS_REGION": "us-west-2"},
	}).Return(tfworkspace.ApplyOutput{
		Output: map[string]any{"vpc_id": "vpc-123"},
	}, nil)

	// Send approve signal after plan
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow("terraform-apply-signal", "approve")
	}, 0)

	env.ExecuteWorkflow(TerraformPlanAndApplyWorkflow, TerraformInput{
		TerraformPath: "aws/vpc",
		Vars:          map[string]any{"name": "test"},
		Env:           map[string]string{"AWS_REGION": "us-west-2"},
		StateKey:      "vpc-test.tfstate",
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result tfworkspace.ApplyOutput
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "vpc-123", result.Output["vpc_id"])
}

func TestTerraformPlanAndApplyWorkflow_Reject(t *testing.T) {
	s := testsuite.WorkflowTestSuite{}
	env := s.NewTestWorkflowEnvironment()

	bundlePath := "/tmp/test-bundle.zip"
	planFile := "/tmp/test.tf-plan"

	env.OnActivity(TerraformBundleEmbeddedTerraformActivity, mock.Anything, mock.Anything).
		Return(bundlePath, nil)
	env.OnActivity(TerraformPlanActivity, mock.Anything, mock.Anything).
		Return(tfworkspace.PlanOutput{
			PlanFile:   planFile,
			HasChanges: true,
		}, nil)

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow("terraform-apply-signal", "reject")
	}, 0)

	env.ExecuteWorkflow(TerraformPlanAndApplyWorkflow, TerraformInput{
		TerraformPath: "aws/vpc",
		Vars:          map[string]any{"name": "test"},
		Env:           map[string]string{"AWS_REGION": "us-west-2"},
		StateKey:      "vpc-test.tfstate",
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result tfworkspace.ApplyOutput
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Nil(t, result.Output)
}

func TestTerraformPlanAndApplyWorkflow_PlanThenApprove(t *testing.T) {
	s := testsuite.WorkflowTestSuite{}
	env := s.NewTestWorkflowEnvironment()

	bundlePath := "/tmp/test-bundle.zip"
	planFile := "/tmp/test.tf-plan"

	env.OnActivity(TerraformBundleEmbeddedTerraformActivity, mock.Anything, mock.Anything).
		Return(bundlePath, nil)
	env.OnActivity(TerraformPlanActivity, mock.Anything, mock.Anything).
		Return(tfworkspace.PlanOutput{
			PlanFile:   planFile,
			HasChanges: true,
		}, nil)
	env.OnActivity(TerraformApplyActivity, mock.Anything, mock.Anything).
		Return(tfworkspace.ApplyOutput{
			Output: map[string]any{"vpc_id": "vpc-456"},
		}, nil)

	signalCount := 0
	env.RegisterDelayedCallback(func() {
		// First signal: re-plan
		env.SignalWorkflow("terraform-apply-signal", "plan")
		signalCount++
	}, 0)
	env.RegisterDelayedCallback(func() {
		// Second signal: approve
		env.SignalWorkflow("terraform-apply-signal", "approve")
		signalCount++
	}, 0)

	env.ExecuteWorkflow(TerraformPlanAndApplyWorkflow, TerraformInput{
		TerraformPath: "aws/vpc",
		Vars:          map[string]any{"name": "test"},
		Env:           map[string]string{"AWS_REGION": "us-west-2"},
		StateKey:      "vpc-test.tfstate",
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.Equal(t, 2, signalCount)
}

func TestTerraformPlanAndApplyWorkflow_NoChanges(t *testing.T) {
	s := testsuite.WorkflowTestSuite{}
	env := s.NewTestWorkflowEnvironment()

	bundlePath := "/tmp/test-bundle.zip"

	env.OnActivity(TerraformBundleEmbeddedTerraformActivity, mock.Anything, mock.Anything).
		Return(bundlePath, nil)
	env.OnActivity(TerraformPlanActivity, mock.Anything, mock.Anything).
		Return(tfworkspace.PlanOutput{
			HasChanges: false,
		}, nil)

	env.ExecuteWorkflow(TerraformPlanAndApplyWorkflow, TerraformInput{
		TerraformPath: "aws/vpc",
		Vars:          map[string]any{"name": "test"},
		Env:           map[string]string{"AWS_REGION": "us-west-2"},
		StateKey:      "vpc-test.tfstate",
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result tfworkspace.ApplyOutput
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Nil(t, result.Output)
}

func TestTerraformPlanAndDestroyWorkflow_Approve(t *testing.T) {
	s := testsuite.WorkflowTestSuite{}
	env := s.NewTestWorkflowEnvironment()

	bundlePath := "/tmp/test-destroy-bundle.zip"
	planFile := "/tmp/test.tf-destroy-plan"

	env.OnActivity(TerraformBundleEmbeddedTerraformDestroyActivity, mock.Anything, mock.Anything).
		Return(bundlePath, nil)
	env.OnActivity(TerraformDestroyPlanActivity, mock.Anything, DestroyPlanInput{
		BundlePath: bundlePath,
		Env:        map[string]string{"AWS_REGION": "us-west-2"},
	}).Return(tfworkspace.PlanOutput{
		PlanFile:   planFile,
		HasChanges: true,
	}, nil)
	env.OnActivity(TerraformApplyActivity, mock.Anything, ApplyInput{
		BundlePath: bundlePath,
		PlanFile:   planFile,
		Env:        map[string]string{"AWS_REGION": "us-west-2"},
	}).Return(tfworkspace.ApplyOutput{}, nil)

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow("terraform-destroy-signal", "approve")
	}, 0)

	env.ExecuteWorkflow(TerraformPlanAndDestroyWorkflow, DestroyTerraformInput{
		TerraformPath: "aws/vpc",
		Env:           map[string]string{"AWS_REGION": "us-west-2"},
		StateKey:      "vpc-test.tfstate",
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
}

func TestTerraformPlanAndDestroyWorkflow_Reject(t *testing.T) {
	s := testsuite.WorkflowTestSuite{}
	env := s.NewTestWorkflowEnvironment()

	bundlePath := "/tmp/test-destroy-bundle.zip"

	env.OnActivity(TerraformBundleEmbeddedTerraformDestroyActivity, mock.Anything, mock.Anything).
		Return(bundlePath, nil)
	env.OnActivity(TerraformDestroyPlanActivity, mock.Anything, mock.Anything).
		Return(tfworkspace.PlanOutput{
			PlanFile:   "/tmp/test.tf-destroy-plan",
			HasChanges: true,
		}, nil)

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow("terraform-destroy-signal", "reject")
	}, 0)

	env.ExecuteWorkflow(TerraformPlanAndDestroyWorkflow, DestroyTerraformInput{
		TerraformPath: "aws/vpc",
		Env:           map[string]string{"AWS_REGION": "us-west-2"},
		StateKey:      "vpc-test.tfstate",
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
}

func TestTerraformPlanAndDestroyWorkflow_NoChanges(t *testing.T) {
	s := testsuite.WorkflowTestSuite{}
	env := s.NewTestWorkflowEnvironment()

	bundlePath := "/tmp/test-destroy-bundle.zip"

	env.OnActivity(TerraformBundleEmbeddedTerraformDestroyActivity, mock.Anything, mock.Anything).
		Return(bundlePath, nil)
	env.OnActivity(TerraformDestroyPlanActivity, mock.Anything, mock.Anything).
		Return(tfworkspace.PlanOutput{
			HasChanges: false,
		}, nil)

	env.ExecuteWorkflow(TerraformPlanAndDestroyWorkflow, DestroyTerraformInput{
		TerraformPath: "aws/vpc",
		Env:           map[string]string{"AWS_REGION": "us-west-2"},
		StateKey:      "vpc-test.tfstate",
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
}
