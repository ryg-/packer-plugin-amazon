// Copyright IBM Corp. 2013, 2025
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/packer-plugin-amazon/common/clients"
	"github.com/hashicorp/packer-plugin-sdk/communicator"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	confighelper "github.com/hashicorp/packer-plugin-sdk/template/config"
)

type runSourceEC2ConnMock struct {
	clients.Ec2Client

	RunInstancesParams []*ec2.RunInstancesInput
	RunInstancesFn     func(*ec2.RunInstancesInput) (*ec2.RunInstancesOutput, error)
}

func (m *runSourceEC2ConnMock) RunInstances(ctx context.Context, params *ec2.RunInstancesInput,
	optFns ...func(*ec2.Options)) (*ec2.RunInstancesOutput, error) {
	m.RunInstancesParams = append(m.RunInstancesParams, params)
	return m.RunInstancesFn(params)
}

func TestStepRunSourceInstance_Run_EnableNestedVirtualization(t *testing.T) {
	cases := []struct {
		name                string
		enableNestedVirt    bool
		expectCPUOptionsSet bool
	}{
		{
			name:                "enabled",
			enableNestedVirt:    true,
			expectCPUOptionsSet: true,
		},
		{
			name:                "disabled",
			enableNestedVirt:    false,
			expectCPUOptionsSet: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ec2Mock := &runSourceEC2ConnMock{
				RunInstancesFn: func(*ec2.RunInstancesInput) (*ec2.RunInstancesOutput, error) {
					// Stop the step after RunInstances so the test stays focused on request construction.
					return nil, errors.New("run instances failed")
				},
			}

			state := new(multistep.BasicStateBag)
			state.Put("ec2v2", ec2Mock)
			state.Put("aws_config", &aws.Config{Region: "us-east-1"})
			state.Put("securityGroupIds", []string{"sg-0123456789abcdef0"})
			state.Put("iamInstanceProfile", "packer-123")
			state.Put("ui", packersdk.TestUi(t))
			state.Put("source_image", testImage())
			state.Put("availability_zone", "us-east-1a")
			state.Put("subnet_id", "")

			step := &StepRunSourceInstance{
				AssociatePublicIpAddress:   confighelper.TriUnset,
				LaunchMappings:             BlockDevices{},
				Comm:                       &communicator.Config{},
				InstanceType:               "c8i.large",
				ExpectedRootDevice:         "ebs",
				EnableNestedVirtualization: tc.enableNestedVirt,
				Tags:                       map[string]string{},
				VolumeTags:                 map[string]string{},
			}

			action := step.Run(context.Background(), state)

			if action != multistep.ActionHalt {
				t.Fatalf("expected ActionHalt because mock RunInstances returns error, got %v", action)
			}

			if state.Get("error") == nil {
				t.Fatalf("expected error to be set in state bag")
			}

			if len(ec2Mock.RunInstancesParams) != 1 {
				t.Fatalf("RunInstances should be called once, got %d", len(ec2Mock.RunInstancesParams))
			}

			runInput := ec2Mock.RunInstancesParams[0]
			if tc.expectCPUOptionsSet {
				if runInput.CpuOptions == nil {
					t.Fatalf("expected CpuOptions to be set when nested virtualization is enabled")
				}
				if runInput.CpuOptions.NestedVirtualization != ec2types.NestedVirtualizationSpecificationEnabled {
					t.Fatalf("expected nested virtualization to be enabled, got %#v",
						runInput.CpuOptions.NestedVirtualization)
				}
				return
			}

			if runInput.CpuOptions != nil {
				t.Fatalf("expected CpuOptions to be nil when nested virtualization is disabled")
			}
		})
	}
}
