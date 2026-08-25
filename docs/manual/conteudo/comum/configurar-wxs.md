# Configurar o W-Access

Para o emulador enxergar um controlador, ele precisa existir no W-Access
com um cadastro específico. Este capítulo mostra o que preencher lá e,
depois, como apontar o emulador para o banco do W-Access.

## O cadastro do controlador no W-Access

No W-Access, a descrição do controlador precisa **começar com a palavra
`emulator`** — é assim, e só assim, que o emulador reconhece que aquele
controlador é dele e deve subir um dispositivo para ele. Um controlador
com qualquer outra descrição é ignorado.

O endereço e a porta de comunicação cadastrados (BaseCommPort) são o IP e
a porta que o emulador vai abrir para esse dispositivo — é neles que o
Site Controller vai tentar falar quando o dispositivo estiver em modo
online.

Um passeio rápido pelas telas do W-Access, na ordem em que você vai
usá-las:

![Tela de login do W-Access](img/gerado/wxs-login.png)

Entre no W-Access com seu usuário.

![Tela inicial do W-Access após o login](img/gerado/wxs-pos-login.png)

Esta é a tela inicial, só para confirmar que o acesso deu certo.

![Lista de controladores no W-Access](img/gerado/wxs-controladores.png)

No menu de dispositivos, abra **Controladores** para ver a lista.

![Cadastro do controlador no W-Access](img/gerado/wxs-inicial.png)

Abra o controlador que vai ser emulado (ou cadastre um novo) e confira
três campos: a descrição começando com `emulator`, o endereço e a porta
de comunicação.

## A tela de conexão do emulador

![Tela de conexao com o W-Access](img/gerado/emulador-configuracoes.png)

No console do emulador, abra a página **Conexão W-Access** e preencha:

- **Host ou endereço IP** — o servidor onde o W-Access roda.
- **Porta** — 1433, a porta padrão do banco do W-Access.
- **Database** — `W_Access`.
- **Usuário** e **Senha** — as credenciais de acesso ao banco.

O botão com o ícone de lupa, ao lado do campo Senha, mostra a senha
digitada em texto simples, para conferir antes de salvar.

## Testar, salvar e sincronizar

Antes de sair da página, clique em **Testar conexão**. Se o teste falhar,
revise servidor, banco, usuário e senha — não adianta salvar uma conexão
que não funciona. Com o teste dando certo, clique em **Salvar**.

Volte para a página **Dispositivos** e use o botão **Sincronizar com o
W-Access**, na barra lateral, para o emulador buscar os controladores com
descrição `emulator` de novo e trazer o que você acabou de cadastrar.

Se depois disso o ponto vermelho continuar aceso ao lado de "Conexão
W-Access" no menu, veja o capítulo Conhecer o console, na seção sobre o
sinal de conexão — ele explica o que esse ponto significa e o que
verificar.

O significado dos modos **Online** e **Standalone**, na coluna Modo da
lista de dispositivos, está no capítulo Conhecer o console — não é
repetido aqui.
