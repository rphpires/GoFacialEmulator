# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Instalar dependências de build
RUN apk add --no-cache git

# Copiar go.mod e go.sum para cache de dependências
COPY go.mod go.sum ./
RUN go mod download

# Copiar código fonte
COPY . .

# Build da aplicação
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o facial_emulator cmd/emulator-service/main.go

# Runtime stage
FROM alpine:latest

WORKDIR /app

# Instalar certificados para HTTPS
RUN apk --no-cache add ca-certificates tzdata

# Copiar binário compilado
COPY --from=builder /app/facial_emulator .

# Copiar configuração e assets
COPY --from=builder /app/configs ./configs
COPY --from=builder /app/web ./web
COPY --from=builder /app/internal/database/migrations ./internal/database/migrations

# Criar diretórios para dados
RUN mkdir -p traces logs

# Expor porta HTTP
EXPOSE 8080

# Executar aplicação
CMD ["./facial_emulator"]
