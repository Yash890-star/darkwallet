package model

import (
	"fmt"
	"strings"
	"time"
)

// SupportedInterval represents valid interval options.
type SupportedInterval struct {
	Key      string        `json:"key"`
	Label    string        `json:"label"`
	Duration time.Duration `json:"duration"`
}

var Intervals = []SupportedInterval{
	{Key: "6h", Label: "Every 6 hrs", Duration: 6 * time.Hour},
	{Key: "12h", Label: "Every 12 hrs", Duration: 12 * time.Hour},
	{Key: "24h", Label: "Every 24 hrs", Duration: 24 * time.Hour},
	{Key: "2d", Label: "Every 2 days", Duration: 48 * time.Hour},
	{Key: "1w", Label: "Every 1 week", Duration: 7 * 24 * time.Hour},
	{Key: "1m", Label: "Every 1 month", Duration: 30 * 24 * time.Hour},
}

// ParseInterval converts string into duration, friendly label, and canonical key.
func ParseInterval(raw string) (time.Duration, string, string, error) {
	clean := strings.ToLower(strings.TrimSpace(raw))
	for _, it := range Intervals {
		if clean == strings.ToLower(it.Key) || clean == strings.ToLower(it.Label) ||
			clean == strings.ReplaceAll(strings.ToLower(it.Label), "every ", "") {
			return it.Duration, it.Label, it.Key, nil
		}
	}

	switch clean {
	case "6h", "6hrs", "every 6hrs", "every 6 hrs":
		return 6 * time.Hour, "Every 6 hrs", "6h", nil
	case "12h", "12hrs", "every 12hrs", "every 12 hrs":
		return 12 * time.Hour, "Every 12 hrs", "12h", nil
	case "24h", "24hrs", "every 24hrs", "every 24 hrs", "1d", "every 1 day":
		return 24 * time.Hour, "Every 24 hrs", "24h", nil
	case "2d", "2days", "every 2 days", "every 2days":
		return 48 * time.Hour, "Every 2 days", "2d", nil
	case "1w", "1week", "every 1 week", "every 1week", "7d":
		return 7 * 24 * time.Hour, "Every 1 week", "1w", nil
	case "1m", "1month", "every 1 month", "every 1month", "30d":
		return 30 * 24 * time.Hour, "Every 1 month", "1m", nil
	default:
		return 0, "", "", fmt.Errorf("invalid interval '%s'. Supported: 6h, 12h, 24h, 2d, 1w, 1m", raw)
	}
}

// GetIntervalFromDuration finds the closest SupportedInterval for a given duration.
func GetIntervalFromDuration(dur time.Duration) (string, string) {
	for _, it := range Intervals {
		if it.Duration == dur {
			return it.Key, it.Label
		}
	}
	return "24h", "Every 24 hrs"
}
