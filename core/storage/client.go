// Package storage streams private objects from Supabase Storage.
// Paid lessons are read after the purchase check. Article HTML uploaded
// from the admin console is read when the article page renders. A role
// resume PDF is read when someone downloads that role's resume. The
// profile photo and site logo, uploaded via /admin/profile, are read on
// every page render that needs them. The browser never receives an
// object URL -- every one of these is proxied through the app's own
// route, which is also what lets a replaced photo/logo take effect
// immediately with no stale-cache risk.
package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/config"
)

const (
	// Bucket holds paid lesson HTML.
	Bucket = "course-content"
	// ArticleBucket is the private bucket shown in Storage for blog HTML.
	ArticleBucket = "articles"
	// ResumeBucket is the private bucket for role-keyed resume PDFs.
	ResumeBucket = "resumes"
	// ProfilePhotoBucket is the private bucket for the profile headshot.
	ProfilePhotoBucket = "profile-photo"
	// LogoBucket is the private bucket for the site logo.
	LogoBucket = "site-logo"
)

// objectPath is courses/{slug}.html or articles/{slug}.html. Slugs come
// from slugify, so this is the only shape the server will send to Storage.
var objectPath = regexp.MustCompile(`^(?:courses|articles)/[a-z0-9]+(?:-[a-z0-9]+)*\.html$`)

// resumePath is resumes/{role_key}.pdf. The role key is slugify(company + role).
var resumePath = regexp.MustCompile(`^resumes/[a-z0-9]+(?:-[a-z0-9]+)*\.pdf$`)

// imageExt matches the three formats sniffed by magic bytes on upload
// (see readImageUpload in core/handlers) -- never trusted from a
// filename or a client-supplied Content-Type.
const imageExt = `jpg|jpeg|png|webp`

var profilePhotoPath = regexp.MustCompile(`^profile/photo\.(?:` + imageExt + `)$`)
var logoPath = regexp.MustCompile(`^brand/logo\.(?:` + imageExt + `)$`)

// CourseObject is the private-bucket path for a course slug.
func CourseObject(slug string) (string, error) {
	return objectFor("courses", slug)
}

// ArticleObject is the private-bucket path for an article slug.
func ArticleObject(slug string) (string, error) {
	return objectFor("articles", slug)
}

// ResumeObject is the private-bucket path for a role key.
func ResumeObject(roleKey string) (string, error) {
	path := "resumes/" + roleKey + ".pdf"
	if !resumePath.MatchString(path) {
		return "", fmt.Errorf("invalid storage object path %q", path)
	}
	return path, nil
}

// ProfilePhotoObject is the private-bucket path for the profile headshot,
// fixed except for the extension (jpg/jpeg/png/webp) sniffed from the
// uploaded bytes.
func ProfilePhotoObject(ext string) (string, error) {
	path := "profile/photo." + ext
	if !profilePhotoPath.MatchString(path) {
		return "", fmt.Errorf("invalid storage object path %q", path)
	}
	return path, nil
}

// LogoObject is the private-bucket path for the site logo, same shape
// as ProfilePhotoObject.
func LogoObject(ext string) (string, error) {
	path := "brand/logo." + ext
	if !logoPath.MatchString(path) {
		return "", fmt.Errorf("invalid storage object path %q", path)
	}
	return path, nil
}

func objectFor(prefix, slug string) (string, error) {
	path := prefix + "/" + slug + ".html"
	if !objectPath.MatchString(path) {
		return "", fmt.Errorf("invalid storage object path %q", path)
	}
	return path, nil
}

// Client talks to Supabase Storage. A secret key (service role) is used
// as-is. Otherwise the client signs in as the dedicated storage reader
// and caches that access token on the warm instance; losing the cache
// only means the next call signs in again.
type Client struct {
	base string
	http *http.Client

	secretKey string
	anonKey   string
	email     string
	password  string

	// localDir, when set, is used instead of Supabase. Docker Compose
	// points it at the mounted content directory.
	localDir string

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

// New returns nil when Storage is not configured, so the rest of the
// site still boots. Lesson pages fail closed until it is set.
// A local content directory takes over Open, Put, and Delete even when
// Supabase credentials are also present.
func New(cfg *config.Config) *Client {
	if cfg == nil {
		return nil
	}
	if dir := strings.TrimSpace(cfg.LocalContentDir); dir != "" {
		return &Client{localDir: dir}
	}
	base := strings.TrimRight(strings.TrimSpace(cfg.SupabaseURL), "/")
	if base == "" {
		return nil
	}
	c := &Client{
		base:      base,
		http:      &http.Client{Timeout: 60 * time.Second},
		secretKey: strings.TrimSpace(cfg.SupabaseSecretKey),
		anonKey:   strings.TrimSpace(cfg.SupabaseAnonKey),
		email:     strings.TrimSpace(cfg.SupabaseStorageEmail),
		password:  cfg.SupabaseStoragePassword,
	}
	if c.secretKey != "" {
		return c
	}
	if c.anonKey == "" || c.email == "" || c.password == "" {
		return nil
	}
	return c
}

// Open streams one object. The caller closes the body.
func (c *Client) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	if c.localDir != "" {
		return c.openLocal(path)
	}
	if !allowedObject(path) {
		return nil, fmt.Errorf("refusing storage path %q", path)
	}
	resp, err := c.do(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, fmt.Errorf("open %s: %s", path, readErr(resp))
	}
	return resp.Body, nil
}

// Put writes an object, replacing any object already at that path.
func (c *Client) Put(ctx context.Context, path, contentType string, body []byte) error {
	if c.localDir != "" {
		return c.putLocal(path, body)
	}
	if !allowedObject(path) {
		return fmt.Errorf("refusing storage path %q", path)
	}
	resp, err := c.do(ctx, http.MethodPost, path, body, contentType)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("put %s: %s", path, readErr(resp))
	}
	return nil
}

// Delete removes an object. A missing object is not an error.
func (c *Client) Delete(ctx context.Context, path string) error {
	if c.localDir != "" {
		return c.deleteLocal(path)
	}
	if path == "" {
		return nil
	}
	if !allowedObject(path) {
		return fmt.Errorf("refusing storage path %q", path)
	}
	resp, err := c.do(ctx, http.MethodDelete, path, nil, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound {
		return nil
	}
	return fmt.Errorf("delete %s: %s", path, readErr(resp))
}

func (c *Client) do(ctx context.Context, method, path string, body []byte, contentType string) (*http.Response, error) {
	apiKey, bearer, err := c.authorize(ctx)
	if err != nil {
		return nil, err
	}
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.objectURL(path), rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("apikey", apiKey)
	req.Header.Set("Authorization", "Bearer "+bearer)
	if method == http.MethodPost {
		req.Header.Set("x-upsert", "true")
		if contentType == "" {
			contentType = "text/html"
		}
		req.Header.Set("Content-Type", contentType)
	}
	return c.http.Do(req)
}

func (c *Client) objectURL(path string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return c.base + "/storage/v1/object/" + url.PathEscape(bucketFor(path)) + "/" + strings.Join(parts, "/")
}

func allowedObject(path string) bool {
	return objectPath.MatchString(path) || resumePath.MatchString(path) ||
		profilePhotoPath.MatchString(path) || logoPath.MatchString(path)
}

func (c *Client) openLocal(path string) (io.ReadCloser, error) {
	full, err := c.localFile(path)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(full)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return f, nil
}

func (c *Client) putLocal(path string, body []byte) error {
	full, err := c.localFile(path)
	if err != nil {
		return err
	}
	// 0777/0666 so a file written in the container can be replaced from
	// the Windows or Mac host, and a file the host drops in can be
	// overwritten by the app user after the entrypoint chmod.
	if err := os.MkdirAll(filepath.Dir(full), 0o777); err != nil {
		return fmt.Errorf("put %s: %w", path, err)
	}
	if err := os.WriteFile(full, body, 0o666); err != nil {
		return fmt.Errorf("put %s: %w", path, err)
	}
	return nil
}

func (c *Client) deleteLocal(path string) error {
	if path == "" {
		return nil
	}
	full, err := c.localFile(path)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete %s: %w", path, err)
	}
	return nil
}

// localFile resolves an object path under localDir. allowedObject rejects
// anything that is not an article, course, or resume key, and filepath.Rel
// rejects a path that would leave the directory.
func (c *Client) localFile(object string) (string, error) {
	if !allowedObject(object) {
		return "", fmt.Errorf("refusing storage path %q", object)
	}
	root, err := filepath.Abs(c.localDir)
	if err != nil {
		return "", err
	}
	full := filepath.Join(root, filepath.FromSlash(object))
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("refusing storage path %q", object)
	}
	return full, nil
}

// bucketFor sends each object prefix to its own private bucket.
func bucketFor(path string) string {
	switch {
	case strings.HasPrefix(path, "articles/"):
		return ArticleBucket
	case strings.HasPrefix(path, "resumes/"):
		return ResumeBucket
	case strings.HasPrefix(path, "profile/"):
		return ProfilePhotoBucket
	case strings.HasPrefix(path, "brand/"):
		return LogoBucket
	default:
		return Bucket
	}
}

func (c *Client) authorize(ctx context.Context) (apiKey, bearer string, err error) {
	if c.secretKey != "" {
		return c.secretKey, c.secretKey, nil
	}
	tok, err := c.accessToken(ctx)
	if err != nil {
		return "", "", err
	}
	return c.anonKey, tok, nil
}

func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.tokenExp) {
		return c.token, nil
	}
	payload, err := json.Marshal(map[string]string{
		"email":    c.email,
		"password": c.password,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/auth/v1/token?grant_type=password", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("apikey", c.anonKey)
	req.Header.Set("Authorization", "Bearer "+c.anonKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("storage sign-in: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("storage sign-in: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("storage sign-in: %s", resp.Status)
	}
	var parsed struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || parsed.AccessToken == "" {
		return "", fmt.Errorf("storage sign-in: no access token")
	}
	ttl := time.Duration(parsed.ExpiresIn) * time.Second
	if ttl < 2*time.Minute {
		ttl = 2 * time.Minute
	}
	c.token = parsed.AccessToken
	c.tokenExp = time.Now().Add(ttl - time.Minute)
	return c.token, nil
}

func readErr(resp *http.Response) string {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	s := strings.TrimSpace(string(b))
	if s == "" || strings.Contains(s, "access_token") || strings.Contains(strings.ToLower(s), "password") {
		return resp.Status
	}
	return resp.Status + ": " + s
}
