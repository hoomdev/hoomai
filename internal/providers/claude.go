package providers

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// claude drives Claude Code headless (`claude -p`). Verified against Claude
// Code 2.1.259: stream-json output, resume by session id, model selection,
// appended system prompt, tool allow/deny, --max-turns (registered but
// absent from --help) and --max-budget-usd.
type claude struct{}

func (claude) Name() string { return "claude" }
func (claude) Bin() string  { return "claude" }

func (claude) Capabilities() Capabilities {
	return Capabilities{
		Structured: true, Continue: true, Resume: true, SessionID: true,
		Model: true, SystemPrompt: true, Tools: true, MaxTurns: true, Budget: true,
	}
}

// Command builds the headless invocation. The tool lists go FIRST: they are
// variadic options in Claude's CLI parser (commander) and swallow every
// positional until the next option — placed last they would eat the prompt.
// The prompt is always the final argument.
func (c claude) Command(req Request) (Invocation, error) {
	p, err := resolve(c.Name(), c.Capabilities(), req)
	if err != nil {
		return Invocation{}, err
	}
	var args []string
	if len(p.allow) > 0 {
		args = append(args, "--allowedTools", strings.Join(p.allow, ","))
	}
	if len(p.deny) > 0 {
		args = append(args, "--disallowedTools", strings.Join(p.deny, ","))
	}
	args = append(args, "-p", "--output-format", "stream-json", "--verbose")
	switch {
	case p.resumeID != "":
		args = append(args, "--resume", p.resumeID)
	case p.cont:
		args = append(args, "--continue")
	}
	if p.model != "" {
		args = append(args, "--model", p.model)
	}
	if p.systemPrompt != "" {
		// append, never --system-prompt: replacing the CLI's own prompt
		// would drop its normal behavior (CLAUDE.md, native subagents)
		args = append(args, "--append-system-prompt", p.systemPrompt)
	}
	if p.maxTurns > 0 {
		args = append(args, "--max-turns", strconv.Itoa(p.maxTurns))
	}
	if p.budgetUSD > 0 {
		// plain decimal, never scientific notation
		args = append(args, "--max-budget-usd", strconv.FormatFloat(p.budgetUSD, 'f', -1, 64))
	}
	args = append(args, p.prompt)
	return Invocation{Bin: c.Bin(), Args: args, Ignored: p.ignored}, nil
}

// Normalize understands Claude Code's stream-json lines. Defensive by
// design: any shape it does not recognize degrades to a text event — the
// log is never lost, only detail.
// ReadOnlyTools is the SAME vocabulary that `hoom agents --target claude`
// writes into .claude/agents/*.md: one table of roles, one set of names.
func (claude) ReadOnlyTools(exec bool) (allow, deny []string) {
	allow = []string{"Read", "Grep", "Glob"}
	deny = []string{"Edit", "Write", "MultiEdit", "NotebookEdit"}
	if exec {
		allow = append(allow, "Bash") // corre hoom verify/check/finding y tests
	} else {
		deny = append(deny, "Bash")
	}
	return allow, deny
}

func (claude) Normalize(line string) []Event {
	line = strings.TrimRight(line, "\r\n")
	if strings.TrimSpace(line) == "" {
		return nil
	}
	now := time.Now().UTC()
	if evs := parseClaudeLine(line, now); evs != nil {
		return evs
	}
	return []Event{{TS: now, Kind: "text", Detail: line}}
}

func parseClaudeLine(line string, ts time.Time) []Event {
	var msg struct {
		Type      string          `json:"type"`
		Subtype   string          `json:"subtype"`
		Result    string          `json:"result"`
		IsError   bool            `json:"is_error"`
		SessionID string          `json:"session_id"`
		Errors    json.RawMessage `json:"errors"`
		Message   struct {
			Content []struct {
				Type  string          `json:"type"`
				Text  string          `json:"text"`
				Name  string          `json:"name"`
				Input json.RawMessage `json:"input"`
			} `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return nil
	}
	switch msg.Type {
	case "system":
		// Only init opens the session: it carries the session id, the
		// handle for --resume. Other system subtypes (hooks, compaction,
		// subagent tasks, status) are not narration hoom understands:
		// they fall back to text with the full line — never lost, never
		// mistaken for a second start.
		if msg.Subtype != "init" {
			return nil
		}
		return []Event{{TS: ts, Kind: "start", Detail: msg.Subtype, SessionID: msg.SessionID}}
	case "result":
		text := clip(msg.Result)
		if text == "" {
			text = errorsText(msg.Errors)
		}
		if msg.Subtype == "success" && !msg.IsError {
			if text == "" {
				text = msg.Subtype
			}
			return []Event{{TS: ts, Kind: "end", Detail: text, SessionID: msg.SessionID}}
		}
		sub := msg.Subtype
		if sub == "" {
			sub = "error"
		}
		detail := sub
		if text != "" {
			detail = sub + ": " + text
		}
		return []Event{{TS: ts, Kind: "error", Detail: detail, SessionID: msg.SessionID}}
	case "assistant", "user":
		var evs []Event
		for _, c := range msg.Message.Content {
			switch c.Type {
			case "text":
				if strings.TrimSpace(c.Text) != "" {
					evs = append(evs, Event{TS: ts, Kind: "text", Detail: clip(c.Text)})
				}
			case "tool_use":
				ev := Event{TS: ts, Kind: "tool", Detail: c.Name}
				var input map[string]any
				if json.Unmarshal(c.Input, &input) == nil {
					if c.Name == "Task" {
						// delegacion a subagente: el rol es el actor visible
						if sub, ok := input["subagent_type"].(string); ok {
							ev.Kind = "agent"
							ev.Agent = sub
						}
					}
					if d := toolDetail(c.Name, input); d != "" {
						ev.Detail = c.Name + ": " + d
					}
				}
				evs = append(evs, ev)
			}
		}
		if evs == nil {
			return nil
		}
		return evs
	}
	return nil
}

// errorsText joins a result's `errors` field when it is a list of strings;
// any other shape yields nothing rather than failing the whole parse.
func errorsText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var list []string
	if json.Unmarshal(raw, &list) == nil {
		return clip(strings.Join(list, "; "))
	}
	return ""
}

// toolDetail extracts the most human-relevant argument of a tool call.
func toolDetail(tool string, input map[string]any) string {
	for _, key := range []string{"file_path", "path", "pattern", "command", "description", "prompt", "query"} {
		if v, ok := input[key].(string); ok && v != "" {
			return clip(v)
		}
	}
	return ""
}

func clip(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}
