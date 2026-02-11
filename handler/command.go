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
	
	// Разбираем команду и аргументы
	args := strings.Fields(m.Content)
	if len(args) == 0 {
		return
	}
	
	command := strings.ToLower(args[0])
	
	switch command {
	case "!clsroles":
		logger.Info("Запуск команды !сlsRoles")
		sc.removeAllRoles(s, m)

	case "!startregistred":
		logger.Info("Запуск команды !startRegistred")
		sc.handleStartRegistrationCommand(s, m, args[1:])

	case "!stopregistred":
		logger.Info("Запуск команды !stopRegistred")
		sc.handleStopRegistrationCommand(s, m, args[1:])

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
