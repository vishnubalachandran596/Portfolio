// Package web embeds the static assets (CSS, JS, images) served at /static.
package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
)

//go:embed all:static
var StaticFS embed.FS

// AssetVersion is a short hash of the embedded static assets, computed
// once at process start. Appended as a "?v=" query string to every CSS/JS
// URL so those responses can be served with a long, immutable Cache-Control
// header: the URL only ever changes when the file's actual bytes do (a new
// deployment embeds different files, producing a different hash), so
// "cache this forever" is genuinely safe rather than risking visitors
// stuck on stale CSS/JS after a deploy.
var AssetVersion = computeAssetVersion()

func computeAssetVersion() string {
	h := sha256.New()
	for _, name := range []string{"static/css/style.css", "static/js/main.js", "static/js/admin.js", "static/js/theme.js"} {
		b, err := StaticFS.ReadFile(name)
		if err != nil {
			continue
		}
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))[:10]
}
