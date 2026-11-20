package ingestion

import (
	"net"
	"strings"
)

type ClientMetadata struct {
	Country    string `json:"country"`
	City       string `json:"city"`
	DeviceType string `json:"device_type"`
	OS         string `json:"os"`
	Browser    string `json:"browser"`
}

type Enricher interface {
	Enrich(clientIP, userAgent, headerCountry string) ClientMetadata
}

type FastEnricher struct{}

func NewEnricher() Enricher {
	return &FastEnricher{}
}

func (e *FastEnricher) Enrich(clientIP, userAgent, headerCountry string) ClientMetadata {
	meta := ClientMetadata{
		Country:    "unknown",
		City:       "",
		DeviceType: "unknown",
		OS:         "unknown",
		Browser:    "other",
	}

	// 1. Resolve Country
	if hc := strings.TrimSpace(headerCountry); hc != "" {
		meta.Country = strings.ToUpper(hc)
	} else if isLocalOrPrivateIP(clientIP) {
		meta.Country = "LOCAL"
	}

	// 2. Parse User-Agent
	ua := strings.TrimSpace(userAgent)
	if ua == "" {
		return meta
	}

	lowerUA := strings.ToLower(ua)

	// Bot detection
	if strings.Contains(lowerUA, "bot") ||
		strings.Contains(lowerUA, "spider") ||
		strings.Contains(lowerUA, "crawl") ||
		strings.Contains(lowerUA, "slurp") ||
		strings.Contains(lowerUA, "mediapartners") {
		meta.DeviceType = "bot"
		return meta
	}

	// OS Detection
	if strings.Contains(lowerUA, "iphone") {
		meta.OS = "iOS"
		meta.DeviceType = "mobile"
	} else if strings.Contains(lowerUA, "ipad") {
		meta.OS = "iOS"
		meta.DeviceType = "tablet"
	} else if strings.Contains(lowerUA, "android") {
		meta.OS = "Android"
		if strings.Contains(lowerUA, "mobile") {
			meta.DeviceType = "mobile"
		} else {
			meta.DeviceType = "tablet"
		}
	} else if strings.Contains(lowerUA, "macintosh") || strings.Contains(lowerUA, "mac os x") {
		meta.OS = "macOS"
		meta.DeviceType = "desktop"
	} else if strings.Contains(lowerUA, "windows") {
		meta.OS = "Windows"
		meta.DeviceType = "desktop"
	} else if strings.Contains(lowerUA, "linux") {
		meta.OS = "Linux"
		meta.DeviceType = "desktop"
	}

	// Tablet check
	if strings.Contains(lowerUA, "tablet") {
		meta.DeviceType = "tablet"
	}

	// Browser Detection (Order matters!)
	if strings.Contains(lowerUA, "edg/") || strings.Contains(lowerUA, "edge/") {
		meta.Browser = "Edge"
	} else if strings.Contains(lowerUA, "chrome") || strings.Contains(lowerUA, "crios") {
		meta.Browser = "Chrome"
	} else if strings.Contains(lowerUA, "firefox") || strings.Contains(lowerUA, "fxios") {
		meta.Browser = "Firefox"
	} else if strings.Contains(lowerUA, "safari") && !strings.Contains(lowerUA, "chrome") && !strings.Contains(lowerUA, "android") {
		meta.Browser = "Safari"
	}

	if meta.DeviceType == "unknown" && (meta.OS == "macOS" || meta.OS == "Windows" || meta.OS == "Linux") {
		meta.DeviceType = "desktop"
	}

	return meta
}

func isLocalOrPrivateIP(ipStr string) bool {
	ipStr = strings.TrimSpace(ipStr)
	if ipStr == "" || ipStr == "127.0.0.1" || ipStr == "::1" || ipStr == "localhost" {
		return true
	}

	// Strip port if present
	if host, _, err := net.SplitHostPort(ipStr); err == nil {
		ipStr = host
	}

	parsedIP := net.ParseIP(ipStr)
	if parsedIP == nil {
		return false
	}

	if parsedIP.IsLoopback() || parsedIP.IsPrivate() || parsedIP.IsUnspecified() || parsedIP.IsLinkLocalUnicast() {
		return true
	}

	return false
}
