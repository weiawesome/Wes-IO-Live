package webrtc

import (
	"fmt"
	"log"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/intervalpli"
	"github.com/pion/webrtc/v4"
)

// PeerManager manages WebRTC peer connections for rooms.
type PeerManager struct {
	iceServers []webrtc.ICEServer
}

// NewPeerManager creates a new PeerManager.
func NewPeerManager(iceServers []webrtc.ICEServer) *PeerManager {
	return &PeerManager{
		iceServers: iceServers,
	}
}

// TrackHandler is called when a track is received.
type TrackHandler func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver)

// StateHandler is called when connection state changes.
type StateHandler func(state webrtc.PeerConnectionState)

// ICECandidateHandler is called when an ICE candidate is generated.
type ICECandidateHandler func(candidate *webrtc.ICECandidate)

// CreatePeerConnection creates a new peer connection with the given handlers.
func (pm *PeerManager) CreatePeerConnection(
	onTrack TrackHandler,
	onState StateHandler,
	onICECandidate ICECandidateHandler,
) (*webrtc.PeerConnection, error) {
	m := &webrtc.MediaEngine{}

	// Register VP8 codec
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeVP8,
			ClockRate:   90000,
			Channels:    0,
			SDPFmtpLine: "",
		},
		PayloadType: 96,
	}, webrtc.RTPCodecTypeVideo); err != nil {
		return nil, err
	}

	// Register VP9 codec
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeVP9,
			ClockRate:   90000,
			Channels:    0,
			SDPFmtpLine: "",
		},
		PayloadType: 98,
	}, webrtc.RTPCodecTypeVideo); err != nil {
		return nil, err
	}

	// Register H264 codec
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeH264,
			ClockRate:   90000,
			Channels:    0,
			SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f",
		},
		PayloadType: 102,
	}, webrtc.RTPCodecTypeVideo); err != nil {
		return nil, err
	}

	// Register Opus codec for audio
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeOpus,
			ClockRate:   48000,
			Channels:    2,
			SDPFmtpLine: "minptime=10;useinbandfec=1",
		},
		PayloadType: 111,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		return nil, err
	}

	// Create interceptor registry with PLI support
	i := &interceptor.Registry{}

	intervalPliFactory, err := intervalpli.NewReceiverInterceptor()
	if err != nil {
		return nil, err
	}
	i.Add(intervalPliFactory)

	if err := webrtc.RegisterDefaultInterceptors(m, i); err != nil {
		return nil, err
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithInterceptorRegistry(i))

	config := webrtc.Configuration{
		ICEServers: pm.iceServers,
	}

	pc, err := api.NewPeerConnection(config)
	if err != nil {
		return nil, err
	}

	// Set up handlers
	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("Track received: %s (type: %s)", track.Codec().MimeType, track.Kind().String())
		if onTrack != nil {
			onTrack(track, receiver)
		}
	})

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("Connection State: %s", state.String())
		if onState != nil {
			onState(state)
		}
	})

	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Printf("ICE Connection State: %s", state.String())
	})

	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil && onICECandidate != nil {
			onICECandidate(candidate)
		}
	})

	return pc, nil
}

// HandleOffer processes an SDP offer and returns an SDP answer.
func (pm *PeerManager) HandleOffer(
	pc *webrtc.PeerConnection,
	offerSDP string,
) (string, error) {
	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offerSDP,
	}

	if err := pc.SetRemoteDescription(offer); err != nil {
		return "", fmt.Errorf("failed to set remote description: %w", err)
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		return "", fmt.Errorf("failed to create answer: %w", err)
	}

	gatherComplete := webrtc.GatheringCompletePromise(pc)

	if err := pc.SetLocalDescription(answer); err != nil {
		return "", fmt.Errorf("failed to set local description: %w", err)
	}

	<-gatherComplete

	return pc.LocalDescription().SDP, nil
}

// AddICECandidate adds an ICE candidate to the peer connection.
func (pm *PeerManager) AddICECandidate(pc *webrtc.PeerConnection, candidateJSON string) error {
	candidate := webrtc.ICECandidateInit{}
	candidate.Candidate = candidateJSON
	return pc.AddICECandidate(candidate)
}
