/**
 * Auth Module - Token management for localStorage
 */
const Auth = {
    TOKEN_KEY: 'access_token',
    REFRESH_TOKEN_KEY: 'refresh_token',
    USER_KEY: 'user',
    EXPIRES_KEY: 'expires_at',

    /**
     * Save authentication data
     */
    save(authResponse) {
        localStorage.setItem(this.TOKEN_KEY, authResponse.access_token);
        localStorage.setItem(this.REFRESH_TOKEN_KEY, authResponse.refresh_token);
        localStorage.setItem(this.USER_KEY, JSON.stringify(authResponse.user));
        localStorage.setItem(this.EXPIRES_KEY, authResponse.expires_at.toString());
    },

    /**
     * Get access token (raw, may be expired)
     */
    getToken() {
        return localStorage.getItem(this.TOKEN_KEY);
    },

    /**
     * Get a valid token for WebSocket/auth. If expired, tries refresh; returns null if cannot get valid token.
     * Call this before Signal.authenticate() / Chat.authenticate() to avoid using stale token.
     */
    async getValidToken() {
        const token = this.getToken();
        if (!token) return null;
        // 若未過期（或剩餘超過 60 秒），直接回傳
        const expiresAt = localStorage.getItem(this.EXPIRES_KEY);
        if (expiresAt) {
            const expiryMs = parseInt(expiresAt, 10) * 1000;
            if (Date.now() < expiryMs - 60000) return token;
        }
        // 過期或快過期：嘗試 refresh
        const refreshToken = this.getRefreshToken();
        if (!refreshToken) return token; // 沒有 refresh token 時仍回傳舊 token，讓後端回錯
        try {
            const base = (typeof API !== 'undefined' && API.USER_SERVICE) ? API.USER_SERVICE : '';
            const response = await fetch(`${base}/api/v1/auth/refresh`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ refresh_token: refreshToken })
            });
            const data = await response.json();
            if (response.ok && data.data && data.data.access_token) {
                this.save(data.data);
                return data.data.access_token;
            }
        } catch (e) {
            console.warn('Auth refresh failed:', e);
        }
        return token;
    },

    /**
     * Get refresh token
     */
    getRefreshToken() {
        return localStorage.getItem(this.REFRESH_TOKEN_KEY);
    },

    /**
     * Get current user
     */
    getUser() {
        const userJson = localStorage.getItem(this.USER_KEY);
        return userJson ? JSON.parse(userJson) : null;
    },

    /**
     * Check if token is expired
     */
    isExpired() {
        const expiresAt = localStorage.getItem(this.EXPIRES_KEY);
        if (!expiresAt) return true;
        return Date.now() >= parseInt(expiresAt) * 1000;
    },

    /**
     * Check if user is logged in
     */
    isLoggedIn() {
        return !!this.getToken() && !this.isExpired();
    },

    /**
     * Clear all auth data
     */
    clear() {
        localStorage.removeItem(this.TOKEN_KEY);
        localStorage.removeItem(this.REFRESH_TOKEN_KEY);
        localStorage.removeItem(this.USER_KEY);
        localStorage.removeItem(this.EXPIRES_KEY);
    },

    /**
     * Get Authorization header
     */
    getAuthHeader() {
        const token = this.getToken();
        return token ? { 'Authorization': `Bearer ${token}` } : {};
    },

    /**
     * Redirect to login if not authenticated
     */
    requireAuth() {
        if (!this.isLoggedIn()) {
            window.location.href = 'index.html';
            return false;
        }
        return true;
    }
};
