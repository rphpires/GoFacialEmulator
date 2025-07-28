package dahua

import (
	"GoFacialEmulator/internal/database"
	"context"
)

// Repository gerencia operações de banco específicas do Dahua
type Repository struct {
	db       database.DBInterface
	deviceID int
}

// NewRepository cria um novo repositório Dahua
func NewRepository(db database.DBInterface, deviceID int) *Repository {
	return &Repository{
		db:       db,
		deviceID: deviceID,
	}
}

// GetTotalUsers retorna total de usuários para este dispositivo
func (r *Repository) GetTotalUsers() (int, error) {
	var count int
	err := r.db.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM emulator.dahua_card_devices WHERE device_id = $1`,
		r.deviceID).Scan(&count)
	return count, err
}

// FindCard busca um cartão por UserID
func (r *Repository) FindCard(userID int) (*Card, error) {
	var card Card
	err := r.db.QueryRow(context.Background(), `
        SELECT dc.id, dc.card_name, dc.user_id, dc.card_no, 
               dc.valid_date_start, dc.valid_date_end, dcd.rec_no
        FROM emulator.dahua_cards dc
        JOIN emulator.dahua_card_devices dcd ON dc.id = dcd.card_id
        WHERE dcd.device_id = $1 AND dc.user_id = $2`,
		r.deviceID, userID).Scan(
		&card.ID, &card.CardName, &card.UserID, &card.CardNo,
		&card.ValidDateStart, &card.ValidDateEnd, &card.RecNo)

	if err != nil {
		return nil, err
	}
	return &card, nil
}

// GetCards retorna lista de cartões com paginação
func (r *Repository) GetCards(limit, offset int) ([]Card, error) {
	rows, err := r.db.Query(context.Background(), `
        SELECT dc.id, dc.card_name, dc.user_id, dc.card_no, 
               dc.valid_date_start, dc.valid_date_end, dcd.rec_no
        FROM emulator.dahua_cards dc
        JOIN emulator.dahua_card_devices dcd ON dc.id = dcd.card_id
        WHERE dcd.device_id = $1
        LIMIT $2 OFFSET $3`,
		r.deviceID, limit, offset)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cards []Card
	for rows.Next() {
		var card Card
		err := rows.Scan(&card.ID, &card.CardName, &card.UserID,
			&card.CardNo, &card.ValidDateStart,
			&card.ValidDateEnd, &card.RecNo)
		if err != nil {
			return nil, err
		}
		cards = append(cards, card)
	}

	return cards, nil
}

// AddCard adiciona um novo cartão
func (r *Repository) AddCard(card *Card) error {
	tx, err := r.db.Begin(context.Background())
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())

	// Inserir cartão
	var cardID int
	err = tx.QueryRow(context.Background(),
		`INSERT INTO emulator.dahua_cards (card_name, user_id, card_no, valid_date_start, valid_date_end)
         VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		card.CardName, card.UserID, card.CardNo, card.ValidDateStart, card.ValidDateEnd).Scan(&cardID)

	if err != nil {
		return err
	}

	// Obter próximo RecNo
	var maxRecNo int
	err = tx.QueryRow(context.Background(),
		`SELECT COALESCE(MAX(rec_no), 0) FROM emulator.dahua_card_devices WHERE device_id = $1`,
		r.deviceID).Scan(&maxRecNo)

	if err != nil {
		return err
	}

	// Associar ao dispositivo
	_, err = tx.Exec(context.Background(),
		`INSERT INTO emulator.dahua_card_devices (card_id, device_id, rec_no)
         VALUES ($1, $2, $3)`,
		cardID, r.deviceID, maxRecNo+1)

	if err != nil {
		return err
	}

	return tx.Commit(context.Background())
}

// RemoveCard remove um cartão por RecNo
func (r *Repository) RemoveCard(recNo int) error {
	_, err := r.db.Exec(context.Background(),
		`DELETE FROM emulator.dahua_card_devices WHERE device_id = $1 AND rec_no = $2`,
		r.deviceID, recNo)
	return err
}

// Face operations
func (r *Repository) FindFaces() (int, error) {
	var count int
	err := r.db.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM emulator.dahua_faces").Scan(&count)
	return count, err
}

func (r *Repository) GetFaces(limit, offset int) ([]Face, error) {
	rows, err := r.db.Query(context.Background(),
		"SELECT user_id, md5 FROM emulator.dahua_faces LIMIT $1 OFFSET $2",
		limit, offset)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var faces []Face
	for rows.Next() {
		var face Face
		err := rows.Scan(&face.UserID, &face.MD5)
		if err != nil {
			return nil, err
		}
		faces = append(faces, face)
	}

	return faces, nil
}

func (r *Repository) AddFace(face *Face) error {
	_, err := r.db.Exec(context.Background(),
		`INSERT INTO emulator.dahua_faces (user_id, md5) VALUES ($1, $2)
         ON CONFLICT (user_id) DO UPDATE SET md5 = $2`,
		face.UserID, face.MD5)
	return err
}

func (r *Repository) RemoveFace(userID int) error {
	_, err := r.db.Exec(context.Background(),
		"DELETE FROM emulator.dahua_faces WHERE user_id = $1", userID)
	return err
}
