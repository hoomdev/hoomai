// Test de regresion del hallazgo 20261015T163243_ebae2a (medium) de la review
// de las correcciones de C3 (.hoom/specs/acciones-desde-la-tarjeta.md,
// CA-344), escrito desde el contrato y el hallazgo, sin leer la
// implementacion: Guardar elige los arboles cuyo candado toma con la tarjeta
// que deriva primero. Si mientras espera un candado la tarjeta gana su
// espacio de trabajo (alguien hace `hoom task start` y un run se empieza a
// registrar ahi), Guardar nunca commitea en ese arbol sin tener su candado.
//
// Se provoca desde afuera con los candados: el test toma el de la raiz antes
// de Guardar (asi Guardar deriva la tarjeta sin espacio de trabajo y se queda
// esperando), crea la tarea, toma el candado del espacio de trabajo nuevo y
// recien entonces suelta el de la raiz.
//
// Sin depender del tiempo (hallazgo f717f9): el test no da por hecho que la
// goroutine de Guardar ya llego a esperar el candado; lo observa. Un intento
// solo cuenta si se vio a Guardar esperando adentro de runcmd.LockArbol (el
// candado que el contrato dice que toma, go doc LockArbol) ANTES de que
// existiera el espacio de trabajo, y todavia esperando, sin haber vuelto y
// dentro de su plazo (runcmd.EsperaArbol), justo antes de soltar la raiz.
// Si no, el intento no ejercito la re-derivacion y se repite con un repo
// nuevo; si en ebIntentos ninguno la ejercita, el test falla. Las
// aserciones (HEAD e indice del espacio de trabajo intactos) se exigen en
// todo intento en que el test tuvo el candado del espacio de trabajo: con el
// candado en manos del test, Guardar nunca pudo tenerlo.
//
// Reusa cbRepoSave, cbItemYAML, cbUnsynced, cbHead, cbEnIndice y cbMismos
// (save_test.go), icEscribir (itemcmd_test.go) y kcTomar
// (hallazgos_candado_test.go).
package itemcmd

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/taskcmd"
)

// ebIntentos es cuantas veces se arma el escenario antes de darlo por
// imposible de ejercitar.
const ebIntentos = 6

// ebMargen es lo que tiene que faltarle al plazo de Guardar para el candado
// de la raiz cuando el test lo suelta: asi Guardar lo toma y no se vence.
const ebMargen = 300 * time.Millisecond

// ebVuelta es lo que devolvio Guardar.
type ebVuelta struct {
	res SaveResult
	err error
}

// ebGuardar corre Guardar (como la CLI: expect nil) en la goroutine que el
// test observa y manda lo que devolvio por ch.
//
//go:noinline
func ebGuardar(root string, ch chan<- ebVuelta) {
	res, err := Save(root, "main", "high", cbSlug, nil)
	ch <- ebVuelta{res, err}
}

// ebEsperandoCandado dice si la goroutine de ebGuardar esta adentro de
// runcmd.LockArbol (esperando un candado de arbol).
func ebEsperandoCandado() bool {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	for _, g := range strings.Split(string(buf), "\n\n") {
		if strings.Contains(g, "itemcmd.ebGuardar(") && strings.Contains(g, "runcmd.LockArbol(") {
			return true
		}
	}
	return false
}

// Hallazgo 20261015T163243_ebae2a (medium), CA-344: Guardar no commitea en
// un espacio de trabajo cuyo candado no tomo. Con el candado del espacio de
// trabajo tomado por otro (un run que se registra), ni su HEAD ni su indice
// cambian, aunque el espacio de trabajo haya aparecido mientras Guardar
// esperaba el candado de la raiz.
func TestHallazgo_ebae2a_CA344_GuardarNoCommiteaUnArbolCuyoCandadoNoTomo(t *testing.T) {
	for i := 1; i <= ebIntentos; i++ {
		ejercitado, porque := ebIntento(t, i)
		if ejercitado {
			return
		}
		t.Logf("CA-344: intento %d de %d no ejercito la re-derivacion: %s", i, ebIntentos, porque)
	}
	t.Fatalf("CA-344: en %d intentos nunca se vio a Guardar esperar el candado de la raiz desde antes de que apareciera el espacio de trabajo hasta que se solto: el escenario del hallazgo ebae2a no se pudo ejercitar", ebIntentos)
}

// ebIntento arma el escenario una vez, con un repo nuevo. Exige las
// aserciones del hallazgo siempre que el test tuvo el candado del espacio de
// trabajo, y dice si el intento ejercito la re-derivacion (y si no, por
// que).
func ebIntento(t *testing.T, intento int) (bool, string) {
	t.Helper()
	root := cbRepoSave(t)
	icEscribir(t, root, ".hoom/items/"+cbSlug+".yaml", cbItemYAML())
	if got := cbUnsynced(t, root); !cbMismos(got, []string{".hoom/items/" + cbSlug + ".yaml"}) {
		t.Fatalf("CA-344: fixture: la tarjeta, sin tarea, tiene sin guardar solo su item: %v", got)
	}
	soltarRaiz := kcTomar(t, root)

	ch := make(chan ebVuelta, 1)
	desde := time.Now()
	go ebGuardar(root, ch)
	var (
		v        ebVuelta
		recibido bool
	)
	esperar := func(plazo time.Duration) {
		if recibido {
			return
		}
		select {
		case v = <-ch:
			recibido = true
		case <-time.After(plazo):
		}
	}
	// Al terminar el intento (tambien por un Fatal) se sueltan los candados
	// y se espera a Guardar: el intento siguiente no ve esta goroutine.
	soltarWt := func() {}
	defer func() {
		soltarRaiz()
		soltarWt()
		esperar(30 * time.Second)
	}()

	// 1. Guardar llega a esperar el candado de la raiz. Todavia no hay
	// espacio de trabajo: la tarjeta que derivo no lo tiene.
	for !ebEsperandoCandado() {
		select {
		case v = <-ch:
			recibido = true
			if time.Since(desde) < 500*time.Millisecond {
				t.Fatalf("CA-344: fixture: con el candado de la raiz tomado, Guardar espera: volvio con %+v %v", v.res, v.err)
			}
			return false, fmt.Sprintf("Guardar volvio (%v) sin que se lo viera esperar el candado: %+v", v.err, v.res)
		default:
		}
		if time.Since(desde) > runcmd.EsperaArbol/2 {
			return false, "no se vio a Guardar esperar el candado de la raiz en " + (runcmd.EsperaArbol / 2).String()
		}
		time.Sleep(2 * time.Millisecond)
	}

	// 2. Mientras espera, aparece el espacio de trabajo y un run empieza a
	// registrarse ahi (toma su candado).
	if err := taskcmd.Start(root, cbSlug, "main"); err != nil {
		t.Fatalf("CA-344: fixture: task start mientras Guardar espera: %v", err)
	}
	wt := filepath.Join(root, ".hoom", "worktrees", cbSlug)
	icEscribir(t, wt, "app.go", "package app\n\nfunc AMedias() {}\n") // lo que un run esta escribiendo
	headWt := cbHead(t, wt)
	indiceWt := cbEnIndice(t, wt)
	suelta, err := runcmd.LockArbol(wt, time.Second)
	if err != nil {
		return false, fmt.Sprintf("el test no pudo tomar el candado del espacio de trabajo (lo tiene Guardar?): %v", err)
	}
	var una sync.Once
	soltarWt = func() { una.Do(suelta) }

	// 3. Justo antes de soltar la raiz: Guardar sigue esperando su candado y
	// le falta para vencerse.
	ejercitado, porque := true, ""
	select {
	case v = <-ch:
		recibido = true
		ejercitado, porque = false, fmt.Sprintf("Guardar volvio (%v) antes de que se soltara la raiz", v.err)
	default:
		switch {
		case time.Since(desde) > runcmd.EsperaArbol-ebMargen:
			ejercitado, porque = false, fmt.Sprintf("se llego a soltar la raiz %v despues de lanzar Guardar: su plazo pudo vencerse", time.Since(desde).Round(time.Millisecond))
		case !ebEsperandoCandado():
			ejercitado, porque = false, "Guardar ya no esperaba el candado al soltar la raiz"
		}
	}
	soltarRaiz()

	esperar(30 * time.Second)
	if !recibido {
		t.Fatalf("CA-344: Guardar no volvio en 30 s (intento %d)", intento)
	}
	if h := cbHead(t, wt); h != headWt {
		t.Fatalf("hallazgo ebae2a (CA-344): Guardar no commitea en un espacio de trabajo cuyo candado no tomo: su HEAD paso de %s a %s (resultado %+v, error %v, intento %d)", headWt, h, v.res, v.err, intento)
	}
	if got := cbEnIndice(t, wt); !cbMismos(got, indiceWt) {
		t.Fatalf("hallazgo ebae2a (CA-344): Guardar no toca el indice de un espacio de trabajo cuyo candado no tomo: %v -> %v (intento %d)", indiceWt, got, intento)
	}
	return ejercitado, porque
}
