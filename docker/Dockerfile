FROM golang:1.17-alpine AS builder
ARG GITLAB_USER
ARG GITLAB_ACCESS_TOKEN
ARG BUILD_MODULE
ENV GOPRIVATE=gitlab.com
RUN apk add --no-cache git
RUN git config --global url."https://$GITLAB_USER:$GITLAB_ACCESS_TOKEN@gitlab.com/".insteadOf "https://gitlab.com/"
COPY . /go/src
WORKDIR /go/src
RUN env GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o ./binary

FROM alpine:3.15
COPY --from=builder /go/src/config /app/config
COPY --from=builder /go/src/binary /app
EXPOSE 8080
WORKDIR /app
ENTRYPOINT ["./binary"]
