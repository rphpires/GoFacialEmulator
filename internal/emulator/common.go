package emulator

import (
	"GoFacialEmulator/internal/models"
)

// Emulator define a interface que todos os emuladores devem implementar
type Emulator interface {
	Start() error
	Stop() error
	IsRunning() bool
	GetInfo() models.Device
	GenerateEvent() error
	GetType() string
	GetTotalUsers() (int, error)
}
