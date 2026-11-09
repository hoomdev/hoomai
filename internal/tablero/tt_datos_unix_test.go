//go:build !windows

// Datos.Archivo y los enlaces que salen del proyecto (CA-559; spec
// tablero-con-paneles, "Los datos": las lineas del archivo de un hallazgo se
// leen por el os.Root del proyecto, y "una ruta que sale del proyecto no se
// lee ni se abre"). Un enlace del proyecto a un archivo o a un directorio de
// afuera da Fuera y ninguna linea; uno a un archivo de adentro se lee. Solo
// Unix: los enlaces se crean con os.Symlink.
package tablero_test

import (
	"os"
	"path/filepath"
	"testing"
)

// CA-559: Archivo por un enlace que sale de la raiz: Fuera, sin lineas (nada
// se leyo); por uno que queda adentro: las lineas del archivo.
func TestCA559_DatosDeArchivoPorUnEnlace(t *testing.T) {
	root, _ := ttProyecto(t)
	fuera, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ttArchivo(t, fuera, "secreto.go", "no se lee\n")
	ttArchivo(t, fuera, "dir/x.go", "tampoco\n")
	ttArchivo(t, root, "real.go", "uno\ndos\n")
	for enlace, destino := range map[string]string{
		"flink":  filepath.Join(fuera, "secreto.go"),
		"dlink":  filepath.Join(fuera, "dir"),
		"dentro": "real.go",
	} {
		if err := os.Symlink(destino, filepath.Join(root, enlace)); err != nil {
			t.Fatalf("no pude crear el enlace %s: %v", enlace, err)
		}
	}
	d := ttDatosDe(t, root)
	for _, ruta := range []string{"flink", "dlink/x.go"} {
		a := d.Archivo(ruta, 1)
		if !a.Fuera || len(a.Lineas) != 0 {
			t.Fatalf("CA-559: %s sale del proyecto por un enlace: Fuera y ninguna linea: %+v", ruta, a)
		}
	}
	if a := d.Archivo("dentro", 1); a.Fuera || a.Err != nil || len(a.Lineas) != 2 || a.Lineas[0] != "uno" {
		t.Fatalf("CA-559: un enlace a un archivo de adentro se lee: %+v", a)
	}
}
