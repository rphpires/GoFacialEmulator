/**
 * device-drawer.js — detalhe de um dispositivo: usuários gravados no
 * emulador e configurações do gerenciador.
 *
 * Porte do modal Bootstrap de device-details.js para o drawer do console.
 * O comportamento é o mesmo; o que muda é que as linhas são construídas
 * como nós com textContent em vez de template strings de HTML — por isso
 * o antigo escapeHtml() não tem equivalente aqui.
 */
(function () {
    'use strict';

    var POR_PAGINA = 10;
    var DEBOUNCE_BUSCA = 300;

    var estado = {
        id: null,
        nome: '',
        pagina: 1,
        total: 0,
        busca: '',
        timerBusca: null
    };

    var focoAnterior = null;
    var ultimoPedido = 0;

    function el(id) { return document.getElementById(id); }

    // ------------------------------------------------------------------
    // Abrir / fechar
    // ------------------------------------------------------------------

    function abrir(id, nome) {
        estado.id = id;
        estado.nome = nome || '';
        estado.pagina = 1;
        estado.busca = '';
        estado.total = 0;

        el('users-search').value = '';
        el('drawer-name').textContent = estado.nome + ' — LC ' + id;

        var device = window.FleetStream.get(id);
        el('drawer-led').setAttribute('data-state', device ? device.status : 'stopped');

        focoAnterior = document.activeElement;

        var drawer = el('device-drawer');
        drawer.hidden = false;
        el('drawer-scrim').hidden = false;
        // Próximo frame: o elemento precisa estar visível para a transição de
        // transform acontecer.
        window.requestAnimationFrame(function () {
            drawer.setAttribute('data-open', 'true');
        });

        selecionarAba('users');

        // As duas abas carregam na abertura, como no modal antigo: trocar de
        // aba fica instantâneo.
        carregarUsuarios();
        carregarConfiguracoes();

        el('drawer-close').focus();
    }

    function fechar() {
        var drawer = el('device-drawer');
        drawer.setAttribute('data-open', 'false');
        el('drawer-scrim').hidden = true;

        window.setTimeout(function () { drawer.hidden = true; }, 200);

        if (focoAnterior && focoAnterior.focus) { focoAnterior.focus(); }
        estado.id = null;
    }

    function selecionarAba(aba) {
        var abas = document.querySelectorAll('.drawer__tab');
        for (var i = 0; i < abas.length; i++) {
            abas[i].setAttribute('aria-selected',
                String(abas[i].getAttribute('data-tab') === aba));
        }

        el('panel-users').hidden = aba !== 'users';
        el('panel-settings').hidden = aba !== 'settings';
    }

    // ------------------------------------------------------------------
    // Mensagens de estado dentro de uma tabela
    // ------------------------------------------------------------------

    function mensagemNaTabela(tbody, colunas, texto, tipo) {
        tbody.textContent = '';

        var tr = document.createElement('tr');
        var td = document.createElement('td');
        td.colSpan = colunas;

        var caixa = document.createElement('div');
        caixa.className = tipo === 'erro' ? 'empty empty--erro' : 'empty';

        var titulo = document.createElement('p');
        titulo.className = 'empty__title';
        titulo.textContent = texto;
        caixa.appendChild(titulo);

        td.appendChild(caixa);
        tr.appendChild(td);
        tbody.appendChild(tr);
    }

    // ------------------------------------------------------------------
    // Usuários
    // ------------------------------------------------------------------

    // silencioso: recarga disparada pelo stream. Mantém a lista atual na
    // tela até a nova chegar, em vez de piscar "Carregando…".
    function carregarUsuarios(silencioso) {
        var tbody = el('users-body');
        if (!silencioso) { mensagemNaTabela(tbody, 5, 'Carregando…'); }

        // Só a resposta mais recente é desenhada: trocar de dispositivo,
        // página ou busca rápido demais não pode deixar a lista de uma
        // consulta antiga na tela.
        var pedido = ++ultimoPedido;

        var params = new URLSearchParams({
            page: String(estado.pagina),
            per_page: String(POR_PAGINA),
            search: estado.busca
        });

        fetch('/api/devices/' + estado.id + '/users?' + params.toString())
            .then(function (resposta) {
                if (!resposta.ok) { throw new Error('HTTP ' + resposta.status); }
                return resposta.json();
            })
            .then(function (dados) {
                if (pedido !== ultimoPedido) { return; }
                estado.total = dados.total || 0;
                renderUsuarios(dados.users || []);
                renderPaginacao();
            })
            .catch(function (erro) {
                if (pedido !== ultimoPedido) { return; }
                mensagemNaTabela(tbody, 5,
                    'Erro ao carregar usuários: ' + erro.message, 'erro');
                el('users-summary').textContent = '';
                el('users-prev').disabled = true;
                el('users-next').disabled = true;
            });
    }

    function renderUsuarios(usuarios) {
        var tbody = el('users-body');

        if (usuarios.length === 0) {
            mensagemNaTabela(tbody, 5, estado.busca
                ? 'Nenhum usuário encontrado para a busca'
                : 'Nenhum usuário gravado neste dispositivo');
            return;
        }

        tbody.textContent = '';

        usuarios.forEach(function (usuario) {
            var tr = document.createElement('tr');

            tr.appendChild(celula(usuario.id, 'mono'));
            tr.appendChild(celula(usuario.name, ''));
            tr.appendChild(celula(usuario.card_no, ''));
            tr.appendChild(celulaFace(usuario.has_face));
            tr.appendChild(celula(formatarValidade(usuario.valid_to), 'num'));

            tbody.appendChild(tr);
        });
    }

    function celula(valor, classe) {
        var td = document.createElement('td');
        if (classe) { td.className = classe; }

        var texto = valor === null || valor === undefined ? '' : String(valor);
        if (texto === '') {
            td.textContent = '—';
            td.style.color = 'var(--text-low)';
        } else {
            td.textContent = texto;
        }

        return td;
    }

    function celulaFace(temFace) {
        var td = document.createElement('td');
        td.className = 'center';

        var badge = document.createElement('span');
        badge.className = 'badge';
        badge.setAttribute('data-state', temFace ? 'running' : 'disabled');
        badge.textContent = temFace ? 'Sim' : 'Não';

        td.appendChild(badge);
        return td;
    }

    // Mesma formatação do modal antigo: DD-MM-AAAA em UTC. Ler em UTC evita
    // a data mudar de dia conforme o fuso de quem abre a tela.
    function formatarValidade(valor) {
        if (!valor) { return ''; }

        var data = new Date(valor);
        if (isNaN(data.getTime())) { return String(valor); }

        var dia = String(data.getUTCDate()).padStart(2, '0');
        var mes = String(data.getUTCMonth() + 1).padStart(2, '0');
        return dia + '-' + mes + '-' + data.getUTCFullYear();
    }

    function renderPaginacao() {
        var primeiro = estado.total === 0 ? 0 : (estado.pagina - 1) * POR_PAGINA + 1;
        var ultimo = Math.min(estado.pagina * POR_PAGINA, estado.total);

        el('users-summary').textContent = estado.total === 0
            ? ''
            : primeiro + '-' + ultimo + ' de ' + estado.total;

        el('users-prev').disabled = estado.pagina <= 1;
        el('users-next').disabled = ultimo >= estado.total;
    }

    // ------------------------------------------------------------------
    // Configurações
    // ------------------------------------------------------------------

    function carregarConfiguracoes() {
        var tbody = el('settings-body');
        mensagemNaTabela(tbody, 2, 'Carregando…');

        pintarLog(window.FleetStream.get(estado.id));

        fetch('/api/devices/' + estado.id + '/settings')
            .then(function (resposta) {
                if (!resposta.ok) { throw new Error('HTTP ' + resposta.status); }
                return resposta.json();
            })
            .then(function (dados) {
                renderConfiguracoes(dados.settings || []);
            })
            .catch(function (erro) {
                mensagemNaTabela(tbody, 2,
                    'Erro ao carregar configurações: ' + erro.message, 'erro');
            });
    }

    function renderConfiguracoes(settings) {
        var tbody = el('settings-body');

        if (settings.length === 0) {
            mensagemNaTabela(tbody, 2, 'Nenhuma configuração gravada');
            return;
        }

        tbody.textContent = '';

        settings.forEach(function (item) {
            var tr = document.createElement('tr');
            tr.appendChild(celula(item.cfg_id, ''));
            tr.appendChild(celula(item.value, 'mono'));
            tbody.appendChild(tr);
        });
    }

    // O emulador precisa estar parado para trocar a flag de log — mesma
    // regra da coluna Log da tabela.
    function pintarLog(device) {
        var log = el('drawer-log');
        log.checked = !!device && device.log_enabled === 1;
        log.disabled = !device || device.status !== 'stopped';
        el('drawer-log-help').textContent = log.disabled
            ? 'Pare o emulador para trocar.'
            : 'Grava na hora.';
    }

    // Grava na hora, como a tabela. A confirmação de volta chega pelo
    // stream e repinta a tabela e o drawer.
    function salvarLog() {
        var log = el('drawer-log');
        var ligado = log.checked;
        log.disabled = true;

        fetch('/api/devices/' + estado.id + '/settings', {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ log_enabled: ligado })
        })
            .then(function (resposta) {
                if (!resposta.ok) { throw new Error('HTTP ' + resposta.status); }
                window.Toast.ok('Log ' + (ligado ? 'ligado' : 'desligado'));
            })
            .catch(function () {
                log.checked = !ligado;
                window.Toast.err('Não foi possível salvar a configuração de log');
            })
            .then(function () { pintarLog(window.FleetStream.get(estado.id)); });
    }

    // ------------------------------------------------------------------
    // Ligação
    // ------------------------------------------------------------------

    document.addEventListener('DOMContentLoaded', function () {
        if (!el('device-drawer')) { return; }

        el('drawer-close').addEventListener('click', fechar);
        el('drawer-scrim').addEventListener('click', fechar);
        el('drawer-log').addEventListener('change', salvarLog);

        var abas = document.querySelectorAll('.drawer__tab');
        for (var i = 0; i < abas.length; i++) {
            abas[i].addEventListener('click', function (evento) {
                selecionarAba(evento.currentTarget.getAttribute('data-tab'));
            });
        }

        // Debounce de 300ms: digitar não deve disparar uma consulta por tecla.
        el('users-search').addEventListener('input', function (evento) {
            var valor = evento.target.value;
            window.clearTimeout(estado.timerBusca);
            estado.timerBusca = window.setTimeout(function () {
                estado.busca = valor.trim();
                estado.pagina = 1;
                carregarUsuarios();
            }, DEBOUNCE_BUSCA);
        });

        el('users-prev').addEventListener('click', function () {
            if (estado.pagina > 1) {
                estado.pagina -= 1;
                carregarUsuarios();
            }
        });

        el('users-next').addEventListener('click', function () {
            if (estado.pagina * POR_PAGINA < estado.total) {
                estado.pagina += 1;
                carregarUsuarios();
            }
        });

        document.addEventListener('keydown', function (evento) {
            if (evento.key === 'Escape' && estado.id !== null) { fechar(); }
        });

        // O drawer acompanha o dispositivo aberto: LED, log, nome e — quando
        // o gerenciador grava ou apaga usuários — a lista de usuários.
        window.FleetStream.subscribe('delta', function (dados) {
            if (estado.id === null) { return; }
            var id = Number(estado.id);

            if (dados.removed.indexOf(id) !== -1) {
                window.Toast.info('O dispositivo ' + id + ' foi removido');
                fechar();
                return;
            }

            var device = null;
            dados.devices.forEach(function (d) { if (d.id === id) { device = d; } });
            if (!device) { return; }

            var antes = dados.previous[id];
            el('drawer-led').setAttribute('data-state', device.status);
            el('drawer-name').textContent = device.name + ' — LC ' + id;
            pintarLog(device);

            if (antes && antes.total_users !== device.total_users) {
                carregarUsuarios(true);
            }
        });
    });

    window.DeviceDrawer = { open: abrir, close: fechar };
})();
