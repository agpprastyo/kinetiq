package ingestion

import (
	"testing"
	"time"
)

func TestClampOccurredAt(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	cases := map[string]struct {
		in   time.Time
		want time.Time
	}{
		"within bounds kept":        {now.Add(-24 * time.Hour), now.Add(-24 * time.Hour)},
		"small clock skew kept":     {now.Add(30 * time.Minute), now.Add(30 * time.Minute)},
		"far future clamped to now": {now.Add(48 * time.Hour), now},
		"far past clamped to now":   {now.Add(-MaxEventAge - time.Hour), now},
		"exactly max age kept":      {now.Add(-MaxEventAge), now.Add(-MaxEventAge)},
		"exactly max future kept":   {now.Add(MaxFutureSkew), now.Add(MaxFutureSkew)},
		"year 3000 clamped":         {time.Date(3000, 1, 1, 0, 0, 0, 0, time.UTC), now},
		"zero time clamped":         {time.Time{}, now},
	}
	for name, c := range cases {
		if got := ClampOccurredAt(c.in, now); !got.Equal(c.want) {
			t.Errorf("%s: got %v want %v", name, got, c.want)
		}
	}
}
