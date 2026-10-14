package aws

import (
	"errors"
	"fmt"
)

// Standard error definitions for AWS orchestrator operations.
var (
	ErrPublicIngressDetected = errors.New("security group violation: public ingress from 0.0.0.0/0 or ::/0 is strictly prohibited")
	ErrVolumeNotFound        = errors.New("EBS volume not found")
	ErrVolumeAlreadyAttached = errors.New("EBS volume is already attached to an active instance")
	ErrVolumeNotAttached     = errors.New("EBS volume is not attached")
	ErrInvalidVolumeType     = errors.New("unsupported volume type (must be gp3)")
)

// SecurityGroupRule describes an ingress or egress networking rule.
type SecurityGroupRule struct {
	Type        string `json:"type"` // "ingress" or "egress"
	CIDR        string `json:"cidr"`
	FromPort    int    `json:"from_port"`
	ToPort      int    `json:"to_port"`
	Protocol    string `json:"protocol"`
	Description string `json:"description,omitempty"`
}

// SecurityGroupConfig represents an AWS security group configuration for zero-ingress validation.
type SecurityGroupConfig struct {
	GroupID     string              `json:"group_id"`
	Name        string              `json:"name"`
	VPCID       string              `json:"vpc_id"`
	Rules       []SecurityGroupRule `json:"rules"`
	Description string              `json:"description,omitempty"`
}

// VolumeState denotes the operational attachment status of an EBS volume.
type VolumeState string

const (
	StateAvailable VolumeState = "available"
	StateAttaching VolumeState = "attaching"
	StateAttached  VolumeState = "attached"
	StateDetaching VolumeState = "detaching"
)

// EBSVolume represents an elastic block storage persistent disk for the burst node.
type EBSVolume struct {
	VolumeID           string      `json:"volume_id"`
	SizeGB             int         `json:"size_gb"`
	VolumeType         string      `json:"volume_type"` // "gp3"
	State              VolumeState `json:"state"`
	AttachedInstanceID string      `json:"attached_instance_id,omitempty"`
	DeviceName         string      `json:"device_name,omitempty"`
}

// SecurityValidationError encapsulates rule violation details.
type SecurityValidationError struct {
	Rule SecurityGroupRule
	Err  error
}

func (e *SecurityValidationError) Error() string {
	return fmt.Sprintf("security validation failed for rule %s %s:%d-%d: %v",
		e.Rule.Type, e.Rule.CIDR, e.Rule.FromPort, e.Rule.ToPort, e.Err)
}

func (e *SecurityValidationError) Unwrap() error {
	return e.Err
}
