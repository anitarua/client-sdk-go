package services

import (
	"context"

	"time"

	"github.com/momentohq/client-sdk-go/internal/models"
	"github.com/momentohq/client-sdk-go/internal/momentoerrors"
	pb "github.com/momentohq/client-sdk-go/internal/protos"
	"github.com/momentohq/client-sdk-go/utils"
	"google.golang.org/grpc/metadata"
)

func (client *ScsDataClient) ListFetch(ctx context.Context, request *models.ListFetchRequest) (models.ListFetchResponse, momentoerrors.MomentoSvcErr) {
	ctx, cancel := context.WithTimeout(ctx, client.requestTimeout)
	defer cancel()
	resp, err := client.grpcClient.ListFetch(
		metadata.NewOutgoingContext(ctx, createNewMetadata(request.CacheName)),
		&pb.XListFetchRequest{ListName: []byte(request.ListName)},
	)
	if err != nil {
		return nil, momentoerrors.ConvertSvcErr(err)
	}
	// Convert from grpc struct to internal struct
	if resp.GetFound() != nil {
		return &models.ListFetchHit{Value: resp.GetFound().Values}, nil
	} else if resp.GetMissing() != nil {
		return &models.ListFetchMiss{}, nil
	} else {
		return nil, momentoerrors.NewMomentoSvcErr(
			momentoerrors.ClientSdkError,
			"Unknown response type for list fetch",
			nil,
		)
	}
}

func (client *ScsDataClient) ListLength(ctx context.Context, request *models.ListLengthRequest) (models.ListLengthResponse, momentoerrors.MomentoSvcErr) {
	ctx, cancel := context.WithTimeout(ctx, client.requestTimeout)
	defer cancel()
	resp, err := client.grpcClient.ListLength(
		metadata.NewOutgoingContext(ctx, createNewMetadata(request.CacheName)),
		&pb.XListLengthRequest{ListName: []byte(request.ListName)},
	)
	if err != nil {
		return nil, momentoerrors.ConvertSvcErr(err)
	}

	switch r := resp.List.(type) {
	case *pb.XListLengthResponse_Found:
		return &models.ListLengthSuccess{Value: r.Found.Length}, nil
	case *pb.XListLengthResponse_Missing:
		return &models.ListLengthSuccess{Value: 0}, nil
	default:
		return nil, momentoerrors.NewMomentoSvcErr(
			momentoerrors.ClientSdkError,
			"Unknown response type for list length",
			nil,
		)
	}
}

func (client *ScsDataClient) ListPushFront(ctx context.Context, request *models.ListPushFrontRequest) (models.ListPushFrontResponse, momentoerrors.MomentoSvcErr) {
	ctx, cancel := context.WithTimeout(ctx, client.requestTimeout)
	defer cancel()
	resp, err := client.grpcClient.ListPushFront(
		metadata.NewOutgoingContext(ctx, createNewMetadata(request.CacheName)),
		&pb.XListPushFrontRequest{
			ListName:           []byte(request.ListName),
			Value:              request.Value,
			TruncateBackToSize: request.TruncateBackToSize,
			RefreshTtl:         request.CollectionTtl.RefreshTtl,
			TtlMilliseconds:    collectionTtlOrDefaultMilliseconds(request.CollectionTtl, client.defaultTtl),
		},
	)
	if err != nil {
		return nil, momentoerrors.ConvertSvcErr(err)
	}
	return &models.ListPushFrontSuccess{Value: resp.ListLength}, nil
}

func (client *ScsDataClient) ListPushBack(ctx context.Context, request *models.ListPushBackRequest) (models.ListPushBackResponse, momentoerrors.MomentoSvcErr) {
	ctx, cancel := context.WithTimeout(ctx, client.requestTimeout)
	defer cancel()
	resp, err := client.grpcClient.ListPushBack(
		metadata.NewOutgoingContext(ctx, createNewMetadata(request.CacheName)),
		&pb.XListPushBackRequest{
			ListName:            []byte(request.ListName),
			Value:               request.Value,
			TruncateFrontToSize: request.TruncateFrontToSize,
			RefreshTtl:          request.CollectionTtl.RefreshTtl,
			TtlMilliseconds:     collectionTtlOrDefaultMilliseconds(request.CollectionTtl, client.defaultTtl),
		},
	)
	if err != nil {
		return nil, momentoerrors.ConvertSvcErr(err)
	}
	return &models.ListPushBackSuccess{Value: resp.ListLength}, nil
}

func (client *ScsDataClient) ListPopFront(ctx context.Context, request *models.ListPopFrontRequest) (models.ListPopFrontResponse, momentoerrors.MomentoSvcErr) {
	ctx, cancel := context.WithTimeout(ctx, client.requestTimeout)
	defer cancel()
	resp, err := client.grpcClient.ListPopFront(
		metadata.NewOutgoingContext(ctx, createNewMetadata(request.CacheName)),
		&pb.XListPopFrontRequest{
			ListName: []byte(request.ListName),
		},
	)
	if err != nil {
		return nil, momentoerrors.ConvertSvcErr(err)
	}
	switch r := resp.List.(type) {
	case *pb.XListPopFrontResponse_Found:
		return &models.ListPopFrontHit{Value: r.Found.Front}, nil
	case *pb.XListPopFrontResponse_Missing:
		return &models.ListPopFrontMiss{}, nil
	default:
		return nil, momentoerrors.NewMomentoSvcErr(
			momentoerrors.ClientSdkError,
			"Unknown response type for list pop front",
			nil,
		)
	}
}

func (client *ScsDataClient) ListPopBack(ctx context.Context, request *models.ListPopBackRequest) (models.ListPopBackResponse, momentoerrors.MomentoSvcErr) {
	ctx, cancel := context.WithTimeout(ctx, client.requestTimeout)
	defer cancel()
	resp, err := client.grpcClient.ListPopBack(
		metadata.NewOutgoingContext(ctx, createNewMetadata(request.CacheName)),
		&pb.XListPopBackRequest{
			ListName: []byte(request.ListName),
		},
	)
	if err != nil {
		return nil, momentoerrors.ConvertSvcErr(err)
	}
	switch r := resp.List.(type) {
	case *pb.XListPopBackResponse_Found:
		return &models.ListPopBackHit{Value: r.Found.Back}, nil
	case *pb.XListPopBackResponse_Missing:
		return &models.ListPopBackMiss{}, nil
	default:
		return nil, momentoerrors.NewMomentoSvcErr(
			momentoerrors.ClientSdkError,
			"Unknown response type for list pop back",
			nil,
		)
	}
}

func (client *ScsDataClient) ListRemoveValue(ctx context.Context, request *models.ListRemoveValueRequest) momentoerrors.MomentoSvcErr {
	ctx, cancel := context.WithTimeout(ctx, client.requestTimeout)
	defer cancel()
	_, err := client.grpcClient.ListRemove(
		metadata.NewOutgoingContext(ctx, createNewMetadata(request.CacheName)),
		&pb.XListRemoveRequest{
			ListName: []byte(request.ListName),
			Remove: &pb.XListRemoveRequest_AllElementsWithValue{
				AllElementsWithValue: request.Value,
			},
		},
	)
	if err != nil {
		return momentoerrors.ConvertSvcErr(err)
	}
	return nil
}

func (client *ScsDataClient) ListDelete(ctx context.Context, request *models.ListDeleteRequest) momentoerrors.MomentoSvcErr {
	ctx, cancel := context.WithTimeout(ctx, client.requestTimeout)
	defer cancel()
	_, err := client.grpcClient.ListErase(
		metadata.NewOutgoingContext(ctx, createNewMetadata(request.CacheName)),
		&pb.XListEraseRequest{
			ListName: []byte(request.ListName),
			Erase:    &pb.XListEraseRequest_All{},
		},
	)
	if err != nil {
		return momentoerrors.ConvertSvcErr(err)
	}
	return nil
}

func collectionTtlOrDefaultMilliseconds(collectionTtl utils.CollectionTTL, defaultTtl time.Duration) uint64 {
	return ttlOrDefaultMilliseconds(collectionTtl.Ttl, defaultTtl)
}

func ttlOrDefaultMilliseconds(ttl time.Duration, defaultTtl time.Duration) uint64 {
	theTtl := defaultTtl
	if ttl != 0 {
		theTtl = ttl
	}
	return uint64(theTtl.Milliseconds())
}