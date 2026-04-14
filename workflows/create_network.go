package workflows

import (
	"fmt"

	"go.temporal.io/sdk/workflow"
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
	// Create the VPC
	vpcOutput, err := CreateVPCWorkflow(ctx, CreateVPCInput{
		Name:      input.Name,
		Region:    input.Region,
		CIDRBlock: input.CIDRBlock,
	})
	if err != nil {
		return CreateDemoNetworkOutput{}, err
	}

	// Create subnets
	if _, err := CreateSubnetsWorkflow(ctx, CreateSubnetsInput{
		Name:    input.Name,
		VpcID:   vpcOutput.VpcID,
		Region:  input.Region,
		Subnets: input.Subnets,
	}); err != nil {
		return CreateDemoNetworkOutput{}, err
	}

	return CreateDemoNetworkOutput{
		VpcID: vpcOutput.VpcID,
	}, nil
}

func CreateVPCWorkflow(ctx workflow.Context, input CreateVPCInput) (CreateVPCOutput, error) {
	applyOutput, err := TerraformPlanAndApplyWorkflow(ctx, TerraformInput{
		TerraformPath: "aws/vpc",
		Vars: map[string]interface{}{
			"cidr_block": input.CIDRBlock,
			"name":       input.Name,
		},
		Env: map[string]string{
			"AWS_REGION": input.Region,
		},
		StateKey: fmt.Sprintf("vpc-%s.tfstate", input.Name),
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

func CreateSubnetsWorkflow(ctx workflow.Context, input CreateSubnetsInput) (CreateSubnetsOutput, error) {
	var subnets []map[string]string
	for _, s := range input.Subnets {
		subnets = append(subnets, map[string]string{
			"cidr_block":        s.CIDRBlock,
			"name":              fmt.Sprintf("%s-%s", input.Name, s.AvailabilityZone),
			"availability_zone": input.Region + s.AvailabilityZone,
		})
	}

	_, err := TerraformPlanAndApplyWorkflow(ctx, TerraformInput{
		TerraformPath: "aws/subnet",
		Vars: map[string]interface{}{
			"vpc_id":  input.VpcID,
			"subnets": subnets,
		},
		Env: map[string]string{
			"AWS_REGION": input.Region,
		},
		StateKey: fmt.Sprintf("subnets-%s.tfstate", input.Name),
	})
	if err != nil {
		return CreateSubnetsOutput{}, err
	}
	return CreateSubnetsOutput{}, nil
}
