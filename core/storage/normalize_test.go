package storage

import (
	"strings"
	"testing"
)

const sampleLesson = `<!DOCTYPE html><html><head><style>
@import url('https://fonts.googleapis.com/css2?family=IBM+Plex+Sans&display=swap');
:root{ --bg:#fff; --fg:#111; --accent:#8B5CF6; --custom:#123456; }
@media (prefers-color-scheme: dark){ :root{ --bg:#000; --fg:#eee; } }
body{ background:var(--bg); color:var(--fg); }
a{ color:var(--accent, red); border-color:var(--custom); }
</style></head><body>
<nav class="local-toc"><h2>On this page</h2><ol><li><a href="#a">A</a></li></ol></nav>
<section><h2>1. Language Foundations</h2>
<h2><span class="sec-num">02</span>Delivery Semantics</h2>
<pre class="code"><code>public class Hello {
    public static void main(String[] args) {
        System.out.println("hi");
    }
}</code></pre>
<pre class="code">package main

import "fmt"

func main() {
	x := 1
	fmt.Println(x)
}</pre>
<pre class="lab-editor">int x = 1;</pre>
<pre>just some text</pre>
<div class="toc-like"><strong>In this course</strong></div>
</section></body></html>`

func TestNormalizeLesson(t *testing.T) {
	out := string(NormalizeLesson([]byte(sampleLesson)))
	mustHave := []string{
		NormalizedMarker,
		"--l-bg:var(--bg)", "--l-fg:var(--ink)", "--l-accent:var(--accent-strong)", "--l-custom:#123456",
		"background:var(--l-bg)", "color:var(--l-accent, red)", "border-color:var(--l-custom)",
		"<h2>Language Foundations</h2>", "<h2>Delivery Semantics</h2>",
		`<code class="language-java">`, `<pre class="code"><code class="language-go">package main`,
		`<pre class="lab-editor">int x = 1;</pre>`, "<pre>just some text</pre>", `<div class="toc-like">`,
	}
	for _, w := range mustHave {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q", w)
		}
	}
	mustNot := []string{"fonts.googleapis", "On this page", "local-toc", "#8B5CF6", "prefers-color-scheme", "sec-num"}
	for _, w := range mustNot {
		if strings.Contains(out, w) {
			t.Errorf("should not contain %q", w)
		}
	}
	if again := string(NormalizeLesson([]byte(out))); again != out {
		t.Error("NormalizeLesson is not idempotent")
	}
}

func TestNormalizeLessonWithoutStyleOrBody(t *testing.T) {
	in := `<h2>Contents</h2><ul><li>x</li></ul><h2>Real section</h2><p>body</p>`
	out := string(NormalizeLesson([]byte(in)))
	if strings.Contains(out, "Contents") || !strings.Contains(out, "<h2>Real section</h2>") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestDetectCodeLanguage(t *testing.T) {
	cases := map[string]string{
		"def train(model):\n    model.fit()\n    print(model)\n":                 "python",
		"const total = items.reduce((a, b) => a + b, 0);\nconsole.log(total);\n": "javascript",
		"SELECT id, title FROM courses WHERE is_published;\n":                    "sql",
		"$ go run ./cmd/server\n$ curl localhost:8080\n":                         "bash",
		"Just prose in a box.": "",
	}
	for code, want := range cases {
		if got := DetectCodeLanguage(code); got != want {
			t.Errorf("DetectCodeLanguage(%q) = %q, want %q", code, got, want)
		}
	}
}

func TestNormalizeLessonDarkApp(t *testing.T) {
	in := `<html><head><style>:root{--bg:#0b1c2c;--ink:#e8f1f6;--teal:#2ec4b6;--font:"Fraunces"}
.side{background:linear-gradient(#0c2233,#0a1c2b);border:1px solid rgba(46,196,182,.25);color:#f4a261}</style></head>
<body><!-- vb:theme dark --><div class="side">x</div></body></html>`
	out := string(NormalizeLesson([]byte(in)))
	for _, w := range []string{"--l-bg:#14110e", "--l-ink:#f5f0ea", "--l-teal:#e0b04a", "--l-font:var(--font-text)", "rgba(224,176,74,0.25)", "color:#f4a261", "color-scheme: dark", "--ink: #f5f0ea"} {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q", w)
		}
	}
	for _, w := range []string{"#0c2233", "#0a1c2b", "46,196,182"} {
		if strings.Contains(out, w) {
			t.Errorf("cool literal %q survived", w)
		}
	}
}
