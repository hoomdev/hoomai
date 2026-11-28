package initcmd

import (
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/profiles"
)

func TestInitLaravel_EscribeAplicaEnYDetectaMutacionConDiff(t *testing.T) {
	dir := t.TempDir()
	veeEscribir(t, dir, "artisan", "#!/usr/bin/env php\n", 0o644)
	veeEscribir(t, dir, "composer.json", `{
  "require-dev": { "pestphp/pest": "^3.0", "phpstan/phpstan": "^1.0", "laravel/pint": "^1.0" }
}`, 0o644)
	veeEscribir(t, dir, "package.json", `{
  "scripts": { "build": "vite build", "test": "vitest run" },
  "dependencies": { "vue": "^3.4.0" }
}`, 0o644)
	veeEscribir(t, dir, "scripts/mutacion.sh", "#!/bin/sh\n# soporta --diff <base>\nexit 0\n", 0o755)

	if _, err := veeCorrer(t, dir, "laravel"); err != nil {
		t.Fatalf("init laravel: %v", err)
	}
	m, err := manifest.Load(dir, profiles.Resolve)
	if err != nil {
		t.Fatalf("manifest.Load: %v", err)
	}
	if got := m.Gates["mutation"].DiffCmdStr(); got != "sh scripts/mutacion.sh --diff {base}" {
		t.Fatalf("mutation.diff_cmd = %q, esperaba modo --diff {base}", got)
	}
	if len(m.Gates["mutation"].AplicaEn) == 0 {
		t.Fatalf("mutation debia quedar con aplica_en en init laravel")
	}
	if len(m.Gates["static"].AplicaEn) == 0 {
		t.Fatalf("static debia quedar con aplica_en en init laravel")
	}
	if len(m.Gates["test_vue"].AplicaEn) == 0 {
		t.Fatalf("test_vue debia quedar con aplica_en en init laravel")
	}
}

func TestAuditarGates_AvisaMutacionGlobalYThreadsMax(t *testing.T) {
	dir := t.TempDir()
	veeEscribir(t, dir, "scripts/mutacion.sh", "#!/bin/sh\nexit 0\n", 0o755)
	veeEscribir(t, dir, "hoom.yaml", `schema: hoom/v1
project: demo
profile: laravel
base_branch: main
policy: strict
gates:
  mutation:
    required: true
    cmd: "vendor/bin/infection --threads=max --min-msi=60"
    diff_cmd: "vendor/bin/infection --threads=max --min-msi=60"
`, 0o644)
	veeEscribir(t, dir, "vendor/bin/infection", "#!/bin/sh\n", 0o755)

	m, err := manifest.Load(dir, profiles.Resolve)
	if err != nil {
		t.Fatal(err)
	}
	avisos := strings.Join(AuditarGates(dir, m), "\n")
	if !strings.Contains(avisos, "diff_cmd") {
		t.Fatalf("esperaba aviso por mutation sin diff_cmd distinto de cmd, got: %q", avisos)
	}
	if !strings.Contains(avisos, "--threads=max") {
		t.Fatalf("esperaba aviso por --threads=max, got: %q", avisos)
	}
}

func TestUpgradeDefaults_OptimizaProyectoExistenteYEsIdempotente(t *testing.T) {
	dir := t.TempDir()
	original := `schema: hoom/v1
project: oceanbuild
profile: laravel
base_branch: main
policy: strict
rutas:
  de_riesgo:
    - "app/Services/Pagos/**"
findings:
  block_on: high
  max_pendientes: 12
  max_rondas: 4
agents:
  writer:
    model: fuerte
  test-writer:
    model: fuerte
    write:
      allow: ["tests/**"]
review:
  model: fuerte
  effort: high
  concurrencia: 1
gates:
  static:
    required: true
    cmd: "vendor/bin/phpstan analyse --no-progress --error-format=raw"
  mutation:
    required: true
    cmd: "vendor/bin/infection --threads=max --no-progress --min-msi=60"
  test_vue:
    required: true
    cmd: "npm test"
  lint:
    required: false
    cmd: ""
`
	veeEscribir(t, dir, "hoom.yaml", original, 0o644)
	veeEscribir(t, dir, "vendor/bin/phpstan", "#!/bin/sh\n", 0o755)
	veeEscribir(t, dir, "vendor/bin/infection", "#!/bin/sh\n", 0o755)

	// 1. Dry-run no modifica disco ni crea .hoom/agents/.
	dry, err := Upgrade(dir, UpgradeOptions{Defaults: true, DryRun: true})
	if err != nil {
		t.Fatalf("Upgrade --defaults --dry-run: %v", err)
	}
	if len(dry.Updated) == 0 || !dry.AgentsUpdated {
		t.Fatalf("dry-run debia reportar cambios y agentes: %+v", dry)
	}
	if got := veeLeer(t, dir+"/hoom.yaml"); got != original {
		t.Fatalf("dry-run no debe modificar hoom.yaml")
	}

	// 2. Corrida real aplica defaults de ahorro de tokens, CPU, diff_cmd, aplica_en y .hoom/agents/.
	res, err := Upgrade(dir, UpgradeOptions{Defaults: true})
	if err != nil {
		t.Fatalf("Upgrade --defaults: %v", err)
	}
	if !res.AgentsUpdated {
		t.Fatalf("debia actualizar .hoom/agents/")
	}
	if !fileExists(dir, ".hoom/agents/00-orquestador.md") {
		t.Fatalf("debia instalar .hoom/agents/00-orquestador.md")
	}

	m, err := manifest.Load(dir, profiles.Resolve)
	if err != nil {
		t.Fatalf("manifest.Load tras --defaults: %v", err)
	}
	// Preserva rutas, max_pendientes y test-writer.write; actualiza niveles/review/rondas/spec.
	if m.Rutas == nil || len(m.Rutas.DeRiesgo) != 1 || m.Rutas.DeRiesgo[0] != "app/Services/Pagos/**" {
		t.Fatalf("rutas.de_riesgo debe preservarse: %+v", m.Rutas)
	}
	if m.FindingsMaxRondas() != 1 || m.Findings == nil || m.Findings.MaxPendientes == nil || *m.Findings.MaxPendientes != 12 {
		t.Fatalf("findings debe actualizar max_rondas=1 preservando max_pendientes=12: %+v", m.Findings)
	}
	if m.Review == nil || m.Review.Model != "medio" || m.Review.ConcurrenciaOrDefault() != 4 {
		t.Fatalf("review debe quedar calibrada (medio, concurrencia 4): %+v", m.Review)
	}
	if m.Agents == nil || m.Agents["writer"].Model != "medio" {
		t.Fatalf("agents.writer debe pasar al nivel medio: %+v", m.Agents)
	}
	if tw := m.Agents["test-writer"]; len(tw.Write.Allow) != 1 || tw.Write.Allow[0] != "tests/**" {
		t.Fatalf("agents.test-writer.write debe preservarse: %+v", tw)
	}
	// Gates optimizados: --threads={threads}, diff_cmd, aplica_en, y lint ausente intacto.
	mut := m.Gates["mutation"]
	if strings.Contains(mut.CmdStr(), "--threads=max") || !strings.Contains(mut.CmdStr(), "--threads={threads}") {
		t.Fatalf("mutation.cmd debia cambiar --threads=max a {threads}: %q", mut.CmdStr())
	}
	if !strings.Contains(mut.DiffCmdStr(), "--git-diff-base={base}") {
		t.Fatalf("mutation.diff_cmd debia quedar acotado al diff: %q", mut.DiffCmdStr())
	}
	if len(mut.AplicaEn) == 0 || len(m.Gates["static"].AplicaEn) == 0 || len(m.Gates["test_vue"].AplicaEn) == 0 {
		t.Fatalf("gates debian recibir aplica_en por stack: mut=%v static=%v vue=%v",
			mut.AplicaEn, m.Gates["static"].AplicaEn, m.Gates["test_vue"].AplicaEn)
	}
	if m.Gates["lint"].CmdStr() != "" || len(m.Gates["lint"].AplicaEn) != 0 {
		t.Fatalf("gate ausente lint no debe alterarse: %+v", m.Gates["lint"])
	}
	// Sin avisos de --threads=max ni de mutation sin diff_cmd tras optimizar.
	joined := strings.Join(res.Warnings, "\n")
	if strings.Contains(joined, "--threads=max") || strings.Contains(joined, "diff_cmd") {
		t.Fatalf("no debia quedar aviso ya resuelto tras --defaults: %v", res.Warnings)
	}

	// 3. Idempotencia: segunda corrida no reporta nuevos cambios en hoom.yaml.
	res2, err := Upgrade(dir, UpgradeOptions{Defaults: true})
	if err != nil {
		t.Fatalf("segundo Upgrade --defaults: %v", err)
	}
	if len(res2.Added) != 0 || len(res2.Updated) != 0 {
		t.Fatalf("segundo Upgrade --defaults debia ser idempotente, got added=%v updated=%v", res2.Added, res2.Updated)
	}
}

