// Package initcmd scaffolds a project: detects the stack, writes hoom.yaml
// with the profile's gates materialized as editable entries, and prepares
// the .hoom directory (verdicts travel in Git; local caches do not).
package initcmd

import (
	"fmt"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"os"
	"path/filepath"
	"strings"

	"github.com/hoomdev/hoomai/internal/entorno"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/profiles"
)

// Run initializes dir. If profileName is empty the stack is auto-detected.
//
// verify-en-el-entorno: a new .hoom/.gitignore is hoomfs.GitignoreBody
// (local.yaml included); one that already exists is completed with the line
// local.yaml when it lacks it (hoomfs.EnsureIgnoredFiles), and nothing else
// in it changes. When the project lives in a subdirectory of its repository
// (entorno.Principal of dir is another folder), the main checkout's
// .hoom/.gitignore is completed the same way, since .hoom/local.yaml is read
// and validated there; when it does not exist it is created with that one
// line, never GitignoreBody (the main checkout may not be a hoom project).
// At the root of its repository, or outside git, nothing outside dir is
// written. A failure to write either .hoom/.gitignore is Run's error (by
// then hoom.yaml is already written, and the error says so). At its end it
// prints, for each entorno of the profile that holds in dir
// (profiles.Sugerencias), its Sugerencia; it never writes .hoom/local.yaml.
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
	fmt.Fprintf(&b, "# Un cmd vacio declara el gate AUSENTE (aunque el perfil traiga diff_cmd):\n")
	fmt.Fprintf(&b, "# se reporta en amarillo, nunca se oculta. Omitir un gate hereda el del perfil.\n")
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
		if g.DiffCmdStr() != "" {
			fmt.Fprintf(&b, "    diff_cmd: %q\n", g.DiffCmdStr())
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
	var completado []string
	if _, err := os.Lstat(gi); os.IsNotExist(err) {
		if err := os.WriteFile(gi, []byte(hoomfs.GitignoreBody()), 0o644); err != nil {
			return fmt.Errorf("%s quedo escrito, pero no se pudo crear .hoom/.gitignore: %w", manifest.FileName, err)
		}
	} else {
		// .hoom/local.yaml is this machine's: git never takes it
		// (verify-en-el-entorno, CA-770); nothing else in the file changes
		completado, err = hoomfs.EnsureIgnoredFiles(abs, hoomfs.LocalFiles...)
		if err != nil {
			return fmt.Errorf("%s quedo escrito, pero no se pudo agregar local.yaml a .hoom/.gitignore: %w", manifest.FileName, err)
		}
	}
	// in a subdirectory of the repository, .hoom/local.yaml is read at the
	// main checkout: its .hoom/.gitignore has to ignore it too (CA-770)
	var enPrincipal []string
	principal := entorno.Principal(abs)
	if !mismaCarpeta(principal, abs) {
		enPrincipal, err = hoomfs.EnsureIgnoredFiles(principal, hoomfs.LocalFiles...)
		if err != nil {
			return fmt.Errorf("%s quedo escrito, pero no se pudo agregar local.yaml al .hoom/.gitignore del checkout principal (%s): %w", manifest.FileName, principal, err)
		}
	}
	fmt.Printf("hoom: creado %s\n", target)
	fmt.Printf("hoom: creado .hoom/verdicts/ (los veredictos viajan en Git; .hoom/cache no)\n")
	fmt.Printf("hoom: creado .hoom/intake/ (documentos del cliente: SRS, planes, minutas)\n")
	fmt.Printf("hoom: creado .hoom/specs/ (vision del producto y specs por tarea)\n")
	if len(completado) > 0 {
		fmt.Printf("hoom: agregado %s a .hoom/.gitignore (es de esta maquina)\n", strings.Join(completado, ", "))
	}
	if len(enPrincipal) > 0 {
		fmt.Printf("hoom: agregado %s al .hoom/.gitignore del checkout principal, %s (es de esta maquina y se lee ahi)\n", strings.Join(enPrincipal, ", "), principal)
	}
	fmt.Printf("hoom: siguiente paso -> revisa los cmd del manifiesto y ejecuta 'hoom verify'\n")
	// where the gates could run instead of the host: only a suggestion,
	// the person writes .hoom/local.yaml (CA-769)
	for _, e := range profiles.Sugerencias(profileName, abs) {
		fmt.Println(e.Sugerencia())
	}
	return nil
}

// mismaCarpeta says whether a and b are the same folder on disk, symlinks
// and spellings aside: git gives the main checkout with its symlinks
// resolved, and dir may come through one.
func mismaCarpeta(a, b string) bool {
	if a == b {
		return true
	}
	fa, errA := os.Stat(a)
	fb, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(fa, fb)
}
