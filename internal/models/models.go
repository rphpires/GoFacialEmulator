package models

// Device representa um dispositivo de controle de acesso
type Device struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	IPAddress     string `json:"ip_address"`
	Port          int    `json:"port"`
	Model         string `json:"model"`
	Enabled       int    `json:"enabled"`
	Type          int    `json:"type"`
	Status        string `json:"status"`
	EventInterval int    `json:"event_interval"`
	TotalUsers    int    `json:"total_users"`
	LogEnabled    int    `json:"log_enabled"`
}

// UserComparison representa uma comparação de usuários entre WXS, site controller e emulador
type UserComparison struct {
	SiteControllerID    int    `json:"site_controller_id"`
	LocalControllerID   int    `json:"local_controller_id"`
	Name                string `json:"name"`
	Port                int    `json:"port"`
	WxsCount            int    `json:"wxs_count"`
	SiteControllerCount int    `json:"site_controller_count"`
	EmulatorCount       int    `json:"emulator_count"`
}

// DahuaCard representa um cartão no sistema Dahua
type DahuaCard struct {
	RecNo          int    `json:"rec_no"`
	CardName       string `json:"card_name"`
	UserID         int    `json:"user_id"`
	CardNo         string `json:"card_no"`
	ValidDateStart string `json:"valid_date_start"`
	ValidDateEnd   string `json:"valid_date_end"`
}

// DahuaFace representa um registro facial no sistema Dahua
type DahuaFace struct {
	UserID int    `json:"user_id"`
	MD5    string `json:"md5"`
}

// HikvisionUser representa um usuário no sistema Hikvision
type HikvisionUser struct {
	EmployeeNo   string `json:"employee_no"`
	Name         string `json:"name"`
	Password     string `json:"password"`
	LocalUIRight string `json:"local_ui_right"`
	BeginTime    string `json:"begin_time"`
	EndTime      string `json:"end_time"`
}

// HikvisionCard representa um cartão no sistema Hikvision
type HikvisionCard struct {
	EmployeeNo string `json:"employee_no"`
	CardNo     string `json:"card_no"`
}

// HikvisionFace representa um registro facial no sistema Hikvision
type HikvisionFace struct {
	UserID    int    `json:"user_id"`
	PhotoData string `json:"photo_data"`
}

// HikvisionFinger representa uma impressão digital no sistema Hikvision
type HikvisionFinger struct {
	CHID      int    `json:"chid"`
	DataIndex int    `json:"data_index"`
	Template  string `json:"template"`
}
