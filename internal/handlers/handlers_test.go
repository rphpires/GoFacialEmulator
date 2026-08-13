package handlers

import (
	"context"
	"testing"

	"GoFacialEmulator/internal/config"
	"GoFacialEmulator/internal/database"
	"GoFacialEmulator/internal/monitoring"
)

// TestRegisterWxsChecker cobre a decisão de main.go/handlers.go que faz uma
// instalação nova (sem WXS configurado) nunca ficar presa em ❌: dado wxsDB
// nil (o que cmd/emulator-service/main.go garante quando o host efetivo —
// settings do banco, com fallback pro config.yaml — está vazio), o
// componente wxs_db não deve aparecer no payload de /monitoring/health;
// dado um wxsDB configurado, deve aparecer.
//
// NewHandler completo exige um *emulator.Manager e um DBInterface de
// verdade (Ping, Query, etc. usados por outros checkers/rotas), o que
// arrastaria pra este teste dependências de banco que não vêm ao caso aqui.
// registerWxsChecker foi extraído de NewHandler exatamente pra isolar essa
// decisão: testamos ela sozinha, com um HealthMonitor limpo.
func TestRegisterWxsChecker(t *testing.T) {
	t.Run("wxsDB nil não registra componente wxs_db", func(t *testing.T) {
		hm := monitoring.NewHealthMonitor()
		registerWxsChecker(hm, nil)

		result := hm.CheckAll(context.Background())
		components, ok := result["components"].(map[string]monitoring.ComponentHealth)
		if !ok {
			t.Fatalf("result[\"components\"] não é map[string]ComponentHealth: %#v", result["components"])
		}
		if _, existe := components["wxs_db"]; existe {
			t.Errorf("wxs_db não deveria aparecer no payload quando WXS não está configurado (wxsDB nil)")
		}

		status, ok := result["status"].(monitoring.HealthStatus)
		if !ok {
			t.Fatalf("result[\"status\"] não é HealthStatus: %#v", result["status"])
		}
		if status != monitoring.HealthStatusHealthy {
			t.Errorf("status geral = %q, quero %q — sem checkers registrados o zero-value já é healthy", status, monitoring.HealthStatusHealthy)
		}
	})

	t.Run("wxsDB configurado registra wxs_db como degraded quando o ping falha", func(t *testing.T) {
		// database.NewWxsDB nunca faz ping na construção (só abre o *sql.DB
		// de forma preguiçosa — ver internal/database/wxs_db.go), então dá
		// pra construir um WxsDB "configurado" sem precisar de um SQL
		// Server de verdade; só o ping, feito pelo checker dentro de
		// CheckAll, é que vai falhar contra esse host inexistente.
		wxsDB, err := database.NewWxsDB(config.DatabaseConfig{
			Host:     "host-que-nao-existe.invalid",
			Port:     1433,
			Database: "W_Access",
			Username: "sa",
			Password: "x",
			Driver:   "mssql",
		})
		if err != nil {
			t.Fatalf("NewWxsDB: %v", err)
		}
		defer wxsDB.Close()

		hm := monitoring.NewHealthMonitor()
		registerWxsChecker(hm, wxsDB)

		result := hm.CheckAll(context.Background())
		components, ok := result["components"].(map[string]monitoring.ComponentHealth)
		if !ok {
			t.Fatalf("result[\"components\"] não é map[string]ComponentHealth: %#v", result["components"])
		}

		wxs, existe := components["wxs_db"]
		if !existe {
			t.Fatalf("wxs_db deveria aparecer no payload quando WXS está configurado")
		}
		if wxs.Status != monitoring.HealthStatusDegraded {
			t.Errorf("wxs_db.status = %q, quero %q — ping falho de uma dependência opcional não pode virar unhealthy/503", wxs.Status, monitoring.HealthStatusDegraded)
		}

		status, ok := result["status"].(monitoring.HealthStatus)
		if !ok {
			t.Fatalf("result[\"status\"] não é HealthStatus: %#v", result["status"])
		}
		if status != monitoring.HealthStatusDegraded {
			t.Errorf("status geral = %q, quero %q", status, monitoring.HealthStatusDegraded)
		}
	})
}
