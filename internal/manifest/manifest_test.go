// Tests adversariales del spec .hoom/specs/gates-ausentes-parciales-verifica.md
// (CA-62): la herencia perfil -> proyecto distingue "no especificado" de
// "explicitamente vacio" tambien para diff_cmd.
package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func strPtr(s string) *string { return &s }

func laravelLikeResolver(name string) (map[string]Gate, string, error) {
	req := true
	return map[string]Gate{
		"mutation": {
			Required: &req,
			Cmd:      strPtr("vendor/bin/infection"),
			DiffCmd:  strPtr("vendor/bin/infection --git-diff-base={base}"),
		},
	}, "perfil de prueba", nil
}

func loadWith(t *testing.T, gatesYAML string) *Manifest {
	t.Helper()
	dir := t.TempDir()
	body := "schema: hoom/v1\nproject: demo\nprofile: fake\ngates:\n" + gatesYAML
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Load(dir, laravelLikeResolver)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// CA-62: diff_cmd omitido hereda el del perfil intacto.
func TestCA62_DiffCmdOmitidoHereda(t *testing.T) {
	m := loadWith(t, "  mutation:\n    cmd: \"otro\"\n")
	g := m.Gates["mutation"]
	if g.CmdStr() != "otro" {
		t.Fatalf("CA-62: cmd del proyecto debe pisar al perfil: %q", g.CmdStr())
	}
	if g.DiffCmdStr() != "vendor/bin/infection --git-diff-base={base}" {
		t.Fatalf("CA-62: diff_cmd omitido debe heredar el del perfil: %q", g.DiffCmdStr())
	}
}

// CA-62: diff_cmd: "" explicito BORRA el diff_cmd del perfil (campo puntero).
func TestCA62_DiffCmdVacioBorra(t *testing.T) {
	m := loadWith(t, "  mutation:\n    diff_cmd: \"\"\n")
	g := m.Gates["mutation"]
	if g.DiffCmdStr() != "" {
		t.Fatalf("CA-62: diff_cmd \"\" explicito debe borrar el del perfil: %q", g.DiffCmdStr())
	}
	if g.CmdStr() != "vendor/bin/infection" {
		t.Fatalf("CA-62: cmd omitido debe heredar el del perfil: %q", g.CmdStr())
	}
}

// CA-61 (parte manifiesto): cmd: "" explicito queda vacio aunque el perfil
// aporte diff_cmd; el runner decide ausencia sobre CmdStr() (ver gates).
func TestCA61_CmdVacioExplicitoSobreviveAlMerge(t *testing.T) {
	m := loadWith(t, "  mutation:\n    cmd: \"\"\n")
	g := m.Gates["mutation"]
	if g.Cmd == nil || g.CmdStr() != "" {
		t.Fatalf("CA-61: cmd \"\" debe quedar explicito y vacio tras el merge: %+v", g)
	}
}
