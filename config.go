package main

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"os"
	"path"
	"runtime"
	"strconv"
)

type Struct struct {
	IsDev bool

	ChatIdChannelLog int64

	DirBot        string
	DirDB         string
	DownloadLimit int

	WelcomeFileId string

	BotDebug      bool
	BotToken      string
	TgPathLocal   string
	TgApiEndpoint string

	AllowVideoFormats []string

	MaxTasks int

	CuteStickers []string
}

var config Struct

func init() {
	dev := os.Getenv("dev") == "true"
	dl, _ := strconv.Atoi(os.Getenv("download_limit"))
	chatIdChannelLog, _ := strconv.ParseInt(os.Getenv("chat_id_channel_log"), 10, 64)
	botDebug := os.Getenv("bot_debug") == "true"

	config = Struct{
		dev,
		chatIdChannelLog,
		os.Getenv("dir_bot"),
		os.Getenv("dir_db"),
		dl,
		os.Getenv("welcome_video_id"),
		botDebug,
		os.Getenv("bot_token"),
		os.Getenv("tg_path_local"),
		"http://" + os.Getenv("tg_api_endpoint") + "/bot%s/%s",
		[]string{".avi", ".mkv", ".mp4", ".MP4", ".m4v", ".flv", ".TS", ".ts", ".mov", ".wmv", ".webm"},
		2,
		[]string{
			"CAACAgIAAxkBAAIEW2OcfHb7yPa6z59rHlFiTTUTkA3XAAJ-GQACHiDBS43V6msCr8MXKwQ",
			"CAACAgIAAxkBAAIRe2OreICfsmmDoEHCgcsn06OdRcplAALMGgACZd54S0GizHhUHVFiKwQ",
			"CAACAgIAAxkBAAIRfWOreMzwPkQDC4jYKGUTeCxNO3TuAAJ3GAAC24IRSEjXhoRmKkUtKwQ",
			"CAACAgIAAxkBAAIRfmOrePNC-c3sDM95Ixi2awl1O2j1AAKGGgACrYHBS0S6i4Mlw7dfKwQ",
			"CAACAgIAAxkBAAIRf2OreSqRlT1RsFG2kR94YPZ_RV44AAIxHAACG_9pSSKkRFrvOqSWKwQ",
		},
	}

	logSetup()
}

func logSetup() {
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
		CallerPrettyfier: func(f *runtime.Frame) (string, string) {
			filename := path.Base(f.File)
			return fmt.Sprintf("%s()", f.Function), fmt.Sprintf(" %s:%d", filename, f.Line)
		},
	})
	if l, err := log.ParseLevel("debug"); err == nil {
		log.SetLevel(l)
		log.SetReportCaller(l == log.DebugLevel)
		log.SetOutput(os.Stdout)
	}
	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel)
}
