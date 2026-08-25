# Roteiro de validacao

Depois de instalar e configurar a conexão com o W-Access, siga estes
passos na ordem. Cada um diz o que fazer e o que precisa acontecer; se
não acontecer, ele diz para onde ir.

## 1. Abrir o console

**Faça:** abra `http://localhost:7070` no navegador.

**Tem que acontecer:** a lista de dispositivos carrega e mostra os
controladores cuja descrição no W-Access começa com `emulator`.

![A lista de dispositivos](img/gerado/emulador-dispositivos.png)

**Se não acontecer:** veja o capítulo Problemas, "a lista de dispositivos
está vazia".

## 2. Iniciar um dispositivo

**Faça:** marque um dispositivo e clique em iniciar (ou use **Iniciar
todos**, na barra lateral).

**Tem que acontecer:** o estado do dispositivo vira `ativo` e o medidor
de frota, no topo da tela, acompanha a mudança na hora.

![O medidor de frota](img/gerado/emulador-medidor.png)

**Se não acontecer:** veja o capítulo Problemas, "um dispositivo não
inicia".

## 3. Conferir no W-Access

**Faça:** no W-Access, abra a lista de controladores e confira o estado
do controlador correspondente.

**Tem que acontecer:** o controlador aparece online.

![Lista de controladores no W-Access](img/gerado/wxs-controladores.png)

**Se não acontecer:** se o controlador aparecer offline no W-Access com a
tela do emulador toda verde (dispositivos ativos, sem nenhum aviso), veja
o capítulo Portas e rede — é o caso mais comum de todos, e também o mais
confuso, porque o emulador não mostra erro nenhum.

## 4. Simular uma leitura facial

**Faça:** nada — com o dispositivo ativo, ele mesmo gera um evento
sozinho a cada tantos segundos quanto a coluna **Intervalo** indicar,
sem precisar de nenhum botão. Se quiser apressar o teste, pare e inicie o
dispositivo de novo, ou reduza o intervalo cadastrado para ele no
W-Access.

**Tem que acontecer:** um evento de leitura facial chega no W-Access,
atribuído a esse controlador.

**Se não acontecer:** confira se o dispositivo continua com o estado
`ativo` — um dispositivo que caiu de volta para `parado` não gera mais
eventos — e revise o passo 3 sobre alcançar o controlador pela rede.

## 5. Conferir os logs

**Faça:** abra a gaveta de detalhes do dispositivo e veja a aba
Configurações; se o registro de log estiver ligado, o evento aparece no
arquivo salvo. Depois, procure a mesma linha no log da aplicação.

![O log do dispositivo](img/gerado/emulador-gaveta-log.png)

**Tem que acontecer:** a mesma leitura simulada no passo 4 aparece tanto
no log do dispositivo quanto no log da aplicação.

**Se não acontecer:** confira se **Gravar log de eventos** estava
marcado antes do evento acontecer — ele só registra o que acontece depois
de ligado — e veja o caminho do log da aplicação para o seu pacote no
capítulo Onde estao os logs.
