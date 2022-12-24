FROM golang:1.18.2-buster as builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN go build -buildvcs=false -o tor-purr-bot .

FROM debian:buster-slim
RUN apt update
RUN apt install -y ffmpeg tzdata python3 python3-venv python3-pip
RUN python3 -m pip install -U yt-dlp
COPY ./ffmpeg .
COPY --from=builder /app/tor-purr-bot .
CMD ["./tor-purr-bot"]