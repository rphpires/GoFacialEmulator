package dahua

import "time"

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

// EventDataDetails detalhes específicos do evento
type EventDataDetails struct {
	CardStatus   int         `json:"CardStatus"`
	CardType     int         `json:"CardType"`
	Door         int         `json:"Door"`
	ErrorCode    int         `json:"ErrorCode"`
	EventGroupID int         `json:"EventGroupID"`
	Method       int         `json:"Method"`
	ReadID       string      `json:"ReadID"`
	Status       int         `json:"Status"`
	Type         string      `json:"Type"`
	UTC          int64       `json:"UTC"`
	UserID       int         `json:"UserID"`
	UserType     int         `json:"UserType"`
	ImageInfo    []ImageInfo `json:"ImageInfo,omitempty"`
}

// ImageInfo informações da imagem do evento
type ImageInfo struct {
	Height int `json:"Height"`
	Length int `json:"Length"`
	Offset int `json:"Offset"`
	Type   int `json:"Type"`
	Width  int `json:"Width"`
}

// EventWithImage evento completo com imagem (para eventos online)
type EventWithImage struct {
	Event
	Channel  int    `json:"Channel,omitempty"`
	FilePath string `json:"FilePath,omitempty"`
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
