//go:build !windows

// Tests adversariales del spec .hoom/specs/evidencia-de-la-review.md, ronda
// 3, que necesitan un git falso al frente del PATH:
//
//   - d00a72 (CA-484, CA-486): el piso para separar la base es git 2.40. Un
//     git 2.40 separa: `--delta` revisa como delta con base_integrada y sin
//     el aviso de "no se separa". El git falso se anuncia 2.40.0 y, como un
//     2.40 de verdad, no conoce la opcion global --attr-source (llego en
//     2.41): la rechaza como git, con exit 129. Para todo lo demas es el git
//     de verdad. (Un falso que solo cambie el anuncio y delegue
//     --attr-source a un git 2.41+ no reproduce un 2.40: el comando andaria.)
package reviewcmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// edrGit240 pone en bin un git que se anuncia 2.40.0, rechaza --attr-source
// como lo rechaza ese git y para todo lo demas es el git de verdad (real).
func edrGit240(t *testing.T, bin, real string) {
	t.Helper()
	s := "#!/bin/sh\n" + edrSaltarOpciones(
		"\t--version|version) echo 'git version 2.40.0'; exit 0 ;;\n"+
			"\t--attr-source|--attr-source=*) echo \"unknown option: $a\" >&2; echo 'usage: git [-v | --version] [-h | --help] [-C <path>] [-c <name>=<value>]' >&2; exit 129 ;;\n") +
		"exec '" + real + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("git", "version").Output()
	if err != nil || strings.TrimSpace(string(out)) != "git version 2.40.0" {
		t.Fatalf("fixture: el git del PATH se anuncia 2.40.0: %q %v", out, err)
	}
	if err := exec.Command("git", "--attr-source=HEAD", "version").Run(); err == nil {
		t.Fatal("fixture: el git 2.40 del PATH no conoce --attr-source")
	}
}

// Hallazgo d00a72 (CA-484, CA-486): con un git 2.40.0, en el caso de CA-484
// (X, un merge limpio de la base, Z), `hoom review --delta` y `--desde D`
// separan la base: delta (o el --desde de hoy), base_integrada = I, la
// evidencia sin lo que trajo la base, y sin el aviso de "no se separa" (ni
// el de git viejo: 2.40 es el piso) ni un error de git.
func TestHallazgod00a72_CA484_CA486_ConGit240LaBaseSeSepara(t *testing.T) {
	for _, delta := range []bool{true, false} {
		caso := "--desde"
		if delta {
			caso = "--delta"
		}
		t.Run(caso, func(t *testing.T) {
			real := raGitReal(t)
			bin := raPATH(t)
			cx := raInstalar(t, bin, "codex", "")
			root, r1, d, i, _ := edrConBaseIntegrada(t, "d00a72", cx)
			edrGit240(t, bin, real)
			opt := Options{Provider: "codex", Spec: edrSpec, Delta: delta}
			if !delta {
				opt.Desde = d
			}
			antes := cx.veces()
			edrLimpio(t, "d00a72", root)
			res, err, out := rdCorrer(t, "d00a72", root, opt)
			if err != nil || res.Status != "revisado" || cx.veces() != antes+1 {
				t.Fatalf("d00a72/CA-484: %s con git 2.40.0 revisa (2.40 es el piso: separa, no falla): %+v %v\n%s", caso, res, err, out)
			}
			if strings.Contains(out, "no se separa") || bdTieneNota(res.Notes, "no se separa") {
				t.Fatalf("d00a72/CA-486: %s con git 2.40.0 no es la review completa del aviso:\n%s", caso, out)
			}
			if m := edrJSON(t, res); m["base_integrada"] != i {
				t.Fatalf("d00a72/CA-484: %s con git 2.40.0 separa la base: base_integrada %s: %v", caso, i[:12], m["base_integrada"])
			}
			if delta && (res.Cobertura != CoberturaDelta || res.DesdeReview != r1.RecordID) {
				t.Fatalf("d00a72/CA-484: --delta con git 2.40.0 es un delta de %s: %+v", r1.RecordID, res)
			}
			diff, _ := edrDiffDelPedido(t, "d00a72", cx.pedido(t, antes+1))
			edrSoloLoNuevo(t, "d00a72/CA-484", caso, diff)
		})
	}
}
