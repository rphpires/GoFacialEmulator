# Console em tempo real — diagnóstico e plano

Data: 2026-10-09 · Branch: `feat/ui-tempo-real`

Objetivo: o console não precisar mais de F5. Toda mudança de estado da frota
chega pela conexão SSE (`/events`) e a tela se atualiza sozinha; ações não
recarregam a página.

## Diagnóstico

### Por que hoje é preciso dar refresh

| # | Sintoma | Causa |
|---|---|---|
| D1 | "Parar todos" não muda a tabela | `Manager.StopAll` não chama `notifyStatusChange` (`internal/emulator/manager.go`) |
| D2 | "Iniciar todos"/lote grande: parte das linhas fica com estado velho | buffer de 10 eventos por listener; excedente é descartado ("channel full, skipping") e o cliente nunca sabe |
| D3 | Coluna Usuários não muda | watchdog grava `total_users` a cada 10 s, mas não notifica |
| D4 | Criar, editar, excluir emulador → `location.reload()` | stream só fala de start/stop; não há evento de inclusão/remoção; a tabela é HTML do servidor |
| D5 | Sincronizar W-Access → `location.reload()` | idem: dispositivos novos/removidos não chegam pelo stream |
| D6 | Botões Editar/Remover não funcionam em linha nova; `data-interval` velho no Editar | listeners ligados por `querySelectorAll` no load; dados do formulário lidos de atributos renderizados uma vez |
| D7 | Start que falha deixa o botão "pendente" para sempre | `/start` responde 303 em qualquer caso; falha não gera evento |
| D8 | Checkbox Log da tabela "não salva" | só é enviado no próximo start; e `updateLogEnabled` grava `WHERE port = $2` com o **ID** do dispositivo (bug) |
| D9 | Alerta de alcançabilidade, modo do dispositivo e lista de usuários do drawer ficam velhos | carregados uma vez; nunca reavaliados |
| D10 | Eventos fora de ordem (stopped antes de running) | `go m.notifyStatusChange(...)`: cada notificação numa goroutine própria |

### Performance

| # | Ponto | Efeito |
|---|---|---|
| P1 | Stream faz `ListDevices()` (query da frota inteira) **por evento, por aba aberta** | StartAll de 1000 = 1000 queries × abas; atraso enche o buffer → D2 |
| P2 | `/start` e `/stop` respondem 303 → `fetch` segue para `GET /` e renderiza a página inteira (query + modos + template) e descarta | 2 queries + render inúteis por clique |
| P3 | `Manager.Start` segura `emulatorMutex` (escrita) durante o start inteiro (até 10 s) | starts em série apesar do semáforo de 20; `ListDevices`/stream bloqueiam durante StartAll → tela congela |
| P4 | Watchdog faz `UPDATE total_users` de todo dispositivo rodando a cada 10 s, mesmo sem mudança, e loga 2 linhas Info por dispositivo | 1000 UPDATEs + 2000 linhas de trace a cada 10 s |
| P5 | `StopAll` segura o mutex durante N UPDATEs no banco | trava start/list enquanto para |
| P6 | `/start` "all" e lote rodam síncronos na requisição HTTP | requisição de minutos; botão preso; timeout do navegador |

### Layout / usabilidade

| # | Ponto |
|---|---|
| L1 | 5 botões no cabeçalho, 3 deles inúteis sem seleção → barra de ações em lote só aparece com seleção, com contador e "limpar seleção" |
| L2 | Filtro exige submit + recarga; paginação recarrega → filtro instantâneo e paginação no cliente, estado na URL |
| L3 | Seleção some ao trocar de página → seleção persistente (Set de IDs) |
| L4 | Estado "erro" não aparece; motivo do erro de start fica escondido → badge erro + tooltip com o último erro |
| L5 | Comparação: Recontar recarrega a página → atualiza a tabela no lugar |

## Plano de implementação

### Fase 1 — Backend: eventos completos, ordenados e baratos
1. `Manager`: listener com buffer maior e flag de overflow (cliente recebe snapshot em vez de perder evento). Notificação síncrona e não-bloqueante (ordem preservada). Tipos: `changed`, `removed`, `resync`.
2. Notificar em: Start (sucesso e falha), Stop, StopAll, watchdog (erro e mudança de `total_users`), Create/CreateRange/Update/Delete, RefreshDevices (resync), log, modo.
3. Stream: agrega eventos numa janela curta (~150 ms) e manda **um** frame `delta` `{devices, removed, counts}` com uma query só. Overflow/resync → `snapshot`.
4. `deviceView` ganha `source`, `ip_address`, `mode`, `last_error` (o suficiente para desenhar a linha inteira no cliente).
5. `/start` e `/stop` respondem JSON; lote/all em background (202). Corrige `updateLogEnabled` (ID, não porta).
6. Watchdog só grava/notifica `total_users` quando muda; log em nível debug.
7. `Start` não segura o mutex global durante o start do emulador (reserva por ID).

### Fase 2 — Frontend: tabela dirigida pelo stream
1. `FleetStream` mantém o store (mapa id → dispositivo) e publica `snapshot`/`delta`.
2. `devices.js` desenha o `tbody` a partir do store, com diff por linha (preserva foco/seleção), filtro instantâneo, ordenação, paginação no cliente e estado na URL.
3. Ações por delegação de eventos; sem `location.reload()` em criar/editar/excluir/sincronizar.
4. Barra de ações em lote contextual (L1), seleção persistente (L3), estado erro com motivo (L4).
5. Log da tabela grava na hora (PUT settings), só com o emulador parado.
6. Drawer acompanha o dispositivo aberto (LED, log, recarrega usuários quando `total_users` muda).
7. Alcançabilidade reavaliada após mudanças de estado (debounce).

### Fase 3 — Demais telas
1. Comparação: Recontar atualiza a tabela sem recarregar.
2. Revisão visual geral (espaçamentos, responsivo, estados vazios/carregando).
