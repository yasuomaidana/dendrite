package aws_test

import (
	"errors"
	"strings"
	"testing"

	"Dendrite/pkg/orchestrator/aws"
)

func TestValidateZeroIngressSecurityGroup(t *testing.T) {
	// 1. Valid zero-ingress security group (only outbound egress)
	validSG := aws.SecurityGroupConfig{
		GroupID: "sg-123456",
		Name:    "dendrite-burst-zero-ingress-sg",
		VPCID:   "vpc-abc123",
		Rules: []aws.SecurityGroupRule{
			{
				Type:        "egress",
				CIDR:        "0.0.0.0/0",
				FromPort:    0,
				ToPort:      0,
				Protocol:    "-1",
				Description: "Allow all outbound traffic for Tailscale",
			},
			{
				Type:        "ingress",
				CIDR:        "100.64.0.0/10", // Tailscale CGNAT range
				FromPort:    22,
				ToPort:      22,
				Protocol:    "tcp",
				Description: "Internal Tailscale SSH",
			},
		},
	}

	if err := aws.ValidateZeroIngressSecurityGroup(validSG); err != nil {
		t.Errorf("expected valid zero-ingress security group to pass, got: %v", err)
	}

	// 2. Violation: Public IPv4 0.0.0.0/0 ingress
	badIPv4SG := aws.SecurityGroupConfig{
		GroupID: "sg-bad-ipv4",
		Rules: []aws.SecurityGroupRule{
			{
				Type:     "ingress",
				CIDR:     "0.0.0.0/0",
				FromPort: 22,
				ToPort:   22,
				Protocol: "tcp",
			},
		},
	}

	err := aws.ValidateZeroIngressSecurityGroup(badIPv4SG)
	if !errors.Is(err, aws.ErrPublicIngressDetected) {
		t.Errorf("expected ErrPublicIngressDetected for 0.0.0.0/0 ingress, got: %v", err)
	}
	if !strings.Contains(err.Error(), "security validation failed") {
		t.Errorf("expected security validation error formatting, got: %v", err)
	}

	// 3. Violation: Public IPv6 ::/0 ingress
	badIPv6SG := aws.SecurityGroupConfig{
		GroupID: "sg-bad-ipv6",
		Rules: []aws.SecurityGroupRule{
			{
				Type:     "ingress",
				CIDR:     "::/0",
				FromPort: 443,
				ToPort:   443,
				Protocol: "tcp",
			},
		},
	}

	err6 := aws.ValidateZeroIngressSecurityGroup(badIPv6SG)
	if !errors.Is(err6, aws.ErrPublicIngressDetected) {
		t.Errorf("expected ErrPublicIngressDetected for ::/0 ingress, got: %v", err6)
	}
}

func TestVolumeLifecycler_Lifecycle(t *testing.T) {
	lifecycler := aws.NewVolumeLifecycler()

	// 1. Register valid gp3 volume
	vol := aws.EBSVolume{
		VolumeID: "vol-0123456789abcdef0",
		SizeGB:   100,
	}
	if err := lifecycler.RegisterVolume(vol); err != nil {
		t.Fatalf("failed to register valid volume: %v", err)
	}

	// 2. Reject non-gp3 volume
	badVol := aws.EBSVolume{
		VolumeID:   "vol-bad",
		VolumeType: "standard",
	}
	if err := lifecycler.RegisterVolume(badVol); !errors.Is(err, aws.ErrInvalidVolumeType) {
		t.Errorf("expected ErrInvalidVolumeType for non-gp3 volume, got: %v", err)
	}

	// 3. Attach volume
	err := lifecycler.AttachVolume("vol-0123456789abcdef0", "i-spot123", "/dev/xvdf")
	if err != nil {
		t.Fatalf("unexpected error attaching volume: %v", err)
	}

	attached, err := lifecycler.GetVolume("vol-0123456789abcdef0")
	if err != nil || attached.State != aws.StateAttached || attached.AttachedInstanceID != "i-spot123" {
		t.Errorf("expected attached state, got %v, err: %v", attached, err)
	}

	// 4. Attach already attached volume fails
	errDup := lifecycler.AttachVolume("vol-0123456789abcdef0", "i-spot456", "/dev/xvdf")
	if !errors.Is(errDup, aws.ErrVolumeAlreadyAttached) {
		t.Errorf("expected ErrVolumeAlreadyAttached, got: %v", errDup)
	}

	// 5. Attach nonexistent volume fails
	if err := lifecycler.AttachVolume("vol-nonexistent", "i-123", ""); !errors.Is(err, aws.ErrVolumeNotFound) {
		t.Errorf("expected ErrVolumeNotFound, got: %v", err)
	}

	// 6. Clean detachment
	if err := lifecycler.DetachVolume("vol-0123456789abcdef0"); err != nil {
		t.Fatalf("unexpected error detaching volume: %v", err)
	}

	detached, err := lifecycler.GetVolume("vol-0123456789abcdef0")
	if err != nil || detached.State != aws.StateAvailable || detached.AttachedInstanceID != "" {
		t.Errorf("expected available state after detachment, got: %v", detached)
	}

	// 7. Detaching unattached volume fails
	if err := lifecycler.DetachVolume("vol-0123456789abcdef0"); !errors.Is(err, aws.ErrVolumeNotAttached) {
		t.Errorf("expected ErrVolumeNotAttached, got: %v", err)
	}

	// 8. Detaching nonexistent volume fails
	if err := lifecycler.DetachVolume("vol-nonexistent"); !errors.Is(err, aws.ErrVolumeNotFound) {
		t.Errorf("expected ErrVolumeNotFound on detachment, got: %v", err)
	}

	// 9. Get nonexistent volume fails
	if _, err := lifecycler.GetVolume("vol-missing"); !errors.Is(err, aws.ErrVolumeNotFound) {
		t.Errorf("expected ErrVolumeNotFound for missing volume")
	}
}

func TestVolumeLifecycler_DefaultDeviceAndExplicitFields(t *testing.T) {
	lifecycler := aws.NewVolumeLifecycler()

	// Register volume with explicit VolumeType "gp3" and explicit State StateAvailable
	vol := aws.EBSVolume{
		VolumeID:   "vol-explicit",
		SizeGB:     50,
		VolumeType: "gp3",
		State:      aws.StateAvailable,
	}
	if err := lifecycler.RegisterVolume(vol); err != nil {
		t.Fatalf("failed to register volume: %v", err)
	}

	// Attach with empty deviceName: must default to "/dev/xvdf"
	if err := lifecycler.AttachVolume("vol-explicit", "i-spot789", ""); err != nil {
		t.Fatalf("unexpected error attaching volume: %v", err)
	}

	attached, err := lifecycler.GetVolume("vol-explicit")
	if err != nil {
		t.Fatalf("failed to get volume: %v", err)
	}
	if attached.DeviceName != "/dev/xvdf" {
		t.Errorf("expected default device /dev/xvdf, got %s", attached.DeviceName)
	}
}

func TestValidateZeroIngressSecurityGroup_EdgeCases(t *testing.T) {
	// Empty rules
	if err := aws.ValidateZeroIngressSecurityGroup(aws.SecurityGroupConfig{}); err != nil {
		t.Errorf("expected nil for empty rules, got: %v", err)
	}

	// SecurityValidationError Error and Unwrap methods
	rule := aws.SecurityGroupRule{
		Type:     "ingress",
		CIDR:     "0.0.0.0/0",
		FromPort: 80,
		ToPort:   80,
	}
	sve := &aws.SecurityValidationError{
		Rule: rule,
		Err:  aws.ErrPublicIngressDetected,
	}
	if !strings.Contains(sve.Error(), "security validation failed for rule") {
		t.Errorf("unexpected Error() string: %s", sve.Error())
	}
	if !errors.Is(sve, aws.ErrPublicIngressDetected) {
		t.Errorf("expected Unwrap to yield ErrPublicIngressDetected")
	}
}
