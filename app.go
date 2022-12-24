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
	TelegramId int64  `db:"telegram_id"`
	Name       string `db:"name"`
	Premium    int    `db:"premium"`
	Forward    int    `db:"forward"`
}

func Run() App {
	app := App{}

	app.Queue = make(chan QueueMessages, 0)

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
	var cleanerWait sync.WaitGroup
	for val := range a.Queue {
		// lock when files are deleting
		a.LockForRemove.Wait()
		cleanerWait.Add(1)

		go func(valIn QueueMessages) {
			if !a.TaskAllowed(valIn.Message.Chat.ID) {
				return
			}

			var UserFromDB User
			_ = a.DB.Get(&UserFromDB, "SELECT premium, forward FROM users WHERE telegram_id = ?",
				valIn.Message.From.ID)

			task := Task{Message: valIn.Message, App: a, UserFromDB: UserFromDB}

			// global queue
			a.ChatsWork.IncPlus(valIn.Message.MessageID, valIn.Message.Chat.ID)

			if valIn.Message.Document != nil && valIn.Message.Document.MimeType == "application/x-bittorrent" {
				files := task.DownloadTorrentFiles()
				if files != nil {
					var c = Convert{Task: task}
					if c.CheckExistVideo() {
						task.SendVideos(c.Run())

					} else {
						task.SendTorFiles()
					}
				}
			}

			if strings.Contains(valIn.Message.Text, "youtube.com/watch?v=") ||
				strings.Contains(valIn.Message.Text, "youtu.be") ||
				strings.Contains(valIn.Message.Text, "youtube.com/shorts") {
				file := task.DownloadYoutube()
				if file != nil {
					var c = Convert{Task: task}
					if c.CheckExistVideo() {
						task.SendVideos(c.Run())
					}
				}
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

func (a *App) TaskAllowed(chatId int64) bool {
	//if config.IsDev {
	//	return true
	//}
	if _, bo := a.ChatsWork.chat.Load(chatId); bo {
		_, err := a.Bot.Send(tgbotapi.NewMessage(chatId, "❗️ Allowed one task"))
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

		a.SendLogToChannel(message.From.ID, "mess", fmt.Sprintf("new user"))
	}

	video := tgbotapi.NewVideo(message.Chat.ID,
		tgbotapi.FileID(config.WelcomeFileId))
	video.Caption = "Just send me torrent file with the video files 😋"
	a.Bot.Send(video)
	mess := tgbotapi.NewMessage(message.Chat.ID, "Or send me youtube link :3 Example: https://www.youtube.com/watch?v=XqwbqxzsA2g")
	mess.DisableWebPagePreview = true
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
