package main

import (
	"fmt"
	"github.com/anacrolix/torrent"
	"github.com/krol44/telegram-bot-api"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

type Task struct {
	App             *App
	Message         *tgbotapi.Message
	File            string
	MessageEditID   int
	MessageTextLast string
	UserFromDB      User
	Translate       *Translate
	Torrent         struct {
		Name     string
		Process  *torrent.Torrent
		Progress int64
	}
	VideoUrlHttp string
	VideoUrlID   string
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
	msg := tgbotapi.NewMessage(t.Message.Chat.ID, "🍀 "+t.Lang("Download is starting soon")+
		"...")

	// creating edit message
	messStat := t.Send(msg)
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

func (t *Task) OpenKeyBoardWithTorrentFiles() []string {
	isMagnet := strings.Contains(t.Message.Text, "magnet:?xt=")

	var (
		torrentProcess *torrent.Torrent
		file           tgbotapi.File
		err            error
	)

	qn, _ := t.App.ChatsWork.m.Load(t.Message.MessageID)
	if !isMagnet {
		file, err = t.App.Bot.GetFile(tgbotapi.FileConfig{FileID: t.Message.Document.FileID})
		if err != nil {
			log.Error(err)
			return nil
		}

		torrentProcess, err = t.App.TorClient.AddTorrentFromFile(config.TgPathLocal + "/" + config.BotToken +
			"/" + file.FilePath)
		if err != nil {
			log.Error(err)
			return nil
		}
		// log
		t.App.SendLogToChannel(t.Message.From.ID, "doc",
			fmt.Sprintf("upload torrent file | his turn: %d", qn.(int)+1), t.Message.Document.FileID)
	} else {
		torrentProcess, err = t.App.TorClient.AddMagnet(t.Message.Text)
		if err != nil {
			log.Error(err)
			return nil
		}
		// log
		t.App.SendLogToChannel(t.Message.From.ID, "mess",
			fmt.Sprintf("torrent magnet | his turn: %d", qn.(int)+1))
	}

	var keyBoardButtons []tgbotapi.KeyboardButton
	for index, val := range torrentProcess.Files() {
		if index >= 50 {
			continue
		}

		str := val.Path() + " ~ " + strconv.FormatInt(val.Length()>>20, 10) + " MB"
		if len([]rune(str)) >= 120 {
			str = str[len([]rune(str))-120:]
		}

		keyBoardButtons = append(keyBoardButtons, tgbotapi.NewKeyboardButton(str))

		t.App.ChatsWork.TorrentProcesses.Store(str, torrentProcess)

		// purify syncMap
		go func(strIn string, chatID int64) {
			time.Sleep(time.Hour)
			tor, bo := t.App.ChatsWork.TorrentProcesses.LoadAndDelete(strIn)
			if bo {
				tor.(*torrent.Torrent).Drop()
			}
		}(str, t.Message.Chat.ID)
	}
	var keyboardButtonRows [][]tgbotapi.KeyboardButton
	for _, val := range keyBoardButtons {
		keyboardButtonRows = append(keyboardButtonRows, tgbotapi.NewKeyboardButtonRow(val))
	}

	if keyboardButtonRows == nil {
		return nil
	}

	var numericKeyboard = tgbotapi.NewReplyKeyboard(keyboardButtonRows...)

	msg := tgbotapi.NewMessage(t.Message.Chat.ID, "📍 "+t.Lang("Choose a file, max size 2 GB"))

	msg.ReplyMarkup = numericKeyboard

	mess := t.Send(msg)

	t.App.ChatsWork.ChosenMessageIDs.Store(mess.Chat.ID, mess.MessageID)

	return nil
}

func (t *Task) CloseKeyBoardWithTorrentFiles() {
	if messEditID, bo := t.App.ChatsWork.ChosenMessageIDs.Load(t.Message.Chat.ID); bo {
		t.App.Bot.Send(tgbotapi.NewDeleteMessage(t.Message.Chat.ID, messEditID.(int)))
	}
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
