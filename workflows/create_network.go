package workflows

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/dynajoe/temporal-terraform-demo/config/awsconfig"
	"github.com/dynajoe/temporal-terraform-demo/terraform"
	"github.com/dynajoe/temporal-terraform-demo/tfactivity"
	"github.com/dynajoe/temporal-terraform-demo/tfexec"
	"github.com/dynajoe/temporal-terraform-demo/tfworkspace"
)

type (
	CreateDemoNetworkInput struct {
		Name      string
		Region    string
		CIDRBlock string
		Subnets   []Subnet
	}

	CreateDemoNetworkOutput struct {
		VpcID string
	}

	Subnet struct {
		AvailabilityZone string
		CIDRBlock        string
	}

	CreateVPCInput struct {
		Name      string
		Region    string
		CIDRBlock string
	}

	CreateVPCOutput struct {
		VpcID string
	}

	CreateSubnetsInput struct {
		Name    string
		Region  string
		VpcID   string
		Subnets []Subnet
	}

	CreateSubnetsOutput struct{}
)

func CreateDemoNetworkWorkflow(ctx workflow.Context, input CreateDemoNetworkInput) (CreateDemoNetworkOutput, error) {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: time.Hour,
		HeartbeatTimeout:    time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    5 * time.Second,
			BackoffCoefficient: 1.3,
			MaximumInterval:    10 * time.Second,
		},
	})

	// Create the VPC
	var vpcOutput CreateVPCOutput
	if err := workflow.ExecuteActivity(ctx, CreateVPCActivity, input).Get(ctx, &vpcOutput); err != nil {
		return CreateDemoNetworkOutput{}, err
	}

	// Create subnets
	var subnetOutput CreateSubnetsOutput
	if err := workflow.ExecuteActivity(ctx, CreateSubnetsActivity, CreateSubnetsInput{
		Name:    input.Name,
		VpcID:   vpcOutput.VpcID,
		Region:  input.Region,
		Subnets: input.Subnets,
	}).Get(ctx, &subnetOutput); err != nil {
		return CreateDemoNetworkOutput{}, err
	}

	return CreateDemoNetworkOutput{
		VpcID: vpcOutput.VpcID,
	}, nil
}

func CreateVPCActivity(ctx context.Context, input CreateVPCInput) (CreateVPCOutput, error) {
	awsConfig := awsconfig.LoadConfig()

	attemptImport := make(map[string]string)

	// Lookup vpc by name for import
	foundVpc, err := findVpcByName(ctx, awsConfig, input.Name)
	if err != nil {
		return CreateVPCOutput{}, err
	}
	if foundVpc.VpcId != nil {
		attemptImport["aws_vpc.vpc"] = *foundVpc.VpcId
	}

	// Temporal activity aware Terraform workspace wrapper
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

	// Apply Terraform
	applyOutput, err := tfa.Apply(ctx, tfworkspace.ApplyInput{
		AttemptImport:  attemptImport,
		AwsCredentials: awsConfig.Credentials,
		Env: map[string]string{
			"AWS_REGION": input.Region,
		},
		Vars: map[string]interface{}{
			"cidr_block": input.CIDRBlock,
			"name":       input.Name,
		},
	})
	if err != nil {
		return CreateVPCOutput{}, err
	}

	// Extract output from Terraform
	vpcID, err := applyOutput.String("vpc_id")
	if err != nil {
		return CreateVPCOutput{}, err
	}

	return CreateVPCOutput{
		VpcID: vpcID,
	}, nil
}

func CreateSubnetsActivity(ctx context.Context, input CreateSubnetsInput) (CreateSubnetsOutput, error) {
	awsConfig := awsconfig.LoadConfig()

	attemptImport := make(map[string]string)

	// Fetch existing subnets for import
	existingSubnets, err := listSubnets(ctx, awsConfig, input.VpcID)
	if err != nil {
		return CreateSubnetsOutput{}, err
	}
	for _, s := range existingSubnets {
		key := fmt.Sprintf(`aws_subnet.subnet["%s"]`, *s.AvailabilityZone)
		attemptImport[key] = *s.SubnetId
	}

	// Temporal activity aware Terraform workspace wrapper
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

	var subnets []map[string]string
	for _, s := range input.Subnets {
		subnets = append(subnets, map[string]string{
			"cidr_block":        s.CIDRBlock,
			"name":              fmt.Sprintf("%s-%s", input.Name, s.AvailabilityZone),
			"availability_zone": input.Region + s.AvailabilityZone,
		})
	}

	// Apply Terraform to create subnets
	if _, err := tfa.Apply(ctx, tfworkspace.ApplyInput{
		AwsCredentials: awsConfig.Credentials,
		AttemptImport:  attemptImport,
		Env: map[string]string{
			"AWS_REGION": input.Region,
		},
		Vars: map[string]interface{}{
			"vpc_id":  input.VpcID,
			"subnets": subnets,
		},
	}); err != nil {
		return CreateSubnetsOutput{}, err
	}

	return CreateSubnetsOutput{}, nil
}

func listSubnets(ctx context.Context, awsConfig aws.Config, vpcID string) ([]types.Subnet, error) {
	client := ec2.NewFromConfig(awsConfig)
	describeOutput, err := client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []string{vpcID},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return describeOutput.Subnets, nil
}

func findVpcByName(ctx context.Context, awsConfig aws.Config, name string) (types.Vpc, error) {
	client := ec2.NewFromConfig(awsConfig)
	vpcOutput, err := client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("tag:Name"),
				Values: []string{name},
			},
		},
	})
	if err != nil {
		return types.Vpc{}, err
	}
	if len(vpcOutput.Vpcs) == 0 {
		return types.Vpc{}, nil
	}
	if len(vpcOutput.Vpcs) > 1 {
		return types.Vpc{}, fmt.Errorf("multiple vpcs found with the name: %s", name)
	}
	return vpcOutput.Vpcs[0], nil
}
