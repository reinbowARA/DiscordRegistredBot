package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// Обработка команды инициализации
func (sc *ServerConfig) handleInitCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Проверка прав администратора
	if !IsAdmin(s, m) {
		logger.Warn("Попытка пользователя использовать команды")
		s.ChannelMessageSend(m.ChannelID, "У вас недостаточно прав для выполнения этой команды")
		return
	}
	
	args := strings.Fields(m.Content)
	if len(args) < 2 {
		showInitHelp(s, m.ChannelID)
		return
	}

	switch args[1] {
	case "guild":
		logger.Info("Запуск команды !init guild")
		if len(args) < 3 {
			s.ChannelMessageSend(m.ChannelID, "Укажите ID сервера: `!init guild <server_id>`")
			return
		}
		sc.GuildID = args[2]

	case "role":
		logger.Info("Запуск команды !init role")
		if len(args) < 3 {
			s.ChannelMessageSend(m.ChannelID, "Укажите ID роли регистрации: `!init role <role_id>`")
			return
		}
		sc.RegistrationRole = args[2]

	case "category":
		logger.Info("Запуск команды !init category")
		if len(args) < 3 {
			s.ChannelMessageSend(m.ChannelID, "Укажите ID категории: `!init category <category_id>`")
			return
		}
		sc.CategoryID = args[2]

	case "channel":
		logger.Info("Запуск команды !init channel")
		if len(args) < 3 {
			s.ChannelMessageSend(m.ChannelID, "Укажите ID канала команд: `!init channel <channel_id>`")
			return
		}
		sc.CommandChannelID = args[2]

	case "guild_role":
		logger.Info("Запуск команды !init guild_role")
		if len(args) < 3 {
			s.ChannelMessageSend(m.ChannelID, "Укажите ID роли согильдийца: `!init guild_role <role_id>`")
			return
		}
		sc.GuildRoleId = args[2]

	case "friend_role":
		logger.Info("Запуск команды !init friend_role")
		if len(args) < 3 {
			s.ChannelMessageSend(m.ChannelID, "Укажите ID роли друга: `!init friend_role <role_id>`")
			return
		}
		sc.FriendRoleId = args[2]

	case "load":
		logger.Info("Запуск команды !init load")
		if len(m.Attachments) == 0 {
			s.ChannelMessageSend(m.ChannelID, "Прикрепите JSON-файл с конфигурацией")
			return
		}

		attachment := m.Attachments[0]
		if !strings.HasSuffix(attachment.Filename, ".json") {
			s.ChannelMessageSend(m.ChannelID, "Файл должен быть в формате JSON")
			return
		}

		// Скачиваем файл
		resp, err := http.Get(attachment.URL)
		if err != nil {
			logger.Error("Ошибка загрузки файла: " + err.Error())
			s.ChannelMessageSend(m.ChannelID, "Ошибка загрузки файла: "+err.Error())
			return
		}
		defer resp.Body.Close()

		// Читаем содержимое
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			logger.Error("Ошибка чтения файла: " + err.Error())
			s.ChannelMessageSend(m.ChannelID, "Ошибка чтения файла: "+err.Error())
			return
		}

		// Парсим JSON
		if err := json.Unmarshal(data, sc); err != nil {
			logger.Error("Ошибка парсинга JSON: "+err.Error())
			s.ChannelMessageSend(m.ChannelID, "Ошибка парсинга JSON: "+err.Error())
			return
		}

		// Применяем конфигурацию
		logger.Info("Файл загружен")
		s.ChannelMessageSend(m.ChannelID, "Конфигурация загружена из файла!")
		sc.showCurrentConfig(s, m.ChannelID)

	case "show":
		logger.Info("Запуск команды !init show")
		sc.showCurrentConfig(s, m.ChannelID)
		return

	default:
		showInitHelp(s, m.ChannelID)
		return
	}

}

// Показать справку по команде !init
func showInitHelp(s *discordgo.Session, channelID string) {
	help := `**Команда настройки сервера:**
!init guild <server_id> - Установить ID сервера
!init role <role_id> - Установить ID роли регистрации
!init category <category_id> - Установить ID категории для каналов
!init channel <channel_id> - Установить ID канала для команд
!init preserved <roles_id> - Установить сохраняемые роли (через запятую)
!init guild_role <role_id> - Установка роли для согильдийцев
!init friend_role <role_id> - Установка роли для друзей
!init load <json file> - Конфигурация через файл
!init show - Показать текущую конфигурацию

**Как получить ID:**
1. Включите режим разработчика в Discord (Настройки > Расширенные)
2. ПКМ на элементе сервера/роли/канала > Копировать ID`

	s.ChannelMessageSend(channelID, help)
}

// Показать текущую конфигурацию
func (sc *ServerConfig) showCurrentConfig(s *discordgo.Session, channelID string) {
	if len(sc.GuildID) == 0 {
		response := "**Конфигурация не задана!**"
		s.ChannelMessageSend(channelID, response)
		return
	}
	response := "**Текущая конфигурация:**\n"
	response += fmt.Sprintf("Сервер (GuildID): ` %s `\n", sc.GuildID)
	response += fmt.Sprintf("Роль регистрации: <@&%s>\n", sc.RegistrationRole)
	response += fmt.Sprintf("Категория каналов: ` %s `\n", sc.CategoryID)
	response += fmt.Sprintf("Канал команд: ` %s `\n", sc.CommandChannelID)
	response += fmt.Sprintf("Роль Согильдийца: <@&%s>\n", sc.GuildRoleId)
	response += fmt.Sprintf("Роль друга: <@&%s>\n", sc.FriendRoleId)

	s.ChannelMessageSend(channelID, response)
}
