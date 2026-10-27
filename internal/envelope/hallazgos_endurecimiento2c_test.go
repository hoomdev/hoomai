// Tests de regresion de la tercera ronda de endurecimiento-2 en envelope.
// Escritos desde el texto de cada hallazgo y el contrato de la correccion,
// SIN LEER LA IMPLEMENTACION. Reusan los helpers e1* (endurecimiento-1),
// e2Colgar (el gancho antesDeEscribir, hallazgos_endurecimiento2_gancho_test.go)
// y e2bFoto y e2bDiferencias (hallazgos_endurecimiento2b_test.go); los de
// este archivo llevan el prefijo e2c.
//
//   - 20261026T21*_f89080 (medium): el primer Write abria .hoom/envelopes
//     por descriptor y publicaba el registro por ese descriptor sin volver a
//     comprobar que siguiera siendo el que se alcanza por
//     root/.hoom/envelopes. Si otro proceso lo renombraba en el medio, Write
//     devolvia nil con el registro en el directorio movido, fuera de su
//     lugar: se rompia la promesa del primer registro en disco (Started).
//     Ahora devuelve error.
//   - 20261026T21*_0c9aec (high, risk), por envelope.Write: Write completa
//     .hoom/.gitignore con hoomfs.EnsureIgnored, que lo leia anclado y lo
//     escribia por ruta; con un root cuyo camino resuelto por el kernel y
//     cuyo camino limpiado sin mirar enlaces son distintos, escribia el
//     .gitignore de otro proyecto (ver hoomfs/hallazgos_endurecimiento2c_test.go).
package envelope

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// CA-333, CA-202 (hallazgo f89080): con el primer Write de un registro
// sostenido por el gancho antesDeEscribir (despues de abrir .hoom/envelopes,
// antes de publicar), .hoom/envelopes (o .hoom entero) se renombra y en su
// lugar se pone uno nuevo vacio. Al soltar, Write devuelve error: no nil con
// el registro fuera de su lugar (en el directorio movido). En un proyecto
// nuevo y en uno que ya tenia otro sobre. Guarda: la escritura llego al
// gancho (si no, el caso no se inyecto).
func TestHallazgo_f89080_CA333_SiEnvelopesSeMueveDuranteElPrimerWriteEsUnError(t *testing.T) {
	const ca = "CA-333 (hallazgo f89080, .hoom/envelopes movido durante el primer Write)"
	for _, mover := range []string{".hoom/envelopes", ".hoom"} {
		for _, previo := range []bool{false, true} {
			nombre := mover + ", " + map[bool]string{false: "proyecto nuevo", true: "proyecto con otro sobre"}[previo]
			t.Run(nombre, func(t *testing.T) {
				root := filepath.Join(t.TempDir(), "proyecto")
				if previo {
					e1Escribir(t, root, e1Rec(e1IDAjeno, "otro sobre"))
				}
				rec := e1Rec(e1IDPropio, "propio")
				c := e2Colgar(t, func(r, id string) bool { return r == root && id == rec.ID })
				var err error
				listo := make(chan struct{})
				go func() {
					defer close(listo)
					err = Write(root, rec)
				}()
				if !e1bEsperar(10*time.Second, func() bool { return c.Llegaron(root, rec.ID) > 0 }) {
					c.Soltar()
					<-listo
					t.Fatalf("%s: fixture: en 10 s el primer Write no llego al gancho antesDeEscribir: el caso no se inyecto", ca)
				}

				movido := filepath.Join(root, mover) + "-movido"
				if err := os.Rename(filepath.Join(root, mover), movido); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Join(root, ".hoom", DirName), 0o755); err != nil {
					t.Fatal(err)
				}
				c.Soltar()
				select {
				case <-listo:
				case <-time.After(10 * time.Second):
					t.Fatalf("%s: fixture: 10 s despues de soltar el gancho, Write no volvio", ca)
				}

				enSuLugar := filepath.Join(root, ".hoom", DirName, rec.ID+".json")
				enMovido := filepath.Join(movido, rec.ID+".json")
				if mover == ".hoom" {
					enMovido = filepath.Join(movido, DirName, rec.ID+".json")
				}
				_, eLugar := os.Lstat(enSuLugar)
				_, eMovido := os.Lstat(enMovido)
				t.Logf("%s: Write devolvio %v; el registro esta en su lugar: %v, en el directorio movido: %v", ca, err, eLugar == nil, eMovido == nil)
				if err == nil {
					t.Errorf("%s: con %s movido entre la apertura y la publicacion, el primer Write devuelve error, no nil (registro en su lugar: %v, en el directorio movido: %v)",
						ca, mover, eLugar == nil, eMovido == nil)
				}
			})
		}
	}
}

// CA-333 (hallazgo 0c9aec, por envelope.Write): con un root
// "<base>/enlace/../p", donde enlace apunta a <base>/sitio/sub, el kernel ve
// el proyecto <base>/sitio/p (sin .hoom) y la limpieza del camino sin mirar
// enlaces ve <base>/p, otro proyecto con su .hoom (.gitignore al que le
// faltan entradas y un sobre). Write (y Cerrar, con su primer reintento si
// fallo) no tocan nada de <base>/p: todo lo que hay bajo su .hoom queda
// identico.
func TestHallazgo_0c9aec_CA333_WriteNoEscribeElGitignoreDeOtroProyecto(t *testing.T) {
	const ca = "CA-333 (hallazgo 0c9aec, envelope.Write con un root enlace/..)"
	if runtime.GOOS == "windows" {
		t.Skip("los enlaces simbolicos en Windows piden privilegios: el caso se prueba en Linux y macOS")
	}
	for _, op := range []string{"Write", "Cerrar"} {
		t.Run(op, func(t *testing.T) {
			base, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(base, "sitio", "sub"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(base, "sitio", "p"), 0o755); err != nil {
				t.Fatal(err)
			}
			victima := filepath.Join(base, "p")
			e1Escribir(t, victima, e1Rec(e1IDAjeno, "victima"))
			if err := os.WriteFile(filepath.Join(victima, ".hoom", ".gitignore"), []byte("de-la-victima/\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(base, "sitio", "sub"), filepath.Join(base, "enlace")); err != nil {
				t.Skipf("este sistema no deja crear enlaces simbolicos: %v", err)
			}
			root := filepath.Join(base, "enlace") + "/../p"
			if _, err := os.Stat(root + "/.hoom"); err == nil {
				t.Fatalf("%s: fixture: por el root %s, el kernel ve <base>/sitio/p, que no tiene .hoom", ca, root)
			}
			antes := e2bFoto(t, filepath.Join(victima, ".hoom"))

			rec := e1Rec(e1IDPropio, "propio")
			if op == "Write" {
				err = Write(root, rec)
			} else {
				err = Cerrar(root, e1Cerrado(rec))
			}
			t.Logf("%s: %s devolvio %v", ca, op, err)
			// con Cerrar, tambien su primer reintento (a 1 s) si fallo
			fin := time.Now()
			if op == "Cerrar" && err != nil {
				fin = fin.Add(1500 * time.Millisecond)
			}
			for {
				if d := e2bDiferencias(antes, e2bFoto(t, filepath.Join(victima, ".hoom"))); len(d) > 0 {
					t.Fatalf("%s: %s por el root %s escribio en otro proyecto (%s): %s", ca, op, root, victima, strings.Join(d, "; "))
				}
				if time.Now().After(fin) {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
		})
	}
}
