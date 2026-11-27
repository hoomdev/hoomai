package initcmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hoomdev/hoomai/internal/manifest"
)

func boolPtr(v bool) *bool    { return &v }
func strPtr(v string) *string { return &v }

func cloneGates(src map[string]manifest.Gate) map[string]manifest.Gate {
	out := make(map[string]manifest.Gate, len(src))
	for k, g := range src {
		cp := g
		if g.Required != nil {
			v := *g.Required
			cp.Required = &v
		}
		if g.Cmd != nil {
			v := *g.Cmd
			cp.Cmd = &v
		}
		if g.DiffCmd != nil {
			v := *g.DiffCmd
			cp.DiffCmd = &v
		}
		if len(g.AplicaEn) > 0 {
			cp.AplicaEn = append([]string(nil), g.AplicaEn...)
		}
		out[k] = cp
	}
	return out
}

func gateAusente(notes string) manifest.Gate {
	return manifest.Gate{
		Required: boolPtr(false),
		Cmd:      strPtr(""),
		Notes:    notes,
	}
}

func fileExists(dir, rel string) bool {
	_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
	return err == nil
}

func dirExists(dir, rel string) bool {
	fi, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
	return err == nil && fi.IsDir()
}

func fileContainsAny(dir, rel string, needles ...string) bool {
	raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		return false
	}
	low := strings.ToLower(string(raw))
	for _, n := range needles {
		if strings.Contains(low, strings.ToLower(n)) {
			return true
		}
	}
	return false
}

type packageJSON struct {
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

func readPackageJSON(dir string) (*packageJSON, bool) {
	raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return nil, false
	}
	var p packageJSON
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, false
	}
	return &p, true
}

func (p *packageJSON) hasDep(names ...string) bool {
	if p == nil {
		return false
	}
	for _, n := range names {
		if _, ok := p.Dependencies[n]; ok {
			return true
		}
		if _, ok := p.DevDependencies[n]; ok {
			return true
		}
	}
	return false
}

func (p *packageJSON) hasRealScript(name string) bool {
	if p == nil {
		return false
	}
	s := strings.TrimSpace(p.Scripts[name])
	if s == "" {
		return false
	}
	// npm init default stub is not a real test command
	if name == "test" && strings.Contains(s, "no test specified") {
		return false
	}
	return true
}

type composerJSON struct {
	Require    map[string]string `json:"require"`
	RequireDev map[string]string `json:"require-dev"`
}

func readComposerJSON(dir string) (*composerJSON, bool) {
	raw, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	if err != nil {
		return nil, false
	}
	var c composerJSON
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, false
	}
	return &c, true
}

func (c *composerJSON) hasPkg(names ...string) bool {
	if c == nil {
		return false
	}
	for _, n := range names {
		if _, ok := c.Require[n]; ok {
			return true
		}
		if _, ok := c.RequireDev[n]; ok {
			return true
		}
	}
	return false
}

// AdaptarGates inspects the actual project files in dir and adjusts the
// profile's base gates so `hoom init` configures commands that exist in this
// repository and declares missing ones explicitly as ABSENT (`cmd: ""`, amber)
// instead of failing `hoom verify` in red on tools the project does not use.
//
// When dir does not contain project manifest files (e.g. an empty directory
// initialized with an explicit --profile), the profile's gates are preserved.
func AdaptarGates(dir, profileName string, base map[string]manifest.Gate) (map[string]manifest.Gate, []string) {
	gates := cloneGates(base)
	var notas []string

	switch profileName {
	case "laravel":
		notas = adaptarLaravel(dir, gates)
	case "node":
		notas = adaptarNode(dir, gates)
	case "python":
		notas = adaptarPython(dir, gates)
	case "go":
		notas = adaptarGo(dir, gates)
	case "kmp", "kmp-compose":
		notas = adaptarKMP(dir, gates)
	}

	if n := adaptarSecurity(dir, gates); n != "" {
		notas = append(notas, n)
	}
	return gates, notas
}

func adaptarSecurity(dir string, gates map[string]manifest.Gate) string {
	if fileExists(dir, ".hoom/semgrep/gate.sh") {
		gates["security"] = manifest.Gate{
			Required: boolPtr(true),
			Cmd:      strPtr("sh .hoom/semgrep/gate.sh"),
			Notes:    "Detectado .hoom/semgrep/gate.sh del proyecto.",
		}
		return "gate security: detectado .hoom/semgrep/gate.sh"
	}
	// Only auto-enable semgrep when the directory actually looks like a project
	// (avoids altering empty-directory init behavior in tests).
	tieneProyecto := fileExists(dir, "composer.json") || fileExists(dir, "package.json") ||
		fileExists(dir, "go.mod") || fileExists(dir, "pyproject.toml") ||
		fileExists(dir, "requirements.txt") || fileExists(dir, "setup.py") ||
		fileExists(dir, "settings.gradle.kts")
	if !tieneProyecto {
		return ""
	}
	if _, err := exec.LookPath("semgrep"); err == nil {
		gates["security"] = manifest.Gate{
			Required: boolPtr(true),
			Cmd:      strPtr("semgrep scan --error --quiet --metrics=off --disable-version-check --config p/default --config p/trailofbits --exclude .hoom"),
			DiffCmd:  strPtr("semgrep scan --error --quiet --metrics=off --disable-version-check --config p/default --config p/trailofbits --exclude .hoom --baseline-commit {base}"),
			Notes:    "Detectado semgrep instalado en el entorno.",
		}
		return "gate security: detectado semgrep en PATH"
	}
	return ""
}

func adaptarLaravel(dir string, gates map[string]manifest.Gate) []string {
	comp, ok := readComposerJSON(dir)
	if !ok {
		return nil
	}
	var notas []string

	hasPest := comp.hasPkg("pestphp/pest") || fileExists(dir, "vendor/bin/pest")
	hasPHPUnit := comp.hasPkg("phpunit/phpunit") || fileExists(dir, "vendor/bin/phpunit") ||
		fileExists(dir, "phpunit.xml") || fileExists(dir, "phpunit.xml.dist")

	switch {
	case hasPest:
		gates["test"] = manifest.Gate{
			Required: boolPtr(true),
			Cmd:      strPtr("vendor/bin/pest --colors=never"),
			Notes:    "Detectado Pest en el proyecto.",
		}
	case hasPHPUnit:
		gates["test"] = manifest.Gate{
			Required: boolPtr(true),
			Cmd:      strPtr("vendor/bin/phpunit"),
			Notes:    "Detectado PHPUnit (sin Pest).",
		}
		notas = append(notas, "gate test: usando vendor/bin/phpunit (Pest no detectado)")
	default:
		gates["test"] = gateAusente("No se detecto Pest ni PHPUnit en composer.json/vendor.")
		notas = append(notas, "gate test: AUSENTE (ni Pest ni PHPUnit detectados)")
	}

	// arch: only enable if Pest and tests/Arch exist
	if hasPest && dirExists(dir, "tests/Arch") {
		gates["arch"] = manifest.Gate{
			Required: boolPtr(true),
			Cmd:      strPtr("vendor/bin/pest --group=arch --colors=never"),
			Notes:    "Detectado tests/Arch con Pest.",
		}
	} else {
		gates["arch"] = gateAusente("No se detecto tests/Arch con Pest; dejalo AUSENTE hasta crear tests de arquitectura.")
		notas = append(notas, "gate arch: AUSENTE (no existe tests/Arch con Pest)")
	}

	// static: PHPStan / Larastan
	hasPHPStan := comp.hasPkg("phpstan/phpstan", "larastan/larastan", "nunomaduro/larastan") ||
		fileExists(dir, "vendor/bin/phpstan") || fileExists(dir, "phpstan.neon") || fileExists(dir, "phpstan.neon.dist")
	if hasPHPStan {
		gates["static"] = manifest.Gate{
			Required: boolPtr(true),
			Cmd:      strPtr("vendor/bin/phpstan analyse --no-progress --error-format=raw"),
			Notes:    "Detectado PHPStan/Larastan (nivel definido en phpstan.neon).",
		}
	} else {
		gates["static"] = gateAusente("No se detecto PHPStan/Larastan en el proyecto.")
		notas = append(notas, "gate static: AUSENTE (PHPStan/Larastan no detectado)")
	}

	// lint: Laravel Pint (non-required by default to avoid breaking legacy repos)
	hasPint := comp.hasPkg("laravel/pint") || fileExists(dir, "vendor/bin/pint")
	if hasPint {
		gates["lint"] = manifest.Gate{
			Required: boolPtr(false),
			Cmd:      strPtr("vendor/bin/pint --test"),
			Notes:    "Detectado Laravel Pint (opcional por defecto; sube a required: true cuando el estilo este limpio).",
		}
	} else {
		gates["lint"] = gateAusente("No se detecto Laravel Pint en el proyecto.")
		notas = append(notas, "gate lint: AUSENTE (Laravel Pint no detectado)")
	}

	// mutation: custom scripts/mutacion.sh or Infection
	hasInfection := comp.hasPkg("infection/infection") || fileExists(dir, "vendor/bin/infection")
	switch {
	case fileExists(dir, "scripts/mutacion.sh"):
		gates["mutation"] = manifest.Gate{
			Required: boolPtr(true),
			Cmd:      strPtr("sh scripts/mutacion.sh"),
			DiffCmd:  strPtr("sh scripts/mutacion.sh"),
			Notes:    "Detectado scripts/mutacion.sh del proyecto.",
		}
	case hasInfection:
		// keep profile's infection gate
	default:
		gates["mutation"] = gateAusente("No se detecto Infection ni scripts/mutacion.sh; AUSENTE en ambar.")
		notas = append(notas, "gate mutation: AUSENTE (ni Infection ni scripts/mutacion.sh detectados)")
	}

	// build & optional frontend tests from package.json
	pkg, hasPkg := readPackageJSON(dir)
	nodePrefix := ""
	if fileExists(dir, "scripts/with-node.sh") {
		nodePrefix = "sh scripts/with-node.sh "
	}
	if hasPkg && pkg.hasRealScript("build") {
		gates["build"] = manifest.Gate{
			Required: boolPtr(true),
			Cmd:      strPtr(nodePrefix + "npm run build"),
			Notes:    "Detectado script build en package.json.",
		}
	} else {
		gates["build"] = gateAusente("No se detecto script build en package.json.")
		notas = append(notas, "gate build: AUSENTE (sin script build en package.json)")
	}
	if hasPkg && pkg.hasRealScript("test") {
		gateName := "test_js"
		if pkg.hasDep("vue", "@vitejs/plugin-vue", "vue-tsc") {
			gateName = "test_vue"
		}
		gates[gateName] = manifest.Gate{
			Required: boolPtr(true),
			Cmd:      strPtr(nodePrefix + "npm test"),
			Notes:    "Detectado script test en package.json.",
		}
		notas = append(notas, fmt.Sprintf("gate %s: detectado script test en package.json", gateName))
	}

	return notas
}

func adaptarNode(dir string, gates map[string]manifest.Gate) []string {
	pkg, ok := readPackageJSON(dir)
	if !ok {
		return nil
	}
	var notas []string

	if pkg.hasRealScript("test") {
		gates["test"] = manifest.Gate{
			Required: boolPtr(true),
			Cmd:      strPtr("npm test"),
			Notes:    "Detectado script test en package.json.",
		}
	} else {
		gates["test"] = gateAusente("No se detecto script test real en package.json.")
		notas = append(notas, "gate test: AUSENTE (sin script test real en package.json)")
	}

	if fileExists(dir, "tsconfig.json") || pkg.hasDep("typescript") {
		gates["static"] = manifest.Gate{
			Required: boolPtr(true),
			Cmd:      strPtr("npx --no-install tsc --noEmit"),
			Notes:    "Detectado TypeScript en el proyecto.",
		}
	} else {
		gates["static"] = gateAusente("Proyecto JavaScript sin TypeScript; static AUSENTE.")
		notas = append(notas, "gate static: AUSENTE (sin tsconfig.json ni typescript)")
	}

	switch {
	case pkg.hasRealScript("lint"):
		gates["lint"] = manifest.Gate{
			Required: boolPtr(false),
			Cmd:      strPtr("npm run lint"),
			Notes:    "Detectado script lint en package.json.",
		}
	case pkg.hasDep("@biomejs/biome"):
		gates["lint"] = manifest.Gate{
			Required: boolPtr(false),
			Cmd:      strPtr("npx --no-install biome check ."),
			Notes:    "Detectado Biome en package.json.",
		}
	case pkg.hasDep("eslint"):
		gates["lint"] = manifest.Gate{
			Required: boolPtr(false),
			Cmd:      strPtr("npx --no-install eslint ."),
			Notes:    "Detectado ESLint en package.json.",
		}
	default:
		gates["lint"] = gateAusente("No se detecto linter en package.json.")
	}

	if pkg.hasRealScript("build") {
		gates["build"] = manifest.Gate{
			Required: boolPtr(true),
			Cmd:      strPtr("npm run build"),
			Notes:    "Detectado script build en package.json.",
		}
	} else {
		gates["build"] = gateAusente("No se detecto script build en package.json.")
	}

	return notas
}

func adaptarPython(dir string, gates map[string]manifest.Gate) []string {
	hasPyProject := fileExists(dir, "pyproject.toml")
	hasReqs := fileExists(dir, "requirements.txt")
	hasSetup := fileExists(dir, "setup.py")
	if !hasPyProject && !hasReqs && !hasSetup {
		return nil
	}
	var notas []string
	hasTool := func(name string) bool {
		return fileContainsAny(dir, "pyproject.toml", name) ||
			fileContainsAny(dir, "requirements.txt", name) ||
			fileContainsAny(dir, "requirements-dev.txt", name)
	}

	switch {
	case hasTool("mypy") || fileExists(dir, "mypy.ini"):
		gates["static"] = manifest.Gate{
			Required: boolPtr(false),
			Cmd:      strPtr("mypy ."),
			Notes:    "Detectado mypy en el proyecto.",
		}
	case hasTool("pyright") || fileExists(dir, "pyrightconfig.json"):
		gates["static"] = manifest.Gate{
			Required: boolPtr(false),
			Cmd:      strPtr("pyright"),
			Notes:    "Detectado pyright en el proyecto.",
		}
	default:
		gates["static"] = gateAusente("No se detecto mypy ni pyright en el proyecto.")
		notas = append(notas, "gate static: AUSENTE (sin mypy ni pyright detectados)")
	}

	if hasTool("ruff") || fileExists(dir, "ruff.toml") || fileExists(dir, ".ruff.toml") {
		gates["lint"] = manifest.Gate{
			Required: boolPtr(false),
			Cmd:      strPtr("ruff check ."),
			Notes:    "Detectado ruff en el proyecto.",
		}
	} else {
		gates["lint"] = gateAusente("No se detecto ruff en el proyecto.")
		notas = append(notas, "gate lint: AUSENTE (ruff no detectado)")
	}

	return notas
}

func adaptarGo(dir string, gates map[string]manifest.Gate) []string {
	if !fileExists(dir, "go.mod") {
		return nil
	}
	var notas []string

	if fileExists(dir, ".golangci.yml") || fileExists(dir, ".golangci.yaml") {
		gates["lint"] = manifest.Gate{
			Required: boolPtr(true),
			Cmd:      strPtr("golangci-lint run ./..."),
			Notes:    "Detectado .golangci.yml en el proyecto.",
		}
	} else if _, err := exec.LookPath("golangci-lint"); err == nil {
		gates["lint"] = manifest.Gate{
			Required: boolPtr(false),
			Cmd:      strPtr("golangci-lint run ./..."),
			Notes:    "Detectado golangci-lint en PATH.",
		}
	}

	if fileExists(dir, "arch-go.yml") {
		gates["arch"] = manifest.Gate{
			Required: boolPtr(true),
			Cmd:      strPtr("arch-go"),
			Notes:    "Detectado arch-go.yml en el proyecto.",
		}
	}

	if _, err := exec.LookPath("gremlins"); err != nil {
		gates["mutation"] = gateAusente("No se detecto gremlins en PATH; AUSENTE en ambar hasta instalarlo.")
		notas = append(notas, "gate mutation: AUSENTE (gremlins no esta en PATH)")
	}

	return notas
}

func adaptarKMP(dir string, gates map[string]manifest.Gate) []string {
	if !fileExists(dir, "settings.gradle.kts") && !fileExists(dir, "build.gradle.kts") {
		return nil
	}
	var notas []string
	hasPlugin := func(name string) bool {
		return fileContainsAny(dir, "build.gradle.kts", name) ||
			fileContainsAny(dir, "settings.gradle.kts", name) ||
			fileContainsAny(dir, "gradle/libs.versions.toml", name)
	}
	if !hasPlugin("detekt") && !fileExists(dir, "config/detekt/detekt.yml") {
		gates["static"] = gateAusente("No se detecto detekt en Gradle; AUSENTE en ambar.")
		notas = append(notas, "gate static: AUSENTE (detekt no detectado en Gradle)")
	}
	if !hasPlugin("konsist") {
		gates["arch"] = gateAusente("No se detecto Konsist en Gradle; AUSENTE en ambar.")
		notas = append(notas, "gate arch: AUSENTE (Konsist no detectado en Gradle)")
	}
	if !hasPlugin("pitest") {
		gates["mutation"] = gateAusente("No se detecto Pitest en Gradle; AUSENTE en ambar.")
		notas = append(notas, "gate mutation: AUSENTE (Pitest no detectado en Gradle)")
	}
	return notas
}

// AuditarGates inspects each configured gate in m and reports warnings when a
// gate command references a project-local script or binary (e.g.
// `sh scripts/mutacion.sh`, `vendor/bin/pest`, `./gradlew`) that does not exist
// in dir. It never modifies files and never executes gate commands.
func AuditarGates(dir string, m *manifest.Manifest) []string {
	if m == nil {
		return nil
	}
	var avisos []string
	names := m.SortedGateNames()
	sort.Strings(names)
	for _, name := range names {
		g := m.Gates[name]
		cmd := strings.TrimSpace(g.CmdStr())
		if cmd == "" {
			continue
		}
		ref := extraerRutaLocal(cmd)
		if ref == "" {
			continue
		}
		if !fileExists(dir, ref) {
			avisos = append(avisos, fmt.Sprintf(
				"gate %q apunta a %q, que no existe en este proyecto (usa cmd: \"\" para declararlo AUSENTE en ambar o crea el archivo)",
				name, ref,
			))
		}
	}
	return avisos
}

// extraerRutaLocal returns the local file path referenced by a gate command
// when it clearly points into the repository (e.g. `sh scripts/foo.sh`,
// `bash scripts/foo.sh`, `./gradlew`, `vendor/bin/pest`, `node_modules/.bin/vitest`).
func extraerRutaLocal(cmd string) string {
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return ""
	}
	tok := fields[0]
	if (tok == "sh" || tok == "bash" || tok == "php" || tok == "node" || tok == "python" || tok == "python3") && len(fields) >= 2 {
		next := fields[1]
		if !strings.HasPrefix(next, "-") && strings.Contains(next, "/") {
			return strings.TrimPrefix(next, "./")
		}
		return ""
	}
	if strings.HasPrefix(tok, "./") || strings.HasPrefix(tok, "vendor/") ||
		strings.HasPrefix(tok, "scripts/") || strings.HasPrefix(tok, "node_modules/") ||
		strings.HasPrefix(tok, ".hoom/") {
		return strings.TrimPrefix(tok, "./")
	}
	return ""
}
