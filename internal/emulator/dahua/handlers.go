package dahua

import (
	"bufio"
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
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
	// TODO: Add endpoint: /cgi-bin/eventManager.cgi
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
	e.tracer.Info("[CONCURRENT] Global.cgi called: action=%s | time=%s | RemoteAddr=%s", action, timeParam, c.Request.RemoteAddr)

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
	e.tracer.Info("[CONCURRENT] ConfigManager called: action=%s | RemoteAddr=%s", action, c.Request.RemoteAddr)

	switch action {
	case "getConfig":
		name := c.Query("name")
		if name == "NETWORK" || name == "network" || name == "Network" {
			deviceReturn := fmt.Sprintf("table.Network.DefaultInterface=eth0\r\n"+
				"table.Network.Domain=dahua\r\n"+
				"table.Network.Hostname=BSC\r\n"+
				"table.Network.eth0.DefaultGateway=192.168.0.1\r\n"+
				"table.Network.eth0.DhcpEnable=false\r\n"+
				"table.Network.eth0.DnsServers[0]=8.8.8.8\r\n"+
				"table.Network.eth0.DnsServers[1]=8.8.4.4\r\n"+
				"table.Network.eth0.EnableDhcpReservedIP=false\r\n"+
				"table.Network.eth0.IPAddress=192.168.0.100\r\n"+
				"table.Network.eth0.MTU=1500\r\n"+
				"table.Network.eth0.PhysicalAddress=%s\r\n"+
				"table.Network.eth0.SubnetMask=255.255.255.0\r\n"+
				"table.Network.eth2.DefaultGateway=192.168.0.1\r\n"+
				"table.Network.eth2.DhcpEnable=true\r\n"+
				"table.Network.eth2.DnsServers[0]=8.8.8.8\r\n"+
				"table.Network.eth2.DnsServers[1]=8.8.4.4\r\n"+
				"table.Network.eth2.EnableDhcpReservedIP=false\r\n"+
				"table.Network.eth2.IPAddress=192.168.0.101\r\n"+
				"table.Network.eth2.MTU=1500\r\n"+
				"table.Network.eth2.PhysicalAddress=00:00:00:00:00:00\r\n"+
				"table.Network.eth2.SubnetMask=255.255.0.0", e.macAddress)

			c.Header("Content-Type", "text/plain; charset=utf-8")
			time.Sleep(450 * time.Millisecond)
			c.String(http.StatusOK, deviceReturn)
		} else {
			e.handleResponse(c, "Unknown config name", http.StatusBadRequest, 50)
		}

	case "setConfig":
		e.tracer.Info("################# SetConfig: %+v", c.Request.URL.Query())

		// Extrair configurações do servidor remoto
		if address := c.Query("PictureHttpUpload.UploadServerList[0].Address"); address != "" {
			e.remoteServer = address
			e.repo.SetSetting("RemoteServer", address)
			e.tracer.Info("RemoteServer configured: %s", address)
		}

		if port := c.Query("PictureHttpUpload.UploadServerList[0].Port"); port != "" {
			e.remotePort = port
			e.repo.SetSetting("RemotePort", port)
			e.tracer.Info("RemotePort configured: %s", port)
		}

		e.remoteServerURL = fmt.Sprintf("http://%s:%s", e.remoteServer, e.remotePort)

		// Configurar modo de autenticação (PictureHttpUpload.Enable)
		// True/true/1 = Online mode (envia eventos para servidor remoto)
		// False/false/0 = Local mode (gera eventos locais via streaming)
		enable := c.Query("PictureHttpUpload.Enable")
		if enable != "" {
			e.tracer.Info("Processing PictureHttpUpload.Enable=%s", enable)

			enableLower := strings.ToLower(enable)
			var localAuthValue string

			// Online mode: Enable=true/True/1 -> LocalAuthentication=0
			// Local mode: Enable=false/False/0 -> LocalAuthentication=1
			if enableLower == "true" || enable == "1" {
				localAuthValue = "0" // Online authentication
				e.tracer.Info("Mode set to ONLINE (LocalAuthentication=0)")
			} else if enableLower == "false" || enable == "0" {
				localAuthValue = "1" // Local authentication
				e.tracer.Info("Mode set to LOCAL (LocalAuthentication=1)")
			} else {
				e.tracer.Warning("Unknown PictureHttpUpload.Enable value: '%s', defaulting to LOCAL mode", enable)
				localAuthValue = "1"
			}

			e.repo.SetSetting("LocalAuthentication", localAuthValue)
		}

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
		UserID flexInt `json:"UserID"`
		Info   struct {
			PhotoData flexStrings `json:"PhotoData"`
		} `json:"Info"`
	}

	if err := c.ShouldBindJSON(&requestData); err != nil {
		// Sem este log, uma face recusada por formato de payload some sem
		// deixar rastro e a tela mostra o usuario como se nao tivesse face.
		e.tracer.Error("[FACE] bind failed: action=%s CT=%q err=%v",
			action, c.Request.Header.Get("Content-Type"), err)
		e.handleResponse(c, "Invalid JSON", http.StatusBadRequest, 50)
		return
	}

	userID := int(requestData.UserID)
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
			e.tracer.Error("[FACE] AddFace failed: UserID=%d err=%v", userID, err)
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
			e.tracer.Error("[FACE] UpdateFace failed: UserID=%d err=%v", userID, err)
			e.handleResponse(c, "Error", http.StatusInternalServerError, 600)
			return
		}
		e.handleResponse(c, "OK", http.StatusOK, 600)

	default:
		e.tracer.Error("[FACE] unknown action=%q (UserID=%d, photo=%d bytes)",
			action, userID, len(photoData))
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
	e.tracer.Info("=== [GET] /cgi-bin/snapManager.cgi - CLIENT CONNECTED ===")
	e.tracer.Info("Client IP: %s", c.ClientIP())
	e.tracer.Info("User-Agent: %s", c.Request.UserAgent())
	e.tracer.Info("Remote Addr: %s", c.Request.RemoteAddr)
	e.tracer.Info("HTTP Proto: %s", c.Request.Proto)
	e.tracer.Info("Connection header: %s", c.Request.Header.Get("Connection"))

	// Hijack da conexão para evitar Transfer-Encoding: chunked
	// O dispositivo real Dahua não usa chunked encoding
	hijacker, ok := c.Writer.(http.Hijacker)
	if !ok {
		e.tracer.Error("Hijacking not supported")
		c.String(http.StatusInternalServerError, "Hijacking not supported")
		return
	}

	conn, bufrw, err := hijacker.Hijack()
	if err != nil {
		e.tracer.Error("Hijack failed: %v", err)
		c.String(http.StatusInternalServerError, "Hijack failed")
		return
	}
	defer conn.Close()

	e.tracer.Info("Connection hijacked successfully, writing raw HTTP response...")

	// Escrever resposta HTTP raw (igual ao dispositivo real Dahua)
	httpResponse := "HTTP/1.1 200 OK\r\n" +
		"Cache-Control: no-cache\r\n" +
		"Pragma: no-cache\r\n" +
		"Connection: Keep-Alive\r\n" +
		"Content-Type: multipart/x-mixed-replace; boundary=myboundary\r\n" +
		"\r\n"

	_, err = bufrw.WriteString(httpResponse)
	if err != nil {
		e.tracer.Error("Failed to write HTTP headers: %v", err)
		return
	}
	bufrw.Flush()

	e.tracer.Info("Headers sent, starting event stream handler...")
	e.handleEventStream(conn, bufrw)
	e.tracer.Info("=== Event stream handler finished ===")
}

func (e *Emulator) handleEventStream(conn net.Conn, bufrw *bufio.ReadWriter) {
	streamStartTime := time.Now()
	e.tracer.Info("=== Event Stream Started at %s ===", streamStartTime.Format("15:04:05.000"))
	e.tracer.Info("EventInterval configured: %d seconds", e.device.EventInterval)

	// Inicializar contadores
	heartbeatCounter := time.Now()
	generatedEventCounter := time.Now()
	e.tracer.Info("[DEBUG] Event counter initialized, first event will be generated in %d seconds", e.device.EventInterval)

	// Verificar configuração inicial
	localAuth, err := e.repo.GetSetting("LocalAuthentication")
	if err != nil {
		e.tracer.Warning("Failed to get LocalAuthentication setting: %v", err)
	} else {
		e.tracer.Info("LocalAuthentication setting: %s (1=local, 0=online)", localAuth)
	}

	loopCounter := 0
	// Loop principal de streaming
	for {
		loopCounter++
		if loopCounter%5 == 0 {
			e.tracer.Info("[DEBUG] Event stream loop iteration #%d - client still connected", loopCounter)
		}

		select {
		case <-e.stopChan:
			e.tracer.Info("Event stream stopped due to emulator shutdown")
			return
		default:
			now := time.Now()
			timeSinceLastEvent := now.Sub(generatedEventCounter)

			// Verificar se é hora de gerar um evento
			if e.device.EventInterval > 0 {
				if now.Sub(generatedEventCounter) >= time.Duration(e.device.EventInterval)*time.Second {
					e.tracer.Info(">> Sending Generated Fake Event <<")
					e.tracer.Info("[DEBUG] Event generation triggered after %.2f seconds", timeSinceLastEvent.Seconds())
					generatedEventCounter = now

					textPart, imagePart, err := e.generateRandomEventParts()
					if err != nil {
						e.tracer.Error("Failed to generate random event: %v", err)
					} else if textPart != nil && imagePart != nil {
						msgTextPart, err := bufrw.Write(textPart)
						if err != nil {
							e.tracer.Error("Failed to write text part: %v", err)
							return
						}
						bufrw.Flush()
						time.Sleep(2 * time.Millisecond)

						// Escrever parte da imagem
						msgImagePart, err := bufrw.Write(imagePart)
						if err != nil {
							e.tracer.Error("Failed to write image part: %v", err)
							return
						}
						bufrw.Flush()
						e.tracer.Info("Both messages sent: %d total bytes (text=%d + image=%d)", msgTextPart+msgImagePart, msgTextPart, msgImagePart)

						// Resetar contador de heartbeat
						heartbeatCounter = time.Now()
					} else {
						e.tracer.Warning("generateRandomEventParts returned nil (probably LocalAuthentication != 1)")
					}
				}
			}

			if now.Sub(heartbeatCounter) >= 10*time.Second {
				e.tracer.Info(">> Sending Heartbeat <<")
				heartbeatCounter = now

				// Heartbeat no formato do equipamento real Dahua
				heartbeatText := "Heartbeat"
				heartbeatData := fmt.Sprintf("\r\n--myboundary\r\nContent-Type: text/plain\r\nContent-Length:%d\r\n\r\n%s\r\n\r\n",
					len(heartbeatText), heartbeatText)

				n, err := bufrw.WriteString(heartbeatData)
				if err != nil {
					e.tracer.Error("Failed to write heartbeat: %v", err)
					return
				}
				bufrw.Flush()
				e.tracer.Info("[DEBUG] Heartbeat sent: %d bytes", n)
			}

			// Verificar se a autenticação local está desativada
			localAuth, err := e.repo.GetSetting("LocalAuthentication")
			if err != nil {
				e.tracer.Warning("[DEBUG] Failed to get LocalAuthentication: %v", err)
			}

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
