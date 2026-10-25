package thermal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
)

// Request is one call to the helper: a single JSON line per connection.
type Request struct {
	Op string `json:"op"`
	// RPMs per fan index for OpSetTargets; the helper clamps to each fan's reported range.
	RPMs []float64 `json:"rpms,omitempty"`
}

// Response answers a Request; Error is empty on success.
type Response struct {
	Version int    `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

const (
	OpVersion    = "version"
	OpSetTargets = "setTargets"
	OpRestore    = "restoreAutomatic"

	// A request line never needs more; anything longer is not ours.
	maxMessageBytes = 4096
	maxFans         = 16
)

// Validate rejects requests the helper must never act on.
func (r Request) Validate() error {
	switch r.Op {
	case OpVersion, OpRestore:
		return nil
	case OpSetTargets:
		if len(r.RPMs) == 0 || len(r.RPMs) > maxFans {
			return fmt.Errorf("invalid fan count %d", len(r.RPMs))
		}
		for _, rpm := range r.RPMs {
			if math.IsNaN(rpm) || math.IsInf(rpm, 0) || rpm < 0 {
				return fmt.Errorf("invalid fan speed %v", rpm)
			}
		}
		return nil
	}
	return fmt.Errorf("unknown request %q", r.Op)
}

// WriteMessage writes v as one JSON line.
func WriteMessage(w io.Writer, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

// ReadMessage reads one JSON line of at most maxMessageBytes into v.
func ReadMessage(r io.Reader, v any) error {
	line, err := bufio.NewReaderSize(io.LimitReader(r, maxMessageBytes), maxMessageBytes).ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return err
	}
	return json.Unmarshal(line, v)
}
