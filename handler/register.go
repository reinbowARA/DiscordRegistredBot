package handler

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// Обработчик нового участника
func (sc *ServerConfig) NewGuildMember(s *discordgo.Session, m *discordgo.GuildMemberAdd) {
	// Получаем конфигурацию сервера
	serverConfig, exists := GetServerConfig(m.GuildID)
	if !exists {
		logger.Warn("Конфигурация сервера не найдена для гильдии " + m.GuildID)
		return
	}

	// Выдаем роль регистрации
	roleID := findRoleID(s, m.GuildID, serverConfig.RegistrationRole)
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
	channel, err := serverConfig.createPrivateChannel(s, m.Member)
	if err != nil {
		logger.Error("Ошибка создания канала: " + err.Error())
		return
	}

	// Получаем конфигурацию регистрации
	regConfig, exists := GetRegistrationConfig(m.GuildID)
	if !exists {
		logger.Warn("Конфигурация регистрации не найдена для гильдии " + m.GuildID)
		return
	}

	// Находим первый вопрос
	var firstQuestion *Question
	minOrder := int(^uint(0) >> 1) // max int
	for i := range regConfig.Questions {
		if regConfig.Questions[i].Order < minOrder {
			minOrder = regConfig.Questions[i].Order
			firstQuestion = &regConfig.Questions[i]
		}
	}

	if firstQuestion == nil {
		logger.Error("Первый вопрос не найден")
		return
	}

	// Инициализация состояния
	mu.Lock()
	session := &UserSession{
		UserID:     m.User.ID,
		ChannelID:  channel.ID,
		CurrentQID: firstQuestion.ID,
		Answers:    make(map[string]UserAnswer),
		Data:       make(map[string]interface{}),
		StartedAt:  time.Now().Unix(),
	}
	registeringUsers[m.User.ID] = session
	mu.Unlock()

	logger.Info("Пользователь ID:" + m.User.ID + "(" + m.User.Username + ") начал регистрацию")
	// Запускаем первый вопрос
	sc.sendNextQuestion(s, session, channel.ID, m.User.ID, regConfig)
}

// Отправка следующего вопроса
func (sc *ServerConfig) sendNextQuestion(s *discordgo.Session, session *UserSession, channelID, userID string, regConfig *RegistrationConfig) {
	// Находим текущий вопрос
	var currentQuestion *Question
	for i := range regConfig.Questions {
		if regConfig.Questions[i].ID == session.CurrentQID {
			currentQuestion = &regConfig.Questions[i]
			break
		}
	}

	if currentQuestion == nil {
		logger.Error("Вопрос не найден: " + session.CurrentQID)
		return
	}

	// Форматируем вопрос
	message := currentQuestion.Text
	if currentQuestion.Type == "single_choice" || currentQuestion.Type == "multiple_choice" {
		message += "\n\n**Варианты ответа:**"
		for _, option := range currentQuestion.Options {
			message += fmt.Sprintf("\n`%s` - %s", option.ID, option.Text)
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
		// Определяем GuildID для контекста
		var guildID string
		if m.GuildID != "" {
			guildID = m.GuildID
		} else {
			// Если GuildID не доступен напрямую, попробуем получить его через канал
			channel, err := s.Channel(m.ChannelID)
			if err == nil && channel.GuildID != "" {
				guildID = channel.GuildID
			}
		}
		
		if guildID != "" {
			// Если канал команд задан, проверяем, что команда вызвана в этом канале
			if sc.CommandChannelID != "" && m.ChannelID != sc.CommandChannelID {
				s.ChannelMessageSend(m.ChannelID, "Команда !init доступна только в канале для команд")
				return
			}
			handleInitCommand(s, m, guildID)
		}
		return
	}

	// Обработка команд администратора
	if sc.CommandChannelID != "" && m.ChannelID == sc.CommandChannelID && strings.HasPrefix(m.Content, "!") {
		sc.handleAdminCommand(s, m)
		return
	}

	// Обработка сообщений в процессе регистрации
	mu.Lock()
	session, ok := registeringUsers[m.Author.ID]
	mu.Unlock()

	if ok && m.ChannelID == session.ChannelID {
		regConfig, exists := GetRegistrationConfig(sc.GuildID)
		if !exists {
			logger.Error("Конфигурация регистрации не найдена")
			return
		}
		sc.processRegistrationAnswer(s, m, session, regConfig)
	}
}

// Обработка ответа на вопрос регистрации
func (sc *ServerConfig) processRegistrationAnswer(s *discordgo.Session, m *discordgo.MessageCreate, session *UserSession, regConfig *RegistrationConfig) {
	answer := strings.TrimSpace(m.Content)

	// Находим текущий вопрос
	var currentQuestion *Question
	for i := range regConfig.Questions {
		if regConfig.Questions[i].ID == session.CurrentQID {
			currentQuestion = &regConfig.Questions[i]
			break
		}
	}

	if currentQuestion == nil {
		logger.Error("Текущий вопрос не найден: " + session.CurrentQID)
		return
	}

	// Валидация ответа
	if !sc.validateAnswer(answer, currentQuestion) {
		s.ChannelMessageSend(m.ChannelID, "Пожалуйста, введите корректный ответ.")
		return
	}

	// Сохраняем ответ
	userAnswer := UserAnswer{
		QuestionID: currentQuestion.ID,
		Value:      answer,
	}

	// Для choice типов находим выбранный вариант
	if currentQuestion.Type == "single_choice" {
		for _, option := range currentQuestion.Options {
			if option.ID == answer {
				userAnswer.Selected = &option
				break
			}
		}
	}

	session.Answers[currentQuestion.ID] = userAnswer

	// Выполняем действия
	sc.executeActions(s, m.Author.ID, currentQuestion.Actions, &userAnswer, session)

	// Определяем следующий вопрос
	nextQID := sc.getNextQuestionID(currentQuestion, session, regConfig)
	if nextQID == "" || nextQID == "end" {
		// Завершаем регистрацию
		sc.completeRegistration(s, session, m.Author.ID, regConfig)
		return
	}

	// Устанавливаем следующий вопрос
	session.CurrentQID = nextQID

	// Отправляем следующий вопрос
	sc.sendNextQuestion(s, session, m.ChannelID, m.Author.ID, regConfig)
}

// Завершение регистрации
func (sc *ServerConfig) completeRegistration(s *discordgo.Session, session *UserSession, userID string, regConfig *RegistrationConfig) {
	channelID := session.ChannelID

	// Выполняем действия завершения
	if regConfig.Completion.Actions != nil {
		for _, action := range regConfig.Completion.Actions {
			sc.executeCompletionAction(s, userID, action, session)
		}
	}

	// Удаляем роль регистрации
	serverConfig, _ := GetServerConfig(sc.GuildID)
	if serverConfig != nil && serverConfig.RegistrationRole != "" {
		roleID := findRoleID(s, sc.GuildID, serverConfig.RegistrationRole)
		if roleID != "" {
			_ = s.GuildMemberRoleRemove(sc.GuildID, userID, roleID)
		}
	}

	// Отправляем сообщение завершения
	s.ChannelMessageSend(channelID, regConfig.Completion.Message)
	logger.Info("Пользователь ID:" + userID + " завершил регистрацию!")

	// Удаление канала
	go func() {
		time.Sleep(30 * time.Second)
		mu.Lock()
		delete(registeringUsers, userID)
		mu.Unlock()
		_, _ = s.ChannelDelete(channelID)
	}()
}

// Валидация ответа
func (sc *ServerConfig) validateAnswer(answer string, question *Question) bool {
	if question.Required && answer == "" {
		return false
	}

	switch question.Type {
	case "single_choice", "multiple_choice":
		// Проверяем, что ответ является одним из ID вариантов
		for _, option := range question.Options {
			if option.ID == answer {
				return true
			}
		}
		return false
	case "text_input":
		if question.Validation != nil {
			if question.Validation.MinLength > 0 && len(answer) < question.Validation.MinLength {
				return false
			}
			if question.Validation.MaxLength > 0 && len(answer) > question.Validation.MaxLength {
				return false
			}
			if question.Validation.Regex != "" {
				// Для простоты пропустим проверку regex
			}
		}
		return true
	case "number_input":
		if question.Validation != nil {
			num, err := strconv.Atoi(answer)
			if err != nil {
				return false
			}
			if question.Validation.MinValue != 0 && num < question.Validation.MinValue {
				return false
			}
			if question.Validation.MaxValue != 0 && num > question.Validation.MaxValue {
				return false
			}
		}
		return true
	default:
		return true
	}
}

// Выполнение действий
func (sc *ServerConfig) executeActions(s *discordgo.Session, userID string, actions []Action, userAnswer *UserAnswer, session *UserSession) {
	for _, action := range actions {
		switch action.Type {
		case "assign_role":
			roleID := sc.resolveTemplate(action.RoleID, userAnswer, session)
			if roleID != "" {
				actualRoleID := findRoleID(s, sc.GuildID, roleID)
				if actualRoleID != "" {
					s.GuildMemberRoleAdd(sc.GuildID, userID, actualRoleID)
				}
			}
		case "save_answer":
			value := sc.resolveTemplate(action.Value, userAnswer, session)
			if action.Storage == "permanent" {
				// Сохраняем в session.Data для постоянного хранения
				session.Data[action.Field] = value
			}
		case "change_nickname":
			nickname := sc.resolveTemplate(action.Format, userAnswer, session)
			s.GuildMemberNickname(sc.GuildID, userID, nickname)
		}
	}
}

// Выполнение действий завершения
func (sc *ServerConfig) executeCompletionAction(s *discordgo.Session, userID string, action Action, session *UserSession) {
	// Аналогично executeActions, но без userAnswer
	switch action.Type {
	case "assign_role":
		roleID := sc.resolveTemplate(action.RoleID, nil, session)
		if roleID != "" {
			actualRoleID := findRoleID(s, sc.GuildID, roleID)
			if actualRoleID != "" {
				s.GuildMemberRoleAdd(sc.GuildID, userID, actualRoleID)
			}
		}
	}
}

// Разрешение шаблонов
func (sc *ServerConfig) resolveTemplate(template string, userAnswer *UserAnswer, session *UserSession) string {
	if template == "" {
		return ""
	}

	result := template
	if userAnswer != nil {
		if userAnswer.Selected != nil {
			result = strings.ReplaceAll(result, "@selected.id", userAnswer.Selected.ID)
			result = strings.ReplaceAll(result, "@selected.role_id", userAnswer.Selected.RoleID)
			result = strings.ReplaceAll(result, "@selected.text", userAnswer.Selected.Text)
		}
		if val, ok := userAnswer.Value.(string); ok {
			result = strings.ReplaceAll(result, "@input", val)
		}
	}

	// Заменяем на значения из session.Data
	for key, value := range session.Data {
		if str, ok := value.(string); ok {
			result = strings.ReplaceAll(result, "{"+key+"}", str)
		}
	}

	return result
}

// Определение следующего вопроса
func (sc *ServerConfig) getNextQuestionID(question *Question, session *UserSession, regConfig *RegistrationConfig) string {
	if question.Next.Type == "static" {
		return question.Next.QuestionID
	}

	if question.Next.Type == "conditional" {
		for _, condition := range question.Next.Conditions {
			if sc.checkCondition(condition.If, session) {
				return condition.QuestionID
			}
		}
		return question.Next.Default
	}

	return "end"
}

// Проверка условия
func (sc *ServerConfig) checkCondition(check ConditionCheck, session *UserSession) bool {
	answer, exists := session.Answers[check.Field]
	if !exists {
		return false
	}

	value := ""
	if val, ok := answer.Value.(string); ok {
		value = val
	}

	switch check.Operator {
	case "equals":
		return value == check.Value
	case "not_equals":
		return value != check.Value
	case "contains":
		return strings.Contains(value, check.Value.(string))
	default:
		return false
	}
}
