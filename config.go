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
	dl, _ := strconv.Atoi(os.Getenv("DOWNLOAD_LIMIT"))
	chatIdChannelLog, _ := strconv.ParseInt(os.Getenv("CHAT_ID_CHANNEL_LOG"), 10, 64)

	config = Struct{
		os.Getenv("DEV") == "true",
		chatIdChannelLog,
		os.Getenv("DIR_BOT"),
		os.Getenv("DIR_DB"),
		dl,
		os.Getenv("WELCOME_VIDEO_ID"),
		os.Getenv("BOT_DEBUG") == "true",
		os.Getenv("BOT_TOKEN"),
		os.Getenv("TG_PATH_LOCAL"),
		"http://" + os.Getenv("TG_API_ENDPOINT") + "/bot%s/%s",
		[]string{".avi", ".mkv", ".mp4", ".m4v", ".flv", ".ts", ".mov", ".wmv", ".webm", ".3gp"},
		2,
		[]string{
			"CAACAgIAAxkBAAIEW2OcfHb7yPa6z59rHlFiTTUTkA3XAAJ-GQACHiDBS43V6msCr8MXKwQ",
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
	if config.IsDev {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}
}
