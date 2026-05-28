FROM golang:1.26-alpine AS build

RUN go install github.com/hibiken/asynqmon@latest

FROM alpine:3.22

COPY --from=build /go/bin/asynqmon /usr/local/bin/asynqmon

EXPOSE 8080

ENTRYPOINT ["asynqmon"]
