/**
 * FleetStream — conexão única com /events.
 *
 * A versão anterior abria um EventSource em header.js e outro no script
 * inline de devices.html: duas conexões por aba, dois listeners no manager
 * e dois caminhos de atualização que discordavam entre si. Aqui existe uma
 * conexão e um barramento de assinaturas.
 *
 * Eventos publicados:
 *   'snapshot' -> { devices: [...], counts: {...} }     estado completo
 *   'device'   -> { device_id, status, device, counts } uma mudança
 *   'status'   -> 'live' | 'reconnecting' | 'down'      saúde do stream
 */
(function () {
    'use strict';

    var BACKOFF_INICIAL = 1000;
    var BACKOFF_MAXIMO = 30000;
    // Só depois desse tempo sem stream é que entra o polling de resgate.
    var LIMIAR_FALLBACK = 30000;
    var INTERVALO_FALLBACK = 10000;

    var fonte = null;
    var backoff = BACKOFF_INICIAL;
    var timerReconexao = null;
    var timerFallback = null;
    var caiuEm = null;
    var assinantes = { snapshot: [], device: [], status: [] };

    var api = {
        state: 'down',
        counts: { total: 0, running: 0, stopped: 0, disabled: 0 },

        subscribe: function (evento, fn) {
            if (!assinantes[evento]) { return function () { }; }
            assinantes[evento].push(fn);
            return function () {
                var i = assinantes[evento].indexOf(fn);
                if (i !== -1) { assinantes[evento].splice(i, 1); }
            };
        },

        start: function () {
            if (fonte) { return; }
            conectar();
        }
    };

    function publicar(evento, dados) {
        assinantes[evento].slice().forEach(function (fn) {
            try {
                fn(dados);
            } catch (erro) {
                console.error('FleetStream: assinante de "' + evento + '" falhou', erro);
            }
        });
    }

    function definirEstado(novo) {
        if (api.state === novo) { return; }
        api.state = novo;
        publicar('status', novo);
    }

    function conectar() {
        fonte = new EventSource('/events');

        fonte.addEventListener('open', function () {
            backoff = BACKOFF_INICIAL;
            caiuEm = null;
            pararFallback();
            definirEstado('live');
        });

        fonte.addEventListener('snapshot', function (evento) {
            var dados = analisar(evento.data);
            if (!dados) { return; }
            api.counts = dados.counts;
            definirEstado('live');
            publicar('snapshot', dados);
        });

        fonte.addEventListener('device', function (evento) {
            var dados = analisar(evento.data);
            if (!dados) { return; }
            api.counts = dados.counts;
            publicar('device', dados);
        });

        fonte.addEventListener('error', function () {
            // O EventSource reconecta sozinho enquanto readyState é CONNECTING.
            // Só tratamos como queda quando ele desiste de vez.
            if (fonte.readyState !== EventSource.CLOSED) {
                definirEstado('reconnecting');
                return;
            }

            fonte.close();
            fonte = null;

            if (caiuEm === null) { caiuEm = Date.now(); }
            definirEstado('reconnecting');
            agendarFallback();

            timerReconexao = window.setTimeout(conectar, backoff);
            backoff = Math.min(backoff * 2, BACKOFF_MAXIMO);
        });
    }

    function analisar(bruto) {
        try {
            return JSON.parse(bruto);
        } catch (erro) {
            console.error('FleetStream: payload inválido', erro, bruto);
            return null;
        }
    }

    /**
     * Rede de segurança: se o stream não voltar em LIMIAR_FALLBACK, passa a
     * consultar /api/status. Note que ele nunca sobrescreve os dados com
     * zeros — o bug antigo pintava "Offline" e apagava as contagens reais na
     * primeira falha de conexão.
     */
    function agendarFallback() {
        if (timerFallback) { return; }

        timerFallback = window.setInterval(function () {
            if (caiuEm === null || Date.now() - caiuEm < LIMIAR_FALLBACK) { return; }

            definirEstado('down');

            fetch('/api/status')
                .then(function (r) { return r.ok ? r.json() : Promise.reject(r.status); })
                .then(function (dados) {
                    api.counts = {
                        total: dados.total_devices,
                        running: dados.running_devices,
                        stopped: dados.stopped_devices,
                        disabled: dados.disabled_devices
                    };
                    publicar('device', { counts: api.counts });
                })
                .catch(function () { /* segue tentando no próximo tick */ });
        }, INTERVALO_FALLBACK);
    }

    function pararFallback() {
        if (timerFallback) {
            window.clearInterval(timerFallback);
            timerFallback = null;
        }
    }

    window.addEventListener('beforeunload', function () {
        if (timerReconexao) { window.clearTimeout(timerReconexao); }
        pararFallback();
        if (fonte) { fonte.close(); }
    });

    window.FleetStream = api;
})();
