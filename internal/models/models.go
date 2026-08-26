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
	// Source separa o que veio do W-Access ("wxs") do que foi cadastrado
	// pela API ou pela tela ("manual"). O sync só apaga o primeiro.
	Source string `json:"source"`
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
