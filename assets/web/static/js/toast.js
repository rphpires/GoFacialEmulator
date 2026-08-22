/**
 * Toast — notificações não bloqueantes.
 *
 * Substitui os alert() espalhados pela aplicação. alert() trava a thread,
 * exige um clique para sumir e não empilha: iniciar oito emuladores dava
 * oito diálogos em fila.
 */
(function () {
    'use strict';

    var DURACAO = { ok: 3000, info: 4000, err: 6000 };

    function container() {
        var el = document.getElementById('toast-region');
        if (!el) {
            el = document.createElement('div');
            el.id = 'toast-region';
            el.className = 'toast-region';
            // polite, não assertive: são confirmações de ação, não emergências.
            el.setAttribute('role', 'status');
            el.setAttribute('aria-live', 'polite');
            document.body.appendChild(el);
        }
        return el;
    }

    function simbolo(tipo) {
        if (tipo === 'err') { return 'alert'; }
        if (tipo === 'ok') { return 'check'; }
        return 'info';
    }

    function mostrar(tipo, mensagem) {
        var toast = document.createElement('div');
        toast.className = 'toast toast--' + tipo;

        var icone = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
        icone.setAttribute('class', 'icon');
        var use = document.createElementNS('http://www.w3.org/2000/svg', 'use');
        use.setAttribute('href', '/static/icons.svg#' + simbolo(tipo));
        icone.appendChild(use);

        var texto = document.createElement('span');
        // textContent, não innerHTML: a mensagem pode carregar texto de erro
        // vindo do servidor.
        texto.textContent = mensagem;

        toast.appendChild(icone);
        toast.appendChild(texto);
        container().appendChild(toast);

        window.setTimeout(function () {
            toast.classList.add('toast--saindo');
            window.setTimeout(function () { toast.remove(); }, 200);
        }, DURACAO[tipo] || 4000);
    }

    window.Toast = {
        ok: function (msg) { mostrar('ok', msg); },
        err: function (msg) { mostrar('err', msg); },
        info: function (msg) { mostrar('info', msg); }
    };
})();
