package render

import (
	"strings"
	"testing"
	"time"

	"github.com/mohamadkrayem/requestCLI/core"
)

func TestEventRendersNameAndData(t *testing.T) {
	got := Event(&core.Event{Name: "delta", Data: []byte(`{"text":"hi"}`)}, Options{})

	for _, want := range []string{eventMarker, "delta", `{"text":"hi"}`} {
		if !strings.Contains(got, want) {
			t.Errorf("Event() = %q, missing %q", got, want)
		}
	}
}

// A frame with no event: field is "message" per the SSE spec. Printing the
// marker alone would leave the reader guessing.
func TestEventNamesAnUnnamedFrame(t *testing.T) {
	got := Event(&core.Event{Data: []byte("x")}, Options{})

	if !strings.Contains(got, implicitEventName) {
		t.Errorf("Event() = %q, want it to name the frame %q", got, implicitEventName)
	}
}

// JSON is compacted rather than pretty-printed: a stream is read as a
// sequence, and expanding each frame over a dozen lines hides it.
func TestEventCompactsJSONOntoOneLine(t *testing.T) {
	data := []byte("{\n  \"a\": 1,\n  \"b\": 2\n}")
	got := Event(&core.Event{Name: "d", Data: data}, Options{})

	if strings.Count(got, "\n") != 0 {
		t.Errorf("Event() = %q, want a single line", got)
	}
	if !strings.Contains(got, `{"a":1,"b":2}`) {
		t.Errorf("Event() = %q, want compacted json", got)
	}
}

// Key order and integer precision have to survive here for the same reason
// they do in the body renderer: this is what the server sent.
func TestEventPreservesJSONFidelity(t *testing.T) {
	data := []byte(`{"id":1234567890123456789,"zebra":1,"apple":2}`)
	got := Event(&core.Event{Name: "d", Data: data}, Options{})

	if !strings.Contains(got, "1234567890123456789") {
		t.Errorf("Event() = %q, want the id intact", got)
	}
	if strings.Index(got, "zebra") > strings.Index(got, "apple") {
		t.Errorf("Event() = %q, want the server's key order", got)
	}
}

func TestEventRawPrintsTheFrameVerbatim(t *testing.T) {
	raw := []byte(": keepalive\nevent: ping\ndata: x\n\n")
	got := Event(&core.Event{Name: "ping", Data: []byte("x"), Raw: raw}, Options{Raw: true})

	if !strings.Contains(got, ": keepalive") {
		t.Errorf("Event(Raw) = %q, want the comment line preserved", got)
	}
	if strings.Contains(got, eventMarker) {
		t.Errorf("Event(Raw) = %q, want no added framing", got)
	}
}

func TestEventTimingIsOptional(t *testing.T) {
	event := &core.Event{Name: "d", Data: []byte("x"), At: 250 * time.Millisecond}

	if got := Event(event, Options{}); strings.Contains(got, "250ms") {
		t.Errorf("Event() = %q, want no timing by default", got)
	}
	if got := Event(event, Options{ShowEventTiming: true}); !strings.Contains(got, "250ms") {
		t.Errorf("Event(ShowEventTiming) = %q, want the offset", got)
	}
}

// Colour is the caller's decision, exactly as it is for Render.
func TestEventHonoursTheColorFlag(t *testing.T) {
	event := &core.Event{Name: "d", Data: []byte("plain")}

	if got := Event(event, Options{}); strings.Contains(got, "\x1b[") {
		t.Errorf("Event() = %q, want no escapes when colour is off", got)
	}
	if got := Event(event, Options{Color: true}); !strings.Contains(got, "\x1b[") {
		t.Errorf("Event(Color) = %q, want escapes when colour is on", got)
	}
}

func TestEventHandlesNil(t *testing.T) {
	if got := Event(nil, Options{}); got != "" {
		t.Errorf("Event(nil) = %q, want empty", got)
	}
}

// Non-JSON payloads keep their line breaks, indented so the frame still reads
// as one unit.
func TestEventIndentsMultiLineText(t *testing.T) {
	got := Event(&core.Event{Name: "log", Data: []byte("line one\nline two")}, Options{})

	if !strings.Contains(got, "line one\n  line two") {
		t.Errorf("Event() = %q, want the continuation indented", got)
	}
}

func TestStreamSummaryReportsCounts(t *testing.T) {
	stats := core.StreamStats{
		Events:     4,
		Bytes:      2048,
		FirstEvent: 340 * time.Millisecond,
		Total:      2 * time.Second,
	}
	got := StreamSummary(stats, Options{})

	for _, want := range []string{"4 events", "2.0 KB", "first 340ms", "total 2.00s", "2.0 events/s"} {
		if !strings.Contains(got, want) {
			t.Errorf("StreamSummary() = %q, missing %q", got, want)
		}
	}
}

// An empty stream must not report a first-event offset or a throughput
// computed by dividing by zero.
func TestStreamSummaryHandlesAnEmptyStream(t *testing.T) {
	got := StreamSummary(core.StreamStats{}, Options{})

	if !strings.Contains(got, "0 events") {
		t.Errorf("StreamSummary() = %q, want an event count", got)
	}
	for _, unwanted := range []string{"first", "events/s", "NaN", "+Inf"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("StreamSummary() = %q, should not contain %q", got, unwanted)
		}
	}
}

// A stream that finished faster than the clock can resolve must not produce a
// confident, meaningless rate.
func TestStreamSummaryOmitsRateForAnInstantStream(t *testing.T) {
	stats := core.StreamStats{Events: 3, Bytes: 10, Total: 100 * time.Microsecond}
	got := StreamSummary(stats, Options{})

	if strings.Contains(got, "events/s") {
		t.Errorf("StreamSummary() = %q, want no rate for a sub-millisecond stream", got)
	}
}

func TestStreamSummaryUsesSingularForOneEvent(t *testing.T) {
	stats := core.StreamStats{Events: 1, Bytes: 4, Total: time.Second}
	if got := StreamSummary(stats, Options{}); !strings.Contains(got, "1 event ") {
		t.Errorf("StreamSummary() = %q, want a singular noun", got)
	}
}

func TestFormatDurationScales(t *testing.T) {
	for _, tc := range []struct {
		in   time.Duration
		want string
	}{
		{500 * time.Microsecond, "500µs"},
		{250 * time.Millisecond, "250ms"},
		{2500 * time.Millisecond, "2.50s"},
	} {
		if got := formatDuration(tc.in); got != tc.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatBytesScales(t *testing.T) {
	for _, tc := range []struct {
		in   int
		want string
	}{
		{512, "512 B"},
		{2048, "2.0 KB"},
		{3 * 1024 * 1024, "3.0 MB"},
	} {
		if got := formatBytes(tc.in); got != tc.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// A keepalive is framing, not content: it renders to nothing so the caller
// prints no line for it, but --raw still shows the frame.
func TestEventDropsKeepalivesUnlessRaw(t *testing.T) {
	keepalive := &core.Event{Comment: true, Raw: []byte(": ping\n\n")}

	if got := Event(keepalive, Options{}); got != "" {
		t.Errorf("Event(keepalive) = %q, want empty", got)
	}
	if got := Event(keepalive, Options{Raw: true}); !strings.Contains(got, ": ping") {
		t.Errorf("Event(keepalive, Raw) = %q, want the frame shown", got)
	}
}
