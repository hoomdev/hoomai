package providers

// codex drives Codex CLI headless (`codex exec`). Plain-text output and
// last-session continuation only; its v2 (native JSON, resume by id) has
// its own spec.
type codex struct{}

func (codex) Name() string                  { return "codex" }
func (codex) Bin() string                   { return "codex" }
func (codex) Capabilities() Capabilities    { return Capabilities{Continue: true} }
func (codex) Normalize(line string) []Event { return textEvents(line) }

func (c codex) Command(req Request) (Invocation, error) {
	p, err := resolve(c.Name(), c.Capabilities(), req)
	if err != nil {
		return Invocation{}, err
	}
	args := []string{"exec"}
	if p.cont {
		args = append(args, "resume", "--last")
	}
	args = append(args, p.prompt)
	return Invocation{Bin: c.Bin(), Args: args, Ignored: p.ignored}, nil
}
