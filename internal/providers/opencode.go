package providers

// opencode drives OpenCode headless (`opencode run`). Plain-text output and
// directory-based continuation only; everything else is declared absent.
type opencode struct{}

func (opencode) Name() string                  { return "opencode" }
func (opencode) Bin() string                   { return "opencode" }
func (opencode) Capabilities() Capabilities    { return Capabilities{Continue: true} }
func (opencode) Normalize(line string) []Event { return textEvents(line) }

func (o opencode) Command(req Request) (Invocation, error) {
	p, err := resolve(o.Name(), o.Capabilities(), req)
	if err != nil {
		return Invocation{}, err
	}
	args := []string{"run"}
	if p.cont {
		args = append(args, "--continue")
	}
	args = append(args, p.prompt)
	return Invocation{Bin: o.Bin(), Args: args, Ignored: p.ignored}, nil
}
