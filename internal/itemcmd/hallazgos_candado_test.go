// Tests de regresion del hallazgo 20261014T180925_ac2feb (medium) de la
// review de C3 (.hoom/specs/acciones-desde-la-tarjeta.md), escritos desde el
// contrato y el hallazgo, sin leer la implementacion: Guardar (itemcmd.Save,
// `hoom item save`, CA-344) mira "la tarjeta no esta en curso" y despues
// commitea en los dos arboles. Un run que se registraba en ese medio quedaba
// con su trabajo a medias commiteado. La decision: un candado por arbol
// (runcmd.LockArbol). Save toma el del espacio de trabajo y el del proyecto
// hasta el ultimo commit, espera hasta runcmd.EsperaArbol y, si no lo
// consigue, se niega con error sin commitear nada.
//
// Reusa el fixture de save_test.go (cbTareaSave, cbEsperadas, cbSinCambios).
package itemcmd

import (
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/runcmd"
)

// kcTope es lo maximo que Save puede tardar en negarse con un candado
// tomado: espera hasta runcmd.EsperaArbol y se niega, no se cuelga.
const kcTope = runcmd.EsperaArbol + 8*time.Second

// kcTomar toma el candado del arbol dir como lo toma un run que se esta
// registrando ahi y devuelve como soltarlo; si nadie lo suelta antes, se
// suelta al final del test.
func kcTomar(t *testing.T, dir string) func() {
	t.Helper()
	suelta, err := runcmd.LockArbol(dir, time.Second)
	if err != nil {
		t.Fatalf("CA-344: el test toma el candado del arbol %s: %v", dir, err)
	}
	var una sync.Once
	soltar := func() { una.Do(suelta) }
	t.Cleanup(soltar)
	return soltar
}

// kcArboles son los dos arboles de la tarjeta: el espacio de trabajo y el
// proyecto.
var kcArboles = []string{"worktree", "proyecto"}

func kcDir(arbol, root, wt string) string {
	if arbol == "proyecto" {
		return root
	}
	return wt
}

// Hallazgo 20261014T180925_ac2feb (medium), CA-344 (con la tarjeta en curso
// se niega sin commitear): con el candado del espacio de trabajo o el del
// proyecto tomado (como lo toma un run que se esta registrando ahi), Save se
// niega en a lo sumo unos segundos, con error, y no commitea nada: el HEAD de
// los dos arboles, lo sin guardar (unsynced) y los dos indices quedan igual.
func TestHallazgo_ac2feb_CA344_CandadoTomadoGuardarSeNiega(t *testing.T) {
	for _, arbol := range kcArboles {
		t.Run(arbol, func(t *testing.T) {
			root, wt := cbTareaSave(t)
			if got := cbUnsynced(t, root); !cbMismos(got, cbEsperadas) {
				t.Fatalf("CA-344: fixture: la tarjeta tiene sin guardar %v, no %v", got, cbEsperadas)
			}
			headRoot, headWt := cbHead(t, root), cbHead(t, wt)
			indiceWt := cbEnIndice(t, wt)
			kcTomar(t, kcDir(arbol, root, wt))

			desde := time.Now()
			res, err := Save(root, "main", "high", cbSlug, nil)
			if tardo := time.Since(desde); tardo > kcTope {
				t.Fatalf("hallazgo ac2feb (CA-344): con el candado del %s tomado Save se niega en a lo sumo %v, no se cuelga: tardo %v", arbol, kcTope, tardo)
			}
			if err == nil {
				t.Fatalf("hallazgo ac2feb (CA-344): con el candado del %s tomado Save se niega con error; devolvio exito: %+v", arbol, res)
			}
			cbSinCambios(t, "hallazgo ac2feb (candado del "+arbol+" tomado)", root, wt, headRoot, headWt)
			if got := cbEnIndice(t, wt); !cbMismos(got, indiceWt) {
				t.Fatalf("hallazgo ac2feb (CA-344): con el candado del %s tomado Save no toca el indice del worktree: %v -> %v", arbol, indiceWt, got)
			}
		})
	}
}

// kcHeadSinT es el HEAD de dir sin pasar por t (se llama desde otra
// goroutine): "" si git falla.
func kcHeadSinT(dir string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// kcAlSoltar es lo que habia en los dos arboles justo antes de soltar el
// candado: si Save no espero, ya habia commiteado.
type kcAlSoltar struct {
	cuando           time.Time
	headRoot, headWt string
}

// Hallazgo 20261014T180925_ac2feb (medium), CA-344: Save ESPERA el candado
// (hasta runcmd.EsperaArbol) y despues guarda: con el del espacio de trabajo
// o el del proyecto tomado y soltado por otra goroutine, mientras estuvo
// tomado no commiteo nada (el HEAD de los dos arboles seguia igual al
// soltarlo), termina despues de que se solto, con un commit por arbol, y la
// tarjeta queda sin nada sin guardar (el archivo ajeno sigue en el indice del
// raiz). Se suelta al segundo y no a los 300 ms: Save sin candado tarda mas
// de 300 ms en commitear, y a los 300 ms el HEAD todavia no lo delataria.
func TestHallazgo_ac2feb_CA344_CandadoSueltoGuardarEsperaYGuarda(t *testing.T) {
	for _, arbol := range kcArboles {
		t.Run(arbol, func(t *testing.T) {
			root, wt := cbTareaSave(t)
			headRoot, headWt := cbHead(t, root), cbHead(t, wt)
			soltar := kcTomar(t, kcDir(arbol, root, wt))
			soltado := make(chan kcAlSoltar, 1)
			go func() {
				time.Sleep(time.Second)
				a := kcAlSoltar{headRoot: kcHeadSinT(root), headWt: kcHeadSinT(wt)}
				a.cuando = time.Now()
				soltar()
				soltado <- a
			}()

			res, err := Save(root, "main", "high", cbSlug, nil)
			fin := time.Now()
			var a kcAlSoltar
			select {
			case a = <-soltado:
			case <-time.After(10 * time.Second):
				t.Fatal("CA-344: la goroutine del test no solto el candado")
			}
			if a.headRoot != headRoot || a.headWt != headWt {
				t.Fatalf("hallazgo ac2feb (CA-344): mientras el candado del %s estuvo tomado Save no commitea nada: al soltarlo el HEAD del raiz era %s (antes %s) y el del worktree %s (antes %s)", arbol, a.headRoot, headRoot, a.headWt, headWt)
			}
			if err != nil {
				t.Fatalf("hallazgo ac2feb (CA-344): con el candado del %s soltado al segundo, Save espera y guarda: %v", arbol, err)
			}
			if fin.Before(a.cuando) {
				t.Fatalf("hallazgo ac2feb (CA-344): Save termino (%s) antes de que se soltara el candado del %s (%s): no lo espero", fin.Format(time.StampMicro), arbol, a.cuando.Format(time.StampMicro))
			}
			if len(res.Commits) != 2 || cbHead(t, root) == headRoot || cbHead(t, wt) == headWt {
				t.Fatalf("hallazgo ac2feb (CA-344): soltado el candado, Save hace un commit por arbol: %+v", res.Commits)
			}
			if got := cbUnsynced(t, root); len(got) != 0 {
				t.Fatalf("hallazgo ac2feb (CA-344): despues de guardar la tarjeta no tiene nada sin guardar: %v", got)
			}
			if got := cbEnIndice(t, root); !cbMismos(got, []string{"ajeno.txt"}) {
				t.Fatalf("hallazgo ac2feb (CA-344): el archivo ajeno sigue en el indice del raiz sin commitear: %v", got)
			}
		})
	}
}
