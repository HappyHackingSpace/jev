# jev

Go SDK, CLI, MCP server and agent skill for [TypeSafe](https://typesafe.ai)'s System One API and its
model, Jev.

Jev does not write text. You send a state (text or JSON) and named questions, and every answer comes
back typed with calibrated probabilities: the probability of yes for a **noul**, a picked option for
a **choice**, a position on your rubric for a **score**, or a bounding box for a **box**.

Not affiliated with TypeSafe. The SDK is written against the
[official API reference](https://docs.typesafe.ai/api) and follows the official SDKs' defaults
(environment variables, retry policy).

## Install

```sh
go install github.com/happyhackingspace/jev/cmd/jev@latest
```

or download a binary from [releases](https://github.com/happyhackingspace/jev/releases).

Set your API key from the [TypeSafe console](https://console.typesafe.ai/keys):

```sh
export TYPESAFE_API_KEY=...
```

| Variable | Default |
| --- | --- |
| `TYPESAFE_API_KEY` | required |
| `TYPESAFE_DEFAULT_MODEL` | `jev-latest` |
| `TYPESAFE_BASE_URL` | `https://api.typesafe.ai` |

## CLI

The state comes from `-s`, `-f <file>` or stdin. JSON objects and arrays are sent as structured
state, so questions can point at fields by path (`` `ticket.subject` ``); `--raw` sends them as a
string instead. `--json` prints the full API response.

```sh
jev noul "Is this message spam?" -s "WIN A FREE CRUISE NOW"
jev choice "Which team should handle this ticket?" -f ticket.json \
  -o billing="Payments, invoices, refunds" -o technical -o other
jev score "How urgent is this message?" -s "The site is down for everyone" \
  -l "Can wait" -l "Needs attention this week" -l "Needs attention today"
jev box "Locate the license plate." --image car.jpg
jev models
```

`noul` prints the probability of yes. `choice` prints the picked option and its confidence, then
every option's probability, most likely first. `score` prints the score out of the top level and
its confidence, then each level's probability. `box` prints `top bottom left right` as fractions of
the image.

`noul --threshold` exits 0 for yes, 1 for no and 2 on errors, so a judgment can gate a script:

```sh
git diff | jev noul "Does this diff change authentication logic?" --threshold 0.7 && echo "needs review"
```

`jev ask` sends a full request in the API's JSON shape, with any number of questions of any type.
Attach images with `--image ref=path` and write the ref where the image belongs:

```sh
jev ask --image photo-1=door.jpg <<'EOF'
{"state": "photo-1", "questions": {
  "person": {"type": "noul", "instructions": "Is there a person in the photo?"},
  "where":  {"type": "box",  "instructions": "Locate the person."}}}
EOF
```

## MCP server

`jev mcp` serves two tools over stdio:

- `jev_ask`: any mix of typed questions about one state, with optional images (local PNG or JPEG
  paths; other files are refused).
- `jev_models`: the models and aliases available to the account.

```sh
claude mcp add jev -- jev mcp
```

Other MCP clients use the same command: `jev` with the argument `mcp`.

## Claude Code plugin

The plugin bundles the MCP server and a skill that teaches agents when and how to use Jev. It needs
the `jev` binary on `PATH`.

```sh
claude plugin marketplace add happyhackingspace/jev
claude plugin install jev@happyhackingspace-jev
```

To use the skill alone, copy [`skills/jev`](skills/jev) into your agent's skills directory. For
designing TypeSafe integrations in depth, TypeSafe's official
[`typesafe-ai` skill](https://github.com/typesafe-ai/skills) complements this one.

## Go SDK

```sh
go get github.com/happyhackingspace/jev
```

```go
client, err := jev.New() // reads TYPESAFE_API_KEY
if err != nil {
	return err
}
resp, err := client.SystemOne(ctx, jev.Request{
	State: map[string]any{"message": "I was charged twice. Please refund the extra charge."},
	Questions: jev.Questions{
		"billing": jev.Noul{Instructions: "Is `message` about billing?"},
		"tone": jev.Choice{
			Instructions: "What is the tone of `message`?",
			Criteria:     map[string]any{"angry": "Upset or hostile", "calm": nil},
		},
		"urgency": jev.Score{
			Instructions: "How urgent is `message`?",
			Criteria:     []any{"Can wait", "This week", "Today"},
		},
	},
})
if err != nil {
	return err
}
billing, _ := resp.Noul("billing")    // float64, probability of yes
tone, _ := resp.Choice("tone")        // tone.Choice, *tone.Confidence, tone.Probabilities
urgency, _ := resp.Score("urgency")   // *urgency.Score, urgency.Legend
```

Images go anywhere a string can, by reference:

```go
img, err := jev.ReadImage("door.jpg") // PNG or JPEG only
ref := jev.NewRef()
resp, err := client.SystemOne(ctx, jev.Request{
	State:     ref,
	Images:    map[string]jev.Image{ref: img},
	Questions: jev.Questions{"person": jev.Box{Instructions: "Locate the person."}},
})
```

Images are uploaded as multipart parts, and the `TypeSafe-Danger-Zone-Images` header is set
automatically when a request has images or box questions. `Request.LongContext` sets the long
context header.

Options: `WithAPIKey`, `WithModel`, `WithBaseURL`, `WithTimeout`, `WithHTTPClient`, `WithRetry`,
`WithUserAgent`. By default the client retries 408, 429 and 5xx (including 529) twice with jittered
exponential backoff, honouring `Retry-After-Ms` and `Retry-After`. API errors are `*jev.APIError`.

## License

[MIT](LICENSE)
