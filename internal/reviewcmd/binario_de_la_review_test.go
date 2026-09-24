// Tests adversariales del contrato "el binario de la review" sobre CA-162
// (.hoom/specs/codex-v2-y-review-cruzada.md: el pedido trae el comando exacto
// de registro) y CA-293 (.hoom/specs/items-y-columna-derivada.md: la tarea de
// la review sale de --task o del --spec). El reviewer corre el 'hoom' que
// encuentra SU shell, y un shell de login reordena el PATH: termina en un hoom
// viejo que no conoce HOOM_TASK y los hallazgos salen sin tarea. Por eso:
//   - el pedido trae el comando con la ruta ABSOLUTA de Options.HoomBin entre
//     comillas simples de shell ("" = os.Executable() de este proceso);
//   - despues de cada pasada, si la review tiene tarea y un hallazgo NUEVO no
//     la lleva, hoom avisa ('aviso:' + ids + tarea esperada) y el registro de
//     review gana Notes. La review igual termina revisado y los hallazgos
//     quedan tal cual.
//
// Providers falsos en el PATH: ningun CLI de IA real.
package reviewcmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/taskcmd"
)

// hbComillas cita s como una palabra de shell entre comillas simples: una
// comilla simple adentro se cierra, se escapa y se reabre: comilla, barra,
// comilla, comilla.
func hbComillas(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// hbArgv pone un codex falso que guarda cada argumento en una linea, FUERA
// del arbol revisado (no dispara el gate de scope), y devuelve el archivo.
func hbArgv(t *testing.T) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "argv.txt")
	fakeProvider(t, "codex", "printf '%s\\n' \"$@\" > '"+f+"'\nexit 0\n")
	return f
}

func hbLeer(t *testing.T, f string) string {
	t.Helper()
	raw, err := os.ReadFile(f)
	if err != nil {
		t.Fatalf("CA-162: el reviewer nunca corrio (no guardo su argv): %v", err)
	}
	return string(raw)
}

// hbHallazgo es el script con el que un reviewer deja un hallazgo como lo
// dejaria un hoom VIEJO (o uno que ignora la tarea): escribe el artefacto
// directo en .hoom/findings/ del directorio donde corre (el arbol revisado).
// task "" = sin la clave "task". Devuelve tambien los bytes exactos.
func hbHallazgo(id, task string) (script, cuerpo string) {
	cuerpo = `{"id":"` + id + `","created_at":"2026-09-24T12:00:00Z","severity":"medium",` +
		`"lens":"risk","file":"app.go","description":"Nuevo() no tiene test (app.go:3)",` +
		`"author":"reviewer@codex"`
	if task != "" {
		cuerpo += `,"task":"` + task + `"`
	}
	cuerpo += "}\n"
	script = "mkdir -p .hoom/findings\n" +
		"cat > .hoom/findings/" + id + ".json <<'EOF'\n" + cuerpo + "EOF\n"
	return script, cuerpo
}

// hbRevisar corre la review y exige revisado: un hallazgo sin tarea no es un
// fallo del reviewer.
func hbRevisar(t *testing.T, root string, opt Options) (Result, string) {
	t.Helper()
	var out bytes.Buffer
	res, err := Run(root, "main", opt, &out)
	if err != nil {
		t.Fatalf("CA-293: la review no se rompe: %v\n%s", err, out.String())
	}
	if res.Status != "revisado" || res.ExitCode != 0 || res.RecordID == "" {
		t.Fatalf("CA-293: un hallazgo sin la tarea no rompe la review: termina revisado con registro: %+v\n%s",
			res, out.String())
	}
	return res, out.String()
}

// hbAviso devuelve la primera linea de la salida que contiene 'aviso:' y
// TODAS las partes.
func hbAviso(salida string, partes ...string) (string, bool) {
	for _, l := range strings.Split(salida, "\n") {
		if !strings.Contains(l, "aviso:") {
			continue
		}
		todas := true
		for _, p := range partes {
			if !strings.Contains(l, p) {
				todas = false
				break
			}
		}
		if todas {
			return l, true
		}
	}
	return "", false
}

// hbRecord lee con Records(dir) el registro de id.
func hbRecord(t *testing.T, dir, id string) Record {
	t.Helper()
	recs, warns := Records(dir)
	for _, r := range recs {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("CA-293: Records(%s) no trae el registro %s (avisos %v)", dir, id, warns)
	return Record{}
}

// hbNota devuelve la primera nota que nombra TODAS las partes.
func hbNota(notas []string, partes ...string) (string, bool) {
	for _, n := range notas {
		todas := true
		for _, p := range partes {
			if !strings.Contains(n, p) {
				todas = false
				break
			}
		}
		if todas {
			return n, true
		}
	}
	return "", false
}

// hbSinNotas exige que el JSON del registro NO tenga la clave "notes" y que
// Records la lea vacia.
func hbSinNotas(t *testing.T, dir, id string) {
	t.Helper()
	m := htRegistro(t, dir, id)
	if v, ok := m["notes"]; ok {
		t.Fatalf("CA-293: sin hallazgos fuera de la tarea el registro no trae \"notes\" (omitido si vacio): %v", v)
	}
	if r := hbRecord(t, dir, id); len(r.Notes) != 0 {
		t.Fatalf("CA-293: sin hallazgos fuera de la tarea Notes queda vacio: %q", r.Notes)
	}
}

// hbSinAvisoDe exige que ninguna linea 'aviso:' nombre ninguna de las partes.
func hbSinAvisoDe(t *testing.T, salida string, partes ...string) {
	t.Helper()
	for _, p := range partes {
		if l, ok := hbAviso(salida, p); ok {
			t.Fatalf("CA-293: no hay aviso sobre %q: %q\n%s", p, l, salida)
		}
	}
}

// hbArtefactoIntacto exige que el hallazgo quede byte a byte como lo dejo el
// reviewer: hoom avisa, no reescribe hallazgos (son inmutables).
func hbArtefactoIntacto(t *testing.T, dir, id, cuerpo string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, ".hoom", "findings", id+".json"))
	if err != nil {
		t.Fatalf("CA-293: el hallazgo %s sigue en .hoom/findings/: %v", id, err)
	}
	if string(raw) != cuerpo {
		t.Fatalf("CA-293: el hallazgo queda tal cual (inmutable):\nquedo:   %q\nescrito: %q", raw, cuerpo)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, ".hoom", "findings", id+"*"))
	if len(matches) != 1 {
		t.Fatalf("CA-293: el aviso no resuelve ni duplica el hallazgo %s: %v", id, matches)
	}
}

// CA-162: con HoomBin explicito, el pedido trae el comando exacto con esa
// ruta ABSOLUTA entre comillas simples de shell (un espacio no la parte, una
// comilla simple se escapa como comilla, barra, comilla, comilla), y sigue
// trayendo 'hoom finding add' y la procedencia --author reviewer@codex.
func TestCA162_ElPedidoTraeElHoomDeLaReviewCitado(t *testing.T) {
	for _, bin := range []string{
		"/opt/hoom de prueba/bin/hoom",
		"/opt/o'hoom/bin/hoom",
		"/opt/d'Art agnan/it's/hoom",
	} {
		t.Run(bin, func(t *testing.T) {
			root := repo(t)
			f := hbArgv(t)

			_, salida := hbRevisar(t, root, Options{Lens: "reliability", Provider: "codex", HoomBin: bin})
			argv := hbLeer(t, f)
			quiero := hbComillas(bin) + " finding add --sev"
			if !strings.Contains(argv, quiero) {
				t.Fatalf("CA-162: el pedido trae el comando con el hoom de la review citado para el shell\n"+
					"quiero: %s\nargv:\n%s\nsalida:\n%s", quiero, argv, salida)
			}
			if !strings.Contains(argv, "hoom finding add") || !strings.Contains(argv, "--author reviewer@codex") {
				t.Fatalf("CA-162: el pedido sigue trayendo 'hoom finding add' y --author reviewer@codex:\n%s", argv)
			}
			// el comando citado es EL de registro: lleva su procedencia en la misma linea
			for _, l := range strings.Split(argv, "\n") {
				if strings.Contains(l, quiero) && !strings.Contains(l[strings.Index(l, quiero):], "--author reviewer@codex") {
					t.Fatalf("CA-162: el comando citado lleva --author reviewer@codex: %q", l)
				}
			}
			if strings.Contains(bin, "'") && strings.Contains(argv, "'"+bin+"' finding add") {
				t.Fatalf("CA-162: una comilla simple en la ruta va escapada como '\\'', no cruda:\n%s", argv)
			}
		})
	}
}

// CA-162: con HoomBin vacio, el comando del pedido usa ESTE binario
// (os.Executable(); en un test, el binario de test) entre comillas simples —
// nunca el 'hoom' que encuentre el PATH, aunque haya uno al frente.
func TestCA162_SinHoomBinElPedidoUsaEsteBinario(t *testing.T) {
	root := repo(t)
	viejo := filepath.Join(t.TempDir(), "viejo")
	if err := os.MkdirAll(viejo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(viejo, "hoom"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", viejo+":"+os.Getenv("PATH"))
	f := hbArgv(t)

	_, salida := hbRevisar(t, root, Options{Lens: "reliability", Provider: "codex"})
	argv := hbLeer(t, f)

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	candidatas := []string{exe}
	if r, err := filepath.EvalSymlinks(exe); err == nil && r != exe {
		candidatas = append(candidatas, r) // misma ruta sin symlinks (/var -> /private/var en macOS)
	}
	ok := false
	for _, c := range candidatas {
		if strings.Contains(argv, hbComillas(c)+" finding add") {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("CA-162: sin HoomBin el pedido cita os.Executable() (%s) seguido de ' finding add'\nargv:\n%s\nsalida:\n%s",
			exe, argv, salida)
	}
	if strings.Contains(argv, hbComillas(filepath.Join(viejo, "hoom"))) {
		t.Fatalf("CA-162: el pedido no usa el hoom que encuentra el PATH:\n%s", argv)
	}
	if !strings.Contains(argv, "hoom finding add") || !strings.Contains(argv, "--author reviewer@codex") {
		t.Fatalf("CA-162: el pedido sigue trayendo 'hoom finding add' y --author reviewer@codex:\n%s", argv)
	}
}

// CA-293: review con --spec (slug valido) y un reviewer que deja un hallazgo
// SIN task (como un hoom viejo): la salida lo avisa con el id y la tarea
// esperada, el registro gana una nota con ambos, la review termina revisado y
// el hallazgo queda tal cual.
func TestCA293_HallazgoSinTareaSeAvisaYQuedaNotado(t *testing.T) {
	root := repo(t)
	write(t, root, htSpec, "# Spec: "+htSlug+"\n")
	const id = "20260924T120000_a1b2c3"
	script, cuerpo := hbHallazgo(id, "")
	fakeProvider(t, "codex", script+"exit 0\n")

	res, salida := hbRevisar(t, root, Options{Lens: "risk", Provider: "codex", Spec: htSpec})
	if len(res.Findings) != 1 || res.Findings[0] != id {
		t.Fatalf("CA-293: el hallazgo sin tarea sigue siendo un hallazgo del reviewer: %+v\n%s", res, salida)
	}
	if _, ok := hbAviso(salida, id, htSlug); !ok {
		t.Fatalf("CA-293: la salida avisa ('aviso:') el hallazgo %s sin la tarea %s en una linea:\n%s", id, htSlug, salida)
	}
	r := hbRecord(t, root, res.RecordID)
	if _, ok := hbNota(r.Notes, id, htSlug); !ok {
		t.Fatalf("CA-293: el registro gana una nota que nombra %s y la tarea %s: %q", id, htSlug, r.Notes)
	}
	if r.Task != htSlug || len(r.Findings) != 1 || r.Findings[0] != id {
		t.Fatalf("CA-293: el registro sigue con su tarea y el hallazgo que hoom vio: %+v", r)
	}
	m := htRegistro(t, root, res.RecordID)
	if notas, _ := m["notes"].([]any); len(notas) == 0 {
		t.Fatalf("CA-293: el JSON del registro trae \"notes\": %v", m)
	}
	hbArtefactoIntacto(t, root, id, cuerpo)
}

// CA-293: un hallazgo con OTRA tarea tampoco es de la review; y cuando hay
// uno bueno y uno malo, el aviso y la nota nombran solo el malo.
func TestCA293_HallazgoConOtraTareaSeAvisaSoloElAjeno(t *testing.T) {
	root := repo(t)
	write(t, root, htSpec, "# Spec: "+htSlug+"\n")
	const bueno, malo = "20260924T120000_b0b0b0", "20260924T120001_c1c1c1"
	sBueno, _ := hbHallazgo(bueno, htSlug)
	sMalo, cMalo := hbHallazgo(malo, "otra-tarea")
	fakeProvider(t, "codex", sBueno+sMalo+"exit 0\n")

	res, salida := hbRevisar(t, root, Options{Lens: "risk", Provider: "codex", Spec: htSpec})
	if len(res.Findings) != 2 {
		t.Fatalf("CA-293: los dos hallazgos son del reviewer: %+v\n%s", res, salida)
	}
	if _, ok := hbAviso(salida, malo, htSlug); !ok {
		t.Fatalf("CA-293: un hallazgo con otra tarea se avisa con su id y la tarea esperada %s:\n%s", htSlug, salida)
	}
	hbSinAvisoDe(t, salida, bueno)
	r := hbRecord(t, root, res.RecordID)
	if _, ok := hbNota(r.Notes, malo, htSlug); !ok {
		t.Fatalf("CA-293: la nota nombra %s y la tarea %s: %q", malo, htSlug, r.Notes)
	}
	if _, ok := hbNota(r.Notes, bueno); ok {
		t.Fatalf("CA-293: la nota no nombra el hallazgo que SI lleva la tarea (%s): %q", bueno, r.Notes)
	}
	hbArtefactoIntacto(t, root, malo, cMalo)
}

// CA-293: con --task (y un --spec de OTRO slug) la tarea esperada es la de
// --task: un hallazgo sin task o con la del spec se avisa nombrando la tarea
// de --task, y la nota queda en el registro del worktree de la tarea.
func TestCA293_ConTareaElControlUsaLaDeTask(t *testing.T) {
	for _, caso := range []struct{ nombre, taskDelHallazgo string }{
		{"sin-task", ""},
		{"con-la-del-spec", htSlug},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			root := htRepoLimpio(t)
			if err := taskcmd.Start(root, "cabina-visual", "main"); err != nil {
				t.Fatal(err)
			}
			dir, err := runcmd.TaskDir(root, "cabina-visual")
			if err != nil {
				t.Fatal(err)
			}
			write(t, dir, "app.go", "package app\n\nfunc Nuevo() {}\n")
			const id = "20260924T120002_d2d2d2"
			script, cuerpo := hbHallazgo(id, caso.taskDelHallazgo)
			fakeProvider(t, "codex", script+"exit 0\n")

			res, salida := hbRevisar(t, root, Options{Lens: "risk", Provider: "codex", Task: "cabina-visual", Spec: htSpec})
			if _, ok := hbAviso(salida, id, "cabina-visual"); !ok {
				t.Fatalf("CA-293: el aviso nombra %s y la tarea esperada de --task (cabina-visual), no la del spec:\n%s",
					id, salida)
			}
			r := hbRecord(t, dir, res.RecordID)
			if _, ok := hbNota(r.Notes, id, "cabina-visual"); !ok {
				t.Fatalf("CA-293: la nota del registro del worktree nombra %s y cabina-visual: %q", id, r.Notes)
			}
			hbArtefactoIntacto(t, dir, id, cuerpo)
		})
	}
}

// CA-293: el control corre despues de CADA pasada: con las 4 lentes, un
// hallazgo sin tarea que aparece solo en la segunda pasada tambien se avisa y
// queda notado.
func TestCA293_ElControlCorreEnCadaPasada(t *testing.T) {
	root := repo(t)
	write(t, root, htSpec, "# Spec: "+htSlug+"\n")
	write(t, root, "internal/auth/login.go", "package auth\n\nfunc Login() {}\n")
	const id = "20260924T120003_e3e3e3"
	script, _ := hbHallazgo(id, "")
	contador := filepath.Join(t.TempDir(), "n.txt")
	fakeProvider(t, "codex", "n=$(cat '"+contador+"' 2>/dev/null || echo 0)\n"+
		"n=$((n+1)); echo $n > '"+contador+"'\n"+
		"if [ $n -eq 2 ]; then\n"+script+"fi\nexit 0\n")

	res, salida := hbRevisar(t, root, Options{Provider: "codex", Spec: htSpec})
	if len(res.Passes) != len(Lentes) || len(res.Passes[1].Findings) != 1 || res.Passes[1].Findings[0] != id {
		t.Fatalf("CA-293: el hallazgo aparece en la segunda pasada: %+v\n%s", res, salida)
	}
	if _, ok := hbAviso(salida, id, htSlug); !ok {
		t.Fatalf("CA-293: el hallazgo sin tarea de la segunda pasada se avisa con %s y %s:\n%s", id, htSlug, salida)
	}
	if _, ok := hbNota(hbRecord(t, root, res.RecordID).Notes, id, htSlug); !ok {
		t.Fatalf("CA-293: el registro nota el hallazgo de la segunda pasada")
	}
}

// CA-293 (guarda): todos los hallazgos llevan la tarea de la review: ni aviso
// ni "notes" en el JSON del registro.
func TestCA293_HallazgosConSuTareaSinAvisoNiNotas(t *testing.T) {
	root := repo(t)
	write(t, root, htSpec, "# Spec: "+htSlug+"\n")
	const id = "20260924T120004_f4f4f4"
	script, _ := hbHallazgo(id, htSlug)
	fakeProvider(t, "codex", script+"exit 0\n")

	res, salida := hbRevisar(t, root, Options{Lens: "risk", Provider: "codex", Spec: htSpec})
	if len(res.Findings) != 1 || res.Findings[0] != id {
		t.Fatalf("CA-293: hoom ve el hallazgo: %+v\n%s", res, salida)
	}
	hbSinAvisoDe(t, salida, id, htSlug)
	hbSinNotas(t, root, res.RecordID)
}

// CA-293 (guarda): sin tarea en la review (sin --task ni --spec, o con un
// --spec que no es slug valido) no hay control: un hallazgo sin task no se
// avisa ni se nota.
func TestCA293_SinTareaEnLaReviewNoHayControl(t *testing.T) {
	for _, caso := range []struct{ nombre, spec string }{
		{"sin-task-ni-spec", ""},
		{"spec-sin-slug-valido", ".hoom/specs/Mi Spec.md"},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			root := repo(t)
			const id = "20260924T120005_a5a5a5"
			script, _ := hbHallazgo(id, "")
			fakeProvider(t, "codex", script+"exit 0\n")

			res, salida := hbRevisar(t, root, Options{Lens: "risk", Provider: "codex", Spec: caso.spec})
			if len(res.Findings) != 1 || res.Findings[0] != id {
				t.Fatalf("CA-293: hoom ve el hallazgo: %+v\n%s", res, salida)
			}
			hbSinAvisoDe(t, salida, id)
			hbSinNotas(t, root, res.RecordID)
		})
	}
}

// hbHoomReal compila el binario real de hoom en un directorio temporal y
// devuelve su ruta (sin tocar el PATH).
func hbHoomReal(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("compila el binario de hoom: se omite con -short")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("sin toolchain de Go en el PATH")
	}
	bin := filepath.Join(t.TempDir(), "hoom")
	cmd := exec.Command(goBin, "build", "-o", bin, "github.com/hoomdev/hoomai/cmd/hoom")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("no pude compilar hoom: %v\n%s", err, out)
	}
	return bin
}

// CA-162 / CA-293, de punta a punta: un hoom VIEJO al frente del PATH (deja
// hallazgos sin task) y un reviewer que SIGUE el pedido: saca del argv el
// comando citado '<hoom>' finding add y lo corre sin --task. Con HoomBin =
// hoom real compilado, el hallazgo lleva la tarea del --spec (HOOM_TASK), no
// hay aviso y el registro no trae "notes".
func TestCA293_E2EElReviewerQueSigueElPedidoNoUsaElHoomViejo(t *testing.T) {
	bin := hbHoomReal(t)
	root := repo(t)
	write(t, root, htSpec, "# Spec: "+htSlug+"\n")
	viejo := filepath.Join(t.TempDir(), "viejo")
	if err := os.MkdirAll(viejo, 0o755); err != nil {
		t.Fatal(err)
	}
	sViejo, _ := hbHallazgo("20260924T120006_0ld0ld", "")
	if err := os.WriteFile(filepath.Join(viejo, "hoom"), []byte("#!/bin/sh\n"+sViejo+"exit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", viejo+":"+os.Getenv("PATH"))
	t.Setenv("HOOM_TASK", "ajena")
	marca := filepath.Join(t.TempDir(), "marca.txt") // fuera del arbol revisado
	fakeProvider(t, "codex",
		"hb=$(printf '%s\\n' \"$@\" | sed -n \"s/.*'\\([^']*\\)' finding add --sev.*/\\1/p\" | head -n 1)\n"+
			"if [ -z \"$hb\" ]; then echo 'sin comando citado en el pedido' > '"+marca+"'; exit 9; fi\n"+
			"echo \"$hb\" > '"+marca+"'\n"+
			"\"$hb\" finding add --sev medium --lens risk --file app.go --author reviewer@codex \"Nuevo() no tiene test (app.go:3)\" || exit 8\n"+
			"exit 0\n")

	var out bytes.Buffer
	res, err := Run(root, "main", Options{Lens: "risk", Provider: "codex", Spec: htSpec, HoomBin: bin}, &out)
	salida := out.String()
	vio, _ := os.ReadFile(marca)
	if err != nil || res.Status != "revisado" || res.RecordID == "" {
		t.Fatalf("CA-162: el reviewer que sigue el pedido corre el hoom citado (%s) y la review termina revisado;\n"+
			"el reviewer vio: %q\nerr=%v res=%+v\n%s", bin, vio, err, res, salida)
	}
	if strings.TrimSpace(string(vio)) != bin {
		t.Fatalf("CA-162: el comando citado del pedido es HoomBin=%s, fue %q", bin, vio)
	}
	if len(res.Findings) != 1 {
		t.Fatalf("CA-293: hoom ve un hallazgo del reviewer: %+v\n%s", res, salida)
	}
	id := res.Findings[0]
	f, ok := abHallazgoEn(t, id, root)
	if !ok {
		t.Fatalf("CA-293: el hallazgo %s quedo registrado", id)
	}
	if f.Task != htSlug {
		t.Fatalf("CA-293: el hoom del pedido (no el viejo del PATH) ata el hallazgo a %s: %+v", htSlug, f)
	}
	crudo, err := os.ReadFile(filepath.Join(root, ".hoom", "findings", id+".json"))
	if err != nil {
		t.Fatalf("CA-293: el artefacto del hallazgo %s: %v", id, err)
	}
	var m map[string]any
	if err := json.Unmarshal(crudo, &m); err != nil || m["task"] != htSlug {
		t.Fatalf("CA-293: el artefacto lleva \"task\": %q (%v):\n%s", htSlug, err, crudo)
	}
	hbSinAvisoDe(t, salida, id, htSlug)
	hbSinNotas(t, root, res.RecordID)
}
