package handler

import (
	"strings"

	"github.com/bwmarrin/discordgo"
)

// Обработка команд администратора
func (sc *ServerConfig)handleAdminCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Проверка прав администратора
	if !IsAdmin(s, m) {
		s.ChannelMessageSend(m.ChannelID, "❌ У вас недостаточно прав для выполнения этой команды")
		return
	}
	switch strings.ToLower(m.Content) {
	case "!clsroles":
		sc.removeAllRoles(s, m)

	case "!startregistred":
		sc.startRegistrationForUnregistered(s, m)

	case "!stopregistred":
		sc.stopAllRegistrations(s, m)

	case "!help":
		sc.showHelp(s, m)

	case "!status":
		sc.handleStatusCommand(s, m)

	default:
		s.ChannelMessageSend(m.ChannelID, "❌ Неизвестная команда. Используй `!help` для списка команд")
	}
}
