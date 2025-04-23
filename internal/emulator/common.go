package emulator

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"GoFacialEmulator/internal/database"
	"GoFacialEmulator/internal/models"
	"GoFacialEmulator/internal/trace"
	"GoFacialEmulator/internal/utils"
)

// Emulator is the interface implemented by all emulators
type Emulator interface {
	// Start starts the emulator
	Start() error
	
	// Stop stops the emulator
	Stop() error
	
	// IsRunning returns true if the emulator is running
	IsRunning() bool
	
	// GetInfo returns information about the emulator
func (e *BaseEmulator) GetInfo() models.Device {
	e.Lock()
	defer e.Unlock()
	
	// Update status
	if e.running {
		e.Device.Status = "running"
	} else {
		e.Device.Status = "stopped"
	}
	
	return e.Device
}

// StartEventGenerator starts a goroutine that periodically generates events
func (e *BaseEmulator) StartEventGenerator(generator func() error) {
	go func() {
		ticker := time.NewTicker(time.Duration(e.Device.EventInterval) * time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				if e.IsRunning() {
					if err := generator(); err != nil {
						e.Tracer.Error("Failed to generate event: %v", err)
					}
				}
			case <-e.stopChan:
				return
			}
		}
	}()
}

// CalculateMD5 calculates the MD5 hash of the given data
func CalculateMD5(data []byte) string {
	hash := md5.New()
	hash.Write(data)
	return fmt.Sprintf("%X", hash.Sum(nil))
}

// DecodeFaceImage decodes a base64-encoded image
func DecodeFaceImage() ([]byte, error) {
	return base64.StdEncoding.DecodeString(PhotoImg)
}

// HandleStatus is a common handler for status requests
func (e *BaseEmulator) HandleStatus(w http.ResponseWriter, r *http.Request) {
	info := e.GetInfo()
	
	// Get total user count
	count, err := e.DB.GetTotalUsers()
	if err == nil {
		info.TotalUsers = count
	}
	
	response := map[string]interface{}{
		"CurrentDatetime": time.Now().Format("2006-01-02 15:04:05"),
		"TotalUsers":      info.TotalUsers,
		"Status":          info.Status,
		"Model":           info.Model,
	}
	
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// SendEventToRemoteServer sends an event to the remote server
func (e *BaseEmulator) SendEventToRemoteServer(event interface{}, contentType string) error {
	// Check if remote authentication is enabled
	localAuth, err := e.DB.GetDeviceSettings("LocalAuthentication")
	if err != nil {
		return fmt.Errorf("failed to get LocalAuthentication setting: %w", err)
	}
	
	if localAuth == "1" {
		// Local authentication is enabled, don't send event to remote server
		return nil
	}
	
	// Get remote server URL
	remoteServer, err := e.DB.GetDeviceSettings("RemoteServer")
	if err != nil {
		return fmt.Errorf("failed to get RemoteServer setting: %w", err)
	}
	
	remotePort, err := e.DB.GetDeviceSettings("RemotePort")
	if err != nil {
		return fmt.Errorf("failed to get RemotePort setting: %w", err)
	}
	
	remoteURL := fmt.Sprintf("http://%s:%s/notification", remoteServer, remotePort)
	
	// Marshal event to JSON
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}
	
	// Send event to remote server
	req, err := http.NewRequest("POST", remoteURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send event: %w", err)
	}
	defer resp.Body.Close()
	
	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}
	
	e.Tracer.Info("Remote server response: %s", string(body))
	
	// If the access was successful, simulate door opening/closing
	if utils.RandomAccessNotDone() {
		time.Sleep(2 * time.Second)
		if err := e.SendDoorEvent("Open"); err != nil {
			e.Tracer.Error("Failed to send door open event: %v", err)
		}
		
		time.Sleep(3 * time.Second)
		if err := e.SendDoorEvent("Close"); err != nil {
			e.Tracer.Error("Failed to send door close event: %v", err)
		}
	}
	
	return nil
}

// SendDoorEvent sends a door status event to the remote server
func (e *BaseEmulator) SendDoorEvent(status string) error {
	// Implementation will vary based on emulator type
	return nil
}
	GetInfo() models.Device
	
	// GenerateEvent generates and sends an event
	GenerateEvent() error
}

// BaseEmulator contains common functionality for all emulators
type BaseEmulator struct {
	sync.Mutex
	Device      models.Device
	DB          *database.EmulatorDB
	Tracer      *trace.Tracer
	Server      *http.Server
	running     bool
	stopChan    chan struct{}
	MacAddress  string
	RemoteURL   string
}

// NewBaseEmulator creates a new base emulator
func NewBaseEmulator(db *database.EmulatorDB, device models.Device, tracer *trace.Tracer) *BaseEmulator {
	macAddress := utils.GenerateMacAddress()
	
	return &BaseEmulator{
		Device:     device,
		DB:         db,
		Tracer:     tracer,
		running:    false,
		stopChan:   make(chan struct{}),
		MacAddress: macAddress,
		RemoteURL:  fmt.Sprintf("http://%s:%d/notification", device.IPAddress, device.EventInterval),
	}
}

// Start starts the emulator
func (e *BaseEmulator) Start() error {
	e.Lock()
	defer e.Unlock()

	if e.running {
		return fmt.Errorf("emulator already running")
	}

	e.Tracer.Info("Starting emulator: %s", e.Device.Name)
	e.stopChan = make(chan struct{})
	e.running = true

	return nil
}

// Stop stops the emulator
func (e *BaseEmulator) Stop() error {
	e.Lock()
	defer e.Unlock()

	if !e.running {
		return fmt.Errorf("emulator not running")
	}

	e.Tracer.Info("Stopping emulator: %s", e.Device.Name)
	close(e.stopChan)
	e.running = false

	if e.Server != nil {
		return e.Server.Close()
	}

	return nil
}

// IsRunning returns true if the emulator is running
func (e *BaseEmulator) IsRunning() bool {
	e.Lock()
	defer e.Unlock()
	return e.running
}

// GetInfo returns information about the emulator