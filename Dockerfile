FROM golang:1.18.2-buster as builder
WORKDIR /app
COPY go.* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o tor-purr-bot tor-purr-bot

FROM debian:buster-slim
RUN apt update
RUN apt install -y ffmpeg tzdata python3 python3-venv python3-pip
RUN python3 -m pip install -U yt-dlp spotdl
COPY ./ffmpeg .
COPY --from=builder /app/tor-purr-bot .
CMD ["./tor-purr-bot"]