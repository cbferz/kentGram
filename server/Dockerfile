FROM golang:1.23-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY . .
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/kentgram-api ./cmd/api

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata && adduser -D -H -u 10001 app
WORKDIR /app
COPY --from=build /out/kentgram-api /app/kentgram-api
USER app
EXPOSE 8080
ENTRYPOINT ["/app/kentgram-api"]
