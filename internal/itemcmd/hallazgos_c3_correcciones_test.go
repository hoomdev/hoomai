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
// Reusa cbRepoSave, cbItemYAML, cbUnsynced, cbHead y cbEnIndice
// (save_test.go), icEscribir (itemcmd_test.go) y kcTomar
// (hallazgos_candado_test.go).
package itemcmd

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/taskcmd"
)

// Hallazgo 20261015T163243_ebae2a (medium), CA-344: Guardar no commitea en
// un espacio de trabajo cuyo candado no tomo. Con el candado del espacio de
// trabajo tomado por otro (un run que se registra), ni su HEAD ni su indice
// cambian, aunque el espacio de trabajo haya aparecido mientras Guardar
// esperaba el candado de la raiz.
func TestHallazgo_ebae2a_CA344_GuardarNoCommiteaUnArbolCuyoCandadoNoTomo(t *testing.T) {
	root := cbRepoSave(t)
	icEscribir(t, root, ".hoom/items/"+cbSlug+".yaml", cbItemYAML())
	if got := cbUnsynced(t, root); !cbMismos(got, []string{".hoom/items/" + cbSlug + ".yaml"}) {
		t.Fatalf("CA-344: fixture: la tarjeta, sin tarea, tiene sin guardar solo su item: %v", got)
	}
	soltarRaiz := kcTomar(t, root)

	type vuelta struct {
		res SaveResult
		err error
	}
	ch := make(chan vuelta, 1)
	go func() {
		res, err := Save(root, "main", "high", cbSlug, nil)
		ch <- vuelta{res, err}
	}()
	recibido := false
	t.Cleanup(func() {
		soltarRaiz()
		if !recibido {
			select {
			case <-ch:
			case <-time.After(30 * time.Second):
			}
		}
	})

	time.Sleep(500 * time.Millisecond) // Guardar ya derivo la tarjeta sin tarea y espera la raiz
	select {
	case v := <-ch:
		recibido = true
		t.Fatalf("CA-344: fixture: con el candado de la raiz tomado, Guardar espera: volvio con %+v %v", v.res, v.err)
	default:
	}
	if err := taskcmd.Start(root, cbSlug, "main"); err != nil {
		t.Fatalf("CA-344: fixture: task start mientras Guardar espera: %v", err)
	}
	wt := filepath.Join(root, ".hoom", "worktrees", cbSlug)
	icEscribir(t, wt, "app.go", "package app\n\nfunc AMedias() {}\n") // lo que un run esta escribiendo
	headWt := cbHead(t, wt)
	indiceWt := cbEnIndice(t, wt)
	soltarWt := kcTomar(t, wt) // un run se esta registrando en el espacio de trabajo
	soltarRaiz()

	var v vuelta
	select {
	case v = <-ch:
		recibido = true
	case <-time.After(30 * time.Second):
		t.Fatal("CA-344: Guardar no volvio en 30 s")
	}
	if h := cbHead(t, wt); h != headWt {
		t.Fatalf("hallazgo ebae2a (CA-344): Guardar no commitea en un espacio de trabajo cuyo candado no tomo: su HEAD paso de %s a %s (resultado %+v, error %v)", headWt, h, v.res, v.err)
	}
	if got := cbEnIndice(t, wt); !cbMismos(got, indiceWt) {
		t.Fatalf("hallazgo ebae2a (CA-344): Guardar no toca el indice de un espacio de trabajo cuyo candado no tomo: %v -> %v", indiceWt, got)
	}
	soltarWt()
}
