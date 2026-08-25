# Problemas comuns

| Sintoma | Causa provável | O que fazer |
|---|---|---|
| A lista de dispositivos está vazia | A conexão com o W-Access falhou, ou nenhum controlador cadastrado tem descrição começando com `emulator` | Confira o ponto vermelho no menu; se estiver aceso, veja a linha abaixo. Se não estiver, confira no W-Access se existe algum controlador com a descrição certa |
| Ponto vermelho ao lado de "Conexão W-Access" no menu | As credenciais gravadas não conseguem conectar no banco do W-Access | Abra a página Conexão W-Access, confira servidor, banco, usuário e senha, e use o botão de testar conexão |
| Tudo verde na tela do emulador e tudo offline no W-Access | A porta do controlador não é alcançável neste ambiente | Leia o aviso no topo da lista de dispositivos e o capítulo Portas e rede — é o caso mais comum, e o mais confuso, porque o emulador não mostra erro nenhum |
| Um dispositivo não inicia | A porta já está em uso por outro processo | Veja o log da aplicação: ele traz o erro do sistema operacional explicando qual porta e por quê |
| A coluna Modo mostra um traço em vez do seletor | O dispositivo não é do modelo Dahua — só esse modelo usa essa configuração | Nada a fazer; é o comportamento esperado para os demais modelos |
| A tela parou de se atualizar sozinha | A atualização automática com o servidor caiu | Recarregue a página; a atualização automática se refaz sozinha |
| Ao iniciar, aparece que a aplicação não respondeu em 60 segundos | A aplicação demorou demais para subir, ou não subiu | Veja `trace.log`; no pacote Docker, rode `docker compose logs app` para ver a saída do contêiner |
| A página abre sem formatação, só texto puro | O pacote está incompleto — faltaram arquivos na instalação | Baixe o pacote de novo; isso não deveria acontecer com uma instalação íntegra |
