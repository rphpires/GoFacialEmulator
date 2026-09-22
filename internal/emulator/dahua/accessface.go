package dahua

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Este arquivo cobre AccessFace.cgi e AccessUser.cgi - endpoints medidos na
// bancada (Intelbras SS 5531 MF EX, firmware 3.002.00IB000.0.R) que o
// emulador simplesmente nao respondia antes: qualquer POST para
// AccessFace.cgi?action=insertMulti caia no NoRoute e virava 404, e o
// gerenciador nunca conseguia cadastrar uma face nova por esse caminho.

const (
	// faceMinBytesAproximado NÃO emula "rosto detectável" - isso precisa ser
	// dito sem rodeios: 288686087 é o code que o device real devolve quando
	// a MODELAGEM FACIAL de verdade não encontra rosto na foto; o emulador
	// não tem modelagem facial, só olha o TAMANHO em bytes. Tamanho é um
	// proxy ruim para "tem rosto", e a prova está nas medições abaixo - não
	// existe um único limiar que separe corretamente os casos observados.
	//
	// Medições na bancada e no harness de teste do gateway, da menor para a
	// maior, todas contra o device real:
	//   - 5.425 bytes   (foto cinza 640x480, sem rosto nenhum)         -> 288686087
	//   - 43.530 bytes  (foto 1600x1600 com um rosto de 80x80)         -> 288686087
	//   - 46.352 bytes  (fixture do harness, face_UT-001.jpg, retrato) -> 200 OK
	//   - 100.063 bytes (foto real medida na bancada)                 -> 200 OK
	//   - amostra de produção (40/502 usuários reais): 83.611-131.874 bytes, mediana 105.273
	//
	// O caso de 43.530 é a prova de que tamanho não é detectabilidade: é
	// MAIOR que o fixture de 46.352 bytes que o device aceita, mas foi
	// rejeitado - porque o rosto dentro da foto é pequeno demais, não porque
	// o arquivo é pequeno. Não há valor entre 43.530 e 46.352 que resolva
	// isso; a característica que decide não é o tamanho do arquivo.
	//
	// Diante disso, este limiar serve só para capturar o caso ÓBVIO de "isto
	// nem é uma foto de pessoa" (o flat-gray de 5.425 bytes) e deixar passar
	// qualquer coisa que possa ser um retrato de verdade - incluindo o
	// fixture de 46.352 bytes do harness, que ANTES este limiar rejeitava
	// erroneamente quando estava em 64 KiB.
	//
	// Divergência assumida e documentada, não escondida atrás de um valor
	// "ajustado": com 16 KiB, o emulador ACEITA o caso de 43.530 bytes
	// (rosto pequeno num canvas grande) que o device real REJEITA. Essa
	// divergência específica não é emulada, e não há como resolvê-la sem
	// modelagem facial de verdade - qualquer tentativa de "consertar" isso
	// só por tamanho volta a rejeitar retratos válidos, como o fixture do
	// harness provou.
	faceMinBytesAproximado = 16 * 1024

	// faceMaxBytes é o limite medido na bancada para o JPEG cru (já
	// decodificado do base64) de AccessFace.cgi?action=insertMulti:
	// 384.515 bytes FORAM aceitos; 423.728 FORAM rejeitados com 286064923
	// (também rejeitados: 468.303, 504.901, 522.840, 653.998). O valor aqui,
	// 393.216 = 384 KiB, é o maior N tal que ceil(N/3)*4 <= 524.288 - ou
	// seja, o device parece limitar a STRING base64 a ~512 KiB:
	// ceil(384.515/3)*4 = 512.688 chars (aceito) e ceil(423.728/3)*4 =
	// 564.972 chars (rejeitado) batem exatamente com essa fórmula.
	faceMaxBytes = 384 * 1024

	// faceHardBadRequestBytes é o SEGUNDO limite medido, distinto do de cima:
	// um payload de 957.047 bytes não devolveu o envelope JSON de erro, e
	// sim um 400 puro "Error\nBad Request!" - sinal de que o device tem um
	// limite de parsing/requisição bem maior e por um caminho de código
	// diferente do de faceMaxBytes. O ponto exato onde esse segundo limite
	// começa NÃO foi medido - só sabemos que 653.998 ainda cai no envelope
	// JSON e 957.047 não cai mais. 800 KiB fica no meio do intervalo
	// desconhecido; é uma aproximação documentada, não um valor medido.
	faceHardBadRequestBytes = 800 * 1024

	// Códigos de FailCodes do envelope de erro de insertMulti, todos medidos
	// na bancada.
	faceCodeNoFaceDetected  = 288686087 // sem rosto detectável na foto
	faceCodeTooLarge        = 286064923 // JPEG maior que faceMaxBytes
	faceCodeAlreadyEnrolled = 286064926 // UserID já tem face cadastrada

	// faceBatchErrorCode e faceBatchErrorMessage são o "code"/"message"
	// externos do envelope - constantes em todos os exemplos medidos,
	// independente de qual FailCode foi disparado.
	faceBatchErrorCode    = 268632336
	faceBatchErrorMessage = "Batch Process Error"
)

// requisicaoInsertMulti é o corpo de POST
// AccessFace.cgi?action=insertMulti: {"FaceList":[{"UserID":...,
// "PhotoData":[...]}]}.
type requisicaoInsertMulti struct {
	FaceList []itemFaceList `json:"FaceList"`
}

type itemFaceList struct {
	UserID    flexInt     `json:"UserID"`
	PhotoData flexStrings `json:"PhotoData"`
}

// handleAccessFaceGet atende GET /cgi-bin/AccessFace.cgi.
func (e *Emulator) handleAccessFaceGet(c *gin.Context) {
	switch c.Query("action") {
	case "get":
		// Medido na bancada: 501.
		e.handleResponse(c, "Error\nNot Implemented!", http.StatusNotImplemented, 50)
	case "list":
		e.handleResponse(c, "Error\nBad Request!", http.StatusBadRequest, 50)
	default:
		e.handleResponse(c, "Unknown action", http.StatusBadRequest, 50)
	}
}

// handleAccessFacePost atende POST /cgi-bin/AccessFace.cgi.
func (e *Emulator) handleAccessFacePost(c *gin.Context) {
	switch c.Query("action") {
	case "insertMulti":
		e.handleAccessFaceInsertMulti(c)
	case "updateMulti":
		e.handleAccessFaceUpdateMulti(c)
	case "list":
		// Medido na bancada: TODA tentativa de body (FaceList/UserIDList/
		// UserID/vazio, com ou sem count/offset) devolveu 400 puro. Não há
		// corpo documentado que funcione - o action existe, mas nenhum
		// formato testado o satisfaz.
		e.handleResponse(c, "Error\nBad Request!", http.StatusBadRequest, 50)
	default:
		e.handleResponse(c, "Unknown action", http.StatusBadRequest, 50)
	}
}

// handleAccessUserGet atende GET /cgi-bin/AccessUser.cgi. Só existe hoje
// para reproduzir o 501 de action=remove medido na bancada.
func (e *Emulator) handleAccessUserGet(c *gin.Context) {
	switch c.Query("action") {
	case "remove":
		e.handleResponse(c, "Error\nNot Implemented!", http.StatusNotImplemented, 50)
	default:
		e.handleResponse(c, "Unknown action", http.StatusBadRequest, 50)
	}
}

// handleAccessFaceInsertMulti implementa POST
// AccessFace.cgi?action=insertMulti - o maior gap encontrado na bancada:
// antes desta rota não existir, todo POST aqui virava 404 no NoRoute e o
// gerenciador não conseguia cadastrar face nenhuma por este caminho.
func (e *Emulator) handleAccessFaceInsertMulti(c *gin.Context) {
	e.handleAccessFaceBatch(c, false)
}

// handleAccessFaceUpdateMulti implementa POST
// AccessFace.cgi?action=updateMulti. Medido via o harness de teste do
// gateway: quando insertMulti é rejeitado com 286064926 (UserID já tem
// face), o driver reenvia a MESMA foto como updateMulti, e o device real
// aceita com 200 OK - é assim que o driver troca a foto de um usuário que já
// tem face cadastrada. A única diferença do insertMulti é que updateMulti
// faz upsert (não rejeita por duplicidade); os outros dois motivos de
// rejeição (tamanho grande, sem rosto) continuam valendo.
func (e *Emulator) handleAccessFaceUpdateMulti(c *gin.Context) {
	e.handleAccessFaceBatch(c, true)
}

// handleAccessFaceBatch é a implementação compartilhada de insertMulti e
// updateMulti - o corpo, o parsing e os limites de tamanho são idênticos;
// só a checagem de duplicidade muda (ver validarFace).
func (e *Emulator) handleAccessFaceBatch(c *gin.Context, eAtualizacao bool) {
	var req requisicaoInsertMulti
	if err := c.ShouldBindJSON(&req); err != nil {
		e.tracer.Warning("[AccessFace] bind falhou: %v", err)
		e.handleResponse(c, "Error\nBad Request!", http.StatusBadRequest, 50)
		return
	}

	if len(req.FaceList) == 0 {
		e.handleResponse(c, "Error\nBad Request!", http.StatusBadRequest, 50)
		return
	}

	type entrada struct {
		userID int
		foto   []byte
	}
	entradas := make([]entrada, 0, len(req.FaceList))

	for _, item := range req.FaceList {
		var fotoB64 string
		if len(item.PhotoData) > 0 {
			fotoB64 = item.PhotoData[0]
		}

		// O segundo limite medido (faceHardBadRequestBytes) dispara sobre o
		// tamanho do JPEG cru; estimamos isso a partir do tamanho do
		// base64 para não precisar decodificar um payload gigante só para
		// descobrir que ele vai ser rejeitado.
		if base64.StdEncoding.DecodedLen(len(fotoB64)) > faceHardBadRequestBytes {
			e.tracer.Warning("[AccessFace] PhotoData acima do limite duro (%d bytes estimados)",
				base64.StdEncoding.DecodedLen(len(fotoB64)))
			e.handleResponse(c, "Error\nBad Request!", http.StatusBadRequest, 50)
			return
		}

		foto, err := base64.StdEncoding.DecodeString(fotoB64)
		if err != nil {
			e.tracer.Warning("[AccessFace] PhotoData nao e base64 valido (UserID=%d): %v", int(item.UserID), err)
			e.handleResponse(c, "Error\nBad Request!", http.StatusBadRequest, 50)
			return
		}
		entradas = append(entradas, entrada{userID: int(item.UserID), foto: foto})
	}

	var falhas []int
	for _, en := range entradas {
		codigo, err := e.validarFace(en.userID, en.foto, eAtualizacao)
		if err != nil {
			e.tracer.Error("[AccessFace] validarFace UserID=%d: %v", en.userID, err)
			e.handleResponse(c, "Error", http.StatusInternalServerError, 50)
			return
		}
		if codigo != 0 {
			falhas = append(falhas, codigo)
		}
	}

	if len(falhas) > 0 {
		e.respondeErroBatchFace(c, falhas)
		return
	}

	for _, en := range entradas {
		hash := md5.Sum(en.foto)
		if err := e.repo.AddFace(en.userID, fmt.Sprintf("%X", hash)); err != nil {
			e.tracer.Error("[AccessFace] AddFace UserID=%d: %v", en.userID, err)
			e.handleResponse(c, "Error", http.StatusInternalServerError, 50)
			return
		}
	}

	e.handleResponse(c, "OK", http.StatusOK, 50)
}

// validarFace decide se uma entrada de FaceList deve ser aceita. Devolve
// failCode=0 quando a foto passa em todas as verificações. eAtualizacao
// distingue insertMulti (rejeita duplicidade) de updateMulti (upsert - nunca
// rejeita por já ter face).
//
// A ORDEM abaixo (tamanho grande -> duplicidade -> sem rosto) é uma decisão
// de projeto, não uma medição: os três casos só foram medidos isoladamente
// na bancada, nunca combinados, então a ordem real de verificação do device
// é desconhecida.
//
// 286064922 ("foto borrada") NÃO é emulado aqui: não há como distinguir
// "borrada" de "nítida" sem detecção de qualidade de imagem de verdade, e
// fingir isso com mais um limite de tamanho seria inventar um comportamento
// sem base na medição. Documentado como lacuna, não fingido.
func (e *Emulator) validarFace(userID int, foto []byte, eAtualizacao bool) (failCode int, err error) {
	if len(foto) > faceMaxBytes {
		return faceCodeTooLarge, nil
	}

	if !eAtualizacao {
		existe, err := e.repo.CheckIfFaceExists(userID)
		if err != nil {
			return 0, err
		}
		if existe {
			return faceCodeAlreadyEnrolled, nil
		}
	}

	if len(foto) < faceMinBytesAproximado {
		return faceCodeNoFaceDetected, nil
	}

	return 0, nil
}

// respondeErroCodigo escreve o envelope de erro simples do device real -
// {"code":N,"message":""}, sem quebra de linha final. Usado para "tabela
// desconhecida" no recordFinder (285278242) e para os dois motivos de
// rejeição de recordUpdater.insert (286064929 / 286064930).
func (e *Emulator) respondeErroCodigo(c *gin.Context, status, codigo int) {
	corpo := struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}{Code: codigo, Message: ""}

	raw, err := json.Marshal(corpo)
	if err != nil {
		e.tracer.Error("[respondeErroCodigo] marshal falhou: %v", err)
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Data(status, "application/json; charset=utf-8", raw)
}

// respondeErroBatchFace escreve o envelope de erro de
// AccessFace.cgi?action=insertMulti. codigos são os FailCodes na ordem em
// que as faces do FaceList falharam; FailCount é len(codigos). Formato
// medido na bancada em três cenários distintos (duplicidade, sem rosto,
// tamanho), sempre com o mesmo code/message externos.
func (e *Emulator) respondeErroBatchFace(c *gin.Context, codigos []int) {
	corpo := struct {
		Code   int `json:"code"`
		Detail struct {
			FailCodes []int `json:"FailCodes"`
			FailCount int   `json:"FailCount"`
		} `json:"detail"`
		Message string `json:"message"`
	}{Code: faceBatchErrorCode, Message: faceBatchErrorMessage}
	corpo.Detail.FailCodes = codigos
	corpo.Detail.FailCount = len(codigos)

	raw, err := json.Marshal(corpo)
	if err != nil {
		e.tracer.Error("[respondeErroBatchFace] marshal falhou: %v", err)
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Data(http.StatusBadRequest, "application/json; charset=utf-8", raw)
}
