package hikvision

import "time"

// User representa um usuário no sistema Hikvision
type User struct {
	EmployeeNo   string    `db:"employee_no"`
	Name         string    `db:"name"`
	Password     string    `db:"password"`
	LocalUIRight string    `db:"local_ui_right"`
	BeginTime    time.Time `db:"begin_time"`
	EndTime      time.Time `db:"end_time"`
}

// Card representa um cartão no sistema Hikvision
type Card struct {
	EmployeeNo string `db:"employee_no"`
	CardNo     string `db:"card_no"`
}

// Face representa um registro facial no sistema Hikvision
type Face struct {
	UserID    int    `db:"user_id"`
	PhotoData string `db:"photo_data"`
}

// Finger representa uma impressão digital no sistema Hikvision
type Finger struct {
	CHID      int    `db:"chid"`
	DataIndex int    `db:"data_index"`
	Template  string `db:"template"`
}

// CountItems representa contadores de diferentes tipos de itens
type CountItems struct {
	Users        int
	Cards        int
	Faces        int
	Fingerprints int
}

// UserSearchResponse estrutura para resposta de busca de usuários
type UserSearchResponse struct {
	UserInfoSearch struct {
		SearchID           string `json:"searchID"`
		ResponseStatusStrg string `json:"responseStatusStrg"`
		NumOfMatches       int    `json:"numOfMatches"`
		TotalMatches       int    `json:"totalMatches"`
		UserInfo           []UserInfo `json:"UserInfo"`
	} `json:"UserInfoSearch"`
}

// UserInfo estrutura para informações do usuário na resposta
type UserInfo struct {
	EmployeeNo         string `json:"employeeNo"`
	Name               string `json:"name"`
	UserType           string `json:"userType"`
	SortByNamePosition int    `json:"sortByNamePosition"`
	SortByNameFlag     string `json:"sortByNameFlag"`
	CloseDelayEnabled  bool   `json:"closeDelayEnabled"`
	Valid              struct {
		Enable    bool   `json:"enable"`
		BeginTime string `json:"beginTime"`
		EndTime   string `json:"endTime"`
		TimeType  string `json:"timeType"`
	} `json:"Valid"`
	BelongGroup  string `json:"belongGroup"`
	Password     string `json:"password"`
	DoorRight    string `json:"doorRight"`
	RightPlan    []struct {
		DoorNo         int    `json:"doorNo"`
		PlanTemplateNo string `json:"planTemplateNo"`
	} `json:"RightPlan"`
	MaxOpenDoorTime    int    `json:"maxOpenDoorTime"`
	OpenDoorTime       int    `json:"openDoorTime"`
	RoomNumber         int    `json:"roomNumber"`
	FloorNumber        int    `json:"floorNumber"`
	LocalUIRight       bool   `json:"localUIRight"`
	Gender             string `json:"gender"`
	NumOfCard          int    `json:"numOfCard"`
	NumOfRemoteControl int    `json:"numOfRemoteControl"`
	NumOfFP            int    `json:"numOfFP"`
	NumOfFace          int    `json:"numOfFace"`
	PersonInfoExtends  []struct {
		Value string `json:"value"`
	} `json:"PersonInfoExtends"`
}

// CardSearchResponse estrutura para resposta de busca de cartões
type CardSearchResponse struct {
	CardInfoSearch struct {
		SearchID           string     `json:"searchID"`
		ResponseStatusStrg string     `json:"responseStatusStrg"`
		NumOfMatches       int        `json:"numOfMatches"`
		TotalMatches       int        `json:"totalMatches"`
		CardInfo           []CardInfo `json:"CardInfo"`
	} `json:"CardInfoSearch"`
}

// CardInfo estrutura para informações do cartão na resposta
type CardInfo struct {
	EmployeeNo               string `json:"employeeNo"`
	CardNo                   string `json:"cardNo"`
	IsCardAsRemoteControlBtn bool   `json:"isCardAsRemoteControlBtn"`
	LeaderCard               string `json:"leaderCard"`
	CardType                 string `json:"cardType"`
}

// FaceSearchResponse estrutura para resposta de busca de faces
type FaceSearchResponse struct {
	SearchID           string     `json:"searchID"`
	ResponseStatusStrg string     `json:"responseStatusStrg"`
	NumOfMatches       int        `json:"numOfMatches"`
	TotalMatches       int        `json:"totalMatches"`
	MatchList          []FaceInfo `json:"MatchList"`
}

// FaceInfo estrutura para informações da face na resposta
type FaceInfo struct {
	FPID      string `json:"FPID"`
	FaceURL   string `json:"faceURL"`
	ModelData string `json:"modelData"`
}

// Event estrutura do evento Hikvision
type Event struct {
	IPAddress        string `json:"ipAddress"`
	IPv6Address      string `json:"ipv6Address"`
	PortNo           int    `json:"portNo"`
	Protocol         string `json:"protocol"`
	MacAddress       string `json:"macAddress"`
	ChannelID        int    `json:"channelID"`
	DateTime         string `json:"dateTime"`
	ActivePostCount  int    `json:"activePostCount"`
	EventType        string `json:"eventType"`
	EventState       string `json:"eventState"`
	EventDescription string `json:"eventDescription"`
	AccessControllerEvent struct {
		DeviceName          string  `json:"deviceName"`
		MajorEventType      int     `json:"majorEventType"`
		SubEventType        int     `json:"subEventType"`
		CardNo              string  `json:"cardNo"`
		CardType            int     `json:"cardType"`
		Name                string  `json:"name"`
		CardReaderKind      int     `json:"cardReaderKind"`
		CardReaderNo        int     `json:"cardReaderNo"`
		VerifyNo            int     `json:"verifyNo"`
		EmployeeNoString    string  `json:"employeeNoString"`
		SerialNo            int     `json:"serialNo"`
		UserType            string  `json:"userType"`
		CurrentVerifyMode   string  `json:"currentVerifyMode"`
		CurrentEvent        bool    `json:"currentEvent"`
		// FrontSerialNo/StatusValue são ponteiros para permitir omissão real
		// (nil) no evento de remote-check request — o dispositivo real, em modo
		// online, envia o evento SEM frontSerialNo e SEM statusValue (indeciso).
		// A ausência de frontSerialNo é justamente o gatilho que faz o SC entrar
		// no path de remote check (IoHikvisionCommunication.py: `if not
		// event_data.get("frontSerialNo")`). Ver [[handleRemoteCheck]].
		FrontSerialNo       *int    `json:"frontSerialNo,omitempty"`
		AttendanceStatus    string  `json:"attendanceStatus"`
		Label               string  `json:"label"`
		StatusValue         *int    `json:"statusValue,omitempty"`
		// RemoteCheck marca o evento como requisição de verificação remota
		// (device indeciso, aguardando PUT /ISAPI/AccessControl/remoteCheck).
		RemoteCheck         bool    `json:"remoteCheck,omitempty"`
		Mask                string  `json:"mask"`
		Helmet              string  `json:"helmet"`
		PicturesNumber      int     `json:"picturesNumber"`
		PurePwdVerifyEnable bool    `json:"purePwdVerifyEnable"`
		FaceRect            struct {
			Height float64 `json:"height"`
			Width  float64 `json:"width"`
			X      float64 `json:"x"`
			Y      float64 `json:"y"`
		} `json:"FaceRect"`
		UnlockRoomNo string `json:"unlockRoomNo"`
	} `json:"AccessControllerEvent"`
}