variable "azure_region" {
  description = "Azure region for the burst compute instance"
  type        = string
  default     = "eastus"
}

variable "resource_group_name" {
  description = "Name of the Azure Resource Group where resources are deployed"
  type        = string
}

variable "subnet_id" {
  description = "Resource ID of the Subnet where the burst VM network interface connects"
  type        = string
}

variable "vm_size" {
  description = "Azure Virtual Machine SKU size for burst compute"
  type        = string
  default     = "Standard_D8s_v5"
}

variable "priority" {
  description = "Compute priority: Spot for elastic cost-optimized burst, or Regular for on-demand"
  type        = string
  default     = "Spot"
}

variable "max_bid_price" {
  description = "Maximum hourly price bid for Spot instances in USD (-1 for unconstrained market cap)"
  type        = string
  default     = "-1"
}

variable "tailscale_auth_key" {
  description = "Tailscale pre-authorized ephemeral auth key for joining the secure mesh"
  type        = string
  sensitive   = true
}

variable "tailscale_hostname" {
  description = "Tailscale overlay hostname"
  type        = string
  default     = "azure-burst-node"
}

variable "persistent_managed_disk_id" {
  description = "Resource ID of an existing persistent Azure Managed Disk to attach (empty string creates a new disk)"
  type        = string
  default     = ""
}

variable "disk_storage_type" {
  description = "Storage account type for the persistent Managed Disk (StandardSSD_LRS or Premium_LRS)"
  type        = string
  default     = "StandardSSD_LRS"
}

variable "disk_size_gb" {
  description = "Size in GB for the persistent Managed Disk when dynamically created"
  type        = number
  default     = 100
}

variable "deployer_ssh_public_key" {
  description = "Pre-shared Ed25519 public SSH key for the deployer administrative user"
  type        = string
}

variable "tags" {
  description = "Resource tags applied to all Azure burst resources"
  type        = map(string)
  default = {
    Project   = "Dendrite"
    ManagedBy = "Terraform"
    Scenario  = "azure-burst"
  }
}
