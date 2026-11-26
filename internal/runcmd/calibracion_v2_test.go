package runcmd

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStartAllowConcurrentRechazaSiHayRunAjenoAunqueHayaUnoPropio(t *testing.T) {
	root := t.TempDir()
	// Simula un run ajeno (de otro proceso vivo, nuestro propio PID pero fuera de m.runs)
	// mas viejo que un run propio concurrente: activeIDs ve ambos y debe rechazar.
	m := NewManager(root)
	foreignID := "20261126T000001_f0re1g"
	if err := os.MkdirAll(runsDir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeMeta(root, Meta{
		ID:        foreignID,
		Provider:  "codex",
		Dir:       root,
		CreatedAt: time.Now().UTC().Add(-2 * time.Minute),
		Status:    StatusRunning,
		ExitCode:  -1,
		PID:       os.Getpid(),
	}); err != nil {
		t.Fatal(err)
	}
	// Run propio mas reciente con AllowConcurrent=true
	ownID := "20261126T000002_0wn001"
	m.runs[ownID] = &run{
		info: Run{ID: ownID, Status: StatusRunning},
		dir:  root,
		opts: StartOptions{AllowConcurrent: true},
	}
	if err := writeMeta(root, Meta{
		ID:        ownID,
		Provider:  "codex",
		Dir:       root,
		CreatedAt: time.Now().UTC(),
		Status:    StatusRunning,
		ExitCode:  -1,
		PID:       os.Getpid(),
	}); err != nil {
		t.Fatal(err)
	}

	// Instalar un CLI falso en PATH para que providers.Lookup("codex") pase el chequeo de instalado.
	bin := t.TempDir()
	fake := filepath.Join(bin, "codex")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, err := m.Start(StartOptions{
		Provider:        "codex",
		Prompt:          "hola",
		Dir:             root,
		AllowConcurrent: true,
	})
	var busy ErrBusy
	if !errors.As(err, &busy) || busy.RunID != foreignID {
		t.Fatalf("Start con AllowConcurrent debe rechazar por el run ajeno %s aunque el mas reciente sea propio; err=%v", foreignID, err)
	}
}
