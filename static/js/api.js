/**
 * API Module - HTTP API wrapper
 */
const API = {
    // Service URLs (empty = use relative URLs via nginx proxy)
    // For direct service access without nginx, set these:
    //   USER_SERVICE: 'http://localhost:8082',
    //   ROOM_SERVICE: 'http://localhost:8083',
    //   SIGNAL_WS: 'ws://localhost:8084/ws',
    //   MEDIA_SERVICE: 'http://localhost:8085',
    USER_SERVICE: '',
    ROOM_SERVICE: '',
    SIGNAL_WS: `${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}/ws`,
    MEDIA_SERVICE: '',

    /**
     * Make an authenticated HTTP request
     */
    async request(url, options = {}) {
        const headers = {
            'Content-Type': 'application/json',
            ...Auth.getAuthHeader(),
            ...options.headers
        };

        const response = await fetch(url, {
            ...options,
            headers
        });

        let data;
        try {
            data = await response.json();
        } catch (_) {
            if (!response.ok) {
                throw new Error(response.status === 502 || response.status === 503
                    ? 'Service temporarily unavailable, please try again later'
                    : 'Request failed, please try again later');
            }
            throw new Error('Invalid response format');
        }

        if (!response.ok) {
            const msg = typeof data === 'object' && data !== null
                ? (data.error || data.message || data.msg)
                : null;
            throw new Error(typeof msg === 'string' ? msg : 'Request failed');
        }

        return data;
    },

    // Auth APIs
    auth: {
        /**
         * Register new user
         */
        async register(email, username, password, displayName = '') {
            const response = await API.request(`${API.USER_SERVICE}/api/v1/auth/register`, {
                method: 'POST',
                body: JSON.stringify({ email, username, password, display_name: displayName })
            });
            if (response.data) {
                Auth.save(response.data);
            }
            return response;
        },

        /**
         * Login user
         */
        async login(email, password) {
            const response = await API.request(`${API.USER_SERVICE}/api/v1/auth/login`, {
                method: 'POST',
                body: JSON.stringify({ email, password })
            });
            if (response.data) {
                Auth.save(response.data);
            }
            return response;
        },

        /**
         * Refresh access token
         */
        async refresh() {
            const refreshToken = Auth.getRefreshToken();
            if (!refreshToken) {
                throw new Error('No refresh token');
            }
            const response = await API.request(`${API.USER_SERVICE}/api/v1/auth/refresh`, {
                method: 'POST',
                body: JSON.stringify({ refresh_token: refreshToken })
            });
            if (response.data) {
                Auth.save(response.data);
            }
            return response;
        },

        /**
         * Logout user
         */
        async logout() {
            try {
                await API.request(`${API.USER_SERVICE}/api/v1/auth/logout`, {
                    method: 'POST'
                });
            } finally {
                Auth.clear();
            }
        }
    },

    // Presence APIs (live status + viewer count from presence-service)
    presence: {
        /**
         * Get list of room IDs that are currently live (broadcasting).
         * @returns {Promise<{rooms: string[], total: number}>}
         */
        async getLiveRooms() {
            const response = await fetch(`${API.USER_SERVICE}/api/v1/live-rooms`, {
                headers: { ...Auth.getAuthHeader() }
            });
            if (!response.ok) {
                if (response.status === 404 || response.status >= 500) return { rooms: [], total: 0 };
                const data = await response.json().catch(() => ({}));
                throw new Error(data.message || 'Failed to get live rooms');
            }
            return response.json();
        },

        /**
         * Get viewer count for a room (from presence-service).
         * @param {string} roomId
         * @returns {Promise<{authenticated: number, anonymous: number, total: number}>}
         */
        async getRoomPresence(roomId) {
            const response = await fetch(`${API.USER_SERVICE}/api/v1/rooms/${roomId}/presence`, {
                headers: { ...Auth.getAuthHeader() }
            });
            if (!response.ok) {
                if (response.status === 404) return { total: 0, authenticated: 0, anonymous: 0 };
                const data = await response.json().catch(() => ({}));
                throw new Error(data.message || 'Failed to get presence');
            }
            return response.json();
        }
    },

    // Room APIs
    rooms: {
        /**
         * List all rooms
         */
        async list(page = 1, pageSize = 20, status = '') {
            let url = `${API.ROOM_SERVICE}/api/v1/rooms?page=${page}&page_size=${pageSize}`;
            if (status) {
                url += `&status=${status}`;
            }
            return API.request(url);
        },

        /**
         * Get a room by ID
         */
        async get(roomId) {
            return API.request(`${API.ROOM_SERVICE}/api/v1/rooms/${roomId}`);
        },

        /**
         * Create a new room
         */
        async create(title, description = '', tags = []) {
            return API.request(`${API.ROOM_SERVICE}/api/v1/rooms`, {
                method: 'POST',
                body: JSON.stringify({ title, description, tags })
            });
        },

        /**
         * Get my rooms
         */
        async my() {
            return API.request(`${API.ROOM_SERVICE}/api/v1/rooms/my`);
        },

        /**
         * Close a room
         */
        async close(roomId) {
            return API.request(`${API.ROOM_SERVICE}/api/v1/rooms/${roomId}`, {
                method: 'DELETE'
            });
        },

        /**
         * Search rooms
         */
        async search(query, page = 1, pageSize = 20) {
            return API.request(
                `${API.ROOM_SERVICE}/api/v1/rooms/search?q=${encodeURIComponent(query)}&page=${page}&page_size=${pageSize}`
            );
        }
    },

    // Media helpers
    media: {
        /**
         * Get Live HLS URL
         */
        getLiveUrl(roomId, sessionId) {
            return `/live/${roomId}/${sessionId}/stream.m3u8`;
        },

        /**
         * Get VOD HLS URL
         */
        getVODUrl(roomId, sessionId) {
            return `/vod/${roomId}/${sessionId}/stream.m3u8`;
        },

        /**
         * List VOD sessions for a room
         */
        async listVODs(roomId) {
            const response = await fetch(`/vod/${roomId}`);
            if (!response.ok) {
                if (response.status === 404) return { sessions: [] };
                throw new Error('Failed to list VODs');
            }
            return response.json();
        },

        /**
         * Get latest VOD for a room
         */
        async getLatestVOD(roomId) {
            const response = await fetch(`/vod/${roomId}/latest`);
            if (!response.ok) {
                throw new Error('No VOD found');
            }
            return response.json();
        },

        /**
         * Get ICE servers for WebRTC
         */
        async getICEServers() {
            try {
                const response = await fetch(`${API.MEDIA_SERVICE}/api/ice-servers`);
                const data = await response.json();
                return data.iceServers || [{ urls: 'stun:stun.l.google.com:19302' }];
            } catch (e) {
                console.warn('Failed to get ICE servers, using default:', e);
                return [{ urls: 'stun:stun.l.google.com:19302' }];
            }
        }
    },

    // Chat history APIs
    Chat: {
        /**
         * Get chat history for a room session
         */
        async getHistory(roomId, sessionId, options = {}) {
            const params = new URLSearchParams();
            if (options.cursor) params.append('cursor', options.cursor);
            if (options.limit) params.append('limit', options.limit);
            if (options.direction) params.append('direction', options.direction);

            const query = params.toString();
            const url = `${API.ROOM_SERVICE}/api/v1/rooms/${roomId}/sessions/${sessionId}/messages${query ? `?${query}` : ''}`;

            return API.request(url, {
                method: 'GET'
            });
        }
    }
};
