package main

import (
	"fmt"
	"github.com/anacrolix/torrent"
	"github.com/krol44/telegram-bot-api"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"os"
	"path"
	"time"
)

type Task struct {
	App             *App
	Message         *tgbotapi.Message
	Files           []string
	MessageEditID   int
	MessageTextLast string
	UserFromDB      User
	Translate       *Translate
	Torrent         struct {
		Name            string
		Process         *torrent.Torrent
		Progress        int64
		Uploaded        int64
		TorrentProgress int64
		TorrentUploaded int64
	}
	VideoUrlID string
}

func (t *Task) Send(ct tgbotapi.Chattable) tgbotapi.Message {
	mess, err := t.App.Bot.Send(ct)
	if err != nil {
		log.Error(ct, err)
		log.Infof("%+v", errors.WithStack(errors.New("Stacktrace")))
	}

	t.App.Logs(mess)

	return mess
}

func (t *Task) Alloc(typeDl string) {
	// creating edit message
	messStat := t.Send(tgbotapi.NewMessage(t.Message.Chat.ID, "🍀 "+t.Lang("Download is starting soon")+
		"..."))
	t.MessageEditID = messStat.MessageID

	for {
		// global queue
		qn, _ := t.App.ChatsWork.m.Load(t.Message.MessageID)
		if qn.(int) < config.MaxTasks {
			break
		}

		t.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEditID, fmt.Sprintf(
			"🍀 "+t.Lang("Download is starting soon")+"...\n\n🚦 "+t.Lang("Your queue")+": %d",
			qn.(int)-config.MaxTasks+1)))

		time.Sleep(4 * time.Second)
	}

	// todo if you need
	//if t.UserFromDB.Premium == 0 && typeDl == "torrent" {
	//	messPremium := tgbotapi.NewMessage(t.Message.Chat.ID, "‼️ "+
	//		t.Lang("Only the first 5 minutes video is "+
	//			"available and torrent in the zip archive don't available")+"\n\n"+
	//		fmt.Sprintf(`<a href="%s">%s</a>`,
	//			"https://www.donationalerts.com/r/torpurrbot",
	//			t.Lang("To donate, for to improve the bot"))+" 🔥\n"+
	//		"("+t.Lang("Write your telegram username in the body message."+
	//		" After donation, you will get full access for 30 days")+")")
	//	messPremium.ParseMode = tgbotapi.ModeHTML
	//	t.Send(messPremium)
	//
	//	rand.Seed(time.Now().Unix())
	//	t.Send(tgbotapi.NewSticker(t.Message.Chat.ID,
	//		tgbotapi.FileID(config.CuteStickers[rand.Intn(len(config.CuteStickers))])))
	//}

	// log
	t.App.SendLogToChannel(t.Message.From.ID, "mess", fmt.Sprintf("start download "+typeDl))
}

func (t *Task) Lang(str string) string {
	return t.Translate.Lang(str)
}

func (t *Task) RemoveMessageEdit() {
	_, _ = t.App.Bot.Send(tgbotapi.NewDeleteMessage(t.Message.Chat.ID, t.MessageEditID))
}

func (t *Task) Cleaner() {
	t.App.LockForRemove.Add(1)

	if config.IsDev == false {
		pathConvert := config.DirBot + "/storage"
		dirs, _ := os.ReadDir(pathConvert)

		for _, val := range dirs {
			err := os.RemoveAll(pathConvert + "/" + val.Name())
			if err != nil {
				log.Error(err)
			}
		}

		log.Info("Folders cleaning...")
	}

	if config.IsDev == false {
		pathTorrent := config.DirBot + "/torrent-client"
		tors, _ := os.ReadDir(pathTorrent)
		for _, val := range tors {
			if val.Name() == ".torrent.db" || val.Name() == ".torrent.db-shm" || val.Name() == ".torrent.db-wal" {
				continue
			}
			err := os.RemoveAll(pathTorrent + "/" + val.Name())
			if err != nil {
				log.Error(err)
			}
		}
	}

	t.App.LockForRemove.Done()
}

func (Task) IsAllowFormatForConvert(pathWay string) bool {
	for _, ext := range config.AllowVideoFormats {
		if ext == path.Ext(pathWay) {
			return true
		}
	}

	return false
}

func (Task) UniqueId(prefix string) string {
	now := time.Now()
	sec := now.Unix()
	use := now.UnixNano() % 0x100000
	return fmt.Sprintf("%s-%08x%05x", prefix, sec, use)
}
