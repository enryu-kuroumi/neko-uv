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
                throw new Error(data.message || 'Server error');
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
        const saleTagHTML = cat.is_on_market ? `<span class="sale-tag on-sale">On market</span>` : '';

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
                    <span class="cat-stat-label">Type</span>
                    <span class="cat-stat-value">${UI.escapeHtml(cat.type)}</span>
                </div>
            </div>
            
            <div class="cat-actions" style="justify-content: center; flex-direction: column; gap: 5px;">
                ${saleTagHTML}
                <div style="display: flex; gap: 5px;">
                    <button onclick="updateCat(${cat.id})" class="btn btn-sm btn-primary">Edit</button>
                    <button onclick="deleteCat(${cat.id})" class="btn btn-sm btn-danger">Del</button>
                </div>
            </div>
        `;
        return card;
    }
};

document.addEventListener('DOMContentLoaded', async () => {
    if (Auth.getToken()) {
        try {
            const me = await Api.get('/me');
            Auth.setUser(me);
            UI.initUserPanel();
        } catch (err) {
            console.error("profile err", err);
        }
    } else {
        Toast.info('Non auth');
    }

    const container = document.getElementById('cats-container');
    if (!container) return;

    try {
        container.innerHTML = `
            <div class="empty-state" style="grid-column: 1 / -1;">
                <div class="spinner"></div>
                <h3 class="empty-state-title">Searching for cats...</h3>
            </div>
        `;
        const response = await Api.get('/cats');
        const cats = response.cats;

        container.innerHTML = '';

        if (!cats || cats.length === 0) {
            container.innerHTML = `
                <div class="empty-state" style="grid-column: 1 / -1;">
                    <h3 class="empty-state-title">No cats!</h3>
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
                <h3 class="empty-state-title">Server err</h3>
            </div>
        `;
        Toast.error('Server error: ' + (error.message || 'Unknown error'));
    }
});

async function spawnCat() {
    await Api.post('/cats/spawn', {});
    location.reload();
}

async function deleteCat(id) {
    if (!confirm("Are you sure?")) return;
    await Api.request(`/cats/${id}`, { method: 'DELETE' });
    location.reload();
}

async function updateCat(id) {
    const newName = prompt("Enter new name:");
    if (!newName) return;
    await Api.request(`/cats/${id}`, { 
        method: 'PUT', 
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({ name: newName }) 
    });
    location.reload();
}
