package certificate

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7E {
			return false
		}
	}
	return true
}

func TestSanitizeText(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Plain Title 101", "Plain Title 101"},
		{"Java — Interview – Prep", "Java - Interview - Prep"},
		{"It’s “great”", "It's \"great\""},
		{"Wait…", "Wait..."},
		{"Café naïve résumé", "Cafe naive resume"},
		{"oﬃce ﬁnal", "office final"}, // ligatures
		{"Straße Æon œuvre", "Strasse AEon oeuvre"},
		{"Fast \U0001F680 course ™", "Fast course TM"}, // emoji dropped, trademark sign unpacked by NFKD
		{"tab\tand\nnewline nbsp", "tab and newline nbsp"},
		{"नमस्ते", ""}, // non-Latin script
		{"  spaced   out  ", "spaced out"},
		{"ctrl\x00\x07chars", "ctrlchars"},
	}
	for _, c := range cases {
		got := SanitizeText(c.in)
		if got != c.want {
			t.Errorf("SanitizeText(%q)=%q want %q", c.in, got, c.want)
		}
		if !isASCII(got) {
			t.Errorf("SanitizeText(%q) not ASCII: %q", c.in, got)
		}
	}
}

func TestSanitizeName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Vishnu Balachandran", "Vishnu Balachandran"},
		{"José García", "Jose Garcia"},
		{"O’Brien-Smith Jr.", "O'Brien-Smith Jr"},
		{"Anne-Marie  D’Souza", "Anne-Marie D'Souza"},
		{"  ... Neo 2000 ...  ", "Neo"}, // digits dropped, punctuation trimmed
		{"Bad<script>Name", "BadscriptName"},
		{"नमस्ते", ""},
		{"Emoji \U0001F600 Person", "Emoji Person"},
		{"", ""},
	}
	for _, c := range cases {
		got := SanitizeName(c.in)
		if got != c.want {
			t.Errorf("SanitizeName(%q)=%q want %q", c.in, got, c.want)
		}
		if !isASCII(got) {
			t.Errorf("SanitizeName(%q) not ASCII: %q", c.in, got)
		}
	}
	long := strings.Repeat("Abcdefgh ", 20)
	if n := SanitizeName(long); len(n) > MaxNameLen || strings.HasSuffix(n, " ") {
		t.Errorf("long name not cut cleanly: %d %q", len(n), n)
	}
}

func TestRecipientNameFallbacks(t *testing.T) {
	if got := RecipientName("नम", "jane.doe+x@example.com"); got != "jane doe x" {
		t.Errorf("email fallback: %q", got)
	}
	if got := RecipientName("", "12345@example.com"); got != "Learner" {
		t.Errorf("final fallback: %q", got)
	}
	if got := RecipientName("Real Name", "x@y.z"); got != "Real Name" {
		t.Errorf("name wins: %q", got)
	}
}

func TestCodeHelpers(t *testing.T) {
	if got := NormalizeCode("abcd-ef01-2345-6789"); got != "ABCDEF0123456789" {
		t.Errorf("NormalizeCode: %q", got)
	}
	for _, bad := range []string{"", "ABCDEF012345678", "ABCDEF01234567890", "ZZZZZZZZZZZZZZZZ", "ABCD EF01 2345 678!"} {
		if got := NormalizeCode(bad); got != "" {
			t.Errorf("NormalizeCode(%q)=%q want empty", bad, got)
		}
	}
	if got := FormatCode("ABCDEF0123456789"); got != "ABCD-EF01-2345-6789" {
		t.Errorf("FormatCode: %q", got)
	}
}

func TestGenerateSmoke(t *testing.T) {
	compress = false
	defer func() { compress = true }()

	pdf, err := Generate(Data{
		RecipientName: "José O’Brien \U0001F600",
		CourseTitle:   "Java Interview Preparation — Core & Collections ™",
		IssuedAt:      time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC),
		Code:          "abcdef0123456789",
		SiteName:      "Vishnu Balachandran",
		IssuerName:    "Vishnu Balachandran",
		ScorePercent:  85,
		VerifyURL:     "https://example.com/certificate/verify/ABCDEF0123456789",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatal("not a PDF")
	}
	if len(pdf) < 1500 {
		t.Fatalf("suspiciously small PDF: %d bytes", len(pdf))
	}
	for _, want := range []string{
		"CERTIFICATE", "OF ACHIEVEMENT", "Jose O'Brien", "has cleared the final quiz and is certified in",
		"Java Interview Preparation - Core & Collections",
		"Final quiz score 85 percent", "05 March 2026", "ABCD-EF01-2345-6789", "Verify this certificate at",
	} {
		// fpdf escapes ( ) \ in strings; none of the expected text has them.
		if !bytes.Contains(pdf, []byte(want)) {
			t.Errorf("PDF is missing %q", want)
		}
	}
	// A landscape A4 page: 842 x 595 points.
	if !bytes.Contains(pdf, []byte("842")) {
		t.Error("expected A4 landscape media box")
	}
}

func TestPrepareIsASCII(t *testing.T) {
	d := Prepare(Data{
		RecipientName: "Émile “Z”",
		CourseTitle:   "Café — \U0001F680",
		SiteName:      "Sïte",
		IssuerName:    "Øyvind",
		Code:          "abcd-ef01-2345-6789",
	})
	for _, s := range []string{d.RecipientName, d.CourseTitle, d.SiteName, d.IssuerName, d.Code} {
		if !isASCII(s) {
			t.Errorf("not ASCII after Prepare: %q", s)
		}
	}
	if d.Code != "ABCDEF0123456789" || d.PassPercent != 70 {
		t.Errorf("defaults wrong: %+v", d)
	}
}
