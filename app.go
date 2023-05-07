package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/anacrolix/torrent"
	"github.com/jmoiron/sqlx"
	"github.com/krol44/telegram-bot-api"
	_ "github.com/lib/pq"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
	"os"
	"strings"
	"sync"
	"time"
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
		if strings.Contains(val.Message.Text, "/start") {
			a.InitUser(val.Message, translate)

			sp := strings.Split(val.Message.Text, " ")
			if len(sp) >= 2 && sp[1] != "" {
				var data struct {
					Url string `db:"url"`
				}

				err := Postgres.Get(&data, `SELECT url FROM links WHERE md5_url = $1`, sp[1])
				if err != nil && err != sql.ErrNoRows {
					log.Error(err)
					continue
				}
				if data.Url != "" {
					val.Message.Text = data.Url
					a.Bot.Send(tgbotapi.NewMessage(val.Message.Chat.ID, data.Url))
				}
			}
		}
		if val.Message.Text == "/info" {
			a.WelcomeMessage(val.Message, translate)
		}
		if val.Message.Text == "/support" {
			a.Bot.Send(tgbotapi.NewMessage(val.Message.Chat.ID,
				translate.Lang("What is happened? Write me right here")))
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
		torrentProcess, _ := a.ChatsWork.TorrentProcesses.Load(val.Message.Text)
		if !(val.Message.Document != nil ||
			strings.HasPrefix(val.Message.Text, "https://") ||
			strings.Contains(val.Message.Text, "magnet:?xt=") ||
			torrentProcess != nil) {
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

					log.Infof("%+v", errors.WithStack(errors.New("Stacktrace")))
					log.Errorf("Crash queue: %s", r)
				}
			}(valIn)

			if !a.TaskAllowed(valIn.Message.Chat.ID, translate) {
				return
			}

			var userFromDB User
			_ = Postgres.Get(&userFromDB, "SELECT premium, language_code FROM users WHERE telegram_id = $1",
				valIn.Message.From.ID)

			task := Task{Message: valIn.Message, App: a, UserFromDB: userFromDB, Translate: translate}
			task.Translate.Code = userFromDB.LanguageCode

			if (valIn.Message.Document != nil && valIn.Message.Document.MimeType == "application/x-bittorrent") ||
				strings.Contains(valIn.Message.Text, "magnet:?xt=") {
				if tp := task.OpenKeyBoardWithTorrentFiles(); tp != nil {
					torrentProcess = tp
				}
			}

			// global queue
			a.ChatsWork.IncPlus(valIn.Message.MessageID, valIn.Message.Chat.ID)

			if torrentProcess != nil {
				task.CloseKeyBoardWithTorrentFiles()
				t := &ObjectTorrent{
					Task:           &task,
					TorrentProcess: torrentProcess.(*torrent.Torrent),
				}
				task.Run(t)

				task.RemoveMessageEdit()
			}

			if strings.HasPrefix(valIn.Message.Text, "https://") {
				if strings.Contains(valIn.Message.Text, "https://open.spotify.com/track/") ||
					strings.Contains(valIn.Message.Text, "https://open.spotify.com/album") {
					ym := &ObjectSpotify{
						Task: &task,
					}
					task.Run(ym)
				} else {
					vuc := &ObjectVideoUrl{
						Task: &task,
					}
					task.Run(vuc)
				}
			}

			if _, bo := task.App.ChatsWork.StopTasks.LoadAndDelete(task.Message.Chat.ID); bo {
				task.Send(tgbotapi.NewMessage(task.Message.Chat.ID, "❗️ "+task.Lang("Task stopped")))
			}

			// global queue
			a.ChatsWork.IncMinus(valIn.Message.MessageID, valIn.Message.Chat.ID)

			// send ad
			go a.SendAd(valIn.Message)

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

func (a *App) SendAd(mess *tgbotapi.Message) {
	time.Sleep(time.Minute * 30)

	var userFromDB User
	_ = Postgres.Get(&userFromDB, "SELECT sent_ad FROM users WHERE telegram_id = $1", mess.From.ID)
	if userFromDB.SentAd == 1 {
		return
	}

	tr := &Translate{Code: mess.From.LanguageCode}

	_, err := a.Bot.Send(tgbotapi.NewMessage(mess.Chat.ID,
		fmt.Sprintf(tr.Lang(
			"I am so appreciative of you for using bot! %s Please,"+
				" share below a message with your friends. Thank you!"), "❤️")))
	if err != nil {
		log.Warn(err)
	}

	_, err = a.Bot.Send(tgbotapi.NewMessage(mess.Chat.ID, signAdvt+"\n\nhttps://t.me/"+a.Bot.Self.UserName))
	if err != nil {
		log.Warn(err)
	}

	_, err = Postgres.Exec("UPDATE users SET sent_ad = 1 WHERE telegram_id = $1", mess.From.ID)
	if err != nil {
		log.Error(err)
	}

	a.SendLogToChannel(mess.From, "mess", "send ad")
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
	user := struct {
		TelegramId int64 `db:"telegram_id"`
	}{}
	_ = Postgres.Get(&user, "SELECT telegram_id FROM users WHERE telegram_id = $1", message.From.ID)

	if user.TelegramId == 0 {
		_, err := Postgres.Exec(`INSERT INTO users (telegram_id, name, date_create, language_code)
							VALUES ($1, $2, $3, $4)`, message.From.ID, message.From.UserName, time.Now(),
			message.From.LanguageCode)
		if err != nil {
			log.Error(err)
		}

		a.SendLogToChannel(message.From, "mess", fmt.Sprintf("new user"))

		a.WelcomeMessage(message, tr)
	}
}

func (a *App) WelcomeMessage(message *tgbotapi.Message, tr *Translate) {
	video := tgbotapi.NewVideo(message.Chat.ID,
		tgbotapi.FileID(config.WelcomeFileId))
	video.Caption = tr.Lang("Just send me torrent file with the video files or files") + " 😋 \n\n" +
		tr.Lang("And also you can send me") + " `magnet:?xt=` 🔗 magnet link"
	a.Bot.Send(video)

	preMess := tr.Lang("Or send me YouTube, TikTok url, examples below") + " 🫡\n\n" +
		tr.Lang("Example") + " YouTube:\n https://www.youtube.com/watch?v=XqwbqxzsA2g\n" +
		tr.Lang("Example") + " TikTok:\n https://vt.tiktok.com/ZS8EYpxHP\n" +
		tr.Lang("Example") + " VK Video:\n https://vk.com/video-118281792_456242739\n" +
		tr.Lang("Example") + " Twitch Clip: https://www.twitch.tv/guhrl/clip/CrowdedCrowdedClintCharlietheUnicor" +
		"n-igG_XEcFiBw2KoVX\n" +
		tr.Lang("Example") + " RuTube Video:\n https://rutube.ru/video/37b5e31d214ee0496e380a028c279c36\n" +
		tr.Lang("Example") + " Spotify track:\n https://open.spotify.com/track/1hEh8Hc9lBAFWUghHBsCel\n" +
		tr.Lang("Example") + " Spotify album:\n https://open.spotify.com/album/1YxUJdI0JWsXGGq8xa1SLt\n"

	var userFromDB User
	_ = Postgres.Get(&userFromDB, "SELECT premium, language_code FROM users WHERE telegram_id = $1",
		message.From.ID)
	if userFromDB.Premium == 1 {
		preMess += "\n\nPremium flags 🤫\n\n" +
			"skip cache id\n   +skip-cache-id\n" +
			"get max quality\n   +quality\n" +
			"slice video, format hh:mm:ss\n   -ss 00:01:00 -to 00:10:00\n\n" +
			"Example: https://video.url +quality +..."
	}

	mess := tgbotapi.NewMessage(message.Chat.ID, preMess)
	mess.DisableWebPagePreview = true
	a.Bot.Send(mess)
}

func (a *App) Logs(message any) {
	marshal, err := json.Marshal(message)
	if err != nil {
		return
	}
	_, err = Postgres.Exec(`INSERT INTO logs (json, date_create) VALUES ($1, $2)`, string(marshal), time.Now())
	if err != nil {
		log.Error(err)
	}
}

func (a *App) IsBlockUser(fromId int64) bool {
	var userFromDB User
	_ = Postgres.Get(&userFromDB, "SELECT block FROM users WHERE telegram_id = $1", fromId)

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

func PostgresConnect() *sqlx.DB {
	connect, err := sqlx.Connect("postgres", os.Getenv("POSTGRES_CONNECT"))
	if err != nil {
		log.Fatal(err)
	}

	connect.SetConnMaxLifetime(time.Minute * 3)
	connect.SetMaxOpenConns(50)
	connect.SetMaxIdleConns(50)

	return connect
}

func (a *App) initTables() {
	if _, err := Postgres.Exec(`
create table if not exists users
(
    telegram_id      bigint  		not null,
    date_create      timestamp  	not null,
    name			 text    		default '' not null,
    premium		     int     		default 0 not null,
  	sent_ad       	 int     		default 0 not null,
  	block            int     		default 0 not null,
	block_why 	     text 	    	default '' not null,
	language_code    varchar(10)	default 'en'
);
create unique index if not exists users_telegram_id_uindex
    on users (telegram_id);

create table if not exists logs
(
	id      serial
        	constraint logs_pk
            primary key,
    json             text 		default '' not null,
    date_create 	 timestamp 	not null
);

create table if not exists cache
(
	id      serial
        	constraint cache_pk
            primary key,
    caption				text 		default '' not null,
    native_path_file	text 		default '' not null,
    native_md5_sum		text 		default '' not null,
    video_url_id		text 		default '' not null,
    tg_from_id			text 		default '' not null,
    tg_file_id			text 		default '' not null,
    tg_file_size		int			default 0  not null,
    date_create			timestamp	not null
);
create index if not exists cache_native_path_file_index
    on cache (native_path_file);
create index if not exists cache_native_md5_sum_index
    on cache (native_md5_sum);
create index if not exists cache_video_url_id_index
    on cache (video_url_id);

create table if not exists links
(
	id      serial
        	constraint links_pk
            primary key,
    md5_url				text 	default '' not null,
    url					text 	default '' not null,
    telegram_id			bigint	not null,
    date_create			text 	default '' not null
);
create index if not exists links_md5_url
    on links (md5_url);
`); err != nil {
		log.Error(err)
	}
}
