package hikvision

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

// Emulator representa o emulador para dispositivos Hikvision
type Emulator struct {
	device           models.Device
	tracer           *trace.Tracer
	repo             *Repository
	server           *http.Server
	running          bool
	stopChan         chan struct{}
	macAddress       string
	deleteInProgress bool

	// Configurações do servidor remoto
	remoteServer    string
	remotePort      string
	remoteServerURL string
}

// NewEmulator cria uma nova instância do emulador Hikvision
func NewEmulator(db database.DBInterface, device models.Device, tracer *trace.Tracer) *Emulator {
	tracer.Info("Initializing Hikvision emulator model: %s", device.Name)

	repo := NewRepository(db, device.ID)
	macAddress := utils.GenerateMacAddress()

	emulator := &Emulator{
		device:           device,
		tracer:           tracer,
		repo:             repo,
		running:          false,
		macAddress:       macAddress,
		deleteInProgress: false,
		stopChan:         make(chan struct{}),
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

	e.tracer.Info("Starting Hikvision emulator: %s", e.device.Name)

	// Configura o router
	router := gin.Default()
	e.SetupRoutes(router)

	// Inicia o servidor HTTP
	addr := fmt.Sprintf("%s:%d", e.device.IPAddress, e.device.Port)
	e.tracer.Info("Starting Hikvision HTTP server on %s", addr)

	e.server = &http.Server{
		Addr:    addr,
		Handler: router,
	}

	// Inicia o servidor em uma goroutine
	go func() {
		if err := e.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			e.tracer.Error("Failed to start Hikvision server: %v", err)
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

	e.tracer.Info("Stopping Hikvision emulator: %s", e.device.Name)

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
	return "Hikvision"
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

// generateOnlineEvent gera um evento online
func (e *Emulator) generateOnlineEvent() error {
	e.tracer.Info("Generating online event")

	// Busca uma linha aleatória de usuário + cartão
	name, cardNo, employeeNo, err := e.repo.GetRandomUserAndCard()
	if err != nil {
		e.tracer.Warning("No user/card found for event generation: %v", err)
		return nil
	}

	// Obtém o fuso horário local
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	currentTime := time.Now().In(loc)

	// Cria o evento
	event := &Event{
		IPAddress:        e.device.IPAddress,
		IPv6Address:      "fe80::be5e:33ff:fe57:a5cb",
		PortNo:           e.device.Port,
		Protocol:         "HTTP",
		MacAddress:       e.macAddress,
		ChannelID:        1,
		DateTime:         currentTime.Format("2006-01-02T15:04:05-03:00"),
		ActivePostCount:  1,
		EventType:        "AccessControllerEvent",
		EventState:       "active",
		EventDescription: "Access Controller Event",
	}

	event.AccessControllerEvent.DeviceName = "subdoorOne"
	event.AccessControllerEvent.MajorEventType = 5
	event.AccessControllerEvent.SubEventType = 75
	event.AccessControllerEvent.CardNo = cardNo
	event.AccessControllerEvent.CardType = 1
	event.AccessControllerEvent.Name = name
	event.AccessControllerEvent.CardReaderKind = 1
	event.AccessControllerEvent.CardReaderNo = 1
	event.AccessControllerEvent.VerifyNo = 189
	event.AccessControllerEvent.EmployeeNoString = employeeNo
	event.AccessControllerEvent.SerialNo = 4435
	event.AccessControllerEvent.UserType = "normal"
	event.AccessControllerEvent.CurrentVerifyMode = "faceOrFpOrCardOrPw"
	event.AccessControllerEvent.CurrentEvent = true
	event.AccessControllerEvent.FrontSerialNo = 4434
	event.AccessControllerEvent.AttendanceStatus = "undefined"
	event.AccessControllerEvent.Label = ""
	event.AccessControllerEvent.StatusValue = 0
	event.AccessControllerEvent.Mask = "no"
	event.AccessControllerEvent.Helmet = "unknown"
	event.AccessControllerEvent.PicturesNumber = 1
	event.AccessControllerEvent.PurePwdVerifyEnable = true
	event.AccessControllerEvent.FaceRect.Height = 0.268
	event.AccessControllerEvent.FaceRect.Width = 0.477
	event.AccessControllerEvent.FaceRect.X = 0.286
	event.AccessControllerEvent.FaceRect.Y = 0.354
	event.AccessControllerEvent.UnlockRoomNo = "3723243075"

	// Enviar evento para servidor remoto
	if err := e.sendEventToRemoteServer(event); err != nil {
		return fmt.Errorf("failed to send event: %w", err)
	}

	// Se for necessário simular eventos de porta (aleatoriamente)
	if utils.RandomAccessNotDone() {
		e.tracer.Info("Sending door events to complete access")
		go e.simulateDoorEvents()
	}

	return nil
}

// sendEventToRemoteServer envia evento para servidor remoto
func (e *Emulator) sendEventToRemoteServer(event *Event) error {
	// Codifica o evento em JSON
	eventJSON, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Formata o evento como multipart
	boundary := "MIME_boundary"
	contentLength := len(eventJSON)
	evtPackage := fmt.Sprintf("\r\n--%s\r\nContent-Type: application/json; charset=\"UTF-8\"\r\nContent-Length: %d\r\n\r\n%s",
		boundary, contentLength, string(eventJSON))

	// Decodifica a imagem
	imageData, err := base64.StdEncoding.DecodeString(PhotoImg)
	if err != nil {
		return fmt.Errorf("failed to decode image: %w", err)
	}

	// Adiciona a imagem ao pacote
	dataPhoto := fmt.Sprintf("\r\n--%s\r\nContent-Disposition: form-data; name=\"Picture\"; filename=\"Picture.jpg\"\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\nContent-ID: pictureImage\r\n\r\n",
		boundary, len(imageData))

	// Envia o evento para o servidor remoto
	remoteURL := e.remoteServerURL + "/notification"
	e.tracer.Info("Sending event to server: %s", remoteURL)

	// Cria o corpo da requisição com o evento e a imagem
	body := evtPackage + dataPhoto + string(imageData)

	// Faz a requisição HTTP
	req, err := http.NewRequest("POST", remoteURL, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", fmt.Sprintf("multipart/form-data; boundary=%s", boundary))

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Verifica a resposta
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
	// Cria o evento de porta no formato Dahua (para compatibilidade)
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

	// Codifica o evento em JSON
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal door event: %w", err)
	}

	// Formata o evento como multipart
	boundary := "myboundary"
	body := fmt.Sprintf("\r\n--%s\r\nContent-Type: text/plain\r\nContent-Disposition: form-data; name=\"info\"\r\n\r\n%s\r\n--%s--\r\n\r\n",
		boundary, string(eventJSON), boundary)

	// Envia o evento para o servidor remoto
	remoteURL := e.remoteServerURL + "/notification"
	req, err := http.NewRequest("POST", remoteURL, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create door event request: %w", err)
	}
	req.Header.Set("Content-Type", fmt.Sprintf("multipart/form-data; boundary=%s", boundary))

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send door event: %w", err)
	}
	defer resp.Body.Close()

	return nil
}

// generateRandomEvent gera um evento aleatório para streaming
func (e *Emulator) generateRandomEvent() ([]byte, error) {
	e.tracer.Info("Generating random event for streaming")

	// Busca uma linha aleatória de usuário + cartão
	name, cardNo, employeeNo, err := e.repo.GetRandomUserAndCard()
	if err != nil {
		return nil, fmt.Errorf("failed to get random user info: %w", err)
	}

	// Obtém o fuso horário local
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	currentTime := time.Now().In(loc)

	// Cria o evento
	event := &Event{
		IPAddress:        e.device.IPAddress,
		IPv6Address:      "fe80::be5e:33ff:fe57:a5cb",
		PortNo:           e.device.Port,
		Protocol:         "HTTP",
		MacAddress:       e.macAddress,
		ChannelID:        1,
		DateTime:         currentTime.Format("2006-01-02T15:04:05-03:00"),
		ActivePostCount:  1,
		EventType:        "AccessControllerEvent",
		EventState:       "active",
		EventDescription: "Access Controller Event",
	}

	event.AccessControllerEvent.DeviceName = "subdoorOne"
	event.AccessControllerEvent.MajorEventType = 5
	event.AccessControllerEvent.SubEventType = 75
	event.AccessControllerEvent.CardNo = cardNo
	event.AccessControllerEvent.CardType = 1
	event.AccessControllerEvent.Name = name
	event.AccessControllerEvent.CardReaderKind = 1
	event.AccessControllerEvent.CardReaderNo = 1
	event.AccessControllerEvent.VerifyNo = 189
	event.AccessControllerEvent.EmployeeNoString = employeeNo
	event.AccessControllerEvent.SerialNo = 4435
	event.AccessControllerEvent.UserType = "normal"
	event.AccessControllerEvent.CurrentVerifyMode = "faceOrFpOrCardOrPw"
	event.AccessControllerEvent.CurrentEvent = true
	event.AccessControllerEvent.FrontSerialNo = 4434
	event.AccessControllerEvent.AttendanceStatus = "undefined"
	event.AccessControllerEvent.Label = ""
	event.AccessControllerEvent.StatusValue = 0
	event.AccessControllerEvent.Mask = "no"
	event.AccessControllerEvent.Helmet = "unknown"
	event.AccessControllerEvent.PicturesNumber = 1
	event.AccessControllerEvent.PurePwdVerifyEnable = true
	event.AccessControllerEvent.FaceRect.Height = 0.268
	event.AccessControllerEvent.FaceRect.Width = 0.477
	event.AccessControllerEvent.FaceRect.X = 0.286
	event.AccessControllerEvent.FaceRect.Y = 0.354
	event.AccessControllerEvent.UnlockRoomNo = "3723243075"

	// Codifica o evento em JSON
	eventJSON, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event: %w", err)
	}

	// Formata o evento como multipart
	boundary := "MIME_boundary"
	contentLength := len(eventJSON)
	evtPackage := fmt.Sprintf("\r\n--%s\r\nContent-Type: application/json; charset=\"UTF-8\"\r\nContent-Length: %d\r\n\r\n%s",
		boundary, contentLength, string(eventJSON))

	// Decodifica a imagem
	imageData, err := base64.StdEncoding.DecodeString(PhotoImg)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// Adiciona a imagem ao pacote
	dataPhoto := fmt.Sprintf("\r\n--%s\r\nContent-Disposition: form-data; name=\"Picture\"; filename=\"Picture.jpg\"\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\nContent-ID: pictureImage\r\n\r\n",
		boundary, len(imageData))

	return []byte(evtPackage + dataPhoto + string(imageData)), nil
}

// getHeartbeatMessage retorna uma mensagem de heartbeat
func (e *Emulator) getHeartbeatMessage() []byte {
	// Cria a mensagem de heartbeat
	heartbeat := map[string]interface{}{
		"ipAddress":        e.device.IPAddress,
		"portNo":           e.device.Port,
		"protocol":         "HTTP",
		"macAddress":       e.macAddress,
		"channelID":        1,
		"dateTime":         time.Now().Format("2006-01-02T15:04:05-07:00"),
		"activePostCount":  1,
		"eventType":        "videoloss",
		"eventState":       "inactive",
		"eventDescription": "videoloss alarm",
	}

	// Codifica o heartbeat em JSON
	heartbeatJSON, _ := json.MarshalIndent(heartbeat, "", "  ")
	contentLength := len(heartbeatJSON)

	// Formata o heartbeat como multipart
	boundary := "MIME_boundary"
	heartbeatMsg := fmt.Sprintf("\r\n--%s\r\nContent-Type: application/json; charset=\"UTF-8\"\r\nContent-Length: %d\r\n\r\n%s\r",
		boundary, contentLength, string(heartbeatJSON))

	return []byte(heartbeatMsg)
}

// handleEventStream gerencia o streaming de eventos
func (e *Emulator) handleEventStream(c *gin.Context) {
	// Inicializar contadores
	heartbeatCounter := time.Now()
	generatedEventCounter := time.Now()

	// Verificar se o cliente desconectou
	clientGone := c.Request.Context().Done()

	// Loop principal de streaming
	for {
		select {
		case <-clientGone:
			e.tracer.Info("Client disconnected from event stream")
			return
		case <-e.stopChan:
			e.tracer.Info("Event stream stopped due to emulator shutdown")
			return
		default:
			// Verificar se é hora de gerar um evento
			now := time.Now()

			if e.device.EventInterval > 0 && now.Sub(generatedEventCounter) >= time.Duration(e.device.EventInterval)*time.Second {
				e.tracer.Info(">> Sending Generated Fake Event <<")
				generatedEventCounter = now

				// Verificar se a autenticação local está ativada
				localAuth, err := e.repo.GetSetting("LocalAuthentication")
				if err != nil {
					e.tracer.Error("Failed to get LocalAuthentication setting: %v", err)
					continue
				}

				if localAuth == "1" {
					// Gerar evento
					eventData, err := e.generateRandomEvent()
					if err != nil {
						e.tracer.Error("Failed to generate random event: %v", err)
						continue
					}

					_, err = c.Writer.Write(eventData)
					if err != nil {
						e.tracer.Error("Failed to write event data: %v", err)
						return
					}
					c.Writer.Flush()
				}
			}

			// Verificar se é hora de enviar um heartbeat
			if now.Sub(heartbeatCounter) >= 10*time.Second {
				e.tracer.Info(">> Sending Heartbeat <<")
				heartbeatCounter = now

				heartbeat := e.getHeartbeatMessage()
				_, err := c.Writer.Write(heartbeat)
				if err != nil {
					e.tracer.Error("Failed to write heartbeat: %v", err)
					return
				}
				c.Writer.Flush()
			}

			// Verificar se a autenticação local está desativada
			localAuth, err := e.repo.GetSetting("LocalAuthentication")
			if err == nil && localAuth == "0" {
				e.tracer.Info("Local authentication disabled, stopping event stream")
				return
			}

			// Pequena pausa para evitar consumo excessivo de CPU
			time.Sleep(2 * time.Second)
		}
	}
}

// PhotoImg constante com imagem base64 para eventos
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
