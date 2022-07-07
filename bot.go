package main

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	tgbotapi "github.com/skinass/telegram-bot-api/v5"
	//tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

/*
для heroku
находясь в корне репы
git subtree push --prefix 04_net2/05_bot heroku master
*/

/*
go mod init  gitlab.com/mailru-go/lectures-2022-1/04_net2/05_bot
*/

// https://api.telegram.org/bot1953480583:AAEU7eBaZnCUt525oUkCMRCQxK1TJmaoVd4/getUpdates

const (
	BotToken   = "5406635928:AAGDMQnrQLub1gux8kt2sWnDNYfCMKOzwN4" // "1953480583:AAEU7eBaZnCUt525oUkCMRCQxK1TJmaoVd4"
	WebhookURL = "https://dz4-vilin-bot.herokuapp.com/"
)

var rss = map[string]string{
	"Habr": "https://habrahabr.ru/rss/best/",
}

type RSS struct {
	Items []Item `xml:"channel>item"`
}

type Item struct {
	URL   string `xml:"guid"`
	Title string `xml:"title"`
}

func getNews(url string) (*RSS, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)

	rss := new(RSS)
	err = xml.Unmarshal(body, rss)
	if err != nil {
		return nil, err
	}

	return rss, nil
}

func main() {
	rand.Seed(time.Now().UnixNano())
	bot, err := tgbotapi.NewBotAPI(BotToken)
	if err != nil {
		log.Fatalf("NewBotAPI failed: %s", err)
	}

	bot.Debug = true
	fmt.Printf("Authorized on account %s\n", bot.Self.UserName)

	wh, err := tgbotapi.NewWebhook(WebhookURL)
	if err != nil {
		log.Fatalf("NewWebhook failed: %s", err)
	}

	_, err = bot.Request(wh)
	if err != nil {
		log.Fatalf("SetWebhook failed: %s", err)
	}

	updates := bot.ListenForWebhook("/")

	http.HandleFunc("/state", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("all is working"))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	go func() {
		log.Fatalln("http err:", http.ListenAndServe(":"+port, nil))
	}()
	fmt.Println("start listen :" + port)

	// получаем все обновления из канала updates
	for update := range updates {
		log.Printf("upd: %#v\n", update)
		url, ok := rss[update.Message.Text]
		if !ok {
			msg := tgbotapi.NewMessage(
				update.Message.Chat.ID,
				`there is only Habr feed availible`,
			)

			msg.ReplyMarkup = &tgbotapi.ReplyKeyboardMarkup{
				Keyboard: [][]tgbotapi.KeyboardButton{
					{
						{
							Text: "Habr",
						},
					},
				},
			}
			bot.Send(msg)
			continue
		}

		rss, err := getNews(url)
		if err != nil {
			bot.Send(tgbotapi.NewMessage(
				update.Message.Chat.ID,
				"sorry, error happend",
			))
		}
		item := rss.Items[rand.Intn(len(rss.Items))]
		bot.Send(tgbotapi.NewMessage(
			update.Message.Chat.ID,
			item.URL+"\n"+item.Title,
		))
	}
}
