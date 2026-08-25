# Portas e rede

Se você chegou aqui pelo passo 3 do Roteiro de validação — controlador
offline no W-Access com a tela do emulador toda verde — este é o capítulo
certo.

## De onde vêm as portas

As portas que cada dispositivo emulado usa **não são escolhidas pelo
emulador**. Elas vêm do cadastro do controlador no W-Access, do campo de
porta de comunicação (`BaseCommPort`). O emulador só obedece: abre a porta
que o W-Access mandou.

## A faixa publicada neste pacote

O pacote Docker roda dentro de um contêiner. Para o Site Controller
conseguir falar com um dispositivo emulado, a porta dele precisa estar
**publicada** — ou seja, redirecionada de dentro do contêiner para a
máquina Windows.

Neste pacote, a faixa publicada é **4000 a 4499**. Um controlador cadastrado
no W-Access com porta dentro dessa faixa funciona normalmente. Um
controlador com porta **fora** dessa faixa sobe do mesmo jeito — o
dispositivo aparece ativo, verde, sem erro nenhum na tela — mas a porta
dele fica escutando só dentro do contêiner, e o Site Controller não
consegue alcançá-la. É exatamente o sintoma do passo 3 do Roteiro de
validação.

Quando isso acontece, a lista de dispositivos mostra um aviso no topo da
tela:

![Aviso de portas nao publicadas](img/gerado/emulador-aviso-portas.png)

## Como alargar a faixa

Se os controladores do seu site usam portas fora de 4000-4499, edite o
arquivo `sistema\docker-compose.yml`, dentro da pasta onde o pacote foi
extraído. Duas partes desse arquivo precisam mudar **juntas, para a mesma
faixa** — mudar só uma delas não resolve:

```yaml
    environment:
      ...
      PUBLISHED_PORT_RANGE: "4000-4499"
    ports:
      - "7070:7070"
      - "4000-4499:4000-4499"
```

Troque os dois trechos `4000-4499` pela faixa que os seus controladores
realmente usam, salve o arquivo e rode `PARAR.bat` seguido de
`INICIAR.bat` para os serviços subirem de novo já com a faixa nova. Não
mexa na porta `7070:7070` — é a porta do próprio console do emulador, e é
sempre a mesma em qualquer pacote.

## Em Linux não há limite

Esse limite de faixa publicada existe só na versão para Windows deste
pacote. Rodando este mesmo pacote Docker em um servidor Linux (usando os
scripts `.sh` em vez dos `.bat`), o contêiner usa a rede do próprio
servidor — não há faixa para configurar, e qualquer porta que o W-Access
peça funciona.
