/**
 * devices.js — tabela de dispositivos.
 *
 * Todo o estado de uma linha (LED, badge, botões, checkbox de log) é
 * derivado de um único data-state, aplicado por pintarLinha(). Antes essa
 * lógica existia duplicada: uma vez no template Go e outra numa template
 * string de JS, que iam divergindo.
 */
(function () {
    'use strict';

    // Definida em Task 8; o stub evita ReferenceError se o modal ainda não
    // existir na página.
    window.abrirFormularioEmulador = window.abrirFormularioEmulador || function () {
        window.Toast.err('Formulário de emulador indisponível');
    };

    var ROTULOS = { running: 'ativo', stopped: 'parado', disabled: 'desabilitado', error: 'erro' };

    // ------------------------------------------------------------------
    // Render de linha
    // ------------------------------------------------------------------

    function pintarLinha(id, estado) {
        var linha = document.getElementById('device-' + id);
        if (!linha) { return; }

        var anterior = linha.getAttribute('data-state');
        linha.setAttribute('data-state', estado);

        var leds = linha.querySelectorAll('.led');
        for (var i = 0; i < leds.length; i++) {
            leds[i].setAttribute('data-state', estado);
            if (anterior !== estado) { piscar(leds[i]); }
        }

        var badge = linha.querySelector('.badge');
        if (badge) {
            badge.setAttribute('data-state', estado);
            // Só o nó de texto: recriar o innerHTML jogaria fora o LED interno.
            var texto = badge.lastChild;
            if (texto && texto.nodeType === Node.TEXT_NODE) {
                texto.nodeValue = ' ' + (ROTULOS[estado] || estado);
            }
        }

        var iniciar = linha.querySelector('.device-start');
        var parar = linha.querySelector('.device-stop');
        var log = linha.querySelector('.log-check');

        if (iniciar) {
            iniciar.disabled = estado !== 'stopped';
            iniciar.removeAttribute('data-pending');
        }
        if (parar) {
            parar.disabled = estado !== 'running';
            parar.removeAttribute('data-pending');
        }
        if (log) { log.disabled = estado !== 'stopped'; }
    }

    function piscar(led) {
        led.classList.remove('led--flash');
        // Força reflow para a animação reiniciar em mudanças consecutivas.
        void led.offsetWidth;
        led.classList.add('led--flash');
    }

    function atualizarContagem(id, totalUsuarios) {
        var linha = document.getElementById('device-' + id);
        if (!linha) { return; }
        var celulas = linha.querySelectorAll('td.num');
        // Ordem das colunas numéricas: porta, intervalo, usuários.
        if (celulas.length >= 3) {
            celulas[2].textContent = String(totalUsuarios);
        }
    }

    // ------------------------------------------------------------------
    // Seleção
    // ------------------------------------------------------------------

    function selecionados() {
        var ids = [];
        var checks = document.querySelectorAll('.device-check:checked');
        for (var i = 0; i < checks.length; i++) {
            ids.push(checks[i].closest('tr').getAttribute('data-id'));
        }
        return ids;
    }

    function logsSelecionados() {
        var mapa = {};
        var checks = document.querySelectorAll('.device-check:checked');
        for (var i = 0; i < checks.length; i++) {
            var linha = checks[i].closest('tr');
            var log = linha.querySelector('.log-check');
            mapa[linha.getAttribute('data-id')] = log ? log.checked : false;
        }
        return mapa;
    }

    function sincronizarSelecao() {
        var todos = document.querySelectorAll('.device-check');
        var marcados = document.querySelectorAll('.device-check:checked');
        var selectAll = document.getElementById('select-all');

        if (selectAll) {
            selectAll.checked = todos.length > 0 && marcados.length === todos.length;
            selectAll.indeterminate = marcados.length > 0 && marcados.length < todos.length;
        }

        // Botões em lote nascem desabilitados: um clique sem seleção antes
        // devolvia um alert("Nenhum dispositivo selecionado"), que é um erro
        // que a interface podia ter evitado.
        var vazio = marcados.length === 0;
        var iniciar = document.getElementById('start-selected');
        var parar = document.getElementById('stop-selected');
        var excluir = document.getElementById('delete-selected');
        if (iniciar) { iniciar.disabled = vazio; }
        if (parar) { parar.disabled = vazio; }
        if (excluir) { excluir.disabled = vazio; }
    }

    // ------------------------------------------------------------------
    // Ações
    // ------------------------------------------------------------------

    function postar(url, corpo) {
        return fetch(url, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(corpo)
        }).then(function (resposta) {
            if (!resposta.ok) { throw new Error('HTTP ' + resposta.status); }
            return resposta;
        });
    }

    function acaoLinha(botao, url, corpo, mensagem) {
        botao.setAttribute('data-pending', 'true');

        postar(url, corpo)
            .then(function () { window.Toast.ok(mensagem); })
            .catch(function () {
                botao.removeAttribute('data-pending');
                window.Toast.err('A operação falhou. Verifique o log do serviço.');
            });
        // O estado final chega pelo SSE, que é quem sabe o resultado de
        // verdade; pintarLinha() limpa o data-pending.
    }

    // Remove um emulador e tira a linha da tela. Nunca rejeita: quem chama
    // em lote precisa do resultado de cada um para montar o resumo, e uma
    // promise rejeitada no meio abortaria as restantes.
    function removerEmulador(id) {
        return fetch('/api/emulators/' + id, { method: 'DELETE' })
            .then(function (resp) {
                return resp.json().then(function (corpo) {
                    if (!resp.ok) {
                        return { ok: false, erro: corpo.error || 'Falha ao remover o emulador' };
                    }
                    var linha = document.getElementById('device-' + id);
                    if (linha) { linha.remove(); }
                    return { ok: true, erro: '' };
                });
            })
            .catch(function (err) {
                return { ok: false, erro: 'Falha ao remover: ' + err.message };
            });
    }

    // Dispositivo vindo do W-Access não pode ser removido — a verdade dele
    // mora lá e o próximo sync o traria de volta. O servidor recusa com 409,
    // mas descobrir isso só depois de disparar N requisições daria uma
    // enxurrada de erros para algo que a própria tela já sabe: o template só
    // desenha o botão .device-remove nas linhas de origem manual.
    function particionarSelecao() {
        var removiveis = [];
        var gerenciados = [];
        selecionados().forEach(function (id) {
            var linha = document.getElementById('device-' + id);
            if (linha && linha.querySelector('.device-remove')) {
                removiveis.push(id);
            } else {
                gerenciados.push(id);
            }
        });
        return { removiveis: removiveis, gerenciados: gerenciados };
    }

    // Sequencial em LIMITE frentes. Um Promise.all sobre a seleção inteira
    // abriria centenas de conexões de uma vez — a criação em lote permite
    // 1000 emuladores, e o navegador enfileiraria tudo de qualquer jeito,
    // só que sem controle nenhum sobre a ordem dos erros.
    function emLotes(ids, tarefa) {
        var LIMITE = 6;
        var resultados = [];
        var proximo = 0;

        function frente() {
            if (proximo >= ids.length) { return Promise.resolve(); }
            var indice = proximo++;
            return tarefa(ids[indice]).then(function (r) {
                resultados[indice] = r;
                return frente();
            });
        }

        var frentes = [];
        for (var i = 0; i < Math.min(LIMITE, ids.length); i++) {
            frentes.push(frente());
        }
        return Promise.all(frentes).then(function () { return resultados; });
    }

    function excluirSelecionados(botao) {
        var particao = particionarSelecao();
        var ids = particao.removiveis;
        var gerenciados = particao.gerenciados;

        if (ids.length === 0) {
            window.Toast.err(gerenciados.length > 0
                ? 'Nenhum dos selecionados pode ser removido: todos vieram do W-Access'
                : 'Nenhum emulador selecionado');
            return;
        }

        var aviso = 'Excluir ' + ids.length + ' emulador(es)?\n\n' +
            'Os cartões, faces e usuários cadastrados neles também serão apagados. ' +
            'Não há como desfazer.';
        if (gerenciados.length > 0) {
            aviso += '\n\n' + gerenciados.length + ' dispositivo(s) da seleção vieram do ' +
                'W-Access e não serão tocados.';
        }
        if (!window.confirm(aviso)) { return; }

        botao.disabled = true;
        emLotes(ids, removerEmulador).then(function (resultados) {
            var falhas = resultados.filter(function (r) { return !r.ok; });
            var removidos = resultados.length - falhas.length;

            if (removidos > 0) { window.Toast.ok(removidos + ' emulador(es) removido(s)'); }
            // A primeira falha basta na tela; o resto sai no log do serviço.
            // Um toast por erro empilharia centenas deles numa seleção grande.
            if (falhas.length > 0) {
                window.Toast.err(falhas.length + ' não removido(s). ' + falhas[0].erro);
            }
            sincronizarSelecao();
        });
    }

    function iniciarAcoes() {
        document.addEventListener('click', function (evento) {
            var iniciar = evento.target.closest('.device-start');
            if (iniciar) {
                var idIniciar = iniciar.getAttribute('data-id');
                var linhaIniciar = document.getElementById('device-' + idIniciar);
                var log = linhaIniciar ? linhaIniciar.querySelector('.log-check') : null;
                var mapaLog = {};
                mapaLog[idIniciar] = log ? log.checked : false;
                acaoLinha(iniciar, '/start', { devices: [idIniciar], enable_log: mapaLog },
                    'Iniciando emulador ' + idIniciar);
                return;
            }

            var parar = evento.target.closest('.device-stop');
            if (parar) {
                var idParar = parar.getAttribute('data-id');
                acaoLinha(parar, '/stop', { devices: [idParar] },
                    'Parando emulador ' + idParar);
                return;
            }

            // Mesmo seletor e mesmo data-attribute da implementação anterior,
            // para o botão Detalhes continuar funcionando igual.
            var detalhes = evento.target.closest('.device-details-btn');
            if (detalhes && window.DeviceDrawer) {
                var linha = detalhes.closest('tr');
                var celulaNome = linha ? linha.querySelector('.device-name-cell') : null;
                window.DeviceDrawer.open(
                    detalhes.getAttribute('data-device-id'),
                    celulaNome ? celulaNome.textContent.trim() : ''
                );
            }
        });

        var iniciarLote = document.getElementById('start-selected');
        if (iniciarLote) {
            iniciarLote.addEventListener('click', function () {
                var ids = selecionados();
                postar('/start', { devices: ids, enable_log: logsSelecionados() })
                    .then(function () { window.Toast.ok('Iniciando ' + ids.length + ' emulador(es)'); })
                    .catch(function () { window.Toast.err('Não foi possível iniciar os emuladores'); });
            });
        }

        var pararLote = document.getElementById('stop-selected');
        if (pararLote) {
            pararLote.addEventListener('click', function () {
                var ids = selecionados();
                postar('/stop', { devices: ids })
                    .then(function () { window.Toast.ok('Parando ' + ids.length + ' emulador(es)'); })
                    .catch(function () { window.Toast.err('Não foi possível parar os emuladores'); });
            });
        }

        // Remover é irreversível e leva junto cartões e faces cadastrados
        // naquele emulador — a confirmação precisa dizer isso, não só "tem
        // certeza?".
        document.querySelectorAll('.device-remove').forEach(function (btn) {
            btn.addEventListener('click', function () {
                var id = btn.getAttribute('data-id');
                var nome = btn.getAttribute('data-name') || id;
                var ok = window.confirm(
                    'Remover o emulador "' + nome + '"?\n\n' +
                    'Os cartões, faces e usuários cadastrados nele também serão apagados. ' +
                    'Não há como desfazer.'
                );
                if (!ok) { return; }

                btn.disabled = true;
                removerEmulador(id).then(function (resultado) {
                    if (resultado.ok) {
                        window.Toast.ok('Emulador ' + nome + ' removido');
                        return;
                    }
                    window.Toast.err(resultado.erro);
                    btn.disabled = false;
                });
            });
        });

        var excluirLote = document.getElementById('delete-selected');
        if (excluirLote) {
            excluirLote.addEventListener('click', function () {
                excluirSelecionados(excluirLote);
            });
        }

        document.querySelectorAll('.device-edit').forEach(function (btn) {
            btn.addEventListener('click', function () {
                var linha = document.getElementById('device-' + btn.getAttribute('data-id'));
                if (!linha) { return; }
                var nomeCel = linha.querySelector('.device-name-cell');
                var portaCel = linha.querySelector('td.num');
                window.abrirFormularioEmulador({
                    id: btn.getAttribute('data-id'),
                    name: nomeCel ? nomeCel.textContent.trim() : '',
                    model: linha.children[3] ? linha.children[3].textContent.trim() : '',
                    port: portaCel ? Number(portaCel.textContent.trim()) : null,
                    // Vêm dos data-* do próprio botão (devices.html), que o
                    // servidor preenche com ip_address/interval/enabled da
                    // linha — ver getCurrentDevicesWithFilters em handlers.go.
                    ip_address: btn.getAttribute('data-ip') || '',
                    event_interval: Number(btn.getAttribute('data-interval')),
                    enabled: btn.getAttribute('data-enabled') === '1'
                });
            });
        });
    }

    function iniciarSelecao() {
        var selectAll = document.getElementById('select-all');
        if (selectAll) {
            selectAll.addEventListener('change', function () {
                var checks = document.querySelectorAll('.device-check');
                for (var i = 0; i < checks.length; i++) {
                    checks[i].checked = selectAll.checked;
                }
                sincronizarSelecao();
            });
        }

        document.addEventListener('change', function (evento) {
            if (evento.target.classList.contains('device-check')) {
                sincronizarSelecao();
            }
        });

        sincronizarSelecao();
    }

    function iniciarPaginacao() {
        var perPage = document.getElementById('per-page');
        if (!perPage) { return; }

        perPage.addEventListener('change', function () {
            var params = new URLSearchParams(window.location.search);
            params.set('page', '1');
            params.set('per_page', perPage.value);
            window.location.search = params.toString();
        });
    }

    // ------------------------------------------------------------------
    // Modo do dispositivo (online / standalone)
    // ------------------------------------------------------------------

    // Troca o modo de um dispositivo. O select só existe para modelos
    // Dahua — ver o {{ if eq .model "Dahua" }} em devices.html.
    function iniciarModo() {
        var selects = document.querySelectorAll('.device-mode');
        for (var i = 0; i < selects.length; i++) {
            // Guarda o valor gravado no banco (o que veio "selected" do
            // template) para poder desfazer se a troca falhar.
            selects[i].setAttribute('data-anterior', selects[i].value);
        }

        document.addEventListener('change', function (evento) {
            var alvo = evento.target.closest('.device-mode');
            if (!alvo) { return; }

            var id = alvo.getAttribute('data-device-id');
            var anterior = alvo.getAttribute('data-anterior') || alvo.value;

            postar('/api/devices/' + id + '/mode', { mode: alvo.value })
                .then(function () {
                    alvo.setAttribute('data-anterior', alvo.value);
                    window.Toast.ok('Dispositivo ' + id + ': modo ' + alvo.value);
                })
                .catch(function () {
                    // Volta o select ao valor anterior: deixar o valor novo
                    // na tela sem ter gravado faria a tela mentir sobre o
                    // estado do dispositivo.
                    alvo.value = anterior;
                    window.Toast.err('Não foi possível trocar o modo do dispositivo ' + id);
                });
        });
    }

    // ------------------------------------------------------------------
    // Aviso de alcançabilidade
    // ------------------------------------------------------------------

    // Preenche o aviso de alcançabilidade. Sem dispositivo problemático o
    // bloco continua escondido — "inalcancavel" e "desconhecido" são os
    // únicos vereditos de reachability.Status que pedem atenção;
    // "nao_iniciado" é o estado normal de um dispositivo parado.
    function carregarAlcancabilidade() {
        var linha = document.getElementById('reachability-alert-row');
        if (!linha) { return; }

        fetch('/api/reachability')
            .then(function (resposta) {
                if (!resposta.ok) { throw new Error('HTTP ' + resposta.status); }
                return resposta.json();
            })
            .then(function (relatorio) {
                var devices = relatorio.devices || [];
                var problematicos = devices.filter(function (d) {
                    return d.status === 'inalcancavel' || d.status === 'desconhecido';
                });
                if (problematicos.length === 0) { return; }

                document.getElementById('reachability-headline').textContent =
                    problematicos.length + ' dispositivo(s) não vão ser alcançados pelo Site Controller';
                document.getElementById('reachability-reason').textContent =
                    problematicos[0].reason || '';
                document.getElementById('reachability-help').textContent =
                    'Veja o capítulo Portas e rede do manual.';

                var lista = document.getElementById('reachability-list');
                lista.textContent = '';
                problematicos.forEach(function (d) {
                    var li = document.createElement('li');
                    li.textContent = 'Dispositivo ' + d.device_id + ' — porta ' + d.port +
                        (d.reason ? ': ' + d.reason : '');
                    lista.appendChild(li);
                });

                var toggle = document.getElementById('reachability-toggle');
                if (toggle) {
                    toggle.addEventListener('click', function () {
                        lista.hidden = !lista.hidden;
                    });
                }

                linha.hidden = false;
            })
            .catch(function () {
                // Sem resposta de /api/reachability não há nada de
                // confiável a mostrar: o bloco continua escondido.
            });
    }

    function iniciarStream() {
        window.FleetStream.subscribe('snapshot', function (dados) {
            dados.devices.forEach(function (device) {
                pintarLinha(device.id, device.status);
                atualizarContagem(device.id, device.total_users);
            });
        });

        window.FleetStream.subscribe('device', function (dados) {
            if (!dados.device) { return; }
            pintarLinha(dados.device.id, dados.device.status);
            atualizarContagem(dados.device.id, dados.device.total_users);
        });
    }

    document.addEventListener('DOMContentLoaded', function () {
        iniciarSelecao();
        iniciarAcoes();
        iniciarPaginacao();
        iniciarStream();
        iniciarModo();
        carregarAlcancabilidade();
    });
})();
