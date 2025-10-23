# LEIA-ME PRIMEIRO - GoFacialEmulator

Bem-vindo ao GoFacialEmulator! Este documento vai te guiar sobre qual documentação usar.

---

## Você Nunca Usou Docker?

### Comece aqui: [GUIA-INSTALACAO-DOCKER.md](GUIA-INSTALACAO-DOCKER.md)

Este é o guia mais completo e detalhado, criado especialmente para iniciantes:

- Explica o que é Docker de forma simples
- Passo-a-passo para instalar Docker no Windows/Linux/Mac
- Instruções detalhadas de cada comando
- Solução de todos os problemas comuns
- Exemplos práticos com imagens e explicações

**Tempo estimado:** 30-60 minutos (primeira vez)

---

## Quer Algo Mais Rápido?

### Use: [CHECKLIST-INSTALACAO.md](CHECKLIST-INSTALACAO.md)

Um checklist visual para marcar conforme avança:

- Lista de verificação passo-a-passo
- Comandos prontos para copiar e colar
- Fluxograma de resolução de problemas
- Perfeito para imprimir ou ter em segunda tela

**Tempo estimado:** 15-30 minutos

---

## Prefere Diagramas Visuais?

### Veja: [DIAGRAMA-DOCKER.md](DIAGRAMA-DOCKER.md)

Documentação visual com diagramas e fluxogramas:

- Arquitetura do sistema desenhada
- Fluxo de instalação ilustrado
- Mapa de comunicação entre containers
- Ciclo de vida dos containers
- Comandos e seus efeitos visuais

**Ideal para:** Pessoas que aprendem melhor com imagens

---

## Já Conhece Docker?

### Acesse: [QUICKSTART-DOCKER.md](QUICKSTART-DOCKER.md)

Início super rápido em 3 passos:

```bash
# 1. Navegar para a pasta
cd GoFacialEmulator

# 2. Iniciar
docker compose up -d

# 3. Acessar
http://localhost:8080
```

**Tempo estimado:** 5-10 minutos

---

## Precisa de Configurações Avançadas?

### Consulte: [DOCKER.md](DOCKER.md)

Documentação técnica completa:

- Configurações avançadas
- Deploy em produção
- Segurança e boas práticas
- Backup e restore
- Monitoramento
- Resource limits

**Para:** Usuários avançados e ambiente de produção

---

## Fluxograma: Qual Guia Usar?

```
         VOCÊ CONHECE DOCKER?
                 │
        ┌────────┴─────────┐
        │                  │
       NÃO                SIM
        │                  │
        ▼                  ▼
   Tem tempo?         Quer detalhes?
        │                  │
   ┌────┴────┐        ┌────┴─────┐
   │         │        │          │
  SIM       NÃO      SIM        NÃO
   │         │        │          │
   ▼         ▼        ▼          ▼
GUIA      CHECKLIST  DOCKER    QUICKSTART
INSTALACAO            .md         .md
.md
```

---

## Resumo dos Documentos

| Documento | Para Quem | Nível | Tempo |
|-----------|-----------|-------|-------|
| **GUIA-INSTALACAO-DOCKER.md** | Iniciantes em Docker | Básico | 30-60 min |
| **CHECKLIST-INSTALACAO.md** | Quem quer algo rápido | Básico | 15-30 min |
| **DIAGRAMA-DOCKER.md** | Aprendizes visuais | Básico | 20-40 min |
| **QUICKSTART-DOCKER.md** | Quem conhece Docker | Intermediário | 5-10 min |
| **DOCKER.md** | Usuários avançados | Avançado | Referência |
| **README.md** | Visão geral do projeto | Todos | Referência |

---

## Recomendação Por Perfil

### Perfil 1: "Nunca usei Docker, quero aprender"
1. Leia: [GUIA-INSTALACAO-DOCKER.md](GUIA-INSTALACAO-DOCKER.md)
2. Use como apoio: [CHECKLIST-INSTALACAO.md](CHECKLIST-INSTALACAO.md)
3. Se tiver dúvidas visuais: [DIAGRAMA-DOCKER.md](DIAGRAMA-DOCKER.md)

### Perfil 2: "Já usei Docker algumas vezes"
1. Comece: [QUICKSTART-DOCKER.md](QUICKSTART-DOCKER.md)
2. Se travar: [GUIA-INSTALACAO-DOCKER.md](GUIA-INSTALACAO-DOCKER.md) (seção de problemas)

### Perfil 3: "Sou desenvolvedor experiente"
1. Veja: [QUICKSTART-DOCKER.md](QUICKSTART-DOCKER.md)
2. Configure: [DOCKER.md](DOCKER.md)
3. Referência técnica: [README.md](README.md)

### Perfil 4: "Vou implantar em produção"
1. Entenda: [DOCKER.md](DOCKER.md) (seção de produção)
2. Configure segurança: [DOCKER.md](DOCKER.md) (seção de segurança)
3. Setup de backup: [DOCKER.md](DOCKER.md) (seção de backup)

---

## Instalação Rápida (3 Comandos)

Se você já tem Docker instalado:

```bash
# 1. Entrar na pasta do projeto
cd GoFacialEmulator

# 2. Iniciar tudo
docker compose up -d

# 3. Verificar
docker compose ps
```

Depois acesse: http://localhost:8080

Se funcionar, parabéns! Se não funcionar, consulte os guias acima.

---

## Comandos Essenciais

Tenha sempre à mão:

```bash
# Ver status
docker compose ps

# Ver logs
docker compose logs -f app

# Parar
docker compose stop

# Iniciar
docker compose start

# Reiniciar
docker compose restart

# Remover (mantém dados)
docker compose down

# Remover tudo (apaga dados)
docker compose down -v
```

---

## Precisa de Ajuda?

### Problema durante instalação
→ Veja seção "Problemas Comuns" em [GUIA-INSTALACAO-DOCKER.md](GUIA-INSTALACAO-DOCKER.md)

### Erro ao executar comando
→ Verifique os logs: `docker compose logs app --tail=100`

### Container não inicia
→ Siga o fluxograma em [CHECKLIST-INSTALACAO.md](CHECKLIST-INSTALACAO.md)

### Dúvida sobre arquitetura
→ Consulte os diagramas em [DIAGRAMA-DOCKER.md](DIAGRAMA-DOCKER.md)

### Configuração avançada
→ Veja [DOCKER.md](DOCKER.md)

---

## Estrutura de Arquivos do Projeto

```
GoFacialEmulator/
│
├── 📄 LEIA-ME-PRIMEIRO.md          ← VOCÊ ESTÁ AQUI
│
├── 📘 Documentação Docker (escolha o seu):
│   ├── GUIA-INSTALACAO-DOCKER.md   ← Iniciantes (passo-a-passo completo)
│   ├── CHECKLIST-INSTALACAO.md     ← Checklist rápido
│   ├── DIAGRAMA-DOCKER.md          ← Documentação visual
│   ├── QUICKSTART-DOCKER.md        ← Início rápido (3 passos)
│   └── DOCKER.md                   ← Documentação técnica completa
│
├── 📄 Arquivos Docker:
│   ├── docker-compose.yml          ← Orquestração dos serviços
│   ├── Dockerfile                  ← Build da aplicação
│   ├── .dockerignore               ← Arquivos ignorados no build
│   └── docker-init.sql             ← Script de inicialização do DB
│
├── 📄 README.md                    ← Visão geral do projeto
│
└── 📁 Código fonte e configs...
```

---

## FAQ Rápido

### Quanto tempo demora para instalar?

- **Primeira vez:** 10-60 minutos (dependendo da internet e conhecimento)
- **Próximas vezes:** 2-5 minutos

### Preciso saber programar?

Não! Os guias são feitos para qualquer pessoa usar.

### Funciona no Windows?

Sim! Funciona em Windows, Linux e Mac.

### Preciso instalar PostgreSQL?

Não! O Docker instala tudo automaticamente.

### E se eu já tenho PostgreSQL instalado?

Sem problema! O Docker usa um PostgreSQL isolado dentro do container.

### Posso usar sem Docker?

Sim, mas terá que instalar Go e PostgreSQL manualmente. Veja "Opção 2: Instalação Manual" no [README.md](README.md).

### Quanto de espaço em disco precisa?

Aproximadamente 500MB-1GB.

### Usa muita memória?

Não, aproximadamente 200-400MB de RAM.

---

## Checklist Rápido de Sucesso

Você instalou corretamente se:

- [ ] `docker compose ps` mostra Status = "Up"
- [ ] http://localhost:8080 abre a interface
- [ ] `docker compose logs app` não mostra erros críticos
- [ ] Consegue clicar nos botões da interface

Se todos os itens acima estão OK, parabéns! Está funcionando perfeitamente.

---

## Próximos Passos Após Instalação

1. **Configure WXS** (se aplicável):
   - Acesse: http://localhost:8080/settings
   - Preencha dados do banco WXS
   - Teste a conexão

2. **Carregue Dispositivos**:
   - No dashboard: http://localhost:8080
   - Clique em "Refresh DB"
   - Aguarde carregar

3. **Inicie Emuladores**:
   - Clique em "Start All" ou em dispositivos específicos

4. **Monitore**:
   - Veja a página de comparação: http://localhost:8080/comparison
   - Acompanhe logs: `docker compose logs -f app`

---

## Começar Agora

**Minha recomendação:**

1. Se nunca usou Docker: Abra [GUIA-INSTALACAO-DOCKER.md](GUIA-INSTALACAO-DOCKER.md)
2. Se já conhece Docker: Abra [QUICKSTART-DOCKER.md](QUICKSTART-DOCKER.md)
3. Mantenha [CHECKLIST-INSTALACAO.md](CHECKLIST-INSTALACAO.md) aberto para consulta

Boa instalação!
