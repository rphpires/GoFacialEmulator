# Antes de comecar

Este manual cobre o pacote Linux do GoFacialEmulator, e vale para dois
públicos diferentes:

- Quem vai instalar num **servidor Linux** de verdade.
- Quem vai instalar dentro do **WSL2**, o Linux que roda dentro do
  Windows.

O pacote e os comandos são os mesmos para os dois casos. A única diferença
é que, se você está no Windows com WSL2, existe um capítulo a mais logo a
seguir (Instalar o WSL2) para preparar esse ambiente antes de tudo o
resto. **Se o seu destino já é um servidor Linux, pule o capítulo Instalar
o WSL2** e vá direto para Instalar o emulador.

## O que ter em mãos

- O arquivo `GoFacialEmulator-linux.zip`.
- Acesso `sudo` na máquina onde vai instalar.
- Os dados de acesso ao banco do W-Access: endereço do servidor, usuário e
  senha. Você vai usá-los mais adiante, no capítulo Configurar o W-Access.

## Onde extrair o pacote

Extraia o ZIP em `/opt/gofacialemulator` ou na pasta pessoal (home) do seu
usuário — qualquer uma das duas funciona; escolha conforme o padrão que
seu ambiente já usa para outros programas.
