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
	DB         *sqlx.DB
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

	// init sqlite
	app.DB, err = sqlx.Connect("sqlite", config.DirDB+"/store.db")
	//defer app.DB.Close()

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
		app.SendLogToChannel(0, "mess", "nvenc in container - error")
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

		// long task
		if !(val.Message.Document != nil ||
			strings.Contains(val.Message.Text, "youtube.com") ||
			strings.Contains(val.Message.Text, "youtu.be") ||
			strings.Contains(val.Message.Text, "tiktok.com")) {
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

					log.Warnf("Crash queue: %s", r)
				}
			}(valIn)

			if !a.TaskAllowed(valIn.Message.Chat.ID, translate) {
				return
			}

			var userFromDB User
			_ = a.DB.Get(&userFromDB, "SELECT premium, language_code FROM users WHERE telegram_id = ?",
				valIn.Message.From.ID)

			task := Task{Message: valIn.Message, App: a, UserFromDB: userFromDB, Translate: translate}
			task.Translate.Code = userFromDB.LanguageCode

			// global queue
			a.ChatsWork.IncPlus(valIn.Message.MessageID, valIn.Message.Chat.ID)

			if valIn.Message.Document != nil && valIn.Message.Document.MimeType == "application/x-bittorrent" {
				files := task.DownloadTorrentFiles()
				if files != nil {
					var c = Convert{Task: task, IsTorrent: true}
					if c.CheckExistVideo() {
						task.SendVideos(c.Run())
					} else {
						task.SendTorFiles()
					}
				}

				task.RemoveMessageEdit()
			}

			if strings.Contains(valIn.Message.Text, "youtube.com") ||
				strings.Contains(valIn.Message.Text, "youtu.be") ||
				strings.Contains(valIn.Message.Text, "tiktok.com") {
				file := task.DownloadVideoUrl()
				if file != nil {
					var c = Convert{Task: task, IsTorrent: false}
					if c.CheckExistVideo() {
						task.SendVideos(c.Run())
					}
				}

				task.RemoveMessageEdit()
			}

			if _, bo := task.App.ChatsWork.StopTasks.LoadAndDelete(task.Message.Chat.ID); bo {
				task.Send(tgbotapi.NewMessage(task.Message.Chat.ID, task.Lang("Task stopped")))
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

func (a *App) SendLogToChannel(howId int64, typeSomething string, something ...string) {
	go func(a *App, typeSomething string, something ...string) {
		var UserFromDB User
		_ = a.DB.Get(&UserFromDB, "SELECT telegram_id, name FROM users WHERE telegram_id = ?", howId)

		if typeSomething == "mess" {
			a.Bot.Send(tgbotapi.NewMessage(config.ChatIdChannelLog,
				fmt.Sprintf("%s (%d) %s", UserFromDB.Name, UserFromDB.TelegramId, something[0])))
		}
		if typeSomething == "doc" {
			if len(something) == 2 {
				sendDoc := tgbotapi.NewDocument(config.ChatIdChannelLog, tgbotapi.FileID(something[1]))
				sendDoc.Caption = fmt.Sprintf("%s (%d) %s",
					UserFromDB.Name, UserFromDB.TelegramId, something[0])
				a.Bot.Send(sendDoc)
			}
		}
		if typeSomething == "video" {
			if len(something) == 2 {
				sendVideo := tgbotapi.NewVideo(config.ChatIdChannelLog, tgbotapi.FileID(something[1]))
				sendVideo.Caption = fmt.Sprintf("%s (%d) %s",
					UserFromDB.Name, UserFromDB.TelegramId, something[0])
				a.Bot.Send(sendVideo)
			}
		}
	}(a, typeSomething, something...)
}

func (a *App) InitUser(message *tgbotapi.Message, tr *Translate) {

	user := struct {
		TelegramId int64 `db:"telegram_id"`
	}{}
	_ = a.DB.Get(&user, "SELECT telegram_id FROM users WHERE telegram_id = ?", message.From.ID)

	if user.TelegramId == 0 {
		_, err := a.DB.Exec(`INSERT INTO users (telegram_id, name, date_create, language_code)
							VALUES (?, ?, datetime('now'), ?)`, message.From.ID, message.From.UserName,
			message.From.LanguageCode)
		if err != nil {
			log.Error(err)
		}

		a.SendLogToChannel(message.From.ID, "mess", fmt.Sprintf("new user"))
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

func (a *App) IsBlockUser(fromId int64) bool {
	var userFromDB User
	_ = a.DB.Get(&userFromDB, "SELECT block FROM users WHERE telegram_id = ?",
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

func (a *App) initTables() {
	if _, err := a.DB.Exec(`
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
