// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.4.0
// - protoc             v3.20.3
// source: cachepubsub.proto

package client_sdk_go

import (
	context "context"

	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.62.0 or later.
const _ = grpc.SupportPackageIsVersion8

const (
	Pubsub_Publish_FullMethodName   = "/cache_client.pubsub.Pubsub/Publish"
	Pubsub_Subscribe_FullMethodName = "/cache_client.pubsub.Pubsub/Subscribe"
)

// PubsubClient is the client API for Pubsub service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
//
// For working with topics in a cache.
// Momento topics are conceptually located on a cache. They are best-effort multicast.
// To use them, create a cache then start subscribing and publishing!
//
// Momento topic subscriptions try to give you information about the quality of the
//
//	stream you are receiving. For example, you might miss messages if your network
//	is slow, or if some intermediate switch fails, or due to rate limiting. It is
//	also possible, though we try to avoid it, that messages could briefly come out
//	of order between subscribers.
//	We try to tell you when things like this happen via a Discontinuity in your
//	subscription stream. If you do not care about occasional discontinuities then
//	don't bother handling them! You might still want to log them just in case ;-)
type PubsubClient interface {
	// Publish a message to a topic.
	//
	// If a topic has no subscribers, then the effect of Publish MAY be either of:
	// * It is dropped and the topic is nonexistent.
	// * It is accepted to the topic as the next message.
	//
	// Publish() does not wait for subscribers to accept. It returns Ok upon accepting
	// the topic value. It also returns Ok if there are no subscribers and the value
	// happens to be dropped. Publish() can not guarantee delivery in theory but in
	// practice it should almost always deliver to subscribers.
	//
	// REQUIRES HEADER authorization: Momento auth token
	Publish(ctx context.Context, in *XPublishRequest, opts ...grpc.CallOption) (*XEmpty, error)
	// Subscribe to notifications from a topic.
	//
	// You will receive a stream of values and (hopefully occasional) discontinuities.
	// Values will appear as copies of the payloads you Publish() to the topic.
	//
	// REQUIRES HEADER authorization: Momento auth token
	Subscribe(ctx context.Context, in *XSubscriptionRequest, opts ...grpc.CallOption) (Pubsub_SubscribeClient, error)
}

type pubsubClient struct {
	cc grpc.ClientConnInterface
}

func NewPubsubClient(cc grpc.ClientConnInterface) PubsubClient {
	return &pubsubClient{cc}
}

func (c *pubsubClient) Publish(ctx context.Context, in *XPublishRequest, opts ...grpc.CallOption) (*XEmpty, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(XEmpty)
	err := c.cc.Invoke(ctx, Pubsub_Publish_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *pubsubClient) Subscribe(ctx context.Context, in *XSubscriptionRequest, opts ...grpc.CallOption) (Pubsub_SubscribeClient, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	stream, err := c.cc.NewStream(ctx, &Pubsub_ServiceDesc.Streams[0], Pubsub_Subscribe_FullMethodName, cOpts...)
	if err != nil {
		return nil, err
	}
	x := &pubsubSubscribeClient{ClientStream: stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type Pubsub_SubscribeClient interface {
	Recv() (*XSubscriptionItem, error)
	grpc.ClientStream
}

type pubsubSubscribeClient struct {
	grpc.ClientStream
}

func (x *pubsubSubscribeClient) Recv() (*XSubscriptionItem, error) {
	m := new(XSubscriptionItem)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// PubsubServer is the server API for Pubsub service.
// All implementations must embed UnimplementedPubsubServer
// for forward compatibility
//
// For working with topics in a cache.
// Momento topics are conceptually located on a cache. They are best-effort multicast.
// To use them, create a cache then start subscribing and publishing!
//
// Momento topic subscriptions try to give you information about the quality of the
//
//	stream you are receiving. For example, you might miss messages if your network
//	is slow, or if some intermediate switch fails, or due to rate limiting. It is
//	also possible, though we try to avoid it, that messages could briefly come out
//	of order between subscribers.
//	We try to tell you when things like this happen via a Discontinuity in your
//	subscription stream. If you do not care about occasional discontinuities then
//	don't bother handling them! You might still want to log them just in case ;-)
type PubsubServer interface {
	// Publish a message to a topic.
	//
	// If a topic has no subscribers, then the effect of Publish MAY be either of:
	// * It is dropped and the topic is nonexistent.
	// * It is accepted to the topic as the next message.
	//
	// Publish() does not wait for subscribers to accept. It returns Ok upon accepting
	// the topic value. It also returns Ok if there are no subscribers and the value
	// happens to be dropped. Publish() can not guarantee delivery in theory but in
	// practice it should almost always deliver to subscribers.
	//
	// REQUIRES HEADER authorization: Momento auth token
	Publish(context.Context, *XPublishRequest) (*XEmpty, error)
	// Subscribe to notifications from a topic.
	//
	// You will receive a stream of values and (hopefully occasional) discontinuities.
	// Values will appear as copies of the payloads you Publish() to the topic.
	//
	// REQUIRES HEADER authorization: Momento auth token
	Subscribe(*XSubscriptionRequest, Pubsub_SubscribeServer) error
	mustEmbedUnimplementedPubsubServer()
}

// UnimplementedPubsubServer must be embedded to have forward compatible implementations.
type UnimplementedPubsubServer struct {
}

func (UnimplementedPubsubServer) Publish(context.Context, *XPublishRequest) (*XEmpty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Publish not implemented")
}
func (UnimplementedPubsubServer) Subscribe(*XSubscriptionRequest, Pubsub_SubscribeServer) error {
	return status.Errorf(codes.Unimplemented, "method Subscribe not implemented")
}
func (UnimplementedPubsubServer) mustEmbedUnimplementedPubsubServer() {}

// UnsafePubsubServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to PubsubServer will
// result in compilation errors.
type UnsafePubsubServer interface {
	mustEmbedUnimplementedPubsubServer()
}

func RegisterPubsubServer(s grpc.ServiceRegistrar, srv PubsubServer) {
	s.RegisterService(&Pubsub_ServiceDesc, srv)
}

func _Pubsub_Publish_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(XPublishRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(PubsubServer).Publish(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Pubsub_Publish_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(PubsubServer).Publish(ctx, req.(*XPublishRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Pubsub_Subscribe_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(XSubscriptionRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(PubsubServer).Subscribe(m, &pubsubSubscribeServer{ServerStream: stream})
}

type Pubsub_SubscribeServer interface {
	Send(*XSubscriptionItem) error
	grpc.ServerStream
}

type pubsubSubscribeServer struct {
	grpc.ServerStream
}

func (x *pubsubSubscribeServer) Send(m *XSubscriptionItem) error {
	return x.ServerStream.SendMsg(m)
}

// Pubsub_ServiceDesc is the grpc.ServiceDesc for Pubsub service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Pubsub_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "cache_client.pubsub.Pubsub",
	HandlerType: (*PubsubServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Publish",
			Handler:    _Pubsub_Publish_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Subscribe",
			Handler:       _Pubsub_Subscribe_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "cachepubsub.proto",
}
