// Package web embeds the static assets (CSS, JS, images) served at /static.
package web

import "embed"

//go:embed all:static
var StaticFS embed.FS
