# Instalar o emulador

Com o Docker Desktop instalado, verde e funcionando, instale o emulador em
si.

## Instalar

Na pasta onde você extraiu o ZIP (`C:\GoFacialEmulator`), dê duplo-clique
em `INSTALAR.bat`. **Isso só precisa ser feito uma vez.**

O instalador confere se o Docker está rodando, carrega a aplicação e
prepara o banco de dados — essa última parte pode levar um ou dois minutos
na primeira vez. Espere até aparecer:

```
✅ Instalado. Rode INICIAR.bat
```

Se aparecer um erro dizendo que o Docker não está rodando, volte ao
capítulo anterior, espere o ícone do Docker Desktop ficar verde e
duplo-clique em `INSTALAR.bat` de novo.

## Iniciar

Dê duplo-clique em `INICIAR.bat`. Espere até aparecer:

```
✅ Rodando em http://localhost:7070
```

Abra esse endereço no navegador. É a tela do console do emulador — o
próximo capítulo, Conhecer o console, explica cada parte dela.

## Parar

Para desligar o emulador, dê duplo-clique em `PARAR.bat`. Os dados
cadastrados continuam salvos; da próxima vez basta `INICIAR.bat` de novo,
sem precisar reinstalar.

## Em Linux

Se você está instalando este mesmo pacote Docker num servidor Linux, em
vez dos arquivos `.bat` use os scripts equivalentes, na mesma pasta:

```
./instalar.sh
./iniciar.sh
./parar.sh
```

Eles fazem exatamente o mesmo que os `.bat` do Windows, com as mesmas
mensagens de espera.
