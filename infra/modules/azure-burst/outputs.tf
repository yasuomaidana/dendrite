output "vm_id" {
  description = "The Resource ID of the Azure Linux Virtual Machine"
  value       = azurerm_linux_virtual_machine.burst_compute.id
}

output "private_ip" {
  description = "Private IP address of the burst compute instance within the virtual network"
  value       = azurerm_network_interface.burst_nic.private_ip_address
}

output "nsg_id" {
  description = "Resource ID of the Network Security Group enforcing zero public ingress"
  value       = azurerm_network_security_group.burst_nsg.id
}

output "nic_id" {
  description = "Resource ID of the network interface attached to the burst VM"
  value       = azurerm_network_interface.burst_nic.id
}

output "attached_disk_id" {
  description = "Resource ID of the persistent Managed Disk attached to the instance"
  value       = var.persistent_managed_disk_id != "" ? var.persistent_managed_disk_id : azurerm_managed_disk.persistent_storage[0].id
}
