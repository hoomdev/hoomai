// Package modelos is hoom's catalog of models and efforts (spec
// tablero-con-paneles, "El catalogo de modelos"): the ONE place that says
// which models and which reasoning efforts hoom offers for each provider it
// passes a model to (today claude and codex), each with its line, in the
// order of the spec's tables, and the version of the provider's CLI the
// entry was verified against. hoom tablero's Modelo and Esfuerzo selectors
// read it.
//
// The catalog proposes, it never forbids: hoom does not validate a model
// (a model that is not here goes as the person writes it, and the provider
// says whether it takes it). hoom.yaml widens it with models.<provider>
// (manifest.ModelCatalog): Modelos joins both lists.
//
// The lines carry no accents: they are the same with UTF-8 and in ASCII.
//
// This package imports providers (whose Capabilities say which providers
// take a model and an effort) and nothing that imports manifest: manifest
// validates models: with ConModelo.
package modelos

// Verificado is the date every entry of the catalog was verified against
// its provider's CLI ("2026-11-09").
const Verificado = "2026-11-09"

// Opcion is one model or one effort of the catalog: its name, as the
// provider's flag takes it ("sonnet", "gpt-6.1-sol", "xhigh"), and its line
// (one sentence without accents: "para la mayoria de las tareas").
type Opcion struct {
	Nombre string
	Linea  string
}

// Catalogo is the entry of one provider hoom passes a model to.
type Catalogo struct {
	// Provider is the provider's name in hoom ("claude", "codex").
	Provider string
	// Version is the CLI the entry was verified against, as the spec names
	// it: "Claude Code 2.1.319" for claude, "codex-cli 0.160.0" for codex.
	Version string
	// Modelos are its models, in the order of the spec's table.
	Modelos []Opcion
	// Esfuerzos are its reasoning efforts, in the order of the spec's
	// table: claude low, medium, high, xhigh and max; codex the same and
	// ultra.
	Esfuerzos []Opcion
}

// Catalogos is the whole catalog: one entry per provider hoom passes a
// model to, in this order: claude, codex. It is exactly the spec's tables
// (CA-572). The result is a copy: changing it changes nothing here.
func Catalogos() []Catalogo {
	return nil
}

// De is the catalog's entry of provider, and false when the catalog has
// none (a provider hoom passes no model to, or one it does not know).
func De(provider string) (Catalogo, bool) {
	return Catalogo{}, false
}

// ConModelo are the names of hoom's providers (providers.All) whose
// Capabilities have Model, sorted: today "claude" and "codex". They are the
// keys models: of hoom.yaml takes, and the providers the catalog has an
// entry for.
func ConModelo() []string {
	return nil
}

// RecibeModelo says whether hoom passes provider a model: whether the
// provider is in hoom's registry and its Capabilities have Model. False for
// gemini and opencode today, and for a name hoom does not know.
func RecibeModelo(provider string) bool {
	return false
}

// RecibeEsfuerzo says whether hoom passes provider a reasoning effort:
// whether the provider is in hoom's registry and its Capabilities have
// Effort. False for gemini and opencode today, and for a name hoom does not
// know.
func RecibeEsfuerzo(provider string) bool {
	return false
}

// Propio is one entry of models.<provider> in hoom.yaml: a model the
// project adds to the catalog, with an optional description.
type Propio struct {
	Nombre      string
	Descripcion string
}

// Modelos is the list of models offered for provider, in this order: the
// catalog's models of provider (none when it has no entry), then the
// propios whose name is not a catalog model of provider, in their order,
// each with its Descripcion as its line ("" without one). A propio with
// the name of a catalog model does not repeat it: when its Descripcion is
// not empty it becomes that model's line, and when it is empty the
// catalog's line stays. Two propios with one name count once: the first.
// The result is a new slice.
func Modelos(provider string, propios []Propio) []Opcion {
	return nil
}
