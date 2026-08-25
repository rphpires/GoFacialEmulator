# Instalar o emulador

Na pasta onde você extraiu o ZIP, rode:

```
sudo bash instalar.sh
```

## O que o instalador pergunta e faz

Primeiro, ele pergunta qual faixa de portas os controladores emulados vão
usar, com um valor padrão já sugerido:

```
Qual faixa de portas os emuladores vao usar?
As portas vem do W-Access (campo BaseCommPort de cada controlador).
Faixa [4000-4499]:
```

Essa faixa precisa **cobrir os valores de `BaseCommPort` cadastrados nos
controladores no W-Access** — se um controlador usa uma porta fora dela,
o dispositivo dele sobe do mesmo jeito, mas fica fora do alcance do que o
instalador liberou no firewall. Para aceitar o padrão sugerido
(4000-4499), basta apertar Enter; para usar outra faixa, digite-a no
formato `inicio-fim`.

Depois disso, sem mais perguntas, o instalador:

1. Instala o PostgreSQL, se ainda não estiver instalado.
2. Liga o banco e cria o usuário e o banco de dados da aplicação.
3. Libera a faixa de portas informada no firewall (`ufw` ou `firewalld`,
   o que estiver ativo na máquina).
4. Confere o limite de arquivos abertos do sistema e ajusta se for
   necessário — veja o porquê no capítulo Portas e rede.

Ao final, aparece:

```
✅ Instalado. Rode ./iniciar.sh
```

**Se o instalador avisar que ajustou o limite de arquivos abertos**, saia
da sessão (feche e abra o terminal, ou desconecte e reconecte por SSH) e
entre de novo **antes** de rodar `./iniciar.sh` — o ajuste só vale para
sessões abertas depois dele.

## Iniciar

```
./iniciar.sh
```

Espere até aparecer:

```
✅ Rodando em http://localhost:7070
```

Abra esse endereço no navegador. É a tela do console do emulador — o
próximo capítulo, Conhecer o console, explica cada parte dela.

## Parar

```
./parar.sh
```

Os dados cadastrados continuam salvos, e o banco de dados continua ligado
como serviço do sistema; da próxima vez basta `./iniciar.sh` de novo.

## Se der "Permission denied"

O arquivo ZIP não preserva a permissão de execução dos scripts. Se
`./iniciar.sh` ou `./parar.sh` derem erro de permissão, chame o
interpretador diretamente:

```
bash iniciar.sh
bash parar.sh
```

Isso não deveria mais acontecer depois de rodar `sudo bash instalar.sh`
uma vez — ele já ajusta a permissão de execução dos dois scripts — mas é a
saída rápida se acontecer antes disso ou num ambiente diferente.
