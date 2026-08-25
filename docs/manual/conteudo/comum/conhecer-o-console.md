# Conhecer o console

O emulador é controlado inteiramente pelo navegador. Este capítulo mostra o
que cada parte da tela faz, para que os capítulos seguintes possam dizer
"clique em Filtrar" sem precisar explicar onde fica o botão Filtrar.

![O console do emulador](img/gerado/emulador-console.png)

A tela é sempre a mesma estrutura: uma barra estreita de ícones à esquerda,
uma barra de estado no topo, e o conteúdo da página no meio — na imagem
acima, a lista de dispositivos.

## A barra lateral e a navegação

![A barra lateral](img/gerado/emulador-rail.png)

A barra à esquerda tem três ícones de página, um botão que abre e fecha a
barra, e três ações que afetam todos os dispositivos de uma vez.

As três páginas:

1. **Dispositivos** — a lista de todos os emuladores, é a página inicial.
2. **Comparação** — confere se o número de usuários bate entre o W-Access,
   o gerenciador e o emulador.
3. **Conexão W-Access** — onde ficam o endereço, o banco, o usuário e a
   senha usados para falar com o W-Access.

Por padrão a barra mostra só os ícones. O primeiro botão, o de três
traços, alterna entre "Expandir" e "Recolher" — expandida, cada ícone
ganha o nome da página ao lado, mais fácil de conferir em qual página você
está.

Abaixo das três páginas, depois de uma linha divisória, ficam as três
ações de frota, que valem para todos os dispositivos cadastrados de uma
vez: **Iniciar todos**, **Parar todos** e **Sincronizar com o W-Access** —
esta última busca de novo, no W-Access, quais controladores existem com
descrição começando por `emulator` e atualiza a lista.

## O sinal de conexão do W-Access

Ao lado do texto "Conexão W-Access", na barra lateral expandida (ou
sobre o ícone, na barra recolhida), pode aparecer um ponto vermelho. Ele
acende quando as credenciais gravadas para o W-Access **não conseguem
conectar** — servidor fora do ar, senha errada, banco incorreto, o que
for.

Quando o ponto acender:

1. Abra a página **Conexão W-Access**.
2. Confira servidor, banco, usuário e senha.
3. Use o botão que testa a conexão antes de salvar.

Sem o ponto vermelho, a conexão está boa e não há nada a fazer.

## O medidor de frota

![O medidor de frota](img/gerado/emulador-medidor.png)

No topo da tela, ao lado do nome do produto, fica uma barra comprida
dividida em pedacinhos — um por dispositivo cadastrado, até 60; acima
disso cada pedacinho passa a representar um grupo proporcional de
dispositivos, não mais um único emulador. A cor de cada pedacinho segue o
estado do dispositivo: ativo, parado ou desabilitado. Ao lado da barra
fica a leitura em números, no formato "ativos / total".

A barra se atualiza sozinha conforme os dispositivos sobem, param ou são
desabilitados — não precisa recarregar a página para ver o número mudar.

Existe ainda um terceiro indicador, à direita da leitura, que fica em
branco na maior parte do tempo: ele só mostra um aviso ("reconectando…"
ou "sem conexão") quando o navegador perde a atualização automática com o
servidor. Com tudo funcionando — como nas capturas deste manual — esse
espaço fica vazio, então não espere ver nada ali.

## A lista de dispositivos

![A lista de dispositivos](img/gerado/emulador-dispositivos.png)

Cada linha é um dispositivo emulado. As colunas, da esquerda para a
direita:

- **Caixa de seleção** — marca a linha para as ações em lote no topo da
  página.
- **ID** — o número do controlador no W-Access, com uma bolinha colorida
  na frente que repete a cor do Estado.
- **Nome** — o nome cadastrado no W-Access.
- **Modelo** — o fabricante emulado (Dahua, Hikvision, e assim por diante).
- **Porta** — a porta de rede que este dispositivo abre.
- **Modo** — explicado a seguir.
- **Log** — liga ou desliga o registro dos eventos deste dispositivo no
  arquivo de log da aplicação; só pode ser alterado com o dispositivo
  parado.
- **Intervalo** — de quantos em quantos segundos o dispositivo gera um
  evento sozinho, quando está ativo.
- **Usuários** — quantos usuários este dispositivo tem cadastrados.
- **Estado** — ativo, parado ou desabilitado, com a mesma bolinha colorida
  da coluna ID.
- **Ações** — os botões de iniciar, parar e abrir os detalhes do
  dispositivo, um a um.

![A coluna Modo](img/gerado/emulador-coluna-modo.png)

A coluna **Modo** tem dois valores possíveis:

- **Online** — o Site Controller do W-Access valida o acesso e é ele quem
  responde ao dispositivo.
- **Standalone** — o próprio dispositivo valida o acesso sozinho e gera o
  evento.

A troca vale na hora, sem precisar salvar nem reiniciar o dispositivo.
Algumas linhas mostram um traço no lugar do seletor: essa configuração só
existe para os dispositivos do modelo Dahua, então os demais modelos não
têm o que escolher ali.

Acima da tabela ficam a caixa de seleção do cabeçalho, que marca ou
desmarca todas as linhas de uma vez, e os botões **Iniciar selecionados**
e **Parar selecionados**, que agem sobre as linhas marcadas.

## Filtrar e paginar

![Os filtros](img/gerado/emulador-filtros.png)

Três campos filtram a lista: o identificador do controlador
(LocalControllerID), o nome e a porta. Preencha o que precisar e clique em
**Filtrar**; o botão **Limpar** volta a mostrar todos os dispositivos.
Abaixo da tabela, o campo **Itens por página** escolhe quantas linhas
aparecem de uma vez (5, 10, 20 ou 50).

## A gaveta de detalhes

A gaveta é o painel que desliza da direita para mostrar tudo sobre um
único dispositivo. Ela abre pelo terceiro botão da coluna Ações, o de
duas pessoas.

![A gaveta, aba de usuarios](img/gerado/emulador-gaveta-usuarios.png)

No topo da gaveta ficam a luz de estado — a mesma cor da coluna Estado —
e o nome do dispositivo com o número do controlador ao lado. Logo abaixo
ficam duas abas.

A aba **Usuários** lista os usuários cadastrados nesse dispositivo, com
cartão, se tem face cadastrada e validade. O campo de busca no topo filtra
por nome ou ID, e a paginação (**Anterior** / **Próxima**) navega pelo
restante da lista.

![A gaveta, aba de configuracoes](img/gerado/emulador-gaveta-config.png)

A aba **Configurações** tem duas partes. Em cima, a seção **Emulador**:
a mesma caixa **Gravar log de eventos** que aparece na coluna Log da
tabela, e o botão **Salvar**.

![O log do dispositivo](img/gerado/emulador-gaveta-log.png)

É esse botão — marque a caixa e clique em **Salvar** — que liga o
registro dos eventos deste dispositivo em arquivo; como na coluna Log, só
funciona com o dispositivo parado. Abaixo, uma tabela lista as
configurações que o gerenciador do W-Access gravou para este dispositivo;
quando não há nenhuma, ela mostra "Nenhuma configuração gravada".

## A tela se atualiza sozinha

O estado dos dispositivos, o medidor de frota e a gaveta de detalhes
mudam sozinhos, sem precisar recarregar a página — é por isso que quase
nenhuma tela deste console tem um botão de "atualizar". Se, por algum
motivo, a tela parar de mudar mesmo com os dispositivos em atividade,
recarregue a página: a atualização automática se refaz sozinha assim que
a página abre de novo.

## A página de comparação

![A pagina de comparacao](img/gerado/emulador-comparacao.png)

Esta página compara, controlador por controlador, o total de usuários
cadastrado no W-Access, no gerenciador do W-Access e no emulador. Ela não
carrega essa comparação sozinha: clique em **Recontar** para que os três
totais sejam apurados de novo. Linhas destacadas na tabela indicam que os
três números não batem entre si — vale conferir o cadastro de usuários
daquele controlador no W-Access.
