package analytics

import (
	"testing"
	"time"
)

func TestParseFunnelParams(t *testing.T) {
	p, err := ParseFunnelParams(" a, b ,,c", "7d", "strict")
	if err != nil || len(p.Steps) != 3 || p.Window != 7*24*time.Hour || p.Order != FunnelOrderStrict {
		t.Fatalf("got %+v %v", p, err)
	}
	if p, err = ParseFunnelParams("a,b", "", ""); err != nil || p.Window != DefaultFunnelWindow || p.Order != FunnelOrderOrdered {
		t.Fatalf("defaults %+v %v", p, err)
	}
	for _, c := range [][3]string{{"a", "", ""}, {"a,b,c,d,e,f", "", ""}, {"a,b", "0s", ""}, {"a,b", "91d", ""}, {"a,b", "7w", ""}, {"a,b", "", "x"}} {
		if _, err := ParseFunnelParams(c[0], c[1], c[2]); err == nil {
			t.Errorf("%v: want error", c)
		}
	}
}
