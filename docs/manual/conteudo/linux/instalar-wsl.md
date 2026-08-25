# Instalar o WSL2

**Este capítulo é só para quem vai instalar o emulador dentro do Windows,
usando o WSL2.** Se o seu destino é um servidor Linux de verdade, pule
este capítulo e vá direto para Instalar o emulador.

## Instalar o WSL2

Abra o **PowerShell como administrador** (clique com o botão direito no
menu Iniciar, ou procure "PowerShell", clique com o botão direito no
resultado e escolha "Executar como administrador").

![PowerShell como administrador](img/manual/wsl-powershell-admin.png)

No PowerShell, rode:

```
wsl --install
```

![Saida do comando wsl --install](img/manual/wsl-install.png)

Quando ele pedir, reinicie o computador.

Depois de reiniciar, o Ubuntu abre sozinho pela primeira vez e pede para
você criar um usuário e uma senha para dentro do Linux — esse usuário é
independente da sua conta do Windows, escolha o que preferir.

![Primeira abertura do Ubuntu pedindo usuario](img/manual/wsl-primeiro-uso.png)

## Atenção: o modo de rede do WSL2

**No WSL2, por padrão, só a própria máquina alcança os emuladores.** Um
Site Controller rodando em outro computador da rede não vai conseguir se
conectar aos dispositivos emulados dentro do WSL2, mesmo com tudo
funcionando perfeitamente por dentro — é o aviso que mais gera chamado de
suporte neste pacote.

Para corrigir, crie (ou edite, se já existir) o arquivo `.wslconfig` na
pasta do seu usuário do Windows, em
`C:\Users\SEU_USUARIO\.wslconfig`, com este conteúdo:

```ini
[wsl2]
networkingMode=mirrored
```

Depois, no PowerShell do Windows (não precisa ser como administrador
desta vez), rode:

```
wsl --shutdown
```

e abra o Ubuntu de novo. A partir daí, outras máquinas da rede alcançam os
emuladores normalmente.

Se você esquecer desse ajuste, o instalador do próximo capítulo
(`instalar.sh`) detecta que o modo espelhado não está ligado e mostra este
mesmo aviso na tela, com o mesmo trecho para colar em `.wslconfig`.
