package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/happyhackingspace/jev"
)

// stateFlags are shared by every single-question command.
type stateFlags struct {
	text  string
	file  string
	raw   bool
	image string
}

func (s *stateFlags) register(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVarP(&s.text, "state", "s", "", "state as text or JSON")
	f.StringVarP(&s.file, "file", "f", "", `read the state from a file ("-" for stdin)`)
	f.BoolVar(&s.raw, "raw", false, "send JSON input as a plain string instead of structured state")
	f.StringVar(&s.image, "image", "", "attach a PNG or JPEG `path`; it becomes the state, or the image field beside text when a state is also given")
	cmd.MarkFlagsMutuallyExclusive("state", "file")
}

// build resolves the flags into a request's state and images. Without -s or
// -f the state is read from stdin when stdin is piped.
func (s *stateFlags) build(stdin io.Reader) (any, map[string]jev.Image, error) {
	var input []byte
	hasText := true
	switch {
	case s.text != "":
		input = []byte(s.text)
	case s.file == "-":
		b, err := io.ReadAll(stdin)
		if err != nil {
			return nil, nil, err
		}
		input = b
	case s.file != "":
		b, err := os.ReadFile(s.file)
		if err != nil {
			return nil, nil, err
		}
		input = b
	case isPiped(stdin):
		b, err := io.ReadAll(stdin)
		if err != nil {
			return nil, nil, err
		}
		input = b
	default:
		hasText = false
	}

	var state any
	if hasText {
		var err error
		if state, err = s.parse(input); err != nil {
			return nil, nil, err
		}
	}
	if s.image == "" {
		if !hasText {
			return nil, nil, errors.New("no state: pass -s, -f, --image, or pipe it on stdin")
		}
		return state, nil, nil
	}

	img, err := jev.ReadImage(s.image)
	if err != nil {
		return nil, nil, err
	}
	ref := jev.NewRef()
	images := map[string]jev.Image{ref: img}
	if !hasText {
		return ref, images, nil
	}
	return map[string]any{"text": state, "image": ref}, images, nil
}

// parse sends JSON objects and arrays as structured state, which lets
// questions point at fields by path. Numbers stay exact.
func (s *stateFlags) parse(input []byte) (any, error) {
	text := strings.TrimSpace(string(input))
	if text == "" {
		return nil, errors.New("state is empty")
	}
	if s.raw || (text[0] != '{' && text[0] != '[') || !json.Valid([]byte(text)) {
		return text, nil
	}
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("decode state: %w", err)
	}
	return v, nil
}

func isPiped(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return r != nil
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice == 0
}

func writeJSON(w io.Writer, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	_, err := w.Write(buf.Bytes())
	return err
}

func newTable(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
}
