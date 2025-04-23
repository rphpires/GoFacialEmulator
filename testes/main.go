package main

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

type Server struct {
	dahua                   *Dahua
	generatedEvent          bool
	generatedEventFrequency time.Duration
}

func NewServer() *Server {
	return &Server{
		dahua:                   NewDahua(),
		generatedEvent:          true,
		generatedEventFrequency: 30 * time.Second,
	}
}

func (s *Server) snapManagerHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("[GET] /cgi-bin/snapManager.cgi")

	// Configura o cabeçalho para streaming de eventos
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Cria um canal para detectar quando o cliente se desconectar
	clientGone := r.Context().Done()

	// Inicializa contadores
	heartbeatCounter := time.Now()
	generatedEventCounter := time.Now()

	// Configura um flusher para garantir que os dados sejam enviados imediatamente
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming não suportado", http.StatusInternalServerError)
		return
	}

	// Loop principal - mantém a conexão aberta
	for {
		select {
		case <-clientGone:
			// Cliente desconectou
			log.Println("Cliente desconectou")
			return
		default:
			now := time.Now()

			// Verifica se é hora de enviar um evento gerado
			if s.generatedEvent && now.Sub(generatedEventCounter) >= s.generatedEventFrequency {
				log.Println(">> Enviando Evento Falso Gerado <<")
				generatedEventCounter = now

				evtPackage := s.dahua.GenerateRandomEvent()
				if evtPackage != nil && s.dahua.GetSettings("LocalAuthentication") == "1" {
					log.Println("## enviando evento")
					fmt.Fprint(w, string(evtPackage))
					flusher.Flush()
				}
			}

			// Verifica se é hora de enviar um heartbeat
			if now.Sub(heartbeatCounter) >= 10*time.Second {
				log.Println(">> Enviando Heartbeat <<")
				heartbeatCounter = now

				heartbeat := "\r\n\r\n\r\n--myboundary\r\nContent-Type: text/plain\r\nContent-Length:9\r\n\r\nHeartbeat"
				fmt.Fprint(w, heartbeat)
				flusher.Flush()
			}

			// Encerra a conexão se a autenticação local estiver desativada
			if s.dahua.GetSettings("LocalAuthentication") == "0" {
				return
			}

			// Pausa para não sobrecarregar a CPU
			time.Sleep(2 * time.Second)
		}
	}
}

func main() {
	server := NewServer()

	http.HandleFunc("/cgi-bin/snapManager.cgi", server.snapManagerHandler)

	log.Println("Servidor iniciado na porta 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// Estrutura Dahua e seus métodos (implementação simplificada)
type Dahua struct {
	settings map[string]string
}

func NewDahua() *Dahua {
	return &Dahua{
		settings: map[string]string{
			"LocalAuthentication": "1",
		},
	}
}

func (d *Dahua) GetSettings(key string) string {
	if val, ok := d.settings[key]; ok {
		return val
	}
	return ""
}

func (d *Dahua) GenerateRandomEvent() []byte {
	// Implementação simplificada - geraria um evento aleatório
	return []byte("\r\n\r\n\r\n--myboundary\r\nContent-Type: application/json\r\nContent-Length:25\r\n\r\n{\"event\":\"randomEvent\"}")
}
