package handler

import (
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
)

// Удаление всех ролей у всех пользователей
func (sc *ServerConfig) removeAllRoles(s *discordgo.Session, m *discordgo.MessageCreate) {
	s.ChannelMessageSend(m.ChannelID, "⏳ Начинаю удаление всех ролей... Это может занять время")

	members, err := s.GuildMembers(sc.GuildID, "", 1000)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, "❌ Ошибка получения списка участников: "+err.Error())
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
		"✅ Удаление ролей завершено!\nУспешно: %d\nНе удалось: %d",
		successCount, failCount))
}

// Запуск регистрации для незарегистрированных
func (sc *ServerConfig) startRegistrationForUnregistered(s *discordgo.Session, m *discordgo.MessageCreate) {
	registrationRoleID := findRoleID(s, sc.GuildID, sc.RegistrationRole)
	if registrationRoleID == "" {
		s.ChannelMessageSend(m.ChannelID, "❌ Роль 'Регистрация' не найдена")
		return
	}

	members, err := s.GuildMembers(sc.GuildID, "", 1000)
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
		"✅ Запущена регистрация для %d пользователей", count))
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
		"✅ Прервано %d регистрационных сессий", count))
}

// Отображение справки по командам
func (sc *ServerConfig) showHelp(s *discordgo.Session, m *discordgo.MessageCreate) {
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

func (sc *ServerConfig) handleStatusCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	mu.Lock()
	activeSessions := len(registeringUsers)
	mu.Unlock()

	// Получаем статистику сервера
	guild, _ := s.Guild(sc.GuildID)

	response := fmt.Sprintf("**🤖 Статус бота:**\nВерсия: 1.0.0\nПинг: %dms\nАктивных сессий: %d\n\n**📊 Статистика сервера:**\nГильдия: %s\nВсего участников: %d\nРолей: %d\n\n**Автор**: <@302859679929729024>",
		s.HeartbeatLatency().Milliseconds(),
		activeSessions,
		guild.Name,
		len(guild.Members),
		len(guild.Roles))

	s.ChannelMessageSend(m.ChannelID, response)
}
