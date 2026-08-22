// Modal de detalhes do dispositivo: usuarios gravados no emulador e
// configuracoes (device_settings). Somente leitura.

const deviceDetails = {
  deviceId: null,
  deviceName: '',
  page: 1,
  perPage: 10,
  total: 0,
  search: '',
  searchTimer: null,
  modal: null,
};

function openDeviceDetails(deviceId, deviceName) {
  deviceDetails.deviceId = deviceId;
  deviceDetails.deviceName = deviceName || '';
  deviceDetails.page = 1;
  deviceDetails.search = '';

  const searchInput = document.getElementById('device-users-search');
  if (searchInput) {
    searchInput.value = '';
  }

  document.getElementById('device-details-title').textContent =
    `${deviceDetails.deviceName} — LC ${deviceId}`;

  if (!deviceDetails.modal) {
    deviceDetails.modal = new bootstrap.Modal(document.getElementById('device-details-modal'));
  }
  deviceDetails.modal.show();

  loadDeviceUsers();
  loadDeviceSettings();
}

function loadDeviceUsers() {
  const tbody = document.getElementById('device-users-body');
  const params = new URLSearchParams({
    page: deviceDetails.page,
    per_page: deviceDetails.perPage,
    search: deviceDetails.search,
  });

  tbody.innerHTML = '<tr><td colspan="5" class="text-center text-muted">Carregando...</td></tr>';

  fetch(`/api/devices/${deviceDetails.deviceId}/users?${params.toString()}`)
    .then(response => {
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }
      return response.json();
    })
    .then(data => {
      deviceDetails.total = data.total || 0;
      renderDeviceUsers(data.users || []);
      renderUsersPagination();
    })
    .catch(error => {
      tbody.innerHTML =
        `<tr><td colspan="5" class="text-center text-danger">Erro ao carregar usuários: ${escapeHtml(error.message)}</td></tr>`;
      document.getElementById('device-users-summary').textContent = '';
    });
}

function renderDeviceUsers(users) {
  const tbody = document.getElementById('device-users-body');

  if (users.length === 0) {
    const message = deviceDetails.search
      ? 'Nenhum usuário encontrado para a busca'
      : 'Nenhum usuário gravado neste dispositivo';
    tbody.innerHTML = `<tr><td colspan="5" class="text-center text-muted">${message}</td></tr>`;
    return;
  }

  tbody.innerHTML = users.map(user => {
    const face = user.has_face
      ? '<span class="badge bg-success">Sim</span>'
      : '<span class="badge bg-secondary">Não</span>';
    return `
      <tr>
        <td class="text-center">${escapeHtml(user.id)}</td>
        <td>${escapeHtml(user.name)}</td>
        <td class="text-center">${escapeHtml(user.card_no) || '<span class="text-muted">—</span>'}</td>
        <td class="text-center">${face}</td>
        <td class="text-center">${formatValidity(user.valid_to)}</td>
      </tr>`;
  }).join('');
}

function renderUsersPagination() {
  const { page, perPage, total } = deviceDetails;
  const first = total === 0 ? 0 : (page - 1) * perPage + 1;
  const last = Math.min(page * perPage, total);

  document.getElementById('device-users-summary').textContent =
    total === 0 ? '' : `${first}-${last} de ${total}`;

  document.getElementById('device-users-prev').disabled = page <= 1;
  document.getElementById('device-users-next').disabled = last >= total;
}

function loadDeviceSettings() {
  const tbody = document.getElementById('device-settings-body');
  tbody.innerHTML = '<tr><td colspan="2" class="text-center text-muted">Carregando...</td></tr>';

  fetch(`/api/devices/${deviceDetails.deviceId}/settings`)
    .then(response => {
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }
      return response.json();
    })
    .then(data => {
      const settings = data.settings || [];
      if (settings.length === 0) {
        tbody.innerHTML =
          '<tr><td colspan="2" class="text-center text-muted">Nenhuma configuração gravada</td></tr>';
        return;
      }
      tbody.innerHTML = settings.map(setting => `
        <tr>
          <td>${escapeHtml(setting.cfg_id)}</td>
          <td>${escapeHtml(setting.value) || '<span class="text-muted">—</span>'}</td>
        </tr>`).join('');
    })
    .catch(error => {
      tbody.innerHTML =
        `<tr><td colspan="2" class="text-center text-danger">Erro ao carregar configurações: ${escapeHtml(error.message)}</td></tr>`;
    });
}

function formatValidity(value) {
  if (!value) {
    return '<span class="text-muted">—</span>';
  }
  const date = new Date(value);
  if (isNaN(date.getTime())) {
    return escapeHtml(value);
  }
  const day = String(date.getUTCDate()).padStart(2, '0');
  const month = String(date.getUTCMonth() + 1).padStart(2, '0');
  return `${day}-${month}-${date.getUTCFullYear()}`;
}

function escapeHtml(value) {
  if (value === null || value === undefined) {
    return '';
  }
  const div = document.createElement('div');
  div.textContent = String(value);
  return div.innerHTML;
}

document.addEventListener('DOMContentLoaded', () => {
  // Delegacao: o template renderiza com text/template (sem escaping), entao o
  // nome do dispositivo nunca entra em contexto JS -- vem do DOM ja renderizado.
  document.addEventListener('click', event => {
    const button = event.target.closest('.device-details-btn');
    if (!button) {
      return;
    }
    event.preventDefault();

    const row = button.closest('tr');
    const nameCell = row ? row.querySelector('.device-name-cell') : null;
    openDeviceDetails(button.dataset.deviceId, nameCell ? nameCell.textContent.trim() : '');
  });

  const searchInput = document.getElementById('device-users-search');
  if (searchInput) {
    searchInput.addEventListener('input', event => {
      clearTimeout(deviceDetails.searchTimer);
      const value = event.target.value;
      deviceDetails.searchTimer = setTimeout(() => {
        deviceDetails.search = value.trim();
        deviceDetails.page = 1;
        loadDeviceUsers();
      }, 300);
    });
  }

  const prev = document.getElementById('device-users-prev');
  if (prev) {
    prev.addEventListener('click', () => {
      if (deviceDetails.page > 1) {
        deviceDetails.page -= 1;
        loadDeviceUsers();
      }
    });
  }

  const next = document.getElementById('device-users-next');
  if (next) {
    next.addEventListener('click', () => {
      if (deviceDetails.page * deviceDetails.perPage < deviceDetails.total) {
        deviceDetails.page += 1;
        loadDeviceUsers();
      }
    });
  }
});
