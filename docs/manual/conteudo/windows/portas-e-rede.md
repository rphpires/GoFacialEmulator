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

Este pacote roda a aplicação diretamente no Windows — não existe faixa de
portas publicada para editar, nem nada parecido. Qualquer porta que o
W-Access mandar, a aplicação tenta abrir direto na máquina.

## O que costuma bloquear: o Firewall do Windows

Quando o Site Controller não consegue se conectar e a tela do emulador
está toda verde (sem nenhum aviso), o primeiro lugar a checar é o
**Firewall do Windows Defender** — ele pode estar barrando a faixa de
portas usada pelos controladores, mesmo com a aplicação escutando nelas
normalmente.

Para liberar uma faixa de portas:

1. Abra **Firewall do Windows Defender** e clique em **Configurações
   avançadas**.
2. Selecione **Regras de entrada**, no menu à esquerda.
3. Clique em **Nova regra**.
4. Escolha o tipo **Porta** e avance.
5. Escolha **TCP** e informe a faixa de portas usada pelos seus
   controladores (por exemplo, `4000-4499`).
6. Siga o assistente permitindo a conexão e conclua a regra.

## Porta já em uso por outro processo

Se outro programa na mesma máquina já estiver usando a porta de um
controlador, o dispositivo correspondente aparece como inalcançável na
tela do emulador, e o log da aplicação (`sistema\logs\trace.log`) traz o
erro do próprio Windows explicando qual porta e por quê. Veja o capítulo
Onde estao os logs para o caminho completo.
