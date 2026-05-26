package policy

import (
	"regexp"
	"strings"
)

// TokenizeShell splits a command line into shell-style tokens, honoring single
// and double quotes and backslash escapes. It does not expand variables or
// globs — it only produces the argv a shell would.
func TokenizeShell(s string) []string {
	var out []string
	var cur strings.Builder
	inSingle, inDouble, escaped := false, false, false
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case r == '\\' && !inSingle:
			escaped = true
		case r == '\'' && !inDouble:
			inSingle = !inSingle
		case r == '"' && !inSingle:
			inDouble = !inDouble
		case !inSingle && !inDouble && (r == ' ' || r == '\t'):
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

// SplitCompound splits a command string at shell separators (&&, ||, ;, |, and
// newlines). Separators inside quoted regions are preserved. Each returned
// segment is trimmed and re-joined with single spaces so downstream matchers
// see canonical whitespace.
func SplitCompound(s string) []string {
	var segments []string
	var cur strings.Builder
	inSingle, inDouble, escaped := false, false, false
	push := func() {
		seg := strings.TrimSpace(cur.String())
		if seg != "" {
			segments = append(segments, collapseSpaces(seg))
		}
		cur.Reset()
	}
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
			continue
		case r == '\\' && !inSingle:
			escaped = true
			continue
		case r == '\'' && !inDouble:
			inSingle = !inSingle
		case r == '"' && !inSingle:
			inDouble = !inDouble
		}
		if !inSingle && !inDouble {
			if r == '\n' || r == ';' {
				push()
				continue
			}
			if i+1 < len(runes) {
				pair := string(r) + string(runes[i+1])
				if pair == "&&" || pair == "||" {
					push()
					i++
					continue
				}
			}
			if r == '|' {
				push()
				continue
			}
		}
		cur.WriteRune(r)
	}
	push()
	return segments
}

// GlobToRegexp converts a shell-style glob (supporting * and ?) into an
// anchored regexp. Other regexp metacharacters are quoted. Returns nil if the
// pattern fails to compile.
func GlobToRegexp(glob string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for _, r := range glob {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil
	}
	return re
}

func collapseSpaces(s string) string {
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		b.WriteRune(r)
		prevSpace = false
	}
	return b.String()
}
