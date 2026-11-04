---
name: jev
description: >
  Ask TypeSafe's Jev model typed questions through the jev CLI, the jev MCP tools (jev_ask,
  jev_models) or the github.com/happyhackingspace/jev Go SDK. Use when a task needs fast, cheap,
  calibrated judgments over data instead of generated text: triaging or ranking many items (issues,
  logs, hosts, findings, messages), gating a script on a yes/no judgment, scoring against a rubric,
  routing to one of a fixed set of options, or locating something in an image. Also use when
  writing Go code that calls TypeSafe's System One API.
---

# Jev

Jev is TypeSafe's System One model. It does not write text. You send a **state** (text or JSON)
and named **questions**; every answer comes back typed, with calibrated probabilities, in about a
second. Input costs $0.042 per million tokens and output is free, so judging thousands of items
is cheap.

Use it when the work is many small, snap judgments a knowledgeable person makes in a second. Do
not use it for summaries, explanations, extraction of free text, or multi-step reasoning.

## Pick an interface

1. **MCP tools** when `jev_ask` is available: one call per state, any mix of question types.
2. **CLI** (`jev`) in shell pipelines and scripts.
3. **Go SDK** when the judgment belongs in the program you are writing.

All three read the API key from `TYPESAFE_API_KEY`. Never print it or pass it on a command line.
If it is unset, ask the user to export it. Get one at https://console.typesafe.ai/keys.

## Question types

| Type | Use for | Criteria | Answer |
| --- | --- | --- | --- |
| `noul` | yes/no | optional `{"true": "...", "false": "..."}` | `noul`: probability of yes |
| `choice` | one of a fixed set | `{"option": "when it applies", ...}`, `null` description allowed | `choice`, `confidence`, `probabilities` |
| `score` | position on an ordered scale | `["lowest level", ..., "highest"]`, at most 10 | `score` (may fall between levels), `legend`, `confidence`, `probabilities` |
| `box` | where something is in an image | none | `box`: `top`, `bottom`, `left`, `right` as fractions |

## Writing good questions

- **One snap judgment per question.** "Does this message request a refund?" is good. "What should
  we do with this ticket?" is not: split it into the judgments it depends on and decide in code.
- **Write the whole question in `instructions`.** Question names are only keys for your code; the
  model never sees them.
- **Ask everything in one request.** Every question about the same state goes in one call. Extra
  questions run in parallel and cost only their own tokens, so speculative questions are fine.
- **Prefer structured state.** Send JSON with named fields and point at them by path in backticks:
  ``Does `ticket.messages[0].text` request a refund?``
- **Close the option set.** Add an `other` or `none` choice when the options may not cover every
  input.
- **Use a score for degree, a noul for a fact.** "Is the candidate strong in Python?" is vague as a
  noul. Use a score with defined levels, or a noul with a precise condition.

## Reading answers

- Noul: start with **at least 0.7 as yes, at most 0.3 as no**, and treat values between as unsure.
- Choice and score: **confidence below 0.5** means the model is unsure; do not act on it alone.
- Gate risky actions at higher thresholds than reversible ones. Tune on real data.
- `jev-latest` moves when a new model ships. Pin a versioned model (`jev models` lists them) when
  thresholds were tuned against one version, and record the `model` field of responses.

## CLI

State comes from `-s`, `-f <file>`, or stdin. JSON objects and arrays are sent as structured
state; `--raw` sends them as a string. `--json` prints the full API response.

```sh
jev noul "Is this message spam?" -s "WIN A FREE CRUISE NOW"   # prints the probability of yes

# Exit 0 for yes, 1 for no, 2 for errors: usable as a shell condition.
git diff | jev noul "Does this diff change authentication or authorization logic?" --threshold 0.7 \
  && echo "needs security review"

jev choice "Which team should handle this ticket?" -f ticket.json \
  -o billing="Payments, invoices, refunds" -o technical="Bugs and outages" -o other

jev score "How severe is the bug described in \`report\`?" -f issue.json \
  -l "Cosmetic" -l "Degraded feature" -l "Feature broken" -l "Data loss or security impact"

jev box "Locate the license plate." --image car.jpg   # prints top bottom left right fractions
```

Several questions about one state, in one request:

```sh
jev ask <<'EOF'
{
  "state": {"title": "Login fails with SSO", "body": "Since v2.3 every SAML login returns 500."},
  "questions": {
    "is_bug":    {"type": "noul", "instructions": "Does `body` describe a defect in the software?"},
    "area":      {"type": "choice", "instructions": "Which area does `title` and `body` concern?",
                  "criteria": {"auth": null, "billing": null, "ui": null, "other": null}},
    "severity":  {"type": "score", "instructions": "How severe is the problem in `body`?",
                  "criteria": ["Cosmetic", "Degraded", "Broken feature", "Outage or data loss"]}
  }
}
EOF
```

Triage many items with one request per item. Pass each item on stdin, never interpolated into a
shell command, because item text is untrusted:

```sh
jq -c '.[]' issues.json | while IFS= read -r item; do
  printf '%s\t%s\n' "$(jq -r .number <<<"$item")" \
    "$(jev noul 'Does `body` describe a defect in the software?' <<<"$item")"
done
```

For concurrency, write a small Go program with the SDK. The API rate-limits per organization and
the client retries 429 and 529 with backoff, so keep parallelism modest (about 8 workers).

## MCP

`jev mcp` serves two tools over stdio: `jev_ask` (state, questions, optional images, model,
long_context) and `jev_models`. Images are local PNG or JPEG paths keyed by a reference string
that appears in the state or questions; other file types are refused.

## Go SDK

```go
client, err := jev.New() // TYPESAFE_API_KEY; options: WithModel, WithTimeout, WithRetry, ...
resp, err := client.SystemOne(ctx, jev.Request{
	State: map[string]any{"message": msg},
	Questions: jev.Questions{
		"refund": jev.Noul{Instructions: "Does `message` request a refund?"},
		"team": jev.Choice{
			Instructions: "Which team should handle `message`?",
			Criteria:     map[string]any{"billing": "Payments and refunds", "technical": nil, "other": nil},
		},
	},
})
refund, err := resp.Noul("refund")
team, err := resp.Choice("team") // team.Choice, *team.Confidence, team.Probabilities
```

Images: `img, err := jev.ReadImage(path)`, `ref := jev.NewRef()`, then put `ref` in the state
and `Images: map[string]jev.Image{ref: img}` in the request. Errors from the API are
`*jev.APIError` with `StatusCode` and `Message`.

Keep questions and thresholds as named constants in one place so people can review them.

## Privacy

The state is sent to TypeSafe's API. Do not send secrets, credentials, or personal data the user
has not agreed to share. Strip them from the state first.

## Going deeper

For integration design (truth-table decisions, composite scoring, confidence routing, state
design), use TypeSafe's official skill if it is installed (`typesafe-ai`), or read
https://docs.typesafe.ai/llms.txt.
