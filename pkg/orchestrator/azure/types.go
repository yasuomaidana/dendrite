package azure

import (
	"errors"
	"fmt"
)

// Standard error definitions for Azure orchestrator operations.
var (
	ErrPublicIngressDetected = errors.New("network security group violation: public inbound ingress from 0.0.0.0/0, ::/0, or Internet is strictly prohibited")
	ErrDiskNotFound          = errors.New("managed disk not found")
	ErrDiskAlreadyAttached   = errors.New("managed disk is already attached to an active virtual machine")
	ErrDiskNotAttached       = errors.New("managed disk is not currently attached")
	ErrInvalidDiskType       = errors.New("unsupported storage account type (must be StandardSSD_LRS or Premium_LRS)")
)

// SecurityRule describes an Azure Network Security Group rule.
type SecurityRule struct {
	Name                     string `json:"name"`
	Priority                 int    `json:"priority"`
	Direction                string `json:"direction"` // "Inbound" or "Outbound"
	Access                   string `json:"access"`    // "Allow" or "Deny"
	Protocol                 string `json:"protocol"`  // "Tcp", "Udp", "*"
	SourcePortRange          string `json:"source_port_range"`
	DestinationPortRange     string `json:"destination_port_range"`
	SourceAddressPrefix      string `json:"source_address_prefix"`
	DestinationAddressPrefix string `json:"destination_address_prefix"`
	Description              string `json:"description,omitempty"`
}

// NetworkSecurityGroupConfig represents an Azure Network Security Group configuration for zero-ingress validation.
type NetworkSecurityGroupConfig struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	ResourceGroup string         `json:"resource_group"`
	Location      string         `json:"location"`
	Rules         []SecurityRule `json:"rules"`
}

// DiskState denotes the operational attachment status of an Azure Managed Disk.
type DiskState string

const (
	StateAvailable DiskState = "available"
	StateAttaching DiskState = "attaching"
	StateAttached  DiskState = "attached"
	StateDetaching DiskState = "detaching"
)

// StorageAccountType defines supported persistent storage performance tiers.
type StorageAccountType string

const (
	StandardSSDLRS StorageAccountType = "StandardSSD_LRS"
	PremiumLRS     StorageAccountType = "Premium_LRS"
)

// ManagedDisk represents an Azure persistent Managed Disk for the burst compute node.
type ManagedDisk struct {
	DiskID             string             `json:"disk_id"`
	Name               string             `json:"name"`
	ResourceGroup      string             `json:"resource_group"`
	Location           string             `json:"location"`
	SizeGB             int                `json:"size_gb"`
	StorageAccountType StorageAccountType `json:"storage_account_type"`
	State              DiskState          `json:"state"`
	AttachedVMID       string             `json:"attached_vm_id,omitempty"`
	LUN                int                `json:"lun"`
	Caching            string             `json:"caching"`
}

// SecurityValidationError encapsulates rule violation details.
type SecurityValidationError struct {
	Rule SecurityRule
	Err  error
}

func (e *SecurityValidationError) Error() string {
	return fmt.Sprintf("azure security validation failed for rule '%s' [%s %s:%s]: %v",
		e.Rule.Name, e.Rule.Direction, e.Rule.SourceAddressPrefix, e.Rule.DestinationPortRange, e.Err)
}

func (e *SecurityValidationError) Unwrap() error {
	return e.Err
}
