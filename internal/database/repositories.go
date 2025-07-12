package database

import (
	"context"
	"fmt"
	"time"
)

// Repository define a interface base para repositórios
type Repository interface {
	GetTotalUsers() (int, error)
	GetSetting(key string) (string, error)
	SetSetting(key, value string) error
}

// BaseRepository contém funcionalidade comum para repositórios
type BaseRepository struct {
	db       *EmulatorDB
	deviceID int
	timeout  time.Duration
}

// NewBaseRepository cria um novo repositório base
func NewBaseRepository(db *EmulatorDB, deviceID int) *BaseRepository {
	return &BaseRepository{
		db:       db,
		deviceID: deviceID,
		timeout:  5 * time.Second,
	}
}

// GetSetting obtém uma configuração do dispositivo
func (r *BaseRepository) GetSetting(key string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	return r.db.GetDeviceSettings(ctx, r.deviceID, key)
}

// SetSetting define uma configuração do dispositivo
func (r *BaseRepository) SetSetting(key, value string) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	return r.db.SetDeviceSettings(ctx, r.deviceID, key, value)
}

// CardRepository define operações específicas para cartões
type CardRepository interface {
	Repository
	FindCard(userID int) (string, []map[string]interface{}, error)
	GetCards(count, offset int) (string, []map[string]interface{}, error)
	AddCard(cardName string, userID int, cardNo string, validStart, validEnd string) (int64, error)
	RemoveCard(identifier interface{}) error
}

// FaceRepository define operações específicas para faces
type FaceRepository interface {
	Repository
	FindFaces() (int, error)
	GetFaces(count, offset int) ([]map[string]interface{}, error)
	AddFace(userID int, data string) error
	RemoveFace(userID int) error
}

// DahuaRepository implementa repositório específico para Dahua
type DahuaRepository struct {
	*BaseRepository
}

// NewDahuaRepository cria um novo repositório Dahua
func NewDahuaRepository(db *EmulatorDB, deviceID int) *DahuaRepository {
	return &DahuaRepository{
		BaseRepository: NewBaseRepository(db, deviceID),
	}
}

// GetTotalUsers retorna o número total de usuários para este dispositivo Dahua
func (r *DahuaRepository) GetTotalUsers() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	return r.db.GetTotalUsers(ctx, "Dahua", r.deviceID)
}

// FindCard encontra um cartão por UserID
func (r *DahuaRepository) FindCard(userID int) (string, []map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	return r.db.FindDahuaCard(ctx, r.deviceID, userID)
}

// GetCards retorna uma lista de cartões com paginação
func (r *DahuaRepository) GetCards(count, offset int) (string, []map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	return r.db.GetDahuaCards(ctx, r.deviceID, count, offset)
}

// AddCard adiciona um novo cartão
func (r *DahuaRepository) AddCard(cardName string, userID int, cardNo string, validStart, validEnd string) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	return r.db.AddDahuaCard(ctx, r.deviceID, cardName, userID, cardNo, validStart, validEnd)
}

// RemoveCard remove um cartão pelo RecNo
func (r *DahuaRepository) RemoveCard(identifier interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	recNo, ok := identifier.(int)
	if !ok {
		return fmt.Errorf("invalid identifier type for Dahua card removal")
	}

	return r.db.RemoveDahuaCard(ctx, r.deviceID, recNo)
}

// FindFaces retorna o número total de faces Dahua
func (r *DahuaRepository) FindFaces() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	return r.db.FindDahuaFaces(ctx)
}

// GetFaces retorna uma lista de faces Dahua com paginação
func (r *DahuaRepository) GetFaces(count, offset int) ([]map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	return r.db.GetDahuaFaces(ctx, count, offset)
}

// AddFace adiciona uma nova face Dahua
func (r *DahuaRepository) AddFace(userID int, md5 string) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	return r.db.AddDahuaFace(ctx, userID, md5)
}

// RemoveFace remove uma face Dahua por UserID
func (r *DahuaRepository) RemoveFace(userID int) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	return r.db.RemoveDahuaFace(ctx, userID)
}

// HikvisionRepository implementa repositório específico para Hikvision
type HikvisionRepository struct {
	*BaseRepository
}

// NewHikvisionRepository cria um novo repositório Hikvision
func NewHikvisionRepository(db *EmulatorDB, deviceID int) *HikvisionRepository {
	return &HikvisionRepository{
		BaseRepository: NewBaseRepository(db, deviceID),
	}
}

// GetTotalUsers retorna o número total de usuários para este dispositivo Hikvision
func (r *HikvisionRepository) GetTotalUsers() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	return r.db.GetTotalUsers(ctx, "Hikvision", r.deviceID)
}

// FindCard encontra um cartão por EmployeeNo
func (r *HikvisionRepository) FindCard(employeeNo interface{}) (string, []map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	empNo, ok := employeeNo.(string)
	if !ok {
		return "", nil, fmt.Errorf("invalid employee number type")
	}

	// Implementar busca específica do Hikvision
	// Por enquanto, retorna implementação básica
	return "found=0", nil, nil
}

// GetCards retorna uma lista de cartões Hikvision com paginação
func (r *HikvisionRepository) GetCards(count, offset int) (string, []map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	// Implementar busca específica do Hikvision
	// Por enquanto, retorna implementação básica
	return "found=0", nil, nil
}

// AddCard adiciona um novo cartão Hikvision
func (r *HikvisionRepository) AddCard(cardName string, employeeNo interface{}, cardNo string, validStart, validEnd string) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	empNo, ok := employeeNo.(string)
	if !ok {
		return 0, fmt.Errorf("invalid employee number type")
	}

	// Implementar adição específica do Hikvision
	_ = empNo // Por enquanto, apenas para evitar warning
	return 0, fmt.Errorf("not implemented")
}

// RemoveCard remove um cartão Hikvision
func (r *HikvisionRepository) RemoveCard(identifier interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	empNo, ok := identifier.(string)
	if !ok {
		return fmt.Errorf("invalid identifier type for Hikvision card removal")
	}

	// Implementar remoção específica do Hikvision
	_ = empNo // Por enquanto, apenas para evitar warning
	return fmt.Errorf("not implemented")
}

// FindFaces retorna o número total de faces Hikvision
func (r *HikvisionRepository) FindFaces() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	// Implementar busca específica do Hikvision
	_ = ctx // Por enquanto, apenas para evitar warning
	return 0, fmt.Errorf("not implemented")
}

// GetFaces retorna uma lista de faces Hikvision com paginação
func (r *HikvisionRepository) GetFaces(count, offset int) ([]map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	// Implementar busca específica do Hikvision
	_ = ctx // Por enquanto, apenas para evitar warning
	return nil, fmt.Errorf("not implemented")
}

// AddFace adiciona uma nova face Hikvision
func (r *HikvisionRepository) AddFace(userID int, photoData string) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	// Implementar adição específica do Hikvision
	_ = ctx // Por enquanto, apenas para evitar warning
	return fmt.Errorf("not implemented")
}

// RemoveFace remove uma face Hikvision por UserID
func (r *HikvisionRepository) RemoveFace(userID int) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	// Implementar remoção específica do Hikvision
	_ = ctx // Por enquanto, apenas para evitar warning
	return fmt.Errorf("not implemented")
}

// RepositoryFactory cria repositórios baseados no tipo de dispositivo
type RepositoryFactory struct {
	db *EmulatorDB
}

// NewRepositoryFactory cria uma nova factory de repositórios
func NewRepositoryFactory(db *EmulatorDB) *RepositoryFactory {
	return &RepositoryFactory{db: db}
}

// CreateCardRepository cria um repositório de cartões baseado no tipo
func (f *RepositoryFactory) CreateCardRepository(deviceType string, deviceID int) (CardRepository, error) {
	switch deviceType {
	case "Dahua":
		return NewDahuaRepository(f.db, deviceID), nil
	case "Hikvision":
		return NewHikvisionRepository(f.db, deviceID), nil
	default:
		return nil, fmt.Errorf("unsupported device type: %s", deviceType)
	}
}

// CreateFaceRepository cria um repositório de faces baseado no tipo
func (f *RepositoryFactory) CreateFaceRepository(deviceType string, deviceID int) (FaceRepository, error) {
	switch deviceType {
	case "Dahua":
		return NewDahuaRepository(f.db, deviceID), nil
	case "Hikvision":
		return NewHikvisionRepository(f.db, deviceID), nil
	default:
		return nil, fmt.Errorf("unsupported device type: %s", deviceType)
	}
}
