package main

// сюда писать код

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	tgbotapi "github.com/skinass/telegram-bot-api/v5"
	"log"
	"os"
)

var (
	// урл выдаст вам игрок или хероку
	// Yet haven`t

	//WebhookURL = "https://beautiful-bryce-canyon-60112.herokuapp.com"

	Configuration = Config{}
)

type Config struct {
	TelegramBotToken string
	WebhookURL       string
}

func startTaskBot(ctx context.Context) error {

	return nil
}

func main() {
	// сюда пишите ваш код
	// Читаю с конфига токен и буду (webHook url), чтобы в общедоступной репе его никто не забрал
	file, err := os.Open("config.json")
	if err != nil {
		log.Fatalf("Open config.json failed: %s", err)
	}
	defer func() {
		err = file.Close()
		if err != nil {
			log.Printf("config.json don`t close:%v", err)
		}
	}()
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&Configuration)
	if err != nil {
		log.Fatalf("Decode failed: %s", err)
	}
	fmt.Println(Configuration.TelegramBotToken)
	bot, err := tgbotapi.NewBotAPI(Configuration.TelegramBotToken)
	if err != nil {
		log.Fatalf("NewBotAPI failed: %s", err)
	}

	bot.Debug = true
	fmt.Printf("Authorized on account %s\n", bot.Self.UserName)

	//Надо будет тоже считать из конфига
	// Юзаем вебхуки, т.к. бесплатная heroku засыпает через какое-то время, если не отправлять запросы приложению.
	wh, err := tgbotapi.NewWebhook(Configuration.WebhookURL)
	if err != nil {
		log.Fatalf("NewWebhook failed: %s", err)
	}

	// Отправляем запрос tgApi, тем самым она теперь будет присылать изменения через вебхуку и бесплатная heroku не будет засыпать
	_, err = bot.Request(wh)
	if err != nil {
		log.Fatalf("SetWebhook failed: %s", err)
	}

	updates := bot.ListenForWebhook("/")

	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	// Таймаут между запросами, чтобы их стало меньше и соответственно каждый быстрее обрабатывался
	u.Timeout = 60

	http.HandleFunc("/state", func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte("all is working"))
		if err != nil {
			log.Printf("Handlefunc /state error write:%v", err)
		}
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	go func() {
		log.Fatalln("http err:", http.ListenAndServe(":"+port, nil))
	}()
	fmt.Println("start listen :" + port)

	// В канал updates будут приходить все новые сообщения.
	for update := range updates {
		//fmt.Printf("upd: %#v\n", update)
		log.Printf("upd: %#v\n", update)
		if update.Message == nil {
			log.Printf("update.Message == nil: %#v\n", update)
			continue
		}
		if update.Message.Chat == nil {
			log.Printf("update.Message.Chat == nil: %#v\n", update)
			continue
		}
		// Создав структуру - можно её отправить обратно боту
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, update.Message.Text)
		msg.ReplyToMessageID = update.Message.MessageID
		_, err = bot.Send(msg)
		if err != nil {
			log.Printf("Send failed : %#v\n", update)
		}
		log.Printf("UPD send: %#v\n", update)
	}
}
