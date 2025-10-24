package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/segmentio/kafka-go"
)

func main() {
	address := os.Getenv("KAFKA_ADDRESS")

	retention := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "kafka_topic_partition_retention_seconds",
			Help: "Elapsed seconds since the oldest retained message was produced to the Kafka partition.",
		},
		[]string{"topic", "partition"},
	)

	reg := prometheus.NewRegistry()
	reg.MustRegister(retention)

	go func() {
		for {
			func() {
				conn, err := kafka.Dial("tcp", address)
				if err != nil {
					slog.Error("failed to dial", "error", err)
					return
				}
				defer conn.Close()

				partitions, err := conn.ReadPartitions()
				if err != nil {
					slog.Error("failed to read partitions", "error", err)
					return
				}
				for _, partition := range partitions {
					func() {
						conn, err := kafka.DialPartition(context.Background(), "tcp", address, partition)
						if err != nil {
							slog.Error("failed to dial partition", "error", err, "partition", partition.ID, "topic", partition.Topic)
							return
						}
						defer conn.Close()

						first, last, err := conn.ReadOffsets()
						if err != nil {
							slog.Error("failed to read offsets", "error", err, "partition", partition.ID, "topic", partition.Topic)
							return
						}
						if first == last {
							return
						}

						reader := kafka.NewReader(kafka.ReaderConfig{
							Brokers:   []string{address},
							Partition: partition.ID,
							Topic:     partition.Topic,
						})
						defer reader.Close()

						msg, err := reader.FetchMessage(context.Background())
						if err != nil {
							slog.Error("failed to fetch message", "error", err, "partition", partition.ID, "topic", partition.Topic)
							return
						}
						retention.WithLabelValues(partition.Topic, strconv.Itoa(partition.ID)).Set(time.Since(msg.Time).Seconds())
					}()
				}
			}()
			time.Sleep(5 * time.Minute)
		}
	}()

	http.Handle("/", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

	server := &http.Server{Addr: ":8080"}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	go func() {
		<-c
		if err := server.Shutdown(context.Background()); err != nil {
			fmt.Println(err)
		}
	}()

	if err := server.ListenAndServe(); err != nil {
		fmt.Println(err)
	}
}
