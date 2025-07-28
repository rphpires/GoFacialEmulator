package emulator

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"GoFacialEmulator/internal/database"
	"GoFacialEmulator/internal/models"
	"GoFacialEmulator/internal/trace"
	"GoFacialEmulator/internal/utils"

	"github.com/gin-gonic/gin"
)

// DahuaEmulator representa o emulador para dispositivos Dahua
type DahuaEmulator struct {
	*BaseEmulator
	repo            *database.DahuaRepository
	remoteServer    string
	remotePort      string
	remoteServerURL string
}

// NewDahuaEmulator cria uma nova instância do emulador Dahua
func NewDahuaEmulator(db *database.EmulatorDB, device models.Device, tracer *trace.Tracer) *DahuaEmulator {
	tracer.Info("Initializing Dahua emulator model: %s", device.Name)

	baseEmulator := NewBaseEmulator(db, device, tracer)
	repo := database.NewDahuaRepository(db, device.ID)

	emulator := &DahuaEmulator{
		BaseEmulator: baseEmulator,
		repo:         repo,
	}

	// Inicializar configurações do servidor remoto
	emulator.initializeRemoteSettings()

	return emulator
}

// initializeRemoteSettings inicializa as configurações do servidor remoto
func (e *DahuaEmulator) initializeRemoteSettings() {
	if server, err := e.GetSetting("RemoteServer"); err == nil && server != "" {
		e.remoteServer = server
	} else {
		e.remoteServer = "localhost"
	}

	if port, err := e.GetSetting("RemotePort"); err == nil && port != "" {
		e.remotePort = port
	} else {
		e.remotePort = "15501"
	}

	e.remoteServerURL = fmt.Sprintf("http://%s:%s", e.remoteServer, e.remotePort)
	e.Tracer.Info("Remote server URL: %s", e.remoteServerURL)
}

// Start inicia o servidor do emulador
func (e *DahuaEmulator) Start() error {
	if err := e.BaseEmulator.Start(); err != nil {
		return err
	}

	// Configura o router
	router := gin.Default()
	e.setupRoutes(router)

	// Inicia o servidor HTTP
	addr := fmt.Sprintf("%s:%d", e.Device.IPAddress, e.Device.Port)
	e.Tracer.Info("Starting Dahua HTTP server on %s", addr)

	server := &http.Server{
		Addr:    addr,
		Handler: router,
	}
	e.Server = server

	// Inicia o servidor em uma goroutine
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			e.Tracer.Error("Failed to start Dahua server: %v", err)
		}
	}()

	// Inicia o gerador de eventos se configurado
	if e.Device.EventInterval > 0 {
		e.StartEventGenerator(e.GenerateEvent)
	}

	return nil
}

// GetType retorna o tipo do emulador
func (e *DahuaEmulator) GetType() string {
	return "Dahua"
}

// GenerateEvent gera e envia um evento - baseado no generate_online_event() do Python
func (e *DahuaEmulator) GenerateEvent() error {
	// Verificar modo de autenticação
	localAuth, err := e.GetSetting("LocalAuthentication")
	if err != nil {
		return fmt.Errorf("failed to get LocalAuthentication setting: %w", err)
	}

	if localAuth == "0" {
		// Modo online - enviar para servidor remoto
		return e.generateOnlineEvent()
	}

	// Modo local - eventos são gerados via streaming
	e.Tracer.Debug("Local authentication mode, events generated via streaming")
	return nil
}

// generateOnlineEvent gera um evento online - replicação do método Python
func (e *DahuaEmulator) generateOnlineEvent() error {
	e.Tracer.Info("Generating online event")

	// Buscar cartão aleatório
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var userID int
	var cardName, cardNo string

	query := "SELECT user_id, card_name, card_no FROM emulator.dahua_cards dc JOIN emulator.dahua_card_devices dcd ON dc.id = dcd.card_id WHERE dcd.device_id = $1 ORDER BY RANDOM() LIMIT 1"
	err := e.DB.QueryRow(ctx, query, e.Device.ID).Scan(&userID, &cardName, &cardNo)
	if err != nil {
		e.Tracer.Warning("No cards found for event generation: %v", err)
		return nil
	}

	// Criar evento básico
	currentTime := time.Now()
	event := map[string]interface{}{
		"Events": []map[string]interface{}{
			{
				"Action": "Pulse",
				"Code":   "AccessControl",
				"Data": map[string]interface{}{
					"CardStatus":   0,
					"CardType":     0,
					"Door":         0,
					"ErrorCode":    96,
					"EventGroupID": 0,
					"Method":       15,
					"ReadID":       "1",
					"Status":       0,
					"Type":         "Entry",
					"UTC":          currentTime.Unix(),
					"UserID":       userID,
					"UserType":     0,
				},
				"Index":           0,
				"PhysicalAddress": e.MacAddress,
			},
		},
		"Time": currentTime.Format("02-01-2006 15:04:05"),
	}

	// Enviar evento básico
	if err := e.sendEventToRemote(event, "multipart/form-data"); err != nil {
		return fmt.Errorf("failed to send basic event: %w", err)
	}

	// Criar evento com imagem
	eventWithImage := e.createEventWithImage(event, currentTime)
	if err := e.sendEventWithImageToRemote(eventWithImage); err != nil {
		return fmt.Errorf("failed to send event with image: %w", err)
	}

	// Simular eventos de porta
	if utils.RandomAccessNotDone() {
		e.simulateDoorEvents()
	}

	return nil
}

// createEventWithImage adiciona informações de imagem ao evento
func (e *DahuaEmulator) createEventWithImage(baseEvent map[string]interface{}, currentTime time.Time) map[string]interface{} {
	// Copiar evento base
	event := make(map[string]interface{})
	for k, v := range baseEvent {
		event[k] = v
	}

	// Adicionar informações de imagem
	events := event["Events"].([]map[string]interface{})
	data := events[0]["Data"].(map[string]interface{})

	data["ImageInfo"] = []map[string]interface{}{
		{
			"Height": 640,
			"Length": 14088,
			"Offset": 0,
			"Type":   1,
			"Width":  360,
		},
	}
	data["Method"] = 4

	event["Channel"] = 0
	event["FilePath"] = fmt.Sprintf("\\/mnt\\/appdata1\\/userpic\\/SnapShot\\/%s\\/%s\\/%s\\/%s.jpg",
		currentTime.Format("2006-01-02"),
		currentTime.Format("15"),
		currentTime.Format("04"),
		currentTime.Format("20060102150405"))

	return event
}

// sendEventToRemote envia evento básico para servidor remoto
func (e *DahuaEmulator) sendEventToRemote(event map[string]interface{}, contentType string) error {
	boundary := "myboundary"

	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	body := fmt.Sprintf("\r\n--%s\r\nContent-Type: text/plain\r\nContent-Disposition: form-data; name=\"info\"\r\n\r\n%s\r\n--%s--\r\n\r\n",
		boundary, string(eventJSON), boundary)

	return e.sendHTTPRequest(body, fmt.Sprintf("multipart/form-data; boundary=%s", boundary))
}

// sendEventWithImageToRemote envia evento com imagem
func (e *DahuaEmulator) sendEventWithImageToRemote(event map[string]interface{}) error {
	boundary := "myboundary"

	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event with image: %w", err)
	}

	// Decodificar imagem
	imageData, err := base64.StdEncoding.DecodeString(PhotoImg)
	if err != nil {
		return fmt.Errorf("failed to decode image: %w", err)
	}

	// Construir multipart
	photoHeader := fmt.Sprintf("\r\n--%s\r\nContent-Type: text/plain\r\nContent-Disposition: form-data; name=\"info\"\r\n\r\n%s\r\n--%s\r\nContent-Type: image/jpeg\r\nContent-Disposition: form-data; name=\"file\"\r\n\r\n",
		boundary, string(eventJSON), boundary)

	photoFooter := fmt.Sprintf("\r\n--%s--\r\n\r\n", boundary)

	body := photoHeader + string(imageData) + photoFooter

	return e.sendHTTPRequest(body, fmt.Sprintf("multipart/form-data; boundary=%s", boundary))
}

// sendHTTPRequest envia requisição HTTP para o servidor remoto
func (e *DahuaEmulator) sendHTTPRequest(body, contentType string) error {
	url := e.remoteServerURL + "/notification"

	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", contentType)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("server returned error: %s", resp.Status)
	}

	e.Tracer.Info("Event sent successfully to %s", url)
	return nil
}

// simulateDoorEvents simula eventos de porta
func (e *DahuaEmulator) simulateDoorEvents() {
	go func() {
		time.Sleep(2 * time.Second)
		if err := e.sendDoorEvent("Open"); err != nil {
			e.Tracer.Error("Failed to send door open event: %v", err)
		}

		time.Sleep(3 * time.Second)
		if err := e.sendDoorEvent("Close"); err != nil {
			e.Tracer.Error("Failed to send door close event: %v", err)
		}
	}()
}

// sendDoorEvent envia evento de porta
func (e *DahuaEmulator) sendDoorEvent(status string) error {
	currentTime := time.Now()
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
				"PhysicalAddress": e.MacAddress,
			},
		},
		"Time": currentTime.Format("02-01-2006 15:04:05"),
	}

	return e.sendEventToRemote(event, "multipart/form-data")
}

// generateRandomEvent gera evento aleatório para streaming - baseado no método Python
func (e *DahuaEmulator) generateRandomEvent() ([]byte, error) {
	e.Tracer.Info("Generating random event for streaming")

	// Buscar cartão aleatório
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var cardName, cardNo string
	query := "SELECT card_name, card_no FROM emulator.dahua_cards dc JOIN emulator.dahua_card_devices dcd ON dc.id = dcd.card_id WHERE dcd.device_id = $1 ORDER BY RANDOM() LIMIT 1"
	err := e.DB.QueryRow(ctx, query, e.Device.ID).Scan(&cardName, &cardNo)
	if err != nil {
		return nil, fmt.Errorf("no cards found: %w", err)
	}

	// Gerar evento no formato Dahua
	genEvt := fmt.Sprintf(`Events[0].Alive=100
Events[0].CardName=%s
Events[0].CardNo=%s
Events[0].CardType=0
Events[0].CreateTime=1711203293
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
Events[0].RealUTC=1711203293
Events[0].Similarity=99
Events[0].SnapPath=/var/tmp/white_part5.jpg
Events[0].Status=1
Events[0].Type=Entry
Events[0].UTC=1711203293
Events[0].UserID=29559
Events[0].UserType=0`, cardName, cardNo)

	// Criar pacote de evento
	evtPackage := fmt.Sprintf("\r\n\r\n\r\n--myboundary\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n%s", len(genEvt), genEvt)

	// Adicionar imagem
	imageData, err := base64.StdEncoding.DecodeString(PhotoImg)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	dataPhoto := fmt.Sprintf("\r\n--myboundary\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", len(imageData))

	result := []byte(evtPackage + dataPhoto)
	result = append(result, imageData...)

	return result, nil
}

// setupRoutes configura todas as rotas do emulador Dahua - baseado no código Python
func (e *DahuaEmulator) setupRoutes(router *gin.Engine) {
	// Configurar handlers de resposta com latência
	handleResponse := func(content string, statusCode int, latencySleep int) gin.HandlerFunc {
		return func(c *gin.Context) {
			if latencySleep > 0 {
				time.Sleep(time.Duration(latencySleep) * time.Millisecond)
			}
			c.Header("Content-Type", "text/plain; charset=utf-8")
			c.String(statusCode, content)
		}
	}

	// Status do emulador
	router.GET("/emulator/get-status", func(c *gin.Context) {
		e.HandleStatus(c.Writer, c.Request)
	})

	// global.cgi - Configurações globais
	router.GET("/cgi-bin/global.cgi", func(c *gin.Context) {
		action := c.Query("action")
		timeParam := c.Query("time")

		e.Tracer.Info("Request to /cgi-bin/global.cgi | action=%s | time=%s", action, timeParam)

		switch action {
		case "getCurrentTime":
			currentTime := time.Now().Format("2006-01-02 15:04:05")
			handleResponse(fmt.Sprintf("result=%s", currentTime), 200, 0)(c)
		case "setCurrentTime", "setConfig":
			handleResponse("OK", 200, 0)(c)
		default:
			c.String(http.StatusBadRequest, "Invalid action")
		}
	})

	// magicBox.cgi - Informações do software
	router.GET("/cgi-bin/magicBox.cgi", func(c *gin.Context) {
		action := c.Query("action")
		if action == "getSoftwareVersion" {
			e.Tracer.Info("Get Software Version: emulator v1.0")
			handleResponse("version=Emulator v1.0", 200, 80)(c)
		} else {
			c.String(http.StatusBadRequest, "Invalid action")
		}
	})

	// configManager.cgi - Gerenciamento de configurações
	router.GET("/cgi-bin/configManager.cgi", func(c *gin.Context) {
		action := c.Query("action")
		switch action {
		case "getConfig":
			name := c.Query("name")
			if strings.ToUpper(name) == "NETWORK" {
				response := fmt.Sprintf("table.Network.eth0.PhysicalAddress=%s\rtable.Network.eth0.SubnetMask=255.255.248.0", e.MacAddress)
				handleResponse(response, 200, 450)(c)
			}
		case "setConfig":
			e.Tracer.Info("SetConfig: %v", c.Request.URL.Query())

			// Configurar servidor remoto
			if remoteServer := c.Query("PictureHttpUpload.UploadServerList[0].Address"); remoteServer != "" {
				e.remoteServer = remoteServer
				e.SetSetting("RemoteServer", remoteServer)
			}

			if remotePort := c.Query("PictureHttpUpload.UploadServerList[0].Port"); remotePort != "" {
				e.remotePort = remotePort
				e.SetSetting("RemotePort", remotePort)
			}

			e.remoteServerURL = fmt.Sprintf("http://%s:%s", e.remoteServer, e.remotePort)

			// Configurar autenticação local
			enableUpload := c.Query("PictureHttpUpload.Enable")
			if enableUpload != "" {
				localAuthValue := "0"
				if enableUpload != "True" {
					localAuthValue = "1"
				}
				e.Tracer.Info("Set LocalAuthentication: %s", localAuthValue)
				e.SetSetting("LocalAuthentication", localAuthValue)
			}

			handleResponse("OK", 200, 0)(c)
		default:
			c.String(http.StatusBadRequest, "Invalid action")
		}
	})

	// accessControl.cgi - Controle de acesso
	router.GET("/cgi-bin/accessControl.cgi", func(c *gin.Context) {
		action := c.Query("action")
		channel := c.Query("channel")

		e.Tracer.Info("Request to /cgi-bin/accessControl.cgi | action=%s | channel=%s", action, channel)

		switch action {
		case "openDoor", "closeDoor":
			handleResponse("OK", 200, 80)(c)
		default:
			c.String(http.StatusBadRequest, "Invalid action")
		}
	})

	// FaceInfoManager.cgi - Gerenciamento de faces
	e.setupFaceRoutes(router)

	// recordFinder.cgi - Busca de registros
	e.setupRecordFinderRoutes(router)

	// recordUpdater.cgi - Atualização de registros
	e.setupRecordUpdaterRoutes(router)

	// snapManager.cgi - Streaming de eventos
	e.setupStreamingRoutes(router)
}

// setupFaceRoutes configura rotas de gerenciamento de faces
func (e *DahuaEmulator) setupFaceRoutes(router *gin.Engine) {
	router.GET("/cgi-bin/FaceInfoManager.cgi", func(c *gin.Context) {
		action := c.Query("action")

		e.Tracer.Info("Request to /cgi-bin/FaceInfoManager.cgi | action=%s", action)

		switch action {
		case "startFind":
			count, err := e.repo.FindFaces()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			token := rand.Intn(30) + 1
			time.Sleep(500 * time.Millisecond)
			c.JSON(http.StatusOK, gin.H{
				"Token": token,
				"Total": count,
			})

		case "doFind":
			count, _ := strconv.Atoi(c.Query("Count"))
			offset, _ := strconv.Atoi(c.Query("Offset"))

			faces, err := e.repo.GetFaces(count, offset)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			var info []gin.H
			for _, face := range faces {
				info = append(info, gin.H{
					"MD5":    face["MD5"],
					"UserID": face["UserID"],
				})
			}

			time.Sleep(50 * time.Millisecond)
			c.JSON(http.StatusOK, gin.H{"Info": info})

		case "stopFind":
			c.String(http.StatusOK, "OK")

		case "remove":
			userID, _ := strconv.Atoi(c.Query("UserID"))
			if err := e.repo.RemoveFace(userID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			time.Sleep(50 * time.Millisecond)
			c.String(http.StatusOK, "OK")

		default:
			c.String(http.StatusBadRequest, "Invalid action")
		}
	})

	router.POST("/cgi-bin/FaceInfoManager.cgi", func(c *gin.Context) {
		var request struct {
			UserID int `json:"UserID"`
			Info   struct {
				PhotoData []string `json:"PhotoData"`
			} `json:"Info"`
		}

		if err := c.BindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		action := c.Query("action")
		e.Tracer.Info("POST to /cgi-bin/FaceInfoManager.cgi | action=%s | UserID=%d", action, request.UserID)

		switch action {
		case "add", "update":
			if len(request.Info.PhotoData) == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "No photo data provided"})
				return
			}

			photoData := request.Info.PhotoData[0]
			photoBytes, err := base64.StdEncoding.DecodeString(photoData)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid photo data"})
				return
			}

			md5Hash := CalculateMD5(photoBytes)

			if action == "update" {
				if err := e.repo.RemoveFace(request.UserID); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
			}

			if err := e.repo.AddFace(request.UserID, md5Hash); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			latency := 550
			if action == "update" {
				latency = 600
			}
			time.Sleep(time.Duration(latency) * time.Millisecond)
			c.String(http.StatusOK, "OK")

		default:
			c.String(http.StatusBadRequest, "Invalid action")
		}
	})
}

// setupRecordFinderRoutes configura rotas de busca de registros
func (e *DahuaEmulator) setupRecordFinderRoutes(router *gin.Engine) {
	router.GET("/cgi-bin/recordFinder.cgi", func(c *gin.Context) {
		action := c.Query("action")
		name := c.Query("name")
		offset := c.Query("offset")
		count, _ := strconv.Atoi(c.Query("count"))
		userID, _ := strconv.Atoi(c.Query("condition.UserID"))

		e.Tracer.Info("Request to /cgi-bin/recordFinder.cgi | action=%s | name=%s | userID=%d", action, name, userID)

		switch action {
		case "find":
			found, cards, err := e.repo.FindCard(userID)
			if err != nil {
				c.String(http.StatusInternalServerError, "Error: "+err.Error())
				return
			}

			if found == "found=0" {
				c.String(http.StatusOK, found)
				return
			}

			response := found + "\n"
			for i, card := range cards {
				response += e.formatCardRecord(i, card)
			}

			time.Sleep(60 * time.Millisecond)
			c.String(http.StatusOK, response)

		case "doSeekFind":
			offsetInt, _ := strconv.Atoi(offset)
			found, cards, err := e.repo.GetCards(count, offsetInt)
			if err != nil {
				c.String(http.StatusInternalServerError, "Error: "+err.Error())
				return
			}

			if found == "found=0" {
				c.String(http.StatusOK, found)
				return
			}

			response := found + "\n"
			for i, card := range cards {
				response += e.formatCardRecord(i, card)
			}

			time.Sleep(350 * time.Millisecond)
			c.String(http.StatusOK, response)

		default:
			c.String(http.StatusBadRequest, "Invalid action")
		}
	})
}

// setupRecordUpdaterRoutes configura rotas de atualização de registros
func (e *DahuaEmulator) setupRecordUpdaterRoutes(router *gin.Engine) {
	router.GET("/cgi-bin/recordUpdater.cgi", func(c *gin.Context) {
		action := c.Query("action")

		e.Tracer.Info("Request to /cgi-bin/recordUpdater.cgi | action=%s", action)

		switch action {
		case "remove":
			recNo, _ := strconv.Atoi(c.Query("recno"))
			if err := e.repo.RemoveCard(recNo); err != nil {
				c.String(http.StatusInternalServerError, "Error: "+err.Error())
				return
			}

			time.Sleep(350 * time.Millisecond)
			c.String(http.StatusOK, "OK")

		case "insert":
			cardName := c.Query("CardName")
			userID, _ := strconv.Atoi(c.Query("UserID"))
			cardNo := c.Query("CardNo")
			validStart := c.Query("ValidDateStart")
			validEnd := c.Query("ValidDateEnd")

			e.Tracer.Info("Insert card: CardName=%s, UserID=%d, CardNo=%s", cardName, userID, cardNo)

			recNo, err := e.repo.AddCard(cardName, userID, cardNo, validStart, validEnd)
			if err != nil {
				c.String(http.StatusBadRequest, "Error\nBad Request!")
				return
			}

			time.Sleep(100 * time.Millisecond)
			c.String(http.StatusOK, fmt.Sprintf("RecNo=%d", recNo))

		default:
			c.String(http.StatusBadRequest, "Invalid action")
		}
	})

	router.POST("/cgi-bin/recordUpdater.cgi", func(c *gin.Context) {
		action := c.Query("action")

		e.Tracer.Info("POST to /cgi-bin/recordUpdater.cgi | action=%s", action)

		if action == "insert" {
			cardName := c.Query("CardName")
			userID, _ := strconv.Atoi(c.Query("UserID"))
			cardNo := c.Query("CardNo")
			validStart := c.Query("ValidDateStart")
			validEnd := c.Query("ValidDateEnd")

			e.Tracer.Info("Insert card: CardName=%s, UserID=%d, CardNo=%s", cardName, userID, cardNo)

			recNo, err := e.repo.AddCard(cardName, userID, cardNo, validStart, validEnd)
			if err != nil {
				c.String(http.StatusBadRequest, "Error\nBad Request!")
				return
			}

			time.Sleep(100 * time.Millisecond)
			c.String(http.StatusOK, fmt.Sprintf("RecNo=%d", recNo))
		} else {
			c.String(http.StatusBadRequest, "Invalid action")
		}
	})
}

// setupStreamingRoutes configura rotas de streaming de eventos
func (e *DahuaEmulator) setupStreamingRoutes(router *gin.Engine) {
	router.GET("/cgi-bin/snapManager.cgi", func(c *gin.Context) {
		e.Tracer.Info("[GET] /cgi-bin/snapManager.cgi")

		// Configurar headers para streaming
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")

		c.Stream(func(w gin.ResponseWriter) bool {
			return e.handleEventStream(w)
		})
	})
}

// handleEventStream gerencia o streaming de eventos
func (e *DahuaEmulator) handleEventStream(w gin.ResponseWriter) bool {
	heartbeatCounter := time.Now()
	generatedEventCounter := time.Now()

	for {
		select {
		case <-e.stopChan:
			return false
		default:
			now := time.Now()

			// Verificar se é hora de gerar evento
			if e.Device.EventInterval > 0 && now.Sub(generatedEventCounter) >= time.Duration(e.Device.EventInterval)*time.Second {
				e.Tracer.Info(">> Sending Generated Fake Event <<")
				generatedEventCounter = now

				localAuth, err := e.GetSetting("LocalAuthentication")
				if err == nil && localAuth == "1" {
					eventData, err := e.generateRandomEvent()
					if err != nil {
						e.Tracer.Error("Failed to generate random event: %v", err)
						continue
					}

					e.Tracer.Info("## yield event")
					w.Write(eventData)
					w.Flush()
				}
			}

			// Verificar se é hora de enviar heartbeat
			if now.Sub(heartbeatCounter) >= 10*time.Second {
				e.Tracer.Info(">> Sending Heartbeat <<")
				heartbeatCounter = now

				heartbeat := "\r\n\r\n\r\n--myboundary\r\nContent-Type: text/plain\r\nContent-Length:9\r\n\r\nHeartbeat"
				w.WriteString(heartbeat)
				w.Flush()
			}

			// Verificar se autenticação local está desativada
			localAuth, err := e.GetSetting("LocalAuthentication")
			if err == nil && localAuth == "0" {
				e.Tracer.Info("Local authentication disabled, stopping event stream")
				return false
			}

			time.Sleep(2 * time.Second)
		}
	}
}

// formatCardRecord formata um registro de cartão para resposta
func (e *DahuaEmulator) formatCardRecord(index int, card map[string]interface{}) string {
	return fmt.Sprintf(`records[%d].CardName=%s
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
records[%d].RecNo=%v
records[%d].RepeatEnterRouteTimeout=4294967295
records[%d].TimeSections[0]=1
records[%d].UseTime=200
records[%d].UserID=%v
records[%d].UserType=0
records[%d].VTOPosition=
records[%d].ValidDateEnd=%s
records[%d].ValidDateStart=%s
`,
		index, card["CardName"],
		index, card["CardNo"],
		index,
		index,
		index,
		index,
		index,
		index,
		index,
		index,
		index, card["RecNo"],
		index,
		index,
		index,
		index, card["UserID"],
		index,
		index,
		index, card["ValidDateEnd"],
		index, card["ValidDateStart"])
}
