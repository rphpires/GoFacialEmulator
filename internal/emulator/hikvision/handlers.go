package hikvision

import (
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

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

// HttpHostNotificationList is the payload sent by the client on
// PUT /ISAPI/Event/notification/httpHosts. We only consume the first
// HttpHostNotification entry and only the fields needed for online event
// delivery (ipAddress, portNo, url).
type HttpHostNotificationList struct {
	XMLName xml.Name                   `xml:"HttpHostNotificationList"`
	Items   []HttpHostNotificationItem `xml:"HttpHostNotification"`
}

type HttpHostNotificationItem struct {
	ID        string `xml:"id"`
	URL       string `xml:"url"`
	IPAddress string `xml:"ipAddress"`
	PortNo    string `xml:"portNo"`
}

// parseHttpHostNotification extracts ipAddress, portNo, url from the raw
// XML body sent by the client. Returns an error if the body is not valid
// XML or if no HttpHostNotification entries are present.
func parseHttpHostNotification(body []byte) (HttpHostNotificationItem, error) {
	var list HttpHostNotificationList
	if err := xml.Unmarshal(body, &list); err != nil {
		return HttpHostNotificationItem{}, fmt.Errorf("invalid XML: %w", err)
	}
	if len(list.Items) == 0 {
		return HttpHostNotificationItem{}, fmt.Errorf("no HttpHostNotification entries")
	}
	return list.Items[0], nil
}

// SetupRoutes configura todas as rotas específicas do Hikvision
func (e *Emulator) SetupRoutes(router *gin.Engine) {
	// Endpoint para verificar o status do emulador
	router.GET("/emulator/get-status", e.handleGetStatus)

	// ======================== AccessControl ========================
	acURL := "/ISAPI/AccessControl"

	// AcsCfg
	router.GET(acURL+"/AcsCfg", e.handleGetAcsCfg)
	router.PUT(acURL+"/AcsCfg", e.handlePutAcsCfg)
	router.PUT(acURL+"/AcsEvent/StorageCfg", e.handlePutStorageCfg)
	router.POST(acURL+"/AcsEvent", e.handlePostAcsEvent)
	router.PUT(acURL+"/Door/param/1", e.handleSetDoorParameters)
	router.PUT(acURL+"/RemoteControl/door/:output_id", e.handleCommandDoor)

	// remoteCheck: em modo online o SC responde a requisição de verificação
	// remota com PUT contendo serialNo + checkResult (success/failed).
	router.PUT(acURL+"/remoteCheck", e.handleRemoteCheck)

	// Capabilities (usado pelo cliente para detectar suporte a online access)
	router.GET(acURL+"/capabilities", e.handleGetAccessControlCapabilities)

	// UserInfo
	router.GET(acURL+"/UserInfo/Count", e.handleGetUserCount)
	router.POST(acURL+"/UserInfo/Search", e.handlePostUserSearch)
	router.POST(acURL+"/UserInfo/Record", e.handlePostUserRecord)
	router.PUT(acURL+"/UserInfo/Modify", e.handlePutUserModify)

	// UserInfoDetail
	router.GET(acURL+"/UserInfoDetail/DeleteProcess", e.handleGetUserDeleteProcess)
	router.PUT(acURL+"/UserInfoDetail/Delete", e.handlePutUserDelete)

	// CardInfo
	router.GET(acURL+"/CardInfo/Count", e.handleGetCardCount)
	router.POST(acURL+"/CardInfo/Search", e.handlePostCardSearch)
	router.POST(acURL+"/CardInfo/Record", e.handlePostCardRecord)

	// FingerPrint
	router.POST(acURL+"/FingerPrint/SetUp", e.handlePostFingerprintSetup)
	router.POST(acURL+"/FingerPrintUploadAll", e.handlePostFingerprintUploadAll)

	// ========================= Intelligent =========================
	intelliURL := "/ISAPI/Intelligent/FDLib"

	router.GET(intelliURL+"/Count", e.handleGetFDLibCount)
	router.POST(intelliURL+"/FDSearch", e.handlePostFingerprintSearch)
	router.GET(intelliURL+"/LOCALS/pic/enrlFace/:user_id", e.handleGetRemoteFace)
	router.PUT(intelliURL+"/FDSearch/Delete", e.handlePutFingerprintDelete)
	router.POST(intelliURL+"/FaceDataRecord", e.handlePostFaceDataRecord)
	router.PUT(intelliURL+"/FDSetUp", e.handlePutFaceSetup)

	// =========================== System ============================
	systemURL := "/ISAPI/System"

	router.GET(systemURL+"/time", e.handleGetDateTime)
	router.PUT(systemURL+"/time", e.handleSetDateTime)
	router.GET(systemURL+"/deviceInfo", e.handleGetDeviceInfo)
	router.PUT(systemURL+"/IO/outputs/:output_id/trigger", e.handleCommandOutput)

	// =========================== Events ============================
	eventURL := "/ISAPI/Event/notification"

	router.GET(eventURL+"/httpHosts", e.handleGetHttpHosts)
	router.PUT(eventURL+"/httpHosts", e.handlePutHttpHosts)
	router.GET(eventURL+"/alertStream", e.handleGetAlertStream)

	// =========================== Status (Custom) ====================
	router.GET("/status", e.handleGetStatus)
	router.GET("/health", e.handleGetStatus) // Alias
}

// ====================== STATUS HANDLERS ======================

func (e *Emulator) handleGetStatus(c *gin.Context) {
	e.tracer.Info("[STATUS] Status request received")

	currentTime := time.Now().Format("2006-01-02 15:04:05")
	count, err := e.repo.GetTotalUsers()

	uptime := int64(0)
	if e.startTime != nil {
		uptime = int64(time.Since(*e.startTime).Seconds())
	}

	response := gin.H{
		"device_id":      e.device.ID,
		"device_name":    e.device.Name,
		"port":           e.device.Port,
		"model":          "Hikvision",
		"status":         e.GetStatus(),
		"is_running":     e.IsRunning(),
		"total_users":    count,
		"uptime_seconds": uptime,
		"current_time":   currentTime,
		"mac_address":    e.macAddress,
		"version":        "1.0.0",
		"event_interval": e.device.EventInterval,
	}

	if err != nil {
		response["total_users_error"] = err.Error()
		e.tracer.Error("[STATUS] Failed to get total users: %v", err)
	}

	c.JSON(http.StatusOK, response)
}

// ====================== ACCESS CONTROL HANDLERS ======================

func (e *Emulator) handleGetAcsCfg(c *gin.Context) {
	localAuth, _ := e.repo.GetSetting("LocalAuthentication")
	remoteCheckDoorEnabled := localAuth == ""

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
}

// handleGetAccessControlCapabilities responde o XML que o cliente Python parseia
// (via xmltodict com namespace stripping) em __get_device_capabilities. O único
// campo consumido é isSupportRemoteCheck, usado para habilitar o modo online.
func (e *Emulator) handleGetAccessControlCapabilities(c *gin.Context) {
	body := `<?xml version="1.0" encoding="UTF-8"?>
<AccessControl version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema">
<isSupportRemoteCheck>true</isSupportRemoteCheck>
</AccessControl>
`
	c.Header("Content-Type", "application/xml")
	c.String(http.StatusOK, body)
}

func (e *Emulator) handlePutAcsCfg(c *gin.Context) {
	var payload struct {
		AcsCfg struct {
			RemoteCheckDoorEnabled bool `json:"remoteCheckDoorEnabled"`
		} `json:"AcsCfg"`
	}

	if err := c.BindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"statusCode":    6,
			"statusString":  "Error",
			"subStatusCode": "Invalid request",
		})
		return
	}

	e.tracer.Info("Setting online mode: %v", payload.AcsCfg.RemoteCheckDoorEnabled)
	localAuthValue := "0"
	if !payload.AcsCfg.RemoteCheckDoorEnabled {
		localAuthValue = "1"
	}

	if err := e.repo.SetSetting("LocalAuthentication", localAuthValue); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"statusCode":    6,
			"statusString":  "Error",
			"subStatusCode": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"statusCode":    1,
		"statusString":  "OK",
		"subStatusCode": "ok",
	})
}

func (e *Emulator) handlePutStorageCfg(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"statusCode":    1,
		"statusString":  "OK",
		"subStatusCode": "ok",
	})
}

func (e *Emulator) handleSetDoorParameters(c *gin.Context) {
	c.XML(http.StatusOK, successXMLResponse())
}

func (e *Emulator) handleCommandDoor(c *gin.Context) {
	outputID := c.Param("output_id")
	e.tracer.Info("New command received to output= %s", outputID)
	c.XML(http.StatusOK, successXMLResponse())
}

// handleRemoteCheck recebe a decisão de acesso do SC em modo online. O SC,
// após processar a requisição de remote-check emitida pelo emulador (evento com
// serialNo e SEM frontSerialNo), responde com PUT /ISAPI/AccessControl/remoteCheck
// carregando { "RemoteCheck": { "serialNo", "checkResult", "info" } }. Este é o
// momento em que a autorização de fato ocorre — o dispositivo real abriria a porta
// em checkResult=="success".
func (e *Emulator) handleRemoteCheck(c *gin.Context) {
	var payload struct {
		RemoteCheck struct {
			SerialNo    int    `json:"serialNo"`
			CheckResult string `json:"checkResult"`
			Info        string `json:"info"`
		} `json:"RemoteCheck"`
	}

	if err := c.BindJSON(&payload); err != nil {
		e.tracer.Error("[remoteCheck] invalid body: %v", err)
		writeHikvisionXML(c, http.StatusBadRequest, "6", "Error", "Invalid request")
		return
	}

	rc := payload.RemoteCheck
	e.tracer.Info("[remoteCheck] SC decision: serialNo=%d checkResult=%q info=%q", rc.SerialNo, rc.CheckResult, rc.Info)

	// Sucesso => dispositivo abriria a porta. Simulamos os eventos de porta para
	// refletir o acesso concedido (não bloqueante).
	if strings.EqualFold(rc.CheckResult, "success") {
		e.tracer.Info("[remoteCheck] access granted (serialNo=%d) -> simulating door open", rc.SerialNo)
		go e.simulateDoorEvents()
	}

	writeHikvisionXML(c, http.StatusOK, "1", "OK", "ok")
}

// handlePostAcsEvent responde à busca de eventos (POST /ISAPI/AccessControl/AcsEvent).
// Em modo online os eventos chegam por webhook, então retornamos "NO MATCH" — o
// suficiente para o polling do cliente não receber 404. Evita ruído nos traces.
func (e *Emulator) handlePostAcsEvent(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"AcsEvent": gin.H{
			"searchID":           "0",
			"responseStatusStrg":  "NO MATCH",
			"numOfMatches":        0,
			"totalMatches":        0,
			"InfoList":            []interface{}{},
		},
	})
}

// ====================== USER INFO HANDLERS ======================

func (e *Emulator) handleGetUserCount(c *gin.Context) {
	counts, err := e.repo.CountItems()
	if err != nil {
		c.XML(http.StatusInternalServerError, errorXMLResponse(err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"UserInfoCount": gin.H{
			"userNumber":                counts.Users,
			"bindFaceUserNumber":        counts.Faces,
			"bindFingerprintUserNumber": counts.Fingerprints,
			"bindCardUserNumber":        counts.Cards,
			"bindRemoteControlNumber":   0,
		},
	})
}

func (e *Emulator) handlePostUserSearch(c *gin.Context) {
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

	users, err := e.repo.GetUsers(payload.UserInfoSearchCond.MaxResults, payload.UserInfoSearchCond.SearchResultPosition)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Converter para formato de resposta
	var userInfos []UserInfo
	for _, user := range users {
		localUIRightBool := user.LocalUIRight != "" && user.LocalUIRight != "0"

		userInfo := UserInfo{
			EmployeeNo:         user.EmployeeNo,
			Name:               user.Name,
			UserType:           "normal",
			SortByNamePosition: 0,
			SortByNameFlag:     "#",
			CloseDelayEnabled:  false,
			Valid: struct {
				Enable    bool   `json:"enable"`
				BeginTime string `json:"beginTime"`
				EndTime   string `json:"endTime"`
				TimeType  string `json:"timeType"`
			}{
				Enable:    true,
				BeginTime: user.BeginTime.Format("2006-01-02T15:04:05"),
				EndTime:   user.EndTime.Format("2006-01-02T15:04:05"),
				TimeType:  "local",
			},
			BelongGroup: "",
			Password:    user.Password,
			DoorRight:   "1",
			RightPlan: []struct {
				DoorNo         int    `json:"doorNo"`
				PlanTemplateNo string `json:"planTemplateNo"`
			}{
				{DoorNo: 1, PlanTemplateNo: "1"},
			},
			MaxOpenDoorTime:    0,
			OpenDoorTime:       0,
			RoomNumber:         0,
			FloorNumber:        0,
			LocalUIRight:       localUIRightBool,
			Gender:             "unknown",
			NumOfCard:          1,
			NumOfRemoteControl: 0,
			NumOfFP:            0,
			NumOfFace:          0,
			PersonInfoExtends: []struct {
				Value string `json:"value"`
			}{
				{Value: ""},
			},
		}
		userInfos = append(userInfos, userInfo)
	}

	// Obter total de usuários
	counts, _ := e.repo.CountItems()

	c.JSON(http.StatusOK, UserSearchResponse{
		UserInfoSearch: struct {
			SearchID           string     `json:"searchID"`
			ResponseStatusStrg string     `json:"responseStatusStrg"`
			NumOfMatches       int        `json:"numOfMatches"`
			TotalMatches       int        `json:"totalMatches"`
			UserInfo           []UserInfo `json:"UserInfo"`
		}{
			SearchID:           "1",
			ResponseStatusStrg: "MORE",
			NumOfMatches:       len(userInfos),
			TotalMatches:       counts.Users,
			UserInfo:           userInfos,
		},
	})
}

func (e *Emulator) handlePostUserRecord(c *gin.Context) {
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

	// Verificar se já existe
	exists, err := e.repo.CheckIfUserExists(payload.UserInfo.EmployeeNo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if exists {
		c.JSON(http.StatusBadRequest, gin.H{
			"statusCode":    6,
			"statusString":  "Invalid Content",
			"subStatusCode": "employeeNoAlreadyExist",
			"errorCode":     1610637344,
			"errorMsg":      "checkUser",
		})
		return
	}

	// Converter para estrutura interna
	localUIRight := "0"
	if payload.UserInfo.LocalUIRight {
		localUIRight = "1"
	}

	beginTime, _ := time.Parse("2006-01-02T15:04:05", payload.UserInfo.Valid.BeginTime)
	endTime, _ := time.Parse("2006-01-02T15:04:05", payload.UserInfo.Valid.EndTime)

	user := &User{
		EmployeeNo:   payload.UserInfo.EmployeeNo,
		Name:         payload.UserInfo.Name,
		Password:     payload.UserInfo.Password,
		LocalUIRight: localUIRight,
		BeginTime:    beginTime,
		EndTime:      endTime,
	}

	if err := e.repo.AddUser(user); err != nil {
		e.tracer.Error("[USER_SYNC] Failed to add user: employeeNo=%s, name=%s, error=%v", user.EmployeeNo, user.Name, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Obter total atualizado
	totalUsers, _ := e.repo.GetTotalUsers()
	e.tracer.Info("[USER_SYNC] User added successfully: employeeNo=%s, name=%s, total_users=%d", user.EmployeeNo, user.Name, totalUsers)

	c.JSON(http.StatusOK, gin.H{
		"statusCode":    1,
		"statusString":  "OK",
		"subStatusCode": "ok",
	})
}

func (e *Emulator) handlePutUserModify(c *gin.Context) {
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

	// Converter para estrutura interna
	localUIRight := "0"
	if payload.UserInfo.LocalUIRight {
		localUIRight = "1"
	}

	beginTime, _ := time.Parse("2006-01-02T15:04:05", payload.UserInfo.Valid.BeginTime)
	endTime, _ := time.Parse("2006-01-02T15:04:05", payload.UserInfo.Valid.EndTime)

	user := &User{
		EmployeeNo:   payload.UserInfo.EmployeeNo,
		Name:         payload.UserInfo.Name,
		Password:     payload.UserInfo.Password,
		LocalUIRight: localUIRight,
		BeginTime:    beginTime,
		EndTime:      endTime,
	}

	if err := e.repo.UpdateUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"statusCode":    1,
		"statusString":  "OK",
		"subStatusCode": "ok",
	})
}

// ====================== USER INFO DETAIL HANDLERS ======================

func (e *Emulator) handleGetUserDeleteProcess(c *gin.Context) {
	status := "success"
	if e.deleteInProgress {
		status = "inProgress"
	}

	c.JSON(http.StatusOK, gin.H{
		"UserInfoDetailDeleteProcess": gin.H{
			"status": status,
		},
	})
}

func (e *Emulator) handlePutUserDelete(c *gin.Context) {
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
	e.deleteInProgress = true
	defer func() { e.deleteInProgress = false }()

	// Excluir usuários
	for _, emp := range payload.UserInfoDetail.EmployeeNoList {
		e.tracer.Info("Deleting user with EmployeeNo= %s", emp.EmployeeNo)

		if err := e.repo.DeleteUser(emp.EmployeeNo); err != nil {
			e.tracer.Error("Failed to delete user %s: %v", emp.EmployeeNo, err)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"statusCode":    1,
		"statusString":  "OK",
		"subStatusCode": "ok",
	})
}

// ====================== CARD INFO HANDLERS ======================

func (e *Emulator) handleGetCardCount(c *gin.Context) {
	counts, err := e.repo.CountItems()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"CardInfoCount": gin.H{
			"cardNumber": counts.Cards,
		},
	})
}

func (e *Emulator) handlePostCardSearch(c *gin.Context) {
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

	cards, err := e.repo.GetCards(payload.CardInfoSearchCond.MaxResults, payload.CardInfoSearchCond.SearchResultPosition)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Converter para formato de resposta
	var cardInfos []CardInfo
	for _, card := range cards {
		cardInfo := CardInfo{
			EmployeeNo:               card.EmployeeNo,
			CardNo:                   card.CardNo,
			IsCardAsRemoteControlBtn: false,
			LeaderCard:               "",
			CardType:                 "normalCard",
		}
		cardInfos = append(cardInfos, cardInfo)
	}

	// Obter total de cartões
	counts, _ := e.repo.CountItems()

	c.JSON(http.StatusOK, CardSearchResponse{
		CardInfoSearch: struct {
			SearchID           string     `json:"searchID"`
			ResponseStatusStrg string     `json:"responseStatusStrg"`
			NumOfMatches       int        `json:"numOfMatches"`
			TotalMatches       int        `json:"totalMatches"`
			CardInfo           []CardInfo `json:"CardInfo"`
		}{
			SearchID:           "1",
			ResponseStatusStrg: "MORE",
			NumOfMatches:       len(cardInfos),
			TotalMatches:       counts.Cards,
			CardInfo:           cardInfos,
		},
	})
}

func (e *Emulator) handlePostCardRecord(c *gin.Context) {
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

	e.tracer.Info("[POST] /CardInfo/Record: content= %+v", payload.CardInfo)

	// Verificar se já existe
	exists, err := e.repo.CheckIfCardExists(payload.CardInfo.EmployeeNo, payload.CardInfo.CardNo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if exists {
		c.JSON(http.StatusBadRequest, gin.H{
			"statusCode":    6,
			"statusString":  "Invalid Content",
			"subStatusCode": "cardNoAlreadyExist",
			"errorCode":     1610637363,
			"errorMsg":      "checkEmployeeNo",
		})
		return
	}

	card := &Card{
		EmployeeNo: payload.CardInfo.EmployeeNo,
		CardNo:     payload.CardInfo.CardNo,
	}

	if err := e.repo.AddCard(card); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"statusCode":    1,
		"statusString":  "OK",
		"subStatusCode": "ok",
	})
}

// ====================== FINGERPRINT HANDLERS ======================

func (e *Emulator) handlePostFingerprintSetup(c *gin.Context) {
	writeHikvisionXML(c, http.StatusOK, "1", "OK", "ok")
}

func (e *Emulator) handlePostFingerprintUploadAll(c *gin.Context) {
	counts, err := e.repo.CountItems()
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
				"recordDataNumber": counts.Fingerprints,
			},
			{
				"FDID":             "2",
				"faceLibType":      "infraredFD",
				"name":             "",
				"recordDataNumber": 0,
			},
		},
	})
}

// ====================== INTELLIGENT/FACE HANDLERS ======================

func (e *Emulator) handleGetFDLibCount(c *gin.Context) {
	counts, err := e.repo.CountItems()
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
				"recordDataNumber": counts.Faces,
			},
			{
				"FDID":             "2",
				"faceLibType":      "infraredFD",
				"name":             "",
				"recordDataNumber": 0,
			},
		},
	})
}

func (e *Emulator) handlePostFingerprintSearch(c *gin.Context) {
	var payload struct {
		MaxResults           int `json:"maxResults"`
		SearchResultPosition int `json:"searchResultPosition"`
	}

	if err := c.BindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	faces, err := e.repo.GetFaces(payload.MaxResults, payload.SearchResultPosition)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Converter para formato de resposta
	deviceURL := fmt.Sprintf("http://%s:%d/ISAPI/Intelligent/FDLib/LOCALS/pic/enrlFace", e.device.IPAddress, e.device.Port)
	var faceInfos []FaceInfo
	for _, face := range faces {
		faceInfo := FaceInfo{
			FPID:      strconv.Itoa(face.UserID),
			FaceURL:   fmt.Sprintf("%s/%d", deviceURL, face.UserID),
			ModelData: "",
		}
		faceInfos = append(faceInfos, faceInfo)
	}

	// Obter total de faces
	counts, _ := e.repo.CountItems()

	c.JSON(http.StatusOK, FaceSearchResponse{
		SearchID:           "1",
		ResponseStatusStrg: "MORE",
		NumOfMatches:       len(faceInfos),
		TotalMatches:       counts.Faces,
		MatchList:          faceInfos,
	})
}

func (e *Emulator) handleGetRemoteFace(c *gin.Context) {
	userID := c.Param("user_id")

	photoData, err := e.repo.GetFace(userID)
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
}

func (e *Emulator) handlePutFingerprintDelete(c *gin.Context) {
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

	if err := e.repo.DeleteFace(payload.FPID[0].Value); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	writeHikvisionXML(c, http.StatusOK, "1", "OK", "ok")
}

func (e *Emulator) handlePostFaceDataRecord(c *gin.Context) {
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
	fileHeader, err := c.FormFile("FaceImage")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Processar a imagem
	base64Image, err := e.processUploadedImage(fileHeader)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Verificar se já existe
	exists, err := e.repo.CheckIfFaceExists(faceData.FPID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if exists {
		c.JSON(http.StatusBadRequest, gin.H{
			"statusCode":    6,
			"statusString":  "Invalid Content",
			"subStatusCode": "cardNoAlreadyExist",
			"errorCode":     1610637363,
			"errorMsg":      "checkEmployeeNo",
		})
		return
	}

	if err := e.repo.AddFace(faceData.FPID, base64Image); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"statusCode":    1,
		"statusString":  "OK",
		"subStatusCode": "ok",
	})
}

func (e *Emulator) handlePutFaceSetup(c *gin.Context) {
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
	fileHeader, err := c.FormFile("FaceImage")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Processar a imagem
	base64Image, err := e.processUploadedImage(fileHeader)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := e.repo.UpdateFace(faceData.FPID, base64Image); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"statusCode":    1,
		"statusString":  "OK",
		"subStatusCode": "ok",
	})
}

// ====================== SYSTEM HANDLERS ======================

func (e *Emulator) handleGetDateTime(c *gin.Context) {
	e.tracer.Info("Polling message received")
	now := time.Now().Format("2006-01-02T15:04:05-07:00")
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Time version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema">
<timeMode>manual</timeMode>
<localTime>%s</localTime>
<timeZone>CST+3:00:00</timeZone>
</Time>
`, now)
	c.Header("Content-Type", "application/xml")
	c.String(http.StatusOK, body)
}

func (e *Emulator) handleSetDateTime(c *gin.Context) {
	writeHikvisionXML(c, http.StatusOK, "1", "OK", "ok")
}

func (e *Emulator) handleGetDeviceInfo(c *gin.Context) {
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
</DeviceInfo>`, strings.ToUpper(e.macAddress))

	c.Header("Content-Type", "application/xml")
	c.String(http.StatusOK, xmlContent)
}

func (e *Emulator) handleCommandOutput(c *gin.Context) {
	outputID := c.Param("output_id")
	e.tracer.Info("Receiving command for output: %s", outputID)
	writeHikvisionXML(c, http.StatusOK, "1", "OK", "ok")
}

// ====================== EVENT HANDLERS ======================

func (e *Emulator) handleGetHttpHosts(c *gin.Context) {
	e.tracer.Info("Getting Info httpHosts")

	remoteServer, _ := e.repo.GetSetting("RemoteServer")
	remotePort, _ := e.repo.GetSetting("RemotePort")
	remoteURL, _ := e.repo.GetSetting("RemoteURL")

	xmlContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<HttpHostNotificationList version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema">
    <HttpHostNotification>
        <id>1</id>
        <url>%s</url>
        <protocolType>HTTP</protocolType>
        <parameterFormatType>XML</parameterFormatType>
        <addressingFormatType>ipaddress</addressingFormatType>
        <ipAddress>%s</ipAddress>
        <portNo>%s</portNo>
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
</HttpHostNotificationList>`, remoteURL, remoteServer, remotePort)

	c.Header("Content-Type", "application/xml")
	c.String(http.StatusOK, xmlContent)
}

func (e *Emulator) handlePutHttpHosts(c *gin.Context) {
	e.tracer.Info("httpHosts: PUT received from %s, Content-Length=%d, Content-Type=%q",
		c.Request.RemoteAddr, c.Request.ContentLength, c.Request.Header.Get("Content-Type"))

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		e.tracer.Error("httpHosts: failed to read body: %v", err)
		writeHikvisionXML(c, http.StatusBadRequest, "6", "Error", "Invalid body")
		return
	}
	e.tracer.Info("httpHosts: body bytes=%d, body=%q", len(body), string(body))

	item, err := parseHttpHostNotification(body)
	if err != nil {
		e.tracer.Error("httpHosts: parse failed: %v", err)
		writeHikvisionXML(c, http.StatusBadRequest, "6", "Error", "Invalid XML")
		return
	}
	e.tracer.Info("httpHosts: parsed ipAddress=%s portNo=%s url=%s", item.IPAddress, item.PortNo, item.URL)

	if err := e.repo.SetSetting("RemoteServer", item.IPAddress); err != nil {
		e.tracer.Error("httpHosts: SetSetting RemoteServer failed: %v", err)
		writeHikvisionXML(c, http.StatusInternalServerError, "6", "Error", err.Error())
		return
	}
	if err := e.repo.SetSetting("RemotePort", item.PortNo); err != nil {
		e.tracer.Error("httpHosts: SetSetting RemotePort failed: %v", err)
		writeHikvisionXML(c, http.StatusInternalServerError, "6", "Error", err.Error())
		return
	}
	if err := e.repo.SetSetting("RemoteURL", item.URL); err != nil {
		e.tracer.Error("httpHosts: SetSetting RemoteURL failed: %v", err)
		writeHikvisionXML(c, http.StatusInternalServerError, "6", "Error", err.Error())
		return
	}

	e.tracer.Info("httpHosts: persisted OK, writing XML response")
	writeHikvisionXML(c, http.StatusOK, "1", "OK", "ok")
	e.tracer.Info("httpHosts: response written (status=%d, wrote=%d bytes)",
		c.Writer.Status(), c.Writer.Size())
}

// writeHikvisionXML escreve a resposta no formato exato que um equipamento
// Hikvision real devolve (com prolog XML e quebras de linha), evitando
// eventuais incompatibilidades de xml.Marshal com parsers que esperam o
// texto literal do firmware.
func writeHikvisionXML(c *gin.Context, status int, statusCode, statusString, subStatusCode string) {
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<ResponseStatus version="1.0" xmlns="http://www.hikvision.com/ver10/XMLSchema">
<requestURL></requestURL>
<statusCode>%s</statusCode>
<statusString>%s</statusString>
<subStatusCode>%s</subStatusCode>
</ResponseStatus>
`, statusCode, statusString, subStatusCode)
	c.Header("Content-Type", "application/xml")
	c.String(status, body)
}

func (e *Emulator) handleGetAlertStream(c *gin.Context) {
	e.tracer.Info("[GET] /alertStream")

	// Hijack da conexão: o dispositivo Hikvision real envia multipart/mixed
	// sem Transfer-Encoding: chunked. O net/http do Go aplicaria chunked por padrão
	// e também imporia WriteTimeout, o que fecha o stream. Assumindo a conexão bruta
	// replicamos exatamente o comportamento do equipamento.
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
	e.tracer.Info("[alertStream] connection hijacked from %s", conn.RemoteAddr())

	// Limpa deadlines herdadas de ReadTimeout/WriteTimeout do http.Server;
	// o stream precisa ficar aberto indefinidamente.
	_ = conn.SetDeadline(time.Time{})

	httpResponse := "HTTP/1.1 200 OK\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Connection: keep-alive\r\n" +
		"Content-Type: multipart/mixed; boundary=MIME_boundary\r\n" +
		"\r\n"

	n, err := bufrw.WriteString(httpResponse)
	if err != nil {
		e.tracer.Error("[alertStream] Failed to write HTTP headers: %v", err)
		return
	}
	if err := bufrw.Flush(); err != nil {
		e.tracer.Error("[alertStream] Failed to flush HTTP headers: %v", err)
		return
	}
	e.tracer.Info("[alertStream] HTTP headers sent (%d bytes)", n)

	e.handleEventStream(conn, bufrw)
	e.tracer.Info("[alertStream] event stream handler returned")
}

// ====================== HELPER FUNCTIONS ======================

func (e *Emulator) processUploadedImage(fileHeader *multipart.FileHeader) (string, error) {
	// Abrir o arquivo
	file, err := fileHeader.Open()
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Ler o conteúdo do arquivo
	imageData, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}

	// Converter para base64
	return base64.StdEncoding.EncodeToString(imageData), nil
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
