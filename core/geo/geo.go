// Package geo reads visitor geolocation from the request — specifically
// the header Vercel's edge network sets on every request it proxies,
// before it ever reaches this app's code. There's no separate geo-IP
// lookup or third-party service to configure.
package geo

import "net/http"

// CountryFromRequest returns the visitor's ISO 3166-1 alpha-2 country
// code (e.g. "IN", "US"), or "" if unknown — always the case in local
// development, since the header only exists once traffic is proxied
// through Vercel. Any client-supplied value of this header is stripped
// and overwritten by Vercel's edge before the request reaches this app,
// so it can't be spoofed to claim a different region.
func CountryFromRequest(r *http.Request) string {
	return r.Header.Get("X-Vercel-IP-Country")
}
