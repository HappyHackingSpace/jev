package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/happyhackingspace/jev"
)

// answerName keys the single question each one-shot command sends.
const answerName = "answer"

// ask sends one question about the state the flags describe.
func (a *app) ask(ctx context.Context, s *stateFlags, q jev.Question) (*jev.Response, jev.Answer, error) {
	state, images, err := s.build(a.stdin)
	if err != nil {
		return nil, jev.Answer{}, err
	}
	c, err := a.client()
	if err != nil {
		return nil, jev.Answer{}, err
	}
	resp, err := c.SystemOne(ctx, jev.Request{
		State:       state,
		Images:      images,
		Questions:   jev.Questions{answerName: q},
		LongContext: a.longContext,
	})
	if err != nil {
		return nil, jev.Answer{}, err
	}
	answer, err := resp.Answer(answerName, q.Type())
	return resp, answer, err
}

func (a *app) noulCmd() *cobra.Command {
	var (
		s         stateFlags
		yes, no   string
		threshold float64
	)
	cmd := &cobra.Command{
		Use:   "noul <question>",
		Short: "Ask a yes/no question and print the probability of yes",
		Example: `  jev noul "Is this message spam?" -s "WIN A FREE CRUISE NOW"
  git diff | jev noul "Does this diff change authentication logic?" --threshold 0.8 && echo review`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if threshold < 0 || threshold > 1 {
				return errors.New("--threshold must be between 0 and 1")
			}
			q := jev.Noul{Instructions: args[0]}
			if yes != "" || no != "" {
				q.Criteria = &jev.NoulCriteria{}
				if yes != "" {
					q.Criteria.True = yes
				}
				if no != "" {
					q.Criteria.False = no
				}
			}
			resp, answer, err := a.ask(cmd.Context(), &s, q)
			if err != nil {
				return err
			}
			if a.jsonOut {
				err = writeJSON(a.stdout, resp)
			} else {
				_, err = fmt.Fprintf(a.stdout, "%.4f\n", *answer.Noul)
			}
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("threshold") && *answer.Noul < threshold {
				return exitCode(exitNo)
			}
			return nil
		},
	}
	s.register(cmd)
	cmd.Flags().StringVar(&yes, "yes", "", "what counts as a yes")
	cmd.Flags().StringVar(&no, "no", "", "what counts as a no")
	cmd.Flags().Float64Var(&threshold, "threshold", 0.5, "exit 0 when the probability is at least this, 1 otherwise")
	return cmd
}

func (a *app) choiceCmd() *cobra.Command {
	var (
		s       stateFlags
		options []string
	)
	cmd := &cobra.Command{
		Use:   "choice <question>",
		Short: "Pick one option and print it with its confidence and probabilities",
		Example: `  jev choice "Which team should handle this ticket?" -f ticket.json \
    -o billing="Payments, invoices, refunds" -o technical -o other`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			criteria := make(map[string]any, len(options))
			for _, o := range options {
				name, desc, hasDesc := strings.Cut(o, "=")
				name = strings.TrimSpace(name)
				if name == "" {
					return fmt.Errorf("option %q has no name", o)
				}
				criteria[name] = nil
				if hasDesc {
					criteria[name] = desc
				}
			}
			resp, answer, err := a.ask(cmd.Context(), &s, jev.Choice{Instructions: args[0], Criteria: criteria})
			if err != nil {
				return err
			}
			if a.jsonOut {
				return writeJSON(a.stdout, resp)
			}
			fmt.Fprintf(a.stdout, "%s\t(confidence %s)\n", answer.Choice, formatConfidence(answer.Confidence))
			return printProbabilities(a, answer.Probabilities, func(k string) string { return k })
		},
	}
	s.register(cmd)
	cmd.Flags().StringArrayVarP(&options, "option", "o", nil, "an option as name or name=description (repeatable)")
	_ = cmd.MarkFlagRequired("option")
	return cmd
}

func (a *app) scoreCmd() *cobra.Command {
	var (
		s      stateFlags
		levels []string
	)
	cmd := &cobra.Command{
		Use:   "score <question>",
		Short: "Rate the state on ordered levels, lowest first",
		Example: `  jev score "How urgent is this message?" -s "The site is down for everyone" \
    -l "Can wait" -l "Needs attention this week" -l "Needs attention today"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			criteria := make([]any, len(levels))
			for i, l := range levels {
				criteria[i] = l
			}
			resp, answer, err := a.ask(cmd.Context(), &s, jev.Score{Instructions: args[0], Criteria: criteria})
			if err != nil {
				return err
			}
			if a.jsonOut {
				return writeJSON(a.stdout, resp)
			}
			fmt.Fprintf(a.stdout, "%.2f / %d\t(confidence %s)\n", *answer.Score, len(levels)-1, formatConfidence(answer.Confidence))
			return printProbabilities(a, answer.Probabilities, func(k string) string {
				return k + " " + fmt.Sprint(answer.Legend[k])
			})
		},
	}
	s.register(cmd)
	cmd.Flags().StringArrayVarP(&levels, "level", "l", nil, "a level description, lowest first (repeatable)")
	_ = cmd.MarkFlagRequired("level")
	return cmd
}

func (a *app) boxCmd() *cobra.Command {
	var s stateFlags
	cmd := &cobra.Command{
		Use:     "box <what to find>",
		Short:   "Locate something in an image and print its box as top bottom left right fractions",
		Example: `  jev box "Locate the license plate." --image car.jpg`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if s.image == "" {
				return errors.New("box needs --image")
			}
			resp, answer, err := a.ask(cmd.Context(), &s, jev.Box{Instructions: args[0]})
			if err != nil {
				return err
			}
			if a.jsonOut {
				return writeJSON(a.stdout, resp)
			}
			b := answer.Box
			_, err = fmt.Fprintf(a.stdout, "%.4f %.4f %.4f %.4f\t(confidence %s)\n", b.Top, b.Bottom, b.Left, b.Right, formatConfidence(answer.Confidence))
			return err
		},
	}
	s.register(cmd)
	return cmd
}

// printProbabilities lists options from most to least likely.
func printProbabilities(a *app, probs map[string]float64, label func(string) string) error {
	keys := slices.SortedFunc(maps.Keys(probs), func(x, y string) int {
		return cmp.Or(cmp.Compare(probs[y], probs[x]), strings.Compare(x, y))
	})
	tw := newTable(a.stdout)
	for _, k := range keys {
		fmt.Fprintf(tw, "  %.4f\t%s\n", probs[k], label(k))
	}
	return tw.Flush()
}

func formatConfidence(c *float64) string {
	if c == nil {
		return "n/a"
	}
	return strconv.FormatFloat(*c, 'f', 2, 64)
}
