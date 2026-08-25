# Instalar o Docker Desktop

O pacote Docker precisa do Docker Desktop instalado e **funcionando** antes
de qualquer outro passo. Se você já tem o Docker Desktop instalado e sabe
que ele está funcionando, pode pular direto para o próximo capítulo.

## Baixar e instalar

Acesse `https://www.docker.com/products/docker-desktop/` e baixe o
instalador para Windows.

![Pagina de download do Docker Desktop](img/manual/docker-download.png)

Execute o instalador baixado. Logo no início ele pergunta sobre as opções
de configuração — **deixe marcada a opção que usa o WSL2**, é o modo que
este pacote espera encontrar.

![Aviso de configuracao do instalador do Docker Desktop](img/manual/docker-wizard-01.png)

A instalação leva alguns minutos.

![Instalacao em andamento](img/manual/docker-wizard-02.png)

Ao final, o instalador avisa que a instalação terminou e pede para
reiniciar o computador. Reinicie quando ele pedir.

![Instalacao concluida, pedindo reinicio](img/manual/docker-wizard-03.png)

## Abrir o Docker Desktop pela primeira vez

Depois de reiniciar, abra o Docker Desktop. Na primeira abertura ele
mostra os termos de uso — aceite para continuar.

![Termos de uso do Docker Desktop](img/manual/docker-termos.png)

## Esperar o indicador ficar verde

Depois de aceitar os termos, o Docker Desktop leva algum tempo para
terminar de subir os componentes internos dele. Só quando esse processo
termina é que o ícone do Docker Desktop fica **verde**.

![Docker Desktop aberto com o icone verde](img/manual/docker-verde.png)

**Espere o indicador ficar verde antes de seguir para o próximo capítulo.**
É aqui que a maioria dos problemas de instalação começa: a pessoa abre o
Docker Desktop, vê a janela abrir e já duplo-clica em `INSTALAR.bat` — mas
o Docker ainda está subindo por dentro, ainda não aceita comandos, e o
instalador do emulador falha logo no primeiro passo, avisando que o Docker
não está rodando. Se isso acontecer, feche o aviso, espere o ícone ficar
verde e rode `INSTALAR.bat` de novo — nada é perdido.
