// Tests adversariales de la review de la review: los hallazgos confirmados
// de la review cruzada sobre "el binario de la review" (CA-162,
// .hoom/specs/codex-v2-y-review-cruzada.md: el pedido trae el comando exacto
// de registro) y "hallazgos con tarea" (CA-293,
// .hoom/specs/items-y-columna-derivada.md: la tarea de la review sale de
// --task o del --spec). Contrato corregido:
//   - registerCmd(goos, bin): fuera de Windows y con bin, la ruta citada para
//     shell POSIX; en Windows (PowerShell, cmd o bash: ninguna cita corre en
//     los tres) o sin bin, 'hoom' a secas (2d2604, f3973a).
//   - si executable (os.Executable) falla y no hay HoomBin: 'aviso:' en la
//     salida (el reviewer usara el hoom de su PATH) y el pedido con 'hoom
//     finding add' a secas; la review sigue (b83d37, fce0e4).
//   - Result.Notes = Record.Notes, y el JSON de Result trae "notes" solo si
//     hay notas (c64133).
//   - la nota se commitea: no lleva la ruta del binario, dice "fuera de la
//     tarea" y "(sin tarea o con otra)", y no culpa a "un hoom que no la
//     conoce" (0c1c65, 8cde63).
//
// Providers falsos en el PATH: ningun CLI de IA real.
package reviewcmd

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/quick"
)

// rrExecutableFalla hace fallar la resolucion de este binario durante el
// test y la restaura al terminar.
func rrExecutableFalla(t *testing.T) {
	t.Helper()
	orig := executable
	executable = func() (string, error) {
		return "", errors.New("rr: no se pudo resolver el ejecutable")
	}
	t.Cleanup(func() { executable = orig })
}

// rrExecutableEs hace que este binario resuelva a ruta durante el test.
func rrExecutableEs(t *testing.T, ruta string) {
	t.Helper()
	orig := executable
	executable = func() (string, error) { return ruta, nil }
	t.Cleanup(func() { executable = orig })
}

// rrJSON serializa el Result como lo hace --json y lo lee como mapa.
func rrJSON(t *testing.T, res Result) map[string]any {
	t.Helper()
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("CA-293: el Result se serializa: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("CA-293: el JSON del Result se lee: %v\n%s", err, raw)
	}
	return m
}

// rrNotasJSON devuelve las notas del JSON como []string (falla si la clave
// no es una lista de strings).
func rrNotasJSON(t *testing.T, m map[string]any) []string {
	t.Helper()
	crudo, ok := m["notes"].([]any)
	if !ok {
		t.Fatalf("CA-293: el JSON del Result trae \"notes\" como lista: %#v", m["notes"])
	}
	out := make([]string, 0, len(crudo))
	for _, v := range crudo {
		s, ok := v.(string)
		if !ok {
			t.Fatalf("CA-293: cada nota del JSON es un string: %#v", v)
		}
		out = append(out, s)
	}
	return out
}

// rrNotaLimpia exige que cada nota no traiga ningun pedazo de la ruta de un
// binario ni la atribucion falsa "un hoom que no la conoce".
func rrNotaLimpia(t *testing.T, donde string, notas []string, rutas ...string) {
	t.Helper()
	for _, n := range notas {
		for _, r := range rutas {
			if r != "" && strings.Contains(n, r) {
				t.Fatalf("CA-293 (0c1c65): la nota se commitea: la de %s no trae la ruta del binario (%q):\n%q",
					donde, r, n)
			}
		}
		for _, culpa := range []string{"no la conoce", "no conoce"} {
			if strings.Contains(n, culpa) {
				t.Fatalf("CA-293 (8cde63): la nota de %s no culpa a un hoom que no conoce la tarea (%q), "+
					"esa causa es falsa con otra --task:\n%q", donde, culpa, n)
			}
		}
	}
}

// rrNotaDice exige que cada nota que nombra id nombre tambien la tarea
// esperada y diga "fuera de la tarea" y "(sin tarea o con otra)".
func rrNotaDice(t *testing.T, donde string, notas []string, id, tarea string) {
	t.Helper()
	if _, ok := hbNota(notas, id); !ok {
		t.Fatalf("CA-293 (0c1c65): una nota de %s nombra el hallazgo %s: %q", donde, id, notas)
	}
	for _, n := range notas {
		if !strings.Contains(n, id) {
			continue
		}
		for _, p := range []string{tarea, "fuera de la tarea", "(sin tarea o con otra)"} {
			if !strings.Contains(n, p) {
				t.Fatalf("CA-293 (0c1c65): la nota de %s que nombra %s dice %q:\n%q", donde, id, p, n)
			}
		}
	}
}

// CA-162 (2d2604, f3973a): registerCmd cita la ruta para shell POSIX fuera
// de Windows; en Windows (ninguna forma de citar corre en PowerShell, cmd y
// bash a la vez) o sin bin, 'hoom' a secas, como antes del cambio.
func TestCA162_RegisterCmdCitaSoloFueraDeWindows(t *testing.T) {
	for _, c := range []struct {
		nombre, goos, bin, quiero string
	}{
		{"linux-simple", "linux", "/usr/local/bin/hoom", `'/usr/local/bin/hoom'`},
		{"linux-espacio", "linux", "/opt/hoom de prueba/bin/hoom", `'/opt/hoom de prueba/bin/hoom'`},
		{"linux-comilla", "linux", "/opt/o'hoom/bin/hoom", `'/opt/o'\''hoom/bin/hoom'`},
		{"darwin-espacio", "darwin", "/Users/ana maria/go/bin/hoom", `'/Users/ana maria/go/bin/hoom'`},
		{"darwin-comillas", "darwin", "/opt/d'Art agnan/it's/hoom", `'/opt/d'\''Art agnan/it'\''s/hoom'`},
		{"darwin-solo-comilla", "darwin", "'", `''\'''`},
		{"freebsd-espacio", "freebsd", "/usr/local/bin/mi hoom", `'/usr/local/bin/mi hoom'`},
		{"windows-espacio", "windows", `C:\Program Files\hoom\hoom.exe`, "hoom"},
		{"windows-comilla", "windows", `C:\Users\o'hoom\bin\hoom.exe`, "hoom"},
		{"windows-posix", "windows", "/opt/hoom de prueba/bin/hoom", "hoom"},
		{"linux-vacio", "linux", "", "hoom"},
		{"darwin-vacio", "darwin", "", "hoom"},
		{"windows-vacio", "windows", "", "hoom"},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			if got := registerCmd(c.goos, c.bin); got != c.quiero {
				t.Fatalf("CA-162: registerCmd(%q, %q)\nquiero: %s\nfue:    %s", c.goos, c.bin, c.quiero, got)
			}
		})
	}
}

// CA-162 (2d2604, f3973a), propiedad: fuera de Windows, la palabra que da
// registerCmd, leida por un shell POSIX, es exactamente la ruta (espacios,
// comillas, $, `, \, comodines, saltos de linea); en Windows siempre es
// 'hoom' a secas.
func TestCA162_RegisterCmdLaCitaSobreviveAlShell(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sin sh en el PATH")
	}
	alfabeto := []rune("ab/ '\"$`\\\n\t*?;&|()<>~#!{}ñ-")
	prop := func(idx []uint8) bool {
		rs := []rune{'/'}
		for _, i := range idx {
			rs = append(rs, alfabeto[int(i)%len(alfabeto)])
		}
		bin := string(rs)
		cita := registerCmd("linux", bin)
		out, err := exec.Command(sh, "-c", "printf '%s' "+cita).Output()
		if err != nil || string(out) != bin {
			t.Logf("CA-162: la cita de %q es %s; el shell leyo %q (err %v)", bin, cita, out, err)
			return false
		}
		if w := registerCmd("windows", bin); w != "hoom" {
			t.Logf("CA-162: en windows registerCmd(%q) = %q, no 'hoom'", bin, w)
			return false
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 150}); err != nil {
		t.Fatalf("CA-162: la cita POSIX de registerCmd no sobrevive al shell: %v", err)
	}
}

// CA-162 (b83d37, fce0e4): si executable falla y no hay HoomBin, la salida
// avisa ('aviso:') que no pudo resolver este binario y que el reviewer usara
// el hoom de su PATH; el pedido trae 'hoom finding add' a secas, sin ninguna
// ruta citada; la review sigue normalmente.
func TestCA162_SiExecutableFallaElPedidoUsaHoomASecas(t *testing.T) {
	exe, _ := os.Executable() // antes de romperlo: el binario de test real
	rrExecutableFalla(t)
	root := repo(t)
	f := hbArgv(t)

	_, salida := hbRevisar(t, root, Options{Lens: "reliability", Provider: "codex"})
	argv := hbLeer(t, f)

	if _, ok := hbAviso(salida, "PATH"); !ok {
		t.Fatalf("CA-162 (b83d37): sin binario resuelto la salida avisa ('aviso:') que el reviewer usara "+
			"el hoom de su PATH:\n%s", salida)
	}
	if !strings.Contains(argv, "hoom finding add") || !strings.Contains(argv, "--author reviewer@codex") {
		t.Fatalf("CA-162 (fce0e4): el pedido trae 'hoom finding add' y --author reviewer@codex:\n%s", argv)
	}
	if strings.Contains(argv, "' finding add") {
		t.Fatalf("CA-162 (fce0e4): sin binario resuelto el pedido no trae ninguna ruta citada seguida de "+
			"' finding add' (ni una cita vacia ''):\n%s", argv)
	}
	if exe != "" && strings.Contains(argv, hbComillas(exe)) {
		t.Fatalf("CA-162 (fce0e4): el pedido no cita un binario que executable no resolvio (%s):\n%s", exe, argv)
	}
}

// CA-162 (b83d37, guarda): con HoomBin explicito, que executable falle no
// importa: el pedido cita HoomBin y no hay aviso de que se usara el PATH.
func TestCA162_ConHoomBinExecutableQueFallaNoImporta(t *testing.T) {
	rrExecutableFalla(t)
	root := repo(t)
	f := hbArgv(t)
	const bin = "/opt/hoom de prueba/bin/hoom"

	_, salida := hbRevisar(t, root, Options{Lens: "reliability", Provider: "codex", HoomBin: bin})
	argv := hbLeer(t, f)
	if !strings.Contains(argv, hbComillas(bin)+" finding add --sev") {
		t.Fatalf("CA-162: con HoomBin el pedido lo cita aunque executable falle:\n%s", argv)
	}
	if l, ok := hbAviso(salida, "PATH"); ok {
		t.Fatalf("CA-162 (b83d37): con HoomBin no se avisa que el reviewer usara el PATH: %q\n%s", l, salida)
	}
}

// CA-293 (c64133): con un hallazgo nuevo fuera de la tarea, Result.Notes no
// esta vacio, es igual a Record.Notes del registro escrito, y el JSON del
// Result (json.Marshal, lo que imprime --json) trae "notes" con lo mismo.
// Tambien con varias pasadas: las notas del Result son TODAS las del
// registro, no solo las de la ultima pasada.
func TestCA293_ResultTraeLasNotasDelRegistro(t *testing.T) {
	t.Run("una-pasada", func(t *testing.T) {
		root := repo(t)
		write(t, root, htSpec, "# Spec: "+htSlug+"\n")
		const id = "20260924T130000_c64133"
		script, _ := hbHallazgo(id, "")
		fakeProvider(t, "codex", script+"exit 0\n")

		res, salida := hbRevisar(t, root, Options{Lens: "risk", Provider: "codex", Spec: htSpec})
		rrNotasIguales(t, root, res, salida, id)
	})
	t.Run("cuatro-lentes", func(t *testing.T) {
		root := repo(t)
		write(t, root, htSpec, "# Spec: "+htSlug+"\n")
		write(t, root, "internal/auth/login.go", "package auth\n\nfunc Login() {}\n")
		const primero, tercero = "20260924T130001_c64133", "20260924T130002_c64133"
		s1, _ := hbHallazgo(primero, "")
		s3, _ := hbHallazgo(tercero, "otra-tarea")
		contador := filepath.Join(t.TempDir(), "n.txt")
		fakeProvider(t, "codex", "n=$(cat '"+contador+"' 2>/dev/null || echo 0)\n"+
			"n=$((n+1)); echo $n > '"+contador+"'\n"+
			"if [ $n -eq 1 ]; then\n"+s1+"fi\n"+
			"if [ $n -eq 3 ]; then\n"+s3+"fi\nexit 0\n")

		res, salida := hbRevisar(t, root, Options{Provider: "codex", Spec: htSpec})
		if len(res.Passes) != len(Lentes) {
			t.Fatalf("CA-293: una ruta de riesgo corre las 4 lentes: %+v\n%s", res, salida)
		}
		rrNotasIguales(t, root, res, salida, primero, tercero)
	})
}

// rrNotasIguales exige Result.Notes no vacio, igual a Record.Notes, con los
// ids, y el JSON del Result con "notes" igual.
func rrNotasIguales(t *testing.T, root string, res Result, salida string, ids ...string) {
	t.Helper()
	if len(res.Notes) == 0 {
		t.Fatalf("CA-293 (c64133): con hallazgos fuera de la tarea Result.Notes no esta vacio: %+v\n%s", res, salida)
	}
	r := hbRecord(t, root, res.RecordID)
	if !reflect.DeepEqual(res.Notes, r.Notes) {
		t.Fatalf("CA-293 (c64133): Result.Notes son las mismas notas que el registro:\nresult:   %q\nregistro: %q",
			res.Notes, r.Notes)
	}
	for _, id := range ids {
		if _, ok := hbNota(res.Notes, id, htSlug); !ok {
			t.Fatalf("CA-293 (c64133): Result.Notes nombra %s y la tarea %s: %q", id, htSlug, res.Notes)
		}
	}
	m := rrJSON(t, res)
	if got := rrNotasJSON(t, m); !reflect.DeepEqual(got, res.Notes) {
		t.Fatalf("CA-293 (c64133): el JSON del Result trae \"notes\" = Result.Notes:\njson:   %q\nresult: %q",
			got, res.Notes)
	}
}

// CA-293 (c64133, guarda): sin discrepancia (el hallazgo lleva la tarea, o la
// review no tiene tarea) Result.Notes esta vacio y el JSON del Result no trae
// la clave "notes".
func TestCA293_SinDiscrepanciaResultSinNotas(t *testing.T) {
	for _, c := range []struct{ nombre, spec, task string }{
		{"hallazgo-con-su-tarea", htSpec, htSlug},
		{"review-sin-tarea", "", ""},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			root := repo(t)
			if c.spec != "" {
				write(t, root, c.spec, "# Spec: "+htSlug+"\n")
			}
			const id = "20260924T130003_c64133"
			script, _ := hbHallazgo(id, c.task)
			fakeProvider(t, "codex", script+"exit 0\n")

			res, salida := hbRevisar(t, root, Options{Lens: "risk", Provider: "codex", Spec: c.spec})
			if len(res.Findings) != 1 || res.Findings[0] != id {
				t.Fatalf("CA-293: hoom ve el hallazgo: %+v\n%s", res, salida)
			}
			if len(res.Notes) != 0 {
				t.Fatalf("CA-293 (c64133): sin discrepancia Result.Notes queda vacio: %q", res.Notes)
			}
			if v, ok := rrJSON(t, res)["notes"]; ok {
				t.Fatalf("CA-293 (c64133): sin discrepancia el JSON del Result no trae \"notes\": %v", v)
			}
			hbSinNotas(t, root, res.RecordID)
		})
	}
}

// CA-293 (0c1c65, 8cde63): la nota se commitea, asi que no lleva la ruta del
// binario (ni HoomBin ni la que resolvio executable, ni pedazos de ellas);
// nombra la tarea esperada y los ids, dice "fuera de la tarea" y "(sin tarea
// o con otra)", y no culpa a "un hoom que no la conoce" (falso cuando el
// reviewer paso otra --task). Vale para la nota del registro (y su JSON
// crudo en disco) y para la de Result.
func TestCA293_LaNotaNoTraeLaRutaDelBinario(t *testing.T) {
	const privada = "/opt/ruta privada/bin/hoom"
	for _, c := range []struct {
		nombre  string
		hoomBin string
		exe     string // "" = executable real
	}{
		{"hoombin-explicito", privada, ""},
		{"executable-privado", "", "/opt/otra ruta privada/libexec/hoom"},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			if c.exe != "" {
				rrExecutableEs(t, c.exe)
			}
			ruta := c.hoomBin
			if ruta == "" {
				ruta = c.exe
			}
			pedazos := []string{ruta, filepath.Dir(ruta), "ruta privada", "/bin/hoom", "/libexec/hoom", "/opt/"}

			root := repo(t)
			write(t, root, htSpec, "# Spec: "+htSlug+"\n")
			const sinTarea, conOtra = "20260924T130004_0c1c65", "20260924T130005_8cde63"
			s1, _ := hbHallazgo(sinTarea, "")
			s2, _ := hbHallazgo(conOtra, "otra-tarea")
			fakeProvider(t, "codex", s1+s2+"exit 0\n")

			res, salida := hbRevisar(t, root, Options{
				Lens: "risk", Provider: "codex", Spec: htSpec, HoomBin: c.hoomBin,
			})
			if len(res.Findings) != 2 {
				t.Fatalf("CA-293: los dos hallazgos son del reviewer: %+v\n%s", res, salida)
			}
			r := hbRecord(t, root, res.RecordID)
			for _, n := range []struct {
				donde string
				notas []string
			}{
				{"Record.Notes", r.Notes},
				{"Result.Notes", res.Notes},
			} {
				rrNotaLimpia(t, n.donde, n.notas, pedazos...)
				rrNotaDice(t, n.donde, n.notas, sinTarea, htSlug)
				rrNotaDice(t, n.donde, n.notas, conOtra, htSlug)
			}
			crudo, err := os.ReadFile(filepath.Join(root, ".hoom", RecordsDir, res.RecordID+".json"))
			if err != nil {
				t.Fatalf("CA-293: el registro de review %s: %v", res.RecordID, err)
			}
			if strings.Contains(string(crudo), "ruta privada") {
				t.Fatalf("CA-293 (0c1c65): el registro commiteable no trae la ruta del binario:\n%s", crudo)
			}
		})
	}
}
