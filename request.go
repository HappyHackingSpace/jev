package jev

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
)

// Request is one System One call: a state and the questions to ask about it.
type Request struct {
	// Model overrides the client's default model for this request.
	Model string `json:"model,omitempty"`
	// State is the content every question refers to: a string, or any
	// JSON-encodable object or array.
	State any `json:"state"`
	// Questions maps your names to the questions to ask.
	Questions Questions `json:"questions"`
	// Images maps a reference string to an image. Write the reference
	// anywhere a string can go in State, Instructions or Criteria and the
	// model sees the image in its place. Use [NewRef] for references that
	// cannot collide with ordinary text.
	Images map[string]Image `json:"-"`
	// LongContext raises the context limits for this request. It is slower
	// and less accurate, so turn it on only when the state needs it.
	LongContext bool `json:"-"`
}

// Image is a PNG or JPEG image to send with a request.
type Image struct {
	Data []byte
	// Format is "png" or "jpg". [ReadImage] and [NewImage] detect it.
	Format string
}

// NewImage wraps image bytes, detecting the format. Only PNG and JPEG are
// accepted, which also keeps arbitrary files from being sent as images.
func NewImage(data []byte) (Image, error) {
	switch http.DetectContentType(data) {
	case "image/png":
		return Image{Data: data, Format: "png"}, nil
	case "image/jpeg":
		return Image{Data: data, Format: "jpg"}, nil
	}
	return Image{}, fmt.Errorf("unsupported image: only PNG and JPEG are accepted")
}

// ReadImage loads a PNG or JPEG image from disk.
func ReadImage(path string) (Image, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Image{}, err
	}
	img, err := NewImage(data)
	if err != nil {
		return Image{}, fmt.Errorf("%s: %w", path, err)
	}
	return img, nil
}

// NewRef returns a random image reference. References are matched as plain
// text, so random ones avoid replacing a word in the state by accident.
func NewRef() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "img-" + hex.EncodeToString(b)
}

func (r Request) needsImages() bool {
	if len(r.Images) > 0 {
		return true
	}
	for _, q := range r.Questions {
		if q.Type() == TypeBox {
			return true
		}
	}
	return false
}
