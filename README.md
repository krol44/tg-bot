## TorPurrBot - Telegram bot
A Very Tiny BitTorrent Client, YouTube, TikTok downloader and Media Player 🔥

### Demo
https://t.me/TorPurrBot

### Setup

1. cd /home
2. git clone https://github.com/krol44/tor-purr-bot
3. cd /home/tor-purr-bot && mkdir storage
4. mkdir storage/telegram-bot-api-data && mkdir storage/bot-data && mkdir storage/bot-db
5. nano .env.secrets
```
COMPOSE_PROJECT_NAME=tor-purr-bot
DEV=false
BOT_DEBUG=false
STORAGE_PATH=/home/tor-purr-bot
TELEGRAM_STAT=1
TELEGRAM_API_ID=0000000
TELEGRAM_API_HASH=aaaaa...
BOT_TOKEN=00000:AAAAAA....
CHAT_ID_CHANNEL_LOG=-000000
DOWNLOAD_LIMIT=30000000
WELCOME_VIDEO_ID=BAACAgIAA.....
```
6. nano dc-without-vpn.yml
```
if you not have Nvidia NVENC, you must delete or comment
#runtime: nvidia
```
7. ./build-without-vpn

### Info
```
TELEGRAM_API_ID, TELEGRAM_API_HASH
get tg api key - https://my.telegram.org/auth?to=apps
```

```
BOT_TOKEN - get from @BotFather
CHAT_ID_CHANNEL_LOG - for logs, who uses the bot
DOWNLOAD_LIMIT - speed download torrent
WELCOME_VIDEO_ID - tg file id, video hello when used command /start
```

```
# web admin sqlite

touch docker-sqlite-client.sh && chmod +x docker-sqlite-client.sh && nano docker-sqlite-client.sh

#!/bin/bash
default_port="8012"
port=${2:-$default_port}

echo "Starting on: http://127.0.0.1:$port"
docker run -d -it --rm \
    -p "$port:8080" \
    -v /home/tor-purr-bot/bot-db:/data \
    -e SQLITE_DATABASE="store.db" \
    coleifer/sqlite-web
```