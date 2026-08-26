// Formulário de emulador: criação de um, criação em lote e edição.
// Um modal só, com os campos que diferem trocados por modo — a alternativa
// eram três formulários com os mesmos seis campos repetidos.

(function () {
  const modal = document.getElementById('emulator-form-modal');
  if (!modal) return;

  const el = (id) => document.getElementById(id);
  const titulo = el('emulator-form-title');
  const erro = el('emulator-form-error');

  let modo = 'criar'; // 'criar' | 'lote' | 'editar'
  let idEmEdicao = null;

  function mostrarCampos() {
    const lote = modo === 'lote';
    el('field-name').hidden = lote;
    el('field-prefix').hidden = !lote;
    el('field-port').hidden = lote;
    el('field-port-range').hidden = !lote;
    // Editar exige o emulador parado, então "iniciar após criar" não se
    // aplica: o campo sumiria de qualquer jeito no submit.
    el('field-autostart').hidden = modo === 'editar';
  }

  function limparErro() {
    erro.hidden = true;
    erro.textContent = '';
  }

  function mostrarErro(texto) {
    erro.textContent = texto;
    erro.hidden = false;
  }

  // Valores de um formulário em branco. Usados tanto para abrir "Novo
  // emulador"/"Criar em lote" limpos quanto como base da edição, que depois
  // sobrescreve só os campos que o dispositivo de fato tem.
  function resetarCampos() {
    el('emulator-name').value = '';
    el('emulator-prefix').value = '';
    el('emulator-model').value = 'Hikvision';
    el('emulator-ip').value = '127.0.0.1';
    el('emulator-port').value = 4000;
    el('emulator-port-start').value = 4000;
    el('emulator-port-end').value = 4009;
    el('emulator-interval').value = 10;
    el('emulator-enabled').checked = true;
    el('emulator-autostart').checked = false;
  }

  function abrir(dados) {
    limparErro();
    // Sempre limpar primeiro: sem isso, abrir "Novo emulador" depois de
    // editar (ou depois de outra criação) reaproveitava nome/porta/IP que
    // ficaram no DOM da abertura anterior. Os ramos abaixo sobrescrevem
    // só o que o modo realmente usa.
    resetarCampos();

    if (dados && dados.id) {
      modo = 'editar';
      idEmEdicao = dados.id;
      titulo.textContent = `Editar emulador ${dados.id}`;
      el('emulator-name').value = dados.name || '';
      el('emulator-model').value = dados.model || 'Hikvision';
      el('emulator-port').value = dados.port || 4000;
      // Sem estes três, corpoComum() enviaria os valores em branco do reset
      // acima (IP 127.0.0.1, intervalo 10, habilitado marcado) num PUT que
      // é substituição total — reescrevendo silenciosamente o IP, o
      // intervalo e o enabled de verdade do dispositivo.
      el('emulator-ip').value = dados.ip_address || '127.0.0.1';
      el('emulator-interval').value =
        Number.isFinite(dados.event_interval) ? dados.event_interval : 10;
      el('emulator-enabled').checked = !!dados.enabled;
    } else if (dados && dados.lote) {
      modo = 'lote';
      idEmEdicao = null;
      titulo.textContent = 'Criar emuladores em lote';
    } else {
      modo = 'criar';
      idEmEdicao = null;
      titulo.textContent = 'Novo emulador';
    }

    mostrarCampos();
    modal.showModal();
  }

  window.abrirFormularioEmulador = abrir;

  el('new-emulator')?.addEventListener('click', () => abrir(null));
  el('new-emulator-range')?.addEventListener('click', () => abrir({ lote: true }));
  el('emulator-form-cancel').addEventListener('click', () => modal.close());

  function corpoComum() {
    return {
      model: el('emulator-model').value,
      ip_address: el('emulator-ip').value.trim(),
      event_interval: Number(el('emulator-interval').value),
      enabled: el('emulator-enabled').checked,
      auto_start: el('emulator-autostart').checked,
    };
  }

  function requisicao() {
    if (modo === 'lote') {
      return {
        url: '/api/emulators/range',
        method: 'POST',
        body: {
          ...corpoComum(),
          name_prefix: el('emulator-prefix').value.trim(),
          port_start: Number(el('emulator-port-start').value),
          port_end: Number(el('emulator-port-end').value),
        },
      };
    }

    const body = {
      ...corpoComum(),
      name: el('emulator-name').value.trim(),
      port: Number(el('emulator-port').value),
    };

    if (modo === 'editar') {
      delete body.auto_start;
      return { url: `/api/emulators/${idEmEdicao}`, method: 'PUT', body };
    }
    return { url: '/api/emulators', method: 'POST', body };
  }

  el('emulator-form-save').addEventListener('click', async () => {
    limparErro();
    const { url, method, body } = requisicao();
    const salvar = el('emulator-form-save');
    salvar.disabled = true;

    try {
      const resp = await fetch(url, {
        method,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      const corpo = await resp.json();

      if (!resp.ok) {
        // O erro de porta traz a lista crua; repeti-la é o que diz ao
        // operador qual porta trocar.
        let msg = corpo.error || 'Falha ao salvar';
        if (Array.isArray(corpo.conflicts) && corpo.conflicts.length > 0) {
          msg = `Portas já em uso: ${corpo.conflicts.join(', ')}`;
        }
        mostrarErro(msg);
        salvar.disabled = false;
        return;
      }

      modal.close();
      window.location.reload();
    } catch (e) {
      mostrarErro(`Falha de rede: ${e.message}`);
      salvar.disabled = false;
    }
  });
})();
