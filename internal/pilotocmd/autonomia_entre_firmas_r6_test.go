// Tests de regresion de la ronda 6 de autonomia-entre-firmas, como
// consumidor externo (proyectos git reales, CLIs de IA falsos en un PATH
// minimo, nunca uno real):
//
//   - hallazgos 20261113T040622_8e2d38, 88ed5f y d908ff (CA-638, CA-644,
//     aclaracion 25), el piloto: despues de un sobre de writer que dejo un
//     hook en el repo de la tarea, `hoom piloto` se detiene con el Why del
//     caso 8 antes de guardar, y el hook no corre, este su hallazgo abierto,
//     corregido o refutado (el gate de ese sobre esta en
//     internal/agentcmd/autonomia_entre_firmas_r6_test.go);
//   - hallazgos e6461a y 897491 (CA-644, aclaracion 26): la vuelta atras del
//     guardado mueve solo la rama de la tarea, y solo si su punta es el
//     commit del guardado;
//   - hallazgo 7527da (CA-644, aclaracion 26): el commit del guardado trae
//     todas las rutas pedidas.
//
// Reusa los helpers de las rondas anteriores (apr5Protegida, apr5Fallo,
// apr5NoGuardo, apr4Hook, apr1Arrancar, apr3Matar...).
package pilotocmd_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/agentcmd"
	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/itemcmd"
	"github.com/hoomdev/hoomai/internal/pilotocmd"
)

// apr6HallazgoDelGate es el hallazgo abierto high del gate de scope cuya
// descripcion nombra ruta, y el arbol donde esta.
func apr6HallazgoDelGate(t *testing.T, ruta string, dirs ...string) (finding.Item, string, bool) {
	t.Helper()
	for _, d := range dirs {
		items, _, err := finding.List(d, apBase, true)
		if err != nil {
			continue
		}
		for _, it := range items {
			if it.Status == finding.StatusOpen && it.Severity == "high" && it.Author == agentcmd.AutorGateDeScope &&
				strings.Contains(it.Description, ruta) {
				return it, d, true
			}
		}
	}
	return finding.Item{}, "", false
}

// apr6TopeChico achica itemcmd.TopeGit a 2 s y lo restaura al final.
func apr6TopeChico(t *testing.T) {
	t.Helper()
	tope := itemcmd.TopeGit
	itemcmd.TopeGit = 2 * time.Second
	t.Cleanup(func() { itemcmd.TopeGit = tope })
}

// apr6Correr corre `hoom piloto <slug>` en root sin esperarlo y exige que
// termine en 30 s (con TopeGit en 2 s); si no, mata los pids y falla.
func apr6Correr(t *testing.T, ca, root, slug, pids string) apSalida {
	t.Helper()
	p := apr1Arrancar(root, slug)
	if !p.esperar(30 * time.Second) {
		apr3Matar(pids)
		p.esperar(2 * time.Minute)
		t.Fatalf("%s: con un post-commit que no termina, el piloto termina en un tiempo acotado (TopeGit 2 s); a los 30 s seguia:\n%s", ca, p.salida())
	}
	return p.salida()
}

// Hallazgos 20261113T040622_8e2d38, 88ed5f y d908ff (CA-638, CA-644,
// aclaracion 25), el piloto: un run de writer (CLI falso, lanzado como por
// el piloto) deja un pre-commit y un reference-transaction en el directorio
// de hooks comun del repo de la tarea y cambia codigo, y no lo guarda. Ese
// es el ultimo sobre de la tarjeta. `hoom piloto` se detiene con exit 1 y el
// Why del caso 8 antes de guardar: no imprime la linea de guardado, no pide
// a nadie ni imprime regla, no crea sobre, no commitea (HEAD y la rama
// quedan donde estaban, el cambio de codigo sigue sin guardar), suelta su
// candado, y ningun hook que dejo el writer corre (sus marcas no aparecen).
// Lo mismo con el hallazgo del gate de ese sobre abierto (y sigue abierto),
// cerrado como corregido o cerrado como refutado.
func TestHallazgos8e2d38_88ed5f_d908ff_CA638_CA644_ElPilotoNoGuardaDespuesDeUnHookDelWriter(t *testing.T) {
	for _, estado := range []string{finding.StatusOpen, finding.StatusCorrected, finding.StatusRefuted} {
		ca := "8e2d38/88ed5f/d908ff/CA-638/CA-644 (hallazgo " + estado + ")"
		f := apPATH(t)
		root, wt, slug := apr5Protegida(t, ca)
		aparte := t.TempDir()
		marcaPC := filepath.Join(aparte, "corrio-pre-commit")
		marcaRT := filepath.Join(aparte, "corrio-reference-transaction")
		apr1Fake(t, f, "claude", "h=$(git rev-parse --git-path hooks)\nmkdir -p \"$h\"\n"+
			"cat > \"$h/pre-commit\" <<'HOOM_HOOK'\n#!/bin/sh\n: > '"+marcaPC+"'\nexit 0\nHOOM_HOOK\nchmod +x \"$h/pre-commit\"\n"+
			"cat > \"$h/reference-transaction\" <<'HOOM_HOOK'\n#!/bin/sh\ncat > /dev/null\n: > '"+marcaRT+"'\nexit 0\nHOOM_HOOK\nchmod +x \"$h/reference-transaction\"\n"+
			"printf 'package app // implementado\\n' > app.go\nexit 0\n")
		var buf bytes.Buffer
		w, err := agentcmd.Run(root, apBase, agentcmd.Options{Role: "writer", Provider: "claude", Task: slug, Prompt: "implementa el spec",
			Pilot: true, Columna: boardcmd.ColWriter, Regla: boardcmd.ReglaCadena}, &buf)
		if err != nil || w.EnvelopeID == "" {
			t.Fatalf("%s: fixture: el sobre del writer corre: %v %+v\n%s", ca, err, w, buf.String())
		}
		for _, h := range []string{"pre-commit", "reference-transaction"} {
			ruta := apGit(t, wt, "rev-parse", "--git-path", "hooks/"+h)
			if !filepath.IsAbs(ruta) {
				ruta = filepath.Join(wt, ruta)
			}
			if !apr1Existe(ruta) {
				t.Fatalf("%s: fixture: el writer dejo el hook %s en %s", ca, h, ruta)
			}
		}
		if err := os.Remove(filepath.Join(f.bin, "claude")); err != nil { // el piloto no tiene a quien lanzar
			t.Fatal(err)
		}
		if estado != finding.StatusOpen { // una persona cierra todos los del gate que nombran un hook
			cerrados := 0
			for h, dir, ok := apr6HallazgoDelGate(t, "hooks/", wt, root); ok; h, dir, ok = apr6HallazgoDelGate(t, "hooks/", wt, root) {
				if _, err := finding.Resolve(dir, h.ID, estado, "lo revise a mano: el hook era lo que habia que hacer",
					"hoom test <test@hoom.dev>"); err != nil {
					t.Fatalf("%s: fixture: una persona cierra %s como %s: %v", ca, h.ID, estado, err)
				}
				if cerrados++; cerrados > 10 {
					t.Fatalf("%s: fixture: el hallazgo %s quedo cerrado", ca, h.ID)
				}
			}
			if cerrados == 0 {
				t.Fatalf("%s: fixture: el sobre del writer dejo abierto un hallazgo high de %q que nombra el hook (aclaracion 25): %+v", ca, agentcmd.AutorGateDeScope, w)
			}
		}
		// lo que haya corrido durante el sobre del writer o al cerrar el
		// hallazgo no cuenta: se mira solo el piloto
		_ = os.Remove(marcaPC)
		_ = os.Remove(marcaRT)
		head, rama := apHead(t, wt), apGit(t, wt, "rev-parse", "hoom/"+slug)
		sobres := len(envelope.List(root))

		s := apRun(t, ca, root, slug)
		ls := apSoloLineasDelPiloto(t, ca, s)
		for _, l := range ls {
			if apGuarda.MatchString(l) || strings.Contains(l, " pide al ") || strings.HasPrefix(l, apPrefijo+"regla ") ||
				strings.Contains(l, "no pudo lanzar") {
				t.Fatalf("%s: despues del sobre que dejo un hook, el piloto no guarda ni lanza a nadie: %q\n%s", ca, l, s)
			}
		}
		if apr1Existe(marcaPC) || apr1Existe(marcaRT) {
			t.Fatalf("%s: el hook que dejo el writer no corre (pre-commit %v, reference-transaction %v):\n%s",
				ca, apr1Existe(marcaPC), apr1Existe(marcaRT), s)
		}
		want := apDetiene + "el trabajo del writer no cerro entregable (" + envelope.StatusNotDeliverable + "): el piloto se detiene ante un rojo"
		if s.code != 1 || apUltima(ls) != want {
			t.Fatalf("%s: se detiene con el Why del caso 8, %q, y exit 1:\n%s", ca, want, s)
		}
		if h, r := apHead(t, wt), apGit(t, wt, "rev-parse", "hoom/"+slug); h != head || r != rama {
			t.Fatalf("%s: no commitea: HEAD %s -> %s, hoom/%s %s -> %s", ca, head, h, slug, rama, r)
		}
		if st := apGit(t, wt, "status", "--porcelain", "--", "app.go"); st == "" {
			t.Fatalf("%s: el cambio de codigo del writer sigue sin guardar", ca)
		}
		if n := len(envelope.List(root)); n != sobres {
			t.Fatalf("%s: ningun sobre nuevo: %d -> %d", ca, sobres, n)
		}
		soltar, err := hoomfs.Lock(pilotocmd.Candado(root, slug), 0)
		if err != nil {
			t.Fatalf("%s: al detenerse, el piloto suelta su candado: %v", ca, err)
		}
		soltar()
		if estado == finding.StatusOpen {
			if _, _, ok := apr6HallazgoDelGate(t, "hooks/", wt, root); !ok {
				t.Fatalf("%s: el hallazgo high de %q que nombra el hook sigue abierto (aclaracion 25)", ca, agentcmd.AutorGateDeScope)
			}
		}
	}
}

// Hallazgo e6461a (CA-644, aclaracion 26): en Tu aprobacion con un cambio
// sin guardar, un post-commit hace que HEAD nombre otra rama (`git
// symbolic-ref HEAD refs/heads/main`) y despues se cuelga mas que TopeGit
// (2 s). `hoom piloto` termina en un tiempo acotado con exit 1 y `no pudo
// guardar el trabajo de la tarjeta: <error>`, sin linea de guardado y con el
// candado libre; la otra rama (main) conserva su punta y la rama de la
// tarea vuelve a donde estaba. Que nombra HEAD despues no se fija.
func TestHallazgoE6461a_CA644_LaVueltaAtrasNoMueveOtraRama(t *testing.T) {
	const ca = "e6461a/CA-644"
	apr6TopeChico(t)
	f := apPATH(t)
	f.cli(t, "claude")
	root := apProyecto(t, "")
	const slug = "otra-rama"
	wt := apTuAprobacion(t, root, slug, apConAuto, true)
	apEscribir(t, wt, "app.go", "package app\n\n// sin guardar\n")
	puntaMain, rama := apGit(t, root, "rev-parse", "refs/heads/main"), apGit(t, wt, "rev-parse", "hoom/"+slug)
	if puntaMain == rama {
		t.Fatalf("%s: fixture: la rama de la tarea y main tienen puntas distintas: %s", ca, puntaMain)
	}

	aparte := t.TempDir()
	pids, corrio := filepath.Join(aparte, "pids"), filepath.Join(aparte, "corrio")
	apr4Hook(t, wt, "post-commit", "#!/bin/sh\necho $$ >> '"+pids+"'\n"+
		"git symbolic-ref HEAD refs/heads/main && : > '"+corrio+"'\nexec sleep 60\n")
	t.Cleanup(func() { apr3Matar(pids) })

	s := apr6Correr(t, ca, root, slug, pids)
	if !apr1Existe(corrio) {
		t.Fatalf("%s: fixture: el post-commit corrio y HEAD paso a nombrar main:\n%s", ca, s)
	}
	apr5Fallo(t, ca, s, root, slug)
	if m := apGit(t, root, "rev-parse", "refs/heads/main"); m != puntaMain {
		t.Fatalf("%s: la vuelta atras no mueve otra rama: main %s -> %s", ca, puntaMain, m)
	}
	if r := apGit(t, root, "rev-parse", "refs/heads/hoom/"+slug); r != rama {
		t.Fatalf("%s: la rama de la tarea vuelve a donde estaba: hoom/%s %s -> %s", ca, slug, rama, r)
	}
	f.ninguna(t, ca, "un piloto que no pudo guardar")
}

// Hallazgo 897491 (CA-644, aclaracion 26): en Tu aprobacion con un cambio
// sin guardar, un post-commit hace otro commit encima (`git commit
// --allow-empty -m otro`) y despues se cuelga mas que TopeGit (2 s). `hoom
// piloto` termina en un tiempo acotado con exit 1 y `no pudo guardar el
// trabajo de la tarjeta: <error>`, sin linea de guardado y con el candado
// libre; la rama de la tarea conserva el commit de encima (no vuelve a la
// punta de antes).
func TestHallazgo897491_CA644_LaVueltaAtrasNoBorraUnCommitDeEncima(t *testing.T) {
	const ca = "897491/CA-644"
	apr6TopeChico(t)
	f := apPATH(t)
	f.cli(t, "claude")
	root := apProyecto(t, "")
	const slug = "commit-encima"
	wt := apTuAprobacion(t, root, slug, apConAuto, true)
	apEscribir(t, wt, "app.go", "package app\n\n// sin guardar\n")
	rama := apGit(t, wt, "rev-parse", "hoom/"+slug)

	aparte := t.TempDir()
	pids, encima := filepath.Join(aparte, "pids"), filepath.Join(aparte, "encima")
	apr4Hook(t, wt, "post-commit", "#!/bin/sh\n[ -n \"$HOOM_ENCIMA\" ] && exit 0\necho $$ >> '"+pids+"'\n"+
		"HOOM_ENCIMA=1 git commit -q --allow-empty -m otro && git rev-parse HEAD > '"+encima+"'\nexec sleep 60\n")
	t.Cleanup(func() { apr3Matar(pids) })

	s := apr6Correr(t, ca, root, slug, pids)
	raw, _ := os.ReadFile(encima)
	arriba := strings.TrimSpace(string(raw))
	if arriba == "" || arriba == rama {
		t.Fatalf("%s: fixture: el post-commit corrio e hizo otro commit encima del guardado: %q\n%s", ca, arriba, s)
	}
	apr5Fallo(t, ca, s, root, slug)
	if r := apGit(t, root, "rev-parse", "refs/heads/hoom/"+slug); r != arriba {
		t.Fatalf("%s: con un commit encima, la vuelta atras no mueve la rama: hoom/%s es %s, el commit de encima %s (antes %s)", ca, slug, r, arriba, rama)
	}
	f.ninguna(t, ca, "un piloto que no pudo guardar")
}

// Hallazgo 7527da (CA-644, aclaracion 26): en Tu aprobacion con dos archivos
// sin guardar (app.go cambiado y otro.go nuevo), un pre-commit saca uno del
// indice (`git reset -q HEAD -- otro.go`) y el commit saldria sin el. `hoom
// piloto` sale con exit 1 y `no pudo guardar el trabajo de la tarjeta:
// <error>`, sin linea de guardado, con HEAD y la rama de la tarea donde
// estaban y el candado libre.
func TestHallazgo7527da_CA644_ElGuardadoTraeTodasLasRutasPedidas(t *testing.T) {
	const ca = "7527da/CA-644"
	f := apPATH(t)
	f.cli(t, "claude")
	root := apProyecto(t, "")
	const slug = "ruta-perdida"
	wt := apTuAprobacion(t, root, slug, apConAuto, true)
	apEscribir(t, wt, "app.go", "package app\n\n// sin guardar\n")
	apEscribir(t, wt, "otro.go", "package app\n\n// nuevo, sin guardar\n")
	antes := apr5Mirar(t, root, wt, slug)
	corrio := filepath.Join(t.TempDir(), "corrio")
	apr4Hook(t, wt, "pre-commit", "#!/bin/sh\n: > '"+corrio+"'\ngit reset -q HEAD -- otro.go\nexit 0\n")

	s := apRun(t, ca, root, slug)
	if !apr1Existe(corrio) {
		t.Fatalf("%s: fixture: el guardado del piloto corre el pre-commit (aclaracion 23):\n%s", ca, s)
	}
	apr5NoGuardo(t, ca, s, antes)
	f.ninguna(t, ca, "un piloto que no pudo guardar")
}
