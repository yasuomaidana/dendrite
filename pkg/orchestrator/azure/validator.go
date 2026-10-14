package azure

import (
	"strings"
)

// ValidateZeroIngressSecurityGroup enforces the AC-2 requirement from FEAT-019:
// Zero inbound rules from public IPv4 (0.0.0.0/0), IPv6 (::/0), wildcard (*), or Internet service tag.
func ValidateZeroIngressSecurityGroup(nsg NetworkSecurityGroupConfig) error {
	for _, rule := range nsg.Rules {
		direction := strings.ToLower(strings.TrimSpace(rule.Direction))
		access := strings.ToLower(strings.TrimSpace(rule.Access))
		source := strings.ToLower(strings.TrimSpace(rule.SourceAddressPrefix))

		if direction == "inbound" && (access == "allow" || access == "") {
			// Prohibit open public ingress across all protocols and ports
			if source == "0.0.0.0/0" || source == "::/0" || source == "*" || source == "internet" || source == "any" {
				return &SecurityValidationError{
					Rule: rule,
					Err:  ErrPublicIngressDetected,
				}
			}
		}
	}
	return nil
}
