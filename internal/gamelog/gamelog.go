// Package gamelog turns the game's output into structured, redacted log
// lines. Minecraft prints log4j XMLLayout events on stdout, mixed with plain
// lines from code that bypasses log4j; the parser takes both, never panics on
// malformed input (Pandora's did), and caps what it buffers.
package gamelog

import (
	"encoding/xml"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Level is a log line's severity, as the console's level chips name them.
type Level string

const (
	Info  Level = "INFO"
	Warn  Level = "WARN"
	Error Level = "ERROR"
	Debug Level = "DEBUG"
)

// Line is one console line.
type Line struct {
	Time     string `json:"time"` // HH:MM:SS
	Level    Level  `json:"level"`
	Thread   string `json:"thread"`
	Message  string `json:"message"`
	Redacted bool   `json:"redacted,omitempty"`
}

// Redactor hides secrets; *logging.Redactor implements it.
type Redactor interface{ Redact(string) string }

// maxEventLines bounds one XML event, so a never-closed event cannot grow forever.
const maxEventLines = 4000

// Parser consumes output line by line. It is not safe for concurrent use;
// stdout and stderr each get their own.
type Parser struct {
	emit   func(Line)
	red    Redactor
	stderr bool
	now    func() time.Time

	inEvent bool
	block   []string
}

// NewParser returns a parser calling emit for every line. Lines from stderr
// default to ERROR.
func NewParser(emit func(Line), red Redactor, stderr bool) *Parser {
	return &Parser{emit: emit, red: red, stderr: stderr, now: time.Now}
}

// Feed takes one line of output (without its newline).
func (p *Parser) Feed(line string) {
	trimmed := strings.TrimSpace(line)
	if !p.inEvent && strings.HasPrefix(trimmed, "<log4j:Event") {
		p.inEvent = true
	}
	if !p.inEvent {
		if trimmed != "" {
			p.plain(line)
		}
		return
	}
	p.block = append(p.block, line)
	if strings.Contains(line, "</log4j:Event>") {
		p.flushEvent()
	} else if len(p.block) >= maxEventLines {
		p.flushPlain()
	}
}

// Close emits anything still buffered (the process ended mid-event).
func (p *Parser) Close() {
	if p.inEvent {
		p.flushPlain()
	}
}

func (p *Parser) flushEvent() {
	ev, ok := parseEvent(strings.Join(p.block, "\n"))
	if !ok {
		p.flushPlain()
		return
	}
	p.block, p.inEvent = p.block[:0], false
	t := p.now()
	if ev.ms > 0 {
		t = time.UnixMilli(ev.ms)
	}
	clock := t.Format("15:04:05")
	for _, l := range splitLines(ev.message) {
		p.out(Line{Time: clock, Level: ev.level, Thread: ev.thread, Message: l})
	}
	for _, l := range splitLines(ev.throwable) {
		p.out(Line{Time: clock, Level: ev.level, Thread: ev.thread, Message: l})
	}
}

// flushPlain gives up on a malformed event and shows its raw lines.
func (p *Parser) flushPlain() {
	block := p.block
	p.block, p.inEvent = nil, false
	for _, l := range block {
		if strings.TrimSpace(l) != "" {
			p.plain(l)
		}
	}
}

// legacyLine matches "[12:34:56] [Render thread/INFO]: message" and older
// "2013-09-19 12:34:56 [INFO] message" output from versions without log4j XML.
var (
	// "[21:40:02] [Server thread/INFO]: …", and (Forge and NeoForge's own
	// console layout) "[13Oct2026 21:40:02.019] [Server thread/INFO] [logger/]: …".
	bracketed = regexp.MustCompile(`^\[(?:\d\d[A-Za-z]{3}\d{4} )?(\d\d:\d\d:\d\d)(?:\.\d+)?\] \[([^\]/]*)/([A-Z]+)\](?: \[[^\]]*\])?:? ?(.*)$`)
	oldStyle  = regexp.MustCompile(`^\d{4}-\d\d-\d\d (\d\d:\d\d:\d\d) \[([A-Z]+)\] ?(.*)$`)
)

func (p *Parser) plain(raw string) {
	l := Line{Time: p.now().Format("15:04:05"), Level: Info, Message: strings.TrimRight(raw, "\r")}
	if p.stderr {
		l.Level = Error
	}
	if m := bracketed.FindStringSubmatch(l.Message); m != nil {
		l.Time, l.Thread, l.Level, l.Message = m[1], m[2], level(m[3]), m[4]
	} else if m := oldStyle.FindStringSubmatch(l.Message); m != nil {
		l.Time, l.Level, l.Message = m[1], level(m[2]), m[3]
	}
	p.out(l)
}

func (p *Parser) out(l Line) {
	if p.red != nil {
		red := p.red.Redact(l.Message)
		l.Redacted = red != l.Message
		l.Message = red
	}
	p.emit(l)
}

func level(s string) Level {
	switch strings.ToUpper(s) {
	case "WARN", "WARNING":
		return Warn
	case "ERROR", "FATAL", "SEVERE":
		return Error
	case "DEBUG", "TRACE", "FINE", "FINER", "FINEST":
		return Debug
	default:
		return Info
	}
}

type event struct {
	ms                 int64
	level              Level
	thread             string
	message, throwable string
}

// parseEvent reads one <log4j:Event> block. Undeclared "log4j:" prefixes are
// fine for encoding/xml; any other trouble makes the caller fall back to plain.
func parseEvent(block string) (event, bool) {
	dec := xml.NewDecoder(strings.NewReader(block))
	dec.Strict = false
	var (
		ev    event
		where string
	)
	for {
		tok, err := dec.Token()
		if err != nil {
			return ev, false // only a complete event counts
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "Event":
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "timestamp":
						ev.ms, _ = strconv.ParseInt(a.Value, 10, 64)
					case "level":
						ev.level = level(a.Value)
					case "thread":
						ev.thread = a.Value
					}
				}
			case "Message", "Throwable":
				where = t.Name.Local
			}
		case xml.CharData:
			switch where {
			case "Message":
				ev.message += string(t)
			case "Throwable":
				ev.throwable += string(t)
			}
		case xml.EndElement:
			if t.Name.Local == "Message" || t.Name.Local == "Throwable" {
				where = ""
			}
			if t.Name.Local == "Event" {
				if ev.level == "" {
					ev.level = Info
				}
				return ev, true
			}
		}
	}
}

func splitLines(s string) []string {
	s = strings.TrimRight(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
