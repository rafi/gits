package json

import (
	"encoding/json"
	"io"
)

// Write marshals env to w as one newline-terminated line.
func Write(w io.Writer, env Envelope) error {
	return WriteValue(w, env)
}

// WriteValue marshals any document to w as one newline-terminated line — the
// shape that makes output pipeable into `jq` without a reader having to know
// how many lines to expect.
//
// It exists for `doctor`, whose report is a list of findings rather than a
// project tree. What the two share is this convention, not the schema.
func WriteValue(w io.Writer, doc any) error {
	raw, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	_, err = w.Write(append(raw, '\n'))
	return err
}
