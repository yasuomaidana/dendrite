package showcase

import (
	"strings"
)

// Known OSI-approved licenses and their characteristics.
var osiLicenses = map[string]struct {
	Copyleft   bool
	Permissive bool
}{
	"GPL-3.0":          {Copyleft: true, Permissive: false},
	"GPL-3.0-only":     {Copyleft: true, Permissive: false},
	"GPL-3.0-or-later": {Copyleft: true, Permissive: false},
	"MIT":              {Copyleft: false, Permissive: true},
	"Apache-2.0":       {Copyleft: false, Permissive: true},
	"BSD-2-Clause":     {Copyleft: false, Permissive: true},
	"BSD-3-Clause":     {Copyleft: false, Permissive: true},
	"CC-BY-4.0":        {Copyleft: false, Permissive: true},
	"CC0-1.0":          {Copyleft: false, Permissive: true},
}

// AuditLicense checks if the license identifier conforms to open-source compliance rules.
func AuditLicense(target, licenseID string) SPDXAuditResult {
	norm := strings.TrimSpace(licenseID)
	info, found := osiLicenses[norm]
	if !found {
		// Check prefix matches e.g. "GPL-3.0"
		for k, v := range osiLicenses {
			if strings.EqualFold(norm, k) {
				found = true
				info = v
				norm = k
				break
			}
		}
	}

	if !found {
		return SPDXAuditResult{
			Target:      target,
			LicenseID:   licenseID,
			OSIApproved: false,
			IsCopyleft:  false,
			Compliant:   false,
			Notes:       "Unrecognized or non-OSI approved license; potentially proprietary or unlicensed",
		}
	}

	notes := "Compliant open-source license; safe for cleanroom reproduction and public benchmarking"
	if info.Copyleft {
		notes += " (Copyleft requirements apply to derived binaries)"
	}

	return SPDXAuditResult{
		Target:      target,
		LicenseID:   norm,
		OSIApproved: true,
		IsCopyleft:  info.Copyleft,
		Compliant:   true,
		Notes:       notes,
	}
}
