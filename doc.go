// Package jev is a Go client for TypeSafe's System One API and its model, Jev.
//
// Jev does not generate text. A request carries a state (text or JSON) and one
// or more named questions, and every answer comes back typed: a probability for
// a [Noul], a picked option for a [Choice], a position on a rubric for a
// [Score], or a bounding box for a [Box].
//
//	client, err := jev.New() // reads TYPESAFE_API_KEY
//	resp, err := client.SystemOne(ctx, jev.Request{
//		State: "I was charged twice. Please refund the extra charge.",
//		Questions: jev.Questions{
//			"billing": jev.Noul{Instructions: "Is this message about billing?"},
//			"urgency": jev.Score{
//				Instructions: "How urgent is this message?",
//				Criteria:     []any{"Can wait", "This week", "Today"},
//			},
//		},
//	})
//	billing, err := resp.Noul("billing")
//
// The API reference lives at https://docs.typesafe.ai/api.
package jev
