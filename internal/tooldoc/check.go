package tooldoc

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// SummaryMax is the first-line discovery budget in runes.
const SummaryMax = 120

// Description joins a valid summary and details with a blank line.
func Description(summary, details string) (string, error) {
	s, err := trimmedSummary(summary)
	if err != nil {
		return "", err
	}
	if details = strings.TrimSpace(details); details == "" {
		return s, nil
	}
	return s + "\n\n" + details, nil
}

// Summary refuses a summary that muster's discovery would not show whole:
// empty, more than one line, or over [SummaryMax] runes.
func Summary(summary string) error {
	_, err := trimmedSummary(summary)
	return err
}

func trimmedSummary(summary string) (string, error) {
	s := strings.TrimSpace(summary)
	n := utf8.RuneCountInString(s)
	switch {
	case n == 0:
		return "", errors.New("tool summary is empty")
	case strings.Contains(s, "\n"):
		return "", fmt.Errorf("tool summary %q spans lines: discovery shows the first line only", s)
	case n > SummaryMax:
		return "", fmt.Errorf("tool summary %q is %d runes, over %d: discovery shows no more", s, n, SummaryMax)
	}
	return s, nil
}

// Budget is what [Check] holds descriptions to, in runes. A zero field is not
// checked.
type Budget struct {
	// Description caps a whole description, summary and details.
	Description int
	// Param caps every description in the input schema: each parameter's,
	// and those of nested properties and array items.
	Param int
	// Prefix is how many leading runes no two tools may share, compared
	// without case and leading space; a description shorter than Prefix is
	// compared whole. It catches a shared boilerplate opening that fills
	// most of the summary, not a shared first word or two.
	Prefix int
}

// DefaultBudget is the budget of giantswarm/giantswarm#38592.
var DefaultBudget = Budget{Description: 600, Param: 150, Prefix: 40}

// Check reports every description and schema budget violation in name order.
func Check(tools []Tool, b Budget) error {
	tools = sortedByName(tools)
	var errs []error
	prefixes := map[string]string{}
	for _, tool := range tools {
		d := tool.Description
		first, _, _ := strings.Cut(d, "\n")
		switch n := utf8.RuneCountInString(strings.TrimSpace(first)); {
		case n == 0:
			errs = append(errs, fmt.Errorf("%s: first line is empty: discovery shows no summary", tool.Name))
		case n > SummaryMax:
			errs = append(errs, fmt.Errorf("%s: first line is %d runes, over %d", tool.Name, n, SummaryMax))
		}
		if b.Description > 0 {
			if n := utf8.RuneCountInString(d); n > b.Description {
				errs = append(errs, fmt.Errorf("%s: description is %d runes, over %d", tool.Name, n, b.Description))
			}
		}
		if p := prefix(d, b.Prefix); p != "" {
			if other, ok := prefixes[p]; ok {
				errs = append(errs, fmt.Errorf("%s: starts like %s: %q", tool.Name, other, p))
			} else {
				prefixes[p] = tool.Name
			}
		}
		if b.Param > 0 {
			errs = append(errs, checkSchema(tool, b.Param)...)
		}
	}
	return errors.Join(errs...)
}

// prefix is the first n runes of d that two tools may not share; "" when n
// is 0 or d is empty.
func prefix(d string, n int) string {
	d = strings.ToLower(strings.TrimSpace(d))
	if n <= 0 || d == "" {
		return ""
	}
	for i := range d {
		if n == 0 {
			return d[:i]
		}
		n--
	}
	return d
}

// checkSchema bounds the descriptions in the complete input schema.
func checkSchema(tool Tool, limit int) []error {
	schema, err := inputSchema(tool)
	if err != nil {
		return []error{err}
	}
	return walkSchema(tool.Name, "", schema, limit)
}

// walkSchema checks the description of node and of everything below it.
// path names a parameter as tool.name, a nested property as tool.name.field
// and an array's items as tool.name[]; other keywords ($defs, anyOf, …)
// appear as themselves.
func walkSchema(tool, path string, node any, limit int) []error {
	var errs []error
	switch v := node.(type) {
	case map[string]any:
		if d, ok := v["description"].(string); ok {
			if n := utf8.RuneCountInString(d); n > limit {
				at := tool
				if path != "" {
					at += "." + path
				}
				errs = append(errs, fmt.Errorf("%s: description is %d runes, over %d", at, n, limit))
			}
		}
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			switch k {
			case "description":
			case "properties":
				props, _ := v[k].(map[string]any)
				names := make([]string, 0, len(props))
				for name := range props {
					names = append(names, name)
				}
				slices.Sort(names)
				for _, name := range names {
					errs = append(errs, walkSchema(tool, join(path, name), props[name], limit)...)
				}
			case "items":
				errs = append(errs, walkSchema(tool, path+"[]", v[k], limit)...)
			default:
				errs = append(errs, walkSchema(tool, join(path, k), v[k], limit)...)
			}
		}
	case []any:
		for i, e := range v {
			errs = append(errs, walkSchema(tool, fmt.Sprintf("%s[%d]", path, i), e, limit)...)
		}
	}
	return errs
}

func join(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}
