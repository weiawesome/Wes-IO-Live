package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"time"

	pb "github.com/weiawesome/wes-io-live/proto/chat"
	"github.com/weiawesome/wes-io-live/chat-service/internal/domain"
	"github.com/weiawesome/wes-io-live/chat-service/internal/hub"
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

	log.Printf("Delivered chat message to room-session %s:%s (%d clients)", req.GetRoomId(), req.GetSessionId(), count)

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

	log.Printf("Delivered system message to room-session %s:%s (%d clients)", req.GetRoomId(), req.GetSessionId(), count)

	return &pb.DeliverSystemMessageResponse{
		Success:        true,
		DeliveredCount: int32(count),
	}, nil
}

func StartGRPCServer(addr string, h *hub.Hub) (*grpc.Server, error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	s := grpc.NewServer()
	pb.RegisterChatDeliveryServiceServer(s, &chatDeliveryServer{hub: h})

	go func() {
		log.Printf("Chat gRPC server listening on %s", addr)
		if err := s.Serve(lis); err != nil {
			log.Printf("gRPC server error: %v", err)
		}
	}()

	return s, nil
}
