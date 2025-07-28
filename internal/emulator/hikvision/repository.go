package hikvision

import (
	"context"
	"fmt"
	"time"

	"GoFacialEmulator/internal/database"
)

// Repository gerencia operações de banco específicas do Hikvision
type Repository struct {
	db       database.DBInterface
	deviceID int
	timeout  time.Duration
}

// NewRepository cria um novo repositório Hikvision
func NewRepository(db database.DBInterface, deviceID int) *Repository {
	return &Repository{
		db:       db,
		deviceID: deviceID,
		timeout:  5 * time.Second,
	}
}

// GetTotalUsers retorna total de usuários para este dispositivo
func (r *Repository) GetTotalUsers() (int, error) {
	var count int
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM emulator.hikvision_card_devices WHERE device_id = $1`,
		r.deviceID).Scan(&count)
	return count, err
}

// GetSetting obtém uma configuração do dispositivo
func (r *Repository) GetSetting(key string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	var value string
	err := r.db.QueryRow(ctx,
		"SELECT value FROM emulator.device_settings WHERE device_id = $1 AND cfg_id = $2",
		r.deviceID, key).Scan(&value)
	return value, err
}

// SetSetting define uma configuração do dispositivo
func (r *Repository) SetSetting(key, value string) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	_, err := r.db.Exec(ctx,
		`INSERT INTO emulator.device_settings (device_id, cfg_id, value) 
		 VALUES ($1, $2, $3) 
		 ON CONFLICT (device_id, cfg_id) DO UPDATE SET value = $3`,
		r.deviceID, key, value)
	return err
}

// GetSetting obtém uma configuração do dispositivo
func (r *Repository) GetIPServer(key string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	var value string
	err := r.db.QueryRow(ctx,
		"SELECT value FROM emulator.device_settings WHERE device_id = $1 AND cfg_id = $2",
		r.deviceID, key).Scan(&value)
	return value, err
}

// CountItems retorna contadores de todos os tipos de itens
func (r *Repository) CountItems() (*CountItems, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	counts := &CountItems{}

	// Contar usuários
	err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM emulator.hikvision_users").Scan(&counts.Users)
	if err != nil {
		return nil, fmt.Errorf("failed to count users: %w", err)
	}

	// Contar cartões
	err = r.db.QueryRow(ctx, "SELECT COUNT(*) FROM emulator.hikvision_cards").Scan(&counts.Cards)
	if err != nil {
		return nil, fmt.Errorf("failed to count cards: %w", err)
	}

	// Contar faces
	err = r.db.QueryRow(ctx, "SELECT COUNT(*) FROM emulator.hikvision_faces").Scan(&counts.Faces)
	if err != nil {
		return nil, fmt.Errorf("failed to count faces: %w", err)
	}

	// Contar impressões digitais
	err = r.db.QueryRow(ctx, "SELECT COUNT(*) FROM emulator.hikvision_fingers").Scan(&counts.Fingerprints)
	if err != nil {
		return nil, fmt.Errorf("failed to count fingerprints: %w", err)
	}

	return counts, nil
}

// ====================== USER OPERATIONS ======================

// CheckIfUserExists verifica se um usuário já existe
func (r *Repository) CheckIfUserExists(employeeNo string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	var count int
	err := r.db.QueryRow(ctx,
		"SELECT COUNT(*) FROM emulator.hikvision_users WHERE employee_no = $1",
		employeeNo).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// AddUser adiciona um novo usuário
func (r *Repository) AddUser(user *User) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
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
		`INSERT INTO emulator.hikvision_users (employee_no, name, password, local_ui_right, begin_time, end_time)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		user.EmployeeNo, user.Name, user.Password, user.LocalUIRight, user.BeginTime, user.EndTime)

	return err
}

// UpdateUser atualiza um usuário existente
func (r *Repository) UpdateUser(user *User) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	_, err := r.db.Exec(ctx,
		`UPDATE emulator.hikvision_users 
		 SET name = $2, password = $3, local_ui_right = $4, begin_time = $5, end_time = $6
		 WHERE employee_no = $1`,
		user.EmployeeNo, user.Name, user.Password, user.LocalUIRight, user.BeginTime, user.EndTime)

	return err
}

// DeleteUser remove um usuário e todos os dados relacionados
func (r *Repository) DeleteUser(employeeNo string) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	// Usar transação para garantir consistência
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Deletar usuário
	_, err = tx.Exec(ctx, "DELETE FROM emulator.hikvision_users WHERE employee_no = $1", employeeNo)
	if err != nil {
		return err
	}

	// Deletar cartão associado
	_, err = tx.Exec(ctx, "DELETE FROM emulator.hikvision_cards WHERE employee_no = $1", employeeNo)
	if err != nil {
		return err
	}

	// Deletar face associada
	_, err = tx.Exec(ctx, "DELETE FROM emulator.hikvision_faces WHERE user_id = $1", employeeNo)
	if err != nil {
		return err
	}

	// Deletar impressões digitais associadas
	_, err = tx.Exec(ctx, "DELETE FROM emulator.hikvision_fingers WHERE chid = $1", employeeNo)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// GetUsers retorna usuários com paginação
func (r *Repository) GetUsers(limit, offset int) ([]*User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	rows, err := r.db.Query(ctx,
		`SELECT employee_no, name, password, local_ui_right, begin_time, end_time 
		 FROM emulator.hikvision_users 
		 LIMIT $1 OFFSET $2`,
		limit, offset)
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
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	var count int
	err := r.db.QueryRow(ctx,
		"SELECT COUNT(*) FROM emulator.hikvision_cards WHERE employee_no = $1 OR card_no = $2",
		employeeNo, cardNo).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// AddCard adiciona um novo cartão
func (r *Repository) AddCard(card *Card) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
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
		`INSERT INTO emulator.hikvision_cards (employee_no, card_no) VALUES ($1, $2)`,
		card.EmployeeNo, card.CardNo)

	return err
}

// GetCards retorna cartões com paginação
func (r *Repository) GetCards(limit, offset int) ([]*Card, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	rows, err := r.db.Query(ctx,
		`SELECT employee_no, card_no FROM emulator.hikvision_cards LIMIT $1 OFFSET $2`,
		limit, offset)
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
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	var count int
	err := r.db.QueryRow(ctx,
		"SELECT COUNT(*) FROM emulator.hikvision_faces WHERE user_id = $1",
		userID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// AddFace adiciona uma nova face
func (r *Repository) AddFace(userID, photoData string) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
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
		`INSERT INTO emulator.hikvision_faces (user_id, photo_data) VALUES ($1, $2)`,
		userID, photoData)

	return err
}

// UpdateFace atualiza uma face existente
func (r *Repository) UpdateFace(userID, photoData string) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	_, err := r.db.Exec(ctx,
		`UPDATE emulator.hikvision_faces SET photo_data = $2 WHERE user_id = $1`,
		userID, photoData)

	return err
}

// DeleteFace remove uma face
func (r *Repository) DeleteFace(userID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	_, err := r.db.Exec(ctx,
		"DELETE FROM emulator.hikvision_faces WHERE user_id = $1", userID)

	return err
}

// GetFace retorna os dados de uma face específica
func (r *Repository) GetFace(userID string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	var photoData string
	err := r.db.QueryRow(ctx,
		"SELECT photo_data FROM emulator.hikvision_faces WHERE user_id = $1",
		userID).Scan(&photoData)

	return photoData, err
}

// GetFaces retorna faces com paginação
func (r *Repository) GetFaces(limit, offset int) ([]*Face, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	rows, err := r.db.Query(ctx,
		`SELECT user_id, photo_data FROM emulator.hikvision_faces LIMIT $1 OFFSET $2`,
		limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var faces []*Face
	for rows.Next() {
		face := &Face{}
		err := rows.Scan(&face.UserID, &face.PhotoData)
		if err != nil {
			return nil, err
		}
		faces = append(faces, face)
	}

	return faces, rows.Err()
}

// ====================== UTILITY OPERATIONS ======================

// GetRandomUserAndCard retorna um usuário e cartão aleatórios para geração de eventos
func (r *Repository) GetRandomUserAndCard() (name, cardNo, employeeNo string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	err = r.db.QueryRow(ctx,
		`SELECT u.name, c.card_no, u.employee_no 
		 FROM emulator.hikvision_users u
		 JOIN emulator.hikvision_cards c ON c.employee_no = u.employee_no
		 ORDER BY RANDOM() 
		 LIMIT 1`).Scan(&name, &cardNo, &employeeNo)

	return
}
