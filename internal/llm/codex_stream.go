package llm

import (
	"bytes"
	"io"

	"github.com/openai/openai-go/packages/ssestream"
)

// codexEventDecoder reuses the shared SSE framing rules while retaining the
// SDK's typed Responses events and error handling. Keepalive comments and empty
// frames are transport noise, not incomplete JSON documents.
type codexEventDecoder struct {
	body    io.ReadCloser
	scanner *sseScanner
	event   ssestream.Event
	err     error
}

func newCodexEventDecoder(body io.ReadCloser) *codexEventDecoder {
	return &codexEventDecoder{body: body, scanner: newSSEScanner(body)}
}

func (d *codexEventDecoder) Next() bool {
	for d.err == nil && d.scanner.Next() {
		frame := d.scanner.Frame()
		if payload := bytes.TrimSpace(frame.errData); len(payload) > 0 {
			d.err = newStreamServerError(payload, false)
			return false
		}
		payload := frame.data
		if len(bytes.TrimSpace(payload)) == 0 {
			continue
		}
		d.event = ssestream.Event{Data: payload}
		return true
	}
	return false
}

func (d *codexEventDecoder) Event() ssestream.Event { return d.event }
func (d *codexEventDecoder) Close() error           { return d.body.Close() }
func (d *codexEventDecoder) Err() error {
	if d.err != nil {
		return d.err
	}
	return d.scanner.Err()
}
