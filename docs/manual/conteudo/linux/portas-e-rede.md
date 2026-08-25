# Portas e rede

Se você chegou aqui pelo passo 3 do Roteiro de validação — controlador
offline no W-Access com a tela do emulador toda verde — este é o capítulo
certo.

## De onde vêm as portas

As portas que cada dispositivo emulado usa **não são escolhidas pelo
emulador**. Elas vêm do cadastro do controlador no W-Access, do campo de
porta de comunicação (`BaseCommPort`). O emulador só obedece: abre a porta
que o W-Access mandou.

## Não há faixa publicada para configurar

Este pacote roda a aplicação diretamente no host — não existe contêiner
nem faixa publicada como no pacote Docker. Qualquer porta que o W-Access
mandar, a aplicação tenta abrir direto na máquina.

## Firewall

O `instalar.sh` já libera, no `ufw` ou no `firewalld` (o que estiver
ativo), a faixa de portas que você informou durante a instalação.

Se depois você mudar as portas dos controladores no W-Access para fora
dessa faixa original, precisa liberar a faixa nova à mão. Com `ufw`, por
exemplo:

```
ufw allow 4000:4499/tcp
```

Troque `4000:4499` pela faixa nova (o `ufw` usa dois-pontos entre início e
fim, não hífen). Com `firewalld`, o comando equivalente é
`firewall-cmd --permanent --add-port=4000-4499/tcp` seguido de
`firewall-cmd --reload`.

## Limite de arquivos abertos

Cada dispositivo emulado ativo mantém um socket de rede aberto. Com
centenas de dispositivos ao mesmo tempo, o limite padrão do sistema (1024
arquivos abertos por usuário) estoura, e novos dispositivos passam a
falhar ao iniciar. O `instalar.sh` calcula o limite necessário para a
faixa de portas informada e ajusta automaticamente quando o valor atual
não é suficiente — mas, como já visto no capítulo anterior, esse ajuste só
vale depois que a sessão for reaberta.

## WSL2: modo de rede

Se você está rodando este pacote dentro do WSL2, vale o mesmo aviso do
capítulo Instalar o WSL2: por padrão, só a própria máquina alcança os
emuladores, e um Site Controller em outro computador não consegue se
conectar. A correção é o modo de rede espelhado (`networkingMode=mirrored`
em `.wslconfig`) — veja o capítulo Instalar o WSL2 para o passo a passo
completo.
