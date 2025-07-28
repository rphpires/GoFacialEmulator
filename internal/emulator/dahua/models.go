package dahua

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
