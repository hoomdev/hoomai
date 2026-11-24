package modelos

// proporcion-por-riesgo: the levels of a model. A level is written where a
// model is written (agents.<rol>.model, agents.<rol>.<r>.model,
// review.model and --model) and hoom translates it with the launch's
// provider, here and only here. The table lives apart from the catalog:
// Catalogos stays exactly the spec's tables (CA-572).

// The levels, in lowercase: `Fuerte` or `MEDIO` are not levels, they are
// model names and pass as written.
const (
	Fuerte = "fuerte"
	Medio  = "medio"
	Barato = "barato"
)

// Niveles are the levels, in this order: fuerte, medio, barato.
var Niveles = []string{Fuerte, Medio, Barato}

// FilaDeNiveles is one row of the table of levels: the model of each level
// for one provider.
type FilaDeNiveles struct {
	Provider string
	Fuerte   string
	Medio    string
	Barato   string
}

// TablaDeNiveles is the table of levels, one row per provider hoom passes a
// model to, in this order: claude (opus, sonnet, haiku: aliases of its CLI)
// and codex (gpt-6-astra, gpt-6.1-sol, gpt-6-luna, verified with the
// catalog on Verificado). It is exactly the spec's table. The result is a
// copy: changing it changes nothing here.
func TablaDeNiveles() []FilaDeNiveles {
	return append([]FilaDeNiveles(nil), tablaDeNiveles...)
}

// tablaDeNiveles is the spec's table; TablaDeNiveles hands out copies.
var tablaDeNiveles = []FilaDeNiveles{
	{Provider: "claude", Fuerte: "opus", Medio: "sonnet", Barato: "haiku"},
	{Provider: "codex", Fuerte: "gpt-6-astra", Medio: "gpt-6.1-sol", Barato: "gpt-6-luna"},
}

// EsNivel says whether m is one of Niveles exactly as written: lowercase,
// without spaces.
func EsNivel(m string) bool {
	for _, n := range Niveles {
		if m == n {
			return true
		}
	}
	return false
}

// Traduccion is a model as one provider receives it.
type Traduccion struct {
	// Modelo is what the provider receives: the model of the level for the
	// provider (TablaDeNiveles), a name that is not a level exactly as
	// written (also for a provider that receives no model, as today), or
	// "" for a level on a provider without a row (gemini, opencode, an
	// unknown or empty provider) and for an empty modelo.
	Modelo string
	// Nivel is the level modelo was ("" when it is a name or empty).
	Nivel string
	// Recibe is RecibeModelo(provider): false for gemini and opencode, and
	// then no model reaches the provider, as today.
	Recibe bool
}

// Traducir is modelo (a level or a name) for provider: the ONE place a level
// becomes a model name. No level reaches a provider, the long command or a
// screen without going through here.
func Traducir(provider, modelo string) Traduccion {
	t := Traduccion{Recibe: RecibeModelo(provider)}
	if !EsNivel(modelo) {
		t.Modelo = modelo
		return t
	}
	t.Nivel = modelo
	for _, f := range tablaDeNiveles {
		if f.Provider != provider {
			continue
		}
		switch modelo {
		case Fuerte:
			t.Modelo = f.Fuerte
		case Medio:
			t.Modelo = f.Medio
		case Barato:
			t.Modelo = f.Barato
		}
	}
	if !t.Recibe {
		t.Modelo = ""
	}
	return t
}
