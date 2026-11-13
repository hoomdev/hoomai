// Tests de regresion de la ronda 1 de autonomia-entre-firmas sobre la
// vuelta del piloto, como consumidor externo (proyectos git reales, CLIs de
// IA falsos en un PATH minimo, nunca uno real):
//
//   - hallazgo 20261112T172019_8f4011 (CA-656, aclaracion 16): cerrar-hallazgos
//     es exclusivo hasta que guarda;
//   - hallazgo 20261112T173402_24e6e0 (CA-638, CA-652, aclaracion 15): el
//     writer que toco un test protegido y vencio no se releva;
//   - hallazgo 20261112T174304_8c4407 (CA-644, aclaracion 17): un comando
//     git del guardado que no termina (un filtro clean o un hook pre-commit,
//     aclaraciones 21 y 23) no cuelga al piloto (itemcmd.TopeGit achicado en
//     el test).
//
// Reusa los helpers de autonomia_entre_firmas_test.go (apPATH, apProyecto,
// apRun, apTrasCerrar, apCerrado...).
package pilotocmd_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/agentcmd"
	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/itemcmd"
	"github.com/hoomdev/hoomai/internal/lanzar"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/pilotocmd"
)

// --- un piloto corriendo en otra goroutine ---

type apr1Piloto struct {
	out, err apBuf
	hecho    chan int
	code     int
	listo    bool
}

// apr1Arrancar corre `hoom piloto <args>` en dir sin esperarlo.
func apr1Arrancar(dir string, args ...string) *apr1Piloto {
	p := &apr1Piloto{hecho: make(chan int, 1)}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(&p.err, "PANICO: %v", r)
				p.hecho <- -1
			}
		}()
		p.hecho <- pilotocmd.Run(args, pilotocmd.Entorno{Dir: dir, Out: &p.out, Err: &p.err})
	}()
	return p
}

// esperar espera hasta d a que el piloto termine y dice si termino.
func (p *apr1Piloto) esperar(d time.Duration) bool {
	if p.listo {
		return true
	}
	select {
	case p.code = <-p.hecho:
		p.listo = true
	case <-time.After(d):
	}
	return p.listo
}

func (p *apr1Piloto) salida() apSalida {
	return apSalida{code: p.code, out: p.out.String(), err: p.err.String()}
}

func apr1Existe(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// apr1Fake instala un CLI falso que se registra como los de apFalsos.cli
// (stdin y argumentos) y despues corre cuerpo.
func apr1Fake(t *testing.T, f *apFalsos, name, cuerpo string) {
	t.Helper()
	final := filepath.Join(f.argv, name)
	tmp := filepath.Join(f.argv, ".tmp-"+name)
	in := filepath.Join(f.argv, "in-"+name)
	s := "#!/bin/sh\n" +
		"cat > '" + in + ".'$$\n" +
		"printf '%s\\000' \"$@\" > '" + tmp + ".'$$\n" +
		"mv '" + tmp + ".'$$ '" + final + ".'$$\n" + cuerpo
	if err := os.WriteFile(filepath.Join(f.bin, name), []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
}

// apr1Gate cambia el comando del gate test del hoom.yaml de root (el de
// apProyecto, `cmd: "true"`) y lo guarda.
func apr1Gate(t *testing.T, root, cmd string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, "hoom.yaml"))
	if err != nil || !strings.Contains(string(raw), "cmd: \"true\"") {
		t.Fatalf("fixture: el hoom.yaml de apProyecto tiene el gate barato: %v\n%s", err, raw)
	}
	apEscribir(t, root, "hoom.yaml", strings.Replace(string(raw), "cmd: \"true\"", "cmd: \""+cmd+"\"", 1))
	apCommit(t, root, "el gate test")
}

// apr1GateQueEspera es un gate que deja dir/esperando al arrancar y no
// termina hasta que aparece dir/soltar (o 60 s): la ventana del test.
func apr1GateQueEspera(t *testing.T) (dir, cmd string) {
	t.Helper()
	dir = t.TempDir()
	script := filepath.Join(dir, "gate.sh")
	s := "#!/bin/sh\n: > '" + filepath.Join(dir, "esperando") + "'\ni=0\n" +
		"while [ ! -f '" + filepath.Join(dir, "soltar") + "' ] && [ $i -lt 1200 ]; do sleep 0.05; i=$((i+1)); done\nexit 0\n"
	if err := os.WriteFile(script, []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir, "sh " + script
}

// apr1Ronda es apRonda(t, "antes", false) con el gate test cambiado a cmd
// antes de abrir la tarea (asi los veredictos de la ronda tienen la huella
// del hoom.yaml que el piloto va a usar).
func apr1Ronda(t *testing.T, cmd string) apRondaFix {
	t.Helper()
	root := apProyecto(t, "autonomia:\n  rondas_de_hallazgos: 1\n")
	apr1Gate(t, root, cmd)
	const slug = "ronda"
	apItem(t, root, slug, apConAuto)
	wt := apTarea(t, root, slug)
	apEscribir(t, wt, apSpecRel(slug), apSpecTexto("- CA-1: el total redondea bien."))
	apFirmar(t, wt, slug)
	apEscribir(t, wt, "precios_test.go", "package app\n\n// CA-1: el total redondea bien\n")
	apCommit(t, wt, "spec firmado y test del test-writer")

	fx := apRondaFix{root: root, wt: wt, slug: slug}
	reg := func(desc string) string {
		h, err := finding.Register(wt, apBase, finding.Draft{Severity: "high", Lens: "correctness", File: "app.go",
			Description: desc, Author: "reviewer", Task: slug})
		if err != nil {
			t.Fatalf("fixture: registrar el hallazgo: %v", err)
		}
		time.Sleep(1100 * time.Millisecond) // ids en orden
		return h.ID
	}
	fx.f1 = reg("el total redondea mal con descuentos")
	fx.f2 = reg("la region se pierde al recargar")
	apEscribir(t, wt, "precios_test.go", "package app\n\n// CA-1: el total redondea bien\n// "+fx.f2+": la region al recargar\n")
	apEscribir(t, wt, "hallazgos_test.go", "package app\n\n// "+fx.f1+": el total con descuento\n")
	apEscribir(t, wt, "app.go", "package app\n\n// el arreglo de la ronda 1\n")
	apCommit(t, wt, "tests de la ronda y arreglo")
	ahora := time.Now().UTC()
	fx.vTW = apVeredicto(t, wt, slug, ahora.Add(-3*time.Minute), "test")
	fx.vW = apVeredicto(t, wt, slug, ahora.Add(-time.Minute), finding.GateName)
	apCommit(t, wt, "veredictos")
	fx.veredictos = apVeredictos(t, wt)

	ids := []string{fx.f1, fx.f2}
	pasos := map[string]int{"run": 3, "scope": 4, "verify": 5, "ok": 6}
	base := ahora.Add(-2 * time.Hour)
	for i, r := range []envelope.Record{
		{Role: "test-writer", Columna: boardcmd.ColTestWriter, Stage: "verify", Status: envelope.StatusNotDeliverable,
			Trasplantados: []string{"precios_test.go"}, Pedido: "los tests del spec"},
		{Role: "writer", Columna: boardcmd.ColWriter, Stage: "ok", Status: envelope.StatusDeliverable, Pedido: "implementa el spec"},
		{Role: "reviewer", Columna: boardcmd.ColReview, Stage: "ok", Status: envelope.StatusDeliverable},
		{Role: "refutador", Columna: boardcmd.ColReview, Stage: "ok", Status: envelope.StatusDeliverable, Pilot: true,
			Regla: boardcmd.ReglaRondaRefutador, Ronda: 1, Hallazgos: ids, Bloqueantes: len(ids), Pedido: "refuta"},
		{Role: "test-writer", Columna: boardcmd.ColReview, Stage: "verify", Status: envelope.StatusNotDeliverable, Pilot: true,
			Regla: boardcmd.ReglaRondaTestWriter, Ronda: 1, Hallazgos: ids, Trasplantados: []string{"hallazgos_test.go"},
			VerdictID: fx.vTW, Verdict: "red", Pedido: "reproduce los hallazgos"},
		{Role: "writer", Columna: boardcmd.ColReview, Stage: "verify", Status: envelope.StatusNotDeliverable, Pilot: true,
			Regla: boardcmd.ReglaRondaWriter, Ronda: 1, VerdictID: fx.vW, Verdict: "red", Pedido: "corrige el codigo"},
	} {
		ini := base.Add(time.Duration(i*10) * time.Minute)
		r.ID = ini.Format("20060102T150405") + fmt.Sprintf("_ae%04x", i+1)
		r.Provider, r.Task, r.Dir, r.Spec = "claude", slug, wt, apSpecRel(slug)
		r.Step, r.Steps = pasos[r.Stage], 6
		r.StartedAt, r.UpdatedAt, r.EndedAt = ini, ini.Add(5*time.Minute), ini.Add(5*time.Minute)
		if r.Status != envelope.StatusDeliverable {
			r.ExitCode = 1
		}
		if err := envelope.Write(root, r); err != nil {
			t.Fatalf("fixture: sobre %s: %v", r.Role, err)
		}
	}
	if n := len(envelope.List(root)); n != 6 {
		t.Fatalf("fixture: los seis sobres de la tarjeta estan en disco: %d", n)
	}
	return fx
}

// apr1AccionDePersona es una accion de rol habilitada que una persona ve en
// la tarjeta y cuyos valores con claude se pueden lanzar hoy.
func apr1AccionDePersona(t *testing.T, root, slug string) boardcmd.Action {
	t.Helper()
	c, err := boardcmd.CardFor(root, apBase, apBlockOn, slug, time.Now().UTC())
	if err != nil {
		t.Fatalf("fixture: CardFor %s: %v", slug, err)
	}
	for _, a := range c.Actions {
		if !a.Enabled || a.Role == "" || !strings.HasPrefix(a.ID, "pedir-") {
			continue
		}
		if lanzar.Comprobar(c, a, apr1Pedido(a)) == nil {
			return a
		}
	}
	t.Fatalf("fixture: la tarjeta %s (%s) tiene una accion de rol que una persona puede lanzar con claude: %+v", slug, c.Column, c.Actions)
	return boardcmd.Action{}
}

func apr1Pedido(a boardcmd.Action) lanzar.Request {
	pedido := a.Pedido
	if pedido == "" {
		pedido = "sigue con la tarjeta"
	}
	return lanzar.Request{Action: a.ID, Provider: "claude", Pedido: pedido}
}

// apr1LanzaUnaPersona es el pedido de una persona (sin Pilot) por la funcion
// de lanzamiento compartida.
func apr1LanzaUnaPersona(t *testing.T, root, slug string, a boardcmd.Action) (*lanzar.Trabajo, error) {
	t.Helper()
	m, err := manifest.Load(root, nil)
	if err != nil {
		t.Fatalf("fixture: hoom.yaml: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return lanzar.Lanzar(ctx, lanzar.Params{Root: root, Manifest: m}, slug, apr1Pedido(a))
}

// Hallazgo 20261112T172019_8f4011 (CA-656, aclaracion 16): mientras el
// piloto corre cerrar-hallazgos (cerrar, verify, guardar), el pedido de una
// persona sobre la misma tarjeta se rechaza: ningun rol corre y no hay sobre
// nuevo. Al terminar, el piloto cerro, guardo y el espacio de trabajo queda
// sin nada por commitear. La ventana: el gate test del verify del paso no
// termina hasta que el test lo suelta. Control: sin el piloto corriendo, el
// mismo pedido de la persona en la misma tarjeta lanza.
func TestHallazgo8f4011_CA656_CerrarHallazgosEsExclusivo(t *testing.T) {
	const ca = "8f4011/CA-656"
	f := apPATH(t)
	f.cli(t, "claude")

	gCtl, cmdCtl := apr1GateQueEspera(t)
	apEscribir(t, gCtl, "soltar", "")
	ctl := apr1Ronda(t, cmdCtl)
	accion := apr1AccionDePersona(t, ctl.root, ctl.slug)
	tr, err := apr1LanzaUnaPersona(t, ctl.root, ctl.slug, accion)
	if err != nil || tr == nil || tr.EnvelopeID == "" {
		t.Fatalf("%s: fixture: sin el piloto corriendo, el pedido %s de una persona lanza: %v %+v", ca, accion.ID, err, tr)
	}
	tr.Wait()

	g, cmd := apr1GateQueEspera(t)
	fx := apr1Ronda(t, cmd)
	if a := apr1AccionDePersona(t, fx.root, fx.slug); a.ID != accion.ID {
		t.Fatalf("%s: fixture: las dos tarjetas ofrecen la misma accion: %s y %s", ca, accion.ID, a.ID)
	}
	claude := f.llamadas(t, "claude")
	soltar := func() { _ = os.WriteFile(filepath.Join(g, "soltar"), nil, 0o644) }

	p := apr1Arrancar(fx.root, fx.slug)
	t.Cleanup(func() { soltar(); p.esperar(2 * time.Minute) })
	limite := time.Now().Add(2 * time.Minute)
	for !apr1Existe(filepath.Join(g, "esperando")) {
		if p.esperar(50 * time.Millisecond) {
			t.Fatalf("%s: fixture: el piloto llega al verify de cerrar-hallazgos (el gate no corrio):\n%s", ca, p.salida())
		}
		if time.Now().After(limite) {
			t.Fatalf("%s: fixture: el gate del verify de cerrar-hallazgos arranca en 2 minutos:\n%s", ca, p.out.String())
		}
	}

	tr, err = apr1LanzaUnaPersona(t, fx.root, fx.slug, accion)
	if err == nil {
		soltar()
		if tr != nil {
			tr.Wait()
		}
		t.Fatalf("%s: mientras el piloto cierra los hallazgos (su verify corriendo), el pedido %s de una persona se rechaza; lanzo %+v", ca, accion.ID, tr)
	}
	if tr != nil {
		tr.Wait()
	}
	if n := len(envelope.List(fx.root)); n != 6 {
		t.Fatalf("%s: el pedido rechazado no crea sobre: %d sobres (eran 6): %v", ca, n, err)
	}
	if n := f.llamadas(t, "claude"); n != claude {
		t.Fatalf("%s: el pedido rechazado no corre ningun rol: claude corrio %d veces (antes %d)", ca, n, claude)
	}

	soltar()
	if !p.esperar(2 * time.Minute) {
		t.Fatalf("%s: soltado el gate, el piloto termina:\n%s", ca, p.out.String())
	}
	s := p.salida()
	if s.code == -1 {
		t.Fatalf("%s: el piloto entro en panico:\n%s", ca, s)
	}
	apTrasCerrar(t, ca, fx, s, []string{fx.f1})
	apCerrado(t, ca, fx, fx.f1, "hallazgos_test.go")
	if want := apDetiene + "quedan hallazgos sin test en la ronda 1: " + fx.f2 + ": te toca"; s.code != 1 || apUltima(apLineas(s.out)) != want {
		t.Fatalf("%s: el piloto termina su paso como sin el pedido de la persona (%q, exit 1):\n%s", ca, want, s)
	}
	if n := len(envelope.List(fx.root)); n != 6 {
		t.Fatalf("%s: ningun trabajo nuevo en la tarjeta: %d sobres", ca, n)
	}
	if n := f.llamadas(t, "claude"); n != claude {
		t.Fatalf("%s: ningun rol corrio en la tarjeta: claude corrio %d veces (antes %d)", ca, n, claude)
	}
}

// apr1Correr corre un rol por el sobre, como lo lanzaria el piloto.
func apr1Correr(t *testing.T, root string, o agentcmd.Options) agentcmd.Result {
	t.Helper()
	var buf bytes.Buffer
	res, err := agentcmd.Run(root, apBase, o, &buf)
	if err != nil || res.EnvelopeID == "" {
		t.Fatalf("fixture: el sobre del %s corre: %v %+v\n%s", o.Role, err, res, buf.String())
	}
	return res
}

// apr1Abierto busca un hallazgo abierto high cuya descripcion empieza con
// pre, en los arboles dados.
func apr1Abierto(t *testing.T, pre string, dirs ...string) (finding.Item, bool) {
	t.Helper()
	for _, d := range dirs {
		items, _, err := finding.List(d, apBase, true)
		if err != nil {
			continue
		}
		for _, it := range items {
			if it.Status == finding.StatusOpen && it.Severity == "high" && strings.HasPrefix(it.Description, pre) {
				return it, true
			}
		}
	}
	return finding.Item{}, false
}

// Hallazgo 20261112T173402_24e6e0 (CA-638, CA-652, aclaracion 15): el writer
// que el piloto lanzo cambia el test protegido y despues muere por su
// timeout. El piloto no lo releva (caso 5) ni lo repite: se detiene sin
// lanzar, con el Why del caso 8, exit 1, y el hallazgo high de
// tests-del-test-writer sigue abierto. De punta a punta: sobres reales con
// CLIs falsos y despues `hoom piloto`.
func TestHallazgo24e6e0_CA638_CA652_ElWriterQueTocoUnProtegidoYSeCortoNoSeReleva(t *testing.T) {
	const ca = "24e6e0/CA-638/CA-652"
	f := apPATH(t)
	root := apProyecto(t, "")
	apr1Gate(t, root, "grep -q implementado app.go") // rojo hasta que haya codigo: la tarjeta queda en Writer
	const slug = "relevo"
	apItem(t, root, slug, apConAuto)
	wt := apTarea(t, root, slug)
	apEscribir(t, wt, apSpecRel(slug), apSpecTexto("- CA-1: el total redondea bien."))
	apFirmar(t, wt, slug)
	apCommit(t, wt, "spec aprobado")
	apColumna(t, ca, root, slug, boardcmd.ColTestWriter)

	apr1Fake(t, f, "claude", "printf 'package app\\n\\n// CA-1: el total redondea bien\\n' > precios_test.go\nexit 0\n")
	tw := apr1Correr(t, root, agentcmd.Options{Role: "test-writer", Provider: "claude", Task: slug, Prompt: "escribi los tests de CA-1"})
	if rec := apRegistro(t, ca, root, tw.EnvelopeID); !reflect.DeepEqual(rec.Trasplantados, []string{"precios_test.go"}) {
		t.Fatalf("%s: fixture: el test-writer protege precios_test.go: %+v %+v", ca, rec, tw)
	}
	apGit(t, wt, "add", "-A")
	apGit(t, wt, "commit", "-q", "--allow-empty", "-m", apGuardaMs+slug)
	apColumna(t, ca, root, slug, boardcmd.ColWriter)

	apr1Fake(t, f, "claude", "printf 'package app\\n\\n// CA-1: el total redondea bien, segun el writer\\n' > precios_test.go\nexec sleep 60\n")
	desde := time.Now()
	w := apr1Correr(t, root, agentcmd.Options{Role: "writer", Provider: "claude", Task: slug, Prompt: "implementa el spec",
		Timeout: 3 * time.Second, Pilot: true, Columna: boardcmd.ColWriter, Regla: boardcmd.ReglaCadena})
	if time.Since(desde) > 45*time.Second || w.Status == envelope.StatusDeliverable || w.ExitCode == 0 {
		t.Fatalf("%s: fixture: el writer muere por su timeout de 3 s sin entregar: %+v (%s)", ca, w, time.Since(desde))
	}
	if raw, _ := os.ReadFile(filepath.Join(wt, "precios_test.go")); !strings.Contains(string(raw), "segun el writer") {
		t.Fatalf("%s: fixture: el writer cambio el test protegido antes de morir: %q", ca, raw)
	}

	f.cli(t, "claude") // si el piloto lo releva, este claude termina enseguida
	claude, sobres := f.llamadas(t, "claude"), len(envelope.List(root))
	s := apRun(t, ca, root, slug)
	ls := apSoloLineasDelPiloto(t, ca, s)
	for _, l := range ls {
		if strings.Contains(l, " pide al ") || strings.HasPrefix(l, apPrefijo+"regla ") || strings.Contains(l, "no pudo lanzar") {
			t.Fatalf("%s: el piloto no releva ni repite al writer que toco un test protegido: %q\n%s", ca, l, s)
		}
	}
	want := apDetiene + "el trabajo del writer no cerro entregable (" + envelope.StatusNotDeliverable + "): el piloto se detiene ante un rojo"
	if s.code != 1 || apUltima(ls) != want {
		t.Fatalf("%s: se detiene con el Why del caso 8, %q, y exit 1:\n%s", ca, want, s)
	}
	if n := f.llamadas(t, "claude"); n != claude {
		t.Fatalf("%s: ningun rol corre: claude corrio %d veces (antes %d)", ca, n, claude)
	}
	if n := len(envelope.List(root)); n != sobres {
		t.Fatalf("%s: ningun sobre nuevo: %d -> %d", ca, sobres, n)
	}
	pre := "tests-del-test-writer: el rol writer escribio precios_test.go"
	if _, ok := apr1Abierto(t, pre, wt, root); !ok {
		t.Fatalf("%s: el hallazgo high que empieza con %q sigue abierto", ca, pre)
	}
}

// Hallazgo 20261112T174304_8c4407 (CA-644, aclaracion 17): con algo que
// no termina en un comando git del guardado y cambios sin guardar en el
// espacio de trabajo, el piloto termina en un tiempo acotado (TopeGit
// achicado a 2 s), con exit 1 y `piloto automatico: se detiene: no pudo
// guardar el trabajo de la tarjeta: <error>`, sin commitear, y suelta su
// candado: otro piloto lo toma (no le dice que ya corre otro).
//
// La aclaracion 21 lo re-expreso con un filtro clean de git configurado en
// el repo de la tarea para app.go (apr3Filtro), en lugar del hook
// pre-commit. La aclaracion 23 (el guardado vuelve a correr los hooks;
// hallazgo low 20261112T205353_d6b391 sobre el nombre) lo re-expresa otra
// vez: corre con los dos mecanismos, el filtro y el mismo hook pre-commit
// que no termina, con las mismas exigencias en cada uno.
func TestHallazgo8c4407_CA644_UnHookQueNoTerminaNoCuelgaAlPiloto(t *testing.T) {
	tope := itemcmd.TopeGit
	itemcmd.TopeGit = 2 * time.Second
	t.Cleanup(func() { itemcmd.TopeGit = tope })

	for _, m := range []struct {
		nombre   string
		instalar func(t *testing.T, wt, script string)
	}{
		{"un filtro clean", apr3Filtro},
		{"un hook pre-commit", func(t *testing.T, wt, script string) { apr4Hook(t, wt, "pre-commit", script) }},
	} {
		ca := "8c4407/CA-644 (" + m.nombre + ")"
		f := apPATH(t)
		f.cli(t, "claude")
		root := apProyecto(t, "")
		const slug = "hook-colgado"
		wt := apTuAprobacion(t, root, slug, apConAuto, true)
		pids := filepath.Join(t.TempDir(), "pids")
		m.instalar(t, wt, "#!/bin/sh\necho $$ >> '"+pids+"'\nexec sleep 60\n")
		apEscribir(t, wt, "app.go", "package app\n\n// sin guardar\n")
		head, rama := apHead(t, wt), apGit(t, wt, "rev-parse", "hoom/"+slug)

		matar := func() {
			raw, _ := os.ReadFile(pids)
			for _, l := range strings.Fields(string(raw)) {
				if pid, err := strconv.Atoi(l); err == nil {
					if pr, err := os.FindProcess(pid); err == nil {
						_ = pr.Kill()
					}
				}
			}
		}
		t.Cleanup(matar)

		for vuelta := 1; vuelta <= 2; vuelta++ {
			desde := time.Now()
			p := apr1Arrancar(root, slug)
			if !p.esperar(30 * time.Second) {
				matar()
				p.esperar(2 * time.Minute)
				t.Fatalf("%s: (vuelta %d) con %s que no termina, el piloto termina en un tiempo acotado (TopeGit 2 s); a los 30 s seguia:\n%s", ca, vuelta, m.nombre, p.salida())
			}
			s := p.salida()
			ls := apSoloLineasDelPiloto(t, ca, s)
			pre := apDetiene + "no pudo guardar el trabajo de la tarjeta: "
			if s.code != 1 || !strings.HasPrefix(apUltima(ls), pre) || strings.TrimSpace(strings.TrimPrefix(apUltima(ls), pre)) == "" {
				t.Fatalf("%s: (vuelta %d) se detiene con `%s<error>` y exit 1 (en %s):\n%s", ca, vuelta, pre, time.Since(desde), s)
			}
			if strings.Contains(s.out+s.err, pilotocmd.MsgYaCorre(slug)) {
				t.Fatalf("%s: (vuelta %d) el piloto anterior solto su candado: este no dice que ya corre otro:\n%s", ca, vuelta, s)
			}
			for _, l := range ls {
				if apGuarda.MatchString(l) {
					t.Fatalf("%s: (vuelta %d) no dice que guardo: %q", ca, vuelta, l)
				}
			}
			if raw, _ := os.ReadFile(pids); strings.TrimSpace(string(raw)) == "" {
				t.Fatalf("%s: fixture: el guardado corrio %s", ca, m.nombre)
			}
			if h, r := apHead(t, wt), apGit(t, wt, "rev-parse", "hoom/"+slug); h != head || r != rama {
				t.Fatalf("%s: (vuelta %d) no commitea nada: HEAD %s -> %s, hoom/%s %s -> %s", ca, vuelta, head, h, slug, rama, r)
			}
			soltar, err := hoomfs.Lock(pilotocmd.Candado(root, slug), 0)
			if err != nil {
				t.Fatalf("%s: (vuelta %d) al detenerse, el piloto suelta su candado: %v", ca, vuelta, err)
			}
			soltar()
		}
		if m.nombre == "un filtro clean" {
			apr3SinFiltro(t, wt) // que git status no corra el filtro que no termina
		}
		if st := apGit(t, wt, "status", "--porcelain"); !strings.Contains(st, "app.go") {
			t.Fatalf("%s: lo no guardado sigue ahi: %q", ca, st)
		}
		f.ninguna(t, ca, "un piloto que no pudo guardar")
	}
}
