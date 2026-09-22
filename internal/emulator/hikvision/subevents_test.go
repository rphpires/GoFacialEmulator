package hikvision

import (
	"math/rand"
	"testing"
)

func TestPickStandaloneSubEvent_OnlyKnownMinors(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	permitido := map[int]bool{
		MinorLegalCardPass:   true,
		MinorCardOutOfDate:   true,
		MinorInvalidCard:     true,
		MinorFingerprintPass: true,
		MinorFingerprintFail: true,
		MinorFaceVerifyPass:  true,
		MinorFaceVerifyFail:  true,
	}
	for i := 0; i < 2000; i++ {
		got := pickStandaloneSubEvent(r)
		if !permitido[got] {
			t.Fatalf("sorteio devolveu subEventType desconhecido: %d", got)
		}
	}
}

func TestPickStandaloneSubEvent_GrantDominates(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	grants := 0
	const n = 2000
	for i := 0; i < n; i++ {
		if subEventIsGrant(pickStandaloneSubEvent(r)) {
			grants++
		}
	}
	// A grande maioria dos acessos num equipamento real é concedida.
	if grants < n*6/10 {
		t.Errorf("acessos concedidos: %d de %d, esperado ao menos 60 por cento", grants, n)
	}
	if grants == n {
		t.Error("nenhuma recusa foi sorteada em 2000 amostras; a variedade nao esta ativa")
	}
}

func TestSubEventIsGrant(t *testing.T) {
	casos := map[int]bool{
		MinorLegalCardPass:   true,
		MinorFingerprintPass: true,
		MinorFaceVerifyPass:  true,
		MinorInvalidCard:     false,
		MinorFaceVerifyFail:  false,
		MinorFingerprintFail: false,
		MinorCardOutOfDate:   false,
	}
	for minor, want := range casos {
		if got := subEventIsGrant(minor); got != want {
			t.Errorf("subEventIsGrant(%d): got %v, want %v", minor, got, want)
		}
	}
}
