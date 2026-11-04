package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/happyhackingspace/jev"
)

// fakeAPI records the last request body and answers with the given JSON.
func fakeAPI(t *testing.T, answer string) *map[string]any {
	t.Helper()
	got := map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = io.WriteString(w, `{"model":"jev-1.14.0","answers":{"answer":`+answer+`},"usage":{"input_tokens":5,"output_tokens":1}}`)
	}))
	t.Cleanup(srv.Close)
	t.Setenv(jev.EnvAPIKey, "test-key")
	t.Setenv(jev.EnvBaseURL, srv.URL)
	return &got
}

func run(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	a := &app{stdout: &out, stderr: io.Discard}
	if stdin != "" {
		a.stdin = strings.NewReader(stdin)
	}
	root := a.rootCmd("test")
	root.SetArgs(args)
	err := root.ExecuteContext(context.Background())
	return out.String(), err
}

func TestNoulThresholdSetsExitCode(t *testing.T) {
	fakeAPI(t, `{"type":"noul","noul":0.2}`)

	out, err := run(t, "", "noul", "Is this spam?", "-s", "hello friend")
	if err != nil || out != "0.2000\n" {
		t.Fatalf("out = %q, err = %v", out, err)
	}
	_, err = run(t, "", "noul", "Is this spam?", "-s", "hello", "--threshold", "0.5")
	var code exitCode
	if !errors.As(err, &code) || code != exitNo {
		t.Fatalf("below threshold: err = %v, want exit %d", err, exitNo)
	}
	if _, err = run(t, "", "noul", "Is this spam?", "-s", "hello", "--threshold", "0.1"); err != nil {
		t.Fatalf("above threshold: err = %v", err)
	}
}

func TestChoiceParsesOptionsAndSortsProbabilities(t *testing.T) {
	got := fakeAPI(t, `{"type":"choice","choice":"billing","confidence":0.9,"probabilities":{"other":0.05,"billing":0.9,"technical":0.05}}`)

	out, err := run(t, "", "choice", "Which team?", "-s", "refund please", "-o", "billing=Payments, refunds", "-o", "technical", "-o", "other")
	if err != nil {
		t.Fatal(err)
	}
	q := (*got)["questions"].(map[string]any)[answerName].(map[string]any)
	criteria := q["criteria"].(map[string]any)
	if criteria["billing"] != "Payments, refunds" || criteria["technical"] != nil || len(criteria) != 3 {
		t.Errorf("criteria = %v", criteria)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if !strings.HasPrefix(lines[0], "billing") || !strings.Contains(lines[1], "billing") || !strings.Contains(lines[2], "other") {
		t.Errorf("output = %q", out)
	}
}

func TestJSONStdinIsSentAsStructuredState(t *testing.T) {
	got := fakeAPI(t, `{"type":"score","score":1.5,"confidence":0.6,"legend":{"0":"low","1":"mid","2":"high"},"probabilities":{"0":0.1,"1":0.3,"2":0.6}}`)

	_, err := run(t, `{"ticket": {"id": 12345678901234567890, "text": "down"}}`, "score", "Urgency of `ticket.text`?", "-l", "low", "-l", "mid", "-l", "high")
	if err != nil {
		t.Fatal(err)
	}
	ticket := (*got)["state"].(map[string]any)["ticket"].(map[string]any)
	if ticket["text"] != "down" {
		t.Errorf("state = %v", (*got)["state"])
	}

	_, err = run(t, `{"a": 1}`, "score", "q", "-l", "x", "--raw")
	if err != nil {
		t.Fatal(err)
	}
	if (*got)["state"] != `{"a": 1}` {
		t.Errorf("--raw state = %v", (*got)["state"])
	}
}

func TestAskSendsRequestFromStdin(t *testing.T) {
	got := fakeAPI(t, `{"type":"noul","noul":0.9}`)

	out, err := run(t, `{"model":"jev-1.13.0","state":"x","questions":{"answer":{"type":"noul","instructions":"q"}}}`, "ask")
	if err != nil {
		t.Fatal(err)
	}
	if (*got)["model"] != "jev-1.13.0" {
		t.Errorf("model = %v", (*got)["model"])
	}
	var resp jev.Response
	if err := json.Unmarshal([]byte(out), &resp); err != nil || *resp.Answers["answer"].Noul != 0.9 {
		t.Errorf("out = %q, err = %v", out, err)
	}
}

func TestMissingStateIsAnError(t *testing.T) {
	fakeAPI(t, `{"type":"noul","noul":0.9}`)
	if _, err := run(t, "", "noul", "q"); err == nil || !strings.Contains(err.Error(), "no state") {
		t.Fatalf("err = %v", err)
	}
}
