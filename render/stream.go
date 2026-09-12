package render

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mohamadkrayem/requestCLI/core"
	"github.com/tidwall/pretty"
)

// eventMarker prefixes every rendered event. It gives a stream a visible
// left edge, so a multi-line payload is obviously one event rather than
// several.
const eventMarker = "●"

// implicitEventName is what the SSE spec calls a frame that carries no
// `event:` field. Showing it beats showing nothing, because "message" is a
// real name a server can also send explicitly.
const implicitEventName = "message"

// Event renders one server-sent event.
//
// Like Render it is a pure function: no globals, no terminal probing, no
// clock. The caller decides colour and passes it in, which is what lets a TUI
// re-render the same event on resize and lets these tests run without a TTY.
//
// Opts.Raw prints the frame exactly as it arrived instead, comments included,
// for debugging the framing rather than the payload.
func Event(event *core.Event, opts Options) string {
	if event == nil {
		return ""
	}

	if opts.Raw {
		return strings.TrimRight(string(event.Raw), "\n")
	}

	// A keepalive is framing, not content. It is dropped here rather than in
	// the parser so that --raw above can still show it.
	if event.Comment {
		return ""
	}

	name := event.Name
	if name == "" {
		name = implicitEventName
	}

	var out strings.Builder
	out.WriteString(colorize(eventMarker, ansiHiCyan, opts.Color))
	out.WriteString(" ")
	out.WriteString(colorize(name, ansiHiBlue, opts.Color))

	if opts.ShowEventTiming {
		fmt.Fprintf(&out, " %s", colorize(formatDuration(event.At), ansiCyan, opts.Color))
	}

	if len(event.Data) > 0 {
		out.WriteString("  ")
		out.WriteString(renderEventData(event.Data, opts.Color))
	}

	return out.String()
}

// renderEventData formats one event payload.
//
// JSON is compacted onto a single line rather than pretty-printed: a stream is
// read as a sequence, and expanding every frame to a dozen lines buries the
// sequence it exists to show. Colour still comes from the shared palette, so a
// delta looks like the rest of the tool's JSON. Non-JSON is passed through with
// continuation lines indented under the marker.
func renderEventData(data []byte, color bool) string {
	if json.Valid(data) {
		compact := pretty.Ugly(data)
		if color {
			compact = pretty.Color(compact, jsonStyle)
		}
		return string(compact)
	}

	text := strings.TrimRight(string(data), "\n")
	return strings.ReplaceAll(text, "\n", "\n  ")
}

// StreamSummary renders the closing line of a stream.
//
// It is printed after the last event, and also after an interrupt — which is
// why it takes stats rather than reading them itself: a caller cut short by a
// signal still has something meaningful to report.
func StreamSummary(stats core.StreamStats, opts Options) string {
	parts := []string{
		fmt.Sprintf("%d %s", stats.Events, plural(stats.Events, "event", "events")),
		formatBytes(stats.Bytes),
	}

	// A stream with no events has no first-event offset worth printing, and
	// dividing by its duration would be meaningless too.
	if stats.Events > 0 {
		parts = append(parts, "first "+formatDuration(stats.FirstEvent))
		parts = append(parts, "total "+formatDuration(stats.Total))
		if rate, ok := eventsPerSecond(stats); ok {
			parts = append(parts, fmt.Sprintf("%.1f events/s", rate))
		}
	}

	line := strings.Join(parts, " · ")
	return colorize(line, ansiHiWhite, opts.Color)
}

// eventsPerSecond reports the throughput, and false when the stream was too
// short to measure one. Reporting a rate derived from a sub-millisecond
// duration would produce a confident, meaningless number.
func eventsPerSecond(stats core.StreamStats) (float64, bool) {
	if stats.Total < time.Millisecond || stats.Events == 0 {
		return 0, false
	}
	return float64(stats.Events) / stats.Total.Seconds(), true
}

// formatDuration prints a duration at a resolution a human reads rather than
// Go's default, which renders a tenth of a second as 100.482289ms.
func formatDuration(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return fmt.Sprintf("%dµs", d.Microseconds())
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	default:
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
}

// formatBytes prints a payload total in the unit that keeps it readable.
func formatBytes(n int) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
