package orchestrator_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAzureBurstModule_FilesAndSecurityGuardrails(t *testing.T) {
	moduleDir := filepath.Join("..", "..", "infra", "modules", "azure-burst")

	// 1. Verify all required Terraform module files exist
	expectedFiles := []string{
		"main.tf",
		"variables.tf",
		"outputs.tf",
		"cloud-init.yaml.tftpl",
	}

	for _, file := range expectedFiles {
		fullPath := filepath.Join(moduleDir, file)
		info, err := os.Stat(fullPath)
		if err != nil {
			t.Fatalf("expected module file %s to exist: %v", file, err)
		}
		if info.Size() == 0 {
			t.Errorf("expected module file %s to be non-empty", file)
		}
	}

	// 2. Verify Security Guardrail AC-2: Zero Public Ingress
	mainContent, err := os.ReadFile(filepath.Join(moduleDir, "main.tf"))
	if err != nil {
		t.Fatalf("failed to read main.tf: %v", err)
	}
	mainStr := string(mainContent)

	if strings.Contains(mainStr, "source_address_prefix      = \"0.0.0.0/0\"") {
		t.Errorf("violation of AC-2: azurerm_network_security_group in main.tf must not allow 0.0.0.0/0 inbound")
	}
	if !strings.Contains(mainStr, "azurerm_network_security_group") {
		t.Errorf("expected azurerm_network_security_group resource in main.tf")
	}
	if !strings.Contains(mainStr, "azurerm_linux_virtual_machine") {
		t.Errorf("expected azurerm_linux_virtual_machine in main.tf")
	}
	if !strings.Contains(mainStr, "azurerm_managed_disk") {
		t.Errorf("expected azurerm_managed_disk in main.tf")
	}
	if !strings.Contains(mainStr, "azurerm_virtual_machine_data_disk_attachment") {
		t.Errorf("expected azurerm_virtual_machine_data_disk_attachment in main.tf")
	}
	if !strings.Contains(mainStr, "azurerm_network_interface") {
		t.Errorf("expected azurerm_network_interface in main.tf")
	}

	// 3. Verify variables
	varContent, err := os.ReadFile(filepath.Join(moduleDir, "variables.tf"))
	if err != nil {
		t.Fatalf("failed to read variables.tf: %v", err)
	}
	varStr := string(varContent)

	requiredVars := []string{
		"azure_region",
		"resource_group_name",
		"subnet_id",
		"vm_size",
		"priority",
		"max_bid_price",
		"tailscale_auth_key",
		"tailscale_hostname",
		"persistent_managed_disk_id",
		"disk_storage_type",
		"disk_size_gb",
		"deployer_ssh_public_key",
		"tags",
	}

	for _, v := range requiredVars {
		if !strings.Contains(varStr, "variable \""+v+"\"") {
			t.Errorf("expected variable %s in variables.tf", v)
		}
	}

	// 4. Verify outputs
	outContent, err := os.ReadFile(filepath.Join(moduleDir, "outputs.tf"))
	if err != nil {
		t.Fatalf("failed to read outputs.tf: %v", err)
	}
	outStr := string(outContent)

	requiredOutputs := []string{
		"vm_id",
		"private_ip",
		"nsg_id",
		"nic_id",
		"attached_disk_id",
	}

	for _, o := range requiredOutputs {
		if !strings.Contains(outStr, "output \""+o+"\"") {
			t.Errorf("expected output %s in outputs.tf", o)
		}
	}

	// 5. Verify cloud-init template
	tplContent, err := os.ReadFile(filepath.Join(moduleDir, "cloud-init.yaml.tftpl"))
	if err != nil {
		t.Fatalf("failed to read cloud-init.yaml.tftpl: %v", err)
	}
	tplStr := string(tplContent)

	requiredTplVars := []string{
		"${hostname}",
		"${tailscale_auth_key}",
		"${deployer_ssh_public_key}",
	}

	for _, tv := range requiredTplVars {
		if !strings.Contains(tplStr, tv) {
			t.Errorf("expected cloud-init template variable %s in cloud-init.yaml.tftpl", tv)
		}
	}

	if !strings.Contains(tplStr, "/var/lib/dendrite/storage") {
		t.Errorf("expected persistent storage mount directory /var/lib/dendrite/storage in cloud-init")
	}
	if !strings.Contains(tplStr, "tailscale up") {
		t.Errorf("expected tailscale up invocation in cloud-init")
	}
}
