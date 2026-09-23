// Tests adversariales del spec .hoom/specs/items-y-columna-derivada.md
// (CA-272): spec.Trace se parte en dos y el tablero usa solo la mitad que LEE.
// spec.Tokens busca el token exacto de cada criterio en los archivos de test
// y nunca ejecuta un marcador de comando.
package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tkEscribir(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// CA-272: Tokens devuelve los criterios sin token, en el orden recibido, y
// cuantos archivos de test escaneo; el token es exacto (CA-1 no es CA-12) y
// .hoom/ no se escanea.
func TestCA272_TokensSoloLee(t *testing.T) {
	root := t.TempDir()
	tkEscribir(t, root, "a_test.go", "package a\n// CA-12: cubre el doce\n// CA-3 tambien\n")
	tkEscribir(t, root, "tests/b.txt", "CA-5 aca\n")
	tkEscribir(t, root, "app.go", "package app // CA-7 no es un archivo de test\n")
	tkEscribir(t, root, ".hoom/specs/x_test.md", "CA-9 en .hoom no cuenta\n")

	missing, scanned, err := Tokens(root, []string{"CA-1", "CA-3", "CA-5", "CA-7", "CA-9", "CA-12"})
	if err != nil {
		t.Fatalf("CA-272: Tokens no falla sobre un arbol legible: %v", err)
	}
	if strings.Join(missing, ",") != "CA-1,CA-7,CA-9" {
		t.Fatalf("CA-272: sin token quedan CA-1 (CA-12 no lo cubre), CA-7 (no es test) y CA-9 (.hoom no se escanea); fue %v", missing)
	}
	if scanned != 2 {
		t.Fatalf("CA-272: se escanean los 2 archivos de test (a_test.go, tests/b.txt), fueron %d", scanned)
	}

	missing, _, err = Tokens(root, nil)
	if err != nil || len(missing) != 0 {
		t.Fatalf("CA-272: sin ids no falta nada: %v %v", missing, err)
	}
}

// CA-272: el marcador de comando no es asunto de Tokens: ni lo lee ni lo
// corre. Un spec con un comando que crea un archivo no deja rastro.
func TestCA272_TokensNoEjecutaMarcadores(t *testing.T) {
	root, path := writeSpec(t, "- CA-1: se marca. [verifica: touch marca]\n- CA-2: con test.\n")
	ids, cmds, issues, err := Lint(path)
	if err != nil || len(issues) != 0 {
		t.Fatalf("CA-272: el spec de prueba pasa lint: %v %v", issues, err)
	}
	if len(cmds["CA-1"]) != 1 {
		t.Fatalf("CA-272: el marcador se declara: %v", cmds)
	}
	var sinComando []string
	for _, id := range ids {
		if len(cmds[id]) == 0 {
			sinComando = append(sinComando, id)
		}
	}
	missing, scanned, err := Tokens(root, sinComando)
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 0 || scanned < 1 {
		t.Fatalf("CA-272: CA-2 tiene su token en item_test.txt: missing=%v scanned=%d", missing, scanned)
	}
	if _, err := os.Stat(filepath.Join(root, "marca")); !os.IsNotExist(err) {
		t.Fatalf("CA-272: Tokens no corre ningun comando: 'marca' no debe existir (err=%v)", err)
	}

	// y Trace sigue siendo la suma de las dos mitades: con el comando
	// ejecutado, el mismo spec queda trazado entero
	tr, err := Trace(root, ids, cmds)
	if err != nil || len(tr.MissingTests) != 0 || tr.ByCmd != 1 || tr.ByTest != 1 {
		t.Fatalf("CA-272: Trace = Tokens + comandos: %+v %v", tr, err)
	}
}
