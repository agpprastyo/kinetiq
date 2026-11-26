package analytics_test

import (
	"strings"
	"testing"
	"time"

	"kinetiq/internal/analytics"
)

func TestParseFilter_Operators(t *testing.T) {
	cases := map[string]string{
		"eq":         `{"property":"browser","op":"eq","value":"chrome"}`,
		"neq":        `{"property":"browser","op":"neq","value":"chrome"}`,
		"contains":   `{"property":"url","op":"contains","value":"pricing"}`,
		"gt":         `{"property":"amount","op":"gt","value":10}`,
		"lt":         `{"property":"amount","op":"lt","value":10.5}`,
		"in":         `{"property":"browser","op":"in","value":["chrome","safari"]}`,
		"is_set":     `{"property":"browser","op":"is_set"}`,
		"is_not_set": `{"property":"browser","op":"is_not_set"}`,
	}
	for op, raw := range cases {
		t.Run(op, func(t *testing.T) {
			f, err := analytics.ParseFilter(raw)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			sql, args, err := f.Compile("e", 3)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if sql == "" || len(args) == 0 {
				t.Fatalf("empty compile: %q %v", sql, args)
			}
			if !strings.Contains(sql, "$4") {
				t.Errorf("placeholders must start after existing args: %s", sql)
			}
		})
	}
}

func TestParseFilter_AndOrNesting(t *testing.T) {
	f, err := analytics.ParseFilter(`{"group":"and","filters":[
		{"property":"a","op":"eq","value":"1"},
		{"group":"or","filters":[
			{"property":"b","op":"eq","value":"2"},
			{"property":"c","op":"is_set"}]}]}`)
	if err != nil {
		t.Fatal(err)
	}
	sql, args, err := f.Compile("e", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, " AND ") || !strings.Contains(sql, " OR ") {
		t.Errorf("expected AND and OR in %s", sql)
	}
	if len(args) != 5 { // a,1 b,2 c
		t.Errorf("args=%v", args)
	}
}

func TestParseFilter_Empty(t *testing.T) {
	f, err := analytics.ParseFilter("")
	if err != nil || f != nil {
		t.Fatalf("empty must be nil,nil got %v %v", f, err)
	}
}

func TestParseFilter_Invalid(t *testing.T) {
	bad := map[string]string{
		"unknown op":         `{"property":"a","op":"like","value":"x"}`,
		"missing property":   `{"op":"eq","value":"x"}`,
		"control char name":  `{"property":"a\u0000b","op":"eq","value":"x"}`,
		"long name":          `{"property":"` + strings.Repeat("a", 129) + `","op":"eq","value":"x"}`,
		"gt non number":      `{"property":"a","op":"gt","value":"x"}`,
		"in not array":       `{"property":"a","op":"in","value":"x"}`,
		"in empty":           `{"property":"a","op":"in","value":[]}`,
		"eq missing value":   `{"property":"a","op":"eq"}`,
		"bad group":          `{"group":"xor","filters":[{"property":"a","op":"is_set"}]}`,
		"empty group":        `{"group":"and","filters":[]}`,
		"bad scope":          `{"property":"a","op":"is_set","scope":"x"}`,
		"not json":           `{`,
		"contains non str":   `{"property":"a","op":"contains","value":5}`,
		"too deep":           `{"group":"and","filters":[{"group":"and","filters":[{"group":"and","filters":[{"group":"and","filters":[{"property":"a","op":"is_set"}]}]}]}]}`,
		"unknown json field": `{"property":"a","op":"is_set","sql":"1=1"}`,
	}
	for name, raw := range bad {
		t.Run(name, func(t *testing.T) {
			f, err := analytics.ParseFilter(raw)
			if err == nil {
				_, _, err = f.Compile("e", 0)
			}
			if err == nil {
				t.Fatalf("expected error for %s", raw)
			}
		})
	}
}

func TestFilter_NeverInlinesUserInput(t *testing.T) {
	evil := `x'); DROP TABLE events;--`
	for _, scope := range []string{"event", "person"} {
		raw := `{"group":"or","filters":[
			{"property":"` + strings.ReplaceAll(evil, `'`, `'`) + `","op":"eq","value":"` + evil + `","scope":"` + scope + `"},
			{"property":"` + evil + `","op":"contains","value":"` + evil + `"},
			{"property":"` + evil + `","op":"in","value":["` + evil + `"]},
			{"property":"` + evil + `","op":"is_not_set","scope":"` + scope + `"}]}`
		f, err := analytics.ParseFilter(raw)
		if err != nil {
			t.Fatal(err)
		}
		sql, args, err := f.Compile("e", 2)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(sql, "DROP") || strings.Contains(sql, "x'") {
			t.Fatalf("user input leaked into SQL: %s", sql)
		}
		if len(args) == 0 {
			t.Fatal("no args")
		}
	}
}

func TestValidatePropertyName(t *testing.T) {
	for _, ok := range []string{"browser", "$device_type", "a.b-c d", "naïve"} {
		if err := analytics.ValidatePropertyName(ok); err != nil {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "a\nb", strings.Repeat("a", 129), "\xff"} {
		if err := analytics.ValidatePropertyName(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestInferPropertyType(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"string"}, "string"},
		{[]string{"number"}, "number"},
		{[]string{"boolean"}, "boolean"},
		{[]string{"object"}, "object"},
		{[]string{"array"}, "array"},
		{[]string{"number", "string"}, "mixed"},
		{[]string{"string", "null"}, "string"},
		{[]string{"null"}, "unknown"},
		{nil, "unknown"},
		{[]string{"string", "number"}, "mixed"},
	}
	for _, c := range cases {
		if got := analytics.InferPropertyType(c.in); got != c.want {
			t.Errorf("%v: got %s want %s", c.in, got, c.want)
		}
	}
}

func TestTopNWithOther(t *testing.T) {
	in := []analytics.BreakdownItem{{Value: "d", Count: 2}, {Value: "a", Count: 5}, {Value: "c", Count: 3}, {Value: "b", Count: 4}}
	out := analytics.TopNWithOther(in, 2)
	if len(out) != 3 || out[0].Value != "a" || out[1].Value != "b" || out[2].Value != "other" || out[2].Count != 5 {
		t.Fatalf("got %+v", out)
	}
	if got := analytics.TopNWithOther(in, 4); len(got) != 4 {
		t.Fatalf("no other expected: %+v", got)
	}
}

func TestSeriesTopN(t *testing.T) {
	t1 := time.Unix(0, 0).UTC()
	t2 := t1.Add(time.Hour)
	rows := []analytics.SeriesRow{
		{Bucket: t1, Value: "a", Count: 5}, {Bucket: t2, Value: "a", Count: 5},
		{Bucket: t1, Value: "b", Count: 2}, {Bucket: t2, Value: "c", Count: 1}, {Bucket: t2, Value: "d", Count: 1},
	}
	s := analytics.SeriesTopN(rows, 2)
	if len(s) != 3 || s[0].Value != "a" || s[1].Value != "b" || s[2].Value != "other" {
		t.Fatalf("got %+v", s)
	}
	if len(s[2].Points) != 1 || s[2].Points[0].Count != 2 {
		t.Fatalf("other points %+v", s[2].Points)
	}
}

func TestParseQueryOptions(t *testing.T) {
	if _, err := analytics.ParseQueryOptions("", "", "", ""); err != nil {
		t.Fatal(err)
	}
	o, err := analytics.ParseQueryOptions(`{"property":"a","op":"is_set"}`, "browser", "", "5")
	if err != nil || o.BreakdownBy != "browser" || o.BreakdownLimit != 5 || o.Filter == nil {
		t.Fatalf("%+v %v", o, err)
	}
	if o, _ := analytics.ParseQueryOptions("", "browser", "", ""); o.BreakdownLimit != 10 {
		t.Errorf("default limit %d", o.BreakdownLimit)
	}
	for _, bad := range [][4]string{{"", "a\x00", "", ""}, {"", "a", "x", ""}, {"", "a", "", "0"}, {"", "a", "", "51"}, {"", "a", "", "z"}, {"{", "", "", ""}} {
		if _, err := analytics.ParseQueryOptions(bad[0], bad[1], bad[2], bad[3]); err == nil {
			t.Errorf("accepted %v", bad)
		}
	}
}
