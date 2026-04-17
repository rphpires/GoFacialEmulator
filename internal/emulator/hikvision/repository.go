package hikvision

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"GoFacialEmulator/internal/cache"
	"GoFacialEmulator/internal/database"
)

const (
	queryUserCount   = "SELECT COUNT(*) FROM emulator.hikvision_users WHERE device_id = $1"
	queryCardCount   = "SELECT COUNT(*) FROM emulator.hikvision_cards WHERE device_id = $1"
	queryFaceCount   = "SELECT COUNT(*) FROM emulator.hikvision_faces WHERE device_id = $1"
	queryFingerCount = "SELECT COUNT(*) FROM emulator.hikvision_fingers WHERE device_id = $1"

	// Query otimizada que busca tudo de uma vez
	queryAllCounts = `
        SELECT 
            (SELECT COUNT(*) FROM emulator.hikvision_users WHERE device_id = $1) as users,
            (SELECT COUNT(*) FROM emulator.hikvision_cards WHERE device_id = $1) as cards,
            (SELECT COUNT(*) FROM emulator.hikvision_faces WHERE device_id = $1) as faces,
            (SELECT COUNT(*) FROM emulator.hikvision_fingers WHERE device_id = $1) as fingers
    `

	queryRandomUserCard = `
        SELECT u.name, c.card_no, u.employee_no 
        FROM emulator.hikvision_users u
        JOIN emulator.hikvision_cards c ON c.employee_no = u.employee_no AND c.device_id = u.device_id
        WHERE u.device_id = $1
        ORDER BY RANDOM() LIMIT 1
    `
)

type Repository struct {
	db       database.DBInterface // Pode ser AdaptivePool ou DualPoolManager
	deviceID int
	cache    *cache.SimpleCache
	timeout  time.Duration
}

func NewRepository(db database.DBInterface, deviceID int) *Repository {
	return &Repository{
		db:       db,
		deviceID: deviceID,
		cache:    cache.NewSimpleCache(),
		timeout:  10 * time.Second, // Timeout maior para alta concorrência
	}
}

// getWriteContext cria contexto para operações de escrita com EmulatorID
func (r *Repository) getWriteContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	ctx = database.WithEmulatorID(ctx, r.deviceID)
	return ctx, cancel
}

// ====================== SETTINGS ======================

// GetSetting obtém uma configuração do dispositivo
func (r *Repository) GetSetting(key string) (string, error) {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	var value string
	err := r.db.QueryRow(ctx,
		"SELECT value FROM emulator.device_settings WHERE device_id = $1 AND cfg_id = $2",
		r.deviceID, key).Scan(&value)
	if err != nil {
		// Retornar valores padrão para algumas configurações
		switch key {
		case "RemoteServer":
			return "localhost", nil
		case "RemotePort":
			return "15501", nil
		case "RemoteURL":
			return "/notification", nil
		case "LocalAuthentication":
			return "1", nil
		default:
			return "", err
		}
	}
	return value, err
}

// SetSetting define uma configuração do dispositivo
func (r *Repository) SetSetting(key, value string) error {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	_, err := r.db.Exec(ctx,
		`INSERT INTO emulator.device_settings (device_id, cfg_id, value) 
		 VALUES ($1, $2, $3) 
		 ON CONFLICT (device_id, cfg_id) DO UPDATE SET value = $3, updated_at = NOW()`,
		r.deviceID, key, value)
	return err
}

// ====================== COUNT OPERATIONS ======================

func (r *Repository) GetTotalUsers() (int, error) {
	// Tentar cache primeiro
	if count, found := r.cache.GetUserCount(r.deviceID); found {
		return count, nil
	}

	// Cache miss - buscar no banco
	ctx, cancel := r.getWriteContext()
	defer cancel()

	var count int
	err := r.db.QueryRow(ctx, queryUserCount, r.deviceID).Scan(&count)
	if err != nil {
		return 0, err
	}

	// Cachear por 60 segundos
	r.cache.SetUserCount(r.deviceID, count, 60*time.Second)
	return count, nil
}

func (r *Repository) CountItems() (*CountItems, error) {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	counts := &CountItems{}
	err := r.db.QueryRow(ctx, queryAllCounts, r.deviceID).Scan(
		&counts.Users, &counts.Cards, &counts.Faces, &counts.Fingerprints)

	if err != nil {
		return nil, err
	}

	// Cachear contagem de usuários
	r.cache.SetUserCount(r.deviceID, counts.Users, 60*time.Second)

	return counts, nil
}

// ====================== USER OPERATIONS ======================

// CheckIfUserExists verifica se um usuário já existe
func (r *Repository) CheckIfUserExists(employeeNo string) (bool, error) {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	var count int
	err := r.db.QueryRow(ctx,
		"SELECT COUNT(*) FROM emulator.hikvision_users WHERE device_id = $1 AND employee_no = $2",
		r.deviceID, employeeNo).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// AddUser adiciona um novo usuário
func (r *Repository) AddUser(user *User) error {
	// Usar contexto de escrita para garantir ordem de operações
	ctx, cancel := r.getWriteContext()
	defer cancel()

	// Verificar se já existe
	exists, err := r.CheckIfUserExists(user.EmployeeNo)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("user with employeeNo %s already exists", user.EmployeeNo)
	}

	_, err = r.db.Exec(ctx,
		`INSERT INTO emulator.hikvision_users (device_id, employee_no, name, password, local_ui_right, begin_time, end_time)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		r.deviceID, user.EmployeeNo, user.Name, user.Password, user.LocalUIRight, user.BeginTime, user.EndTime)

	if err == nil {
		r.cache.InvalidateDevice(r.deviceID)
	}

	return err
}

// UpdateUser atualiza um usuário existente
func (r *Repository) UpdateUser(user *User) error {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	_, err := r.db.Exec(ctx,
		`UPDATE emulator.hikvision_users 
		 SET name = $3, password = $4, local_ui_right = $5, begin_time = $6, end_time = $7, updated_at = NOW()
		 WHERE device_id = $1 AND employee_no = $2`,
		r.deviceID, user.EmployeeNo, user.Name, user.Password, user.LocalUIRight, user.BeginTime, user.EndTime)

	if err == nil {
		r.cache.InvalidateDevice(r.deviceID)
	}

	return err
}

func (r *Repository) DeleteUser(employeeNo string) error {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	// Usar transação para garantir consistência
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Deletar usuário
	_, err = tx.Exec(ctx,
		"DELETE FROM emulator.hikvision_users WHERE device_id = $1 AND employee_no = $2",
		r.deviceID, employeeNo)
	if err != nil {
		return err
	}

	// Deletar cartão associado
	_, err = tx.Exec(ctx,
		"DELETE FROM emulator.hikvision_cards WHERE device_id = $1 AND employee_no = $2",
		r.deviceID, employeeNo)
	if err != nil {
		return err
	}

	// Deletar face associada
	_, err = tx.Exec(ctx,
		"DELETE FROM emulator.hikvision_faces WHERE device_id = $1 AND user_id = $2",
		r.deviceID, employeeNo)
	if err != nil {
		return err
	}

	// Deletar impressões digitais
	_, err = tx.Exec(ctx,
		"DELETE FROM emulator.hikvision_fingers WHERE device_id = $1 AND chid = $2",
		r.deviceID, employeeNo)
	if err != nil {
		return err
	}

	// Commit da transação
	if err = tx.Commit(ctx); err != nil {
		return err
	}

	// Invalidar cache APENAS após commit bem sucedido
	r.cache.InvalidateDevice(r.deviceID)

	return nil
}

// GetUsers retorna usuários com paginação
func (r *Repository) GetUsers(limit, offset int) ([]*User, error) {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	rows, err := r.db.Query(ctx,
		`SELECT employee_no, name, password, local_ui_right, begin_time, end_time 
		 FROM emulator.hikvision_users 
		 WHERE device_id = $1
		 ORDER BY employee_no
		 LIMIT $2 OFFSET $3`,
		r.deviceID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		user := &User{}
		err := rows.Scan(&user.EmployeeNo, &user.Name, &user.Password,
			&user.LocalUIRight, &user.BeginTime, &user.EndTime)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}

	return users, rows.Err()
}

// ====================== CARD OPERATIONS ======================

// CheckIfCardExists verifica se um cartão já existe
func (r *Repository) CheckIfCardExists(employeeNo, cardNo string) (bool, error) {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	var count int
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM emulator.hikvision_cards 
		 WHERE device_id = $1 AND (employee_no = $2 OR card_no = $3)`,
		r.deviceID, employeeNo, cardNo).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// AddCard adiciona um novo cartão
func (r *Repository) AddCard(card *Card) error {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	// Verificar se já existe
	exists, err := r.CheckIfCardExists(card.EmployeeNo, card.CardNo)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("card with employeeNo %s or cardNo %s already exists", card.EmployeeNo, card.CardNo)
	}

	_, err = r.db.Exec(ctx,
		`INSERT INTO emulator.hikvision_cards (device_id, employee_no, card_no) 
		 VALUES ($1, $2, $3)`,
		r.deviceID, card.EmployeeNo, card.CardNo)

	if err == nil {
		r.cache.InvalidateDevice(r.deviceID)
	}

	return err
}

// GetCards retorna cartões com paginação
func (r *Repository) GetCards(limit, offset int) ([]*Card, error) {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	rows, err := r.db.Query(ctx,
		`SELECT employee_no, card_no 
		 FROM emulator.hikvision_cards 
		 WHERE device_id = $1
		 ORDER BY employee_no
		 LIMIT $2 OFFSET $3`,
		r.deviceID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cards []*Card
	for rows.Next() {
		card := &Card{}
		err := rows.Scan(&card.EmployeeNo, &card.CardNo)
		if err != nil {
			return nil, err
		}
		cards = append(cards, card)
	}

	return cards, rows.Err()
}

// ====================== FACE OPERATIONS ======================

// CheckIfFaceExists verifica se uma face já existe para o usuário
func (r *Repository) CheckIfFaceExists(userID string) (bool, error) {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	var count int
	err := r.db.QueryRow(ctx,
		"SELECT COUNT(*) FROM emulator.hikvision_faces WHERE device_id = $1 AND user_id = $2",
		r.deviceID, userID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// AddFace adiciona uma nova face
func (r *Repository) AddFace(userID, photoData string) error {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	// Verificar se já existe
	exists, err := r.CheckIfFaceExists(userID)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("face for userID %s already exists", userID)
	}

	_, err = r.db.Exec(ctx,
		`INSERT INTO emulator.hikvision_faces (device_id, user_id, photo_data) 
		 VALUES ($1, $2, $3)`,
		r.deviceID, userID, photoData)

	return err
}

// UpdateFace atualiza uma face existente
func (r *Repository) UpdateFace(userID, photoData string) error {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	_, err := r.db.Exec(ctx,
		`UPDATE emulator.hikvision_faces 
		 SET photo_data = $3, updated_at = NOW() 
		 WHERE device_id = $1 AND user_id = $2`,
		r.deviceID, userID, photoData)

	return err
}

// DeleteFace remove uma face
func (r *Repository) DeleteFace(userID string) error {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	_, err := r.db.Exec(ctx,
		"DELETE FROM emulator.hikvision_faces WHERE device_id = $1 AND user_id = $2",
		r.deviceID, userID)

	return err
}

// GetFace retorna os dados de uma face específica
func (r *Repository) GetFace(userID string) (string, error) {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	var photoData string
	err := r.db.QueryRow(ctx,
		"SELECT photo_data FROM emulator.hikvision_faces WHERE device_id = $1 AND user_id = $2",
		r.deviceID, userID).Scan(&photoData)

	return photoData, err
}

// GetFaces retorna faces com paginação
func (r *Repository) GetFaces(limit, offset int) ([]*Face, error) {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	rows, err := r.db.Query(ctx,
		`SELECT user_id, photo_data 
		 FROM emulator.hikvision_faces 
		 WHERE device_id = $1
		 ORDER BY user_id
		 LIMIT $2 OFFSET $3`,
		r.deviceID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var faces []*Face
	for rows.Next() {
		face := &Face{}
		var userIDStr string
		err := rows.Scan(&userIDStr, &face.PhotoData)
		if err != nil {
			return nil, err
		}
		// Converter string para int
		if userID, err := strconv.Atoi(userIDStr); err == nil {
			face.UserID = userID
		}
		faces = append(faces, face)
	}

	return faces, rows.Err()
}

// ====================== UTILITY OPERATIONS ======================

func (r *Repository) GetRandomUserAndCard() (name, cardNo, employeeNo string, err error) {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	err = r.db.QueryRow(ctx, queryRandomUserCard, r.deviceID).Scan(&name, &cardNo, &employeeNo)
	return
}
