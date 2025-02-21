package grpcmanagers

import (
	"sync"

	"github.com/momentohq/client-sdk-go/internal/interceptor"
	"github.com/momentohq/client-sdk-go/internal/models"
	"github.com/momentohq/client-sdk-go/internal/momentoerrors"
	pb "github.com/momentohq/client-sdk-go/internal/protos"
	"google.golang.org/grpc"
)

type NumGrpcStreams struct {
	mu    sync.Mutex
	count int
}

func (n *NumGrpcStreams) Increment() momentoerrors.MomentoSvcErr {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.count == 99 {
		return momentoerrors.NewMomentoSvcErr(momentoerrors.LimitExceededError, "Maximum number of streams reached", nil)
	}
	n.count++
	return nil
}

func (n *NumGrpcStreams) Decrement() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.count--
}

func (n *NumGrpcStreams) Reset() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.count = 0
}

func (n *NumGrpcStreams) Get() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.count
}

type TopicGrpcManager struct {
	Conn           *grpc.ClientConn
	StreamClient   pb.PubsubClient
	NumGrpcStreams NumGrpcStreams
}

func NewStreamTopicGrpcManager(request *models.TopicStreamGrpcManagerRequest) (*TopicGrpcManager, momentoerrors.MomentoSvcErr) {
	endpoint := request.CredentialProvider.GetCacheEndpoint()
	authToken := request.CredentialProvider.GetAuthToken()

	headerInterceptors := []grpc.StreamClientInterceptor{
		interceptor.AddStreamHeaderInterceptor(authToken),
	}

	conn, err := grpc.NewClient(
		endpoint,
		AllDialOptions(
			request.GrpcConfiguration,
			request.CredentialProvider.IsCacheEndpointSecure(),
			grpc.WithChainStreamInterceptor(headerInterceptors...),
			grpc.WithChainUnaryInterceptor(interceptor.AddAuthHeadersInterceptor(authToken)),
		)...,
	)

	if err != nil {
		return nil, momentoerrors.ConvertSvcErr(err)
	}
	return &TopicGrpcManager{
		Conn:         conn,
		StreamClient: pb.NewPubsubClient(conn),
	}, nil
}

func (topicManager *TopicGrpcManager) Close() momentoerrors.MomentoSvcErr {
	topicManager.NumGrpcStreams.Reset()
	return momentoerrors.ConvertSvcErr(topicManager.Conn.Close())
}
