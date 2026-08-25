package dahua

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// flexInt aceita o inteiro tanto como numero quanto como string JSON. O
// W-Access manda "UserID": "1" no FaceInfoManager e um int puro rejeitaria
// o payload inteiro, descartando a face.
type flexInt int

func (f *flexInt) UnmarshalJSON(data []byte) error {
	texto := string(data)
	if texto == "null" {
		return nil
	}

	if len(texto) > 1 && texto[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		if s == "" {
			return nil
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("UserID %q nao e inteiro: %w", s, err)
		}
		*f = flexInt(n)
		return nil
	}

	var n int
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	*f = flexInt(n)
	return nil
}

// flexStrings aceita uma lista de strings ou uma string solta. Dahua real
// documenta PhotoData como lista, mas gerenciadores mandam as duas formas.
type flexStrings []string

func (f *flexStrings) UnmarshalJSON(data []byte) error {
	texto := string(data)
	if texto == "null" {
		return nil
	}

	if len(texto) > 1 && texto[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*f = flexStrings{s}
		return nil
	}

	var lista []string
	if err := json.Unmarshal(data, &lista); err != nil {
		return err
	}
	*f = flexStrings(lista)
	return nil
}

// Card representa um cartão no sistema Dahua
type Card struct {
	RecNo          int       `db:"rec_no"`
	CardName       string    `db:"card_name"`
	UserID         int       `db:"user_id"`
	CardNo         string    `db:"card_no"`
	ValidDateStart time.Time `db:"valid_date_start"`
	ValidDateEnd   time.Time `db:"valid_date_end"`
}

// Face representa um registro facial no sistema Dahua
type Face struct {
	UserID int    `db:"user_id"`
	MD5    string `db:"md5"`
}

// CountItems representa contadores de diferentes tipos de itens
type CountItems struct {
	Cards int
	Faces int
}

// Event estrutura do evento Dahua
type Event struct {
	Events []EventData `json:"Events"`
	Time   string      `json:"Time"`
}

// EventData estrutura dos dados do evento Dahua
type EventData struct {
	Action          string           `json:"Action"`
	Code            string           `json:"Code"`
	Data            EventDataDetails `json:"Data"`
	Index           int              `json:"Index"`
	PhysicalAddress string           `json:"PhysicalAddress"`
}

// EventDataDetails detalhes específicos do evento (formato igual ao dispositivo real Dahua)
type EventDataDetails struct {
	Alive            int               `json:"Alive,omitempty"`
	BlockId          int               `json:"BlockId,omitempty"`
	CardName         string            `json:"CardName,omitempty"`
	CardNo           string            `json:"CardNo,omitempty"`
	CardStatus       int               `json:"CardStatus,omitempty"`
	CardType         int               `json:"CardType"`
	CreateTime       int64             `json:"CreateTime,omitempty"`
	Door             int               `json:"Door"`
	ErrorCode        int               `json:"ErrorCode"`
	EventGroupID     int               `json:"EventGroupID,omitempty"`
	FaceIndex        int               `json:"FaceIndex,omitempty"`
	FeatureId        int               `json:"FeatureId,omitempty"`
	HatColor         string            `json:"HatColor,omitempty"`
	HatType          int               `json:"HatType,omitempty"`
	ImageInfo        []ImageInfo       `json:"ImageInfo,omitempty"`
	Method           int               `json:"Method"`
	ObjectProperties *ObjectProperties `json:"ObjectProperties,omitempty"`
	ReaderID         string            `json:"ReaderID"`
	RealUTC          int64             `json:"RealUTC,omitempty"`
	Similarity       int               `json:"Similarity,omitempty"`
	SnapPath         string            `json:"SnapPath,omitempty"`
	Status           int               `json:"Status"`
	Type             string            `json:"Type"`
	UTC              int64             `json:"UTC"`
	UserID           string            `json:"UserID"`
	UserType         int               `json:"UserType"`
}

// ObjectProperties propriedades do objeto detectado
type ObjectProperties struct {
	HatInfo *HatInfo `json:"HatInfo,omitempty"`
}

// HatInfo informações sobre chapéu/boné
type HatInfo struct {
	HatColor string `json:"HatColor"`
	HatType  int    `json:"HatType"`
}

// ImageInfo informações da imagem do evento
type ImageInfo struct {
	Height int `json:"Height"`
	Length int `json:"Length"`
	Offset int `json:"Offset"`
	Type   int `json:"Type"`
	Width  int `json:"Width"`
}

// OnlineEvent evento completo para envio ao servidor remoto (formato igual ao dispositivo real Dahua)
type OnlineEvent struct {
	Channel  int         `json:"Channel"`
	Events   []EventData `json:"Events"`
	FilePath string      `json:"FilePath"`
	Time     string      `json:"Time"`
}

// FindFaceResponse resposta da busca de faces
type FindFaceResponse struct {
	Token int `json:"Token"`
	Total int `json:"Total"`
}

// GetFaceResponse resposta da obtenção de faces
type GetFaceResponse struct {
	Info []FaceInfo `json:"Info"`
}

// FaceInfo informações da face
type FaceInfo struct {
	MD5    string `json:"MD5"`
	UserID int    `json:"UserID"`
}
