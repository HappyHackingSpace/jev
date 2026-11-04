package jev

import (
	"encoding/json"
	"fmt"
)

// Question types as they appear in the type field of questions and answers.
const (
	TypeNoul   = "noul"
	TypeChoice = "choice"
	TypeScore  = "score"
	TypeBox    = "box"
)

// Question is one typed judgment about a request's state. Instructions and
// criteria accept a string, or any JSON-encodable object or array.
type Question interface {
	Type() string
}

// Questions maps the names you choose to questions. Names key the answers and
// are never shown to the model, so put the whole question in Instructions.
type Questions map[string]Question

// Noul asks a yes/no question and is answered with the probability of yes.
type Noul struct {
	Instructions any
	// Criteria optionally clarifies what counts as yes and no.
	Criteria *NoulCriteria
}

// NoulCriteria describes what counts as a yes (True) and a no (False).
type NoulCriteria struct {
	True  any `json:"true,omitempty"`
	False any `json:"false,omitempty"`
}

// Choice picks one option. Criteria maps each option name to a description of
// when it applies; a nil description lets the name speak for itself.
type Choice struct {
	Instructions any
	Criteria     map[string]any
}

// Score rates the state on an ordered rubric. The position of each level in
// Criteria is its score, starting at zero.
type Score struct {
	Instructions any
	Criteria     []any
}

// Box locates what Instructions describe in the request's image. It needs an
// image in the request and a model that supports boxes (Jev 1.14 or later).
type Box struct {
	Instructions any
}

func (Noul) Type() string   { return TypeNoul }
func (Choice) Type() string { return TypeChoice }
func (Score) Type() string  { return TypeScore }
func (Box) Type() string    { return TypeBox }

type wireQuestion struct {
	Type         string `json:"type"`
	Instructions any    `json:"instructions,omitempty"`
	Criteria     any    `json:"criteria,omitempty"`
}

func (q Noul) MarshalJSON() ([]byte, error) {
	w := wireQuestion{Type: TypeNoul, Instructions: q.Instructions}
	if q.Criteria != nil {
		w.Criteria = q.Criteria
	}
	return json.Marshal(w)
}

func (q Choice) MarshalJSON() ([]byte, error) {
	return json.Marshal(wireQuestion{Type: TypeChoice, Instructions: q.Instructions, Criteria: q.Criteria})
}

func (q Score) MarshalJSON() ([]byte, error) {
	return json.Marshal(wireQuestion{Type: TypeScore, Instructions: q.Instructions, Criteria: q.Criteria})
}

func (q Box) MarshalJSON() ([]byte, error) {
	return json.Marshal(wireQuestion{Type: TypeBox, Instructions: q.Instructions})
}

// UnmarshalJSON decodes questions written in the API's own JSON shape, so a
// request file can be loaded straight into a [Request].
func (qs *Questions) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	out := make(Questions, len(raw))
	for name, body := range raw {
		q, err := decodeQuestion(body)
		if err != nil {
			return fmt.Errorf("question %q: %w", name, err)
		}
		out[name] = q
	}
	*qs = out
	return nil
}

func decodeQuestion(body json.RawMessage) (Question, error) {
	var head struct {
		Type         string          `json:"type"`
		Instructions any             `json:"instructions"`
		Criteria     json.RawMessage `json:"criteria"`
	}
	if err := json.Unmarshal(body, &head); err != nil {
		return nil, err
	}
	hasCriteria := len(head.Criteria) > 0 && string(head.Criteria) != "null"
	switch head.Type {
	case TypeNoul:
		q := Noul{Instructions: head.Instructions}
		if hasCriteria {
			q.Criteria = &NoulCriteria{}
			if err := json.Unmarshal(head.Criteria, q.Criteria); err != nil {
				return nil, err
			}
		}
		return q, nil
	case TypeChoice:
		q := Choice{Instructions: head.Instructions}
		if hasCriteria {
			if err := json.Unmarshal(head.Criteria, &q.Criteria); err != nil {
				return nil, err
			}
		}
		return q, nil
	case TypeScore:
		q := Score{Instructions: head.Instructions}
		if hasCriteria {
			if err := json.Unmarshal(head.Criteria, &q.Criteria); err != nil {
				return nil, err
			}
		}
		return q, nil
	case TypeBox:
		return Box{Instructions: head.Instructions}, nil
	}
	return nil, fmt.Errorf("unknown question type %q", head.Type)
}
