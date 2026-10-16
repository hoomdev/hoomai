// Tests adversariales de tres hallazgos de la review de C4 sobre el doctor
// (.hoom/specs/historia-doctor-y-cinta.md, pieza 3: CA-367, CA-369, CA-370):
//
//   - e5d1e4: un .hoom/worktrees (o .hoom/items) que existe y no se puede
//     leer no es "nada": Doctor devuelve error y 'hoom board doctor' no dice
//     "sin problemas de coherencia". Que no exista sigue siendo nada.
//   - f34bc3: un registro de sobre plantado a mano (id que no es el nombre de
//     su archivo, o sin forma de id) no llega a envelope.List ni a ninguna
//     accion del doctor; y ningun valor de telemetria (rol, provider, sesion)
//     se convierte en un comando que, pegado en sh -c, haga otra cosa.
//   - 5435b7: la tarea de un hallazgo (o de un veredicto) escrita a mano con
//     metacaracteres no llega sin escapar a la accion de evidencia-sin-item.
//
// "Pegar la accion" es literal: cada accion que el doctor devuelve o imprime
// se corre con /bin/sh -c (sh POSIX), con un PATH donde hoom y rm son falsos
// (anotan sus argumentos y no hacen nada) y touch es el de verdad. Si aparece
// un archivo PWNED*, la accion ejecuto algo que no era suyo. Ningun CLI de IA.
package boardcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/hoomfs"
)

// ---------------------------------------------------------------------------
// Arnes: hoom y rm falsos, y pegar una accion en sh -c.
// ---------------------------------------------------------------------------

// hdFalso es un directorio con un hoom y un rm falsos: cada invocacion anota
// en log una linea "== <nombre>" y despues un argumento por linea.
type hdFalso struct{ bin, log string }

func hdFalsos(t *testing.T) hdFalso {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"hoom", "rm"} {
		script := "#!/bin/sh\n{ printf '%s\\n' '== " + n + "'; for a in \"$@\"; do printf '%s\\n' \"$a\"; done; } >> \"$HD_LOG\"\nexit 0\n"
		if err := os.WriteFile(filepath.Join(bin, n), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return hdFalso{bin: bin, log: filepath.Join(dir, "llamadas.log")}
}

// pegar corre linea con /bin/sh -c en dir, con los falsos primero en el PATH.
func (f hdFalso) pegar(t *testing.T, dir, linea string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", linea)
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "PATH=") && !strings.HasPrefix(kv, "HOOM_TASK=") && !strings.HasPrefix(kv, "HD_LOG=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env,
		"PATH="+f.bin+string(os.PathListSeparator)+os.Getenv("PATH")+string(os.PathListSeparator)+"/usr/bin:/bin",
		"HD_LOG="+f.log)
	out, _ := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("pegar %q en sh -c no termino en 30 s", linea)
	}
	return string(out)
}

// llamadas devuelve cada invocacion de los falsos: [nombre, args...].
func (f hdFalso) llamadas(t *testing.T) [][]string {
	t.Helper()
	raw, err := os.ReadFile(f.log)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out [][]string
	for _, l := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		if strings.HasPrefix(l, "== ") {
			out = append(out, []string{strings.TrimPrefix(l, "== ")})
			continue
		}
		if len(out) > 0 {
			out[len(out)-1] = append(out[len(out)-1], l)
		}
	}
	return out
}

// hdCandidatas son las formas en que una persona pega una accion: entera, y
// desde el comando que nombra (la accion de borrar es una frase que termina
// en 'rm <ruta>').
func hdCandidatas(accion string) []string {
	out := []string{accion}
	for _, cmd := range []string{"rm ", "hoom "} {
		if i := strings.Index(accion, cmd); i > 0 {
			out = append(out, accion[i:])
		}
	}
	return out
}

// hdAccionesTexto saca las acciones que imprime 'hoom board doctor'.
func hdAccionesTexto(stdout string) []string {
	var out []string
	for _, l := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(l, "    Accion: ") {
			out = append(out, strings.TrimPrefix(l, "    Accion: "))
		}
	}
	return out
}

// hdAcciones junta las acciones del reporte y las del texto, sin repetir.
func hdAcciones(r DoctorReport, texto string) []string {
	visto := map[string]bool{}
	var out []string
	for _, a := range append(drAccionesDe(r.Problems), hdAccionesTexto(texto)...) {
		if !visto[a] {
			visto[a] = true
			out = append(out, a)
		}
	}
	return out
}

func drAccionesDe(ps []Problem) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Action)
	}
	return out
}

// hdBuscar devuelve las rutas (relativas a raiz) de los archivos llamados
// exactamente marca: lo que deja un 'touch <marca>' inyectado (los registros
// plantados llevan la marca dentro de un nombre mas largo y no cuentan).
func hdBuscar(t *testing.T, raiz, marca string) []string {
	t.Helper()
	var out []string
	_ = filepath.WalkDir(raiz, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Name() == marca {
			rel, _ := filepath.Rel(raiz, p)
			out = append(out, rel)
		}
		return nil
	})
	sort.Strings(out)
	return out
}

func hdFmtAcciones(as []string) string {
	var b strings.Builder
	for _, a := range as {
		fmt.Fprintf(&b, "  %q\n", a)
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// e5d1e4: un directorio que existe y no se puede leer no es "nada".
// ---------------------------------------------------------------------------

func hdNoRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("corre como root: un chmod no le quita la lectura, el test no probaria nada")
	}
}

// hdSinLectura deja dir con modo hasta el final del test (se restaura antes
// de que t.TempDir lo borre: los Cleanup corren al reves).
func hdSinLectura(t *testing.T, dir string, modo os.FileMode) {
	t.Helper()
	if err := os.Chmod(dir, modo); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if _, err := os.ReadDir(dir); err == nil {
		t.Skip("este sistema deja listar un directorio sin permiso de lectura")
	}
}

// hdEspacioSuelto: un espacio de trabajo de 'hoom task' sin item, con cambios
// y sin veredicto (sin-veredicto del proyecto).
func hdEspacioSuelto(t *testing.T, root string) {
	t.Helper()
	wt := bdWorktree(t, root, "suelto")
	bdEscribir(t, wt, "suelto.go", "package app\n")
}

// hdTarjetaConCambios: la tarjeta precios con su espacio de trabajo con
// cambios y sin veredicto (sin-veredicto de la tarjeta).
func hdTarjetaConCambios(t *testing.T, root string) {
	t.Helper()
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	wt := bdWorktree(t, root, bdSlug)
	bdEscribir(t, wt, "precios.go", "package app\n")
}

// CA-369 / CA-370 (hallazgo e5d1e4): si .hoom/worktrees o .hoom/items existe
// pero no se puede leer (sin ningun permiso, o atravesable sin lectura),
// Doctor devuelve error en vez de un reporte que calla lo que no vio, y por
// lo tanto 'hoom board doctor' (texto y --json) no dice que no hay problemas.
func TestCA369_e5d1e4_DirectorioIlegibleEsUnError(t *testing.T) {
	hdNoRoot(t)
	bin := bxBinario(t)
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	casos := []struct {
		nombre, dir string
		modo        os.FileMode
		armar       func(*testing.T, string)
	}{
		{".hoom/worktrees sin ningun permiso (000)", ".hoom/worktrees", 0o000, hdEspacioSuelto},
		{".hoom/worktrees atravesable pero sin lectura (300)", ".hoom/worktrees", 0o300, hdEspacioSuelto},
		{".hoom/items sin ningun permiso (000)", ".hoom/items", 0o000, hdTarjetaConCambios},
		{".hoom/items atravesable pero sin lectura (300)", ".hoom/items", 0o300, hdTarjetaConCambios},
	}
	for _, cs := range casos {
		t.Run(cs.nombre, func(t *testing.T) {
			root := bdRepo(t, "findings:\n  block_on: high\n")
			cs.armar(t, root)
			r, err := Doctor(root, "main", "high", now)
			if err != nil || len(r.Problems) == 0 {
				t.Fatalf("precondicion: legible, el fixture tiene un problema que el doctor ve: err=%v problemas=%v", err, drIDs(r.Problems))
			}

			hdSinLectura(t, filepath.Join(root, cs.dir), cs.modo)
			r, err = Doctor(root, "main", "high", now)
			if err == nil {
				t.Errorf("CA-369: %s existe y no se puede leer: Doctor devuelve error, no un reporte de %d problemas %v que calla lo que no pudo ver",
					cs.dir, len(r.Problems), drIDs(r.Problems))
			}

			txt := bxCorrer(t, bin, root, "board", "doctor")
			if strings.Contains(txt.stdout, "sin problemas de coherencia") {
				t.Errorf("CA-370: con %s ilegible 'hoom board doctor' no puede decir que no hay problemas (exit %d):\n%s%s",
					cs.dir, txt.exit, txt.stdout, txt.stderr)
			}
			js := bxCorrer(t, bin, root, "board", "doctor", "--json")
			var rep struct {
				Problems []json.RawMessage `json:"problems"`
			}
			if js.exit == 0 && json.Unmarshal([]byte(js.stdout), &rep) == nil && rep.Problems != nil && len(rep.Problems) == 0 {
				t.Errorf("CA-370: con %s ilegible 'hoom board doctor --json' no puede salir 0 con {\"problems\": []}:\n%s%s",
					cs.dir, js.stdout, js.stderr)
			}
		})
	}
}

// CA-369 / CA-370 (hallazgo e5d1e4, guarda): que .hoom/worktrees y
// .hoom/items no existan (o existan vacios y legibles) sigue siendo "nada":
// sin error y sin problemas.
func TestCA369_e5d1e4_Guarda_DirectorioQueNoExisteEsNada(t *testing.T) {
	bin := bxBinario(t)
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	root := bdRepo(t, "")
	for _, d := range []string{".hoom/worktrees", ".hoom/items"} {
		if _, err := os.Stat(filepath.Join(root, d)); err == nil {
			t.Fatalf("precondicion: el repo del fixture no tiene %s", d)
		}
	}
	mirar := func(cuando string) {
		t.Helper()
		r, err := Doctor(root, "main", "high", now)
		if err != nil || len(r.Problems) != 0 {
			t.Fatalf("CA-369: [%s] es nada: sin error y sin problemas, fue err=%v problemas=%v", cuando, err, drIDs(r.Problems))
		}
		txt := bxCorrer(t, bin, root, "board", "doctor")
		if txt.exit != 0 || txt.stdout != "hoom board doctor: sin problemas de coherencia\n" {
			t.Fatalf("CA-370: [%s] 'hoom board doctor' sale 0 sin problemas, fue exit %d:\n%s%s", cuando, txt.exit, txt.stdout, txt.stderr)
		}
	}
	mirar("sin .hoom/worktrees ni .hoom/items")
	for _, d := range []string{".hoom/worktrees", ".hoom/items"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mirar("con .hoom/worktrees y .hoom/items vacios y legibles")
}

// ---------------------------------------------------------------------------
// f34bc3: sobres plantados y valores de telemetria hostiles.
// ---------------------------------------------------------------------------

const (
	hdNormal    = "20260923T090000_norm01" // la forma de los ids de los tests del doctor
	hdCerrado   = "20260923T090000_otro01" // un sobre cerrado legitimo: nadie propone borrarlo
	hdPlantado1 = "20260923T090000_arch01" // archivo cuyo id es el de hdCerrado
)

// hdMarcasPlantadas no pueden aparecer en ninguna accion del doctor.
var hdMarcasPlantadas = []string{"PWNED", "VICTIMA", "otro01", "arch01", "inye01", "vict0"}

type hdPlantados struct {
	base, root string
	normales   []string // los sobres huerfanos de hoom, que el doctor si lista
	victimas   []string // archivos fuera de .hoom/envelopes que nadie borra
	now        time.Time
}

// hdSobresPlantados arma un proyecto anidado en base/a/b/proyecto (asi
// '../../../VICTIMA' cae dentro del directorio del test) con:
//   - dos sobres huerfanos normales, sin tarea: hdNormal y uno de NewID;
//   - un sobre cerrado legitimo hdCerrado (no es huerfano);
//   - registros plantados a mano, abiertos y de un PID muerto, cuyo id no es
//     el nombre de su archivo o no tiene forma de id.
//
// Todos los abiertos empiezan y se actualizan a la vez: ninguno releva a otro.
func hdSobresPlantados(t *testing.T) hdPlantados {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "a", "b", "proyecto")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	bdGit(t, root, "init", "-b", "main")
	bdGit(t, root, "config", "user.email", "test@hoom.dev")
	bdGit(t, root, "config", "user.name", "hoom test")
	bdEscribir(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n")
	bdEscribir(t, root, ".hoom/.gitignore", hoomfs.GitignoreBody())
	bdEscribir(t, root, "app.go", "package app\n")
	bdGit(t, root, "add", "-A")
	bdGit(t, root, "commit", "-m", "inicial")

	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	m := func(min int) time.Time { return now.Add(time.Duration(-min) * time.Minute) }
	muerto := bdPIDMuerto(t)
	abierto := func(id string) envelope.Record {
		return envelope.Record{ID: id, Role: "writer", Provider: "claude", Dir: root, Stage: "run", Step: 3, Steps: 5,
			Status: envelope.StatusRunning, PID: muerto, StartedAt: m(60), UpdatedAt: m(10)}
	}
	plantar := func(archivo string, rec envelope.Record) {
		raw, err := json.MarshalIndent(rec, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		bdEscribir(t, root, filepath.Join(".hoom", envelope.DirName, archivo), string(raw)+"\n")
	}

	nuevo := envelope.NewID()
	for _, id := range []string{hdNormal, nuevo} {
		plantar(id+".json", abierto(id))
	}
	cerrado := abierto(hdCerrado)
	cerrado.Status, cerrado.Stage, cerrado.Step = envelope.StatusDeliverable, "ok", 5
	cerrado.StartedAt, cerrado.UpdatedAt, cerrado.EndedAt = m(120), m(110), m(110)
	plantar(hdCerrado+".json", cerrado)

	// sin forma de id, con su propio nombre de archivo
	for _, id := range []string{"x; touch PWNED; #", "$(touch PWNED)", "`touch PWNED`", "a b"} {
		plantar(id+".json", abierto(id))
	}
	// sin forma de id, con otro nombre de archivo
	plantar("20260923T090000_inye01.json", abierto("x; touch PWNED; #"))
	plantar("20260923T090000_vict01.json", abierto("../../../VICTIMA"))
	plantar("20260923T090000_vict02.json", abierto("../../VICTIMA"))
	// con forma de id, pero no la de su archivo: apunta al sobre cerrado
	plantar(hdPlantado1+".json", abierto(hdCerrado))

	// lo que un rm de esas rutas borraria fuera de .hoom/envelopes
	victimas := []string{
		filepath.Join(root, "VICTIMA.json"),           // .hoom/envelopes/../../VICTIMA.json
		filepath.Join(base, "a", "b", "VICTIMA.json"), // .hoom/envelopes/../../../VICTIMA.json
		filepath.Join(root, "b.json"),                 // el segundo argumento de 'rm .hoom/envelopes/a b.json'
	}
	for _, v := range victimas {
		if err := os.WriteFile(v, []byte("no me borres\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return hdPlantados{base: base, root: root, normales: []string{hdNormal, nuevo}, victimas: victimas, now: now}
}

// CA-369 (hallazgo f34bc3): un registro de sobre plantado a mano en
// .hoom/envelopes cuyo id no es el nombre de su archivo, o no tiene forma de
// id, no aparece en envelope.List ni en ninguna accion del doctor; y pegar en
// sh -c cualquier accion que el doctor devuelve o imprime no crea PWNED, no
// borra nada fuera de .hoom/envelopes ni el sobre cerrado legitimo.
func TestCA369_f34bc3_SobresPlantadosNoLleganAUnaAccion(t *testing.T) {
	p := hdSobresPlantados(t)

	// envelope.List: solo los de hoom (los normales y el cerrado, uno cada uno)
	porID := map[string][]envelope.Record{}
	for _, r := range envelope.List(p.root) {
		porID[r.ID] = append(porID[r.ID], r)
	}
	ids := []string{}
	for id := range porID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	want := append([]string{hdCerrado}, p.normales...)
	sort.Strings(want)
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("CA-369: envelope.List devuelve solo los registros cuyo id es el nombre de su archivo y tiene forma de id\nquiere: %q\nfue:    %q", want, ids)
	}
	if rs := porID[hdCerrado]; len(rs) != 1 || rs[0].Status != envelope.StatusDeliverable {
		t.Errorf("CA-369: %s es el sobre cerrado de su archivo, una vez; el plantado en %s.json no lo suplanta: %+v", hdCerrado, hdPlantado1, rs)
	}

	r, err := Doctor(p.root, "main", "high", p.now)
	if err != nil {
		t.Fatalf("CA-369: Doctor sobre el proyecto: %v", err)
	}
	for _, pr := range r.Problems {
		for _, marca := range hdMarcasPlantadas {
			if strings.Contains(pr.Action, marca) {
				t.Errorf("CA-369: un registro plantado llego a una accion del doctor (%s, contiene %q): %q", pr.ID, marca, pr.Action)
			}
		}
	}

	bin := bxBinario(t)
	txt := bxCorrer(t, bin, p.root, "board", "doctor")
	acciones := hdAcciones(r, txt.stdout)
	falsos := hdFalsos(t)
	for _, a := range acciones {
		for _, l := range hdCandidatas(a) {
			falsos.pegar(t, p.root, l)
		}
	}
	if got := hdBuscar(t, p.base, "PWNED"); len(got) > 0 {
		t.Errorf("CA-369: pegar en sh -c las acciones del doctor ejecuto un comando ajeno: aparecio %v\nacciones:\n%s", got, hdFmtAcciones(acciones))
	}
	for _, v := range p.victimas {
		if raw, err := os.ReadFile(v); err != nil || string(raw) != "no me borres\n" {
			t.Errorf("CA-369: pegar las acciones del doctor borro %s, fuera de .hoom/envelopes (%v)", v, err)
		}
	}
	sobres := filepath.Join(p.root, ".hoom", envelope.DirName) + string(filepath.Separator)
	for _, ll := range falsos.llamadas(t) {
		if ll[0] != "rm" {
			continue
		}
		for _, a := range ll[1:] {
			if strings.HasPrefix(a, "-") {
				continue
			}
			ruta := a
			if !filepath.IsAbs(ruta) {
				ruta = filepath.Join(p.root, ruta)
			}
			ruta = filepath.Clean(ruta)
			if !strings.HasPrefix(ruta, sobres) {
				t.Errorf("CA-369: una accion del doctor manda a borrar %q, que queda fuera de .hoom/envelopes (%s)", a, ruta)
			} else if filepath.Base(ruta) == hdCerrado+".json" {
				t.Errorf("CA-369: una accion del doctor manda a borrar el sobre cerrado legitimo %q", a)
			}
		}
	}
}

// CA-369 (hallazgo f34bc3, guarda): entre los plantados, cada sobre huerfano
// de hoom (id de hoom, rol de hoom, sin tarea) sigue apareciendo con su
// accion del contrato, y pegarla borra exactamente su registro.
func TestCA369_f34bc3_Guarda_SobreNormalEntreLosPlantados(t *testing.T) {
	p := hdSobresPlantados(t)
	r, err := Doctor(p.root, "main", "high", p.now)
	if err != nil {
		t.Fatalf("CA-369: Doctor sobre el proyecto: %v", err)
	}
	for _, id := range p.normales {
		var got *Problem
		for i := range r.Problems {
			if pr := r.Problems[i]; pr.ID == ProbSobreHuerfano && pr.What == drWhatHuerfanoProyecto(id, "writer", "run") {
				got = &r.Problems[i]
			}
		}
		if got == nil {
			t.Fatalf("CA-369: el sobre huerfano %s sigue apareciendo:\n%s", id, drFmt(r.Problems))
		}
		if got.Slug != "" || got.Plain != "" || got.Action != drAccionBorrar(id) {
			t.Fatalf("CA-369: el sobre huerfano %s trae la accion del contrato %q: %+v", id, drAccionBorrar(id), *got)
		}
		drRefCon(t, "CA-369", "sobre normal entre plantados", *got, id)

		falsos := hdFalsos(t)
		for _, l := range hdCandidatas(got.Action) {
			falsos.pegar(t, p.root, l)
		}
		ll := falsos.llamadas(t)
		if len(ll) != 1 || !reflect.DeepEqual(ll[0], []string{"rm", ".hoom/envelopes/" + id + ".json"}) {
			t.Fatalf("CA-369: pegar la accion de %s llama a rm con su registro y nada mas, fue %q", id, ll)
		}
	}
}

// hdSesionesHostiles son ids de sesion (lo que se reanuda) con
// metacaracteres de shell; todos crean PWNED si sh los interpreta.
var hdSesionesHostiles = []string{
	"x; touch PWNED; #",
	"$(touch PWNED)",
	"`touch PWNED`",
	"a && touch PWNED",
	"a | touch PWNED",
	"a\ntouch PWNED",
	"'; touch PWNED; '",
	"\"; touch PWNED; \"",
}

// hdRolesHostiles no son roles de hoom.
var hdRolesHostiles = []string{
	"writer; touch PWNED; #",
	"$(touch PWNED)",
	"writer' ; touch PWNED ; '",
}

// CA-367 / CA-369 (hallazgo f34bc3): un sobre interrumpido de una tarjeta
// cuyo session id tiene metacaracteres de shell, cuyo rol no es un rol de
// hoom, o cuyo provider no es un provider, no produce una accion con ese
// valor sin escapar: pegar en sh -c cada accion del doctor de la tarjeta no
// crea PWNED (el doctor la escapa o no propone ese comando).
func TestCA367_f34bc3_SobreDeTarjetaConValoresHostiles(t *testing.T) {
	var casos []acFix
	for _, s := range hdSesionesHostiles {
		for _, prov := range []string{"claude", "codex"} {
			ev := drInterrumpido(drWriterQuieto(), "writer", prov)
			ev.Runs[len(ev.Runs)-1].Meta.ProviderSessionID = s
			casos = append(casos, acFix{fmt.Sprintf("sesion %q de %s", s, prov), ev})
		}
	}
	for _, rol := range hdRolesHostiles {
		casos = append(casos, acFix{fmt.Sprintf("rol %q", rol), drInterrumpido(drWriterQuieto(), rol, "claude")})
	}
	casos = append(casos, acFix{"provider hostil", drInterrumpido(drWriterQuieto(), "writer", "claude; touch PWNED; #")})

	falsos := hdFalsos(t)
	for _, f := range casos {
		c := Derive(f.ev)
		if c.Doctor == nil {
			t.Errorf("CA-367: [%s] doctor es una lista, nunca nil", f.nombre)
			continue
		}
		dir := t.TempDir()
		acciones := drAccionesDe(c.Doctor)
		for _, a := range acciones {
			for _, l := range hdCandidatas(a) {
				falsos.pegar(t, dir, l)
			}
		}
		if got := hdBuscar(t, dir, "PWNED"); len(got) > 0 {
			t.Errorf("CA-367: [%s] pegar en sh -c la accion del doctor ejecuto un comando ajeno (aparecio %v):\n%s",
				f.nombre, got, hdFmtAcciones(acciones))
		}
	}
}

// CA-367 (hallazgo f34bc3, guarda): un sobre interrumpido normal (rol de hoom,
// provider de hoom, sesion de hoom o UUID) sigue dando su accion de reanudar
// exacta, y pegarla llama a hoom con esos argumentos, uno por uno.
func TestCA367_f34bc3_Guarda_SobreDeTarjetaNormal(t *testing.T) {
	for _, cs := range []struct{ provider, sesion string }{
		{"claude", "sesion-abc123"},
		{"claude", "3f2b8c1e-7d4a-4b9e-9c2f-1a2b3c4d5e6f"},
		{"codex", "0199c3a4-5b6d-7e8f-9a0b-1c2d3e4f5a6b"},
	} {
		nombre := cs.provider + " con sesion " + cs.sesion
		ev := drInterrumpido(drWriterQuieto(), "writer", cs.provider)
		ev.Runs[len(ev.Runs)-1].Meta.ProviderSessionID = cs.sesion
		_, ps := drDoctor(t, "CA-367", nombre, ev)
		accion := "hoom agent --role writer --task precios --spec .hoom/specs/precios.md --provider " + cs.provider +
			" --resume " + cs.sesion + ` "<pedido>"`
		drMismos(t, "CA-367", nombre, ps, []drProb{{ProbSobreHuerfano, bdSlug,
			drWhatHuerfano("env-caido", "writer", 3, 5, "run"), drPlainHuerfano("writer"), accion}})

		falsos := hdFalsos(t)
		dir := t.TempDir()
		falsos.pegar(t, dir, ps[0].Action)
		want := []string{"hoom", "agent", "--role", "writer", "--task", "precios", "--spec", ".hoom/specs/precios.md",
			"--provider", cs.provider, "--resume", cs.sesion, "<pedido>"}
		if ll := falsos.llamadas(t); len(ll) != 1 || !reflect.DeepEqual(ll[0], want) {
			t.Fatalf("CA-367: [%s] pegar la accion llama a hoom con %q, fue %q", nombre, want, ll)
		}
		if got := hdBuscar(t, dir, "PWNED"); len(got) > 0 {
			t.Fatalf("CA-367: [%s] la accion normal no crea nada: %v", nombre, got)
		}
	}
}

// ---------------------------------------------------------------------------
// 5435b7: la tarea de un hallazgo (o de un veredicto) escrita a mano.
// ---------------------------------------------------------------------------

var hdTareasHostiles = []string{"x;touch PWNED_TASK;#", "$(touch PWNED_TASK)"}

// hdHallazgoConTarea registra un hallazgo valido y despues reescribe a mano
// su "task" en el JSON, como llegaria por Git sin pasar por Register.
func hdHallazgoConTarea(t *testing.T, root, desc, tarea string) {
	t.Helper()
	f, err := finding.Register(root, "main", finding.Draft{Severity: "medium", Lens: "risk", Description: desc,
		Author: "reviewer", Task: "temporal"})
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, ".hoom", "findings", f.ID+".json")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("precondicion: el hallazgo vive en %s: %v", p, err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["task"] != "temporal" {
		t.Fatalf("precondicion: el JSON del hallazgo trae \"task\": %s", raw)
	}
	m["task"] = tarea
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, append(out, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

// hdSinInyeccion exige que ningun evidencia-sin-item lleve en su accion una
// tarea hostil y que pegar en sh -c cada accion (del reporte y del texto) no
// cree un archivo con marca.
func hdSinInyeccion(t *testing.T, root string, hostiles []string, marca string) {
	t.Helper()
	r, err := Doctor(root, "main", "high", time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("CA-369: Doctor sobre el proyecto: %v", err)
	}
	for _, p := range r.Problems {
		for _, h := range hostiles {
			if p.ID == ProbEvidenciaSinItem && strings.Contains(p.Action, h) {
				t.Errorf("CA-369: evidencia-sin-item lleva la tarea hostil %q en su accion: %q", h, p.Action)
			}
		}
	}
	txt := bxCorrer(t, bxBinario(t), root, "board", "doctor")
	acciones := hdAcciones(r, txt.stdout)
	falsos := hdFalsos(t)
	for _, a := range acciones {
		for _, l := range hdCandidatas(a) {
			falsos.pegar(t, root, l)
		}
	}
	if got := hdBuscar(t, root, marca); len(got) > 0 {
		t.Errorf("CA-369: pegar en sh -c las acciones del doctor ejecuto un comando ajeno: aparecio %v\nacciones:\n%s", got, hdFmtAcciones(acciones))
	}
}

// CA-369 (hallazgo 5435b7): un hallazgo escrito a mano en .hoom/findings con
// una "task" que tiene metacaracteres no produce un evidencia-sin-item cuya
// accion contenga ese valor, y pegar las acciones del doctor en sh -c no crea
// PWNED_TASK.
func TestCA369_5435b7_TareaDeHallazgoConMetacaracteres(t *testing.T) {
	root := bdRepo(t, "findings:\n  block_on: high\n")
	for i, h := range hdTareasHostiles {
		hdHallazgoConTarea(t, root, fmt.Sprintf("hallazgo hostil %d", i), h)
	}
	drHallazgoDisco(t, root, "valida", "hallazgo de una tarea sin item")
	hdSinInyeccion(t, root, hdTareasHostiles, "PWNED_TASK")
}

// CA-369 (hallazgo 5435b7, el mismo camino por el otro origen de la tarea):
// un veredicto cuyo spec es .hoom/specs/<t>.md con metacaracteres en <t> no
// produce un evidencia-sin-item cuya accion contenga <t>, y pegar las
// acciones del doctor en sh -c no crea PWNED_VER.
func TestCA369_5435b7_TareaDeVeredictoConMetacaracteres(t *testing.T) {
	root := bdRepo(t, "findings:\n  block_on: high\n")
	hostiles := []string{"x;touch PWNED_VER;#", "$(touch PWNED_VER)"}
	for i, h := range hostiles {
		bdVeredictoDisco(t, root, ".hoom/specs/"+h+".md", bdT0.Add(time.Duration(i)*time.Minute), false, bdGatesVerdes())
	}
	// precondicion: los veredictos de una tarea sin item si llegan al doctor
	bdVeredictoDisco(t, root, ".hoom/specs/otra.md", bdT0.Add(time.Hour), false, bdGatesVerdes())
	r, err := Doctor(root, "main", "high", time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("CA-369: Doctor sobre el proyecto: %v", err)
	}
	visto := false
	for _, p := range r.Problems {
		visto = visto || (p.ID == ProbEvidenciaSinItem && p.Slug == "otra" && p.Action == drAccionItem("otra"))
	}
	if !visto {
		t.Fatalf("precondicion: el veredicto de la tarea otra sin item da su evidencia-sin-item:\n%s", drFmt(r.Problems))
	}
	hdSinInyeccion(t, root, hostiles, "PWNED_VER")
}

// CA-369 (hallazgo 5435b7, guarda): un hallazgo cuya tarea es un slug valido
// sin item sigue dando su evidencia-sin-item con la accion del contrato,
// aunque al lado haya uno hostil, y pegarla llama a hoom con ese slug.
func TestCA369_5435b7_Guarda_TareaValidaSinItem(t *testing.T) {
	root := bdRepo(t, "findings:\n  block_on: high\n")
	hdHallazgoConTarea(t, root, "hallazgo hostil", hdTareasHostiles[0])
	drHallazgoDisco(t, root, "valida", "hallazgo de una tarea sin item")
	r, err := Doctor(root, "main", "high", time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("CA-369: Doctor sobre el proyecto: %v", err)
	}
	var got *Problem
	for i := range r.Problems {
		if r.Problems[i].ID == ProbEvidenciaSinItem && r.Problems[i].Slug == "valida" {
			got = &r.Problems[i]
		}
	}
	if got == nil || got.What != "hay 1 hallazgo de la tarea valida y ningun item la representa" || got.Plain != "" ||
		got.Action != drAccionItem("valida") {
		t.Fatalf("CA-369: la tarea valida sin item sigue dando su evidencia-sin-item del contrato:\n%s", drFmt(r.Problems))
	}
	falsos := hdFalsos(t)
	falsos.pegar(t, root, got.Action)
	if ll := falsos.llamadas(t); len(ll) != 1 || !reflect.DeepEqual(ll[0], []string{"hoom", "item", "add", "<titulo>", "--slug", "valida"}) {
		t.Fatalf("CA-369: pegar la accion llama a hoom item add con el slug, fue %q", ll)
	}
}
