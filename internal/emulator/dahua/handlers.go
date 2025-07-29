package dahua

import (
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// SetupRoutes configura todas as rotas específicas do Dahua
func (e *Emulator) SetupRoutes(router *gin.Engine) {
	// Endpoint para verificar o status do emulador
	router.GET("/emulator/get-status", e.handleGetStatus)

	// ======================== Global ========================
	router.GET("/cgi-bin/global.cgi", e.handleGlobal)

	// ======================== MagicBox ========================
	router.GET("/cgi-bin/magicBox.cgi", e.handleMagicBox)

	// ======================== ConfigManager ========================
	router.GET("/cgi-bin/configManager.cgi", e.handleConfigManager)

	// ======================== AccessControl ========================
	router.GET("/cgi-bin/accessControl.cgi", e.handleAccessControl)

	// ======================== FaceInfoManager ========================
	router.GET("/cgi-bin/FaceInfoManager.cgi", e.handleFaceInfoManagerGet)
	router.POST("/cgi-bin/FaceInfoManager.cgi", e.handleFaceInfoManagerPost)

	// ======================== RecordFinder ========================
	router.GET("/cgi-bin/recordFinder.cgi", e.handleRecordFinder)

	// ======================== RecordUpdater ========================
	router.GET("/cgi-bin/recordUpdater.cgi", e.handleRecordUpdaterGet)
	router.POST("/cgi-bin/recordUpdater.cgi", e.handleRecordUpdaterPost)

	// ======================== SnapManager (Streaming) ========================
	router.GET("/cgi-bin/snapManager.cgi", e.handleSnapManager)
}

// ====================== STATUS HANDLERS ======================

func (e *Emulator) handleGetStatus(c *gin.Context) {
	e.tracer.Info("/emulator/get-status: connect")

	currentTime := time.Now().Format("2006-01-02 15:04:05")
	count, _ := e.repo.GetTotalUsers()

	c.JSON(http.StatusOK, gin.H{
		"CurrentDatetime": currentTime,
		"TotalUsers":      count,
		"Status":          e.GetStatus(),
		"Model":           "Dahua",
		"MacAddress":      e.macAddress,
		"Version":         "1.0.0",
	})
}

// ====================== GLOBAL HANDLERS ======================

func (e *Emulator) handleGlobal(c *gin.Context) {
	action := c.Query("action")
	timeParam := c.Query("time")
	e.tracer.Info("New [GET] Request at: '/cgi-bin/global.cgi' | action=%s | time=%s", action, timeParam)

	switch action {
	case "getCurrentTime":
		currentTime := time.Now().Format("2006-01-02 15:04:05")
		e.handleResponse(c, fmt.Sprintf("result=%s", currentTime), http.StatusOK, 50)
	case "setCurrentTime":
		e.handleResponse(c, "OK", http.StatusOK, 50)
	case "setConfig":
		e.handleResponse(c, "OK", http.StatusOK, 50)
	default:
		e.handleResponse(c, "Unknown action", http.StatusBadRequest, 50)
	}
}

// ====================== MAGICBOX HANDLERS ======================

func (e *Emulator) handleMagicBox(c *gin.Context) {
	action := c.Query("action")

	switch action {
	case "getSoftwareVersion":
		e.tracer.Info("Get Software Version: emulator v1.0")
		e.handleResponse(c, "version=Emulator v1.0", http.StatusOK, 80)
	default:
		e.handleResponse(c, "Unknown action", http.StatusBadRequest, 50)
	}
}

// ====================== CONFIG MANAGER HANDLERS ======================

func (e *Emulator) handleConfigManager(c *gin.Context) {
	action := c.Query("action")

	switch action {
	case "getConfig":
		name := c.Query("name")
		if name == "NETWORK" || name == "network" {
			deviceReturn := fmt.Sprintf(`table.Network.eth0.PhysicalAddress=%s
table.Network.eth0.SubnetMask=255.255.248.0`, e.macAddress)

			c.Header("Content-Type", "text/plain; charset=utf-8")
			time.Sleep(450 * time.Millisecond)
			c.String(http.StatusOK, deviceReturn)
		} else {
			e.handleResponse(c, "Unknown config name", http.StatusBadRequest, 50)
		}

	case "setConfig":
		e.tracer.Info("SetConfig: %+v", c.Request.URL.Query())

		// Extrair configurações do servidor remoto
		if address := c.Query("PictureHttpUpload.UploadServerList[0].Address"); address != "" {
			e.remoteServer = address
			e.repo.SetSetting("RemoteServer", address)
		}

		if port := c.Query("PictureHttpUpload.UploadServerList[0].Port"); port != "" {
			e.remotePort = port
			e.repo.SetSetting("RemotePort", port)
		}

		e.remoteServerURL = fmt.Sprintf("http://%s:%s", e.remoteServer, e.remotePort)

		// Configurar autenticação local
		enable := c.Query("PictureHttpUpload.Enable")
		e.tracer.Info("Set LocalAuthentication: PictureHttpUpload.Enable=%s", enable)

		localAuthValue := "1" // Padrão: local
		if enable == "True" {
			localAuthValue = "0" // Online authentication
		}
		e.repo.SetSetting("LocalAuthentication", localAuthValue)

		e.handleResponse(c, "OK", http.StatusOK, 50)

	default:
		e.handleResponse(c, "Unknown action", http.StatusBadRequest, 50)
	}
}

// ====================== ACCESS CONTROL HANDLERS ======================

func (e *Emulator) handleAccessControl(c *gin.Context) {
	action := c.Query("action")
	channel := c.Query("channel")

	switch action {
	case "openDoor":
		e.tracer.Info("Command openDoor output: %s", channel)
		e.handleResponse(c, "OK", http.StatusOK, 80)
	case "closeDoor":
		e.tracer.Info("Command closeDoor output: %s", channel)
		e.handleResponse(c, "OK", http.StatusOK, 80)
	default:
		e.handleResponse(c, "Unknown action", http.StatusBadRequest, 50)
	}
}

// ====================== FACE INFO MANAGER HANDLERS ======================

func (e *Emulator) handleFaceInfoManagerGet(c *gin.Context) {
	action := c.Query("action")

	switch action {
	case "startFind":
		time.Sleep(500 * time.Millisecond)
		response, err := e.repo.FindRemoteFaces()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, response)

	case "doFind":
		time.Sleep(50 * time.Millisecond)
		count, _ := strconv.Atoi(c.Query("Count"))
		offset, _ := strconv.Atoi(c.Query("Offset"))

		response, err := e.repo.GetRemoteFaces(count, offset)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, response)

	case "stopFind":
		e.handleResponse(c, "OK", http.StatusOK, 50)

	case "remove":
		time.Sleep(50 * time.Millisecond)
		userID, _ := strconv.Atoi(c.Query("UserID"))
		err := e.repo.RemoveFace(userID)
		if err != nil {
			e.handleResponse(c, "Error", http.StatusInternalServerError, 50)
			return
		}
		e.handleResponse(c, "OK", http.StatusOK, 50)

	default:
		e.handleResponse(c, "Invalid action", http.StatusBadRequest, 50)
	}
}

func (e *Emulator) handleFaceInfoManagerPost(c *gin.Context) {
	action := c.Query("action")

	var requestData struct {
		UserID int `json:"UserID"`
		Info   struct {
			PhotoData []string `json:"PhotoData"`
		} `json:"Info"`
	}

	if err := c.ShouldBindJSON(&requestData); err != nil {
		e.handleResponse(c, "Invalid JSON", http.StatusBadRequest, 50)
		return
	}

	userID := requestData.UserID
	var photoData string
	if len(requestData.Info.PhotoData) > 0 {
		photoData = requestData.Info.PhotoData[0]
	}

	switch action {
	case "add":
		// Calcular MD5 da imagem
		var md5Hash string
		if photoData != "" {
			imageBytes, err := base64.StdEncoding.DecodeString(photoData)
			if err == nil {
				hash := md5.Sum(imageBytes)
				md5Hash = fmt.Sprintf("%X", hash)
			}
		}

		e.tracer.Info("Add Face: UserID=%d, md5=%s", userID, md5Hash)
		err := e.repo.AddFace(userID, md5Hash)
		if err != nil {
			e.handleResponse(c, "Error", http.StatusInternalServerError, 550)
			return
		}
		e.handleResponse(c, "OK", http.StatusOK, 550)

	case "update":
		// Calcular MD5 da imagem
		var md5Hash string
		if photoData != "" {
			imageBytes, err := base64.StdEncoding.DecodeString(photoData)
			if err == nil {
				hash := md5.Sum(imageBytes)
				md5Hash = fmt.Sprintf("%X", hash)
			}
		}

		e.tracer.Info("Update Face: UserID=%d, md5=%s", userID, md5Hash)

		// Remover e adicionar novamente (como no Python)
		e.repo.RemoveFace(userID)
		err := e.repo.AddFace(userID, md5Hash)
		if err != nil {
			e.handleResponse(c, "Error", http.StatusInternalServerError, 600)
			return
		}
		e.handleResponse(c, "OK", http.StatusOK, 600)

	default:
		e.handleResponse(c, "Invalid action", http.StatusBadRequest, 50)
	}
}

// ====================== RECORD FINDER HANDLERS ======================

func (e *Emulator) handleRecordFinder(c *gin.Context) {
	action := c.Query("action")
	name := c.Query("name")
	offset := c.Query("offset")
	count, _ := strconv.Atoi(c.Query("count"))
	userID, _ := strconv.Atoi(c.Query("condition.UserID"))

	e.tracer.Info("RecordFinder: action=%s, name=%s", action, name)

	switch action {
	case "find":
		response, err := e.repo.FindCard(userID)
		if err != nil {
			e.handleResponse(c, "Error", http.StatusInternalServerError, 60)
			return
		}
		e.handleResponse(c, response, http.StatusOK, 60)

	case "doSeekFind":
		offsetInt, _ := strconv.Atoi(offset)
		response, err := e.repo.GetCards(count, offsetInt)
		if err != nil {
			e.handleResponse(c, "Error", http.StatusInternalServerError, 350)
			return
		}
		e.handleResponse(c, response, http.StatusOK, 350)

	default:
		e.handleResponse(c, "Unknown action", http.StatusBadRequest, 50)
	}
}

// ====================== RECORD UPDATER HANDLERS ======================

func (e *Emulator) handleRecordUpdaterGet(c *gin.Context) {
	e.tracer.Info("[GET] /recordUpdater.cgi: %+v", c.Request.URL.Query())
	action := c.Query("action")

	switch action {
	case "remove":
		recNo, _ := strconv.Atoi(c.Query("recno"))
		err := e.repo.RemoveCard(recNo)
		if err != nil {
			e.handleResponse(c, "Error", http.StatusInternalServerError, 350)
			return
		}
		e.handleResponse(c, "OK", http.StatusOK, 350)

	case "insert":
		e.handleCardInsert(c)

	default:
		e.handleResponse(c, "Unknown action", http.StatusBadRequest, 50)
	}
}

func (e *Emulator) handleRecordUpdaterPost(c *gin.Context) {
	e.tracer.Info("[POST] /recordUpdater.cgi: %+v", c.Request.URL.Query())
	action := c.Query("action")

	switch action {
	case "insert":
		e.handleCardInsert(c)
	default:
		e.handleResponse(c, "Unknown action", http.StatusBadRequest, 50)
	}
}

func (e *Emulator) handleCardInsert(c *gin.Context) {
	cardName := c.Query("CardName")
	userID, _ := strconv.Atoi(c.Query("UserID"))
	cardNo := c.Query("CardNo")
	validDateStart := c.Query("ValidDateStart")
	validDateEnd := c.Query("ValidDateEnd")

	e.tracer.Info("recordUpdater..insert: CardName=%s, UserID=%d, CardNo=%s", cardName, userID, cardNo)

	// Converter datas
	startTime, _ := time.Parse("2006-01-02 15:04:05", validDateStart)
	endTime, _ := time.Parse("2006-01-02 15:04:05", validDateEnd)

	recNo, err := e.repo.AddCard(cardName, userID, cardNo, startTime, endTime)
	if err != nil {
		e.handleResponse(c, "Error\nBad Request!", http.StatusBadRequest, 100)
		return
	}

	msg := fmt.Sprintf("RecNo=%d", recNo)
	e.handleResponse(c, msg, http.StatusOK, 100)
}

// ====================== SNAP MANAGER (STREAMING) ======================

func (e *Emulator) handleSnapManager(c *gin.Context) {
	e.tracer.Info("[GET] /cgi-bin/snapManager.cgi - Starting event stream")

	// Configurar headers para streaming
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
	c.Writer.Flush()

	e.handleEventStream(c)
}

// handleEventStream gerencia o streaming de eventos Dahua
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

				heartbeat := []byte("\r\n\r\n\r\n--myboundary\r\nContent-Type: text/plain\r\nContent-Length:9\r\n\r\nHeartbeat")
				if _, err := c.Writer.Write(heartbeat); err != nil {
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

// ====================== HELPER FUNCTIONS ======================

// handleResponse é um helper para respostas com latência simulada
func (e *Emulator) handleResponse(c *gin.Context, content string, statusCode int, latencySleep int) {
	c.Header("Content-Type", "text/plain; charset=utf-8")

	// Simular latência como no Python
	time.Sleep(time.Duration(latencySleep) * time.Millisecond)

	c.String(statusCode, content)
}
