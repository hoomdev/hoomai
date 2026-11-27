package agentcmd

import (
	"path/filepath"
	"testing"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/manifest"
)

// TestSubdirectorio_ScopeYPrechecks verifica que un proyecto en un
// subdirectorio del repositorio Git (ej. electronic_invoices/) normalice las
// rutas relativas al proyecto antes de evaluar allow/deny/universal,
// localChanges y specInHead.
func TestSubdirectorio_ScopeYPrechecks(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	git(t, repo, "config", "user.email", "test@hoom.dev")
	git(t, repo, "config", "user.name", "hoom test")

	sub := filepath.Join(repo, "electronic_invoices")
	write(t, sub, "hoom.yaml", "schema: hoom/v1\nproject: dte\nbase_branch: main\n")
	write(t, sub, "app/Services/DteService.php", "<?php\nclass DteService {}\n")
	write(t, sub, "tests/Feature/ExistenteTest.php", "<?php\nclass ExistenteTest {}\n")
	write(t, sub, ".hoom/specs/b20.md", "# spec B20\n")
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-m", "inicial")

	charRole, err := agents.Lookup("characterizer")
	if err != nil {
		t.Fatal(err)
	}
	writerRole, err := agents.Lookup("writer")
	if err != nil {
		t.Fatal(err)
	}
	m := &manifest.Manifest{}
	charPol := PolicyFor(m, charRole)
	writerPol := PolicyFor(m, writerRole)

	// 1. specInHead encuentra el spec commiteado en el subdirectorio.
	specPath := filepath.Join(sub, ".hoom", "specs", "b20.md")
	if err := specInHead(sub, specPath); err != nil {
		t.Fatalf("specInHead fallo en subdirectorio con spec commiteado: %v", err)
	}

	// 2. Characterizer escribe tests + matriz + fixtures y commitea: 0 violaciones.
	before := Take(sub, "main")
	write(t, sub, "tests/Feature/HuellaEventos12Test.php", "<?php\nclass HuellaEventos12Test {}\n")
	write(t, sub, "tests/Feature/HuellaEventos12Matriz.php", "<?php\nclass HuellaEventos12Matriz {}\n")
	write(t, sub, "tests/fixtures/inv_01.json", `{"tipo":"invalidacion"}`+"\n")

	// Antes del commit, localChanges debe detectar los archivos bajo tests/ (normalizados).
	dirty := localChanges(sub, charPol)
	if len(dirty) != 3 || dirty[0] != "tests/Feature/HuellaEventos12Matriz.php" {
		t.Fatalf("localChanges en subdirectorio = %v; esperaba los 3 archivos bajo tests/", dirty)
	}

	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-m", "Fijar huella 1.2")

	after := Take(sub, "main")
	sc := CheckScope(before, after, charPol)
	if !sc.OK || len(sc.Violations) != 0 {
		t.Fatalf("characterizer en subdirectorio debe pasar en verde: touched=%v violations=%+v", sc.Touched, sc.Violations)
	}

	// 3. Tocar app/ desde characterizer sigue siendo fuera-de-scope.
	beforeApp := Take(sub, "main")
	write(t, sub, "app/Services/DteService.php", "<?php\nclass DteService { public int $x = 1; }\n")
	scApp := CheckScope(beforeApp, Take(sub, "main"), charPol)
	if scApp.OK || len(scApp.Violations) != 1 || scApp.Violations[0].Path != "app/Services/DteService.php" || scApp.Violations[0].Rule != RuleOutOfScope {
		t.Fatalf("editar app/ debe ser fuera-de-scope: %+v", scApp)
	}
	write(t, sub, "app/Services/DteService.php", "<?php\nclass DteService {}\n")

	// 4. Un archivo fuera del subdirectorio (aunque se llame tests/...Test.php en la raiz)
	// NO debe hacer match con tests/** ni **/*Test.php del proyecto.
	beforeOut := Take(sub, "main")
	write(t, repo, "tests/AfueraTest.php", "<?php\n")
	scOut := CheckScope(beforeOut, Take(sub, "main"), charPol)
	if scOut.OK || len(scOut.Violations) != 1 || scOut.Violations[0].Path != "../tests/AfueraTest.php" {
		t.Fatalf("escribir fuera del subdirectorio debe marcar violacion ../tests/AfueraTest.php: %+v", scOut)
	}

	// 5. El writer sigue bloqueado si edita .hoom/specs/ o hoom.yaml en el subdirectorio.
	beforeW := Take(sub, "main")
	write(t, sub, ".hoom/specs/b20.md", "# spec editado\n")
	write(t, sub, "hoom.yaml", "schema: hoom/v1\nproject: dte\nbase_branch: main\n# manipulado\n")
	scW := CheckScope(beforeW, Take(sub, "main"), writerPol)
	if scW.OK || !scW.Tampering {
		t.Fatalf("writer editando spec y hoom.yaml en subdirectorio debe fallar con tampering: %+v", scW)
	}
	sawSpec, sawYAML := false, false
	for _, v := range scW.Violations {
		if v.Path == ".hoom/specs/b20.md" && v.Rule == RuleOutOfScope {
			sawSpec = true
		}
		if v.Path == "hoom.yaml" && v.Rule == RuleTampering {
			sawYAML = true
		}
	}
	if !sawSpec || !sawYAML {
		t.Fatalf("esperaba violaciones para .hoom/specs/b20.md y hoom.yaml: %+v", scW.Violations)
	}
}
