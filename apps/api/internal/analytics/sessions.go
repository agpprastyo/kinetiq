package analytics

import "time"

const DefaultSessionTimeout = 30 * time.Minute

type Session struct {
	SessionID       string    `json:"session_id"`
	DistinctID      string    `json:"distinct_id"`
	StartedAt       time.Time `json:"started_at"`
	EndedAt         time.Time `json:"ended_at"`
	DurationSeconds int64     `json:"duration_seconds"`
	EventCount      int       `json:"event_count"`
	Pageviews       int       `json:"pageviews"`
	EntryPage       string    `json:"entry_page"`
	ExitPage        string    `json:"exit_page"`
	Referrer        string    `json:"referrer,omitempty"`
	ReferringDomain string    `json:"referring_domain,omitempty"`
	UTMSource       string    `json:"utm_source,omitempty"`
	UTMMedium       string    `json:"utm_medium,omitempty"`
	UTMCampaign     string    `json:"utm_campaign,omitempty"`
	UTMTerm         string    `json:"utm_term,omitempty"`
	UTMContent      string    `json:"utm_content,omitempty"`
}

type SessionCount struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

type SessionsResponse struct {
	TotalSessions       int64          `json:"total_sessions"`
	AvgDurationSeconds  int64          `json:"avg_duration_seconds"`
	TopEntryPages       []SessionCount `json:"top_entry_pages"`
	TopReferringDomains []SessionCount `json:"top_referring_domains"`
	Sessions            []Session      `json:"sessions"`
}
