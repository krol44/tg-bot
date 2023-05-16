package main

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/anacrolix/torrent"
	"github.com/dustin/go-humanize"
	"github.com/krol44/telegram-bot-api"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Task struct {
	App             *App
	Message         *tgbotapi.Message
	File            string
	Files           []string
	FileConverted   FileConverted
	MessageEditID   int
	MessageTextLast string
	UserFromDB      User
	Translate       *Translate
	Torrent         struct {
		Name     string
		Process  *torrent.Torrent
		Progress int64
	}
	DescriptionUrl string
	UrlIDForCache  string
}

func (t *Task) Run(th ObjectHandler) {
	th.Download()
	th.Convert()
	th.Send()
	th.Clean()
}

func (t *Task) Send(ct tgbotapi.Chattable) (tgbotapi.Message, bool) {
	mess, err := t.App.Bot.Send(ct)
	t.App.Logs(mess)

	if err != nil {
		if strings.Contains(err.Error(), "bot was blocked by the user") ||
			strings.Contains(err.Error(), "message to edit not found") {
			log.Warn(ct, err)
			return tgbotapi.Message{}, true
		}

		log.Error(ct, err)
		log.Infof("%+v", errors.WithStack(errors.New("Stacktrace")))

		return tgbotapi.Message{}, true
	}

	return mess, false
}

func (t *Task) Alloc(typeDl string) bool {
	qn, _ := t.App.ChatsWork.m.Load(t.Message.MessageID)
	t.App.SendLogToChannel(t.Message.From, "mess",
		fmt.Sprintf("downloading %s - %s | his turn: %d",
			typeDl, t.Message.Text, qn.(int)+1))

	msg := tgbotapi.NewMessage(t.Message.Chat.ID, "🍀 "+t.Lang("Download is starting soon")+"...")

	// creating edit message
	messStat, err := t.Send(msg)
	if err {
		return false
	}
	t.MessageEditID = messStat.MessageID

	for {
		if _, bo := t.App.ChatsWork.StopTasks.Load(t.Message.Chat.ID); bo {
			return true
		}

		// global queue
		qn, _ := t.App.ChatsWork.m.Load(t.Message.MessageID)
		if qn.(int) < config.MaxTasks {
			break
		}

		ms := fmt.Sprintf(
			"🍀 "+t.Lang("Download is starting soon")+"...\n\n🚦 "+t.Lang("Your queue")+": %d",
			qn.(int)-config.MaxTasks+1)

		if ms != t.MessageTextLast {
			_, err := t.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEditID, ms))
			if err {
				return false
			}
			t.MessageTextLast = ms
		}

		time.Sleep(4 * time.Second)
	}

	return true
}

func (t *Task) AllocTorrent(typeDl string) bool {
	qn, _ := t.App.TorrentChatsWork.m.Load(t.Message.MessageID)
	t.App.SendLogToChannel(t.Message.From, "mess",
		fmt.Sprintf("downloading %s - %s | his turn torrent: %d",
			typeDl, t.Message.Text, qn.(int)+1))

	msg := tgbotapi.NewMessage(t.Message.Chat.ID, "🍀 "+t.Lang("Download is starting soon")+"...")

	// creating edit message
	messStat, err := t.Send(msg)
	if err {
		return false
	}
	t.MessageEditID = messStat.MessageID

	for {
		if _, bo := t.App.ChatsWork.StopTasks.Load(t.Message.Chat.ID); bo {
			return true
		}

		// torrent queue
		qn, _ := t.App.TorrentChatsWork.m.Load(t.Message.MessageID)
		if qn.(int) < config.MaxTasksTorrent {
			break
		}

		ms := fmt.Sprintf(
			"🍀 "+t.Lang("Download is starting soon")+"...\n\n🚦 "+t.Lang("Your queue")+": %d",
			qn.(int)-config.MaxTasksTorrent+1)

		if ms != t.MessageTextLast {
			_, err := t.Send(tgbotapi.NewEditMessageText(t.Message.Chat.ID, t.MessageEditID, ms))
			if err {
				return false
			}
			t.MessageTextLast = ms
		}

		time.Sleep(4 * time.Second)
	}

	return true
}

func (t *Task) Limit(typeDl string) bool {
	if t.UserFromDB.Premium == 1 {
		return false
	}

	var ld struct {
		Quantity int `db:"quantity"`
	}
	err := Postgres.Get(&ld, `SELECT count(id) AS quantity FROM limits
	                             WHERE type_object = $1 AND telegram_id = $2 AND
	                           date_create BETWEEN now() - INTERVAL '24 hour' AND now()`, typeDl, t.Message.From.ID)
	if err != nil && err != sql.ErrNoRows {
		log.Error(err)
		return false
	}

	re := false
	if ld.Quantity >= 2 && typeDl == "torrent" {
		re = true
	} else if ld.Quantity >= 5 {
		re = true
	}

	if re {
		ms := tgbotapi.NewMessage(t.Message.Chat.ID, "😔 "+typeDl+" - "+
			t.Lang("limit exceeded, try again in 24 hours")+"\n\n❤️ "+t.Lang("Support me and get unlimited")+
			"\n https://boosty.to/torpurrbot",
		)
		ms.DisableWebPagePreview = true
		t.Send(ms)

		t.App.SendLogToChannel(t.Message.From, "mess", fmt.Sprintf("🪫 limit exceeded - "+typeDl))

		return true
	}

	_, err = Postgres.Exec(`INSERT INTO limits (type_object, telegram_id, date_create)
									VALUES ($1, $2, NOW())`, typeDl, t.Message.From.ID)
	if err != nil {
		log.Error(err)
		return false
	}

	return false
}

func (t *Task) OpenKeyBoardWithTorrentFiles() *torrent.Torrent {
	isMagnet := strings.Contains(t.Message.Text, "magnet:?xt=")

	var (
		torrentProcess *torrent.Torrent
		file           tgbotapi.File
		err            error
	)

	var isError bool

	if !isMagnet {
		file, err = t.App.Bot.GetFile(tgbotapi.FileConfig{FileID: t.Message.Document.FileID})
		if err != nil {
			isError = true
			log.Error(err)
		}

		torrentProcess, err = t.App.TorClient.AddTorrentFromFile(config.TgPathLocal + "/" + config.BotToken +
			"/" + file.FilePath)
		if err != nil {
			isError = true
			log.Error(err)
		}

		t.App.SendLogToChannel(t.Message.From, "doc",
			"upload torrent file", t.Message.Document.FileID)
	} else {
		torrentProcess, err = t.App.TorClient.AddMagnet(t.Message.Text)
		if err != nil {
			isError = true
			log.Warn(err)
		}

		t.App.SendLogToChannel(t.Message.From, "mess", "torrent magnet")
	}

	if isError {
		t.Send(tgbotapi.NewMessage(t.Message.Chat.ID,
			"😔 "+t.Lang("Bad torrent file or magnet link")))
		t.App.SendLogToChannel(t.Message.From, "mess",
			"Bad torrent file or magnet link")
		return nil
	}

	ctxTimeLimit, cancel := context.WithTimeout(context.Background(), time.Second*30)
	m, _ := t.Send(tgbotapi.NewMessage(t.Message.Chat.ID, "🕚 "+t.Lang("Getting data from torrent, please wait")))

	select {
	case <-torrentProcess.GotInfo():
	case <-ctxTimeLimit.Done():
	}
	cancel()
	t.App.Bot.Send(tgbotapi.NewDeleteMessage(t.Message.Chat.ID, m.MessageID))

	if torrentProcess.Info() == nil {
		t.Send(tgbotapi.NewMessage(t.Message.Chat.ID,
			"😔 "+t.Lang("No data in the torrent file or magnet link, no seeds to get info")))
		t.App.SendLogToChannel(t.Message.From, "mess",
			"error torrent - no files or time limit get info")
		return nil
	}

	if len(torrentProcess.Files()) == 1 {
		f := torrentProcess.Files()[0]
		t.Message.Text = f.Path() + " ~ " + strconv.FormatInt(f.Length()>>20, 10) + " MB"
		return torrentProcess
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

	mess, _ := t.Send(msg)

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

func (t *Task) StatDlTor(fileChosen *torrent.File) (string, float64) {
	if t.Torrent.Process.Info() == nil {
		return "", 0
	}

	currentProgress := t.Torrent.Process.BytesCompleted()
	downloadSpeed := humanize.Bytes(uint64(currentProgress-t.Torrent.Progress)) + "/s"
	t.Torrent.Progress = currentProgress

	ctlInfo := fileChosen.FileInfo().Length
	complete := humanize.Bytes(uint64(fileChosen.BytesCompleted()))
	size := humanize.Bytes(uint64(ctlInfo))
	var percentage float64
	if ctlInfo != 0 {
		percentage = float64(fileChosen.BytesCompleted()) / float64(ctlInfo) * 100
	}

	stat := fmt.Sprintf(
		"🔥 "+t.Lang("Progress")+": \t%s / %s  %.2f%%\n\n🔽 "+t.Lang("Speed")+
			": %s (Act. peers %d / Total %d)",
		complete, size, percentage, downloadSpeed, t.Torrent.Process.Stats().ActivePeers,
		t.Torrent.Process.Stats().TotalPeers)

	return stat, percentage
}

func (t *Task) GetTimeSlice() ([]string, bool) {
	regx := regexp.MustCompile(`-ss (.*?) -to (.{8})`)
	matches := regx.FindStringSubmatch(t.Message.Text)

	if len(matches) == 3 {
		_, err := time.Parse(time.TimeOnly, matches[1])
		if err != nil {
			return nil, false
		}
		_, err = time.Parse(time.TimeOnly, matches[2])
		if err != nil {
			return nil, false
		}

		return []string{matches[1], matches[2]}, true
	}

	return nil, false
}

func (t *Task) PremiumAd(typeDl string) {
	if t.UserFromDB.Premium == 0 && typeDl == "torrent" {
		messPremium := tgbotapi.NewMessage(t.Message.Chat.ID, "‼️ "+
			t.Lang("Only the first 5 minutes video is "+
				"available and torrent in the zip archive don't available")+"\n\n"+
			fmt.Sprintf(`<a href="%s">%s</a>`,
				"https://www.donationalerts.com/r/torpurrbot",
				t.Lang("To donate, for to improve the bot"))+" 🔥\n"+
			"("+t.Lang("Write your telegram username in the body message."+
			" After donation, you will get full access for 30 days")+")")
		messPremium.ParseMode = tgbotapi.ModeHTML
		t.Send(messPremium)

		rand.New(rand.NewSource(time.Now().UnixNano()))
		t.Send(tgbotapi.NewSticker(t.Message.Chat.ID,
			tgbotapi.FileID(config.CuteStickers[rand.Intn(len(config.CuteStickers))])))
	}
}

func (Task) DirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return err
	})
	return size, err
}

func (Task) IsAllowFormatForConvert(pathWay string) bool {
	for _, ext := range config.AllowVideoFormats {
		if strings.ToLower(ext) == strings.ToLower(path.Ext(pathWay)) {
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
