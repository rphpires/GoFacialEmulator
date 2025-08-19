/**
 * Ultra-Clean Header JavaScript - Invenzi Emulators
 * Seguindo tendências UX 2025: Micro-interações, performance, acessibilidade
 */

// ========================================
// Configurações do Header
// ========================================

const HeaderConfig = {
    scrollThreshold: 50,
    updateInterval: 15000, // 15 segundos
    animationDuration: 200
};

// ========================================
// Inicialização do Header
// ========================================

/**
 * Inicializa o sistema do header quando DOM estiver pronto
 */
function initializeHeader() {
    setupScrollEffects();
    initializeStatusSystem();
    setupKeyboardNavigation();
    setupLogoFallback();
}

/**
 * Configura efeitos de scroll sutis
 */
function setupScrollEffects() {
    const header = document.querySelector('.clean-header');
    if (!header) return;

    let isScrolled = false;

    const handleScroll = throttle(() => {
        const shouldAddClass = window.scrollY > HeaderConfig.scrollThreshold;

        if (shouldAddClass !== isScrolled) {
            isScrolled = shouldAddClass;
            header.classList.toggle('scrolled', isScrolled);
        }
    }, 16); // 60fps

    window.addEventListener('scroll', handleScroll, { passive: true });
}

/**
 * Inicializa sistema de status com SSE
 */
function initializeStatusSystem() {
    // Tentar conectar via SSE
    if (typeof EventSource !== 'undefined') {
        connectToSSE();
    }

    // Fallback para polling
    fetchSystemStatus();
    setInterval(fetchSystemStatus, HeaderConfig.updateInterval);
}

/**
 * Conecta ao Server-Sent Events para updates em tempo real
 */
function connectToSSE() {
    const eventSource = new EventSource('/events');

    eventSource.onmessage = function (event) {
        try {
            const data = JSON.parse(event.data);
            updateSystemStatus(data);
        } catch (e) {
            console.warn('Erro ao processar evento SSE:', e);
        }
    };

    eventSource.onerror = function (event) {
        console.warn('Conexão SSE perdida, usando fallback');
        updateSystemStatus({ status: 'offline' });
    };

    // Cleanup ao sair da página
    window.addEventListener('beforeunload', () => {
        eventSource.close();
    });
}

// ========================================
// Gerenciamento de Status
// ========================================

/**
 * Atualiza o status do sistema com micro-animações
 * @param {Object} data - Dados do status
 */
function updateSystemStatus(data) {
    const statusText = document.getElementById('statusText');
    const statusDot = document.getElementById('statusDot');

    if (!statusText || !statusDot) return;

    // Determinar novo status
    let newText, newColor, shouldPulse = false;

    if (data.running_count > 0) {
        newText = window.innerWidth <= 576 ? '' : `${data.running_count} Ativos`;
        newColor = '#10b981'; // Verde
        shouldPulse = true;
    } else if (data.total_count > 0) {
        newText = window.innerWidth <= 576 ? '' : 'Inativo';
        newColor = '#f59e0b'; // Âmbar
    } else {
        newText = window.innerWidth <= 576 ? '' : 'Offline';
        newColor = '#ef4444'; // Vermelho
    }

    // Atualizar com animação suave
    animateStatusChange(statusText, statusDot, newText, newColor, shouldPulse);
}

/**
 * Anima mudanças de status de forma suave
 */
function animateStatusChange(textEl, dotEl, newText, newColor, shouldPulse) {
    // Fade out
    textEl.style.opacity = '0.5';
    dotEl.style.opacity = '0.5';

    setTimeout(() => {
        // Atualizar valores
        textEl.textContent = newText;
        dotEl.style.background = newColor;

        // Gerenciar pulsação
        dotEl.classList.toggle('pulsing', shouldPulse);

        // Fade in
        textEl.style.opacity = '1';
        dotEl.style.opacity = '1';
    }, HeaderConfig.animationDuration / 2);
}

/**
 * Busca status via API como fallback
 */
async function fetchSystemStatus() {
    try {
        const response = await fetch('/api/status');
        if (!response.ok) throw new Error('Network response was not ok');

        const data = await response.json();
        updateSystemStatus({
            running_count: data.running_devices || 0,
            total_count: data.total_devices || 0
        });
    } catch (error) {
        console.warn('Erro ao buscar status:', error);
        updateSystemStatus({ status: 'offline' });
    }
}

// ========================================
// Logo e Fallback
// ========================================

/**
 * Configura fallback do logo
 */
function setupLogoFallback() {
    const logoImg = document.querySelector('.company-logo');
    const logoFallback = document.getElementById('logoFallback');

    if (!logoImg || !logoFallback) return;

    logoImg.addEventListener('load', () => {
        console.log('Logo carregado com sucesso');
    });

    logoImg.addEventListener('error', () => {
        console.log('Erro ao carregar logo - usando fallback');
        logoImg.style.display = 'none';
        logoFallback.style.display = 'flex';
    });
}

// ========================================
// Acessibilidade e Navegação
// ========================================

/**
 * Configura navegação por teclado
 */
function setupKeyboardNavigation() {
    const focusableElements = document.querySelectorAll(
        '.navbar-brand, .settings-btn, .logo-container'
    );

    focusableElements.forEach(element => {
        // Adicionar indicadores visuais de foco
        element.addEventListener('focus', handleFocus);
        element.addEventListener('blur', handleBlur);
    });
}

/**
 * Manipula eventos de foco
 */
function handleFocus(event) {
    event.currentTarget.style.outline = '2px solid #ff8c00';
    event.currentTarget.style.outlineOffset = '2px';
}

/**
 * Manipula eventos de perda de foco
 */
function handleBlur(event) {
    event.currentTarget.style.outline = '';
    event.currentTarget.style.outlineOffset = '';
}

// ========================================
// Responsive Behavior
// ========================================

/**
 * Gerencia comportamento responsivo
 */
function setupResponsiveBehavior() {
    const handleResize = debounce(() => {
        // Revalidar status text baseado no tamanho da tela
        const statusText = document.getElementById('statusText');
        if (statusText && window.innerWidth <= 576) {
            statusText.textContent = '';
        }
    }, 250);

    window.addEventListener('resize', handleResize);
}

// ========================================
// Utilitários de Performance
// ========================================

/**
 * Throttle function para scroll
 */
function throttle(func, limit) {
    let inThrottle;
    return function () {
        const args = arguments;
        const context = this;
        if (!inThrottle) {
            func.apply(context, args);
            inThrottle = true;
            setTimeout(() => inThrottle = false, limit);
        }
    };
}

/**
 * Debounce function para resize
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

// ========================================
// Inicialização e Cleanup
// ========================================

/**
 * Inicializa quando DOM estiver pronto
 */
document.addEventListener('DOMContentLoaded', function () {
    initializeHeader();
    setupResponsiveBehavior();
});

/**
 * Cleanup ao sair da página
 */
window.addEventListener('beforeunload', function () {
    // Cleanup de event listeners se necessário
});

// ========================================
// API pública do Header
// ========================================

window.HeaderManager = {
    updateStatus: updateSystemStatus,
    fetchStatus: fetchSystemStatus,
    config: HeaderConfig
};