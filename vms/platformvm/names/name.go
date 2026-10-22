// Package names holds the rules of the native naming registry that every
// component must apply identically: the node when verifying name
// transactions and answering resolution requests, the wallet when building
// them, and the transaction service when resolving payment recipients.
package names

import (
	"errors"
	"fmt"
	"strings"
)

const (
	MinLength = 3
	MaxLength = 32
)

var (
	ErrInvalidName = errors.New("invalid name")

	// reserved names can never be registered. They would let a registrant
	// impersonate the network or have an ambiguous meaning in user
	// interfaces.
	reserved = map[string]struct{}{
		"dijets": {}, "djt": {}, "djtx": {}, "admin": {}, "administrator": {},
		"root": {}, "system": {}, "validator": {}, "validators": {},
		"treasury": {}, "foundation": {}, "official": {}, "support": {},
		"security": {}, "wallet": {}, "explorer": {}, "bridge": {}, "null": {},
		"undefined": {}, "none": {}, "unknown": {}, "anonymous": {},
		"everyone": {}, "self": {}, "me": {}, "names": {}, "registry": {},
	}
)

// Rule names a violated naming rule. It is reported to API clients as the
// suffix of the "invalid_name:<rule>" error reason.
type Rule string

const (
	RuleEmpty          Rule = "empty"
	RuleNonASCII       Rule = "non_ascii"
	RuleTooShort       Rule = "too_short"
	RuleTooLong        Rule = "too_long"
	RuleCharacter      Rule = "character"
	RuleFirstCharacter Rule = "first_character"
	RuleLastCharacter  Rule = "last_character"
	RuleDoubleHyphen   Rule = "double_hyphen"
	RuleChainPrefix    Rule = "chain_prefix"
	RuleHierarchical   Rule = "hierarchical"
	RuleReserved       Rule = "reserved"
	RuleNotCanonical   Rule = "not_canonical"
)

// Error reports why a name was rejected.
type Error struct {
	Name string
	Rule Rule
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s %q: %s", ErrInvalidName, e.Name, e.Rule)
}

func (*Error) Unwrap() error {
	return ErrInvalidName
}

func invalid(name string, rule Rule) error {
	return &Error{Name: name, Rule: rule}
}

// Canonicalise converts user input into the canonical form of a name and
// validates it. It trims surrounding whitespace, strips one leading "@" and
// lowercases ASCII letters. Non-ASCII input is rejected, never
// transliterated: v1 names are ASCII only so that visually confusable names
// cannot exist.
func Canonicalise(input string) (string, error) {
	name := strings.TrimSpace(input)
	name = strings.TrimPrefix(name, "@")
	for i := 0; i < len(name); i++ {
		if name[i] >= 0x80 {
			return "", invalid(input, RuleNonASCII)
		}
	}
	name = strings.ToLower(name)
	if err := Validate(name); err != nil {
		return "", err
	}
	return name, nil
}

// Validate checks that [name] is a canonical, registrable name. On-chain
// transactions must carry names that pass Validate unchanged, so that every
// name has exactly one encoding.
func Validate(name string) error {
	switch {
	case len(name) == 0:
		return invalid(name, RuleEmpty)
	case len(name) < MinLength:
		return invalid(name, RuleTooShort)
	case len(name) > MaxLength:
		return invalid(name, RuleTooLong)
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 0x80:
			return invalid(name, RuleNonASCII)
		case c == '.':
			return invalid(name, RuleHierarchical)
		case c >= 'A' && c <= 'Z':
			return invalid(name, RuleNotCanonical)
		case !isLower(c) && !isDigit(c) && c != '-':
			return invalid(name, RuleCharacter)
		}
	}
	switch last := name[len(name)-1]; {
	case !isLower(name[0]):
		return invalid(name, RuleFirstCharacter)
	case !isLower(last) && !isDigit(last):
		return invalid(name, RuleLastCharacter)
	case name[1] == '-':
		// "v-…", "m-…" and "u-…" could be mistaken for chain-prefixed
		// addresses such as "V-local1…".
		return invalid(name, RuleChainPrefix)
	case strings.Contains(name, "--"):
		// Reserved for a future encoding of Unicode names.
		return invalid(name, RuleDoubleHyphen)
	}
	if _, ok := reserved[name]; ok {
		return invalid(name, RuleReserved)
	}
	return nil
}

// IsReserved reports whether [name] is on the reserved list.
func IsReserved(name string) bool {
	_, ok := reserved[name]
	return ok
}

func isLower(c byte) bool { return c >= 'a' && c <= 'z' }

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
