package storage

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestWriteWatermarkedInsertsBeforeBody(t *testing.T) {
	src := strings.NewReader("<html><body><p>hi</p></body></html>")
	var dst bytes.Buffer
	if err := WriteWatermarked(&dst, src, "MARK"); err != nil {
		t.Fatal(err)
	}
	got := dst.String()
	want := "<html><body><p>hi</p>MARK</body></html>"
	if got != want {
		t.Fatalf("got %q", got)
	}
}

func TestWriteWatermarkedAppendsWhenNoBody(t *testing.T) {
	var dst bytes.Buffer
	if err := WriteWatermarked(&dst, strings.NewReader("plain"), "MARK"); err != nil {
		t.Fatal(err)
	}
	if got := dst.String(); got != "plainMARK" {
		t.Fatalf("got %q", got)
	}
}

func TestWriteWatermarkedMarkerSplitAcrossReads(t *testing.T) {
	src := &oneByteReader{s: []byte("<html><body>x</body></html>")}
	var dst bytes.Buffer
	if err := WriteWatermarked(&dst, src, "MARK"); err != nil {
		t.Fatal(err)
	}
	if got := dst.String(); got != "<html><body>xMARK</body></html>" {
		t.Fatalf("got %q", got)
	}
}

func TestWriteWatermarkedUsesLastBody(t *testing.T) {
	src := strings.NewReader("<body>one</body><body>two</body>")
	var dst bytes.Buffer
	if err := WriteWatermarked(&dst, src, "MARK"); err != nil {
		t.Fatal(err)
	}
	if got := dst.String(); got != "<body>one</body><body>twoMARK</body>" {
		t.Fatalf("got %q", got)
	}
}

type oneByteReader struct{ s []byte }

func (r *oneByteReader) Read(p []byte) (int, error) {
	if len(r.s) == 0 {
		return 0, io.EOF
	}
	if len(p) == 0 {
		return 0, nil
	}
	p[0] = r.s[0]
	r.s = r.s[1:]
	return 1, nil
}
