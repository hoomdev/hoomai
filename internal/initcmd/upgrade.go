package initcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

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
}

// UpgradeResult summarizes what `hoom upgrade` added or reported.
type UpgradeResult struct {
	Target     string
	Added      []string
	Warnings   []string
	Gitignored []string
}

// Upgrade updates an existing hoom.yaml in place by appending only missing
// default sections (findings, agents, review, spec.topes) calibrated for
// token and time savings, without ever modifying existing gates, rutas,
// comments or user-defined sections. It also audits existing gate commands
// for missing project-local scripts/binaries and ensures .hoom/.gitignore
// ignores machine-local files.
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

	res := UpgradeResult{Target: target}
	var addBuf strings.Builder
	for _, s := range secciones {
		if present[s.key] {
			continue
		}
		res.Added = append(res.Added, s.key)
		addBuf.WriteString(s.text)
	}

	if len(res.Added) > 0 {
		updated := string(raw)
		if updated != "" && !strings.HasSuffix(updated, "\n") {
			updated += "\n"
		}
		updated += addBuf.String()

		// Validate the upgraded YAML in a temp dir before touching disk.
		tmpDir, err := os.MkdirTemp("", "hoom-upgrade-*")
		if err != nil {
			return UpgradeResult{}, err
		}
		defer os.RemoveAll(tmpDir)
		if err := os.WriteFile(filepath.Join(tmpDir, manifest.FileName), []byte(updated), 0o600); err != nil {
			return UpgradeResult{}, err
		}
		if _, err := manifest.Load(tmpDir, profiles.Resolve); err != nil {
			return UpgradeResult{}, fmt.Errorf("upgrade produjo un hoom.yaml invalido: %w", err)
		}

		if !opt.DryRun {
			if err := os.WriteFile(target, []byte(updated), info.Mode().Perm()); err != nil {
				return UpgradeResult{}, err
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

	res.Warnings = AuditarGates(abs, m)
	return res, nil
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
