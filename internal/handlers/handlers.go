package handlers

import (
	"context"
	"net/http"
	"strconv"
	"text/template"
	"time"

	"GoFacialEmulator/internal/database"
	"GoFacialEmulator/internal/emulator"
	"GoFacialEmulator/internal/trace"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// Handler gerencia todas as rotas HTTP - baseado no EmulatorService.py
type Handler struct {
	manager   *emulator.Manager
	serviceDB *database.SimpleOptimizedPool
	wxsDB     *database.WxsDB
	tracer    *trace.Tracer
	upgrader  websocket.Upgrader
}

// NewHandler cria uma nova instância de Handler
func NewHandler(manager *emulator.Manager, serviceDB *database.SimpleOptimizedPool, wxsDB *database.WxsDB, tracer *trace.Tracer) *Handler {
	return &Handler{
		manager:   manager,
		serviceDB: serviceDB,
		wxsDB:     wxsDB,
		tracer:    tracer,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Permitir todas as origens em desenvolvimento
			},
		},
	}
}

// Router configura e retorna o router HTTP
func (h *Handler) Router() http.Handler {
	router := gin.Default()

	// Configurar middleware
	router.Use(h.loggingMiddleware())
	router.Use(gin.Recovery())
	router.Use(h.corsMiddleware())

	// Servir arquivos estáticos
	router.Static("/static", "./web/static")
	// Definir funções auxiliares para templates
	// funcMap := template.FuncMap{
	// 	"sub": func(a, b int) int { return a - b },
	// 	"add": func(a, b int) int { return a + b },
	// 	"le":  func(a, b int) bool { return a <= b },
	// 	"ge":  func(a, b int) bool { return a >= b },
	// 	"lt":  func(a, b int) bool { return a < b },
	// 	"gt":  func(a, b int) bool { return a > b },
	// 	"eq":  func(a, b interface{}) bool { return a == b },
	// 	"ne":  func(a, b interface{}) bool { return a != b },
	// }

	// files := []string{
	// 	"web/templates/base.html",
	// 	"web/templates/devices.html",
	// 	"web/templates/comparison.html",
	// 	"web/templates/header.html",
	// 	"web/templates/footer.html",
	// 	"web/templates/sidebar.html",
	// 	"web/templates/metrics.html",
	// 	"web/templates/pagination.html",
	// }

	// tmpl := template.Must(template.New("").Funcs(funcMap).ParseFiles(files...))
	// router.SetHTMLTemplate(tmpl)

	// Rotas da interface web - baseadas no EmulatorService.py
	h.setupWebRoutes(router)

	// Rotas da API REST
	h.setupAPIRoutes(router)

	// WebSocket para atualizações em tempo real
	router.GET("/ws", h.handleWebSocket)

	// Rota de saúde
	router.GET("/health", h.healthCheck)

	return router
}

func (h *Handler) loadTemplate(templateName string) *template.Template {
	funcMap := template.FuncMap{
		"sub": func(a, b int) int { return a - b },
		"add": func(a, b int) int { return a + b },
		"le":  func(a, b int) bool { return a <= b },
		"ge":  func(a, b int) bool { return a >= b },
		"lt":  func(a, b int) bool { return a < b },
		"gt":  func(a, b int) bool { return a > b },
		"eq":  func(a, b interface{}) bool { return a == b },
		"ne":  func(a, b interface{}) bool { return a != b },
	}

	baseFiles := []string{
		"web/templates/base.html",
		"web/templates/header.html",
		"web/templates/footer.html",
		"web/templates/sidebar.html",
		"web/templates/metrics.html",
		"web/templates/pagination.html",
	}

	files := append(baseFiles, templateName)
	return template.Must(template.New("").Funcs(funcMap).ParseFiles(files...))
}

// setupWebRoutes configura rotas da interface web
func (h *Handler) setupWebRoutes(router *gin.Engine) {
	// Página principal - equivalente ao main_page() do Python
	router.Any("/", h.mainPage)

	// Ações de controle
	router.POST("/start", h.startEmulators)
	router.POST("/stop", h.stopEmulators)
	router.GET("/refresh", h.refreshDevices)
	router.GET("/recreate", h.recreateEmulators) // Placeholder por enquanto

	// Página de comparação
	router.GET("/comparison", h.comparisonPage)
	router.GET("/comparison_refresh", h.comparisonRefresh)
}

// setupAPIRoutes configura rotas da API
func (h *Handler) setupAPIRoutes(router *gin.Engine) {
	api := router.Group("/api")
	{
		// Controle de emuladores
		emulators := api.Group("/emulators")
		{
			emulators.GET("/start", h.apiStartEmulators)
			emulators.GET("/stop", h.apiStopEmulators)
			emulators.GET("/refresh", h.apiRefreshEmulators)
		}

		// Gerenciamento de dispositivos
		devices := api.Group("/devices")
		{
			devices.GET("", h.listDevices)
			devices.GET("/:id", h.getDevice)
			devices.POST("/:id/start", h.startSingleDevice)
			devices.POST("/:id/stop", h.stopSingleDevice)
			devices.PUT("/:id/settings", h.updateDeviceSettings)
		}

		// Status do sistema
		api.GET("/status", h.getSystemStatus)
		api.GET("/comparison", h.getUserComparisons)
	}
}

// middlewares
func (h *Handler) loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		latency := time.Since(start)
		h.tracer.Info("HTTP %s %s - %d - %v",
			c.Request.Method,
			c.Request.URL.Path,
			c.Writer.Status(),
			latency)
	}
}

func (h *Handler) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// Web Interface Handlers

// mainPage gerencia a página principal - baseado no main_page() do Python
func (h *Handler) mainPage(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "10"))

	devices, deviceStatusOk, err := h.getCurrentDevices()
	h.tracer.Info("## Before MainPage, devices: %s", devices)
	if err != nil {
		h.tracer.Error("Failed to get current devices: %v", err)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": err.Error()})
		return
	}

	h.tracer.Info("DEBUG: Total devices found: %d", len(devices))
	_, comparisonStatusOk, err := h.getComparisonPageContent()
	if err != nil {
		h.tracer.Error("Failed to get comparison content: %v", err)
		comparisonStatusOk = 0
	}

	// Paginação
	totalDevices := len(devices)
	start := (page - 1) * perPage
	end := start + perPage
	if end > totalDevices {
		end = totalDevices
	}

	var paginatedDevices []map[string]interface{}
	if start < totalDevices {
		paginatedDevices = devices[start:end]
	}

	totalPages := (totalDevices + perPage - 1) / perPage

	qntTotalEmulators := len(devices)
	counterCards := map[string]interface{}{
		"total":                   qntTotalEmulators,
		"comparison_status_ok":    comparisonStatusOk,
		"comparison_status_error": qntTotalEmulators - comparisonStatusOk,
		"running":                 deviceStatusOk,
		"stopped":                 qntTotalEmulators - deviceStatusOk,
	}

	h.tracer.Info("##: paginatedDevices: %s", paginatedDevices)

	context := gin.H{
		"devices":       paginatedDevices,
		"page":          page,
		"total_pages":   totalPages,
		"per_page":      perPage,
		"counter_cards": counterCards,
	}

	tmpl := h.loadTemplate("web/templates/devices.html")
	tmpl.ExecuteTemplate(c.Writer, "base.html", context)
}

// startEmulators inicia emuladores selecionados
func (h *Handler) startEmulators(c *gin.Context) {
	var requestBody struct {
		Devices   []string        `json:"devices"`
		EnableLog map[string]bool `json:"enable_log"`
	}

	if err := c.ShouldBindJSON(&requestBody); err != nil {
		h.tracer.Error("Failed to bind JSON: %v", err)
		c.Redirect(http.StatusSeeOther, "/")
		return
	}

	h.tracer.Info(">>> Starting Emulators")

	// Atualizar configurações de log
	h.updateLogEnabled(requestBody.EnableLog)

	h.tracer.Info("## Start Emulators - Devices received: %+v", requestBody.Devices)

	// Iniciar emuladores
	for _, deviceStr := range requestBody.Devices {
		if deviceStr == "all" {
			h.manager.StartAll()
			break
		}

		h.tracer.Info("## strconv.Atoi deviceID: %s", deviceStr)
		deviceID, err := strconv.Atoi(deviceStr)
		if err != nil {
			h.tracer.Error("Invalid device ID: %s", deviceStr)
			continue
		}

		h.tracer.Info("## Starting single deviceID: %s", deviceStr)
		if err := h.manager.Start(deviceID); err != nil {
			h.tracer.Error("Failed to start device %d: %v", deviceID, err)
		}
	}

	c.Redirect(http.StatusSeeOther, "/")
}

// stopEmulators para emuladores selecionados
func (h *Handler) stopEmulators(c *gin.Context) {
	var requestBody struct {
		Devices []string `json:"devices"`
	}

	if err := c.ShouldBindJSON(&requestBody); err != nil {
		h.tracer.Error("Failed to bind JSON: %v", err)
		c.Redirect(http.StatusSeeOther, "/")
		return
	}

	h.tracer.Info(">>> Stopping Emulators")

	for _, deviceStr := range requestBody.Devices {
		if deviceStr == "all" {
			h.manager.StopAll()
			break
		}

		deviceID, err := strconv.Atoi(deviceStr)
		if err != nil {
			h.tracer.Error("Invalid device ID: %s", deviceStr)
			continue
		}

		if err := h.manager.Stop(deviceID); err != nil {
			h.tracer.Error("Failed to stop device %d: %v", deviceID, err)
		}
	}

	c.Redirect(http.StatusSeeOther, "/")
}

// refreshDevices atualiza lista de dispositivos
func (h *Handler) refreshDevices(c *gin.Context) {
	h.tracer.Info(">>> Refreshing database")

	if err := h.manager.RefreshDevices(); err != nil {
		h.tracer.Error("Failed to refresh devices: %v", err)
	}

	c.Redirect(http.StatusSeeOther, "/")
}

// recreateEmulators recria executáveis (placeholder por enquanto)
func (h *Handler) recreateEmulators(c *gin.Context) {
	h.tracer.Info(">>> Recreating emulator executable: BEGIN")
	// TODO: Implementar lógica de recriação se necessário
	h.tracer.Info(">>> Recreating emulator executable: END")
	c.Redirect(http.StatusSeeOther, "/")
}

// comparisonPage mostra página de comparação
func (h *Handler) comparisonPage(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "10"))

	devices, deviceStatusOk, err := h.getCurrentDevices()
	if err != nil {
		h.tracer.Error("Failed to get current devices: %v", err)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": err.Error()})
		return
	}

	countValues, comparisonStatusOk, err := h.getComparisonPageContent()
	if err != nil {
		h.tracer.Error("Failed to get comparison content: %v", err)
		countValues = []map[string]interface{}{}
		comparisonStatusOk = 0
	}

	qntTotalEmulators := len(devices)
	counterCards := map[string]interface{}{
		"total":                   qntTotalEmulators,
		"comparison_status_ok":    comparisonStatusOk,
		"comparison_status_error": qntTotalEmulators - comparisonStatusOk,
		"running":                 deviceStatusOk,
		"stopped":                 qntTotalEmulators - deviceStatusOk,
	}

	// Paginação
	start := (page - 1) * perPage
	end := start + perPage
	if end > len(countValues) {
		end = len(countValues)
	}

	var paginatedValues []map[string]interface{}
	if start < len(countValues) {
		paginatedValues = countValues[start:end]
	}

	totalPages := (len(countValues) + perPage - 1) / perPage

	context := gin.H{
		"values":        paginatedValues,
		"page":          page,
		"total_pages":   totalPages,
		"per_page":      perPage,
		"counter_cards": counterCards,
	}

	tmpl := h.loadTemplate("web/templates/comparison.html")
	tmpl.ExecuteTemplate(c.Writer, "base.html", context)
}

// comparisonRefresh atualiza dados de comparação
func (h *Handler) comparisonRefresh(c *gin.Context) {
	h.refreshUsersComparison()
	c.Redirect(http.StatusSeeOther, "/comparison")
}

// API Handlers

// apiStartEmulators inicia todos os emuladores via API
func (h *Handler) apiStartEmulators(c *gin.Context) {
	h.tracer.Info(">>> Starting Emulators")

	if err := h.manager.StartAll(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"response": "Start Emulators command: OK"})
}

// apiStopEmulators para todos os emuladores via API
func (h *Handler) apiStopEmulators(c *gin.Context) {
	h.tracer.Info(">>> Stopping Emulators")
	h.manager.StopAll()
	c.JSON(http.StatusOK, gin.H{"response": "Stop Emulators command: OK"})
}

// apiRefreshEmulators atualiza emuladores via API
func (h *Handler) apiRefreshEmulators(c *gin.Context) {
	h.tracer.Info("Refreshing Emulators")

	if err := h.manager.RefreshDevices(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"response": "Refresh Emulators command: OK"})
}

// listDevices lista todos os dispositivos
func (h *Handler) listDevices(c *gin.Context) {
	devices, err := h.manager.ListDevices()
	if err != nil {
		h.tracer.Error("Failed to list devices: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve devices"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"devices": devices,
		"count":   len(devices),
	})
}

// getDevice obtém informações de um dispositivo específico
func (h *Handler) getDevice(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid device ID"})
		return
	}

	device, err := h.manager.GetDevice(id)
	if err != nil {
		h.tracer.Error("Failed to get device %d: %v", id, err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Device not found"})
		return
	}

	c.JSON(http.StatusOK, device)
}

// startSingleDevice inicia um dispositivo específico
func (h *Handler) startSingleDevice(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid device ID"})
		return
	}
	h.tracer.Info("startSingleDevice: device_port=%d", id)

	if err := h.manager.Start(id); err != nil {
		h.tracer.Error("Failed to start device %d: %v", id, err)
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	h.tracer.Info("Device %d started successfully", id)
	c.JSON(http.StatusOK, gin.H{"message": "Device started successfully"})
}

// stopSingleDevice para um dispositivo específico
func (h *Handler) stopSingleDevice(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid device ID"})
		return
	}

	if err := h.manager.Stop(id); err != nil {
		h.tracer.Error("Failed to stop device %d: %v", id, err)
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	h.tracer.Info("Device %d stopped successfully", id)
	c.JSON(http.StatusOK, gin.H{"message": "Device stopped successfully"})
}

// updateDeviceSettings atualiza configurações de um dispositivo
func (h *Handler) updateDeviceSettings(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid device ID"})
		return
	}

	var request struct {
		LogEnabled bool `json:"log_enabled"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if err := h.manager.UpdateDeviceSettings(id, request.LogEnabled); err != nil {
		h.tracer.Error("Failed to update device settings %d: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update settings"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Settings updated successfully"})
}

// getSystemStatus retorna status geral do sistema
func (h *Handler) getSystemStatus(c *gin.Context) {
	devices, err := h.manager.ListDevices()
	if err != nil {
		h.tracer.Error("Failed to get system status: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve system status"})
		return
	}

	runningCount := 0
	for _, device := range devices {
		if device.Status == "running" {
			runningCount++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"total_devices":   len(devices),
		"running_devices": runningCount,
		"stopped_devices": len(devices) - runningCount,
		"timestamp":       time.Now().UTC(),
	})
}

// getUserComparisons retorna comparações de usuários
func (h *Handler) getUserComparisons(c *gin.Context) {
	// Primeiro, atualizar as comparações
	h.refreshUsersComparison()

	// currentValues, comparisonStatusOk, nil := h.getComparisonPageContent()
	comparisons, _, err := h.getComparisonPageContent()
	if err != nil {
		h.tracer.Error("Failed to get user comparisons: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve user comparisons"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"comparisons": comparisons,
		"count":       len(comparisons),
	})
}

// healthCheck verifica saúde do sistema
func (h *Handler) healthCheck(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Verificar conexão com banco de dados
	err := h.serviceDB.Ping(ctx)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "unhealthy",
			"error":  "database connection failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"timestamp": time.Now().UTC(),
		"version":   "1.0.0",
	})
}

// WebSocket Handler para atualizações em tempo real
func (h *Handler) handleWebSocket(c *gin.Context) {
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.tracer.Error("Failed to upgrade WebSocket: %v", err)
		return
	}
	defer conn.Close()

	h.tracer.Info("WebSocket client connected")

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Esta é a forma mais simples que resolve o warning
	for range ticker.C {
		devices, deviceStatusOk, err := h.getCurrentDevices()
		if err != nil {
			h.tracer.Error("Failed to get devices for WebSocket: %v", err)
			continue
		}

		update := map[string]interface{}{
			"type":          "status_update",
			"devices":       devices,
			"running_count": deviceStatusOk,
			"timestamp":     time.Now().UTC(),
		}

		conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := conn.WriteJSON(update); err != nil {
			h.tracer.Error("Failed to write WebSocket message: %v", err)
			return
		}
	}
}

// getCurrentDevices obtém dispositivos atuais - baseado no get_current_devices() do Python
func (h *Handler) getCurrentDevices() ([]map[string]interface{}, int, error) {
	devices, err := h.manager.ListDevices()
	if err != nil {
		return nil, 0, err
	}

	var currentDevices []map[string]interface{}
	deviceStatusRunning := 0

	for _, device := range devices {
		if device.Status == "running" {
			deviceStatusRunning++
		}

		currentDevices = append(currentDevices, map[string]interface{}{
			"lc_id":       device.ID,
			"name":        device.Name,
			"ip_address":  device.IPAddress,
			"port":        device.Port,
			"log_enabled": device.LogEnabled,
			"model":       device.Model,
			"status":      device.Status,
			"enabled":     device.Enabled,
			"interval":    device.EventInterval,
			"total":       device.TotalUsers,
		})
	}

	return currentDevices, deviceStatusRunning, nil
}

// updateLogEnabled atualiza configurações de log - baseado no update_log_enabled() do Python
func (h *Handler) updateLogEnabled(devices map[string]bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for portStr, enabled := range devices {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			h.tracer.Error("Invalid port: %s", portStr)
			continue
		}

		logEnabled := 0
		if enabled {
			logEnabled = 1
		}

		query := "UPDATE service.devices SET log_enabled = $1 WHERE port = $2"
		_, err = h.serviceDB.Exec(ctx, query, logEnabled, port)
		if err != nil {
			h.tracer.Error("Failed to update log setting for port %d: %v", port, err)
		}
	}
}

// getComparisonPageContent obtém dados de comparação - baseado no get_comparison_page_content() do Python
func (h *Handler) getComparisonPageContent() ([]map[string]interface{}, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `
		SELECT 
			uc.site_controller_id, 
			uc.local_controller_id,
			d.name, 
			uc.base_comm_port,
			uc.wxs_count, 
			uc.site_controller_count, 
			d.total_users 
		FROM service.users_comparison uc
		JOIN service.devices d ON d.local_controller_id = uc.local_controller_id
	`

	rows, err := h.serviceDB.Query(ctx, query)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var currentValues []map[string]interface{}
	comparisonStatusOk := 0

	for rows.Next() {
		var siteControllerID, localControllerID, port, wxsTotal, siteControllerTotal, emTotal int
		var name string

		err := rows.Scan(&siteControllerID, &localControllerID, &name, &port, &wxsTotal, &siteControllerTotal, &emTotal)
		if err != nil {
			h.tracer.Error("Failed to scan comparison row: %v", err)
			continue
		}

		if wxsTotal == siteControllerTotal && siteControllerTotal == emTotal {
			comparisonStatusOk++
		}

		currentValues = append(currentValues, map[string]interface{}{
			"site_controller_id":    siteControllerID,
			"local_controller_id":   localControllerID,
			"name":                  name,
			"port":                  port,
			"wxs_total":             wxsTotal,
			"site_controller_total": siteControllerTotal,
			"emulator_total":        emTotal,
		})
	}

	return currentValues, comparisonStatusOk, nil
}

// refreshUsersComparison atualiza comparação de usuários - baseado no refresh_users_comparison() do Python
func (h *Handler) refreshUsersComparison() {
	h.tracer.Info("Refreshing users comparison")

	// Obter contagens do WXS
	counts, err := h.wxsDB.CountCHIDsByLocalController()
	if err != nil {
		h.tracer.Error("Failed to count CHIDs: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Atualizar tabela de comparação
	for siteControllerID, lcCounts := range counts {
		for lcID, info := range lcCounts {
			port := info[0].(int)
			count := info[1].(int)

			// Verificar se registro existe
			var exists int
			err := h.serviceDB.QueryRow(ctx,
				"SELECT COUNT(*) FROM service.users_comparison WHERE local_controller_id = $1",
				lcID).Scan(&exists)

			if err != nil {
				h.tracer.Error("Failed to check user count existence: %v", err)
				continue
			}

			if exists > 0 {
				// Atualizar registro existente
				_, err = h.serviceDB.Exec(ctx, `
					UPDATE service.users_comparison 
					SET wxs_count = $1, site_controller_id = $2, base_comm_port = $3 
					WHERE local_controller_id = $4`,
					count, siteControllerID, port, lcID)
			} else {
				// Inserir novo registro
				_, err = h.serviceDB.Exec(ctx, `
					INSERT INTO service.users_comparison
					(site_controller_id, local_controller_id, base_comm_port, wxs_count, site_controller_count) 
					VALUES ($1, $2, $3, $4, 0)`,
					siteControllerID, lcID, port, count)
			}

			if err != nil {
				h.tracer.Error("Failed to update user count: %v", err)
			}
		}
	}

	// TODO: Atualizar contagens do site controller se necessário
	h.tracer.Info("Users comparison refreshed successfully")
}
