package emulator

import (
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
	GeneratedEventCounter time.Time
}

// NewDahuaEmulator cria uma nova instância do emulador Dahua
func NewDahuaEmulator(db *database.EmulatorDB, device models.Device, tracer *trace.Tracer) *DahuaEmulator {
	tracer.Info("Initializing Dahua emulator model: %s", device.Name)

	baseEmulator := NewBaseEmulator(db, device, tracer)

	emulator := &DahuaEmulator{
		BaseEmulator:          baseEmulator,
		GeneratedEventCounter: time.Now(),
	}

	return emulator
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
			e.Tracer.Error("Failed to start server: %v", err)
		}
	}()

	// Inicia o gerador de eventos
	if e.Device.EventInterval > 0 {
		e.StartEventGenerator(e.GenerateEvent)
	}

	return nil
}

// GenerateEvent gera e envia um evento
func (e *DahuaEmulator) GenerateEvent() error {
	// Verifica se a autenticação local está ativada
	localAuth, err := e.DB.GetDeviceSettings("LocalAuthentication")
	if err != nil {
		return fmt.Errorf("failed to get LocalAuthentication setting: %w", err)
	}

	if localAuth == "0" {
		// Modo de autenticação remota, enviar para o servidor remoto
		return e.generateOnlineEvent()
	} else {
		// Modo de autenticação local, não precisa enviar eventos
		e.Tracer.Info("Local authentication mode, not generating events")
	}

	return nil
}

// generateOnlineEvent gera um evento online e o envia para o servidor remoto
func (e *DahuaEmulator) generateOnlineEvent() error {
	e.Tracer.Info("Generating online event")

	// Busca um cartão aleatório no banco de dados
	var cardInfo struct {
		UserID   int
		CardName string
		CardNo   string
	}

	query := "SELECT UserID, CardName, CardNo FROM DahuaCard ORDER BY RANDOM() LIMIT 1"
	err := e.DB.QueryRow(query).Scan(&cardInfo.UserID, &cardInfo.CardName, &cardInfo.CardNo)
	if err != nil {
		return fmt.Errorf("failed to get random card: %w", err)
	}

	// Cria o evento
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
					"UserID":       cardInfo.UserID,
					"UserType":     0,
				},
				"Index":           0,
				"PhysicalAddress": e.MacAddress,
			},
		},
		"Time": currentTime.Format("02-01-2006 15:04:05"),
	}

	// Formata o evento como multipart
	boundary := "myboundary"
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Cria o pacote de evento
	body := fmt.Sprintf("\r\n--"+boundary+"\r\nContent-Type: text/plain\r\nContent-Disposition: form-data; name=\"info\"\r\n\r\n%s\r\n--"+boundary+"--\r\n\r\n", string(eventJSON))

	// Obtém o servidor remoto
	remoteServer, err := e.DB.GetDeviceSettings("RemoteServer")
	if err != nil {
		return fmt.Errorf("failed to get RemoteServer setting: %w", err)
	}

	remotePort, err := e.DB.GetDeviceSettings("RemotePort")
	if err != nil {
		return fmt.Errorf("failed to get RemotePort setting: %w", err)
	}

	// Envia o evento para o servidor remoto
	remoteURL := fmt.Sprintf("http://%s:%s/notification", remoteServer, remotePort)
	e.Tracer.Info("Sending event to server: %s", remoteURL)

	req, err := http.NewRequest("POST", remoteURL, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send event: %w", err)
	}
	defer resp.Body.Close()

	// Verifica a resposta
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response: %s", resp.Status)
	}

	// Envia evento com foto
	e.Tracer.Info("Sending event with photo")
	event["Events"][0]["Data"].(map[string]interface{})["ImageInfo"] = []map[string]interface{}{
		{
			"Height": 640,
			"Length": 14088,
			"Offset": 0,
			"Type":   1,
			"Width":  360,
		},
	}
	event["Channel"] = 0
	event["Events"].([]map[string]interface{})[0]["Data"].(map[string]interface{})["Method"] = 4
	event["FilePath"] = "\\/mnt\\/appdata1\\/userpic\\/SnapShot\\/" + currentTime.Format("2006-01-02") + "\\/" + currentTime.Format("15") + "\\/" + currentTime.Format("04") + "\\/" + currentTime.Format("20060102150405") + ".jpg"

	// Codificar a imagem
	imageData, err := base64.StdEncoding.DecodeString(PhotoImg)
	if err != nil {
		return fmt.Errorf("failed to decode image: %w", err)
	}

	// Construir o multipart com a imagem
	eventJSON, err = json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event with image: %w", err)
	}

	// Cria o pacote de evento com imagem
	photoHeader := fmt.Sprintf("\r\n--"+boundary+"\r\nContent-Type: text/plain\r\nContent-Disposition: form-data; name=\"info\"\r\n\r\n%s\r\n--"+boundary+"\r\nContent-Type: image/jpeg\r\nContent-Disposition: form-data; name=\"file\"\r\n\r\n", string(eventJSON))
	photoFooter := fmt.Sprintf("\r\n--%s--\r\n\r\n", boundary)

	// Enviar o evento com a foto
	req, err = http.NewRequest("POST", remoteURL, strings.NewReader(photoHeader+string(imageData)+photoFooter))
	if err != nil {
		return fmt.Errorf("failed to create image request: %w", err)
	}
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)

	resp, err = client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send image event: %w", err)
	}
	defer resp.Body.Close()

	// Simula eventos de porta se necessário
	if utils.RandomAccessNotDone() {
		e.Tracer.Info("Simulating door open/close events")
		time.Sleep(2 * time.Second)
		if err := e.sendDoorEvent("Open"); err != nil {
			e.Tracer.Error("Failed to send door open event: %v", err)
		}

		time.Sleep(3 * time.Second)
		if err := e.sendDoorEvent("Close"); err != nil {
			e.Tracer.Error("Failed to send door close event: %v", err)
		}
	}

	return nil
}

// sendDoorEvent envia um evento de estado da porta
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

	// Obtém o servidor remoto
	remoteServer, err := e.DB.GetDeviceSettings("RemoteServer")
	if err != nil {
		return fmt.Errorf("failed to get RemoteServer setting: %w", err)
	}

	remotePort, err := e.DB.GetDeviceSettings("RemotePort")
	if err != nil {
		return fmt.Errorf("failed to get RemotePort setting: %w", err)
	}

	// Formata o evento como multipart
	boundary := "myboundary"
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal door event: %w", err)
	}

	// Cria o pacote de evento
	body := fmt.Sprintf("\r\n--"+boundary+"\r\nContent-Type: text/plain\r\nContent-Disposition: form-data; name=\"info\"\r\n\r\n%s\r\n--"+boundary+"--\r\n\r\n", string(eventJSON))

	// Envia o evento para o servidor remoto
	remoteURL := fmt.Sprintf("http://%s:%s/notification", remoteServer, remotePort)
	req, err := http.NewRequest("POST", remoteURL, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create door event request: %w", err)
	}
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send door event: %w", err)
	}
	defer resp.Body.Close()

	return nil
}

// GenerateRandomEvent gera um evento aleatório para streaming
func (e *DahuaEmulator) GenerateRandomEvent() ([]byte, error) {
	e.Tracer.Info("Generating random event for streaming")

	// Busca um cartão aleatório no banco de dados
	var cardInfo struct {
		CardName string
		CardNo   string
	}

	query := "SELECT CardName, CardNo FROM DahuaCard ORDER BY RANDOM() LIMIT 1"
	err := e.DB.QueryRow(query).Scan(&cardInfo.CardName, &cardInfo.CardNo)
	if err != nil {
		return nil, fmt.Errorf("failed to get random card: %w", err)
	}

	// Cria o evento
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
Events[0].UserType=0`, cardInfo.CardName, cardInfo.CardNo)

	// Cria o pacote de evento
	evtPackage := fmt.Sprintf(`

--myboundary
Content-Type: text/plain
Content-Length: %d

%s`, len(genEvt), genEvt)

	// Decodifica a imagem
	imageData, err := base64.StdEncoding.DecodeString(PhotoImg)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// Adiciona a imagem ao pacote
	dataPhoto := fmt.Sprintf("\r\n--myboundary\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", len(imageData))

	return []byte(evtPackage + dataPhoto + string(imageData)), nil
}

// setupRoutes configura as rotas do emulador Dahua
func (e *DahuaEmulator) setupRoutes(router *gin.Engine) {
	// Endpoint para verificar o status do emulador
	router.GET("/emulator/get-status", func(c *gin.Context) {
		e.HandleStatus(c.Writer, c.Request)
	})

	// Configurar rotas para global.cgi
	router.GET("/cgi-bin/global.cgi", func(c *gin.Context) {
		action := c.Query("action")
		timeParam := c.Query("time")

		e.Tracer.Info("Request to /cgi-bin/global.cgi | action=%s | time=%s", action, timeParam)

		switch action {
		case "getCurrentTime":
			currentTime := time.Now().Format("2006-01-02 15:04:05")
			c.String(http.StatusOK, fmt.Sprintf("result=%s", currentTime))
		case "setCurrentTime", "setConfig":
			c.String(http.StatusOK, "OK")
		default:
			c.String(http.StatusBadRequest, "Invalid action")
		}
	})

	// Configurar rotas para magicBox.cgi
	router.GET("/cgi-bin/magicBox.cgi", func(c *gin.Context) {
		action := c.Query("action")
		if action == "getSoftwareVersion" {
			e.Tracer.Info("Get Software Version: emulator v1.0")
			time.Sleep(80 * time.Millisecond)
			c.String(http.StatusOK, "version=Emulator v1.0")
		} else {
			c.String(http.StatusBadRequest, "Invalid action")
		}
	})

	// Configurar rotas para configManager.cgi
	router.GET("/cgi-bin/configManager.cgi", func(c *gin.Context) {
		action := c.Query("action")
		switch action {
		case "getConfig":
			name := c.Query("name")
			if strings.ToUpper(name) == "NETWORK" {
				response := fmt.Sprintf(`
table.Network.eth0.PhysicalAddress=%s
table.Network.eth0.SubnetMask=255.255.248.0`, e.MacAddress)
				c.String(http.StatusOK, response)
			}
		case "setConfig":
			e.Tracer.Info("SetConfig: %v", c.Request.URL.Query())

			remoteServer := c.Query("PictureHttpUpload.UploadServerList[0].Address")
			if remoteServer != "" {
				e.DB.SetDeviceSettings("RemoteServer", remoteServer)
			}

			remotePort := c.Query("PictureHttpUpload.UploadServerList[0].Port")
			if remotePort != "" {
				e.DB.SetDeviceSettings("RemotePort", remotePort)
			}

			enableUpload := c.Query("PictureHttpUpload.Enable")
			if enableUpload != "" {
				localAuthValue := "0"
				if enableUpload != "True" {
					localAuthValue = "1"
				}
				e.Tracer.Info("Set LocalAuthentication: %s", localAuthValue)
				e.DB.SetDeviceSettings("LocalAuthentication", localAuthValue)
			}

			c.String(http.StatusOK, "OK")
		default:
			c.String(http.StatusBadRequest, "Invalid action")
		}
	})

	// Configurar rotas para accessControl.cgi
	router.GET("/cgi-bin/accessControl.cgi", func(c *gin.Context) {
		action := c.Query("action")
		channel := c.Query("channel")

		e.Tracer.Info("Request to /cgi-bin/accessControl.cgi | action=%s | channel=%s", action, channel)

		switch action {
		case "openDoor", "closeDoor":
			time.Sleep(80 * time.Millisecond)
			c.String(http.StatusOK, "OK")
		default:
			c.String(http.StatusBadRequest, "Invalid action")
		}
	})

	// Configurar rotas para FaceInfoManager.cgi
	router.GET("/cgi-bin/FaceInfoManager.cgi", func(c *gin.Context) {
		action := c.Query("action")

		e.Tracer.Info("Request to /cgi-bin/FaceInfoManager.cgi | action=%s", action)

		switch action {
		case "startFind":
			// Obter contagem de faces
			count, err := e.DB.FindDahuaFaces()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			token := rand.Intn(30) + 1
			c.JSON(http.StatusOK, gin.H{
				"Token": token,
				"Total": count,
			})

		case "doFind":
			count, _ := strconv.Atoi(c.Query("Count"))
			offset, _ := strconv.Atoi(c.Query("Offset"))

			faces, err := e.DB.GetDahuaFaces(count, offset)
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

			c.JSON(http.StatusOK, gin.H{"Info": info})

		case "stopFind":
			c.String(http.StatusOK, "OK")

		case "remove":
			userID, _ := strconv.Atoi(c.Query("UserID"))
			if err := e.DB.RemoveDahuaFace(userID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			c.String(http.StatusOK, "OK")

		default:
			c.String(http.StatusBadRequest, "Invalid action")
		}
	})

	// POST para FaceInfoManager.cgi
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
				if err := e.DB.RemoveDahuaFace(request.UserID); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
			}

			if err := e.DB.AddDahuaFace(request.UserID, md5Hash); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			c.String(http.StatusOK, "OK")

		default:
			c.String(http.StatusBadRequest, "Invalid action")
		}
	})

	// Configurar rotas para recordFinder.cgi
	router.GET("/cgi-bin/recordFinder.cgi", func(c *gin.Context) {
		action := c.Query("action")
		name := c.Query("name")
		offset := c.Query("offset")
		count, _ := strconv.Atoi(c.Query("count"))
		userID, _ := strconv.Atoi(c.Query("condition.UserID"))

		e.Tracer.Info("Request to /cgi-bin/recordFinder.cgi | action=%s | name=%s | userID=%d", action, name, userID)

		switch action {
		case "find":
			found, cards, err := e.DB.FindDahuaCard(userID)
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
				response += fmt.Sprintf("records[%d].CardName=%s\n", i, card["CardName"])
				response += fmt.Sprintf("records[%d].CardNo=%s\n", i, card["CardNo"])
				response += fmt.Sprintf("records[%d].CardStatus=0\n", i)
				response += fmt.Sprintf("records[%d].CardType=0\n", i)
				response += fmt.Sprintf("records[%d].CitizenIDNo=\n", i)
				response += fmt.Sprintf("records[%d].Doors[0]=0\n", i)
				response += fmt.Sprintf("records[%d].DynamicCheckCode=\n", i)
				response += fmt.Sprintf("records[%d].FirstEnter=false\n", i)
				response += fmt.Sprintf("records[%d].Handicap=false\n", i)
				response += fmt.Sprintf("records[%d].IsValid=false\n", i)
				response += fmt.Sprintf("records[%d].Password=\n", i)
				response += fmt.Sprintf("records[%d].RecNo=%d\n", i, card["RecNo"])
				response += fmt.Sprintf("records[%d].RepeatEnterRouteTimeout=4294967295\n", i)
				response += fmt.Sprintf("records[%d].TimeSections[0]=1\n", i)
				response += fmt.Sprintf("records[%d].UseTime=200\n", i)
				response += fmt.Sprintf("records[%d].UserID=%d\n", i, card["UserID"])
				response += fmt.Sprintf("records[%d].UserType=0\n", i)
				response += fmt.Sprintf("records[%d].VTOPosition=\n", i)
				response += fmt.Sprintf("records[%d].ValidDateEnd=%s\n", i, card["ValidDateEnd"])
				response += fmt.Sprintf("records[%d].ValidDateStart=%s\n", i, card["ValidDateStart"])
			}

			c.String(http.StatusOK, response)

		case "doSeekFind":
			offsetInt, _ := strconv.Atoi(offset)
			found, cards, err := e.DB.GetDahuaCards(count, offsetInt)
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
				response += fmt.Sprintf("records[%d].CardName=%s\n", i, card["CardName"])
				response += fmt.Sprintf("records[%d].CardNo=%s\n", i, card["CardNo"])
				response += fmt.Sprintf("records[%d].CardStatus=0\n", i)
				response += fmt.Sprintf("records[%d].CardType=0\n", i)
				response += fmt.Sprintf("records[%d].CitizenIDNo=\n", i)
				response += fmt.Sprintf("records[%d].Doors[0]=0\n", i)
				response += fmt.Sprintf("records[%d].DynamicCheckCode=\n", i)
				response += fmt.Sprintf("records[%d].FirstEnter=false\n", i)
				response += fmt.Sprintf("records[%d].Handicap=false\n", i)
				response += fmt.Sprintf("records[%d].IsValid=false\n", i)
				response += fmt.Sprintf("records[%d].Password=\n", i)
				response += fmt.Sprintf("records[%d].RecNo=%d\n", i, card["RecNo"])
				response += fmt.Sprintf("records[%d].RepeatEnterRouteTimeout=4294967295\n", i)
				response += fmt.Sprintf("records[%d].TimeSections[0]=1\n", i)
				response += fmt.Sprintf("records[%d].UseTime=200\n", i)
				response += fmt.Sprintf("records[%d].UserID=%d\n", i, card["UserID"])
				response += fmt.Sprintf("records[%d].UserType=0\n", i)
				response += fmt.Sprintf("records[%d].VTOPosition=\n", i)
				response += fmt.Sprintf("records[%d].ValidDateEnd=%s\n", i, card["ValidDateEnd"])
				response += fmt.Sprintf("records[%d].ValidDateStart=%s\n", i, card["ValidDateStart"])
			}

			time.Sleep(350 * time.Millisecond)
			c.String(http.StatusOK, response)

		default:
			c.String(http.StatusBadRequest, "Invalid action")
		}
	})

	// Configurar rotas para recordUpdater.cgi
	router.GET("/cgi-bin/recordUpdater.cgi", func(c *gin.Context) {
		action := c.Query("action")

		e.Tracer.Info("Request to /cgi-bin/recordUpdater.cgi | action=%s", action)

		switch action {
		case "remove":
			recNo, _ := strconv.Atoi(c.Query("recno"))
			if err := e.DB.RemoveDahuaCard(recNo); err != nil {
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

			recNo, err := e.DB.AddDahuaCard(cardName, userID, cardNo, validStart, validEnd)
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

	// POST para recordUpdater.cgi
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

			recNo, err := e.DB.AddDahuaCard(cardName, userID, cardNo, validStart, validEnd)
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

	// Configurar rota para snapManager.cgi (streaming de eventos)
	router.GET("/cgi-bin/snapManager.cgi", func(c *gin.Context) {
		e.Tracer.Info("[GET] /cgi-bin/snapManager.cgi")

		// Configurar headers para streaming
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Writer.Flush()

		// Inicializar contadores
		heartbeatCounter := time.Now()
		generatedEventCounter := time.Now()

		// Verificar se o cliente desconectou
		clientGone := c.Request.Context().Done()

		// Loop principal de streaming
		for {
			select {
			case <-clientGone:
				e.Tracer.Info("Client disconnected from event stream")
				return
			case <-e.stopChan:
				e.Tracer.Info("Event stream stopped due to emulator shutdown")
				return
			default:
				// Verificar se é hora de gerar um evento
				now := time.Now()

				if e.Device.EventInterval > 0 && now.Sub(generatedEventCounter) >= time.Duration(e.Device.EventInterval)*time.Second {
					e.Tracer.Info(">> Sending Generated Fake Event <<")
					generatedEventCounter = now

					// Verificar se a autenticação local está ativada
					localAuth, err := e.DB.GetDeviceSettings("LocalAuthentication")
					if err != nil {
						e.Tracer.Error("Failed to get LocalAuthentication setting: %v", err)
						continue
					}

					if localAuth == "1" {
						// Gerar evento
						eventData, err := e.GenerateRandomEvent()
						if err != nil {
							e.Tracer.Error("Failed to generate random event: %v", err)
							continue
						}

						e.Tracer.Info("## yield event")
						_, err = c.Writer.Write(eventData)
						if err != nil {
							e.Tracer.Error("Failed to write event data: %v", err)
							return
						}
						c.Writer.Flush()
					}
				}

				// Verificar se é hora de enviar um heartbeat
				if now.Sub(heartbeatCounter) >= 10*time.Second {
					e.Tracer.Info(">> Sending Heartbeat <<")
					heartbeatCounter = now

					heartbeat := "\r\n\r\n\r\n--myboundary\r\nContent-Type: text/plain\r\nContent-Length:9\r\n\r\nHeartbeat"
					_, err := c.Writer.WriteString(heartbeat)
					if err != nil {
						e.Tracer.Error("Failed to write heartbeat: %v", err)
						return
					}
					c.Writer.Flush()
				}

				// Verificar se a autenticação local está desativada
				localAuth, err := e.DB.GetDeviceSettings("LocalAuthentication")
				if err == nil && localAuth == "0" {
					e.Tracer.Info("Local authentication disabled, stopping event stream")
					return
				}

				// Pequena pausa para evitar consumo excessivo de CPU
				time.Sleep(2 * time.Second)
			}
		}
	})
}
