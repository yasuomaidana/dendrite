package store

import (
	"fmt"
	"strings"

	"Dendrite/pkg/store/status"
)

// DefaultStoreCapacityBytes is the default storage limit (e.g. 10 GB).
const DefaultStoreCapacityBytes int64 = 10 * 1024 * 1024 * 1024

// RenderStorageBar renders an SVG / CSS visual storage bar.
func RenderStorageBar(report *status.StoreStatusReport, capacityBytes int64) string {
	if capacityBytes <= 0 {
		capacityBytes = DefaultStoreCapacityBytes
	}

	percentUsed := (float64(report.TotalCompBytes) / float64(capacityBytes)) * 100
	if percentUsed > 100 {
		percentUsed = 100
	}

	var sb strings.Builder
	sb.WriteString("<div class=\"dendrite-storage-bar-widget\" style=\"font-family: sans-serif; padding: 12px; background: #1e1e2e; border-radius: 8px; color: #cdd6f4;\">\n")
	sb.WriteString(fmt.Sprintf("  <div style=\"display: flex; justify-content: space-between; margin-bottom: 6px;\">\n"))
	sb.WriteString(fmt.Sprintf("    <span style=\"font-weight: bold;\">Persistent Segmented Store</span>\n"))
	sb.WriteString(fmt.Sprintf("    <span>%.2f MB / %.2f GB (%.1f%%)</span>\n",
		float64(report.TotalCompBytes)/(1024*1024),
		float64(capacityBytes)/(1024*1024*1024),
		percentUsed))
	sb.WriteString("  </div>\n")

	sb.WriteString("  <div style=\"width: 100%; height: 12px; background: #313244; border-radius: 6px; overflow: hidden; display: flex;\">\n")
	sb.WriteString(fmt.Sprintf("    <div style=\"width: %.1f%%; background: linear-gradient(90deg, #89b4fa, #a6e3a1); transition: width 0.3s;\"></div>\n", percentUsed))
	sb.WriteString("  </div>\n")

	sb.WriteString("  <div style=\"display: flex; gap: 16px; margin-top: 8px; font-size: 12px; color: #a6adc8;\">\n")
	sb.WriteString(fmt.Sprintf("    <span>Total Projects: %d</span>\n", report.TotalStores))
	sb.WriteString(fmt.Sprintf("    <span>Segments: %d</span>\n", report.TotalSegments))
	sb.WriteString(fmt.Sprintf("    <span>Stored: %d</span>\n", report.StoredStores))
	sb.WriteString(fmt.Sprintf("    <span>Partial: %d</span>\n", report.PartialStores))
	sb.WriteString("  </div>\n")
	sb.WriteString("</div>\n")

	return sb.String()
}

// RenderDashboard emits the full HTML markup for the Store Inspector Web UI tab.
func RenderDashboard(report *status.StoreStatusReport) string {
	var sb strings.Builder
	sb.WriteString(RenderStorageBar(report, DefaultStoreCapacityBytes))
	sb.WriteString("\n<table class=\"table table-striped\" style=\"width: 100%; border-collapse: collapse; margin-top: 16px; text-align: left;\">\n")
	sb.WriteString("  <thead>\n    <tr style=\"background: #181825; color: #cdd6f4;\">\n")
	sb.WriteString("      <th style=\"padding: 8px;\">Store ID</th>\n")
	sb.WriteString("      <th style=\"padding: 8px;\">Kind</th>\n")
	sb.WriteString("      <th style=\"padding: 8px;\">State</th>\n")
	sb.WriteString("      <th style=\"padding: 8px;\">Progress</th>\n")
	sb.WriteString("      <th style=\"padding: 8px;\">Segments</th>\n")
	sb.WriteString("      <th style=\"padding: 8px;\">Size</th>\n")
	sb.WriteString("      <th style=\"padding: 8px;\">Tokens</th>\n")
	sb.WriteString("    </tr>\n  </thead>\n  <tbody>\n")

	for _, s := range report.Stores {
		badgeColor := "#a6e3a1" // green
		if s.State != "STORED" {
			badgeColor = "#f9e2af" // yellow
		}

		sb.WriteString("    <tr style=\"border-bottom: 1px solid #313244;\">\n")
		sb.WriteString(fmt.Sprintf("      <td style=\"padding: 8px; font-family: monospace;\">%s</td>\n", s.StoreID))
		sb.WriteString(fmt.Sprintf("      <td style=\"padding: 8px;\">%s</td>\n", s.Kind))
		sb.WriteString(fmt.Sprintf("      <td style=\"padding: 8px;\"><span style=\"color: %s; font-weight: bold;\">%s</span></td>\n", badgeColor, s.State))
		sb.WriteString(fmt.Sprintf("      <td style=\"padding: 8px;\">%.1f%%</td>\n", s.Progress))
		sb.WriteString(fmt.Sprintf("      <td style=\"padding: 8px;\">%d</td>\n", s.SegmentsCount))
		sb.WriteString(fmt.Sprintf("      <td style=\"padding: 8px;\">%d KB</td>\n", s.CompressedBytes/1024))
		sb.WriteString(fmt.Sprintf("      <td style=\"padding: 8px;\">%d</td>\n", s.TokensEstimated))
		sb.WriteString("    </tr>\n")
	}

	sb.WriteString("  </tbody>\n</table>\n")
	return sb.String()
}
