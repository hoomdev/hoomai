package providers

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// claude drives Claude Code headless (`claude -p`). Verified against Claude
// Code 2.1.259 and re-verified on 2.1.263: stream-json output, resume by
// session id, model selection, appended system prompt, tool allow/deny,
// --max-turns (registered but absent from --help) and --max-budget-usd. The
// `result` line reports what the invocation spent — per invocation, never per
// session: a --resume of the same session reported its own turn and its own
// cost, not the accumulated total.
type claude struct{}

func (claude) Name() string { return "claude" }
func (claude) Bin() string  { return "claude" }

func (claude) Capabilities() Capabilities {
	return Capabilities{
		Structured: true, Continue: true, Resume: true, SessionID: true,
		Model: true, SystemPrompt: true, Tools: true, ReadOnly: true, Unattended: true,
		MaxTurns: true, Budget: true,
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
	allow, deny := p.allow, p.deny
	if p.readOnly {
		// the role's limit, in the vocabulary of THIS CLI; what the caller
		// asked for on its own is kept, not replaced
		ra, rd := claudeReadOnlyTools(p.exec)
		allow, deny = dedup(append(allow, ra...)), dedup(append(deny, rd...))
	}
	if p.unattended && !p.readOnly {
		// nobody answers a permission prompt in -p mode: a tool that is not
		// pre-approved is denied in silence. A writing role gets its tools
		// up front, by NAME and not by a bypass mode, so the argv in the run
		// log says exactly what the role could do.
		allow = dedup(append(allow, claudeWriteTools()...))
	}
	if len(allow) > 0 {
		args = append(args, "--allowedTools", strings.Join(allow, ","))
	}
	if len(deny) > 0 {
		args = append(args, "--disallowedTools", strings.Join(deny, ","))
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

// claudeWriteTools is what a role that writes needs when nobody attends the
// run: read, edit, create and run. The scope gate after the run, not this
// list, is what bounds WHERE it may write.
func claudeWriteTools() []string {
	return []string{"Read", "Grep", "Glob", "Edit", "Write", "MultiEdit", "NotebookEdit", "Bash"}
}

// claudeReadOnlyTools translates the read-only intention into Claude's OWN
// tool names — the SAME vocabulary `hoom agents --target claude` writes into
// .claude/agents/*.md. It lives in the adapter because the adapter is what
// knows the dialect: the caller only says that the role does not write.
func claudeReadOnlyTools(exec bool) (allow, deny []string) {
	allow = []string{"Read", "Grep", "Glob"}
	deny = []string{"Edit", "Write", "MultiEdit", "NotebookEdit"}
	if exec {
		allow = append(allow, "Bash") // corre hoom verify/check/finding y tests
	} else {
		deny = append(deny, "Bash")
	}
	return allow, deny
}

// Normalize understands Claude Code's stream-json lines. Defensive by
// design: a top-level type it does not recognize degrades to a text event —
// the log is never lost, only detail. What it DOES recognize as the CLI
// talking about itself (system, rate_limit_event) becomes a `system` event
// instead of raw JSON pretending to be narration.
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

// claudeMsg is the part of a stream-json line hoom knows how to read.
// Verified against Claude Code 2.1.263: `exit_code` arrives as a STRING
// ("0"), so it is read as raw JSON and rendered as text — decoding it as an
// integer would break the whole line for a field that is only narration.
type claudeMsg struct {
	Type      string          `json:"type"`
	Subtype   string          `json:"subtype"`
	Result    string          `json:"result"`
	IsError   bool            `json:"is_error"`
	SessionID string          `json:"session_id"`
	Errors    json.RawMessage `json:"errors"`
	// system/hook_*: which hook ran and how it went
	HookName string          `json:"hook_name"`
	Outcome  string          `json:"outcome"`
	ExitCode json.RawMessage `json:"exit_code"`
	// result: what the invocation spent. Per invocation, never per session
	// (measured on a --resume: the same session reported 1 turn twice).
	NumTurns     int      `json:"num_turns"`
	DurationMS   int      `json:"duration_ms"`
	TotalCostUSD *float64 `json:"total_cost_usd"`
	Usage        struct {
		InputTokens         int `json:"input_tokens"`
		OutputTokens        int `json:"output_tokens"`
		CacheReadTokens     int `json:"cache_read_input_tokens"`
		CacheCreationTokens int `json:"cache_creation_input_tokens"`
	} `json:"usage"`
	RateLimit *struct {
		Status         string `json:"status"`
		RateLimitType  string `json:"rateLimitType"`
		UnifiedWindows map[string]struct {
			Utilization float64 `json:"utilization"`
		} `json:"unifiedWindows"`
	} `json:"rate_limit_info"`
	Message struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
	} `json:"message"`
}

func parseClaudeLine(line string, ts time.Time) []Event {
	var msg claudeMsg
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return nil
	}
	switch msg.Type {
	case "system":
		// Only init opens the session: it carries the session id, the
		// handle for --resume. Every other subtype (hooks, compaction,
		// subagent tasks, status) is the CLI talking about ITSELF, and that
		// has its own kind: never a second start, never raw JSON pretending
		// to be narration.
		if msg.Subtype != "init" {
			return []Event{{TS: ts, Kind: "system", Detail: claudeSystemDetail(msg, line)}}
		}
		return []Event{{TS: ts, Kind: "start", Detail: msg.Subtype, SessionID: msg.SessionID}}
	case "rate_limit_event":
		// plumbing too, and it carries a session_id that must NOT open a
		// session: only init does that.
		return []Event{{TS: ts, Kind: "system", Detail: claudeRateLimitDetail(msg, line)}}
	case "result":
		text := clip(msg.Result)
		if text == "" {
			text = errorsText(msg.Errors)
		}
		// the cost was spent whether the invocation succeeded or failed: a
		// run that failed expensively has to be able to say how much.
		usage := claudeUsage(msg)
		if msg.Subtype == "success" && !msg.IsError {
			if text == "" {
				text = msg.Subtype
			}
			return []Event{{TS: ts, Kind: "end", Detail: text, SessionID: msg.SessionID, Usage: usage}}
		}
		sub := msg.Subtype
		if sub == "" {
			sub = "error"
		}
		detail := sub
		if text != "" {
			detail = sub + ": " + text
		}
		return []Event{{TS: ts, Kind: "error", Detail: detail, SessionID: msg.SessionID, Usage: usage}}
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

// claudeSystemDetail summarizes the plumbing lines hoom knows how to read.
// A subtype it does not know keeps the WHOLE line as detail: the kind already
// says it is the CLI talking about itself, and the content must not get worse
// for not having been understood.
func claudeSystemDetail(msg claudeMsg, line string) string {
	hook := strings.TrimSpace(msg.HookName)
	switch {
	case msg.Subtype == "hook_started" && hook != "":
		return "hook " + hook + " arranco"
	case msg.Subtype == "hook_response" && hook != "":
		detail := "hook " + hook
		if out := strings.TrimSpace(msg.Outcome); out != "" {
			detail += ": " + out
		}
		if code := rawText(msg.ExitCode); code != "" {
			detail += " (exit " + code + ")"
		}
		return detail
	case msg.Subtype != "":
		return msg.Subtype + ": " + clip(line)
	}
	return clip(line)
}

// claudeRateLimitDetail names the windows the line actually carries, in
// stable order. A window that is not there is not invented.
func claudeRateLimitDetail(msg claudeMsg, line string) string {
	if msg.RateLimit == nil {
		return clip(line)
	}
	detail := "rate limit"
	if st := strings.TrimSpace(msg.RateLimit.Status); st != "" {
		detail += " " + st
	}
	names := make([]string, 0, len(msg.RateLimit.UnifiedWindows))
	for name := range msg.RateLimit.UnifiedWindows {
		names = append(names, name)
	}
	sort.Strings(names)
	var windows []string
	for _, name := range names {
		windows = append(windows, fmt.Sprintf("%s %.1f%%", name, msg.RateLimit.UnifiedWindows[name].Utilization*100))
	}
	if len(windows) == 0 {
		if t := strings.TrimSpace(msg.RateLimit.RateLimitType); t != "" {
			return detail + " (" + t + ")"
		}
		return detail
	}
	return detail + ": " + strings.Join(windows, ", ")
}

// claudeUsage reads what the invocation spent. Claude's own accounting:
// input_tokens and cache_creation are both NEW input (the second one also got
// written to cache); cache_read is what came back from the cache.
func claudeUsage(msg claudeMsg) *Usage {
	u := &Usage{
		CostUSD:      msg.TotalCostUSD,
		Turns:        msg.NumTurns,
		InputTokens:  msg.Usage.InputTokens + msg.Usage.CacheCreationTokens,
		CachedTokens: msg.Usage.CacheReadTokens,
		OutputTokens: msg.Usage.OutputTokens,
		DurationMS:   msg.DurationMS,
	}
	if u.Empty() {
		return nil
	}
	return u
}

// rawText renders a JSON scalar as the text a human reads, whether the CLI
// sent it as a string or as a number.
func rawText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(string(raw))
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
