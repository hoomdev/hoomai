// Tests del writer para los hallazgos de la delta 4 de costo-por-spec
// (registro 20261108T043012_eb69b9).
package costcmd

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Hallazgos 66c8c7 y c05fcb — CA-546: una copia que no cuenta guarda solo
// su huella, su fuente y su aviso; nunca el registro. Los worktrees cargan
// el mismo historial, asi que la memoria va con los registros, no con las
// copias.
func TestHallazgo_66c8c7_c05fcb_CA546_UnaCopiaNoRetieneElRegistro(t *testing.T) {
	var mirar func(camino string, ty reflect.Type)
	mirar = func(camino string, ty reflect.Type) {
		switch ty.Kind() {
		case reflect.Struct:
			for i := 0; i < ty.NumField(); i++ {
				mirar(camino+"."+ty.Field(i).Name, ty.Field(i).Type)
			}
		case reflect.Array:
			mirar(camino+"[]", ty.Elem())
		case reflect.String, reflect.Bool, reflect.Uint8, reflect.Int, reflect.Int64:
		default:
			t.Errorf("%s es %s: una copia no guarda nada que crezca con el registro", camino, ty)
		}
	}
	mirar("copia", reflect.TypeOf(copia{}))

	// a big record in the project and the same in four worktrees: one round,
	// nothing to say
	root := ccRoot(t)
	run := ccMeta("alfa", "reviewer", "codex", r1T0, ccMin(5))
	ccRun(t, root, run)
	lentes := make([]string, 20000)
	for i := range lentes {
		lentes[i] = strings.Repeat("l", 50)
	}
	rec := ccRegistro(t, root, "alfa", "completa", r1T0.Add(ccMin(6)), lentes, ccLensRuns([]string{"risk"}, run), nil)
	for _, wt := range []string{"alfa", "beta", "gama", "delta"} {
		ccWorktree(t, root, wt)
		d3Copiar(t, root, filepath.Join(root, ".hoom", "worktrees", wt), rec.ID, nil)
	}
	r := ccBuild(t, root, "alfa")
	if ta := ccTarea(t, "CA-546", r, "alfa"); ta.Rondas != 1 || len(ta.RondasReview[0].Lentes) != len(lentes) || len(r.Avisos) != 0 {
		t.Fatalf("copias identicas de un registro grande: una ronda y sin avisos: rondas %d avisos %q", ta.Rondas, r.Avisos)
	}
	// and a copy that differs is still said: a later one of the task, and
	// one another task's worktree carries
	cambio := func(s string) string { return strings.Replace(s, `"completa"`, `"delta"`, 1) }
	d3Copiar(t, root, filepath.Join(root, ".hoom", "worktrees", "alfa"), rec.ID, cambio)
	d3Copiar(t, root, filepath.Join(root, ".hoom", "worktrees", "gama"), rec.ID, cambio)
	r = ccBuild(t, root, "alfa")
	r1Avisa(t, r, `tiene otra copia, distinta, en el worktree "alfa"`)
	r1Avisa(t, r, `el worktree "gama" tiene el registro `+rec.ID)
	if len(r.Avisos) != 2 {
		t.Fatalf("solo las dos copias distintas se dicen: %q", r.Avisos)
	}
}

// Hallazgos 66c8c7 y c05fcb — el aviso de una tarea que no es un slug
// muestra a lo sumo maxEnAviso bytes de ella: un registro puede pesar
// 256 MiB, y su tarea con el.
func TestHallazgo_66c8c7_c05fcb_CA546_ElAvisoNoCargaLaTareaEntera(t *testing.T) {
	root := ccRoot(t)
	rec := ccRegistro(t, root, "alfa", "completa", r1T0, []string{"risk"}, nil, nil)
	larga := "Alfa" + strings.Repeat("ñ", 1<<19)
	d3Copiar(t, root, root, rec.ID, func(s string) string { return strings.Replace(s, `"task": "alfa"`, `"task": "`+larga+`"`, 1) })
	r, err := Build(root, "")
	if err != nil {
		t.Fatal(err)
	}
	r1Avisa(t, r, rec.ID)
	for _, a := range r.Avisos {
		if len(a) > 256 {
			t.Fatalf("un aviso de %d bytes: la tarea va recortada", len(a))
		}
		if strings.Contains(a, "no es un slug") && !strings.Contains(a, `"Alfañ`) {
			t.Fatalf("el aviso muestra el principio de la tarea: %q", a)
		}
	}
	for _, s := range []string{"", "corto", strings.Repeat("a", maxEnAviso), strings.Repeat("ñ", maxEnAviso)} {
		got := recortar(s)
		if len(got) > maxEnAviso+len("...") || !strings.HasPrefix(s, strings.TrimSuffix(got, "...")) {
			t.Fatalf("recortar(%q) = %q", s, got)
		}
		if len(s) <= maxEnAviso && got != s {
			t.Fatalf("lo que entra va entero: recortar(%q) = %q", s, got)
		}
	}
}
