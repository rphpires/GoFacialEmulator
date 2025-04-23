package database

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"GoFacialEmulator/internal/config"
	"GoFacialEmulator/internal/trace"

	"github.com/jackc/pgconn"
	_ "github.com/lib/pq" // PostgreSQL driver
)

// Mantemos o WxsDB diferente, pois parece ser uma conexão a um banco específico
// que pode não usar pgx, mas sim o driver padrão do PostgreSQL
type WxsDB struct {
	*PostgresDBPool
	trace *trace.Tracer
	mu    sync.Mutex
}

// NewWxsDB cria uma nova conexão com o banco de dados WXS
func NewWxsDB(cfg config.DatabaseConfig) (*WxsDB, error) {
	pool, err := NewPostgresDBPool(cfg)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar pool para WXS: %w", err)
	}

	return &WxsDB{
		PostgresDBPool: pool,
		trace:          trace.NewTracer(),
	}, nil
}

// ReadData executa uma consulta SQL e retorna os resultados
func (w *WxsDB) ReadData(query string, args ...interface{}) ([][]interface{}, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	ctx := context.Background()
	rows, err := w.PostgresDBPool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("erro ao executar consulta: %w", err)
	}
	defer rows.Close()

	// Obter descrições das colunas
	fieldDescriptions := rows.FieldDescriptions()
	colCount := len(fieldDescriptions)

	var result [][]interface{}
	for rows.Next() {
		// Criar um slice de interface{} para armazenar os valores das colunas
		values := make([]interface{}, colCount)
		// Criar um slice de ponteiros para os valores
		scanArgs := make([]interface{}, colCount)
		for i := range values {
			scanArgs[i] = &values[i]
		}

		// Escanear a linha para os ponteiros
		if err := rows.Scan(scanArgs...); err != nil {
			return nil, fmt.Errorf("erro ao escanear linha: %w", err)
		}

		// Converter []byte para string se necessário
		row := make([]interface{}, colCount)
		for i, v := range values {
			if b, ok := v.([]byte); ok {
				row[i] = string(b)
			} else {
				row[i] = v
			}
		}

		result = append(result, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erro ao iterar linhas: %w", err)
	}

	return result, nil
}

// ReadSingleRow executa uma consulta SQL e retorna uma única linha
func (w *WxsDB) ReadSingleRow(query string, args ...interface{}) ([]interface{}, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	ctx := context.Background()
	row := w.PostgresDBPool.QueryRow(ctx, query, args...)

	// Usar reflection para identificar as colunas, mas simplificando
	// assumindo que sabemos o número de colunas esperado
	// (isto é uma simplificação; uma implementação real precisaria de mais trabalho)
	values := make([]interface{}, 10) // Assumindo no máximo 10 colunas
	scanArgs := make([]interface{}, 10)
	for i := range values {
		scanArgs[i] = &values[i]
	}

	if err := row.Scan(scanArgs...); err != nil {
		return nil, err
	}

	// Converter []byte para string se necessário
	result := make([]interface{}, 10)
	for i, v := range values {
		if b, ok := v.([]byte); ok {
			result[i] = string(b)
		} else {
			result[i] = v
		}
	}

	return result, nil
}

// ExecuteUpdate executa uma instrução SQL de atualização
func (w *WxsDB) ExecuteUpdate(query string, args ...interface{}) (pgconn.CommandTag, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	ctx := context.Background()
	return w.PostgresDBPool.Exec(ctx, query, args...)
}

// ExecuteInsert executa uma instrução SQL de inserção
func (w *WxsDB) ExecuteInsert(query string, args ...interface{}) (pgconn.CommandTag, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	ctx := context.Background()
	return w.PostgresDBPool.Exec(ctx, query, args...)
}

// ExecuteProcedure executa um procedimento armazenado
func (w *WxsDB) ExecuteProcedure(procedureName string, params ...interface{}) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Construir a chamada do procedimento
	query := fmt.Sprintf("CALL %s", procedureName)
	if len(params) > 0 {
		query += "("
		placeholders := make([]string, len(params))
		for i := range placeholders {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
		}
		query += strings.Join(placeholders, ", ") + ")"
	}

	ctx := context.Background()
	_, err := w.PostgresDBPool.Exec(ctx, query, params...)
	if err != nil {
		return fmt.Errorf("erro ao executar procedimento %s: %w", procedureName, err)
	}

	return nil
}

// CountCHIDsByLocalController conta CHIDs agrupados por controlador local
func (w *WxsDB) CountCHIDsByLocalController() (map[int]map[int][2]interface{}, error) {
	query := `
	SELECT 
		lc.SiteControllerID,
		lc.LocalControllerID, 
		lc.BaseCommPort,
		COUNT(DISTINCT ca.CHID) AS CHID_Count
	FROM 
		CfgHWLocalControllers lc
	LEFT JOIN 
		CfgHWReaders rdr ON lc.LocalControllerID = rdr.LocalControllerID
	LEFT JOIN 
		CfgACAccessLevelsContents al_cont ON rdr.ReaderID = al_cont.ReaderID
	LEFT JOIN 
		CHAccessLevels ca ON al_cont.AccessLevelID = ca.AccessLevelID
		AND ca.CHID IN (
			SELECT CHID 
			FROM CHCards
			WHERE IPRdrUserID IS NOT NULL
		)
	GROUP BY 
		lc.LocalControllerID, lc.SiteControllerID, lc.BaseCommPort
	`

	rows, err := w.ReadData(query)
	if err != nil {
		return nil, err
	}

	// Mapa de SiteControllerID -> Mapa de LocalControllerID -> [port, count]
	result := make(map[int]map[int][2]interface{})

	for _, row := range rows {
		// Conversão segura dos valores
		siteControllerID, _ := toInt(row[0])
		localControllerID, _ := toInt(row[1])
		port := row[2]
		count := row[3]

		if _, ok := result[siteControllerID]; !ok {
			result[siteControllerID] = make(map[int][2]interface{})
		}

		result[siteControllerID][localControllerID] = [2]interface{}{port, count}
	}

	return result, nil
}

// GetLocalControllers obtém todos os controladores locais configurados como emuladores
func (w *WxsDB) GetLocalControllers() ([]map[string]interface{}, error) {
	query := `
	SELECT 
		LocalControllerID, 
		LocalControllerName, 
		IPAddress, 
		BaseCommPort, 
		LocalControllerEnabled,
		LocalControllerType,
		LocalControllerDescription 
	FROM CfgHWLocalControllers
	WHERE LocalControllerDescription LIKE 'emulator%'
	`

	rows, err := w.ReadData(query)
	if err != nil {
		return nil, err
	}

	var result []map[string]interface{}
	for _, row := range rows {
		// Conversão segura dos valores
		id, _ := toInt(row[0])
		name, _ := toString(row[1])
		ip, _ := toString(row[2])
		port, _ := toInt(row[3])
		enabled, _ := toBool(row[4])
		lcType, _ := toInt(row[5])
		description, _ := toString(row[6])

		// Analisar o intervalo de eventos da descrição
		var eventInterval int = 0
		parts := strings.Split(description, "_")
		if len(parts) == 2 {
			if interval, err := strconv.Atoi(parts[1]); err == nil {
				eventInterval = interval
			}
		}

		// Determinar o modelo com base no tipo
		var model string
		if contains(DAHUA_CONTROLLER_TYPES, lcType) {
			model = "Dahua"
		} else if contains(HIKVISION_CONTROLLER_TYPES, lcType) {
			model = "Hikvision"
		} else {
			model = "-"
		}

		result = append(result, map[string]interface{}{
			"LocalControllerID": id,
			"Name":              name,
			"IPAddress":         ip,
			"Port":              port,
			"Type":              lcType,
			"Enabled":           enabled,
			"Model":             model,
			"EventInterval":     eventInterval,
		})
	}

	return result, nil
}

// CountUsersInSiteController conta usuários em um banco de dados de controlador de site
func (w *WxsDB) CountUsersInSiteController(controllerID int) (map[int]int, error) {
	// Esta função normalmente acessaria o banco de dados do controlador de site
	// Mas por enquanto, retornaremos apenas um placeholder
	return map[int]int{}, nil
}

// Constantes para tipos de controladores
var (
	DAHUA_CONTROLLER_TYPES = []int{
		22111, // DHI-ASI7213X-T1
		22121, // DHI-ASI7213Y-V3
		22131, // DHI-ASI7214Y-V3
	}

	HIKVISION_CONTROLLER_TYPES = []int{
		21101, // DS-K1T671
		21102, // DS-K1T673
	}
)

// Função auxiliar para verificar se um slice contém um valor
func contains(slice []int, val int) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

// Funções auxiliares de conversão segura
func toInt(val interface{}) (int, error) {
	switch v := val.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case int32:
		return int(v), nil
	case float64:
		return int(v), nil
	case string:
		return strconv.Atoi(v)
	default:
		return 0, fmt.Errorf("não é possível converter %T para int", val)
	}
}

func toString(val interface{}) (string, error) {
	switch v := val.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

func toBool(val interface{}) (bool, error) {
	switch v := val.(type) {
	case bool:
		return v, nil
	case int, int64, int32:
		return v != 0, nil
	case string:
		return strconv.ParseBool(v)
	default:
		return false, fmt.Errorf("não é possível converter %T para bool", val)
	}
}
