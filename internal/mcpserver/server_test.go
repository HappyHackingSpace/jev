package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/happyhackingspace/jev"
)

func connect(t *testing.T, client ClientFunc) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := New("test", client).Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func TestAskReturnsStructuredAnswers(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = io.WriteString(w, `{"model":"jev-1.14.0","answers":{
			"spam":{"type":"noul","noul":0.97},
			"lang":{"type":"choice","choice":"en","confidence":0.8,"probabilities":{"en":0.9,"other":0.1}}},
			"usage":{"input_tokens":9,"output_tokens":2}}`)
	}))
	defer srv.Close()
	cs := connect(t, func() (*jev.Client, error) { return jev.New(jev.WithAPIKey("k"), jev.WithBaseURL(srv.URL)) })

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "jev_ask",
		Arguments: map[string]any{
			"state": map[string]any{"message": "WIN A FREE CRUISE"},
			"questions": map[string]any{
				"spam": map[string]any{"type": "noul", "instructions": "Is `message` spam?"},
				"lang": map[string]any{"type": "choice", "instructions": "Language of `message`?", "criteria": map[string]any{"en": nil, "other": nil}},
			},
		},
	})
	if err != nil || res.IsError {
		t.Fatalf("err = %v, result = %+v", err, res)
	}
	raw, _ := json.Marshal(res.StructuredContent)
	var resp jev.Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatal(err)
	}
	if p, _ := resp.Noul("spam"); p != 0.97 {
		t.Errorf("spam = %v", p)
	}
	if c, _ := resp.Choice("lang"); c.Choice != "en" {
		t.Errorf("lang = %+v", c)
	}
	if got["state"].(map[string]any)["message"] != "WIN A FREE CRUISE" {
		t.Errorf("state sent = %v", got["state"])
	}
}

func TestAskRefusesNonImageFiles(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(secret, []byte("not an image"), 0o600); err != nil {
		t.Fatal(err)
	}
	cs := connect(t, func() (*jev.Client, error) { return nil, errors.New("must not be called") })

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "jev_ask",
		Arguments: map[string]any{
			"state":     "ref-1",
			"images":    map[string]any{"ref-1": secret},
			"questions": map[string]any{"q": map[string]any{"type": "noul", "instructions": "?"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(textOf(res), "only PNG and JPEG") {
		t.Fatalf("result = %s", textOf(res))
	}
}

func TestMissingKeyIsReportedAsToolError(t *testing.T) {
	cs := connect(t, func() (*jev.Client, error) { return nil, errors.New("typesafe: no API key") })
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "jev_models", Arguments: map[string]any{}})
	if err != nil || !res.IsError || !strings.Contains(textOf(res), "no API key") {
		t.Fatalf("err = %v, result = %s", err, textOf(res))
	}
}

func TestToolsAreListed(t *testing.T) {
	cs := connect(t, func() (*jev.Client, error) { return nil, errors.New("unused") })
	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range tools.Tools {
		names[tool.Name] = true
	}
	if !names["jev_ask"] || !names["jev_models"] || len(names) != 2 {
		t.Fatalf("tools = %v", names)
	}
}

func textOf(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}
