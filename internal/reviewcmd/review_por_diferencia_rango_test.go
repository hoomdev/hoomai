// Tests adversariales del spec .hoom/specs/review-por-diferencia.md
// (CA-421, CA-422): los errores de --desde y --delta, su orden, el rango sin
// nada que revisar, y de donde sale el comienzo de un --delta. Los fixtures
// estan en review_por_diferencia_helpers_test.go.
package reviewcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------- CA-421

// CA-421: "--desde que no es un commit ... da su error sin pasadas ni
// registro". Un nombre que no existe, un sha de 40 hex que no esta en el
// repositorio, el arbol de HEAD y un blob (objetos que existen pero no son
// commits): `--desde <c>: no es un commit de este repositorio`, con <c> tal
// como se paso. Con un cambio de codigo y con uno solo de documentacion (el
// --desde se valida antes de medir: nunca termina SIN REVISAR).
func TestCA421_DesdeQueNoEsUnCommit(t *testing.T) {
	for _, docs := range []bool{false, true} {
		bin := raPATH(t)
		root := raRepo(t, "")
		if docs {
			raDocs(t, root)
		} else {
			raCambio(t, root)
		}
		cx := raInstalar(t, bin, "codex", "")
		arbol := rdGit(t, root, "rev-parse", "HEAD^{tree}")
		blob := rdGit(t, root, "rev-parse", "HEAD:app.go")
		for _, c := range []string{"no-existe-rd", strings.Repeat("0f", 20), arbol, blob} {
			caso := c
			if docs {
				caso += " (solo documentacion)"
			}
			raLimpio(t, "CA-421", root)
			res, err, out := rdCorrer(t, "CA-421", root, Options{Provider: "codex", Desde: c})
			msg := rdNegada(t, "CA-421", caso, root, cx, 0, 0, res, err, out, rdErrNoCommit(c))
			if raDiceSinRevisar(msg) {
				t.Fatalf("CA-421: %s: un --desde invalido no termina SIN REVISAR:\n%s", caso, msg)
			}
		}
	}
}

// CA-421: "--desde ... que no es ancestro de HEAD": un commit de main
// posterior a la rama y uno de otra rama: `--desde <c>: no es un ancestro de
// HEAD (la review revisa de <c> a HEAD)`, sin pasadas ni registro.
func TestCA421_DesdeQueNoEsAncestroDeHEAD(t *testing.T) {
	bin := raPATH(t)
	root, _, _, _ := rdRamaDos(t, "", "")
	git(t, root, "checkout", "-q", "main")
	write(t, root, "main.go", "package app\n\nvar Main = 1\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "main avanza")
	deMain := rdSha(t, root, "HEAD")
	git(t, root, "checkout", "-q", "-b", "otra")
	write(t, root, "otra.go", "package app\n\nvar Otra = 1\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "otra rama")
	deOtra := rdSha(t, root, "HEAD")
	git(t, root, "checkout", "-q", "feature")
	cx := raInstalar(t, bin, "codex", "")

	for _, c := range []string{deMain, deOtra} {
		raLimpio(t, "CA-421", root)
		res, err, out := rdCorrer(t, "CA-421", root, Options{Provider: "codex", Desde: c})
		rdNegada(t, "CA-421", "no ancestro "+c[:12], root, cx, 0, 0, res, err, out, rdErrNoAncestro(c))
	}
}

// CA-421: "un valor que empieza con - se rechaza sin llamar a git". Valores
// que git tomaria como opciones (uno de ellos escribiria un archivo con
// --output): el error de --desde que nombra el valor, sin pasadas ni
// registro, sin que ningun git reciba el valor y sin crear el archivo.
func TestCA421_DesdeQueEmpiezaConGuionNoLlegaAGit(t *testing.T) {
	real := raGitReal(t)
	bin := raPATH(t)
	root, _, _, _ := rdRamaDos(t, "", "")
	cx := raInstalar(t, bin, "codex", "")
	pwned := filepath.Join(t.TempDir(), "pwned-ca421")
	// valores unicos (salvo "-"): si alguno aparece en un argumento de git,
	// es porque el valor llego a git
	for _, v := range []string{"--output=" + pwned, "--rd-ca421-opcion", "-rdCA421", "-"} {
		log := rdGitEspia(t, bin, real)
		raLimpio(t, "CA-421", root)
		res, err, out := rdCorrer(t, "CA-421", root, Options{Provider: "codex", Desde: v})
		rdNegada(t, "CA-421", "desde "+v, root, cx, 0, 0, res, err, out, "--desde "+v+": ")
		if rdGitRecibio(t, log, v) && v != "-" {
			t.Fatalf("CA-421: un --desde que empieza con '-' (%q) se rechaza sin llamar a git; git lo recibio", v)
		}
		if _, err := os.Stat(pwned); err == nil {
			t.Fatalf("CA-421: --desde %q no llega a git: se creo %s", v, pwned)
		}
	}
}

// CA-421 (Orden): "guarda de arbol sucio -> hoom.yaml -> base -> --desde o
// --delta". Con el arbol sucio y un --desde invalido (no es un commit, no es
// ancestro, empieza con '-') o un --delta sin registros: la negativa de arbol
// sucio que nombra la ruta, nada sin commitear en la salida, sin pasadas.
func TestCA421_ArbolSucioVaAntesQueElDesde(t *testing.T) {
	bin := raPATH(t)
	root, _, _, _ := rdRamaDos(t, "", ".hoom/specs/x.md")
	git(t, root, "checkout", "-q", "main")
	write(t, root, "main.go", "package app\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "main avanza")
	deMain := rdSha(t, root, "HEAD")
	git(t, root, "checkout", "-q", "feature")
	cx := raInstalar(t, bin, "codex", "")
	const oculto = "SIN-COMMITEAR-CA421-ZX92"
	write(t, root, "nuevo.go", "package app\n\n// "+oculto+"\n")
	for _, opt := range []Options{
		{Provider: "codex", Desde: "no-existe-rd"},
		{Provider: "codex", Desde: deMain},
		{Provider: "codex", Desde: "--output=/dev/null"},
		{Provider: "codex", Delta: true, Spec: ".hoom/specs/x.md"},
	} {
		res, err, out := rdCorrer(t, "CA-421", root, opt)
		msg := rdNegada(t, "CA-421", "arbol sucio", root, cx, 0, 0, res, err, out, raErrSucio("nuevo.go"))
		for _, otro := range []string{"no es un commit", "no es un ancestro", "--delta: la tarea", oculto} {
			if strings.Contains(msg, otro) {
				t.Fatalf("CA-421: con el arbol sucio va primero la negativa de arbol sucio, no %q (%+v):\n%s", otro, opt, msg)
			}
		}
	}
}

// CA-421 (Orden): hoom.yaml y la base van antes que --desde: con un
// hoom.yaml de la base invalido, o una base que no existe, y un --desde que
// no es un commit, el error es el de hoom.yaml o el de git, no el de
// --desde; nunca SIN REVISAR, sin pasadas.
func TestCA421_HoomYamlYBaseVanAntesQueElDesde(t *testing.T) {
	bin := raPATH(t)
	cx := raInstalar(t, bin, "codex", "")

	root := raRepo(t, "review:\n  max_evidence_kib: 16385\n")
	raCambio(t, root)
	raLimpio(t, "CA-421", root)
	res, err, out := rdCorrer(t, "CA-421", root, Options{Provider: "codex", Desde: "no-existe-rd"})
	msg := rdNegada(t, "CA-421", "hoom.yaml invalido", root, cx, 0, 0, res, err, out, "review: max_evidence_kib no puede pasar de 16384")
	if strings.Contains(msg, rdErrNoCommit("no-existe-rd")) {
		t.Fatalf("CA-421: hoom.yaml va antes que --desde:\n%s", msg)
	}

	root = raConBaseInexistente(t)
	raCambio(t, root)
	raLimpio(t, "CA-421", root)
	res, err, out = raRunBaseConReloj(t, "CA-421", 60*time.Second, root, "rama-que-no-existe", Options{Provider: "codex", Desde: "no-existe-rd"})
	msg = raMsg(out, err)
	if err == nil || len(res.Passes) != 0 || raDiceSinRevisar(msg) || cx.veces() != 0 {
		t.Fatalf("CA-421: con la base rota el error es el de git, sin pasadas ni SIN REVISAR: %+v %v\n%s", res, err, out)
	}
	if strings.Contains(msg, rdErrNoCommit("no-existe-rd")) {
		t.Fatalf("CA-421: la base va antes que --desde: el error es el de git, no el de --desde:\n%s", msg)
	}
}

// CA-421: "un rango sin cambios en las rutas de la evidencia ... termina SIN
// REVISAR con exit 0 y sin registro": --desde HEAD, y un rango que solo toca
// .hoom/ fuera de .hoom/agents/ (el spec, una nota), dicen
// `SIN REVISAR - no hay cambios desde <desde12>: no hay nada que revisar`.
// Aunque el cambio entero sea grande (la regla sobre el cambio entero pediria
// las 4): no hay nada que revisar en el rango.
func TestCA421_RangoSinCambiosEsSinRevisar(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	rdCommit(t, root, "grande", map[string]string{"grande.go": rdCodigo(450, "Grande")})
	cx := raInstalar(t, bin, "codex", "")

	head := rdSha(t, root, "HEAD")
	for _, c := range []string{"HEAD", head, head[:12]} {
		raLimpio(t, "CA-421", root)
		res, err, out := rdCorrer(t, "CA-421", root, Options{Provider: "codex", Desde: c})
		rdSinRevisar(t, "CA-421", "desde "+c, root, cx, 0, 0, res, err, out, rdSinCambios(head[:12]))
	}

	c := head
	rdCommit(t, root, "solo .hoom", map[string]string{
		".hoom/specs/x.md": "# Spec x\n\ncambio solo del spec\n",
		".hoom/notas.txt":  "una nota\n",
	})
	raLimpio(t, "CA-421", root)
	res, err, out := rdCorrer(t, "CA-421", root, Options{Provider: "codex", Desde: c})
	rdSinRevisar(t, "CA-421", "solo .hoom/", root, cx, 0, 0, res, err, out, rdSinCambios(c[:12]))
}

// CA-421: un rango cuyo cambio en las rutas de la evidencia es solo
// documentacion (README.md, docs/) termina
// `SIN REVISAR - lo que cambio desde <desde12> es solo documentacion: no se invoca review`,
// exit 0, sin registro, aunque el cambio entero tenga 450 lineas de codigo.
func TestCA421_RangoSoloDocumentacionEsSinRevisar(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	c := rdCommit(t, root, "grande", map[string]string{"grande.go": rdCodigo(450, "Grande")})
	rdCommit(t, root, "docs", map[string]string{"README.md": "# demo\n\nmas prosa\n", "docs/guia.md": "# guia\n"})
	cx := raInstalar(t, bin, "codex", "")
	res, err, out := rdCorrer(t, "CA-421", root, Options{Provider: "codex", Desde: c})
	rdSinRevisar(t, "CA-421", "solo documentacion", root, cx, 0, 0, res, err, out, rdSoloDocs(c[:12]))
}

// ---------------------------------------------------------------- CA-422

// rdDeltaRepo arma la rama feature con el spec .hoom/specs/x.md (la tarea
// x) y un primer commit de codigo (A, MARCA-DE-ANTES).
func rdDeltaRepo(t *testing.T) (root string) {
	t.Helper()
	root = raRepo(t, "")
	rdCommit(t, root, "A", map[string]string{
		".hoom/specs/x.md": "# Spec x\n\n- CA-1: algo.\n",
		"antes.go":         "package app\n\n// MARCA-DE-ANTES\nfunc Antes() {}\n",
	})
	return root
}

// CA-422: "--delta revisa desde el hasta del registro mas nuevo de la tarea
// completa o delta cuyo hasta es ancestro de HEAD". Una review sin rango
// (completa) en A; un commit B con codigo nuevo; --delta revisa de A a B: el
// pedido trae solo lo de B (la evidencia del contrato para ese rango), y el
// registro dice desde = el hasta de la primera, desde_review = su id,
// cobertura delta y hasta = B. Otro commit C y --delta de nuevo: arranca en
// el hasta del delta (el mas nuevo), no en el de la completa.
func TestCA422_DeltaRevisaDesdeElHastaDeLaUltimaReview(t *testing.T) {
	bin := raPATH(t)
	const spec = ".hoom/specs/x.md"
	root := rdDeltaRepo(t)
	cx := raInstalar(t, bin, "codex", "")

	r1, out := rdRevisar(t, "CA-422", root, Options{Provider: "codex", Spec: spec})
	if r1.Status != "revisado" || r1.RecordID == "" || r1.Cobertura != CoberturaCompleta {
		t.Fatalf("CA-422/CA-423: la primera review (sin rango) es completa y deja registro: %+v\n%s", r1, out)
	}
	h1 := rdSha(t, root, "HEAD")
	rdEsperarSegundo()
	b := rdCommit(t, root, "B", map[string]string{"nuevo.go": "package app\n\n// MARCA-DEL-DELTA\nfunc Nuevo() {}\n"})
	ev := rdOraculo(t, "CA-422", root, h1, spec)

	r2, out := rdRevisar(t, "CA-422", root, Options{Provider: "codex", Spec: spec, Delta: true})
	if r2.Status != "revisado" || cx.veces() != 2 {
		t.Fatalf("CA-422: --delta revisa: %+v\n%s", r2, out)
	}
	ped := cx.pedido(t, 2)
	if !strings.Contains(ped, raBloque(t, ev, spec, true)) || strings.Contains(ped, "MARCA-DE-ANTES") || !strings.Contains(ped, "MARCA-DEL-DELTA") {
		t.Fatalf("CA-422: el pedido de --delta trae solo lo que cambio desde el hasta de la review anterior (%s):\n%s", h1[:12], ped)
	}
	rec := rdRegistro(t, "CA-422", root, r2.RecordID)
	if rec.Desde != h1 || rec.Hasta != b || rec.Cobertura != CoberturaDelta || rec.DesdeReview != r1.RecordID {
		t.Fatalf("CA-422: el registro del delta dice desde %s, hasta %s, delta, desde_review %s: %+v", h1[:12], b[:12], r1.RecordID, rec)
	}

	rdEsperarSegundo()
	c := rdCommit(t, root, "C", map[string]string{"otro.go": "package app\n\n// MARCA-DE-C\nfunc Otro() {}\n"})
	r3, out := rdRevisar(t, "CA-422", root, Options{Provider: "codex", Spec: spec, Delta: true})
	rec = rdRegistro(t, "CA-422", root, r3.RecordID)
	if rec.Desde != b || rec.Hasta != c || rec.Cobertura != CoberturaDelta || rec.DesdeReview != r2.RecordID {
		t.Fatalf("CA-422: el segundo --delta arranca en el hasta del delta (el mas nuevo, %s): %+v\n%s", b[:12], rec, out)
	}
	if ped := cx.pedido(t, 3); strings.Contains(ped, "MARCA-DEL-DELTA") || !strings.Contains(ped, "MARCA-DE-C") {
		t.Fatalf("CA-422: el segundo --delta revisa solo C:\n%s", ped)
	}
}

// CA-422: "Si hay varios con ese hasta, el mas nuevo": dos reviews completas
// sobre el mismo HEAD; el --delta encadena con la segunda. Y un registro
// parcial mas nuevo (un --desde a mano) no sirve para encadenar: el delta
// sigue desde la completa.
func TestCA422_DeltaEncadenaConElMasNuevoYSaltaLosParciales(t *testing.T) {
	bin := raPATH(t)
	const spec = ".hoom/specs/x.md"
	root := rdDeltaRepo(t)
	rdCommit(t, root, "A2", map[string]string{"antes2.go": "package app\n\nfunc Antes2() {}\n"})
	raInstalar(t, bin, "codex", "")

	primera, _ := rdRevisar(t, "CA-422", root, Options{Provider: "codex", Spec: spec})
	rdEsperarSegundo()
	segunda, _ := rdRevisar(t, "CA-422", root, Options{Provider: "codex", Spec: spec})
	if primera.RecordID == segunda.RecordID || segunda.Cobertura != CoberturaCompleta {
		t.Fatalf("CA-422/CA-423: dos completas sobre el mismo HEAD: %+v %+v", primera, segunda)
	}
	h := rdSha(t, root, "HEAD")
	rdEsperarSegundo()
	parcial, _ := rdRevisar(t, "CA-422", root, Options{Provider: "codex", Spec: spec, Desde: rdSha(t, root, "HEAD~1")})
	if parcial.Cobertura != CoberturaParcial {
		t.Fatalf("CA-422/CA-423: un --desde a mano deja un parcial: %+v", parcial)
	}
	rdEsperarSegundo()
	rdCommit(t, root, "B", map[string]string{"nuevo.go": "package app\n\nfunc Nuevo() {}\n"})

	res, out := rdRevisar(t, "CA-422", root, Options{Provider: "codex", Spec: spec, Delta: true})
	rec := rdRegistro(t, "CA-422", root, res.RecordID)
	if rec.Cobertura != CoberturaDelta || rec.Desde != h || rec.DesdeReview != segunda.RecordID {
		t.Fatalf("CA-422: con dos completas con el mismo hasta el delta encadena con la mas nueva (%s), no con el parcial: %+v\n%s",
			segunda.RecordID, rec, out)
	}
}

// CA-422: "cuyo hasta es ancestro de HEAD": despues de reescribir la
// historia (reset --hard + commit nuevo), el registro mas nuevo queda con su
// hasta fuera de la historia; el --delta encadena con uno mas viejo cuyo
// hasta sigue siendo ancestro.
func TestCA422_DeltaSaltaLosHastaFueraDeLaHistoria(t *testing.T) {
	bin := raPATH(t)
	const spec = ".hoom/specs/x.md"
	root := rdDeltaRepo(t)
	raInstalar(t, bin, "codex", "")

	viejo, _ := rdRevisar(t, "CA-422", root, Options{Provider: "codex", Spec: spec})
	h1 := rdSha(t, root, "HEAD")
	rdEsperarSegundo()
	rdCommit(t, root, "B", map[string]string{"b.go": "package app\n\nfunc B() {}\n"})
	nuevo, _ := rdRevisar(t, "CA-422", root, Options{Provider: "codex", Spec: spec})
	if nuevo.Cobertura != CoberturaCompleta {
		t.Fatalf("CA-422/CA-423: la review sin rango es completa: %+v", nuevo)
	}
	git(t, root, "reset", "-q", "--hard", h1)
	rdEsperarSegundo()
	rdCommit(t, root, "B reescrito", map[string]string{"b.go": "package app\n\nfunc BReescrito() {}\n"})

	res, out := rdRevisar(t, "CA-422", root, Options{Provider: "codex", Spec: spec, Delta: true})
	rec := rdRegistro(t, "CA-422", root, res.RecordID)
	if rec.Desde != h1 || rec.DesdeReview != viejo.RecordID || rec.Cobertura != CoberturaDelta {
		t.Fatalf("CA-422: el delta salta el registro cuyo hasta ya no es ancestro de HEAD y encadena con %s: %+v\n%s", viejo.RecordID, rec, out)
	}
}

// CA-422: "sin uno asi (ningun registro, solo registros sin hasta, parcial,
// o con hasta fuera de la historia) da el error de --delta sin pasadas". Mas
// los hostiles: un registro de otra tarea, uno parcial por una --lens a mano
// que no cubre la regla, y uno de la tarea que esta en OTRO arbol (los
// registros se buscan en el mismo arbol donde se escriben). Siempre
// `--delta: la tarea <t> no tiene una review completa o delta que llegue a
// este HEAD: corre hoom review sin --delta`, sin pasadas ni registro, nunca
// SIN REVISAR.
func TestCA422_DeltaSinRegistroQueEncadeneEsError(t *testing.T) {
	const spec = ".hoom/specs/x.md"
	casos := []struct {
		nombre string
		armar  func(t *testing.T) (root string, opt Options)
	}{
		{"sin-registros", func(t *testing.T) (string, Options) {
			root := rdDeltaRepo(t)
			return root, Options{Provider: "codex", Spec: spec, Delta: true}
		}},
		{"sin-registros-solo-documentacion", func(t *testing.T) (string, Options) {
			root := raRepo(t, "")
			rdCommit(t, root, "docs", map[string]string{".hoom/specs/x.md": "# x\n", "README.md": "# demo\n\nprosa\n"})
			return root, Options{Provider: "codex", Spec: spec, Delta: true}
		}},
		{"solo-registros-viejos-sin-hasta", func(t *testing.T) (string, Options) {
			root := rdDeltaRepo(t)
			rdRegistroViejoAMano(t, root, "20260920T100000_v1ej0a", "x", Lentes)
			rdRegistroViejoAMano(t, root, "20260920T110000_v1ej0b", "x", []string{LenteDominante})
			return root, Options{Provider: "codex", Spec: spec, Delta: true}
		}},
		{"solo-un-parcial-por-desde", func(t *testing.T) (string, Options) {
			root := rdDeltaRepo(t)
			rdCommit(t, root, "B", map[string]string{"b.go": "package app\n\nfunc B() {}\n"})
			rdRevisar(t, "CA-422", root, Options{Provider: "codex", Spec: spec, Desde: rdSha(t, root, "HEAD~1")})
			rdCommit(t, root, "C", map[string]string{"c.go": "package app\n\nfunc C() {}\n"})
			return root, Options{Provider: "codex", Spec: spec, Delta: true}
		}},
		{"solo-un-parcial-por-lens-a-mano", func(t *testing.T) (string, Options) {
			root := rdDeltaRepo(t)
			rdRevisar(t, "CA-422", root, Options{Provider: "codex", Spec: spec, Lens: "risk"})
			rdCommit(t, root, "C", map[string]string{"c.go": "package app\n\nfunc C() {}\n"})
			return root, Options{Provider: "codex", Spec: spec, Delta: true}
		}},
		{"hasta-fuera-de-la-historia", func(t *testing.T) (string, Options) {
			root := rdDeltaRepo(t)
			rdCommit(t, root, "B", map[string]string{"b.go": "package app\n\nfunc B() {}\n"})
			rdRevisar(t, "CA-422", root, Options{Provider: "codex", Spec: spec})
			git(t, root, "reset", "-q", "--hard", "HEAD~1")
			rdCommit(t, root, "B reescrito", map[string]string{"b.go": "package app\n\nfunc BReescrito() {}\n"})
			return root, Options{Provider: "codex", Spec: spec, Delta: true}
		}},
		{"solo-un-registro-de-otra-tarea", func(t *testing.T) (string, Options) {
			root := rdDeltaRepo(t)
			rdRevisar(t, "CA-422", root, Options{Provider: "codex", Spec: ".hoom/specs/otra.md"})
			rdCommit(t, root, "C", map[string]string{"c.go": "package app\n\nfunc C() {}\n"})
			return root, Options{Provider: "codex", Spec: spec, Delta: true}
		}},
		{"el-registro-esta-en-otro-arbol", func(t *testing.T) (string, Options) {
			root := raRepo(t, "")
			wt := cbTarea(t, root, "x")
			ancestro := rdSha(t, wt, "HEAD")
			rdCommit(t, wt, "mas codigo en la tarea", map[string]string{"t.go": "package app\n\nfunc T() {}\n"})
			rdRegistroAMano(t, root, Record{ID: "20260927T100000_0tr0ar", CreatedAt: time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC),
				Task: "x", Spec: spec, Lenses: Lentes, Provider: "codex", Cross: CrossYes, Findings: []string{},
				WritersDeclared: []string{}, Usage: []LensUsage{}, Desde: rdSha(t, root, "main"), Hasta: ancestro, Cobertura: CoberturaCompleta})
			return root, Options{Provider: "codex", Task: "x", Delta: true}
		}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			bin := raPATH(t)
			cx := raInstalar(t, bin, "codex", "")
			root, opt := c.armar(t)
			dir := root
			if opt.Task != "" {
				dir = filepath.Join(root, ".hoom", "worktrees", opt.Task)
			}
			raLimpio(t, "CA-422", dir)
			veces, registros := cx.veces(), rdRegistrosEn(dir)
			res, err, out := rdCorrer(t, "CA-422", root, opt)
			msg := rdNegada(t, "CA-422", c.nombre, dir, cx, veces, registros, res, err, out, rdErrDelta("x"))
			if raDiceSinRevisar(msg) {
				t.Fatalf("CA-422: %s: el error de --delta no es SIN REVISAR:\n%s", c.nombre, msg)
			}
		})
	}
}

// CA-422 (caso limite): "--delta con hasta = HEAD: SIN REVISAR - no hay
// cambios desde ...", exit 0, sin registro. Tambien cuando despues de la
// review solo se commiteo .hoom/ (el registro mismo, un spec).
func TestCA422_DeltaSinCambiosEsSinRevisar(t *testing.T) {
	bin := raPATH(t)
	const spec = ".hoom/specs/x.md"
	root := rdDeltaRepo(t)
	cx := raInstalar(t, bin, "codex", "")
	rdRevisar(t, "CA-422", root, Options{Provider: "codex", Spec: spec})
	h := rdSha(t, root, "HEAD")

	res, err, out := rdCorrer(t, "CA-422", root, Options{Provider: "codex", Spec: spec, Delta: true})
	rdSinRevisar(t, "CA-422", "hasta = HEAD", root, cx, 1, 1, res, err, out, rdSinCambios(h[:12]))

	raCommit(t, root, "el registro de la review y el spec")
	write(t, root, spec, "# Spec x\n\n- CA-1: algo.\n- CA-2: mas.\n")
	raCommit(t, root, "spec")
	res, err, out = rdCorrer(t, "CA-422", root, Options{Provider: "codex", Spec: spec, Delta: true})
	rdSinRevisar(t, "CA-422", "solo .hoom/ despues", root, cx, 1, 1, res, err, out, rdSinCambios(h[:12]))
}

// CA-422: la tarea de la review es la de --task y los registros se buscan en
// el arbol donde se escriben (el worktree de la tarea): una review completa
// con --task deja su registro en el worktree y el --delta con --task
// encadena con el.
func TestCA422_DeltaConTareaBuscaEnSuWorktree(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	wt := cbTarea(t, root, "precios")
	raInstalar(t, bin, "codex", "")
	r1, out := rdRevisar(t, "CA-422", root, Options{Provider: "codex", Task: "precios"})
	if r1.Cobertura != CoberturaCompleta || r1.RecordID == "" {
		t.Fatalf("CA-422/CA-423: la review sin rango de la tarea es completa: %+v\n%s", r1, out)
	}
	h1 := rdSha(t, wt, "HEAD")
	rdEsperarSegundo()
	b := rdCommit(t, wt, "mas codigo", map[string]string{"t.go": "package app\n\nfunc T() {}\n"})
	r2, out := rdRevisar(t, "CA-422", root, Options{Provider: "codex", Task: "precios", Delta: true})
	rec := rdRegistro(t, "CA-422", wt, r2.RecordID)
	if rec.Cobertura != CoberturaDelta || rec.Desde != h1 || rec.Hasta != b || rec.DesdeReview != r1.RecordID || rec.Task != "precios" {
		t.Fatalf("CA-422: --delta --task encadena con la review del worktree: %+v\n%s", rec, out)
	}
}
