package storage

import "testing"

func TestArticleObject(t *testing.T) {
	got, err := ArticleObject("what-actually-breaks-at-5-million-requests-a-day")
	if err != nil {
		t.Fatal(err)
	}
	if got != "articles/what-actually-breaks-at-5-million-requests-a-day.html" {
		t.Fatalf("got %s", got)
	}
	if _, err := ArticleObject("../etc/passwd"); err == nil {
		t.Fatal("expected invalid slug to fail")
	}
	if _, err := ArticleObject("courses/java-fundamentals"); err == nil {
		t.Fatal("expected a slash in the slug to fail")
	}
}

func TestArticleHTMLExtractsBody(t *testing.T) {
	raw := []byte("<!DOCTYPE html><html><head><title>T</title></head><body class=\"x\"><p>Hello</p></body></html>")
	got := ArticleHTML(raw)
	if got != "<p>Hello</p>" {
		t.Fatalf("got %q", got)
	}
}

func TestArticleHTMLKeepsFragment(t *testing.T) {
	raw := []byte("  <h2>A heading</h2>\n<p>Body</p>  ")
	got := ArticleHTML(raw)
	if got != "<h2>A heading</h2>\n<p>Body</p>" {
		t.Fatalf("got %q", got)
	}
}
