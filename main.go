package main

import (
	"crypto/md5"
	"database/sql"
	"fmt"
	"github.com/jmoiron/sqlx"
	"github.com/krol44/telegram-bot-api"
	log "github.com/sirupsen/logrus"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var Postgres *sqlx.DB

func main() {
	log.Info("TorPurrBot is running...")

	Postgres = PostgresConnect()

	app := Run()
	go app.ObserverQueue()

	for update := range app.BotUpdates {
		if update.Message != nil {
			// logs
			app.Logs(update.Message)

			isBlock := app.IsBlockUser(update.Message.From.ID)

			if update.Message.Text != "" {
				suffix := "sent message: "
				if isBlock {
					suffix = "[blocked] sent message: "
				}
				app.SendLogToChannel(update.Message.From, "mess", suffix+update.Message.Text)
			}

			if isBlock {
				continue
			}

			app.Queue <- struct{ Message *tgbotapi.Message }{Message: update.Message}
		}

		if update.InlineQuery != nil {
			if update.InlineQuery.Query == "" {
				continue
			}

			var urlVideo string
			sp := strings.Split(update.InlineQuery.Query, "&")
			if len(sp) >= 1 {
				urlVideo = sp[0]
			}

			var cache struct {
				ID       int    `db:"id"`
				TgFileId string `db:"tg_file_id"`
				Caption  string `db:"caption"`
				Path     string `db:"native_path_file"`
			}

			err := Postgres.Get(&cache, `SELECT tg_file_id, caption, native_path_file
												FROM cache WHERE caption LIKE $1`, "%"+urlVideo+"%")
			if err != nil && err != sql.ErrNoRows {
				log.Error(err)
				continue
			}

			md5Url := fmt.Sprintf("%x", md5.Sum([]byte(update.InlineQuery.Query)))
			_, err = Postgres.Exec(`INSERT INTO links (md5_url, url, telegram_id, date_create)
									VALUES($1, $2, $3, $4)`,
				md5Url, update.InlineQuery.Query, update.InlineQuery.From.ID, time.Now())

			var inlineConf tgbotapi.InlineConfig
			if cache.TgFileId == "" {
				inlineConf = tgbotapi.InlineConfig{
					InlineQueryID:     update.InlineQuery.ID,
					IsPersonal:        true,
					CacheTime:         1,
					SwitchPMText:      "No found cache video, click to create",
					SwitchPMParameter: md5Url,
				}
			} else {
				var res []interface{}

				if path.Ext(cache.Path) == ".mp4" {
					b := tgbotapi.NewInlineQueryResultCachedVideo(fmt.Sprintf("%d", cache.ID),
						cache.TgFileId, cache.Caption)
					b.Caption = cache.Caption + signAdvt
					res = append(res, b)
				} else if path.Ext(cache.Path) == ".mp3" {
					b := tgbotapi.NewInlineQueryResultCachedAudio(fmt.Sprintf("%d", cache.ID), cache.TgFileId)
					b.Caption = cache.Caption + signAdvt
					res = append(res, b)
				} else {
					b := tgbotapi.NewInlineQueryResultCachedDocument(fmt.Sprintf("%d", cache.ID),
						cache.TgFileId, cache.Caption)
					b.Caption = cache.Caption + signAdvt
					res = append(res, b)
				}

				inlineConf = tgbotapi.InlineConfig{
					InlineQueryID: update.InlineQuery.ID,
					IsPersonal:    true,
					CacheTime:     1,
					Results:       res,
				}
			}

			if err != nil {
				log.Error(err)
				continue
			}

			if _, err := app.Bot.Request(inlineConf); err != nil {
				log.Error(err)
			}
		}

		if update.ChannelPost != nil && update.ChannelPost.Chat.ID == config.ChatIdChannelLog {
			sp := strings.Split(update.ChannelPost.Text, " ")

			if len(sp) == 2 && sp[0] == "/premium" {
				var userFromDB User
				_ = Postgres.Get(&userFromDB, `SELECT name, premium, language_code
														FROM users WHERE telegram_id = $1`, sp[1])

				tr := Translate{Code: userFromDB.LanguageCode}
				premium := 0
				premiumText := tr.Lang("Premium is disabled") + " 😔"
				if userFromDB.Premium == 0 {
					premium = 1
					premiumText = tr.Lang("Premium is enabled") + " 🎉"
				}

				_, err := Postgres.Exec("UPDATE users SET premium = $1 WHERE telegram_id = $2", premium, sp[1])
				if err != nil {
					log.Error(err)
				}

				if whoId, err := strconv.Atoi(sp[1]); err == nil {
					app.SendLogToChannel(&tgbotapi.User{ID: int64(whoId)}, "mess", premiumText)
					_, _ = app.Bot.Send(tgbotapi.NewMessage(int64(whoId), premiumText))
				}
			}

			if len(sp) == 2 && sp[0] == "/block" {
				var userFromDB User
				_ = Postgres.Get(&userFromDB, "SELECT name, block FROM users WHERE telegram_id = $1", sp[1])

				block := 0
				if userFromDB.Block == 0 {
					block = 1
				}

				_, err := Postgres.Exec("UPDATE users SET block = $1 WHERE telegram_id = $2", block, sp[1])
				if err != nil {
					log.Error(err)
				}
				if whoId, err := strconv.Atoi(sp[1]); err == nil {
					app.SendLogToChannel(&tgbotapi.User{ID: int64(whoId)}, "mess",
						fmt.Sprintf("block=%d", block))
				}
			}

			if update.ChannelPost.ReplyToMessage != nil {
				regx := regexp.MustCompile(` \((.*?)\) `)
				matches := regx.FindStringSubmatch(update.ChannelPost.ReplyToMessage.Text)

				if len(matches) == 2 {
					replayChatId, _ := strconv.Atoi(matches[1])
					_, err := app.Bot.Send(tgbotapi.NewMessage(int64(replayChatId),
						"Support: "+update.ChannelPost.Text))
					if err != nil {
						log.Error(err)
					}
				}
			}
		}

		if update.MyChatMember != nil {
			if update.MyChatMember.NewChatMember.Status == "kicked" {
				_, err := Postgres.Exec("UPDATE users SET block_why = $1 WHERE telegram_id = $2",
					"user kicked bot", update.MyChatMember.From.ID)
				if err != nil {
					log.Error(err)
				}

				app.SendLogToChannel(&tgbotapi.User{ID: update.MyChatMember.From.ID,
					UserName: update.MyChatMember.From.UserName},
					"mess", "🩸 user kicked bot")
			}
		}
	}
}
