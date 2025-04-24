package emulator

import (
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"GoFacialEmulator/internal/models"
	"GoFacialEmulator/internal/trace"
	"GoFacialEmulator/internal/utils"

	"GoFacialEmulator/internal/database"

	"github.com/gin-gonic/gin"
)

// XMLResponseStatus representa a estrutura de resposta XML padrão do Hikvision
type XMLResponseStatus struct {
	XMLName       xml.Name `xml:"ResponseStatus"`
	Version       string   `xml:"version,attr"`
	Xmlns         string   `xml:"xmlns,attr"`
	RequestURL    string   `xml:"requestURL"`
	StatusCode    string   `xml:"statusCode"`
	StatusString  string   `xml:"statusString"`
	SubStatusCode string   `xml:"subStatusCode"`
}

// HikvisionEmulator representa o emulador para dispositivos Hikvision
type HikvisionEmulator struct {
	*BaseEmulator
	DeleteInProgress bool
}

// NewHikvisionEmulator cria uma nova instância do emulador Hikvision
func NewHikvisionEmulator(db *database.EmulatorDB, device models.Device, tracer *trace.Tracer) *HikvisionEmulator {
	tracer.Info("Initializing Hikvision emulator model: %s", device.Name)

	baseEmulator := NewBaseEmulator(db, device, tracer)

	emulator := &HikvisionEmulator{
		BaseEmulator:     baseEmulator,
		DeleteInProgress: false,
	}

	return emulator
}

// Start inicia o servidor do emulador
func (e *HikvisionEmulator) Start() error {
	if err := e.BaseEmulator.Start(); err != nil {
		return err
	}

	// Configura o router
	router := gin.Default()
	e.setupRoutes(router)

	// Inicia o servidor HTTP
	addr := fmt.Sprintf("%s:%d", e.Device.IPAddress, e.Device.Port)
	e.Tracer.Info("Starting Hikvision HTTP server on %s", addr)

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
func (e *HikvisionEmulator) GenerateEvent() error {
	// Verifica se a autenticação local está ativada
	localAuth, err := e.GetDeviceSetting("LocalAuthentication")
	if err != nil {
		return fmt.Errorf("failed to get LocalAuthentication setting: %w", err)
	}

	if localAuth == "0" {
		// Modo de autenticação remota, enviar para o servidor remoto
		return e.generateOnlineEvent()
	}

	return nil
}

// generateOnlineEvent gera um evento online
func (e *HikvisionEmulator) generateOnlineEvent() error {
	e.Tracer.Info("Generating online event")

	// Busca uma linha aleatória de usuário + cartão
	var userInfo struct {
		Name       string
		CardNo     string
		EmployeeNo string
	}

	query := `
SELECT 
    u.name, c.cardNo, u.employeeNo 
FROM 
    hikvisionUser u
JOIN 
    HikvisionCard c 
ON 
    c.employeeNo = u.employeeNo
ORDER BY 
    RANDOM() 
LIMIT 1
`
	row := e.BaseEmulator.QueryRow(query, nil, &userInfo.Name, &userInfo.CardNo, &userInfo.EmployeeNo)
	if row != nil {
		return fmt.Errorf("failed to get random user info: %w", row)
	}

	// Obtém o fuso horário local
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	currentTime := time.Now().In(loc)

	// Cria o evento
	event := map[string]interface{}{
		"ipAddress":        e.Device.IPAddress,
		"ipv6Address":      "fe80::be5e:33ff:fe57:a5cb",
		"portNo":           e.Device.Port,
		"protocol":         "HTTP",
		"macAddress":       e.MacAddress,
		"channelID":        1,
		"dateTime":         currentTime.Format("2006-01-02T15:04:05-03:00"),
		"activePostCount":  1,
		"eventType":        "AccessControllerEvent",
		"eventState":       "active",
		"eventDescription": "Access Controller Event",
		"AccessControllerEvent": map[string]interface{}{
			"deviceName":          "subdoorOne",
			"majorEventType":      5,
			"subEventType":        75,
			"cardNo":              userInfo.CardNo,
			"cardType":            1,
			"name":                userInfo.Name,
			"cardReaderKind":      1,
			"cardReaderNo":        1,
			"verifyNo":            189,
			"employeeNoString":    userInfo.EmployeeNo,
			"serialNo":            4435,
			"userType":            "normal",
			"currentVerifyMode":   "faceOrFpOrCardOrPw",
			"currentEvent":        true,
			"frontSerialNo":       4434,
			"attendanceStatus":    "undefined",
			"label":               "",
			"statusValue":         0,
			"mask":                "no",
			"helmet":              "unknown",
			"picturesNumber":      1,
			"purePwdVerifyEnable": true,
			"FaceRect": map[string]interface{}{
				"height": 0.268,
				"width":  0.477,
				"x":      0.286,
				"y":      0.354,
			},
			"unlockRoomNo": "3723243075",
		},
	}

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

	// Obtém o servidor remoto
	remoteServer, err := e.GetDeviceSetting("RemoteServer")
	if err != nil {
		return fmt.Errorf("failed to get RemoteServer setting: %w", err)
	}

	remotePort, err := e.GetDeviceSetting("RemotePort")
	if err != nil {
		return fmt.Errorf("failed to get RemotePort setting: %w", err)
	}

	// Envia o evento para o servidor remoto
	remoteURL := fmt.Sprintf("http://%s:%s/notification", remoteServer, remotePort)
	e.Tracer.Info("Sending event to server: %s", remoteURL)

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
		return fmt.Errorf("failed to send event: %w", err)
	}
	defer resp.Body.Close()

	// Verifica a resposta
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response: %s", resp.Status)
	}

	// Se for necessário simular eventos de porta (aleatoriamente)
	if utils.RandomAccessNotDone() {
		e.Tracer.Info("Sending door events to complete access")
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
func (e *HikvisionEmulator) sendDoorEvent(status string) error {
	// Obtém o servidor remoto
	remoteServer, err := e.GetDeviceSetting("RemoteServer")
	if err != nil {
		return fmt.Errorf("failed to get RemoteServer setting: %w", err)
	}

	remotePort, err := e.GetDeviceSetting("RemotePort")
	if err != nil {
		return fmt.Errorf("failed to get RemotePort setting: %w", err)
	}

	// Cria o evento
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
				"PhysicalAddress": e.MacAddress,
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
	remoteURL := fmt.Sprintf("http://%s:%s/notification", remoteServer, remotePort)
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
func (e *HikvisionEmulator) generateRandomEvent() ([]byte, error) {
	e.Tracer.Info("Generating random event for streaming")

	// Busca uma linha aleatória de usuário + cartão
	var userInfo struct {
		Name       string
		CardNo     string
		EmployeeNo string
	}

	query := `
SELECT 
    u.name, c.cardNo, u.employeeNo 
FROM 
    hikvisionUser u
JOIN 
    HikvisionCard c 
ON 
    c.employeeNo = u.employeeNo
ORDER BY 
    RANDOM() 
LIMIT 1
`
	err := e.BaseEmulator.QueryRow(query, nil, &userInfo.Name, &userInfo.CardNo, &userInfo.EmployeeNo)
	if err != nil {
		return nil, fmt.Errorf("failed to get random user info: %w", err)
	}

	// Obtém o fuso horário local
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	currentTime := time.Now().In(loc)

	// Cria o evento
	event := map[string]interface{}{
		"ipAddress":        e.Device.IPAddress,
		"ipv6Address":      "fe80::be5e:33ff:fe57:a5cb",
		"portNo":           e.Device.Port,
		"protocol":         "HTTP",
		"macAddress":       e.MacAddress,
		"channelID":        1,
		"dateTime":         currentTime.Format("2006-01-02T15:04:05-03:00"),
		"activePostCount":  1,
		"eventType":        "AccessControllerEvent",
		"eventState":       "active",
		"eventDescription": "Access Controller Event",
		"AccessControllerEvent": map[string]interface{}{
			"deviceName":          "subdoorOne",
			"majorEventType":      5,
			"subEventType":        75,
			"cardNo":              userInfo.CardNo,
			"cardType":            1,
			"name":                userInfo.Name,
			"cardReaderKind":      1,
			"cardReaderNo":        1,
			"verifyNo":            189,
			"employeeNoString":    userInfo.EmployeeNo,
			"serialNo":            4435,
			"userType":            "normal",
			"currentVerifyMode":   "faceOrFpOrCardOrPw",
			"currentEvent":        true,
			"frontSerialNo":       4434,
			"attendanceStatus":    "undefined",
			"label":               "",
			"statusValue":         0,
			"mask":                "no",
			"helmet":              "unknown",
			"picturesNumber":      1,
			"purePwdVerifyEnable": true,
			"FaceRect": map[string]interface{}{
				"height": 0.268,
				"width":  0.477,
				"x":      0.286,
				"y":      0.354,
			},
			"unlockRoomNo": "3723243075",
		},
	}

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
func (e *HikvisionEmulator) getHeartbeatMessage() []byte {
	// Cria a mensagem de heartbeat
	heartbeat := map[string]interface{}{
		"ipAddress":        e.Device.IPAddress,
		"portNo":           e.Device.Port,
		"protocol":         "HTTP",
		"macAddress":       e.MacAddress,
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

// countItems retorna contagens de diferentes tipos de itens
func (e *HikvisionEmulator) countItems() (int, int, int, int, error) {
	var usersCount, cardsCount, facesCount, fingerprintsCount int

	err := e.BaseEmulator.QueryRow("SELECT COUNT(*) FROM HikvisionUser", nil, &usersCount)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	err = e.BaseEmulator.QueryRow("SELECT COUNT(*) FROM HikvisionCard", nil, &cardsCount)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	err = e.BaseEmulator.QueryRow("SELECT COUNT(*) FROM HikvisionFace", nil, &facesCount)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	err = e.BaseEmulator.QueryRow("SELECT COUNT(*) FROM HikvisionFinger", nil, &fingerprintsCount)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	return usersCount, cardsCount, facesCount, fingerprintsCount, nil
}

// setupRoutes configura as rotas do emulador Hikvision
func (e *HikvisionEmulator) setupRoutes(router *gin.Engine) {
	// Endpoint para verificar o status do emulador
	router.GET("/emulator/get-status", func(c *gin.Context) {
		e.HandleStatus(c.Writer, c.Request)
	})

	// ------------------------ AccessControl ------------------------
	acURL := "/ISAPI/AccessControl"

	// AcsCfg
	router.GET(acURL+"/AcsCfg", func(c *gin.Context) {
		localAuth, _ := e.GetDeviceSetting("LocalAuthentication")
		remoteCheckDoorEnabled := localAuth == "1"

		c.JSON(http.StatusOK, gin.H{
			"AcsCfg": gin.H{
				"uploadCapPic":                        true,
				"saveCapPic":                          true,
				"protocol":                            "Private",
				"voicePrompt":                         false,
				"showPicture":                         false,
				"showEmployeeNo":                      false,
				"showName":                            true,
				"desensitiseEmployeeNo":               true,
				"desensitiseName":                     true,
				"uploadVerificationPic":               true,
				"saveVerificationPic":                 true,
				"saveFacePic":                         false,
				"remoteCheckDoorEnabled":              remoteCheckDoorEnabled,
				"remoteStandaloneEnabled":             true,
				"remoteCheckSet":                      0,
				"checkChannelType":                    "ISAPIListen",
				"externalCardReaderEnabled":           false,
				"combinationAuthenticationTimeout":    5,
				"combinationAuthenticationLimitOrder": true,
			},
		})
	})

	router.PUT(acURL+"/AcsCfg", func(c *gin.Context) {
		var payload struct {
			AcsCfg struct {
				RemoteCheckDoorEnabled bool `json:"remoteCheckDoorEnabled"`
			} `json:"AcsCfg"`
		}

		if err := c.BindJSON(&payload); err != nil {
			c.XML(http.StatusBadRequest, errorXMLResponse("Invalid request"))
			return
		}

		e.Tracer.Info("Setting online mode: %v", payload.AcsCfg.RemoteCheckDoorEnabled)
		localAuthValue := "0"
		if !payload.AcsCfg.RemoteCheckDoorEnabled {
			localAuthValue = "1"
		}

		if err := e.SetDeviceSetting("LocalAuthentication", localAuthValue); err != nil {
			c.XML(http.StatusInternalServerError, errorXMLResponse(err.Error()))
			return
		}

		c.XML(http.StatusOK, successXMLResponse())
	})

	router.PUT(acURL+"/AcsEvent/StorageCfg", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"statusCode":    1,
			"statusString":  "OK",
			"subStatusCode": "ok",
		})
	})

	router.PUT(acURL+"/Door/param/1", func(c *gin.Context) {
		c.XML(http.StatusOK, successXMLResponse())
	})

	router.PUT(acURL+"/RemoteControl/door/:output_id", func(c *gin.Context) {
		outputID := c.Param("output_id")
		e.Tracer.Info("New command received to output= %s", outputID)
		c.XML(http.StatusOK, successXMLResponse())
	})

	// UserInfo
	router.GET(acURL+"/UserInfo/Count", func(c *gin.Context) {
		usersCount, cardsCount, facesCount, fingerprintsCount, err := e.countItems()
		if err != nil {
			c.XML(http.StatusInternalServerError, errorXMLResponse(err.Error()))
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"UserInfoCount": gin.H{
				"userNumber":                usersCount,
				"bindFaceUserNumber":        facesCount,
				"bindFingerprintUserNumber": fingerprintsCount,
				"bindCardUserNumber":        cardsCount,
				"bindRemoteControlNumber":   0,
			},
		})
	})

	router.POST(acURL+"/UserInfo/Search", func(c *gin.Context) {
		var payload struct {
			UserInfoSearchCond struct {
				MaxResults           int `json:"maxResults"`
				SearchResultPosition int `json:"searchResultPosition"`
			} `json:"UserInfoSearchCond"`
		}

		if err := c.BindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Obter usuários do banco de dados
		rows, err := e.Query("SELECT * FROM HikvisionUser LIMIT ? OFFSET ?",
			payload.UserInfoSearchCond.MaxResults,
			payload.UserInfoSearchCond.SearchResultPosition)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer rows.Close()

		// Preparar resposta
		var users []gin.H
		noOfMatches := 0

		for rows.Next() {
			var employeeNo, name, password, localUIRight, beginTime, endTime string
			if err := rows.Scan(&employeeNo, &name, &password, &localUIRight, &beginTime, &endTime); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			localUIRightBool := localUIRight != "" && localUIRight != "0"

			users = append(users, gin.H{
				"employeeNo":         employeeNo,
				"name":               name,
				"userType":           "normal",
				"sortByNamePosition": 0,
				"sortByNameFlag":     "#",
				"closeDelayEnabled":  false,
				"Valid": gin.H{
					"enable":    true,
					"beginTime": beginTime,
					"endTime":   endTime,
					"timeType":  "local",
				},
				"belongGroup": "",
				"password":    password,
				"doorRight":   "1",
				"RightPlan": []gin.H{
					{
						"doorNo":         1,
						"planTemplateNo": "1",
					},
				},
				"maxOpenDoorTime":    0,
				"openDoorTime":       0,
				"roomNumber":         0,
				"floorNumber":        0,
				"localUIRight":       localUIRightBool,
				"gender":             "unknown",
				"numOfCard":          1,
				"numOfRemoteControl": 0,
				"numOfFP":            0,
				"numOfFace":          0,
				"PersonInfoExtends": []gin.H{
					{"value": ""},
				},
			})
			noOfMatches++
		}

		if err := rows.Err(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Obter contagem total de usuários
		var totalUsers int
		err = e.BaseEmulator.QueryRow("SELECT COUNT(*) FROM HikvisionUser", nil, &totalUsers)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"UserInfoSearch": gin.H{
				"searchID":           "1",
				"responseStatusStrg": "MORE",
				"numOfMatches":       noOfMatches,
				"totalMatches":       totalUsers,
				"UserInfo":           users,
			},
		})
	})

	router.POST(acURL+"/UserInfo/Record", func(c *gin.Context) {
		var payload struct {
			UserInfo struct {
				EmployeeNo   string `json:"employeeNo"`
				Name         string `json:"name"`
				Password     string `json:"password"`
				LocalUIRight bool   `json:"localUIRight"`
				Valid        struct {
					BeginTime string `json:"beginTime"`
					EndTime   string `json:"endTime"`
				} `json:"Valid"`
			} `json:"UserInfo"`
		}

		if err := c.BindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Verificar se o usuário já existe
		var exists int
		err := e.BaseEmulator.QueryRow("SELECT COUNT(*) FROM HikvisionUser WHERE employeeNo = ?",
			[]interface{}{payload.UserInfo.EmployeeNo},
			&exists)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		if exists > 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"statusCode":    6,
				"statusString":  "Invalid Content",
				"subStatusCode": "employeeNoAlreadyExist",
				"errorCode":     1610637344,
				"errorMsg":      "checkUser",
			})
			return
		}

		// Converter boolean para string para armazenamento
		localUIRight := "0"
		if payload.UserInfo.LocalUIRight {
			localUIRight = "1"
		}

		// Inserir o usuário
		_, err = e.BaseEmulator.Exec(
			"INSERT INTO HikvisionUser VALUES (?, ?, ?, ?, ?, ?)",
			payload.UserInfo.EmployeeNo,
			payload.UserInfo.Name,
			payload.UserInfo.Password,
			localUIRight,
			payload.UserInfo.Valid.BeginTime,
			payload.UserInfo.Valid.EndTime,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"statusCode":    1,
			"statusString":  "OK",
			"subStatusCode": "ok",
		})
	})

	router.PUT(acURL+"/UserInfo/Modify", func(c *gin.Context) {
		var payload struct {
			UserInfo struct {
				EmployeeNo   string `json:"employeeNo"`
				Name         string `json:"name"`
				Password     string `json:"password"`
				LocalUIRight bool   `json:"localUIRight"`
				Valid        struct {
					BeginTime string `json:"beginTime"`
					EndTime   string `json:"endTime"`
				} `json:"Valid"`
			} `json:"UserInfo"`
		}

		if err := c.BindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Converter boolean para string para armazenamento
		localUIRight := "0"
		if payload.UserInfo.LocalUIRight {
			localUIRight = "1"
		}

		// Atualizar o usuário
		_, err := e.BaseEmulator.Exec(
			"UPDATE HikvisionUser SET name = ?, password = ?, localUIRight = ?, beginTime = ?, endTime = ? WHERE employeeNo = ?",
			payload.UserInfo.Name,
			payload.UserInfo.Password,
			localUIRight,
			payload.UserInfo.Valid.BeginTime,
			payload.UserInfo.Valid.EndTime,
			payload.UserInfo.EmployeeNo,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"statusCode":    1,
			"statusString":  "OK",
			"subStatusCode": "ok",
		})
	})

	// UserInfoDetail
	router.GET(acURL+"/UserInfoDetail/DeleteProcess", func(c *gin.Context) {
		status := "success"
		if e.DeleteInProgress {
			status = "inProgress"
		}

		c.JSON(http.StatusOK, gin.H{
			"UserInfoDetailDeleteProcess": gin.H{
				"status": status,
			},
		})
	})

	router.PUT(acURL+"/UserInfoDetail/Delete", func(c *gin.Context) {
		var payload struct {
			UserInfoDetail struct {
				Mode           string `json:"mode"`
				EmployeeNoList []struct {
					EmployeeNo string `json:"employeeNo"`
				} `json:"EmployeeNoList"`
			} `json:"UserInfoDetail"`
		}

		if err := c.BindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if payload.UserInfoDetail.Mode != "byEmployeeNo" {
			c.JSON(http.StatusBadRequest, gin.H{
				"statusCode":    77,
				"statusString":  "Error",
				"subStatusCode": "Emulator Invalid Mode",
			})
			return
		}

		// Marcar início do processo de exclusão
		e.DeleteInProgress = true
		defer func() { e.DeleteInProgress = false }()

		// Excluir usuários
		for _, emp := range payload.UserInfoDetail.EmployeeNoList {
			e.Tracer.Info("Deleting user with EmployeeNo= %s", emp.EmployeeNo)

			// Iniciar transação
			tx, err := e.BaseEmulator.Begin()
			if err != nil {
				e.Tracer.Error("Failed to begin transaction: %v", err)
				continue
			}

			// Excluir usuário e dados relacionados
			_, err = tx.Exec("DELETE FROM HikvisionUser WHERE employeeNo = ?", emp.EmployeeNo)
			if err != nil {
				tx.Rollback()
				e.Tracer.Error("Failed to delete user: %v", err)
				continue
			}

			_, err = tx.Exec("DELETE FROM HikvisionCard WHERE employeeNo = ?", emp.EmployeeNo)
			if err != nil {
				tx.Rollback()
				e.Tracer.Error("Failed to delete card: %v", err)
				continue
			}

			_, err = tx.Exec("DELETE FROM HikvisionFace WHERE UserID = ?", emp.EmployeeNo)
			if err != nil {
				tx.Rollback()
				e.Tracer.Error("Failed to delete face: %v", err)
				continue
			}

			_, err = tx.Exec("DELETE FROM HikvisionFinger WHERE CHID = ?", emp.EmployeeNo)
			if err != nil {
				tx.Rollback()
				e.Tracer.Error("Failed to delete finger: %v", err)
				continue
			}

			// Confirmar transação
			if err := tx.Commit(); err != nil {
				e.Tracer.Error("Failed to commit transaction: %v", err)
				continue
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"statusCode":    1,
			"statusString":  "OK",
			"subStatusCode": "ok",
		})
	})

	// CardInfo
	router.GET(acURL+"/CardInfo/Count", func(c *gin.Context) {
		var cardCount int
		err := e.BaseEmulator.QueryRow("SELECT COUNT(*) FROM HikvisionCard", nil, &cardCount)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"CardInfoCount": gin.H{
				"cardNumber": cardCount,
			},
		})
	})

	router.POST(acURL+"/CardInfo/Search", func(c *gin.Context) {
		var payload struct {
			CardInfoSearchCond struct {
				MaxResults           int `json:"maxResults"`
				SearchResultPosition int `json:"searchResultPosition"`
			} `json:"CardInfoSearchCond"`
		}

		if err := c.BindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Obter cartões do banco de dados
		rows, err := e.BaseEmulator.Query("SELECT * FROM HikvisionCard LIMIT ? OFFSET ?",
			payload.CardInfoSearchCond.MaxResults,
			payload.CardInfoSearchCond.SearchResultPosition)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer rows.Close()

		// Preparar resposta
		var cards []gin.H
		noOfMatches := 0

		for rows.Next() {
			var employeeNo, cardNo string
			if err := rows.Scan(&employeeNo, &cardNo); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			cards = append(cards, gin.H{
				"employeeNo":               employeeNo,
				"cardNo":                   cardNo,
				"isCardAsRemoteControlBtn": false,
				"leaderCard":               "",
				"cardType":                 "normalCard",
			})
			noOfMatches++
		}

		if err := rows.Err(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Obter contagem total de cartões
		var totalCards int
		err = e.BaseEmulator.QueryRow("SELECT COUNT(*) FROM HikvisionCard", nil, &totalCards)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"CardInfoSearch": gin.H{
				"searchID":           "1",
				"responseStatusStrg": "MORE",
				"numOfMatches":       noOfMatches,
				"totalMatches":       totalCards,
				"CardInfo":           cards,
			},
		})
	})

	router.POST(acURL+"/CardInfo/Record", func(c *gin.Context) {
		var payload struct {
			CardInfo struct {
				EmployeeNo string `json:"employeeNo"`
				CardNo     string `json:"cardNo"`
			} `json:"CardInfo"`
		}

		if err := c.BindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		e.Tracer.Info("[POST] /CardInfo/Record: content= %+v", payload.CardInfo)

		// Verificar se o cartão ou empregado já existe
		var exists int
		err := e.BaseEmulator.QueryRow("SELECT COUNT(*) FROM HikvisionCard WHERE employeeNo = ? OR cardNo = ?",
			[]interface{}{payload.CardInfo.EmployeeNo, payload.CardInfo.CardNo},
			&exists)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		if exists > 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"statusCode":    6,
				"statusString":  "Invalid Content",
				"subStatusCode": "cardNoAlreadyExist",
				"errorCode":     1610637363,
				"errorMsg":      "checkEmployeeNo",
			})
			return
		}

		// Inserir o cartão
		_, err = e.BaseEmulator.Exec(
			"INSERT INTO HikvisionCard VALUES (?, ?)",
			payload.CardInfo.EmployeeNo,
			payload.CardInfo.CardNo,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"statusCode":    1,
			"statusString":  "OK",
			"subStatusCode": "ok",
		})
	})

	// FingerPrint
	router.POST(acURL+"/FingerPrint/SetUp", func(c *gin.Context) {
		c.String(http.StatusOK, "OK")
	})

	router.POST(acURL+"/FingerPrintUploadAll", func(c *gin.Context) {
		var fpCount int
		err := e.BaseEmulator.QueryRow("SELECT COUNT(*) FROM HikvisionFinger", nil, &fpCount)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"statusCode":    1,
			"statusString":  "OK",
			"subStatusCode": "ok",
			"FDRecordDataInfo": []gin.H{
				{
					"FDID":             "1",
					"faceLibType":      "blackFD",
					"name":             "",
					"recordDataNumber": fpCount,
				},
				{
					"FDID":             "2",
					"faceLibType":      "infraredFD",
					"name":             "",
					"recordDataNumber": 0,
				},
			},
		})
	})

	// ------------------------- Intelligent -------------------------
	intelliURL := "/ISAPI/Intelligent/FDLib"

	router.GET(intelliURL+"/Count", func(c *gin.Context) {
		var faceCount int
		err := e.BaseEmulator.QueryRow("SELECT COUNT(*) FROM HikvisionFace", nil, &faceCount)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"statusCode":    1,
			"statusString":  "OK",
			"subStatusCode": "ok",
			"FDRecordDataInfo": []gin.H{
				{
					"FDID":             "1",
					"faceLibType":      "blackFD",
					"name":             "",
					"recordDataNumber": faceCount,
				},
				{
					"FDID":             "2",
					"faceLibType":      "infraredFD",
					"name":             "",
					"recordDataNumber": 0,
				},
			},
		})
	})

	router.POST(intelliURL+"/FDSearch", func(c *gin.Context) {
		var payload struct {
			MaxResults           int `json:"maxResults"`
			SearchResultPosition int `json:"searchResultPosition"`
		}

		if err := c.BindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Obter faces do banco de dados
		rows, err := e.BaseEmulator.Query("SELECT * FROM HikvisionFace LIMIT ? OFFSET ?",
			payload.MaxResults, payload.SearchResultPosition)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer rows.Close()

		// Preparar resposta
		var faces []gin.H
		noOfMatches := 0
		deviceURL := fmt.Sprintf("http://%s:%d/LOCALS/pic/enrlFace", e.Device.IPAddress, e.Device.Port)

		for rows.Next() {
			var userID int
			var photoData string
			if err := rows.Scan(&userID, &photoData); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			faces = append(faces, gin.H{
				"FPID":      strconv.Itoa(userID),
				"faceURL":   fmt.Sprintf("%s/%d", deviceURL, userID),
				"modelData": "",
			})
			noOfMatches++
		}

		if err := rows.Err(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Obter contagem total de faces
		var totalFaces int
		err = e.BaseEmulator.QueryRow("SELECT COUNT(*) FROM HikvisionFace", nil, &totalFaces)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"searchID":           "1",
			"responseStatusStrg": "MORE",
			"numOfMatches":       noOfMatches,
			"totalMatches":       totalFaces,
			"MatchList":          faces,
		})
	})

	router.GET(intelliURL+"/LOCALS/pic/enrlFace/:user_id", func(c *gin.Context) {
		userID := c.Param("user_id")

		// Obter a imagem do banco de dados
		var photoData string
		err := e.BaseEmulator.QueryRow("SELECT PhotoData FROM HikvisionFace WHERE UserID = ?", []interface{}{userID}, &photoData)
		if err != nil {
			c.String(http.StatusNotFound, "Face not found")
			return
		}

		// Decodificar a imagem
		imageData, err := base64.StdEncoding.DecodeString(photoData)
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to decode image")
			return
		}

		// Enviar a imagem como resposta
		c.Data(http.StatusOK, "image/jpeg", imageData)
	})

	router.PUT(intelliURL+"/FDSearch/Delete", func(c *gin.Context) {
		var payload struct {
			FPID []struct {
				Value string `json:"value"`
			} `json:"FPID"`
		}

		if err := c.BindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if len(payload.FPID) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No FPID provided"})
			return
		}

		// Excluir a face
		_, err := e.BaseEmulator.Exec("DELETE FROM HikvisionFace WHERE UserID = ?", payload.FPID[0].Value)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.String(http.StatusOK, "OK")
	})

	router.POST(intelliURL+"/FaceDataRecord", func(c *gin.Context) {
		// Extrair os dados do formulário
		faceDataRecord := c.PostForm("FaceDataRecord")
		if faceDataRecord == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No FaceDataRecord provided"})
			return
		}

		// Analisar os dados JSON
		var faceData struct {
			FPID string `json:"FPID"`
		}
		if err := json.Unmarshal([]byte(faceDataRecord), &faceData); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Obter o arquivo de imagem
		file, err := c.FormFile("FaceImage")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Abrir o arquivo para leitura
		src, err := file.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer src.Close()

		// Ler o conteúdo do arquivo
		imageData, err := io.ReadAll(src)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Converter para base64
		base64Image := base64.StdEncoding.EncodeToString(imageData)

		// Verificar se o usuário já tem uma face registrada
		var exists int
		err = e.BaseEmulator.QueryRow("SELECT COUNT(*) FROM HikvisionFace WHERE UserID = ?", []interface{}{faceData.FPID}, &exists)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		if exists > 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"statusCode":    6,
				"statusString":  "Invalid Content",
				"subStatusCode": "cardNoAlreadyExist",
				"errorCode":     1610637363,
				"errorMsg":      "checkEmployeeNo",
			})
			return
		}

		// Inserir a face
		_, err = e.BaseEmulator.Exec(
			"INSERT INTO HikvisionFace VALUES (?, ?)",
			faceData.FPID,
			base64Image,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"statusCode":    1,
			"statusString":  "OK",
			"subStatusCode": "ok",
		})
	})

	router.PUT(intelliURL+"/FDSetUp", func(c *gin.Context) {
		// Extrair os dados do formulário
		faceDataRecord := c.PostForm("FaceDataRecord")
		if faceDataRecord == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No FaceDataRecord provided"})
			return
		}

		// Analisar os dados JSON
		var faceData struct {
			FPID string `json:"FPID"`
		}
		if err := json.Unmarshal([]byte(faceDataRecord), &faceData); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Obter o arquivo de imagem
		file, err := c.FormFile("FaceImage")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Abrir o arquivo para leitura
		src, err := file.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer src.Close()

		// Ler o conteúdo do arquivo
		imageData, err := io.ReadAll(src)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Converter para base64
		base64Image := base64.StdEncoding.EncodeToString(imageData)

		// Atualizar a face
		_, err = e.BaseEmulator.Exec(
			"UPDATE HikvisionFace SET PhotoData = ? WHERE UserID = ?",
			base64Image,
			faceData.FPID,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"statusCode":    1,
			"statusString":  "OK",
			"subStatusCode": "ok",
		})
	})

	// --------------------------- System ----------------------------
	systemURL := "/ISAPI/System"

	router.GET(systemURL+"/time", func(c *gin.Context) {
		e.Tracer.Info("Polling message received")
		c.XML(http.StatusOK, `<?xml version="1.0" encoding="UTF-8"?>
<DeviceInfo version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema">
    <deviceStatus>OK</deviceStatus>
</DeviceInfo>`)
	})

	router.PUT(systemURL+"/time", func(c *gin.Context) {
		c.String(http.StatusOK, "OK")
	})

	router.GET(systemURL+"/deviceInfo", func(c *gin.Context) {
		xmlContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<DeviceInfo version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema">
    <deviceName>subdoorOne</deviceName>
    <deviceID>255</deviceID>
    <model>DS-K1T673DX-BR</model>
    <serialNumber>DS-K1T673DX-BR20240206V031800ENAA8066966</serialNumber>
    <macAddress>%s</macAddress>
    <firmwareVersion>V3.18.0</firmwareVersion>
    <firmwareReleasedDate>build 240206</firmwareReleasedDate>
    <encoderVersion>V2.7</encoderVersion>
    <encoderReleasedDate>build 240122</encoderReleasedDate>
    <deviceType>ACS</deviceType>
    <subDeviceType>accessControlTerminal</subDeviceType>
    <telecontrolID>1</telecontrolID>
    <localZoneNum>2</localZoneNum>
    <alarmOutNum>1</alarmOutNum>
    <relayNum>2</relayNum>
    <electroLockNum>1</electroLockNum>
    <RS485Num>1</RS485Num>
    <manufacturer>Go Emulator</manufacturer>
    <OEMCode>1</OEMCode>
    <customizedInfo>DZP20240116048</customizedInfo>
    <bspVersion>V1.17.0.642101 build 2023-11-29</bspVersion>
    <dspVersion>V2.7</dspVersion>
    <marketType>2</marketType>
    <productionDate>2023-04-28</productionDate>
</DeviceInfo>`, e.MacAddress)

		c.Header("Content-Type", "application/xml")
		c.String(http.StatusOK, xmlContent)
	})

	router.PUT(systemURL+"/IO/outputs/:output_id/trigger", func(c *gin.Context) {
		outputID := c.Param("output_id")
		e.Tracer.Info("Receiving command for output: %s", outputID)
		c.String(http.StatusOK, "OK")
	})

	// --------------------------- Events ----------------------------
	eventURL := "/ISAPI/Event/notification"

	router.GET(eventURL+"/httpHosts", func(c *gin.Context) {
		e.Tracer.Info("Getting Info httpHosts")
		xmlContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<HttpHostNotificationList version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema">
    <HttpHostNotification>
        <id>1</id>
        <url>/w-access</url>
        <protocolType>HTTP</protocolType>
        <parameterFormatType>XML</parameterFormatType>
        <addressingFormatType>ipaddress</addressingFormatType>
        <ipAddress>172.16.17.20</ipAddress>
        <portNo>15501</portNo>
        <httpAuthenticationMethod>none</httpAuthenticationMethod>
        <SubscribeEvent>
            <heartbeat>30</heartbeat>
            <eventMode>all</eventMode>
        </SubscribeEvent>
    </HttpHostNotification>
    <HttpHostNotification>
        <id>2</id>
        <url></url>
        <protocolType>EHome</protocolType>
        <parameterFormatType>XML</parameterFormatType>
        <addressingFormatType>ipaddress</addressingFormatType>
        <ipAddress>0.0.0.0</ipAddress>
        <portNo>0</portNo>
        <httpAuthenticationMethod>none</httpAuthenticationMethod>
    </HttpHostNotification>
</HttpHostNotificationList>`)

		c.Header("Content-Type", "application/xml")
		c.String(http.StatusOK, xmlContent)
	})

	router.PUT(eventURL+"/httpHosts", func(c *gin.Context) {
		e.Tracer.Info("Receiving configuration from server")
		c.String(http.StatusOK, "OK")
	})

	router.GET(eventURL+"/alertStream", func(c *gin.Context) {
		e.Tracer.Info("[GET] /alertStream")

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
					localAuth, err := e.GetDeviceSetting("LocalAuthentication")
					if err != nil {
						e.Tracer.Error("Failed to get LocalAuthentication setting: %v", err)
						continue
					}

					if localAuth == "1" {
						// Gerar evento
						eventData, err := e.generateRandomEvent()
						if err != nil {
							e.Tracer.Error("Failed to generate random event: %v", err)
							continue
						}

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

					heartbeat := e.getHeartbeatMessage()
					_, err := c.Writer.Write(heartbeat)
					if err != nil {
						e.Tracer.Error("Failed to write heartbeat: %v", err)
						return
					}
					c.Writer.Flush()
				}

				// Verificar se a autenticação local está desativada
				localAuth, err := e.GetDeviceSetting("LocalAuthentication")
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

// Funções auxiliares para gerar respostas XML
func successXMLResponse() XMLResponseStatus {
	return XMLResponseStatus{
		Version:       "1.0",
		Xmlns:         "http://www.hikvision.com/ver10/XMLSchema",
		RequestURL:    "",
		StatusCode:    "1",
		StatusString:  "OK",
		SubStatusCode: "ok",
	}
}

func errorXMLResponse(message string) XMLResponseStatus {
	return XMLResponseStatus{
		Version:       "1.0",
		Xmlns:         "http://www.hikvision.com/ver10/XMLSchema",
		RequestURL:    "",
		StatusCode:    "6",
		StatusString:  "Error",
		SubStatusCode: message,
	}
}
