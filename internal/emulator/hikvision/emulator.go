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
	startTime        *time.Time // Timestamp de quando o emulador foi iniciado

	// Configurações do servidor remoto
	remoteServer    string
	remotePort      string
	remoteServerURL string
}

// NewEmulator cria uma nova instância do emulador Hikvision
func NewEmulator(db *database.AdaptivePool, device models.Device, tracer *trace.Tracer) *Emulator {
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
	// Usar 0.0.0.0 para aceitar conexões de qualquer interface (necessário para Docker)
	addr := fmt.Sprintf("0.0.0.0:%d", e.device.Port)
	e.tracer.Info("Starting Hikvision HTTP server on %s (device: %s)", addr, e.device.IPAddress)

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

	// Registrar horário de início
	now := time.Now()
	e.startTime = &now

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

	// Buscar uma linha aleatória de usuário + cartão
	name, cardNo, employeeNo, err := e.repo.GetRandomUserAndCard()
	if err != nil {
		e.tracer.Warning("No user/card found for event generation: %v", err)
		return nil
	}

	// Obtém o fuso horário local
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	currentTime := time.Now().In(loc)

	// Criar o evento
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

	// Preencher dados do evento
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

func (e *Emulator) sendEventToRemoteServer(event *Event) error {
	// Codifica o evento em JSON
	eventJSON, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Decodifica a imagem
	imageData, err := GetPhotoImageData()
	if err != nil {
		return fmt.Errorf("failed to decode image: %w", err)
	}

	boundary := "MIME_boundary"

	// Constrói o corpo multipart corretamente
	var body strings.Builder

	// Primeira parte - JSON
	body.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	body.WriteString("Content-Type: application/json; charset=\"UTF-8\"\r\n")
	body.WriteString(fmt.Sprintf("Content-Length: %d\r\n", len(eventJSON)))
	body.WriteString("\r\n")
	body.WriteString(string(eventJSON))
	body.WriteString("\r\n")

	// Segunda parte - Imagem
	body.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	body.WriteString("Content-Disposition: form-data; name=\"Picture\"; filename=\"Picture.jpg\"\r\n")
	body.WriteString("Content-Type: image/jpeg\r\n")
	body.WriteString(fmt.Sprintf("Content-Length: %d\r\n", len(imageData)))
	body.WriteString("Content-ID: pictureImage\r\n")
	body.WriteString("\r\n")
	body.Write(imageData)
	body.WriteString("\r\n")

	// Boundary final
	body.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	// Envia o evento para o servidor remoto
	remoteURL := e.remoteServerURL + "/notification"
	e.tracer.Info("Sending event to server: %s", remoteURL)

	// Faz a requisição HTTP
	req, err := http.NewRequest("POST", remoteURL, strings.NewReader(body.String()))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", fmt.Sprintf("multipart/x-mixed-replace; boundary=%s", boundary))

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

	// Verificar se autenticação local está ativada
	localAuth, err := e.repo.GetSetting("LocalAuthentication")
	if err != nil || localAuth != "1" {
		return nil, nil // Não gerar evento se não for modo local
	}

	// Buscar uma linha aleatória de usuário + cartão
	name, cardNo, employeeNo, err := e.repo.GetRandomUserAndCard()
	if err != nil {
		return nil, fmt.Errorf("failed to get random user info: %w", err)
	}
	e.tracer.Info("generateRandomEvent.Hik: Nome=%s", name)

	// Obtém o fuso horário local
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	currentTime := time.Now().In(loc)

	// Criar evento
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

	// Preencher dados do evento de controle de acesso
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

	// Codificar o evento em JSON
	eventJSON, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event: %w", err)
	}

	imageData, err := GetPhotoImageData()
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	boundary := "MIME_boundary"

	var body strings.Builder

	// Primeira parte - JSON
	body.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	body.WriteString("Content-Type: application/json; charset=\"UTF-8\"\r\n")
	body.WriteString(fmt.Sprintf("Content-Length: %d\r\n", len(eventJSON)))
	body.WriteString("\r\n")
	body.WriteString(string(eventJSON))
	body.WriteString("\r\n")

	// Segunda parte - Imagem
	body.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	body.WriteString("Content-Disposition: form-data; name=\"Picture\"; filename=\"Picture.jpg\"\r\n")
	body.WriteString("Content-Type: image/jpeg\r\n")
	body.WriteString(fmt.Sprintf("Content-Length: %d\r\n", len(imageData)))
	body.WriteString("Content-ID: pictureImage\r\n")
	body.WriteString("\r\n")
	body.Write(imageData)
	body.WriteString("\r\n")

	// Boundary final
	body.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	return []byte(body.String()), nil
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
	e.tracer.Info("[GET] /alertStream - Starting event stream")

	// Configurar headers para streaming
	c.Writer.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=MIME_boundary")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
	c.Writer.Flush()

	// 	// Configurar headers para streaming
	// c.Writer.Header().Set("Content-Type", "text/event-stream")

	// Inicializar contadores
	heartbeatCounter := time.Now()
	generatedEventCounter := time.Now()

	// Verificar se o cliente desconectou
	clientGone := c.Request.Context().Done()

	// Loop principal de streaming - equivalente ao generate_heartbeat() do Python
	for {
		select {
		case <-clientGone:
			e.tracer.Info("Client disconnected from event stream")
			return
		case <-e.stopChan:
			e.tracer.Info("Event stream stopped due to emulator shutdown")
			return
		default:
			now := time.Now()

			// Verificar se é hora de gerar um evento
			if e.device.EventInterval > 0 && now.Sub(generatedEventCounter) >= time.Duration(e.device.EventInterval)*time.Second {
				e.tracer.Info(">> Sending Generated Fake Event <<")
				generatedEventCounter = now

				// Gerar evento
				eventData, err := e.generateRandomEvent()
				if err != nil {
					e.tracer.Error("Failed to generate random event: %v", err)
				} else if eventData != nil {
					if _, err := c.Writer.Write(eventData); err != nil {
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
				if _, err := c.Writer.Write(heartbeat); err != nil {
					e.tracer.Error("Failed to write heartbeat: %v", err)
					return
				}
				c.Writer.Flush()
			}

			// Pequena pausa para evitar consumo excessivo de CPU
			time.Sleep(2 * time.Second)
		}
	}
}

func (e *Emulator) GetTotalUsers() (int, error) {
	return e.repo.GetTotalUsers()
}

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

// PhotoImg constante com imagem base64 para eventos (mesma do Hikvision)
const PhotoImg = "/9j/4AAQSkZJRgABAQEAYABgAAD/4QAiRXhpZgAATU0AKgAAAAgAAQESAAMAAAABAAEAAAAAAAD/2wBDAAIBAQIBAQICAgICAgICAwUDAwMDAwYEBAMFBwYHBwcGBwcICQsJCAgKCAcHCg0KCgsMDAwMBwkODw0MDgsMDAz/2wBDAQICAgMDAwYDAwYMCAcIDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAz/wAARCAKuAq4DASIAAhEBAxEB/8QAHwAAAQUBAQEBAQEAAAAAAAAAAAECAwQFBgcICQoL/8QAtRAAAgEDAwIEAwUFBAQAAAF9AQIDAAQRBRIhMUEGE1FhByJxFDKBkaEII0KxwRVS0fAkM2JyggkKFhcYGRolJicoKSo0NTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqDhIWGh4iJipKTlJWWl5iZmqKjpKWmp6ipqrKztLW2t7i5usLDxMXGx8jJytLT1NXW19jZ2uHi4+Tl5ufo6erx8vP09fb3+Pn6/8QAHwEAAwEBAQEBAQEBAQAAAAAAAAECAwQFBgcICQoL/8QAtREAAgECBAQDBAcFBAQAAQJ3AAECAxEEBSExBhJBUQdhcRMiMoEIFEKRobHBCSMzUvAVYnLRChYkNOEl8RcYGRomJygpKjU2Nzg5OkNERUZHSElKU1RVVldYWVpjZGVmZ2hpanN0dXZ3eHl6goOEhYaHiImKkpOUlZaXmJmaoqOkpaanqKmqsrO0tba3uLm6wsPExcbHyMnK0tPU1dbX2Nna4uPk5ebn6Onq8vP09fb3+Pn6/9oADAMBAAIRAxEAPwD9/KKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAoooo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RQAUUUUAFcn8e/FQ8C/AzxprbEKujaFfXxJ7eVbu/wD7LXWV4j/wUs8QN4V/4J4/HHUVOGtfAmssPr9ilH9aAP5Ff+CO3hT/AIW1/wAFa/gLZXI81b/x1Y3coPfy5vPb/wBANf2rjpX8c3/BuDog8Q/8FpvgUjAMLXVLm657eXZXDD+Qr+xodKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooArazp0esaTc2koDRXUTwuD3DKQR+tfxRfsmag3wI/4K2+AjGxtz4Z+KVrbZHBVY9TEZH5ZFf2zHiv4lfjnEPBv/AAVw8UqvTTvizcOMD+7qxNAH9tKDaDn1NLTYnEkasOjDNOoAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACvnT/gruxT/AIJdftAFSQf+ED1bn/t1kr6LrwH/AIKraZ/bP/BNH49WwG4y+A9Y4+lnIf6UAfy//wDBsHEsv/Ba74P7hnadSYexGn3Ff1+qciv48/8Ag2k1VdJ/4LV/BQs237Rd3sA56lrC4AFf2GL90UALRRRQAUUUUAFFFFABRRRQAUUUUAFFFFABRRRQAUUUUAFFFFABRRRQAUUUUAFFFFABRRRQAUUUUAFFFFABRRRQAUUUUAI5wB9a/iZ/aSdfEH/BXPxp5JyLv4sXSoRz11VgK/tllcRxlmOFUZJ9BX8Svw6t2+LX/BWvRfKJb/hIvi1E6454l1cH+RoA/tqgQxwop/hUCnUgpaACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigArzb9sfwyPGX7I3xS0kr5n9peEtVtguM7i9nKAPzNek1S8SaQniDw7f2EgDR31tJbsD0IdSp/nQB/GV/wQa8SL4Q/wCCxP7Pd27FFbxbBasemPOR4f8A2ev7QhwK/iE/Zc1WT9nb/gpz4EuHzEfB3xJtIpT02CHUlRvpwDX9vQYEdc0ALRRRQAUUUUAFFFFABRRRQAUU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AAef/E7/AIRf/gnH4L8MhgH8V+NoGIzy0dtbTOf/AB5krxX/AIMhPhl5Phr4++MWTma60rRY2xz8iTzOP/H0oA/e5Pu8dKWiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKSQZQ9qWg9KAP57v+D3H4Cm18W/A/wCJsMKql5aX3hm7kC9WjZbiEE+uHmx9K+ov+DOX4+D4lf8ABMrWPBksxe5+HXiy5tlUtkrb3SLcp9BvaYD6V6z/AMHQ/wCxxq37Xf8AwSr8RN4d059U8Q/D3UIPFdvbxqWllghV0uggHUiGR3x32V+MP/Brh/wVA0j9gH9sm88HeM7uCx8B/F9YNOuL+Z9sel38bN9lnc9BGxkaNiem5T0BoA/q0oqK2nFwAwZWVlBBXkEeoqWgAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAoJxRXE/tC/H/wAK/sv/AAg17x5411i10Pwv4YtHvb+7nYABVHCLn7zscKqjlmIAoA/AX/g9l/aEi8R/Hv4PfDC1uUk/4RrR7rX7+JXyYpbqRYogw7HZAx+jV9zf8GhfwKk+FP8AwSYtfEFzbtDc/ELxJf6urMuDJBGUtoz7j9y5/Gv59v2uvjb40/4LJf8ABTzVte0rT55td+KfiCLSfD2m8ubO13LDbRHHTZEFZz0BDGv7C/2Qv2dNL/ZI/Zk8B/DTRgP7O8E6HbaTG4GBK0aAPJ9Xfcx92oA9IooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigCG+tVvLdopI45YpFKOjqGVlIwQQeoIzX8t3/AAcZ/wDBB7Wf2Gvirq/xd+GejXN78GvEt21zeW9ohkPhC5kO5onAHy2zMSY36LnaSOM/1L1R8Q+GrDxXo93p+p2ltf2F9E0FzbXEKyxTxsMMjowIZSOoIxQB/Mt/wSL/AODrHxh+xl4J0n4efGjR9R+I/gfSYlttO1i2nVdb0qFeFibe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RRRQAUUUUAFFFFABRRRQAUUUUAFFFFABRRRQAUUUUAFFFFABRRRQAUUUUAFFFFABRRRQAUUUUAFFFFABRRRQAUUUUAFFFFABRRRQAUUUUAFFFFAFLX9Dt/Eml3NheQpcWV7C9vcROMrJG6lWU+xUkV/FJ8a/C+pf8E0P+Cp+vWGnvLbXnwg8ftPZOh2sYbe8EsR/4FFt/A1/bRX8kX/B1j8NYfh5/wAFm/H01vGI08S6Vpestjjcz2yxsf8AvqI0Af1jeBPFVp468H6XrliVay1qzhv4GByHjljEin8mFa9fNX/BHH4hzfFX/glj8A9duJDLPd+CtOjkY92iiER/VK+laACiiigAooooAKKKKACiiigAPSvw3/4Pep3j/Zs+BEIbCSeJdRcj1ItYwD/48fzr9yG6Gvw1/wCD37/k3b4C/wDYxan/AOk0NAHSf8GTVnFH+w98W5lUeZL42QOe5AsocfzP51+01fi7/wAGTf8AyYp8WP8Asd1/9IYK/aKgAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACv5Yv+DxpBH/wVmsyAAX8D6bkgdf3tzX9Ttfyx/8AB49/ylksP+xH03/0dc0Afuh/wb1OZP8AgjD+z+SSSPDrDn2up6+zK+Mv+Def/lC/8AP+xef/ANKp6+zaACiiigAooooAKKKKACiiigAboa/DX/g9+/5N2+Av/Yxan/6TQ1+5TdDX4a/8Hv3/ACbt8Bf+xi1P/wBJoaAOo/4Mm/8AkxT4sf8AY7r/AOkMFftFX4u/8GTf/JinxY/7Hdf/AEhgr9oqACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAK/lj/wCDx7/lLJYf9iPpv/o65r+pyv5Y/wDg8e/5SyWH/Yj6b/6O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DRtHg8OWmkPb6xHGlx5sTyuzARuw24kGDnsa/Uv/AIgffBP/AEcB4r/8JqD/AOP1Jb/8GQngWKeMzfH3xfLCCN6J4ct1Zl7gEzEA4zg4NAH6Bf8ABvP/AMoYPgBzn/inm/8ASqevs2vPP2Uv2bfDv7H/AOzz4S+GXhGO4i8NeDNOj02xW4fzJmVclndu7MxLHtljjivQ6ACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigAooooAKKKKACiiigD/9k="
