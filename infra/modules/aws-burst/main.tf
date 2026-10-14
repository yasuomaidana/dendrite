terraform {
  required_version = ">= 1.5.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

# Zero-Ingress Security Group: Prohibits 0.0.0.0/0 inbound; all access strictly via Tailscale overlay
resource "aws_security_group" "burst_node" {
  name        = "dendrite-burst-zero-ingress-sg"
  description = "Zero public ingress security group for Dendrite AWS burst node"
  vpc_id      = var.vpc_id

  # Outbound egress allows encrypted Tailscale mesh handshakes and package updates
  egress {
    description = "Allow all outbound traffic for Tailscale mesh coordination"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(var.tags, {
    Name = "dendrite-aws-burst-sg"
  })
}

# Cloud-Init User Data Rendering
locals {
  rendered_cloud_init = templatefile("${path.module}/cloud-init.yaml.tftpl", {
    hostname                = var.tailscale_hostname
    tailscale_auth_key      = var.tailscale_auth_key
    deployer_ssh_public_key = var.deployer_ssh_public_key
  })
}

# Spot Instance Request for elastic compute offloading
resource "aws_spot_instance_request" "burst_compute" {
  ami                  = var.ami_id
  instance_type        = var.instance_type
  spot_price           = var.spot_price != "" ? var.spot_price : null
  spot_type            = "one-time"
  wait_for_fulfillment = true
  subnet_id            = var.subnet_id

  vpc_security_group_ids = [aws_security_group.burst_node.id]
  user_data              = local.rendered_cloud_init

  root_block_device {
    volume_type           = "gp3"
    volume_size           = 50
    delete_on_termination = true
    encrypted             = true
  }

  tags = merge(var.tags, {
    Name = "dendrite-aws-burst-spot"
  })
}

# Attach persistent EBS volume (stores Ghidra caches, AST index, etc.)
resource "aws_volume_attachment" "persistent_storage" {
  count       = var.persistent_ebs_volume_id != "" ? 1 : 0
  device_name = "/dev/xvdf"
  volume_id   = var.persistent_ebs_volume_id
  instance_id = aws_spot_instance_request.burst_compute.spot_instance_id

  # Allow clean unmounting on termination
  stop_instance_before_detaching = false
  force_detach                   = false
}
