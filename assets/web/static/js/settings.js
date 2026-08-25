/**
 * settings.js — configurações de conexão com o W-Access.
 *
 * As mensagens de resultado usam textContent e não innerHTML: o campo
 * result.error vem do servidor e pode conter a mensagem crua do driver.
 */
(function () {
    'use strict';

    function coletar() {
        return {
            host: document.getElementById('wxs-host').value,
            port: parseInt(document.getElementById('wxs-port').value, 10),
            database: document.getElementById('wxs-database').value,
            username: document.getElementById('wxs-username').value,
            password: document.getElementById('wxs-password').value
        };
    }

    function mostrarResultado(ok, mensagem) {
        var alvo = document.getElementById('wxs-result');
        alvo.hidden = false;
        alvo.className = 'panel__note ' + (ok ? 'panel__note--ok' : 'panel__note--erro');
        alvo.textContent = mensagem;
    }

    function enviar(url, botao, rotuloOcupado, aoSucesso) {
        var rotuloOriginal = botao.textContent;
        botao.disabled = true;
        botao.textContent = rotuloOcupado;

        return fetch(url, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(coletar())
        })
            .then(function (r) { return r.json(); })
            .then(function (resultado) {
                if (resultado.success) {
                    aoSucesso();
                } else {
                    mostrarResultado(false, resultado.error || 'A operação falhou.');
                    window.Toast.err('A operação falhou');
                }
            })
            .catch(function (erro) {
                mostrarResultado(false, String(erro.message || erro));
                window.Toast.err('Não foi possível falar com o serviço');
            })
            .then(function () {
                botao.disabled = false;
                botao.textContent = rotuloOriginal;
            });
    }

    // Salvar ou testar muda o veredito: refaz a aferição ignorando o cache
    // do servidor para o ponto do rail não ficar desatualizado.
    function reavaliarSinal() {
        if (window.WxsSignal) { window.WxsSignal.verificar(true); }
    }

    document.addEventListener('DOMContentLoaded', function () {
        var alternar = document.getElementById('toggle-password');
        var senha = document.getElementById('wxs-password');
        if (!alternar || !senha) { return; }

        alternar.addEventListener('click', function () {
            var visivel = senha.type === 'text';
            senha.type = visivel ? 'password' : 'text';
            alternar.setAttribute('aria-pressed', String(!visivel));
            alternar.setAttribute('aria-label', visivel ? 'Mostrar senha' : 'Ocultar senha');
        });

        document.getElementById('test-connection').addEventListener('click', function () {
            enviar('/api/settings/test-wxs-connection', this, 'Testando…', function () {
                mostrarResultado(true, 'Conexão bem-sucedida.');
                window.Toast.ok('Conexão bem-sucedida');
            }).then(reavaliarSinal);
        });

        document.getElementById('wxs-form').addEventListener('submit', function (evento) {
            evento.preventDefault();
            var botao = this.querySelector('button[type="submit"]');

            enviar('/api/settings/wxs', botao, 'Salvando…', function () {
                mostrarResultado(true, 'Configurações salvas e conexão reiniciada.');
                window.Toast.ok('Configurações salvas');
            }).then(reavaliarSinal);
        });
    });
})();
