package monitoring

import (
	"context"
	"errors"
	"testing"
)

// statsVazio é um getStatsFn mínimo pros checkers de teste — o conteúdo não
// importa pra esses testes, só precisa não ser nil.
func statsVazio() map[string]interface{} { return map[string]interface{}{} }

// checkerFalso implementa HealthChecker com um resultado fixo, pra testar a
// agregação de status do HealthMonitor sem precisar de bancos reais.
type checkerFalso struct {
	nome   string
	status HealthStatus
}

func (c *checkerFalso) Name() string { return c.nome }
func (c *checkerFalso) Check(ctx context.Context) ComponentHealth {
	return ComponentHealth{Name: c.nome, Status: c.status}
}

// TestCheckAll_StatusGeral cobre a regra de agregação do HealthMonitor: só
// um componente "unhealthy" deve derrubar o status geral (e, por
// consequência em quickHealthCheck, o HTTP para 503). "degraded" não deve.
// Esta é a regressão que faria toda instalação nova (WXS opcional, não
// configurado) reportar ❌ em INICIAR.bat/iniciar.sh pra sempre.
func TestCheckAll_StatusGeral(t *testing.T) {
	casos := []struct {
		nome      string
		registrar func(hm *HealthMonitor)
		quero     HealthStatus
	}{
		{
			nome: "só checkers saudáveis reporta healthy",
			registrar: func(hm *HealthMonitor) {
				hm.RegisterChecker(&checkerFalso{nome: "a", status: HealthStatusHealthy})
				hm.RegisterChecker(&checkerFalso{nome: "b", status: HealthStatusHealthy})
			},
			quero: HealthStatusHealthy,
		},
		{
			// Simula o checker real de wxs_db (NewDatabaseHealthCheckerWithFailureStatus
			// com HealthStatusDegraded), a mesma construção usada em
			// internal/handlers/handlers.go.
			nome: "wxs_db com ping falho reporta degraded, não unhealthy",
			registrar: func(hm *HealthMonitor) {
				hm.RegisterChecker(&checkerFalso{nome: "service_db", status: HealthStatusHealthy})
				hm.RegisterChecker(NewDatabaseHealthCheckerWithFailureStatus(
					"wxs_db",
					func(ctx context.Context) error { return errors.New("connection refused") },
					statsVazio,
					HealthStatusDegraded,
				))
			},
			quero: HealthStatusDegraded,
		},
		{
			// Simula service_db/emulator_db (NewDatabaseHealthChecker, sem
			// customizar o failureStatus): dependência obrigatória, ping
			// falho deve continuar derrubando o status geral.
			nome: "banco obrigatório com ping falho reporta unhealthy",
			registrar: func(hm *HealthMonitor) {
				hm.RegisterChecker(NewDatabaseHealthChecker(
					"service_db",
					func(ctx context.Context) error { return errors.New("connection refused") },
					statsVazio,
				))
			},
			quero: HealthStatusUnhealthy,
		},
		{
			// Mistura: uma dependência opcional degradada não deve mascarar
			// uma dependência obrigatória de fato fora do ar.
			nome: "banco obrigatório fora do ar prevalece sobre wxs_db degradado",
			registrar: func(hm *HealthMonitor) {
				hm.RegisterChecker(NewDatabaseHealthChecker(
					"service_db",
					func(ctx context.Context) error { return errors.New("connection refused") },
					statsVazio,
				))
				hm.RegisterChecker(NewDatabaseHealthCheckerWithFailureStatus(
					"wxs_db",
					func(ctx context.Context) error { return errors.New("connection refused") },
					statsVazio,
					HealthStatusDegraded,
				))
			},
			quero: HealthStatusUnhealthy,
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			hm := NewHealthMonitor()
			c.registrar(hm)

			result := hm.CheckAll(context.Background())
			status, ok := result["status"].(HealthStatus)
			if !ok {
				t.Fatalf("result[\"status\"] não é HealthStatus: %#v", result["status"])
			}
			if status != c.quero {
				t.Errorf("status geral = %q, quero %q", status, c.quero)
			}
		})
	}
}

// TestCheckAll_ComponenteWxsDbApareceComoDegraded confere o componente
// individual, não só o agregado: o payload precisa trazer wxs_db com
// status "degraded" quando o ping falha, pra quem olhar /monitoring/health
// entender o motivo sem precisar adivinhar a partir do status geral.
func TestCheckAll_ComponenteWxsDbApareceComoDegraded(t *testing.T) {
	hm := NewHealthMonitor()
	hm.RegisterChecker(NewDatabaseHealthCheckerWithFailureStatus(
		"wxs_db",
		func(ctx context.Context) error { return errors.New("dial tcp: no such host") },
		statsVazio,
		HealthStatusDegraded,
	))

	result := hm.CheckAll(context.Background())
	components, ok := result["components"].(map[string]ComponentHealth)
	if !ok {
		t.Fatalf("result[\"components\"] não é map[string]ComponentHealth: %#v", result["components"])
	}

	wxs, existe := components["wxs_db"]
	if !existe {
		t.Fatalf("componente wxs_db ausente do payload")
	}
	if wxs.Status != HealthStatusDegraded {
		t.Errorf("wxs_db.status = %q, quero %q", wxs.Status, HealthStatusDegraded)
	}
	if wxs.Message == "" {
		t.Errorf("wxs_db.message vazio; deveria explicar por que o ping falhou")
	}
}

// TestNewDatabaseHealthChecker_PadraoUsaUnhealthy garante que o construtor
// original (usado por service_db e emulator_db) continua reportando
// "unhealthy" no ping falho por padrão — ou seja, que dar um status
// customizado ao wxs_db não amoleceu as dependências obrigatórias.
func TestNewDatabaseHealthChecker_PadraoUsaUnhealthy(t *testing.T) {
	checker := NewDatabaseHealthChecker(
		"service_db",
		func(ctx context.Context) error { return errors.New("connection refused") },
		statsVazio,
	)

	health := checker.Check(context.Background())
	if health.Status != HealthStatusUnhealthy {
		t.Errorf("status = %q, quero %q — NewDatabaseHealthChecker não deveria amolecer dependências obrigatórias", health.Status, HealthStatusUnhealthy)
	}
}
