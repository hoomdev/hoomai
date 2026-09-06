package providers

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// codex drives Codex CLI headless (`codex exec --json`). Verified against
// Codex CLI 0.151.0: JSONL stream, a thread id it reports and resumes by id,
// model selection, its own instructions PREPENDED without replacing Codex's,
// and the limit of a read-only role imposed with the SANDBOX. Codex does not
// name its tools, so the role's limit travels as intention (ReadOnly/Exec)
// and lands here as `sandbox_mode`: that is exactly why the vocabulary of the
// Request is hoom's and not one CLI's.
type codex struct{}

func (codex) Name() string { return "codex" }
func (codex) Bin() string  { return "codex" }

func (codex) Capabilities() Capabilities {
	return Capabilities{
		Structured: true, Continue: true, Resume: true, SessionID: true,
		Model: true, SystemPrompt: true, ReadOnly: true,
		// Tools: Codex names no tools. MaxTurns/Budget: it has no caps.
	}
}

// Command builds the headless invocation. The prompt is always the final
// argument; on a resume the session id goes immediately after `resume`, which
// is the positional order the CLI declares.
func (c codex) Command(req Request) (Invocation, error) {
	p, err := resolve(c.Name(), c.Capabilities(), req)
	if err != nil {
		return Invocation{}, err
	}
	args := []string{"exec"}
	switch {
	case p.resumeID != "":
		args = append(args, "resume", p.resumeID)
	case p.cont:
		args = append(args, "resume", "--last")
	}
	args = append(args, "--json")
	if p.model != "" {
		args = append(args, "-m", p.model)
	}
	if p.systemPrompt != "" {
		// developer_instructions is PREPENDED to Codex's own developer
		// message: it adds, it never replaces. The value is encoded as a TOML
		// basic string because `-c key=value` parses the value as TOML and
		// only falls back to a raw literal when that fails — a contract that
		// parsed as a bool would be a hard error, and a quoted one would lose
		// its quotes.
		args = append(args, "-c", "developer_instructions="+tomlString(p.systemPrompt))
	}
	if p.readOnly {
		// `codex exec resume` has NO -s/--sandbox flag, so the mode travels
		// as config in BOTH forms: one path, verified in both.
		mode := "read-only"
		if p.exec {
			mode = "workspace-write" // corre hoom finding y los tests
		}
		args = append(args, "-c", "sandbox_mode="+tomlString(mode))
	}
	args = append(args, p.prompt)
	return Invocation{Bin: c.Bin(), Args: args, Ignored: p.ignored}, nil
}

// tomlString encodes any text as a TOML basic string, so the round trip
// through `-c key=value` is exact instead of depending on a fallback.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u%04X`, r)
				continue
			}
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// Normalize understands Codex's JSONL. Same rule as every adapter: what it
// does not recognize degrades to a text event with the whole line — the log
// loses detail, never content.
func (codex) Normalize(line string) []Event {
	line = strings.TrimRight(line, "\r\n")
	if strings.TrimSpace(line) == "" {
		return nil
	}
	now := time.Now().UTC()
	if evs, known := parseCodexLine(line, now); known {
		return evs
	}
	return []Event{{TS: now, Kind: "text", Detail: line}}
}

// codexItem is the part of an item hoom knows how to read. Everything else
// is reached generically, so no field name is invented.
type codexItem struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Message  string `json:"message"`
	Command  string `json:"command"`
	Status   string `json:"status"`
	ExitCode *int   `json:"exit_code"`
	Changes  []struct {
		Path string `json:"path"`
		Kind string `json:"kind"`
	} `json:"changes"`
}

// parseCodexLine returns the events of one line and whether the line was a
// shape Codex declares. A known shape that narrates nothing (turn.started,
// item.updated) returns no events AND known: silence on purpose is not the
// same as a line hoom failed to read.
func parseCodexLine(line string, ts time.Time) ([]Event, bool) {
	var msg struct {
		Type     string          `json:"type"`
		ThreadID string          `json:"thread_id"`
		Message  string          `json:"message"`
		Item     json.RawMessage `json:"item"`
		Error    *struct {
			Message string `json:"message"`
		} `json:"error"`
		Usage *struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return nil, false
	}
	switch msg.Type {
	case "thread.started":
		// the only event that opens the session: it carries the thread id,
		// the handle for `codex exec resume`
		return []Event{{TS: ts, Kind: "start", Detail: "thread", SessionID: msg.ThreadID}}, true
	case "turn.started", "item.updated":
		return nil, true
	case "turn.completed":
		detail := "turn completado"
		if msg.Usage != nil {
			detail = fmt.Sprintf("turn completado (%d tokens de entrada, %d de salida)",
				msg.Usage.InputTokens, msg.Usage.OutputTokens)
		}
		return []Event{{TS: ts, Kind: "end", Detail: detail}}, true
	case "turn.failed":
		detail := "turn fallido"
		if msg.Error != nil && strings.TrimSpace(msg.Error.Message) != "" {
			detail = clip(msg.Error.Message)
		}
		return []Event{{TS: ts, Kind: "error", Detail: detail}}, true
	case "error":
		detail := clip(msg.Message)
		if detail == "" {
			detail = "error"
		}
		return []Event{{TS: ts, Kind: "error", Detail: detail}}, true
	case "item.started", "item.completed":
		var it codexItem
		if len(msg.Item) == 0 || json.Unmarshal(msg.Item, &it) != nil {
			return nil, false
		}
		return codexItemEvents(msg.Type == "item.started", it, msg.Item, ts), true
	}
	return nil, false
}

// codexItemEvents maps ONE item. Actions are narrated when they start, which
// is when the narration is useful, and speak again only if they failed:
// Normalize is stateless by contract, so it cannot deduplicate a completed
// item against its own start.
func codexItemEvents(started bool, it codexItem, raw json.RawMessage, ts time.Time) []Event {
	switch it.Type {
	case "agent_message":
		if started || strings.TrimSpace(it.Text) == "" {
			return nil
		}
		return []Event{{TS: ts, Kind: "text", Detail: clip(it.Text)}}
	case "error":
		if started {
			return nil
		}
		detail := clip(it.Message)
		if detail == "" {
			detail = "error"
		}
		return []Event{{TS: ts, Kind: "error", Detail: detail}}
	case "command_execution":
		if started {
			return []Event{{TS: ts, Kind: "tool", Detail: "command_execution: " + clip(it.Command)}}
		}
		if !codexFailed(it) {
			return nil
		}
		return []Event{{TS: ts, Kind: "tool",
			Detail: fmt.Sprintf("command_execution fallo (exit %s): %s", exitText(it), clip(it.Command))}}
	case "file_change":
		if started {
			return []Event{{TS: ts, Kind: "tool", Detail: "file_change: " + changesText(it)}}
		}
		if !codexFailed(it) {
			return nil
		}
		return []Event{{TS: ts, Kind: "tool", Detail: "file_change fallo: " + changesText(it)}}
	}
	// A type this adapter never saw run: it is named, not guessed, and its
	// most readable field travels with it.
	if started {
		return nil
	}
	detail := codexDetail(raw)
	if detail == "" {
		detail = clip(string(raw))
	}
	return []Event{{TS: ts, Kind: "text", Detail: it.Type + ": " + detail}}
}

func codexFailed(it codexItem) bool {
	return it.Status == "failed" || (it.ExitCode != nil && *it.ExitCode != 0)
}

func exitText(it codexItem) string {
	if it.ExitCode == nil {
		return "?"
	}
	return fmt.Sprintf("%d", *it.ExitCode)
}

// changesText names what a file_change touched: the first path plus how many
// more, which is what a human reads in a live stream.
func changesText(it codexItem) string {
	if len(it.Changes) == 0 {
		return "sin rutas"
	}
	first := it.Changes[0]
	out := first.Kind + " " + first.Path
	if n := len(it.Changes) - 1; n > 0 {
		out = fmt.Sprintf("%s (+%d rutas)", out, n)
	}
	return clip(out)
}

// codexDetail picks the most human-relevant string field of an unmapped item,
// the same way the Claude adapter reads a tool call.
func codexDetail(raw json.RawMessage) string {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	for _, key := range []string{"text", "message", "command", "query", "title", "path"} {
		if v, ok := m[key].(string); ok && strings.TrimSpace(v) != "" {
			return clip(v)
		}
	}
	return ""
}
