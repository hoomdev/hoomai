// Tests adversariales del spec .hoom/specs/tablero-de-solo-lectura.md
// (CA-311, CA-312): spec.Criteria saca el enunciado de cada criterio de SU
// item en la seccion "criterios de aceptacion", y spec.IndexTokens recorre
// una vez los archivos de test y guarda, por token entero, quien lo cita.
// spec.Tokens pasa a ser el indice y da el mismo resultado de antes.
package spec

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// crSpec escribe un spec en root/.hoom/specs/x.md (fuera del escaneo de tests).
func crSpec(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	p := filepath.Join(root, ".hoom", "specs", "x.md")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const crCuerpo = `# Spec: x

## Objetivo
x

## No-goals
x

## Contratos
El limite de CA-7 se discute aca, y CA-3 tambien.

## Casos límite
x

## Criterios de aceptación

- CA-1: la primera linea del criterio
  sigue en una linea de continuacion
  y termina en otra.
- CA-2: se verifica por comando. [verifica: go test ./x]
- CA-10: el decimo, que depende de CA-9.
- CA-3: el tercero.

## Decisiones
- CA-5: esto es una decision, no un criterio.

## Riesgos
x
`

// crTextos arma id -> text y exige el orden de spec.Lint.
func crTextos(t *testing.T, path string) map[string]string {
	t.Helper()
	ids, _, issues, err := Lint(path)
	if err != nil || len(issues) != 0 {
		t.Fatalf("el spec de prueba pasa lint: %v %v", issues, err)
	}
	crit, err := Criteria(path)
	if err != nil {
		t.Fatalf("CA-311: Criteria no falla sobre un spec legible: %v", err)
	}
	var got []string
	out := map[string]string{}
	for _, c := range crit {
		got = append(got, c.ID)
		out[c.ID] = c.Text
	}
	if !reflect.DeepEqual(got, ids) {
		t.Fatalf("CA-311: un Criterion por id de spec.Lint, en su orden:\nlint:     %v\nCriteria: %v", ids, got)
	}
	return out
}

// CA-311: el enunciado sale del item de lista de la seccion de criterios
// que empieza con el token: continuaciones unidas por un espacio, sin el
// prefijo "CA-n:" y sin el marcador verifica. Un id citado en otra parte
// (contratos, dentro de otro criterio, un item de otra seccion) tiene text
// vacio.
func TestCA311_CriteriaEnunciados(t *testing.T) {
	txt := crTextos(t, crSpec(t, crCuerpo))
	quiere := map[string]string{
		"CA-1":  "la primera linea del criterio sigue en una linea de continuacion y termina en otra.",
		"CA-2":  "se verifica por comando.",
		"CA-3":  "el tercero.",
		"CA-5":  "",
		"CA-7":  "",
		"CA-9":  "",
		"CA-10": "el decimo, que depende de CA-9.",
	}
	for id, want := range quiere {
		got, ok := txt[id]
		if !ok {
			t.Fatalf("CA-311: falta el criterio %s: %v", id, txt)
		}
		if got != want {
			t.Errorf("CA-311: text de %s debe ser %q, fue %q", id, want, got)
		}
	}
	if len(txt) != len(quiere) {
		t.Fatalf("CA-311: exactamente los ids de spec.Lint: %v", txt)
	}
}

// CA-311: el marcador verifica en la linea del token se quita aunque el
// criterio siga en otra linea, y el encabezado se reconoce sin acentos y en
// mayusculas. Un criterio que no es el primero de la seccion tambien.
func TestCA311_CriteriaMarcadorYEncabezado(t *testing.T) {
	body := strings.NewReplacer(
		"## Criterios de aceptación", "## CRITERIOS DE ACEPTACION",
		"- CA-1: la primera linea del criterio\n", "- CA-1: la primera linea del criterio [verifica: true]\n",
	).Replace(crCuerpo)
	txt := crTextos(t, crSpec(t, body))
	norm := strings.Join(strings.Fields(txt["CA-1"]), " ")
	if norm != "la primera linea del criterio sigue en una linea de continuacion y termina en otra." {
		t.Fatalf("CA-311: sin el marcador verifica y con la continuacion unida: %q", txt["CA-1"])
	}
	if strings.Contains(txt["CA-1"], "verifica") || strings.Contains(txt["CA-1"], "[") {
		t.Fatalf("CA-311: el marcador verifica no queda en el texto: %q", txt["CA-1"])
	}
	if txt["CA-1"] != strings.TrimSpace(txt["CA-1"]) {
		t.Fatalf("CA-311: el texto va recortado: %q", txt["CA-1"])
	}
	if txt["CA-3"] != "el tercero." || txt["CA-5"] != "" {
		t.Fatalf("CA-311: con el encabezado en mayusculas y sin acento: %v", txt)
	}
}

// CA-311: sin la seccion de criterios, todo id tiene text vacio (el spec no
// pasa lint, pero Criteria igual responde por los ids de Lint); un archivo
// que no existe es error.
func TestCA311_CriteriaSinSeccionYSinArchivo(t *testing.T) {
	p := crSpec(t, "# Spec\n\n## Objetivo\n- CA-1: un item suelto fuera de la seccion.\n\nCA-2 citado.\n")
	crit, err := Criteria(p)
	if err != nil {
		t.Fatalf("CA-311: Criteria sobre un spec legible no falla: %v", err)
	}
	ids, _, _, _ := Lint(p)
	if len(crit) != len(ids) || len(crit) != 2 {
		t.Fatalf("CA-311: un Criterion por id de Lint (%v): %+v", ids, crit)
	}
	for _, c := range crit {
		if c.Text != "" {
			t.Fatalf("CA-311: fuera de la seccion de criterios no hay enunciado: %+v", c)
		}
	}
	if _, err := Criteria(filepath.Join(t.TempDir(), "no-existe.md")); err == nil {
		t.Fatal("CA-311: Criteria de un archivo inexistente es error")
	}
}

// crArbol arma un arbol con archivos de test y de los otros.
func crArbol(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	w := func(rel, body string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w("a_test.go", "package a\n// CA-12: cubre el doce\n// CA-3 tambien, y CA-3 otra vez\n")
	w("tests/b.txt", "CA-5 aca\n")
	w("z/c_spec.js", "// CA-3 y CA-100\n")
	w("app.go", "package app // CA-7 no es un archivo de test\n")
	w(".hoom/specs/x_test.md", "CA-9 en .hoom no cuenta\n")
	w("vendor/v_test.go", "CA-3 en vendor no cuenta\n")
	w("node_modules/m/m_test.js", "CA-3 en node_modules no cuenta\n")
	w("grande_test.txt", "CA-20\n"+strings.Repeat("x", 2<<20+10)+"\n")
	return root
}

// CA-312: el indice guarda por token ENTERO los archivos de test que lo
// citan: CA-1 no coincide dentro de CA-12 ni CA-10 dentro de CA-100, con el
// filtro de hoy (nombre con test o spec, sin skipDirs, hasta 2 MiB). Rutas
// relativas al arbol, con barras, ordenadas; nunca nil.
func TestCA312_IndexTokensPorTokenEntero(t *testing.T) {
	x, err := IndexTokens(crArbol(t))
	if err != nil {
		t.Fatalf("CA-312: IndexTokens no falla sobre un arbol legible: %v", err)
	}
	casos := map[string][]string{
		"CA-12":  {"a_test.go"},
		"CA-3":   {"a_test.go", "z/c_spec.js"},
		"CA-5":   {"tests/b.txt"},
		"CA-100": {"z/c_spec.js"},
		"CA-1":   {},
		"CA-10":  {},
		"CA-7":   {},
		"CA-9":   {},
		"CA-20":  {},
		"CA-999": {},
	}
	for id, want := range casos {
		got := x.Files(id)
		if got == nil {
			t.Fatalf("CA-312: Files(%s) nunca es nil", id)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("CA-312: Files(%s) debe ser %q, fue %q", id, want, got)
		}
	}
	if x.Scanned != 3 {
		t.Fatalf("CA-312: se leen 3 archivos de test (a_test.go, tests/b.txt, z/c_spec.js), fueron %d", x.Scanned)
	}
}

// crSkipDirs, crEsTest y crToken copian el filtro y el match de spec.Tokens
// tal como eran antes del indice (copiados, no importados: el writer puede
// reescribir los internos del paquete).
var crSkipDirs = map[string]bool{
	".git": true, ".hoom": true, "vendor": true, "node_modules": true,
	"dist": true, "build": true, "storage": true, ".gradle": true,
	".idea": true, ".vscode": true, "bootstrap": true,
}

func crEsTest(rel string) bool {
	l := strings.ToLower(rel)
	return strings.Contains(l, "test") || strings.Contains(l, "spec")
}

func crToken(s, id string) bool {
	idx := 0
	for {
		i := strings.Index(s[idx:], id)
		if i < 0 {
			return false
		}
		pos := idx + i
		end := pos + len(id)
		if end >= len(s) || s[end] < '0' || s[end] > '9' {
			return true
		}
		idx = end
	}
}

// crTokensViejo es spec.Tokens tal como era antes del indice: la referencia
// de "el mismo resultado que antes".
func crTokensViejo(root string, ids []string) (missing []string, scanned int, err error) {
	found := map[string]bool{}
	if len(ids) > 0 {
		err = filepath.WalkDir(root, func(path string, d os.DirEntry, werr error) error {
			if werr != nil {
				return nil
			}
			if d.IsDir() {
				if crSkipDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			if !crEsTest(rel) {
				return nil
			}
			info, ierr := d.Info()
			if ierr != nil || info.Size() > 2<<20 {
				return nil
			}
			raw, rerr := os.ReadFile(path)
			if rerr != nil {
				return nil
			}
			scanned++
			s := string(raw)
			for _, id := range ids {
				if !found[id] && crToken(s, id) {
					found[id] = true
				}
			}
			return nil
		})
		if err != nil {
			return nil, scanned, err
		}
	}
	for _, id := range ids {
		if !found[id] {
			missing = append(missing, id)
		}
	}
	return missing, scanned, nil
}

// CA-312: spec.Tokens da el mismo resultado que antes (faltantes en el orden
// recibido y archivos escaneados), tambien sin ids y sobre un arbol que no
// existe.
func TestCA312_TokensMismoResultadoQueAntes(t *testing.T) {
	arbol := crArbol(t)
	listas := [][]string{
		{"CA-1", "CA-3", "CA-5", "CA-7", "CA-9", "CA-12", "CA-20", "CA-100"},
		{"CA-100", "CA-10", "CA-1"},
		{"CA-12"},
		{"CA-404"},
		nil,
		{},
	}
	for _, root := range []string{arbol, filepath.Join(t.TempDir(), "no-existe")} {
		for _, ids := range listas {
			wm, ws, werr := crTokensViejo(root, ids)
			gm, gs, gerr := Tokens(root, ids)
			if (werr == nil) != (gerr == nil) {
				t.Fatalf("CA-312: Tokens(%v) falla igual que antes: antes %v, ahora %v", ids, werr, gerr)
			}
			if strings.Join(gm, ",") != strings.Join(wm, ",") || gs != ws {
				t.Fatalf("CA-312: Tokens(%s, %v) da lo mismo que antes:\nantes: missing=%v scanned=%d\nahora: missing=%v scanned=%d",
					filepath.Base(root), ids, wm, ws, gm, gs)
			}
		}
	}
}
