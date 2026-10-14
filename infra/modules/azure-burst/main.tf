terraform {
  required_version = ">= 1.5.0"
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.90"
    }
  }
}

# Network Security Group enforcing AC-2: Zero Public Ingress (0.0.0.0/0:22 prohibited)
# Administrative access is strictly routed through private overlay mesh (Tailscale / WireGuard)
resource "azurerm_network_security_group" "burst_nsg" {
  name                = "dendrite-burst-zero-ingress-nsg"
  location            = var.azure_region
  resource_group_name = var.resource_group_name

  security_rule {
    name                       = "AllowAllEgress"
    priority                   = 100
    direction                  = "Outbound"
    access                     = "Allow"
    protocol                   = "*"
    source_port_range          = "*"
    destination_port_range     = "*"
    source_address_prefix      = "*"
    destination_address_prefix = "*"
    description                = "Allow all outbound traffic for Tailscale mesh handshakes and package updates"
  }

  security_rule {
    name                       = "AllowTailscaleMeshInbound"
    priority                   = 200
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "22"
    source_address_prefix      = "100.64.0.0/10"
    destination_address_prefix = "*"
    description                = "Permit SSH strictly within Tailscale CGNAT private address space"
  }

  tags = merge(var.tags, {
    Name = "dendrite-azure-burst-nsg"
  })
}

# Cloud-Init Multi-Mesh Bootstrap rendering
locals {
  rendered_cloud_init = templatefile("${path.module}/cloud-init.yaml.tftpl", {
    hostname                = var.tailscale_hostname
    tailscale_auth_key      = var.tailscale_auth_key
    deployer_ssh_public_key = var.deployer_ssh_public_key
  })
}

# Network Interface without public IP (strictly private subnet)
resource "azurerm_network_interface" "burst_nic" {
  name                = "dendrite-burst-nic"
  location            = var.azure_region
  resource_group_name = var.resource_group_name

  ip_configuration {
    name                          = "internal"
    subnet_id                     = var.subnet_id
    private_ip_address_allocation = "Dynamic"
  }

  tags = var.tags
}

resource "azurerm_network_interface_security_group_association" "burst_nic_nsg" {
  network_interface_id      = azurerm_network_interface.burst_nic.id
  network_security_group_id = azurerm_network_security_group.burst_nsg.id
}

# Persistent Azure Managed Disk (stores Ghidra symbols, Tree-sitter DBs, volume caches)
resource "azurerm_managed_disk" "persistent_storage" {
  count                = var.persistent_managed_disk_id == "" ? 1 : 0
  name                 = "dendrite-burst-persistent-disk"
  location             = var.azure_region
  resource_group_name  = var.resource_group_name
  storage_account_type = var.disk_storage_type
  create_option        = "Empty"
  disk_size_gb         = var.disk_size_gb

  tags = merge(var.tags, {
    Component = "storage"
  })
}

# Elastic Spot Virtual Machine
resource "azurerm_linux_virtual_machine" "burst_compute" {
  name                  = "dendrite-azure-burst-spot"
  location              = var.azure_region
  resource_group_name   = var.resource_group_name
  size                  = var.vm_size
  admin_username        = "deployer"
  network_interface_ids = [azurerm_network_interface.burst_nic.id]

  priority        = var.priority
  eviction_policy = var.priority == "Spot" ? "Deallocate" : null
  max_bid_price   = var.priority == "Spot" && var.max_bid_price != "" ? var.max_bid_price : null

  admin_ssh_key {
    username   = "deployer"
    public_key = var.deployer_ssh_public_key
  }

  os_disk {
    caching              = "ReadWrite"
    storage_account_type = "StandardSSD_LRS"
    disk_size_gb         = 50
  }

  source_image_reference {
    publisher = "Canonical"
    offer     = "0001-com-ubuntu-server-jammy"
    sku       = "22_04-lts-gen2"
    version   = "latest"
  }

  custom_data = base64encode(local.rendered_cloud_init)

  tags = merge(var.tags, {
    Name = "dendrite-azure-burst-spot"
  })
}

# Data Disk Attachment for Persistent Managed Disk on boot (LUN 0)
resource "azurerm_virtual_machine_data_disk_attachment" "burst_data_attachment" {
  managed_disk_id    = var.persistent_managed_disk_id != "" ? var.persistent_managed_disk_id : azurerm_managed_disk.persistent_storage[0].id
  virtual_machine_id = azurerm_linux_virtual_machine.burst_compute.id
  lun                = 0
  caching            = "ReadWrite"
  create_option      = "Attach"
}
