variable "aws_region" {
  description = "AWS region for the burst compute instance"
  type        = string
  default     = "us-east-1"
}

variable "vpc_id" {
  description = "VPC ID where the spot instance is deployed"
  type        = string
}

variable "subnet_id" {
  description = "Subnet ID where the spot instance is deployed"
  type        = string
}

variable "ami_id" {
  description = "AMI ID for the Debian or Ubuntu Linux compute image"
  type        = string
}

variable "instance_type" {
  description = "EC2 instance type for burst compute"
  type        = string
  default     = "c6i.2xlarge"
}

variable "spot_price" {
  description = "Maximum hourly spot price bid in USD (empty string for on-demand market price)"
  type        = string
  default     = ""
}

variable "tailscale_auth_key" {
  description = "Tailscale pre-authorized ephemeral auth key"
  type        = string
  sensitive   = true
}

variable "tailscale_hostname" {
  description = "Tailscale overlay hostname"
  type        = string
  default     = "aws-burst-node"
}

variable "persistent_ebs_volume_id" {
  description = "ID of the existing persistent gp3 EBS volume to attach on boot"
  type        = string
  default     = ""
}

variable "deployer_ssh_public_key" {
  description = "Pre-shared Ed25519 public SSH key for the deployer user"
  type        = string
}

variable "tags" {
  description = "Resource tags applied to the burst node and attachments"
  type        = map(string)
  default = {
    Project   = "Dendrite"
    ManagedBy = "Terraform"
    Scenario  = "aws-burst"
  }
}
