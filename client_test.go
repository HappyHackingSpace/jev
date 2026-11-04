package jev_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/happyhackingspace/jev"
)

// pngBytes is a 1x1 PNG, enough for content sniffing.
var pngBytes = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x02\x00\x00\x00\x90wS\xde\x00\x00\x00\x0cIDATx\x9cc\xf8\x0f\x00\x00\x01\x01\x00\x05\x18\xd8N\x00\x00\x00\x00IEND\xaeB`\x82")

func newClient(t *testing.T, h http.HandlerFunc, opts ...jev.Option) *jev.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	fast := jev.RetryPolicy{MaxRetries: 2, InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond}
	c, err := jev.New(append([]jev.Option{jev.WithAPIKey("test-key"), jev.WithBaseURL(srv.URL), jev.WithRetry(fast)}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestSystemOneEncodesQuestionsAndDecodesAnswers(t *testing.T) {
	var got map[string]any
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" || r.Method != http.MethodPost {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing bearer token")
		}
		if r.Header.Get("TypeSafe-Danger-Zone-Images") != "" {
			t.Errorf("images header sent for a text request")
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = io.WriteString(w, `{"model":"jev-1.14.0","answers":{
			"billing":{"type":"noul","noul":0},
			"tone":{"type":"choice","choice":"calm","confidence":0.7,"probabilities":{"calm":0.8,"angry":0.2}},
			"urgency":{"type":"score","score":1.7,"confidence":0.55,"legend":{"0":"low","1":"mid","2":"high"},"probabilities":{"0":0.1,"1":0.1,"2":0.8}}},
			"usage":{"input_tokens":120,"output_tokens":12}}`)
	})

	resp, err := c.SystemOne(context.Background(), jev.Request{
		State: map[string]any{"message": "charged twice"},
		Questions: jev.Questions{
			"billing": jev.Noul{Instructions: "About billing?", Criteria: &jev.NoulCriteria{True: "A billing issue"}},
			"tone":    jev.Choice{Instructions: "Tone?", Criteria: map[string]any{"calm": nil, "angry": "Hostile"}},
			"urgency": jev.Score{Instructions: "Urgency?", Criteria: []any{"low", "mid", "high"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if got["model"] != jev.ModelLatest {
		t.Errorf("model = %v, want default %s", got["model"], jev.ModelLatest)
	}
	qs := got["questions"].(map[string]any)
	tone := qs["tone"].(map[string]any)
	if tone["type"] != "choice" || tone["criteria"].(map[string]any)["calm"] != nil {
		t.Errorf("choice encoded wrong: %v", tone)
	}
	if _, ok := tone["criteria"].(map[string]any)["calm"]; !ok {
		t.Errorf("option without description was dropped")
	}
	billing := qs["billing"].(map[string]any)
	if _, ok := billing["criteria"].(map[string]any)["false"]; ok {
		t.Errorf("unset noul criterion was sent: %v", billing)
	}

	if p, err := resp.Noul("billing"); err != nil || p != 0 {
		t.Errorf("noul = %v, %v; want 0", p, err)
	}
	if a, err := resp.Choice("tone"); err != nil || a.Choice != "calm" || *a.Confidence != 0.7 {
		t.Errorf("choice = %+v, %v", a, err)
	}
	if a, err := resp.Score("urgency"); err != nil || *a.Score != 1.7 || a.Legend["2"] != "high" {
		t.Errorf("score = %+v, %v", a, err)
	}
	if _, err := resp.Noul("tone"); err == nil {
		t.Errorf("reading a choice as a noul should fail")
	}
	if resp.Model != "jev-1.14.0" || resp.Usage.InputTokens != 120 {
		t.Errorf("response metadata = %+v", resp)
	}
}

func TestSystemOneSendsImagesAsMultipart(t *testing.T) {
	ref := jev.NewRef()
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("TypeSafe-Danger-Zone-Images") != "true" {
			t.Errorf("images header missing")
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		var req struct {
			State  string `json:"state"`
			Assets map[string]struct {
				Type, Part string
			} `json:"assets"`
		}
		if err := json.Unmarshal([]byte(r.FormValue("request")), &req); err != nil {
			t.Fatal(err)
		}
		asset := req.Assets[ref]
		if req.State != ref || asset.Type != "image" {
			t.Fatalf("request = %+v", req)
		}
		f, hdr, err := r.FormFile(asset.Part)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(f)
		if string(data) != string(pngBytes) || hdr.Header.Get("Content-Type") != "image/png" {
			t.Errorf("image part mangled")
		}
		_, _ = io.WriteString(w, `{"model":"jev-1.14.0","answers":{"where":{"type":"box","box":{"top":0.2,"bottom":0.6,"left":0.1,"right":0.5},"confidence":null}},"usage":{"input_tokens":1,"output_tokens":1}}`)
	})

	img, err := jev.NewImage(pngBytes)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.SystemOne(context.Background(), jev.Request{
		State:     ref,
		Images:    map[string]jev.Image{ref: img},
		Questions: jev.Questions{"where": jev.Box{Instructions: "Find the square."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	box, err := resp.Box("where")
	if err != nil || box.Box.Top != 0.2 || box.Confidence != nil {
		t.Errorf("box = %+v, %v", box, err)
	}
}

func TestNewImageRejectsNonImages(t *testing.T) {
	if _, err := jev.NewImage([]byte("id_rsa contents")); err == nil {
		t.Fatal("non-image bytes were accepted")
	}
}

func TestRetriesRateLimitsThenSucceeds(t *testing.T) {
	var calls atomic.Int32
	c := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) < 3 {
			w.Header().Set("Retry-After-Ms", "1")
			w.WriteHeader(529)
			return
		}
		_, _ = io.WriteString(w, `{"models":[{"name":"jev-latest","description":"d","release_date":"2026-10-14"}]}`)
	})
	models, err := c.Models(context.Background())
	if err != nil || len(models) != 1 || calls.Load() != 3 {
		t.Fatalf("models = %v, err = %v, calls = %d", models, err, calls.Load())
	}
}

func TestDoesNotRetryValidationErrors(t *testing.T) {
	var calls atomic.Int32
	c := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, `{"detail":[{"loc":["body","questions","u","criteria"],"msg":"Field required","type":"missing"}]}`)
	})
	_, err := c.SystemOne(context.Background(), jev.Request{State: "x", Questions: jev.Questions{"u": jev.Score{}}})
	var apiErr *jev.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 422 || calls.Load() != 1 {
		t.Fatalf("err = %v, calls = %d", err, calls.Load())
	}
	if !strings.Contains(apiErr.Message, "body.questions.u.criteria: Field required") {
		t.Errorf("message = %q", apiErr.Message)
	}
}

func TestGivesUpAfterMaxRetries(t *testing.T) {
	var calls atomic.Int32
	c := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"detail":"slow down"}`)
	})
	_, err := c.Models(context.Background())
	if err == nil || !strings.Contains(err.Error(), "slow down") || calls.Load() != 3 {
		t.Fatalf("err = %v, calls = %d", err, calls.Load())
	}
}

func TestNewRequiresAPIKey(t *testing.T) {
	t.Setenv(jev.EnvAPIKey, "")
	if _, err := jev.New(); err == nil {
		t.Fatal("client created without an API key")
	}
}

func TestQuestionsRoundTripThroughJSON(t *testing.T) {
	in := `{"state":"s","questions":{
		"a":{"type":"noul","instructions":"q","criteria":{"true":"yes"}},
		"b":{"type":"choice","criteria":{"x":null,"y":"why"}},
		"c":{"type":"score","criteria":["lo","hi"]},
		"d":{"type":"box","instructions":"find"}}}`
	var req jev.Request
	if err := json.Unmarshal([]byte(in), &req); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"a": "noul", "b": "choice", "c": "score", "d": "box"} {
		if got := req.Questions[name].Type(); got != want {
			t.Errorf("%s: type %s, want %s", name, got, want)
		}
	}
	if req.Questions["a"].(jev.Noul).Criteria.True != "yes" {
		t.Errorf("noul criteria lost")
	}
	bad := `{"questions":{"z":{"type":"essay"}}}`
	if err := json.Unmarshal([]byte(bad), &req); err == nil {
		t.Errorf("unknown question type accepted")
	}
}
