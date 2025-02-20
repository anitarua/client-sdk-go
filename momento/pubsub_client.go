package momento

import (
	"context"
	"math"

	"github.com/momentohq/client-sdk-go/config"
	"github.com/momentohq/client-sdk-go/config/logger"
	"github.com/momentohq/client-sdk-go/internal"
	"github.com/momentohq/client-sdk-go/internal/grpcmanagers"
	"github.com/momentohq/client-sdk-go/internal/models"
	"github.com/momentohq/client-sdk-go/internal/momentoerrors"
	pb "github.com/momentohq/client-sdk-go/internal/protos"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type pubSubClient struct {
	streamTopicManagers []*grpcmanagers.TopicGrpcManager
	endpoint            string
	log                 logger.MomentoLogger
}

var numChannels uint32

func newPubSubClient(request *models.PubSubClientRequest) (*pubSubClient, momentoerrors.MomentoSvcErr) {
	numSubscriptions := float64(request.TopicsConfiguration.GetMaxSubscriptions())
	numGrpcChannels := request.TopicsConfiguration.GetNumGrpcChannels()

	// numSubscriptions is deprecated. Nevertheless, check that numGrpcChannels and numSubscriptions
	// are not both set. They should be mutually exclusive configs.
	if numGrpcChannels > 0 && numSubscriptions > 0 {
		return nil, NewMomentoError(momentoerrors.InvalidArgumentError, "Cannot accept both maxSubscriptions and numGrpcChannels as arguments; please use numGrpcChannels as maxSubscriptions is deprecated", nil)
	}

	if numGrpcChannels > 0 {
		numChannels = numGrpcChannels
	} else if numSubscriptions > 0 {
		// a single channel can support 100 streams, so we need to create enough
		// channels to handle the maximum number of subscriptions
		// plus one for the publishing channel
		numChannels = uint32(math.Ceil((numSubscriptions + 1) / 100.0))
	} else {
		numChannels = 1
	}
	streamTopicManagers := make([]*grpcmanagers.TopicGrpcManager, 0)

	// NOTE: This is hard-coded for now but we may want to expose it via TopicConfiguration in the future,
	// as we do with some of the other clients. Defaults to keep-alive pings enabled.
	grpcConfig := config.NewStaticGrpcConfiguration(&config.GrpcConfigurationProps{})
	for i := 0; uint32(i) < numChannels; i++ {
		streamTopicManager, err := grpcmanagers.NewStreamTopicGrpcManager(&models.TopicStreamGrpcManagerRequest{
			CredentialProvider: request.CredentialProvider,
			GrpcConfiguration:  grpcConfig,
		})
		if err != nil {
			return nil, err
		}
		streamTopicManagers = append(streamTopicManagers, streamTopicManager)
	}

	return &pubSubClient{
		streamTopicManagers: streamTopicManagers,
		endpoint:            request.CredentialProvider.GetCacheEndpoint(),
		log:                 request.Log,
	}, nil
}

func (client *pubSubClient) getNextStreamTopicManager() (*grpcmanagers.TopicGrpcManager, momentoerrors.MomentoSvcErr) {
	// get next stream topic manager that is not full (number of stream connections <100)
	for _, topicManager := range client.streamTopicManagers {
		if topicManager.NumGrpcStreams.Load() < 100 {
			return topicManager, nil
		}
	}
	return nil, NewMomentoError(momentoerrors.LimitExceededError, "All grpc channels are occupied, cannot send new publish or subscribe requests", nil)
}

func (client *pubSubClient) topicSubscribe(ctx context.Context, request *TopicSubscribeRequest) (*grpcmanagers.TopicGrpcManager, grpc.ClientStream, context.Context, context.CancelFunc, error) {
	topicManager, grpcErr := client.getNextStreamTopicManager()
	if grpcErr != nil {
		return nil, nil, nil, nil, grpcErr
	}

	// add metadata to context
	requestMetadata := internal.CreateMetadata(ctx, internal.Topic)

	// add withCancel to context
	cancelContext, cancelFunction := context.WithCancel(requestMetadata)

	var header, trailer metadata.MD
	topicManager.NumGrpcStreams.Add(1)
	clientStream, err := topicManager.StreamClient.Subscribe(cancelContext, &pb.XSubscriptionRequest{
		CacheName:                   request.CacheName,
		Topic:                       request.TopicName,
		ResumeAtTopicSequenceNumber: request.ResumeAtTopicSequenceNumber,
		SequencePage:                request.SequencePage,
	})

	if err != nil {
		topicManager.NumGrpcStreams.Add(-1)
		cancelFunction()
		if clientStream != nil {
			header, _ = clientStream.Header()
			trailer = clientStream.Trailer()
		}
		return nil, nil, nil, nil, momentoerrors.ConvertSvcErr(err, header, trailer)
	}

	if topicManager.NumGrpcStreams.Load() > 0 && (int64(numChannels*100)-topicManager.NumGrpcStreams.Load() < 10) {
		client.log.Warn("WARNING: approaching grpc maximum concurrent stream limit, %d remaining of total %d streams\n", int64(numChannels*100)-topicManager.NumGrpcStreams.Load(), numChannels*100)
	}

	return topicManager, clientStream, cancelContext, cancelFunction, err
}

func (client *pubSubClient) topicPublish(ctx context.Context, request *TopicPublishRequest) error {
	topicManager, grpcErr := client.getNextStreamTopicManager()
	if grpcErr != nil {
		return grpcErr
	}

	requestMetadata := internal.CreateMetadata(ctx, internal.Topic)

	var header, trailer metadata.MD
	switch value := request.Value.(type) {
	case String:
		_, err := topicManager.StreamClient.Publish(requestMetadata, &pb.XPublishRequest{
			CacheName: request.CacheName,
			Topic:     request.TopicName,
			Value: &pb.XTopicValue{
				Kind: &pb.XTopicValue_Text{
					Text: value.asString(),
				},
			},
		}, grpc.Header(&header), grpc.Trailer(&trailer))
		if err != nil {
			return momentoerrors.ConvertSvcErr(err, header, trailer)
		}
		return err
	case Bytes:
		_, err := topicManager.StreamClient.Publish(requestMetadata, &pb.XPublishRequest{
			CacheName: request.CacheName,
			Topic:     request.TopicName,
			Value: &pb.XTopicValue{
				Kind: &pb.XTopicValue_Binary{
					Binary: value.asBytes(),
				},
			},
		}, grpc.Header(&header), grpc.Trailer(&trailer))
		if err != nil {
			return momentoerrors.ConvertSvcErr(err, header, trailer)
		}
		return err
	default:
		return momentoerrors.NewMomentoSvcErr(
			momentoerrors.InvalidArgumentError,
			"error encoding topic value only support []byte or string currently", nil,
		)
	}
}

func (client *pubSubClient) close() {
	for clientIndex := range client.streamTopicManagers {
		defer client.streamTopicManagers[clientIndex].Close()
	}
}
