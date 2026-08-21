// Package providers knows the supported AI CLIs and how to drive them
// headless. hoomAI never talks to a model API: it executes the USER'S own
// CLI as a subprocess — same auth, same config, same subagents as when the
// user types it in a terminal. All provider-specific knowledge (binary,
// headless flags, session continuation, output format) lives in this one
// table; the rest of the system sees a normalized event stream.
package providers

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Event is the normalized unit of a run's narration. One schema for every
// provider so the UI (and the future v4 theater) is written once.
// Kind: start | text | tool | agent | end | error.
type Event struct {
	TS     time.Time `json:"ts"`
	Kind   string    `json:"kind"`
	Agent  string    `json:"agent,omitempty"`
	Detail string    `json:"detail,omitempty"`
}

// Spec describes how to run one provider headless.
type Spec struct {
	Name string
	Bin  string
	// args builds the full argument list. cont=true continues the previous
	// session in the same directory using the CLI's native mechanism.
	args func(prompt string, cont bool) []string
	// Structured marks providers whose stdout is machine-parseable
	// (stream-json). The rest degrade to plain text events — declared
	// degradation, never an error.
	Structured bool
	// CanContinue is false for CLIs without native headless resume; a
	// continuation then starts fresh and the run says so out loud.
	CanContinue bool
}

// table is the single source of truth for supported providers, mirroring
// the targets of `hoom agents`.
var table = []Spec{
	{
		Name: "claude", Bin: "claude", Structured: true, CanContinue: true,
		args: func(prompt string, cont bool) []string {
			a := []string{"-p", "--output-format", "stream-json", "--verbose"}
			if cont {
				a = append(a, "--continue")
			}
			return append(a, prompt)
		},
	},
	{
		Name: "opencode", Bin: "opencode", Structured: false, CanContinue: true,
		args: func(prompt string, cont bool) []string {
			a := []string{"run"}
			if cont {
				a = append(a, "--continue")
			}
			return append(a, prompt)
		},
	},
	{
		Name: "codex", Bin: "codex", Structured: false, CanContinue: true,
		args: func(prompt string, cont bool) []string {
			if cont {
				return []string{"exec", "resume", "--last", prompt}
			}
			return []string{"exec", prompt}
		},
	},
	{
		Name: "gemini", Bin: "gemini", Structured: false, CanContinue: false,
		args: func(prompt string, cont bool) []string {
			return []string{"-p", prompt}
		},
	},
}

// Info is a provider's availability on this machine.
type Info struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Bin       string `json:"bin,omitempty"`
}

// Detect reports every supported provider and whether its binary is in
// PATH right now.
func Detect() []Info {
	out := make([]Info, 0, len(table))
	for _, s := range table {
		info := Info{Name: s.Name}
		if path, err := exec.LookPath(s.Bin); err == nil {
			info.Installed = true
			info.Bin = path
		}
		out = append(out, info)
	}
	return out
}

// JSONBytes renders Detect exactly as both the CLI and the Studio emit it.
func JSONBytes() ([]byte, error) {
	return json.MarshalIndent(Detect(), "", "  ")
}

// Lookup returns the spec for a provider name.
func Lookup(name string) (Spec, error) {
	for _, s := range table {
		if s.Name == name {
			return s, nil
		}
	}
	names := make([]string, 0, len(table))
	for _, s := range table {
		names = append(names, s.Name)
	}
	return Spec{}, fmt.Errorf("provider desconocido %q (soportados: %s)", name, strings.Join(names, ", "))
}

// Command materializes the headless invocation for a prompt.
func (s Spec) Command(prompt string, cont bool) (bin string, args []string) {
	return s.Bin, s.args(prompt, cont)
}

// Normalize converts one stdout line into events. Structured providers get
// a real parser with a text fallback for anything unrecognized; the rest
// are text as-is. Empty lines produce nothing.
func (s Spec) Normalize(line string) []Event {
	line = strings.TrimRight(line, "\r\n")
	if strings.TrimSpace(line) == "" {
		return nil
	}
	now := time.Now().UTC()
	if s.Structured && s.Name == "claude" {
		if evs := parseClaudeLine(line, now); evs != nil {
			return evs
		}
	}
	return []Event{{TS: now, Kind: "text", Detail: line}}
}

// parseClaudeLine understands Claude Code's stream-json lines. Defensive by
// design: any shape it does not recognize returns nil and the caller falls
// back to a text event — the log is never lost, only detail.
func parseClaudeLine(line string, ts time.Time) []Event {
	var msg struct {
		Type    string `json:"type"`
		Subtype string `json:"subtype"`
		Result  string `json:"result"`
		Message struct {
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
		return []Event{{TS: ts, Kind: "start", Detail: msg.Subtype}}
	case "result":
		detail := msg.Subtype
		if msg.Result != "" {
			detail = msg.Result
		}
		return []Event{{TS: ts, Kind: "end", Detail: clip(detail)}}
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
