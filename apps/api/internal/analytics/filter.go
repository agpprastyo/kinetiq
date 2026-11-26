package analytics

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxFilterDepth      = 3
	maxFilterConditions = 20
	maxFilterInValues   = 100
	maxPropertyNameLen  = 128
)

var aliasRe = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// Filter is the shared filter DSL: either a group (AND/OR of filters) or a leaf condition.
// Every property name and value reaches SQL as a bind parameter, never as text.
type Filter struct {
	Group    string          `json:"group,omitempty"`
	Filters  []*Filter       `json:"filters,omitempty"`
	Property string          `json:"property,omitempty"`
	Op       string          `json:"op,omitempty"`
	Value    json.RawMessage `json:"value,omitempty"`
	Scope    string          `json:"scope,omitempty"` // "event" (default) or "person"
}

// ParseFilter decodes and validates a JSON filter. Empty input means no filter.
func ParseFilter(raw string) (*Filter, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var f Filter
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("invalid filter JSON: %w", err)
	}
	if err := f.validate(1, new(int)); err != nil {
		return nil, err
	}
	return &f, nil
}

func ValidatePropertyName(name string) error {
	if name == "" || len(name) > maxPropertyNameLen || !utf8.ValidString(name) {
		return errors.New("property name must be 1-128 valid UTF-8 bytes")
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return errors.New("property name contains control characters")
		}
	}
	return nil
}

func (f *Filter) isGroup() bool { return f.Group != "" }

func (f *Filter) validate(depth int, count *int) error {
	if f.isGroup() {
		if f.Group != "and" && f.Group != "or" {
			return errors.New("group must be 'and' or 'or'")
		}
		if f.Property != "" || f.Op != "" || len(f.Value) > 0 || f.Scope != "" {
			return errors.New("group cannot carry property/op/value/scope")
		}
		if depth > maxFilterDepth {
			return errors.New("filter nesting too deep")
		}
		if len(f.Filters) == 0 {
			return errors.New("group must contain at least one filter")
		}
		for _, c := range f.Filters {
			if c == nil {
				return errors.New("null filter in group")
			}
			if err := c.validate(depth+1, count); err != nil {
				return err
			}
		}
		return nil
	}
	if len(f.Filters) > 0 {
		return errors.New("filters requires 'group'")
	}
	*count++
	if *count > maxFilterConditions {
		return errors.New("too many filter conditions")
	}
	if err := ValidatePropertyName(f.Property); err != nil {
		return err
	}
	if f.Scope != "" && f.Scope != "event" && f.Scope != "person" {
		return errors.New("scope must be 'event' or 'person'")
	}
	_, err := f.leafValue()
	return err
}

// leafValue validates op/value and returns the bind argument (nil for is_set/is_not_set).
func (f *Filter) leafValue() (any, error) {
	switch f.Op {
	case "is_set", "is_not_set":
		if len(f.Value) > 0 {
			return nil, fmt.Errorf("op '%s' takes no value", f.Op)
		}
		return nil, nil
	case "eq", "neq":
		return scalarText(f.Value)
	case "contains":
		var s string
		if err := json.Unmarshal(f.Value, &s); err != nil {
			return nil, errors.New("op 'contains' requires a string value")
		}
		return s, nil
	case "gt", "lt":
		var n float64
		if len(f.Value) == 0 || json.Unmarshal(f.Value, &n) != nil {
			return nil, fmt.Errorf("op '%s' requires a number value", f.Op)
		}
		return n, nil
	case "in":
		var items []json.RawMessage
		if len(f.Value) == 0 || json.Unmarshal(f.Value, &items) != nil || len(items) == 0 || len(items) > maxFilterInValues {
			return nil, errors.New("op 'in' requires a non-empty array (max 100)")
		}
		out := make([]string, len(items))
		for i, it := range items {
			s, err := scalarText(it)
			if err != nil {
				return nil, err
			}
			out[i] = s
		}
		return out, nil
	}
	return nil, fmt.Errorf("unsupported op %q", f.Op)
}

func scalarText(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", errors.New("value is required")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return "", errors.New("invalid value")
	}
	switch t := v.(type) {
	case string:
		return t, nil
	case bool:
		return strconv.FormatBool(t), nil
	case json.Number:
		f, err := t.Float64()
		if err != nil {
			return "", errors.New("invalid number")
		}
		return strconv.FormatFloat(f, 'f', -1, 64), nil
	}
	return "", errors.New("value must be a string, number or boolean")
}

// Compile renders a parenthesised SQL boolean expression over `alias` (an events or
// resolved_events alias). Placeholders start at $(argOffset+1).
func (f *Filter) Compile(alias string, argOffset int) (string, []any, error) {
	if !aliasRe.MatchString(alias) {
		return "", nil, errors.New("invalid alias")
	}
	if err := f.validate(1, new(int)); err != nil {
		return "", nil, err
	}
	var args []any
	sql := f.compile(alias, argOffset, &args)
	return sql, args, nil
}

func (f *Filter) compile(alias string, off int, args *[]any) string {
	bind := func(v any) string {
		*args = append(*args, v)
		return fmt.Sprintf("$%d", off+len(*args))
	}
	if f.isGroup() {
		parts := make([]string, len(f.Filters))
		for i, c := range f.Filters {
			parts[i] = c.compile(alias, off, args)
		}
		return "(" + strings.Join(parts, " "+strings.ToUpper(f.Group)+" ") + ")"
	}

	props := scopedProps(alias, f.Scope)
	key := bind(f.Property) + "::text"
	val, _ := f.leafValue()
	text := fmt.Sprintf("(%s ->> %s)", props, key)
	switch f.Op {
	case "is_set":
		return fmt.Sprintf("(COALESCE(jsonb_typeof(%s -> %s), 'null') <> 'null')", props, key)
	case "is_not_set":
		return fmt.Sprintf("(COALESCE(jsonb_typeof(%s -> %s), 'null') = 'null')", props, key)
	case "eq":
		return fmt.Sprintf("(%s = %s::text)", text, bind(val))
	case "neq":
		return fmt.Sprintf("(%s IS DISTINCT FROM %s::text)", text, bind(val))
	case "contains":
		return fmt.Sprintf("(strpos(lower(COALESCE(%s, '')), lower(%s::text)) > 0)", text, bind(val))
	case "in":
		return fmt.Sprintf("(%s = ANY(%s::text[]))", text, bind(val))
	default: // gt, lt
		op := map[string]string{"gt": ">", "lt": "<"}[f.Op]
		return fmt.Sprintf("(CASE WHEN jsonb_typeof(%s -> %s) = 'number' THEN (%s)::numeric %s %s::numeric ELSE false END)", props, key, text, op, bind(val))
	}
}

// scopedProps returns the jsonb expression holding event or person properties for a row.
func scopedProps(alias, scope string) string {
	if scope == "person" {
		return fmt.Sprintf(`(SELECT p.properties FROM persons p JOIN person_distinct_ids m ON m.person_id = p.id WHERE m.project_id = %[1]s.project_id AND m.distinct_id = %[1]s.distinct_id)`, alias)
	}
	return alias + ".properties"
}

// BreakdownItem is one bucket of a breakdown; the remainder bucket has Value "other".
type BreakdownItem = PropertyBreakdownItem

// InferPropertyType collapses observed jsonb_typeof values into a stable type name.
func InferPropertyType(observed []string) string {
	seen := map[string]bool{}
	for _, t := range observed {
		if t != "null" && t != "" {
			seen[t] = true
		}
	}
	switch len(seen) {
	case 0:
		return "unknown"
	case 1:
		for t := range seen {
			return t
		}
	}
	return "mixed"
}
