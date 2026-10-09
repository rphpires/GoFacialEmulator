/**
 * devices.js — tabela de dispositivos.
 *
 * A tabela é uma projeção do estado da frota que o FleetStream mantém.
 * Filtro, ordenação e paginação rodam aqui, sobre a frota que já está no
 * navegador; o servidor só manda mudanças. Antes o HTML vinha pronto do
 * servidor e só o LED/badge de linhas já existentes era atualizado — linha
 * nova, removida, contagem de usuários ou edição pediam F5.
 *
 * Cada linha é criada uma vez e atualizada célula a célula: uma célula só é
 * redesenhada quando os dados que ela mostra mudam. Isso preserva foco,
 * seleção e um <select> aberto quando chega uma atualização.
 */
(function () {
    'use strict';

    window.abrirFormularioEmulador = window.abrirFormularioEmulador || function () {
        window.Toast.err('Formulário de emulador indisponível');
    };

    var ROTULOS = {
        running: 'ativo', stopped: 'parado', disabled: 'desabilitado',
        error: 'erro', starting: 'iniciando'
    };
    // Ordem de "Estado" ao ordenar: o que pede atenção primeiro.
    var PESO_ESTADO = { error: 0, starting: 1, running: 2, stopped: 3, disabled: 4 };
    var POR_PAGINA = [10, 25, 50, 100];
    var JANELA_PAGINAS = 7;

    var estado = {
        busca: '',
        status: '',
        model: '',
        source: '',
        sort: 'id',
        dir: 'asc',
        pagina: 1,
        porPagina: 10,
        selecionados: new Set(),
        // IDs com ação em andamento: o botão fica "pendente" até a próxima
        // atualização do dispositivo chegar pelo stream.
        pendentes: new Set(),
        // IDs recém-criados: a linha pisca quando aparecer.
        destacar: new Set()
    };

    var linhas = new Map(); // id -> <tr>
    var visiveis = [];      // dispositivos da página atual, na ordem
    var filtrados = [];     // todos os que passam no filtro, ordenados

    function el(id) { return document.getElementById(id); }

    // ------------------------------------------------------------------
    // Estado efetivo de um dispositivo
    // ------------------------------------------------------------------

    // "erro" não é um estado gravado: é um dispositivo parado cujo último
    // start falhou. Mostrar isso é o que diz ao operador por que o botão
    // Iniciar "não fez nada".
    function estadoDe(d) {
        if (d.status === 'stopped' && d.last_error) { return 'error'; }
        return d.status;
    }

    // ------------------------------------------------------------------
    // Filtro, ordenação, paginação
    // ------------------------------------------------------------------

    function passaNoFiltro(d) {
        if (estado.status && estadoDe(d) !== estado.status) { return false; }
        if (estado.model && d.model !== estado.model) { return false; }
        if (estado.source && d.source !== estado.source) { return false; }
        if (estado.busca) {
            var q = estado.busca.toLowerCase();
            if (String(d.id).indexOf(q) === -1 &&
                String(d.port).indexOf(q) === -1 &&
                (d.name || '').toLowerCase().indexOf(q) === -1) {
                return false;
            }
        }
        return true;
    }

    function chaveOrdem(d) {
        switch (estado.sort) {
            case 'name': return (d.name || '').toLowerCase();
            case 'model': return d.model || '';
            case 'port': return d.port;
            case 'users': return d.total_users;
            case 'status': return PESO_ESTADO[estadoDe(d)];
            default: return d.id;
        }
    }

    function comparar(a, b) {
        var ka = chaveOrdem(a);
        var kb = chaveOrdem(b);
        var r = ka < kb ? -1 : ka > kb ? 1 : a.id - b.id;
        return estado.dir === 'desc' ? -r : r;
    }

    function recalcular() {
        filtrados = window.FleetStream.all().filter(passaNoFiltro).sort(comparar);

        var paginas = Math.max(1, Math.ceil(filtrados.length / estado.porPagina));
        if (estado.pagina > paginas) { estado.pagina = paginas; }

        var inicio = (estado.pagina - 1) * estado.porPagina;
        visiveis = filtrados.slice(inicio, inicio + estado.porPagina);
    }

    // ------------------------------------------------------------------
    // Construção de nós
    // ------------------------------------------------------------------

    function no(tag, classe, texto) {
        var n = document.createElement(tag);
        if (classe) { n.className = classe; }
        if (texto !== undefined) { n.textContent = texto; }
        return n;
    }

    function icone(nome) {
        var svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
        svg.setAttribute('class', 'icon');
        svg.setAttribute('aria-hidden', 'true');
        var use = document.createElementNS('http://www.w3.org/2000/svg', 'use');
        use.setAttribute('href', '/static/icons.svg#' + nome);
        svg.appendChild(use);
        return svg;
    }

    function botao(classe, iconeNome, titulo, rotuloOculto) {
        var b = no('button', 'btn btn--sm ' + classe);
        b.type = 'button';
        b.title = titulo;
        b.appendChild(icone(iconeNome));
        b.appendChild(no('span', 'visually-hidden', rotuloOculto));
        return b;
    }

    function led(st) {
        var l = no('span', 'led');
        l.setAttribute('data-state', st);
        l.setAttribute('aria-hidden', 'true');
        return l;
    }

    // Cada coluna: sig() resume o que a célula mostra; render() a desenha.
    // A célula só é redesenhada quando sig muda.
    var COLUNAS = [
        {
            classe: 'center',
            sig: function (d) { return String(estado.selecionados.has(d.id)); },
            render: function (td, d) {
                var c = td.firstChild;
                if (!c) {
                    c = no('input', 'check device-check');
                    c.type = 'checkbox';
                    td.appendChild(c);
                }
                c.setAttribute('aria-label', 'Selecionar ' + d.name);
                c.checked = estado.selecionados.has(d.id);
            }
        },
        {
            sig: function (d) { return estadoDe(d) + '|' + d.id; },
            render: function (td, d) {
                var antigo = td.querySelector('.led');
                var anterior = antigo ? antigo.getAttribute('data-state') : null;
                td.textContent = '';
                var span = no('span', 'grid__id');
                var l = led(estadoDe(d));
                span.appendChild(l);
                span.appendChild(document.createTextNode(' ' + d.id));
                td.appendChild(span);
                if (anterior && anterior !== estadoDe(d)) { piscar(l); }
            }
        },
        {
            classe: 'grid__name device-name-cell',
            sig: function (d) { return d.name; },
            render: function (td, d) { td.textContent = d.name; td.title = d.name; }
        },
        {
            sig: function (d) { return d.model; },
            render: function (td, d) { td.textContent = d.model; }
        },
        {
            // Origem manda em quais ações a linha oferece: o que veio do
            // W-Access é sobrescrito pelo próximo sync.
            sig: function (d) { return d.source; },
            render: function (td, d) {
                td.textContent = '';
                td.appendChild(d.source === 'manual'
                    ? no('span', 'badge badge--manual', 'Manual')
                    : no('span', 'badge badge--wxs', 'W-Access'));
            }
        },
        {
            classe: 'num',
            sig: function (d) { return String(d.port); },
            render: function (td, d) { td.textContent = d.port; }
        },
        {
            // LocalAuthentication só é lido pelo emulador Dahua.
            classe: 'center',
            sig: function (d) { return d.model + '|' + d.mode; },
            render: function (td, d) {
                if (d.model !== 'Dahua') {
                    td.textContent = '';
                    var traco = no('span', 'text-low', '—');
                    traco.title = 'Este modelo não usa esta configuração';
                    td.appendChild(traco);
                    return;
                }
                var s = td.querySelector('select');
                if (!s) {
                    td.textContent = '';
                    s = no('select', 'select device-mode');
                    [['online', 'Online'], ['standalone', 'Standalone']].forEach(function (o) {
                        var opt = no('option', '', o[1]);
                        opt.value = o[0];
                        s.appendChild(opt);
                    });
                    td.appendChild(s);
                }
                s.setAttribute('aria-label', 'Modo de ' + d.name);
                s.value = d.mode;
            }
        },
        {
            classe: 'center',
            sig: function (d) { return d.log_enabled + '|' + d.status; },
            render: function (td, d) {
                var c = td.firstChild;
                if (!c) {
                    c = no('input', 'check log-check');
                    c.type = 'checkbox';
                    td.appendChild(c);
                }
                c.checked = d.log_enabled === 1;
                c.disabled = d.status !== 'stopped';
                c.setAttribute('aria-label', 'Gravar log de ' + d.name);
                c.title = c.disabled ? 'Pare o emulador para trocar' : 'Gravar log de eventos';
            }
        },
        {
            classe: 'num',
            sig: function (d) { return String(d.interval); },
            render: function (td, d) { td.textContent = d.interval + 's'; }
        },
        {
            classe: 'num',
            sig: function (d) { return String(d.total_users); },
            render: function (td, d, primeira) {
                td.textContent = d.total_users;
                if (!primeira) { realcar(td); }
            }
        },
        {
            sig: function (d) { return estadoDe(d) + '|' + (d.last_error || ''); },
            render: function (td, d) {
                var st = estadoDe(d);
                td.textContent = '';
                var b = no('span', 'badge');
                b.setAttribute('data-state', st);
                b.appendChild(led(st));
                b.appendChild(document.createTextNode(' ' + (ROTULOS[st] || st)));
                if (d.last_error) { b.title = 'Último start falhou: ' + d.last_error; }
                td.appendChild(b);
            }
        },
        {
            sig: function (d) {
                return [d.status, d.source, estado.pendentes.has(d.id)].join('|');
            },
            render: function (td, d) {
                td.textContent = '';
                var acoes = no('div', 'row-actions');
                var pendente = estado.pendentes.has(d.id);

                var iniciar = botao('btn--go device-start', 'play', 'Iniciar', 'Iniciar ' + d.name);
                iniciar.disabled = d.status !== 'stopped' || pendente;
                var parar = botao('btn--halt device-stop', 'stop', 'Parar', 'Parar ' + d.name);
                parar.disabled = d.status !== 'running' || pendente;
                if (pendente) {
                    (d.status === 'running' ? parar : iniciar).setAttribute('data-pending', 'true');
                }
                acoes.appendChild(iniciar);
                acoes.appendChild(parar);
                acoes.appendChild(botao('device-details-btn', 'users', 'Usuários e configurações', 'Detalhes de ' + d.name));

                if (d.source === 'manual') {
                    var editar = botao('device-edit', 'gear', 'Editar', 'Editar ' + d.name);
                    editar.disabled = d.status !== 'stopped';
                    if (editar.disabled) { editar.title = 'Pare o emulador para editar'; }
                    acoes.appendChild(editar);
                    acoes.appendChild(botao('btn--halt device-remove', 'close', 'Remover', 'Remover ' + d.name));
                }
                td.appendChild(acoes);
            }
        }
    ];

    function piscar(alvo) {
        alvo.classList.remove('led--flash');
        void alvo.offsetWidth; // reinicia a animação em mudanças seguidas
        alvo.classList.add('led--flash');
    }

    function realcar(td) {
        td.classList.remove('cell--changed');
        void td.offsetWidth;
        td.classList.add('cell--changed');
    }

    function criarLinha(d) {
        var tr = no('tr');
        tr.id = 'device-' + d.id;
        tr.setAttribute('data-id', d.id);
        tr._sigs = [];
        COLUNAS.forEach(function (col) {
            tr.appendChild(no('td', col.classe || ''));
        });
        atualizarLinha(tr, d, true);
        return tr;
    }

    function atualizarLinha(tr, d, primeira) {
        tr.setAttribute('data-state', estadoDe(d));
        tr.setAttribute('aria-selected', String(estado.selecionados.has(d.id)));
        COLUNAS.forEach(function (col, i) {
            var sig = col.sig(d);
            if (tr._sigs[i] === sig) { return; }
            tr._sigs[i] = sig;
            col.render(tr.children[i], d, primeira);
        });
    }

    // ------------------------------------------------------------------
    // Render
    // ------------------------------------------------------------------

    function render() {
        recalcular();

        var tbody = el('device-rows');
        var usados = new Set();

        if (visiveis.length === 0) {
            tbody.textContent = '';
            tbody.appendChild(linhaVazia());
        } else {
            var vazia = tbody.querySelector('.grid__empty');
            if (vazia) { vazia.remove(); }

            visiveis.forEach(function (d, i) {
                var tr = linhas.get(d.id);
                if (tr) {
                    atualizarLinha(tr, d, false);
                } else {
                    tr = criarLinha(d);
                    linhas.set(d.id, tr);
                }
                if (estado.destacar.has(d.id)) {
                    estado.destacar.delete(d.id);
                    tr.classList.add('row--new');
                }
                usados.add(tr);
                // Só move o nó quando a posição muda: mover um <tr> com foco
                // dentro tira o foco.
                if (tbody.children[i] !== tr) {
                    tbody.insertBefore(tr, tbody.children[i] || null);
                }
            });

            Array.prototype.slice.call(tbody.children).forEach(function (tr) {
                if (!usados.has(tr)) { tr.remove(); }
            });
        }

        renderPaginacao();
        renderResumo();
        renderSelecao();
        renderOrdenacao();
    }

    function linhaVazia() {
        var tr = no('tr', 'grid__empty');
        var td = no('td');
        td.colSpan = COLUNAS.length;
        var caixa = no('div', 'empty');
        var total = window.FleetStream.all().length;

        if (!window.FleetStream.ready) {
            caixa.appendChild(no('p', 'empty__title', 'Carregando dispositivos…'));
        } else if (total === 0) {
            caixa.appendChild(no('p', 'empty__title', 'Nenhum dispositivo cadastrado'));
            caixa.appendChild(no('p', '', 'Crie um emulador ou sincronize com o W-Access para trazer os dispositivos.'));
        } else {
            caixa.appendChild(no('p', 'empty__title', 'Nenhum dispositivo corresponde ao filtro'));
            var limpar = no('button', 'btn btn--action', 'Limpar filtros');
            limpar.type = 'button';
            limpar.addEventListener('click', limparFiltros);
            caixa.appendChild(limpar);
        }
        td.appendChild(caixa);
        tr.appendChild(td);
        return tr;
    }

    function renderResumo() {
        var total = window.FleetStream.all().length;
        var alvo = el('grid-summary');
        if (filtrados.length === 0) {
            alvo.textContent = total === 0 ? '' : '0 de ' + total;
            return;
        }
        var inicio = (estado.pagina - 1) * estado.porPagina + 1;
        var fim = inicio + visiveis.length - 1;
        alvo.textContent = inicio + '–' + fim + ' de ' + filtrados.length +
            (filtrados.length !== total ? ' (filtrados de ' + total + ')' : '');
    }

    function renderPaginacao() {
        var nav = el('pager-pages');
        var paginas = Math.max(1, Math.ceil(filtrados.length / estado.porPagina));
        nav.textContent = '';

        function link(rotulo, pagina, extra) {
            var b = no('button', 'pager__page');
            b.type = 'button';
            if (typeof rotulo === 'string') {
                b.textContent = rotulo;
            } else {
                b.appendChild(rotulo);
            }
            b.disabled = pagina < 1 || pagina > paginas;
            if (pagina === estado.pagina && !extra) { b.setAttribute('aria-current', 'page'); }
            if (extra) { b.setAttribute('aria-label', extra); }
            b.addEventListener('click', function () { irParaPagina(pagina); });
            nav.appendChild(b);
        }

        link(icone('chevron-left'), estado.pagina - 1, 'Página anterior');

        var inicio = Math.max(1, estado.pagina - Math.floor(JANELA_PAGINAS / 2));
        var fim = Math.min(paginas, inicio + JANELA_PAGINAS - 1);
        inicio = Math.max(1, fim - JANELA_PAGINAS + 1);
        for (var p = inicio; p <= fim; p++) { link(String(p), p); }

        link(icone('chevron-right'), estado.pagina + 1, 'Próxima página');
    }

    function renderOrdenacao() {
        document.querySelectorAll('.grid__sort').forEach(function (b) {
            var th = b.closest('th');
            var ativo = b.getAttribute('data-sort') === estado.sort;
            th.setAttribute('aria-sort', ativo ? (estado.dir === 'asc' ? 'ascending' : 'descending') : 'none');
        });
    }

    function irParaPagina(pagina) {
        estado.pagina = pagina;
        salvarNaURL();
        render();
        el('device-grid').scrollIntoView({ block: 'nearest' });
    }

    // ------------------------------------------------------------------
    // Seleção — persiste entre páginas e filtros
    // ------------------------------------------------------------------

    function renderSelecao() {
        // Remove da seleção o que deixou de existir.
        estado.selecionados.forEach(function (id) {
            if (!window.FleetStream.get(id)) { estado.selecionados.delete(id); }
        });

        var n = estado.selecionados.size;
        el('bulk-bar').hidden = n === 0;
        el('bulk-count').textContent = n + (n === 1 ? ' selecionado' : ' selecionados');

        var naoSelecionadosNoFiltro = filtrados.filter(function (d) {
            return !estado.selecionados.has(d.id);
        }).length;
        var selFiltrados = el('select-filtered');
        selFiltrados.hidden = naoSelecionadosNoFiltro === 0;
        selFiltrados.textContent = 'Selecionar todos os ' + filtrados.length + ' filtrados';

        var naPagina = visiveis.filter(function (d) { return estado.selecionados.has(d.id); }).length;
        var selectAll = el('select-all');
        selectAll.checked = visiveis.length > 0 && naPagina === visiveis.length;
        selectAll.indeterminate = naPagina > 0 && naPagina < visiveis.length;
    }

    function selecionar(ids, marcar) {
        ids.forEach(function (id) {
            if (marcar) { estado.selecionados.add(id); } else { estado.selecionados.delete(id); }
        });
        render();
    }

    function selecionadosArray() {
        return Array.from(estado.selecionados);
    }

    // ------------------------------------------------------------------
    // Filtros e URL
    // ------------------------------------------------------------------

    function lerDaURL() {
        var p = new URLSearchParams(window.location.search);
        estado.busca = p.get('q') || '';
        estado.status = p.get('status') || '';
        estado.model = p.get('model') || '';
        estado.source = p.get('source') || '';
        estado.sort = p.get('sort') || 'id';
        estado.dir = p.get('dir') === 'desc' ? 'desc' : 'asc';
        estado.pagina = Math.max(1, parseInt(p.get('page'), 10) || 1);
        var pp = parseInt(p.get('per_page'), 10);
        estado.porPagina = POR_PAGINA.indexOf(pp) !== -1 ? pp : 10;

        el('filter-q').value = estado.busca;
        el('filter-status').value = estado.status;
        el('filter-model').value = estado.model;
        el('filter-source').value = estado.source;
        el('per-page').value = String(estado.porPagina);
    }

    function salvarNaURL() {
        var p = new URLSearchParams();
        if (estado.busca) { p.set('q', estado.busca); }
        if (estado.status) { p.set('status', estado.status); }
        if (estado.model) { p.set('model', estado.model); }
        if (estado.source) { p.set('source', estado.source); }
        if (estado.sort !== 'id') { p.set('sort', estado.sort); }
        if (estado.dir !== 'asc') { p.set('dir', estado.dir); }
        if (estado.pagina !== 1) { p.set('page', String(estado.pagina)); }
        if (estado.porPagina !== 10) { p.set('per_page', String(estado.porPagina)); }
        var qs = p.toString();
        window.history.replaceState(null, '', window.location.pathname + (qs ? '?' + qs : ''));
    }

    function aplicarFiltros() {
        estado.busca = el('filter-q').value.trim();
        estado.status = el('filter-status').value;
        estado.model = el('filter-model').value;
        estado.source = el('filter-source').value;
        estado.pagina = 1;
        salvarNaURL();
        render();
    }

    function limparFiltros() {
        el('filter-form').reset();
        aplicarFiltros();
    }

    function iniciarFiltros() {
        var form = el('filter-form');
        var timer = null;

        form.addEventListener('submit', function (e) { e.preventDefault(); });
        form.addEventListener('reset', function () {
            // O reset do navegador muda os campos depois deste evento.
            window.setTimeout(aplicarFiltros, 0);
        });
        el('filter-q').addEventListener('input', function () {
            window.clearTimeout(timer);
            timer = window.setTimeout(aplicarFiltros, 120);
        });
        ['filter-status', 'filter-model', 'filter-source'].forEach(function (id) {
            el(id).addEventListener('change', aplicarFiltros);
        });

        el('per-page').addEventListener('change', function () {
            estado.porPagina = Number(el('per-page').value);
            estado.pagina = 1;
            salvarNaURL();
            render();
        });

        document.querySelectorAll('.grid__sort').forEach(function (b) {
            b.addEventListener('click', function () {
                var col = b.getAttribute('data-sort');
                if (estado.sort === col) {
                    estado.dir = estado.dir === 'asc' ? 'desc' : 'asc';
                } else {
                    estado.sort = col;
                    estado.dir = 'asc';
                }
                salvarNaURL();
                render();
            });
        });

        // "/" foca a busca, como em outras ferramentas de console.
        document.addEventListener('keydown', function (e) {
            if (e.key !== '/' || e.ctrlKey || e.metaKey || e.altKey) { return; }
            var alvo = e.target;
            if (alvo.closest('input, textarea, select, dialog[open]')) { return; }
            e.preventDefault();
            el('filter-q').focus();
        });
    }

    // ------------------------------------------------------------------
    // Ações
    // ------------------------------------------------------------------

    function enviar(url, metodo, corpo) {
        return fetch(url, {
            method: metodo,
            headers: { 'Content-Type': 'application/json' },
            body: corpo === undefined ? undefined : JSON.stringify(corpo)
        }).then(function (resposta) {
            return resposta.json().catch(function () { return {}; }).then(function (dados) {
                if (!resposta.ok) {
                    throw new Error(dados.error || 'HTTP ' + resposta.status);
                }
                return dados;
            });
        });
    }

    function marcarPendente(id, sim) {
        if (sim) { estado.pendentes.add(id); } else { estado.pendentes.delete(id); }
        render();
    }

    // Resume as falhas de /start e /stop num toast só.
    function avisarFalhas(dados, verbo) {
        var falhas = (dados && dados.failed) || [];
        if (falhas.length === 0) { return false; }
        var primeira = falhas[0];
        window.Toast.err('Não foi possível ' + verbo + ' ' +
            (falhas.length === 1 ? 'o dispositivo ' + primeira.id : falhas.length + ' dispositivos') +
            ': ' + primeira.error);
        return true;
    }

    function controlar(ids, url, verbo, mensagem) {
        ids.forEach(function (id) { estado.pendentes.add(id); });
        render();

        enviar(url, 'POST', { devices: ids.map(String) })
            .then(function (dados) {
                if (!avisarFalhas(dados, verbo)) { window.Toast.ok(mensagem); }
            })
            .catch(function (erro) {
                window.Toast.err('Não foi possível ' + verbo + ': ' + erro.message);
            })
            .then(function () {
                // Lote em background (202) termina pelo stream; aqui só se
                // libera o botão — o estado final chega na atualização.
                ids.forEach(function (id) { estado.pendentes.delete(id); });
                render();
            });
    }

    // Remove um emulador. Nunca rejeita: quem chama em lote precisa do
    // resultado de cada um para montar o resumo.
    function removerEmulador(id) {
        return enviar('/api/emulators/' + id, 'DELETE')
            .then(function () { return { ok: true, erro: '' }; })
            .catch(function (err) { return { ok: false, erro: err.message }; });
    }

    // Concorrência limitada: a criação em lote permite 1000 emuladores, e
    // um Promise.all abriria centenas de conexões de uma vez.
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
        for (var i = 0; i < Math.min(LIMITE, ids.length); i++) { frentes.push(frente()); }
        return Promise.all(frentes).then(function () { return resultados; });
    }

    function excluirSelecionados(botaoExcluir) {
        var removiveis = [];
        var gerenciados = 0;
        selecionadosArray().forEach(function (id) {
            var d = window.FleetStream.get(id);
            if (d && d.source === 'manual') { removiveis.push(id); } else { gerenciados++; }
        });

        if (removiveis.length === 0) {
            window.Toast.err('Nenhum dos selecionados pode ser removido: todos vieram do W-Access');
            return;
        }

        var aviso = 'Excluir ' + removiveis.length + ' emulador(es)?\n\n' +
            'Os cartões, faces e usuários cadastrados neles também serão apagados. ' +
            'Não há como desfazer.';
        if (gerenciados > 0) {
            aviso += '\n\n' + gerenciados + ' dispositivo(s) da seleção vieram do ' +
                'W-Access e não serão tocados.';
        }
        if (!window.confirm(aviso)) { return; }

        botaoExcluir.disabled = true;
        emLotes(removiveis, removerEmulador).then(function (resultados) {
            var falhas = resultados.filter(function (r) { return !r.ok; });
            var removidos = resultados.length - falhas.length;
            if (removidos > 0) { window.Toast.ok(removidos + ' emulador(es) removido(s)'); }
            if (falhas.length > 0) {
                window.Toast.err(falhas.length + ' não removido(s). ' + falhas[0].erro);
            }
            botaoExcluir.disabled = false;
        });
    }

    function idDaLinha(alvo) {
        var tr = alvo.closest('tr[data-id]');
        return tr ? Number(tr.getAttribute('data-id')) : null;
    }

    function iniciarAcoes() {
        var tbody = el('device-rows');

        // Delegação: as linhas nascem e morrem com o stream, então nenhum
        // listener é ligado a um botão específico.
        tbody.addEventListener('click', function (e) {
            var alvo = e.target.closest('button');
            if (!alvo) { return; }
            var id = idDaLinha(alvo);
            var d = window.FleetStream.get(id);
            if (!d) { return; }

            if (alvo.classList.contains('device-start')) {
                controlar([id], '/start', 'iniciar', 'Emulador ' + d.name + ' iniciado');
            } else if (alvo.classList.contains('device-stop')) {
                controlar([id], '/stop', 'parar', 'Emulador ' + d.name + ' parado');
            } else if (alvo.classList.contains('device-details-btn') && window.DeviceDrawer) {
                window.DeviceDrawer.open(id, d.name);
            } else if (alvo.classList.contains('device-edit')) {
                window.abrirFormularioEmulador({
                    id: d.id, name: d.name, model: d.model, port: d.port,
                    ip_address: d.ip_address, event_interval: d.interval,
                    enabled: d.enabled === 1
                });
            } else if (alvo.classList.contains('device-remove')) {
                if (!window.confirm('Remover o emulador "' + d.name + '"?\n\n' +
                    'Os cartões, faces e usuários cadastrados nele também serão apagados. ' +
                    'Não há como desfazer.')) { return; }
                alvo.disabled = true;
                removerEmulador(id).then(function (r) {
                    if (r.ok) {
                        window.Toast.ok('Emulador ' + d.name + ' removido');
                    } else {
                        window.Toast.err(r.erro);
                        alvo.disabled = false;
                    }
                });
            }
        });

        tbody.addEventListener('change', function (e) {
            var alvo = e.target;
            var id = idDaLinha(alvo);
            if (id === null) { return; }

            if (alvo.classList.contains('device-check')) {
                selecionar([id], alvo.checked);
            } else if (alvo.classList.contains('log-check')) {
                trocarLog(alvo, id);
            } else if (alvo.classList.contains('device-mode')) {
                trocarModo(alvo, id);
            }
        });

        el('select-all').addEventListener('change', function (e) {
            selecionar(visiveis.map(function (d) { return d.id; }), e.target.checked);
        });
        el('select-filtered').addEventListener('click', function () {
            selecionar(filtrados.map(function (d) { return d.id; }), true);
        });
        el('clear-selection').addEventListener('click', function () {
            estado.selecionados.clear();
            render();
        });

        el('start-selected').addEventListener('click', function () {
            var ids = selecionadosArray();
            controlar(ids, '/start', 'iniciar', 'Iniciando ' + ids.length + ' emulador(es)');
        });
        el('stop-selected').addEventListener('click', function () {
            var ids = selecionadosArray();
            controlar(ids, '/stop', 'parar', 'Parando ' + ids.length + ' emulador(es)');
        });
        el('delete-selected').addEventListener('click', function () {
            excluirSelecionados(el('delete-selected'));
        });
    }

    // Log grava na hora. Antes a flag só ia junto no próximo start e o
    // checkbox parecia não fazer nada.
    function trocarLog(alvo, id) {
        var ligado = alvo.checked;
        alvo.disabled = true;
        enviar('/api/devices/' + id + '/settings', 'PUT', { log_enabled: ligado })
            .then(function () {
                window.Toast.ok('Log ' + (ligado ? 'ligado' : 'desligado') + ' no dispositivo ' + id);
            })
            .catch(function (erro) {
                alvo.checked = !ligado;
                window.Toast.err('Não foi possível gravar o log: ' + erro.message);
            })
            .then(function () { alvo.disabled = false; });
    }

    function trocarModo(alvo, id) {
        var d = window.FleetStream.get(id);
        var anterior = d ? d.mode : alvo.value;
        enviar('/api/devices/' + id + '/mode', 'POST', { mode: alvo.value })
            .then(function () { window.Toast.ok('Dispositivo ' + id + ': modo ' + alvo.value); })
            .catch(function () {
                // Volta o select: deixar o valor novo sem ter gravado faria
                // a tela mentir sobre o dispositivo.
                alvo.value = anterior;
                window.Toast.err('Não foi possível trocar o modo do dispositivo ' + id);
            });
    }

    // ------------------------------------------------------------------
    // Aviso de alcançabilidade
    // ------------------------------------------------------------------

    var timerAlcance = null;

    // Reavalia o aviso. Some sozinho quando o problema deixa de existir —
    // antes ele era carregado uma vez e ficava na tela até um F5.
    function carregarAlcancabilidade() {
        var linha = el('reachability-alert-row');
        if (!linha) { return; }

        fetch('/api/reachability')
            .then(function (resposta) {
                if (!resposta.ok) { throw new Error('HTTP ' + resposta.status); }
                return resposta.json();
            })
            .then(function (relatorio) {
                var problematicos = (relatorio.devices || []).filter(function (d) {
                    return d.status === 'inalcancavel' || d.status === 'desconhecido';
                });
                if (problematicos.length === 0) {
                    linha.hidden = true;
                    return;
                }

                el('reachability-headline').textContent =
                    problematicos.length + ' dispositivo(s) não vão ser alcançados pelo Site Controller';
                el('reachability-reason').textContent = problematicos[0].reason || '';
                el('reachability-help').textContent = 'Veja o capítulo Portas e rede do manual.';

                var lista = el('reachability-list');
                lista.textContent = '';
                problematicos.forEach(function (d) {
                    lista.appendChild(no('li', '', 'Dispositivo ' + d.device_id + ' — porta ' + d.port +
                        (d.reason ? ': ' + d.reason : '')));
                });
                linha.hidden = false;
            })
            .catch(function () { /* sem resposta confiável, não mexe no aviso */ });
    }

    function agendarAlcancabilidade() {
        window.clearTimeout(timerAlcance);
        timerAlcance = window.setTimeout(carregarAlcancabilidade, 2000);
    }

    function iniciarAlcancabilidade() {
        var toggle = el('reachability-toggle');
        if (toggle) {
            toggle.addEventListener('click', function () {
                var lista = el('reachability-list');
                lista.hidden = !lista.hidden;
                toggle.setAttribute('aria-expanded', String(!lista.hidden));
                toggle.textContent = lista.hidden ? 'ver a lista' : 'esconder a lista';
            });
        }
        carregarAlcancabilidade();
    }

    // ------------------------------------------------------------------
    // Stream
    // ------------------------------------------------------------------

    var renderAgendado = false;

    // Várias atualizações no mesmo frame viram um render só.
    function agendarRender() {
        if (renderAgendado) { return; }
        renderAgendado = true;
        window.requestAnimationFrame(function () {
            renderAgendado = false;
            render();
        });
    }

    function iniciarStream() {
        window.FleetStream.subscribe('snapshot', function () {
            // Linhas antigas podem ser de dispositivos que não existem mais.
            linhas.forEach(function (tr, id) {
                if (!window.FleetStream.get(id)) { linhas.delete(id); }
            });
            agendarRender();
        });

        window.FleetStream.subscribe('delta', function (dados) {
            var mudouEstado = false;
            dados.devices.forEach(function (d) {
                var antes = dados.previous[d.id];
                if (!antes || antes.status !== d.status) { mudouEstado = true; }
            });
            dados.removed.forEach(function (id) {
                linhas.delete(id);
                estado.selecionados.delete(id);
                mudouEstado = true;
            });
            agendarRender();
            if (mudouEstado) { agendarAlcancabilidade(); }
        });
    }

    // Chamado pelo formulário depois de criar: as linhas novas piscam ao
    // aparecer.
    window.DevicesTable = {
        destacar: function (ids) {
            (ids || []).forEach(function (id) { estado.destacar.add(Number(id)); });
            agendarRender();
        }
    };

    document.addEventListener('DOMContentLoaded', function () {
        if (!el('device-rows')) { return; }

        lerDaURL();
        iniciarFiltros();
        iniciarAcoes();
        iniciarStream();
        iniciarAlcancabilidade();

        var inicial = el('fleet-initial');
        var dados = null;
        try {
            dados = inicial ? JSON.parse(inicial.textContent) : null;
        } catch (e) {
            dados = null;
        }
        if (dados) {
            window.FleetStream.seed(dados, null);
        } else {
            render();
        }
    });
})();
