package app

import (
	"os"

	xinput "github.com/charmbracelet/x/input"
)

// inputReader changes raw stdin bytes to x/input events. Each ReadEvents
// call returns one batch. The pending queue holds the rest of the batch.
type inputReader struct {
	r       *xinput.Reader
	pending []xinput.Event
}

func newInputReader(f *os.File) *inputReader {
	r, err := xinput.NewReader(f, os.Getenv("TERM"), 0)
	if err != nil {
		return nil
	}
	return &inputReader{r: r}
}

// next waits for one input event. It returns ok=false when the reader ends.
func (ir *inputReader) next() (xinput.Event, bool) {
	if ir == nil || ir.r == nil {
		return nil, false
	}
	if len(ir.pending) > 0 {
		ev := ir.pending[0]
		ir.pending = ir.pending[1:]
		return ev, true
	}
	events, err := ir.r.ReadEvents()
	if err != nil {
		return nil, false
	}
	if len(events) == 0 {
		return nil, false
	}
	ir.pending = events[1:]
	return events[0], true
}
