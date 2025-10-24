package main

import (
	"context"
	"fmt"
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
					fmt.Println(err)
					return
				}
				defer conn.Close()

				partitions, err := conn.ReadPartitions()
				if err != nil {
					fmt.Println(err)
					return
				}
				for _, partition := range partitions {
					func() {
						reader := kafka.NewReader(kafka.ReaderConfig{
							Brokers:   []string{address},
							Partition: partition.ID,
							Topic:     partition.Topic,
						})
						defer reader.Close()

						ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
						defer cancel()
						msg, err := reader.FetchMessage(ctx)
						if err != nil {
							fmt.Println(err)
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
