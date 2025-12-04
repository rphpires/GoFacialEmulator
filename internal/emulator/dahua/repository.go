// internal/emulator/dahua/repository.go (versão otimizada)
package dahua

import (
	"GoFacialEmulator/internal/cache"
	"GoFacialEmulator/internal/database"
	"context"
	"fmt"
	"time"
)

// Queries pré-definidas (prepared statements automáticos)
const (
	queryCardCount = "SELECT COUNT(*) FROM emulator.dahua_cards WHERE device_id = $1"
	queryFaceCount = "SELECT COUNT(*) FROM emulator.dahua_faces WHERE device_id = $1"

	// Query otimizada que busca tudo de uma vez
	queryAllCounts = `
        SELECT 
            (SELECT COUNT(*) FROM emulator.dahua_cards WHERE device_id = $1) as cards,
            (SELECT COUNT(*) FROM emulator.dahua_faces WHERE device_id = $1) as faces
    `

	queryRandomCard = `
        SELECT card_name, card_no, user_id 
        FROM emulator.dahua_cards 
        WHERE device_id = $1
        ORDER BY RANDOM() LIMIT 1
    `

	queryNextRecNo = `
        SELECT COALESCE(MIN(t1.rec_no + 1), 1) 
        FROM emulator.dahua_cards AS t1
        LEFT JOIN emulator.dahua_cards AS t2 ON t1.rec_no + 1 = t2.rec_no AND t1.device_id = t2.device_id
        WHERE t1.device_id = $1 AND t2.rec_no IS NULL
    `
)

// Repository gerencia operações de banco específicas do Dahua
type Repository struct {
	db       database.DBInterface // Pode ser AdaptivePool ou DualPoolManager
	deviceID int
	cache    *cache.SimpleCache
	timeout  time.Duration
}

// NewRepository cria um novo repositório Dahua
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
	ctx, cancel := r.getWriteContext()
	ctx = database.WithEmulatorID(ctx, r.deviceID)
	return ctx, cancel
}

// getReadContext cria contexto para operações de leitura (sem EmulatorID)
func (r *Repository) getReadContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), r.timeout)
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

// GetTotalUsers com cache (Dahua usa cartões como usuários)
func (r *Repository) GetTotalUsers() (int, error) {
	// Tentar cache primeiro
	if count, found := r.cache.GetUserCount(r.deviceID); found {
		return count, nil
	}

	// Cache miss - buscar no banco
	ctx, cancel := r.getWriteContext()
	defer cancel()

	var count int
	err := r.db.QueryRow(ctx, queryCardCount, r.deviceID).Scan(&count)
	if err != nil {
		return 0, err
	}

	// Cachear por 60 segundos
	r.cache.SetUserCount(r.deviceID, count, 60*time.Second)
	return count, nil
}

// CountItems otimizado - uma única query
func (r *Repository) CountItems() (*CountItems, error) {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	counts := &CountItems{}
	err := r.db.QueryRow(ctx, queryAllCounts, r.deviceID).Scan(
		&counts.Cards, &counts.Faces)

	if err != nil {
		return nil, fmt.Errorf("failed to count items: %w", err)
	}

	// Cachear contagem de cartões (usuários)
	r.cache.SetUserCount(r.deviceID, counts.Cards, 60*time.Second)

	return counts, nil
}

// ====================== CARD OPERATIONS ======================

// CheckIfCardExists verifica se um cartão já existe
func (r *Repository) CheckIfCardExists(cardNo string, userID int) (bool, error) {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	var count int
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM emulator.dahua_cards 
         WHERE device_id = $1 AND (card_no = $2 OR user_id = $3)`,
		r.deviceID, cardNo, userID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetNextRecNo obtém o próximo RecNo disponível
func (r *Repository) GetNextRecNo() (int, error) {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	var recNo int
	err := r.db.QueryRow(ctx, queryNextRecNo, r.deviceID).Scan(&recNo)

	if err != nil {
		// Se não há registros, começar do 1
		recNo = 1
	}

	return recNo, nil
}

// AddCard adiciona um novo cartão
func (r *Repository) AddCard(cardName string, userID int, cardNo string, validDateStart, validDateEnd time.Time) (int, error) {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	// Verificar se já existe
	exists, err := r.CheckIfCardExists(cardNo, userID)
	if err != nil {
		return 0, err
	}
	if exists {
		return 0, fmt.Errorf("card with UserID %d or CardNo %s already exists", userID, cardNo)
	}

	// Obter próximo RecNo
	recNo, err := r.GetNextRecNo()
	if err != nil {
		return 0, err
	}

	_, err = r.db.Exec(ctx,
		`INSERT INTO emulator.dahua_cards (device_id, rec_no, card_name, user_id, card_no, valid_date_start, valid_date_end) 
         VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		r.deviceID, recNo, cardName, userID, cardNo, validDateStart, validDateEnd)

	// Invalidar cache APENAS se sucesso
	if err == nil {
		r.cache.InvalidateDevice(r.deviceID)
	}

	return recNo, err
}

// RemoveCard remove um cartão pelo RecNo
func (r *Repository) RemoveCard(recNo int) error {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	_, err := r.db.Exec(ctx,
		"DELETE FROM emulator.dahua_cards WHERE device_id = $1 AND rec_no = $2",
		r.deviceID, recNo)

	// Invalidar cache APENAS se sucesso
	if err == nil {
		r.cache.InvalidateDevice(r.deviceID)
	}

	return err
}

// FindCard encontra cartões por UserID - retorna formato Dahua
func (r *Repository) FindCard(userID int) (string, error) {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	rows, err := r.db.Query(ctx,
		`SELECT rec_no, card_name, user_id, card_no, valid_date_start, valid_date_end 
         FROM emulator.dahua_cards 
         WHERE device_id = $1 AND user_id = $2`,
		r.deviceID, userID)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var cards []*Card
	for rows.Next() {
		card := &Card{}
		err := rows.Scan(&card.RecNo, &card.CardName, &card.UserID, &card.CardNo,
			&card.ValidDateStart, &card.ValidDateEnd)
		if err != nil {
			return "", err
		}
		cards = append(cards, card)
	}

	if len(cards) == 0 {
		return "found=0", nil
	}

	// Formatar resposta no estilo Dahua
	response := fmt.Sprintf("found=1\n%s", r.formatCardToResponse(cards))
	return response, nil
}

// GetCards retorna cartões com paginação - formato Dahua
func (r *Repository) GetCards(count, offset int) (string, error) {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	rows, err := r.db.Query(ctx,
		`SELECT rec_no, card_name, user_id, card_no, valid_date_start, valid_date_end 
         FROM emulator.dahua_cards 
         WHERE device_id = $1
         ORDER BY rec_no
         LIMIT $2 OFFSET $3`,
		r.deviceID, count, offset)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var cards []*Card
	for rows.Next() {
		card := &Card{}
		err := rows.Scan(&card.RecNo, &card.CardName, &card.UserID, &card.CardNo,
			&card.ValidDateStart, &card.ValidDateEnd)
		if err != nil {
			return "", err
		}
		cards = append(cards, card)
	}

	if len(cards) == 0 {
		return "found=0", nil
	}

	// Formatar resposta no estilo Dahua
	response := fmt.Sprintf("found=%d\n%s", len(cards), r.formatCardToResponse(cards))
	return response, nil
}

// formatCardToResponse formata cartões para resposta no formato Dahua (igual ao Python)
func (r *Repository) formatCardToResponse(cards []*Card) string {
	response := ""
	for i, card := range cards {
		rec := fmt.Sprintf(`records[%d].CardName=%s
records[%d].CardNo=%s
records[%d].CardStatus=0
records[%d].CardType=0
records[%d].CitizenIDNo=
records[%d].Doors[0]=0
records[%d].DynamicCheckCode=
records[%d].FirstEnter=false
records[%d].Handicap=false
records[%d].IsValid=false
records[%d].Password=
records[%d].RecNo=%d
records[%d].RepeatEnterRouteTimeout=4294967295
records[%d].TimeSections[0]=1
records[%d].UseTime=200
records[%d].UserID=%d
records[%d].UserType=0
records[%d].VTOPosition=
records[%d].ValidDateEnd=%s
records[%d].ValidDateStart=%s
`,
			i, card.CardName,
			i, card.CardNo,
			i,
			i,
			i,
			i,
			i,
			i,
			i,
			i,
			i,
			i, card.RecNo,
			i,
			i,
			i,
			i, card.UserID,
			i,
			i,
			i, card.ValidDateEnd.Format("2006-01-02 15:04:05"),
			i, card.ValidDateStart.Format("2006-01-02 15:04:05"))
		response += rec
	}
	return response
}

// ====================== FACE OPERATIONS ======================

// CheckIfFaceExists verifica se uma face já existe para o usuário
func (r *Repository) CheckIfFaceExists(userID int) (bool, error) {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	var count int
	err := r.db.QueryRow(ctx,
		"SELECT COUNT(*) FROM emulator.dahua_faces WHERE device_id = $1 AND user_id = $2",
		r.deviceID, userID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// AddFace adiciona uma nova face
func (r *Repository) AddFace(userID int, md5 string) error {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	_, err := r.db.Exec(ctx,
		`INSERT INTO emulator.dahua_faces (device_id, user_id, md5_hash) 
         VALUES ($1, $2, $3)
         ON CONFLICT (device_id, user_id) DO UPDATE SET md5_hash = $3, updated_at = NOW()`,
		r.deviceID, userID, md5)

	// Invalidar cache APENAS se sucesso
	if err == nil {
		r.cache.InvalidateDevice(r.deviceID)
	}

	return err
}

// RemoveFace remove uma face
func (r *Repository) RemoveFace(userID int) error {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	_, err := r.db.Exec(ctx,
		"DELETE FROM emulator.dahua_faces WHERE device_id = $1 AND user_id = $2",
		r.deviceID, userID)

	// Invalidar cache APENAS se sucesso
	if err == nil {
		r.cache.InvalidateDevice(r.deviceID)
	}

	return err
}

// FindRemoteFaces retorna informações para busca de faces
func (r *Repository) FindRemoteFaces() (*FindFaceResponse, error) {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	var count int
	err := r.db.QueryRow(ctx, queryFaceCount, r.deviceID).Scan(&count)
	if err != nil {
		return nil, err
	}

	// Token aleatório entre 1-30 como no Python
	return &FindFaceResponse{
		Token: 1 + (int(time.Now().Unix()) % 30),
		Total: count,
	}, nil
}

// GetRemoteFaces retorna faces com paginação
func (r *Repository) GetRemoteFaces(count, offset int) (*GetFaceResponse, error) {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	rows, err := r.db.Query(ctx,
		`SELECT user_id, md5_hash 
         FROM emulator.dahua_faces 
         WHERE device_id = $1
         ORDER BY user_id
         LIMIT $2 OFFSET $3`,
		r.deviceID, count, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var faces []FaceInfo
	for rows.Next() {
		var userID int
		var md5 string
		err := rows.Scan(&userID, &md5)
		if err != nil {
			return nil, err
		}
		faces = append(faces, FaceInfo{
			MD5:    md5,
			UserID: userID,
		})
	}

	return &GetFaceResponse{Info: faces}, nil
}

// ====================== UTILITY OPERATIONS ======================

// GetRandomCard retorna um cartão aleatório para geração de eventos - com cache
func (r *Repository) GetRandomCard() (cardName, cardNo string, userID int, err error) {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	err = r.db.QueryRow(ctx, queryRandomCard, r.deviceID).Scan(&cardName, &cardNo, &userID)
	return
}
