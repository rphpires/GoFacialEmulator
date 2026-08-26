package database

import (
	"context"
	"strings"
	"testing"
)

// GetWxsSettingsFromDB precisa filtrar host vazio na própria query. Uma
// linha-sentinela gravada por SetSyncEnabled (host='', porta=0, ...) só
// existe para guardar o toggle de sync — ela não é uma conexão de W-Access
// configurada. Sem o filtro, essa linha vira "a configuração gravada" pela
// simples ordenação por id, e cmd/emulator-service/main.go, ao ver a
// chamada ter sucesso em vez de pgx.ErrNoRows, nunca cai no fallback para
// configs/config.yaml/WXS_*/WXS_DB_*: o operador perde a conexão que
// configurou por arquivo/env assim que toca no toggle de sync uma vez.
//
// O fake não filtra de verdade (linhaFalsa/rowsFalsas devolvem sempre o
// mesmo valor fixo), então o teste confirma pelo texto da query emitida —
// no estilo dos demais testes deste pacote — que a cláusula está presente.
func TestGetWxsSettingsFromDBFiltraHostVazioNaQuery(t *testing.T) {
	db := &dbFalso{linha: linhaFalsa{
		valores: []interface{}{1, "", 0, "", "", ""},
	}}

	_, _ = GetWxsSettingsFromDB(context.Background(), db)

	if len(db.queryRowSQLs) != 1 {
		t.Fatalf("quero 1 QueryRow, tenho %d: %v", len(db.queryRowSQLs), db.queryRowSQLs)
	}
	query := db.queryRowSQLs[0]
	if !strings.Contains(query, "host <> ''") {
		t.Errorf("quero a query filtrando host vazio (\"host <> ''\"), tenho %q", query)
	}
	if !strings.Contains(query, "FROM service.wxs_settings") {
		t.Errorf("quero a query lendo de service.wxs_settings, tenho %q", query)
	}
}

// Pino de intenção: o filtro "WHERE host <> ''" em GetWxsSettingsFromDB não
// é acidental nem cosmético — é o que faz uma linha-sentinela (gravada só
// para guardar sync_enabled, sem conexão nenhuma) responder pgx.ErrNoRows
// nesta função, exatamente como se não houvesse linha alguma. Removê-lo
// "para simplificar" reintroduz a regressão: no próximo boot, uma
// instalação com W-Access configurado via configs/config.yaml ou
// WXS_*/WXS_DB_* (sem nunca ter passado pela tela de configurações) tem sua
// conexão descartada silenciosamente assim que o operador flipar o toggle
// de sync pela primeira vez, porque main.go só cai no fallback do arquivo
// quando esta função devolve erro — não quando ela devolve uma linha com
// host vazio. GetSyncEnabled e SetSyncEnabled continuam lendo/gravando a
// linha mais recente SEM esse filtro, de propósito: são eles que respondem
// "qual é o valor do toggle?", e o toggle mora até numa linha-sentinela.
func TestGetWxsSettingsFromDBFiltroHostVazioNaoEhSimplificavel(t *testing.T) {
	db := &dbFalso{linha: linhaFalsa{
		valores: []interface{}{1, "", 0, "", "", ""},
	}}

	_, _ = GetWxsSettingsFromDB(context.Background(), db)

	query := db.queryRowSQLs[0]
	if !strings.Contains(query, "host <> ''") {
		t.Fatal(`GetWxsSettingsFromDB tem que filtrar "WHERE host <> ''": uma linha-sentinela ` +
			"gravada por SetSyncEnabled (só o toggle, sem conexão) não pode ser devolvida " +
			"como se fosse a configuração de W-Access gravada — main.go só cai no fallback " +
			"de configs/config.yaml/WXS_* quando esta função devolve pgx.ErrNoRows.")
	}
}
