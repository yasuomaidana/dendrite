package azure_test

import (
	"errors"
	"strings"
	"testing"

	"Dendrite/pkg/orchestrator/azure"
)

func TestValidateZeroIngressSecurityGroup(t *testing.T) {
	// 1. Valid zero-ingress NSG (outbound allow, inbound restricted to Tailscale private CGNAT)
	validNSG := azure.NetworkSecurityGroupConfig{
		ID:            "/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Network/networkSecurityGroups/nsg-valid",
		Name:          "dendrite-burst-zero-ingress-nsg",
		ResourceGroup: "rg1",
		Location:      "eastus",
		Rules: []azure.SecurityRule{
			{
				Name:                     "AllowAllEgress",
				Priority:                 100,
				Direction:                "Outbound",
				Access:                   "Allow",
				Protocol:                 "*",
				SourcePortRange:          "*",
				DestinationPortRange:     "*",
				SourceAddressPrefix:      "*",
				DestinationAddressPrefix: "*",
			},
			{
				Name:                     "AllowTailscaleInbound",
				Priority:                 200,
				Direction:                "Inbound",
				Access:                   "Allow",
				Protocol:                 "Tcp",
				SourcePortRange:          "*",
				DestinationPortRange:     "22",
				SourceAddressPrefix:      "100.64.0.0/10",
				DestinationAddressPrefix: "*",
			},
		},
	}

	if err := azure.ValidateZeroIngressSecurityGroup(validNSG); err != nil {
		t.Errorf("expected valid zero-ingress NSG to pass, got: %v", err)
	}

	// 2. Violation: Public IPv4 0.0.0.0/0 ingress
	badIPv4NSG := azure.NetworkSecurityGroupConfig{
		ID:   "nsg-bad-ipv4",
		Name: "nsg-bad-ipv4",
		Rules: []azure.SecurityRule{
			{
				Name:                     "BadPublicSSH",
				Priority:                 110,
				Direction:                "Inbound",
				Access:                   "Allow",
				Protocol:                 "Tcp",
				DestinationPortRange:     "22",
				SourceAddressPrefix:      "0.0.0.0/0",
				DestinationAddressPrefix: "*",
			},
		},
	}

	err := azure.ValidateZeroIngressSecurityGroup(badIPv4NSG)
	if !errors.Is(err, azure.ErrPublicIngressDetected) {
		t.Errorf("expected ErrPublicIngressDetected for 0.0.0.0/0 ingress, got: %v", err)
	}
	if !strings.Contains(err.Error(), "azure security validation failed") {
		t.Errorf("expected security validation error formatting, got: %v", err)
	}

	// 3. Violation: Public IPv6 ::/0 ingress
	badIPv6NSG := azure.NetworkSecurityGroupConfig{
		ID:   "nsg-bad-ipv6",
		Name: "nsg-bad-ipv6",
		Rules: []azure.SecurityRule{
			{
				Name:                     "BadPublicIPv6",
				Priority:                 120,
				Direction:                "Inbound",
				Access:                   "Allow",
				Protocol:                 "Tcp",
				DestinationPortRange:     "443",
				SourceAddressPrefix:      "::/0",
				DestinationAddressPrefix: "*",
			},
		},
	}

	err6 := azure.ValidateZeroIngressSecurityGroup(badIPv6NSG)
	if !errors.Is(err6, azure.ErrPublicIngressDetected) {
		t.Errorf("expected ErrPublicIngressDetected for ::/0 ingress, got: %v", err6)
	}

	// 4. Violation: Inbound wildcard (*) or "Internet"
	badInternetNSG := azure.NetworkSecurityGroupConfig{
		ID:   "nsg-bad-internet",
		Name: "nsg-bad-internet",
		Rules: []azure.SecurityRule{
			{
				Name:                     "BadInternetServiceTag",
				Priority:                 130,
				Direction:                "Inbound",
				Access:                   "Allow",
				Protocol:                 "*",
				DestinationPortRange:     "*",
				SourceAddressPrefix:      "Internet",
				DestinationAddressPrefix: "*",
			},
		},
	}

	errInternet := azure.ValidateZeroIngressSecurityGroup(badInternetNSG)
	if !errors.Is(errInternet, azure.ErrPublicIngressDetected) {
		t.Errorf("expected ErrPublicIngressDetected for Internet service tag, got: %v", errInternet)
	}

	// 5. Inbound Deny rule for 0.0.0.0/0 is allowed (it denies public access)
	denyNSG := azure.NetworkSecurityGroupConfig{
		ID:   "nsg-deny-public",
		Name: "nsg-deny-public",
		Rules: []azure.SecurityRule{
			{
				Name:                     "DenyPublicIngress",
				Priority:                 4000,
				Direction:                "Inbound",
				Access:                   "Deny",
				Protocol:                 "*",
				SourceAddressPrefix:      "0.0.0.0/0",
				DestinationAddressPrefix: "*",
			},
		},
	}
	if err := azure.ValidateZeroIngressSecurityGroup(denyNSG); err != nil {
		t.Errorf("expected Deny public rule to pass validation, got: %v", err)
	}
}

func TestDiskLifecycler_Lifecycle(t *testing.T) {
	lifecycler := azure.NewDiskLifecycler()

	// 1. Register valid StandardSSD_LRS Managed Disk
	disk1 := azure.ManagedDisk{
		DiskID:             "/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Compute/disks/disk-01",
		Name:               "disk-01",
		ResourceGroup:      "rg1",
		Location:           "eastus",
		SizeGB:             100,
		StorageAccountType: azure.StandardSSDLRS,
	}
	if err := lifecycler.RegisterDisk(disk1); err != nil {
		t.Fatalf("failed to register valid disk: %v", err)
	}

	// 2. Register valid Premium_LRS Managed Disk
	disk2 := azure.ManagedDisk{
		DiskID:             "/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Compute/disks/disk-02",
		Name:               "disk-02",
		ResourceGroup:      "rg1",
		Location:           "eastus",
		SizeGB:             256,
		StorageAccountType: azure.PremiumLRS,
	}
	if err := lifecycler.RegisterDisk(disk2); err != nil {
		t.Fatalf("failed to register valid premium disk: %v", err)
	}

	// 3. Reject unsupported storage type
	badDisk := azure.ManagedDisk{
		DiskID:             "disk-bad",
		StorageAccountType: "Standard_LRS",
	}
	if err := lifecycler.RegisterDisk(badDisk); !errors.Is(err, azure.ErrInvalidDiskType) {
		t.Errorf("expected ErrInvalidDiskType, got: %v", err)
	}

	// 4. Attach disk
	err := lifecycler.AttachDisk(disk1.DiskID, "vm-burst-spot-01", 0, "ReadWrite")
	if err != nil {
		t.Fatalf("unexpected error attaching disk: %v", err)
	}

	attached, err := lifecycler.GetDisk(disk1.DiskID)
	if err != nil || attached.State != azure.StateAttached || attached.AttachedVMID != "vm-burst-spot-01" {
		t.Errorf("expected attached state, got %v, err: %v", attached, err)
	}
	if attached.LUN != 0 || attached.Caching != "ReadWrite" {
		t.Errorf("expected LUN 0 and ReadWrite caching, got LUN %d, caching %s", attached.LUN, attached.Caching)
	}

	// 5. Attach already attached disk fails
	errDup := lifecycler.AttachDisk(disk1.DiskID, "vm-burst-spot-02", 1, "ReadOnly")
	if !errors.Is(errDup, azure.ErrDiskAlreadyAttached) {
		t.Errorf("expected ErrDiskAlreadyAttached, got: %v", errDup)
	}

	// 6. Safe detach disk
	err = lifecycler.DetachDisk(disk1.DiskID)
	if err != nil {
		t.Fatalf("unexpected error detaching disk: %v", err)
	}

	detached, err := lifecycler.GetDisk(disk1.DiskID)
	if err != nil || detached.State != azure.StateAvailable || detached.AttachedVMID != "" {
		t.Errorf("expected available state after detachment, got %v, err: %v", detached, err)
	}

	// 7. Detaching already available disk fails
	errDetachedAgain := lifecycler.DetachDisk(disk1.DiskID)
	if !errors.Is(errDetachedAgain, azure.ErrDiskNotAttached) {
		t.Errorf("expected ErrDiskNotAttached, got: %v", errDetachedAgain)
	}

	// 8. Operations on nonexistent disk
	if _, err := lifecycler.GetDisk("nonexistent"); !errors.Is(err, azure.ErrDiskNotFound) {
		t.Errorf("expected ErrDiskNotFound, got: %v", err)
	}
	if err := lifecycler.AttachDisk("nonexistent", "vm1", 0, ""); !errors.Is(err, azure.ErrDiskNotFound) {
		t.Errorf("expected ErrDiskNotFound, got: %v", err)
	}
	if err := lifecycler.DetachDisk("nonexistent"); !errors.Is(err, azure.ErrDiskNotFound) {
		t.Errorf("expected ErrDiskNotFound, got: %v", err)
	}

	// 9. ListDisks
	list := lifecycler.ListDisks()
	if len(list) != 2 {
		t.Errorf("expected 2 disks in list, got %d", len(list))
	}
}
