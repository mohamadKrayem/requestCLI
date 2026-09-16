package core

import (
	"bufio"
	"bytes"
	"io"
	"strconv"
	"time"
)

// maxEventLine caps a single line of an event stream. Individual SSE lines are
// small in practice — a model delta is a few hundred bytes — but a server that
// never sends a newline must not be able to grow the buffer without bound.
const maxEventLine = 1 << 20 // 1 MiB

// Event is one dispatched server-sent event.
//
// The fields follow the SSE grammar: a frame is a run of `field: value` lines
// terminated by a blank line. Raw keeps the frame exactly as it arrived so a
// protocol problem is still visible when the parsed view looks fine.
type Event struct {
	// Name is the `event:` field. It is empty when the server did not send
	// one, which the SSE spec treats as the implicit name "message".
	Name string
	// Data is the concatenation of every `data:` line in the frame, joined
	// with newlines and with the trailing newline removed, per the spec.
	Data []byte
	// ID is the `id:` field, empty when absent.
	ID string
	// Retry is the `retry:` reconnection hint. Zero when absent or unparseable.
	Retry time.Duration

	// Raw is the frame as received, including comment lines and the blank
	// terminator. This is what --raw prints.
	Raw []byte
	// Comment marks a frame that carried only comment lines — how servers
	// send keepalives. It is not a dispatched event: it does not count
	// towards the stats and renderers drop it unless asked for raw frames.
	// It is surfaced rather than swallowed so --raw can show the framing it
	// exists to show.
	Comment bool
	// Index is the 0-based position of this event in the stream.
	Index int
	// At is how long after the request started this event was dispatched.
	At time.Duration
}

// StreamStats summarises a stream. It is read after the stream ends, or after
// the caller stops early.
type StreamStats struct {
	// Events is the number of dispatched events.
	Events int
	// Bytes is the total size of the Data payloads, excluding framing.
	Bytes int
	// FirstEvent is the offset of the first dispatched event from the start of
	// the request — time-to-first-token for a model API.
	FirstEvent time.Duration
	// Total is the offset of the last event, or of the end of the stream.
	Total time.Duration
}

// Stream yields server-sent events as they arrive.
//
// It never buffers the whole response: Next reads only as far as the next
// frame terminator. The caller owns the underlying connection and must Close
// it, which is why Send hands back a Result with a Close method rather than
// closing the body itself the way the buffered path does.
type Stream struct {
	scanner *bufio.Scanner
	closer  io.Closer
	started time.Time
	elapsed func() time.Duration

	index int
	stats StreamStats
	done  bool
}

// newStream wraps an already-open response body.
//
// elapsed is injected so tests can drive time deterministically; in production
// it measures from the moment the request was sent, so At and FirstEvent are
// offsets a user can compare against the TTFB on the status line.
func newStream(r io.Reader, closer io.Closer, started time.Time, elapsed func() time.Duration) *Stream {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxEventLine)
	scanner.Split(scanEventLines)

	if elapsed == nil {
		elapsed = func() time.Duration { return time.Since(started) }
	}
	return &Stream{
		scanner: scanner,
		closer:  closer,
		started: started,
		elapsed: elapsed,
	}
}

// Next returns the next event, or io.EOF when the stream has ended.
//
// A frame carrying no recognised field — a run of comments, which is how
// servers send keepalives — is not an event and is skipped rather than
// dispatched empty.
func (s *Stream) Next() (*Event, error) {
	if s.done {
		return nil, io.EOF
	}

	var (
		event    Event
		data     []byte
		raw      []byte
		hasField bool
	)

	for s.scanner.Scan() {
		line := s.scanner.Bytes()
		raw = append(raw, line...)
		raw = append(raw, '\n')

		// Blank line: dispatch.
		if len(line) == 0 {
			if !hasField {
				// A comment-only frame — a keepalive. Returned immediately
				// rather than accumulated, so a stream of nothing but
				// keepalives cannot grow this buffer without bound, and
				// --raw can still show them.
				return &Event{Comment: true, Raw: raw, At: s.elapsed()}, nil
			}
			event.Data = trimTrailingNewline(data)
			event.Raw = raw
			return s.dispatch(&event), nil
		}

		// A comment line is kept in Raw and otherwise ignored.
		if line[0] == ':' {
			continue
		}

		// "If the line contains no colon, the whole line is the field name and
		// the value is empty." A single leading space after the colon is part
		// of the framing, not the value.
		field, value, _ := bytes.Cut(line, []byte{':'})
		value = bytes.TrimPrefix(value, []byte{' '})

		switch string(field) {
		case "event":
			event.Name = string(value)
			hasField = true
		case "data":
			data = append(data, value...)
			data = append(data, '\n')
			hasField = true
		case "id":
			// The spec requires ignoring an id containing NUL.
			if !bytes.ContainsRune(value, 0) {
				event.ID = string(value)
			}
			hasField = true
		case "retry":
			if ms, err := strconv.Atoi(string(value)); err == nil && ms >= 0 {
				event.Retry = time.Duration(ms) * time.Millisecond
			}
			hasField = true
		default:
			// An unknown field is ignored, but it still makes this a frame
			// rather than a keepalive.
			hasField = true
		}
	}

	s.done = true
	s.stats.Total = s.elapsed()

	if err := s.scanner.Err(); err != nil {
		return nil, err
	}

	// The stream ended without a blank line terminating the last frame. The
	// SSE spec discards it; this does not. A tool whose job is showing what
	// the server sent must not silently drop the final frame — that is exactly
	// the case someone reaches for a debugger to look at.
	if hasField {
		event.Data = trimTrailingNewline(data)
		event.Raw = raw
		return s.dispatch(&event), nil
	}

	return nil, io.EOF
}

// dispatch stamps an assembled event with its position and timing.
func (s *Stream) dispatch(event *Event) *Event {
	event.Index = s.index
	event.At = s.elapsed()

	s.index++
	s.stats.Events++
	s.stats.Bytes += len(event.Data)
	if s.stats.Events == 1 {
		s.stats.FirstEvent = event.At
	}
	s.stats.Total = event.At

	return event
}

// Stats returns the running summary. It is meaningful mid-stream, so a caller
// interrupted by a signal can still report what it received.
func (s *Stream) Stats() StreamStats { return s.stats }

// Close releases the underlying connection.
func (s *Stream) Close() error {
	if s.closer == nil {
		return nil
	}
	return s.closer.Close()
}

// trimTrailingNewline removes the single newline that accumulating data lines
// leaves behind. It returns nil for an empty payload so an event carrying no
// data is distinguishable from one carrying an empty string.
func trimTrailingNewline(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	return data[:len(data)-1]
}

// scanEventLines splits on any of the three line terminators the SSE grammar
// allows: CRLF, LF and a bare CR.
//
// bufio.ScanLines handles only the first two, and treating a bare CR as
// ordinary content would merge every frame of a CR-delimited stream into one.
func scanEventLines(data []byte, atEOF bool) (int, []byte, error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	for i := 0; i < len(data); i++ {
		switch data[i] {
		case '\n':
			return i + 1, data[:i], nil
		case '\r':
			// A CR at the end of the buffer may be the first half of a CRLF
			// that has not arrived yet. Waiting costs one more read; guessing
			// would emit a spurious empty line and dispatch early.
			if i == len(data)-1 && !atEOF {
				return 0, nil, nil
			}
			if i+1 < len(data) && data[i+1] == '\n' {
				return i + 2, data[:i], nil
			}
			return i + 1, data[:i], nil
		}
	}

	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}
