package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v4"
)

// GetSyncEnabled diz se o vínculo com o Invenzi está ligado. Sem linha
// explícita em service.wxs_settings, segue wxsConfigurado: uma instalação
// que configura o W-Access via configs/config.yaml ou WXS_*/WXS_DB_* (sem
// nunca ter passado pela tela de configurações, logo sem linha gravada)
// continua sincronizando como já fazia, em vez de ser silenciosamente
// desligada por uma migração. Só uma instalação nova, sem W-Access
// configurado de jeito nenhum, cai em false.
func GetSyncEnabled(ctx context.Context, db DBInterface, wxsConfigurado bool) (bool, error) {
	var ligado bool
	err := db.QueryRow(ctx, `
		SELECT sync_enabled
		FROM service.wxs_settings
		ORDER BY id DESC
		LIMIT 1
	`).Scan(&ligado)

	if errors.Is(err, pgx.ErrNoRows) {
		return wxsConfigurado, nil
	}
	if err != nil {
		return false, fmt.Errorf("erro ao ler sync_enabled: %w", err)
	}
	return ligado, nil
}

// SetSyncEnabled grava o toggle na linha mais recente de wxs_settings. Sem
// nenhuma linha gravada — instalação que nunca passou pela tela de
// configurações — cria uma com host/porta/etc. vazios só para guardar o
// toggle; SaveWxsSettingsFromDB substitui essa linha quando o operador enfim
// configurar o W-Access. O toggle não pode depender de uma conexão já
// existir: era esse acoplamento que deixava o operador sem como religar o
// sync depois que ele foi desligado por engano.
func SetSyncEnabled(ctx context.Context, db DBInterface, ligado bool) error {
	var id int
	err := db.QueryRow(ctx,
		"SELECT id FROM service.wxs_settings ORDER BY id DESC LIMIT 1").Scan(&id)

	if errors.Is(err, pgx.ErrNoRows) {
		_, err := db.Exec(ctx, `
			INSERT INTO service.wxs_settings (host, port, database, username, password, sync_enabled)
			VALUES ('', 0, '', '', '', $1)`, ligado)
		if err != nil {
			return fmt.Errorf("erro ao criar configuração de W-Access: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("erro ao localizar configuração de W-Access: %w", err)
	}

	_, err = db.Exec(ctx,
		"UPDATE service.wxs_settings SET sync_enabled = $1, updated_at = NOW() WHERE id = $2",
		ligado, id)
	if err != nil {
		return fmt.Errorf("erro ao gravar sync_enabled: %w", err)
	}
	return nil
}
