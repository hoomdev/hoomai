package providers

// ConEstado is the optional interface of an adapter that declares the state
// its CLI keeps on the machine (contencion-del-writer, "El estado del
// provider"). It is NOT part of Provider: a provider that does not implement
// it (the fake providers of the tests, gemini, opencode) keeps compiling and
// running, and a role that executes with it gets the encierro level
// `ninguno` with the motive `<provider> no declara su estado`
// (encierro.MotivoSinEstado). claude and codex implement it.
type ConEstado interface {
	Estado() Estado
}

// Estado is what a CLI keeps outside and inside the tree, as its adapter
// declares it.
//
// Paths outside the tree are absolute or start with "~/" (the $HOME of the
// process that reads them); paths inside the tree are relative to the tree's
// root, with "/", and one that ends in "/" names a whole directory.
type Estado struct {
	// Escribe are the paths outside the tree its CLI writes during a run
	// (sessions, logs, credentials it refreshes). The encierro of a role that
	// runs with this provider leaves writable, outside the task's tree and
	// its cache, only these (plus encierro.escribibles of hoom.yaml).
	Escribe []string
	// EnArbol is its config inside the tree. Together with `.claude/`,
	// `.codex/` and `.mcp.json`, which always count, it is what the rule
	// config-de-agentes compares in the tree (agentcmd.FotoConfigAgentes),
	// with or without encierro; with encierro it stays read-only inside the
	// run tree when it exists.
	EnArbol []string
	// Corre says how to read, from each config file outside the tree, only
	// what makes code run in the next session: config-de-agentes compares
	// that and nothing else of the file (agentcmd.FotoConfigAgentes).
	Corre []Corre
}

// Corre is one config file outside the tree and the part of it that makes
// code run.
type Corre struct {
	// Archivo is the file, absolute or starting with "~/".
	Archivo string
	// Claves are the JSON paths whose value makes code run, each one a list
	// of object keys joined with "." where "*" stands for any one key at
	// that level: "hooks" is the top-level key hooks, and
	// "projects.*.mcpServers" the key mcpServers of every entry of the
	// top-level object projects (the keys of projects are paths, dots
	// included: "*" matches one whole key). A key that is not there reads as
	// absent, and so does a file that is not there; a file that is not valid
	// JSON reads as a whole. Changing any other key of the file is not a
	// change. Empty Claves = the whole file, byte for byte (a file that is
	// not JSON, like codex's config.toml).
	Claves []string
}
