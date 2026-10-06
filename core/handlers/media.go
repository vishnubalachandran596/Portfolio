package handlers

import (
	"bytes"
	"context"
	"io"
	"net/http"
)

// maxImageUploadBytes caps a profile photo or logo upload. Generous for a
// headshot or a logo graphic; well short of anything that would meaningfully
// slow the admin form's multipart parse.
const maxImageUploadBytes = 5 << 20 // 5MB

// sniffImageExt identifies jpg/png/webp by magic bytes, never by a
// client-supplied Content-Type or filename -- a browser's declared MIME
// type for a file input is easy to spoof and has no bearing on what the
// bytes actually are. Returns "" for anything else, which readImageUpload
// turns into a rejection.
func sniffImageExt(b []byte) string {
	switch {
	case len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF:
		return "jpg"
	case len(b) >= 8 && bytes.Equal(b[:8], []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}):
		return "png"
	case len(b) >= 12 && string(b[0:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return "webp"
	default:
		return ""
	}
}

func imageContentType(ext string) string {
	switch ext {
	case "png":
		return "image/png"
	case "webp":
		return "image/webp"
	default:
		return "image/jpeg"
	}
}

// readImageUpload reads an optional image file from a multipart form
// field. A missing field is not an error (nothing to replace); an empty
// file the same. Mirrors readResumeUpload's shape in admin_content.go,
// reused for both the profile photo and the logo rather than writing
// this parsing/validation twice.
func readImageUpload(r *http.Request, field string) (data []byte, ext string, errMsg string) {
	file, _, err := r.FormFile(field)
	if err == http.ErrMissingFile {
		return nil, "", ""
	}
	if err != nil {
		return nil, "", "Could not read the uploaded image."
	}
	defer file.Close()
	b, err := io.ReadAll(io.LimitReader(file, maxImageUploadBytes+1))
	if err != nil {
		return nil, "", "Could not read the uploaded image."
	}
	if len(b) == 0 {
		return nil, "", ""
	}
	if len(b) > maxImageUploadBytes {
		return nil, "", "Image must be 5MB or smaller."
	}
	ext = sniffImageExt(b)
	if ext == "" {
		return nil, "", "Image must be a JPEG, PNG, or WEBP file."
	}
	return b, ext, ""
}

// replaceStoredImage uploads data to the path objectFor(ext) builds and,
// when the previously stored object used a different extension (a PNG
// replaced by a JPEG, say), deletes that stale object so two files for
// the same image never sit in the bucket at once. Returns the new
// object path to store on the profile row.
func (a *App) replaceStoredImage(
	ctx context.Context,
	objectFor func(ext string) (string, error),
	oldObject string,
	data []byte,
	ext string,
) (string, error) {
	path, err := objectFor(ext)
	if err != nil {
		return "", err
	}
	if err := a.CourseStorage.Put(ctx, path, imageContentType(ext), data); err != nil {
		return "", err
	}
	if oldObject != "" && oldObject != path {
		_ = a.CourseStorage.Delete(ctx, oldObject)
	}
	return path, nil
}

// ProfilePhoto streams the uploaded headshot. Public, no auth -- it
// renders on Home and About for every visitor. 404 until one is
// uploaded via /admin/profile.
func (a *App) ProfilePhoto(w http.ResponseWriter, r *http.Request) {
	a.serveStoredImage(w, r, func() (string, error) {
		p, err := a.Profile.Get(r.Context())
		return p.PhotoObject, err
	})
}

// Logo streams the uploaded site logo, same shape as ProfilePhoto.
func (a *App) Logo(w http.ResponseWriter, r *http.Request) {
	a.serveStoredImage(w, r, func() (string, error) {
		p, err := a.Profile.Get(r.Context())
		return p.LogoObject, err
	})
}

func (a *App) serveStoredImage(w http.ResponseWriter, r *http.Request, lookup func() (string, error)) {
	if a.CourseStorage == nil {
		a.NotFound(w, r)
		return
	}
	object, err := lookup()
	if err != nil {
		a.serverError(w, "load image object", err)
		return
	}
	if object == "" {
		a.NotFound(w, r)
		return
	}
	rc, err := a.CourseStorage.Open(r.Context(), object)
	if err != nil {
		a.serverError(w, "open image", err)
		return
	}
	defer rc.Close()
	raw, err := io.ReadAll(io.LimitReader(rc, maxImageUploadBytes+1))
	if err != nil {
		a.serverError(w, "read image", err)
		return
	}
	ext := sniffImageExt(raw)
	w.Header().Set("Content-Type", imageContentType(ext))
	// Always revalidate: the admin can replace this image at any time,
	// and unlike the CSS/JS asset pipeline this path has no content-hash
	// query string to bust a long-lived cache with -- correctness over
	// a cache hit, same lesson as the earlier vishnu-photo.jpg incident.
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(raw)
}
