'use strict';

const CONFIG = {
    API_URL: 'http://localhost:8080/api/v1',
};

const Auth = {
    getToken: () => localStorage.getItem('token'),
    setToken: (token) => localStorage.setItem('token', token),
    getUser: () => JSON.parse(localStorage.getItem('user') || 'null'),
    setUser: (user) => localStorage.setItem('user', JSON.stringify(user)),
    logout: () => {
        localStorage.removeItem('token');
        localStorage.removeItem('user');
        window.location.href = '/login.html';
    }
};

const Api = {
    async request(endpoint, options = {}) {
        const token = Auth.getToken();
        const headers = {
            'Content-Type': 'application/json',
            ...(token && { 'Authorization': `Bearer ${token}` }),
            ...options.headers
        };

        try {
            const response = await fetch(`${CONFIG.API_URL}${endpoint}`, { ...options, headers });
            const data = await response.json();

            if (!response.ok) {
                if (response.status === 401) Auth.logout();
                throw new Error(data.message || 'Ошибка сервера');
            }
            return data;
        } catch (error) {
            console.error(`[API Error] ${endpoint}:`, error);
            throw error;
        }
    },
    get: (endpoint) => Api.request(endpoint, { method: 'GET' }),
    post: (endpoint, body) => Api.request(endpoint, { method: 'POST', body: JSON.stringify(body) })
};


const Toast = {
    show(message, type = 'info') {
        let container = document.querySelector('.toast-container');
        if (!container) {
            container = document.createElement('div');
            container.className = 'toast-container';
            document.body.appendChild(container);
        }

        const toast = document.createElement('div');

        toast.className = `toast ${type}`;
        toast.textContent = message;

        container.appendChild(toast);

        setTimeout(() => {
            toast.style.opacity = '0';
            setTimeout(() => toast.remove(), 300);
        }, 3000);
    },
    success: (msg) => Toast.show(msg, 'success'),
    error: (msg) => Toast.show(msg, 'error'),
    info: (msg) => Toast.show(msg, 'info')
};

const UI = {
    escapeHtml(str) {
        if (!str) return '';
        const div = document.createElement('div');
        div.textContent = str;
        return div.innerHTML;
    },

    initUserPanel() {
        const user = Auth.getUser();
        if (user) {
            const goldEl = document.getElementById('nav-gold');
            if (goldEl) goldEl.textContent = `${user.gold || 0}`;
        }
    },

    createCatCard(cat) {
        const card = document.createElement('article');

        card.className = `cat-card`;

        const rarityClass = (cat.rarity || 'COMMON').toLowerCase();

        const saleTagHTML = cat.is_on_market
            ? `<span class="sale-tag on-sale">On market</span>`
            : '';

        card.innerHTML = `
            <div class="cat-avatar rarity-${rarityClass}">
                <img src="${cat.image_url || '/placeholder.png'}" alt="${UI.escapeHtml(cat.name)}" style="width: 100%; height: 100%; object-fit: cover; border-radius: 50%;">
            </div>
            <h3 class="cat-name">${UI.escapeHtml(cat.name)}</h3>
            <span class="rarity-badge ${rarityClass}">${cat.rarity}</span>
            
            <div class="cat-stats">
                <div class="cat-stat">
                    <span class="cat-stat-label">GPM</span>
                    <span class="cat-stat-value gold">${cat.gpm} 🪙</span>
                </div>
                <div class="cat-stat">
                    <span class="cat-stat-label">Тип</span>
                    <span class="cat-stat-value">${UI.escapeHtml(cat.type)}</span>
                </div>
            </div>
            
            <div class="cat-actions" style="justify-content: center;">
                ${saleTagHTML}
            </div>
        `;
        return card;
    }
};

document.addEventListener('DOMContentLoaded', async () => {
    UI.initUserPanel();

    Auth.setUser({ username: 'Player1', gold: 1250 });
    UI.initUserPanel();

    const container = document.getElementById('cats-container');
    if (!container) return;

    try {
        container.innerHTML = `
            <div class="empty-state" style="grid-column: 1 / -1;">
                <div class="spinner"></div>
                <h3 class="empty-state-title">Ищем котиков...</h3>
            </div>
        `;

        const cats = [
            { id: 1, name: 'Fluffy', rarity: 'COMMON', gpm: 10, type: 'Fluffy', image_url: 'https://png.pngtree.com/png-clipart/20250807/original/pngtree-cute-chibi-cat-illustration-pastel-colors-minimalist-flat-design-png-image_21644944.png', is_on_market: false },
            { id: 2, name: 'Shadow', rarity: 'RARE', gpm: 50, type: 'Sphynx', image_url: 'https://png.pngtree.com/png-clipart/20250807/original/pngtree-cute-chibi-cat-illustration-pastel-colors-minimalist-flat-design-png-image_21644944.png', is_on_market: true },
            { id: 3, name: 'Higl', rarity: 'LEGENDARY', gpm: 250, type: 'Siamese', image_url: 'https://png.pngtree.com/png-clipart/20250807/original/pngtree-cute-chibi-cat-illustration-pastel-colors-minimalist-flat-design-png-image_21644944.png', is_on_market: false }
        ];

        container.innerHTML = '';

        if (!cats || cats.length === 0) {
            container.innerHTML = `
                <div class="empty-state" style="grid-column: 1 / -1;">
                    <h3 class="empty-state-title">You dont have any cats!</h3>
                </div>
            `;
            return;
        }

        cats.forEach(cat => {
            container.appendChild(UI.createCatCard(cat));
        });

    } catch (error) {
        container.innerHTML = `
            <div class="empty-state" style="grid-column: 1 / -1;">
                <div class="empty-state-icon" style="color: var(--color-error);">⚠️</div>
                <h3 class="empty-state-title">Cannot load data from server</h3>
            </div>
        `;
        Toast.error('Server error: ' + error.message || 'Unknown error');
    }
});
