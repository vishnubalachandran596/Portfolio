package storage

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// NormalizedMarker is written into every file NormalizeLesson has
// processed, so a second pass is a cheap no-op and an agent can tell a
// conforming file at a glance.
const NormalizedMarker = "<!-- vb:normalized v1 -->"

// lessonTokenMap maps the custom properties uploaded lessons were
// written with (each course shipped its own palette — IBM Plex, amber,
// violet) onto the site design tokens in docs/DESIGN.md. A lesson keeps
// its components (concept cards, labs, walkthroughs) but takes the
// site's colours, type and dark mode, because the lesson iframe links
// the site stylesheet first. Unknown names keep their original light
// value under the renamed property.
var lessonTokenMap = map[string]string{
	"bg":             "var(--bg)",
	"bg2":            "var(--bg-subtle)",
	"fg":             "var(--ink)",
	"ink":            "var(--ink)",
	"card":           "var(--surface)",
	"panel":          "var(--surface)",
	"panel2":         "var(--surface-sunken)",
	"border":         "var(--line)",
	"line":           "var(--line)",
	"muted":          "var(--ink-2)",
	"primary":        "var(--accent)",
	"primary-fg":     "var(--on-accent)",
	"accent":         "var(--accent-strong)",
	"accent-fg":      "var(--on-accent)",
	"navy":           "var(--ink)",
	"teal":           "var(--accent-strong)",
	"teal2":          "var(--accent)",
	"ok":             "var(--success)",
	"good":           "var(--success)",
	"bad":            "var(--danger)",
	"danger":         "var(--danger)",
	"warn":           "var(--accent)",
	"font":           "var(--font-text)",
	"display":        "var(--font-display)",
	"mono":           "var(--font-mono)",
	"radius":         "var(--radius-md)",
	"shadow":         "var(--shadow-sm)",
	"code-bg":        "var(--surface-sunken)",
	"code-fg":        "var(--ink)",
	"code-border":    "var(--line)",
	"code-inline-bg": "var(--surface-sunken)",
}

// DarkThemeDirective opts a lesson built as a dark "app" (hard-coded
// dark panels, light text) into the site's dark palette in both colour
// schemes. Without it, mapping onto the light tokens would put dark text
// on its dark panels. Put the comment anywhere in the file.
const DarkThemeDirective = "vb:theme dark"

// lessonDarkMap is lessonTokenMap resolved against the site's dark
// tokens (style.css, [data-theme="dark"]) as literal values, so a
// dark-app lesson looks the same whatever scheme the reader uses.
// siteDarkTokens mirrors style.css's [data-theme="dark"] block. Keep the
// two in step when the palette changes (docs/DESIGN.md).
const siteDarkTokens = "color-scheme: dark;--bg: #14110e;--bg-subtle: #1a1612;--surface: #201b17;--surface-sunken: #17130f;--ink: #f5f0ea;--ink-2: #cbbfb2;--ink-3: #a39585;--line: #2e2721;--line-strong: #453b33;--accent: #e0b04a;--accent-strong: #f0c76a;--accent-soft: #2a2114;--on-accent: #14110e;--band: #0f0d0b;--success: #4ade80;--success-soft: #0f2a1a;--danger: #f87171;--danger-soft: #2c1414;--header-glass: rgba(20, 17, 14, 0.8);--focus-ring: 0 0 0 0.1875rem rgba(224, 176, 74, 0.4);--shadow-sm: 0 1px 2px rgba(0, 0, 0, 0.4);--shadow-md: 0 0.75rem 2rem -0.75rem rgba(0, 0, 0, 0.7);--shadow-lg: 0 1.5rem 3.5rem -1rem rgba(0, 0, 0, 0.8)"

var lessonDarkMap = map[string]string{
	"bg": "#14110e", "bg2": "#1a1612", "fg": "#f5f0ea", "ink": "#f5f0ea",
	"card": "#201b17", "panel": "#201b17", "panel2": "#17130f",
	"border": "#2e2721", "line": "#2e2721", "muted": "#cbbfb2",
	"primary": "#e0b04a", "primary-fg": "#14110e", "accent": "#f0c76a", "accent-fg": "#14110e",
	"navy": "#2e2721", "teal": "#e0b04a", "teal2": "#f0c76a",
	"ok": "#4ade80", "good": "#4ade80", "bad": "#f87171", "danger": "#f87171", "warn": "#e0b04a",
	"code-bg": "#17130f", "code-fg": "#f5f0ea", "code-border": "#2e2721", "code-inline-bg": "#17130f",
	"shadow": "0 0.75rem 2rem -0.75rem rgba(0, 0, 0, 0.7)",
}

var (
	styleBlock     = regexp.MustCompile(`(?is)(<style\b[^>]*>)(.*?)(</style>)`)
	fontImport     = regexp.MustCompile(`(?i)@import\s+url\(\s*['"]?https?://fonts\.(?:googleapis|gstatic)\.com[^)]*\)\s*;?`)
	rootBlock      = regexp.MustCompile(`(?s):root\s*\{([^{}]*)\}`)
	darkMediaBlock = regexp.MustCompile(`(?s)@media\s*\(\s*prefers-color-scheme\s*:\s*dark\s*\)\s*\{\s*(?::root\s*\{[^{}]*\}\s*)+\}`)
	customPropDecl = regexp.MustCompile(`(--[A-Za-z0-9_-]+)\s*:\s*([^;]+);?`)
	secNumSpan     = regexp.MustCompile(`(?is)<span\b[^>]*class=["'][^"']*\bsec-num\b[^"']*["'][^>]*>.*?</span>\s*`)
	h2Block        = regexp.MustCompile(`(?is)(<h2\b[^>]*>)(.*?)(</h2>)`)
	leadingNumber  = regexp.MustCompile(`^(\s*)(?:(?:part|step|section|module|chapter)\s+)?\d+(?:\.\d+)*\s*[.):—–-]\s+`)
	preBlock       = regexp.MustCompile(`(?is)<pre\b([^>]*)>(.*?)</pre>`)
	codeOpen       = regexp.MustCompile(`(?is)^\s*<code\b([^>]*)>`)
	langClass      = regexp.MustCompile(`(?i)\b(?:language|lang)-[a-z0-9+#]+`)
	classAttr      = regexp.MustCompile(`(?is)\bclass\s*=\s*(["'])(.*?)(["'])`)
	openTagAt      = regexp.MustCompile(`(?is)<(nav|aside|div|section)\b[^>]*>`)
	listOpen       = regexp.MustCompile(`(?is)^\s*<(ol|ul)\b`)
	tocContainer   = regexp.MustCompile(`(?is)<(nav|aside|div|section)\b[^>]*\bclass\s*=\s*["'](?:[^"']*\s)?(?:local-toc|lesson-toc|toc|contents)(?:\s[^"']*)?["'][^>]*>`)
)

// NormalizeLesson rewrites an uploaded course lesson or article to the
// site's content conventions (docs/DESIGN.md, "Content updates"). It is
// applied on every lesson and article upload and by cmd/lessonfmt for
// files already in Storage, so the stored file is the canonical,
// conforming version. It is idempotent.
//
//  1. Theme: Google Fonts @imports are dropped (the CSP blocks them
//     anyway); :root palettes, light and dark, are replaced by one block
//     mapping the lesson's own property names (renamed --l-*) onto site
//     tokens. Components keep their rules and pick up site colours,
//     type and dark mode.
//  2. Navigation: an in-lesson "On this page"/"Contents" block is removed
//     — the reader's contents rail replaces it.
//  3. Headings: section numbers ("1. ", <span class="sec-num">) are
//     stripped from <h2>; the reader and outline number sections.
//  4. Code: each <pre> without a language gets <code class="language-x">
//     when the snippet is unambiguously Java, Go, Python, JavaScript,
//     SQL or shell, which gives runnable languages a Run button. Lab
//     editors (class lab-*) are left alone.
func NormalizeLesson(raw []byte) []byte {
	s := string(raw)
	if strings.Contains(s, NormalizedMarker) {
		return raw
	}
	s = normalizeTheme(s, strings.Contains(s, DarkThemeDirective))
	s = removeLocalTOC(s)
	s = h2Block.ReplaceAllStringFunc(s, func(m string) string {
		p := h2Block.FindStringSubmatch(m)
		inner := secNumSpan.ReplaceAllString(p[2], "")
		inner = leadingNumber.ReplaceAllString(inner, "$1")
		return p[1] + inner + p[3]
	})
	s = preBlock.ReplaceAllStringFunc(s, tagCodeLanguage)

	if i := strings.Index(strings.ToLower(s), "<body"); i >= 0 {
		if j := strings.Index(s[i:], ">"); j >= 0 {
			at := i + j + 1
			return []byte(s[:at] + "\n" + NormalizedMarker + s[at:])
		}
	}
	return []byte(NormalizedMarker + "\n" + s)
}

func normalizeTheme(s string, dark bool) string {
	names := map[string]string{} // name (without --) -> first value seen
	var order []string
	s = styleBlock.ReplaceAllStringFunc(s, func(m string) string {
		p := styleBlock.FindStringSubmatch(m)
		css := fontImport.ReplaceAllString(p[2], "")
		collect := func(body string) {
			for _, d := range customPropDecl.FindAllStringSubmatch(body, -1) {
				n := strings.TrimPrefix(d[1], "--")
				if _, ok := names[n]; !ok {
					names[n] = strings.TrimSpace(d[2])
					order = append(order, n)
				}
			}
		}
		css = darkMediaBlock.ReplaceAllString(css, "")
		css = rootBlock.ReplaceAllStringFunc(css, func(rb string) string {
			body := rootBlock.FindStringSubmatch(rb)[1]
			collect(body)
			rest := strings.TrimSpace(customPropDecl.ReplaceAllString(body, ""))
			if rest == "" {
				return ""
			}
			return ":root{" + rest + "}"
		})
		return p[1] + css + p[3]
	})
	if len(names) == 0 {
		return s
	}
	sort.SliceStable(order, func(i, j int) bool { return len(order[i]) > len(order[j]) })
	rename := func(text string) string {
		for _, n := range order {
			text = regexp.MustCompile(`var\(\s*--`+regexp.QuoteMeta(n)+`\s*([,)])`).ReplaceAllString(text, "var(--l-"+n+"$1")
			text = strings.ReplaceAll(text, `'--`+n+`'`, `'--l-`+n+`'`)
			text = strings.ReplaceAll(text, `"--`+n+`"`, `"--l-`+n+`"`)
		}
		return text
	}
	s = rename(s)

	if dark {
		s = recolorCool(s)
	}
	var b strings.Builder
	b.WriteString("\n/* vb: lesson palette mapped onto site tokens (docs/DESIGN.md) */\n:root{")
	keys := append([]string(nil), order...)
	sort.Strings(keys)
	for _, n := range keys {
		v, ok := lessonTokenMap[n]
		if dark {
			if dv, dok := lessonDarkMap[n]; dok {
				v, ok = dv, true
			} else if ok && !strings.HasPrefix(n, "font") && n != "display" && n != "mono" && n != "radius" {
				ok = false
			}
		}
		if !ok {
			v = rename(names[n])
		}
		b.WriteString("--l-" + n + ":" + v + ";")
	}
	b.WriteString("}\n")
	if dark {
		// The site tokens themselves go dark too, so the site's own rules
		// inside the frame (headings, links, buttons) match the panels.
		b.WriteString(":root{" + siteDarkTokens + "}\n")
		b.WriteString("html,body{background:#14110e;color:#f5f0ea}\n")
	}
	mapping := b.String()
	inserted := false
	s = styleBlock.ReplaceAllStringFunc(s, func(m string) string {
		if inserted {
			return m
		}
		inserted = true
		p := styleBlock.FindStringSubmatch(m)
		return p[1] + mapping + p[2] + p[3]
	})
	return s
}

// removeLocalTOC drops a lesson's own "On this page" / "Contents" block.
// When the heading sits inside a nav/aside (or an element whose class
// mentions toc/contents), that whole element goes; otherwise the heading
// and the list right after it go.
func removeLocalTOC(s string) string {
	// Containers marked as a table of contents by class, whatever their
	// heading looks like (some lessons use <strong>In this course</strong>).
	for guard := 0; guard < 5; guard++ {
		m := tocContainer.FindStringSubmatchIndex(s)
		if m == nil {
			break
		}
		tag := strings.ToLower(s[m[2]:m[3]])
		end := matchClose(s, m[0], tag)
		if end < 0 {
			break
		}
		s = s[:m[0]] + s[end:]
	}
	for guard := 0; guard < 5; guard++ {
		loc := -1
		var hEnd int
		for _, m := range h2Block.FindAllStringSubmatchIndex(s, -1) {
			text := strings.TrimSpace(tagPattern.ReplaceAllString(s[m[4]:m[5]], ""))
			if isNavHeadingText(text) {
				loc, hEnd = m[0], m[1]
				break
			}
		}
		if loc < 0 {
			return s
		}
		if start, end, ok := enclosingTOC(s, loc); ok {
			s = s[:start] + s[end:]
			continue
		}
		after := s[hEnd:]
		if lm := listOpen.FindStringSubmatchIndex(after); lm != nil {
			tag := strings.ToLower(after[lm[2]:lm[3]])
			if end := matchClose(after, lm[2]-1, tag); end > 0 {
				s = s[:loc] + after[end:]
				continue
			}
		}
		s = s[:loc] + after
	}
	return s
}

func isNavHeadingText(t string) bool {
	switch strings.ToLower(strings.Trim(strings.TrimSpace(t), ":")) {
	case "on this page", "in this lesson", "in this course", "contents", "table of contents", "toc", "navigation", "jump to", "jump to a section":
		return true
	}
	return false
}

// enclosingTOC finds the innermost nav/aside/div/section that opens before
// pos and closes after it, and returns its span when it is a navigation
// container.
func enclosingTOC(s string, pos int) (int, int, bool) {
	opens := openTagAt.FindAllStringSubmatchIndex(s[:pos], -1)
	for i := len(opens) - 1; i >= 0; i-- {
		o := opens[i]
		tag := strings.ToLower(s[o[2]:o[3]])
		end := matchClose(s, o[0], tag)
		if end <= pos {
			continue
		}
		open := strings.ToLower(s[o[0]:o[1]])
		cls := ""
		if c := classAttr.FindStringSubmatch(open); c != nil {
			cls = c[2]
		}
		if tag == "nav" || tag == "aside" || strings.Contains(cls, "toc") || strings.Contains(cls, "contents") {
			return o[0], end, true
		}
		return 0, 0, false
	}
	return 0, 0, false
}

// matchClose returns the index just past the </tag> that closes the
// element whose opening tag starts at from, counting nested same-name tags.
func matchClose(s string, from int, tag string) int {
	re := regexp.MustCompile(`(?i)<(/?)` + tag + `\b[^>]*>`)
	depth := 0
	for _, m := range re.FindAllStringSubmatchIndex(s[from:], -1) {
		if s[from+m[2]:from+m[3]] == "/" {
			depth--
			if depth == 0 {
				return from + m[1]
			}
		} else {
			depth++
		}
	}
	return -1
}

func tagCodeLanguage(m string) string {
	p := preBlock.FindStringSubmatch(m)
	attrs, inner := p[1], p[2]
	if c := classAttr.FindStringSubmatch(attrs); c != nil {
		if strings.Contains(c[2], "lab-") || langClass.MatchString(c[2]) {
			return m
		}
	}
	if co := codeOpen.FindStringSubmatch(inner); co != nil && langClass.MatchString(co[1]) {
		return m
	}
	lang := DetectCodeLanguage(htmlText(inner))
	if lang == "" {
		return m
	}
	if co := codeOpen.FindStringSubmatchIndex(inner); co != nil {
		openTag := inner[co[0]:co[1]]
		var newOpen string
		if c := classAttr.FindStringSubmatchIndex(openTag); c != nil {
			newOpen = openTag[:c[4]] + strings.TrimSpace(openTag[c[4]:c[5]]+" language-"+lang) + openTag[c[5]:]
		} else {
			newOpen = strings.Replace(openTag, "<code", `<code class="language-`+lang+`"`, 1)
		}
		return "<pre" + attrs + ">" + inner[:co[0]] + newOpen + inner[co[1]:] + "</pre>"
	}
	return "<pre" + attrs + `><code class="language-` + lang + `">` + inner + "</code></pre>"
}

func htmlText(s string) string {
	s = tagPattern.ReplaceAllString(s, "")
	r := strings.NewReplacer("&lt;", "<", "&gt;", ">", "&amp;", "&", "&quot;", `"`, "&#39;", "'", "&#x27;", "'", "&nbsp;", " ")
	return r.Replace(s)
}

var (
	javaSignals = regexp.MustCompile(`(?m)\b(public|private|protected)\s+(static\s+)?(final\s+)?(class|interface|record|enum|void|[A-Z]\w*(<[^>]*>)?)\b|System\.out\.|^\s*import\s+java\.|@Override|\bnew\s+[A-Z]\w*(<[^>]*>)?\(|\b(List|Map|Set|Optional|CompletableFuture)<`)
	goSignals   = regexp.MustCompile(`(?m)^\s*package\s+\w+\s*$|^\s*func\s+(\([^)]*\)\s*)?\w+\(|\bfmt\.\w+\(|\w+\s*:=\s*|^\s*import\s+\($|\bhttp\.HandleFunc\(`)
	pySignals   = regexp.MustCompile(`(?m)^\s*def\s+\w+\(.*\)\s*(->\s*[^:]+)?:\s*$|^\s*(import|from)\s+(torch|numpy|pandas|fastapi|typing|os|json)\b|^\s*class\s+\w+(\([^)]*\))?:\s*$|\bself\.\w+|^\s*print\(`)
	jsSignals   = regexp.MustCompile(`(?m)\bfunction\s+\w*\s*\(|^\s*(const|let|var)\s+\w+\s*=|=>\s*[{(]|\bconsole\.log\(|\bdocument\.\w+|\brequire\(['"]|\bawait\s+fetch\(|\bnew\s+Razorpay\(`)
	sqlSignals  = regexp.MustCompile(`(?m)^\s*(SELECT|INSERT\s+INTO|CREATE\s+(TABLE|INDEX|UNIQUE)|UPDATE\s+\w+\s+SET|ALTER\s+TABLE|DELETE\s+FROM|WITH\s+\w+\s+AS)\b`)
	shSignals   = regexp.MustCompile(`(?m)^\s*(\$\s+|#\s*!|go\s+(run|build|test|mod)\b|npm\s+|npx\s+|curl\s+|docker\s+|kubectl\s+|git\s+|psql\s+|pip\s+|export\s+[A-Z_]+=|mvn\s+|gradle\s+|cd\s+)`)
)

// DetectCodeLanguage guesses a snippet's language from strong signals
// only. It returns "" when unsure — an unlabelled block simply gets no
// Run button, which is better than opening Java in a Python compiler.
func DetectCodeLanguage(code string) string {
	score := map[string]int{
		"java":       len(javaSignals.FindAllString(code, -1)),
		"go":         len(goSignals.FindAllString(code, -1)),
		"python":     len(pySignals.FindAllString(code, -1)),
		"javascript": len(jsSignals.FindAllString(code, -1)),
		"sql":        len(sqlSignals.FindAllString(code, -1)),
		"bash":       len(shSignals.FindAllString(code, -1)),
	}
	semis := strings.Count(code, ";\n") + strings.Count(code, ";\r\n")
	if semis == 0 {
		score["java"] /= 2
	}
	best, bestN, second := "", 0, 0
	for _, l := range []string{"java", "go", "python", "javascript", "sql", "bash"} {
		if n := score[l]; n > bestN {
			best, second, bestN = l, bestN, n
		} else if n > second {
			second = n
		}
	}
	if bestN == 0 || (bestN < 2 && second > 0) || bestN == second {
		return ""
	}
	return best
}

var colorLiteral = regexp.MustCompile(`#[0-9a-fA-F]{6}\b|#[0-9a-fA-F]{3}\b|rgba?\(\s*\d+\s*,\s*\d+\s*,\s*\d+\s*(?:,\s*[\d.]+\s*)?\)`)

// recolorCool maps a dark-app lesson's cool (teal/navy/blue) colour
// literals onto the site's warm dark palette by lightness: dark blues
// become the dark surfaces, saturated teals become the gold accent, pale
// blue text becomes ink. Warm and neutral colours are left alone.
func recolorCool(s string) string {
	return colorLiteral.ReplaceAllStringFunc(s, func(lit string) string {
		r, g, b, a, ok := parseColor(lit)
		if !ok {
			return lit
		}
		h, sat, l := toHSL(r, g, b)
		if h < 165 || h > 250 || sat < 0.12 {
			return lit
		}
		var hex string
		switch {
		case l < 0.08:
			hex = "#14110e"
		case l < 0.12:
			hex = "#17130f"
		case l < 0.16:
			hex = "#1a1612"
		case l < 0.24:
			hex = "#201b17"
		case l < 0.34:
			hex = "#2e2721"
		case l > 0.78:
			hex = "#f5f0ea"
		case l > 0.62:
			hex = "#cbbfb2"
		default:
			hex = "#e0b04a"
		}
		if a < 0 {
			return hex
		}
		var rr, gg, bb int
		fmt.Sscanf(hex, "#%02x%02x%02x", &rr, &gg, &bb)
		return fmt.Sprintf("rgba(%d,%d,%d,%s)", rr, gg, bb, strconv.FormatFloat(a, 'f', -1, 64))
	})
}

func parseColor(lit string) (r, g, b int, a float64, ok bool) {
	a = -1
	if strings.HasPrefix(lit, "#") {
		h := lit[1:]
		if len(h) == 3 {
			h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
		}
		if _, err := fmt.Sscanf(h, "%02x%02x%02x", &r, &g, &b); err != nil {
			return 0, 0, 0, 0, false
		}
		return r, g, b, a, true
	}
	inner := lit[strings.Index(lit, "(")+1 : len(lit)-1]
	parts := strings.Split(inner, ",")
	if len(parts) < 3 {
		return 0, 0, 0, 0, false
	}
	vals := make([]float64, len(parts))
	for i, p := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return 0, 0, 0, 0, false
		}
		vals[i] = v
	}
	if len(vals) == 4 {
		a = vals[3]
	}
	return int(vals[0]), int(vals[1]), int(vals[2]), a, true
}

func toHSL(r, g, b int) (h, s, l float64) {
	rf, gf, bf := float64(r)/255, float64(g)/255, float64(b)/255
	mx := math.Max(rf, math.Max(gf, bf))
	mn := math.Min(rf, math.Min(gf, bf))
	l = (mx + mn) / 2
	if mx == mn {
		return 0, 0, l
	}
	d := mx - mn
	if l > 0.5 {
		s = d / (2 - mx - mn)
	} else {
		s = d / (mx + mn)
	}
	switch mx {
	case rf:
		h = (gf - bf) / d
		if gf < bf {
			h += 6
		}
	case gf:
		h = (bf-rf)/d + 2
	default:
		h = (rf-gf)/d + 4
	}
	return h * 60, s, l
}
