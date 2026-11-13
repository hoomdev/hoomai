// Tests adversariales del spec .hoom/specs/contencion-del-writer.md sobre el
// piloto puro (CA-692): un sobre cortado por git-ajeno o por
// config-de-agentes es, para el piloto, uno cortado por material-de-git: no
// se releva aunque traiga corte tiempo, no se reintenta, y se detiene antes
// del proximo guardado con el Why del caso 8. Estados armados a mano, sin
// disco (como piloto_writer_test.go).
package boardcmd

import (
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// cdwMarca es una de las tres marcas equivalentes del sobre.
type cdwMarca int

const (
	cdwMaterial cdwMarca = iota
	cdwGitAjeno
	cdwConfig
)

func (m cdwMarca) String() string {
	return [...]string{"TocoMaterialGit", "TocoGitAjeno", "TocoConfigDeAgentes"}[m]
}

func cdwMarcar(s Sobre, m cdwMarca) Sobre {
	switch m {
	case cdwMaterial:
		s.TocoMaterialGit = true
	case cdwGitAjeno:
		s.TocoGitAjeno = true
	case cdwConfig:
		s.TocoConfigDeAgentes = true
	}
	return s
}

const cdwWhy8 = "el trabajo del writer no cerro entregable (no-entregable): el piloto se detiene ante un rojo"

// CA-692: el writer cortado por tiempo que ademas toco git ajeno o la config
// de los agentes no se releva (ni con Resume ni sin el): caso 8, como el
// que toco el material; sin marca, si se releva (control).
func TestCA692_ConCorteTiempoNoSeReleva(t *testing.T) {
	const ca = "CA-692"
	c := wpTarjeta(ColWriter, wpAccion(ActPedirWriter, "writer", wpClaude))
	s := wpSobre("writer", "claude", "run", envelope.StatusNotDeliverable, ColWriter, nil)
	s.Record.Corte, s.Record.Pedido, s.SesionID = envelope.CorteTiempo, "implementa", "ses-1"
	for _, conResume := range []bool{true, false} {
		e := Estado{Tarjeta: c, Sobres: []Sobre{s}}
		if conResume {
			e.Proveedores = []providers.Info{{Name: "claude", Capabilities: providers.Capabilities{Resume: true}}}
		}
		if d := Piloto(e); !d.Launch || d.Regla != ReglaRelevoPorTiempo {
			t.Fatalf("%s: control: sin marca el corte por tiempo se releva: %+v", ca, d)
		}
		for _, m := range []cdwMarca{cdwGitAjeno, cdwConfig, cdwMaterial} {
			e.Sobres = []Sobre{cdwMarcar(s, m)}
			d := Piloto(e)
			if d.Launch || d.Caso != 8 || d.Why != cdwWhy8 || d.Regla == ReglaRelevoPorTiempo {
				t.Fatalf("%s: %s con corte tiempo (Resume %v) no se releva: caso 8 con %q: %+v", ca, m, conResume, cdwWhy8, d)
			}
		}
	}
}

// CA-692: el writer cortado en scope por git-ajeno o config-de-agentes no se
// reintenta aunque la politica tenga reintentos: caso 8.
func TestCA692_NoSeReintenta(t *testing.T) {
	const ca = "CA-692"
	c := wpTarjeta(ColWriter, wpAccion(ActPedirWriter, "writer", wpClaude))
	rojo := wpVeredicto("v1", map[string]string{"test": verdict.StatusFail})
	for _, s := range []Sobre{
		wpSobre("writer", "claude", "scope", envelope.StatusNotDeliverable, ColWriter, nil),
		wpSobre("writer", "claude", "verify", envelope.StatusNotDeliverable, ColWriter, rojo),
	} {
		e := Estado{Tarjeta: c, Politica: manifest.AutonomiaPolicy{ReintentosAnteRojo: 2}}
		for _, m := range []cdwMarca{cdwGitAjeno, cdwConfig} {
			e.Sobres = []Sobre{cdwMarcar(s, m)}
			d := Piloto(e)
			if d.Launch || d.Regla == ReglaReintento || d.Caso != 8 || d.Why != cdwWhy8 {
				t.Fatalf("%s: %s en %s no se reintenta: caso 8: %+v", ca, m, s.Record.Stage, d)
			}
		}
	}
}

// CA-692: ParadaAntesDeGuardar se detiene (caso 8, su Why) cuando el sobre
// mas nuevo toco git ajeno o la config de los agentes, igual que con el
// material; uno viejo no cuenta; sin marca no se detiene.
func TestCA692_ParadaAntesDeGuardar(t *testing.T) {
	const ca = "CA-692"
	limpio := wpSobre("writer", "claude", "ok", envelope.StatusDeliverable, ColWriter, wpVeredicto("v1", nil))
	cortado := wpSobre("writer", "claude", "scope", envelope.StatusNotDeliverable, ColWriter, nil)
	if _, para := ParadaAntesDeGuardar([]Sobre{limpio, cortado}); para {
		t.Fatalf("%s: control: sin marca no se detiene antes de guardar", ca)
	}
	dMat, paraMat := ParadaAntesDeGuardar([]Sobre{limpio, cdwMarcar(cortado, cdwMaterial)})
	if !paraMat || dMat.Caso != 8 || dMat.Launch {
		t.Fatalf("%s: fixture: con el material se detiene en el caso 8: %+v", ca, dMat)
	}
	for _, m := range []cdwMarca{cdwGitAjeno, cdwConfig} {
		d, para := ParadaAntesDeGuardar([]Sobre{limpio, cdwMarcar(cortado, m)})
		if !para || !reflect.DeepEqual(d, dMat) {
			t.Fatalf("%s: %s detiene el guardado como el material: %+v vs %+v", ca, m, d, dMat)
		}
		if _, para := ParadaAntesDeGuardar([]Sobre{cdwMarcar(cortado, m), limpio}); para {
			t.Fatalf("%s: %s en un sobre viejo no detiene el guardado de uno nuevo limpio", ca, m)
		}
	}
	if _, para := ParadaAntesDeGuardar(nil); para {
		t.Fatalf("%s: sin sobres no se detiene", ca)
	}
}

// cdwEstado genera un Estado de la columna writer con un sobre nuevo de un
// rol, etapa y corte al azar, y la politica y los proveedores al azar.
type cdwEstado struct {
	e     Estado
	marca cdwMarca
}

func (cdwEstado) Generate(r *rand.Rand, _ int) reflect.Value {
	roles := []string{"writer", "test-writer", "refutador"}
	cols := map[string]string{"writer": ColWriter, "test-writer": ColTestWriter, "refutador": ColReview}
	rol := roles[r.Intn(len(roles))]
	col := cols[rol]
	etapas := []string{"run", "scope", "verify", "check"}
	etapa := etapas[r.Intn(len(etapas))]
	var v *verdict.Verdict
	if etapa == "verify" || etapa == "check" {
		gates := map[string]string{}
		if r.Intn(2) == 0 {
			gates["test"] = verdict.StatusFail
		}
		v = wpVeredicto("v"+etapa, gates)
	}
	s := wpSobre(rol, []string{"claude", "codex"}[r.Intn(2)], etapa, envelope.StatusNotDeliverable, col, v)
	if r.Intn(2) == 0 {
		s.Record.Corte, s.Record.Pedido, s.SesionID = envelope.CorteTiempo, "pedido", "ses-x"
	}
	var previos []Sobre
	for i := r.Intn(3); i > 0; i-- {
		previos = append(previos, wpSobre("test-writer", "claude", "verify", envelope.StatusNotDeliverable, ColTestWriter,
			wpVeredicto("vp", map[string]string{"test": verdict.StatusFail})))
	}
	c := wpTarjeta(col, wpAccion(ActPedirWriter, "writer", wpClaude), wpAccion(ActPedirTestWriter, "test-writer", wpClaude))
	e := Estado{Tarjeta: c, Sobres: append(previos, s), Politica: manifest.AutonomiaPolicy{ReintentosAnteRojo: r.Intn(3)}}
	if r.Intn(2) == 0 {
		e.Proveedores = []providers.Info{{Name: "claude", Capabilities: providers.Capabilities{Resume: true}}}
	}
	return reflect.ValueOf(cdwEstado{e: e, marca: cdwMarca(r.Intn(3))})
}

// CA-692 (propiedad): para cualquier Estado, Piloto y ParadaAntesDeGuardar
// deciden lo mismo con TocoGitAjeno, con TocoConfigDeAgentes y con
// TocoMaterialGit en el sobre mas nuevo.
func TestCA692_PropiedadLasTresMarcasSonLaMisma(t *testing.T) {
	const ca = "CA-692"
	f := func(g cdwEstado) bool {
		con := func(m cdwMarca) Estado {
			e := g.e
			e.Sobres = append([]Sobre(nil), g.e.Sobres...)
			n := len(e.Sobres) - 1
			e.Sobres[n] = cdwMarcar(e.Sobres[n], m)
			return e
		}
		ref := Piloto(con(cdwMaterial))
		refP, refPara := ParadaAntesDeGuardar(con(cdwMaterial).Sobres)
		for _, m := range []cdwMarca{cdwGitAjeno, cdwConfig} {
			if d := Piloto(con(m)); !reflect.DeepEqual(d, ref) {
				t.Logf("Piloto con %s: %+v; con TocoMaterialGit: %+v", m, d, ref)
				return false
			}
			if d, para := ParadaAntesDeGuardar(con(m).Sobres); para != refPara || !reflect.DeepEqual(d, refP) {
				t.Logf("ParadaAntesDeGuardar con %s: %v %+v; con TocoMaterialGit: %v %+v", m, para, d, refPara, refP)
				return false
			}
		}
		if ref.Launch && ref.Regla == ReglaRelevoPorTiempo {
			t.Logf("un sobre marcado nunca se releva: %+v", ref)
			return false
		}
		return true
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 300}); err != nil {
		t.Fatalf("%s: %v", ca, err)
	}
}
