package main

// сюда писать код

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	tgbotapi "github.com/skinass/telegram-bot-api/v5"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"text/template"
)

var (
	Tasks            = make(map[int64]Task, 0)
	Users            = make(map[int64]User)
	lastIdTask int64 = 0

	Configuration = Config{}

	tmpl    = template.New("TemplTasks")
	tmplNew = template.New("TemplNewTask")
)

type Config struct {
	TelegramBotToken string
	WebhookURL       string
}

type Task struct {
	Text      string
	IdOwn     int64
	IdPerform int64
}

type User struct {
	UserName       string
	TasksOwnId     []int64
	TasksPerformId []int64
}

func (u *User) maxLenSl() int {
	if len(u.TasksPerformId) > len(u.TasksOwnId) {
		return len(u.TasksPerformId)
	}
	return len(u.TasksOwnId)
}

func (u *User) DelElemFromSl(whichSl string, elem int64) error {
	sl := make([]int64, u.maxLenSl())
	if whichSl == "TasksOwnId" {
		copy(sl, u.TasksOwnId)
	} else {
		copy(sl, u.TasksPerformId)
	}

	for key, val := range sl {
		if val == elem {
			sl = append(sl[:key], sl[key+1:]...)
			if whichSl == "TasksOwnId" {
				copy(u.TasksOwnId, sl)
				u.TasksOwnId = u.TasksOwnId[:len(u.TasksOwnId)-1]
			} else {
				copy(u.TasksPerformId, sl)
				u.TasksPerformId = u.TasksPerformId[:len(u.TasksPerformId)-1]
			}
			return nil
		}
	}

	return fmt.Errorf("elem: %d not exist in slice %s", elem, whichSl)
}

func writeMsgTasks(chose []int64, IdUser int64, assignBool bool) (string, error) {
	buf := bytes.Buffer{}
	tmpl, err := tmpl.Parse(TemplTasks)
	if err != nil {
		return "", fmt.Errorf("tmpl.Parse(TemplTasks): %v", err)
	}

	// Не красиво и нет кучи проверок
	for _, val := range chose {
		err := tmpl.Execute(&buf, struct {
			Key             int64
			Val             string
			FreeTask        bool
			AssignBool      bool
			MyTask          bool
			UserNamePerform string
			UserNameOwner   string
		}{Key: val,
			Val:             Tasks[val].Text,
			FreeTask:        Tasks[val].IdPerform == 0,
			AssignBool:      assignBool,
			MyTask:          IdUser == Tasks[val].IdPerform,
			UserNamePerform: Users[Tasks[val].IdPerform].UserName,
			UserNameOwner:   Users[Tasks[val].IdOwn].UserName})
		if err != nil {
			log.Printf("Error template is: %v", err)
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

func startTaskBot(ctx context.Context) (resErr error) {
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

	log.Printf("Bot token: %v\n", Configuration.TelegramBotToken)
	bot, err := tgbotapi.NewBotAPI(Configuration.TelegramBotToken)
	if err != nil {
		return fmt.Errorf("NewBotAPI failed: %v", err)
	}

	bot.Debug = true

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
		port = "8081"
	}
	go func() {
		log.Fatalf("Http err: %v", http.ListenAndServe(":"+port, nil))
	}()
	log.Printf("start listen :%v\n", port)

	// В канал updates будут приходить все новые сообщения.
	for update := range updates {
		log.Printf("upd: %#v\n", update)
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
		command := update.Message.Text

		msgText := ""

		_, ok := Users[update.Message.From.ID]
		if !ok {
			Users[update.Message.From.ID] = User{
				UserName:       update.Message.From.UserName,
				TasksOwnId:     make([]int64, 0),
				TasksPerformId: make([]int64, 0),
			}
		}

		switch {
		case strings.HasPrefix(command, "/tasks"):
			{
				if len(Tasks) == 0 {
					msgText = "Нет задач"
					break
				}

				choseAll := make([]int64, 0)
				//слайс из Id всех задач
				for i := int64(0); i <= lastIdTask; i++ {
					_, ok := Tasks[i]
					if ok {
						choseAll = append(choseAll, i)
					}
				}
				msgText, err = writeMsgTasks(choseAll, update.Message.From.ID, true)
				if err != nil {
					return fmt.Errorf("writeMsgTasks: %v", err)
				}
			}
		case strings.HasPrefix(command, "/my"):
			{
				// add check ok and no nil Task perform
				msgText, err = writeMsgTasks(Users[update.Message.From.ID].TasksPerformId, update.Message.From.ID, false)
				if err != nil {
					return fmt.Errorf("writeMsgTasks: %v", err)
				}
			}
		case strings.HasPrefix(command, "/owner"):
			{
				msgText, err = writeMsgTasks(Users[update.Message.From.ID].TasksOwnId, update.Message.From.ID, false)
				if err != nil {
					return fmt.Errorf("writeMsgTasks: %v", err)
				}
			}
		case strings.HasPrefix(command, "/new"):
			{
				taskText := strings.Trim(command, "/new ")
				lastIdTask++
				Tasks[lastIdTask] = Task{Text: taskText,
					IdOwn: update.Message.From.ID}

				UserCur, ok := Users[update.Message.From.ID]
				if !ok {
					lastIdTask--
					return fmt.Errorf("user[%v] not exist(/new) %v", update.Message.From.ID, err)
				}
				UserCur.TasksOwnId = append(Users[update.Message.From.ID].TasksOwnId, lastIdTask)
				Users[update.Message.From.ID] = UserCur

				tmplNew, err = tmpl.Parse(TemplNewTask)
				if err != nil {
					return fmt.Errorf("tmpl.Parse(TemplNewTask): %v", err)
				}
				buf := bytes.Buffer{}
				err = tmplNew.Execute(&buf, struct {
					Task string
					Id   int64
				}{Task: taskText, Id: lastIdTask})
				msgText = buf.String()
				//msgText = strings.TrimRight(msgText, "\n")
				//msgText += "\n"
			}
		case strings.HasPrefix(command, "/assign_"):
			{
				tmpIdTask, err := strconv.Atoi(strings.Trim(command, "/assign_"))
				if err != nil {
					return fmt.Errorf("strconv.Atoi with idTask wrong(/assign_): %v", err)
				}
				idTask := int64(tmpIdTask)
				UserCur, ok := Users[update.Message.From.ID]
				if !ok {
					return fmt.Errorf("users[%v] not exist(/assign_) %v", update.Message.From.ID, err)
				}
				if idTask > lastIdTask || idTask < 1 {
					msgText = "Нет такой задачи"
				}

				if idPerform := Tasks[idTask].IdPerform; idPerform != 0 {
					userPerform, ok := Users[idPerform]
					if !ok {
						log.Printf("Users[%d] not exist", idPerform)
					}
					err = userPerform.DelElemFromSl("TasksPerformId", idTask)
					if err != nil {
						log.Printf("DelElemFromSl(\"TasksPerformId\", idTask) is failed. Error is: %v", err)
					}

					msg := tgbotapi.NewMessage(Tasks[idTask].IdPerform, fmt.Sprintf(`Задача "%v" назначена на @%v`, Tasks[idTask].Text, UserCur.UserName))
					_, err = bot.Send(msg)
					if err != nil {
						log.Printf("Send failed(assign) : %#v\n", update)
					}
				}
				if Tasks[idTask].IdPerform == 0 && update.Message.From.ID != Tasks[idTask].IdOwn {
					msg := tgbotapi.NewMessage(Tasks[idTask].IdOwn, fmt.Sprintf(`Задача "%v" назначена на @%v`, Tasks[idTask].Text, UserCur.UserName))
					_, err = bot.Send(msg)
					if err != nil {
						log.Printf("Send failed(assign if 2 ) : %#v\n", update)
					}
				}

				TmpTask, ok := Tasks[idTask]
				if !ok {
					log.Printf("Tasks[%d] not exist (assign)\n", idTask)
				}
				TmpTask.IdPerform = update.Message.From.ID
				Tasks[idTask] = TmpTask

				UserCur.TasksPerformId = append(UserCur.TasksPerformId, idTask)
				Users[update.Message.From.ID] = UserCur
				msgText = fmt.Sprintf(`Задача "%v" назначена на вас`, Tasks[idTask].Text)
			}
		case strings.HasPrefix(command, "/unassign_"):
			{
				tmpIdTask, err := strconv.Atoi(strings.Trim(command, "/unassign_"))
				if err != nil {
					return fmt.Errorf("strconv.Atoi with idTask wrong(/unassign_): %v", err)
				}
				UserCur, ok := Users[update.Message.From.ID]
				if !ok {
					return fmt.Errorf("users[%v] not exist(/unassign_) %v", update.Message.From.ID, err)
				}
				idTask := int64(tmpIdTask)
				if idTask > lastIdTask || idTask < 1 {
					msgText = "Нет такой задачи"
				}
				log.Printf("Slice %v, ID %v", UserCur.TasksPerformId, idTask)

				if idPerform := Tasks[idTask].IdPerform; idPerform == update.Message.From.ID {
					// Удаление элемента из слайса
					err = UserCur.DelElemFromSl("TasksPerformId", idTask)
					if err != nil {
						log.Printf("DelElemFromSl(\"TasksPerformId\", idTask) is failed. Error is: %v", err)
					}
					Users[update.Message.From.ID] = UserCur

					tmpTask, ok := Tasks[idTask]
					if !ok {
						log.Printf("Tasks[%d] not exist", idTask)
					}
					tmpTask.IdPerform = 0
					Tasks[idTask] = tmpTask

					msgText = "Принято"
					msg := tgbotapi.NewMessage(Tasks[idTask].IdOwn, fmt.Sprintf(`Задача "%v" осталась без исполнителя`, Tasks[idTask].Text))
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
				tmpTask, err := strconv.Atoi(strings.Trim(command, "/resolve_"))
				if err != nil {
					return fmt.Errorf("strconv.Atoi with idTask wrong(/resolve_): %v", err)
				}
				UserCur, ok := Users[update.Message.From.ID]
				if !ok {
					return fmt.Errorf("users[%v] not exist(/resolve_) %v", update.Message.From.ID, err)
				}
				idTask := int64(tmpTask)
				if idTask > lastIdTask || idTask < 0 {
					msgText = "Нет такой задачи"
				}
				if idPerform := Tasks[idTask].IdPerform; idPerform == update.Message.From.ID {
					log.Printf("Auniq")
					err = UserCur.DelElemFromSl("TasksPerformId", idTask)
					if err != nil {
						log.Printf("DelElemFromSl(\"TasksPerformId\", idTask) is failed. Error is: %v", err)
					}
					Users[update.Message.From.ID] = UserCur

					userOwn := Users[Tasks[idTask].IdOwn]
					err = userOwn.DelElemFromSl("TasksOwnId", idTask)
					if err != nil {
						log.Printf("DelElemFromSl(\"TasksOwnId\", idTask) is failed. Error is: %v", err)
					}
					Users[Tasks[idTask].IdOwn] = userOwn

					msgText = fmt.Sprintf(`Задача "%v" выполнена`, Tasks[idTask].Text)
					if update.Message.From.ID != Tasks[idTask].IdOwn {
						msg := tgbotapi.NewMessage(Tasks[idTask].IdOwn, fmt.Sprintf(`Задача "%v" выполнена @%v`, Tasks[idTask].Text, UserCur.UserName))
						_, err = bot.Send(msg)
						if err != nil {
							log.Printf("Send failed(unassign) : %#v\n", update)
						}
					}
					delete(Tasks, idTask)
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
		log.Printf("UPD SEND: %#v\n", update)
	}
	return nil

}

func main() {
	err := startTaskBot(context.Background())
	if err != nil {
		log.Printf("Error in startTaskBot: %v", err)
	}
}
