package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/happyhackingspace/jev"
)

func (a *app) askCmd() *cobra.Command {
	var images []string
	cmd := &cobra.Command{
		Use:   "ask [request.json]",
		Short: "Send a full request with any number of questions and print the response",
		Long: `ask sends a request written in the System One API's JSON shape: state,
questions and an optional model. Read it from a file, or from stdin when no
file is given or the file is "-".

Images are attached with --image ref=path. Write the ref anywhere a string can
go in the request and the model sees the image there.`,
		Example: `  jev ask request.json
  jev ask --image photo-1=door.jpg <<'EOF'
  {"state": "photo-1", "questions": {
    "person": {"type": "noul", "instructions": "Is there a person in the photo?"},
    "where":  {"type": "box", "instructions": "Locate the person."}}}
  EOF`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			src := a.stdin
			if len(args) == 1 && args[0] != "-" {
				f, err := os.Open(args[0])
				if err != nil {
					return err
				}
				defer func() { _ = f.Close() }()
				src = f
			}
			var req jev.Request
			dec := json.NewDecoder(src)
			dec.UseNumber()
			if err := dec.Decode(&req); err != nil {
				return fmt.Errorf("decode request: %w", err)
			}
			if req.State == nil {
				return fmt.Errorf("request has no state")
			}
			for _, spec := range images {
				ref, path, ok := strings.Cut(spec, "=")
				if !ok || ref == "" || path == "" {
					return fmt.Errorf("--image %q: want ref=path", spec)
				}
				img, err := jev.ReadImage(path)
				if err != nil {
					return err
				}
				if req.Images == nil {
					req.Images = map[string]jev.Image{}
				}
				req.Images[ref] = img
			}
			req.LongContext = req.LongContext || a.longContext

			c, err := a.client()
			if err != nil {
				return err
			}
			resp, err := c.SystemOne(cmd.Context(), req)
			if err != nil {
				return err
			}
			return writeJSON(a.stdout, resp)
		},
	}
	cmd.Flags().StringArrayVar(&images, "image", nil, "attach an image as ref=path (repeatable)")
	return cmd
}
