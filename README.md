<div align="center">

# Wes-IO-Live

**A Real-Time Live Streaming Platform with Microservice Architecture** · WebRTC Push · HLS Playback · S3 VOD

[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)](./LICENSE)
[![WebRTC](https://img.shields.io/badge/WebRTC-Push-333333?style=flat-square&logo=webrtc)](https://webrtc.org/)
[![HLS](https://img.shields.io/badge/HLS-Playback-0D47A1?style=flat-square)](https://developer.apple.com/documentation/http_live_streaming)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?style=flat-square&logo=docker&logoColor=white)](https://www.docker.com/)
[![S3](https://img.shields.io/badge/S3-VOD-569A31?style=flat-square&logo=amazons3&logoColor=white)](https://aws.amazon.com/s3/)

---

[📖 中文](README.zh.md) · [📖 English](README.md)

[![GitHub](https://img.shields.io/badge/GitHub-weiawesome%2FWes--IO--Live-181717?style=flat-square&logo=github)](https://github.com/weiawesome/Wes-IO-Live)
[![Star](https://img.shields.io/github/stars/weiawesome/Wes-IO-Live?style=flat-square&logo=github)](https://github.com/weiawesome/Wes-IO-Live)

</div>

---

## ✨ Features

| Capability | Description |
|------------|-------------|
| **WebRTC Push**       | Direct streaming from browser, ultra-low latency, no plugin required |
| **HLS Live Playback** | HLS.js player for viewers, high compatibility |
| **S3 / MinIO VOD**    | HLS automatically uploads to S3 after live ends, supports playback |
| **Microservice Architecture** | Auth / User / Room / Signal / Media / Chat are separated for scalability |
| **Real-time Chat**    | WebSocket chat + Kafka + Cassandra message persistence |
| **STUN/TURN**         | ICE service for NAT traversal/relay, handles complex network scenarios |

---

## 🏗 Architecture Overview

All requests go through Nginx as a single entry point. Each microservice handles authentication, room management, signaling, media, playback, and chat. The stack uses PostgreSQL, Redis, MinIO/S3, Cassandra, and Kafka.

![Architecture Diagram](./assets/00-architecture.png)

> In the diagram: **Nginx** acts as the API Gateway/static and WebSocket proxy; **User / Room / Signal / ICE / Playback / Chat** are business/signaling services; **Media Service** handles WebRTC ingest, FFmpeg-to-HLS conversion, S3 upload; **Auth** provides JWT via gRPC; storage relies on **PostgreSQL**, **Redis**, **MinIO/S3**, and **Cassandra**.

---

## Demo

1. **Register / Sign in**

   ![Register / Sign in](./assets/01-register.gif)

2. **Create Room**

   ![Create room](./assets/02-create-room.gif)

3. **Start Live Stream**

   ![Start live stream](./assets/03-live-stream.gif)

4. **Watch Live (multi-device)**

   ![Watch live](./assets/04-view-live.gif)

5. **Watch VOD after Live Ends**

   ![Watch VOD](./assets/05-view-vod.gif)

---

## Quick Start

```bash
docker-compose up -d
```

**After startup, complete the following manually:**

1. **MinIO: Create VOD bucket**  
   In the MinIO console or CLI, create a bucket named `vod` (for storing HLS files for VOD playback).

2. **Cassandra: Create chat Keyspace and Table**  
   In the project root, run:
   ```bash
   docker exec -i cassandra cqlsh < chat-persist-service/migrations/001_create_tables.cql
   ```
   Or enter the container and manually paste/run the content of [001_create_tables.cql](chat-persist-service/migrations/001_create_tables.cql) in `cqlsh` to create the `wes_chat` keyspace and `messages_by_room_session` table:
   ```bash
   docker exec -it cassandra cqlsh
   ```

- **Home:** http://localhost:8080  
- Register / Sign in → Create Room → Start Streaming

---

## 📄 License

[MIT License](./LICENSE) · Copyright (c) 2026 Wes (Tcweeei)
