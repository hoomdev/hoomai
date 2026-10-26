// Tests de regresion de la segunda ronda de endurecimiento-2 en envelope.
// Escritos desde el texto del hallazgo y el contrato de la correccion, SIN
// LEER LA IMPLEMENTACION. Reusan los helpers e1* (endurecimiento-1); los de
// este archivo llevan el prefijo e2b.
//
//   - 20261026T20*_4f418f (high; follow-up de 28f494): 28f494 protegio el
//     reintento de un cierre, pero el PRIMER Write (y con el el primer
//     intento de Cerrar) seguia escribiendo por ruta: si .hoom o
//     .hoom/envelopes era un enlace simbolico a otro proyecto, creaba
//     directorios y publicaba el registro alla, y alteraba la telemetria
//     ajena. Ahora no escribe en el otro proyecto y devuelve error; en un
//     proyecto sin .hoom (o sin carpeta), Write la crea y escribe como
//     siempre.
package envelope

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// e2bFoto es todo lo que hay bajo dir: cada ruta relativa con su tipo y, si
// es un archivo, su contenido.
func e2bFoto(t *testing.T, dir string) map[string]string {
	t.Helper()
	foto := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		fi, err := os.Lstat(p)
		if err != nil {
			return err
		}
		v := fi.Mode().Type().String()
		if fi.Mode().IsRegular() {
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			v += " " + string(b)
		}
		foto[rel] = v
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return foto
}

// e2bDiferencias describe en que difieren dos fotos.
func e2bDiferencias(antes, despues map[string]string) []string {
	var out []string
	for p, v := range despues {
		if w, ok := antes[p]; !ok {
			out = append(out, "aparecio "+p)
		} else if w != v {
			out = append(out, "cambio "+p)
		}
	}
	for p := range antes {
		if _, ok := despues[p]; !ok {
			out = append(out, "desaparecio "+p)
		}
	}
	sort.Strings(out)
	return out
}

// CA-333, CA-202 (hallazgo 4f418f): en un proyecto cuyo .hoom/envelopes (o
// .hoom entero) es un enlace simbolico, absoluto o relativo, al de otro
// proyecto, el PRIMER Write de un registro, y el primer intento de Cerrar,
// no escriben en el otro proyecto: todo lo que hay bajo su .hoom queda
// identico (el <id>.json del mismo id que ya tenia, su .gitignore, y nada
// nuevo; o, si no tenia ese registro, sigue sin tenerlo), y Write y Cerrar
// devuelven error. Despues de un Cerrar que fallo, durante 1.5 s (el primer
// reintento es a 1 s) tampoco.
func TestHallazgo_4f418f_CA333_ElPrimerWriteNoEscribeFueraDelProyecto(t *testing.T) {
	const ca = "CA-333 (hallazgo 4f418f, primer Write con .hoom enlazado a otro proyecto)"
	for _, op := range []string{"Write", "Cerrar"} {
		for _, que := range []string{".hoom/envelopes", ".hoom"} {
			for _, relativo := range []bool{false, true} {
				for _, tiene := range []bool{true, false} {
					nombre := fmt.Sprintf("%s, %s enlace %s, el otro proyecto %s", op, que,
						map[bool]string{false: "absoluto", true: "relativo"}[relativo],
						map[bool]string{true: "tiene el registro", false: "no tiene el registro"}[tiene])
					t.Run(nombre, func(t *testing.T) {
						t.Parallel()
						base, err := filepath.EvalSymlinks(t.TempDir())
						if err != nil {
							t.Fatal(err)
						}
						root, otro := filepath.Join(base, "proyecto"), filepath.Join(base, "otro")
						rec := e1Rec(e1IDPropio, "propio")
						ajeno := e1Rec(e1IDPropio, "ajeno")
						ajeno.Note = "registro del otro proyecto: el primer Write no lo toca"
						if !tiene {
							ajeno = e1Rec(e1IDAjeno, "ajeno")
						}
						e1Escribir(t, otro, ajeno)

						enlace, destino := filepath.Join(root, ".hoom", DirName), filepath.Join(otro, ".hoom", DirName)
						rel := filepath.Join("..", "..", "otro", ".hoom", DirName)
						if que == ".hoom" {
							enlace, destino = filepath.Join(root, ".hoom"), filepath.Join(otro, ".hoom")
							rel = filepath.Join("..", "otro", ".hoom")
						}
						if relativo {
							destino = rel
						}
						e1Enlace(t, destino, enlace)
						if _, err := os.Stat(filepath.Join(root, ".hoom", DirName)); err != nil {
							t.Fatalf("%s: fixture: a traves del enlace, .hoom/envelopes es el del otro proyecto: %v", ca, err)
						}
						antes := e2bFoto(t, filepath.Join(otro, ".hoom"))

						if op == "Write" {
							err = Write(root, rec)
						} else {
							err = Cerrar(root, e1Cerrado(rec))
						}
						if err == nil {
							t.Errorf("%s: con %s enlazado a otro proyecto, el primer %s no escribe ahi y devuelve error, no nil", ca, que, op)
						}
						fin := time.Now()
						if op == "Cerrar" {
							fin = fin.Add(1500 * time.Millisecond)
						}
						for {
							if d := e2bDiferencias(antes, e2bFoto(t, filepath.Join(otro, ".hoom"))); len(d) > 0 {
								t.Fatalf("%s: con %s enlazado a otro proyecto, %s escribio en el otro proyecto: %s", ca, que, op, strings.Join(d, "; "))
							}
							if time.Now().After(fin) {
								break
							}
							time.Sleep(20 * time.Millisecond)
						}
					})
				}
			}
		}
	}
}

// CA-333, CA-202 (hallazgo 4f418f, guarda del contrato de siempre): en un
// proyecto sin .hoom, y en uno cuya carpeta todavia no existe, el primer
// Write (o el primer Cerrar) crea lo que falta y escribe el registro como
// siempre: da nil y el registro esta en disco, en el proyecto.
func TestHallazgo_4f418f_CA333_ElPrimerWriteCreaLoQueFalta(t *testing.T) {
	const ca = "CA-333 (hallazgo 4f418f, guarda: primer Write en un proyecto sin .hoom)"
	for _, op := range []string{"Write", "Cerrar"} {
		for _, carpeta := range []bool{true, false} {
			nombre := fmt.Sprintf("%s, %s", op, map[bool]string{true: "proyecto sin .hoom", false: "sin carpeta del proyecto"}[carpeta])
			t.Run(nombre, func(t *testing.T) {
				root := filepath.Join(t.TempDir(), "proyecto")
				if carpeta {
					if err := os.Mkdir(root, 0o755); err != nil {
						t.Fatal(err)
					}
				}
				rec := e1Rec(e1IDPropio, "propio")
				quiero := rec
				var err error
				if op == "Write" {
					err = Write(root, rec)
				} else {
					quiero = e1Cerrado(rec)
					err = Cerrar(root, quiero)
				}
				if err != nil {
					t.Fatalf("%s: %s en %s crea lo que falta y escribe: da nil, no %v", ca, op, nombre, err)
				}
				r, ok := e1EnDisco(root, rec.ID)
				if !ok || r.Status != quiero.Status {
					t.Errorf("%s: %s dio nil: el registro esta en disco en el proyecto: %+v %v", ca, op, r, ok)
				}
				if fi, err := os.Lstat(filepath.Join(root, ".hoom", DirName, rec.ID+".json")); err != nil || !fi.Mode().IsRegular() {
					t.Errorf("%s: el registro es un archivo comun en .hoom/envelopes del proyecto: %v %v", ca, fi, err)
				}
				if b, err := os.ReadFile(filepath.Join(root, ".hoom", DirName, rec.ID+".json")); err == nil && !bytes.Contains(b, []byte(rec.ID)) {
					t.Errorf("%s: el registro en disco es el escrito: %s", ca, b)
				}
			})
		}
	}
}
