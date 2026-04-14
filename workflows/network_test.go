package workflows

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"

	"github.com/dynajoe/temporal-terraform-demo/tfworkspace"
)

func TestCreateDemoNetworkWorkflow(t *testing.T) {
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
	// Both VPC and subnet workflows share the same mocked apply activity.
	// The VPC workflow extracts vpc_id from the output; the subnet workflow ignores it.
	env.OnActivity(TerraformApplyActivity, mock.Anything, mock.Anything).
		Return(tfworkspace.ApplyOutput{
			Output: map[string]any{"vpc_id": "vpc-abc123"},
		}, nil)

	// VPC approve, then subnets approve
	signalCount := 0
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow("terraform-apply-signal", "approve")
		signalCount++
	}, 0)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow("terraform-apply-signal", "approve")
		signalCount++
	}, 0)

	env.ExecuteWorkflow(CreateDemoNetworkWorkflow, CreateDemoNetworkInput{
		Name:      "test-network",
		Region:    "us-west-2",
		CIDRBlock: "10.0.0.0/16",
		Subnets: []Subnet{
			{AvailabilityZone: "a", CIDRBlock: "10.0.1.0/24"},
			{AvailabilityZone: "b", CIDRBlock: "10.0.2.0/24"},
		},
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result CreateDemoNetworkOutput
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "vpc-abc123", result.VpcID)
}

func TestDestroyDemoNetworkWorkflow(t *testing.T) {
	s := testsuite.WorkflowTestSuite{}
	env := s.NewTestWorkflowEnvironment()

	bundlePath := "/tmp/test-destroy-bundle.zip"
	planFile := "/tmp/test.tf-destroy-plan"

	env.OnActivity(TerraformBundleEmbeddedTerraformDestroyActivity, mock.Anything, mock.Anything).
		Return(bundlePath, nil)
	env.OnActivity(TerraformDestroyPlanActivity, mock.Anything, mock.Anything).
		Return(tfworkspace.PlanOutput{
			PlanFile:   planFile,
			HasChanges: true,
		}, nil)
	env.OnActivity(TerraformApplyActivity, mock.Anything, mock.Anything).
		Return(tfworkspace.ApplyOutput{}, nil)

	// Approve subnets destroy, then VPC destroy
	signalCount := 0
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow("terraform-destroy-signal", "approve")
		signalCount++
	}, 0)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow("terraform-destroy-signal", "approve")
		signalCount++
	}, 0)

	env.ExecuteWorkflow(DestroyDemoNetworkWorkflow, DestroyDemoNetworkInput{
		Name:   "test-network",
		Region: "us-west-2",
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
}

func TestCreateVPCWorkflow(t *testing.T) {
	s := testsuite.WorkflowTestSuite{}
	env := s.NewTestWorkflowEnvironment()

	bundlePath := "/tmp/test-bundle.zip"
	planFile := "/tmp/test.tf-plan"

	env.OnActivity(TerraformBundleEmbeddedTerraformActivity, mock.Anything, BundleEmbeddedTerraformInput{
		TerraformPath: "aws/vpc",
		Vars:          map[string]any{"cidr_block": "10.0.0.0/16", "name": "my-vpc"},
		StateKey:      "vpc-my-vpc.tfstate",
	}).Return(bundlePath, nil)
	env.OnActivity(TerraformPlanActivity, mock.Anything, mock.Anything).
		Return(tfworkspace.PlanOutput{
			PlanFile:   planFile,
			HasChanges: true,
		}, nil)
	env.OnActivity(TerraformApplyActivity, mock.Anything, mock.Anything).
		Return(tfworkspace.ApplyOutput{
			Output: map[string]any{"vpc_id": "vpc-xyz"},
		}, nil)

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow("terraform-apply-signal", "approve")
	}, 0)

	env.ExecuteWorkflow(CreateVPCWorkflow, CreateVPCInput{
		Name:      "my-vpc",
		Region:    "us-west-2",
		CIDRBlock: "10.0.0.0/16",
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result CreateVPCOutput
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, "vpc-xyz", result.VpcID)
}

func TestDestroyVPCWorkflow(t *testing.T) {
	s := testsuite.WorkflowTestSuite{}
	env := s.NewTestWorkflowEnvironment()

	bundlePath := "/tmp/test-destroy-bundle.zip"
	planFile := "/tmp/test.tf-destroy-plan"

	env.OnActivity(TerraformBundleEmbeddedTerraformDestroyActivity, mock.Anything, BundleEmbeddedTerraformDestroyInput{
		TerraformPath: "aws/vpc",
		StateKey:      "vpc-my-vpc.tfstate",
	}).Return(bundlePath, nil)
	env.OnActivity(TerraformDestroyPlanActivity, mock.Anything, mock.Anything).
		Return(tfworkspace.PlanOutput{
			PlanFile:   planFile,
			HasChanges: true,
		}, nil)
	env.OnActivity(TerraformApplyActivity, mock.Anything, mock.Anything).
		Return(tfworkspace.ApplyOutput{}, nil)

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow("terraform-destroy-signal", "approve")
	}, 0)

	env.ExecuteWorkflow(DestroyVPCWorkflow, DestroyVPCInput{
		Name:   "my-vpc",
		Region: "us-west-2",
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
}
