package certificate

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// ASCII rule for certificate text
//
// Everything printed on a certificate (and shown on its verification page)
// is plain printable ASCII: no accented letters, ligatures, fancy quotes or
// dashes, emoji, symbols or control characters. A viewer, printer or font
// can then never substitute a glyph or print a box. The rule, applied by
// SanitizeText and SanitizeName:
//
//  1. Typographic punctuation is mapped to its plain form: curly single
//     and double quotes become ' and ", every dash and minus becomes -,
//     an ellipsis becomes ..., and every Unicode space becomes one space.
//  2. A few letters with no decomposition are transliterated: ae, AE, oe,
//     OE, ss, o, O, d, D, l, L, th, TH, i.
//  3. The text is split into base letters plus combining marks (Unicode
//     NFKD, which also unpacks ligatures such as the fi ligature and
//     full-width forms) and the marks are dropped, so "Jose with an acute
//     accent" becomes "Jose".
//  4. Anything still outside printable ASCII (letters of other scripts,
//     emoji, symbols, control characters) is removed.
//  5. Runs of whitespace collapse to one space and the ends are trimmed.
//
// SanitizeName is stricter: it keeps only letters, spaces, full stops,
// apostrophes and hyphens, drops leading/trailing punctuation, and limits
// the result to MaxNameLen characters (cut at a word boundary). A name with
// no Latin letters at all comes out empty; RecipientName then falls back to
// the part of the email before the @, and finally to "Learner".

// MaxNameLen is the longest recipient name printed on a certificate.
const MaxNameLen = 60

var asciiMap = strings.NewReplacer(
	"‘", "'", "’", "'", "‚", "'", "‛", "'", "′", "'", "´", "'", "`", "'",
	"“", "\"", "”", "\"", "„", "\"", "‟", "\"", "″", "\"",
	"‐", "-", "‑", "-", "‒", "-", "–", "-", "—", "-", "―", "-", "−", "-",
	"…", "...",
	"æ", "ae", "Æ", "AE", "œ", "oe", "Œ", "OE", "ß", "ss",
	"ø", "o", "Ø", "O", "đ", "d", "Đ", "D", "ł", "l", "Ł", "L",
	"þ", "th", "Þ", "TH", "ð", "d", "Ð", "D", "ı", "i",
	"×", "x",
)

// SanitizeText applies the ASCII rule above to free text such as a course
// title. The result contains only printable ASCII (0x20 to 0x7E).
func SanitizeText(s string) string {
	s = asciiMap.Replace(s)
	var b strings.Builder
	for _, r := range norm.NFKD.String(s) {
		switch {
		case unicode.IsSpace(r):
			b.WriteByte(' ')
		case r >= 0x20 && r <= 0x7E:
			b.WriteRune(r)
		}
		// everything else (marks, other scripts, emoji, controls) is dropped
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// SanitizeName applies the stricter name rule: letters, spaces, '.', '\” and
// '-' only, at most MaxNameLen characters. It may return "".
func SanitizeName(s string) string {
	s = SanitizeText(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == ' ', r == '.', r == '\'', r == '-':
			b.WriteRune(r)
		}
	}
	name := strings.Join(strings.Fields(b.String()), " ")
	name = strings.Trim(name, " .'-")
	if len(name) > MaxNameLen {
		name = name[:MaxNameLen]
		if i := strings.LastIndex(name, " "); i > 0 {
			name = name[:i]
		}
		name = strings.Trim(name, " .'-")
	}
	return name
}

// RecipientName is the name printed on a certificate: the sanitised account
// name, else the sanitised local part of the email (dots, underscores and
// plus signs read as spaces), else "Learner". Always plain ASCII, never "".
func RecipientName(name, email string) string {
	if n := SanitizeName(name); len(n) >= 2 {
		return n
	}
	local := email
	if i := strings.Index(local, "@"); i >= 0 {
		local = local[:i]
	}
	local = strings.NewReplacer(".", " ", "_", " ", "+", " ").Replace(local)
	if n := SanitizeName(local); len(n) >= 2 {
		return n
	}
	return "Learner"
}

// NormalizeCode turns a typed or displayed certificate code into the stored
// form (upper-case hex, no separators). It returns "" for anything that is
// not a 16-character hex code.
func NormalizeCode(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		switch {
		case r >= '0' && r <= '9', r >= 'A' && r <= 'F':
			b.WriteRune(r)
		case r == '-', r == ' ':
		default:
			return ""
		}
	}
	if b.Len() != 16 {
		return ""
	}
	return b.String()
}

// FormatCode groups a stored code as XXXX-XXXX-XXXX-XXXX for display.
func FormatCode(code string) string {
	if len(code) != 16 {
		return code
	}
	return code[0:4] + "-" + code[4:8] + "-" + code[8:12] + "-" + code[12:16]
}
