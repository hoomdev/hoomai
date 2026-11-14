// Ronda 2 del test-writer de contencion-del-writer: la foto final del
// guardado del piloto queda despues de su ultimo git, el deshacer incluido
// (id 110c02; aclaracion 21), en cualquier sistema.
package pilotocmd_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/itemcmd"
)

// CA-688 y CA-689 (id 110c02; aclaracion 21): un guardado que falla despues
// de su commit (un post-commit crea `shallow`, material de git que no
// existia, y deja una marca) deshace su commit; un hook reference-transaction
// que actua solo despues de ese commit (con la marca) crea en el arbol
// .claude/settings.json, que no existia, mientras corre el git del deshacer.
// El error del guardado nombra tambien .claude/settings.json con el texto de
// itemcmd.ErrHookCambioConfig, y HEAD queda donde estaba. Si el
// reference-transaction no corrio en el deshacer (el archivo no esta), no hay
// nada que probar: queda en el log.
func TestR2_110c02_CA688_CA689_LaFotoFinalDelGuardadoVaDespuesDelDeshacer(t *testing.T) {
	const ca = "CA-689"
	apPATH(t)
	cdwHome(t)
	const slug = "fotofinal"
	root, wt := cdwTarjeta(t, "", slug)
	comun := cdwComun(t, wt)
	shallow := filepath.Join(comun, "shallow")
	rel := ".claude/settings.json"
	abs := filepath.Join(wt, ".claude", "settings.json")
	if apr1Existe(shallow) || apr1Existe(abs) {
		t.Fatalf("%s: fixture: ni shallow ni %s existen antes del guardado", ca, rel)
	}
	marca := filepath.Join(t.TempDir(), "commiteado")
	// shallow con el commit nuevo: un archivo shallow valido, que no rompe
	// el git que sigue
	apr4Hook(t, wt, "post-commit", "#!/bin/sh\ngit rev-parse HEAD > '"+shallow+"'\n: > '"+marca+"'\nexit 0\n")
	apr4Hook(t, wt, "reference-transaction", "#!/bin/sh\ncat >/dev/null\n[ -f '"+marca+"' ] || exit 0\n"+
		"mkdir -p '"+filepath.Dir(abs)+"'\nprintf '%s' '{\"hooks\":{\"Stop\":[{\"command\":\"r2-del-deshacer\"}]}}' > '"+abs+"'\nexit 0\n")
	head := apHead(t, wt)
	res, err := itemcmd.SaveTarea(root, apBase, apBlockOn, slug)
	if !apr1Existe(marca) {
		t.Fatalf("%s: fixture: el guardado commitea y corre el post-commit: %+v %v", ca, res, err)
	}
	if err == nil || !strings.Contains(err.Error(), "shallow") {
		t.Fatalf("%s: fixture: el guardado falla despues de su commit por el shallow que creo el post-commit: %+v %v", ca, res, err)
	}
	if h := apHead(t, wt); h != head {
		t.Fatalf("%s: el guardado deshace su commit: HEAD %s -> %s", ca, head, h)
	}
	if !apr1Existe(abs) {
		t.Logf("%s: el reference-transaction no corrio en el deshacer (%s no esta): nada que probar; error: %v", ca, rel, err)
		return
	}
	wantRel := itemcmd.ErrHookCambioConfig([]string{rel})
	wantAbs := itemcmd.ErrHookCambioConfig([]string{abs})
	if !strings.Contains(err.Error(), wantRel) && !strings.Contains(err.Error(), wantAbs) {
		t.Fatalf("%s: la foto final va despues del deshacer: el error nombra tambien %s con %q: %v", ca, rel, wantRel, err)
	}
}
