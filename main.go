package main

import (
	"crypto/md5"
	"fmt"
	"github.com/krol44/telegram-bot-api"
	log "github.com/sirupsen/logrus"
	"regexp"
	"strconv"
	"strings"
)

func main() {
	log.Info("TorPurrBot is running...")

	app := Run()
	go app.ObserverQueue()

	for update := range app.BotUpdates {
		if update.Message != nil {
			// logs sqlite
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

			db := Sqlite()
			_, err := db.Exec(`INSERT INTO links (md5_url, url, telegram_id, date_create)
									VALUES(?, ?, ?, datetime('now'))`,
				md5Url, update.InlineQuery.Query, update.InlineQuery.From.ID)
			db.Close()
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
				db := Sqlite()

				var userFromDB User
				_ = db.Get(&userFromDB, "SELECT name, premium, language_code FROM users WHERE telegram_id = ?",
					sp[1])

				tr := Translate{Code: userFromDB.LanguageCode}
				premium := 0
				premiumText := tr.Lang("Premium is disabled") + " 😔"
				if userFromDB.Premium == 0 {
					premium = 1
					premiumText = tr.Lang("Premium is enabled") + " 🎉"
				}
				_, err := db.Exec("UPDATE users SET premium = ? WHERE telegram_id = ?", premium, sp[1])
				if err != nil {
					log.Error(err)
				}

				db.Close()

				if whoId, err := strconv.Atoi(sp[1]); err == nil {
					app.SendLogToChannel(&tgbotapi.User{ID: int64(whoId)}, "mess", premiumText)
					_, _ = app.Bot.Send(tgbotapi.NewMessage(int64(whoId), premiumText))
				}
			}

			if len(sp) == 2 && sp[0] == "/block" {
				db := Sqlite()

				var userFromDB User
				_ = db.Get(&userFromDB, "SELECT name, block FROM users WHERE telegram_id = ?",
					sp[1])

				block := 0
				if userFromDB.Block == 0 {
					block = 1
				}
				_, err := db.Exec("UPDATE users SET block = ? WHERE telegram_id = ?", block, sp[1])
				if err != nil {
					log.Error(err)
				}
				if whoId, err := strconv.Atoi(sp[1]); err == nil {
					app.SendLogToChannel(&tgbotapi.User{ID: int64(whoId)}, "mess",
						fmt.Sprintf("block=%d", block))
				}

				db.Close()
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
				db := Sqlite()
				_, err := db.Exec("UPDATE users SET block = ?, block_why = ? WHERE telegram_id = ?", 1,
					"user kicked bot",
					update.MyChatMember.From.ID)
				if err != nil {
					log.Error(err)
				}
				db.Close()

				app.SendLogToChannel(&tgbotapi.User{ID: update.MyChatMember.From.ID,
					UserName: update.MyChatMember.From.UserName},
					"mess", "🩸 user kicked bot")
			}
		}
	}
}
