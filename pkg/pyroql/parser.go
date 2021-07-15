package pyroql

import (
	"regexp"
	"strings"
	"unicode"
)

type Query struct {
	AppName  string
	Matchers []*TagMatcher

	q string // The original query string.
}

func (q *Query) String() string { return q.q }

type TagMatcher struct {
	Key   string
	Value string
	Op

	R *regexp.Regexp
}

type Op int

const (
	_         Op = iota
	EQL          // =
	NEQ          // !=
	EQL_REGEX    // =~
	NEQ_REGEX    // !~
)

func IsKeyRuneAllowed(r rune) bool {
	return unicode.IsDigit(r) || unicode.IsLetter(r) ||
		r == '-' || r == '_' || r == '.' || r == '/'
}

func IsKeyReserved(s string) bool {
	switch s {
	case "__name__":
		return true
	}
	return false
}

// ParseQuery parses a string of $app_name<{<$tag_matchers>}> form.
func ParseQuery(s string) (*Query, error) {
	s = strings.TrimSpace(s)
	q := Query{q: s}

	for offset, c := range s {
		switch c {
		case '{':
			if offset == 0 {
				return nil, ErrAppNameIsRequired
			}
			if s[len(s)-1] != '}' {
				return nil, newErr(ErrInvalidQuerySyntax, "expected } at the end")
			}
			m, err := ParseMatchers(s[offset+1 : len(s)-1])
			if err != nil {
				return nil, err
			}
			q.AppName = s[:offset]
			q.Matchers = m
			return &q, nil
		default:
			if !IsKeyRuneAllowed(c) {
				return nil, newErr(ErrInvalidAppName, s[:offset+1])
			}
		}
	}

	if len(s) == 0 {
		return nil, ErrAppNameIsRequired
	}

	q.AppName = s
	return &q, nil
}

// ParseMatchers parses a string of $tag_matcher<,$tag_matchers> form.
func ParseMatchers(s string) ([]*TagMatcher, error) {
	var matchers []*TagMatcher
	// TODO: allow escaped ',' in value?
	for _, t := range strings.Split(s, ",") {
		if t == "" {
			continue
		}
		m, err := ParseMatcher(strings.TrimSpace(t))
		if err != nil {
			return nil, err
		}
		matchers = append(matchers, m)
	}
	if len(matchers) == 0 && len(s) != 0 {
		return nil, newErr(ErrInvalidMatchersSyntax, s)
	}
	return matchers, nil
}

// ParseMatcher parses a string of $tag_key$op"$tag_value" form,
// where $op is one of the supported match operators.
func ParseMatcher(s string) (*TagMatcher, error) {
	var tm TagMatcher
	var offset int
	var c rune

loop:
	for offset, c = range s {
		r := len(s) - (offset + 1)
		switch c {
		case '=':
			switch {
			case r <= 2:
				return nil, newErr(ErrInvalidValueSyntax, s)
			case s[offset+1] == '"':
				tm.Op = EQL
			case s[offset+1] == '~':
				if r <= 3 {
					return nil, newErr(ErrInvalidValueSyntax, s)
				}
				tm.Op = EQL_REGEX
			default:
				// Just for more meaningful error message.
				if s[offset+2] != '"' {
					return nil, newErr(ErrInvalidValueSyntax, s)
				}
				return nil, newErr(ErrUnknownOp, s)
			}
			break loop
		case '!':
			if r <= 3 {
				return nil, newErr(ErrInvalidValueSyntax, s)
			}
			switch s[offset+1] {
			case '=':
				tm.Op = NEQ
			case '~':
				tm.Op = NEQ_REGEX
			default:
				return nil, newErr(ErrUnknownOp, s)
			}
			break loop
		default:
			if !IsKeyRuneAllowed(c) {
				return nil, newErr(ErrInvalidKey, s)
			}
		}
	}

	k := s[:offset]
	if IsKeyReserved(k) {
		return nil, newErr(ErrKeyReserved, k)
	}

	var v string
	var ok bool
	switch tm.Op {
	default:
		return nil, newErr(ErrMatchOperatorIsRequired, s)
	case EQL:
		v, ok = unquote(s[offset+1:])
	case NEQ, EQL_REGEX, NEQ_REGEX:
		v, ok = unquote(s[offset+2:])
	}
	if !ok {
		return nil, newErr(ErrInvalidValueSyntax, v)
	}

	// Compile regex, it applicable.
	switch tm.Op {
	case EQL_REGEX, NEQ_REGEX:
		r, err := regexp.Compile(v)
		if err != nil {
			return nil, newErr(err, v)
		}
		tm.R = r
	}

	tm.Key = k
	tm.Value = v
	return &tm, nil
}

func unquote(s string) (string, bool) {
	if s[0] != '"' || s[len(s)-1] != '"' {
		return s, false
	}
	return s[1 : len(s)-1], true
}
