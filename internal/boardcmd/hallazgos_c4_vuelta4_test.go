// Tests de regresion de la cuarta vuelta de la review de la cabina C4 sobre
// .hoom/specs/historia-doctor-y-cinta.md, en boardcmd: el doctor del
// proyecto (pieza 3, CA-369 y CA-370). Escritos desde el spec y la
// descripcion del hallazgo, sin leer la implementacion; repos git reales en
// t.TempDir(), el binario real para 'hoom board doctor' y el arnes de
// hallazgos_c4_doctor_test.go (hoom y rm falsos, y pegar cada accion en
// /bin/sh -c). Los helpers de este archivo llevan el prefijo c4v4.
//
//   - 20261019T153725_7913f8 (risk): el doctor toma como espacio de trabajo
//     de 'hoom task' cualquier directorio de .hoom/worktrees, sea o no un
//     slug, y pone su nombre sin escapar en la accion 'cd <dir> && hoom
//     verify'. 'hoom task' solo crea espacios de trabajo con nombre de slug
//     (CA-263): un directorio cuyo nombre no es un slug no es un espacio de
//     trabajo de 'hoom task', no aparece en el doctor, y ninguna accion del
//     doctor, pegada en sh -c, ejecuta lo que traiga su nombre.
package boardcmd

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/item"
)

// c4v4NoSlugs son nombres de directorio de .hoom/worktrees que no son slugs
// (^[a-z0-9][a-z0-9-]*$, hasta 64). Los primeros crean PWNED si sh los
// interpreta; los demas son inofensivos en sh, pero tampoco son slugs.
var c4v4NoSlugs = []string{
	"x; touch PWNED; #",
	"$(touch PWNED)",
	"`touch PWNED`",
	"a && touch PWNED",
	"a | touch PWNED",
	"a\ntouch PWNED",
	"'; touch PWNED; '",
	"\"; touch PWNED; \"",
	"Mayus",
	"a_b",
	"-guion",
	"ñandu",
	strings.Repeat("x", 65),
}

// c4v4Guarda es el espacio de trabajo con slug valido que acompana a cada
// nombre hostil. Se llama "a" a proposito: con "a && touch PWNED" o
// "a\ntouch PWNED" sin escapar, el cd a .hoom/worktrees/a funciona y el
// touch corre.
const c4v4Guarda = "a"

var c4v4Ahora = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

// c4v4Espacio arma en root el espacio de trabajo .hoom/worktrees/<nombre>
// como lo arma 'hoom task' (git worktree add desde main, en su rama), con un
// archivo nuevo y sin veredicto, y devuelve la huella de su arbol. La rama
// se pasa aparte: git no acepta como rama todos los nombres de c4v4NoSlugs.
func c4v4Espacio(t *testing.T, root, nombre, rama string) string {
	t.Helper()
	wt := filepath.Join(root, ".hoom", "worktrees", nombre)
	bdGit(t, root, "worktree", "add", "-q", "-b", rama, wt, "main")
	bdEscribir(t, wt, "cambio.go", "package app\n")
	info := gitx.Snapshot(wt, "main")
	if len(info.ChangedFiles) == 0 || info.ChangeFingerprint == "" {
		t.Fatalf("fixture: el espacio de trabajo %q tiene cambios y huella: %+v", nombre, info)
	}
	return info.ChangeFingerprint
}

// c4v4SinVeredicto es el sin-veredicto del proyecto del contrato para el
// espacio de trabajo de slug, con o sin spec ahi.
func c4v4SinVeredicto(slug, huella string, conSpec bool) drProb {
	dir := ".hoom/worktrees/" + slug
	accion := "cd " + dir + " && hoom verify"
	if conSpec {
		accion += " --spec .hoom/specs/" + slug + ".md"
	}
	return drProb{ProbSinVeredicto, slug, drWhatSinVerProyecto(dir, huella), "", accion}
}

// CA-369 / CA-370 (hallazgo 7913f8): un directorio de .hoom/worktrees cuyo
// nombre no es un slug, armado como un espacio de trabajo de tarea con
// cambios y sin veredicto, no es un espacio de trabajo de 'hoom task': no
// aparece en ningun problema de Doctor (ni en su slug, ni en what, ni en la
// accion, ni en la ref) ni en el texto de 'hoom board doctor', y pegar en sh
// -c cada accion del doctor (con hoom y rm falsos) no crea PWNED. En el
// mismo proyecto, el espacio de trabajo de slug valido "a" con cambios sin
// veredicto es el unico problema, con su accion del contrato.
func TestCA369_CA370_7913f8_EspacioQueNoEsSlugNoLlegaAlDoctor(t *testing.T) {
	bin := bxBinario(t)
	for i, nombre := range c4v4NoSlugs {
		t.Run(fmt.Sprintf("%q", nombre), func(t *testing.T) {
			if item.ValidSlug(nombre) {
				t.Fatalf("fixture: %q no es un slug", nombre)
			}
			root := bdRepo(t, "findings:\n  block_on: high\n")
			huella := c4v4Espacio(t, root, c4v4Guarda, "hoom/"+c4v4Guarda)
			c4v4Espacio(t, root, nombre, fmt.Sprintf("hoom/no-slug-%d", i))

			r, err := Doctor(root, "main", "high", c4v4Ahora)
			if err != nil {
				t.Fatalf("CA-369: Doctor sobre el proyecto: %v", err)
			}
			for _, p := range r.Problems {
				for _, c := range [][2]string{{"slug", p.Slug}, {"what", p.What}, {"action", p.Action}, {"ref", p.Ref}} {
					if strings.Contains(c[1], nombre) {
						t.Errorf("CA-369 (hallazgo 7913f8): el directorio %q de .hoom/worktrees no es un slug ni un espacio de trabajo de 'hoom task', y aparece en el %s de un %s: %q",
							nombre, c[0], p.ID, c[1])
					}
				}
			}
			w := c4v4SinVeredicto(c4v4Guarda, huella, false)
			if len(r.Problems) != 1 || r.Problems[0].ID != w.id || r.Problems[0].Slug != w.slug || r.Problems[0].What != w.what ||
				r.Problems[0].Plain != w.plain || r.Problems[0].Action != w.action {
				t.Errorf("CA-369 (hallazgo 7913f8): el unico problema es el sin-veredicto del espacio de trabajo de slug valido %q\nquiere:\n%sfue:\n%s",
					c4v4Guarda, drFmtW([]drProb{w}), drFmt(r.Problems))
			}

			txt := bxCorrer(t, bin, root, "board", "doctor")
			if txt.exit != 0 || !strings.HasPrefix(txt.stdout, "hoom board doctor: 1 problema\n") || strings.Contains(txt.stdout, nombre) {
				t.Errorf("CA-370 (hallazgo 7913f8): 'hoom board doctor' sale 0 con 1 problema y no nombra %q (exit %d):\n%s%s",
					nombre, txt.exit, txt.stdout, txt.stderr)
			}

			acciones := hdAcciones(r, txt.stdout)
			falsos := hdFalsos(t)
			for _, a := range acciones {
				for _, l := range hdCandidatas(a) {
					falsos.pegar(t, root, l)
				}
			}
			if got := hdBuscar(t, root, "PWNED"); len(got) > 0 {
				t.Errorf("CA-369 (hallazgo 7913f8): pegar en sh -c las acciones del doctor ejecuto lo que trae el nombre %q: aparecio %v\nacciones:\n%s",
					nombre, got, hdFmtAcciones(acciones))
			}
		})
	}
}

// CA-369 (hallazgo 7913f8, guarda): un espacio de trabajo de 'hoom task' con
// nombre de slug valido (de 1 caracter, que empieza con un digito, con
// guiones, de 64 caracteres), con cambios y sin veredicto, sigue dando su
// sin-veredicto del proyecto con el what, el slug y la accion exactos del
// contrato (con --spec si el spec existe en el espacio de trabajo), y pegar
// esa accion en sh -c llama a hoom verify con esos argumentos y nada mas.
func TestCA369_7913f8_Guarda_EspacioConSlugSigueDandoSinVeredicto(t *testing.T) {
	root := bdRepo(t, "findings:\n  block_on: high\n")
	casos := []struct {
		slug    string
		conSpec bool
	}{
		{"a", false},
		{"9-tareas", false},
		{"suelto-con-spec", true},
		{strings.Repeat("s", 64), false},
	}
	want := map[string]drProb{}
	for _, cs := range casos {
		if !item.ValidSlug(cs.slug) {
			t.Fatalf("fixture: %q es un slug", cs.slug)
		}
		wt := bdWorktree(t, root, cs.slug)
		bdEscribir(t, wt, "cambio.go", "package app\n")
		if cs.conSpec {
			bdEscribir(t, wt, ".hoom/specs/"+cs.slug+".md", bdSpecTexto("- CA-1: algo.", true))
		}
		huella := gitx.Snapshot(wt, "main").ChangeFingerprint
		if huella == "" {
			t.Fatalf("fixture: el espacio de trabajo %s tiene huella", cs.slug)
		}
		want[cs.slug] = c4v4SinVeredicto(cs.slug, huella, cs.conSpec)
	}

	r, err := Doctor(root, "main", "high", c4v4Ahora)
	if err != nil {
		t.Fatalf("CA-369: Doctor sobre el proyecto: %v", err)
	}
	if len(r.Problems) != len(casos) {
		t.Fatalf("CA-369 (hallazgo 7913f8, guarda): un sin-veredicto por cada espacio de trabajo de slug valido (%d):\n%s", len(casos), drFmt(r.Problems))
	}
	for _, cs := range casos {
		w := want[cs.slug]
		var got *Problem
		for i := range r.Problems {
			if r.Problems[i].Slug == cs.slug {
				got = &r.Problems[i]
			}
		}
		if got == nil || got.ID != w.id || got.What != w.what || got.Plain != w.plain || got.Action != w.action {
			t.Fatalf("CA-369 (hallazgo 7913f8, guarda): el espacio de trabajo %s da su sin-veredicto del contrato\nquiere:\n%sfue:\n%s",
				cs.slug, drFmtW([]drProb{w}), drFmt(r.Problems))
		}
		drRefCon(t, "CA-369", "espacio de slug valido", *got, ".hoom/worktrees/"+cs.slug)

		falsos := hdFalsos(t)
		falsos.pegar(t, root, got.Action)
		llamada := []string{"hoom", "verify"}
		if cs.conSpec {
			llamada = append(llamada, "--spec", ".hoom/specs/"+cs.slug+".md")
		}
		if ll := falsos.llamadas(t); len(ll) != 1 || !reflect.DeepEqual(ll[0], llamada) {
			t.Fatalf("CA-369 (hallazgo 7913f8, guarda): pegar la accion de %s (%q) llama a %q y nada mas, fue %q", cs.slug, got.Action, llamada, ll)
		}
	}
	if got := hdBuscar(t, root, "PWNED"); len(got) > 0 {
		t.Fatalf("CA-369 (hallazgo 7913f8, guarda): las acciones de los espacios de slug valido no crean nada: %v", got)
	}
}
