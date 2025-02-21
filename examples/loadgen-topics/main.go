package main

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HdrHistogram/hdrhistogram-go"
	"github.com/loov/hrtime"
	"github.com/momentohq/client-sdk-go/auth"
	"github.com/momentohq/client-sdk-go/config"
	"github.com/momentohq/client-sdk-go/config/logger"
	"github.com/momentohq/client-sdk-go/config/logger/momento_default_logger"
	"github.com/momentohq/client-sdk-go/momento"
)

const (
	CacheItemTtlSeconds = 60
)

type topicsLoadGeneratorOptions struct {
	cacheName         string
	logLevel          momento_default_logger.LogLevel
	showStatsInterval time.Duration
	messageBytes      int
	numberOfUsers     int
	numberOfTopics    int
	maxPublishTps     int
	howLongToRun      time.Duration
}

type loadGenerator struct {
	loggerFactory     logger.MomentoLoggerFactory
	logger            logger.MomentoLogger
	topicClientConfig config.TopicsConfiguration
	options           topicsLoadGeneratorOptions
	messageValue      string
}

type ErrorCounter struct {
	unavailable   int64
	timeout       int64
	limitExceeded int64
}

func newLoadGenerator(config config.TopicsConfiguration, options topicsLoadGeneratorOptions) *loadGenerator {
	loggerFactory := config.GetLoggerFactory()
	lgLogger := loggerFactory.GetLogger("topic-loadgen")
	unixMilli := time.Now().UnixMilli()
	timestampLength := len(strconv.FormatInt(unixMilli, 10))
	if options.messageBytes < timestampLength {
		panic(fmt.Sprintf("Error: messageBytes must be at least %d", timestampLength))
	}
	messageValue := strings.Repeat("x", options.messageBytes-timestampLength)
	return &loadGenerator{
		loggerFactory:     loggerFactory,
		logger:            lgLogger,
		topicClientConfig: config,
		options:           options,
		messageValue:      messageValue,
	}
}

func (r *loadGenerator) init(ctx context.Context) (momento.TopicClient, momento.CacheClient) {
	CacheName := r.options.cacheName
	credentialProvider, err := auth.FromEnvironmentVariable("MOMENTO_API_KEY")
	if err != nil {
		panic(err)
	}

	cacheClient, err := momento.NewCacheClient(config.LaptopLatest(), credentialProvider, time.Second*CacheItemTtlSeconds)
	if err != nil {
		panic(err)
	}

	if _, err := cacheClient.CreateCache(ctx, &momento.CreateCacheRequest{CacheName: CacheName}); err != nil {
		panic(err)
	}

	client, err := momento.NewTopicClient(r.topicClientConfig, credentialProvider)
	if err != nil {
		panic(err)
	}

	return client, cacheClient
}

func teardown(ctx context.Context, cacheName string, cacheClient momento.CacheClient) {
	if _, err := cacheClient.DeleteCache(ctx, &momento.DeleteCacheRequest{CacheName: cacheName}); err != nil {
		panic(err)
	}
}

func (ec *ErrorCounter) updateErrors(err string) {
	if err == momento.ServerUnavailableError {
		ec.unavailable++
	} else if err == momento.TimeoutError {
		ec.timeout++
	} else if err == momento.LimitExceededError {
		ec.limitExceeded++
	}
}

func user(
	ctx context.Context,
	id int,
	subscribeChan chan int64,
	publishChan chan int64,
	subscribeErrChan chan string,
	publishErrChan chan string,
	client momento.TopicClient,
	cacheName string,
	topicName string,
	messageValue string,
	publishTps int,
) {
	subscription, err := client.Subscribe(ctx, &momento.TopicSubscribeRequest{
		CacheName: cacheName,
		TopicName: topicName,
	})
	if err != nil {
		panic(err)
	}
	go func() { pollForMessages(ctx, id, subscription, subscribeChan, subscribeErrChan) }()
	go func() {
		publishMessages(ctx, id, publishChan, publishErrChan, client, cacheName, topicName, messageValue, publishTps)
	}()
}

func publishMessages(
	ctx context.Context,
	id int,
	publishChan chan int64,
	publishErrChan chan string,
	client momento.TopicClient,
	cacheName string,
	topicName string,
	messageValue string,
	publishTps int,
) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			publishStart := hrtime.Now()
			_, err := client.Publish(ctx, &momento.TopicPublishRequest{
				CacheName: cacheName,
				TopicName: topicName,
				Value: momento.String(
					fmt.Sprintf("%s%s", strconv.FormatInt(time.Now().UnixMilli(), 10), messageValue),
				),
			})
			if err != nil {
				processError(err, publishErrChan)
			} else {
				publishChan <- hrtime.Since(publishStart).Milliseconds()
			}
			sleepMillis := 1000 / publishTps
			time.Sleep(time.Millisecond * time.Duration(sleepMillis))
		}
	}
}

func pollForMessages(
	ctx context.Context, id int, sub momento.TopicSubscription, subscribeChan chan int64, subscribeErrChan chan string,
) {
	timestampLength := len(strconv.FormatInt(time.Now().UnixMilli(), 10))
	for {
		select {
		case <-ctx.Done():
			return
		default:
			item, err := sub.Item(ctx)
			if err != nil {
				processError(err, subscribeErrChan)
				return
			}
			timestamp, err := strconv.ParseInt(fmt.Sprintf("%v", item)[0:timestampLength], 10, 64)
			if err != nil {
				processError(err, subscribeErrChan)
				return
			} else {
				elapsed := time.Now().UnixMilli() - timestamp
				subscribeChan <- elapsed
			}
		}
	}
}

func processError(err error, errChan chan string) {
	switch mErr := err.(type) {
	case momento.MomentoError:
		if mErr.Code() == momento.ServerUnavailableError ||
			mErr.Code() == momento.TimeoutError ||
			mErr.Code() == momento.LimitExceededError {
			errChan <- mErr.Code()
		} else {
			panic(fmt.Sprintf("unrecognized result: %T", mErr))
		}
	default:
		// panic(fmt.Sprintf("unknown error type %T", err))
		// fmt.Printf("unknown error type %T\n", err)
	}
}

func printStats(
	subscribes *hdrhistogram.Histogram,
	publishes *hdrhistogram.Histogram,
	subscribeErrorCounter ErrorCounter,
	publishErrorCounter ErrorCounter,
	startTime time.Duration,
) {
	fmt.Println("[num goroutines]: ", runtime.NumGoroutine())

	successfulSubscriptionRequests := subscribes.TotalCount()
	totalSubscriptionRequests := successfulSubscriptionRequests +
		subscribeErrorCounter.timeout +
		subscribeErrorCounter.unavailable +
		subscribeErrorCounter.limitExceeded
	totalSubscriptionTps := int(math.Round(
		float64(totalSubscriptionRequests * 1000 / hrtime.Since(startTime).Milliseconds()),
	))
	subscribeSuccessTps := int(math.Round(
		float64(successfulSubscriptionRequests * 1000 / hrtime.Since(startTime).Milliseconds()),
	))
	subscribeSuccessPct := readablePercentage(successfulSubscriptionRequests, totalSubscriptionRequests)

	successfulPublishRequests := publishes.TotalCount()
	totalPublishRequests := successfulPublishRequests +
		publishErrorCounter.timeout +
		publishErrorCounter.unavailable +
		publishErrorCounter.limitExceeded
	totalPublishTps := int(math.Round(
		float64(totalPublishRequests * 1000 / hrtime.Since(startTime).Milliseconds()),
	))
	publishSuccessTps := int(math.Round(
		float64(successfulPublishRequests * 1000 / hrtime.Since(startTime).Milliseconds()),
	))
	publishSuccessPct := readablePercentage(successfulPublishRequests, totalPublishRequests)

	fmt.Println("==============================\ncumulative stats:")
	fmt.Printf(
		"%20s: %d (%d tps)\n",
		"total subscription requests",
		totalSubscriptionRequests,
		totalSubscriptionTps,
	)

	fmt.Printf("%20s: %d (%d%%) (%d tps)\n", "subscribe success", successfulSubscriptionRequests, subscribeSuccessPct, subscribeSuccessTps)
	fmt.Printf("%20s: %d (%d%%)\n", "unavailable", subscribeErrorCounter.unavailable, readablePercentage(subscribeErrorCounter.unavailable, totalSubscriptionRequests))
	fmt.Printf("%20s: %d (%d%%)\n", "timeout exceeded", subscribeErrorCounter.timeout, readablePercentage(subscribeErrorCounter.timeout, totalSubscriptionRequests))
	fmt.Printf("%20s: %d (%d%%)\n\n", "limit exceeded", subscribeErrorCounter.limitExceeded, readablePercentage(subscribeErrorCounter.limitExceeded, totalSubscriptionRequests))

	fmt.Printf(
		"%20s: %d (%d tps)\n",
		"total publish requests",
		totalPublishRequests,
		totalPublishTps,
	)
	fmt.Printf("%20s: %d (%d%%) (%d tps)\n", "publish success", successfulPublishRequests, publishSuccessPct, publishSuccessTps)
	fmt.Printf("%20s: %d (%d%%)\n", "unavailable", publishErrorCounter.unavailable, readablePercentage(publishErrorCounter.unavailable, totalSubscriptionRequests))
	fmt.Printf("%20s: %d (%d%%)\n", "timeout exceeded", publishErrorCounter.timeout, readablePercentage(publishErrorCounter.timeout, totalSubscriptionRequests))
	fmt.Printf("%20s: %d (%d%%)\n\n", "limit exceeded", publishErrorCounter.limitExceeded, readablePercentage(publishErrorCounter.limitExceeded, totalPublishRequests))
	fmt.Printf(
		"cumulative subscription latencies:\n%20s: %d\n%20s: %d\n%20s: %d\n%20s: %d\n%20s: %d\n%20s: %d\n\n",
		"total requests",
		subscribes.TotalCount(),
		"p50",
		subscribes.ValueAtQuantile(50.0),
		"p90",
		subscribes.ValueAtQuantile(90.0),
		"p99",
		subscribes.ValueAtQuantile(99.0),
		"p99.9",
		subscribes.ValueAtQuantile(99.9),
		"max",
		subscribes.Max(),
	)
	fmt.Printf(
		"cumulative publish latencies:\n%20s: %d\n%20s: %d\n%20s: %d\n%20s: %d\n%20s: %d\n%20s: %d\n\n",
		"total requests",
		publishes.TotalCount(),
		"p50",
		publishes.ValueAtQuantile(50.0),
		"p90",
		publishes.ValueAtQuantile(90.0),
		"p99",
		publishes.ValueAtQuantile(99.0),
		"p99.9",
		publishes.ValueAtQuantile(99.9),
		"max",
		publishes.Max(),
	)
}

func readablePercentage(numerator int64, denominator int64) int {
	return int(math.Ceil(float64(numerator) / float64(denominator) * 100))
}

func timer(
	ctx context.Context,
	subscribeChan chan int64,
	publishChan chan int64,
	subscribeErrChan chan string,
	publishErrChan chan string,
	statsInterval time.Duration,
) {
	subscribeHistogram := hdrhistogram.New(1, 500000, 1)
	publishHistogram := hdrhistogram.New(1, 500000, 1)
	subscribeErrorCounter := ErrorCounter{}
	publishErrorCounter := ErrorCounter{}

	startTime := hrtime.Now()
	origStartTime := startTime

	for {
		if hrtime.Since(startTime) > statsInterval {
			printStats(subscribeHistogram, publishHistogram, subscribeErrorCounter, publishErrorCounter, origStartTime)
			startTime = hrtime.Now()
		}

		select {
		case <-ctx.Done():
			fmt.Println("\n=====> run complete <=====")
			fmt.Println("[num goroutines]: ", runtime.NumGoroutine())
			printStats(subscribeHistogram, publishHistogram, subscribeErrorCounter, publishErrorCounter, origStartTime)
			return
		case subscribeMessage := <-subscribeChan:
			if err := subscribeHistogram.RecordValue(subscribeMessage); err != nil {
				panic(err)
			}
		case publishMessage := <-publishChan:
			if err := publishHistogram.RecordValue(publishMessage); err != nil {
				panic(err)
			}
		case errCode := <-subscribeErrChan:
			subscribeErrorCounter.updateErrors(errCode)
		case errCode := <-publishErrChan:
			publishErrorCounter.updateErrors(errCode)
		default:
			time.Sleep(time.Millisecond * 25)
		}
	}
}

func (r *loadGenerator) run(ctx context.Context, client momento.TopicClient) {
	cancelContext, cancelFunction := context.WithTimeout(ctx, r.options.howLongToRun)
	defer cancelFunction()

	var wg sync.WaitGroup
	subscribeChan := make(chan int64, r.options.numberOfUsers)
	publishChan := make(chan int64, r.options.numberOfUsers)
	subscribeErrChan := make(chan string, r.options.numberOfUsers)
	publishErrChan := make(chan string, r.options.numberOfUsers)

	wg.Add(1)
	go func() {
		defer wg.Done()
		timer(cancelContext, subscribeChan, publishChan, subscribeErrChan, publishErrChan, r.options.showStatsInterval)
	}()

	// Launch and run users. Each user subscribes to a random topic over which it
	// publishes and receives.
	randSeed := rand.NewSource(time.Now().UnixNano())
	randGenerator := rand.New(randSeed)

	for i := 1; i <= r.options.numberOfUsers; i++ {

		// choose a topic at random
		topicName := fmt.Sprintf("topic-%d", randGenerator.Intn(r.options.numberOfTopics))

		// USER GOROUTINES
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			user(
				cancelContext,
				i,
				subscribeChan,
				publishChan,
				subscribeErrChan,
				publishErrChan,
				client,
				r.options.cacheName,
				topicName,
				r.messageValue,
				r.options.maxPublishTps,
			)
		}()
	}

	wg.Wait()
	fmt.Println("all goroutines done")
}

func main() {
	ctx := context.Background()
	cacheName := "go-topic-loadgen"

	opts := topicsLoadGeneratorOptions{
		cacheName:         cacheName,
		logLevel:          momento_default_logger.DEBUG,
		showStatsInterval: time.Second * 30,
		// must be at least 13 to accommodate an epoch timestamp value to calculate latency
		messageBytes:   13,
		numberOfUsers:  1500,
		numberOfTopics: 1,
		// maxPublishTps is per-user
		maxPublishTps: 1,
		howLongToRun:  time.Minute * 15,
	}

	lgCfg := config.TopicsDefaultWithLogger(
		momento_default_logger.NewDefaultMomentoLoggerFactory(momento_default_logger.DEBUG),
	).WithNumGrpcChannels(16)

	loadGenerator := newLoadGenerator(lgCfg, opts)
	client, cacheClient := loadGenerator.init(ctx)
	defer teardown(ctx, opts.cacheName, cacheClient)

	runStart := time.Now()
	loadGenerator.run(ctx, client)
	runTotal := time.Since(runStart)
	fmt.Printf("completed in %f seconds\n", runTotal.Seconds())
	client.Close()
}
