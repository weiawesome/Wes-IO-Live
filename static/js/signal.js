/**
 * Signal Module - WebSocket signaling wrapper
 */
class SignalClient {
    constructor() {
        this.ws = null;
        this.handlers = {};
        this.reconnectAttempts = 0;
        this.maxReconnectAttempts = 5;
        this.reconnectDelay = 1000;
        this.authenticated = false;
        this.pendingMessages = [];
    }

    /**
     * Connect to signal server
     */
    connect() {
        return new Promise((resolve, reject) => {
            if (this.ws && this.ws.readyState === WebSocket.OPEN) {
                resolve();
                return;
            }

            this.ws = new WebSocket(API.SIGNAL_WS);

            this.ws.onopen = () => {
                console.log('Signal: Connected');
                this.reconnectAttempts = 0;
                resolve();
            };

            this.ws.onclose = (event) => {
                console.log('Signal: Disconnected', event.code);
                this.authenticated = false;
                this.emit('disconnected', event);
                this.attemptReconnect();
            };

            this.ws.onerror = (error) => {
                console.error('Signal: Error', error);
                reject(error);
            };

            this.ws.onmessage = (event) => {
                try {
                    const message = JSON.parse(event.data);
                    this.handleMessage(message);
                } catch (e) {
                    console.error('Signal: Failed to parse message', e);
                }
            };
        });
    }

    /**
     * Disconnect from signal server
     */
    disconnect() {
        if (this.ws) {
            this.ws.close();
            this.ws = null;
        }
        this.authenticated = false;
    }

    /**
     * Attempt to reconnect
     */
    attemptReconnect() {
        if (this.reconnectAttempts >= this.maxReconnectAttempts) {
            console.log('Signal: Max reconnect attempts reached');
            this.emit('reconnect_failed');
            return;
        }

        this.reconnectAttempts++;
        const delay = this.reconnectDelay * Math.pow(2, this.reconnectAttempts - 1);
        console.log(`Signal: Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts})`);

        setTimeout(() => {
            this.connect()
                .then(() => {
                    // Re-authenticate after reconnect
                    const token = Auth.getToken();
                    if (token) {
                        this.authenticate(token);
                    }
                })
                .catch(() => {
                    this.attemptReconnect();
                });
        }, delay);
    }

    /**
     * Handle incoming message
     */
    handleMessage(message) {
        console.log('Signal: Received', message.type, message);

        switch (message.type) {
            case 'auth_result':
                if (message.success) {
                    this.authenticated = true;
                    // Send pending messages
                    this.pendingMessages.forEach(msg => this.send(msg));
                    this.pendingMessages = [];
                }
                break;
            case 'pong':
                // Heartbeat response
                break;
        }

        this.emit(message.type, message);
    }

    /**
     * Send a message
     */
    send(message) {
        if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
            console.error('Signal: Not connected');
            return false;
        }

        // Queue non-auth messages if not authenticated
        if (message.type !== 'auth' && message.type !== 'ping' && !this.authenticated) {
            this.pendingMessages.push(message);
            return true;
        }

        console.log('Signal: Sending', message.type, message);
        this.ws.send(JSON.stringify(message));
        return true;
    }

    /**
     * Authenticate with token
     */
    authenticate(token) {
        return this.send({
            type: 'auth',
            token: token
        });
    }

    /**
     * Join a room
     */
    joinRoom(roomId) {
        return this.send({
            type: 'join_room',
            room_id: roomId
        });
    }

    /**
     * Leave a room
     */
    leaveRoom(roomId) {
        return this.send({
            type: 'leave_room',
            room_id: roomId
        });
    }

    /**
     * Start broadcast with SDP offer
     */
    startBroadcast(roomId, offer) {
        return this.send({
            type: 'start_broadcast',
            room_id: roomId,
            offer: offer
        });
    }

    /**
     * Stop broadcast
     */
    stopBroadcast(roomId) {
        return this.send({
            type: 'stop_broadcast',
            room_id: roomId
        });
    }

    /**
     * Send ICE candidate
     */
    sendICECandidate(roomId, candidate) {
        return this.send({
            type: 'ice_candidate',
            room_id: roomId,
            candidate: candidate
        });
    }

    /**
     * Send ping (heartbeat)
     */
    ping() {
        return this.send({ type: 'ping' });
    }

    /**
     * Register event handler
     */
    on(type, handler) {
        if (!this.handlers[type]) {
            this.handlers[type] = [];
        }
        this.handlers[type].push(handler);
    }

    /**
     * Remove event handler
     */
    off(type, handler) {
        if (!this.handlers[type]) return;
        this.handlers[type] = this.handlers[type].filter(h => h !== handler);
    }

    /**
     * Emit event
     */
    emit(type, data) {
        if (!this.handlers[type]) return;
        this.handlers[type].forEach(handler => {
            try {
                handler(data);
            } catch (e) {
                console.error('Signal: Handler error', e);
            }
        });
    }
}

// Global signal client instance
const Signal = new SignalClient();
