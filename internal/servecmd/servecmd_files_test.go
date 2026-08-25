// Tests adversariales del spec .hoom/specs/arroba-archivos.md
// (CA-47, CA-49, CA-52): el endpoint de rutas y el autocompletado @ de la
// UI embebida.
package servecmd

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// CA-47 + CA-49: /api/files responde un array JSON de RUTAS (jamas
// contenido), filtrado por q, con tope de 20; sin git responde [] (CA-50
// via el paquete filesearch).
func TestCA47_CA49_EndpointDeRutas(t *testing.T) {
	dir := newGitProject(t) // trae hoom.yaml commiteado
	s := newServer(t, dir)

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/files?q=hoom", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("CA-47: esperaba 200, obtuve %d", rec.Code)
	}
	var paths []string
	if err := json.Unmarshal(rec.Body.Bytes(), &paths); err != nil {
		t.Fatalf("CA-49: la respuesta debe ser un array JSON de strings: %v", err)
	}
	found := false
	for _, p := range paths {
		if p == "hoom.yaml" {
			found = true
		}
		if strings.Contains(p, "schema: hoom/v1") {
			t.Fatalf("CA-49: la respuesta contiene CONTENIDO de archivo, no solo rutas")
		}
	}
	if !found {
		t.Fatalf("CA-47: hoom.yaml debia matchear la query: %v", paths)
	}
	if len(paths) > 20 {
		t.Fatalf("CA-47: el tope es 20, llegaron %d", len(paths))
	}

	// proyecto sin git: array vacio, sin error (CA-50 en el borde HTTP)
	dir2 := newProject(t)
	s2 := newServer(t, dir2)
	rec2 := httptest.NewRecorder()
	s2.Handler().ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/files?q=x", nil))
	if rec2.Code != http.StatusOK || strings.TrimSpace(rec2.Body.String()) != "[]" {
		t.Fatalf("CA-49: sin git debe responder [] con 200, obtuve %d %q", rec2.Code, rec2.Body.String())
	}
}

// CA-52: la UI embebida trae el autocompletado @ atado a los textareas del
// cockpit y del review, con dropdown e insercion.
func TestCA52_AutocompletadoEnLaUI(t *testing.T) {
	raw, err := fs.ReadFile(uiFS, "ui/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(raw)
	for _, marca := range []string{
		`id="ac"`,                 // el dropdown existe
		"/api/files?q=",           // consulta el endpoint del binario
		"attachFileAutocomplete",  // el componente se ata a los textareas
		`"runprompt", "runinput"`, // los textareas del cockpit
		`"review-text"`,           // el textarea de review de specs
		"ArrowDown",               // navegacion por teclado
		"acInsert",                // insercion de la ruta completa
	} {
		if !strings.Contains(html, marca) {
			t.Fatalf("CA-52: la UI no contiene %q", marca)
		}
	}
}
