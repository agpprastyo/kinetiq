package ingestion

import (
	"testing"
)

func TestEnricher_ParseUserAgent(t *testing.T) {
	enricher := NewEnricher()

	tests := []struct {
		name            string
		ua              string
		expectedDevice  string
		expectedOS      string
		expectedBrowser string
	}{
		{
			name:            "Chrome on macOS Desktop",
			ua:              "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			expectedDevice:  "desktop",
			expectedOS:      "macOS",
			expectedBrowser: "Chrome",
		},
		{
			name:            "Edge on Windows Desktop",
			ua:              "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0",
			expectedDevice:  "desktop",
			expectedOS:      "Windows",
			expectedBrowser: "Edge",
		},
		{
			name:            "Firefox on Linux Desktop",
			ua:              "Mozilla/5.0 (X11; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/119.0",
			expectedDevice:  "desktop",
			expectedOS:      "Linux",
			expectedBrowser: "Firefox",
		},
		{
			name:            "Safari on iPhone Mobile",
			ua:              "Mozilla/5.0 (iPhone; CPU iPhone OS 17_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Mobile/15E148 Safari/604.1",
			expectedDevice:  "mobile",
			expectedOS:      "iOS",
			expectedBrowser: "Safari",
		},
		{
			name:            "Chrome on Android Mobile",
			ua:              "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.6099.43 Mobile Safari/537.36",
			expectedDevice:  "mobile",
			expectedOS:      "Android",
			expectedBrowser: "Chrome",
		},
		{
			name:            "iPad Tablet Safari",
			ua:              "Mozilla/5.0 (iPad; CPU OS 16_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.5 Mobile/15E148 Safari/604.1",
			expectedDevice:  "tablet",
			expectedOS:      "iOS",
			expectedBrowser: "Safari",
		},
		{
			name:            "Googlebot Crawler",
			ua:              "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
			expectedDevice:  "bot",
			expectedOS:      "unknown",
			expectedBrowser: "other",
		},
		{
			name:            "Empty User-Agent",
			ua:              "",
			expectedDevice:  "unknown",
			expectedOS:      "unknown",
			expectedBrowser: "other",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			meta := enricher.Enrich("127.0.0.1", tc.ua, "")
			if meta.DeviceType != tc.expectedDevice {
				t.Errorf("expected device %q, got %q", tc.expectedDevice, meta.DeviceType)
			}
			if meta.OS != tc.expectedOS {
				t.Errorf("expected OS %q, got %q", tc.expectedOS, meta.OS)
			}
			if meta.Browser != tc.expectedBrowser {
				t.Errorf("expected browser %q, got %q", tc.expectedBrowser, meta.Browser)
			}
		})
	}
}

func TestEnricher_GeoIPLookup(t *testing.T) {
	enricher := NewEnricher()

	tests := []struct {
		name            string
		ip              string
		headerCountry   string
		expectedCountry string
	}{
		{
			name:            "Cloudflare header takes precedence",
			ip:              "8.8.8.8",
			headerCountry:   "ID",
			expectedCountry: "ID",
		},
		{
			name:            "Localhost IP fallback",
			ip:              "127.0.0.1",
			headerCountry:   "",
			expectedCountry: "LOCAL",
		},
		{
			name:            "IPv6 localhost fallback",
			ip:              "::1",
			headerCountry:   "",
			expectedCountry: "LOCAL",
		},
		{
			name:            "Private network 192.168.x.x fallback",
			ip:              "192.168.1.100",
			headerCountry:   "",
			expectedCountry: "LOCAL",
		},
		{
			name:            "Private network 10.x.x.x fallback",
			ip:              "10.0.4.15",
			headerCountry:   "",
			expectedCountry: "LOCAL",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			meta := enricher.Enrich(tc.ip, "Mozilla/5.0", tc.headerCountry)
			if meta.Country != tc.expectedCountry {
				t.Errorf("expected country %q, got %q", tc.expectedCountry, meta.Country)
			}
		})
	}
}
