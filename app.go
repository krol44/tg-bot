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
	"strings"
	"sync"
)

type App struct {
	Bot        *tgbotapi.BotAPI
	BotUpdates tgbotapi.UpdatesChannel
	TorClient  *torrent.Client
	Queue      chan QueueMessages

	ChatsWork     ChatsWork
	LockForRemove sync.WaitGroup
}

type QueueMessages struct {
	Message *tgbotapi.Message
}

type User struct {
	TelegramId   int64  `db:"telegram_id"`
	Name         string `db:"name"`
	Premium      int    `db:"premium"`
	Block        int    `db:"block"`
	LanguageCode string `db:"language_code"`
}

func Run() App {
	app := App{}

	// create queue
	app.Queue = make(chan QueueMessages, 0)

	// create lock turn
	app.ChatsWork = ChatsWork{m: sync.Map{}}

	var err error
	// init bot
	app.Bot, err = tgbotapi.NewBotAPIWithAPIEndpoint(config.BotToken, config.TgApiEndpoint)

	if err != nil {
		log.Panic(err)
		os.Exit(1)
	}
	app.Bot.Debug = config.BotDebug

	log.Infof("Authorized on account %s", app.Bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	app.BotUpdates = app.Bot.GetUpdatesChan(u)

	// create table is not exist
	app.initTables()

	// create folders if not exist
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

	// check nvenc
	ch := Convert{}.healthNvenc()
	if !ch {
		app.SendLogToChannel(&tgbotapi.User{UserName: "debug"},
			"mess", "nvenc in container - error")
	}

	return app
}

func (a *App) ObserverQueue() {
	var cleanerWait sync.WaitGroup
	for val := range a.Queue {
		// set language
		translate := &Translate{Code: val.Message.From.LanguageCode}

		// commands
		if val.Message.Text == "/start" || val.Message.Text == "/info" {
			a.InitUser(val.Message, translate)
			continue
		}
		if val.Message.Text == "/support" {
			a.Bot.Send(tgbotapi.NewMessage(val.Message.Chat.ID, translate.Lang("Write a message right here")))
			continue
		}
		if val.Message.Text == "/stop" {
			a.ChatsWork.StopTasks.Store(val.Message.Chat.ID, true)
			continue
		}

		// find yt in entities
		for _, en := range val.Message.Entities {
			if en.IsTextLink() {
				val.Message.Text = en.URL
			}
		}

		// long task
		torrentProcess, existTorProc := a.ChatsWork.TorrentProcesses.Load(val.Message.Text)
		if !(val.Message.Document != nil ||
			strings.Contains(val.Message.Text, "youtube.com") ||
			strings.Contains(val.Message.Text, "youtu.be") ||
			strings.Contains(val.Message.Text, "tiktok.com") ||
			strings.Contains(val.Message.Text, "magnet:?xt=") ||
			existTorProc) {
			continue
		}

		// lock when files are deleting
		a.LockForRemove.Wait()
		cleanerWait.Add(1)

		go func(valIn QueueMessages) {
			// if fatal, execute cleaning
			defer func(valIn QueueMessages) {
				if r := recover(); r != nil {
					// global queue
					a.ChatsWork.IncMinus(valIn.Message.MessageID, valIn.Message.Chat.ID)

					cleanerWait.Done()
					cleanerWait.Wait()

					log.Error("Crash queue: %s", r)
				}
			}(valIn)

			if !a.TaskAllowed(valIn.Message.Chat.ID, translate) {
				return
			}

			db := Sqlite()
			var userFromDB User
			_ = db.Get(&userFromDB, "SELECT premium, language_code FROM users WHERE telegram_id = ?",
				valIn.Message.From.ID)
			db.Close()

			task := Task{Message: valIn.Message, App: a, UserFromDB: userFromDB, Translate: translate}
			task.Translate.Code = userFromDB.LanguageCode

			// global queue
			a.ChatsWork.IncPlus(valIn.Message.MessageID, valIn.Message.Chat.ID)

			if (valIn.Message.Document != nil && valIn.Message.Document.MimeType == "application/x-bittorrent") ||
				strings.Contains(valIn.Message.Text, "magnet:?xt=") {
				task.OpenKeyBoardWithTorrentFiles()
			}

			if existTorProc {
				task.CloseKeyBoardWithTorrentFiles()

				if task.DownloadTorrentFile(torrentProcess.(*torrent.Torrent)) {
					var c = Convert{Task: task, IsTorrent: true}
					if c.CheckExistVideo() {
						task.SendVideo(c.Run())
					} else {
						if false { //if userFromDB.Premium == 0 {
							task.Send(tgbotapi.NewMessage(task.Message.Chat.ID,
								fmt.Sprintf("❗️ "+task.Lang("Available only for users who support us"))))
						} else {
							task.SendTorFile()
						}
					}
				}

				task.RemoveMessageEdit()
			}

			if strings.Contains(valIn.Message.Text, "youtube.com") ||
				strings.Contains(valIn.Message.Text, "youtu.be") ||
				strings.Contains(valIn.Message.Text, "tiktok.com") {

				if task.DownloadVideoUrl() {
					var c = Convert{Task: task, IsTorrent: false}
					if c.CheckExistVideo() {
						task.SendVideo(c.Run())
					}
				}

				task.RemoveMessageEdit()
			}

			if _, bo := task.App.ChatsWork.StopTasks.LoadAndDelete(task.Message.Chat.ID); bo {
				task.Send(tgbotapi.NewMessage(task.Message.Chat.ID, "❗️ "+task.Lang("Task stopped")))
			}

			// global queue
			a.ChatsWork.IncMinus(valIn.Message.MessageID, valIn.Message.Chat.ID)

			// lock when files are deleting
			cleanerWait.Done()
			cleanerWait.Wait()
			task.Cleaner()
		}(val)
	}
}

func (a *App) TaskAllowed(chatId int64, tr *Translate) bool {
	//if config.IsDev {
	//	return true
	//}
	if _, bo := a.ChatsWork.chat.Load(chatId); bo {
		_, err := a.Bot.Send(tgbotapi.NewMessage(chatId, "❗️ "+tr.Lang("Allowed only one task")))
		if err != nil {
			log.Error(err)
		}
		return false
	}

	return true
}

func (a *App) SendLogToChannel(messFrom *tgbotapi.User, typeSomething string, something ...string) {
	go func(a *App, messFrom *tgbotapi.User, typeSomething string, something ...string) {
		if typeSomething == "mess" {
			a.Bot.Send(tgbotapi.NewMessage(config.ChatIdChannelLog,
				fmt.Sprintf("%s (%d) %s", messFrom.UserName, messFrom.ID, something[0])))
		}
		if typeSomething == "doc" {
			if len(something) == 2 {
				sendDoc := tgbotapi.NewDocument(config.ChatIdChannelLog, tgbotapi.FileID(something[1]))
				sendDoc.Caption = fmt.Sprintf("%s (%d) %s",
					messFrom.UserName, messFrom.ID, something[0])
				a.Bot.Send(sendDoc)
			}
		}
		if typeSomething == "video" {
			if len(something) == 2 {
				sendVideo := tgbotapi.NewVideo(config.ChatIdChannelLog, tgbotapi.FileID(something[1]))
				sendVideo.Caption = fmt.Sprintf("%s (%d) %s",
					messFrom.UserName, messFrom.ID, something[0])
				a.Bot.Send(sendVideo)
			}
		}
	}(a, messFrom, typeSomething, something...)
}

func (a *App) InitUser(message *tgbotapi.Message, tr *Translate) {
	db := Sqlite()
	defer db.Close()

	user := struct {
		TelegramId int64 `db:"telegram_id"`
	}{}
	_ = db.Get(&user, "SELECT telegram_id FROM users WHERE telegram_id = ?", message.From.ID)

	if user.TelegramId == 0 {
		_, err := db.Exec(`INSERT INTO users (telegram_id, name, date_create, language_code)
							VALUES (?, ?, datetime('now'), ?)`, message.From.ID, message.From.UserName,
			message.From.LanguageCode)
		if err != nil {
			log.Error(err)
		}

		a.SendLogToChannel(message.From, "mess", fmt.Sprintf("new user"))
	}

	video := tgbotapi.NewVideo(message.Chat.ID,
		tgbotapi.FileID(config.WelcomeFileId))
	video.Caption = tr.Lang("Just send me torrent file with the video files") + " 😋"
	a.Bot.Send(video)
	mess := tgbotapi.NewMessage(message.Chat.ID, tr.Lang("Or send me youtube, tiktok url")+" :3\n"+
		tr.Lang("Example")+": https://www.youtube.com/watch?v=XqwbqxzsA2g\n"+
		tr.Lang("Example")+": https://vt.tiktok.com/ZS8jY2NVd")
	mess.DisableWebPagePreview = true
	a.Bot.Send(mess)
}

func (a *App) Logs(message any) {
	db := Sqlite()
	defer db.Close()

	marshal, err := json.Marshal(message)
	if err != nil {
		return
	}
	_, err = db.Exec(`INSERT INTO logs (json, date_create)
							VALUES (?, datetime('now'))`, string(marshal))
	if err != nil {
		log.Error(err)
	}
}

func (a *App) IsBlockUser(fromId int64) bool {
	db := Sqlite()
	defer db.Close()

	var userFromDB User
	_ = db.Get(&userFromDB, "SELECT block FROM users WHERE telegram_id = ?",
		fromId)

	if userFromDB.Block == 1 {
		return true
	}

	return false
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

func Sqlite() *sqlx.DB {
	conn, err := sqlx.Connect("sqlite", config.DirDB+"/store.db")
	if err != nil {
		log.Error(err)
	}
	return conn
}

func (a *App) initTables() {
	db := Sqlite()
	defer db.Close()

	if _, err := db.Exec(`
create table if not exists users
(
  telegram_id   BIGINT,
  name          TEXT,
  date_create   TEXT,
  premium       INT     default 0,
  block         INT     default 0,
  language_code VARCHAR(10) default 'en'
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

create table if not exists cache
(
    id integer
        constraint cache_pk
            primary key autoincrement,
    caption				text,
    native_path_file	text,
    native_md5_sum		text,
    video_url_id		text,
    tg_from_id			text,
    tg_file_id			text,
    tg_file_size		int,
    date_create			text
);
create index if not exists cache_native_path_file_index
    on cache (native_path_file);
create index if not exists cache_native_md5_sum_index
    on cache (native_md5_sum);
create index if not exists cache_video_url_id_index
    on cache (video_url_id);
				`); err != nil {
		log.Error(err)
	}
}
