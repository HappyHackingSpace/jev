// Package cli implements the jev command.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/happyhackingspace/jev"
)

// Exit codes follow grep: a noul under its threshold is a clean "no", which
// scripts must be able to tell apart from a failure.
const (
	exitYes   = 0
	exitNo    = 1
	exitError = 2
)

// exitCode ends the command with a code other than exitError.
type exitCode int

func (e exitCode) Error() string { return fmt.Sprintf("exit %d", int(e)) }

type app struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer

	model       string
	timeout     time.Duration
	longContext bool
	jsonOut     bool
}

// Execute runs the command line and returns the process exit code.
func Execute(ctx context.Context, version string, args []string) int {
	jev.Version = version
	a := &app{stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr}
	root := a.rootCmd(version)
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	var code exitCode
	switch {
	case err == nil:
		return exitYes
	case errors.As(err, &code):
		return int(code)
	default:
		fmt.Fprintln(a.stderr, "jev:", err)
		return exitError
	}
}

func (a *app) rootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:   "jev",
		Short: "Ask TypeSafe's Jev typed questions from the command line",
		Long: `jev asks TypeSafe's System One model, Jev, typed questions about a state
and prints calibrated answers: a probability, a picked option, a score or a box.

The API key is read from TYPESAFE_API_KEY.`,
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetIn(a.stdin)
	root.SetOut(a.stdout)
	root.SetErr(a.stderr)

	f := root.PersistentFlags()
	f.StringVarP(&a.model, "model", "m", "", "model or alias (default $"+jev.EnvModel+" or "+jev.ModelLatest+")")
	f.DurationVar(&a.timeout, "timeout", jev.DefaultTimeout, "timeout of each HTTP attempt")
	f.BoolVar(&a.longContext, "long-context", false, "raise the context limit (slower, less accurate)")
	f.BoolVar(&a.jsonOut, "json", false, "print the full API response as JSON")

	root.AddCommand(
		a.noulCmd(), a.choiceCmd(), a.scoreCmd(), a.boxCmd(),
		a.askCmd(), a.modelsCmd(), a.mcpCmd(),
	)
	return root
}

func (a *app) client() (*jev.Client, error) {
	opts := []jev.Option{jev.WithTimeout(a.timeout), jev.WithUserAgent("jev-cli")}
	if a.model != "" {
		opts = append(opts, jev.WithModel(a.model))
	}
	return jev.New(opts...)
}

func (a *app) modelsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "models",
		Short: "List the models and aliases available to your account",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := a.client()
			if err != nil {
				return err
			}
			models, err := c.Models(cmd.Context())
			if err != nil {
				return err
			}
			if a.jsonOut {
				return writeJSON(a.stdout, map[string]any{"models": models})
			}
			tw := newTable(a.stdout)
			for _, m := range models {
				fmt.Fprintf(tw, "%s\t%s\t%s\n", m.Name, m.ReleaseDate, m.Description)
			}
			return tw.Flush()
		},
	}
}
