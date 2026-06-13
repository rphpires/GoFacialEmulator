/**
 * Main JavaScript - Facial Emulators
 * Scripts gerais da aplicação
 */

// ========================================
// Configurações Globais
// ========================================

const AppConfig = {
    apiBaseUrl: '/api',
    updateInterval: 30000, // 30 segundos
    animationDuration: 300
};

// ========================================
// Inicialização da Aplicação
// ========================================

/**
 * Inicializa a aplicação quando o DOM estiver pronto
 */
document.addEventListener('DOMContentLoaded', function () {
    initializeApp();
    setupEventListeners();
    loadInitialData();
});

/**
 * Inicialização principal da aplicação
 */
function initializeApp() {
    console.log('Facial Emulators - Aplicação iniciada');

    // Inicializar tooltips do Bootstrap
    initializeTooltips();

    // Configurar AJAX global
    setupAjaxDefaults();

    // Adicionar classes de animação
    addFadeInAnimation();
}

// ========================================
// Bootstrap Components
// ========================================

/**
 * Inicializa todos os tooltips do Bootstrap
 */
function initializeTooltips() {
    const tooltipTriggerList = [].slice.call(document.querySelectorAll('[data-bs-toggle="tooltip"]'));
    tooltipTriggerList.map(function (tooltipTriggerEl) {
        return new bootstrap.Tooltip(tooltipTriggerEl);
    });
}

/**
 * Inicializa todos os popovers do Bootstrap
 */
function initializePopovers() {
    const popoverTriggerList = [].slice.call(document.querySelectorAll('[data-bs-toggle="popover"]'));
    popoverTriggerList.map(function (popoverTriggerEl) {
        return new bootstrap.Popover(popoverTriggerEl);
    });
}

// ========================================
// Event Listeners
// ========================================

/**
 * Configura event listeners globais
 */
function setupEventListeners() {
    // Confirmação para ações perigosas
    document.querySelectorAll('[data-confirm]').forEach(element => {
        element.addEventListener('click', handleConfirmAction);
    });

    // Auto-submit para formulários
    document.querySelectorAll('[data-auto-submit]').forEach(element => {
        element.addEventListener('change', handleAutoSubmit);
    });

    // Loading states para botões
    document.querySelectorAll('form').forEach(form => {
        form.addEventListener('submit', handleFormSubmit);
    });
}

/**
 * Manipula ações que precisam de confirmação
 */
function handleConfirmAction(event) {
    const message = event.currentTarget.getAttribute('data-confirm');
    if (!confirm(message)) {
        event.preventDefault();
        return false;
    }
}

/**
 * Manipula auto-submit de formulários
 */
function handleAutoSubmit(event) {
    const form = event.currentTarget.closest('form');
    if (form) {
        form.submit();
    }
}

/**
 * Adiciona loading state aos formulários
 */
function handleFormSubmit(event) {
    const form = event.currentTarget;
    const submitBtn = form.querySelector('button[type="submit"]');

    if (submitBtn) {
        submitBtn.disabled = true;
        submitBtn.innerHTML = '<span class="spinner-border spinner-border-sm" role="status"></span> Carregando...';
    }
}

// ========================================
// AJAX e API
// ========================================

/**
 * Configurações padrão para AJAX
 */
function setupAjaxDefaults() {
    // Configurar headers padrão se usando jQuery
    if (typeof $ !== 'undefined') {
        $.ajaxSetup({
            beforeSend: function (xhr) {
                xhr.setRequestHeader('X-Requested-With', 'XMLHttpRequest');
            }
        });
    }
}

/**
 * Wrapper para fetch com configurações padrão
 */
async function apiRequest(url, options = {}) {
    const defaultOptions = {
        headers: {
            'Content-Type': 'application/json',
            'X-Requested-With': 'XMLHttpRequest'
        }
    };

    const config = { ...defaultOptions, ...options };

    try {
        const response = await fetch(url, config);

        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }

        return await response.json();
    } catch (error) {
        console.error('API Request Error:', error);
        showNotification('Erro na requisição', 'danger');
        throw error;
    }
}

// ========================================
// Notificações e Feedback
// ========================================

/**
 * Exibe notificação para o usuário
 */
function showNotification(message, type = 'info', duration = 3000) {
    const alertHtml = `
    <div class="alert alert-${type} alert-dismissible fade show position-fixed" 
         style="top: 80px; right: 20px; z-index: 9999; min-width: 300px;" 
         role="alert">
      ${message}
      <button type="button" class="btn-close" data-bs-dismiss="alert"></button>
    </div>
  `;

    document.body.insertAdjacentHTML('beforeend', alertHtml);

    // Auto-remove após duração especificada
    if (duration > 0) {
        setTimeout(() => {
            const alert = document.querySelector('.alert:last-of-type');
            if (alert) {
                const bsAlert = new bootstrap.Alert(alert);
                bsAlert.close();
            }
        }, duration);
    }
}

/**
 * Exibe loading spinner
 */
function showLoading(container) {
    const loadingHtml = `
    <div class="d-flex justify-content-center align-items-center p-4" data-loading>
      <div class="spinner-border text-primary" role="status">
        <span class="visually-hidden">Carregando...</span>
      </div>
    </div>
  `;

    if (typeof container === 'string') {
        container = document.querySelector(container);
    }

    if (container) {
        container.innerHTML = loadingHtml;
    }
}

/**
 * Remove loading spinner
 */
function hideLoading(container) {
    if (typeof container === 'string') {
        container = document.querySelector(container);
    }

    if (container) {
        const loading = container.querySelector('[data-loading]');
        if (loading) {
            loading.remove();
        }
    }
}

// ========================================
// Animações e UI
// ========================================

/**
 * Adiciona animação fade-in aos elementos
 */
function addFadeInAnimation() {
    const elements = document.querySelectorAll('main, .card, .table');
    elements.forEach((element, index) => {
        element.style.animationDelay = `${index * 0.1}s`;
        element.classList.add('fade-in');
    });
}

/**
 * Smooth scroll para âncoras
 */
function smoothScrollTo(target) {
    const element = document.querySelector(target);
    if (element) {
        element.scrollIntoView({
            behavior: 'smooth',
            block: 'start'
        });
    }
}

// ========================================
// Utilidades
// ========================================

/**
 * Formata números com separadores
 */
function formatNumber(num) {
    return new Intl.NumberFormat('pt-BR').format(num);
}

/**
 * Formata data para exibição
 */
function formatDate(date) {
    return new Intl.DateTimeFormat('pt-BR', {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit'
    }).format(new Date(date));
}

/**
 * Debounce function para otimizar eventos
 */
function debounce(func, wait) {
    let timeout;
    return function executedFunction(...args) {
        const later = () => {
            clearTimeout(timeout);
            func(...args);
        };
        clearTimeout(timeout);
        timeout = setTimeout(later, wait);
    };
}

/**
 * Carrega dados iniciais se necessário
 */
function loadInitialData() {
    // Implementar carregamento de dados específicos se necessário
    console.log('Dados iniciais carregados');
}

// ========================================
// Exportar para uso global
// ========================================

window.App = {
    config: AppConfig,
    api: apiRequest,
    notify: showNotification,
    showLoading: showLoading,
    hideLoading: hideLoading,
    formatNumber: formatNumber,
    formatDate: formatDate,
    smoothScrollTo: smoothScrollTo,
    debounce: debounce
};