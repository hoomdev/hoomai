package pilotocmd_test

import (
	"testing"

	"github.com/hoomdev/hoomai/internal/itemcmd"
)

// Hallazgo 20261113T030054_048c45 (aclaracion 24), el caso que el chequeo de
// disco no ve: un test protegido esta entre las rutas que el guardado pide
// (un trasplante pendiente de guardar) y un pre-commit stagea para esa ruta
// otro blob con `git update-index --cacheinfo`, sin tocar el disco. El
// guardado falla, la rama queda donde estaba y el test protegido en HEAD
// sigue como estaba.
func TestWriter048c45UnHookNoCambiaElBlobDeUnProtegidoPedido(t *testing.T) {
	const ca = "048c45 (protegido pedido, blob del hook)"
	apPATH(t)
	root, wt, slug := apr5Protegida(t, ca)
	const pendiente = "package app\n\n// CA-1: el total redondea bien\n// CA-1: y el trasplante nuevo del test-writer\n"
	apEscribir(t, wt, "precios_test.go", pendiente)
	head := apHead(t, wt)
	apr4Hook(t, wt, "pre-commit", "#!/bin/sh\n"+
		"b=$(printf 'package app\\n\\n// colado por el hook\\n' | git hash-object -w --stdin)\n"+
		"git update-index --cacheinfo 100644,$b,precios_test.go\nexit 0\n")

	res, err := itemcmd.SaveTarea(root, apBase, apBlockOn, slug)
	if err == nil {
		t.Fatalf("%s: el guardado falla si un hook cambia en el indice el blob de un test protegido pedido: %+v (HEAD:precios_test.go %q)",
			ca, res, apGit(t, wt, "show", "HEAD:precios_test.go"))
	}
	if h := apHead(t, wt); h != head {
		t.Fatalf("%s: la rama queda donde estaba: HEAD %s -> %s (%v)", ca, head, h, err)
	}
	if enHead := apGit(t, wt, "show", "HEAD:precios_test.go"); enHead+"\n" != apr5Protegido {
		t.Fatalf("%s: el test protegido en HEAD queda como estaba: %q", ca, enHead)
	}
}

// Sin hook, el mismo trasplante pendiente se guarda tal cual: el chequeo no
// confunde el blob que stagea el propio guardado con uno colado.
func TestWriter048c45ElProtegidoPedidoSinHookSeGuarda(t *testing.T) {
	const ca = "048c45 (protegido pedido, sin hook)"
	apPATH(t)
	root, wt, slug := apr5Protegida(t, ca)
	const pendiente = "package app\n\n// CA-1: el total redondea bien\n// CA-1: y el trasplante nuevo del test-writer\n"
	apEscribir(t, wt, "precios_test.go", pendiente)

	res, err := itemcmd.SaveTarea(root, apBase, apBlockOn, slug)
	if err != nil || len(res.Commits) != 1 {
		t.Fatalf("%s: el guardado commitea el trasplante pendiente: %+v %v", ca, res, err)
	}
	if enHead := apGit(t, wt, "show", "HEAD:precios_test.go"); enHead+"\n" != pendiente {
		t.Fatalf("%s: HEAD tiene el trasplante pendiente: %q", ca, enHead)
	}
}
