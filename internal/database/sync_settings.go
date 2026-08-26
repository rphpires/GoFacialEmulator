package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v4"
)

// ErrNoWxsSettings: ligar ou desligar o sync exige uma conexão de W-Access
// gravada. Sem ela o toggle não teria sobre o que agir.
var ErrNoWxsSettings = errors.New("nenhuma configuração de W-Access gravada")

// GetSyncEnabled diz se o vínculo com o Invenzi está ligado. Sem linha em
// service.wxs_settings devolve false sem erro: instalação que nunca
// configurou o W-Access não sincroniza, e isso é estado normal, não falha.
func GetSyncEnabled(ctx context.Context, db DBInterface) (bool, error) {
	var ligado bool
	err := db.QueryRow(ctx, `
		SELECT sync_enabled
		FROM service.wxs_settings
		ORDER BY id DESC
		LIMIT 1
	`).Scan(&ligado)

	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("erro ao ler sync_enabled: %w", err)
	}
	return ligado, nil
}

// SetSyncEnabled grava o toggle na linha mais recente de wxs_settings.
func SetSyncEnabled(ctx context.Context, db DBInterface, ligado bool) error {
	var id int
	err := db.QueryRow(ctx,
		"SELECT id FROM service.wxs_settings ORDER BY id DESC LIMIT 1").Scan(&id)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoWxsSettings
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
