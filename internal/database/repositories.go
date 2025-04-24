package database

import (
	"context"
)

// DahuaRepository é uma estrutura que encapsula operações do banco de dados específicas do Dahua
type DahuaRepository struct {
	db       *EmulatorDB
	deviceID int
}

// NewDahuaRepository cria um novo repositório Dahua para um dispositivo específico
func NewDahuaRepository(db *EmulatorDB, deviceID int) *DahuaRepository {
	return &DahuaRepository{
		db:       db,
		deviceID: deviceID,
	}
}

// FindCard encontra um cartão por UserID
func (r *DahuaRepository) FindCard(userID int) (string, []map[string]interface{}, error) {
	ctx := context.Background()
	return r.db.FindDahuaCard(ctx, r.deviceID, userID)
}

// GetCards retorna uma lista de cartões com paginação
func (r *DahuaRepository) GetCards(count, offset int) (string, []map[string]interface{}, error) {
	ctx := context.Background()
	return r.db.GetDahuaCards(ctx, r.deviceID, count, offset)
}

// AddCard adiciona um novo cartão
func (r *DahuaRepository) AddCard(cardName string, userID int, cardNo string, validStart, validEnd string) (int64, error) {
	ctx := context.Background()
	return r.db.AddDahuaCard(ctx, r.deviceID, cardName, userID, cardNo, validStart, validEnd)
}

// RemoveCard remove um cartão pelo RecNo
func (r *DahuaRepository) RemoveCard(recNo int) error {
	ctx := context.Background()
	return r.db.RemoveDahuaCard(ctx, r.deviceID, recNo)
}

// GetTotalUsers retorna o número total de usuários para este dispositivo
func (r *DahuaRepository) GetTotalUsers() (int, error) {
	ctx := context.Background()
	return r.db.GetTotalUsers(ctx, "Dahua", r.deviceID)
}

// GetSetting obtém uma configuração do dispositivo
func (r *DahuaRepository) GetSetting(cfgID string) (string, error) {
	ctx := context.Background()
	return r.db.GetDeviceSettings(ctx, r.deviceID, cfgID)
}

// SetSetting define uma configuração do dispositivo
func (r *DahuaRepository) SetSetting(cfgID, value string) error {
	ctx := context.Background()
	return r.db.SetDeviceSettings(ctx, r.deviceID, cfgID, value)
}

// FindFaces retorna o número total de faces Dahua
func (r *DahuaRepository) FindFaces() (int, error) {
	ctx := context.Background()
	return r.db.FindDahuaFaces(ctx)
}

// GetFaces retorna uma lista de faces Dahua com paginação
func (r *DahuaRepository) GetFaces(count, offset int) ([]map[string]interface{}, error) {
	ctx := context.Background()
	return r.db.GetDahuaFaces(ctx, count, offset)
}

// AddFace adiciona uma nova face Dahua
func (r *DahuaRepository) AddFace(userID int, md5 string) error {
	ctx := context.Background()
	return r.db.AddDahuaFace(ctx, userID, md5)
}

// RemoveFace remove uma face Dahua por UserID
func (r *DahuaRepository) RemoveFace(userID int) error {
	ctx := context.Background()
	return r.db.RemoveDahuaFace(ctx, userID)
}

// HikvisionRepository é uma estrutura que encapsula operações do banco de dados específicas do Hikvision
type HikvisionRepository struct {
	db       *EmulatorDB
	deviceID int
}

// NewHikvisionRepository cria um novo repositório Hikvision para um dispositivo específico
func NewHikvisionRepository(db *EmulatorDB, deviceID int) *HikvisionRepository {
	return &HikvisionRepository{
		db:       db,
		deviceID: deviceID,
	}
}

// GetTotalUsers retorna o número total de usuários para este dispositivo
func (r *HikvisionRepository) GetTotalUsers() (int, error) {
	ctx := context.Background()
	return r.db.GetTotalUsers(ctx, "Hikvision", r.deviceID)
}

// GetSetting obtém uma configuração do dispositivo
func (r *HikvisionRepository) GetSetting(cfgID string) (string, error) {
	ctx := context.Background()
	return r.db.GetDeviceSettings(ctx, r.deviceID, cfgID)
}

// SetSetting define uma configuração do dispositivo
func (r *HikvisionRepository) SetSetting(cfgID, value string) error {
	ctx := context.Background()
	return r.db.SetDeviceSettings(ctx, r.deviceID, cfgID, value)
}

// Adicione aqui os métodos específicos para o Hikvision
