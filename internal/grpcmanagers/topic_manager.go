package grpcmanagers

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/momentohq/client-sdk-go/internal/interceptor"
	"github.com/momentohq/client-sdk-go/internal/models"
	"github.com/momentohq/client-sdk-go/internal/momentoerrors"
	pb "github.com/momentohq/client-sdk-go/internal/protos"
	"google.golang.org/grpc"
)

type TopicGrpcManager struct {
	Conn                   *grpc.ClientConn
	StreamClient           pb.PubsubClient
	NumActiveSubscriptions atomic.Int64
	ManagerId              int
}

func NewStreamTopicGrpcManager(request *models.TopicStreamGrpcManagerRequest, id int) (*TopicGrpcManager, momentoerrors.MomentoSvcErr) {
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

	newTopicManager := &TopicGrpcManager{
		Conn:         conn,
		StreamClient: pb.NewPubsubClient(conn),
		ManagerId:    id,
	}

	// occasionally print number of active subscriptions
	go func() {
		for {
			<-time.After(15 * time.Second)
			fmt.Printf("Channel %d active subscriptions: %d\n", newTopicManager.ManagerId, newTopicManager.NumActiveSubscriptions.Load())
		}
	}()

	return newTopicManager, nil
}

func (topicManager *TopicGrpcManager) Close() momentoerrors.MomentoSvcErr {
	topicManager.NumActiveSubscriptions.Store(0)
	return momentoerrors.ConvertSvcErr(topicManager.Conn.Close())
}
