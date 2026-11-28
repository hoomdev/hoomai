package initcmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/entorno"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/profiles"
)

const (
	bloqueFindings = `findings:
  block_on: high
  max_rondas: 1
`
	bloqueAgents = `# Modelo por rol en niveles (fuerte, medio, barato): hoom los traduce al
# modelo de cada provider; un nombre de modelo tambien vale. Cada rol puede
# cambiar sus valores segun el riesgo de la tarjeta (bajo, normal, alto).
agents:
  arquitecto: { model: fuerte }
  designer: { model: medio }
  scout: { model: barato }
  writer: { model: medio, alto: { model: fuerte } }
  test-writer: { model: medio, alto: { model: fuerte } }
  characterizer: { model: medio, alto: { model: fuerte } }
  analista: { model: medio }
  refutador: { model: medio, alto: { model: fuerte } }
`
	bloqueReview = `review:
  model: medio
  effort: high
  concurrencia: 4
`
	bloqueSpec = `# Topes del spec segun el riesgo declarado de la tarjeta (0 = sin tope).
spec:
  topes:
    bajo: { palabras: 400, criterios: 4, citas_por_criterio: 10 }
    normal: { palabras: 800, criterios: 8, citas_por_criterio: 10 }
    alto: { palabras: 2000, criterios: 15, citas_por_criterio: 0 }
`
)

// UpgradeOptions configures `hoom upgrade`.
type UpgradeOptions struct {
	DryRun bool
	// Defaults applies the latest calibrated token- and time-saving defaults
	// to an existing project: updates findings, agents, review and spec.topes,
	// optimizes gates (--threads={threads}, aplica_en, safe diff_cmd) without
	// overwriting custom comandos/rutas/aplica_en, and refreshes .hoom/agents/.
	Defaults bool
}

// UpgradeResult summarizes what `hoom upgrade` added, updated or reported.
type UpgradeResult struct {
	Target        string
	Added         []string
	Updated       []string
	AgentsUpdated bool
	Warnings      []string
	Gitignored    []string
}

// Upgrade updates an existing hoom.yaml in place. By default it appends only
// missing sections (findings, agents, review, spec.topes) without touching
// existing gates, rutas, comments or user-defined sections. With Defaults=true
// (`hoom upgrade --defaults`), it also migrates existing sections and gates to
// the latest fast/token-saving defaults (preserving custom rutas, comandos,
// agents.<rol>.write, findings.max_pendientes and custom aplica_en/diff_cmd)
// and refreshes .hoom/agents/ so the optimization travels in Git to every
// machine with zero local configuration.
func Upgrade(dir string, opt UpgradeOptions) (UpgradeResult, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return UpgradeResult{}, err
	}
	target := filepath.Join(abs, manifest.FileName)
	info, err := os.Stat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return UpgradeResult{}, fmt.Errorf("%s no existe; ejecuta 'hoom init' para inicializar el proyecto", target)
		}
		return UpgradeResult{}, err
	}

	m, err := manifest.Load(abs, profiles.Resolve)
	if err != nil {
		return UpgradeResult{}, err
	}

	raw, err := os.ReadFile(target)
	if err != nil {
		return UpgradeResult{}, err
	}

	res := UpgradeResult{Target: target}
	var updated string
	changed := false

	if opt.Defaults {
		var added, upd []string
		updated, added, upd, err = aplicarDefaults(abs, m.Profile, raw)
		if err != nil {
			return UpgradeResult{}, err
		}
		res.Added = added
		res.Updated = upd
		changed = len(added) > 0 || len(upd) > 0
	} else {
		present, err := topLevelKeys(raw)
		if err != nil {
			return UpgradeResult{}, fmt.Errorf("hoom.yaml invalido: %w", err)
		}
		secciones := []struct {
			key  string
			text string
		}{
			{"findings", bloqueFindings},
			{"agents", bloqueAgents},
			{"review", bloqueReview},
			{"spec", bloqueSpec},
		}
		var addBuf strings.Builder
		for _, s := range secciones {
			if present[s.key] {
				continue
			}
			res.Added = append(res.Added, s.key)
			addBuf.WriteString(s.text)
		}
		if len(res.Added) > 0 {
			updated = string(raw)
			if updated != "" && !strings.HasSuffix(updated, "\n") {
				updated += "\n"
			}
			updated += addBuf.String()
			changed = true
		}
	}

	mAudit := m
	if changed {
		// Validate the upgraded YAML in a temp dir before touching disk.
		tmpDir, err := os.MkdirTemp("", "hoom-upgrade-*")
		if err != nil {
			return UpgradeResult{}, err
		}
		defer os.RemoveAll(tmpDir)
		if err := os.WriteFile(filepath.Join(tmpDir, manifest.FileName), []byte(updated), 0o600); err != nil {
			return UpgradeResult{}, err
		}
		loaded, err := manifest.Load(tmpDir, profiles.Resolve)
		if err != nil {
			return UpgradeResult{}, fmt.Errorf("upgrade produjo un hoom.yaml invalido: %w", err)
		}
		mAudit = loaded

		if !opt.DryRun {
			if err := os.WriteFile(target, []byte(updated), info.Mode().Perm()); err != nil {
				return UpgradeResult{}, err
			}
		}
	}

	if opt.Defaults {
		res.AgentsUpdated = true
		if !opt.DryRun {
			if err := agents.Install(abs); err != nil {
				return res, fmt.Errorf("no se pudieron actualizar los contratos en .hoom/agents/: %w", err)
			}
		}
	}

	if !opt.DryRun {
		gi := filepath.Join(abs, ".hoom", ".gitignore")
		if _, err := os.Lstat(gi); err == nil {
			added, err := hoomfs.EnsureIgnoredFiles(abs, hoomfs.LocalFiles...)
			if err != nil {
				return res, fmt.Errorf("no se pudo actualizar .hoom/.gitignore: %w", err)
			}
			res.Gitignored = append(res.Gitignored, added...)
		}
		principal := entorno.Principal(abs)
		if !mismaCarpeta(principal, abs) {
			if _, err := hoomfs.EnsureIgnoredFiles(principal, hoomfs.LocalFiles...); err != nil {
				return res, fmt.Errorf("no se pudo actualizar .hoom/.gitignore en %s: %w", principal, err)
			}
		}
	}

	res.Warnings = AuditarGates(abs, mAudit)
	return res, nil
}

func aplicarDefaults(dir, profile string, raw []byte) (string, []string, []string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return "", nil, nil, fmt.Errorf("hoom.yaml invalido: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return "", nil, nil, fmt.Errorf("hoom.yaml invalido: la raiz debe ser un mapa YAML")
	}
	root := doc.Content[0]

	var added, updated []string

	// 1. Optimizar gates existentes sin destruir comandos custom ni aplica_en/diff_cmd propios.
	if gatesNode := findMapValue(root, "gates"); gatesNode != nil && gatesNode.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(gatesNode.Content); i += 2 {
			gateName := gatesNode.Content[i].Value
			gateMap := gatesNode.Content[i+1]
			if gateMap.Kind != yaml.MappingNode {
				continue
			}
			cmdNode := findMapValue(gateMap, "cmd")
			cmdVal := ""
			if cmdNode != nil && cmdNode.Kind == yaml.ScalarNode {
				cmdVal = strings.TrimSpace(cmdNode.Value)
			}
			if cmdVal == "" {
				continue // gate declarado ausente: respetar intacto
			}

			// --threads=max -> --threads={threads}
			threadsChanged := false
			if strings.Contains(cmdNode.Value, "--threads=max") {
				cmdNode.Value = strings.ReplaceAll(cmdNode.Value, "--threads=max", "--threads={threads}")
				cmdVal = strings.TrimSpace(cmdNode.Value)
				threadsChanged = true
			}
			diffNode := findMapValue(gateMap, "diff_cmd")
			if diffNode != nil && diffNode.Kind == yaml.ScalarNode && strings.Contains(diffNode.Value, "--threads=max") {
				diffNode.Value = strings.ReplaceAll(diffNode.Value, "--threads=max", "--threads={threads}")
				threadsChanged = true
			}
			if threadsChanged {
				updated = append(updated, fmt.Sprintf("gates.%s (threads)", gateName))
			}

			// diff_cmd seguro cuando falta o es identico a cmd
			diffVal := ""
			if diffNode != nil && diffNode.Kind == yaml.ScalarNode {
				diffVal = strings.TrimSpace(diffNode.Value)
			}
			if diffVal == "" || diffVal == cmdVal {
				if sugerido := defaultDiffCmdParaGate(dir, profile, gateName, cmdVal); sugerido != "" && sugerido != diffVal {
					setMapScalar(gateMap, "diff_cmd", sugerido)
					updated = append(updated, fmt.Sprintf("gates.%s.diff_cmd", gateName))
				}
			}

			// aplica_en por stack solo si el gate no declara ninguno todavia
			if findMapValue(gateMap, "aplica_en") == nil {
				if patrones := defaultAplicaEnParaGate(profile, gateName); len(patrones) > 0 {
					setMapStringSlice(gateMap, "aplica_en", patrones)
					updated = append(updated, fmt.Sprintf("gates.%s.aplica_en", gateName))
				}
			}
		}
	}

	// 2. Actualizar o agregar secciones de ahorro de tokens y tiempos.
	secciones := []struct {
		key  string
		text string
	}{
		{"findings", bloqueFindings},
		{"agents", bloqueAgents},
		{"review", bloqueReview},
		{"spec", bloqueSpec},
	}
	for _, s := range secciones {
		wantVal, err := parseSectionValue(s.text, s.key)
		if err != nil {
			return "", nil, nil, err
		}
		existing := findMapValue(root, s.key)
		if existing == nil {
			root.Content = append(root.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s.key},
				wantVal,
			)
			added = append(added, s.key)
			continue
		}
		// Preservar ajustes locales de politica/territorio dentro de findings y agents.
		switch s.key {
		case "findings":
			if mp := findMapValue(existing, "max_pendientes"); mp != nil {
				setMapNode(wantVal, "max_pendientes", cloneNode(mp))
			}
		case "agents":
			preservarWritesDeAgentes(existing, wantVal)
		}
		if !yamlNodesEqual(existing, wantVal) {
			replaceMapValue(root, s.key, wantVal)
			updated = append(updated, s.key)
		}
	}

	if len(added) == 0 && len(updated) == 0 {
		return string(raw), nil, nil, nil
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return "", nil, nil, fmt.Errorf("no se pudo serializar hoom.yaml: %w", err)
	}
	if err := enc.Close(); err != nil {
		return "", nil, nil, fmt.Errorf("no se pudo cerrar el serializador YAML: %w", err)
	}
	return buf.String(), added, updated, nil
}

func defaultAplicaEnParaGate(profile, gate string) []string {
	switch profile {
	case "laravel":
		switch gate {
		case "static", "lint", "arch":
			return []string{"app/**", "routes/**", "config/**", "database/**", "tests/**", "composer.json", "composer.lock"}
		case "mutation":
			return []string{"app/**", "routes/**", "config/**", "database/**", "tests/**"}
		case "test_vue", "test_js":
			return []string{"resources/**", "package.json", "package-lock.json", "vite.config.*"}
		}
	case "go":
		switch gate {
		case "test", "static", "lint", "arch", "mutation", "build":
			return []string{"**/*.go", "go.mod", "go.sum"}
		}
	case "node":
		switch gate {
		case "test", "static", "lint", "arch", "mutation", "build":
			return []string{"src/**", "app/**", "tests/**", "package.json", "package-lock.json", "tsconfig*.json", "vite.config.*"}
		}
	case "python":
		switch gate {
		case "test", "static", "lint", "arch", "mutation", "build":
			return []string{"**/*.py", "pyproject.toml", "requirements*.txt"}
		}
	}
	return nil
}

func defaultDiffCmdParaGate(dir, profile, gate, cmd string) string {
	switch gate {
	case "mutation":
		if strings.Contains(cmd, "infection") && !strings.Contains(cmd, "--git-diff-base") {
			out := cmd
			if !strings.Contains(out, "--threads") {
				out += " --threads={threads}"
			}
			return out + " --git-diff-filter=AM --git-diff-base={base}"
		}
		if strings.Contains(cmd, "scripts/mutacion.sh") && !strings.Contains(cmd, "--diff") &&
			fileContainsAny(dir, "scripts/mutacion.sh", "--diff") {
			return "sh scripts/mutacion.sh --diff {base}"
		}
	case "security":
		if strings.Contains(cmd, "semgrep scan") && !strings.Contains(cmd, "--baseline-commit") {
			return cmd + " --baseline-commit {base}"
		}
	case "test":
		if profile == "go" && strings.HasPrefix(cmd, "go test") && !strings.Contains(cmd, "{packages}") {
			return "go test -timeout 10m {packages}"
		}
	}
	return ""
}

func parseSectionValue(text, key string) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return nil, err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, fmt.Errorf("bloque por defecto %s vacio", key)
	}
	v := findMapValue(doc.Content[0], key)
	if v == nil {
		return nil, fmt.Errorf("bloque por defecto sin clave %s", key)
	}
	return v, nil
}

func findMapValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func replaceMapValue(m *yaml.Node, key string, val *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = val
			return
		}
	}
}

func setMapNode(m *yaml.Node, key string, val *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = val
			return
		}
	}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		val,
	)
}

func setMapScalar(m *yaml.Node, key, val string) {
	setMapNode(m, key, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Style: yaml.DoubleQuotedStyle, Value: val})
}

func setMapStringSlice(m *yaml.Node, key string, vals []string) {
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle}
	for _, s := range vals {
		seq.Content = append(seq.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Style: yaml.DoubleQuotedStyle, Value: s})
	}
	setMapNode(m, key, seq)
}

func preservarWritesDeAgentes(existing, want *yaml.Node) {
	if existing == nil || existing.Kind != yaml.MappingNode || want == nil || want.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(existing.Content); i += 2 {
		role := existing.Content[i].Value
		roleMap := existing.Content[i+1]
		if roleMap.Kind != yaml.MappingNode {
			continue
		}
		writeNode := findMapValue(roleMap, "write")
		if writeNode == nil {
			continue
		}
		dest := findMapValue(want, role)
		if dest == nil {
			dest = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			setMapNode(want, role, dest)
		}
		setMapNode(dest, "write", cloneNode(writeNode))
	}
}

func cloneNode(n *yaml.Node) *yaml.Node {
	if n == nil {
		return nil
	}
	cp := *n
	if len(n.Content) > 0 {
		cp.Content = make([]*yaml.Node, len(n.Content))
		for i, c := range n.Content {
			cp.Content[i] = cloneNode(c)
		}
	}
	return &cp
}

func yamlNodesEqual(a, b *yaml.Node) bool {
	var va, vb any
	if err := a.Decode(&va); err != nil {
		return false
	}
	if err := b.Decode(&vb); err != nil {
		return false
	}
	ba, err1 := yaml.Marshal(va)
	bb, err2 := yaml.Marshal(vb)
	return err1 == nil && err2 == nil && bytes.Equal(ba, bb)
}

func topLevelKeys(raw []byte) (map[string]bool, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	keys := map[string]bool{}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return keys, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("la raiz debe ser un mapa YAML")
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		keys[root.Content[i].Value] = true
	}
	return keys, nil
}
