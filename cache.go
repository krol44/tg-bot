package main

import (
	"crypto/md5"
	"database/sql"
	"fmt"
	tgbotapi "github.com/krol44/telegram-bot-api"
	log "github.com/sirupsen/logrus"
	"os"
	"path"
	"strings"
)

type Cache struct {
	Task *Task
}
type CacheRow struct {
	id             int    `db:"id"`
	Caption        string `db:"caption"`
	NativePathFile string `db:"native_path_file"`
	NativeMd5Sum   string `db:"native_md_5_sum"`
	VideoUrlId     string `db:"video_url_id"`
	TgFromID       string `db:"tg_from_id"`
	TgFileID       string `db:"tg_file_id"`
	TgFileSize     int    `db:"tg_file_size"`
	DateCreate     string `db:"date_create"`
}

func (c Cache) Add(tgFileId string, tgFileSize int, NativeFilePath string) {
	db := Sqlite()
	defer db.Close()

	var md5Sum string
	if file, err := os.ReadFile(NativeFilePath); err == nil {
		md5Sum = fmt.Sprintf("%x", md5.Sum(file))
	}

	caption := strings.TrimSuffix(path.Base(NativeFilePath), path.Ext(path.Base(NativeFilePath)))

	var urlHttp string
	if c.Task.DescriptionUrl != "" {
		urlHttp = "\n" + c.Task.DescriptionUrl
	}

	_, err := db.Exec(`INSERT INTO cache
		(caption, native_path_file, native_md5_sum, video_url_id, tg_from_id, tg_file_id, tg_file_size, date_create)
		VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))`,
		caption+urlHttp, NativeFilePath, md5Sum, c.Task.UrlIDForCache, c.Task.Message.From.ID,
		tgFileId, tgFileSize)
	if err != nil {
		log.Error(err)
	}
}

func (c Cache) TrySend(typeSome string, pathway string) bool {
	db := Sqlite()
	defer db.Close()

	var row CacheRow
	err := db.Get(&row,
		"SELECT caption, tg_file_id FROM cache WHERE native_path_file = ? ORDER BY id DESC", pathway)
	if err != nil {
		return false
	}

	if typeSome == "video" {
		sob := tgbotapi.NewVideo(c.Task.Message.Chat.ID, tgbotapi.FileID(row.TgFileID))
		sob.Caption = row.Caption + signAdvt

		_, err := c.Task.App.Bot.Send(sob)
		if err != nil {
			return false
		}
		c.Task.App.SendLogToChannel(c.Task.Message.From, "mess",
			"video sent from cache - "+row.Caption)
	}
	if typeSome == "doc" {
		sob := tgbotapi.NewDocument(c.Task.Message.Chat.ID, tgbotapi.FileID(row.TgFileID))
		sob.Caption = row.Caption + signAdvt
		_, err := c.Task.App.Bot.Send(sob)
		if err != nil {
			return false
		}

		c.Task.App.SendLogToChannel(c.Task.Message.From, "mess",
			"doc sent from cache - "+row.Caption)
	}

	return true
}

func (c Cache) GetFileIdThroughMd5(NativeFilePath string) string {
	db := Sqlite()
	defer db.Close()

	var md5Sum string
	if file, err := os.ReadFile(NativeFilePath); err == nil {
		md5Sum = fmt.Sprintf("%x", md5.Sum(file))
	}

	if md5Sum == "" {
		return ""
	}

	var row CacheRow
	err := db.Get(&row,
		"SELECT tg_file_id FROM cache WHERE native_md5_sum = ? ORDER BY id DESC", md5Sum)
	if err != nil {
		return ""
	}

	return row.TgFileID
}

func (c Cache) TrySendThroughMd5(NativeFilePath string) bool {
	db := Sqlite()
	defer db.Close()

	var md5Sum string
	if file, err := os.ReadFile(NativeFilePath); err == nil {
		md5Sum = fmt.Sprintf("%x", md5.Sum(file))
	}

	if md5Sum == "" {
		return false
	}

	var row CacheRow
	err := db.Get(&row,
		"SELECT caption, tg_file_id FROM cache WHERE native_md5_sum = ? ORDER BY id DESC", md5Sum)
	if err != nil {
		return false
	}

	sob := tgbotapi.NewVideo(c.Task.Message.Chat.ID, tgbotapi.FileID(row.TgFileID))
	sob.Caption = row.Caption + signAdvt

	_, err = c.Task.App.Bot.Send(sob)
	if err != nil {
		return false
	}
	c.Task.App.SendLogToChannel(c.Task.Message.From, "mess",
		"video sent from cache md5 - "+row.Caption)

	return true
}

func (c Cache) TrySendThroughID() bool {
	db := Sqlite()
	defer db.Close()

	var row CacheRow
	err := db.Get(&row,
		"SELECT caption, tg_file_id FROM cache WHERE video_url_id = ? ORDER BY id DESC", c.Task.UrlIDForCache)
	if err != nil && err != sql.ErrNoRows {
		log.Error(err)
		return false
	}

	if row.TgFileID == "" {
		return false
	}

	sob := tgbotapi.NewVideo(c.Task.Message.Chat.ID, tgbotapi.FileID(row.TgFileID))
	sob.Caption = row.Caption + signAdvt

	_, err = c.Task.App.Bot.Send(sob)
	if err != nil {
		log.Error(err)
		return false
	}
	c.Task.App.SendLogToChannel(c.Task.Message.From, "mess",
		"video sent from cache video url id - "+row.Caption)

	return true
}
