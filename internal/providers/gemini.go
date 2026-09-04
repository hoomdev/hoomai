package providers

// gemini drives Gemini CLI headless (`gemini -p`). No structured output,
// no session continuation in headless mode: every optional field is
// declared absent and reported when sent.
type gemini struct{}

func (gemini) Name() string                  { return "gemini" }
func (gemini) Bin() string                   { return "gemini" }
func (gemini) Capabilities() Capabilities    { return Capabilities{} }
func (gemini) Normalize(line string) []Event { return textEvents(line) }

func (g gemini) Command(req Request) (Invocation, error) {
	p, err := resolve(g.Name(), g.Capabilities(), req)
	if err != nil {
		return Invocation{}, err
	}
	return Invocation{Bin: g.Bin(), Args: []string{"-p", p.prompt}, Ignored: p.ignored}, nil
}
