package main

import (
	"crypto/md5"
	"fmt"
	"github.com/jmoiron/sqlx"
	"github.com/krol44/telegram-bot-api"
	log "github.com/sirupsen/logrus"
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
				suffix := "Sent message: "
				if isBlock {
					suffix = "[blocked] Sent message: "
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

			md5Url := fmt.Sprintf("%x", md5.Sum([]byte(update.InlineQuery.Query)))

			url := "https://t.me/" + app.Bot.Self.UserName + "?start=" + md5Url
			article := tgbotapi.NewInlineQueryResultArticle(update.InlineQuery.ID,
				"🫡 Generated url", "Downloading and watching through "+app.Bot.Self.UserName+" 🫡 \n"+url)

			_, err := Postgres.Exec(`INSERT INTO links (md5_url, url, telegram_id, date_create)
									VALUES($1, $2, $3, $4)`,
				md5Url, update.InlineQuery.Query, update.InlineQuery.From.ID, time.Now())

			if err != nil {
				log.Error(err)
				continue
			}

			article.Description = url

			inlineConf := tgbotapi.InlineConfig{
				InlineQueryID: update.InlineQuery.ID,
				IsPersonal:    false,
				CacheTime:     1,
				Results:       []interface{}{article},
			}

			if _, err := app.Bot.Request(inlineConf); err != nil {
				log.Println(err)
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
				_, err := Postgres.Exec("UPDATE users SET block = $1, block_why = $2 WHERE telegram_id = $3",
					1, "user kicked bot", update.MyChatMember.From.ID)
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
