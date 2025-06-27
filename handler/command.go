package handler

import (
	"strings"

	"github.com/bwmarrin/discordgo"
)

// Обработка команд администратора
func (sc *ServerConfig) handleAdminCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Проверка прав администратора
	if !IsAdmin(s, m) {
		logger.Warn("Попытка пользователя использовать команды")
		s.ChannelMessageSend(m.ChannelID, "У вас недостаточно прав для выполнения этой команды")
		return
	}
	switch strings.ToLower(m.Content) {
	case "!clsroles":
		logger.Info("Запуск команды !сlsRoles")
		sc.removeAllRoles(s, m)

	case "!startregistred":
		logger.Info("Запуск команды !startRegistred")
		sc.startRegistrationForUnregistered(s, m)

	case "!stopregistred":
		logger.Info("Запуск команды !stopRegistred")
		sc.stopAllRegistrations(s, m)

	case "!help":
		logger.Info("Запуск команды !help")
		sc.showHelp(s, m)

	case "!status":
		logger.Info("Запуск команды !status")
		sc.handleStatusCommand(s, m)

	default:
		s.ChannelMessageSend(m.ChannelID, "Неизвестная команда. Используй `!help` для списка команд")
	}
}
