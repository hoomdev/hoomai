package initcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/profiles"
)

func TestInitLaravel_DetectaHerramientasRealesYDejaAusentesEnAmbar(t *testing.T) {
	// Proyecto Laravel estilo factura electronica: tiene Pest, PHPStan, Pint y
	// tests/Arch, pero NO tiene package.json, ni scripts/with-node.sh, ni
	// Infection, ni scripts/mutacion.sh.
	dir := t.TempDir()
	veeEscribir(t, dir, "artisan", "#!/usr/bin/env php\n", 0o644)
	veeEscribir(t, dir, "composer.json", `{
  "require": { "laravel/framework": "^11.0" },
  "require-dev": {
    "pestphp/pest": "^3.0",
    "larastan/larastan": "^2.9",
    "laravel/pint": "^1.0"
  }
}`, 0o644)
	veeEscribir(t, dir, "tests/Arch/GatesContractTest.php", "<?php\n", 0o644)

	if _, err := veeCorrer(t, dir, "laravel"); err != nil {
		t.Fatalf("init laravel: %v", err)
	}
	m, err := manifest.Load(dir, profiles.Resolve)
	if err != nil {
		t.Fatalf("manifest.Load: %v", err)
	}

	if got := m.Gates["test"].CmdStr(); !strings.Contains(got, "vendor/bin/pest") || !m.Gates["test"].IsRequired() {
		t.Fatalf("gate test esperaba pest required: %+v", m.Gates["test"])
	}
	if got := m.Gates["arch"].CmdStr(); !strings.Contains(got, "--group=arch") || !m.Gates["arch"].IsRequired() {
		t.Fatalf("gate arch esperaba pest --group=arch required: %+v", m.Gates["arch"])
	}
	if got := m.Gates["static"].CmdStr(); !strings.Contains(got, "phpstan") || !m.Gates["static"].IsRequired() {
		t.Fatalf("gate static esperaba phpstan required: %+v", m.Gates["static"])
	}
	if got := m.Gates["lint"].CmdStr(); !strings.Contains(got, "pint") || m.Gates["lint"].IsRequired() {
		t.Fatalf("gate lint esperaba pint opcional (required=false): %+v", m.Gates["lint"])
	}
	if got := m.Gates["mutation"].CmdStr(); got != "" || m.Gates["mutation"].IsRequired() {
		t.Fatalf("gate mutation debia quedar AUSENTE (cmd vacio, required=false): %+v", m.Gates["mutation"])
	}
	if got := m.Gates["build"].CmdStr(); got != "" || m.Gates["build"].IsRequired() {
		t.Fatalf("gate build debia quedar AUSENTE (cmd vacio, required=false): %+v", m.Gates["build"])
	}
	if _, ok := m.Gates["test_vue"]; ok {
		t.Fatalf("no debe crear test_vue cuando no hay package.json")
	}

	// Preset de ahorro de tokens y tiempo por defecto.
	if m.FindingsBlockOn() != "high" || m.FindingsMaxRondas() != 1 {
		t.Fatalf("findings default: block_on=%q max_rondas=%d", m.FindingsBlockOn(), m.FindingsMaxRondas())
	}
	if m.Review == nil || m.Review.Model != "medio" || m.Review.Effort != "high" || m.Review.ConcurrenciaOrDefault() != 4 {
		t.Fatalf("review default calibrado: %+v", m.Review)
	}
}

func TestInitLaravel_DetectaScriptsFrontendYMutacionCuandoExisten(t *testing.T) {
	// Proyecto Laravel estilo oceanbuild: con Vue, with-node.sh y mutacion.sh.
	dir := t.TempDir()
	veeEscribir(t, dir, "artisan", "#!/usr/bin/env php\n", 0o644)
	veeEscribir(t, dir, "composer.json", `{
  "require-dev": { "pestphp/pest": "^3.0", "phpstan/phpstan": "^1.0", "laravel/pint": "^1.0" }
}`, 0o644)
	veeEscribir(t, dir, "package.json", `{
  "scripts": { "build": "vite build", "test": "vitest run" },
  "dependencies": { "vue": "^3.4.0" }
}`, 0o644)
	veeEscribir(t, dir, "scripts/with-node.sh", "#!/bin/sh\nexec \"$@\"\n", 0o755)
	veeEscribir(t, dir, "scripts/mutacion.sh", "#!/bin/sh\nexit 0\n", 0o755)

	if _, err := veeCorrer(t, dir, "laravel"); err != nil {
		t.Fatalf("init laravel full: %v", err)
	}
	m, err := manifest.Load(dir, profiles.Resolve)
	if err != nil {
		t.Fatalf("manifest.Load: %v", err)
	}
	if got := m.Gates["build"].CmdStr(); got != "sh scripts/with-node.sh npm run build" {
		t.Fatalf("gate build = %q", got)
	}
	if got := m.Gates["test_vue"].CmdStr(); got != "sh scripts/with-node.sh npm test" {
		t.Fatalf("gate test_vue = %q", got)
	}
	if got := m.Gates["mutation"].CmdStr(); got != "sh scripts/mutacion.sh" {
		t.Fatalf("gate mutation = %q", got)
	}
}

func TestInitNode_JSSinTypeScriptDejaStaticAusente(t *testing.T) {
	dir := t.TempDir()
	veeEscribir(t, dir, "package.json", `{"name":"demo","scripts":{"test":"vitest run"}}`, 0o644)
	if _, err := veeCorrer(t, dir, "node"); err != nil {
		t.Fatalf("init node: %v", err)
	}
	m, err := manifest.Load(dir, profiles.Resolve)
	if err != nil {
		t.Fatalf("manifest.Load: %v", err)
	}
	if got := m.Gates["test"].CmdStr(); got != "npm test" {
		t.Fatalf("gate test = %q", got)
	}
	if got := m.Gates["static"].CmdStr(); got != "" {
		t.Fatalf("static en JS puro debe quedar AUSENTE, got=%q", got)
	}
}

func TestUpgrade_AgregaSeccionesFaltantesSinTocarGatesNiRutas(t *testing.T) {
	dir := t.TempDir()
	original := `# Manifiesto propio del proyecto
schema: hoom/v1
project: factura-electronica
profile: laravel
base_branch: main
policy: strict
rutas:
  de_riesgo:
    - "app/Services/DTE/**"
gates:
  build:
    required: false
    cmd: ""
  lint:
    required: false
    cmd: "vendor/bin/pint --test"
  static:
    required: true
    cmd: "vendor/bin/phpstan analyse --no-progress --error-format=raw"
  security:
    required: true
    cmd: "semgrep scan --error --quiet"
  arch:
    required: true
    cmd: "vendor/bin/pest --group=arch --colors=never"
  test:
    required: true
    cmd: "vendor/bin/pest --colors=never"
  mutation:
    required: false
    cmd: ""
`
	veeEscribir(t, dir, "hoom.yaml", original, 0o644)

	// DryRun no modifica el archivo.
	dryRes, err := Upgrade(dir, UpgradeOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Upgrade dry-run: %v", err)
	}
	if len(dryRes.Added) != 4 {
		t.Fatalf("dry-run esperaba 4 secciones faltantes, got %v", dryRes.Added)
	}
	if afterDry := veeLeer(t, filepath.Join(dir, "hoom.yaml")); afterDry != original {
		t.Fatalf("dry-run modifico hoom.yaml")
	}

	// Upgrade real agrega las 4 secciones preservando el contenido original al principio.
	res, err := Upgrade(dir, UpgradeOptions{})
	if err != nil {
		t.Fatalf("Upgrade: %v", err)
	}
	if len(res.Added) != 4 {
		t.Fatalf("Upgrade esperaba agregar 4 secciones, got %v", res.Added)
	}
	after := veeLeer(t, filepath.Join(dir, "hoom.yaml"))
	if !strings.HasPrefix(after, original) {
		t.Fatalf("Upgrade altero el contenido original de hoom.yaml:\n%s", after)
	}

	m, err := manifest.Load(dir, profiles.Resolve)
	if err != nil {
		t.Fatalf("manifest.Load tras Upgrade: %v", err)
	}
	if m.FindingsBlockOn() != "high" || m.FindingsMaxRondas() != 1 {
		t.Fatalf("findings tras upgrade: block_on=%q max_rondas=%d", m.FindingsBlockOn(), m.FindingsMaxRondas())
	}
	if m.Review == nil || m.Review.Model != "medio" || m.Review.Effort != "high" || m.Review.ConcurrenciaOrDefault() != 4 {
		t.Fatalf("review tras upgrade: %+v", m.Review)
	}
	if got := m.Gates["lint"].IsRequired(); got {
		t.Fatalf("Upgrade no debia cambiar lint.required")
	}
	if got := m.Gates["mutation"].CmdStr(); got != "" {
		t.Fatalf("Upgrade no debia cambiar mutation.cmd")
	}

	// Idempotencia: segunda corrida no agrega nada ni modifica el archivo.
	res2, err := Upgrade(dir, UpgradeOptions{})
	if err != nil {
		t.Fatalf("segundo Upgrade: %v", err)
	}
	if len(res2.Added) != 0 {
		t.Fatalf("segundo Upgrade debia agregar 0 secciones, got %v", res2.Added)
	}
	if after2 := veeLeer(t, filepath.Join(dir, "hoom.yaml")); after2 != after {
		t.Fatalf("segundo Upgrade modifico hoom.yaml")
	}
}

func TestUpgrade_RespetaSeccionesYaDefinidasYAuditaScriptsFaltantes(t *testing.T) {
	dir := t.TempDir()
	existing := `schema: hoom/v1
project: demo
profile: laravel
base_branch: main
policy: strict
review:
  model: fuerte
  concurrencia: 2
gates:
  build:
    required: true
    cmd: "sh scripts/with-node.sh npm run build"
  mutation:
    required: true
    cmd: "sh scripts/mutacion.sh"
  test:
    required: true
    cmd: "vendor/bin/pest --colors=never"
`
	veeEscribir(t, dir, "hoom.yaml", existing, 0o644)
	veeEscribir(t, dir, "vendor/bin/pest", "#!/bin/sh\n", 0o755)

	res, err := Upgrade(dir, UpgradeOptions{})
	if err != nil {
		t.Fatalf("Upgrade: %v", err)
	}
	// Solo deben agregarse findings, agents, spec (NO review, que ya existe).
	for _, s := range res.Added {
		if s == "review" {
			t.Fatalf("Upgrade no debe sobreescribir ni duplicar review existente: %v", res.Added)
		}
	}
	m, err := manifest.Load(dir, profiles.Resolve)
	if err != nil {
		t.Fatalf("manifest.Load: %v", err)
	}
	if m.Review.Model != "fuerte" || m.Review.ConcurrenciaOrDefault() != 2 {
		t.Fatalf("Upgrade altero review existente: %+v", m.Review)
	}

	// AuditarGates debe avisar por scripts/with-node.sh y scripts/mutacion.sh, pero NO por vendor/bin/pest (que existe).
	joined := strings.Join(res.Warnings, "\n")
	if !strings.Contains(joined, "scripts/with-node.sh") || !strings.Contains(joined, "scripts/mutacion.sh") {
		t.Fatalf("esperaba avisos por scripts ausentes, got: %v", res.Warnings)
	}
	if strings.Contains(joined, "vendor/bin/pest") {
		t.Fatalf("no debia avisar por vendor/bin/pest que si existe: %v", res.Warnings)
	}
}

func TestUpgrade_ErrorSiNoExisteManifest(t *testing.T) {
	dir := t.TempDir()
	if _, err := Upgrade(dir, UpgradeOptions{}); err == nil {
		t.Fatal("esperaba error si no existe hoom.yaml")
	}
	if _, err := os.Stat(filepath.Join(dir, "hoom.yaml")); !os.IsNotExist(err) {
		t.Fatal("no debe crear hoom.yaml en Upgrade")
	}
}
