/**
 * FleetStream — conexão única com /events e o estado da frota no cliente.
 *
 * Guarda a frota inteira (id -> dispositivo) e a mantém em dia com o que o
 * servidor manda. As telas leem daqui e assinam as mudanças; ninguém mais
 * precisa recarregar a página para ver um dispositivo novo, removido ou
 * com outro estado.
 *
 * Eventos publicados:
 *   'snapshot' -> { devices: [...], counts }            frota substituída
 *   'delta'    -> { devices: [...], removed: [ids], counts, previous: {id: antigo} }
 *   'counts'   -> { total, running, stopped, disabled }
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
    var assinantes = { snapshot: [], delta: [], counts: [], status: [] };
    var frota = new Map();

    var api = {
        state: 'down',
        counts: { total: 0, running: 0, stopped: 0, disabled: 0 },
        // Verdadeiro depois do primeiro estado completo (HTML ou snapshot).
        ready: false,

        subscribe: function (evento, fn) {
            if (!assinantes[evento]) { return function () { }; }
            assinantes[evento].push(fn);
            return function () {
                var i = assinantes[evento].indexOf(fn);
                if (i !== -1) { assinantes[evento].splice(i, 1); }
            };
        },

        get: function (id) {
            return frota.get(Number(id)) || null;
        },

        all: function () {
            return Array.from(frota.values());
        },

        /**
         * Semeia o estado com a frota que o servidor embutiu no HTML. O
         * snapshot do stream, quando chegar, substitui.
         */
        seed: function (devices, counts) {
            if (api.ready) { return; }
            substituir(devices || []);
            api.ready = true;
            publicar('snapshot', { devices: api.all(), counts: counts || api.counts });
            // Sem contagens, o medidor fica com o que o servidor semeou nos
            // data-* do header em vez de ser zerado.
            if (counts) {
                api.counts = counts;
                publicar('counts', api.counts);
            }
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

    function substituir(devices) {
        frota = new Map();
        devices.forEach(function (d) { frota.set(d.id, d); });
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
            substituir(dados.devices || []);
            api.counts = dados.counts;
            api.ready = true;
            definirEstado('live');
            publicar('snapshot', { devices: api.all(), counts: api.counts });
            publicar('counts', api.counts);
        });

        fonte.addEventListener('delta', function (evento) {
            var dados = analisar(evento.data);
            if (!dados) { return; }

            // previous deixa o assinante comparar antes/depois (ex.: o
            // drawer só recarrega a lista quando total_users muda).
            var anteriores = {};
            (dados.devices || []).forEach(function (d) {
                anteriores[d.id] = frota.get(d.id) || null;
                frota.set(d.id, d);
            });
            (dados.removed || []).forEach(function (id) {
                anteriores[id] = frota.get(id) || null;
                frota.delete(id);
            });

            api.counts = dados.counts;
            publicar('delta', {
                devices: dados.devices || [],
                removed: dados.removed || [],
                counts: dados.counts,
                previous: anteriores
            });
            publicar('counts', api.counts);
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
     * consultar /api/status. Nunca sobrescreve os dados com zeros — o bug
     * antigo pintava "Offline" e apagava as contagens reais na primeira
     * falha de conexão.
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
                    publicar('counts', api.counts);
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
