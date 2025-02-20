package momento

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/momentohq/client-sdk-go/config/logger"
	"github.com/momentohq/client-sdk-go/internal/grpcmanagers"
	pb "github.com/momentohq/client-sdk-go/internal/protos"

	"google.golang.org/grpc"
)

type TopicSubscription interface {
	// Item returns only subscription events that contain a string or byte message.
	// Example:
	//
	//	item, err := sub.Item(ctx)
	//	if err != nil {
	//		panic(err)
	//	}
	//	switch msg := item.(type) {
	//	case momento.String:
	//		fmt.Printf("received message as string: '%v'\n", msg)
	//	case momento.Bytes:
	//		fmt.Printf("received message as bytes: '%v'\n", msg)
	//	}
	Item(ctx context.Context) (TopicValue, error)

	// Event returns all possible topics subscription events, such as messages,
	// discontinuities, and heartbeats.
	//
	// Example:
	//
	//	event, err := sub.Event(ctx)
	//	if err != nil {
	//		panic(err)
	//	}
	//
	//	switch e := event.(type) {
	//	case momento.TopicItem:
	//		fmt.Printf("received item with sequence number %d\n", e.GetTopicSequenceNumber())
	//		fmt.Printf("received item with publisher Id %s\n", e.GetPublisherId())
	//		switch msg := e.GetValue().(type) {
	//		case momento.String:
	//			fmt.Printf("received message as string: '%v'\n", msg)
	//		case momento.Bytes:
	//			fmt.Printf("received message as bytes: '%v'\n", msg)
	//		}
	//	case momento.TopicHeartbeat:
	//		fmt.Printf("received heartbeat\n")
	//	case momento.TopicDiscontinuity:
	//			fmt.Printf("received discontinuity\n")
	//	}
	Event(ctx context.Context) (TopicEvent, error)

	// Close closes the subscription stream.
	Close()
}

type topicSubscription struct {
	topicManager            *grpcmanagers.TopicGrpcManager
	grpcClient              grpc.ClientStream
	momentoTopicClient      *pubSubClient
	cacheName               string
	topicName               string
	log                     logger.MomentoLogger
	lastKnownSequenceNumber uint64
	lastKnownSequencePage   uint64
	cancelContext           context.Context
	cancelFunction          context.CancelFunc
	reconnectAttemptNumber  int
}

func (s *topicSubscription) Item(ctx context.Context) (TopicValue, error) {
	for {
		item, err := s.Event(ctx)
		if err != nil {
			return nil, err
		}

		switch item := item.(type) {
		case TopicItem:
			return item.GetValue(), nil
		case TopicHeartbeat:
			continue
		case TopicDiscontinuity:
			continue
		}
	}
}

func (s *topicSubscription) Event(ctx context.Context) (TopicEvent, error) {
	for {
		// Its totally possible a client just calls `cancel` on the `context` immediately after subscribing to an
		// item, so we should check that here.
		select {
		case <-ctx.Done():
			// fix the count to avoid excess warning logs
			numGrpcStreams.Add(-1)

			// Context has been canceled, return an error
			return nil, ctx.Err()
		case <-s.cancelContext.Done():
			// fix the count to avoid excess warning logs
			numGrpcStreams.Add(-1)

			// Context has been canceled, return an error
			return nil, s.cancelContext.Err()
		default:
			// Proceed as is
		}

		rawMsg := new(pb.XSubscriptionItem)
		if err := s.grpcClient.RecvMsg(rawMsg); err != nil {
			select {
			case <-ctx.Done():
				{
					// fix the count to avoid excess warning logs
					numGrpcStreams.Add(-1)

					s.log.Info("Subscription context is done; closing subscription.")
					return nil, ctx.Err()
				}
			case <-s.cancelContext.Done():
				{
					// fix the count to avoid excess warning logs
					numGrpcStreams.Add(-1)

					// s.log.Info("Subscription context is cancelled; closing subscription.")
					return nil, s.cancelContext.Err()
				}
			default:
				{
					// fix the count to avoid excess warning logs
					numGrpcStreams.Add(-1)

					// Attempt to reconnect
					s.log.Error("stream disconnected YO, attempting to reconnect err:", fmt.Sprint(err))
					s.attemptReconnect(ctx)
				}
			}

			// retry getting the latest item
			continue
		}

		switch typedMsg := rawMsg.Kind.(type) {
		case *pb.XSubscriptionItem_Discontinuity:
			s.log.Trace("received discontinuity item: %+v", typedMsg.Discontinuity)
			return NewTopicDiscontinuity(typedMsg.Discontinuity.LastTopicSequence, typedMsg.Discontinuity.NewTopicSequence, typedMsg.Discontinuity.NewSequencePage), nil
		case *pb.XSubscriptionItem_Item:
			s.lastKnownSequenceNumber = typedMsg.Item.GetTopicSequenceNumber()
			s.lastKnownSequencePage = typedMsg.Item.GetSequencePage()
			publisherId := typedMsg.Item.GetPublisherId()

			s.log.Trace("received item with sequence number %d, sequence page %d, and publisher Id %s", s.lastKnownSequenceNumber, s.lastKnownSequencePage, publisherId)

			switch subscriptionItem := typedMsg.Item.Value.Kind.(type) {
			case *pb.XTopicValue_Text:
				return NewTopicItem(String(subscriptionItem.Text), String(publisherId), s.lastKnownSequenceNumber, s.lastKnownSequencePage), nil
			case *pb.XTopicValue_Binary:
				return NewTopicItem(Bytes(subscriptionItem.Binary), String(publisherId), s.lastKnownSequenceNumber, s.lastKnownSequencePage), nil
			}
		case *pb.XSubscriptionItem_Heartbeat:
			s.log.Trace("received heartbeat item")
			return TopicHeartbeat{}, nil
		default:
			s.log.Warn("Unrecognized response detected.",
				"response", fmt.Sprint(typedMsg))
			continue
		}
	}
}

func randomInRange(min int, max int) int {
	if min >= max {
		return min
	}
	return min + int(float64(max-min)*rand.Float64())
}

func (s *topicSubscription) exponentialDelayWithJitter() time.Duration {
	initialDelayMs := 500

	// previous delay * 3 if reconnectAttemptNumber is >0
	maxDelayMs := initialDelayMs
	if s.reconnectAttemptNumber > 0 {
		maxDelayMs = int(3.0 * float64(initialDelayMs) * math.Pow(2, float64(s.reconnectAttemptNumber-1)))
	}

	exponentialDelayMs := float64(initialDelayMs) * math.Pow(2, float64(s.reconnectAttemptNumber))
	jitteredDelayMs := randomInRange(int(exponentialDelayMs), maxDelayMs)
	return time.Duration(jitteredDelayMs) * time.Millisecond
}

func (s *topicSubscription) attemptReconnect(ctx context.Context) {
	// This will attempt to reconnect indefinetly

	s.reconnectAttemptNumber++
	reconnectDelay := s.exponentialDelayWithJitter()
	for {
		s.log.Info("Attempting reconnecting to client stream after delaying for %d milliseconds", reconnectDelay)
		time.Sleep(reconnectDelay)
		newTopicManager, newStream, cancelContext, cancelFunction, err := s.momentoTopicClient.topicSubscribe(ctx, &TopicSubscribeRequest{
			CacheName:                   s.cacheName,
			TopicName:                   s.topicName,
			ResumeAtTopicSequenceNumber: s.lastKnownSequenceNumber,
			SequencePage:                s.lastKnownSequencePage,
		})

		if err != nil {
			s.log.Warn("failed to reconnect to stream, will try again soon")
		} else {
			s.log.Info("successfully reconnected to subscription stream")
			s.topicManager = newTopicManager
			s.grpcClient = newStream
			s.cancelContext = cancelContext
			s.cancelFunction = cancelFunction
			s.reconnectAttemptNumber = 0
			return
		}
	}
}

func (s *topicSubscription) Close() {
	numGrpcStreams.Add(-1)
	s.cancelFunction()
}
