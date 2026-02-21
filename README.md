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
| **Logging Monitoring** | Elasticsearch, Fluentd, Kibana |
| **Performance Monitoring** | Node Exporter, Cadvisor, Prometheus, Grafana |
| **Search**            | CDC + Elasticsearch |

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

1. **MinIO: Create bucket and set permissions, event notifications, and lifecycle**  
   ```bash
   docker exec -i minio bash < minio/bucket_settings.sh
   ```
   This will create `users`, `users-processed`, and `vod` buckets, and set the corresponding permissions.
   And create event notification to send `put` events from `users` bucket to `minio-events` topic.
   And create bucket lifecycle rule: `delete-pending` objects are deleted after 7 days.
   
2. **Cassandra: Create chat Keyspace and Table**  
   In the project root, run:
   ```bash
   docker exec -i cassandra cqlsh < chat-persist-service/migrations/001_create_tables.cql
   ```
   Or enter the container and manually paste/run the content of [001_create_tables.cql](chat-persist-service/migrations/001_create_tables.cql) in `cqlsh` to create the `wes_chat` keyspace and `messages_by_room_session` table:
   ```bash
   docker exec -it cassandra cqlsh
   ```

   Start the services that depend on Cassandra:
   ```bash
   docker-compose up -d chat-history-service chat-persist-service
   ```
3. **CDC: Create Kafka Topics and Connectors**
   1. Create Topics in kafka (Source)
   ```bash
   docker exec -it kafka kafka-topics --bootstrap-server kafka:9092 \
      --create --topic connect-configs-pg-kafka \
      --partitions 1 --replication-factor 1 \
      --config cleanup.policy=compact

   docker exec -it kafka kafka-topics --bootstrap-server kafka:9092 \
      --create --topic connect-offsets-pg-kafka \
      --partitions 1 --replication-factor 1 \
      --config cleanup.policy=compact

   docker exec -it kafka kafka-topics --bootstrap-server kafka:9092 \
      --create --topic connect-status-pg-kafka \
      --partitions 1 --replication-factor 1 \
      --config cleanup.policy=compact
   ```
   2. Create CDC connectors (Source)
   ```shell
   # Start connect-pg-kafka container
   docker-compose up -d connect-pg-kafka

   # Create CDC connector (users)
   curl -X POST -H "Content-Type: application/json" \
      -d @connect/pg-kafka/source_users.json http://localhost:9083/connectors

   # Create CDC connector (rooms)
   curl -X POST -H "Content-Type: application/json" \
      -d @connect/pg-kafka/source_rooms.json http://localhost:9083/connectors
   ```
   3. Create Topics in kafka (Sink)
   ```shell
   docker exec -it kafka kafka-topics --bootstrap-server kafka:9092 \
      --create --topic connect-configs-kafka-es \
      --partitions 1 --replication-factor 1 \
      --config cleanup.policy=compact

   docker exec -it kafka kafka-topics --bootstrap-server kafka:9092 \
      --create --topic connect-offsets-kafka-es \
      --partitions 1 --replication-factor 1 \
      --config cleanup.policy=compact

   docker exec -it kafka kafka-topics --bootstrap-server kafka:9092 \
      --create --topic connect-status-kafka-es \
      --partitions 1 --replication-factor 1 \
      --config cleanup.policy=compact

   docker exec -it kafka kafka-topics --bootstrap-server kafka:9092 \
      --create --topic dlq-users \
      --partitions 1 --replication-factor 1 \
      --config cleanup.policy=compact

   docker exec -it kafka kafka-topics --bootstrap-server kafka:9092 \
      --create --topic dlq-rooms \
      --partitions 1 --replication-factor 1 \
      --config cleanup.policy=compact
   ```
   4. Create CDC connectors (Sink)
   ```shell
   # Start connect-kafka-es container
   docker-compose up -d connect-kafka-es

   # Create CDC connector (users)
   curl -X POST -H "Content-Type: application/json" \
      -d @connect/kafka-es/sink_users.json http://localhost:9084/connectors

   # Create CDC connector (rooms)
   curl -X POST -H "Content-Type: application/json" \
      -d @connect/kafka-es/sink_rooms.json http://localhost:9084/connectors
   ```

- **Home:** http://localhost:8080  
- Register / Sign in → Create Room → Start Streaming

---

## 🔍 Monitoring

### Unified Logging Structure [/pkg/log](./pkg/log)
```json
{
  "level":"info",
  "service":"xxx-service",
  "time":"YYYY-MM-DDTHH:MM:SSZ",
  // other custom fields based on specific microservice needs
}
```
Easily parsed by Fluentd and stored in Elasticsearch for monitoring.

### Kibana Dashboard Examples

1. Log Data
   ![Log Data](./assets/06-00-kibana.png)
2. Service Log Count
   ![Service Log Count](./assets/06-01-service-events.png)
3. Service Action Count (Audit Events)
   ![Service Action Count](./assets/06-02-service-actions.png)
4. Dashboard Example
   ![Dashboard Example](./assets/06-03-dashboard.png)

Above are just simple examples, you can adjust them according to your needs.

---

## 📊 Performance Monitoring

### Grafana Dashboard Examples

0. Grafana Dashboard
   ![Grafana Dashboard](./assets/07-00-grafana.png)
1. Node Exporter ([Template ID: 1860](https://grafana.com/grafana/dashboards/1860-node-exporter-full/))

   ![Node Exporter](./assets/07-03-node-exporter.png)

2. Cadvisor ([Template ID: 14282](https://grafana.com/grafana/dashboards/14282-cadvisor-exporter/))

   ![Cadvisor](./assets/07-01-cadvisor.png)

   ![Cadvisor](./assets/07-02-cadvisor.png)

Above are just simple examples, you can adjust them according to your needs.

---

## 📄 License

[MIT License](./LICENSE) · Copyright (c) 2026 Wes (Tcweeei)
