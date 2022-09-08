package main

// сюда писать код

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	tgbotapi "github.com/skinass/telegram-bot-api/v5"
	"golang.org/x/exp/slices"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"text/template"
)

type Config struct {
	TelegramBotToken string
	WebhookURL       string
}

//type IdTask int64
//
//type IdUser int64

type Task struct {
	Val       string
	IdOwn     int64
	IdPerform int64
}

type User struct {
	UserName       string
	TasksOwnId     []int64
	TasksPerformId []int64
}

var (
	Tasks = make([]Task, 0)
	Users = make(map[int64]User)

	Configuration = Config{}

	tmpl    = template.New("TemplTasks")
	tmplNew = template.New("TemplNewTask")
)

func writeMsgTasks(chose []int64, IdUser int64) (string, error) {
	buf := bytes.Buffer{}
	tmpl, err := tmpl.Parse(TemplTasks)
	if err != nil {
		return "", fmt.Errorf("tmpl.Parse(TemplTasks): %v", err)
	}
	for key, val := range Tasks {
		if slices.Contains(chose, int64(key+1)) && val.Val != "" {
			tmpl.Execute(&buf, struct {
				Key             int
				Val             string
				FreeTask        bool
				MyTask          bool
				UserNamePerform string
				UserNameOwner   string
			}{Key: key + 1,
				Val:             val.Val,
				FreeTask:        val.IdPerform == 0,
				MyTask:          IdUser == val.IdPerform,
				UserNamePerform: Users[val.IdPerform].UserName,
				UserNameOwner:   Users[val.IdOwn].UserName})
		}
	}
	if buf.Bytes() != nil {
		msgText := buf.String()
		msgText = strings.TrimRight(msgText, "\n")
		//msgText += "\n"
		return msgText, nil
	} else {
		return `Нет задач`, nil
	}
}

func startTaskBot(ctx context.Context) error {
	// сюда пишите ваш код
	// Читаю с конфига токен и буду (webHook url), чтобы в общедоступной репе его никто не забрал
	file, err := os.Open("config.json")
	if err != nil {
		return fmt.Errorf("open config.json failed: %v", err)
	}
	defer func() {
		err = file.Close()
		if err != nil {
			log.Printf("config.json don`t close: %v", err)
		}
	}()
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&Configuration)
	if err != nil {
		return fmt.Errorf("decode failed: %s", err)
	}
	fmt.Println(Configuration.TelegramBotToken)
	bot, err := tgbotapi.NewBotAPI(Configuration.TelegramBotToken)
	if err != nil {
		return fmt.Errorf("NewBotAPI failed: %v", err)
	}

	bot.Debug = true
	fmt.Printf("Authorized on account %s\n", bot.Self.UserName)

	//Надо будет тоже считать из конфига
	// Юзаем вебхуки, т.к. бесплатная heroku засыпает через какое-то время, если не отправлять запросы приложению.
	wh, err := tgbotapi.NewWebhook(Configuration.WebhookURL)
	if err != nil {
		return fmt.Errorf("NewWebhook failed: %v", err)
	}

	// Отправляем запрос tgApi, тем самым она теперь будет присылать изменения через вебхуку и бесплатная heroku не будет засыпать
	_, err = bot.Request(wh)
	if err != nil {
		return fmt.Errorf("SetWebhook failed: %v", err)
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
		log.Fatalf("Http err: %v", http.ListenAndServe(":"+port, nil))
	}()
	fmt.Println("start listen :" + port)
	// В канал updates будут приходить все новые сообщения.
	for update := range updates {
		//fmt.Printf("upd: %#v\n", update)
		log.Printf("upd: %#v\n", update)
		command := update.Message.Text
		if update.Message == nil {
			log.Printf("update.Message == nil: %#v\n", update)
			continue
		}
		if update.Message.Chat == nil {
			log.Printf("update.Message.Chat == nil: %#v\n", update)
			continue
		}
		if update.Message.From == nil {
			log.Printf("update.Message.From == nil: %#v\n", update)
			continue
		}
		msgText := ""
		switch {
		case strings.HasPrefix(command, "/tasks"):
			{
				_, ok := Users[update.Message.From.ID]
				if !ok {
					Users[update.Message.From.ID] = User{
						UserName:       update.Message.From.UserName,
						TasksOwnId:     make([]int64, 0),
						TasksPerformId: make([]int64, 0),
					}
				}
				if len(Tasks) == 0 {
					msgText = "Нет задач"
					break
				}

				choseAll := make([]int64, 0)
				// слайс из Id всех задач
				for _, val := range Users {
					choseAll = append(choseAll, val.TasksOwnId...)
				}
				msgText, err = writeMsgTasks(choseAll, update.Message.From.ID)
				if err != nil {
					return fmt.Errorf("writeMsgTasks: %v", err)
				}
			}
		case strings.HasPrefix(command, "/my"):
			{
				msgText, err = writeMsgTasks(Users[update.Message.From.ID].TasksPerformId, update.Message.From.ID)
				if err != nil {
					return fmt.Errorf("writeMsgTasks: %v", err)
				}
			}
		case strings.HasPrefix(command, "/owner"):
			{
				msgText, err = writeMsgTasks(Users[update.Message.From.ID].TasksOwnId, update.Message.From.ID)
				if err != nil {
					return fmt.Errorf("writeMsgTasks: %v", err)
				}
			}
		case strings.HasPrefix(command, "/new"):
			{
				command = strings.Trim(command, "/new ")
				Tasks = append(Tasks, Task{Val: command, IdOwn: update.Message.From.ID})
				UserCur, ok := Users[update.Message.From.ID]
				if !ok {
					return fmt.Errorf("users[%v] not exist(/new) %v", update.Message.From.ID, err)
				}
				UserCur.TasksOwnId = append(Users[update.Message.From.ID].TasksOwnId, int64(len(Tasks)))
				Users[update.Message.From.ID] = UserCur
				tmplNew, err = tmpl.Parse(TemplNewTask)
				if err != nil {
					return fmt.Errorf("tmpl.Parse(TemplNewTask): %v", err)
				}
				buf := bytes.Buffer{}
				err = tmplNew.Execute(&buf, struct {
					Task string
					Id   int
				}{Task: command, Id: len(Tasks)})
				msgText = buf.String()
				//msgText = strings.TrimRight(msgText, "\n")
				//msgText += "\n"
			}
		case strings.HasPrefix(command, "/assign_"):
			{
				idTask, err := strconv.Atoi(strings.Trim(command, "/assign_"))
				if err != nil {
					return fmt.Errorf("strconv.Atoi with idTask wrong(/assign_): %v", err)
				}
				UserCur, ok := Users[update.Message.From.ID]
				if !ok {
					return fmt.Errorf("users[%v] not exist(/assign_) %v", update.Message.From.ID, err)
				}
				if idTask > len(Tasks) || Tasks[idTask-1].Val == "" {
					msgText = "Нет такой задачи"
				}
				if Tasks[idTask-1].IdPerform != 0 {
					msg := tgbotapi.NewMessage(Tasks[idTask-1].IdPerform, fmt.Sprintf(`Задача "%v" назначена на @%v`, Tasks[idTask-1].Val, UserCur.UserName))
					_, err = bot.Send(msg)
					if err != nil {
						log.Printf("Send failed(assign) : %#v\n", update)
					}
				}

				//UserCur.TasksPerformId[idTask-1] = UserCur.TasksPerformId[len(UserCur.TasksPerformId)-1]
				//UserCur.TasksPerformId[len(UserCur.TasksPerformId)-1] = 0
				//UserCur.TasksPerformId = UserCur.TasksPerformId[:len(UserCur.TasksPerformId)-1]
				//Users[update.Message.From.ID] = UserCur

				Tasks[idTask-1].IdPerform = update.Message.From.ID
				UserCur.TasksPerformId = append(UserCur.TasksPerformId, int64(idTask))
				Users[update.Message.From.ID] = UserCur
				msgText = fmt.Sprintf(`Задача "%v" назначена на вас`, Tasks[idTask-1].Val)
			}
		case strings.HasPrefix(command, "/unassign_"):
			{
				idTask, err := strconv.Atoi(strings.Trim(command, "/unassign_"))
				if err != nil {
					return fmt.Errorf("strconv.Atoi with idTask wrong(/unassign_): %v", err)
				}
				UserCur, ok := Users[update.Message.From.ID]
				if !ok {
					return fmt.Errorf("users[%v] not exist(/unassign_) %v", update.Message.From.ID, err)
				}
				if idTask > len(Tasks) || Tasks[idTask-1].Val == "" {
					msgText = "Нет такой задачи"
				}
				log.Printf("Slice %v, ID %v", UserCur.TasksPerformId, idTask)
				if slices.Contains(UserCur.TasksPerformId, int64(idTask)) {
					// Удаление элемента из слайса
					UserCur.TasksPerformId[idTask-1] = UserCur.TasksPerformId[len(UserCur.TasksPerformId)-1]
					UserCur.TasksPerformId[len(UserCur.TasksPerformId)-1] = 0
					UserCur.TasksPerformId = UserCur.TasksPerformId[:len(UserCur.TasksPerformId)-1]
					Users[update.Message.From.ID] = UserCur

					Tasks[idTask-1].IdPerform = 0
					msgText = "Принято"
					msg := tgbotapi.NewMessage(Tasks[idTask-1].IdOwn, fmt.Sprintf(`Задача "%v" осталась без исполнителя`, Tasks[idTask-1].Val))
					_, err = bot.Send(msg)
					if err != nil {
						log.Printf("Send failed(unassign) : %#v\n", update)
					}
				} else {
					msgText = `Задача не на вас`
				}
			}
		case strings.HasPrefix(command, "/resolve_"):
			{
				idTask, err := strconv.Atoi(strings.Trim(command, "/resolve_"))
				if err != nil {
					return fmt.Errorf("strconv.Atoi with idTask wrong(/resolve_): %v", err)
				}
				UserCur, ok := Users[update.Message.From.ID]
				if !ok {
					return fmt.Errorf("users[%v] not exist(/resolve_) %v", update.Message.From.ID, err)
				}
				if idTask > len(Tasks) || Tasks[idTask-1].Val == "" {
					msgText = "Нет такой задачи"
				}
				if slices.Contains(UserCur.TasksPerformId, int64(idTask)) {
					// Удаление элемента из слайса
					UserCur.TasksPerformId[idTask-1] = UserCur.TasksPerformId[len(UserCur.TasksPerformId)-1]
					UserCur.TasksPerformId[len(UserCur.TasksPerformId)-1] = 0
					UserCur.TasksPerformId = UserCur.TasksPerformId[:len(UserCur.TasksPerformId)-1]
					Users[update.Message.From.ID] = UserCur

					Tasks[idTask-1].IdPerform = 0
					msgText = fmt.Sprintf(`Задача "%v" выполнена`, Tasks[idTask-1].Val)
					// Если задачу выполнил не владелец сообщения
					if update.Message.From.ID != Tasks[idTask-1].IdOwn {
						msg := tgbotapi.NewMessage(Tasks[idTask-1].IdOwn, fmt.Sprintf(`Задача "%v" выполнена @%v`, Tasks[idTask-1].Val, UserCur.UserName))
						_, err = bot.Send(msg)
						if err != nil {
							log.Printf("Send failed(unassign) : %#v\n", update)
						}
					}
					Tasks[idTask-1] = Task{}
				} else {
					msgText = `Задача не на вас`
				}
			}
		default:
			msgText = "Нет такой команды"
		}

		// Создав структуру - можно её отправить обратно боту
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, msgText)
		msg.ReplyToMessageID = update.Message.MessageID
		_, err = bot.Send(msg)
		if err != nil {
			log.Printf("Send failed(general) : %#v\n", update)
		}
		log.Printf("UPD send: %#v\n", update)
	}
	return nil

}

func main() {
	err := startTaskBot(context.Background())
	if err != nil {
		log.Printf("Error in startTaskBot: %v", err)
	}
}
