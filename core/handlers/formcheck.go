package handlers

import (
	"strings"
	"unicode"
)

// Field rules for the public forms (contact, booking, newsletter), which send
// an email to whatever address is typed. They keep scam text in other scripts
// and free-form pitches out of the name box and out of the stored message.

// validPersonName accepts a first name, first and last name, or first, middle
// and last name: one to three words of English letters, with an optional
// apostrophe, hyphen or dot inside a word ("D'Souza", "Anne-Marie", "A. K.").
// A single word needs at least two letters.
func validPersonName(name string) bool {
	n := strings.TrimSpace(name)
	if n == "" || len(n) > 60 {
		return false
	}
	words := strings.Fields(n)
	if len(words) < 1 || len(words) > 3 || strings.Join(words, " ") != n {
		return false
	}
	letters := 0
	for _, w := range words {
		if len(w) > 30 || !isASCIILetter(rune(w[0])) {
			return false
		}
		for _, r := range w {
			switch {
			case isASCIILetter(r):
				letters++
			case r == '\'' || r == '-' || r == '.':
			default:
				return false
			}
		}
	}
	return letters >= 2 || (len(words) > 1 && letters >= 1)
}

func isASCIILetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// englishText accepts text whose letters are all in the Latin script (accents
// are fine); digits, punctuation, line breaks and ordinary symbols pass.
// Letters from any other script, and control characters, do not.
func englishText(s string) bool {
	for _, r := range s {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
		case unicode.IsControl(r):
			return false
		case unicode.IsLetter(r) && !unicode.Is(unicode.Latin, r):
			return false
		}
	}
	return true
}

// asciiOnly reports whether every byte of s is ASCII.
func asciiOnly(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// validPhone accepts an optional phone number: digits, spaces and + - ( ) .
func validPhone(p string) bool {
	if len(p) > 30 {
		return false
	}
	for _, r := range p {
		if !(r >= '0' && r <= '9') && !strings.ContainsRune(" +-().", r) {
			return false
		}
	}
	return true
}

// fieldKind names the rule a visitor-supplied field must follow. Every public
// form (anything a visitor, not the admin, can type into) declares its fields
// with checkField, so a new field gets the same English-only, real-name rules
// by choosing a kind instead of writing its own checks.
type fieldKind int

const (
	fieldName  fieldKind = iota // 1-3 words of English letters (validPersonName)
	fieldLine                   // short single-line English text (titles, topics)
	fieldText                   // multi-line English text (messages, comments)
	fieldPhone                  // digits and + - ( ) .
	fieldEmail                  // ASCII e-mail address
)

// fieldSpec describes one field: Label is used in the message ("Your name"),
// Max is the rune limit for line/text kinds (0 = a default for the kind).
type fieldSpec struct {
	Label    string
	Kind     fieldKind
	Max      int
	Required bool
}

// checkField returns "" when value is acceptable, else a message for the visitor.
func checkField(spec fieldSpec, value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		if spec.Required {
			return spec.Label + " is required."
		}
		return ""
	}
	max := spec.Max
	switch spec.Kind {
	case fieldName:
		if !validPersonName(v) {
			return spec.Label + " must be a real name in English letters: first name, first and last name, or first, middle and last name."
		}
	case fieldPhone:
		if !validPhone(v) {
			return spec.Label + " may contain only digits and + - ( ) ."
		}
	case fieldEmail:
		if !validEmail(strings.ToLower(v)) {
			return "Enter a valid email address, using English letters and digits only."
		}
	case fieldLine, fieldText:
		if max == 0 {
			max = 200
			if spec.Kind == fieldText {
				max = 5000
			}
		}
		if len([]rune(v)) > max {
			return spec.Label + " is too long."
		}
		if !englishText(v) || (spec.Kind == fieldLine && strings.ContainsAny(v, "\r\n")) {
			return spec.Label + " must be written in English."
		}
	}
	return ""
}

// firstFieldError runs checkField over (spec, value) pairs and returns the
// first message, or "".
func firstFieldError(pairs ...any) string {
	for i := 0; i+1 < len(pairs); i += 2 {
		if msg := checkField(pairs[i].(fieldSpec), pairs[i+1].(string)); msg != "" {
			return msg
		}
	}
	return ""
}
