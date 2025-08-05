package dahua

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"GoFacialEmulator/internal/database"
	"GoFacialEmulator/internal/models"
	"GoFacialEmulator/internal/trace"
	"GoFacialEmulator/internal/utils"

	"github.com/gin-gonic/gin"
)

// Emulator representa o emulador para dispositivos Dahua
type Emulator struct {
	device     models.Device
	tracer     *trace.Tracer
	repo       *Repository
	server     *http.Server
	running    bool
	stopChan   chan struct{}
	macAddress string

	// Configurações do servidor remoto
	remoteServer    string
	remotePort      string
	remoteServerURL string
}

// NewEmulator cria uma nova instância do emulador Dahua
func NewEmulator(db *database.AdaptivePool, device models.Device, tracer *trace.Tracer) *Emulator {
	tracer.Info("Initializing Dahua emulator model: %s v1.1", device.Name)

	repo := NewRepository(db, device.ID)
	macAddress := utils.GenerateMacAddress()

	emulator := &Emulator{
		device:     device,
		tracer:     tracer,
		repo:       repo,
		running:    false,
		macAddress: macAddress,
		stopChan:   make(chan struct{}),
	}

	// Inicializar configurações do servidor remoto
	emulator.initializeRemoteSettings()

	return emulator
}

// initializeRemoteSettings inicializa as configurações do servidor remoto
func (e *Emulator) initializeRemoteSettings() {
	if server, err := e.repo.GetSetting("RemoteServer"); err == nil && server != "" {
		e.remoteServer = server
	} else {
		e.remoteServer = "localhost"
	}

	if port, err := e.repo.GetSetting("RemotePort"); err == nil && port != "" {
		e.remotePort = port
	} else {
		e.remotePort = "15501"
	}

	e.remoteServerURL = fmt.Sprintf("http://%s:%s", e.remoteServer, e.remotePort)
	e.tracer.Info("Remote server URL: %s", e.remoteServerURL)
}

// Start inicia o servidor do emulador
func (e *Emulator) Start() error {
	if e.running {
		return fmt.Errorf("emulator already running")
	}

	e.tracer.Info("Starting Dahua emulator: %s", e.device.Name)

	// Configura o router
	router := gin.Default()
	e.SetupRoutes(router)

	// Inicia o servidor HTTP
	addr := fmt.Sprintf("%s:%d", e.device.IPAddress, e.device.Port)
	e.tracer.Info("Starting Dahua HTTP server on %s", addr)

	e.server = &http.Server{
		Addr:    addr,
		Handler: router,
	}

	// Inicia o servidor em uma goroutine
	go func() {
		if err := e.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			e.tracer.Error("Failed to start Dahua server: %v", err)
		}
	}()

	e.running = true

	// Inicia o gerador de eventos se configurado
	if e.device.EventInterval > 0 {
		go e.startEventGenerator()
	}

	return nil
}

// Stop para o emulador
func (e *Emulator) Stop() error {
	if !e.running {
		return fmt.Errorf("emulator not running")
	}

	e.tracer.Info("Stopping Dahua emulator: %s", e.device.Name)

	// Sinalizar parada
	close(e.stopChan)
	e.running = false

	// Parar servidor HTTP se estiver rodando
	if e.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return e.server.Shutdown(ctx)
	}

	return nil
}

// IsRunning retorna true se o emulador estiver rodando
func (e *Emulator) IsRunning() bool {
	return e.running
}

// GetInfo retorna informações sobre o emulador
func (e *Emulator) GetInfo() models.Device {
	device := e.device
	if e.running {
		device.Status = "running"
	} else {
		device.Status = "stopped"
	}

	// Atualizar contagem de usuários
	if count, err := e.repo.GetTotalUsers(); err == nil {
		device.TotalUsers = count
	}

	return device
}

// GetType retorna o tipo do emulador
func (e *Emulator) GetType() string {
	return "Dahua"
}

// GetStatus retorna o status atual do emulador
func (e *Emulator) GetStatus() string {
	if e.running {
		return "running"
	}
	return "stopped"
}

// GenerateEvent gera e envia um evento
func (e *Emulator) GenerateEvent() error {
	// Verifica se a autenticação local está ativada
	localAuth, err := e.repo.GetSetting("LocalAuthentication")
	if err != nil {
		return fmt.Errorf("failed to get LocalAuthentication setting: %w", err)
	}

	if localAuth == "0" {
		// Modo de autenticação remota, enviar para o servidor remoto
		return e.generateOnlineEvent()
	}

	return nil
}

// startEventGenerator inicia o gerador de eventos automático
func (e *Emulator) startEventGenerator() {
	e.tracer.Info("Starting event generator with interval: %ds", e.device.EventInterval)

	ticker := time.NewTicker(time.Duration(e.device.EventInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if e.running {
				if err := e.GenerateEvent(); err != nil {
					e.tracer.Error("Failed to generate event: %v", err)
				}
			}
		case <-e.stopChan:
			e.tracer.Info("Event generator stopped")
			return
		}
	}
}

// generateRandomEvent gera um evento aleatório para streaming local
func (e *Emulator) generateRandomEvent() ([]byte, error) {
	e.tracer.Info("Generating random event for streaming")

	// Verificar se autenticação local está ativada
	localAuth, err := e.repo.GetSetting("LocalAuthentication")
	if err != nil || localAuth != "1" {
		return nil, nil // Não gerar evento se não for modo local
	}

	// Buscar um cartão aleatório
	cardName, cardNo, userID, err := e.repo.GetRandomCard()
	if err != nil {
		return nil, fmt.Errorf("failed to get random card: %w", err)
	}

	// Gerar evento no formato Dahua (texto simples)
	eventText := fmt.Sprintf(`Events[0].Alive=100
Events[0].CardName=%s
Events[0].CardNo=%s
Events[0].CardType=0
Events[0].CreateTime=%d
Events[0].Door=0
Events[0].ErrorCode=0
Events[0].EventBaseInfo.Action=Pulse
Events[0].EventBaseInfo.Code=AccessControl
Events[0].EventBaseInfo.Index=0
Events[0].FaceIndex=0
Events[0].ImageInfo[0].Height=384
Events[0].ImageInfo[0].Length=15225
Events[0].ImageInfo[0].Offset=0
Events[0].ImageInfo[0].Type=1
Events[0].ImageInfo[0].Width=640
Events[0].ImageInfo[1].Height=420
Events[0].ImageInfo[1].Length=21710
Events[0].ImageInfo[1].Offset=15225
Events[0].ImageInfo[1].Type=0
Events[0].ImageInfo[1].Width=360
Events[0].ImageInfo[2].Height=608
Events[0].ImageInfo[2].Length=22531
Events[0].ImageInfo[2].Offset=36935
Events[0].ImageInfo[2].Type=2
Events[0].ImageInfo[2].Width=480
Events[0].Method=15
Events[0].ReaderID=1
Events[0].RealUTC=%d
Events[0].Similarity=99
Events[0].SnapPath=/var/tmp/white_part5.jpg
Events[0].Status=1
Events[0].Type=Entry
Events[0].UTC=%d
Events[0].UserID=%d
Events[0].UserType=0
`, cardName, cardNo, time.Now().Unix(), time.Now().Unix(), time.Now().Unix(), userID)

	// Formatar como multipart
	boundary := "myboundary"
	eventPackage := fmt.Sprintf("\r\n\r\n\r\n--%s\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n%s",
		boundary, len(eventText), eventText)

	// Decodificar a imagem
	imageData, err := GetPhotoImageData()
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// Adicionar a imagem ao pacote
	dataPhoto := fmt.Sprintf("\r\n--%s\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n",
		boundary, len(imageData))

	return []byte(eventPackage + dataPhoto + string(imageData)), nil
}

// generateOnlineEvent gera um evento online para envio ao servidor remoto
func (e *Emulator) generateOnlineEvent() error {
	e.tracer.Info("Generating online event")

	// Buscar um cartão aleatório
	_, _, userID, err := e.repo.GetRandomCard()
	if err != nil {
		e.tracer.Warning("No card found for event generation: %v", err)
		return nil
	}

	currentTime := time.Now().UTC()

	// Criar evento base
	event := &Event{
		Events: []EventData{
			{
				Action: "Pulse",
				Code:   "AccessControl",
				Data: EventDataDetails{
					CardStatus:   0,
					CardType:     0,
					Door:         0,
					ErrorCode:    96,
					EventGroupID: 0,
					Method:       15,
					ReadID:       "1",
					Status:       0,
					Type:         "Entry",
					UTC:          currentTime.Unix(),
					UserID:       userID,
					UserType:     0,
				},
				Index:           0,
				PhysicalAddress: e.macAddress,
			},
		},
		Time: currentTime.Format("02-01-2006 15:04:05"),
	}

	// Criar primeira parte do evento
	eventJSON, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Formatar como multipart
	boundary := "myboundary"
	eventPart := fmt.Sprintf("\r\n--%s\r\nContent-Type: text/plain\r\nContent-Disposition: form-data; name=\"info\"\r\n\r\n%s\r\n--%s--\r\n\r\n",
		boundary, string(eventJSON), boundary)

	// Enviar primeira parte do evento
	if err := e.sendEventToRemoteServer(eventPart); err != nil {
		return fmt.Errorf("failed to send first event: %w", err)
	}

	// Criar segunda parte com imagem (se necessário)
	eventWithImage := &EventWithImage{
		Event:    *event,
		Channel:  0,
		FilePath: "\\/mnt\\/appdata1\\/userpic\\/SnapShot\\/2024-04-16\\/21\\/07\\/20240416210702098.jpg",
	}

	// Adicionar ImageInfo
	eventWithImage.Events[0].Data.ImageInfo = []ImageInfo{
		{
			Height: 640,
			Length: 14088,
			Offset: 0,
			Type:   1,
			Width:  360,
		},
	}
	eventWithImage.Events[0].Data.Method = 4

	// Codificar evento com imagem
	eventWithImageJSON, err := json.MarshalIndent(eventWithImage, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal event with image: %w", err)
	}

	// Decodificar a imagem
	imageData, err := GetPhotoImageData()
	if err != nil {
		return fmt.Errorf("failed to decode image: %w", err)
	}

	// Formatar evento com imagem
	eventWithImagePart := fmt.Sprintf("\r\n--%s\r\nContent-Type: text/plain\r\nContent-Disposition: form-data; name=\"info\"\r\n\r\n%s\r\n--%s\r\nContent-Type: image/jpeg\r\nContent-Disposition: form-data; name=\"file\"\r\n\r\n",
		boundary, string(eventWithImageJSON), boundary)

	fullEventWithImage := eventWithImagePart + string(imageData) + fmt.Sprintf("\r\n--%s--\r\n\r\n", boundary)

	// Enviar segunda parte com imagem
	if err := e.sendEventToRemoteServer(fullEventWithImage); err != nil {
		return fmt.Errorf("failed to send event with image: %w", err)
	}

	// Se for necessário simular eventos de porta (aleatoriamente)
	if utils.RandomAccessNotDone() {
		e.tracer.Info("Sending door events to complete access")
		go e.simulateDoorEvents()
	}

	return nil
}

// sendEventToRemoteServer envia evento para servidor remoto
func (e *Emulator) sendEventToRemoteServer(eventData string) error {
	remoteURL := e.remoteServerURL + "/notification"
	e.tracer.Info("Sending event to server: %s", remoteURL)

	// Fazer a requisição HTTP
	req, err := http.NewRequest("POST", remoteURL, strings.NewReader(eventData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "multipart/form-data; boundary=myboundary")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Verificar a resposta
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response: %s", resp.Status)
	}

	e.tracer.Info("Event sent successfully to %s", remoteURL)
	return nil
}

// simulateDoorEvents simula eventos de porta
func (e *Emulator) simulateDoorEvents() {
	time.Sleep(2 * time.Second)
	if err := e.sendDoorEvent("Open"); err != nil {
		e.tracer.Error("Failed to send door open event: %v", err)
	}

	time.Sleep(3 * time.Second)
	if err := e.sendDoorEvent("Close"); err != nil {
		e.tracer.Error("Failed to send door close event: %v", err)
	}
}

// sendDoorEvent envia um evento de estado da porta
func (e *Emulator) sendDoorEvent(status string) error {
	currentTime := time.Now().UTC()
	event := map[string]interface{}{
		"Events": []map[string]interface{}{
			{
				"Action": "Pulse",
				"Code":   "DoorStatus",
				"Data": map[string]interface{}{
					"Status": status,
					"UTC":    currentTime.Unix(),
				},
				"Index":           0,
				"PhysicalAddress": e.macAddress,
			},
		},
		"Time": currentTime.Format("02-01-2006 15:04:05"),
	}

	// Codificar o evento em JSON
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal door event: %w", err)
	}

	// Formatar o evento como multipart
	boundary := "myboundary"
	body := fmt.Sprintf("\r\n--%s\r\nContent-Type: text/plain\r\nContent-Disposition: form-data; name=\"info\"\r\n\r\n%s\r\n--%s--\r\n\r\n",
		boundary, string(eventJSON), boundary)

	// Enviar o evento para o servidor remoto
	return e.sendEventToRemoteServer(body)
}

// PhotoImg constante com imagem base64 para eventos (mesma do Hikvision)
const PhotoImg = `
/9j/4AAQSkZJRgABAQEAYABgAAD/2wBDAAIBAQIBAQICAgICAgICAwUDAwMDAwYEBAMFBwYHBwcG
BwcICQsJCAgKCAcHCg0KCgsMDAwMBwkODw0MDgsMDAz/2wBDAQICAgMDAwYDAwYMCAcIDAwMDAwM
DAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAz/wAARCADIAKADASIA
AhEBAxEB/8QAHwAAAQUBAQEBAQEAAAAAAAAAAAECAwQFBgcICQoL/8QAtRAAAgEDAwIEAwUFBAQA
AAF9AQIDAAQRBRIhMUEGE1FhByJxFDKBkaEII0KxwRVS0fAkM2JyggkKFhcYGRolJicoKSo0NTY3
ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqDhIWGh4iJipKTlJWWl5iZmqKjpKWm
p6ipqrKztLW2t7i5usLDxMXGx8jJytLT1NXW19jZ2uHi4+Tl5ufo6erx8vP09fb3+Pn6/8QAHwEA
AwEBAQEBAQEBAQAAAAAAAAECAwQFBgcICQoL/8QAtREAAgECBAQDBAcFBAQAAQJ3AAECAxEEBSEx
BhJBUQdhcRMiMoEIFEKRobHBCSMzUvAVYnLRChYkNOEl8RcYGRomJygpKjU2Nzg5OkNERUZHSElK
U1RVVldYWVpjZGVmZ2hpanN0dXZ3eHl6goOEhYaHiImKkpOUlZaXmJmaoqOkpaanqKmqsrO0tba3
uLm6wsPExcbHyMnK0tPU1dbX2Nna4uPk5ebn6Onq8vP09fb3+Pn6/9oADAMBAAIRAxEAPwD9/KKK
KACiiigAooooA=
`

// GetCleanPhotoBase64 retorna a string base64 limpa para uso
func GetCleanPhotoBase64() string {
	// Remove quebras de linha, espaços e caracteres de controle
	cleaned := strings.ReplaceAll(PhotoImg, "\n", "")
	cleaned = strings.ReplaceAll(cleaned, "\r", "")
	cleaned = strings.ReplaceAll(cleaned, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "\t", "")

	// Remove o primeiro e último caractere se forem quebras de linha vazias
	cleaned = strings.TrimSpace(cleaned)

	return cleaned
}

// GetPhotoImageData retorna os dados da imagem decodificados
func GetPhotoImageData() ([]byte, error) {
	cleanBase64 := GetCleanPhotoBase64()
	return base64.StdEncoding.DecodeString(cleanBase64)
}
