package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/rs/zerolog"
	"github.com/weiawesome/wes-io-live/chat-service/internal/domain"
	"github.com/weiawesome/wes-io-live/chat-service/internal/hub"
	"github.com/weiawesome/wes-io-live/pkg/log"
	pb "github.com/weiawesome/wes-io-live/proto/chat"
	"google.golang.org/grpc"
)

type chatDeliveryServer struct {
	pb.UnimplementedChatDeliveryServiceServer
	hub *hub.Hub
}

func (s *chatDeliveryServer) DeliverMessage(ctx context.Context, req *pb.DeliverMessageRequest) (*pb.DeliverMessageResponse, error) {
	msg := req.GetMessage()
	if msg == nil {
		return &pb.DeliverMessageResponse{
			Success:      false,
			ErrorMessage: "message is required",
		}, nil
	}

	outMsg := &domain.ChatMessageOut{
		Type:      domain.MsgTypeChatMessage,
		MessageID: msg.GetMessageId(),
		UserID:    msg.GetUserId(),
		Username:  msg.GetUsername(),
		RoomID:    msg.GetRoomId(),
		SessionID: msg.GetSessionId(),
		Content:   msg.GetContent(),
		Timestamp: msg.GetTimestampUnixMs(),
	}

	data, err := json.Marshal(outMsg)
	if err != nil {
		return &pb.DeliverMessageResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("failed to marshal message: %v", err),
		}, nil
	}

	count := s.hub.GetRoomSessionClientCount(req.GetRoomId(), req.GetSessionId())
	s.hub.BroadcastRawToRoomSession(req.GetRoomId(), req.GetSessionId(), data, "")

	l := log.Ctx(ctx)
	l.Info().Str("room_id", req.GetRoomId()).Str("session_id", req.GetSessionId()).Int("clients", count).Msg("delivered chat message")

	return &pb.DeliverMessageResponse{
		Success:        true,
		DeliveredCount: int32(count),
	}, nil
}

func (s *chatDeliveryServer) DeliverSystemMessage(ctx context.Context, req *pb.DeliverSystemMessageRequest) (*pb.DeliverSystemMessageResponse, error) {
	outMsg := &domain.SystemMessageOut{
		Type:            "system_message",
		Content:         req.GetContent(),
		SystemEventType: req.GetSystemEventType(),
		RoomID:          req.GetRoomId(),
		SessionID:       req.GetSessionId(),
		Timestamp:       time.Now().UnixMilli(),
	}

	data, err := json.Marshal(outMsg)
	if err != nil {
		return &pb.DeliverSystemMessageResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("failed to marshal system message: %v", err),
		}, nil
	}

	count := s.hub.GetRoomSessionClientCount(req.GetRoomId(), req.GetSessionId())
	s.hub.BroadcastRawToRoomSession(req.GetRoomId(), req.GetSessionId(), data, "")

	l := log.Ctx(ctx)
	l.Info().Str("room_id", req.GetRoomId()).Str("session_id", req.GetSessionId()).Int("clients", count).Msg("delivered system message")

	return &pb.DeliverSystemMessageResponse{
		Success:        true,
		DeliveredCount: int32(count),
	}, nil
}

func StartGRPCServer(addr string, h *hub.Hub, logger zerolog.Logger) (*grpc.Server, error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	s := grpc.NewServer(
		grpc.UnaryInterceptor(log.UnaryServerInterceptor(logger)),
		grpc.StreamInterceptor(log.StreamServerInterceptor(logger)),
	)
	pb.RegisterChatDeliveryServiceServer(s, &chatDeliveryServer{hub: h})

	go func() {
		l := log.L()
		l.Info().Str("address", addr).Msg("chat grpc server listening")
		if err := s.Serve(lis); err != nil {
			l.Error().Err(err).Msg("grpc server error")
		}
	}()

	return s, nil
}
