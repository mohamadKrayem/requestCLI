package core

import (
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// readAll drains a stream built over a fixed string, with a deterministic
// clock so the timing fields are assertable.
func readAll(t *testing.T, raw string) ([]*Event, *Stream) {
	t.Helper()

	var tick time.Duration
	stream := newStream(strings.NewReader(raw), nil, time.Now(), func() time.Duration {
		tick += 10 * time.Millisecond
		return tick
	})

	var events []*Event
	for {
		event, err := stream.Next()
		if errors.Is(err, io.EOF) {
			return events, stream
		}
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		events = append(events, event)
	}
}

func TestStreamParsesFields(t *testing.T) {
	events, _ := readAll(t, "event: delta\ndata: hello\nid: 1\nretry: 250\n\n")

	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	got := events[0]
	if got.Name != "delta" {
		t.Errorf("Name = %q, want delta", got.Name)
	}
	if string(got.Data) != "hello" {
		t.Errorf("Data = %q, want hello", got.Data)
	}
	if got.ID != "1" {
		t.Errorf("ID = %q, want 1", got.ID)
	}
	if got.Retry != 250*time.Millisecond {
		t.Errorf("Retry = %v, want 250ms", got.Retry)
	}
}

// Multiple data lines join with newlines, and the trailing one the accumulator
// leaves behind is removed.
func TestStreamJoinsDataLines(t *testing.T) {
	events, _ := readAll(t, "data: one\ndata: two\ndata: three\n\n")

	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if string(events[0].Data) != "one\ntwo\nthree" {
		t.Errorf("Data = %q, want one\\ntwo\\nthree", events[0].Data)
	}
}

// Exactly one leading space is framing; a second one is data.
func TestStreamStripsOneLeadingSpace(t *testing.T) {
	events, _ := readAll(t, "data:  padded\n\ndata:tight\n\n")

	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if string(events[0].Data) != " padded" {
		t.Errorf("Data = %q, want %q", events[0].Data, " padded")
	}
	if string(events[1].Data) != "tight" {
		t.Errorf("Data = %q, want tight", events[1].Data)
	}
}

// Keepalives are surfaced as comment frames, not as dispatched events: --raw
// needs to be able to show them, but they must not count as data or push the
// event index along.
func TestStreamMarksCommentOnlyFrames(t *testing.T) {
	events, stream := readAll(t, ": keepalive\n\n: another\n\ndata: real\n\n")

	if len(events) != 3 {
		t.Fatalf("got %d frames, want 3 (two keepalives and one event)", len(events))
	}
	for i := range 2 {
		if !events[i].Comment {
			t.Errorf("frame %d: Comment = false, want true", i)
		}
		if len(events[i].Raw) == 0 {
			t.Errorf("frame %d: Raw is empty; --raw would show nothing", i)
		}
	}

	real := events[2]
	if real.Comment {
		t.Error("the data frame was marked as a comment")
	}
	if string(real.Data) != "real" {
		t.Errorf("Data = %q, want real", real.Data)
	}
	if real.Index != 0 {
		t.Errorf("Index = %d, want 0 — keepalives must not consume an index", real.Index)
	}
	if stream.Stats().Events != 1 {
		t.Errorf("Stats().Events = %d, want 1 — keepalives are not events", stream.Stats().Events)
	}
}

// All three SSE line terminators must frame events. Treating a bare CR as
// content would collapse a CR-delimited stream into a single event.
func TestStreamHandlesEveryLineTerminator(t *testing.T) {
	for name, raw := range map[string]string{
		"LF":   "data: a\n\ndata: b\n\n",
		"CRLF": "data: a\r\n\r\ndata: b\r\n\r\n",
		"CR":   "data: a\r\rdata: b\r\r",
	} {
		t.Run(name, func(t *testing.T) {
			events, _ := readAll(t, raw)
			if len(events) != 2 {
				t.Fatalf("got %d events, want 2", len(events))
			}
			if string(events[0].Data) != "a" || string(events[1].Data) != "b" {
				t.Errorf("got %q and %q, want a and b", events[0].Data, events[1].Data)
			}
		})
	}
}

// A field with no colon is a name with an empty value, per the grammar. The
// result is an empty payload, which is deliberately not the same as nil: nil
// means the frame carried no data field at all, and a client debugging a
// protocol needs to tell "sent an empty string" from "sent nothing".
func TestStreamHandlesValuelessField(t *testing.T) {
	events, _ := readAll(t, "data\n\nevent: ping\n\n")

	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if events[0].Data == nil || len(events[0].Data) != 0 {
		t.Errorf("bare `data` gave Data = %v, want an empty non-nil payload", events[0].Data)
	}
	if events[1].Data != nil {
		t.Errorf("frame with no data field gave Data = %q, want nil", events[1].Data)
	}
}

// The spec discards a frame that EOF cuts short. A debugging client must not:
// the truncated final frame is often the thing being investigated.
func TestStreamDispatchesUnterminatedFinalFrame(t *testing.T) {
	events, _ := readAll(t, "data: complete\n\ndata: truncated\n")

	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if string(events[1].Data) != "truncated" {
		t.Errorf("Data = %q, want truncated", events[1].Data)
	}
}

func TestStreamIgnoresIDContainingNUL(t *testing.T) {
	events, _ := readAll(t, "id: a\x00b\ndata: x\n\n")

	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].ID != "" {
		t.Errorf("ID = %q, want empty", events[0].ID)
	}
}

// Raw keeps the frame as received, comments included, so --raw can show what
// the parsed view hides.
func TestStreamKeepsRawFrame(t *testing.T) {
	events, _ := readAll(t, ": note\nevent: ping\ndata: x\n\n")

	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	want := ": note\nevent: ping\ndata: x\n\n"
	if string(events[0].Raw) != want {
		t.Errorf("Raw = %q, want %q", events[0].Raw, want)
	}
}

func TestStreamStatsTrackFirstAndTotal(t *testing.T) {
	events, stream := readAll(t, "data: aa\n\ndata: bbbb\n\n")

	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	stats := stream.Stats()
	if stats.Events != 2 {
		t.Errorf("Events = %d, want 2", stats.Events)
	}
	if stats.Bytes != 6 {
		t.Errorf("Bytes = %d, want 6", stats.Bytes)
	}
	if stats.FirstEvent != events[0].At {
		t.Errorf("FirstEvent = %v, want %v", stats.FirstEvent, events[0].At)
	}
	if stats.Total < events[1].At {
		t.Errorf("Total = %v, want >= %v", stats.Total, events[1].At)
	}
}

// Next keeps reporting EOF once the stream has ended, so a caller that loops
// past the end does not spin or panic.
func TestStreamNextIsIdempotentAtEOF(t *testing.T) {
	_, stream := readAll(t, "data: x\n\n")

	for range 3 {
		if _, err := stream.Next(); !errors.Is(err, io.EOF) {
			t.Fatalf("Next after EOF = %v, want io.EOF", err)
		}
	}
}

// sseServer streams frames on demand. release lets the test hold the server
// mid-stream, which is what distinguishes real incremental delivery from a
// buffered read that merely looks like one.
func sseServer(t *testing.T, frames []string, release <-chan struct{}) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("ResponseWriter is not a Flusher")
			return
		}
		for i, frame := range frames {
			_, _ = io.WriteString(w, frame)
			flusher.Flush()
			if release != nil && i == 0 {
				<-release
			}
		}
	}))
	t.Cleanup(server.Close)
	return server
}

// text/event-stream turns on streaming without --stream.
func TestSendAutoDetectsEventStream(t *testing.T) {
	server := sseServer(t, []string{"data: a\n\n", "data: b\n\n"}, nil)

	req := NewRequest(http.MethodGet, server.URL)
	result, err := req.Send(SendOptions{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	defer func() { _ = result.Close() }()

	if result.Stream == nil {
		t.Fatal("Stream is nil; text/event-stream should have streamed")
	}
	if result.Body != nil {
		t.Errorf("Body = %q, want nil alongside a stream", result.Body)
	}
	if result.Timing.TTFB <= 0 {
		t.Error("TTFB was not recorded")
	}

	var got []string
	for {
		event, err := result.Stream.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		got = append(got, string(event.Data))
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("events = %v, want [a b]", got)
	}
}

// The point of the whole change: the first event must be readable while the
// server is still holding the connection open. A buffered implementation
// blocks here until the handler returns, so this test fails on a regression
// to io.ReadAll.
func TestSendDeliversEventsBeforeTheStreamEnds(t *testing.T) {
	release := make(chan struct{})
	server := sseServer(t, []string{"data: first\n\n", "data: second\n\n"}, release)

	req := NewRequest(http.MethodGet, server.URL)
	result, err := req.Send(SendOptions{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	defer func() { _ = result.Close() }()

	first := make(chan string, 1)
	go func() {
		event, err := result.Stream.Next()
		if err != nil {
			close(first)
			return
		}
		first <- string(event.Data)
	}()

	select {
	case data, ok := <-first:
		if !ok {
			t.Fatal("Next failed before the stream was released")
		}
		if data != "first" {
			t.Errorf("first event = %q, want first", data)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the first event did not arrive while the server held the stream open")
	}

	close(release)
}

// --stream forces incremental delivery for a server that streams under some
// other content type.
func TestSendStreamFlagForcesStreamingOnAnyContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "data: forced\n\n")
	}))
	defer server.Close()

	req := NewRequest(http.MethodGet, server.URL)
	result, err := req.Send(SendOptions{Timeout: 5 * time.Second, Stream: true})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	defer func() { _ = result.Close() }()

	if result.Stream == nil {
		t.Fatal("Stream is nil despite SendOptions.Stream")
	}
	event, err := result.Stream.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if string(event.Data) != "forced" {
		t.Errorf("Data = %q, want forced", event.Data)
	}
}

// Without --stream an ordinary response must behave exactly as before:
// buffered body, no stream, Total recorded.
func TestSendStillBuffersOrdinaryResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer server.Close()

	req := NewRequest(http.MethodGet, server.URL)
	result, err := req.Send(SendOptions{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	defer func() { _ = result.Close() }()

	if result.Stream != nil {
		t.Error("an ordinary response must not stream")
	}
	if string(result.Body) != `{"ok":true}` {
		t.Errorf("Body = %q", result.Body)
	}
	if result.Timing.Total <= 0 {
		t.Error("Total was not recorded for a buffered response")
	}
}

// Close on a buffered result is a no-op, so callers can always defer it.
func TestResultCloseIsSafeOnEveryResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "body")
	}))
	defer server.Close()

	req := NewRequest(http.MethodGet, server.URL)
	result, err := req.Send(SendOptions{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	for range 2 {
		if err := result.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}
	if err := (*Result)(nil).Close(); err != nil {
		t.Errorf("Close on nil: %v", err)
	}
}

// The deadline must still bound connecting and the response headers; only the
// body of a stream escapes it.
func TestSendTimeoutStillBoundsHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Second)
		w.Header().Set("Content-Type", "text/event-stream")
	}))
	defer server.Close()

	req := NewRequest(http.MethodGet, server.URL)
	start := time.Now()
	_, err := req.Send(SendOptions{Timeout: 100 * time.Millisecond})
	if err == nil {
		t.Fatal("expected a timeout waiting for headers")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("took %v; the header deadline was not enforced", elapsed)
	}
	if !strings.Contains(err.Error(), "timed out after 100ms") {
		t.Errorf("error = %q, want it to name the timeout", err)
	}
}

// A gzip-encoded event stream must still arrive frame by frame. gzip.Reader
// decodes incrementally only if the server flushes its writer per frame.
func TestGzippedEventStreamIsIncremental(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Content-Encoding", "gzip")
		w.WriteHeader(http.StatusOK)

		gz := gzip.NewWriter(w)
		flusher := w.(http.Flusher)

		_, _ = io.WriteString(gz, "data: first\n\n")
		_ = gz.Flush()
		flusher.Flush()

		<-release

		_, _ = io.WriteString(gz, "data: second\n\n")
		_ = gz.Close()
		flusher.Flush()
	}))
	defer server.Close()

	req := NewRequest(http.MethodGet, server.URL)
	result, err := req.Send(SendOptions{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	defer func() { _ = result.Close() }()

	if result.Stream == nil {
		t.Fatal("gzipped event stream did not stream")
	}

	got := make(chan string, 1)
	go func() {
		event, err := result.Stream.Next()
		if err != nil {
			close(got)
			return
		}
		got <- string(event.Data)
	}()

	select {
	case data, ok := <-got:
		if !ok {
			t.Fatal("Next failed on a gzipped stream")
		}
		if data != "first" {
			t.Errorf("first event = %q, want first", data)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("gzipped stream buffered: the first event did not arrive while the server held it open")
	}

	close(release)

	event, err := result.Stream.Next()
	if err != nil {
		t.Fatalf("second Next: %v", err)
	}
	if string(event.Data) != "second" {
		t.Errorf("second event = %q, want second", event.Data)
	}
	if _, err := result.Stream.Next(); !errors.Is(err, io.EOF) {
		t.Errorf("want io.EOF at end, got %v", err)
	}
}
