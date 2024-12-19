package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"math"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/golang/snappy"
	"github.com/peterbourgon/ff/v4"
	"github.com/peterbourgon/ff/v4/ffhelp"
	dto "github.com/prometheus/client_model/go"
	prom_config "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/util/fmtutil"
	"golang.org/x/exp/rand"
	"google.golang.org/protobuf/proto"
)

const (
	binName        = "faucet_agent"
	timeout        = 15 * time.Second
	initialBackoff = 5 * time.Second
	maxBackoff     = 5 * time.Minute
)

var (
	logLevel    *string
	slogLevel   *slog.LevelVar = new(slog.LevelVar)
	promUrl     *string
	eventSocket *string

	conn net.Conn

	retries int
)

// Print program usage
func printUsage(fs ff.Flags) {
	fmt.Fprintf(os.Stderr, "%s\n", ffhelp.Flags(fs))
	os.Exit(1)
}

// Print program version
func printVersion() {
	fmt.Printf("%s v%s built on %s\n", binName, version.Version, version.BuildDate)
	os.Exit(0)
}

func init() {
	fs := ff.NewFlagSet(binName)
	displayVersion := fs.BoolLong("version", "Print version")
	logLevel = fs.StringEnumLong(
		"log-level",
		"Log level: debug, info, warn, error",
		"info",
		"debug",
		"error",
		"warn",
	)
	promUrl = fs.StringLong(
		"prometheus-remote-write-uri",
		"http://localhost:9090/api/v1/write",
		"Prometheus remote write URI",
	)

	eventSocket = fs.StringLong(
		"event-socket",
		"/run/faucet/event.sock",
		"Path to faucet event socket",
	)

	err := ff.Parse(fs, os.Args[1:],
		ff.WithEnvVarPrefix(strings.ToUpper(binName)),
		ff.WithEnvVarSplit(" "),
	)
	if err != nil {
		printUsage(fs)
	}

	if *displayVersion {
		printVersion()
	}

	switch *logLevel {
	case "debug":
		slogLevel.Set(slog.LevelDebug)
	case "info":
		slogLevel.Set(slog.LevelInfo)
	case "warn":
		slogLevel.Set(slog.LevelWarn)
	case "error":
		slogLevel.Set(slog.LevelError)
	}

	logger := slog.New(
		slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slogLevel,
		}),
	)
	slog.SetDefault(logger)
}

func handleEvent(ctx context.Context, promClient remote.WriteClient, eventString string) {
	var event FaucetEvent
	if err := json.Unmarshal([]byte(eventString), &event); err != nil {
		slog.Error("Failed to parse JSON message", "message", eventString)
	}

	metrics := map[string]*dto.MetricFamily{}

	if event.L3Learn != nil {
		slog.Debug(
			"Received L3 learn event",
			"timestamp",
			time.UnixMilli(int64(event.Time*1000)),
			"dp",
			event.DpName,
			"event",
			event.L3Learn,
		)

		labels := []*dto.LabelPair{
			{
				Name:  proto.String("mac"),
				Value: proto.String(event.L3Learn.EthSrc),
			},
			{
				Name:  proto.String("ip"),
				Value: proto.String(event.L3Learn.L3SrcIP),
			},
			{
				Name:  proto.String("port"),
				Value: proto.String(strconv.Itoa(event.L3Learn.PortNo)),
			},
			{
				Name:  proto.String("vid"),
				Value: proto.String(strconv.Itoa(event.L3Learn.Vid)),
			},
		}

		metrics["faucet_mac_ip_info"] = &dto.MetricFamily{
			Name: proto.String("faucet_mac_ip_info"),
			Type: dto.MetricType_COUNTER.Enum(),
			Metric: []*dto.Metric{
				{
					Label: labels,
					Untyped: &dto.Untyped{
						Value: proto.Float64(1),
					},
					TimestampMs: proto.Int64(int64(event.Time * 1000)),
				},
			},
		}
	}

	writeRequest, err := fmtutil.MetricFamiliesToWriteRequest(
		metrics,
		map[string]string{},
	)
	if err != nil {
		log.Printf("Unable to format write request: %s", err)
	}

	rawRequest, err := writeRequest.Marshal()
	if err != nil {
		log.Printf("Unable to marshal write request: %s", err)
	}

	compressedRequest := snappy.Encode(nil, rawRequest)

	_, err = promClient.Store(ctx, compressedRequest, 0)
	if err != nil {
		log.Printf("Unable to send write request to prometheus: %s", err)
	}
}

func socketConnect(ctx context.Context, socket string, promClient remote.WriteClient) {
	var err error

	conn, err = net.Dial("unix", socket)
	if err != nil {
		slog.Error("Failed to connect to unix socket", "socket", socket, "error", err.Error())

		return
	}

	slog.Info("Connected to unix socket", "socket", socket)

	scanner := bufio.NewScanner(conn)

	for {
		select {
		case <-ctx.Done():
			if conn != nil {
				conn.Close()
			}

			return
		default:
			if scanner.Scan() {
				handleEvent(ctx, promClient, scanner.Text())
				retries = 0
			} else if ctx.Err() != nil {
				return
			} else {
				if err := scanner.Err(); err != nil {
					slog.Error("Error reading from socket", "error", err.Error())
				} else {
					slog.Info("Got EOF from unix socket")
				}

				if conn != nil {
					conn.Close()
				}

				return
			}
		}
	}
}

func main() {
	u, err := url.Parse(*promUrl)
	if err != nil {
		slog.Error(
			"Failed to parse prometheus remote write uri",
			"url",
			*promUrl,
			"error",
			err.Error(),
		)
		os.Exit(1)
	}

	promClient, err := remote.NewWriteClient(binName, &remote.ClientConfig{
		URL:     &prom_config.URL{URL: u},
		Timeout: model.Duration(timeout),
	})
	if err != nil {
		slog.Error("Failed to create prometheus remote write client", "error", err.Error())
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exitSignal := make(chan os.Signal, 1)
	signal.Notify(exitSignal, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-exitSignal
		slog.Info("Cleaning up and exiting")
		cancel()
		if conn != nil {
			conn.Close()
		}
	}()

	retries = 0

	for {
		select {
		case <-ctx.Done():
			return
		default:
			socketConnect(ctx, *eventSocket, promClient)

			if ctx.Err() == nil {
				slog.Info(
					"Waiting before reconnecting to event socket",
					"retries",
					retries,
					"backoff",
					backoff(initialBackoff, maxBackoff, retries),
				)

				backoffDelay(ctx, backoff(initialBackoff, maxBackoff, retries))

				retries++
			}
		}
	}
}

func backoff(initial time.Duration, maximum time.Duration, retries int) time.Duration {
	expo := int(math.Pow(2, float64(retries)))

	half := int(expo / 2)

	random := 0
	if half >= 1 {
		random = rand.Intn(half)
	}

	return min((initial + time.Duration(expo+random)*time.Second), maximum)
}

func backoffDelay(ctx context.Context, delay time.Duration) {
	timer := time.NewTimer(delay)
	select {
	case <-ctx.Done():
		timer.Stop()
	case <-timer.C:
		return
	}
}
