package jev

import "fmt"

// Response holds the answers to one request, keyed by question name.
type Response struct {
	// Model is the versioned model that answered, even when the request
	// named an alias such as jev-latest.
	Model   string            `json:"model"`
	Answers map[string]Answer `json:"answers"`
	Usage   Usage             `json:"usage"`
}

// Usage reports token counts. Only input tokens are billed.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Answer is one typed answer. Type says which fields are set.
type Answer struct {
	Type string `json:"type"`
	// Noul is the probability of yes, from 0 to 1.
	Noul *float64 `json:"noul,omitempty"`
	// Choice is the option with the highest probability.
	Choice string `json:"choice,omitempty"`
	// Score is the probability-weighted level and may fall between levels.
	Score *float64 `json:"score,omitempty"`
	// Confidence is how sure the model is of a choice, score or box. It is
	// nil for nouls, and for boxes when the model gives none.
	Confidence *float64 `json:"confidence,omitempty"`
	// Probabilities maps each option, or each level index for a score, to
	// its probability.
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	// Legend maps each score level index to the criterion it stands for.
	Legend map[string]any  `json:"legend,omitempty"`
	Box    *BoxCoordinates `json:"box,omitempty"`
}

// BoxCoordinates are the edges of a box as fractions of the image, measured
// from the top and from the left.
type BoxCoordinates struct {
	Top    float64 `json:"top"`
	Bottom float64 `json:"bottom"`
	Left   float64 `json:"left"`
	Right  float64 `json:"right"`
}

// Answer returns the named answer and checks that it has the wanted type.
func (r *Response) Answer(name, wantType string) (Answer, error) {
	a, ok := r.Answers[name]
	if !ok {
		return Answer{}, fmt.Errorf("no answer named %q", name)
	}
	if a.Type != wantType {
		return Answer{}, fmt.Errorf("answer %q is a %s, not a %s", name, a.Type, wantType)
	}
	return a, nil
}

// Noul returns the probability of yes for the named noul question.
func (r *Response) Noul(name string) (float64, error) {
	a, err := r.Answer(name, TypeNoul)
	if err != nil {
		return 0, err
	}
	if a.Noul == nil {
		return 0, fmt.Errorf("answer %q has no noul value", name)
	}
	return *a.Noul, nil
}

// Choice returns the named choice answer.
func (r *Response) Choice(name string) (Answer, error) { return r.Answer(name, TypeChoice) }

// Score returns the named score answer.
func (r *Response) Score(name string) (Answer, error) { return r.Answer(name, TypeScore) }

// Box returns the named box answer.
func (r *Response) Box(name string) (Answer, error) { return r.Answer(name, TypeBox) }

// Model describes a model or alias available to the account.
type Model struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ReleaseDate string `json:"release_date"`
}
