package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

// Конфигурация сервера
type ServerConfig struct {
	GuildID          string `json:"server_id"`
	RegistrationRole string `json:"registration_role_id"`
	CategoryID       string `json:"category_id"`
	CommandChannelID string `json:"command_channel_id"`
	PreservedRoles   string `json:"preserved_roles"`
	GuildRoleId      string `json:"guild_role_id"`
	FriendRoleId     string `json:"friend_role_id"`
}

// Глобальные переменные
var (
	config            ServerConfig
	configFile        = "server_config.json"
	QuestionsFilePath = "question.json"
	questions         []Question
	registeringUsers  = make(map[string]*RegistrationState)
	mu                sync.Mutex
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

func main() {
	// Загружаем конфигурацию
	if err := loadConfig(); err != nil {
		fmt.Println("⚠️ Конфигурация не загружена. Используйте команду !init для настройки")
	}

	// Загружаем вопросы
	if err := loadQuestions(); err != nil {
		fmt.Println("Ошибка загрузки вопросов:", err)
		return
	}
	err := godotenv.Load()
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	session, err := discordgo.New("Bot " + os.Getenv("DISCORD_BOT_TOKEN"))
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

// Загрузка конфигурации
func loadConfig() error {
	file, err := os.ReadFile(configFile)
	if err != nil {
		return err
	}
	return json.Unmarshal(file, &config)
}

// Сохранение конфигурации
func saveConfig() error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		fmt.Println(err.Error())
		return err
	}
	return os.WriteFile(configFile, data, 0644)
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
	if m.GuildID != config.GuildID {
		return
	}

	// Выдаем роль регистрации
	roleID := findRoleID(s, m.GuildID, config.RegistrationRole)
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
func createPrivateChannel(s *discordgo.Session, member *discordgo.Member) (*discordgo.Channel, error) {
	channelName := "регистрация-" + strings.ToLower(member.User.Username)

	channelData := discordgo.GuildChannelCreateData{
		Name:     channelName,
		Type:     discordgo.ChannelTypeGuildText,
		ParentID: config.CategoryID,
		PermissionOverwrites: []*discordgo.PermissionOverwrite{
			{ID: member.User.ID, Type: discordgo.PermissionOverwriteTypeMember,
				Allow: discordgo.PermissionViewChannel | discordgo.PermissionSendMessages},
			{ID: s.State.User.ID, Type: discordgo.PermissionOverwriteTypeMember,
				Allow: discordgo.PermissionAll},
			{ID: config.GuildID, Type: discordgo.PermissionOverwriteTypeRole,
				Deny: discordgo.PermissionViewChannel},
		},
	}

	return s.GuildChannelCreateComplex(config.GuildID, channelData)
}

// Обработчик сообщений
func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Игнорируем сообщения ботов
	if m.Author.Bot {
		return
	}

	// Обработка команды !init
	if strings.HasPrefix(m.Content, "!init") {
		handleInitCommand(s, m)
		return
	}

	// Обработка команд администратора
	if m.ChannelID == config.CommandChannelID && strings.HasPrefix(m.Content, "!") {
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
	err := s.GuildMemberNickname(config.GuildID, userID, nickname)
	if err != nil {
		s.ChannelMessageSend(channelID, "⚠️ Ошибка при смене ника: "+err.Error())
	} else {
		s.ChannelMessageSend(channelID, "✅ Твой ник успешно изменен на: "+nickname)
	}

	// Удаляем роль регистрации
	_ = s.GuildMemberRoleRemove(config.GuildID, userID, config.RegistrationRole)

	// Формируем сводку
	summary := "🎉 Регистрация завершена!\n"
	for i, answer := range state.Answers {
		q := questions[i]
		if q.Switch != nil {
			if answer == "1" {
				err = s.GuildMemberRoleAdd(config.GuildID, userID, config.GuildRoleId)
				if err != nil {
					fmt.Println(err.Error())
				}
			} else if answer == "2" {
				err = s.GuildMemberRoleAdd(config.GuildID, userID, config.FriendRoleId)
				if err != nil {
					fmt.Println(err.Error())
				}
			}
		}
	}
	s.ChannelMessageSend(channelID, summary)

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

// Поиск ID роли по имени
func findRoleID(s *discordgo.Session, guildID, roleID string) string {
	roles, err := s.GuildRoles(guildID)
	if err != nil {
		fmt.Println("Ошибка получения ролей:", err)
		return ""
	}

	for _, role := range roles {
		if strings.EqualFold(role.ID, roleID) {
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

	case "!status":
		handleStatusCommand(s, m)

	default:
		s.ChannelMessageSend(m.ChannelID, "❌ Неизвестная команда. Используй `!help` для списка команд")
	}
}

// Обработка команды инициализации
func handleInitCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	args := strings.Fields(m.Content)
	if len(args) < 2 {
		showInitHelp(s, m.ChannelID)
		return
	}

	switch args[1] {
	case "guild":
		if len(args) < 3 {
			s.ChannelMessageSend(m.ChannelID, "❌ Укажите ID сервера: `!init guild <server_id>`")
			return
		}
		config.GuildID = args[2]

	case "role":
		if len(args) < 3 {
			s.ChannelMessageSend(m.ChannelID, "❌ Укажите ID роли регистрации: `!init role <role_id>`")
			return
		}
		config.RegistrationRole = args[2]

	case "category":
		if len(args) < 3 {
			s.ChannelMessageSend(m.ChannelID, "❌ Укажите ID категории: `!init category <category_id>`")
			return
		}
		config.CategoryID = args[2]

	case "channel":
		if len(args) < 3 {
			s.ChannelMessageSend(m.ChannelID, "❌ Укажите ID канала команд: `!init channel <channel_id>`")
			return
		}
		config.CommandChannelID = args[2]

	case "preserved":
		if len(args) < 3 {
			s.ChannelMessageSend(m.ChannelID, "❌ Укажите сохраняемые роли: `!init preserved Роль1,Роль2,...`")
			return
		}
		config.PreservedRoles = args[2]

	case "guild_role":
		if len(args) < 3 {
			s.ChannelMessageSend(m.ChannelID, "❌ Укажите ID роли согильдийца: `!init guild_role <role_id>`")
			return
		}
		config.GuildRoleId = args[2]

	case "friend_role":
		if len(args) < 3 {
			s.ChannelMessageSend(m.ChannelID, "❌ Укажите ID роли друга: `!init friend_role <role_id>`")
			return
		}
		config.RegistrationRole = args[2]

	case "load":
		if len(m.Attachments) == 0 {
			s.ChannelMessageSend(m.ChannelID, "❌ Прикрепите JSON-файл с конфигурацией")
			return
		}

		attachment := m.Attachments[0]
		if !strings.HasSuffix(attachment.Filename, ".json") {
			s.ChannelMessageSend(m.ChannelID, "❌ Файл должен быть в формате JSON")
			return
		}

		// Скачиваем файл
		resp, err := http.Get(attachment.URL)
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "❌ Ошибка загрузки файла: "+err.Error())
			return
		}
		defer resp.Body.Close()

		// Читаем содержимое
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "❌ Ошибка чтения файла: "+err.Error())
			return
		}

		// Парсим JSON
		var newConfig ServerConfig
		if err := json.Unmarshal(data, &newConfig); err != nil {
			s.ChannelMessageSend(m.ChannelID, "❌ Ошибка парсинга JSON: "+err.Error())
			return
		}

		// Применяем конфигурацию
		config = newConfig
		s.ChannelMessageSend(m.ChannelID, "✅ Конфигурация загружена из файла!")
		showCurrentConfig(s, m.ChannelID)

	case "save":
		if err := saveConfig(); err != nil {
			s.ChannelMessageSend(m.ChannelID, "❌ Ошибка сохранения: "+err.Error())
		} else {
			s.ChannelMessageSend(m.ChannelID, "✅ Конфигурация сохранена!")
		}
		if err := loadConfig(); err != nil {
			s.ChannelMessageSend(m.ChannelID, "Ошибка применения настроек")
		}
		return

	case "show":
		showCurrentConfig(s, m.ChannelID)
		return

	default:
		showInitHelp(s, m.ChannelID)
		return
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf(
		"✅ Параметр '%s' обновлен. Не забудьте сохранить командой `!init save`", args[1]))
}

// Показать справку по команде !init
func showInitHelp(s *discordgo.Session, channelID string) {
	help := `**⚙️ Команда настройки сервера:**
!init guild <server_id> - Установить ID сервера
!init role <role_id> - Установить ID роли регистрации
!init category <category_id> - Установить ID категории для каналов
!init channel <channel_id> - Установить ID канала для команд
!init preserved <roles_id> - Установить сохраняемые роли (через запятую)
!init guild_role <role_id> - Установка роли для согильдийцев
!init friend_role <role_id> - Установка роли для друзей
!init load <json file> - Конфигурация через файл
!init save - Сохранить конфигурацию
!init show - Показать текущую конфигурацию

**ℹ️ Как получить ID:**
1. Включите режим разработчика в Discord (Настройки > Расширенные)
2. ПКМ на элементе сервера/роли/канала > Копировать ID`

	s.ChannelMessageSend(channelID, help)
}

// Показать текущую конфигурацию
func showCurrentConfig(s *discordgo.Session, channelID string) {
	if _, err := os.Stat(configFile); err == os.ErrNotExist {
		response := "**Конфигурация не задана!**"
		s.ChannelMessageSend(channelID, response)
		return
	}
	response := "**Текущая конфигурация:**\n"
	response += fmt.Sprintf("Сервер (GuildID): ` %s `\n", config.GuildID)
	response += fmt.Sprintf("Роль регистрации: <@&%s>\n", config.RegistrationRole)
	response += fmt.Sprintf("Категория каналов: ` %s `\n", config.CategoryID)
	response += fmt.Sprintf("Канал команд: ` %s `\n", config.CommandChannelID)
	var preservedRoles string
	for _, v := range strings.Split(config.PreservedRoles, ",") {
		preservedRoles += "<@&" + v + ">, "
	}
	response += fmt.Sprintf("Сохраняемые роли: %s\n", preservedRoles)
	response += fmt.Sprintf("Роль Согильдийца: <@&%s>\n", config.GuildRoleId)
	response += fmt.Sprintf("Роль друга: <@&%s>\n", config.FriendRoleId)
	response += "\nИспользуйте `!init save` для сохранения изменений"

	s.ChannelMessageSend(channelID, response)
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

	members, err := s.GuildMembers(config.GuildID, "", 1000)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "❌ Ошибка получения списка участников: "+err.Error())
		return
	}

	everyoneRoleID := ""
	roles, err := s.GuildRoles(config.GuildID)
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

		_, err := s.GuildMemberEdit(config.GuildID, member.User.ID, &discordgo.GuildMemberParams{
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
	registrationRoleID := findRoleID(s, config.GuildID, config.RegistrationRole)
	if registrationRoleID == "" {
		s.ChannelMessageSend(m.ChannelID, "❌ Роль 'Регистрация' не найдена")
		return
	}

	members, err := s.GuildMembers(config.GuildID, "", 1000)
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
			err := s.GuildMemberRoleAdd(config.GuildID, member.User.ID, registrationRoleID)
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
						GuildID: config.GuildID,
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

func handleStatusCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	mu.Lock()
	activeSessions := len(registeringUsers)
	mu.Unlock()

	// Получаем статистику сервера
	guild, _ := s.Guild(config.GuildID)

	response := fmt.Sprintf("**🤖 Статус бота:**\nВерсия: 1.0.0\nПинг: %dms\nАктивных сессий: %d\n\n**📊 Статистика сервера:**\nГильдия: %s\nВсего участников: %d\nРолей: %d\n",
		s.HeartbeatLatency().Milliseconds(),
		activeSessions,
		guild.Name,
		len(guild.Members),
		len(guild.Roles))

	s.ChannelMessageSend(m.ChannelID, response)
}
