// internal/emulator/dahua/repository.go (versão otimizada)
package dahua

import (
	"GoFacialEmulator/internal/cache"
	"GoFacialEmulator/internal/database"
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrCardUserIDTaken e ErrCardNoTaken distinguem os dois motivos pelos quais
// o device real (medido na bancada, Intelbras SS 5531 MF EX) rejeita um
// recordUpdater.cgi?action=insert em AccessControlCard. O device só permite
// UMA linha por UserID (ErrCardUserIDTaken, code 286064929) e não permite o
// mesmo CardNo em duas linhas de UserID diferentes (ErrCardNoTaken, code
// 286064930). Antes as duas causas eram indistinguíveis: CheckIfCardExists
// devolvia um bool só, e o handler respondia sempre "Error\nBad Request!".
var (
	ErrCardUserIDTaken = errors.New("dahua: UserID já possui um cartão AccessControlCard")
	ErrCardNoTaken     = errors.New("dahua: CardNo pertence a um UserID diferente")
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

// cardExistsForUserID verifica se o UserID já tem uma linha em
// AccessControlCard. O device real permite só UMA por UserID.
func (r *Repository) cardExistsForUserID(userID int) (bool, error) {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	var count int
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM emulator.dahua_cards
         WHERE device_id = $1 AND user_id = $2`,
		r.deviceID, userID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// cardNoBelongsToOtherUser verifica se o CardNo já está associado a um
// UserID diferente do que está sendo inserido.
func (r *Repository) cardNoBelongsToOtherUser(cardNo string, userID int) (bool, error) {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	var count int
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM emulator.dahua_cards
         WHERE device_id = $1 AND card_no = $2 AND user_id <> $3`,
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

	// Ordem medida na bancada: um UserID que já tem cartão é rejeitado
	// (286064929) mesmo que o CardNo seja novo; só depois disso o CardNo
	// duplicado de outro UserID é rejeitado (286064930). Os dois casos nunca
	// foram medidos combinados, mas essa ordem é a única testável com os dois
	// erros sendo mutuamente exclusivos nos exemplos da bancada.
	temCartao, err := r.cardExistsForUserID(userID)
	if err != nil {
		return 0, err
	}
	if temCartao {
		return 0, ErrCardUserIDTaken
	}

	cartaoDeOutro, err := r.cardNoBelongsToOtherUser(cardNo, userID)
	if err != nil {
		return 0, err
	}
	if cartaoDeOutro {
		return 0, ErrCardNoTaken
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

// FindRemoteFaces retorna informações para busca de faces - Total é a
// contagem GLOBAL do device (sem filtro por UserID). Só é correto quando a
// requisição não veio com Condition.UserID; ver FindRemoteFacesByUserID para
// o caso filtrado.
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

// FindRemoteFacesByUserID resolve Total para UM UserID específico - 0 ou 1,
// nunca a contagem global. Medido na bancada: FaceInfoManager.cgi?action=
// startFind&Condition.UserID=<id> devolve Total=0 quando o UserID não tem
// face e Total=1 quando tem (a especificação do fabricante, seção 12.3.4,
// documenta exatamente isso: "Total ... return 0 if not found"). Antes o
// handler ignorava Condition.UserID e chamava FindRemoteFaces(), que devolve
// a contagem GLOBAL de faces do device - crescente e alheia ao UserID
// consultado, inclusive subindo depois de um remove de outro usuário. O
// gateway usa esse Total para decidir HasFace; com a contagem global,
// HasFace nunca dava false.
func (r *Repository) FindRemoteFacesByUserID(userID int) (*FindFaceResponse, error) {
	existe, err := r.CheckIfFaceExists(userID)
	if err != nil {
		return nil, err
	}

	total := 0
	if existe {
		total = 1
	}

	// Token não carrega significado medido (a especificação não documenta um
	// valor esperado) - mantido no mesmo esquema do FindRemoteFaces só para
	// não introduzir uma segunda convenção de Token no mesmo endpoint.
	return &FindFaceResponse{
		Token: 1 + (int(time.Now().Unix()) % 30),
		Total: total,
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

// GetCardUserIDByRecNo resolve o UserID a partir do RecNo. O gerenciador
// remove cartões por RecNo, mas a face é indexada por UserID — e o device real
// derruba as duas coisas juntas.
func (r *Repository) GetCardUserIDByRecNo(recNo int) (int, error) {
	ctx, cancel := r.getWriteContext()
	defer cancel()

	var userID int
	err := r.db.QueryRow(ctx,
		"SELECT user_id FROM emulator.dahua_cards WHERE device_id = $1 AND rec_no = $2",
		r.deviceID, recNo).Scan(&userID)
	if err != nil {
		return 0, err
	}
	return userID, nil
}
