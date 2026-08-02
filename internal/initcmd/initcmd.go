// Package initcmd scaffolds a project: detects the stack, writes hoom.yaml
// with the profile's gates materialized as editable entries, and prepares
// the .hoom directory (verdicts travel in Git; local caches do not).
package initcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/profiles"
)

// Run initializes dir. If profileName is empty the stack is auto-detected.
func Run(dir, projectName, profileName string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	target := filepath.Join(abs, manifest.FileName)
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("%s ya existe; edita el archivo o borralo para re-inicializar", target)
	}
	if profileName == "" {
		profileName, err = profiles.DetectStack(abs)
		if err != nil {
			return err
		}
		fmt.Printf("hoom: stack detectado -> perfil %q\n", profileName)
	}
	gates, desc, err := profiles.Resolve(profileName)
	if err != nil {
		return err
	}
	if projectName == "" {
		projectName = filepath.Base(abs)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# hoomAI - manifiesto de verificacion\n")
	fmt.Fprintf(&b, "# Perfil %s: %s\n", profileName, desc)
	fmt.Fprintf(&b, "# Los gates vienen del perfil; edita cmd/diff_cmd segun tu proyecto.\n")
	fmt.Fprintf(&b, "# Un cmd vacio declara el gate AUSENTE: se reporta en amarillo, nunca se oculta.\n")
	fmt.Fprintf(&b, "schema: %s\n", manifest.Schema)
	fmt.Fprintf(&b, "project: %s\n", projectName)
	fmt.Fprintf(&b, "profile: %s\n", profileName)
	fmt.Fprintf(&b, "base_branch: %s\n", manifest.DefaultBase)
	fmt.Fprintf(&b, "policy: %s\n", manifest.PolicyStrict)
	fmt.Fprintf(&b, "gates:\n")
	tmp := &manifest.Manifest{Gates: gates}
	for _, name := range tmp.SortedGateNames() {
		g := gates[name]
		fmt.Fprintf(&b, "  %s:\n", name)
		fmt.Fprintf(&b, "    required: %v\n", g.IsRequired())
		fmt.Fprintf(&b, "    cmd: %q\n", g.CmdStr())
		if g.DiffCmd != "" {
			fmt.Fprintf(&b, "    diff_cmd: %q\n", g.DiffCmd)
		}
		if g.Partial != "" {
			fmt.Fprintf(&b, "    partial: %q\n", g.Partial)
		}
		if g.Notes != "" {
			fmt.Fprintf(&b, "    # %s\n", g.Notes)
		}
	}
	if err := os.WriteFile(target, []byte(b.String()), 0o644); err != nil {
		return err
	}

	for _, d := range []string{"verdicts", "specs", "intake"} {
		if err := os.MkdirAll(filepath.Join(abs, ".hoom", d), 0o755); err != nil {
			return err
		}
	}
	gi := filepath.Join(abs, ".hoom", ".gitignore")
	if _, err := os.Stat(gi); os.IsNotExist(err) {
		_ = os.WriteFile(gi, []byte("cache/\n"), 0o644)
	}
	fmt.Printf("hoom: creado %s\n", target)
	fmt.Printf("hoom: creado .hoom/verdicts/ (los veredictos viajan en Git; .hoom/cache no)\n")
	fmt.Printf("hoom: creado .hoom/intake/ (documentos del cliente: SRS, planes, minutas)\n")
	fmt.Printf("hoom: creado .hoom/specs/ (vision del producto y specs por tarea)\n")
	fmt.Printf("hoom: siguiente paso -> revisa los cmd del manifiesto y ejecuta 'hoom verify'\n")
	return nil
}
