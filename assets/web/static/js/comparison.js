/**
 * comparison.js
 *
 * A versão anterior tinha duas definições de refresh_counter — uma que
 * fazia o fetch e outra que só dava alert("Refresh acionado!") — e nenhuma
 * delas informava o resultado nem recarregava a tabela. O botão parecia
 * não fazer nada.
 */
(function () {
    'use strict';

    function iniciarRefresh() {
        var botao = document.getElementById('refresh-comparison');
        if (!botao) { return; }

        botao.addEventListener('click', function () {
            if (!window.confirm(
                'A recontagem exige que o serviço dos gerenciadores virtuais esteja ' +
                'parado. Confirme antes de continuar.')) {
                return;
            }

            botao.disabled = true;
            window.Toast.info('Recontando usuários…');

            fetch('/comparison_refresh')
                .then(function (resposta) {
                    if (!resposta.ok) { throw new Error('HTTP ' + resposta.status); }
                    window.Toast.ok('Recontagem concluída. Recarregando…');
                    window.setTimeout(function () { window.location.reload(); }, 800);
                })
                .catch(function () {
                    window.Toast.err('Não foi possível recontar os usuários');
                    botao.disabled = false;
                });
        });
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

    document.addEventListener('DOMContentLoaded', function () {
        iniciarRefresh();
        iniciarPaginacao();
    });
})();
