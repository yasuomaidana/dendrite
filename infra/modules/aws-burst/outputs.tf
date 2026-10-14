output "spot_request_id" {
  description = "The ID of the spot instance request"
  value       = aws_spot_instance_request.burst_compute.id
}

output "instance_id" {
  description = "The EC2 instance ID launched from the spot request"
  value       = aws_spot_instance_request.burst_compute.spot_instance_id
}

output "private_ip" {
  description = "Private IP address of the burst compute instance within the VPC"
  value       = aws_spot_instance_request.burst_compute.private_ip
}

output "security_group_id" {
  description = "Security group ID enforcing zero public ingress"
  value       = aws_security_group.burst_node.id
}

output "attached_ebs_volume_id" {
  description = "The persistent EBS volume ID attached to the instance"
  value       = var.persistent_ebs_volume_id != "" ? var.persistent_ebs_volume_id : null
}
