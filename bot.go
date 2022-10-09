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

const (
	RespOnInternalErr = "Внутренние ошибки на серверной части TaskBot. За помощью обращаться к https://t.me/Vilin0"
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

type Message struct {
	Text   string
	ChatId int64
	// На какое сообщение ответит бот, чтоб в общих чатах было попонятнее.
	ReplyToMessageID int
}

func (m *Message) sendMes(bot *tgbotapi.BotAPI) error {
	msg := tgbotapi.NewMessage(m.ChatId, m.Text)
	msg.ReplyToMessageID = m.ReplyToMessageID
	_, err := bot.Send(msg)
	if err != nil {
		return fmt.Errorf("send failed: %#v. In sendMes", m.Text)
	}

	return nil
}

type Task struct {
	Text      string
	IdOwn     int64
	IdPerform int64
}

func isTaskExists(idTask int64) bool {
	if idTask > lastIdTask || idTask < 1 {
		return false
	}
	for key := range Tasks {
		if key == idTask {
			return true
		}
	}

	return false
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
			return "", fmt.Errorf("error in tmpl.Execute is: %v", err)
		}
	}

	if buf.Bytes() != nil {
		msgText := buf.String()
		msgText = strings.TrimRight(msgText, "\n")
		return msgText, nil
	} else {
		return `Нет задач`, nil
	}
}

func startTaskBot(ctx context.Context) (resErr error) {
	// Читаю с конфига токен и WebHook url, чтобы в общедоступной репе его никто не забрал
	file, err := os.Open("config.json")
	if err != nil {
		return fmt.Errorf("open config.json failed: %v. In startTaskBot", err)
	}
	defer func() {
		err = file.Close()
		if err != nil {
			log.Printf("config.json don`t close: %v. In startTaskBot", err)
		}
	}()

	decoder := json.NewDecoder(file)
	err = decoder.Decode(&Configuration)
	if err != nil {
		return fmt.Errorf("decode failed: %s. In startTaskBot", err)
	}

	log.Printf("Bot token: %v\nWebhookUrl: %v. In startTaskBot", Configuration.TelegramBotToken, Configuration.WebhookURL)
	bot, err := tgbotapi.NewBotAPI(Configuration.TelegramBotToken)
	if err != nil {
		return fmt.Errorf("NewBotAPI failed: %v. In startTaskBot", err)
	}

	bot.Debug = true

	// Юзаем вебхуки, т.к. бесплатная heroku засыпает через какое-то время, если не отправлять запросы приложению.
	wh, err := tgbotapi.NewWebhook(Configuration.WebhookURL)
	if err != nil {
		return fmt.Errorf("NewWebhook failed: %v. In startTaskBot", err)
	}

	// Отправляем запрос tgApi, тем самым она теперь будет присылать изменения через вебхуку и бесплатная heroku не будет засыпать
	_, err = bot.Request(wh)
	if err != nil {
		return fmt.Errorf("SetWebhook failed: %v. In startTaskBot", err)
	}

	updates := bot.ListenForWebhook("/")

	log.Printf("Authorized on account %s. In startTaskBot", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	// Таймаут между запросами, чтобы их стало меньше и соответственно каждый быстрее обрабатывался
	u.Timeout = 60

	http.HandleFunc("/state", func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte("all is working"))
		if err != nil {
			log.Printf("Handlefunc /state error write:%v. In startTaskBot", err)
		}
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}
	go func() {
		log.Fatalf("Http err: %v. In startTaskBot", http.ListenAndServe(":"+port, nil))
	}()
	log.Printf("start listen :%v. In startTaskBot", port)

	// В канал updates будут приходить все новые сообщения.
	for update := range updates {
		log.Printf("Get update: %#v. In startTaskBot", update)
		if update.Message == nil {
			log.Printf("update.Message == nil: %#v\n. In startTaskBot", update)
			continue
		}
		if update.Message.Chat == nil {
			log.Printf("update.Message.Chat == nil: %#v\n. In startTaskBot", update)
			continue
		}
		if update.Message.From == nil {
			log.Printf("update.Message.From == nil: %#v\n. In startTaskBot", update)
			continue
		}
		command := update.Message.Text
		IDCurUser := update.Message.From.ID
		msgText := ""

		_, ok := Users[IDCurUser]
		if !ok {
			Users[IDCurUser] = User{
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
				msgText, err = writeMsgTasks(choseAll, IDCurUser, true)
				if err != nil {
					log.Printf("writeMsgTasks error is: %v. In /tasks. In startTaskBot", err)
					msgText = RespOnInternalErr
				}
			}
		case strings.HasPrefix(command, "/my"):
			{
				_, ok := Users[IDCurUser]
				if !ok {
					log.Printf("Users[%d] not exist. In /my. In startTaskBot", IDCurUser)
					msgText = RespOnInternalErr
					break
				}

				msgText, err = writeMsgTasks(Users[IDCurUser].TasksPerformId, IDCurUser, false)
				if err != nil {
					log.Printf("writeMsgTasks error is: %v. In /my. In startTaskBot", err)
					msgText = RespOnInternalErr
				}
			}
		case strings.HasPrefix(command, "/owner"):
			{
				_, ok := Users[IDCurUser]
				if !ok {
					log.Printf("Users[%d] not exist. In /owner. In startTaskBot", IDCurUser)
					msgText = RespOnInternalErr
					break
				}

				msgText, err = writeMsgTasks(Users[IDCurUser].TasksOwnId, IDCurUser, false)
				if err != nil {
					log.Printf("writeMsgTasks error is: %v. In /owner. In startTaskBot", err)
					msgText = RespOnInternalErr
				}
			}
		case strings.HasPrefix(command, "/new"):
			{
				taskText := strings.Trim(command, "/new ")
				if taskText == "" {
					msgText = "Давай те не будем пытаться создать пустую задачу. Какой в этом смысл?"
					break
				}
				lastIdTask++
				Tasks[lastIdTask] = Task{Text: taskText,
					IdOwn: IDCurUser}

				UserCur, ok := Users[IDCurUser]
				if !ok {
					lastIdTask--
					log.Printf("Users[%d] not exist. In /new. In startTaskBot", IDCurUser)
					msgText = RespOnInternalErr
					break
				}
				UserCur.TasksOwnId = append(Users[IDCurUser].TasksOwnId, lastIdTask)
				Users[IDCurUser] = UserCur

				tmplNew, err = tmpl.Parse(TemplNewTask)
				if err != nil {
					log.Printf("tmpl.Parse(TemplNewTask): %v. In /new. In startTaskBot", err)
					msgText = RespOnInternalErr
					break
				}
				buf := bytes.Buffer{}
				err = tmplNew.Execute(&buf, struct {
					Task string
					Id   int64
				}{Task: taskText, Id: lastIdTask})
				msgText = buf.String()
			}
		case strings.HasPrefix(command, "/assign_"):
			{
				tmpIdTask, err := strconv.Atoi(strings.Trim(command, "/assign_"))
				if err != nil {
					log.Printf("strconv.Atoi with tmpIdTask wrong. In /assign_: %v", err)
					msgText = "You write uncorrected value with /assign_"
					break
				}
				idTask := int64(tmpIdTask)
				UserCur, ok := Users[IDCurUser]
				if !ok {
					log.Printf("Users[%d] not exist. In /assign_. In startTaskBot", IDCurUser)
					msgText = RespOnInternalErr
					break
				}

				if !isTaskExists(idTask) {
					msgText = "Нет такой задачи"
					mes := Message{
						Text:             "Нет такой задачи",
						ChatId:           update.Message.Chat.ID,
						ReplyToMessageID: update.Message.MessageID,
					}
					err := mes.sendMes(bot)
					if err != nil {
						log.Printf("Error is: %+v. In /unassign(send perform). In startTaskBot", err)
					}
					continue
				}

				idPerform := Tasks[idTask].IdPerform
				if idPerform == IDCurUser {
					msgText = fmt.Sprintf(`Задача "%v" уже назначена на вас`, Tasks[idTask].Text)
					break
				}

				if idPerform != 0 {
					userPerform, ok := Users[idPerform]
					if !ok {
						log.Printf("Users[%d] not exist. In /assign_. In startTaskBot", idPerform)
						msgText = RespOnInternalErr
						break
					}
					err = userPerform.DelElemFromSl("TasksPerformId", idTask)
					if err != nil {
						log.Printf("DelElemFromSl(\"TasksPerformId\", idTask) is failed in /assign_. Error is: %v. In /assign_. In startTaskBot", err)
						msgText = RespOnInternalErr
						break
					}

					mes := Message{Text: fmt.Sprintf(`Задача "%v" назначена на @%v`, Tasks[idTask].Text, UserCur.UserName),
						ChatId: Tasks[idTask].IdPerform, ReplyToMessageID: 0}
					err = mes.sendMes(bot)
					if err != nil {
						log.Printf("Error is: %+v. In /assign(send perform). In startTaskBot", err)
						continue
					}
				}

				if Tasks[idTask].IdPerform == 0 && IDCurUser != Tasks[idTask].IdOwn {
					mes := Message{Text: fmt.Sprintf(`Задача "%v" назначена на @%v`, Tasks[idTask].Text, UserCur.UserName),
						ChatId:           Tasks[idTask].IdOwn,
						ReplyToMessageID: 0}
					err = mes.sendMes(bot)
					if err != nil {
						log.Printf("Error is: %+v. In /assign(send own). In startTaskBot", err)
						continue
					}
				}

				TmpTask, ok := Tasks[idTask]
				if !ok {
					log.Printf("Tasks[%d] not exist (assign)\n", idTask)
					msgText = RespOnInternalErr
					break
				}
				TmpTask.IdPerform = IDCurUser
				Tasks[idTask] = TmpTask

				UserCur.TasksPerformId = append(UserCur.TasksPerformId, idTask)
				Users[IDCurUser] = UserCur
				msgText = fmt.Sprintf(`Задача "%v" назначена на вас`, Tasks[idTask].Text)
			}
		case strings.HasPrefix(command, "/unassign_"):
			{
				tmpIdTask, err := strconv.Atoi(strings.Trim(command, "/unassign_"))
				if err != nil {
					log.Printf("strconv.Atoi with tmpIdTask wrong. In /unassign_: %v", err)
					msgText = "You write uncorrected value with /unassign_"
					break
				}
				UserCur, ok := Users[IDCurUser]
				if !ok {
					log.Printf("Users[%d] not exist. In /unassign_. In startTaskBot", IDCurUser)
					msgText = RespOnInternalErr
					break
				}
				idTask := int64(tmpIdTask)
				if !isTaskExists(idTask) {
					msgText = "Нет такой задачи"
					mes := Message{
						Text:             "Нет такой задачи",
						ChatId:           update.Message.Chat.ID,
						ReplyToMessageID: update.Message.MessageID,
					}
					err := mes.sendMes(bot)
					if err != nil {
						log.Printf("Error is: %+v. In /unassign(send perform). In startTaskBot", err)
					}
					continue
				}

				if idPerform := Tasks[idTask].IdPerform; idPerform == IDCurUser {
					// Удаление элемента из слайса
					err = UserCur.DelElemFromSl("TasksPerformId", idTask)
					if err != nil {
						log.Printf("DelElemFromSl(\"TasksPerformId\", idTask) is failed in /unassign_. Error is: %v", err)
						msgText = RespOnInternalErr
						break
					}
					Users[IDCurUser] = UserCur

					tmpTask, ok := Tasks[idTask]
					if !ok {
						log.Printf("Tasks[%d] not exist", idTask)
						msgText = RespOnInternalErr
						break
					}
					tmpTask.IdPerform = 0
					Tasks[idTask] = tmpTask

					msg := Message{
						Text:             fmt.Sprintf(`Задача "%v" осталась без исполнителя`, Tasks[idTask].Text),
						ChatId:           Tasks[idTask].IdOwn,
						ReplyToMessageID: 0,
					}
					err := msg.sendMes(bot)
					if err != nil {
						log.Printf("Error is: %+v. In /unassign(send perform). In startTaskBot", err)
						continue
					}

					msgText = "Принято"
				} else {
					msgText = `Задача не на вас`
				}
			}
		case strings.HasPrefix(command, "/resolve_"):
			{
				tmpTask, err := strconv.Atoi(strings.Trim(command, "/resolve_"))
				if err != nil {
					log.Printf("strconv.Atoi with tmpIdTask wrong. In /resolve_: %v", err)
					msgText = "You write uncorrected value with /resolve"
					break
				}
				UserCur, ok := Users[IDCurUser]
				if !ok {
					log.Printf("Users[%d] not exist. In /resolve_. In startTaskBot", IDCurUser)
					msgText = RespOnInternalErr
					break
				}
				idTask := int64(tmpTask)
				if !isTaskExists(idTask) {
					msgText = "Нет такой задачи"
					mes := Message{
						Text:             "Нет такой задачи",
						ChatId:           update.Message.Chat.ID,
						ReplyToMessageID: update.Message.MessageID,
					}
					err := mes.sendMes(bot)
					if err != nil {
						log.Printf("Error is: %+v. In /unassign(send perform). In startTaskBot", err)
					}
					continue
				}

				if idPerform := Tasks[idTask].IdPerform; idPerform == IDCurUser {
					err = UserCur.DelElemFromSl("TasksPerformId", idTask)
					if err != nil {
						log.Printf("DelElemFromSl(\"TasksPerformId\", idTask) is failed in /resolve_. Error is: %v", err)
						msgText = RespOnInternalErr
						break
					}
					Users[IDCurUser] = UserCur

					userOwn := Users[Tasks[idTask].IdOwn]
					err = userOwn.DelElemFromSl("TasksOwnId", idTask)
					if err != nil {
						log.Printf("DelElemFromSl(\"TasksOwnId\", idTask) is failed in /resolve_. Error is: %v", err)
						msgText = RespOnInternalErr
						break
					}
					Users[Tasks[idTask].IdOwn] = userOwn

					msgText = fmt.Sprintf(`Задача "%v" выполнена`, Tasks[idTask].Text)
					if IDCurUser != Tasks[idTask].IdOwn {
						mes := Message{
							Text:             fmt.Sprintf(`Задача "%v" выполнена @%v`, Tasks[idTask].Text, UserCur.UserName),
							ChatId:           Tasks[idTask].IdOwn,
							ReplyToMessageID: 0,
						}
						err = mes.sendMes(bot)
						if err != nil {
							log.Printf("Error is: %+v. In /resolve_ (send own). In startTaskBot", err)
						}
					}
					delete(Tasks, idTask)
				} else {
					msgText = `Задача не на вас`
				}
			}
		case command == "/help":
			{
				msgText =
					`/tasks - выдает описание всех текущих задач и за кем они закреплены.

					/new *** - создает новую задачу, где вместо звездочек вы пишете название задачи.

					/assign_*  - прикрепляет задачу к вам, чтобы вы могли её выполнить. Вместо звездочки напишите номер существующей задачи.

					/unassign_* - открепляет задачу от вас. Вместо звездочки напишите номер существующей задачи.
	
					/resolve_* - решает задачу. Удаляет задачу из списка. Вместо звездочки напишите номер существующей задачи.

					/my - показывает задачи, закрепленные за вами.

					/owner - показывает задачи, созданные вами.
						`
			}
		default:
			msgText = "Нет такой команды. /help для отображения описания команд."
		}

		msg := Message{
			Text:             msgText,
			ChatId:           update.Message.Chat.ID,
			ReplyToMessageID: update.Message.MessageID,
		}
		err = msg.sendMes(bot)
		if err != nil {
			log.Printf("Error is: %+v. In general startTaskBot", err)
		}
		log.Printf("Update secessfully send: %#v\n", update)
	}
	return nil

}

func main() {
	err := startTaskBot(context.Background())
	if err != nil {
		log.Printf("Error in main: %v", err)
	}
}
