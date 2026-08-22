package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"text/template"
	"time"

	"GoFacialEmulator/assets"
	"GoFacialEmulator/internal/config"
	"GoFacialEmulator/internal/database"
	"GoFacialEmulator/internal/emulator"
	"GoFacialEmulator/internal/monitoring"
	"GoFacialEmulator/internal/trace"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// Handler gerencia todas as rotas HTTP - baseado no EmulatorService.py
type Handler struct {
	manager       *emulator.Manager
	serviceDB     database.DBInterface // Pode ser AdaptivePool ou DualPoolManager
	wxsDB         *database.WxsDB
	appVersion    string
	tracer        *trace.Tracer
	upgrader      websocket.Upgrader
	healthMonitor *monitoring.HealthMonitor
	metrics       *monitoring.Metrics
}

// NewHandler cria uma nova instância de Handler
func NewHandler(manager *emulator.Manager, serviceDB database.DBInterface, wxsDB *database.WxsDB, appVersion string, tracer *trace.Tracer) *Handler {
	// Criar sistema de monitoramento
	healthMonitor := monitoring.NewHealthMonitor()
	metrics := monitoring.NewMetrics()

	// Registrar health checkers
	healthMonitor.RegisterChecker(monitoring.NewDatabaseHealthChecker(
		"service_db",
		func(ctx context.Context) error { return serviceDB.Ping(ctx) },
		func() map[string]interface{} {
			if dpm, ok := serviceDB.(*database.DualPoolManager); ok {
				return dpm.GetStats()
			} else if ap, ok := serviceDB.(*database.AdaptivePool); ok {
				return ap.GetStats()
			}
			return map[string]interface{}{"status": "unknown"}
		},
	))

	healthMonitor.RegisterChecker(monitoring.NewDatabaseHealthChecker(
		"emulator_db",
		func(ctx context.Context) error { return manager.EmulatorDB.Ping(ctx) },
		func() map[string]interface{} {
			if dpm, ok := manager.EmulatorDB.(*database.DualPoolManager); ok {
				return dpm.GetStats()
			} else if ap, ok := manager.EmulatorDB.(*database.AdaptivePool); ok {
				return ap.GetStats()
			}
			return map[string]interface{}{"status": "unknown"}
		},
	))

	registerWxsChecker(healthMonitor, wxsDB)

	healthMonitor.RegisterChecker(monitoring.NewEmulatorHealthChecker(
		func() ([]map[string]interface{}, error) {
			devices, err := manager.ListDevices()
			if err != nil {
				return nil, err
			}
			result := make([]map[string]interface{}, len(devices))
			for i, d := range devices {
				result[i] = map[string]interface{}{
					"id":     d.ID,
					"status": d.Status,
					"name":   d.Name,
				}
			}
			return result, nil
		},
	))

	return &Handler{
		manager:       manager,
		serviceDB:     serviceDB,
		wxsDB:         wxsDB,
		appVersion:    appVersion,
		tracer:        tracer,
		healthMonitor: healthMonitor,
		metrics:       metrics,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Permitir todas as origens em desenvolvimento
			},
		},
	}
}

// registerWxsChecker registra o health checker de wxs_db, mas só quando
// wxsDB está de fato configurado (não nil) — cmd/emulator-service/main.go
// deixa wxsDB nil quando o host efetivo (settings do banco, com fallback
// pro config.yaml) está vazio, que é o estado normal de uma instalação nova
// antes de o cliente configurar em /settings. Sem essa checagem, uma
// instalação sem WXS nunca teria como aparecer saudável.
//
// wxs_db é uma dependência opcional (W-Access externo): os emuladores
// continuam servindo dispositivos sem ele, só refresh/comparação degradam.
// Por isso o ping falho reporta "degraded", não "unhealthy" — não deve
// derrubar o status geral (e o HTTP de /monitoring/health/quick) para um
// cliente com WXS configurado mas temporariamente fora do ar. Extraído do
// corpo de NewHandler para ser testável sem precisar construir um Handler
// inteiro (veja handlers_test.go).
func registerWxsChecker(hm *monitoring.HealthMonitor, wxsDB *database.WxsDB) {
	if wxsDB == nil {
		return
	}
	hm.RegisterChecker(monitoring.NewDatabaseHealthCheckerWithFailureStatus(
		"wxs_db",
		func(ctx context.Context) error { return wxsDB.Ping(ctx) },
		func() map[string]interface{} { return map[string]interface{}{"connected": true} },
		monitoring.HealthStatusDegraded,
	))
}

func (h *Handler) withBaseContext(extra gin.H) gin.H {
	context := gin.H{
		"app_version": h.appVersion,
	}

	for key, value := range extra {
		context[key] = value
	}

	return context
}

// Router configura e retorna o router HTTP
func (h *Handler) Router() http.Handler {
	// Criar router sem middlewares padrão (vamos adicionar os nossos)
	router := gin.New()

	// Configurar middlewares na ordem correta
	router.Use(monitoring.RecoveryMiddleware(h.metrics, h.tracer)) // Primeiro: Recovery
	router.Use(monitoring.MetricsMiddleware(h.metrics, h.tracer))  // Segundo: Métricas
	router.Use(h.corsMiddleware())                                 // Terceiro: CORS

	// Servir arquivos estaticos embutidos no binario
	router.StaticFS("/static", http.FS(assets.StaticFS()))

	// Rotas da interface web - baseadas no EmulatorService.py
	h.setupWebRoutes(router)

	// Rotas da API REST
	h.setupAPIRoutes(router)

	// Rotas de monitoramento
	h.setupMonitoringRoutes(router)

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
		"default": func(defaultValue interface{}, value interface{}) interface{} {
			if value == nil {
				return defaultValue
			}
			return value
		},
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
	return template.Must(template.New("").Funcs(funcMap).ParseFS(assets.Templates(), files...))
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

	// Página de configurações
	router.GET("/settings", h.settingsPage)

	router.GET("/events", h.handleSSE)
}

// setupMonitoringRoutes configura rotas de monitoramento e observabilidade
func (h *Handler) setupMonitoringRoutes(router *gin.Engine) {
	monitoring := router.Group("/monitoring")
	{
		// Health check avançado
		monitoring.GET("/health", h.advancedHealthCheck)
		monitoring.GET("/health/quick", h.quickHealthCheck)

		// Métricas
		monitoring.GET("/metrics", h.getMetrics)
		monitoring.GET("/metrics/endpoints", h.getEndpointMetrics)
		monitoring.GET("/metrics/errors", h.getTopErrors)
		monitoring.GET("/metrics/database", h.getDatabaseMetrics)

		// Debug e diagnóstico
		monitoring.GET("/debug/goroutines", h.getGoroutineCount)
		monitoring.GET("/debug/memory", h.getMemoryStats)
	}
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
			devices.GET("/:id/users", h.getDeviceUsers)
			devices.GET("/:id/settings", h.getDeviceSettings)
		}

		// Status do sistema
		api.GET("/status", h.getSystemStatus)
		api.GET("/comparison", h.getUserComparisons)
		api.GET("/pool-stats", h.getPoolStats)
		api.GET("/refresh-status", h.getRefreshStatus)

		// Configurações
		settings := api.Group("/settings")
		{
			settings.POST("/test-wxs-connection", h.testWxsConnection)
			settings.POST("/wxs", h.saveWxsSettings)
		}
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

func (h *Handler) mainPage(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "10"))

	// Capturar filtros da query string
	filters := map[string]string{
		"id":   c.Query("id"),
		"name": c.Query("name"),
		"port": c.Query("port"),
	}

	// Dispositivos filtrados para a tabela
	devices, _, err := h.getCurrentDevicesWithFilters(filters)
	if err != nil {
		h.tracer.Error("Failed to get current devices: %v", err)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": err.Error()})
		return
	}

	// Paginação dos dispositivos filtrados
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

	// Contadores vêm de countFleet para o header concordar com a tabela:
	// desabilitado é um estado próprio, não um "parado".
	fleetDevices, err := h.manager.ListDevices()
	if err != nil {
		h.tracer.Error("Failed to list devices for counters: %v", err)
		fleetDevices = nil
	}
	counts := countFleet(fleetDevices)

	context := gin.H{
		"devices":       paginatedDevices,
		"page":          page,
		"total_pages":   totalPages,
		"per_page":      perPage,
		"page_range":    paginationRange(page, totalPages),
		"counter_cards": counts.toMap(),
		"filters":       filters,
	}

	tmpl := h.loadTemplate("web/templates/devices.html")
	tmpl.ExecuteTemplate(c.Writer, "base.html", h.withBaseContext(context))
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

	// Executar refresh em background (goroutine)
	go func() {
		if err := h.manager.RefreshDevices(); err != nil {
			h.tracer.Error("Failed to refresh devices: %v", err)
		}
	}()

	// Retornar imediatamente com sucesso
	c.JSON(http.StatusOK, gin.H{
		"message": "Refresh iniciado em background",
		"status":  "started",
	})
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
		"page_range":    paginationRange(page, totalPages),
		"per_page":      perPage,
		"counter_cards": counterCards,
	}

	tmpl := h.loadTemplate("web/templates/comparison.html")
	tmpl.ExecuteTemplate(c.Writer, "base.html", h.withBaseContext(context))
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

	counts := countFleet(devices)

	c.JSON(http.StatusOK, gin.H{
		"total_devices":    counts.Total,
		"running_devices":  counts.Running,
		"stopped_devices":  counts.Stopped,
		"disabled_devices": counts.Disabled,
		"timestamp":        time.Now().UTC(),
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

// getCurrentDevices obtém dispositivos atuais - baseado no get_current_devices() do Python
func (h *Handler) getCurrentDevices() ([]map[string]interface{}, int, error) {
	devices, err := h.manager.ListDevices()
	if err != nil {
		return nil, 0, err
	}

	var currentDevices []map[string]interface{}
	deviceStatusRunning := 0

	for _, device := range devices {
		// Determinar status considerando se está habilitado
		status := device.Status
		if device.Enabled == 0 {
			status = "disabled"
		} else if device.Status == "running" {
			deviceStatusRunning++
		}

		currentDevices = append(currentDevices, map[string]interface{}{
			"lc_id":       device.ID,
			"name":        device.Name,
			"ip_address":  device.IPAddress,
			"port":        device.Port,
			"log_enabled": device.LogEnabled,
			"model":       device.Model,
			"status":      status,
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

	h.tracer.Info("Users comparison refreshed successfully")
}

func (h *Handler) handleSSE(c *gin.Context) {
	h.tracer.Info("SSE client connected")

	// Configurar headers para SSE
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	// Adicionar listener para mudanças de status
	listener := h.manager.AddStatusListener()
	defer h.manager.RemoveStatusListener(listener)

	h.tracer.Info("SSE listener added successfully")

	// Canal para detectar desconexão do cliente
	clientGone := c.Request.Context().Done()

	for {
		select {
		case event, ok := <-listener:
			if !ok {
				h.tracer.Info("SSE listener closed")
				return
			}

			h.tracer.Info("SSE received event: Device %d -> %s", event.DeviceID, event.Status)

			// Obter contadores atualizados de TODOS os dispositivos
			allDevices, err := h.manager.ListDevices()
			if err != nil {
				h.tracer.Error("Failed to get devices for SSE: %v", err)
				continue
			}

			runningCount := 0
			for _, device := range allDevices {
				if device.Status == "running" {
					runningCount++
				}
			}

			totalCount := len(allDevices)
			stoppedCount := totalCount - runningCount

			// Enviar evento SSE
			data := map[string]interface{}{
				"device_id":     event.DeviceID,
				"status":        event.Status,
				"name":          event.Name,
				"running_count": runningCount,
				"stopped_count": stoppedCount,
				"total_count":   totalCount,
			}

			jsonData, err := json.Marshal(data)
			if err != nil {
				h.tracer.Error("Failed to marshal SSE data: %v", err)
				continue
			}

			h.tracer.Info("Sending SSE data: %s", string(jsonData))

			// Formato SSE: data: {json}\n\n
			_, err = fmt.Fprintf(c.Writer, "data: %s\n\n", jsonData)
			if err != nil {
				h.tracer.Error("Failed to write SSE data: %v", err)
				return
			}

			// Forçar envio imediato
			if f, ok := c.Writer.(http.Flusher); ok {
				f.Flush()
			}

		case <-clientGone:
			h.tracer.Info("SSE client disconnected")
			return
		}
	}
}
func (h *Handler) getPoolStats(c *gin.Context) {
	stats := h.manager.GetPoolStats()
	c.JSON(http.StatusOK, stats)
}

func (h *Handler) getCurrentDevicesWithFilters(filters map[string]string) ([]map[string]interface{}, int, error) {
	devices, err := h.manager.ListDevicesWithFilters(filters)
	if err != nil {
		return nil, 0, err
	}

	var currentDevices []map[string]interface{}
	deviceStatusRunning := 0

	for _, device := range devices {
		// Determinar status considerando se está habilitado
		status := device.Status
		if device.Enabled == 0 {
			status = "disabled"
		} else if device.Status == "running" {
			deviceStatusRunning++
		}

		currentDevices = append(currentDevices, map[string]interface{}{
			"lc_id":       device.ID,
			"name":        device.Name,
			"ip_address":  device.IPAddress,
			"port":        device.Port,
			"log_enabled": device.LogEnabled,
			"model":       device.Model,
			"status":      status,
			"enabled":     device.Enabled,
			"interval":    device.EventInterval,
			"total":       device.TotalUsers,
		})
	}

	return currentDevices, deviceStatusRunning, nil
}

// getRefreshStatus retorna o status da operação de refresh
func (h *Handler) getRefreshStatus(c *gin.Context) {
	inProgress := h.manager.IsRefreshInProgress()

	c.JSON(http.StatusOK, gin.H{
		"in_progress": inProgress,
		"completed":   !inProgress,
	})
}

// settingsPage exibe a página de configurações
func (h *Handler) settingsPage(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Carregar configurações WXS do banco
	wxsSettings, err := database.GetWxsSettingsFromDB(ctx, h.serviceDB)
	if err != nil {
		h.tracer.Error("Failed to load WXS settings: %v", err)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"error": "Erro ao carregar configurações",
		})
		return
	}

	tmpl := h.loadTemplate("web/templates/settings.html")
	tmpl.ExecuteTemplate(c.Writer, "base.html", h.withBaseContext(gin.H{
		"wxs_settings": wxsSettings,
	}))
}

// testWxsConnection testa a conexão com o WXS
func (h *Handler) testWxsConnection(c *gin.Context) {
	var settings database.WxsSettings
	if err := c.ShouldBindJSON(&settings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Dados inválidos: " + err.Error(),
		})
		return
	}

	// Testar conexão
	err := database.TestWxsConnection(&settings)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Conexão bem-sucedida",
	})
}

// saveWxsSettings salva as configurações WXS e reconecta
func (h *Handler) saveWxsSettings(c *gin.Context) {
	var settings database.WxsSettings
	if err := c.ShouldBindJSON(&settings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Dados inválidos: " + err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Salvar no banco
	err := database.SaveWxsSettingsFromDB(ctx, h.serviceDB, &settings)
	if err != nil {
		h.tracer.Error("Failed to save WXS settings: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Erro ao salvar configurações: " + err.Error(),
		})
		return
	}

	// Reconectar com as novas configurações
	h.tracer.Info("Reconnecting to WXS with new settings...")

	// Fechar conexão antiga se existir
	if h.wxsDB != nil {
		h.wxsDB.Close()
	}

	// Criar nova conexão
	cfg := config.DatabaseConfig{
		Host:     settings.Host,
		Port:     settings.Port,
		Database: settings.Database,
		Username: settings.Username,
		Password: settings.Password,
		Driver:   "mssql",
	}

	newWxsDB, err := database.NewWxsDB(cfg)
	if err != nil {
		h.tracer.Error("Failed to reconnect to WXS: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Configurações salvas, mas falha ao reconectar: " + err.Error(),
		})
		return
	}

	// Testar a nova conexão
	testCtx, testCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer testCancel()

	if err := newWxsDB.Ping(testCtx); err != nil {
		newWxsDB.Close()
		h.tracer.Error("New WXS connection ping failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Configurações salvas, mas falha no ping: " + err.Error(),
		})
		return
	}

	// Atualizar a conexão no handler
	h.wxsDB = newWxsDB

	// Atualizar a conexão no manager também
	h.manager.WxsDB = newWxsDB

	h.tracer.Info("WXS connection updated successfully")

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Configurações salvas e conexão reiniciada com sucesso",
	})
}

// ====================================================================
// Monitoring and Observability Handlers
// ====================================================================

// advancedHealthCheck retorna health check detalhado de todos os componentes
func (h *Handler) advancedHealthCheck(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	result := h.healthMonitor.CheckAll(ctx)

	// Status HTTP baseado no status geral
	status := result["status"].(monitoring.HealthStatus)
	httpStatus := http.StatusOK
	if status == monitoring.HealthStatusUnhealthy {
		httpStatus = http.StatusServiceUnavailable
	} else if status == monitoring.HealthStatusDegraded {
		httpStatus = http.StatusOK // 200 mas com aviso
	}

	c.JSON(httpStatus, result)
}

// quickHealthCheck retorna health check rápido (usa cache)
func (h *Handler) quickHealthCheck(c *gin.Context) {
	result := h.healthMonitor.QuickCheck()

	status := result["status"].(monitoring.HealthStatus)
	httpStatus := http.StatusOK
	if status == monitoring.HealthStatusUnhealthy {
		httpStatus = http.StatusServiceUnavailable
	}

	c.JSON(httpStatus, result)
}

// getMetrics retorna snapshot completo das métricas
func (h *Handler) getMetrics(c *gin.Context) {
	snapshot := h.metrics.GetSnapshot()
	c.JSON(http.StatusOK, snapshot)
}

// getEndpointMetrics retorna métricas detalhadas por endpoint
func (h *Handler) getEndpointMetrics(c *gin.Context) {
	snapshot := h.metrics.GetSnapshot()
	endpoints := snapshot["endpoints"]

	c.JSON(http.StatusOK, gin.H{
		"endpoints": endpoints,
		"timestamp": time.Now().UTC(),
	})
}

// getTopErrors retorna endpoints com mais erros
func (h *Handler) getTopErrors(c *gin.Context) {
	limit := 10
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	topErrors := h.metrics.GetTopErrorEndpoints(limit)

	c.JSON(http.StatusOK, gin.H{
		"top_errors": topErrors,
		"limit":      limit,
		"timestamp":  time.Now().UTC(),
	})
}

// getGoroutineCount retorna contagem de goroutines
func (h *Handler) getGoroutineCount(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"goroutines": runtime.NumGoroutine(),
		"timestamp":  time.Now().UTC(),
	})
}

// getMemoryStats retorna estatísticas de memória
func (h *Handler) getMemoryStats(c *gin.Context) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	c.JSON(http.StatusOK, gin.H{
		"alloc_mb":       memStats.Alloc / 1024 / 1024,
		"total_alloc_mb": memStats.TotalAlloc / 1024 / 1024,
		"sys_mb":         memStats.Sys / 1024 / 1024,
		"num_gc":         memStats.NumGC,
		"gc_pause_ms":    float64(memStats.PauseNs[(memStats.NumGC+255)%256]) / 1000000,
		"heap_alloc_mb":  memStats.HeapAlloc / 1024 / 1024,
		"heap_sys_mb":    memStats.HeapSys / 1024 / 1024,
		"heap_idle_mb":   memStats.HeapIdle / 1024 / 1024,
		"heap_inuse_mb":  memStats.HeapInuse / 1024 / 1024,
		"timestamp":      time.Now().UTC(),
	})
}

// getDatabaseMetrics retorna métricas dos pools de banco de dados
func (h *Handler) getDatabaseMetrics(c *gin.Context) {
	stats := h.manager.GetPoolStats()
	c.JSON(http.StatusOK, stats)
}
