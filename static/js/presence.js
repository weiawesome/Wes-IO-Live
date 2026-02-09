/**
 * Presence Module - WebSocket presence client for viewer count
 */
const Presence = {
    ws: null,
    handlers: {},
    roomId: null,
    pingInterval: null,
    reconnectAttempts: 0,
    maxReconnectAttempts: 5,
    reconnectDelay: 1000,

    WS_PATH: '/presence',

    getState() {
        return {
            connected: this.ws && this.ws.readyState === WebSocket.OPEN,
            roomId: this.roomId
        };
    },

    connect() {
        return new Promise((resolve, reject) => {
            if (this.ws && this.ws.readyState === WebSocket.OPEN) {
                resolve();
                return;
            }

            const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
            this.ws = new WebSocket(`${protocol}//${location.host}${this.WS_PATH}`);

            this.ws.onopen = () => {
                this.reconnectAttempts = 0;
                this.startHeartbeat();
                resolve();
            };

            this.ws.onerror = (err) => {
                reject(err);
            };

            this.ws.onclose = () => {
                this.stopHeartbeat();
                this.emit('disconnected', {});
                this.attemptReconnect();
            };

            this.ws.onmessage = (event) => {
                try {
                    const message = JSON.parse(event.data);
                    this.handleMessage(message);
                } catch (e) {
                    console.error('Presence: Failed to parse message', e);
                }
            };
        });
    },

    disconnect() {
        this.reconnectAttempts = this.maxReconnectAttempts; // Prevent reconnection
        if (this.ws) {
            this.ws.close();
            this.ws = null;
        }
        this.stopHeartbeat();
    },

    attemptReconnect() {
        if (this.reconnectAttempts >= this.maxReconnectAttempts) {
            console.warn('Presence: Max reconnect attempts reached');
            return;
        }

        this.reconnectAttempts++;
        const delay = this.reconnectDelay * Math.pow(2, this.reconnectAttempts - 1);
        console.log(`Presence: Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts})`);

        setTimeout(async () => {
            try {
                await this.connect();
                // Rejoin room if we were in one
                if (this.roomId && this._lastJoinParams) {
                    this.joinRoom(
                        this.roomId,
                        this._lastJoinParams.token,
                        this._lastJoinParams.deviceHash
                    );
                }
            } catch (e) {
                console.error('Presence: Reconnect failed', e);
            }
        }, delay);
    },

    /**
     * Join a room with authentication
     * @param {string} roomId - Room ID to join
     * @param {string} token - JWT token for authenticated users (optional)
     * @param {string} deviceHash - Device hash for anonymous users (optional)
     */
    joinRoom(roomId, token = null, deviceHash = null) {
        this.roomId = roomId;
        this._lastJoinParams = { token, deviceHash };

        const message = {
            type: 'join',
            room_id: roomId
        };

        if (token) {
            message.token = token;
        } else if (deviceHash) {
            message.device_hash = deviceHash;
        } else {
            // Generate device hash if neither provided
            message.device_hash = this.generateDeviceHash();
        }

        return this.send(message);
    },

    leaveRoom(roomId) {
        const id = roomId || this.roomId;
        if (!id) return;

        this.send({ type: 'leave', room_id: id });
        if (this.roomId === id) {
            this.roomId = null;
            this._lastJoinParams = null;
        }
    },

    send(message) {
        if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
            console.warn('Presence: not connected');
            return false;
        }
        this.ws.send(JSON.stringify(message));
        return true;
    },

    handleMessage(message) {
        switch (message.type) {
            case 'joined':
                // Room joined successfully, count is available
                break;
            case 'count':
                // Viewer count updated
                break;
            case 'pong':
                // Heartbeat response
                break;
            case 'error':
                console.error('Presence error:', message.message);
                break;
            default:
                break;
        }

        this.emit(message.type, message);
    },

    startHeartbeat() {
        if (this.pingInterval) return;
        this.pingInterval = setInterval(() => {
            if (this.ws && this.ws.readyState === WebSocket.OPEN) {
                this.send({ type: 'ping' });
            }
        }, 30000);
    },

    stopHeartbeat() {
        if (this.pingInterval) {
            clearInterval(this.pingInterval);
            this.pingInterval = null;
        }
    },

    on(type, handler) {
        if (!this.handlers[type]) {
            this.handlers[type] = [];
        }
        this.handlers[type].push(handler);
    },

    off(type, handler) {
        if (!this.handlers[type]) return;
        this.handlers[type] = this.handlers[type].filter(h => h !== handler);
    },

    emit(type, data) {
        if (!this.handlers[type]) return;
        this.handlers[type].forEach(handler => {
            try {
                handler(data);
            } catch (e) {
                console.error('Presence: handler error', e);
            }
        });
    },

    /**
     * Generate a device fingerprint hash for anonymous users
     * @returns {string} SHA-256 hash of device fingerprint
     */
    generateDeviceHash() {
        const fp = {
            screen: `${screen.width}x${screen.height}x${screen.colorDepth}`,
            timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
            language: navigator.language,
            platform: navigator.platform
        };

        // Simple hash function (use crypto.subtle for production)
        const str = JSON.stringify(fp);
        let hash = 0;
        for (let i = 0; i < str.length; i++) {
            const char = str.charCodeAt(i);
            hash = ((hash << 5) - hash) + char;
            hash = hash & hash; // Convert to 32bit integer
        }
        return Math.abs(hash).toString(16).padStart(8, '0');
    },

    /**
     * Generate a proper SHA-256 device hash (async)
     * @returns {Promise<string>} SHA-256 hash of device fingerprint
     */
    async generateDeviceHashAsync() {
        const fp = {
            screen: `${screen.width}x${screen.height}x${screen.colorDepth}`,
            timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
            language: navigator.language,
            platform: navigator.platform
        };

        const str = JSON.stringify(fp);
        const encoder = new TextEncoder();
        const data = encoder.encode(str);

        try {
            const hashBuffer = await crypto.subtle.digest('SHA-256', data);
            const hashArray = Array.from(new Uint8Array(hashBuffer));
            return hashArray.map(b => b.toString(16).padStart(2, '0')).join('');
        } catch (e) {
            // Fallback to simple hash
            return this.generateDeviceHash();
        }
    }
};
