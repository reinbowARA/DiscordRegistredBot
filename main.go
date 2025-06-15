package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
)

// Конфигурация
const (
	BotToken          = "TOKEN"
	GuildID           = "ID_GUILD"
	RegistrationRole  = "Регистрация"
	CategoryID        = "CATEGORY_ID"
	QuestionsFilePath = "question.json"
	CommandChannelID  = "ID_CHANEL" // ID канала для команд
)

// Структуры для вопросов
type Question struct {
	Question string            `json:"question"`
	Result   string            `json:"result"`
	Switch   map[string]string `json:"switch,omitempty"`
}

// Состояние регистрации
type RegistrationState struct {
	Step            int
	ChannelID       string
	Answers         []string
	CurrentQuestion *Question
}

var (
	questions        []Question
	registeringUsers = make(map[string]*RegistrationState)
	mu               sync.Mutex // Мьютекс для безопасного доступа к регистрирующимся пользователям
)

func main() {
	// Загружаем вопросы
	if err := loadQuestions(); err != nil {
		fmt.Println("Ошибка загрузки вопросов:", err)
		return
	}

	session, err := discordgo.New("Bot " + BotToken)
	if err != nil {
		fmt.Println("Ошибка создания сессии:", err)
		return
	}

	session.AddHandler(newGuildMember)
	session.AddHandler(messageCreate)

	session.Identify.Intents = discordgo.IntentsGuildMessages |
		discordgo.IntentsGuildMembers |
		discordgo.IntentsGuilds

	err = session.Open()
	if err != nil {
		fmt.Println("Ошибка подключения:", err)
		return
	}
	defer session.Close()

	fmt.Println("Бот запущен! Для остановки Ctrl+C")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}

// Загрузка вопросов из JSON
func loadQuestions() error {
	file, err := os.ReadFile(QuestionsFilePath)
	if err != nil {
		return err
	}
	return json.Unmarshal(file, &questions)
}

// Обработчик нового участника
func newGuildMember(s *discordgo.Session, m *discordgo.GuildMemberAdd) {
	if m.GuildID != GuildID {
		return
	}

	// Выдаем роль регистрации
	roleID := findRoleID(s, m.GuildID, RegistrationRole)
	if roleID == "" {
		fmt.Println("Роль 'Регистрация' не найдена!")
		return
	}

	err := s.GuildMemberRoleAdd(m.GuildID, m.User.ID, roleID)
	if err != nil {
		fmt.Println("Ошибка выдачи роли:", err)
		return
	}

	// Создаем приватный канал
	channel, err := createPrivateChannel(s, m.Member)
	if err != nil {
		fmt.Println("Ошибка создания канала:", err)
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

	// Запускаем первый вопрос
	sendNextQuestion(s, state, channel.ID, m.User.ID)
}

// Отправка следующего вопроса
func sendNextQuestion(s *discordgo.Session, state *RegistrationState, channelID, userID string) {
	if state.Step >= len(questions) {
		completeRegistration(s, state, userID)
		return
	}

	// Получаем текущий вопрос
	currentQuestion := questions[state.Step]
	state.CurrentQuestion = &currentQuestion

	// Форматируем вопрос
	message := currentQuestion.Question
	if currentQuestion.Switch != nil {
		message += "\n\n**Варианты ответа:**"
		for key, value := range currentQuestion.Switch {
			message += fmt.Sprintf("\n`%s` - %s", key, value)
		}
	}

	s.ChannelMessageSend(channelID, message)
}

// Создание приватного канала
func createPrivateChannel(s *discordgo.Session, member *discordgo.Member) (*discordgo.Channel, error) {
	channelName := "регистрация-" + strings.ToLower(member.User.Username)

	channelData := discordgo.GuildChannelCreateData{
		Name:     channelName,
		Type:     discordgo.ChannelTypeGuildText,
		ParentID: CategoryID,
		PermissionOverwrites: []*discordgo.PermissionOverwrite{
			{ID: member.User.ID, Type: discordgo.PermissionOverwriteTypeMember,
				Allow: discordgo.PermissionViewChannel | discordgo.PermissionSendMessages},
			{ID: s.State.User.ID, Type: discordgo.PermissionOverwriteTypeMember,
				Allow: discordgo.PermissionAll},
			{ID: GuildID, Type: discordgo.PermissionOverwriteTypeRole,
				Deny: discordgo.PermissionViewChannel},
		},
	}

	return s.GuildChannelCreateComplex(GuildID, channelData)
}

// Обработчик сообщений
func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Игнорируем сообщения ботов
	if m.Author.Bot {
		return
	}

	// Обработка команд администратора
	if m.ChannelID == CommandChannelID && strings.HasPrefix(m.Content, "!") {
		handleAdminCommand(s, m)
		return
	}

	// Обработка сообщений в процессе регистрации
	mu.Lock()
	state, ok := registeringUsers[m.Author.ID]
	mu.Unlock()

	if ok && m.ChannelID == state.ChannelID && state.CurrentQuestion != nil {
		processRegistrationAnswer(s, m, state)
	}
}

// Обработка ответа на вопрос регистрации
func processRegistrationAnswer(s *discordgo.Session, m *discordgo.MessageCreate, state *RegistrationState) {
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
				"⚠️ Пожалуйста, выбери один из предложенных вариантов: "+
					strings.Join(options, ", "))
			return
		}
	}

	// Сохраняем ответ
	state.Answers = append(state.Answers, answer)
	state.Step++

	// Переходим к следующему вопросу
	sendNextQuestion(s, state, m.ChannelID, m.Author.ID)
}

// Завершение регистрации
func completeRegistration(s *discordgo.Session, state *RegistrationState, userID string) {
	channelID := state.ChannelID

	// Извлекаем ник из первого ответа
	nickParts := strings.Split(state.Answers[0], "(")
	nickname := strings.TrimSpace(nickParts[0])

	// Меняем никнейм
	err := s.GuildMemberNickname(GuildID, userID, nickname)
	if err != nil {
		s.ChannelMessageSend(channelID, "⚠️ Ошибка при смене ника: "+err.Error())
	} else {
		s.ChannelMessageSend(channelID, "✅ Твой ник успешно изменен на: "+nickname)
	}

	// Удаляем роль регистрации
	if roleID := findRoleID(s, GuildID, RegistrationRole); roleID != "" {
		_ = s.GuildMemberRoleRemove(GuildID, userID, roleID)
	}

	// Формируем сводку
	summary := "🎉 Регистрация завершена!\n\n**Твои ответы:**\n"
	for i, answer := range state.Answers {
		q := questions[i]
		if q.Switch != nil {
			answer = q.Switch[answer] + " (" + answer + ")"
			if answer == "1" {
				err = s.GuildMemberRoleAdd(GuildID, userID, "1383890782355849296")
				if err != nil {
					fmt.Println(err.Error())
				}
			} else if answer == "2"{
				err = s.GuildMemberRoleAdd(GuildID, userID, "1383891120332734585")
				if err != nil {
					fmt.Println(err.Error())
				}
			}
		}
		summary += fmt.Sprintf("%d. **%s**\n   → %s\n", i+1, q.Question, answer)
	}
	s.ChannelMessageSend(channelID, summary)

	// Отправляем дополнительную информацию по выбору
	handleChoice(s, state, channelID, userID)

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
func handleChoice(s *discordgo.Session, state *RegistrationState, channelID, userID string) {
	if len(state.Answers) < 2 {
		return
	}

	choice := state.Answers[1]
	switch choice {
	case "1":
		s.ChannelMessageSend(channelID, "\nДобро пожаловать в нашу гильдию! Ознакомься с правилами в соответствующем канале.")
	case "2":
		s.ChannelMessageSend(channelID, "\nМы рады сотрудничать! Наш офицер по найму свяжется с тобой в ближайшее время.")
	case "3":
		s.ChannelMessageSend(channelID, "\nСпасибо за интерес! Наши представители ответят на твои вопросы в ближайшее время.")
	}
}

// Поиск ID роли по имени
func findRoleID(s *discordgo.Session, guildID, roleName string) string {
	roles, err := s.GuildRoles(guildID)
	if err != nil {
		fmt.Println("Ошибка получения ролей:", err)
		return ""
	}

	for _, role := range roles {
		if strings.EqualFold(role.Name, roleName) {
			return role.ID
		}
	}
	return ""
}

// Обработка команд администратора
func handleAdminCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Проверка прав администратора
	if !isAdmin(s, m) {
		s.ChannelMessageSend(m.ChannelID, "❌ У вас недостаточно прав для выполнения этой команды")
		return
	}

	switch strings.ToLower(m.Content) {
	case "!clsroles":
		removeAllRoles(s, m)

	case "!startregistred":
		startRegistrationForUnregistered(s, m)

	case "!stopregistred":
		stopAllRegistrations(s, m)

	case "!help":
		showHelp(s, m)

	default:
		s.ChannelMessageSend(m.ChannelID, "❌ Неизвестная команда. Используй `!help` для списка команд")
	}
}

// Проверка прав администратора
func isAdmin(s *discordgo.Session, m *discordgo.MessageCreate) bool {
	perms, err := s.UserChannelPermissions(m.Author.ID, m.ChannelID)
	if err != nil {
		fmt.Println("Ошибка проверки прав:", err)
		return false
	}
	return perms&discordgo.PermissionAdministrator != 0
}

// Удаление всех ролей у всех пользователей
func removeAllRoles(s *discordgo.Session, m *discordgo.MessageCreate) {
	s.ChannelMessageSend(m.ChannelID, "⏳ Начинаю удаление всех ролей... Это может занять время")

	members, err := s.GuildMembers(GuildID, "", 1000)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "❌ Ошибка получения списка участников: "+err.Error())
		return
	}

	everyoneRoleID := ""
	roles, err := s.GuildRoles(GuildID)
	if err == nil {
		for _, role := range roles {
			if role.Name == "@everyone" {
				everyoneRoleID = role.ID
				break
			}
		}
	}

	successCount := 0
	failCount := 0

	for _, member := range members {
		// Пропускаем ботов
		if member.User.Bot {
			continue
		}

		// Оставляем только роль @everyone
		newRoles := []string{}
		if everyoneRoleID != "" {
			newRoles = append(newRoles, everyoneRoleID)
		}

		_, err := s.GuildMemberEdit(GuildID, member.User.ID, &discordgo.GuildMemberParams{
			Roles: &newRoles,
		})

		if err != nil {
			fmt.Printf("Ошибка удаления ролей у %s: %v\n", member.User.Username, err)
			failCount++
		} else {
			successCount++
		}

		// Задержка для предотвращения лимитов
		time.Sleep(200 * time.Millisecond)
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf(
		"✅ Удаление ролей завершено!\nУспешно: %d\nНе удалось: %d",
		successCount, failCount))
}

// Запуск регистрации для незарегистрированных
func startRegistrationForUnregistered(s *discordgo.Session, m *discordgo.MessageCreate) {
	registrationRoleID := findRoleID(s, GuildID, RegistrationRole)
	if registrationRoleID == "" {
		s.ChannelMessageSend(m.ChannelID, "❌ Роль 'Регистрация' не найдена")
		return
	}

	members, err := s.GuildMembers(GuildID, "", 1000)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "❌ Ошибка получения списка участников: "+err.Error())
		return
	}

	count := 0
	for _, member := range members {
		// Пропускаем ботов
		if member.User.Bot {
			continue
		}

		// Проверяем наличие роли регистрации
		hasRegistrationRole := false
		for _, role := range member.Roles {
			if role == registrationRoleID {
				hasRegistrationRole = true
				break
			}
		}

		// Пропускаем уже зарегистрированных
		if hasRegistrationRole {
			continue
		}

		// Проверяем, не в процессе ли уже регистрации
		mu.Lock()
		_, inProgress := registeringUsers[member.User.ID]
		mu.Unlock()

		if !inProgress {
			// Добавляем роль регистрации
			err := s.GuildMemberRoleAdd(GuildID, member.User.ID, registrationRoleID)
			if err != nil {
				fmt.Printf("Ошибка выдачи роли %s: %v\n", member.User.Username, err)
				continue
			}

			// Запускаем процесс регистрации
			go func(userID string) {
				// Даем время для выдачи роли
				time.Sleep(1 * time.Second)
				newGuildMember(s, &discordgo.GuildMemberAdd{
					Member: &discordgo.Member{
						GuildID: GuildID,
						User:    member.User,
					},
				})
			}(member.User.ID)

			count++
			time.Sleep(500 * time.Millisecond) // Задержка между запусками
		}
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf(
		"✅ Запущена регистрация для %d пользователей", count))
}

// Принудительное прерывание регистраций
func stopAllRegistrations(s *discordgo.Session, m *discordgo.MessageCreate) {
	mu.Lock()
	defer mu.Unlock()

	count := 0
	for userID, state := range registeringUsers {
		// Удаляем канал
		_, err := s.ChannelDelete(state.ChannelID)
		if err != nil {
			fmt.Printf("Ошибка удаления канала %s: %v\n", state.ChannelID, err)
		} else {
			count++
		}

		// Удаляем из списка регистрирующихся
		delete(registeringUsers, userID)
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf(
		"✅ Прервано %d регистрационных сессий", count))
}

// Отображение справки по командам
func showHelp(s *discordgo.Session, m *discordgo.MessageCreate) {
	helpMessage := `**📝 Доступные команды администратора:**

!clsRoles - Удаляет ВСЕ роли у ВСЕХ пользователей сервера
!startRegistred - Запускает регистрацию для пользователей без роли "Регистрация"
!stopRegistred - Принудительно прерывает ВСЕ активные регистрационные сессии
!help - Показывает это сообщение

**⚠️ Внимание:**
- Команды работают только в специальном канале для команд
- Требуют прав администратора
- Команда !clsRoles необратима и удаляет ВСЕ роли у ВСЕХ пользователей
- Прерванные регистрации (!stopRegistred) потребуют повторного запуска`

	s.ChannelMessageSend(m.ChannelID, helpMessage)
}
