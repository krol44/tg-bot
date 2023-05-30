## TgBot - Telegram bot
A Very Tiny BitTorrent Client, YouTube, TikTok downloader 🔥

### Support
Torrent, youtube.com, tiktok.com,
vk.com/video, twitch.tv/videos,
twitch.tv/*****/clip, rutube.ru/video, coub.com/view,
open.spotify.com/track, open.spotify.com/album and etc

### Demo
https://t.me/TorPurrBot

### Setup

1. cd /home
2. git clone https://github.com/krol44/tg-bot
3. cd /home/tor-purr-bot && mkdir storage
4. mkdir storage/telegram-bot-api-data && mkdir storage/bot-data && mkdir storage/postgres-data
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
POSTGRES_PASSWORD=demo_pass
CHAT_ID_CHANNEL_LOG=-000000
DOWNLOAD_LIMIT=30000000
WELCOME_VIDEO_ID=BAACAgIAA.....
```
6. nano dc-without-vpn.yml
```
if you have Nvidia NVENC, you may uncomment
runtime: nvidia
```
7. ./build-without-vpn

### Info
```
TELEGRAM_API_ID, TELEGRAM_API_HASH
get tg api key - https://my.telegram.org/auth?to=apps
```

```
BOT_TOKEN - get from @BotFather
POSTGRES_PASSWORD - change password for postgresSQL
CHAT_ID_CHANNEL_LOG - for logs, who uses the bot
DOWNLOAD_LIMIT - speed download torrent
WELCOME_VIDEO_ID - tg file id, video hello when used command /start
```
