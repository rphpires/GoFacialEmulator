package dahua

import (
	"bufio"
	"crypto/md5"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// crlf e o terminador de linha do protocolo HTTP.
const crlf = "\r\n"

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

	// ======================== EventManager (Streaming sem snapshot) ========================
	router.GET("/cgi-bin/eventManager.cgi", e.handleEventManager)

	// ======================== AccessFace ========================
	router.GET("/cgi-bin/AccessFace.cgi", e.handleAccessFaceGet)
	router.POST("/cgi-bin/AccessFace.cgi", e.handleAccessFacePost)

	// ======================== AccessUser ========================
	router.GET("/cgi-bin/AccessUser.cgi", e.handleAccessUserGet)

	// NOTA: /cgi-bin/AccessFingerprint.cgi (insertMulti/removeMulti/get) segue
	// deliberadamente não implementado — decisão de escopo de 2026-08-26, uso
	// raro. Ver docs/superpowers/specs/2026-08-26-comparacao-endpoints-sc-v2.md.
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
		saida, ok := e.configs.Render(name)
		if !ok {
			// Antes so a tabela Network era atendida; name=AccessControl
			// devolvia 400 e o gerenciador nunca lia CardStoreFormat.
			e.tracer.Warning("[ConfigManager] tabela desconhecida: %q", name)
			e.handleResponse(c, "Error\r\nUnknown config name", http.StatusBadRequest, 50)
			return
		}
		c.Header("Content-Type", "text/plain; charset=utf-8")
		time.Sleep(450 * time.Millisecond)
		c.String(http.StatusOK, saida)

	case "setConfig":
		n := e.configs.SetAllFromQuery(c.Request.URL.Query())
		e.tracer.Info("[ConfigManager] setConfig gravou %d chave(s): %+v", n, c.Request.URL.Query())
		e.aplicarConfigDeRede()
		e.handleResponse(c, "OK", http.StatusOK, 50)

	default:
		e.handleResponse(c, "Unknown action", http.StatusBadRequest, 50)
	}
}

// aplicarConfigDeRede reflete no estado do emulador as chaves de setConfig que
// mudam comportamento: destino do push online e modo de operacao. Antes isso
// estava embutido no handler e so olhava PictureHttpUpload.*; o
// Intelbras_ModeCfg.DeviceMode (commit 81cc5d2 do gerenciador) era descartado.
func (e *Emulator) aplicarConfigDeRede() {
	if e.repo == nil || e.configs == nil {
		return
	}

	if addr := e.configs.Get("PictureHttpUpload.UploadServerList[0].Address"); addr != "" {
		e.remoteServer = addr
		_ = e.repo.SetSetting("RemoteServer", addr)
	}
	if port := e.configs.Get("PictureHttpUpload.UploadServerList[0].Port"); port != "" {
		e.remotePort = port
		_ = e.repo.SetSetting("RemotePort", port)
	}
	e.remoteServerURL = fmt.Sprintf("http://%s:%s", e.remoteServer, e.remotePort)

	if path := e.configs.Get("PictureHttpUpload.UploadServerList[0].Uploadpath"); path != "" {
		_ = e.repo.SetSetting("RemoteUploadPath", path)
	}

	// Modo de operacao. PictureHttpUpload.Enable e a fonte historica;
	// Intelbras_ModeCfg.DeviceMode (0 OFFLINE, 1 POST_EVENTS, 2 ONLINE) tem
	// precedencia quando presente, porque e o que o gerenciador passou a usar.
	localAuth := ""
	switch strings.ToLower(e.configs.Get("PictureHttpUpload.Enable")) {
	case "true", "1":
		localAuth = "0"
	case "false", "0":
		localAuth = "1"
	}
	switch e.configs.Get("Intelbras_ModeCfg.DeviceMode") {
	case "2":
		localAuth = "0"
	case "0":
		localAuth = "1"
	}
	if localAuth != "" {
		e.tracer.Info("[ConfigManager] LocalAuthentication=%s", localAuth)
		_ = e.repo.SetSetting("LocalAuthentication", localAuth)
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
	case "captureFingerprint":
		// O device responde OK na hora e, segundos depois, empurra pelo stream
		// um evento com CollectResult — é nesse evento que enroll_fingerprint
		// destrava (IoDahuaCommunication.py:2219 e :1356).
		userID := c.Query("info.UserID")
		e.tracer.Info("Command captureFingerprint: readerID=%s userID=%s timeout=%s",
			c.Query("info.ReaderID"), userID, c.Query("timeout"))
		if e.enrollFn != nil {
			e.enrollFn(userID)
		}
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

		// Medido na bancada: com Condition.UserID presente, Total é 0 ou 1
		// PARA AQUELE UserID - não a contagem global de faces do device. O
		// gateway usa Total para decidir HasFace (spec do fabricante, seção
		// 12.3.4: "Total ... return 0 if not found"); devolver a contagem
		// global aqui faz HasFace nunca dar false, mesmo depois de um
		// remove. Sem Condition.UserID, a contagem global é o
		// comportamento correto (não há UserID para filtrar por).
		var response *FindFaceResponse
		var err error
		if userIDStr := c.Query("Condition.UserID"); userIDStr != "" {
			userID, _ := strconv.Atoi(userIDStr)
			response, err = e.repo.FindRemoteFacesByUserID(userID)
		} else {
			response, err = e.repo.FindRemoteFaces()
		}
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
		// Idempotente no device real: UserID sem face devolve 200 OK, nao
		// erro. RemoveFace faz um DELETE simples - zero linhas afetadas nao e
		// erro em Postgres, entao isso ja funciona sem tratamento especial.
		time.Sleep(50 * time.Millisecond)
		userID, _ := strconv.Atoi(c.Query("UserID"))
		err := e.repo.RemoveFace(userID)
		if err != nil {
			e.handleResponse(c, "Error", http.StatusInternalServerError, 50)
			return
		}
		e.handleResponse(c, "OK", http.StatusOK, 50)

	case "list", "find", "getCollect":
		// Medido na bancada: as tres devolvem 501 no device real.
		e.handleResponse(c, "Error\nNot Implemented!", http.StatusNotImplemented, 50)

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

// errCodeUnknownTable e o code devolvido pelo device real quando `name` nao e
// uma tabela que ele conhece. Medido na bancada com name=AccessFace.
const errCodeUnknownTable = 285278242

func (e *Emulator) handleRecordFinder(c *gin.Context) {
	action := c.Query("action")
	name := c.Query("name")

	e.tracer.Info("RecordFinder: action=%s, name=%s", action, name)

	// AccessControlCardRec e o historico de acessos; AccessControlCard e a
	// tabela de cartoes. Antes o handler ignorava name e devolvia cartoes para
	// as duas, e read_events() os interpretava como eventos - gerando eventos
	// offline fantasmas a cada varredura.
	if name == "AccessControlCardRec" {
		e.handleRecordFinderCardRec(c, action)
		return
	}

	if name != "AccessControlCard" {
		// Medido na bancada (name=AccessFace): o device real nao devolve
		// "Unknown action"/400 texto puro para tabela inexistente - devolve
		// esse envelope JSON especifico, tanto para find quanto para
		// getQuerySize. Antes o handler ignorava `name` por completo e
		// tratava qualquer tabela como se fosse AccessControlCard.
		e.tracer.Warning("[RecordFinder] tabela desconhecida: %q", name)
		e.respondeErroCodigo(c, http.StatusBadRequest, errCodeUnknownTable)
		return
	}

	switch action {
	case "find":
		if userIDStr := c.Query("condition.UserID"); userIDStr != "" {
			userID, _ := strconv.Atoi(userIDStr)
			response, err := e.repo.FindCard(userID)
			if err != nil {
				e.handleResponse(c, "Error", http.StatusInternalServerError, 60)
				return
			}
			e.handleResponse(c, response, http.StatusOK, 60)
			return
		}

		// Sem condition.UserID: medido na bancada que offset=0, 100, 260, 500
		// e 12000 devolvem TODOS found=50 com o mesmo primeiro registro
		// (RecNo=15) - ou seja, o device real IGNORA offset neste action e so
		// aplica count sobre o inicio da tabela. Antes o handler chamava
		// FindCard(0), que so devolve cartoes do UserID 0 (found=0 sempre).
		count, _ := strconv.Atoi(c.Query("count"))
		response, err := e.repo.GetCards(count, 0)
		if err != nil {
			e.handleResponse(c, "Error", http.StatusInternalServerError, 60)
			return
		}
		e.handleResponse(c, response, http.StatusOK, 60)

	case "doSeekFind":
		// doSeekFind pagina corretamente com offset/count - diferente de
		// find, que ignora offset (ver acima). GetCards já usa LIMIT/OFFSET
		// nativo do Postgres, que termina sozinho na última página parcial e
		// devolve found=0 além do fim da tabela - já correto.
		offset, _ := strconv.Atoi(c.Query("offset"))
		count, _ := strconv.Atoi(c.Query("count"))
		response, err := e.repo.GetCards(count, offset)
		if err != nil {
			e.handleResponse(c, "Error", http.StatusInternalServerError, 350)
			return
		}
		e.handleResponse(c, response, http.StatusOK, 350)

	case "getQuerySize":
		if e.countCardsFn == nil {
			e.handleResponse(c, "Error", http.StatusInternalServerError, 50)
			return
		}
		n, err := e.countCardsFn()
		if err != nil {
			e.handleResponse(c, "Error", http.StatusInternalServerError, 50)
			return
		}
		// Medido na bancada: as DUAS linhas vem no corpo (Size= seguido de
		// count=), nao so count=. Antes so a segunda linha era escrita.
		e.handleResponse(c, fmt.Sprintf("Size=%d\ncount=%d", n, n), http.StatusOK, 50)

	case "startFind", "stopFind":
		// Medido na bancada: 501 puro, nao 400. AccessControlCard nao suporta
		// a paginacao "stateful" desses actions - so doSeekFind/find.
		e.handleResponse(c, "Error\nNot Implemented!", http.StatusNotImplemented, 50)

	default:
		e.handleResponse(c, "Unknown action", http.StatusBadRequest, 50)
	}
}

// handleRecordFinderCardRec atende as consultas ao historico de acessos
// (tabela AccessControlCardRec). O gerenciador manda StartTime/EndTime em
// epoch e le found=N seguido de records[i].* - IoDahuaCommunication.py:852.
func (e *Emulator) handleRecordFinderCardRec(c *gin.Context, action string) {
	if e.eventLog == nil {
		e.handleResponse(c, "found=0", http.StatusOK, 50)
		return
	}

	switch action {
	case "find", "doSeekFind":
		inicioEpoch, _ := strconv.ParseInt(c.Query("StartTime"), 10, 64)
		fimEpoch, _ := strconv.ParseInt(c.Query("EndTime"), 10, 64)
		if fimEpoch == 0 {
			fimEpoch = time.Now().Unix()
		}
		recs := e.eventLog.Between(time.Unix(inicioEpoch, 0), time.Unix(fimEpoch, 0))
		e.tracer.Info("RecordFinder[CardRec]: %d evento(s) entre %d e %d", len(recs), inicioEpoch, fimEpoch)
		e.handleResponse(c, e.eventLog.Render(recs), http.StatusOK, 60)

	case "getQuerySize":
		e.handleResponse(c, fmt.Sprintf("count=%d", e.eventLog.Count()), http.StatusOK, 50)

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

		// O device real apaga a face junto com o cartão — é exatamente o
		// comportamento que motivou o commit 8d86b14 do gerenciador (reenvio
		// proativo da face depois de um update de cartão, que é delete+insert).
		if userID, err := e.repo.GetCardUserIDByRecNo(recNo); err == nil && userID != 0 {
			if err := e.repo.RemoveFace(userID); err != nil {
				e.tracer.Warning("[recordUpdater.remove] face do UserID=%d nao removida: %v", userID, err)
			} else {
				e.tracer.Info("[recordUpdater.remove] face do UserID=%d removida junto com o cartao", userID)
			}
		}

		if err := e.repo.RemoveCard(recNo); err != nil {
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

// maxCardNameBytes é o limite medido na bancada para CardName, em BYTES de
// UTF-8 - não em runas, e o device REJEITA um nome longo demais, não trunca:
//
//	32 x 'A' (32 bytes)          -> aceito, RecNo=<n>
//	33 x 'A' (33 bytes)          -> 400 "Error\nBad Request!"
//	16 x 'á' (32 bytes, 2B/rune) -> aceito
//	32 x 'á' (64 bytes)          -> 400
//	16 'A' + 16 'á' (48 bytes)   -> 400
//
// O antigo maxCardName=31 contava runas, não bytes, e truncava em vez de
// rejeitar - as duas coisas divergem do device real: com truncamento o
// emulador respondia 200 onde o device responde 400, e um bug no
// gerenciador que gerasse um nome longo demais passaria contra o emulador e
// falharia em campo.
//
// O campo é UTF-8 puro e não é sanitizado/normalizado: 'Sonia Acao (teste)',
// 'Sônia Ação' e "José D'Ávila-Neto, Jr." voltaram byte-a-byte idênticos na
// bancada - acento, apóstrofo, parênteses, vírgula, ponto e hífen passam sem
// alteração, contanto que caibam no limite de bytes.
const maxCardNameBytes = 32

func (e *Emulator) handleCardInsert(c *gin.Context) {
	cardName := c.Query("CardName")
	userID, _ := strconv.Atoi(c.Query("UserID"))
	cardNo := c.Query("CardNo")
	validDateStart := c.Query("ValidDateStart")
	validDateEnd := c.Query("ValidDateEnd")

	e.tracer.Info("recordUpdater..insert: CardName=%s, UserID=%d, CardNo=%s", cardName, userID, cardNo)

	if len(cardName) > maxCardNameBytes {
		// Medido na bancada: o device rejeita, não trunca - ver comentário
		// de maxCardNameBytes.
		e.handleResponse(c, "Error\nBad Request!", http.StatusBadRequest, 100)
		return
	}

	// Converter datas
	startTime, _ := time.Parse("2006-01-02 15:04:05", validDateStart)
	endTime, _ := time.Parse("2006-01-02 15:04:05", validDateEnd)

	recNo, err := e.repo.AddCard(cardName, userID, cardNo, startTime, endTime)
	if err != nil {
		switch {
		case errors.Is(err, ErrCardUserIDTaken):
			// Medido na bancada: o device real só aceita UMA linha
			// AccessControlCard por UserID.
			e.respondeErroCodigo(c, http.StatusBadRequest, 286064929)
		case errors.Is(err, ErrCardNoTaken):
			// Medido na bancada: CardNo já pertence a outro UserID.
			e.respondeErroCodigo(c, http.StatusBadRequest, 286064930)
		default:
			e.handleResponse(c, "Error\nBad Request!", http.StatusBadRequest, 100)
		}
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
	e.handleEventStream(conn, bufrw, true)
	e.tracer.Info("=== Event stream handler finished ===")
}

// handleEventManager atende GET /cgi-bin/eventManager.cgi?action=attach.
// E o stream que o gerenciador usa quando NAO esta salvando snapshots
// (IoDahuaCommunication.py:1337). Mesmo transporte do snapManager - multipart
// com boundary "myboundary" sobre conexao hijackada, sem chunked encoding -,
// mas o cliente so espera as parts text/plain. Antes esta rota nao existia e o
// device ficava mudo em modo standalone com snapshot desligado.
func (e *Emulator) handleEventManager(c *gin.Context) {
	action := c.Query("action")
	if action != "attach" {
		e.handleResponse(c, "Unknown action", http.StatusBadRequest, 50)
		return
	}

	e.tracer.Info("=== [GET] /cgi-bin/eventManager.cgi - CLIENT CONNECTED (codes=%s) ===", c.Query("codes"))

	hijacker, ok := c.Writer.(http.Hijacker)
	if !ok {
		e.tracer.Error("Hijacking not supported")
		c.String(http.StatusInternalServerError, "Hijacking not supported")
		return
	}
	conn, bufrw, err := hijacker.Hijack()
	if err != nil {
		e.tracer.Error("Hijack failed: %v", err)
		return
	}
	defer conn.Close()

	httpResponse := "HTTP/1.1 200 OK" + crlf +
		"Cache-Control: no-cache" + crlf +
		"Pragma: no-cache" + crlf +
		"Connection: Keep-Alive" + crlf +
		"Content-Type: multipart/x-mixed-replace; boundary=myboundary" + crlf +
		crlf
	if _, err := bufrw.WriteString(httpResponse); err != nil {
		e.tracer.Error("Failed to write HTTP headers: %v", err)
		return
	}
	bufrw.Flush()

	e.handleEventStream(conn, bufrw, false)
	e.tracer.Info("=== eventManager stream finished ===")
}

// handleEventStream escreve as parts multipart no socket ja hijackado.
// comImagem distingue os dois consumidores: snapManager.cgi espera o par
// text/plain + image/jpeg; eventManager.cgi (usado quando o gerenciador NAO
// salva snapshots) espera so a part de texto.
func (e *Emulator) handleEventStream(conn net.Conn, bufrw *bufio.ReadWriter, comImagem bool) {
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

			// Parts empurradas fora do ciclo normal (hoje: o evento de coleta
			// de digital) saem antes de qualquer outra coisa.
			for drenando := true; drenando; {
				select {
				case part := <-e.pushChan:
					if _, err := bufrw.Write(part); err != nil {
						e.tracer.Error("Failed to write pushed part: %v", err)
						return
					}
					bufrw.Flush()
					e.tracer.Info("[stream] part empurrada: %d bytes", len(part))
					heartbeatCounter = now
				default:
					drenando = false
				}
			}

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

						msgImagePart := 0
						if comImagem {
							time.Sleep(2 * time.Millisecond)

							// Escrever parte da imagem
							msgImagePart, err = bufrw.Write(imagePart)
							if err != nil {
								e.tracer.Error("Failed to write image part: %v", err)
								return
							}
							bufrw.Flush()
						}
						e.tracer.Info("Event sent: %d total bytes (text=%d + image=%d)", msgTextPart+msgImagePart, msgTextPart, msgImagePart)

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
