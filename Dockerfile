FROM public.ecr.aws/docker/library/golang:1.25.2 AS builder

WORKDIR /usr/src/app

# pre-copy/cache go.mod for pre-downloading dependencies and only redownloading them in subsequent builds if they change
COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN go build -o /usr/local/bin/app .


FROM public.ecr.aws/docker/library/debian:bookworm-slim

RUN groupadd --gid 1000 go \
  && useradd --uid 1000 --gid go --shell /bin/bash --create-home go

WORKDIR /home/go/app

COPY --from=builder /usr/local/bin/app kafka_metrics_server

USER go

ENTRYPOINT ["./kafka_metrics_server"]
