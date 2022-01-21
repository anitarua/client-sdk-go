package main

import (
	"context"
	"crypto/tls"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

type controlGrpcManager struct {
	Conn	*grpc.ClientConn
}

func NewControlGrpcManager(authToken string, endPoint string) (controlGrpcManager, error) {
	config := &tls.Config{
		InsecureSkipVerify: false,
	}
	conn, err := grpc.Dial(endPoint, grpc.WithTransportCredentials(credentials.NewTLS(config)), grpc.WithDisableRetry(), grpc.WithUnaryInterceptor(addHeadersInterceptor(authToken)))
	return controlGrpcManager{Conn: conn}, err
}

func (cm *controlGrpcManager) Close() error {
	return cm.Conn.Close()
}

func addHeadersInterceptor(authToken string) func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	return func (ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", authToken)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
