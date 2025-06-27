package handler

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// Обработчик нового участника
func (sc *ServerConfig) NewGuildMember(s *discordgo.Session, m *discordgo.GuildMemberAdd) {
	if m.GuildID != sc.GuildID {
		return
	}

	// Выдаем роль регистрации
	roleID := findRoleID(s, m.GuildID, sc.RegistrationRole)
	if roleID == "" {
		logger.Warn("Роль 'Регистрация' не найдена!")
		return
	}

	err := s.GuildMemberRoleAdd(m.GuildID, m.User.ID, roleID)
	if err != nil {
		logger.Error("Ошибка выдачи роли: " + err.Error())
		return
	}

	// Создаем приватный канал
	channel, err := sc.createPrivateChannel(s, m.Member)
	if err != nil {
		logger.Error("Ошибка создания канала: " + err.Error())
		return
	}

	// Инициализация состояния
	mu.Lock()
	state := &RegistrationState{
		Step:      0,
		ChannelID: channel.ID,
		Answers:   []string{},
	}
	registeringUsers[m.User.ID] = state
	mu.Unlock()
	logger.Info("Пользователь ID:" + m.User.ID + "(" + m.User.Username + ") начал регистрацию")
	// Запускаем первый вопрос
	sc.sendNextQuestion(s, state, channel.ID, m.User.ID)
}

// Отправка следующего вопроса
func (sc *ServerConfig) sendNextQuestion(s *discordgo.Session, state *RegistrationState, channelID, userID string) {
	if state.Step >= len(questions) {
		sc.completeRegistration(s, state, userID)
		return
	}

	// Получаем текущий вопрос
	currentQuestion := questions[state.Step]
	state.CurrentQuestion = &currentQuestion

	// Форматируем вопрос
	message := currentQuestion.Question
	if currentQuestion.Switch != nil {
		message += "\n\n**Варианты ответа:**"
		keys := make([]int, 0, len(currentQuestion.Switch))
		for k := range currentQuestion.Switch {
			key, _ := strconv.Atoi(k)
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool {
			return keys[i] < keys[j]
		})
		for _, key := range keys {
			message += fmt.Sprintf("\n`%d` - %s", key, currentQuestion.Switch[strconv.Itoa(key)])
		}
	}

	s.ChannelMessageSend(channelID, message)
}

// Создание приватного канала
func (sc *ServerConfig) createPrivateChannel(s *discordgo.Session, member *discordgo.Member) (*discordgo.Channel, error) {
	channelName := "регистрация-" + strings.ToLower(member.User.Username)

	channelData := discordgo.GuildChannelCreateData{
		Name:     channelName,
		Type:     discordgo.ChannelTypeGuildText,
		ParentID: sc.CategoryID,
		PermissionOverwrites: []*discordgo.PermissionOverwrite{
			{ID: member.User.ID, Type: discordgo.PermissionOverwriteTypeMember,
				Allow: discordgo.PermissionViewChannel | discordgo.PermissionSendMessages},
			{ID: s.State.User.ID, Type: discordgo.PermissionOverwriteTypeMember,
				Allow: discordgo.PermissionAll},
			{ID: sc.GuildID, Type: discordgo.PermissionOverwriteTypeRole,
				Deny: discordgo.PermissionViewChannel},
		},
	}

	return s.GuildChannelCreateComplex(sc.GuildID, channelData)
}

// Обработчик сообщений
func (sc *ServerConfig) MessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Игнорируем сообщения ботов
	if m.Author.Bot {
		return
	}

	// Обработка команды !init
	if strings.HasPrefix(m.Content, "!init") {
		sc.handleInitCommand(s, m)
		return
	}

	// Обработка команд администратора
	if m.ChannelID == sc.CommandChannelID && strings.HasPrefix(m.Content, "!") {
		sc.handleAdminCommand(s, m)
		return
	}

	// Обработка сообщений в процессе регистрации
	mu.Lock()
	state, ok := registeringUsers[m.Author.ID]
	mu.Unlock()

	if ok && m.ChannelID == state.ChannelID && state.CurrentQuestion != nil {
		sc.processRegistrationAnswer(s, m, state)
	}
}

// Обработка ответа на вопрос регистрации
func (sc *ServerConfig) processRegistrationAnswer(s *discordgo.Session, m *discordgo.MessageCreate, state *RegistrationState) {
	answer := strings.TrimSpace(m.Content)
	q := state.CurrentQuestion

	// Валидация ответа
	if q.Result == "int" {
		valid := false
		for key := range q.Switch {
			if key == answer {
				valid = true
				break
			}
		}
		if !valid {
			options := []string{}
			for key := range q.Switch {
				options = append(options, "`"+key+"`")
			}
			s.ChannelMessageSend(m.ChannelID,
				"Пожалуйста, выбери один из предложенных вариантов: "+
					strings.Join(options, ", "))
			return
		}
	}

	// Сохраняем ответ
	state.Answers = append(state.Answers, answer)
	state.Step++

	// Переходим к следующему вопросу
	sc.sendNextQuestion(s, state, m.ChannelID, m.Author.ID)
}

// Завершение регистрации
func (sc *ServerConfig) completeRegistration(s *discordgo.Session, state *RegistrationState, userID string) {
	channelID := state.ChannelID

	// Извлекаем ник из первого ответа
	nickParts := strings.Split(state.Answers[0], "(")
	nickname := strings.TrimSpace(nickParts[0])

	// Меняем никнейм
	err := s.GuildMemberNickname(sc.GuildID, userID, nickname)
	if err != nil {
		logger.Error("Ошибка при смене ника: " + err.Error())
		s.ChannelMessageSend(channelID, "Ошибка при смене ника: "+err.Error())
	} else {
		s.ChannelMessageSend(channelID, "Твой ник успешно изменен на: "+nickname)
	}

	// Удаляем роль регистрации
	_ = s.GuildMemberRoleRemove(sc.GuildID, userID, sc.RegistrationRole)

	// Формируем сводку
	summary := "🎉 Регистрация завершена!\n"
	for i, answer := range state.Answers {
		q := questions[i]
		if q.Switch != nil {
			if answer == "1" {
				err = s.GuildMemberRoleAdd(sc.GuildID, userID, sc.GuildRoleId)
				if err != nil {
					fmt.Println(err.Error())
				}
			} else if answer == "2" {
				err = s.GuildMemberRoleAdd(sc.GuildID, userID, sc.FriendRoleId)
				if err != nil {
					fmt.Println(err.Error())
				}
			}
		}
	}
	s.ChannelMessageSend(channelID, summary)
	logger.Info("Пользователь ID:" + userID + " завершил регистрацию!")
	// Отправляем дополнительную информацию по выбору
	handleChoice(s, state, channelID)

	// Удаление канала
	go func() {
		time.Sleep(30 * time.Second)
		mu.Lock()
		delete(registeringUsers, userID)
		mu.Unlock()
		_, _ = s.ChannelDelete(channelID)
	}()
}

// Обработка выбора пользователя
func handleChoice(s *discordgo.Session, state *RegistrationState, channelID string) {
	if len(state.Answers) < 2 {
		return
	}

	choice := state.Answers[1]
	switch choice {
	case "1":
		s.ChannelMessageSend(channelID, "\nДобро пожаловать в нашу гильдию! Ознакомься с правилами в соответствующем канале.")
	case "2":
		s.ChannelMessageSend(channelID, "\nМы рады сотрудничать! оставьте расписание в `общий` чат")
	case "3":
		s.ChannelMessageSend(channelID, "\nСпасибо за интерес! Наши представители ответят на твои вопросы в ближайшее время.")
	}
}
