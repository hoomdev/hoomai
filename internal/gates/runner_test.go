// Tests adversariales del spec .hoom/specs/gates-ausentes-parciales-verifica.md
// (CA-61): un gate con cmd vacio es AUSENTE aunque exista diff_cmd heredado
// y diff activo — la promesa "cmd vacio declara el gate AUSENTE" se cumple.
package gates

import (
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/verdict"
)

func strPtr(s string) *string { return &s }

// CA-61: cmd "" + diff_cmd del perfil + diff activo => absent, jamas se
// ejecuta el comando heredado (el caso reportado: infection exit 127 rojo).
func TestCA61_CmdVacioConDiffCmdHeredadoEsAusente(t *testing.T) {
	g := manifest.Gate{
		Cmd:     strPtr(""),
		DiffCmd: strPtr("exit 127"), // si se ejecutara, el gate seria FAIL
	}
	git := gitx.Info{IsRepo: true, Base: "main", ChangedFiles: []string{"app/Foo.php"}}

	cmdStr, scope := selectCommand(g, git, Options{})
	if cmdStr != "" || scope != "none" {
		t.Fatalf("CA-61: sin cmd no hay gate; diff_cmd no debe resucitarlo: cmd=%q scope=%q", cmdStr, scope)
	}

	m := &manifest.Manifest{Dir: t.TempDir(), Gates: map[string]manifest.Gate{"mutation": g}}
	res := runOne("mutation", g, m, git, Options{Timeout: time.Minute, TailSize: 100})
	if res.Status != verdict.StatusAbsent {
		t.Fatalf("CA-61: esperaba absent (amarillo), no %q (exit %d)", res.Status, res.ExitCode)
	}
}

// CA-61 (control): un gate con cmd y diff_cmd validos sigue eligiendo diff
// cuando hay diff activo — la regla nueva no rompe el scoping normal.
func TestCA61_GateNormalSigueUsandoDiff(t *testing.T) {
	g := manifest.Gate{
		Cmd:     strPtr("echo full"),
		DiffCmd: strPtr("echo diff {files}"),
	}
	git := gitx.Info{IsRepo: true, Base: "main", ChangedFiles: []string{"a.go"}}
	cmdStr, scope := selectCommand(g, git, Options{})
	if scope != "diff" || cmdStr != "echo diff a.go" {
		t.Fatalf("CA-61: scoping por diff roto: cmd=%q scope=%q", cmdStr, scope)
	}
}
