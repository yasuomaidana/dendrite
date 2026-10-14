package orchestrator_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAWSBurstModule_FilesAndSecurityGuardrails(t *testing.T) {
	moduleDir := filepath.Join("..", "..", "infra", "modules", "aws-burst")

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

	if strings.Contains(mainStr, "ingress {") {
		t.Errorf("violation of AC-2: aws_security_group in main.tf must have zero public ingress rules")
	}
	if !strings.Contains(mainStr, "aws_security_group") {
		t.Errorf("expected aws_security_group resource in main.tf")
	}
	if !strings.Contains(mainStr, "aws_spot_instance_request") {
		t.Errorf("expected aws_spot_instance_request in main.tf")
	}
	if !strings.Contains(mainStr, "aws_volume_attachment") {
		t.Errorf("expected aws_volume_attachment in main.tf")
	}

	// 3. Verify variables
	varContent, err := os.ReadFile(filepath.Join(moduleDir, "variables.tf"))
	if err != nil {
		t.Fatalf("failed to read variables.tf: %v", err)
	}
	varStr := string(varContent)

	requiredVars := []string{
		"vpc_id",
		"subnet_id",
		"ami_id",
		"instance_type",
		"tailscale_auth_key",
		"tailscale_hostname",
		"persistent_ebs_volume_id",
		"deployer_ssh_public_key",
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
		"spot_request_id",
		"instance_id",
		"private_ip",
		"security_group_id",
		"attached_ebs_volume_id",
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

	if !strings.HasPrefix(tplStr, "#cloud-config") {
		t.Errorf("expected cloud-init template to start with #cloud-config")
	}
	if !strings.Contains(tplStr, "tailscale up --authkey=${tailscale_auth_key}") {
		t.Errorf("expected tailscale up command in cloud-init template")
	}
}
