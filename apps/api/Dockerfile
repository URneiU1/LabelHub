FROM golang:1.26-alpine AS build

WORKDIR /src

RUN apk add --no-cache git

COPY go.work ./
COPY apps/api/go.mod apps/api/go.sum ./apps/api/
COPY apps/ai-worker/go.mod apps/ai-worker/go.sum ./apps/ai-worker/
COPY pkg/exporter/go.mod pkg/exporter/go.sum ./pkg/exporter/
COPY pkg/llmreview/go.mod ./pkg/llmreview/
COPY pkg/reviewsampling/go.mod ./pkg/reviewsampling/

RUN cd apps/api && go mod download
RUN cd apps/ai-worker && go mod download
RUN cd pkg/exporter && go mod download
RUN cd pkg/reviewsampling && go mod download

COPY apps/api ./apps/api
COPY apps/ai-worker ./apps/ai-worker
COPY pkg/exporter ./pkg/exporter
COPY pkg/llmreview ./pkg/llmreview
COPY pkg/reviewsampling ./pkg/reviewsampling

RUN CGO_ENABLED=0 go build -tags nomsgpack -p 1 -o /out/api ./apps/api/cmd/server
RUN CGO_ENABLED=0 go build -p 1 -o /out/worker ./apps/ai-worker/cmd/worker
RUN CGO_ENABLED=0 go build -p 1 -o /out/seed ./apps/api/cmd/seed

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata \
	&& addgroup -S labelhub \
	&& adduser -S labelhub -G labelhub

WORKDIR /app

COPY --from=build /out/api /usr/local/bin/api
COPY --from=build /out/worker /usr/local/bin/worker
COPY --from=build /out/seed /usr/local/bin/seed
COPY --from=build /src/apps/api/internal/migration ./internal/migration

RUN mkdir -p /var/lib/labelhub/exports /var/lib/labelhub/uploads \
	&& chown -R labelhub:labelhub /var/lib/labelhub

USER labelhub

EXPOSE 8080

CMD ["api"]
