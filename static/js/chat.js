/**
 * Chat Module - WebSocket chat client
 */
const Chat = {
    ws: null,
    handlers: {},
    authenticated: false,
    userId: null,
    username: null,
    roomId: null,
    sessionId: null,
    pingInterval: null,
    reconnecting: false,

    WS_PATH: '/chat/ws',

    getState() {
        return {
            connected: this.ws && this.ws.readyState === WebSocket.OPEN,
            authenticated: this.authenticated,
            roomId: this.roomId,
            sessionId: this.sessionId,
            userId: this.userId,
            username: this.username
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
                this.startHeartbeat();
                resolve();
            };

            this.ws.onerror = (err) => {
                reject(err);
            };

            this.ws.onclose = () => {
                this.stopHeartbeat();
                this.authenticated = false;
                this.emit('disconnected', {});
            };

            this.ws.onmessage = (event) => {
                try {
                    const message = JSON.parse(event.data);
                    this.handleMessage(message);
                } catch (e) {
                    console.error('Chat: Failed to parse message', e);
                }
            };
        });
    },

    disconnect() {
        if (this.ws) {
            this.ws.close();
            this.ws = null;
        }
        this.stopHeartbeat();
        this.authenticated = false;
    },

    authenticate(token) {
        return this.send({ type: 'auth', token });
    },

    joinRoom(roomId, sessionId) {
        this.roomId = roomId;
        this.sessionId = sessionId;
        return this.send({ type: 'join_room', room_id: roomId, session_id: sessionId });
    },

    leaveRoom() {
        this.send({ type: 'leave_room' });
        this.roomId = null;
        this.sessionId = null;
    },

    sendMessage(content) {
        if (!content) return false;
        return this.send({ type: 'chat_message', content });
    },

    send(message) {
        if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
            console.warn('Chat: not connected');
            return false;
        }
        this.ws.send(JSON.stringify(message));
        return true;
    },

    handleMessage(message) {
        switch (message.type) {
            case 'auth_result':
                this.authenticated = !!message.success;
                if (message.success) {
                    this.userId = message.user_id;
                    this.username = message.username;
                }
                break;
            case 'room_joined':
                break;
            case 'chat_message':
                break;
            case 'error':
                console.error('Chat error:', message.code, message.message);
                break;
            case 'pong':
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
                console.error('Chat: handler error', e);
            }
        });
    }
};
