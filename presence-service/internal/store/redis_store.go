package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/weiawesome/wes-io-live/presence-service/internal/domain"
)

// RedisConfig holds Redis connection configuration.
type RedisConfig struct {
	Address       string
	Password      string
	DB            int
	PubSubChannel string // channel for room count updates (e.g. presence:room_updates)
	InstanceID    string // optional, for origin_instance_id in pub payload
}

// redisStore implements PresenceStore using Redis.
type redisStore struct {
	client        *redis.Client
	pubSubChannel string
	instanceID    string
}

// NewRedisStore creates a new Redis-backed presence store.
func NewRedisStore(cfg RedisConfig) (PresenceStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Address,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	channel := cfg.PubSubChannel
	if channel == "" {
		channel = "presence:room_updates"
	}

	return &redisStore{
		client:        client,
		pubSubChannel: channel,
		instanceID:    cfg.InstanceID,
	}, nil
}

// Redis key patterns:
// presence:room:{room_id}:users         SET<user_id>      - authenticated users in room
// presence:room:{room_id}:devices       SET<device_hash>  - anonymous devices in room
// presence:user:{user_id}               STRING<room_id>   - user -> room mapping
// presence:device:{device_hash}         STRING<room_id>   - device -> room mapping
// presence:live_rooms                   SET<room_id>      - rooms currently live
// presence:room:{room_id}:live_status   HASH              - live status for a room
//   - is_live: "true" | "false"
//   - broadcaster_id: user_id
//   - started_at: unix timestamp

func roomUsersKey(roomID string) string {
	return fmt.Sprintf("presence:room:%s:users", roomID)
}

func roomDevicesKey(roomID string) string {
	return fmt.Sprintf("presence:room:%s:devices", roomID)
}

func userRoomKey(userID string) string {
	return fmt.Sprintf("presence:user:%s", userID)
}

func deviceRoomKey(deviceHash string) string {
	return fmt.Sprintf("presence:device:%s", deviceHash)
}

const liveRoomsKey = "presence:live_rooms"

func roomLiveStatusKey(roomID string) string {
	return fmt.Sprintf("presence:room:%s:live_status", roomID)
}

func (s *redisStore) AddUser(ctx context.Context, roomID, userID string, ttl time.Duration) error {
	pipe := s.client.TxPipeline()
	pipe.SAdd(ctx, roomUsersKey(roomID), userID)
	pipe.Set(ctx, userRoomKey(userID), roomID, ttl)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *redisStore) RemoveUser(ctx context.Context, roomID, userID string) error {
	pipe := s.client.TxPipeline()
	pipe.SRem(ctx, roomUsersKey(roomID), userID)
	pipe.Del(ctx, userRoomKey(userID))
	_, err := pipe.Exec(ctx)
	return err
}

func (s *redisStore) AddDevice(ctx context.Context, roomID, deviceHash string, ttl time.Duration) error {
	pipe := s.client.TxPipeline()
	pipe.SAdd(ctx, roomDevicesKey(roomID), deviceHash)
	pipe.Set(ctx, deviceRoomKey(deviceHash), roomID, ttl)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *redisStore) RemoveDevice(ctx context.Context, roomID, deviceHash string) error {
	pipe := s.client.TxPipeline()
	pipe.SRem(ctx, roomDevicesKey(roomID), deviceHash)
	pipe.Del(ctx, deviceRoomKey(deviceHash))
	_, err := pipe.Exec(ctx)
	return err
}

func (s *redisStore) GetCount(ctx context.Context, roomID string) (domain.PresenceCount, error) {
	pipe := s.client.TxPipeline()
	usersCmd := pipe.SCard(ctx, roomUsersKey(roomID))
	devicesCmd := pipe.SCard(ctx, roomDevicesKey(roomID))

	_, err := pipe.Exec(ctx)
	if err != nil {
		return domain.PresenceCount{}, err
	}

	authCount := int(usersCmd.Val())
	anonCount := int(devicesCmd.Val())

	return domain.PresenceCount{
		Authenticated: authCount,
		Anonymous:     anonCount,
		Total:         authCount + anonCount,
	}, nil
}

func (s *redisStore) SetUserRoom(ctx context.Context, userID, roomID string, ttl time.Duration) error {
	return s.client.Set(ctx, userRoomKey(userID), roomID, ttl).Err()
}

func (s *redisStore) GetUserRoom(ctx context.Context, userID string) (string, error) {
	val, err := s.client.Get(ctx, userRoomKey(userID)).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

func (s *redisStore) DeleteUserRoom(ctx context.Context, userID string) error {
	return s.client.Del(ctx, userRoomKey(userID)).Err()
}

func (s *redisStore) SetDeviceRoom(ctx context.Context, deviceHash, roomID string, ttl time.Duration) error {
	return s.client.Set(ctx, deviceRoomKey(deviceHash), roomID, ttl).Err()
}

func (s *redisStore) GetDeviceRoom(ctx context.Context, deviceHash string) (string, error) {
	val, err := s.client.Get(ctx, deviceRoomKey(deviceHash)).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

func (s *redisStore) DeleteDeviceRoom(ctx context.Context, deviceHash string) error {
	return s.client.Del(ctx, deviceRoomKey(deviceHash)).Err()
}

func (s *redisStore) RefreshTTL(ctx context.Context, identity *domain.UserIdentity, roomID string, ttl time.Duration) error {
	if identity.IsAuth {
		return s.client.Expire(ctx, userRoomKey(identity.UserID), ttl).Err()
	}
	return s.client.Expire(ctx, deviceRoomKey(identity.DeviceHash), ttl).Err()
}

func (s *redisStore) PublishRoomUpdate(ctx context.Context, roomID string, count domain.PresenceCount) error {
	payload := domain.RoomUpdatePayload{RoomID: roomID, Count: count, OriginInstanceID: s.instanceID}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.client.Publish(ctx, s.pubSubChannel, string(data)).Err()
}

func (s *redisStore) SetRoomLive(ctx context.Context, roomID, broadcasterID string) error {
	pipe := s.client.TxPipeline()

	// Add to live rooms set
	pipe.SAdd(ctx, liveRoomsKey, roomID)

	// Set live status hash
	statusKey := roomLiveStatusKey(roomID)
	pipe.HSet(ctx, statusKey, map[string]interface{}{
		"is_live":        "true",
		"broadcaster_id": broadcasterID,
		"started_at":     strconv.FormatInt(time.Now().Unix(), 10),
	})

	_, err := pipe.Exec(ctx)
	return err
}

func (s *redisStore) SetRoomOffline(ctx context.Context, roomID string) error {
	pipe := s.client.TxPipeline()

	// Remove from live rooms set
	pipe.SRem(ctx, liveRoomsKey, roomID)

	// Delete live status hash
	pipe.Del(ctx, roomLiveStatusKey(roomID))

	_, err := pipe.Exec(ctx)
	return err
}

func (s *redisStore) GetRoomLiveStatus(ctx context.Context, roomID string) (*domain.LiveStatus, error) {
	statusKey := roomLiveStatusKey(roomID)
	result, err := s.client.HGetAll(ctx, statusKey).Result()
	if err != nil {
		return nil, err
	}

	// If no data, room is not live
	if len(result) == 0 {
		return &domain.LiveStatus{IsLive: false}, nil
	}

	status := &domain.LiveStatus{
		IsLive:        result["is_live"] == "true",
		BroadcasterID: result["broadcaster_id"],
	}

	if startedAt, ok := result["started_at"]; ok {
		if ts, err := strconv.ParseInt(startedAt, 10, 64); err == nil {
			status.StartedAt = ts
		}
	}

	return status, nil
}

func (s *redisStore) GetAllLiveRooms(ctx context.Context) ([]string, error) {
	return s.client.SMembers(ctx, liveRoomsKey).Result()
}

func (s *redisStore) PublishLiveStatusUpdate(ctx context.Context, roomID string, isLive bool) error {
	payload := map[string]interface{}{
		"type":               "live_status",
		"room_id":            roomID,
		"is_live":            isLive,
		"origin_instance_id": s.instanceID,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.client.Publish(ctx, s.pubSubChannel, string(data)).Err()
}

func (s *redisStore) Close() error {
	return s.client.Close()
}
