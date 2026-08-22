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
        if (iniciar) { iniciar.disabled = vazio; }
        if (parar) { parar.disabled = vazio; }
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
    });
})();
