// Tests adversariales de la ENMIENDA 1 de .hoom/specs/evidencia-en-disco.md
// y de los hallazgos de su review (bbe304/72b57f/a4f360, 64f17b/86842f,
// e87f8b, 7a2c12):
//
//   - CA-440: la huella es el sha256 del contenido ENTERO, tambien de un
//     archivo de mas de 16 MiB.
//   - CA-442: un symlink o un FIFO creado con un nombre valido es
//     manipulacion ("la evidencia es un archivo regular"), y nunca se sigue
//     ni se lee. Y una ruta que ya es manipulacion no se marca dos veces: la
//     resolucion sin forma de un rol que no es refutador es UNA manipulacion.
//   - CA-444: una evidencia que hoom no puede leer (un archivo o un
//     directorio sin permiso de lectura), en la foto de antes o en la de
//     despues, es manipulacion ("la evidencia no se puede leer").
//
// Usa los helpers ed* de evidencia_en_disco_test.go y los qc*/qcr* de
// quien-cierra-un-hallazgo. Las fotos que pueden colgarse (un FIFO, un
// symlink a un FIFO) corren con reloj.
package agentcmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/finding"
)

// Frases del contrato de la enmienda 1.
const (
	edr1Regular  = "la evidencia es un archivo regular"
	edr1Ilegible = "la evidencia no se puede leer"
)

// edr1Reloj es cuanto puede tardar una foto o un gate: mas es que siguio o
// leyo una entrada que no es un archivo regular.
const edr1Reloj = 30 * time.Second

const edr1MiB = 1 << 20

func edr1Rol(t *testing.T, slug string) agents.Role {
	t.Helper()
	r, err := agents.Lookup(slug)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// edr1Take es Take con reloj.
func edr1Take(t *testing.T, ca, caso, root string) Snapshot {
	t.Helper()
	ch := make(chan Snapshot, 1)
	go func() { ch <- Take(root, "main") }()
	select {
	case s := <-ch:
		return s
	case <-time.After(edr1Reloj):
		t.Fatalf("%s: %s: Take no volvio en %s: la foto siguio o leyo una entrada que no es un archivo regular", ca, caso, edr1Reloj)
	}
	return Snapshot{}
}

// edr1Gate es Gate con reloj.
func edr1Gate(t *testing.T, ca, caso, root string, rol agents.Role, before, after Snapshot, pol Policy) ScopeResult {
	t.Helper()
	ch := make(chan ScopeResult, 1)
	go func() { ch <- Gate(root, "main", "", rol, before, after, pol, nil) }()
	select {
	case res := <-ch:
		return res
	case <-time.After(edr1Reloj):
		t.Fatalf("%s: %s: el gate del rol %s no volvio en %s: siguio o leyo una entrada que no es un archivo regular", ca, caso, rol.Slug, edr1Reloj)
	}
	return ScopeResult{}
}

// edr1Soltar registra, al final del test, abrir para escribir (sin
// bloquear) cada FIFO: suelta una lectura que se haya quedado colgada.
func edr1Soltar(t *testing.T, fifos ...string) {
	t.Helper()
	t.Cleanup(func() {
		for _, p := range fifos {
			for i := 0; i < 16; i++ {
				f, err := os.OpenFile(p, os.O_WRONLY|syscall.O_NONBLOCK, 0)
				if err != nil {
					break
				}
				f.Close()
				time.Sleep(10 * time.Millisecond)
			}
		}
	})
}

// edr1Fifo crea un FIFO en p.
func edr1Fifo(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(p, 0o644); err != nil {
		t.Fatalf("fixture: mkfifo %s: %v", p, err)
	}
	edr1Soltar(t, p)
}

// edr1Symlink crea en root/rel un symlink a destino.
func edr1Symlink(t *testing.T, root, rel, destino string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(destino, p); err != nil {
		t.Fatalf("fixture: symlink %s -> %s: %v", rel, destino, err)
	}
}

// edr1NoRoot omite los tests de permisos como root: chmod 000 no le quita
// la lectura.
func edr1NoRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("CA-444: como root chmod 000 no quita la lectura")
	}
}

// edr1Restaurar devuelve el permiso de p al final del test (antes de que se
// borre su directorio temporal).
func edr1Restaurar(t *testing.T, p string, modo os.FileMode) {
	t.Helper()
	t.Cleanup(func() { _ = os.Chmod(p, modo) })
}

// edr1SinLectura le quita a root/rel todo permiso y exige que de verdad no
// se pueda leer. Al final del test se lo devuelve.
func edr1SinLectura(t *testing.T, root, rel string) {
	t.Helper()
	p := filepath.Join(root, rel)
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	modo := os.FileMode(0o644)
	if fi.IsDir() {
		modo = 0o755
	}
	edr1Restaurar(t, p, modo)
	if err := os.Chmod(p, 0); err != nil {
		t.Fatal(err)
	}
	if fi.IsDir() {
		if _, err := os.ReadDir(p); err == nil {
			t.Skipf("CA-444: el sistema de archivos deja leer %s con modo 000", rel)
		}
	} else if _, err := os.ReadFile(p); err == nil {
		t.Skipf("CA-444: el sistema de archivos deja leer %s con modo 000", rel)
	}
}

// edr1ConLectura le devuelve el permiso a root/rel.
func edr1ConLectura(t *testing.T, root, rel string, modo os.FileMode) {
	t.Helper()
	if err := os.Chmod(filepath.Join(root, rel), modo); err != nil {
		t.Fatal(err)
	}
}

// edr1Una exige UNA manipulacion en ruta cuyo detalle dice todas las frases
// de alguno de los juegos (sin juegos: cualquier detalle), con el gate
// marcado. La devuelve.
func edr1Una(t *testing.T, ca, caso string, res ScopeResult, ruta string, juegos ...[]string) Violation {
	t.Helper()
	ms := edManipulaciones(res.Violations, ruta)
	if len(ms) != 1 {
		t.Fatalf("%s: %s: %s es UNA violacion de manipulacion (hay %d): %s", ca, caso, ruta, len(ms), edLista(res.Violations))
	}
	if len(juegos) > 0 {
		alguno := false
		for _, j := range juegos {
			todas := true
			for _, f := range j {
				if !strings.Contains(ms[0].Detail, f) {
					todas = false
				}
			}
			alguno = alguno || todas
		}
		if !alguno {
			t.Fatalf("%s: %s: el detalle de la manipulacion en %s dice alguno de %q: %q", ca, caso, ruta, juegos, ms[0].Detail)
		}
	}
	if !res.Tampering || res.OK {
		t.Fatalf("%s: %s: el gate queda marcado como manipulacion: %+v", ca, caso, res)
	}
	return ms[0]
}

// edr1BuscarIlegible busca una manipulacion "la evidencia no se puede leer"
// en dir o debajo, con el gate marcado.
func edr1BuscarIlegible(res ScopeResult, dir string) (Violation, bool) {
	if !res.Tampering || res.OK {
		return Violation{}, false
	}
	for _, v := range res.Violations {
		if v.Rule != RuleTampering || !strings.Contains(v.Detail, edr1Ilegible) {
			continue
		}
		if v.Path == dir || strings.HasPrefix(v.Path, dir+"/") {
			return v, true
		}
	}
	return Violation{}, false
}

// edr1ExigirIlegible exige una manipulacion "la evidencia no se puede leer"
// en dir o debajo, con el gate marcado. La devuelve.
func edr1ExigirIlegible(t *testing.T, ca, caso string, res ScopeResult, dir string) Violation {
	t.Helper()
	v, ok := edr1BuscarIlegible(res, dir)
	if !ok {
		t.Fatalf("%s: %s: una evidencia ilegible en %s es manipulacion con %q: %s (ok=%v, tampering=%v)",
			ca, caso, dir, edr1Ilegible, edLista(res.Violations), res.OK, res.Tampering)
	}
	return v
}

// edr1HallazgoDelGate exige que el hallazgo del gate gid este en el disco,
// sea high y no tenga resolucion. Lo lee directo, sin finding.List: con
// evidencia ilegible al lado, la lista no es lo que se mide aca.
func edr1HallazgoDelGate(t *testing.T, ca, caso, root, gid string) {
	t.Helper()
	if gid == "" {
		t.Fatalf("%s: %s: la manipulacion registra su hallazgo high del gate (finding_id vacio)", ca, caso)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".hoom", "findings", gid+".json"))
	if err != nil {
		t.Fatalf("%s: %s: el hallazgo del gate %s quedo en .hoom/findings/: %v", ca, caso, gid, err)
	}
	var f finding.Finding
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("%s: %s: el hallazgo del gate %s es JSON: %v", ca, caso, gid, err)
	}
	if f.Severity != "high" {
		t.Fatalf("%s: %s: el hallazgo del gate %s es high: %s", ca, caso, gid, f.Severity)
	}
	if existeEn(t, filepath.Join(root, ".hoom", "findings", gid+".res.json")) {
		t.Fatalf("%s: %s: el hallazgo del gate %s queda abierto", ca, caso, gid)
	}
}

// edr1HallazgoJSON es un hallazgo valido (a mano) con el id y la severidad.
func edr1HallazgoJSON(id, sev string) []byte {
	return []byte(`{"id":"` + id + `","created_at":"2026-10-01T00:00:00Z","severity":"` + sev + `","lens":"risk",` +
		`"file":"app.go","description":"el retry no respeta el backoff","author":"reviewer@codex"}` + "\n")
}

// edr1HallazgoGrande es un hallazgo JSON valido cuya descripcion mide
// relleno bytes y cuya severidad va DESPUES de ella. high y low dan el mismo
// tamano (low lleva un espacio, que el JSON permite).
func edr1HallazgoGrande(id, sev string, relleno int) []byte {
	var b bytes.Buffer
	b.WriteString(`{"id":"` + id + `","created_at":"2026-10-01T00:00:00Z","lens":"risk","file":"app.go","description":"`)
	b.Write(bytes.Repeat([]byte("x"), relleno))
	b.WriteString(`","author":"reviewer@codex","fingerprint":"0000000000000000","severity":"` + sev + `"`)
	b.WriteString(strings.Repeat(" ", 4-len(sev)))
	b.WriteString("}\n")
	return b.Bytes()
}

// edr1PrimerDistinto es el primer byte en que a y b difieren (-1 si son
// iguales).
func edr1PrimerDistinto(a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	if len(a) != len(b) {
		return n
	}
	return -1
}

// edr1HallazgosEn devuelve los hallazgos (<id>.json, no resoluciones) que
// hay en .hoom/findings/ de root.
func edr1HallazgosEn(t *testing.T, root string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, n := range filesIn(t, filepath.Join(root, ".hoom", "findings")) {
		if edFormaHallazgo.MatchString(".hoom/findings/"+n) && !strings.HasSuffix(n, ".res.json") {
			out[n] = true
		}
	}
	return out
}

// ================================================================ CA-440: el contenido entero

// CA-440: la huella es el sha256 del contenido ENTERO. Alrededor del borde
// de 16 MiB (uno menos, justo, uno y dos mas, y 17 MiB y pico), en un
// hallazgo que git ignora, dos contenidos del mismo tamano que solo difieren
// en el ULTIMO byte dan, cada uno, el sha256 de todos sus bytes. Y para el
// writer y el refutador el cambio es UNA manipulacion "cambio durante el
// run".
func TestCA440_LaHuellaEsElSHA256DelContenidoEnteroAlrededorDe16MiB(t *testing.T) {
	qcLimpiarEntorno(t)
	root := repo(t)
	qcr1Anexar(t, qcr1Exclude(t, root), ".hoom/findings/")
	tamanos := []int{16*edr1MiB - 1, 16 * edr1MiB, 16*edr1MiB + 1, 16*edr1MiB + 2, 17*edr1MiB + 3}
	contenido := func(n int, ultimo byte) []byte {
		b := bytes.Repeat([]byte("0123456789abcdef"), n/16+1)[:n]
		b[n-1] = ultimo
		return b
	}
	rutas := make([]string, len(tamanos))
	antes := make([]string, len(tamanos))
	despues := make([]string, len(tamanos))
	for i, n := range tamanos {
		rutas[i] = fmt.Sprintf(".hoom/findings/20261001T0000%02d_abcdef.json", i)
		a := contenido(n, 'a')
		antes[i] = edSHA(a)
		write(t, root, rutas[i], string(a))
		if !qcr1Ignorado(t, root, rutas[i]) {
			t.Fatalf("CA-440: fixture: git ignora %s", rutas[i])
		}
	}
	before := Take(root, "main")
	for i, n := range tamanos {
		b := contenido(n, 'b')
		despues[i] = edSHA(b)
		write(t, root, rutas[i], string(b))
	}
	after := Take(root, "main")

	for i, n := range tamanos {
		hb, ha := strings.ToLower(before.Huellas[rutas[i]]), strings.ToLower(after.Huellas[rutas[i]])
		if hb != antes[i] || ha != despues[i] {
			t.Fatalf("CA-440: un archivo de %d bytes (16 MiB %+d): la huella es el sha256 de su contenido entero:\nantes   %q, se esperaba %q\ndespues %q, se esperaba %q",
				n, n-16*edr1MiB, hb, antes[i], ha, despues[i])
		}
	}
	for _, slug := range []string{"writer", finding.RolQueRefuta} {
		rol := edr1Rol(t, slug)
		res := Gate(root, "main", "", rol, before, after, PolicyFor(nil, rol), nil)
		for i, r := range rutas {
			edExigir(t, "CA-440", fmt.Sprintf("rol %s, %d bytes cambiados en el ultimo", slug, tamanos[i]), res, r, edAppendOnly, r, edCambio)
		}
	}
}

// CA-440 (con CA-441) de punta a punta: un hallazgo high de mas de 16 MiB que
// git ignora, con la severidad DESPUES del relleno. 'hoom agent --role
// writer' implementa y lo reescribe con severidad low y el MISMO tamano (los
// primeros 17 MiB, identicos). El sobre no termina verde: UNA manipulacion
// "cambio durante el run", su hallazgo high, corte en scope; y la corrida
// siguiente de un writer legitimo queda roja en findings_open.
func TestCA440_ElWriterQueBajaUnHallazgoDeMasDe16MiBConElMismoTamanoNoTerminaVerde(t *testing.T) {
	const caso = "hallazgo de 17 MiB bajado a low con el mismo tamano"
	root := edRepo(t, true, false)
	qcr1Anexar(t, qcr1Exclude(t, root), ".hoom/findings/")
	id := "20261001T000000_b16b16"
	rel := ".hoom/findings/" + id + ".json"
	alto := edr1HallazgoGrande(id, "high", 17*edr1MiB)
	bajo := edr1HallazgoGrande(id, "low", 17*edr1MiB)
	if len(alto) != len(bajo) || len(alto) <= 16*edr1MiB+1 {
		t.Fatalf("CA-440: fixture: mismo tamano y mas de 16 MiB: %d / %d", len(alto), len(bajo))
	}
	if d := edr1PrimerDistinto(alto, bajo); d <= 17*edr1MiB {
		t.Fatalf("CA-440: fixture: los dos contenidos son identicos en sus primeros 17 MiB (difieren en %d)", d)
	}
	write(t, root, rel, string(alto))
	if !qcr1Ignorado(t, root, rel) {
		t.Fatalf("CA-440: fixture: git ignora %s", rel)
	}
	items, _, err := finding.List(root, "main", true)
	if err != nil {
		t.Fatal(err)
	}
	visto := false
	for _, it := range items {
		if it.ID == id {
			visto = it.Severity == "high" && it.Status == finding.StatusOpen
		}
	}
	if !visto {
		t.Fatalf("CA-440: fixture: finding.List ve %s como un hallazgo high abierto", id)
	}

	fakeProvider(t, "claude", edImplementar+edCopiar(edPreparado(t, bajo), rel)+"exit 0\n")
	res := edCorrer(t, "CA-440", caso, root, Options{Role: "writer", Prompt: "implementa"})
	if !bytes.Equal(edLeer(t, root, rel), bajo) {
		t.Fatalf("CA-440: fixture: el writer falso dejo %s en low con el mismo tamano", rel)
	}
	if res.Status == "entregable" || res.Verdict == "green" || res.ExitCode == 0 {
		t.Errorf("CA-440: %s: el sobre no termina verde: status=%s verdict=%s exit=%d stage=%s scope=%s",
			caso, res.Status, res.Verdict, res.ExitCode, res.Stage, edLista(res.Scope.Violations))
	}
	v := edExigir(t, "CA-440", caso, res.Scope, rel, edAppendOnly, rel, edCambio)
	gid := edHallazgoDelGate(t, "CA-440", caso, root, v)
	if res.Stage != "scope" || res.ExitCode != 1 || res.VerdictID != "" || res.Status != "no-entregable" {
		t.Fatalf("CA-440: %s: la manipulacion corta el sobre en scope, no-entregable, exit 1, sin veredicto: %+v", caso, res)
	}
	edSiguienteRoja(t, "CA-440", caso, root, gid)
}

// ================================================================ CA-442: solo archivos regulares

// CA-442: para cada forma de esconder (o no) lo creado y para CADA rol, el
// refutador incluido: un symlink creado con un nombre valido de hallazgo o
// de veredicto (a un archivo fuera de .hoom, a un directorio, o colgado) es
// UNA manipulacion con "la evidencia es un archivo regular" y su ruta, con
// su hallazgo del gate. Nunca se sigue: la huella no es el sha256 del
// destino. Una resolucion <id>.res.json que es un symlink tambien es UNA
// manipulacion (en el refutador, por no ser un archivo regular). (Cada rol
// corre en paralelo, en su copia del repo armado para la variante; los
// destinos de afuera son los mismos para todos.)
func TestCA442_UnSymlinkConNombreValidoEsManipulacionYNoSeSigue(t *testing.T) {
	qcLimpiarEntorno(t)
	for _, e := range edEscondenLoNuevo() {
		t.Run(e.caso, func(t *testing.T) {
			t.Parallel()
			plantilla := repo(t)
			e.armar(t, plantilla)
			id, _ := edHallazgo(t, plantilla, "high", "a refutar")
			afuera := t.TempDir()

			type enlace struct {
				ruta, destino string
				contenido     []byte // el del destino, si es un archivo
			}
			hallazgo := ".hoom/findings/20261001T010101_5e1f00.json"
			hallazgoAfuera := filepath.Join(afuera, "hallazgo.json")
			hallazgoBytes := edr1HallazgoJSON("20261001T010101_5e1f00", "low")
			veredictoAfuera := filepath.Join(afuera, "veredicto.json")
			veredictoBytes := []byte("{\"verdict\":\"green\"}\n")
			resolucionAfuera := filepath.Join(afuera, "resolucion.json")
			resolucionBytes := []byte(qcResolucionAMano(id, finding.StatusRefuted))
			for p, b := range map[string][]byte{hallazgoAfuera: hallazgoBytes, veredictoAfuera: veredictoBytes, resolucionAfuera: resolucionBytes} {
				if err := os.WriteFile(p, b, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			dirAfuera := filepath.Join(afuera, "dir")
			write(t, afuera, "dir/20261001T010104_5e1f03.json", string(edr1HallazgoJSON("20261001T010104_5e1f03", "high")))

			enlaces := []enlace{
				{hallazgo, hallazgoAfuera, hallazgoBytes},
				{".hoom/findings/20261001T010103_5e1f02.json", filepath.Join(afuera, "no-existe.json"), nil},
				{".hoom/findings/20261001T010104_5e1f03.json", dirAfuera, nil},
				{".hoom/verdicts/2026-10-01T01-01-01Z_5e1f0000.json", veredictoAfuera, veredictoBytes},
			}
			res := qcResRel(id)

			if err := os.MkdirAll(filepath.Join(plantilla, ".hoom", "verdicts"), 0o755); err != nil {
				t.Fatal(err)
			}
			for _, rol := range agents.Roles() {
				t.Run("rol "+rol.Slug, func(t *testing.T) {
					t.Parallel()
					caso := e.caso + ", rol " + rol.Slug
					root := edCopiarArbol(t, plantilla)
					before := edr1Take(t, "CA-442", caso+", foto de antes", root)
					for _, l := range enlaces {
						edr1Symlink(t, root, l.ruta, l.destino)
					}
					edr1Symlink(t, root, res, resolucionAfuera)
					after := edr1Take(t, "CA-442", caso+", foto con symlinks", root)

					todos := append(append([]enlace(nil), enlaces...), enlace{res, resolucionAfuera, resolucionBytes})
					for _, l := range todos {
						if e.oculto && !qcr1Ignorado(t, root, l.ruta) {
							t.Fatalf("CA-442: fixture: con %s git ignora %s", e.caso, l.ruta)
						}
						if l.contenido != nil && strings.EqualFold(after.Huellas[l.ruta], edSHA(l.contenido)) {
							t.Fatalf("CA-442: %s: la foto no sigue el symlink %s: su huella es el sha256 del destino %s", caso, l.ruta, l.destino)
						}
					}

					r := edr1Gate(t, "CA-442", caso, root, rol, before, after, PolicyFor(nil, rol))
					for _, l := range enlaces {
						v := edExigir(t, "CA-442", caso+", symlink a "+filepath.Base(l.destino), r, l.ruta, edr1Regular, l.ruta)
						if v.FindingID == "" {
							t.Fatalf("CA-442: %s: la manipulacion en %s registra su hallazgo high del gate", caso, l.ruta)
						}
					}
					if rol.Slug == finding.RolQueRefuta {
						edExigir(t, "CA-442", caso+", resolucion symlink", r, res, edr1Regular, res)
					} else {
						edr1Una(t, "CA-442", caso+", resolucion symlink", r, res, []string{edr1Regular, res}, []string{qcNoCierra(rol.Slug)})
					}
				})
			}
		})
	}
}

// CA-442 ("nunca se sigue ni se lee"): un symlink con nombre valido de
// hallazgo o de veredicto que lleva a un FIFO fuera de .hoom (que nadie
// escribe). Para cada forma de esconder (o no) lo creado: ni la foto ni el
// gate se cuelgan siguiendolo, y para el writer y el refutador es UNA
// manipulacion con "la evidencia es un archivo regular" y su ruta.
func TestCA442_UnSymlinkAUnFIFOConNombreValidoNoCuelgaLaFoto(t *testing.T) {
	qcLimpiarEntorno(t)
	for _, e := range edEscondenLoNuevo() {
		t.Run(e.caso, func(t *testing.T) {
			t.Parallel()
			root := repo(t)
			e.armar(t, root)
			fifoAfuera := filepath.Join(t.TempDir(), "fifo")
			edr1Fifo(t, fifoAfuera)
			if err := os.MkdirAll(filepath.Join(root, ".hoom", "verdicts"), 0o755); err != nil {
				t.Fatal(err)
			}
			before := edr1Take(t, "CA-442", e.caso+", foto de antes", root)
			enlaces := []string{".hoom/findings/20261001T010102_5e1f01.json", ".hoom/verdicts/2026-10-01T01-01-02Z_5e1f0001.json"}
			for _, l := range enlaces {
				edr1Symlink(t, root, l, fifoAfuera)
				if e.oculto && !qcr1Ignorado(t, root, l) {
					t.Fatalf("CA-442: fixture: con %s git ignora %s", e.caso, l)
				}
			}
			after := edr1Take(t, "CA-442", e.caso+", foto con symlinks a un FIFO", root)
			for _, slug := range []string{"writer", finding.RolQueRefuta} {
				rol := edr1Rol(t, slug)
				caso := e.caso + ", rol " + slug
				r := edr1Gate(t, "CA-442", caso, root, rol, before, after, PolicyFor(nil, rol))
				for _, l := range enlaces {
					edExigir(t, "CA-442", caso+", symlink a un FIFO", r, l, edr1Regular, l)
				}
			}
		})
	}
}

// CA-442: para cada forma de esconder (o no) lo creado y para CADA rol, el
// refutador incluido: un FIFO creado con un nombre valido de hallazgo, de
// veredicto o de resolucion es UNA manipulacion con "la evidencia es un
// archivo regular" y su ruta. Nunca se lee: ni la foto ni el gate se cuelgan
// en el FIFO (que nadie escribe). (Cada rol corre en paralelo, en su copia
// del repo armado para la variante.)
func TestCA442_UnFIFOConNombreValidoEsManipulacionYNoCuelgaLaFoto(t *testing.T) {
	qcLimpiarEntorno(t)
	for _, e := range edEscondenLoNuevo() {
		t.Run(e.caso, func(t *testing.T) {
			t.Parallel()
			plantilla := repo(t)
			e.armar(t, plantilla)
			id, _ := edHallazgo(t, plantilla, "high", "a refutar")
			if err := os.MkdirAll(filepath.Join(plantilla, ".hoom", "verdicts"), 0o755); err != nil {
				t.Fatal(err)
			}
			fifos := []string{".hoom/findings/20261001T020202_f1f0f1.json", ".hoom/verdicts/2026-10-01T02-02-02Z_f1f0f1f0.json"}
			res := qcResRel(id)
			for _, rol := range agents.Roles() {
				t.Run("rol "+rol.Slug, func(t *testing.T) {
					t.Parallel()
					caso := e.caso + ", rol " + rol.Slug
					root := edCopiarArbol(t, plantilla)
					before := edr1Take(t, "CA-442", caso+", foto de antes", root)
					for _, f := range []string{fifos[0], fifos[1], res} {
						edr1Fifo(t, filepath.Join(root, f))
						if e.oculto && !qcr1Ignorado(t, root, f) {
							t.Fatalf("CA-442: fixture: con %s git ignora %s", e.caso, f)
						}
					}
					after := edr1Take(t, "CA-442", caso+", foto con FIFOs", root)

					r := edr1Gate(t, "CA-442", caso, root, rol, before, after, PolicyFor(nil, rol))
					for _, f := range fifos {
						v := edExigir(t, "CA-442", caso+", FIFO", r, f, edr1Regular, f)
						if v.FindingID == "" {
							t.Fatalf("CA-442: %s: la manipulacion en %s registra su hallazgo high del gate", caso, f)
						}
					}
					if rol.Slug == finding.RolQueRefuta {
						edExigir(t, "CA-442", caso+", resolucion FIFO", r, res, edr1Regular, res)
					} else {
						edr1Una(t, "CA-442", caso+", resolucion FIFO", r, res, []string{edr1Regular, res}, []string{qcNoCierra(rol.Slug)})
					}
				})
			}
		})
	}
}

// CA-442 (caso limite): cambiar un hallazgo que git ignora por un symlink a
// una copia byte a byte de si mismo fuera de .hoom no es "reescribirlo
// igual": el symlink no se sigue, y la evidencia que era un archivo regular
// cambio. UNA manipulacion.
func TestCA442_CambiarUnHallazgoPorUnSymlinkAlMismoContenidoEsManipulacion(t *testing.T) {
	qcLimpiarEntorno(t)
	root := repo(t)
	qcr1Anexar(t, qcr1Exclude(t, root), ".hoom/findings/")
	_, rel := edHallazgo(t, root, "high", "el retry no respeta el backoff")
	copia := filepath.Join(t.TempDir(), "copia.json")
	if err := os.WriteFile(copia, edLeer(t, root, rel), 0o644); err != nil {
		t.Fatal(err)
	}
	before := Take(root, "main")
	if err := os.Remove(filepath.Join(root, rel)); err != nil {
		t.Fatal(err)
	}
	edr1Symlink(t, root, rel, copia)
	if !bytes.Equal(edLeer(t, root, rel), edLeer(t, filepath.Dir(copia), filepath.Base(copia))) {
		t.Fatalf("CA-442: fixture: el symlink lleva al mismo contenido")
	}
	after := Take(root, "main")
	for _, slug := range []string{"writer", finding.RolQueRefuta} {
		rol := edr1Rol(t, slug)
		r := Gate(root, "main", "", rol, before, after, PolicyFor(nil, rol), nil)
		edr1Una(t, "CA-442", "rol "+slug+", hallazgo cambiado por un symlink a su copia", r, rel,
			[]string{edAppendOnly, rel, edCambio}, []string{edr1Regular, rel})
	}
}

// CA-442 de punta a punta: 'hoom agent --role writer' implementa y crea un
// symlink con nombre valido de hallazgo (.hoom/findings/<id>.json) a un
// hallazgo high fuera de .hoom, que git ve o que ignora. La corrida corta en
// scope: UNA manipulacion con "la evidencia es un archivo regular", su
// hallazgo high del gate en el disco, sin veredicto. Y en la corrida
// siguiente, un writer que cambia el DESTINO del symlink (high -> low) no
// baja nada en silencio: el sobre no termina verde.
func TestCA442_ElSymlinkCreadoCortaYCambiarSuDestinoDespuesNoBajaElHallazgo(t *testing.T) {
	for _, oculto := range []bool{false, true} {
		nombre := "git lo ve"
		if oculto {
			nombre = "git lo ignora"
		}
		t.Run(nombre, func(t *testing.T) {
			root := edRepo(t, true, false)
			if oculto {
				qcr1Anexar(t, qcr1Exclude(t, root), ".hoom/findings/")
			}
			id := "20261001T030303_5e1f5e"
			rel := ".hoom/findings/" + id + ".json"
			destino := filepath.Join(t.TempDir(), "hallazgo.json")
			if err := os.WriteFile(destino, edr1HallazgoJSON(id, "high"), 0o644); err != nil {
				t.Fatal(err)
			}
			fakeProvider(t, "claude", edImplementar+"mkdir -p .hoom/findings\nln -s "+qcComillas(destino)+" "+qcComillas(rel)+"\nexit 0\n")
			res := edCorrer(t, "CA-442", nombre+", crea el symlink", root, Options{Role: "writer", Prompt: "implementa"})
			if fi, err := os.Lstat(filepath.Join(root, rel)); err != nil || fi.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("CA-442: fixture: el writer falso creo el symlink %s: %v", rel, err)
			}
			if oculto && !qcr1Ignorado(t, root, rel) {
				t.Fatalf("CA-442: fixture: git ignora %s", rel)
			}
			// sin cortar el test: la corrida siguiente se mide igual
			ms := edManipulaciones(res.Scope.Violations, rel)
			switch {
			case len(ms) != 1 || !strings.Contains(ms[0].Detail, edr1Regular) || !strings.Contains(ms[0].Detail, rel):
				t.Errorf("CA-442: %s: el symlink creado es UNA manipulacion con %q y su ruta: %s", nombre, edr1Regular, edLista(res.Scope.Violations))
			case res.Stage != "scope" || res.ExitCode != 1 || res.VerdictID != "" || res.Status != "no-entregable":
				t.Errorf("CA-442: %s: el symlink corta el sobre en scope, no-entregable, exit 1, sin veredicto: %+v", nombre, res)
			default:
				edr1HallazgoDelGate(t, "CA-442", nombre+", crea el symlink", root, ms[0].FindingID)
			}

			// la corrida siguiente cambia el destino, no el symlink
			fakeProvider(t, "claude", "printf 'package app // la corrida siguiente\\n' > app.go\n"+
				edCopiar(edPreparado(t, edr1HallazgoJSON(id, "low")), destino)+"exit 0\n")
			res2 := edCorrer(t, "CA-442", nombre+", cambia el destino", root, Options{Role: "writer", Prompt: "implementa"})
			if !bytes.Contains(edLeer(t, filepath.Dir(destino), filepath.Base(destino)), []byte(`"low"`)) {
				t.Fatalf("CA-442: fixture: el writer falso bajo el destino del symlink a low")
			}
			if res2.Status == "entregable" || res2.Verdict == "green" || res2.ExitCode == 0 {
				t.Fatalf("CA-442: %s: cambiar el destino del symlink no deja verde a la corrida siguiente: status=%s verdict=%s exit=%d stage=%s scope=%s",
					nombre, res2.Status, res2.Verdict, res2.ExitCode, res2.Stage, edLista(res2.Scope.Violations))
			}
		})
	}
}

// ---------------------------------------------------------------- 7a2c12: una ruta, una manipulacion

// CA-442 (una ruta que ya es manipulacion no se duplica; hallazgo 7a2c12):
// para CADA rol que no es refutador y cada forma de esconder (o no) lo
// creado, una resolucion sin la forma de hoom (x.res.json, hex en
// mayusculas, otro largo, sin id) es UNA manipulacion — con el detalle de la
// forma o con el de quien-cierra-un-hallazgo — y deja UN hallazgo del gate.
// Control: una resolucion CON la forma de hoom tambien es una sola (la de
// quien-cierra-un-hallazgo). (Cada rol corre en paralelo, en su copia del
// repo armado para la variante.)
func TestCA442_UnaResolucionSinFormaDeUnRolQueNoEsRefutadorEsUnaSolaManipulacion(t *testing.T) {
	malas := []string{
		".hoom/findings/x.res.json",
		".hoom/findings/20260101T000000_ABCDEF.res.json",
		".hoom/findings/20260101T000000_abcde.res.json",
		".hoom/findings/.res.json",
	}
	qcLimpiarEntorno(t)
	for _, e := range edEscondenLoNuevo() {
		t.Run(e.caso, func(t *testing.T) {
			t.Parallel()
			plantilla := repo(t)
			e.armar(t, plantilla)
			id, _ := edHallazgo(t, plantilla, "high", "a cerrar")
			for _, rol := range agents.Roles() {
				if rol.Slug == finding.RolQueRefuta {
					continue
				}
				t.Run("rol "+rol.Slug, func(t *testing.T) {
					t.Parallel()
					caso := e.caso + ", rol " + rol.Slug
					root := edCopiarArbol(t, plantilla)
					before := Take(root, "main")
					for _, m := range malas {
						write(t, root, m, qcResolucionAMano(id, finding.StatusRefuted))
					}
					buena := qcResRel(id)
					write(t, root, buena, qcResolucionAMano(id, finding.StatusRefuted))
					after := Take(root, "main")
					if e.oculto {
						for _, m := range append(append([]string(nil), malas...), buena) {
							if !qcr1Ignorado(t, root, m) {
								t.Fatalf("CA-442: fixture: con %s git ignora %s", e.caso, m)
							}
						}
					}

					antes := edr1HallazgosEn(t, root)
					r := Gate(root, "main", "", rol, before, after, todo(), nil)
					nuevos := 0
					for n := range edr1HallazgosEn(t, root) {
						if !antes[n] {
							nuevos++
						}
					}
					for _, m := range malas {
						var todas []Violation
						for _, v := range r.Violations {
							if v.Path == m {
								todas = append(todas, v)
							}
						}
						if len(todas) != 1 {
							t.Fatalf("CA-442: %s: %s es UNA violacion (hay %d): %s", caso, m, len(todas), edLista(r.Violations))
						}
						v := edr1Una(t, "CA-442", caso, r, m, []string{edForma("findings"), m}, []string{qcNoCierra(rol.Slug)})
						if v.FindingID == "" {
							t.Fatalf("CA-442: %s: la manipulacion en %s registra su hallazgo high del gate", caso, m)
						}
					}
					edr1Una(t, "CA-442", caso+", resolucion con forma", r, buena, []string{qcNoCierra(rol.Slug)})
					if quiere := len(malas) + 1; len(r.Violations) != quiere || nuevos != quiere {
						t.Fatalf("CA-442: %s: %d rutas, %d violaciones y %d hallazgos del gate (hay %d violaciones y %d hallazgos): %s",
							caso, quiere, quiere, quiere, len(r.Violations), nuevos, edLista(r.Violations))
					}
				})
			}
		})
	}
}

// ================================================================ CA-444: la evidencia ilegible

// CA-444: para cada forma de esconder (o no) lo creado y para CADA rol, el
// refutador incluido: un hallazgo y un veredicto creados con nombre valido y
// modo 000 son UNA manipulacion; cuando git no los ve, con "la evidencia no
// se puede leer" y su ruta. Con su hallazgo del gate. (Cada rol corre en
// paralelo, en su copia del repo armado para la variante.)
func TestCA444_UnaEvidenciaCreadaSinPermisoDeLecturaEsManipulacion(t *testing.T) {
	edr1NoRoot(t)
	qcLimpiarEntorno(t)
	for _, e := range edEscondenLoNuevo() {
		t.Run(e.caso, func(t *testing.T) {
			t.Parallel()
			plantilla := repo(t)
			e.armar(t, plantilla)
			if err := os.MkdirAll(filepath.Join(plantilla, ".hoom", "findings"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(plantilla, ".hoom", "verdicts"), 0o755); err != nil {
				t.Fatal(err)
			}
			creadas := []string{".hoom/findings/20261001T040404_000000.json", ".hoom/verdicts/2026-10-01T04-04-04Z_00000000.json"}
			for _, rol := range agents.Roles() {
				t.Run("rol "+rol.Slug, func(t *testing.T) {
					t.Parallel()
					caso := e.caso + ", rol " + rol.Slug
					root := edCopiarArbol(t, plantilla)
					before := Take(root, "main")
					write(t, root, creadas[0], string(edr1HallazgoJSON("20261001T040404_000000", "low")))
					write(t, root, creadas[1], "{\"verdict\":\"green\"}\n")
					for _, c := range creadas {
						edr1SinLectura(t, root, c)
						if e.oculto && !qcr1Ignorado(t, root, c) {
							t.Fatalf("CA-444: fixture: con %s git ignora %s", e.caso, c)
						}
					}
					after := Take(root, "main")
					r := Gate(root, "main", "", rol, before, after, PolicyFor(nil, rol), nil)
					for _, c := range creadas {
						var v Violation
						if e.oculto {
							v = edExigir(t, "CA-444", caso+", creado con modo 000", r, c, edr1Ilegible, c)
						} else {
							v = edr1Una(t, "CA-444", caso+", creado con modo 000", r, c)
						}
						if rol.Slug == "writer" {
							edr1HallazgoDelGate(t, "CA-444", caso, root, v.FindingID)
						} else if v.FindingID == "" {
							t.Fatalf("CA-444: %s: la manipulacion en %s registra su hallazgo high del gate", caso, c)
						}
					}
				})
			}
		})
	}
}

// CA-444: para cada forma de esconder la evidencia que ya existia y para
// CADA rol: bajar un hallazgo de high a low y quitarle la lectura,
// quitarsela SIN cambiarlo a otro hallazgo, a un veredicto y a una
// aprobacion: UNA manipulacion en cada ruta; cuando git no la ve, con "la
// evidencia no se puede leer" y su ruta. (Cada rol corre en paralelo, en su
// copia de la evidencia armada para el escondite.)
func TestCA444_QuitarleLaLecturaAUnaEvidenciaQueExistiaEsManipulacion(t *testing.T) {
	edr1NoRoot(t)
	qcLimpiarEntorno(t)
	for _, e := range edEscondites() {
		t.Run(e.caso, func(t *testing.T) {
			t.Parallel()
			plantilla, r, _ := edArmarEvidenciaSinEntorno(t, e)
			for _, rol := range agents.Roles() {
				t.Run("rol "+rol.Slug, func(t *testing.T) {
					t.Parallel()
					caso := e.caso + ", rol " + rol.Slug
					root := edCopiarArbol(t, plantilla)
					before := Take(root, "main")
					write(t, root, r["f1"], string(edBajarSeveridad(t, edLeer(t, root, r["f1"]))))
					ilegibles := []string{r["f1"], r["f2"], r["v1"], r["a1"]}
					for _, p := range ilegibles {
						edr1SinLectura(t, root, p)
					}
					after := Take(root, "main")
					oculta := map[string]bool{}
					for _, p := range ilegibles {
						oculta[p] = edOculta(t, root, p, e)
					}
					if !oculta[r["f1"]] || !oculta[r["f2"]] {
						t.Fatalf("CA-444: fixture: con %s git no ve los hallazgos", e.caso)
					}
					res := Gate(root, "main", "", rol, before, after, PolicyFor(nil, rol), nil)
					for _, p := range ilegibles {
						var v Violation
						if oculta[p] {
							v = edExigir(t, "CA-444", caso+", sin lectura", res, p, edr1Ilegible, p)
						} else {
							v = edr1Una(t, "CA-444", caso+", sin lectura", res, p)
						}
						if v.FindingID == "" {
							t.Fatalf("CA-444: %s: la manipulacion en %s registra su hallazgo high del gate", caso, p)
						}
					}
				})
			}
		})
	}
}

// CA-444: la foto de ANTES tambien cuenta. Un hallazgo high que git ignora y
// que la corrida encuentra ilegible: el rol que lo edita y lo vuelve a dejar
// ilegible (ilegible en las dos fotos), el que le devuelve la lectura y lo
// baja, y el que solo le devuelve la lectura. En cada caso, para el writer y
// el refutador: UNA manipulacion con "la evidencia no se puede leer".
func TestCA444_UnaEvidenciaIlegibleEnLaFotoDeAntesEsManipulacion(t *testing.T) {
	edr1NoRoot(t)
	qcLimpiarEntorno(t)
	for _, c := range []struct {
		caso    string
		durante func(t *testing.T, root, rel string, bajo []byte)
	}{
		{"ilegible en las dos fotos, editada en el medio", func(t *testing.T, root, rel string, bajo []byte) {
			edr1ConLectura(t, root, rel, 0o644)
			write(t, root, rel, string(bajo))
			edr1SinLectura(t, root, rel)
		}},
		{"le devuelve la lectura y la baja", func(t *testing.T, root, rel string, bajo []byte) {
			edr1ConLectura(t, root, rel, 0o644)
			write(t, root, rel, string(bajo))
		}},
		{"solo le devuelve la lectura", func(t *testing.T, root, rel string, bajo []byte) {
			edr1ConLectura(t, root, rel, 0o644)
		}},
	} {
		t.Run(c.caso, func(t *testing.T) {
			t.Parallel()
			root := repo(t)
			qcr1Anexar(t, qcr1Exclude(t, root), ".hoom/findings/")
			_, rel := edHallazgo(t, root, "high", "el retry no respeta el backoff")
			bajo := edBajarSeveridad(t, edLeer(t, root, rel))
			edr1SinLectura(t, root, rel)
			before := Take(root, "main")
			c.durante(t, root, rel, bajo)
			after := Take(root, "main")
			for _, slug := range []string{"writer", finding.RolQueRefuta} {
				rol := edr1Rol(t, slug)
				res := Gate(root, "main", "", rol, before, after, PolicyFor(nil, rol), nil)
				edExigir(t, "CA-444", c.caso+", rol "+slug, res, rel, edr1Ilegible, rel)
			}
		})
	}
}

// CA-444: un DIRECTORIO de evidencia (.hoom/findings, .hoom/verdicts,
// .hoom/approvals) que git ignora y que hoom no puede leer, en la foto de
// despues, en la de antes, o en las dos (con un cambio adentro en el medio):
// manipulacion "la evidencia no se puede leer" en el directorio o debajo.
// Para el writer.
func TestCA444_UnDirectorioDeEvidenciaIlegibleEsManipulacion(t *testing.T) {
	edr1NoRoot(t)
	qcLimpiarEntorno(t)
	wr := edr1Rol(t, "writer")
	for _, d := range []string{"findings", "verdicts", "approvals"} {
		for _, cuando := range []string{"despues", "antes", "las dos"} {
			caso := ".hoom/" + d + " ilegible " + cuando
			t.Run(caso, func(t *testing.T) {
				t.Parallel()
				root, r, _ := edArmarEvidenciaSinEntorno(t, edEscondites()[0]) // .gitignore de la raiz
				dir := ".hoom/" + d
				adentro := map[string]string{"findings": r["f1"], "verdicts": r["v1"], "approvals": r["a1"]}[d]
				if !qcr1Ignorado(t, root, adentro) {
					t.Fatalf("CA-444: fixture: git ignora %s", adentro)
				}
				editar := func() {
					raw := edLeer(t, root, adentro)
					if d == "findings" {
						raw = edBajarSeveridad(t, raw)
					} else {
						raw = append(raw, ' ')
					}
					write(t, root, adentro, string(raw))
				}
				if cuando != "despues" {
					edr1SinLectura(t, root, dir)
				}
				before := Take(root, "main")
				if cuando != "despues" {
					edr1ConLectura(t, root, dir, 0o755)
				}
				editar()
				if cuando != "antes" {
					edr1SinLectura(t, root, dir)
				}
				after := Take(root, "main")
				res := Gate(root, "main", "", wr, before, after, PolicyFor(nil, wr), nil)
				edr1ExigirIlegible(t, "CA-444", caso, res, dir)
			})
		}
	}
}

// CA-444 (la cadena de dos corridas, hallazgo e87f8b): la corrida N deja
// ilegible el .hoom/findings/ que git ignora (y el hallazgo de su gate puede
// no persistirse); la corrida N+1 arranca con el directorio ilegible, el rol
// le devuelve la lectura, baja a low un hallazgo high y borra otro. Las dos
// cortan con "la evidencia no se puede leer": la N+1 nunca queda OK.
func TestCA444_LaCorridaSiguienteAUnaQueDejoIlegibleLosHallazgosCorta(t *testing.T) {
	edr1NoRoot(t)
	qcLimpiarEntorno(t)
	for _, slug := range []string{"writer", finding.RolQueRefuta} {
		t.Run(slug, func(t *testing.T) {
			t.Parallel()
			rol := edr1Rol(t, slug)
			root := repo(t)
			write(t, root, ".gitignore", ".hoom/findings/\n")
			edCommit(t, root, "la raiz ignora los hallazgos")
			_, f1 := edHallazgo(t, root, "high", "uno")
			_, f2 := edHallazgo(t, root, "high", "dos")
			bajo := edBajarSeveridad(t, edLeer(t, root, f1))
			dir := ".hoom/findings"

			// corrida N: deja el directorio ilegible
			before := Take(root, "main")
			edr1SinLectura(t, root, dir)
			after := Take(root, "main")
			res := Gate(root, "main", "", rol, before, after, PolicyFor(nil, rol), nil)
			if _, ok := edr1BuscarIlegible(res, dir); !ok {
				// no corta el test: la corrida N+1 se mide igual
				t.Errorf("CA-444: corrida N (rol %s): dejar ilegible %s es manipulacion con %q: %s (ok=%v)",
					slug, dir, edr1Ilegible, edLista(res.Violations), res.OK)
			}

			// corrida N+1: arranca ilegible; el rol la devuelve, baja y borra
			before = Take(root, "main")
			edr1ConLectura(t, root, dir, 0o755)
			write(t, root, f1, string(bajo))
			if err := os.Remove(filepath.Join(root, f2)); err != nil {
				t.Fatal(err)
			}
			after = Take(root, "main")
			res = Gate(root, "main", "", rol, before, after, PolicyFor(nil, rol), nil)
			if res.OK || !res.Tampering {
				t.Fatalf("CA-444: corrida N+1 (rol %s): arrancar con los hallazgos ilegibles, bajar uno y borrar otro nunca queda OK: %s", slug, edLista(res.Violations))
			}
			edr1ExigirIlegible(t, "CA-444", "corrida N+1, rol "+slug, res, dir)
		})
	}
}

// CA-444 de punta a punta, la misma cadena con 'hoom agent --role writer' en
// un proyecto con findings.block_on: high y dos hallazgos high que git
// ignora: el sobre N implementa y deja ilegible .hoom/findings (corta, no
// verde); el sobre N+1 le devuelve la lectura, implementa, baja uno a low y
// borra el otro: corta en scope con "la evidencia no se puede leer" y su
// hallazgo high; y un writer legitimo despues queda rojo en findings_open
// por ese hallazgo.
func TestCA444_ElSobreSiguienteAUnoQueDejoIlegiblesLosHallazgosCortaYNoBaja(t *testing.T) {
	edr1NoRoot(t)
	root := edRepo(t, true, false)
	qcr1Anexar(t, qcr1Exclude(t, root), ".hoom/findings/")
	_, f1 := edHallazgo(t, root, "high", "uno")
	_, f2 := edHallazgo(t, root, "high", "dos")
	bajo := edBajarSeveridad(t, edLeer(t, root, f1))
	dir := ".hoom/findings"
	edr1Restaurar(t, filepath.Join(root, dir), 0o755)

	fakeProvider(t, "claude", edImplementar+"chmod 000 .hoom/findings\nexit 0\n")
	res := edCorrer(t, "CA-444", "sobre N", root, Options{Role: "writer", Prompt: "implementa"})
	if fi, err := os.Stat(filepath.Join(root, dir)); err != nil || fi.Mode().Perm() != 0 {
		t.Fatalf("CA-444: fixture: el writer falso dejo %s con modo 000: %v", dir, err)
	}
	if _, err := os.ReadDir(filepath.Join(root, dir)); err == nil {
		t.Skip("CA-444: el sistema de archivos deja leer un directorio con modo 000")
	}
	if res.Status == "entregable" || res.Verdict == "green" || res.ExitCode == 0 {
		t.Fatalf("CA-444: sobre N: dejar ilegibles los hallazgos no deja verde al sobre: %+v", res)
	}
	if _, ok := edr1BuscarIlegible(res.Scope, dir); !ok {
		// no corta el test: el sobre N+1 se mide igual
		t.Errorf("CA-444: sobre N: dejar ilegible %s es manipulacion con %q: %s (ok=%v)",
			dir, edr1Ilegible, edLista(res.Scope.Violations), res.Scope.OK)
	}

	// implementa OTRA cosa: el mismo app.go que la corrida N no seria una entrega
	fakeProvider(t, "claude", "chmod 755 .hoom/findings\nprintf 'package app // la corrida N+1\\n' > app.go\n"+edCopiar(edPreparado(t, bajo), f1)+
		"rm -f "+qcComillas(f2)+"\nexit 0\n")
	res = edCorrer(t, "CA-444", "sobre N+1", root, Options{Role: "writer", Prompt: "implementa"})
	if existeEn(t, filepath.Join(root, f2)) || !bytes.Equal(edLeer(t, root, f1), bajo) {
		t.Fatalf("CA-444: fixture: el writer falso de N+1 bajo %s y borro %s", f1, f2)
	}
	if res.Status == "entregable" || res.Verdict == "green" || res.ExitCode == 0 {
		t.Fatalf("CA-444: sobre N+1: arrancar con los hallazgos ilegibles, bajar uno y borrar otro no deja verde al sobre: status=%s verdict=%s exit=%d stage=%s scope=%s",
			res.Status, res.Verdict, res.ExitCode, res.Stage, edLista(res.Scope.Violations))
	}
	v := edr1ExigirIlegible(t, "CA-444", "sobre N+1", res.Scope, dir)
	if res.Stage != "scope" || res.ExitCode != 1 || res.VerdictID != "" || res.Status != "no-entregable" {
		t.Fatalf("CA-444: sobre N+1: la manipulacion corta en scope, no-entregable, exit 1, sin veredicto: %+v", res)
	}
	edr1HallazgoDelGate(t, "CA-444", "sobre N+1", root, v.FindingID)
	edSiguienteRoja(t, "CA-444", "sobre N+1", root, v.FindingID)
}
