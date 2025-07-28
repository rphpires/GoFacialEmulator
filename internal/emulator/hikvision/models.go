package hikvision

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
