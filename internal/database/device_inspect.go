package database

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// DeviceUserRow representa um usuario do dispositivo em formato unificado,
// independente do modelo (Hikvision ou Dahua).
type DeviceUserRow struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	CardNo  string `json:"card_no"`
	HasFace bool   `json:"has_face"`
	ValidTo string `json:"valid_to"`
}

// DeviceSettingRow representa uma configuracao do dispositivo.
type DeviceSettingRow struct {
	CfgID string `json:"cfg_id"`
	Value string `json:"value"`
}

const (
	// Hikvision: usuario e a entidade principal; cartoes sao agregados para
	// nao duplicar linhas (UNIQUE e por card_no, nao por employee_no).
	queryHikvisionUsers = `
        SELECT u.employee_no,
               u.name,
               COALESCE(string_agg(c.card_no, ', ' ORDER BY c.card_no), '') AS card_no,
               BOOL_OR(f.user_id IS NOT NULL) AS has_face,
               u.end_time
        FROM emulator.hikvision_users u
        LEFT JOIN emulator.hikvision_cards c
               ON c.device_id = u.device_id AND c.employee_no = u.employee_no
        LEFT JOIN emulator.hikvision_faces f
               ON f.device_id = u.device_id AND f.user_id = u.employee_no
        WHERE u.device_id = $1
          AND ($2 = '' OR u.name ILIKE '%' || $2 || '%' OR u.employee_no ILIKE '%' || $2 || '%')
        GROUP BY u.employee_no, u.name, u.end_time
        ORDER BY u.employee_no
        LIMIT $3 OFFSET $4`

	queryHikvisionUsersCount = `
        SELECT COUNT(*)
        FROM emulator.hikvision_users u
        WHERE u.device_id = $1
          AND ($2 = '' OR u.name ILIKE '%' || $2 || '%' OR u.employee_no ILIKE '%' || $2 || '%')`

	// Dahua nao tem tabela de usuarios: a linha vem do cartao
	// (UNIQUE(device_id, user_id) garante um cartao por usuario).
	queryDahuaUsers = `
        SELECT d.user_id::text,
               COALESCE(d.card_name, ''),
               d.card_no,
               (f.user_id IS NOT NULL) AS has_face,
               d.valid_date_end
        FROM emulator.dahua_cards d
        LEFT JOIN emulator.dahua_faces f
               ON f.device_id = d.device_id AND f.user_id = d.user_id
        WHERE d.device_id = $1
          AND ($2 = '' OR d.card_name ILIKE '%' || $2 || '%' OR d.user_id::text ILIKE '%' || $2 || '%')
        ORDER BY d.user_id
        LIMIT $3 OFFSET $4`

	queryDahuaUsersCount = `
        SELECT COUNT(*)
        FROM emulator.dahua_cards d
        WHERE d.device_id = $1
          AND ($2 = '' OR d.card_name ILIKE '%' || $2 || '%' OR d.user_id::text ILIKE '%' || $2 || '%')`

	queryDeviceSettings = `
        SELECT cfg_id, COALESCE(value, '')
        FROM emulator.device_settings
        WHERE device_id = $1
        ORDER BY cfg_id`
)

const deviceInspectTimeout = 10 * time.Second

// ListDeviceUsers retorna a pagina de usuarios do dispositivo e o total de
// registros que atendem ao filtro. O model define de quais tabelas ler.
func ListDeviceUsers(ctx context.Context, db DBInterface, model string, deviceID int, search string, limit, offset int) ([]DeviceUserRow, int, error) {
	var listQuery, countQuery string

	switch strings.ToLower(model) {
	case "hikvision":
		listQuery, countQuery = queryHikvisionUsers, queryHikvisionUsersCount
	case "dahua":
		listQuery, countQuery = queryDahuaUsers, queryDahuaUsersCount
	default:
		return nil, 0, fmt.Errorf("unsupported device model: %s", model)
	}

	ctx, cancel := context.WithTimeout(ctx, deviceInspectTimeout)
	defer cancel()

	var total int
	if err := db.QueryRow(ctx, countQuery, deviceID, search).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count device users: %w", err)
	}

	rows, err := db.Query(ctx, listQuery, deviceID, search, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list device users: %w", err)
	}
	defer rows.Close()

	users := make([]DeviceUserRow, 0, limit)
	for rows.Next() {
		var (
			user    DeviceUserRow
			hasFace *bool
			validTo *time.Time
		)
		if err := rows.Scan(&user.ID, &user.Name, &user.CardNo, &hasFace, &validTo); err != nil {
			return nil, 0, fmt.Errorf("scan device user: %w", err)
		}
		user.HasFace = hasFace != nil && *hasFace
		if validTo != nil {
			// UTC para o front exibir a mesma data gravada, sem deslocamento de fuso
			user.ValidTo = validTo.UTC().Format(time.RFC3339)
		}
		users = append(users, user)
	}

	return users, total, rows.Err()
}

// ListDeviceSettings retorna as configuracoes gravadas para o dispositivo.
func ListDeviceSettings(ctx context.Context, db DBInterface, deviceID int) ([]DeviceSettingRow, error) {
	ctx, cancel := context.WithTimeout(ctx, deviceInspectTimeout)
	defer cancel()

	rows, err := db.Query(ctx, queryDeviceSettings, deviceID)
	if err != nil {
		return nil, fmt.Errorf("list device settings: %w", err)
	}
	defer rows.Close()

	settings := make([]DeviceSettingRow, 0)
	for rows.Next() {
		var setting DeviceSettingRow
		if err := rows.Scan(&setting.CfgID, &setting.Value); err != nil {
			return nil, fmt.Errorf("scan device setting: %w", err)
		}
		settings = append(settings, setting)
	}

	return settings, rows.Err()
}
