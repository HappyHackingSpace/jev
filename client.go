package jev

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"strings"
	"time"
)

// Defaults and environment variables, shared with TypeSafe's official SDKs.
const (
	DefaultBaseURL = "https://api.typesafe.ai"
	DefaultTimeout = 60 * time.Second

	EnvAPIKey  = "TYPESAFE_API_KEY"
	EnvBaseURL = "TYPESAFE_BASE_URL"
	EnvModel   = "TYPESAFE_DEFAULT_MODEL"
)

// Model aliases. An alias moves when a new model ships, so pin a versioned
// name such as "jev-1.14.0" when thresholds were tuned against one version.
const (
	ModelLatest  = "jev-latest"
	ModelPreview = "jev-preview"
)

// Version is the SDK version, sent in the User-Agent header.
var Version = "dev"

// maxResponseBody bounds how much of a response is read into memory.
const maxResponseBody = 16 << 20

// Client calls the System One API. It is safe for concurrent use.
type Client struct {
	apiKey    string
	baseURL   string
	model     string
	userAgent string
	http      *http.Client
	retry     RetryPolicy
}

// Option configures a [Client].
type Option func(*Client)

// WithAPIKey sets the API key instead of reading TYPESAFE_API_KEY.
func WithAPIKey(key string) Option { return func(c *Client) { c.apiKey = key } }

// WithBaseURL points the client at another API host, such as a proxy.
func WithBaseURL(url string) Option { return func(c *Client) { c.baseURL = url } }

// WithModel sets the default model, used when a request names none.
func WithModel(model string) Option { return func(c *Client) { c.model = model } }

// WithHTTPClient replaces the HTTP client, for custom transports or proxies.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithTimeout sets the timeout of each HTTP attempt.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		h := *c.http
		h.Timeout = d
		c.http = &h
	}
}

// WithRetry replaces the retry policy.
func WithRetry(p RetryPolicy) Option { return func(c *Client) { c.retry = p } }

// WithUserAgent prefixes the User-Agent header with your application's name.
func WithUserAgent(ua string) Option {
	return func(c *Client) { c.userAgent = ua + " " + c.userAgent }
}

// New returns a client configured from options, falling back to the
// TYPESAFE_* environment variables and then to the defaults.
func New(opts ...Option) (*Client, error) {
	c := &Client{
		apiKey:    os.Getenv(EnvAPIKey),
		baseURL:   cmp.Or(os.Getenv(EnvBaseURL), DefaultBaseURL),
		model:     cmp.Or(os.Getenv(EnvModel), ModelLatest),
		userAgent: "happyhackingspace-jev/" + Version,
		http:      &http.Client{Timeout: DefaultTimeout},
		retry:     DefaultRetryPolicy(),
	}
	for _, opt := range opts {
		opt(c)
	}
	c.apiKey = strings.TrimSpace(c.apiKey)
	c.baseURL = strings.TrimRight(strings.TrimSpace(c.baseURL), "/")
	if c.apiKey == "" {
		return nil, fmt.Errorf("typesafe: no API key: set %s or use WithAPIKey", EnvAPIKey)
	}
	if strings.ContainsAny(c.apiKey, " \t\r\n") {
		return nil, errors.New("typesafe: API key contains whitespace")
	}
	return c, nil
}

// SystemOne asks every question in the request about its state.
func (c *Client) SystemOne(ctx context.Context, req Request) (*Response, error) {
	if len(req.Questions) == 0 {
		return nil, errors.New("typesafe: request has no questions")
	}
	if req.Model == "" {
		req.Model = c.model
	}
	body, contentType, err := encodeRequest(req)
	if err != nil {
		return nil, err
	}
	header := http.Header{"Content-Type": {contentType}}
	if req.needsImages() {
		header.Set("TypeSafe-Danger-Zone-Images", "true")
	}
	if req.LongContext {
		header.Set("TypeSafe-Danger-Zone-Long-Context", "true")
	}
	var resp Response
	if err := c.do(ctx, http.MethodPost, "/v1/systemone", header, body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Models lists the models and aliases available to the account.
func (c *Client) Models(ctx context.Context) ([]Model, error) {
	var resp struct {
		Models []Model `json:"models"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/models", nil, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Models, nil
}

// encodeRequest sends images as multipart file parts, which the API
// recommends over base64 because base64 inflates the body by a third.
func encodeRequest(req Request) ([]byte, string, error) {
	type asset struct {
		Type string `json:"type"`
		Part string `json:"part"`
	}
	type wire struct {
		Request
		Assets map[string]asset `json:"assets,omitempty"`
	}
	w := wire{Request: req}
	if len(req.Images) == 0 {
		b, err := json.Marshal(w)
		return b, "application/json", err
	}

	w.Assets = make(map[string]asset, len(req.Images))
	parts := make([]string, 0, len(req.Images))
	for ref := range req.Images {
		part := fmt.Sprintf("image-%d", len(parts))
		parts = append(parts, ref)
		w.Assets[ref] = asset{Type: "image", Part: part}
	}
	js, err := json.Marshal(w)
	if err != nil {
		return nil, "", err
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="request"`)
	h.Set("Content-Type", "application/json")
	pw, err := mw.CreatePart(h)
	if err != nil {
		return nil, "", err
	}
	if _, err := pw.Write(js); err != nil {
		return nil, "", err
	}
	for i, ref := range parts {
		img := req.Images[ref]
		name := fmt.Sprintf("image-%d", i)
		ext := cmp.Or(img.Format, "png")
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename="%s.%s"`, name, name, ext))
		h.Set("Content-Type", "image/"+strings.Replace(ext, "jpg", "jpeg", 1))
		pw, err := mw.CreatePart(h)
		if err != nil {
			return nil, "", err
		}
		if _, err := pw.Write(img.Data); err != nil {
			return nil, "", err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), mw.FormDataContentType(), nil
}

func (c *Client) do(ctx context.Context, method, path string, header http.Header, body []byte, out any) error {
	for retry := 0; ; retry++ {
		res := c.attempt(ctx, method, path, header, body, out)
		if res.err == nil || !res.retry || retry >= c.retry.MaxRetries || ctx.Err() != nil {
			return res.err
		}
		wait := res.wait
		if wait < 0 {
			wait = c.retry.backoff(retry)
		}
		select {
		case <-ctx.Done():
			return res.err
		case <-time.After(wait):
		}
	}
}

type result struct {
	err   error
	retry bool
	// wait is the server-requested delay before a retry, or -1 for none.
	wait time.Duration
}

// attempt makes one HTTP call and reports whether a failure is worth retrying.
func (c *Client) attempt(ctx context.Context, method, path string, header http.Header, body []byte, out any) result {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return result{err: err}
	}
	for k, v := range header {
		req.Header[k] = v
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return result{err: fmt.Errorf("typesafe: %w", err), retry: true, wait: -1}
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return result{err: fmt.Errorf("typesafe: read response: %w", err), retry: true, wait: -1}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := newAPIError(resp.StatusCode, raw)
		wait, ok := retryAfter(resp.Header)
		if !ok {
			wait = -1
		}
		return result{err: apiErr, retry: apiErr.Retryable(), wait: wait}
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return result{err: fmt.Errorf("typesafe: decode response: %w", err)}
	}
	return result{}
}
