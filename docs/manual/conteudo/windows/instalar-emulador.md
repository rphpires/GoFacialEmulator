# Instalar o emulador

Na pasta onde você extraiu o ZIP (`C:\GoFacialEmulator`):

![Pasta extraida em C:\GoFacialEmulator](img/manual/windows-pasta.png)

## Instalar

Dê duplo-clique em `INSTALAR.bat`. Ele cria o banco de dados embutido do
zero, o que demora cerca de **um minuto** — bem mais devagar que o
`INICIAR.bat` de todo dia, então não se assuste com a espera. Ao final,
aparece:

```
✅ Instalado. Rode INICIAR.bat
```

![Janela do INSTALAR.bat concluida](img/manual/windows-instalar.png)

Rodar `INSTALAR.bat` de novo, por engano ou para conferir, não faz mal
nenhum: ele detecta que o banco já existe e não apaga nada — só avisa "O
banco já estava instalado. Nada a fazer." e termina.

## Iniciar

Dê duplo-clique em `INICIAR.bat`. Espere até aparecer:

```
✅ Rodando em http://localhost:7070
```

Abra esse endereço no navegador. É a tela do console do emulador — o
próximo capítulo, Conhecer o console, explica cada parte dela.

Se a janela avisar que a porta 7070 já está em uso, é provável que o
emulador já esteja rodando de uma vez anterior: tente abrir
`http://localhost:7070` direto no navegador antes de rodar `PARAR.bat` e
tentar de novo.

## Parar

Dê duplo-clique em `PARAR.bat`. Ele encerra a aplicação **e** o banco de
dados embutido juntos — inclusive quando a janela da aplicação já foi
fechada antes, o que é comum quando ela roda minimizada. Os dados
cadastrados continuam salvos; da próxima vez basta `INICIAR.bat` de novo.
