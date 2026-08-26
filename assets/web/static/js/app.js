/**
 * app.js — comportamento do shell: rail, fleet meter, ações globais.
 */
(function () {
    'use strict';

    var CHAVE_RAIL = 'fe.rail.expanded';

    // ------------------------------------------------------------------
    // Rail
    // ------------------------------------------------------------------

    function iniciarRail() {
        var shell = document.getElementById('shell');
        var toggle = document.getElementById('rail-toggle');
        if (!shell || !toggle) { return; }

        // O estado antes era perdido a cada navegação, e o menu voltava
        // recolhido em toda página.
        var expandido = window.localStorage.getItem(CHAVE_RAIL) === 'true';
        aplicar(expandido);

        toggle.addEventListener('click', function () {
            expandido = !expandido;
            aplicar(expandido);
            window.localStorage.setItem(CHAVE_RAIL, String(expandido));
        });

        function aplicar(aberto) {
            // Classe/atributo, não escrita de estilo em JS: a transição fica no
            // CSS e o prefers-reduced-motion continua valendo.
            shell.setAttribute('data-rail', aberto ? 'expanded' : 'collapsed');
            toggle.setAttribute('aria-expanded', String(aberto));
            toggle.querySelector('.rail__label').textContent = aberto ? 'Recolher' : 'Expandir';
        }
    }

    function marcarNavAtiva() {
        var caminho = window.location.pathname;
        var itens = document.querySelectorAll('.rail__item[data-nav]');

        for (var i = 0; i < itens.length; i++) {
            if (itens[i].getAttribute('data-nav') === caminho) {
                itens[i].setAttribute('aria-current', 'page');
            }
        }
    }

    // ------------------------------------------------------------------
    // Sinal da conexão com o W-Access
    // ------------------------------------------------------------------

    // O ponto vermelho no item "Conexão W-Access" acende quando as
    // credenciais gravadas não conectam. A verificação é assíncrona e nunca
    // bloqueia a página: falha de rede aqui apaga o ponto em vez de mentir.
    var WxsSignal = (function () {
        var INTERVALO = 60000;

        function pintar(ok, mensagem) {
            var ponto = document.getElementById('wxs-dot');
            if (!ponto) { return; }

            ponto.hidden = ok;
            if (!ok) {
                var texto = 'Conexão com o W-Access indisponível' +
                    (mensagem ? ': ' + mensagem : '.');
                ponto.setAttribute('aria-label', texto);
                ponto.setAttribute('title', texto);
            }
        }

        function verificar(forcar) {
            var url = '/api/settings/wxs-status' + (forcar ? '?force=1' : '');

            return fetch(url)
                .then(function (r) { return r.json(); })
                .then(function (dados) { pintar(!!dados.ok, dados.error); })
                .catch(function () { pintar(true, ''); });
        }

        function start() {
            if (!document.getElementById('wxs-dot')) { return; }
            verificar(false);
            window.setInterval(function () { verificar(false); }, INTERVALO);
        }

        return { start: start, verificar: verificar };
    })();

    // ------------------------------------------------------------------
    // Fleet meter
    // ------------------------------------------------------------------

    var FleetMeter = (function () {
        var MAX_SEGMENTOS = 60;

        function render(counts) {
            var barra = document.getElementById('meter-bar');
            var leitura = document.getElementById('meter-reading');
            if (!barra || !counts) { return; }

            var estados = []
                .concat(preencher('running', counts.running))
                .concat(preencher('stopped', counts.stopped))
                .concat(preencher('disabled', counts.disabled));

            // Frotas grandes: um segmento por emulador viraria um borrão. Acima
            // do teto, os segmentos passam a ser proporcionais.
            if (estados.length > MAX_SEGMENTOS) {
                estados = amostrar(estados, MAX_SEGMENTOS);
            }

            barra.textContent = '';
            estados.forEach(function (estado) {
                var seg = document.createElement('span');
                seg.className = 'meter__seg';
                seg.setAttribute('data-state', estado);
                barra.appendChild(seg);
            });

            barra.setAttribute('aria-label',
                counts.running + ' de ' + counts.total + ' emuladores ativos, ' +
                counts.stopped + ' parados, ' + counts.disabled + ' desabilitados');

            leitura.textContent = '';
            var ativos = document.createElement('b');
            ativos.textContent = String(counts.running);
            leitura.appendChild(ativos);
            leitura.appendChild(document.createTextNode(' / ' + counts.total));
        }

        function preencher(estado, quantidade) {
            var saida = [];
            for (var i = 0; i < quantidade; i++) { saida.push(estado); }
            return saida;
        }

        function amostrar(itens, tamanho) {
            var saida = [];
            var passo = itens.length / tamanho;
            for (var i = 0; i < tamanho; i++) {
                saida.push(itens[Math.floor(i * passo)]);
            }
            return saida;
        }

        return { render: render };
    })();

    function iniciarMeter() {
        var meter = document.getElementById('fleet-meter');
        var saude = document.getElementById('meter-health');
        if (!meter) { return; }

        // Semeia com o que o servidor renderizou, para a barra não aparecer
        // vazia antes do snapshot chegar.
        FleetMeter.render({
            total: Number(meter.getAttribute('data-total')),
            running: Number(meter.getAttribute('data-running')),
            stopped: Number(meter.getAttribute('data-stopped')),
            disabled: Number(meter.getAttribute('data-disabled'))
        });

        window.FleetStream.subscribe('snapshot', function (dados) {
            FleetMeter.render(dados.counts);
        });

        window.FleetStream.subscribe('device', function (dados) {
            FleetMeter.render(dados.counts);
        });

        window.FleetStream.subscribe('status', function (estado) {
            // Reconectando não apaga os números: só avisa que podem estar
            // defasados. O comportamento antigo zerava tudo e escrevia
            // "Offline" na primeira falha de rede.
            if (estado === 'live') {
                saude.textContent = '';
            } else if (estado === 'reconnecting') {
                saude.textContent = 'reconectando…';
            } else {
                saude.textContent = 'sem conexão';
            }
        });
    }

    // ------------------------------------------------------------------
    // Ações globais
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

    function iniciarAcoesGlobais() {
        var iniciarTodos = document.getElementById('start-all');
        var pararTodos = document.getElementById('stop-all');
        var sincronizar = document.getElementById('sync-db');

        if (iniciarTodos) {
            iniciarTodos.addEventListener('click', function () {
                iniciarTodos.disabled = true;
                postar('/start', { devices: ['all'], enable_log: {} })
                    .then(function () { window.Toast.ok('Iniciando todos os emuladores'); })
                    .catch(function () { window.Toast.err('Não foi possível iniciar os emuladores'); })
                    .then(function () { iniciarTodos.disabled = false; });
            });
        }

        if (pararTodos) {
            pararTodos.addEventListener('click', function () {
                pararTodos.disabled = true;
                postar('/stop', { devices: ['all'] })
                    .then(function () { window.Toast.ok('Parando todos os emuladores'); })
                    .catch(function () { window.Toast.err('Não foi possível parar os emuladores'); })
                    .then(function () { pararTodos.disabled = false; });
            });
        }

        if (sincronizar) {
            sincronizar.addEventListener('click', function () {
                if (!window.confirm(
                    'Sincronizar com o W-Access atualiza os dispositivos disponíveis e ' +
                    'suas configurações. Confirme que o serviço dos gerenciadores ' +
                    'virtuais está parado antes de continuar.')) {
                    return;
                }
                sincronizarBanco(sincronizar);
            });
        }
    }

    function sincronizarBanco(botao) {
        botao.disabled = true;
        window.Toast.info('Sincronizando com o W-Access…');

        fetch('/refresh')
            .then(function (resposta) {
                // 409: sync desligado nas configurações — não há o que
                // buscar. É uma recusa esperada, não uma falha genérica;
                // o corpo traz a mensagem do servidor para o operador.
                if (resposta.status === 409) {
                    return resposta.json().then(function (corpo) {
                        var erro = new Error(corpo.error || 'Sincronização com o W-Access está desligada');
                        erro.tratado = true;
                        throw erro;
                    });
                }
                if (!resposta.ok) { throw new Error('HTTP ' + resposta.status); }
                return aguardarConclusao();
            })
            .then(function () {
                window.Toast.ok('Sincronização concluída. Recarregando…');
                window.setTimeout(function () { window.location.reload(); }, 800);
            })
            .catch(function (erro) {
                window.Toast.err(erro && erro.tratado ? erro.message : 'Falha ao sincronizar com o W-Access');
                botao.disabled = false;
            });
    }

    // O refresh roda em background no servidor; /api/refresh-status é o
    // único jeito de saber que terminou.
    function aguardarConclusao() {
        var INTERVALO = 1000;
        var LIMITE = 60;

        return new Promise(function (resolve, reject) {
            var tentativas = 0;

            var timer = window.setInterval(function () {
                tentativas++;

                if (tentativas > LIMITE) {
                    window.clearInterval(timer);
                    reject(new Error('timeout'));
                    return;
                }

                fetch('/api/refresh-status')
                    .then(function (r) { return r.json(); })
                    .then(function (dados) {
                        if (dados.completed) {
                            window.clearInterval(timer);
                            resolve();
                        }
                    })
                    .catch(function () { /* segue tentando */ });
            }, INTERVALO);
        });
    }

    document.addEventListener('DOMContentLoaded', function () {
        iniciarRail();
        marcarNavAtiva();
        iniciarMeter();
        iniciarAcoesGlobais();
        WxsSignal.start();
        window.FleetStream.start();
    });

    window.FleetMeter = FleetMeter;
    window.WxsSignal = WxsSignal;
})();
