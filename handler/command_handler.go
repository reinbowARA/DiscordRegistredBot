package handler

import (
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// Удаление всех ролей у всех пользователей
func (sc *ServerConfig) removeAllRoles(s *discordgo.Session, m *discordgo.MessageCreate) {
	s.ChannelMessageSend(m.ChannelID, "Начинаю удаление всех ролей... Это может занять время")

	members, err := s.GuildMembers(sc.GuildID, "", 1000)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "Ошибка получения списка участников: "+err.Error())
		return
	}

	everyoneRoleID := ""
	roles, err := s.GuildRoles(sc.GuildID)
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

		_, err := s.GuildMemberEdit(sc.GuildID, member.User.ID, &discordgo.GuildMemberParams{
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
		"Удаление ролей завершено!\nУспешно: %d\nНе удалось: %d",
		successCount, failCount))
}

// Запуск регистрации для незарегистрированных
func (sc *ServerConfig) startRegistrationForUnregistered(s *discordgo.Session, m *discordgo.MessageCreate) {
	registrationRoleID := findRoleID(s, sc.GuildID, sc.RegistrationRole)
	if registrationRoleID == "" {
		s.ChannelMessageSend(m.ChannelID, "Роль 'Регистрация' не найдена")
		return
	}

	members, err := s.GuildMembers(sc.GuildID, "", 1000)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "Ошибка получения списка участников: "+err.Error())
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
			err := s.GuildMemberRoleAdd(sc.GuildID, member.User.ID, registrationRoleID)
			if err != nil {
				fmt.Printf("Ошибка выдачи роли %s: %v\n", member.User.Username, err)
				continue
			}

			// Запускаем процесс регистрации
			go func(userID string) {
				// Даем время для выдачи роли
				time.Sleep(1 * time.Second)
				sc.NewGuildMember(s, &discordgo.GuildMemberAdd{
					Member: &discordgo.Member{
						GuildID: sc.GuildID,
						User:    member.User,
					},
				})
			}(member.User.ID)

			count++
			time.Sleep(500 * time.Millisecond) // Задержка между запусками
		}
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf(
		"Запущена регистрация для %d пользователей", count))
}

// Принудительное прерывание регистраций
func (sc *ServerConfig) stopAllRegistrations(s *discordgo.Session, m *discordgo.MessageCreate) {
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
		"Прервано %d регистрационных сессий", count))
}

// Отображение справки по командам
func (sc *ServerConfig) showHelp(s *discordgo.Session, m *discordgo.MessageCreate) {
	helpMessage := `**Доступные команды администратора:**

!clsRoles - Удаляет ВСЕ роли у ВСЕХ пользователей сервера
!startRegistred [--all] [--user_id USER_ID] - Запускает регистрацию для пользователей без роли "Регистрация"
!stopRegistred [--all] [--user_id USER_ID] - Принудительно прерывает активные регистрационные сессии
!help - Показывает это сообщение

Флаги:
--all           - Применяется ко всем пользователям (по умолчанию)
--user_id ID    - Применяется к конкретному пользователю по ID

**Внимание:**
- Команды работают только в специальном канале для команд
- Требуют прав администратора
- Команда !clsRoles необратима и удаляет ВСЕ роли у ВСЕХ пользователей
- Прерванные регистрации (!stopRegistred) потребуют повторного запуска`

	s.ChannelMessageSend(m.ChannelID, helpMessage)
}

func (sc *ServerConfig) handleStatusCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	mu.Lock()
	activeSessions := len(registeringUsers)
	mu.Unlock()

	// Получаем статистику сервера
	guild, _ := s.Guild(sc.GuildID)

	response := fmt.Sprintf("**Статус бота:**\nВерсия: 1.0.0\nПинг: %dms\nАктивных сессий: %d\n\n**Статистика сервера:**\nГильдия: %s\nВсего участников: %d\nРолей: %d\n\n**Автор**: <@302859679929729024>",
		s.HeartbeatLatency().Milliseconds(),
		activeSessions,
		guild.Name,
		len(guild.Members),
		len(guild.Roles))

	s.ChannelMessageSend(m.ChannelID, response)
}

// Обработка команды startRegistration с флагами
func (sc *ServerConfig) handleStartRegistrationCommand(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	userID := ""

	// Парсим аргументы
	for i := 0; i < len(args); i++ {
		arg := strings.ToLower(args[i])
		switch arg {
		case "--all":
			// Флаг --all является поведением по умолчанию, когда не указан --user_id
			continue
		case "--user_id":
			if i+1 < len(args) {
				userID = args[i+1]
				i++ // Пропускаем следующий аргумент (значение user_id)
			}
		}
	}

	if userID != "" {
		// Запуск регистрации для конкретного пользователя
		sc.startRegistrationForUser(s, m, userID)
	} else {
		// Запуск регистрации для всех незарегистрированных (поведение по умолчанию)
		sc.startRegistrationForUnregistered(s, m)
	}
}

// Обработка команды stopRegistration с флагами
func (sc *ServerConfig) handleStopRegistrationCommand(s *discordgo.Session, m *discordgo.MessageCreate, args []string) {
	userID := ""

	// Парсим аргументы
	for i := 0; i < len(args); i++ {
		arg := strings.ToLower(args[i])
		switch arg {
		case "--all":
			// Флаг --all является поведением по умолчанию, когда не указан --user_id
			continue
		case "--user_id":
			if i+1 < len(args) {
				userID = args[i+1]
				i++ // Пропускаем следующий аргумент (значение user_id)
			}
		}
	}

	if userID != "" {
		// Остановка регистрации для конкретного пользователя
		sc.stopRegistrationForUser(s, m, userID)
	} else {
		// Остановка всех регистраций (поведение по умолчанию)
		sc.stopAllRegistrations(s, m)
	}
}

// Запуск регистрации для конкретного пользователя
func (sc *ServerConfig) startRegistrationForUser(s *discordgo.Session, m *discordgo.MessageCreate, userID string) {
	registrationRoleID := findRoleID(s, sc.GuildID, sc.RegistrationRole)
	if registrationRoleID == "" {
		s.ChannelMessageSend(m.ChannelID, "Роль 'Регистрация' не найдена")
		return
	}

	// Получаем информацию о пользователе
	member, err := s.GuildMember(sc.GuildID, userID)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "Пользователь не найден: "+err.Error())
		return
	}

	// Пропускаем ботов
	if member.User.Bot {
		s.ChannelMessageSend(m.ChannelID, "Боты не могут проходить регистрацию")
		return
	}

	// Проверяем наличие роли регистрации
	hasRegistrationRole := false
	for _, role := range member.Roles {
		if role == registrationRoleID {
			hasRegistrationRole = true
			break
		}
	}

	// Если пользователь уже имеет роль регистрации, пропускаем
	if hasRegistrationRole {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Пользователь <@%s> уже имеет роль регистрации", userID))
		return
	}

	// Проверяем, не в процессе ли уже регистрации
	mu.Lock()
	_, inProgress := registeringUsers[userID]
	mu.Unlock()

	if inProgress {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Пользователь <@%s> уже находится в процессе регистрации", userID))
		return
	}

	// Добавляем роль регистрации
	err = s.GuildMemberRoleAdd(sc.GuildID, userID, registrationRoleID)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Ошибка выдачи роли пользователю <@%s>: %v", userID, err))
		return
	}

	// Запускаем процесс регистрации
	go func() {
		// Даем время для выдачи роли
		time.Sleep(1 * time.Second)
		sc.NewGuildMember(s, &discordgo.GuildMemberAdd{
			Member: &discordgo.Member{
				GuildID: sc.GuildID,
				User:    member.User,
			},
		})
	}()

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Запущена регистрация для пользователя <@%s>", userID))
}

// Остановка регистрации для конкретного пользователя
func (sc *ServerConfig) stopRegistrationForUser(s *discordgo.Session, m *discordgo.MessageCreate, userID string) {
	mu.Lock()
	state, exists := registeringUsers[userID]
	if !exists {
		mu.Unlock()
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Пользователь <@%s> не находится в процессе регистрации", userID))
		return
	}

	// Удаляем канал
	_, err := s.ChannelDelete(state.ChannelID)
	mu.Unlock()

	if err != nil {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Ошибка удаления канала пользователя <@%s>: %v", userID, err))
	} else {
		// Удаляем из списка регистрирующихся
		mu.Lock()
		delete(registeringUsers, userID)
		mu.Unlock()
		
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Регистрация пользователя <@%s> прервана", userID))
	}
}
