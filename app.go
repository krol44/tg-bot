package main

import (
	"encoding/json"
	"fmt"
	"github.com/anacrolix/torrent"
	"github.com/jmoiron/sqlx"
	"github.com/krol44/telegram-bot-api"
	log "github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
	_ "modernc.org/sqlite"
	"os"
	"sync"
)

type App struct {
	Bot        *tgbotapi.BotAPI
	BotUpdates tgbotapi.UpdatesChannel
	TorClient  *torrent.Client
	DB         *sqlx.DB
	Queue      chan QueueMessages

	ActiveRange   int
	ActiveRangeMu sync.RWMutex

	RangeUser   map[int64]int
	RangeUserMu sync.RWMutex

	LockForRemove sync.WaitGroup
}

type Task struct {
	App                *App
	Message            *tgbotapi.Message
	TorrentProcess     *torrent.Torrent
	TorrentProgress    int64
	TorrentUploaded    int64
	Files              []*torrent.File
	FolderConvert      string
	FileConvertPath    string
	FileConvertPathOut string
	FileCoverPath      string
	FileName           string
	PercentConvert     float64
	MessageEdit        int
	UserFromDB         User
}

type QueueMessages struct {
	Message *tgbotapi.Message
}

type User struct {
	Premium int `db:"premium"`
	Forward int `db:"forward"`
}

func Run() App {
	app := App{}

	app.Queue = make(chan QueueMessages, 100)

	app.RangeUser = make(map[int64]int)

	var err error
	// init bot
	app.Bot, err = tgbotapi.NewBotAPIWithAPIEndpoint(config.BotToken, config.TgApiEndpoint)

	if err != nil {
		log.Panic(err)
		os.Exit(1)
	}
	app.Bot.Debug = config.BotDebug

	log.Printf("Authorized on account %s", app.Bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	app.BotUpdates = app.Bot.GetUpdatesChan(u)

	// init sqlite
	app.DB, err = sqlx.Connect("sqlite", config.DirDB+"/store.db")
	//defer app.DB.Close()

	app.initTables()

	app.initFolders()

	// init torrent
	torrentConfig := torrent.NewDefaultClientConfig()

	torrentConfig.DataDir = config.DirBot + "/torrent-client"
	torrentConfig.NoUpload = true
	torrentConfig.DownloadRateLimiter = rate.NewLimiter(rate.Limit(config.DownloadLimit), config.DownloadLimit)

	app.TorClient, err = torrent.NewClient(torrentConfig)
	//defer app.TorClient.Close()

	if err != nil {
		log.Panic(err)
		os.Exit(1)
	}

	return app
}

func (a *App) ObserverQueue() {
	go a.queueRange()
}

func (a *App) queueRange() {
	var cleanerWait sync.WaitGroup
	for val := range a.Queue {
		a.LockForRemove.Wait()
		cleanerWait.Add(1)

		go func(valIn QueueMessages) {
			var UserFromDB User
			_ = a.DB.Get(&UserFromDB, "SELECT premium, forward FROM users WHERE telegram_id = ?",
				valIn.Message.From.ID)

			task := Task{Message: valIn.Message, App: a, UserFromDB: UserFromDB}

			next := task.DownloadTorrentFiles()

			if next {
				if task.CheckExistVideo() {
					task.SendVideos()
				} else {
					task.SendFiles()
				}
			}

			// user queue
			a.RangeUserMu.Lock()
			a.RangeUser[task.Message.From.ID]--
			a.RangeUserMu.Unlock()

			// global queue
			a.ActiveRangeMu.Lock()
			a.ActiveRange--
			a.ActiveRangeMu.Unlock()

			cleanerWait.Done()
			cleanerWait.Wait()
			task.Cleaner()
		}(val)
	}

}

func (a *App) SendLogToChannel(typeSomething string, something ...string) {
	go func(a *App, typeSomething string, something ...string) {
		if typeSomething == "mess" {
			a.Bot.Send(tgbotapi.NewMessage(config.ChatIdChannelLog, something[0]))
		}
		if typeSomething == "doc" {
			if len(something) == 2 {
				sendDoc := tgbotapi.NewDocument(config.ChatIdChannelLog, tgbotapi.FileID(something[1]))
				sendDoc.Caption = something[0]
				a.Bot.Send(sendDoc)
			}
		}
		if typeSomething == "video" {
			if len(something) == 2 {
				sendVideo := tgbotapi.NewVideo(config.ChatIdChannelLog, tgbotapi.FileID(something[1]))
				sendVideo.Caption = something[0]
				a.Bot.Send(sendVideo)
			}
		}
	}(a, typeSomething, something...)
}

func (a *App) InitUser(message *tgbotapi.Message) {

	user := struct {
		TelegramId int64 `db:"telegram_id"`
	}{}
	_ = a.DB.Get(&user, "SELECT telegram_id FROM users WHERE telegram_id = ?", message.From.ID)

	if user.TelegramId == 0 {
		_, err := a.DB.Exec(`INSERT INTO users (telegram_id, name, date_create)
							VALUES (?, ?, datetime('now'))`, message.From.ID, message.From.UserName)
		if err != nil {
			log.Error(err)
		}

		_, err = a.DB.Exec(`INSERT INTO chats (chat_id, date_create)
							VALUES (?, datetime('now'))`, message.Chat.ID)
		if err != nil {
			log.Error(err)
		}

		a.SendLogToChannel("mess", fmt.Sprintf("@%s (%d) - new user",
			message.From.UserName, message.From.ID))
	}

	mess := tgbotapi.NewVideo(message.Chat.ID,
		tgbotapi.FileID("BAACAgIAAxkBAAOJY51_mtI3lFa51TYo0wI6wsF6l6sAAlwmAAK_relIYD7N9rxwfS4rBA"))
	mess.Caption = "Just send me torrent file with the video files 😋"
	a.Bot.Send(mess)
}

func (a *App) Logs(message *tgbotapi.Message) {
	marshal, err := json.Marshal(message)
	if err != nil {
		return
	}
	_, err = a.DB.Exec(`INSERT INTO logs (json, date_create)
							VALUES (?, datetime('now'))`, string(marshal))
	if err != nil {
		log.Error(err)
	}
}

func (a *App) initFolders() {
	err := os.Mkdir(config.DirBot+"/torrent-client", os.ModePerm)
	if err != nil {
		log.Info(err)
	}

	err = os.Mkdir(config.DirBot+"/storage", os.ModePerm)
	if err != nil {
		log.Info(err)
	}
}

func (a *App) initTables() {
	if _, err := a.DB.Exec(`
				create table if not exists users
(
    telegram_id BIGINT,
    name		text,
    date_create text,
    premium     int default 0,
    forward     int default 0
);

create unique index if not exists users_telegram_id_uindex
    on users (telegram_id);

create table if not exists logs
(
    id integer
        constraint logs_pk
            primary key autoincrement,
    json             text,
    date_create 	 text
);
create table if not exists chats
(
    chat_id BIGINT,
    date_create 	 text
);
create unique index if not exists chats_chat_id_uindex
    on chats (chat_id);
				`); err != nil {
		log.Error(err)
	}
}
