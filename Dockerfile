# syntax=docker/dockerfile:1

FROM docker.m.daocloud.io/library/node:22-bookworm-slim AS dashboard_remote_builder

WORKDIR /mailbox/webui
RUN sed -i 's/deb.debian.org/mirrors.ustc.edu.cn/g' /etc/apt/sources.list.d/debian.sources     && apt-get update     && apt-get install -y --no-install-recommends libprotobuf-dev protobuf-compiler     && rm -rf /var/lib/apt/lists/*
COPY common-lib/ui /common-lib/ui
COPY common-lib/proto /common-lib/proto
COPY common-lib/scripts /common-lib/scripts
COPY mailbox/proto /mailbox/proto
COPY mailbox/webui ./
RUN npm ci && SOURCE_ROOT=/ npm run build

FROM docker.m.daocloud.io/library/golang:1.26-alpine AS builder

WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct
ENV PATH=/root/go/bin:$PATH

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk add --no-cache git protobuf-dev \
    && go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11 \
    && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1

COPY common-lib /common-lib
COPY mailbox/services/mailbox-api/go.mod mailbox/services/mailbox-api/go.sum* ./
RUN go mod edit -replace github.com/byte-v-forge/common-lib=/common-lib \
    && go mod download

COPY mailbox/proto ./proto
RUN mkdir -p /generated/pb \
    && protoc -I proto -I /common-lib/proto --go_out=/generated/pb --go-grpc_out=/generated/pb \
      proto/email.proto \
      proto/mailbox_register.proto \
      proto/mailbox_service.proto

COPY mailbox/services/mailbox-api ./
RUN --mount=type=cache,target=/root/.cache/go-build \
    rm -rf pb \
    && cp -R /generated/pb ./pb \
    && go build -o /out/mailbox .

FROM docker.m.daocloud.io/library/alpine:latest

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=builder /out/mailbox /app/bin/mailbox
COPY --from=dashboard_remote_builder /mailbox/webui/dist /app/dashboard/mailbox

EXPOSE 50051 8080 8082
CMD ["/app/bin/mailbox"]
