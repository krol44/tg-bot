FROM golang:1.18.2-buster as builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -a -installsuffix cgo -o tor-purr-bot .

FROM alpine:latest
RUN apk add ffmpeg tzdata
COPY --from=builder /app/tor-purr-bot .
CMD ["./tor-purr-bot"]