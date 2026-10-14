package aws

import (
	"strings"
)

// ValidateZeroIngressSecurityGroup enforces the AC-2 requirement from FEAT-007:
// Zero inbound rules from public IPv4 (0.0.0.0/0) or IPv6 (::/0).
func ValidateZeroIngressSecurityGroup(sg SecurityGroupConfig) error {
	for _, rule := range sg.Rules {
		ruleType := strings.ToLower(strings.TrimSpace(rule.Type))
		cidr := strings.TrimSpace(rule.CIDR)

		if ruleType == "ingress" {
			// Prohibit open public ingress across all protocols and ports
			if cidr == "0.0.0.0/0" || cidr == "::/0" {
				return &SecurityValidationError{
					Rule: rule,
					Err:  ErrPublicIngressDetected,
				}
			}
		}
	}
	return nil
}
