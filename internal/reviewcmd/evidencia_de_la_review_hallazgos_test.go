// Tests adversariales del spec .hoom/specs/evidencia-de-la-review.md, ronda
// 3: los hallazgos de la review de la correccion, escritos desde el spec.
//
//   - 686b61 (CA-481, CA-482): la entrada del estado tiene una forma. Un
//     directorio .hoom/ratchet.json/ o .hoom/.gitignore/, o un ARCHIVO
//     .hoom/intake o .hoom/cache, no son estado: commiteados van en la
//     evidencia de la review, y sin commitear la frenan.
//   - 3febb4 (CA-488): el contrato 06 dice exactamente que es el estado de
//     hoom: tambien .hoom/intake/, .hoom/ratchet.json, .hoom/.gitignore y los
//     directorios locales; sus dos copias son iguales.
//
// Los fixtures estan en evidencia_de_la_review_helpers_test.go.
package reviewcmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/hoomfs"
)

// ---------------------------------------------------------------- 686b61

// Hallazgo 686b61 (CA-481, CA-482): por cada forma ajena (un directorio
// .hoom/ratchet.json/, un archivo .hoom/intake, un directorio
// .hoom/.gitignore/ en lugar del archivo, un archivo .hoom/cache), commiteada
// en la rama: Evidence la trae, con su linea; editada sin commitear: Evidence
// y `hoom review` devuelven el error de arbol sucio con esa ruta, sin pasadas
// ni registro.
func TestHallazgo686b61_CA481_CA482_LasFormasAjenasDelEstadoEntranYFrenan(t *testing.T) {
	for _, forma := range []string{".hoom/ratchet.json/x", ".hoom/intake", ".hoom/.gitignore/x", ".hoom/cache"} {
		t.Run(strings.ReplaceAll(forma, "/", "_"), func(t *testing.T) {
			bin := raPATH(t)
			root := raRepo(t, "")
			raRama(t, root)
			if strings.HasPrefix(forma, ".hoom/.gitignore/") {
				// el archivo de la base deja lugar al directorio
				git(t, root, "rm", "-q", "--", ".hoom/.gitignore")
			}
			write(t, root, forma, "MARCA-FORMA "+forma+"\n")
			write(t, root, "app.go", "package app\n\nfunc ConLaForma() {}\n")
			git(t, root, "add", "-f", "--", forma, "app.go")
			git(t, root, "commit", "-q", "-m", "la rama commitea "+forma)
			edrLimpio(t, "686b61", root)

			ev, err := Evidence(root, "main", "", raTopeGrande)
			if err != nil || ev.Over {
				t.Fatalf("686b61/CA-481: Evidence arma la evidencia con %s commiteado: %v (Over %v)", forma, err, ev.Over)
			}
			sec := raSeccion(string(ev.Diff), "diff --git a/"+forma+" b/"+forma)
			if !strings.Contains(sec, "\n+MARCA-FORMA "+forma+"\n") {
				t.Fatalf("686b61/CA-481: %s no es estado de hoom: la evidencia trae su cambio commiteado:\n%s", forma, ev.Diff)
			}

			write(t, root, forma, "MARCA-FORMA-SIN-COMMITEAR\n")
			ev, err, _ = raEvidenceConReloj(t, "686b61", 20*time.Second, root, "main", "", raTopeGrande)
			if err == nil || err.Error() != raErrSucio(forma) || len(ev.Diff) != 0 {
				t.Fatalf("686b61/CA-482: con %s editado sin commitear Evidence devuelve %q: %v", forma, raErrSucio(forma), err)
			}
			cx := raInstalar(t, bin, "codex", "")
			res, rerr, out := raRunConReloj(t, "686b61", 30*time.Second, root, Options{Provider: "codex", Lens: "risk"})
			if msg := rdNegada(t, "686b61/CA-482", forma, root, cx, 0, 0, res, rerr, out, raErrSucio(forma)); strings.Contains(msg, "MARCA-FORMA-SIN-COMMITEAR") {
				t.Fatalf("686b61/CA-482: lo sin commitear no aparece en la negativa:\n%s", msg)
			}
		})
	}
}

// ---------------------------------------------------------------- 3febb4

// edrRaizDelModulo es el directorio del go.mod, hacia arriba del paquete.
func edrRaizDelModulo(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		padre := filepath.Dir(dir)
		if padre == dir {
			t.Skip("los fuentes no estan en el directorio de trabajo: no hay go.mod hacia arriba")
		}
		dir = padre
	}
}

// Hallazgo 3febb4 (CA-488): el contrato 06 enumera el estado de hoom
// entero, como la tabla del spec: ademas de veredictos, hallazgos,
// registros, items, aprobaciones y specs, nombra .hoom/intake/,
// .hoom/ratchet.json, .hoom/.gitignore y cada directorio local
// (hoomfs.Local). Las dos copias (la embebida en el binario y la del
// proyecto) son iguales, byte a byte, y la embebida es la que da
// agents.Contract sin contrato en el proyecto.
func TestHallazgo3febb4_CA488_ElContrato06NombraTodoElEstadoDeHoom(t *testing.T) {
	raiz := edrRaizDelModulo(t)
	embebida, err := os.ReadFile(filepath.Join(raiz, "internal", "agents", "assets", "agents", "06-reviewer.md"))
	if err != nil {
		t.Fatal(err)
	}
	proyecto, err := os.ReadFile(filepath.Join(raiz, ".hoom", "agents", "06-reviewer.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(embebida, proyecto) {
		t.Fatalf("3febb4/CA-488: las dos copias del contrato 06 son iguales")
	}
	if emb := raEmbebido(t); !raMismoTexto(emb, string(embebida)) {
		t.Fatalf("3febb4/CA-488: la copia embebida es internal/agents/assets/agents/06-reviewer.md")
	}
	texto := string(embebida)
	nombres := []string{"intake/", "ratchet.json", ".hoom/.gitignore", "estado de hoom"}
	for _, l := range hoomfs.Local {
		nombres = append(nombres, l)
	}
	for _, n := range nombres {
		if !strings.Contains(texto, n) {
			t.Fatalf("3febb4/CA-488: el contrato 06 nombra %q como parte del estado de hoom:\n%s", n, texto)
		}
	}
	// los nombra donde dice que es el estado de hoom: en el mismo parrafo
	parrafo := ""
	for _, p := range strings.Split(texto, "\n\n") {
		if strings.Contains(p, "estado de hoom") && strings.Contains(p, "ratchet.json") {
			parrafo = p
			break
		}
	}
	if parrafo == "" {
		t.Fatalf("3febb4/CA-488: un parrafo del contrato 06 dice que es el estado de hoom y nombra ratchet.json:\n%s", texto)
	}
	for _, n := range nombres {
		if !strings.Contains(parrafo, n) {
			t.Fatalf("3febb4/CA-488: el parrafo del estado de hoom nombra %q (la lista es cerrada, no ejemplos):\n%s", n, parrafo)
		}
	}
}
