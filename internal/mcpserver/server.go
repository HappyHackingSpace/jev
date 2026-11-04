// Package mcpserver exposes Jev as Model Context Protocol tools.
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/happyhackingspace/jev"
)

// ClientFunc returns the client a tool call uses. Tools build it per call so
// the server still starts, and explains itself, when no API key is set.
type ClientFunc func() (*jev.Client, error)

// New returns an MCP server with the Jev tools registered.
func New(version string, client ClientFunc) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "jev", Title: "Jev (TypeSafe System One)", Version: version}, &mcp.ServerOptions{
		Instructions: instructions,
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "jev_ask",
		Title:       "Ask Jev typed questions",
		Description: askDescription,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(true)},
	}, askHandler(client))
	mcp.AddTool(s, &mcp.Tool{
		Name:        "jev_models",
		Title:       "List Jev models",
		Description: "List the TypeSafe models and aliases available to the account, with release dates.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(true)},
	}, modelsHandler(client))
	return s
}

const instructions = `Jev is TypeSafe's System One model. It answers typed questions about a state with calibrated probabilities instead of text: fast (about a second), cheap, and constrained to the options you give. Use it for snap judgments that code can act on: classify, gate, triage, score or locate. Do not use it for anything that needs generated text or multi-step reasoning. Put every question about the same state in one jev_ask call; extra questions are nearly free.`

const askDescription = `Ask Jev one or more typed questions about a state, in one request.

state: the content to judge, as a string or a JSON object/array. Prefer JSON with named fields and refer to them in instructions by path in backticks, e.g. ` + "`ticket.subject`" + `.

questions: map of your own names to questions. Names are never shown to the model, so write the full question in instructions. Types:
- noul: yes/no. criteria optional {"true": "...", "false": "..."}. Answer: noul = probability of yes (0-1).
- choice: pick one. criteria is an object of option name -> description (null to let the name speak). Add an "other" option when the list may not cover every input. Answer: choice, confidence, probabilities.
- score: ordered rubric. criteria is an array of level descriptions, lowest first (max 10). Answer: score (may fall between levels), legend, confidence, probabilities.
- box: locate something in an image (needs an image). Answer: box {top,bottom,left,right} as fractions.

images: optional map of reference -> local PNG/JPEG path. Write the reference wherever the image belongs in state, instructions or criteria. Use references that cannot occur in normal text.

Ask one focused judgment per question; split complex judgments into several questions and combine them yourself. Treat confidence below 0.5 as the model being unsure. For a noul, start with at least 0.7 as yes and at most 0.3 as no, and treat values between as unsure.`

type askInput struct {
	State       any                      `json:"state" jsonschema:"the content to judge: a string, or a JSON object or array"`
	Questions   map[string]questionInput `json:"questions" jsonschema:"questions keyed by names you choose; answers use the same names"`
	Images      map[string]string        `json:"images,omitempty" jsonschema:"image reference to local PNG or JPEG file path"`
	Model       string                   `json:"model,omitempty" jsonschema:"model or alias, e.g. jev-latest or jev-1.14.0"`
	LongContext bool                     `json:"long_context,omitempty" jsonschema:"raise the context limit for large states; slower and less accurate"`
}

type questionInput struct {
	Type         string `json:"type" jsonschema:"noul, choice, score or box"`
	Instructions any    `json:"instructions,omitempty" jsonschema:"the full question or statement to judge"`
	Criteria     any    `json:"criteria,omitempty" jsonschema:"noul: {true,false} descriptions; choice: {option: description}; score: [levels lowest first]; box: omit"`
}

func askHandler(client ClientFunc) mcp.ToolHandlerFor[askInput, *jev.Response] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in askInput) (*mcp.CallToolResult, *jev.Response, error) {
		req, err := in.request()
		if err != nil {
			return nil, nil, err
		}
		c, err := client()
		if err != nil {
			return nil, nil, err
		}
		resp, err := c.SystemOne(ctx, req)
		if err != nil {
			return nil, nil, err
		}
		return nil, resp, nil
	}
}

// request reuses the SDK's decoder for the API's question shape, so the tool
// accepts exactly what the API documents.
func (in askInput) request() (jev.Request, error) {
	if in.State == nil {
		return jev.Request{}, fmt.Errorf("state is required")
	}
	raw, err := json.Marshal(in.Questions)
	if err != nil {
		return jev.Request{}, err
	}
	var qs jev.Questions
	if err := json.Unmarshal(raw, &qs); err != nil {
		return jev.Request{}, err
	}
	req := jev.Request{Model: in.Model, State: in.State, Questions: qs, LongContext: in.LongContext}
	for ref, path := range in.Images {
		img, err := jev.ReadImage(path)
		if err != nil {
			return jev.Request{}, err
		}
		if req.Images == nil {
			req.Images = map[string]jev.Image{}
		}
		req.Images[ref] = img
	}
	return req, nil
}

type modelsOutput struct {
	Models []jev.Model `json:"models"`
}

func modelsHandler(client ClientFunc) mcp.ToolHandlerFor[struct{}, modelsOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, modelsOutput, error) {
		c, err := client()
		if err != nil {
			return nil, modelsOutput{}, err
		}
		models, err := c.Models(ctx)
		if err != nil {
			return nil, modelsOutput{}, err
		}
		return nil, modelsOutput{Models: models}, nil
	}
}

func ptr[T any](v T) *T { return &v }
